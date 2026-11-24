package web

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

func (s *Server) findingsList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := gen.ListFindingsParams{SiteID: queryID(r, "site"), ContractorID: queryID(r, "contractor"), Q: queryStr(r, "q")}
	switch r.URL.Query().Get("scope") {
	case "", "open":
		p.OpenOnly = true
	case "overdue":
		p.OverdueOnly = true
	case "closed":
		st := gen.FindingStatusClosed
		p.Status = &st
	}
	rows, err := s.q.ListFindings(ctx, p)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	lk, err := s.lookupOptions(ctx, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.FindingsPage(views.FindingsListData{
		Rows: rows, Query: r.URL.Query(), Sites: lk.Sites, Contractors: lk.Contractors, Today: time.Now().Format(time.DateOnly),
	}))
}

func findingValues(f gen.GetFindingRow) views.FindingFormValues {
	return views.FindingFormValues{
		Description: f.Description, CorrectiveAction: f.CorrectiveAction, Owner: f.Owner,
		DueDate: fmtDate(f.DueDate), Severity: string(f.Severity), Status: string(f.Status),
	}
}

func (s *Server) findingEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	f, err := s.q.GetFinding(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.FindingEdit(views.FindingEditData{F: f, V: findingValues(f)}))
}

func (s *Server) findingUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	cur, err := s.q.GetFinding(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if cur.Status == gen.FindingStatusClosed {
		s.badRequest(w, r, "الملاحظة مغلقة. أعد فتحها أولاً.")
		return
	}
	f := newForm(r)
	v := views.FindingFormValues{
		Description: f.Required("description", "وصف الملاحظة"), CorrectiveAction: f.Required("corrective_action", "الإجراء التصحيحي"),
		Owner: f.Str("owner"), DueDate: f.Str("due_date"), Severity: f.Str("severity"), Status: f.Str("status"),
	}
	due := f.OptDate("due_date")
	f.MaxLen("description", 1500)
	f.MaxLen("corrective_action", 1000)
	f.MaxLen("owner", 80)
	sev, st := gen.FindingSeverity(v.Severity), gen.FindingStatus(v.Status)
	if !sev.Valid() {
		f.Errors["severity"] = "قيمة غير صالحة"
	}
	if !st.Valid() || st == gen.FindingStatusClosed {
		f.Errors["status"] = "قيمة غير صالحة"
	}
	if !f.Valid() {
		s.render(w, r, http.StatusUnprocessableEntity, views.FindingEdit(views.FindingEditData{F: cur, V: v, Errors: f.Errors}))
		return
	}
	if _, err := s.q.UpdateFinding(ctx, gen.UpdateFindingParams{
		ID: id, Description: v.Description, CorrectiveAction: v.CorrectiveAction, Owner: v.Owner,
		DueDate: due, Severity: sev, Status: st,
	}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "finding", id, "updated", nil)
	s.flash(ctx, "تم حفظ الملاحظة", "ok")
	http.Redirect(w, r, "/findings/"+idStr(id)+"/edit", http.StatusSeeOther)
}

func (s *Server) findingClose(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	cur, err := s.q.GetFinding(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	f := newForm(r)
	note := f.Required("closure_note", "كيف تمت المعالجة")
	f.MaxLen("closure_note", 1000)
	if !f.Valid() {
		s.render(w, r, http.StatusUnprocessableEntity, views.FindingEdit(views.FindingEditData{F: cur, V: findingValues(cur), Errors: f.Errors}))
		return
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.CloseFinding(ctx, gen.CloseFindingParams{ID: id, ClosureNote: note, ClosedBy: currentUserID(ctx)}); err != nil {
			return err
		}
		s.audit(ctx, q, "finding", id, "closed", nil)
		return s.refreshWorkOrderStatus(ctx, q, cur.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, "تم إغلاق الملاحظة", "ok")
	http.Redirect(w, r, "/work-orders/"+idStr(cur.WorkOrderID), http.StatusSeeOther)
}

func (s *Server) findingReopen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	cur, err := s.q.GetFinding(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.ReopenFinding(ctx, id); err != nil {
			return err
		}
		s.audit(ctx, q, "finding", id, "reopened", nil)
		return s.refreshWorkOrderStatus(ctx, q, cur.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, "أُعيد فتح الملاحظة", "")
	http.Redirect(w, r, "/findings/"+idStr(id)+"/edit", http.StatusSeeOther)
}

func (s *Server) findingDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	cur, err := s.q.GetFinding(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if cur.InspectionID != nil {
		in, err := s.q.GetInspection(ctx, *cur.InspectionID)
		if err == nil && in.Status == gen.InspectionStatusSubmitted && cur.Status != gen.FindingStatusClosed {
			// Deleting it could make a failed inspection look unsupported; keep the record.
			s.badRequest(w, r, "هذه الملاحظة مرتبطة بفحص مرسل. أغلقها بدلاً من حذفها.")
			return
		}
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteFinding(ctx, id); err != nil {
			return err
		}
		s.audit(ctx, q, "finding", id, "deleted", map[string]any{"work_order": cur.WorkOrderID})
		return s.refreshWorkOrderStatus(ctx, q, cur.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, "تم حذف الملاحظة", "")
	http.Redirect(w, r, "/work-orders/"+idStr(cur.WorkOrderID), http.StatusSeeOther)
}
