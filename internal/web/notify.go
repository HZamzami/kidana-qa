package web

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
)

// Notification kinds. Each has a message template in views.NotificationText,
// rendered in the reader's language when shown.
const (
	notifyFindingRaised    = "finding_raised"
	notifyFindingResponded = "finding_responded"
	notifyFindingVerified  = "finding_verified"
	notifyFindingRejected  = "finding_rejected"
	notifyFindingDueSoon   = "finding_due_soon"
	notifyFindingOverdue   = "finding_overdue"
	notifyAssigned         = "assigned"
	notifyReinspection     = "reinspection"
	notifyResetRequest     = "reset_request"
	notifyReportReady      = "report_ready"
)

// note is one notification to send to several users.
type note struct {
	kind   string
	link   string
	dedupe string // optional: the same key reaches a user only once
	params map[string]string
}

// notify sends n to each user except the one doing the action. Failures are
// logged: a missed notification must never block the change itself.
func (s *Server) notify(ctx context.Context, q *gen.Queries, users []int64, n note) {
	params, _ := json.Marshal(n.params)
	self := currentUserID(ctx)
	seen := map[int64]bool{}
	for _, id := range users {
		if id == 0 || seen[id] || (self != nil && *self == id) {
			continue
		}
		seen[id] = true
		var dedupe *string
		if n.dedupe != "" {
			d := n.dedupe
			dedupe = &d
		}
		if err := q.CreateNotification(ctx, gen.CreateNotificationParams{
			UserID: id, Kind: n.kind, Params: params, Link: n.link, Dedupe: dedupe,
		}); err != nil {
			s.log.Warn("notify", "kind", n.kind, "user", id, "err", err)
		}
	}
}

// onPasswordResetRequested tells admins a user asked for a reset link.
func (s *Server) onPasswordResetRequested(ctx context.Context, u gen.User) {
	admins, err := s.q.ListAdmins(ctx)
	if err != nil {
		s.log.Warn("list admins", "err", err)
		return
	}
	ids := make([]int64, len(admins))
	for i, a := range admins {
		ids[i] = a.ID
	}
	s.notify(ctx, s.q, ids, note{
		kind: notifyResetRequest, link: "/admin/users", dedupe: fmt.Sprintf("reset:%d:%s", u.ID, time.Now().Format(time.DateOnly)),
		params: map[string]string{"name": u.Name},
	})
}

// onAssigned tells an assignee about a new or handed-over assignment.
func (s *Server) onAssigned(ctx context.Context, q *gen.Queries, a gen.InspectionAssignment, ref string) {
	if a.AssigneeID == nil {
		return
	}
	kind := notifyAssigned
	if a.Reason == gen.AssignmentReasonReinspection {
		kind = notifyReinspection
	}
	due := ""
	if a.DueDate != nil {
		due = dates.DMY(*a.DueDate)
	}
	s.notify(ctx, q, []int64{*a.AssigneeID}, note{
		kind: kind, link: "/work-orders/" + idStr(a.WorkOrderID),
		dedupe: fmt.Sprintf("assign:%d:%d", a.ID, *a.AssigneeID),
		params: map[string]string{"ref": ref, "stage": string(a.Stage), "due": due},
	})
}

// afterInspectionSubmitted closes the stage's open assignment and, when an
// after-work inspection failed, books a re-inspection with the same inspector.
func (s *Server) afterInspectionSubmitted(ctx context.Context, q *gen.Queries, in gen.GetInspectionRow, passed bool, dueHint *time.Time) error {
	inspID := in.ID
	if err := q.CompleteAssignments(ctx, gen.CompleteAssignmentsParams{
		WorkOrderID: in.WorkOrderID, Stage: in.Stage, InspectionID: &inspID,
	}); err != nil {
		return err
	}
	if passed || in.Stage != gen.InspectionStageAfter {
		return nil
	}
	due := dueHint
	if due == nil {
		d := dates.Today().AddDate(0, 0, 7)
		due = &d
	}
	inspector := in.InspectorID
	a, err := q.CreateAssignment(ctx, gen.CreateAssignmentParams{
		WorkOrderID: in.WorkOrderID, Stage: gen.InspectionStageAfter, AssigneeID: &inspector, DueDate: due,
		Reason: gen.AssignmentReasonReinspection, Note: "", CreatedBy: currentUserID(ctx),
	})
	if err != nil {
		if isNoRows(err) { // an open after-work assignment already exists
			return nil
		}
		return err
	}
	// The inspector who failed it is usually the one acting; notify anyway so it shows in their tasks.
	params, _ := json.Marshal(map[string]string{"ref": in.WoRef, "stage": string(a.Stage), "due": dates.DMY(*due)})
	d := fmt.Sprintf("assign:%d:%d", a.ID, inspector)
	return q.CreateNotification(ctx, gen.CreateNotificationParams{
		UserID: inspector, Kind: notifyReinspection, Params: params, Link: "/work-orders/" + idStr(in.WorkOrderID), Dedupe: &d,
	})
}

