package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"github.com/HZamzami/kidana-qa/internal/dates"
	"github.com/HZamzami/kidana-qa/internal/db"
	"github.com/HZamzami/kidana-qa/internal/db/gen"
	"github.com/HZamzami/kidana-qa/internal/domain"
	"github.com/HZamzami/kidana-qa/internal/excel"
	"github.com/HZamzami/kidana-qa/internal/views"
	webassets "github.com/HZamzami/kidana-qa/web"
)

// describeScope turns the filters into a human-readable line for reports.
func (s *Server) describeScope(ctx context.Context, sc reportScope) string {
	var parts []string
	if sc.F.Stage != "" {
		parts = append(parts, "المرحلة: "+domain.Stage(sc.F.Stage).Label())
	}
	if sc.F.Type != "" {
		parts = append(parts, "النوع: "+domain.Label(domain.WOTypeLabels, sc.F.Type))
	}
	sites, systems, contractors, err := s.lookupOptions(ctx, false)
	if err == nil {
		find := func(opts []views.Option, v string) string {
			for _, o := range opts {
				if o.Value == v {
					return o.Label
				}
			}
			return v
		}
		if sc.F.Site != "" {
			parts = append(parts, "الموقع: "+find(sites, sc.F.Site))
		}
		if sc.F.System != "" {
			parts = append(parts, "النظام: "+find(systems, sc.F.System))
		}
		if sc.F.Contractor != "" {
			parts = append(parts, "المقاول: "+find(contractors, sc.F.Contractor))
		}
	}
	if sc.F.From != "" || sc.F.To != "" {
		parts = append(parts, fmt.Sprintf("الفترة: %s إلى %s", orEllipsis(sc.F.From), orEllipsis(sc.F.To)))
	}
	if len(parts) == 0 {
		return "جميع الفحوصات"
	}
	return strings.Join(parts, " · ")
}

func orEllipsis(s string) string {
	if s == "" {
		return "…"
	}
	return s
}

