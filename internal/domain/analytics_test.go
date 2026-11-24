package domain

import (
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	late, early := 3, -1
	rows := []InspectionRow{
		{ID: 1, WorkOrderID: 10, Stage: StageDuring, Score: 95, Passed: true, Site: "منى", Contractor: "أ", Asset: "AC-1", DelayDays: &late},
		{ID: 2, WorkOrderID: 10, Stage: StageAfter, Score: 60, Passed: false, Site: "منى", Contractor: "أ", Asset: "AC-1", DelayDays: &late},
		{ID: 3, WorkOrderID: 11, Stage: StageAfter, Score: 70, Passed: false, CriticalFailed: true, Site: "عرفات", Contractor: "ب", Asset: "AC-1", DelayDays: &early},
	}
	items := []ItemRow{
		{InspectionID: 2, Stage: StageAfter, Code: "C1", Label: "جودة", Rating: RatingNonCompliant},
		{InspectionID: 3, Stage: StageAfter, Code: "C1", Label: "جودة", Rating: RatingPartial},
		{InspectionID: 3, Stage: StageAfter, Code: "C2", Label: "قائمة", Rating: RatingNA},
		{InspectionID: 1, Stage: StageDuring, Code: "Q1", Label: "سلامة", Rating: RatingCompliant},
	}
	S := Summarize(rows, items, FindingStats{Open: 2, Overdue: 1})

	if S.Total != 3 || S.DuringCount != 1 || S.AfterCount != 2 || S.PassCount != 1 || S.FailCount != 2 {
		t.Fatalf("counts wrong: %+v", S)
	}
	if S.WorkOrders != 2 || S.LateCount != 1 || S.OnTimeRate == nil || *S.OnTimeRate != 50 {
		t.Fatalf("timing wrong: wos=%d late=%d ontime=%v", S.WorkOrders, S.LateCount, S.OnTimeRate)
	}
	after := S.CriteriaFor(StageAfter)
	if len(after) != 1 || after[0].N != 2 || after[0].Compliance != 25 || after[0].Fails != 1 {
		t.Fatalf("after-work criteria wrong (NA must be excluded): %+v", after)
	}
	if len(S.Criteria) != 2 || S.Criteria[0].Stage != StageDuring {
		t.Fatalf("criteria must be split by stage, during first: %+v", S.Criteria)
	}
	if len(S.RepeatFailAssets) != 1 || S.RepeatFailAssets[0].Fails != 2 {
		t.Fatalf("repeat fails wrong: %+v", S.RepeatFailAssets)
	}
	if S.BySite[0].Name != "منى" || S.BySite[0].Count != 2 {
		t.Fatalf("by site wrong: %+v", S.BySite)
	}

	ins := Insights(S)
	joined := ""
	for _, i := range ins {
		joined += i.Text + "\n"
	}
	for _, want := range []string{"خطر سلامة", "أضعف معيار", "إخفاق متكرر", "تجاوزت تاريخ الإغلاق", "أعلى من نتيجة الفحص بعد التنفيذ"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing insight %q in:\n%s", want, joined)
		}
	}
}

func TestInsightsEmpty(t *testing.T) {
	if got := Insights(Summary{}); len(got) != 1 || got[0].Tone != "info" {
		t.Fatalf("got %+v", got)
	}
}
