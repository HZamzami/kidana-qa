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

// AuditEntityLabels name what a change-history entry is about.
var AuditEntityLabels = map[string]string{ //nolint:gosec // labels, not credentials
	"work_order": "أمر العمل", "inspection": "فحص", "finding": "ملاحظة", "assignment": "تكليف",
	"attachment": "صورة", "asset": "أصل", "user": "مستخدم", "sampling_rule": "قاعدة عينة",
	"checklist_template": "قائمة فحص", "import": "استيراد", "sites": "موقع", "systems": "منظومة",
	"asset-groups": "مجموعة أصل", "contractors": "مقاول", "sla": "زمن الحل", "api_token": "مفتاح API",
}

// AuditActionLabels describe what happened in a change-history entry.
var AuditActionLabels = map[string]string{ //nolint:gosec // labels, not credentials
	"created": "أُنشئ", "updated": "عُدّل", "deleted": "حُذف", "started": "بدأ", "submitted": "أُرسل",
	"reopened": "أُعيد فتحه", "draft_deleted": "حُذفت المسودة", "closed": "أُغلقت", "verified": "تم التحقق وأُغلقت",
	"responded": "ردّ المقاول", "response_rejected": "لم يُقبل الرد", "bulk_updated": "تعديل جماعي",
	"status_auto": "تغيّرت الحالة تلقائياً", "claimed": "أُخذ التكليف", "cancelled": "أُلغي", "uploaded": "رُفعت",
	"annotated": "رُسم عليها", "login": "دخول", "login_sso": "دخول موحّد", "login_recovery_code": "دخول برمز استرداد",
	"password_changed": "غيّر كلمة المرور", "password_reset": "كلمة مرور مؤقتة", "password_reset_link": "رابط إعادة تعيين",
	"password_reset_used": "استُخدم رابط إعادة التعيين", "password_reset_requested": "طلب إعادة تعيين",
	"2fa_enabled": "فعّل التحقق بخطوتين", "2fa_disabled": "أوقف التحقق بخطوتين", "2fa_reset_by_admin": "أوقف المدير التحقق بخطوتين",
	"2fa_recovery_codes": "رموز استرداد جديدة", "excel_import": "استيراد Excel", "used": "استُخدم",
}

// AuditFieldLabels name the fields shown in a change's details.
var AuditFieldLabels = map[string]string{ //nolint:gosec // labels, not credentials
	"ref": "رقم أمر العمل", "type": "النوع", "site": "الموقع", "location": "الموقع التفصيلي", "system": "المنظومة",
	"asset_group": "مجموعة الأصل", "asset": "الأصل", "contractor": "المقاول", "technician": "الفني", "priority": "الأولوية",
	"description": "الوصف", "planned_date": "التاريخ المجدول", "actual_date": "التنفيذ الفعلي", "status": "الحالة",
	"corrective_action": "الإجراء التصحيحي", "owner": "المسؤول", "owner_user": "مسؤول المقاول", "due_date": "الإغلاق المستهدف",
	"severity": "الأهمية", "root_cause_category": "السبب الجذري", "root_cause": "تحليل السبب الجذري",
	"from": "من", "to": "إلى", "score": "النتيجة", "passed": "مطابق", "reason": "السبب", "stage": "المرحلة",
}