func (s *Server) reportPrint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sc := parseScope(r)
	rep, err := s.loadReport(ctx, sc)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	findings, err := s.q.ListOpenFindingsScoped(ctx, gen.ListOpenFindingsScopedParams{
		SiteID: sc.Params.SiteID, SystemID: sc.Params.SystemID, ContractorID: sc.Params.ContractorID, Type: sc.Params.Type,
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := views.ReportData{
		Generated: dates.Both(time.Now().In(dates.Riyadh)), Scope: s.describeScope(ctx, sc),
		S: rep.Summary, Insights: domain.Insights(rep.Summary),
		Findings: findings, Today: time.Now().In(dates.Riyadh).Format(time.DateOnly),
	}
	for _, row := range rep.Rows {
		rr := views.ReportRow{
			Ref: row.Ref, Stage: string(row.Stage), Attempt: int(row.Attempt), Date: dates.DMY(row.InspectedOn), Hijri: dates.Hijri(row.InspectedOn),
			Site: row.SiteName, System: row.SystemName, Contractor: row.ContractorName, Inspector: row.InspectorName,
		}
		if row.Score != nil {
			rr.Score = *row.Score
		}
		if row.Passed != nil {
			rr.Passed = *row.Passed
		}
		d.Rows = append(d.Rows, rr)
	}
	s.render(w, r, http.StatusOK, views.ReportPrint(d))
}

func (s *Server) exportExcel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sc := parseScope(r)
	rep, err := s.loadReport(ctx, sc)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	ratings := map[int64]map[string]domain.Rating{}
	critSeen := map[string]bool{}
	var crits []excel.CriterionColumn
	for _, it := range rep.Items {
		key := it.Code + "|" + it.Label
		if !critSeen[key] {
			critSeen[key] = true
			crits = append(crits, excel.CriterionColumn{Code: it.Code, Label: it.Label})
		}
		if it.Rating == nil {
			continue
		}
		if ratings[it.InspectionID] == nil {
			ratings[it.InspectionID] = map[string]domain.Rating{}
		}
		ratings[it.InspectionID][it.Code] = domain.Rating(*it.Rating)
	}
	excel.SortCriteria(crits)

	rows := make([]excel.InspectionExport, 0, len(rep.Rows))
	for _, row := range rep.Rows {
		e := excel.InspectionExport{
			Ref: row.Ref, Type: string(row.Type), Site: row.SiteName, System: row.SystemName, Asset: row.Asset,
			Location: row.Location, Contractor: row.ContractorName, Technician: row.Technician,
			Stage: string(row.Stage), Template: row.TemplateName, Inspector: row.InspectorName,
			Attempt: int(row.Attempt), Date: row.InspectedOn, CriticalFailed: row.CriticalFailed,
			Ratings: ratings[row.ID], Notes: row.Notes,
		}
		if row.Score != nil {
			e.Score = *row.Score
		}
		if row.Passed != nil {
			e.Passed = *row.Passed
		}
		if d, ok := domain.DelayDays(row.PlannedDate, row.ActualDate); ok {
			e.DelayDays = &d
		}
		rows = append(rows, e)
	}

	fs, err := s.q.ListAllFindingsForExport(ctx)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	findings := make([]excel.FindingExport, 0, len(fs))
	for _, f := range fs {
		crit := ""
		if f.CriterionCode != "" {
			crit = f.CriterionCode + " - " + f.CriterionLabel
		}
		findings = append(findings, excel.FindingExport{
			ID: f.ID, Ref: f.WoRef, Site: f.SiteName, Contractor: f.ContractorName, Criterion: crit,
			Severity: string(f.Severity), Status: string(f.Status), Description: f.Description,
			CorrectiveAction: f.CorrectiveAction, Owner: f.Owner, ClosureNote: f.ClosureNote,
			DueDate: f.DueDate, CreatedAt: f.CreatedAt, ClosedAt: f.ClosedAt,
		})
	}

	checklists, err := s.activeChecklists(ctx)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	logo, _ := webassets.Static.ReadFile("static/img/kidana-logo.png")
	book, err := excel.Report(excel.ReportInput{
		Scope: s.describeScope(ctx, sc), Summary: rep.Summary, Insights: domain.Insights(rep.Summary),
		Rows: rows, Criteria: crits, Findings: findings, Checklists: checklists, Logo: logo,
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.sendWorkbook(w, r, book, "Kidana-QA-Report_"+time.Now().Format(time.DateOnly)+".xlsx")
}

// activeChecklists returns the active QA and QC checklists for the criteria sheet.
func (s *Server) activeChecklists(ctx context.Context) ([]excel.ChecklistExport, error) {
	var out []excel.ChecklistExport
	for _, stage := range []gen.InspectionStage{gen.InspectionStageQa, gen.InspectionStageQc} {
		t, err := s.q.GetActiveTemplate(ctx, stage)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		crits, err := s.q.ListCriteria(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		cl := excel.ChecklistExport{Stage: string(t.Stage), Name: t.Name, Version: int(t.Version)}
		for _, c := range crits {
			cl.Criteria = append(cl.Criteria, excel.CriterionExport{
				Code: c.Code, Label: c.Label, Description: c.Description, Weight: int(c.Weight), Critical: c.IsCritical,
			})
		}
		out = append(out, cl)
	}
	return out, nil
}

func (s *Server) sendWorkbook(w http.ResponseWriter, r *http.Request, f *excelize.File, name string) {
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if err := f.Write(w); err != nil {
		s.log.Error("write workbook", "err", err, "path", r.URL.Path)
	}
}

func (s *Server) dataPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, views.DataPage(nil))
}

func (s *Server) workOrderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	names := func(opts []views.Option) []string {
		out := make([]string, len(opts))
		for i, o := range opts {
			out[i] = o.Label
		}
		return out
	}
	sites, systems, contractors, err := s.lookupOptions(ctx, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	book, err := excel.WorkOrderTemplate(names(sites), names(systems), names(contractors))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.sendWorkbook(w, r, book, "Kidana-QA-WorkOrders-Template.xlsx")
}

const maxImportBytes = 15 << 20

func (s *Server) importExcel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		s.render(w, r, http.StatusBadRequest, views.DataPage(&views.ImportOutcome{Error: "الملف كبير جداً (الحد 15MB) أو غير صالح."}))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, hdr, err := r.FormFile("file")
	if err != nil {
		s.render(w, r, http.StatusBadRequest, views.DataPage(&views.ImportOutcome{Error: "اختر ملف Excel بصيغة \u200e.xlsx"}))
		return
	}
	defer func() { _ = file.Close() }()
	if !strings.HasSuffix(strings.ToLower(hdr.Filename), ".xlsx") {
		s.render(w, r, http.StatusBadRequest, views.DataPage(&views.ImportOutcome{Error: "الملف يجب أن يكون بصيغة \u200e.xlsx"}))
		return
	}

	res, err := excel.Parse(file)
	if err != nil {
		msg := "تعذّرت قراءة الملف. تأكد أنه ملف Excel صالح."
		if errors.Is(err, excel.ErrUnrecognised) {
			msg = "لم يتم التعرف على أعمدة الملف. استخدم قالب أوامر العمل."
		}
		s.render(w, r, http.StatusUnprocessableEntity, views.DataPage(&views.ImportOutcome{Error: msg}))
		return
	}

	out := &views.ImportOutcome{FileName: hdr.Filename, Skipped: res.Skipped, Warnings: res.Warnings}
	out.KindLabel = "أوامر العمل"
	if err := s.importWorkOrders(ctx, res, out); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "import", 0, "excel_import", map[string]any{"file": hdr.Filename, "kind": out.KindLabel, "created": out.Created})
	s.render(w, r, http.StatusOK, views.DataPage(out))
}

