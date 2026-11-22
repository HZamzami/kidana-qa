package excel

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// SheetWorkOrders is the data sheet of the work order import template.
const SheetWorkOrders = "أوامر العمل"

// Kind of an imported workbook.
type Kind int

const (
	KindUnknown Kind = iota
	KindWorkOrders
)

// WorkOrderRow is one row of the work order template.
type WorkOrderRow struct {
	Row                                                  int
	Ref, Type, Site, Location, System, Asset, Contractor string
	Technician, Priority, Description                    string
	PlannedDate, ActualDate                              *time.Time
}

// Result of parsing a workbook.
type Result struct {
	Kind       Kind
	Sheet      string
	WorkOrders []WorkOrderRow
	Skipped    int
	Warnings   []string
}

func (r *Result) warn(format string, args ...any) {
	if len(r.Warnings) < 300 {
		r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
	}
}

// ErrUnrecognised is returned when no known sheet/header layout is found.
var ErrUnrecognised = errors.New("unrecognised workbook layout")

// Parse reads a work order import workbook (the template, or any sheet with
// the same column titles).
func Parse(r io.Reader) (*Result, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, err
		}
		// Look for the header row in the first 10 rows.
		for hr := 0; hr < len(rows) && hr < 10; hr++ {
			h := newHeader(rows[hr])
			if h.find("رقم أمر العمل") >= 0 && h.find("الموقع") >= 0 && h.find("النظام") >= 0 {
				res := &Result{Kind: KindWorkOrders, Sheet: name}
				parseWorkOrders(res, h, rows[hr+1:], hr+2)
				return res, nil
			}
		}
	}
	return nil, ErrUnrecognised
}

func emptyRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func parseWorkOrders(res *Result, h header, rows [][]string, firstRow int) {
	col := map[string]int{
		"ref": h.find("رقم أمر العمل"), "type": h.find("النوع"), "site": h.find("الموقع"),
		"location": h.find("الموقع التفصيلي"), "system": h.find("النظام"), "asset": h.find("الأصل / المعدة", "الأصل"),
		"contractor": h.find("المقاول"), "technician": h.find("الفني"), "priority": h.find("الأولوية"),
		"planned": h.find("التاريخ المجدول"), "actual": h.find("تاريخ التنفيذ الفعلي"), "desc": h.find("وصف العمل"),
	}
	// "الموقع" also prefixes "الموقع التفصيلي"; make sure they are different columns.
	if col["site"] == col["location"] {
		col["site"] = h.find("الموقع *")
	}
	for i, row := range rows {
		n := firstRow + i
		if emptyRow(row) {
			continue
		}
		w := WorkOrderRow{
			Row: n, Ref: cell(row, col["ref"]), Site: cell(row, col["site"]), Location: cell(row, col["location"]),
			System: cell(row, col["system"]), Asset: cell(row, col["asset"]), Contractor: cell(row, col["contractor"]),
			Technician: cell(row, col["technician"]), Description: cell(row, col["desc"]),
		}
		var missing []string
		if w.Ref == "" {
			missing = append(missing, "رقم أمر العمل")
		}
		if w.Site == "" {
			missing = append(missing, "الموقع")
		}
		if w.System == "" {
			missing = append(missing, "النظام")
		}
		t, ok := ParseType(cell(row, col["type"]))
		if !ok || t == "" {
			missing = append(missing, "النوع")
		}
		if len(missing) > 0 {
			res.Skipped++
			res.warn("الصف %d: تم تجاهله — حقول إلزامية فارغة أو غير صالحة: %s", n, strings.Join(missing, "، "))
			continue
		}
		w.Type = t
		if p, ok := ParsePriority(cell(row, col["priority"])); ok {
			w.Priority = p
		} else {
			res.warn("الصف %d: أولوية غير معروفة «%s» — تُركت فارغة", n, cell(row, col["priority"]))
		}
		var okDate bool
		if w.PlannedDate, okDate = ParseDate(cell(row, col["planned"])); !okDate {
			res.warn("الصف %d: تاريخ مجدول غير صالح «%s»", n, cell(row, col["planned"]))
		}
		if w.ActualDate, okDate = ParseDate(cell(row, col["actual"])); !okDate {
			res.warn("الصف %d: تاريخ تنفيذ غير صالح «%s»", n, cell(row, col["actual"]))
		}
		res.WorkOrders = append(res.WorkOrders, w)
	}
}
