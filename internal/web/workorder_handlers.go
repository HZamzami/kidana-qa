package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/HZamzami/kidana-quality/internal/appctx"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

const woPageSize = 25

func (s *Server) workOrdersList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	filter := woFilter(r)
	total, err := s.q.CountWorkOrders(ctx, filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows, err := s.q.ListWorkOrders(ctx, gen.ListWorkOrdersParams{
		SiteID: filter.SiteID, SystemID: filter.SystemID, AssetGroupID: filter.AssetGroupID, ContractorID: filter.ContractorID,
		Type: filter.Type, Status: filter.Status, Priority: filter.Priority, Q: filter.Q,
		Lim: woPageSize, Off: int32((page - 1) * woPageSize), //nolint:gosec // bounded by page count
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	lk, err := s.lookupOptions(ctx, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	q.Del("page")
	d := views.WOListData{
		Rows: rows, Total: total, Page: page, PageSize: woPageSize, Query: q,
		Sites: lk.Sites, Systems: lk.Systems, AssetGroups: lk.AssetGroups, Contractors: lk.Contractors, Statuses: woStatusOptions(true), Priorities: priorityOptions(),
	}
	s.render(w, r, http.StatusOK, views.WorkOrdersPage(d))
}

// woFilter reads the work order list filters from the query string. The list
// and the filtered print share it, so a print always matches the screen.
func woFilter(r *http.Request) gen.CountWorkOrdersParams {
	q := r.URL.Query()
	f := gen.CountWorkOrdersParams{
		SiteID: queryID(r, "site"), SystemID: queryID(r, "system"), AssetGroupID: queryID(r, "group"),
		ContractorID: queryID(r, "contractor"), Q: queryStr(r, "q"),
	}
	if t := gen.WoType(q.Get("type")); t.Valid() {
		f.Type = &t
	}
	if st := gen.WoStatus(q.Get("status")); st.Valid() {
		f.Status = &st
	}
	if p := gen.WoPriority(q.Get("priority")); p.Valid() {
		f.Priority = &p
	}
	return f
}

func (s *Server) woFormData(r *http.Request, id int64, v views.WOFormValues, errs map[string]string) (views.WOFormData, error) {
	lk, err := s.lookupOptions(r.Context(), true)
	if err != nil {
		return views.WOFormData{}, err
	}
	return views.WOFormData{
		ID: id, V: v, Errors: errs,
		Sites: lk.Sites, Systems: lk.Systems, AssetGroups: lk.AssetGroups, Contractors: lk.Contractors,
		Priorities: priorityOptions(), Statuses: woStatusOptions(false),
	}, nil
}

// workOrderNew shows an empty form, or one prefilled from another work order:
//
//	?from=ID            copy: everything except the number and dates
//	?from=ID&keep=ctx   "save and add another": only the shared context
func (s *Server) workOrderNew(w http.ResponseWriter, r *http.Request) {
	v := views.WOFormValues{Type: "WO", Priority: string(gen.WoPriorityNormal)}
	if from := queryID(r, "from"); from != nil {
		wo, err := s.q.GetWorkOrder(r.Context(), *from)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		v = views.WOFormValues{Type: string(wo.Type), SiteID: idStr(wo.SiteID), SystemID: idStr(wo.SystemID), AssetGroupID: idStr(wo.AssetGroupID)}
		if wo.ContractorID != nil {
			v.ContractorID = idStr(*wo.ContractorID)
		}
		if r.URL.Query().Get("keep") != "ctx" {
			v.Location, v.Asset, v.Technician, v.Description = wo.Location, wo.Asset, wo.Technician, wo.Description
			v.Priority = string(wo.Priority)
		}
	}
	d, err := s.woFormData(r, 0, v, nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.WorkOrderForm(d))
}

// woInput is the validated form input shared by create and update.
type woInput struct {
	Ref, Location, Asset, Technician, Description string
	Type                                          gen.WoType
	SiteID, SystemID, AssetGroupID                int64
	ContractorID                                  *int64
	Priority                                      gen.WoPriority
	PlannedDate, ActualDate                       *time.Time
	Status                                        gen.WoStatus
}

func (s *Server) parseWO(r *http.Request, id int64) (woInput, views.WOFormValues, map[string]string, error) {
	_ = r.ParseForm()
	f := newForm(r)
	v := views.WOFormValues{
		Ref: f.Str("ref"), Type: f.Str("type"), SiteID: f.Str("site_id"), Location: f.Str("location"),
		SystemID: f.Str("system_id"), AssetGroupID: f.Str("asset_group_id"), Asset: f.Str("asset"), ContractorID: f.Str("contractor_id"),
		Technician: f.Str("technician"), Priority: f.Str("priority"), Description: f.Str("description"),
		PlannedDate: f.Str("planned_date"), ActualDate: f.Str("actual_date"), Status: f.Str("status"),
	}
	in := woInput{
		Ref:          f.Required("ref", appctx.T(r.Context(), "رقم أمر العمل")),
		Location:     v.Location,
		Asset:        v.Asset,
		Technician:   v.Technician,
		Description:  v.Description,
		SiteID:       f.ReqID("site_id", appctx.T(r.Context(), "الموقع")),
		SystemID:     f.ReqID("system_id", appctx.T(r.Context(), "المنظومة")),
		AssetGroupID: f.ReqID("asset_group_id", appctx.T(r.Context(), "مجموعة الأصل")),
		ContractorID: f.OptID("contractor_id"),
		PlannedDate:  f.OptDate("planned_date"),
		ActualDate:   f.OptDate("actual_date"),
	}
	for name, n := range map[string]int{"ref": 60, "location": 120, "asset": 120, "technician": 80, "description": 2000} {
		f.MaxLen(name, n)
	}
	in.Type = gen.WoType(v.Type)
	if !in.Type.Valid() {
		f.Errors["type"] = appctx.T(r.Context(), "اختر النوع")
	}
	if v.Priority == "" {
		v.Priority = string(gen.WoPriorityNormal) // the column default
	}
	in.Priority = gen.WoPriority(v.Priority)
	if !in.Priority.Valid() {
		f.Errors["priority"] = appctx.T(r.Context(), "اختر الأولوية")
	}
	in.Status = gen.WoStatus(v.Status)
	if id != 0 && (!in.Status.Valid()) {
		f.Errors["status"] = appctx.T(r.Context(), "اختر الحالة")
	}
	if in.Ref != "" {
		exists, err := s.q.GetWorkOrderRefExists(r.Context(), gen.GetWorkOrderRefExistsParams{Ref: in.Ref, ExcludeID: id})
		if err != nil {
			return in, v, nil, err
		}
		if exists {
			f.Errors["ref"] = appctx.T(r.Context(), "يوجد أمر عمل مسجل بهذا الرقم")
		}
	}
	return in, v, f.Errors, nil
}

func (s *Server) workOrderCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, v, errs, err := s.parseWO(r, 0)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(errs) > 0 {
		d, err := s.woFormData(r, 0, v, errs)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		s.render(w, r, http.StatusUnprocessableEntity, views.WorkOrderForm(d))
		return
	}
	wo, err := s.q.CreateWorkOrder(ctx, gen.CreateWorkOrderParams{
		Ref: in.Ref, Type: in.Type, SiteID: in.SiteID, Location: in.Location, SystemID: in.SystemID, AssetGroupID: in.AssetGroupID,
		Asset: in.Asset, ContractorID: in.ContractorID, Technician: in.Technician, Priority: in.Priority,
		Description: in.Description, PlannedDate: in.PlannedDate, ActualDate: in.ActualDate, CreatedBy: currentUserID(ctx),
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "work_order", wo.ID, "created", map[string]any{"ref": wo.Ref})
	s.flash(ctx, appctx.T(ctx, "تم إنشاء أمر العمل ")+wo.Ref, "ok")
	if r.PostFormValue("next") == "new" {
		// "save and add another": keep type, site, system, asset group and contractor
		http.Redirect(w, r, "/work-orders/new?keep=ctx&from="+idStr(wo.ID), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/work-orders/"+idStr(wo.ID), http.StatusSeeOther)
}

func (s *Server) workOrderShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	wo, err := s.q.GetWorkOrder(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	ins, err := s.q.ListInspectionsByWorkOrder(ctx, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	fs, err := s.q.ListFindingsByWorkOrder(ctx, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.WorkOrderShow(views.WOShowData{
		WO: wo, Inspections: ins, Findings: fs, Today: time.Now().Format(time.DateOnly),
	}))
}

func (s *Server) workOrderEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	wo, err := s.q.GetWorkOrder(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	v := views.WOFormValues{
		Ref: wo.Ref, Type: string(wo.Type), SiteID: idStr(wo.SiteID), Location: wo.Location,
		SystemID: idStr(wo.SystemID), AssetGroupID: idStr(wo.AssetGroupID), Asset: wo.Asset, Technician: wo.Technician,
		Priority: string(wo.Priority), Description: wo.Description, Status: string(wo.Status),
		PlannedDate: fmtDate(wo.PlannedDate), ActualDate: fmtDate(wo.ActualDate),
	}
	if wo.ContractorID != nil {
		v.ContractorID = idStr(*wo.ContractorID)
	}
	d, err := s.woFormData(r, id, v, nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if wo.Status == gen.WoStatusAccepted {
		// accepted is derived; show it but keep the select valid
		d.Statuses = woStatusOptions(true)
	}
	s.render(w, r, http.StatusOK, views.WorkOrderForm(d))
}

func (s *Server) workOrderUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	in, v, errs, err := s.parseWO(r, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(errs) > 0 {
		d, err := s.woFormData(r, id, v, errs)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		s.render(w, r, http.StatusUnprocessableEntity, views.WorkOrderForm(d))
		return
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		wo, err := q.UpdateWorkOrder(ctx, gen.UpdateWorkOrderParams{
			ID: id, Ref: in.Ref, Type: in.Type, SiteID: in.SiteID, Location: in.Location, SystemID: in.SystemID, AssetGroupID: in.AssetGroupID,
			Asset: in.Asset, ContractorID: in.ContractorID, Technician: in.Technician, Priority: in.Priority,
			Description: in.Description, PlannedDate: in.PlannedDate, ActualDate: in.ActualDate, Status: in.Status,
		})
		if err != nil {
			return err
		}
		s.audit(ctx, q, "work_order", wo.ID, "updated", nil)
		// A manual status change may not contradict the acceptance rule.
		return s.refreshWorkOrderStatus(ctx, q, wo.ID)
	})
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "تم حفظ أمر العمل"), "ok")
	http.Redirect(w, r, "/work-orders/"+idStr(id), http.StatusSeeOther)
}

func fmtDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.DateOnly)
}

// workOrderDelete removes a work order entered by mistake (admin only). It is
// refused once any inspection was submitted: that is a quality record and the
// work order should be cancelled instead.
func (s *Server) workOrderDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	wo, err := s.q.GetWorkOrder(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	facts, err := s.q.AcceptanceFacts(ctx, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if facts.HasSubmitted {
		s.badRequest(w, r, appctx.T(ctx, "لا يمكن حذف أمر عمل عليه فحص مرسل. غيّر حالته إلى «ملغى» بدلاً من ذلك."))
		return
	}
	photos, err := s.q.ListAttachmentsByWorkOrder(ctx, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteWorkOrder(ctx, id); err != nil {
			return err
		}
		s.audit(ctx, q, "work_order", id, "deleted", map[string]any{"ref": wo.Ref})
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, p := range photos {
		_ = s.store.Delete(ctx, p.StorageKey)
	}
	s.flash(ctx, appctx.T(ctx, "تم حذف أمر العمل ")+wo.Ref, "")
	http.Redirect(w, r, "/work-orders", http.StatusSeeOther)
}
