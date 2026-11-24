package excel

import (
	_ "image/png" // excelize needs the PNG decoder registered to embed the logo
	"sort"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/i18n"
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
	Ref, Type, Site, System, AssetGroup, Asset, Location, Contractor, Technician string
	Stage, Template, Inspector                                                   string
	Attempt                                                                      int
	Date                                                                         time.Time
	Score                                                                        float64
	Passed, CriticalFailed                                                       bool
	DelayDays                                                                    *int
	Ratings                                                                      map[string]domain.Rating // by criterion code
	Notes                                                                        string
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

// ChecklistExport is an active checklist shown on the criteria sheet.
type ChecklistExport struct {
	Stage, Name string
	Version     int
	Criteria    []CriterionExport
}

// CriterionExport is one criterion of a checklist.
type CriterionExport struct {
	Code, Label, Description string
	Weight                   int
	Critical                 bool
}

// ReportInput is everything the export workbook contains.
type ReportInput struct {
	Scope      string
	Summary    domain.Summary
	Insights   []domain.Insight
	Rows       []InspectionExport
	Criteria   []CriterionColumn // rating columns on the inspections sheet
	Findings   []FindingExport
	Checklists []ChecklistExport // active checklists for the criteria sheet
	Logo       []byte            // PNG shown on the summary sheet (optional)
	Lang       i18n.Lang         // language of headings and labels
}

// Report builds the export workbook: summary, inspections, findings and criteria.
func Report(in ReportInput) (*excelize.File, error) {
	f := excelize.NewFile()
	st, err := newStyles(f)
	if err != nil {
		return nil, err
	}
	if err := summarySheet(in.Lang, f, st, in.Scope, in.Summary, in.Insights, in.Logo); err != nil {
		return nil, err
	}
	if err := inspectionsSheet(in.Lang, f, st, in.Rows, in.Criteria); err != nil {
		return nil, err
	}
	if err := findingsSheet(in.Lang, f, st, in.Findings); err != nil {
		return nil, err
	}
	if err := criteriaSheet(in.Lang, f, st, in.Checklists); err != nil {
		return nil, err
	}
	f.SetActiveSheet(0)
	return f, nil
}

func summarySheet(l i18n.Lang, f *excelize.File, st styles, scope string, S domain.Summary, insights []domain.Insight, logo []byte) error {
	sh := i18n.T(l, "ملخص")
	if err := f.SetSheetName("Sheet1", sh); err != nil {
		return err
	}
	rtl := l != i18n.EN
	_ = f.SetSheetView(sh, 0, &excelize.ViewOptions{RightToLeft: &rtl, ShowGridLines: boolPtr(false)})
	_ = f.SetColWidth(sh, "A", "A", 34)
	_ = f.SetColWidth(sh, "B", "E", 15)

	r := 1
	if len(logo) > 0 {
		// 1000×163 px logo scaled to ~280 px wide across the first three rows.
		if err := f.AddPictureFromBytes(sh, "A1", &excelize.Picture{
			Extension: ".png", File: logo,
			Format: &excelize.GraphicOptions{ScaleX: 0.28, ScaleY: 0.28, OffsetX: 4, OffsetY: 6, Positioning: "oneCell"},
		}); err == nil {
			for i := 1; i <= 3; i++ {
				_ = f.SetRowHeight(sh, i, 18)
			}
			r = 4
		}
	}
	_ = f.SetCellValue(sh, axis(1, r), i18n.T(l, "تقرير جودة الصيانة — فحص أثناء التنفيذ وفحص بعد التنفيذ"))
	_ = f.SetCellStyle(sh, axis(1, r), axis(1, r), st.title)
	r++
	_ = f.SetCellValue(sh, axis(1, r), i18n.Tf(l, "إدارة جودة العمليات · تاريخ التقرير %s · النطاق: %s", dates.BothIn(l, time.Now().In(dates.Riyadh)), scope))
	r += 2

	title := func(t string) {
		_ = f.SetCellValue(sh, axis(1, r), t)
		_ = f.SetCellStyle(sh, axis(1, r), axis(5, r), st.sub)
		r++
	}
	title(i18n.T(l, "المؤشرات الرئيسية"))
	kpis := [][]any{
		{i18n.T(l, "الفحوصات المرسلة"), S.Total}, {i18n.T(l, "فحص أثناء التنفيذ"), S.DuringCount}, {i18n.T(l, "فحص بعد التنفيذ"), S.AfterCount},
		{i18n.T(l, "أوامر العمل"), S.WorkOrders}, {i18n.T(l, "متوسط أثناء التنفيذ %"), round1(S.AvgDuring)}, {i18n.T(l, "متوسط بعد التنفيذ %"), round1(S.AvgAfter)},
		{i18n.T(l, "متوسط النتيجة %"), round1(S.AvgScore)}, {i18n.T(l, "نسبة المطابقة %"), round1(S.PassRate)},
		{i18n.T(l, "غير مطابق"), S.FailCount}, {i18n.T(l, "إخفاق في بند حرج"), S.CriticalFails},
		{i18n.T(l, "ملاحظات مفتوحة"), S.Findings.Open}, {i18n.T(l, "ملاحظات متأخرة الإغلاق"), S.Findings.Overdue},
		{i18n.T(l, "الالتزام بالموعد %"), round1(S.OnTimeRate)}, {i18n.T(l, "متوسط التأخير (يوم)"), round1(S.AvgDelay)},
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
		setRow(f, sh, r, []any{i18n.T(l, "البيان"), i18n.T(l, "عدد الفحوصات"), i18n.T(l, "متوسط النتيجة %"), i18n.T(l, "نسبة المطابقة %"), i18n.T(l, "غير مطابق")}, st.head)
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
			_ = f.SetCellValue(sh, axis(1, r), i18n.T(l, "لا توجد بيانات"))
			r++
		}
		r++
	}
	group(i18n.T(l, "الأداء حسب الموقع"), S.BySite)
	group(i18n.T(l, "الأداء حسب المقاول"), S.ByContractor)
	group(i18n.T(l, "الأداء حسب المنظومة"), S.BySystem)
	group(i18n.T(l, "الأداء حسب مجموعة الأصل"), S.ByAssetGroup)

	title(i18n.T(l, "نسبة المطابقة لكل معيار"))
	setRow(f, sh, r, []any{i18n.T(l, "المعيار"), i18n.T(l, "عدد التقييمات"), i18n.T(l, "نسبة المطابقة %"), i18n.T(l, "غير مطابق"), i18n.T(l, "حرج")}, st.head)
	r++
	for _, c := range S.Criteria {
		setRow(f, sh, r, []any{c.Code + " - " + c.Label}, st.text)
		crit := ""
		if c.Critical {
			crit = i18n.T(l, "نعم")
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
	title(i18n.T(l, "الأنماط والمخاطر والتوصيات"))
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

func inspectionsSheet(l i18n.Lang, f *excelize.File, st styles, rows []InspectionExport, crits []CriterionColumn) error {
	sh := i18n.T(l, "الفحوصات")
	if _, err := f.NewSheet(sh); err != nil {
		return err
	}
	rtl := l != i18n.EN
	_ = f.SetSheetView(sh, 0, &excelize.ViewOptions{RightToLeft: &rtl})
	headers := []string{i18n.T(l, "رقم أمر العمل"), i18n.T(l, "النوع"), i18n.T(l, "الموقع"), i18n.T(l, "الموقع التفصيلي"), i18n.T(l, "المنظومة"), i18n.T(l, "مجموعة الأصل"), i18n.T(l, "الأصل / المعدة"), i18n.T(l, "المقاول"), i18n.T(l, "الفني"),
		i18n.T(l, "المرحلة"), i18n.T(l, "المحاولة"), i18n.T(l, "تاريخ الفحص"), i18n.T(l, "التاريخ الهجري"), i18n.T(l, "المدقق"), i18n.T(l, "قائمة الفحص"), i18n.T(l, "النتيجة %"), i18n.T(l, "التقدير"), i18n.T(l, "الحالة"), i18n.T(l, "بند حرج"), i18n.T(l, "التأخير (يوم)"), i18n.T(l, "ملاحظات")}
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
	_ = f.SetColWidth(sh, "U", "U", 40) // notes

	for i, r := range rows {
		row := i + 2
		result := i18n.T(l, "مطابق")
		if !r.Passed {
			result = i18n.T(l, "غير مطابق")
		}
		critical := ""
		if r.CriticalFailed {
			critical = i18n.T(l, "نعم")
		}
		var delay any = ""
		if r.DelayDays != nil {
			delay = *r.DelayDays
		}
		vals := []any{r.Ref, i18n.T(l, domain.Label(domain.WOTypeLabels, r.Type)), r.Site, r.Location, r.System, r.AssetGroup, r.Asset, r.Contractor, r.Technician,
			i18n.T(l, domain.Stage(r.Stage).Label()), r.Attempt, r.Date, dates.HijriIn(l, r.Date), r.Inspector, r.Template, r.Score, i18n.T(l, domain.GradeFor(r.Score)), result, critical, delay, r.Notes}
		for _, c := range crits {
			vals = append(vals, i18n.T(l, r.Ratings[c.Code].Label()))
		}
		for ci, v := range vals {
			a := axis(ci+1, row)
			_ = f.SetCellValue(sh, a, v)
			style := st.text
			switch {
			case ci == 11:
				style = st.date
			case ci == 17 && r.Passed:
				style = st.pass
			case ci == 17:
				style = st.fail
			case ci == 10 || ci == 15 || ci == 19:
				style = st.num
			}
			_ = f.SetCellStyle(sh, a, a, style)
		}
	}
	_ = f.SetPanes(sh, &excelize.Panes{Freeze: true, XSplit: 1, YSplit: 1, TopLeftCell: "B2", ActivePane: "bottomRight"})
	return f.AutoFilter(sh, "A1:"+axis(len(headers), max(len(rows)+1, 2)), nil)
}

func findingsSheet(l i18n.Lang, f *excelize.File, st styles, items []FindingExport) error {
	sh := i18n.T(l, "الملاحظات")
	if _, err := f.NewSheet(sh); err != nil {
		return err
	}
	rtl := l != i18n.EN
	_ = f.SetSheetView(sh, 0, &excelize.ViewOptions{RightToLeft: &rtl})
	headers := []string{i18n.T(l, "رقم"), i18n.T(l, "أمر العمل"), i18n.T(l, "الموقع"), i18n.T(l, "المقاول"), i18n.T(l, "المعيار"), i18n.T(l, "الأهمية"), i18n.T(l, "الحالة"), i18n.T(l, "الوصف"), i18n.T(l, "الإجراء التصحيحي"), i18n.T(l, "المسؤول"), i18n.T(l, "الإغلاق المستهدف"), i18n.T(l, "الإغلاق المستهدف (هجري)"), i18n.T(l, "تاريخ التسجيل"), i18n.T(l, "تاريخ الإغلاق"), i18n.T(l, "ملاحظة الإغلاق")}
	setRow(f, sh, 1, toAny(headers), st.head)
	_ = f.SetColWidth(sh, "A", "O", 15)
	_ = f.SetColWidth(sh, "H", "I", 40)
	for i, it := range items {
		row := i + 2
		vals := []any{it.ID, it.Ref, it.Site, it.Contractor, it.Criterion, i18n.T(l, domain.Label(domain.SeverityLabels, it.Severity)),
			i18n.T(l, domain.Label(domain.FindingStatusLabels, it.Status)), it.Description, it.CorrectiveAction, it.Owner,
			datePtr(it.DueDate), hijriPtr(l, it.DueDate), dayOf(it.CreatedAt.In(dates.Riyadh)), datePtr(it.ClosedAt), it.ClosureNote}
		for ci, v := range vals {
			a := axis(ci+1, row)
			_ = f.SetCellValue(sh, a, v)
			style := st.text
			switch ci {
			case 7, 8, 14:
				style = st.wrap
			case 10, 12, 13:
				style = st.date
			}
			_ = f.SetCellStyle(sh, a, a, style)
		}
	}
	return f.AutoFilter(sh, "A1:"+axis(len(headers), max(len(items)+1, 2)), nil)
}

// datePtr returns a real date value (shown as dd/mm/yyyy by the date style), or "".
func datePtr(t *time.Time) any {
	if t == nil {
		return ""
	}
	return dayOf(*t)
}

// dayOf drops the time so Excel stores a whole-day date.
func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func hijriPtr(l i18n.Lang, t *time.Time) any {
	if t == nil {
		return ""
	}
	return dates.HijriIn(l, *t)
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

// SortCriteria orders criterion columns: during codes, then after codes, then any others.
func SortCriteria(cs []CriterionColumn) {
	rank := func(code string) int {
		switch {
		case len(code) > 0 && code[0] == 'D':
			return 0
		case len(code) > 0 && code[0] == 'A':
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

// criteriaSheet lists the active checklists and the scoring rules, so the
// export explains how every score was computed.
func criteriaSheet(l i18n.Lang, f *excelize.File, st styles, lists []ChecklistExport) error {
	sh := i18n.T(l, "المعايير")
	if _, err := f.NewSheet(sh); err != nil {
		return err
	}
	rtl := l != i18n.EN
	_ = f.SetSheetView(sh, 0, &excelize.ViewOptions{RightToLeft: &rtl, ShowGridLines: boolPtr(false)})
	_ = f.SetColWidth(sh, "A", "A", 8)
	_ = f.SetColWidth(sh, "B", "B", 34)
	_ = f.SetColWidth(sh, "C", "C", 80)
	_ = f.SetColWidth(sh, "D", "E", 10)

	r := 1
	for _, cl := range lists {
		_ = f.SetCellValue(sh, axis(1, r), i18n.Tf(l, "%s — %s (الإصدار %d)", i18n.T(l, domain.Stage(cl.Stage).Label()), cl.Name, cl.Version))
		_ = f.SetCellStyle(sh, axis(1, r), axis(5, r), st.sub)
		r++
		setRow(f, sh, r, []any{i18n.T(l, "الرمز"), i18n.T(l, "البند"), i18n.T(l, "الوصف / طريقة التحقق"), i18n.T(l, "الوزن"), i18n.T(l, "حرج")}, st.head)
		r++
		total := 0
		for _, c := range cl.Criteria {
			crit := ""
			if c.Critical {
				crit = i18n.T(l, "نعم")
			}
			setRow(f, sh, r, []any{c.Code, c.Label, c.Description}, st.wrap)
			setRow2(f, sh, r, 4, []any{c.Weight, crit}, st.num)
			total += c.Weight
			r++
		}
		setRow(f, sh, r, []any{"", i18n.T(l, "المجموع")}, st.bold)
		setRow2(f, sh, r, 4, []any{total}, st.bold)
		r += 2
	}

	_ = f.SetCellValue(sh, axis(1, r), i18n.T(l, "طريقة الحساب"))
	_ = f.SetCellStyle(sh, axis(1, r), axis(5, r), st.sub)
	r++
	rules := [][]any{
		{"", i18n.T(l, "النتيجة"), i18n.T(l, "مجموع (الوزن × معامل التقييم) ÷ مجموع أوزان البنود المنطبقة × 100")},
		{"", i18n.T(l, "مطابق"), i18n.T(l, "100% من وزن البند")},
		{"", i18n.T(l, "مطابق جزئياً"), i18n.T(l, "50% من وزن البند")},
		{"", i18n.T(l, "غير مطابق"), "0%"},
		{"", i18n.T(l, "لا ينطبق"), i18n.T(l, "يُستبعد من الحساب")},
		{"", i18n.T(l, "حد المطابقة"), i18n.Tf(l, "%s%% — وأي بند حرج غير مطابق يُسقط الفحص مهما كانت النتيجة", Fmt1(domain.PassThreshold))},
	}
	for _, g := range domain.Grades {
		rules = append(rules, []any{"", i18n.T(l, "التقدير: ") + g.Label, i18n.Tf(l, "من %s%%", Fmt1(g.Min))})
	}
	for _, row := range rules {
		setRow(f, sh, r, row, st.text)
		r++
	}
	return nil
}

// Fmt1 formats a number with at most one decimal (plain text for Excel).
func Fmt1(v float64) string { return domain.Fmt1(v) }

// FindingsWorkbook exports a filtered findings list on its own.
func FindingsWorkbook(l i18n.Lang, items []FindingExport) (*excelize.File, error) {
	f := excelize.NewFile()
	st, err := newStyles(f)
	if err != nil {
		return nil, err
	}
	if err := findingsSheet(l, f, st, items); err != nil {
		return nil, err
	}
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, err
	}
	f.SetActiveSheet(0)
	return f, nil
}
