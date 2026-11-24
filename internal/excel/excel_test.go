package excel

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/HZamzami/kidana-quality/internal/domain"
)

func TestParseDate(t *testing.T) {
	cases := map[string]string{
		"2026-11-18": "2026-11-18",
		"18/11/2026": "2026-11-18",
		"١٨/١١/٢٠٢٦": "2026-11-18",
		"46344":      "2026-11-18", // Excel serial
		"":           "",
	}
	for in, want := range cases {
		got, ok := ParseDate(in)
		if !ok {
			t.Errorf("ParseDate(%q) not ok", in)
			continue
		}
		s := ""
		if got != nil {
			s = got.Format("2006-01-02")
		}
		if s != want {
			t.Errorf("ParseDate(%q) = %q, want %q", in, s, want)
		}
	}
	for _, bad := range []string{"2026-02-30", "tomorrow"} {
		if _, ok := ParseDate(bad); ok {
			t.Errorf("ParseDate(%q) should fail", bad)
		}
	}
}

func TestParseRatingAndType(t *testing.T) {
	ratings := map[string]domain.Rating{"مطابق": "C", "مطابق جزئياً": "P", "غير مطابق": "N", "لا ينطبق": "NA", "✓": "C", "n/a": "NA"}
	for in, want := range ratings {
		if got, ok := ParseRating(in); !ok || got != want {
			t.Errorf("ParseRating(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := ParseRating("ربما"); ok {
		t.Error("unknown rating should fail")
	}
	types := map[string]string{"صيانة وقائية": "PM", "أمر عمل": "WO", "PM": "PM", "بلاغ": "WO"}
	for in, want := range types {
		if got, ok := ParseType(in); !ok || got != want {
			t.Errorf("ParseType(%q) = %q want %q", in, got, want)
		}
	}
}

func TestTemplateRoundTrip(t *testing.T) {
	f, err := WorkOrderTemplate([]string{"منى"}, []string{"الكهرباء"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sh := SheetWorkOrders
	_ = f.SetSheetRow(sh, "A2", &[]any{"WO-1", "أمر عمل", "منى", "مخيم 12", "الكهرباء", "لوحة 3", "مؤسسة الريان", "", "عالي", "2026-11-01", "2026-11-03", "استبدال قاطع"})
	_ = f.SetSheetRow(sh, "A3", &[]any{"", "أمر عمل", "منى"})                                                                                        // missing ref + system → skipped
	_ = f.SetSheetRow(sh, "A4", &[]any{"WO-2", "صيانة وقائية", "منى", "", "الكهرباء", "", "", "", "", time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)}) // real date cell
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	res, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindWorkOrders || len(res.WorkOrders) != 2 || res.Skipped != 1 {
		t.Fatalf("got kind=%v rows=%d skipped=%d warnings=%v", res.Kind, len(res.WorkOrders), res.Skipped, res.Warnings)
	}
	w := res.WorkOrders[0]
	if w.Ref != "WO-1" || w.Type != "WO" || w.Site != "منى" || w.Location != "مخيم 12" || w.System != "الكهرباء" ||
		w.Priority != "high" || w.PlannedDate == nil || w.PlannedDate.Format("2006-01-02") != "2026-11-01" {
		t.Fatalf("bad row: %+v", w)
	}
	if d := res.WorkOrders[1].PlannedDate; d == nil || d.Format("2006-01-02") != "2026-11-05" {
		t.Fatalf("real date cell not parsed: %v", d)
	}
}

func TestReportBuilds(t *testing.T) {
	S := domain.Summarize([]domain.InspectionRow{{ID: 1, WorkOrderID: 1, Stage: domain.StageQC, Score: 90, Passed: true, Site: "منى"}}, nil, domain.FindingStats{})
	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	f, err := Report(ReportInput{
		Scope: "الكل", Summary: S, Insights: domain.Insights(S),
		Rows:       []InspectionExport{{Ref: "WO-1", Stage: "qc", Score: 90, Passed: true, Ratings: map[string]domain.Rating{"C1": "C"}}},
		Criteria:   []CriterionColumn{{"C1", "جودة"}},
		Findings:   []FindingExport{{ID: 1, Ref: "WO-1", DueDate: &due, CreatedAt: due}},
		Checklists: []ChecklistExport{{Stage: "qc", Name: "فحص", Version: 1, Criteria: []CriterionExport{{Code: "C1", Label: "جودة", Weight: 100}}}},
		Logo:       png1x1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.GetSheetList(); len(got) != 4 || got[3] != "المعايير" {
		t.Fatalf("sheets = %v", got)
	}
	if pics, err := f.GetPictures("ملخص", "A1"); err != nil || len(pics) != 1 {
		t.Fatalf("logo missing: %v %v", len(pics), err)
	}
	// Due date is a real date cell, not text.
	if typ, _ := f.GetCellType("الملاحظات", "K2"); typ == excelize.CellTypeSharedString || typ == excelize.CellTypeInlineString {
		t.Fatalf("due date stored as text (%v)", typ)
	}
}

// 1x1 transparent PNG
var png1x1 = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 0x49, 0x48, 0x44, 0x52, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89, 0, 0, 0, 0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0, 1, 0, 0, 5, 0, 1, 0x0d, 0x0a, 0x2d, 0xb4, 0, 0, 0, 0, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}
