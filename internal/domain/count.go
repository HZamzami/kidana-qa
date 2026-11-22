package domain

import "fmt"

// Noun holds the forms Arabic needs to agree with a number.
type Noun struct {
	One  string // 1 and 0:     فحص
	Two  string // 2:           فحصان
	Few  string // 3–10:        فحوصات
	Many string // 11 and over: فحصاً (singular, accusative)
}

var (
	NounInspection = Noun{"فحص", "فحصان", "فحوصات", "فحصاً"}
	NounFinding    = Noun{"ملاحظة", "ملاحظتان", "ملاحظات", "ملاحظة"}
	NounWorkOrder  = Noun{"أمر عمل", "أمرا عمل", "أوامر عمل", "أمر عمل"}
	NounItem       = Noun{"بند", "بندان", "بنود", "بنداً"}
	NounDay        = Noun{"يوم", "يومان", "أيام", "يوماً"}
	NounRating     = Noun{"تقييم", "تقييمان", "تقييمات", "تقييماً"}
	NounCase       = Noun{"حالة", "حالتان", "حالات", "حالة"}
	NounUser       = Noun{"مستخدم", "مستخدمان", "مستخدمين", "مستخدماً"}
	NounEntry      = Noun{"عنصر", "عنصران", "عناصر", "عنصراً"}
	NounAttempt    = Noun{"محاولة", "محاولتان", "محاولات", "محاولة"}
)

// Count formats n with the correctly agreeing noun:
// 1 فحص، 2 فحصان، 5 فحوصات، 12 فحصاً. Digits stay Western (as in the CMMS).
// For 101–110 etc. Arabic follows the last two digits.
func Count(n int, w Noun) string {
	switch m := n % 100; {
	case n == 2:
		return w.Two
	case n == 1, n == 0:
		return fmt.Sprintf("%d %s", n, w.One)
	case m >= 3 && m <= 10:
		return fmt.Sprintf("%d %s", n, w.Few)
	case m == 1 || m == 2 || m == 0:
		return fmt.Sprintf("%d %s", n, w.One)
	default:
		return fmt.Sprintf("%d %s", n, w.Many)
	}
}
