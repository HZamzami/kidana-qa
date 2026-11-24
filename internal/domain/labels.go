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

var FindingStatusOrder = []string{"open", "in_progress", "closed"}

var FindingStatusLabels = map[string]string{
	"open":        "مفتوحة",
	"in_progress": "قيد المعالجة",
	"closed":      "مغلقة",
}

var SeverityLabels = map[string]string{
	"normal":   "عادية",
	"critical": "حرجة",
}

var RoleOrder = []string{"admin", "inspector", "viewer"}

var RoleLabels = map[string]string{
	"admin":     "مدير النظام",
	"inspector": "مدقق",
	"viewer":    "مشاهد",
}

// Label returns m[key], or key itself when there is no label.
func Label(m map[string]string, key string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return key
}
