// Package excel builds and reads the .xlsx files: the report export, the work
// order import template, and the work order import.
package excel

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/i18n"
)

var (
	diacritics = regexp.MustCompile(`[\x{064B}-\x{0652}\x{0640}]`)
	spaces     = regexp.MustCompile(`\s+`)
	isoDate    = regexp.MustCompile(`^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})`)
	dmyDate    = regexp.MustCompile(`^(\d{1,2})[-/.](\d{1,2})[-/.](\d{4})`)
)

// Norm normalises Arabic text for tolerant matching: strips diacritics and
// tatweel, unifies alef, yaa and taa-marbuta forms, collapses spaces, lowercases.
func Norm(s string) string {
	s = diacritics.ReplaceAllString(s, "")
	s = strings.NewReplacer("أ", "ا", "إ", "ا", "آ", "ا", "ى", "ي", "ة", "ه", "*", "").Replace(s)
	s = spaces.ReplaceAllString(s, " ")
	return strings.ToLower(strings.TrimSpace(s))
}

// ParseDate accepts yyyy-mm-dd, dd/mm/yyyy (and . or - separators), Arabic
// digits, and Excel serial numbers. ok is false for a non-empty invalid value.
func ParseDate(v string) (t *time.Time, ok bool) {
	v = domain.ASCIIDigits(v)
	if v == "" {
		return nil, true
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil && n > 20000 && n < 80000 {
		// Excel serial date (days since 1899-12-30)
		d := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(math.Floor(n)))
		return &d, true
	}
	if m := isoDate.FindStringSubmatch(v); m != nil {
		return validDate(m[1], m[2], m[3])
	}
	if m := dmyDate.FindStringSubmatch(v); m != nil {
		return validDate(m[3], m[2], m[1])
	}
	return nil, false
}

func validDate(y, m, d string) (*time.Time, bool) {
	yy, _ := strconv.Atoi(y)
	mm, _ := strconv.Atoi(m)
	dd, _ := strconv.Atoi(d)
	t := time.Date(yy, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	if t.Year() != yy || int(t.Month()) != mm || t.Day() != dd {
		return nil, false
	}
	return &t, true
}

var ratingAliases = map[string]domain.Rating{
	"مطابق": domain.RatingCompliant, "نعم": domain.RatingCompliant, "✓": domain.RatingCompliant, "1": domain.RatingCompliant,
	"100%": domain.RatingCompliant, "c": domain.RatingCompliant, "pass": domain.RatingCompliant,
	"مطابق جزئيا": domain.RatingPartial, "مطابق جزئي": domain.RatingPartial, "جزئي": domain.RatingPartial, "جزئيا": domain.RatingPartial,
	"p": domain.RatingPartial, "0.5": domain.RatingPartial, "50%": domain.RatingPartial,
	"غير مطابق": domain.RatingNonCompliant, "لا": domain.RatingNonCompliant, "x": domain.RatingNonCompliant, "✗": domain.RatingNonCompliant,
	"0": domain.RatingNonCompliant, "n": domain.RatingNonCompliant, "fail": domain.RatingNonCompliant,
	"لا ينطبق": domain.RatingNA, "غير منطبق": domain.RatingNA, "n/a": domain.RatingNA, "na": domain.RatingNA, "-": domain.RatingNA,
}

// ParseRating maps the many ways people write a rating. ok is false for a
// non-empty unknown value; an empty value returns "" and ok.
func ParseRating(v string) (domain.Rating, bool) {
	s := Norm(v)
	if s == "" {
		return "", true
	}
	if r, ok := ratingAliases[s]; ok {
		return r, true
	}
	return "", false
}

var (
	pmPattern = regexp.MustCompile(`وقائ|pm|preventive`)
	woPattern = regexp.MustCompile(`امر|بلاغ|تصحيح|wo|corrective|work`)
)

// ParseType returns "PM" or "WO"; ok is false for an unknown non-empty value.
func ParseType(v string) (string, bool) {
	s := Norm(v)
	switch {
	case s == "":
		return "", true
	case pmPattern.MatchString(s):
		return "PM", true
	case woPattern.MatchString(s):
		return "WO", true
	}
	return "", false
}

// priorityAliases maps spellings people type to the enum. The retired levels
// (متوسط، منخفض) fold into "normal" so older sheets still import.
var priorityAliases = map[string]string{
	"عادي": "normal", "عاديه": "normal", "normal": "normal",
	"متوسط": "normal", "منخفض": "normal", "medium": "normal", "low": "normal",
	"عالي": "high", "عاليه": "high", "high": "high",
	"طارئ": "emergency", "طارئه": "emergency", "طوارئ": "emergency", "emergency": "emergency",
}

// ParsePriority returns the enum value; ok is false for unknown non-empty
// values. A blank cell is "normal", the column default.
func ParsePriority(v string) (string, bool) {
	s := Norm(v)
	if s == "" {
		return "normal", true
	}
	p, ok := priorityAliases[s]
	return p, ok
}

// header maps normalised column titles to their index.
type header map[string]int

func newHeader(row []string) header {
	h := header{}
	for i, c := range row {
		n := Norm(c)
		if n != "" {
			if _, dup := h[n]; !dup {
				h[n] = i
			}
		}
	}
	return h
}

// find returns the index of the first column whose title matches one of the
// names exactly, or starts with it (to tolerate suffixes like " *").
func (h header) find(names ...string) int {
	// Accept the English column titles too.
	for _, name := range names {
		if en := i18n.T(i18n.EN, name); en != name {
			names = append(names, en)
		}
	}
	for _, name := range names {
		if i, ok := h[Norm(name)]; ok {
			return i
		}
	}
	for _, name := range names {
		n := Norm(name)
		for title, i := range h {
			if strings.HasPrefix(title, n) {
				return i
			}
		}
	}
	return -1
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}
