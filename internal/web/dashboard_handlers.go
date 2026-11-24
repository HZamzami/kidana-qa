package web

import (
	"context"
	"net/http"
	"net/url"

	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// reportScope is the filter set shared by the dashboard, printed report and Excel export.
type reportScope struct {
	F      views.DashFilters
	Params gen.ListSubmittedInspectionsParams
}

func parseScope(r *http.Request) reportScope {
	q := r.URL.Query()
	sc := reportScope{F: views.DashFilters{
		Stage: q.Get("stage"), Type: q.Get("type"), Site: q.Get("site"), System: q.Get("system"),
		Contractor: q.Get("contractor"), From: q.Get("from"), To: q.Get("to"), Result: q.Get("result"),
	}}
	p := gen.ListSubmittedInspectionsParams{
		SiteID: queryID(r, "site"), SystemID: queryID(r, "system"), ContractorID: queryID(r, "contractor"),
		DateFrom: queryDate(r, "from"), DateTo: queryDate(r, "to"),
	}
	if st := gen.InspectionStage(sc.F.Stage); st.Valid() {
		p.Stage = &st
	} else {
		sc.F.Stage = ""
	}
	switch sc.F.Result {
	case "pass", "fail":
		passed := sc.F.Result == "pass"
		p.Passed = &passed
	default:
		sc.F.Result = ""
	}
	if t := gen.WoType(sc.F.Type); t.Valid() {
		p.Type = &t
	} else {
		sc.F.Type = ""
	}
	sc.Params = p
	return sc
}

func (sc reportScope) query() string {
	v := url.Values{}
	for k, val := range map[string]string{
		"stage": sc.F.Stage, "type": sc.F.Type, "site": sc.F.Site, "system": sc.F.System,
		"contractor": sc.F.Contractor, "from": sc.F.From, "to": sc.F.To, "result": sc.F.Result,
	} {
		if val != "" {
			v.Set(k, val)
		}
	}
	return v.Encode()
}

// reportData loads and aggregates the submitted inspections in scope.
type reportData struct {
	Rows    []gen.ListSubmittedInspectionsRow
	Items   []gen.ListItemsForInspectionsRow
	Summary domain.Summary
}

func (s *Server) loadReport(ctx context.Context, sc reportScope) (reportData, error) {
	var d reportData
	rows, err := s.q.ListSubmittedInspections(ctx, sc.Params)
	if err != nil {
		return d, err
	}
	ids := make([]int64, len(rows))
	inRows := make([]domain.InspectionRow, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
		ir := domain.InspectionRow{
			ID: r.ID, WorkOrderID: r.WorkOrderID, Stage: domain.Stage(r.Stage), CriticalFailed: r.CriticalFailed,
			Type: string(r.Type), Site: r.SiteName, System: r.SystemName, Contractor: r.ContractorName, Asset: r.Asset,
		}
		if r.Score != nil {
			ir.Score = *r.Score
		}
		if r.Passed != nil {
			ir.Passed = *r.Passed
		}
		if days, ok := domain.DelayDays(r.PlannedDate, r.ActualDate); ok {
			ir.DelayDays = &days
		}
		inRows[i] = ir
	}
	items, err := s.q.ListItemsForInspections(ctx, ids)
	if err != nil {
		return d, err
	}
	itRows := make([]domain.ItemRow, 0, len(items))
	for _, it := range items {
		if it.Rating == nil {
			continue
		}
		itRows = append(itRows, domain.ItemRow{
			InspectionID: it.InspectionID, Stage: domain.Stage(it.Stage), Code: it.Code, Label: it.Label,
			Rating: domain.Rating(*it.Rating), Critical: it.IsCritical,
		})
	}
	fs, err := s.q.FindingStats(ctx, gen.FindingStatsParams{
		SiteID: sc.Params.SiteID, SystemID: sc.Params.SystemID, ContractorID: sc.Params.ContractorID, Type: sc.Params.Type,
	})
	if err != nil {
		return d, err
	}
	d.Rows, d.Items = rows, items
	d.Summary = domain.Summarize(inRows, itRows, domain.FindingStats{
		Open: int(fs.OpenCount), Overdue: int(fs.OverdueCount), OpenCritical: int(fs.OpenCritical),
	})
	return d, nil
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sc := parseScope(r)
	rep, err := s.loadReport(ctx, sc)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	sites, systems, contractors, err := s.lookupOptions(ctx, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.Dashboard(views.DashboardData{
		S: rep.Summary, Insights: domain.Insights(rep.Summary), F: sc.F, Query: sc.query(),
		Sites: sites, Systems: systems, Contractors: contractors,
	}))
}
