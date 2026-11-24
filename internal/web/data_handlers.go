package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/i18n"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/excel"
	"github.com/HZamzami/kidana-quality/internal/views"
	webassets "github.com/HZamzami/kidana-quality/web"
)

// describeScope turns the filters into a human-readable line for reports.
func (s *Server) describeScope(ctx context.Context, sc reportScope) string {
	var parts []string
	if sc.F.Stage != "" {
		parts = append(parts, appctx.T(ctx, "المرحلة: ")+appctx.T(ctx, domain.Stage(sc.F.Stage).Label()))
	}
	if sc.F.Type != "" {
		parts = append(parts, appctx.T(ctx, "النوع: ")+appctx.T(ctx, domain.Label(domain.WOTypeLabels, sc.F.Type)))
	}
	lk, err := s.lookupOptions(ctx, false)
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
			parts = append(parts, appctx.T(ctx, "الموقع: ")+find(lk.Sites, sc.F.Site))
		}
		if sc.F.System != "" {
			parts = append(parts, appctx.T(ctx, "المنظومة: ")+find(lk.Systems, sc.F.System))
		}
		if sc.F.AssetGroup != "" {
			parts = append(parts, appctx.T(ctx, "مجموعة الأصل: ")+find(lk.AssetGroups, sc.F.AssetGroup))
		}
		if sc.F.Contractor != "" {
			parts = append(parts, appctx.T(ctx, "المقاول: ")+find(lk.Contractors, sc.F.Contractor))
		}
	}
	switch sc.F.Result {
	case "pass":
		parts = append(parts, appctx.T(ctx, "النتيجة: مطابق"))
	case "fail":
		parts = append(parts, appctx.T(ctx, "النتيجة: غير مطابق"))
	}
	if sc.F.From != "" || sc.F.To != "" {
		parts = append(parts, appctx.Tf(ctx, "الفترة: %s إلى %s", orEllipsis(sc.F.From), orEllipsis(sc.F.To)))
	}
	if len(parts) == 0 {
		return appctx.T(ctx, "جميع الفحوصات")
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
		SiteID: sc.Params.SiteID, AssetGroupID: sc.Params.AssetGroupID, ContractorID: sc.Params.ContractorID, Type: sc.Params.Type,
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := views.ReportData{
		Generated: dates.BothIn(appctx.Lang(ctx), time.Now().In(dates.Riyadh)), Scope: s.describeScope(ctx, sc),
		S: rep.Summary, Insights: domain.InsightsIn(appctx.Lang(ctx), rep.Summary),
		Findings: findings, Today: time.Now().In(dates.Riyadh).Format(time.DateOnly),
	}
	for _, row := range rep.Rows {
		rr := views.ReportRow{
			Ref: row.Ref, Stage: string(row.Stage), Attempt: int(row.Attempt), Date: dates.DMY(row.InspectedOn), Hijri: dates.HijriIn(appctx.Lang(ctx), row.InspectedOn),
			Site: i18n.Pick(appctx.Lang(ctx), row.SiteName, row.SiteNameEn), System: i18n.Pick(appctx.Lang(ctx), row.SystemName, row.SystemNameEn), AssetGroup: i18n.Pick(appctx.Lang(ctx), row.AssetGroupName, row.AssetGroupNameEn), Contractor: i18n.Pick(appctx.Lang(ctx), row.ContractorName, row.ContractorNameEn), Inspector: row.InspectorName,
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
			Ref: row.Ref, Type: string(row.Type), Site: i18n.Pick(appctx.Lang(ctx), row.SiteName, row.SiteNameEn), System: i18n.Pick(appctx.Lang(ctx), row.SystemName, row.SystemNameEn), AssetGroup: i18n.Pick(appctx.Lang(ctx), row.AssetGroupName, row.AssetGroupNameEn), Asset: row.Asset,
			Location: row.Location, Contractor: i18n.Pick(appctx.Lang(ctx), row.ContractorName, row.ContractorNameEn), Technician: row.Technician,
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
			crit = f.CriterionCode + " - " + i18n.Pick(appctx.Lang(ctx), f.CriterionLabel, f.CriterionLabelEn)
		}
		findings = append(findings, excel.FindingExport{
			ID: f.ID, Ref: f.WoRef, Site: i18n.Pick(appctx.Lang(ctx), f.SiteName, f.SiteNameEn), Contractor: i18n.Pick(appctx.Lang(ctx), f.ContractorName, f.ContractorNameEn), Criterion: crit,
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
		Scope: s.describeScope(ctx, sc), Summary: rep.Summary, Insights: domain.InsightsIn(appctx.Lang(ctx), rep.Summary),
		Rows: rows, Criteria: crits, Findings: findings, Checklists: checklists, Logo: logo, Lang: appctx.Lang(ctx),
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.sendWorkbook(w, r, book, "Kidana-Quality-Report_"+time.Now().Format(time.DateOnly)+".xlsx")
}

// activeChecklists returns the active during and after checklists for the criteria sheet.
func (s *Server) activeChecklists(ctx context.Context) ([]excel.ChecklistExport, error) {
	var out []excel.ChecklistExport
	for _, stage := range []gen.InspectionStage{gen.InspectionStageDuring, gen.InspectionStageAfter} {
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
	lk, err := s.lookupOptions(ctx, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	book, err := excel.WorkOrderTemplate(appctx.Lang(ctx), names(lk.Sites), names(lk.Systems), names(lk.AssetGroups), names(lk.Contractors))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.sendWorkbook(w, r, book, "Kidana-Quality-WorkOrders-Template.xlsx")
}

const maxImportBytes = 15 << 20

func (s *Server) importExcel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		s.render(w, r, http.StatusBadRequest, views.DataPage(&views.ImportOutcome{Error: appctx.T(ctx, "الملف كبير جداً (الحد 15MB) أو غير صالح.")}))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, hdr, err := r.FormFile("file")
	if err != nil {
		s.render(w, r, http.StatusBadRequest, views.DataPage(&views.ImportOutcome{Error: appctx.T(ctx, "اختر ملف Excel بصيغة \u200e.xlsx")}))
		return
	}
	defer func() { _ = file.Close() }()
	if !strings.HasSuffix(strings.ToLower(hdr.Filename), ".xlsx") {
		s.render(w, r, http.StatusBadRequest, views.DataPage(&views.ImportOutcome{Error: appctx.T(ctx, "الملف يجب أن يكون بصيغة \u200e.xlsx")}))
		return
	}

	res, err := excel.Parse(file, appctx.Lang(ctx))
	if err != nil {
		msg := appctx.T(ctx, "تعذّرت قراءة الملف. تأكد أنه ملف Excel صالح.")
		if errors.Is(err, excel.ErrUnrecognised) {
			msg = appctx.T(ctx, "لم يتم التعرف على أعمدة الملف. استخدم قالب أوامر العمل.")
		}
		s.render(w, r, http.StatusUnprocessableEntity, views.DataPage(&views.ImportOutcome{Error: msg}))
		return
	}

	out := &views.ImportOutcome{FileName: hdr.Filename, Skipped: res.Skipped, Warnings: res.Warnings}
	out.KindLabel = appctx.T(ctx, "أوامر العمل")
	if err := s.importWorkOrders(ctx, res, out); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "import", 0, "excel_import", map[string]any{"file": hdr.Filename, "kind": out.KindLabel, "created": out.Created})
	s.render(w, r, http.StatusOK, views.DataPage(out))
}

// lookupResolver finds or creates sites/systems/asset groups/contractors by name during an import.
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
			lr.created = append(lr.created, appctx.T(lr.ctx, "موقع: ")+name)
		}
		id = x.ID
	case "system":
		var x gen.System
		if x, err = lr.q.GetSystemByName(lr.ctx, name); errors.Is(err, pgx.ErrNoRows) {
			x, err = lr.q.CreateSystem(lr.ctx, gen.CreateSystemParams{Name: name, Sort: 100})
			lr.created = append(lr.created, appctx.T(lr.ctx, "منظومة: ")+name)
		}
		id = x.ID
	case "asset_group":
		var x gen.AssetGroup
		if x, err = lr.q.GetAssetGroupByName(lr.ctx, name); errors.Is(err, pgx.ErrNoRows) {
			x, err = lr.q.CreateAssetGroup(lr.ctx, gen.CreateAssetGroupParams{Name: name, Sort: 100})
			lr.created = append(lr.created, appctx.T(lr.ctx, "مجموعة أصل: ")+name)
		}
		id = x.ID
	case "contractor":
		var x gen.Contractor
		if x, err = lr.q.GetContractorByName(lr.ctx, name); errors.Is(err, pgx.ErrNoRows) {
			x, err = lr.q.CreateContractor(lr.ctx, gen.CreateContractorParams{Name: name})
			lr.created = append(lr.created, appctx.T(lr.ctx, "مقاول: ")+name)
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
				out.Warnings = append(out.Warnings, appctx.Tf(ctx, "الصف %d: أمر العمل %s موجود مسبقاً — تم تجاهله", row.Row, row.Ref))
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
			assetGroupID, err := lr.id("asset_group", row.AssetGroup)
			if err != nil {
				return err
			}
			contractorID, err := lr.optional("contractor", row.Contractor)
			if err != nil {
				return err
			}
			wo, err := q.CreateWorkOrder(ctx, gen.CreateWorkOrderParams{
				Ref: row.Ref, Type: gen.WoType(row.Type), SiteID: siteID, Location: row.Location, SystemID: systemID, AssetGroupID: assetGroupID,
				Asset: row.Asset, ContractorID: contractorID, Technician: row.Technician, Priority: gen.WoPriority(row.Priority),
				Description: row.Description, PlannedDate: row.PlannedDate, ActualDate: row.ActualDate, CreatedBy: currentUserID(ctx),
			})
			if err != nil {
				return err
			}
			if err := s.sampleNewWorkOrder(ctx, q, wo); err != nil {
				return err
			}
			out.Created++
		}
		if len(lr.created) > 0 {
			out.Extra = append(out.Extra, appctx.T(ctx, "أُضيفت قيم مرجعية جديدة: ")+strings.Join(lr.created, appctx.T(ctx, "، ")))
		}
		return nil
	})
}
