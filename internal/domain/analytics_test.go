package domain

import (
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	late, early := 3, -1
	rows := []InspectionRow{
		{ID: 1, WorkOrderID: 10, Stage: StageQA, Score: 95, Passed: true, Site: "منى", Contractor: "أ", Asset: "AC-1", DelayDays: &late},
		{ID: 2, WorkOrderID: 10, Stage: StageQC, Score: 60, Passed: false, Site: "منى", Contractor: "أ", Asset: "AC-1", DelayDays: &late},
		{ID: 3, WorkOrderID: 11, Stage: StageQC, Score: 70, Passed: false, CriticalFailed: true, Site: "عرفات", Contractor: "ب", Asset: "AC-1", DelayDays: &early},
	}
	items := []ItemRow{
		{InspectionID: 2, Code: "C1", Label: "جودة", Rating: RatingNonCompliant},
		{InspectionID: 3, Code: "C1", Label: "جودة", Rating: RatingPartial},
		{InspectionID: 3, Code: "C2", Label: "قائمة", Rating: RatingNA},
	}
	S := Summarize(rows, items, FindingStats{Open: 2, Overdue: 1})

	if S.Total != 3 || S.QACount != 1 || S.QCCount != 2 || S.PassCount != 1 || S.FailCount != 2 {
		t.Fatalf("counts wrong: %+v", S)
	}
	if S.WorkOrders != 2 || S.LateCount != 1 || S.OnTimeRate == nil || *S.OnTimeRate != 50 {
		t.Fatalf("timing wrong: wos=%d late=%d ontime=%v", S.WorkOrders, S.LateCount, S.OnTimeRate)
	}
	if len(S.Criteria) != 1 || S.Criteria[0].N != 2 || S.Criteria[0].Compliance != 25 || S.Criteria[0].Fails != 1 {
		t.Fatalf("criteria wrong (NA must be excluded): %+v", S.Criteria)
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
