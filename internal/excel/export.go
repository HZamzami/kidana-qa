package excel

import (
	"fmt"
	"sort"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/HZamzami/kidana-qa/internal/dates"
	"github.com/HZamzami/kidana-qa/internal/domain"
)

const font = "Avenir Next Arabic" // brand font; Excel substitutes it if not installed

// Brand colours (ARGB without alpha for excelize fills).
const (
	colInk   = "4C4F53"
	colGold  = "B59666"
	colGreen = "627D4C"
	colZebra = "F6F6F6"
	colPass  = "E0E5DB"
	colFail  = "F2E1DD"
)

// InspectionExport is one submitted inspection in the export.
type InspectionExport struct {
	Ref, Type, Site, System, Asset, Location, Contractor, Technician string
	Stage, Template, Inspector                                       string
	Attempt                                                          int
	Date                                                             time.Time
	Score                                                            float64
	Passed, CriticalFailed                                           bool
	DelayDays                                                        *int
	Ratings                                                          map[string]domain.Rating // by criterion code
	Notes                                                            string
}

// CriterionColumn is one criterion column in the inspections sheet.
type CriterionColumn struct {
	Code, Label string
}

// FindingExport is one finding in the export.
type FindingExport struct {
	ID                                                 int64
	Ref, Site, Contractor, Criterion, Severity, Status string
	Description, CorrectiveAction, Owner, ClosureNote  string
	DueDate                                            *time.Time
	CreatedAt                                          time.Time
	ClosedAt                                           *time.Time
}

type styles struct {
	head, headGold, text, wrap, date, num, pass, fail, title, sub, bold int
}

func newStyles(f *excelize.File) (styles, error) {
	var s styles
	var err error
	border := []excelize.Border{
		{Type: "left", Color: "EDEDEE", Style: 1}, {Type: "right", Color: "EDEDEE", Style: 1},
		{Type: "top", Color: "EDEDEE", Style: 1}, {Type: "bottom", Color: "EDEDEE", Style: 1},
	}
	rtl := &excelize.Alignment{Vertical: "center", ReadingOrder: 2}
	mk := func(st *excelize.Style) int {
		if err != nil {
			return 0
		}
		var id int
		id, err = f.NewStyle(st)
		return id
	}
	headFill := func(c string) excelize.Fill { return excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{c}} }
	s.head = mk(&excelize.Style{Font: &excelize.Font{Family: font, Bold: true, Color: "FFFFFF", Size: 11}, Fill: headFill(colInk), Border: border,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true, ReadingOrder: 2}})
	s.headGold = mk(&excelize.Style{Font: &excelize.Font{Family: font, Bold: true, Color: "FFFFFF", Size: 11}, Fill: headFill(colGold), Border: border,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true, ReadingOrder: 2}})
	s.text = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 10.5}, Border: border, Alignment: rtl})
	s.wrap = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 10.5}, Border: border, Alignment: &excelize.Alignment{Vertical: "top", WrapText: true, ReadingOrder: 2}})
	s.date = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 10.5}, Border: border, CustomNumFmt: strPtr("dd/mm/yyyy"), Alignment: rtl})
	s.num = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 10.5}, Border: border, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}})
	s.pass = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 10.5, Bold: true, Color: colGreen}, Fill: headFill(colPass), Border: border, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}})
	s.fail = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 10.5, Bold: true, Color: colInk}, Fill: headFill(colFail), Border: border, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}})
	s.title = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 16, Bold: true, Color: colInk}, Alignment: rtl})
	s.sub = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 12, Bold: true, Color: colInk}, Border: []excelize.Border{{Type: "bottom", Color: colGold, Style: 2}}, Alignment: rtl})
	s.bold = mk(&excelize.Style{Font: &excelize.Font{Family: font, Size: 11, Bold: true}, Border: border, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}})
	return s, err
}

func strPtr(s string) *string { return &s }

func axis(col, row int) string {
	a, _ := excelize.CoordinatesToCellName(col, row)
	return a
}

