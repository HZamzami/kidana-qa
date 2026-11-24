package web

import (
	"net/http"
	"strconv"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// ---------------------------------------------------------------- notifications

func (s *Server) notificationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.q.ListNotifications(ctx, gen.ListNotificationsParams{UserID: appctx.User(ctx).ID, Limit: 200})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.NotificationsPage(rows))
}

// notificationsBell is the header bell, refreshed by htmx every minute.
func (s *Server) notificationsBell(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, err := s.q.CountUnreadNotifications(ctx, appctx.User(ctx).ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.Bell(n))
}

// notificationOpen marks a notification read and follows its link.
func (s *Server) notificationOpen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	link, err := s.q.MarkNotificationRead(ctx, gen.MarkNotificationReadParams{ID: id, UserID: appctx.User(ctx).ID})
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if link == "" || link[0] != '/' {
		link = "/notifications"
	}
	http.Redirect(w, r, link, http.StatusSeeOther)
}

func (s *Server) notificationsReadAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.q.MarkAllNotificationsRead(ctx, appctx.User(ctx).ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// ---------------------------------------------------------------- my tasks

func (s *Server) tasksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := appctx.User(ctx)
	d := views.TasksData{}
	var err error
	if u.Role == gen.UserRoleContractor {
		open := true
		if d.ContractorFindings, err = s.q.ListFindings(ctx, gen.ListFindingsParams{ContractorID: u.ContractorID, OpenOnly: open}); err != nil {
			s.serverError(w, r, err)
			return
		}
		s.render(w, r, http.StatusOK, views.TasksPage(d))
		return
	}
	me := u.ID
	if d.Mine, err = s.q.ListOpenAssignments(ctx, &me); err != nil {
		s.serverError(w, r, err)
		return
	}
	if appctx.CanEdit(ctx) {
		if d.Pool, err = s.q.ListOpenAssignments(ctx, nil); err != nil {
			s.serverError(w, r, err)
			return
		}
		// Inspectors verify the responses to findings they raised; admins see all.
		var by *int64
		if !appctx.IsAdmin(ctx) {
			by = &me
		}
		if d.ToVerify, err = s.q.ListFindingsToVerify(ctx, by); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	s.render(w, r, http.StatusOK, views.TasksPage(d))
}

// ---------------------------------------------------------------- assignments

// assignmentCreate assigns an inspection of a work order (manual), to a
// person or to the pool.
func (s *Server) assignmentCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	woID, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	wo, err := s.q.GetWorkOrder(ctx, woID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	f := newForm(r)
	stage := gen.InspectionStage(f.Str("stage"))
	if !stage.Valid() {
		s.badRequest(w, r, appctx.T(ctx, "مرحلة غير صالحة"))
		return
	}
	a, err := s.q.CreateAssignment(ctx, gen.CreateAssignmentParams{
		WorkOrderID: woID, Stage: stage, AssigneeID: f.OptID("assignee_id"), DueDate: f.OptDate("due_date"),
		Reason: gen.AssignmentReasonManual, Note: f.Str("note"), CreatedBy: currentUserID(ctx),
	})
	switch {
	case isNoRows(err):
		s.flash(ctx, appctx.T(ctx, "يوجد تكليف مفتوح لهذه المرحلة. عدّله بدلاً من إضافة آخر."), "warn")
	case err != nil:
		s.serverError(w, r, err)
		return
	default:
		s.audit(ctx, s.q, "assignment", a.ID, "created", map[string]any{"stage": stage, "assignee": a.AssigneeID})
		s.onAssigned(ctx, s.q, a, wo.Ref)
		s.flash(ctx, appctx.T(ctx, "تم إسناد الفحص"), "ok")
	}
	http.Redirect(w, r, "/work-orders/"+idStr(woID), http.StatusSeeOther)
}

// assignmentClaim takes an assignment from the pool.
func (s *Server) assignmentClaim(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.assignmentFromPath(w, r)
	if !ok {
		return
	}
	n, err := s.q.ClaimAssignment(ctx, gen.ClaimAssignmentParams{ID: a.ID, AssigneeID: currentUserID(ctx)})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if n == 0 {
		s.flash(ctx, appctx.T(ctx, "أخذ هذا التكليف شخص آخر"), "warn")
	} else {
		s.audit(ctx, s.q, "assignment", a.ID, "claimed", nil)
		s.flash(ctx, appctx.T(ctx, "أصبح التكليف ضمن مهامك"), "ok")
	}
	http.Redirect(w, r, backTo(r), http.StatusSeeOther)
}

// assignmentUpdate hands an assignment to someone else or changes its due date.
func (s *Server) assignmentUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.assignmentFromPath(w, r)
	if !ok {
		return
	}
	f := newForm(r)
	assignee := f.OptID("assignee_id")
	if err := s.q.ReassignAssignment(ctx, gen.ReassignAssignmentParams{ID: a.ID, AssigneeID: assignee, DueDate: f.OptDate("due_date")}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "assignment", a.ID, "updated", map[string]any{"assignee": assignee})
	if assignee != nil && (a.AssigneeID == nil || *a.AssigneeID != *assignee) {
		wo, err := s.q.GetWorkOrder(ctx, a.WorkOrderID)
		if err == nil {
			a.AssigneeID = assignee // the dedupe key includes the person, so a hand-over notifies again
			s.onAssigned(ctx, s.q, a, wo.Ref)
		}
	}
	s.flash(ctx, appctx.T(ctx, "تم الحفظ"), "ok")
	http.Redirect(w, r, backTo(r), http.StatusSeeOther)
}

