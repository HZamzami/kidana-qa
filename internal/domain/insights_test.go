package domain

import (
	"testing"
	"time"
)

func day(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func TestSLA(t *testing.T) {
	targets := SLATargets{"emergency": 1, "normal": 7}
	due, ok := DueFrom(day("2026-11-01"), "emergency", targets)
	if !ok || !due.Equal(day("2026-11-02")) {
		t.Fatalf("due: %v", due)
	}
	if _, ok := DueFrom(day("2026-11-01"), "unknown", targets); ok {
		t.Fatal("unknown key")
	}
	d := day("2026-11-10")
	ptr := func(s string) *time.Time { v := day(s); return &v }
	cases := []struct {
		done  *time.Time
		today string
		want  string
	}{
		{ptr("2026-11-10"), "2026-11-20", SLAMet},
		{ptr("2026-11-11"), "2026-11-20", SLABreached},
		{nil, "2026-11-11", SLABreached},
		{nil, "2026-11-09", SLAAtRisk},
		{nil, "2026-11-10", SLAAtRisk},
		{nil, "2026-11-05", SLAOnTrack},
	}
	for _, c := range cases {
		if got := SLAState(&d, c.done, day(c.today)); got != c.want {
			t.Errorf("done=%v today=%s: got %s want %s", c.done, c.today, got, c.want)
		}
	}
	if SLAState(nil, nil, d) != "" {
		t.Fatal("no due date, no state")
	}
	if r := SLARate([]string{SLAMet, SLABreached, SLAOnTrack, SLAMet}); r == nil || int(*r) != 66 {
		t.Fatalf("rate: %v", r)
	}
	if SLARate([]string{SLAOnTrack}) != nil {
		t.Fatal("nothing measurable yet")
	}
}

func TestTrendByMonth(t *testing.T) {
	rows := []InspectionRow{
		{Date: day("2026-09-03"), Score: 80, Passed: true},
		{Date: day("2026-09-20"), Score: 60},
		{Date: day("2026-11-01"), Score: 100, Passed: true},
	}
	got := TrendByMonth(rows, day("2026-09-15"), day("2026-11-30"))
	if len(got) != 3 || got[0].Count != 2 || got[0].Avg != 70 || got[0].PassRate != 50 || got[1].Avg != -1 || got[2].Month != "2026-11" {
		t.Fatalf("trend: %+v", got)
	}
}

func TestComposite(t *testing.T) {
	w := DefaultScoreWeights
	if got := Composite(ScoreParts{After: 90, During: 80, SLA: 70, FindingsOnTime: 60}, w); got != 78 {
		t.Fatalf("composite = %v", got)
	}
	// Unknown parts are left out and the rest rescaled.
	if got := Composite(ScoreParts{After: 90, During: -1, SLA: -1, FindingsOnTime: -1}, w); got != 90 {
		t.Fatalf("only after known = %v", got)
	}
	if Composite(ScoreParts{-1, -1, -1, -1}, w) != -1 {
		t.Fatal("nothing known")
	}
	r := Rank(map[int64]float64{1: 70, 2: 90, 3: -1})
	if r[0].ID != 2 || r[0].Rank != 1 || r[2].ID != 3 || r[2].Rank != 0 {
		t.Fatalf("rank: %+v", r)
	}
}
