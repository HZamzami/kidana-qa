package web

import (
	"context"
	"net/http"
	"time"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/i18n"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// scorePeriod is the scorecard period: from/to in the query, else the last
// 12 months.
func scorePeriod(r *http.Request) (from, to time.Time) {
	to = dates.Today()
	if d := queryDate(r, "to"); d != nil {
		to = *d
	}
	from = to.AddDate(-1, 0, 1)
	if d := queryDate(r, "from"); d != nil {
		from = *d
	}
	return from, to
}

// contractorScores computes every contractor's scorecard for the period.
func (s *Server) contractorScores(ctx context.Context, r *http.Request, from, to time.Time) ([]views.ScoreRow, error) {
	insp, err := s.q.ContractorInspectionStats(ctx, gen.ContractorInspectionStatsParams{DateFrom: from, DateTo: to})
	if err != nil {
		return nil, err
	}
	fstats, err := s.q.ContractorFindingStats(ctx, gen.ContractorFindingStatsParams{DateFrom: from, DateTo: to})
	if err != nil {
		return nil, err
	}
	wos, err := s.q.ListWorkOrdersForSLA(ctx, gen.ListWorkOrdersForSLAParams{DateFrom: &from, DateTo: &to})
	if err != nil {
		return nil, err
	}
	byCon := map[int64]gen.ContractorFindingStatsRow{}
	for _, f := range fstats {
		if f.ContractorID != nil {
			byCon[*f.ContractorID] = f
		}
	}
	states := map[int64][]string{}
	today := dates.Today()
	for _, w := range wos {
		if w.ContractorID != nil {
			states[*w.ContractorID] = append(states[*w.ContractorID], domain.SLAState(w.PlannedDate, w.ActualDate, today))
		}
	}
	weights := s.scoreWeights(r)
	l := appctx.Lang(ctx)
	rows := make([]views.ScoreRow, 0, len(insp))
	scores := map[int64]float64{}
	for _, c := range insp {
		if !c.Active && c.Inspections == 0 {
			continue
		}
		row := views.ScoreRow{
			ID: c.ID, Name: i18n.Pick(l, c.Name, c.NameEn), Inspections: int(c.Inspections),
			During: c.DuringAvg, After: c.AfterAvg, FirstPass: -1, SLA: -1, OnTime: -1, AvgClose: -1,
		}
		if c.FirstAfter > 0 {
			row.FirstPass = float64(c.FirstAfterPassed) / float64(c.FirstAfter) * 100
		}
		if rate := domain.SLARate(states[c.ID]); rate != nil {
			row.SLA = *rate
		}
		if f, ok := byCon[c.ID]; ok {
			row.Findings, row.Open, row.Overdue, row.AvgClose = int(f.Total), int(f.Open), int(f.Overdue), f.AvgDaysToClose
			if f.Closed > 0 {
				row.OnTime = float64(f.ClosedOnTime) / float64(f.Closed) * 100
			}
		}
		row.Composite = domain.Composite(domain.ScoreParts{After: row.After, During: row.During, SLA: row.SLA, FindingsOnTime: row.OnTime}, weights)
		scores[c.ID] = row.Composite
		rows = append(rows, row)
	}
	rank := map[int64]int{}
	order := map[int64]int{}
	for i, rk := range domain.Rank(scores) {
		rank[rk.ID], order[rk.ID] = rk.Rank, i
	}
	sorted := make([]views.ScoreRow, len(rows))
	for _, row := range rows {
		row.Rank = rank[row.ID]
		sorted[order[row.ID]] = row
	}
	return sorted, nil
}

// contractorsPage ranks contractors by their composite score.
func (s *Server) contractorsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if scope := appctx.ContractorScope(ctx); scope != nil {
		http.Redirect(w, r, "/contractors/"+idStr(*scope), http.StatusSeeOther)
		return
	}
	from, to := scorePeriod(r)
	rows, err := s.contractorScores(ctx, r, from, to)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ContractorsPage(views.ContractorsData{Rows: rows, From: from, To: to, Weights: s.scoreWeights(r)}))
}

// contractorScorecard is one contractor's page; print=true is the A4 report.
func (s *Server) contractorScorecard(w http.ResponseWriter, r *http.Request, print bool) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok || !canSeeContractor(ctx, &id) {
		s.notFound(w, r)
		return
	}
	from, to := scorePeriod(r)
	rows, err := s.contractorScores(ctx, r, from, to)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := views.ScorecardData{From: from, To: to, Of: len(rows), Print: print, Printed: time.Now().In(dates.Riyadh)}
	found := false
	for _, row := range rows {
		if row.ID == id {
			d.Row, found = row, true
		}
	}
	if !found {
		s.notFound(w, r)
		return
	}
	sc := reportScope{Params: gen.ListSubmittedInspectionsParams{ContractorID: &id, DateFrom: &from, DateTo: &to}}
	rep, err := s.loadReport(ctx, sc)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d.Summary, d.Trend = rep.Summary, rep.Trend
	if d.FindingsTrend, err = s.q.FindingsByMonth(ctx, gen.FindingsByMonthParams{ContractorID: &id, DateFrom: to.AddDate(0, -11, 0), DateTo: to}); err != nil {
		s.serverError(w, r, err)
		return
	}
	overdue := true
	if d.Overdue, err = s.q.ListFindings(ctx, gen.ListFindingsParams{ContractorID: &id, OverdueOnly: overdue}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ScorecardPage(d))
}

func (s *Server) contractorPage(w http.ResponseWriter, r *http.Request) {
	s.contractorScorecard(w, r, false)
}

func (s *Server) contractorReport(w http.ResponseWriter, r *http.Request) {
	s.contractorScorecard(w, r, true)
}