func (s *Server) assignmentCancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.assignmentFromPath(w, r)
	if !ok {
		return
	}
	if _, err := s.q.CancelAssignment(ctx, a.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "assignment", a.ID, "cancelled", nil)
	s.flash(ctx, appctx.T(ctx, "أُلغي التكليف"), "")
	http.Redirect(w, r, backTo(r), http.StatusSeeOther)
}

func (s *Server) assignmentFromPath(w http.ResponseWriter, r *http.Request) (gen.InspectionAssignment, bool) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return gen.InspectionAssignment{}, false
	}
	a, err := s.q.GetAssignment(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return a, false
	}
	return a, true
}

// assignees are the people an inspection can be assigned to.
func (s *Server) assignees(r *http.Request) ([]views.Option, error) {
	rows, err := s.q.ListAssignableUsers(r.Context())
	if err != nil {
		return nil, err
	}
	out := make([]views.Option, len(rows))
	for i, u := range rows {
		out[i] = views.Option{Value: idStr(u.ID), Label: u.Name}
	}
	return out, nil
}

// ---------------------------------------------------------------- sampling rules (admin)

func (s *Server) samplingPage(w http.ResponseWriter, r *http.Request) {
	rules, err := s.q.ListSamplingRules(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.SamplingPage(rules))
}

func (s *Server) samplingCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := newForm(r)
	stage := gen.InspectionStage(f.Str("stage"))
	pct := f.Int("percent", -1)
	if !stage.Valid() || pct < 0 || pct > 100 {
		s.flash(ctx, appctx.T(ctx, "اختر المرحلة ونسبة من 0 إلى 100"), "warn")
		http.Redirect(w, r, "/admin/sampling", http.StatusSeeOther)
		return
	}
	p := gen.CreateSamplingRuleParams{Stage: stage, Percent: int32(pct)} //nolint:gosec // 0..100
	if t := gen.WoType(f.Str("wo_type")); t.Valid() {
		p.WoType = &t
	}
	if pr := gen.WoPriority(f.Str("priority")); pr.Valid() {
		p.Priority = &pr
	}
	if err := s.q.CreateSamplingRule(ctx, p); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "sampling_rule", 0, "created", map[string]any{"stage": stage, "percent": pct})
	s.flash(ctx, appctx.T(ctx, "تمت الإضافة"), "ok")
	http.Redirect(w, r, "/admin/sampling", http.StatusSeeOther)
}

func (s *Server) samplingUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	if r.PostFormValue("delete") != "" {
		if err := s.q.DeleteSamplingRule(ctx, id); err != nil {
			s.serverError(w, r, err)
			return
		}
		s.audit(ctx, s.q, "sampling_rule", id, "deleted", nil)
		http.Redirect(w, r, "/admin/sampling", http.StatusSeeOther)
		return
	}
	pct, err := strconv.Atoi(r.PostFormValue("percent"))
	if err != nil || pct < 0 || pct > 100 {
		s.flash(ctx, appctx.T(ctx, "اختر المرحلة ونسبة من 0 إلى 100"), "warn")
		http.Redirect(w, r, "/admin/sampling", http.StatusSeeOther)
		return
	}
	if err := s.q.UpdateSamplingRule(ctx, gen.UpdateSamplingRuleParams{ID: id, Percent: int32(pct), Active: r.PostFormValue("active") == "on"}); err != nil { //nolint:gosec // 0..100
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "sampling_rule", id, "updated", map[string]any{"percent": pct})
	s.flash(ctx, appctx.T(ctx, "تم الحفظ"), "ok")
	http.Redirect(w, r, "/admin/sampling", http.StatusSeeOther)
}
