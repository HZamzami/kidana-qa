package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// auditPage is the admin change log, filtered and paged.
func (s *Server) auditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	const pageSize = 100
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	rows, err := s.q.SearchAudit(ctx, gen.SearchAuditParams{
		UserID: queryID(r, "user"), Entity: queryStr(r, "entity"), DateFrom: queryDate(r, "from"), DateTo: queryDate(r, "to"),
		Lim: pageSize + 1, Off: int32((page - 1) * pageSize), //nolint:gosec // bounded paging
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := views.AuditData{Query: r.URL.Query(), Page: page, More: len(rows) > pageSize}
	if d.More {
		rows = rows[:pageSize]
	}
	for _, h := range rows {
		d.Entries = append(d.Entries, views.HistoryEntry{Entity: h.Entity, EntityID: h.EntityID, Action: h.Action, User: h.UserName, Details: h.Details, At: h.CreatedAt})
	}
	users, err := s.q.ListUsers(ctx)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, u := range users {
		d.Users = append(d.Users, views.Option{Value: idStr(u.ID), Label: u.Name})
	}
	d.Query.Del("page")
	s.render(w, r, http.StatusOK, views.AuditPage(d))
}

// ---------------------------------------------------------------- targets and weights (admin)

func (s *Server) scoreWeights(r *http.Request) domain.ScoreWeights {
	w := domain.DefaultScoreWeights
	if v, err := s.q.GetSetting(r.Context(), "scorecard_weights"); err == nil {
		_ = json.Unmarshal([]byte(v), &w)
	}
	return w
}

func (s *Server) slaPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListSLATargets(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.SLAPage(rows, s.scoreWeights(r)))
}

func (s *Server) slaSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = r.ParseForm()
	for _, key := range []string{"emergency", "high", "normal", "finding_critical", "finding_normal"} {
		days, err := strconv.Atoi(r.PostFormValue("days_" + key))
		if err != nil || days < 0 || days > 365 {
			continue
		}
		if err := s.q.UpdateSLATarget(ctx, gen.UpdateSLATargetParams{Key: key, Days: int32(days)}); err != nil { //nolint:gosec // 0..365
			s.serverError(w, r, err)
			return
		}
	}
	weight := func(k string) int {
		v, err := strconv.Atoi(r.PostFormValue("w_" + k))
		if err != nil || v < 0 || v > 100 {
			return 0
		}
		return v
	}
	ws := domain.ScoreWeights{After: weight("after"), During: weight("during"), SLA: weight("sla"), Findings: weight("findings")}
	if ws.After+ws.During+ws.SLA+ws.Findings != 100 {
		s.flash(ctx, appctx.T(ctx, "مجموع أوزان التقييم يجب أن يساوي 100. لم تُحفظ الأوزان."), "warn")
	} else {
		b, _ := json.Marshal(ws)
		if err := s.q.SetSetting(ctx, gen.SetSettingParams{Key: "scorecard_weights", Value: string(b)}); err != nil {
			s.serverError(w, r, err)
			return
		}
		s.flash(ctx, appctx.T(ctx, "تم الحفظ"), "ok")
	}
	s.audit(ctx, s.q, "sla", 0, "updated", nil)
	http.Redirect(w, r, "/admin/sla", http.StatusSeeOther)
}
