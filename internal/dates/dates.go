// Package dates formats dates the way Kidana uses them: Gregorian as
// dd/mm/yyyy, with the Hijri date (Umm al-Qura, the official Saudi calendar)
// shown alongside as read-only information.
//
// Storage and form values stay ISO (yyyy-mm-dd); this package is display only.
package dates

import (
	"fmt"
	"time"

	hijri "github.com/hablullah/go-hijri"

	"github.com/HZamzami/kidana-quality/internal/i18n"
)

// HijriMonths are the Umm al-Qura month names.
var HijriMonths = [12]string{
	"محرم", "صفر", "ربيع الأول", "ربيع الآخر", "جمادى الأولى", "جمادى الآخرة",
	"رجب", "شعبان", "رمضان", "شوال", "ذو القعدة", "ذو الحجة",
}

// DMY formats as 22/11/2026.
func DMY(t time.Time) string { return t.Format("02/01/2006") }

// DMYTime formats as 22/11/2026 14:05 in the Riyadh time zone.
func DMYTime(t time.Time) string { return t.In(Riyadh).Format("02/01/2006 15:04") }

// Riyadh is UTC+3 all year (no DST); fixed so it works without tzdata.
var Riyadh = time.FixedZone("AST", 3*60*60)

// HijriParts converts a Gregorian date to Umm al-Qura day, month, year.
// ok is false outside the supported range (1356–1500 AH, i.e. 1937–2077).
func HijriParts(t time.Time) (day, month, year int, ok bool) {
	d := time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)
	h, err := hijri.CreateUmmAlQuraDate(d)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(h.Day), int(h.Month), int(h.Year), true
}

// Hijri formats as 13/06/1448 هـ, or "" when out of range.
func Hijri(t time.Time) string {
	d, m, y, ok := HijriParts(t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%02d/%02d/%d هـ", d, m, y)
}

// HijriDigits formats as 13/06/1448 (no suffix), or "" when out of range.
// Views render it LTR and add "هـ" outside the number so bidi stays clean.
func HijriDigits(t time.Time) string {
	d, m, y, ok := HijriParts(t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%02d/%02d/%d", d, m, y)
}

// HijriLong formats as 13 جمادى الآخرة 1448 هـ, or "" when out of range.
func HijriLong(t time.Time) string {
	d, m, y, ok := HijriParts(t)
	if !ok || m < 1 || m > 12 {
		return ""
	}
	return fmt.Sprintf("%d %s %d هـ", d, HijriMonths[m-1], y)
}

// Both formats as "22/11/2026 — 13/06/1448 هـ".
func Both(t time.Time) string {
	if h := Hijri(t); h != "" {
		return DMY(t) + " — " + h
	}
	return DMY(t)
}

// HijriMonthsEn are the Umm al-Qura month names in English.
var HijriMonthsEn = [12]string{
	"Muharram", "Safar", "Rabi al-Awwal", "Rabi al-Thani", "Jumada al-Ula", "Jumada al-Akhirah",
	"Rajab", "Shaban", "Ramadan", "Shawwal", "Dhu al-Qadah", "Dhu al-Hijjah",
}

// HijriIn is Hijri in the given language: "13/06/1448 AH" in English.
func HijriIn(l i18n.Lang, t time.Time) string {
	if l != i18n.EN {
		return Hijri(t)
	}
	if h := HijriDigits(t); h != "" {
		return h + " AH"
	}
	return ""
}

// HijriLongIn is HijriLong in the given language: "13 Jumada al-Akhirah 1448 AH".
func HijriLongIn(l i18n.Lang, t time.Time) string {
	if l != i18n.EN {
		return HijriLong(t)
	}
	d, m, y, ok := HijriParts(t)
	if !ok || m < 1 || m > 12 {
		return ""
	}
	return fmt.Sprintf("%d %s %d AH", d, HijriMonthsEn[m-1], y)
}

// BothIn is Both in the given language.
func BothIn(l i18n.Lang, t time.Time) string {
	if h := HijriIn(l, t); h != "" {
		return DMY(t) + " — " + h
	}
	return DMY(t)
}

// Today is the current Riyadh calendar date as midnight UTC, the same form
// DATE columns come back in.
func Today() time.Time {
	y, m, d := time.Now().In(Riyadh).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
