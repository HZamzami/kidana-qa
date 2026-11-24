package domain

import (
	"fmt"

	"github.com/HZamzami/kidana-quality/internal/i18n"
)

// Noun holds the forms Arabic needs to agree with a number, plus the English
// singular and plural.
type Noun struct {
	One  string // 1 and 0:     فحص
	Two  string // 2:           فحصان
	Few  string // 3–10:        فحوصات
	Many string // 11 and over: فحصاً (singular, accusative)

	EnOne, EnMany string // inspection, inspections
}

var (
	NounInspection = Noun{"فحص", "فحصان", "فحوصات", "فحصاً", "inspection", "inspections"}
	NounFinding    = Noun{"ملاحظة", "ملاحظتان", "ملاحظات", "ملاحظة", "finding", "findings"}
	NounWorkOrder  = Noun{"أمر عمل", "أمرا عمل", "أوامر عمل", "أمر عمل", "work order", "work orders"}
	NounItem       = Noun{"بند", "بندان", "بنود", "بنداً", "item", "items"}
	NounDay        = Noun{"يوم", "يومان", "أيام", "يوماً", "day", "days"}
	NounRating     = Noun{"تقييم", "تقييمان", "تقييمات", "تقييماً", "rating", "ratings"}
	NounCase       = Noun{"حالة", "حالتان", "حالات", "حالة", "case", "cases"}
	NounUser       = Noun{"مستخدم", "مستخدمان", "مستخدمين", "مستخدماً", "user", "users"}
	NounEntry      = Noun{"عنصر", "عنصران", "عناصر", "عنصراً", "entry", "entries"}
	NounAttempt    = Noun{"محاولة", "محاولتان", "محاولات", "محاولة", "attempt", "attempts"}
)

// CountIn is Count in the given language: "5 inspections" in English.
func CountIn(l i18n.Lang, n int, w Noun) string {
	if l != i18n.EN {
		return Count(n, w)
	}
	if n == 1 {
		return "1 " + w.EnOne
	}
	return fmt.Sprintf("%d %s", n, w.EnMany)
}

// DelayPhrase describes a work order's delay in days (positive = late) in
// natural wording: «متأخر يوماً واحداً», «متأخر يومين», «متأخر 5 أيام».
func DelayPhrase(l i18n.Lang, d int) string {
	if l == i18n.EN {
		switch {
		case d == 0:
			return "On time"
		case d == 1:
			return "1 day late"
		case d > 1:
			return fmt.Sprintf("%d days late", d)
		case d == -1:
			return "1 day early"
		}
		return fmt.Sprintf("%d days early", -d)
	}
	word, n := "متأخر", d
	if d < 0 {
		word, n = "مبكر", -d
	}
	switch n {
	case 0:
		return "في الموعد"
	case 1:
		return word + " يوماً واحداً"
	case 2:
		return word + " يومين"
	}
	return word + " " + Count(n, NounDay)
}

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
