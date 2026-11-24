package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db"
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
		var saved domain.ScoreWeights
		if json.Unmarshal([]byte(v), &saved) == nil && domain.WeightGap(saved.After, saved.During, saved.SLA, saved.Findings) == 0 {
			w = saved // never use weights that don't total 100
		}
	}
	return w
}

func (s *Server) slaPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListSLATargets(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := views.SLAData{Keys: views.SLAKeys, Days: map[string]string{}, Weights: map[string]string{}}
	for _, row := range rows {
		d.Days[row.Key] = strconv.Itoa(int(row.Days))
	}
	ws := s.scoreWeights(r)
	d.Weights["after"], d.Weights["during"] = strconv.Itoa(ws.After), strconv.Itoa(ws.During)
	d.Weights["sla"], d.Weights["findings"] = strconv.Itoa(ws.SLA), strconv.Itoa(ws.Findings)
	s.render(w, r, http.StatusOK, views.SLAPage(d))
}

// slaSave saves the resolution targets and the scorecard weights together:
// nothing is saved unless every value is valid and the weights total 100.
func (s *Server) slaSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = r.ParseForm()
	d := views.SLAData{Keys: views.SLAKeys, Days: map[string]string{}, Weights: map[string]string{}}
	days := map[string]int{}
	for _, key := range views.SLAKeys {
		v := domain.ASCIIDigits(r.PostFormValue("days_" + key))
		d.Days[key] = v
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 365 {
			d.Errors = append(d.Errors, appctx.Tf(ctx, "«%s»: اكتب عدد أيام من 1 إلى 365.", appctx.T(ctx, views.SLALabel(key))))
			continue
		}
		days[key] = n
	}
	weights := map[string]int{}
	weightsOK := true
	for _, key := range views.WeightKeys {
		v := domain.ASCIIDigits(r.PostFormValue("w_" + key))
		d.Weights[key] = v
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 100 {
			d.Errors = append(d.Errors, appctx.Tf(ctx, "«%s»: اكتب وزناً من 0 إلى 100.", appctx.T(ctx, views.WeightLabel(key))))
			weightsOK = false
			continue
		}
		weights[key] = n
	}
	ws := domain.ScoreWeights{After: weights["after"], During: weights["during"], SLA: weights["sla"], Findings: weights["findings"]}
	if weightsOK {
		if msg := weightProblem(ctx, ws.After, ws.During, ws.SLA, ws.Findings); msg != "" {
			d.Errors = append(d.Errors, msg)
		}
	}
	if len(d.Errors) > 0 {
		s.render(w, r, http.StatusUnprocessableEntity, views.SLAPage(d))
		return
	}
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for key, n := range days {
			if err := q.UpdateSLATarget(ctx, gen.UpdateSLATargetParams{Key: key, Days: int32(n)}); err != nil { //nolint:gosec // 1..365
				return err
			}
		}
		b, _ := json.Marshal(ws)
		if err := q.SetSetting(ctx, gen.SetSettingParams{Key: "scorecard_weights", Value: string(b)}); err != nil {
			return err
		}
		s.audit(ctx, q, "sla", 0, "updated", map[string]any{"days": days, "weights": ws})
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "تم الحفظ"), "ok")
	http.Redirect(w, r, "/admin/sla", http.StatusSeeOther)
}