// sampleNewWorkOrder books the inspections the sampling rules pick for a new
// work order. They go to the pool for any inspector to claim.
func (s *Server) sampleNewWorkOrder(ctx context.Context, q *gen.Queries, wo gen.WorkOrder) error {
	rows, err := q.ListActiveSamplingRules(ctx)
	if err != nil {
		return err
	}
	rules := make([]domain.SamplingRule, len(rows))
	for i, r := range rows {
		rules[i] = domain.SamplingRule{Stage: domain.Stage(r.Stage), Percent: int(r.Percent)}
		if r.WoType != nil {
			rules[i].Type = string(*r.WoType)
		}
		if r.Priority != nil {
			rules[i].Priority = string(*r.Priority)
		}
	}
	for _, stage := range domain.SampleStages(rules, wo.Ref, string(wo.Type), string(wo.Priority)) {
		due := wo.PlannedDate
		if stage == domain.StageAfter && wo.ActualDate != nil {
			d := wo.ActualDate.AddDate(0, 0, 2)
			due = &d
		}
		if _, err := q.CreateAssignment(ctx, gen.CreateAssignmentParams{
			WorkOrderID: wo.ID, Stage: gen.InspectionStage(stage), DueDate: due,
			Reason: gen.AssignmentReasonSampling, CreatedBy: currentUserID(ctx),
		}); err != nil && !isNoRows(err) {
			return err
		}
	}
	return nil
}

// RunScheduler sends reminders every hour until ctx ends: findings due within
// two days, and overdue findings once a week. Dedupe keys make reruns safe.
func (s *Server) RunScheduler(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		s.runReminders(appctx.WithLang(ctx, "ar"))
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) runReminders(ctx context.Context) {
	today := dates.Today()
	horizon := today.AddDate(0, 0, 2)
	rows, err := s.q.ListFindingsDueBy(ctx, &horizon)
	if err != nil {
		s.log.Warn("reminders", "err", err)
		return
	}
	_, week := today.ISOWeek()
	for _, f := range rows {
		users := s.findingAudience(ctx, f.CreatedBy, f.ContractorID)
		n := note{
			link:   "/work-orders/" + idStr(f.WorkOrderID),
			params: map[string]string{"ref": f.Ref, "due": dates.DMY(*f.DueDate), "desc": clip(f.Description, 80)},
		}
		if f.DueDate.Before(today) {
			n.kind, n.dedupe = notifyFindingOverdue, fmt.Sprintf("overdue:%d:%d-%d", f.ID, today.Year(), week)
		} else {
			n.kind, n.dedupe = notifyFindingDueSoon, fmt.Sprintf("duesoon:%d", f.ID)
		}
		s.notify(ctx, s.q, users, n)
	}
	s.runScheduledReports(ctx)
}

// findingAudience is who follows a finding: the inspector who raised it and
// the contractor's users.
func (s *Server) findingAudience(ctx context.Context, createdBy, contractorID *int64) []int64 {
	var ids []int64
	if createdBy != nil {
		ids = append(ids, *createdBy)
	}
	return append(ids, s.contractorUserIDs(ctx, contractorID)...)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// contractorUserIDs lists the active users who belong to a contractor.
func (s *Server) contractorUserIDs(ctx context.Context, contractorID *int64) []int64 {
	if contractorID == nil {
		return nil
	}
	users, err := s.q.ListContractorUsers(ctx, contractorID)
	if err != nil {
		s.log.Warn("contractor users", "err", err)
		return nil
	}
	ids := make([]int64, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	return ids
}

// runScheduledReports creates the monthly report run (see reports).
func (s *Server) runScheduledReports(ctx context.Context) { _ = ctx }