// lookupResolver finds or creates sites/systems/contractors by name during an import.
type lookupResolver struct {
	q       *gen.Queries
	ctx     context.Context
	cache   map[string]int64
	created []string
}

func (lr *lookupResolver) id(kind, name string) (int64, error) {
	key := kind + "|" + excel.Norm(name)
	if id, ok := lr.cache[key]; ok {
		return id, nil
	}
	var id int64
	var err error
	switch kind {
	case "site":
		var x gen.Site
		if x, err = lr.q.GetSiteByName(lr.ctx, name); errors.Is(err, pgx.ErrNoRows) {
			x, err = lr.q.CreateSite(lr.ctx, gen.CreateSiteParams{Name: name, Sort: 100})
			lr.created = append(lr.created, "موقع: "+name)
		}
		id = x.ID
	case "system":
		var x gen.System
		if x, err = lr.q.GetSystemByName(lr.ctx, name); errors.Is(err, pgx.ErrNoRows) {
			x, err = lr.q.CreateSystem(lr.ctx, gen.CreateSystemParams{Name: name, Sort: 100})
			lr.created = append(lr.created, "نظام: "+name)
		}
		id = x.ID
	case "contractor":
		var x gen.Contractor
		if x, err = lr.q.GetContractorByName(lr.ctx, name); errors.Is(err, pgx.ErrNoRows) {
			x, err = lr.q.CreateContractor(lr.ctx, gen.CreateContractorParams{Name: name})
			lr.created = append(lr.created, "مقاول: "+name)
		}
		id = x.ID
	}
	if err != nil {
		return 0, err
	}
	lr.cache[key] = id
	return id, nil
}

func (lr *lookupResolver) optional(kind, name string) (*int64, error) {
	if name == "" {
		return nil, nil
	}
	id, err := lr.id(kind, name)
	return &id, err
}

func priorityPtr(p string) *gen.WoPriority {
	if p == "" {
		return nil
	}
	v := gen.WoPriority(p)
	return &v
}

// importWorkOrders creates work orders; existing refs are skipped. One transaction.
func (s *Server) importWorkOrders(ctx context.Context, res *excel.Result, out *views.ImportOutcome) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		lr := &lookupResolver{q: q, ctx: ctx, cache: map[string]int64{}}
		for _, row := range res.WorkOrders {
			exists, err := q.GetWorkOrderRefExists(ctx, gen.GetWorkOrderRefExistsParams{Ref: row.Ref, ExcludeID: 0})
			if err != nil {
				return err
			}
			if exists {
				out.Skipped++
				out.Warnings = append(out.Warnings, fmt.Sprintf("الصف %d: أمر العمل %s موجود مسبقاً — تم تجاهله", row.Row, row.Ref))
				continue
			}
			siteID, err := lr.id("site", row.Site)
			if err != nil {
				return err
			}
			systemID, err := lr.id("system", row.System)
			if err != nil {
				return err
			}
			contractorID, err := lr.optional("contractor", row.Contractor)
			if err != nil {
				return err
			}
			if _, err := q.CreateWorkOrder(ctx, gen.CreateWorkOrderParams{
				Ref: row.Ref, Type: gen.WoType(row.Type), SiteID: siteID, Location: row.Location, SystemID: systemID,
				Asset: row.Asset, ContractorID: contractorID, Technician: row.Technician, Priority: priorityPtr(row.Priority),
				Description: row.Description, PlannedDate: row.PlannedDate, ActualDate: row.ActualDate, CreatedBy: currentUserID(ctx),
			}); err != nil {
				return err
			}
			out.Created++
		}
		if len(lr.created) > 0 {
			out.Extra = append(out.Extra, "أُضيفت قيم مرجعية جديدة: "+strings.Join(lr.created, "، "))
		}
		return nil
	})
}