func setRow(f *excelize.File, sheet string, row int, values []any, style int) {
	for i, v := range values {
		a := axis(i+1, row)
		_ = f.SetCellValue(sheet, a, v)
		_ = f.SetCellStyle(sheet, a, a, style)
	}
}

// Report builds the export workbook: summary, inspections and findings.
func Report(scope string, S domain.Summary, insights []domain.Insight, rows []InspectionExport, crits []CriterionColumn, findings []FindingExport) (*excelize.File, error) {
	f := excelize.NewFile()
	st, err := newStyles(f)
	if err != nil {
		return nil, err
	}
	if err := summarySheet(f, st, scope, S, insights); err != nil {
		return nil, err
	}
	if err := inspectionsSheet(f, st, rows, crits); err != nil {
		return nil, err
	}
	if err := findingsSheet(f, st, findings); err != nil {
		return nil, err
	}
	f.SetActiveSheet(0)
	return f, nil
}

func summarySheet(f *excelize.File, st styles, scope string, S domain.Summary, insights []domain.Insight) error {
	const sh = "ملخص"
	if err := f.SetSheetName("Sheet1", sh); err != nil {
		return err
	}
	rtl := true
	_ = f.SetSheetView(sh, 0, &excelize.ViewOptions{RightToLeft: &rtl, ShowGridLines: boolPtr(false)})
	_ = f.SetColWidth(sh, "A", "A", 34)
	_ = f.SetColWidth(sh, "B", "E", 15)

	r := 1
	_ = f.SetCellValue(sh, axis(1, r), "تقرير جودة الصيانة — رقابة أثناء التنفيذ وفحص بعد التنفيذ")
	_ = f.SetCellStyle(sh, axis(1, r), axis(1, r), st.title)
	r++
	_ = f.SetCellValue(sh, axis(1, r), fmt.Sprintf("إدارة جودة العمليات · تاريخ التقرير %s · النطاق: %s", dates.Both(time.Now().In(dates.Riyadh)), scope))
	r += 2

	title := func(t string) {
		_ = f.SetCellValue(sh, axis(1, r), t)
		_ = f.SetCellStyle(sh, axis(1, r), axis(5, r), st.sub)
		r++
	}
	title("المؤشرات الرئيسية")
	kpis := [][]any{
		{"الفحوصات المرسلة", S.Total}, {"رقابة أثناء التنفيذ (QA)", S.QACount}, {"فحص بعد التنفيذ (QC)", S.QCCount},
		{"أوامر العمل", S.WorkOrders}, {"متوسط QA %", round1(S.AvgQA)}, {"متوسط QC %", round1(S.AvgQC)},
		{"متوسط النتيجة %", round1(S.AvgScore)}, {"نسبة المطابقة %", round1(S.PassRate)},
		{"غير مطابق", S.FailCount}, {"إخفاق في بند حرج", S.CriticalFails},
		{"ملاحظات مفتوحة", S.Findings.Open}, {"ملاحظات متأخرة الإغلاق", S.Findings.Overdue},
		{"الالتزام بالموعد %", round1(S.OnTimeRate)}, {"متوسط التأخير (يوم)", round1(S.AvgDelay)},
	}
	for _, k := range kpis {
		setRow(f, sh, r, k[:1], st.text)
		_ = f.SetCellValue(sh, axis(2, r), k[1])
		_ = f.SetCellStyle(sh, axis(2, r), axis(2, r), st.bold)
		r++
	}
	r++

	group := func(name string, gs []domain.Group) {
		title(name)
		setRow(f, sh, r, []any{"البيان", "عدد الفحوصات", "متوسط النتيجة %", "نسبة المطابقة %", "غير مطابق"}, st.head)
		r++
		for _, g := range gs {
			setRow(f, sh, r, []any{g.Name}, st.text)
			setRow2(f, sh, r, 2, []any{g.Count, domain.Fmt1(g.Avg), domain.Fmt1(g.PassRate), g.Fails}, st.num)
			style := st.pass
			if g.Avg < domain.PassThreshold {
				style = st.fail
			}
			_ = f.SetCellStyle(sh, axis(3, r), axis(3, r), style)
			r++
		}
		if len(gs) == 0 {
			_ = f.SetCellValue(sh, axis(1, r), "لا توجد بيانات")
			r++
		}
		r++
	}
	group("الأداء حسب الموقع", S.BySite)
	group("الأداء حسب المقاول", S.ByContractor)
	group("الأداء حسب النظام", S.BySystem)

	title("نسبة المطابقة لكل معيار")
	setRow(f, sh, r, []any{"المعيار", "عدد التقييمات", "نسبة المطابقة %", "غير مطابق", "حرج"}, st.head)
	r++
	for _, c := range S.Criteria {
		setRow(f, sh, r, []any{c.Code + " - " + c.Label}, st.text)
		crit := ""
		if c.Critical {
			crit = "نعم"
		}
		setRow2(f, sh, r, 2, []any{c.N, domain.Fmt1(c.Compliance), c.Fails, crit}, st.num)
		style := st.pass
		if c.Compliance < domain.PassThreshold {
			style = st.fail
		}
		_ = f.SetCellStyle(sh, axis(3, r), axis(3, r), style)
		r++
	}
	r++
	title("الأنماط والمخاطر والتوصيات")
	for _, in := range insights {
		a := axis(1, r)
		_ = f.MergeCell(sh, a, axis(5, r))
		_ = f.SetCellValue(sh, a, "• "+in.Text)
		_ = f.SetCellStyle(sh, a, axis(5, r), st.wrap)
		_ = f.SetRowHeight(sh, r, float64(max(20, (len([]rune(in.Text))/80+1)*16)))
		r++
	}
	return nil
}

