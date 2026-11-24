package domain

// Arabic labels for enum values stored in the database. Keeping them here
// (not in templates) means the UI, Excel export and reports always agree.

var WOTypeLabels = map[string]string{
	"PM": "صيانة وقائية",
	"WO": "أمر عمل",
}

var PriorityOrder = []string{"normal", "high", "emergency"}

var PriorityLabels = map[string]string{
	"normal":    "عادي",
	"high":      "عالي",
	"emergency": "طارئ",
}

var FindingStatusOrder = []string{"open", "in_progress", "responded", "closed"}

var FindingStatusLabels = map[string]string{
	"open":        "مفتوحة",
	"in_progress": "قيد المعالجة",
	"responded":   "بانتظار التحقق",
	"closed":      "مغلقة",
}

// RootCauseOrder lists finding root-cause categories in display order.
var RootCauseOrder = []string{"people", "procedure", "materials", "equipment", "supervision", "design", "external", "other"}

var RootCauseLabels = map[string]string{
	"people":      "الأفراد والتدريب",
	"procedure":   "الإجراء",
	"materials":   "المواد وقطع الغيار",
	"equipment":   "المعدات والأدوات",
	"supervision": "الإشراف",
	"design":      "التصميم والموقع",
	"external":    "جهة خارجية",
	"other":       "أخرى",
}

var SeverityLabels = map[string]string{
	"normal":   "عادية",
	"critical": "حرجة",
}

var RoleOrder = []string{"admin", "inspector", "viewer", "contractor"}

var RoleLabels = map[string]string{
	"admin":      "مدير النظام",
	"inspector":  "مدقق",
	"viewer":     "مشاهد",
	"contractor": "مقاول",
}

// Label returns m[key], or key itself when there is no label.
func Label(m map[string]string, key string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return key
}
