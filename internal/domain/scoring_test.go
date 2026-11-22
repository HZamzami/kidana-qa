package domain

import (
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	// Eight criteria with mixed weights (15,15,20,15,10,10,5,10 = 100).
	mixed := func(r ...Rating) []Item {
		w := []int{15, 15, 20, 15, 10, 10, 5, 10}
		items := make([]Item, len(w))
		for i := range w {
			items[i] = Item{Weight: w[i]}
			if i < len(r) {
				items[i].Rating = r[i]
			}
		}
		return items
	}
	C, P, N, NA := RatingCompliant, RatingPartial, RatingNonCompliant, RatingNA

	tests := []struct {
		name     string
		items    []Item
		score    float64
		scored   bool
		passed   bool
		grade    string
		critical bool
		complete bool
	}{
		{"all compliant", mixed(C, C, C, C, C, C, C, C), 100, true, true, "ممتاز", false, true},
		{"one partial on K3", mixed(C, C, P, C, C, C, C, C), 90, true, true, "ممتاز", false, true},
		{"K3 fail drops below threshold", mixed(C, C, N, C, C, C, C, C), 80, true, true, "جيد جداً", false, true},
		{"two fails", mixed(C, N, N, C, C, C, C, C), 65, true, false, "مقبول", false, true},
		{"NA excluded from denominator", mixed(C, C, C, C, NA, NA, C, C), 100, true, true, "ممتاز", false, true},
		{"NA plus partial", mixed(P, C, C, C, NA, C, C, C), 91.7, true, true, "ممتاز", false, true},
		{"incomplete still scores rated items", mixed(C, N), 50, true, false, "ضعيف", false, false},
		{"nothing rated", mixed(), 0, false, false, "", false, false},
		{"all NA", mixed(NA, NA, NA, NA, NA, NA, NA, NA), 0, false, false, "", false, true},
		{
			"critical fail overrides a high score",
			[]Item{{Weight: 30, Critical: true, Rating: N}, {Weight: 70, Rating: C}, {Weight: 100, Rating: C}},
			85, true, false, "جيد جداً", true, true,
		},
		{
			"critical partial is not a critical fail",
			[]Item{{Weight: 30, Critical: true, Rating: P}, {Weight: 70, Rating: C}},
			85, true, true, "جيد جداً", false, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.items)
			if got.Scored != tt.scored || got.Score != tt.score || got.Passed != tt.passed ||
				got.Grade != tt.grade || got.CriticalFailed != tt.critical || got.Complete() != tt.complete {
				t.Fatalf("Evaluate() = %+v (complete=%v), want score=%v scored=%v passed=%v grade=%q critical=%v complete=%v",
					got, got.Complete(), tt.score, tt.scored, tt.passed, tt.grade, tt.critical, tt.complete)
			}
		})
	}
}

func TestGradeFor(t *testing.T) {
	cases := map[float64]string{100: "ممتاز", 90: "ممتاز", 89.9: "جيد جداً", 80: "جيد جداً", 79.9: "جيد", 70: "جيد", 60: "مقبول", 59.9: "ضعيف", 0: "ضعيف"}
	for score, want := range cases {
		if got := GradeFor(score); got != want {
			t.Errorf("GradeFor(%v) = %q, want %q", score, got, want)
		}
	}
}

func TestDelayDays(t *testing.T) {
	d := func(s string) *time.Time { v, _ := time.Parse("2006-01-02", s); return &v }
	if n, ok := DelayDays(d("2026-11-01"), d("2026-11-04")); !ok || n != 3 {
		t.Errorf("late: got %d %v", n, ok)
	}
	if n, ok := DelayDays(d("2026-11-04"), d("2026-11-01")); !ok || n != -3 {
		t.Errorf("early: got %d %v", n, ok)
	}
	if _, ok := DelayDays(nil, d("2026-11-01")); ok {
		t.Error("missing planned date should not be ok")
	}
}

func TestNextWOStatus(t *testing.T) {
	tests := []struct {
		name string
		in   AcceptanceInput
		want WOStatus
	}{
		{"open, nothing submitted", AcceptanceInput{Current: WOOpen}, WOOpen},
		{"QA submitted moves to in progress", AcceptanceInput{Current: WOOpen, HasSubmittedWork: true}, WOInProgress},
		{"passing QC, no findings => accepted", AcceptanceInput{Current: WOCompleted, HasSubmittedQC: true, LatestQCPassed: true, HasSubmittedWork: true}, WOAccepted},
		{"passing QC with open finding stays", AcceptanceInput{Current: WOCompleted, HasSubmittedQC: true, LatestQCPassed: true, OpenFindings: 1, HasSubmittedWork: true}, WOCompleted},
		{"failed latest QC", AcceptanceInput{Current: WOInProgress, HasSubmittedQC: true, HasSubmittedWork: true}, WOInProgress},
		{"accepted but finding reopened steps back", AcceptanceInput{Current: WOAccepted, HasSubmittedQC: true, LatestQCPassed: true, OpenFindings: 1, HasSubmittedWork: true}, WOCompleted},
		{"cancelled is sticky", AcceptanceInput{Current: WOCancelled, HasSubmittedQC: true, LatestQCPassed: true}, WOCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextWOStatus(tt.in); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestValidateSubmit(t *testing.T) {
	failing := Evaluate([]Item{{Weight: 10, Rating: RatingNonCompliant}})
	passing := Evaluate([]Item{{Weight: 10, Rating: RatingCompliant}})
	if errs := ValidateSubmit(SubmitCheck{Result: failing}); len(errs) != 1 {
		t.Errorf("failed inspection without finding: want 1 error, got %v", errs)
	}
	if errs := ValidateSubmit(SubmitCheck{Result: failing, FindingsLinked: 1}); len(errs) != 0 {
		t.Errorf("failed inspection with finding: want ok, got %v", errs)
	}
	if errs := ValidateSubmit(SubmitCheck{Result: passing, UnratedLabels: []string{"x"}}); len(errs) != 1 {
		t.Errorf("unrated: want 1 error, got %v", errs)
	}
	if errs := ValidateSubmit(SubmitCheck{Result: passing}); len(errs) != 0 {
		t.Errorf("passing: want ok, got %v", errs)
	}
}