func setRow2(f *excelize.File, sh string, row, startCol int, values []any, style int) {
	for i, v := range values {
		a := axis(startCol+i, row)
		_ = f.SetCellValue(sh, a, v)
		_ = f.SetCellStyle(sh, a, a, style)
	}
}

func inspectionsSheet(f *excelize.File, st styles, rows []InspectionExport, crits []CriterionColumn) error {
	const sh = "الفحوصات"
	if _, err := f.NewSheet(sh); err != nil {
		return err
	}
	rtl := true
	_ = f.SetSheetView(sh, 0, &excelize.ViewOptions{RightToLeft: &rtl})
	headers := []string{"رقم أمر العمل", "النوع", "الموقع", "الموقع التفصيلي", "النظام", "الأصل / المعدة", "المقاول", "الفني",
		"المرحلة", "المحاولة", "تاريخ الفحص", "التاريخ الهجري", "المدقق", "قائمة الفحص", "النتيجة %", "التقدير", "الحالة", "بند حرج", "التأخير (يوم)", "ملاحظات"}
	base := len(headers)
	for _, c := range crits {
		headers = append(headers, c.Code+" - "+c.Label)
	}
	for i, h := range headers {
		a := axis(i+1, 1)
		_ = f.SetCellValue(sh, a, h)
		style := st.head
		if i >= base {
			style = st.headGold
		}
		_ = f.SetCellStyle(sh, a, a, style)
	}
	_ = f.SetRowHeight(sh, 1, 34)
	_ = f.SetColWidth(sh, "A", axisCol(len(headers)), 15)
	_ = f.SetColWidth(sh, "T", "T", 40)

	for i, r := range rows {
		row := i + 2
		result := "مطابق"
		if !r.Passed {
			result = "غير مطابق"
		}
		critical := ""
		if r.CriticalFailed {
			critical = "نعم"
		}
		var delay any = ""
		if r.DelayDays != nil {
			delay = *r.DelayDays
		}
		vals := []any{r.Ref, domain.Label(domain.WOTypeLabels, r.Type), r.Site, r.Location, r.System, r.Asset, r.Contractor, r.Technician,
			domain.Stage(r.Stage).Label(), r.Attempt, r.Date, dates.Hijri(r.Date), r.Inspector, r.Template, r.Score, domain.GradeFor(r.Score), result, critical, delay, r.Notes}
		for _, c := range crits {
			vals = append(vals, r.Ratings[c.Code].Label())
		}
		for ci, v := range vals {
			a := axis(ci+1, row)
			_ = f.SetCellValue(sh, a, v)
			style := st.text
			switch {
			case ci == 10:
				style = st.date
			case ci == 16 && r.Passed:
				style = st.pass
			case ci == 16:
				style = st.fail
			case ci == 9 || ci == 14 || ci == 18:
				style = st.num
			}
			_ = f.SetCellStyle(sh, a, a, style)
		}
	}
	_ = f.SetPanes(sh, &excelize.Panes{Freeze: true, XSplit: 1, YSplit: 1, TopLeftCell: "B2", ActivePane: "bottomRight"})
	return f.AutoFilter(sh, "A1:"+axis(len(headers), max(len(rows)+1, 2)), nil)
}

