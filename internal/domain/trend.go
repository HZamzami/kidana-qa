package domain

import "time"

// MonthPoint is one month of the quality trend. Avg and PassRate are -1 when
// the month has no submitted inspections.
type MonthPoint struct {
	Month    string // YYYY-MM
	Count    int
	Avg      float64
	PassRate float64
}

// TrendByMonth groups submitted inspections by month, oldest first, with
// every month between from and to present.
func TrendByMonth(rows []InspectionRow, from, to time.Time) []MonthPoint {
	type acc struct {
		sum     float64
		n, pass int
	}
	by := map[string]*acc{}
	for _, r := range rows {
		k := r.Date.Format("2006-01")
		if by[k] == nil {
			by[k] = &acc{}
		}
		by[k].sum += r.Score
		by[k].n++
		if r.Passed {
			by[k].pass++
		}
	}
	var out []MonthPoint
	for m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(to); m = m.AddDate(0, 1, 0) {
		k := m.Format("2006-01")
		p := MonthPoint{Month: k, Avg: -1, PassRate: -1}
		if a := by[k]; a != nil && a.n > 0 {
			p.Count, p.Avg, p.PassRate = a.n, a.sum/float64(a.n), float64(a.pass)/float64(a.n)*100
		}
		out = append(out, p)
	}
	return out
}
