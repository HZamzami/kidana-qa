package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-qa/internal/db"
	"github.com/HZamzami/kidana-qa/internal/db/gen"
	"github.com/HZamzami/kidana-qa/internal/views"
)

const woPageSize = 25

func (s *Server) workOrdersList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	var typ *gen.WoType
	if t := gen.WoType(q.Get("type")); t.Valid() {
		typ = &t
	}
	var status *gen.WoStatus
	if st := gen.WoStatus(q.Get("status")); st.Valid() {
		status = &st
	}
	filter := gen.CountWorkOrdersParams{
		SiteID: queryID(r, "site"), SystemID: queryID(r, "system"), ContractorID: queryID(r, "contractor"),
		Type: typ, Status: status, Q: queryStr(r, "q"),
	}
	total, err := s.q.CountWorkOrders(ctx, filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows, err := s.q.ListWorkOrders(ctx, gen.ListWorkOrdersParams{
		SiteID: filter.SiteID, SystemID: filter.SystemID, ContractorID: filter.ContractorID,
		Type: filter.Type, Status: filter.Status, Q: filter.Q,
		Lim: woPageSize, Off: int32((page - 1) * woPageSize), //nolint:gosec // bounded by page count
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	sites, systems, contractors, err := s.lookupOptions(ctx, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	q.Del("page")
	d := views.WOListData{
		Rows: rows, Total: total, Page: page, PageSize: woPageSize, Query: q,
		Sites: sites, Systems: systems, Contractors: contractors, Statuses: woStatusOptions(true),
	}
	s.render(w, r, http.StatusOK, views.WorkOrdersPage(d))
}

func (s *Server) woFormData(r *http.Request, id int64, v views.WOFormValues, errs map[string]string) (views.WOFormData, error) {
	sites, systems, contractors, err := s.lookupOptions(r.Context(), true)
	if err != nil {
		return views.WOFormData{}, err
	}
	return views.WOFormData{
		ID: id, V: v, Errors: errs,
		Sites: sites, Systems: systems, Contractors: contractors,
		Priorities: priorityOptions(), Statuses: woStatusOptions(false),
	}, nil
}

func (s *Server) workOrderNew(w http.ResponseWriter, r *http.Request) {
	d, err := s.woFormData(r, 0, views.WOFormValues{Type: "WO"}, nil)
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
	SiteID, SystemID                              int64
	ContractorID                                  *int64
	Priority                                      *gen.WoPriority
	PlannedDate, ActualDate                       *time.Time
	Status                                        gen.WoStatus
}

func (s *Server) parseWO(r *http.Request, id int64) (woInput, views.WOFormValues, map[string]string, error) {
	_ = r.ParseForm()
	f := newForm(r)
	v := views.WOFormValues{
		Ref: f.Str("ref"), Type: f.Str("type"), SiteID: f.Str("site_id"), Location: f.Str("location"),
		SystemID: f.Str("system_id"), Asset: f.Str("asset"), ContractorID: f.Str("contractor_id"),
		Technician: f.Str("technician"), Priority: f.Str("priority"), Description: f.Str("description"),
		PlannedDate: f.Str("planned_date"), ActualDate: f.Str("actual_date"), Status: f.Str("status"),
	}
	in := woInput{
		Ref:          f.Required("ref", "رقم أمر العمل"),
		Location:     v.Location,
		Asset:        v.Asset,
		Technician:   v.Technician,
		Description:  v.Description,
		SiteID:       f.ReqID("site_id", "الموقع"),
		SystemID:     f.ReqID("system_id", "النظام"),
		ContractorID: f.OptID("contractor_id"),
		PlannedDate:  f.OptDate("planned_date"),
		ActualDate:   f.OptDate("actual_date"),
	}
	for name, n := range map[string]int{"ref": 60, "location": 120, "asset": 120, "technician": 80, "description": 2000} {
		f.MaxLen(name, n)
	}
	in.Type = gen.WoType(v.Type)
	if !in.Type.Valid() {
		f.Errors["type"] = "اختر النوع"
	}
	if v.Priority != "" {
		p := gen.WoPriority(v.Priority)
		if p.Valid() {
			in.Priority = &p
		} else {
			f.Errors["priority"] = "قيمة غير صالحة"
		}
	}
	in.Status = gen.WoStatus(v.Status)
	if id != 0 && (!in.Status.Valid()) {
		f.Errors["status"] = "اختر الحالة"
	}
	if in.Ref != "" {
		exists, err := s.q.GetWorkOrderRefExists(r.Context(), gen.GetWorkOrderRefExistsParams{Ref: in.Ref, ExcludeID: id})
		if err != nil {
			return in, v, nil, err
		}
		if exists {
			f.Errors["ref"] = "يوجد أمر عمل مسجل بهذا الرقم"
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
		Ref: in.Ref, Type: in.Type, SiteID: in.SiteID, Location: in.Location, SystemID: in.SystemID,
		Asset: in.Asset, ContractorID: in.ContractorID, Technician: in.Technician, Priority: in.Priority,
		Description: in.Description, PlannedDate: in.PlannedDate, ActualDate: in.ActualDate, CreatedBy: currentUserID(ctx),
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "work_order", wo.ID, "created", map[string]any{"ref": wo.Ref})
	s.flash(ctx, "تم إنشاء أمر العمل "+wo.Ref, "ok")
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
		SystemID: idStr(wo.SystemID), Asset: wo.Asset, Technician: wo.Technician,
		Description: wo.Description, Status: string(wo.Status),
		PlannedDate: fmtDate(wo.PlannedDate), ActualDate: fmtDate(wo.ActualDate),
	}
	if wo.ContractorID != nil {
		v.ContractorID = idStr(*wo.ContractorID)
	}
	if wo.Priority != nil {
		v.Priority = string(*wo.Priority)
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
			ID: id, Ref: in.Ref, Type: in.Type, SiteID: in.SiteID, Location: in.Location, SystemID: in.SystemID,
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
	s.flash(ctx, "تم حفظ أمر العمل", "ok")
	http.Redirect(w, r, "/work-orders/"+idStr(id), http.StatusSeeOther)
}

func fmtDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.DateOnly)
}