func findingsSheet(f *excelize.File, st styles, items []FindingExport) error {
	const sh = "الملاحظات"
	if _, err := f.NewSheet(sh); err != nil {
		return err
	}
	rtl := true
	_ = f.SetSheetView(sh, 0, &excelize.ViewOptions{RightToLeft: &rtl})
	headers := []string{"رقم", "أمر العمل", "الموقع", "المقاول", "المعيار", "الأهمية", "الحالة", "الوصف", "الإجراء التصحيحي", "المسؤول", "الإغلاق المستهدف", "الإغلاق المستهدف (هجري)", "تاريخ التسجيل", "تاريخ الإغلاق", "ملاحظة الإغلاق"}
	setRow(f, sh, 1, toAny(headers), st.head)
	_ = f.SetColWidth(sh, "A", "O", 15)
	_ = f.SetColWidth(sh, "H", "I", 40)
	for i, it := range items {
		row := i + 2
		vals := []any{it.ID, it.Ref, it.Site, it.Contractor, it.Criterion, domain.Label(domain.SeverityLabels, it.Severity),
			domain.Label(domain.FindingStatusLabels, it.Status), it.Description, it.CorrectiveAction, it.Owner,
			datePtr(it.DueDate), hijriPtr(it.DueDate), dates.DMY(it.CreatedAt.In(dates.Riyadh)), datePtr(it.ClosedAt), it.ClosureNote}
		for ci, v := range vals {
			a := axis(ci+1, row)
			_ = f.SetCellValue(sh, a, v)
			style := st.text
			if ci == 7 || ci == 8 || ci == 14 {
				style = st.wrap
			}
			_ = f.SetCellStyle(sh, a, a, style)
		}
	}
	return f.AutoFilter(sh, "A1:"+axis(len(headers), max(len(items)+1, 2)), nil)
}

func datePtr(t *time.Time) any {
	if t == nil {
		return ""
	}
	return dates.DMY(*t)
}

func hijriPtr(t *time.Time) any {
	if t == nil {
		return ""
	}
	return dates.Hijri(*t)
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func axisCol(n int) string {
	c, _ := excelize.ColumnNumberToName(n)
	return c
}

func round1(v *float64) any {
	if v == nil {
		return "—"
	}
	return domain.Fmt1(*v)
}

func boolPtr(b bool) *bool { return &b }

// SortCriteria orders criterion columns: QA codes, then QC codes, then any others.
func SortCriteria(cs []CriterionColumn) {
	rank := func(code string) int {
		switch {
		case len(code) > 0 && code[0] == 'Q':
			return 0
		case len(code) > 0 && code[0] == 'C':
			return 1
		}
		return 2
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if rank(cs[i].Code) != rank(cs[j].Code) {
			return rank(cs[i].Code) < rank(cs[j].Code)
		}
		return cs[i].Code < cs[j].Code
	})
}
