package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// printCap bounds a filtered print: beyond this the browser's print preview
// gets slow, and the user should narrow the filters instead.
const printCap = 300

// workOrderPrint renders one work order as a printable A4 record.
func (s *Server) workOrderPrint(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	rows, err := s.q.ListWorkOrdersForPrint(r.Context(), gen.ListWorkOrdersForPrintParams{ID: &id, Lim: 1})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(rows) == 0 {
		s.notFound(w, r)
		return
	}
	s.renderPrint(w, r, rows, 1, "/work-orders/"+idStr(id))
}

// workOrdersPrint renders every work order matching the list filters, one
// record per A4 page, as if each were printed and the PDFs combined.
func (s *Server) workOrdersPrint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := woFilter(r)
	total, err := s.q.CountWorkOrders(ctx, f)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows, err := s.q.ListWorkOrdersForPrint(ctx, gen.ListWorkOrdersForPrintParams{
		SiteID: f.SiteID, SystemID: f.SystemID, AssetGroupID: f.AssetGroupID, ContractorID: f.ContractorID,
		Type: f.Type, Status: f.Status, Priority: f.Priority, Q: f.Q, Lim: printCap,
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	q := r.URL.Query()
	q.Del("page")
	back := "/work-orders"
	if enc := q.Encode(); enc != "" {
		back += "?" + enc
	}
	s.renderPrint(w, r, rows, int(total), back)
}

func (s *Server) renderPrint(w http.ResponseWriter, r *http.Request, rows []gen.ListWorkOrdersForPrintRow, total int, back string) {
	ctx := r.Context()
	cards, err := s.loadPrintCards(ctx, rows)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := views.WOPrintData{
		Cards: cards, Total: total, Back: back, Printed: time.Now().In(dates.Riyadh),
		Scope: s.describeWOFilter(ctx, r.URL.Query()),
	}
	if u := appctx.User(ctx); u != nil {
		d.PrintedBy = u.Name
	}
	s.render(w, r, http.StatusOK, views.WorkOrdersPrint(d))
}

// loadPrintCards loads inspections, items and findings for the work orders in
// three batch queries and assembles one card per work order.
func (s *Server) loadPrintCards(ctx context.Context, rows []gen.ListWorkOrdersForPrintRow) ([]views.WOPrintCard, error) {
	ids := make([]int64, len(rows))
	for i, wo := range rows {
		ids[i] = wo.ID
	}
	ins, err := s.q.ListSubmittedInspectionsForWorkOrders(ctx, ids)
	if err != nil {
		return nil, err
	}
	inIDs := make([]int64, len(ins))
	for i, in := range ins {
		inIDs[i] = in.ID
	}
	items, err := s.q.ListItemsForPrint(ctx, inIDs)
	if err != nil {
		return nil, err
	}
	fs, err := s.q.ListFindingsForWorkOrders(ctx, ids)
	if err != nil {
		return nil, err
	}

	itemsBy := map[int64][]views.PrintItem{}
	for _, it := range items {
		pi := views.PrintItem{Code: it.Code, Label: it.Label, LabelEn: it.LabelEn, Weight: int(it.Weight), Critical: it.IsCritical, Note: it.Note}
		if it.Rating != nil {
			pi.Rating = domain.Rating(*it.Rating)
		}
		itemsBy[it.InspectionID] = append(itemsBy[it.InspectionID], pi)
	}
	// Rows arrive ordered by attempt, so the last one per stage is the latest.
	type key struct {
		wo    int64
		stage gen.InspectionStage
	}
	latest := map[key]*views.PrintInspection{}
	for _, in := range ins {
		k := key{in.WorkOrderID, in.Stage}
		pi := &views.PrintInspection{
			Stage: domain.Stage(in.Stage), Attempt: int(in.Attempt), Inspector: in.InspectorName,
			Date: in.InspectedOn, CriticalFailed: in.CriticalFailed, Notes: in.Notes, Items: itemsBy[in.ID],
		}
		if in.Score != nil {
			pi.Score = *in.Score
		}
		if in.Passed != nil {
			pi.Passed = *in.Passed
		}
		if prev := latest[k]; prev != nil {
			pi.History = append(prev.History, views.PrintAttempt{Attempt: prev.Attempt, Score: prev.Score, Passed: prev.Passed})
		}
		latest[k] = pi
	}
	findingsBy := map[int64][]gen.ListFindingsForWorkOrdersRow{}
	for _, f := range fs {
		findingsBy[f.WorkOrderID] = append(findingsBy[f.WorkOrderID], f)
	}

	cards := make([]views.WOPrintCard, len(rows))
	for i, wo := range rows {
		c := views.WOPrintCard{
			WO:       wo,
			During:   latest[key{wo.ID, gen.InspectionStageDuring}],
			After:    latest[key{wo.ID, gen.InspectionStageAfter}],
			Findings: findingsBy[wo.ID],
		}
		open := 0
		for _, f := range c.Findings {
			if f.Status != gen.FindingStatusClosed {
				open++
			}
		}
		c.OpenFindings = open
		in := domain.VerdictInput{Status: domain.WOStatus(wo.Status), OpenFindings: open}
		if c.After != nil {
			in.HasSubmittedAfter, in.LatestAfterPassed = true, c.After.Passed
		}
		c.Verdict = domain.WOVerdict(in)
		cards[i] = c
	}
	return cards, nil
}

// describeWOFilter turns the list filters into a readable line for the print toolbar.
func (s *Server) describeWOFilter(ctx context.Context, q url.Values) string {
	lk, err := s.lookupOptions(ctx, false)
	if err != nil {
		return ""
	}
	find := func(opts []views.Option, v string) string {
		for _, o := range opts {
			if o.Value == v {
				return o.Label
			}
		}
		return v
	}
	var parts []string
	add := func(label, v string) {
		if v != "" {
			parts = append(parts, label+": "+v)
		}
	}
	if v := q.Get("q"); v != "" {
		add(appctx.T(ctx, "بحث"), "«"+v+"»")
	}
	if v := q.Get("type"); v != "" {
		add(appctx.T(ctx, "النوع"), domain.Label(domain.WOTypeLabels, v))
	}
	if v := q.Get("status"); v != "" {
		add(appctx.T(ctx, "الحالة"), domain.WOStatus(v).Label())
	}
	if v := q.Get("priority"); v != "" {
		add(appctx.T(ctx, "الأولوية"), domain.Label(domain.PriorityLabels, v))
	}
	if v := q.Get("site"); v != "" {
		add(appctx.T(ctx, "الموقع"), find(lk.Sites, v))
	}
	if v := q.Get("system"); v != "" {
		add(appctx.T(ctx, "المنظومة"), find(lk.Systems, v))
	}
	if v := q.Get("group"); v != "" {
		add(appctx.T(ctx, "مجموعة الأصل"), find(lk.AssetGroups, v))
	}
	if v := q.Get("contractor"); v != "" {
		add(appctx.T(ctx, "المقاول"), find(lk.Contractors, v))
	}
	return strings.Join(parts, " · ")
}
