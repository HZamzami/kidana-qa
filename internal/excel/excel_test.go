package excel

import (
	"bytes"
	"testing"

	"github.com/HZamzami/kidana-qa/internal/domain"
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
	_ = f.SetSheetRow(sh, "A3", &[]any{"", "أمر عمل", "منى"}) // missing ref + system → skipped
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	res, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindWorkOrders || len(res.WorkOrders) != 1 || res.Skipped != 1 {
		t.Fatalf("got kind=%v rows=%d skipped=%d warnings=%v", res.Kind, len(res.WorkOrders), res.Skipped, res.Warnings)
	}
	w := res.WorkOrders[0]
	if w.Ref != "WO-1" || w.Type != "WO" || w.Site != "منى" || w.Location != "مخيم 12" || w.System != "الكهرباء" ||
		w.Priority != "high" || w.PlannedDate == nil || w.PlannedDate.Format("2006-01-02") != "2026-11-01" {
		t.Fatalf("bad row: %+v", w)
	}
}

func TestReportBuilds(t *testing.T) {
	S := domain.Summarize([]domain.InspectionRow{{ID: 1, WorkOrderID: 1, Stage: domain.StageQC, Score: 90, Passed: true, Site: "منى"}}, nil, domain.FindingStats{})
	f, err := Report("الكل", S, domain.Insights(S), []InspectionExport{{Ref: "WO-1", Stage: "qc", Score: 90, Passed: true, Ratings: map[string]domain.Rating{"C1": "C"}}},
		[]CriterionColumn{{"C1", "جودة"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.GetSheetList(); len(got) != 3 {
		t.Fatalf("sheets = %v", got)
	}
}
