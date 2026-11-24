package excel

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/HZamzami/kidana-quality/internal/domain"
)

// woColumns of the work order import template, in order.
var woColumns = []struct {
	Title    string
	Required bool
	Width    float64
}{
	{"رقم أمر العمل", true, 18},
	{"النوع", true, 14},
	{"الموقع", true, 16},
	{"الموقع التفصيلي", false, 22},
	{"المنظومة", true, 18},
	{"مجموعة الأصل", true, 20},
	{"الأصل / المعدة", false, 20},
	{"المقاول", false, 20},
	{"الفني", false, 16},
	{"الأولوية", false, 11},
	{"التاريخ المجدول / استحقاق SLA", false, 18},
	{"تاريخ التنفيذ الفعلي", false, 16},
	{"وصف العمل", false, 40},
}

// WorkOrderTemplate builds the bulk work order import template with
// drop-down lists for the reference values.
func WorkOrderTemplate(sites, systems, assetGroups, contractors []string) (*excelize.File, error) {
	f := excelize.NewFile()
	st, err := newStyles(f)
	if err != nil {
		return nil, err
	}
	sh := SheetWorkOrders
	if err := f.SetSheetName("Sheet1", sh); err != nil {
		return nil, err
	}
	rtl := true
	_ = f.SetSheetView(sh, 0, &excelize.ViewOptions{RightToLeft: &rtl})
	for i, c := range woColumns {
		a := axis(i+1, 1)
		title := c.Title
		if c.Required {
			title += " *"
		}
		_ = f.SetCellValue(sh, a, title)
		_ = f.SetCellStyle(sh, a, a, st.head)
		col := axisCol(i + 1)
		_ = f.SetColWidth(sh, col, col, c.Width)
	}
	_ = f.SetRowHeight(sh, 1, 34)
	_ = f.SetPanes(sh, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	// Hidden sheet with the lists referenced by data validation.
	const lists = "_lists"
	if _, err := f.NewSheet(lists); err != nil {
		return nil, err
	}
	columns := map[string][]string{
		"A": {domain.WOTypeLabels["PM"], domain.WOTypeLabels["WO"]},
		"B": sites, "C": systems, "D": assetGroups, "E": contractors,
		"F": {domain.PriorityLabels["normal"], domain.PriorityLabels["high"], domain.PriorityLabels["emergency"]},
	}
	for col, vals := range columns {
		for i, v := range vals {
			_ = f.SetCellValue(lists, fmt.Sprintf("%s%d", col, i+1), v)
		}
	}
	_ = f.SetSheetVisible(lists, false)

	const last = 2000
	addList := func(col, listCol string, n int, strict bool) {
		if n == 0 {
			return
		}
		dv := excelize.NewDataValidation(true)
		dv.Sqref = fmt.Sprintf("%s2:%s%d", col, col, last)
		dv.SetSqrefDropList(fmt.Sprintf("%s!$%s$1:$%s$%d", lists, listCol, listCol, n))
		if strict {
			dv.SetError(excelize.DataValidationErrorStyleStop, "قيمة غير صحيحة", "اختر من القائمة")
		} else {
			// sites/systems/asset groups/contractors accept new names (they are created on import)
			dv.SetError(excelize.DataValidationErrorStyleInformation, "قيمة جديدة", "سيُضاف الاسم الجديد عند الاستيراد")
		}
		_ = f.AddDataValidation(sh, dv)
	}
	// template column ← list column: type, site, system, asset group, contractor, priority
	addList("B", "A", 2, true)
	addList("C", "B", len(sites), false)
	addList("E", "C", len(systems), false)
	addList("F", "D", len(assetGroups), false)
	addList("H", "E", len(contractors), false)
	addList("J", "F", len(domain.PriorityOrder), true)

	// Date columns hold real Excel dates shown as dd/mm/yyyy and validated like
	// the drop-down columns: anything that is not a date between 2000 and 2099
	// is rejected. The import reads the stored date value, whatever the locale.
	dateStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr("dd/mm/yyyy")})
	_ = f.SetColStyle(sh, "K:L", dateStyle)
	dv := excelize.NewDataValidation(true)
	dv.Sqref = fmt.Sprintf("K2:L%d", last)
	if err := dv.SetRange(36526, 73050, excelize.DataValidationTypeDate, excelize.DataValidationOperatorBetween); err == nil { // 2000-01-01 … 2099-12-31
		dv.SetError(excelize.DataValidationErrorStyleStop, "تاريخ غير صالح", "أدخل تاريخاً صحيحاً بصيغة dd/mm/yyyy، مثال 18/11/2026")
		dv.SetInput("التاريخ", "dd/mm/yyyy — تأكد أن التاريخ المعروض يطابق ما تقصده")
		_ = f.AddDataValidation(sh, dv)
	}

	// Instructions
	const help = "التعليمات"
	if _, err := f.NewSheet(help); err != nil {
		return nil, err
	}
	_ = f.SetSheetView(help, 0, &excelize.ViewOptions{RightToLeft: &rtl})
	_ = f.SetColWidth(help, "A", "A", 110)
	lines := []string{
		"استيراد أوامر العمل — نظام جودة الصيانة",
		"1) صف لكل أمر عمل في ورقة «أوامر العمل» بدءاً من الصف الثاني. الأعمدة المعلّمة بـ * إلزامية.",
		"2) لا تغيّر عناوين الأعمدة — الاستيراد يعتمد عليها.",
		"3) أعمدة التاريخ خلايا تاريخ حقيقية بصيغة dd/mm/yyyy (مثال 18/11/2026)، ويرفض Excel أي قيمة ليست تاريخاً. تأكد أن التاريخ المعروض يطابق ما تقصده. التاريخ الهجري يُعرض في النظام تلقائياً ولا يُدخل هنا.",
		"4) أوامر العمل الموجودة مسبقاً (بنفس الرقم) تُتجاهل ولا تُعدَّل.",
		"5) المواقع والمنظومات ومجموعات الأصول والمقاولون غير المسجلين يُضافون تلقائياً — راجعهم بعد الاستيراد من الإعدادات.",
		"6) الأولوية: عادي أو عالي أو طارئ. إن تُركت فارغة تُعتبر «عادي».",
	}
	for i, l := range lines {
		_ = f.SetCellValue(help, fmt.Sprintf("A%d", i+1), l)
	}
	_ = f.SetCellStyle(help, "A1", "A1", st.title)
	f.SetActiveSheet(0)
	return f, nil
}
