// Package views contains the templ components (pages, partials, components).
package views

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/a-h/templ"

	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/domain"
)

// AssetVersion is appended to static URLs to bust caches after a deploy.
var AssetVersion = "dev"

func asset(path string) string { return path + "?v=" + AssetVersion }

func idStr(id int64) string { return strconv.FormatInt(id, 10) }

func itoa(n int) string { return strconv.Itoa(n) }

func u(format string, args ...any) templ.SafeURL {
	return templ.URL(fmt.Sprintf(format, args...))
}

func path(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// isoDate is for comparisons and form values only (yyyy-mm-dd). Display dates with
// dmy / hijri or the DateText component.
func isoDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.DateOnly)
}

func dmy(t time.Time) string { return dates.DMY(t) }

func dateTime(t time.Time) string { return dates.DMYTime(t) }

func echoG(iso string) string {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return ""
	}
	return dates.DMY(t)
}

func echoH(iso string) string {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return ""
	}
	return dates.HijriDigits(t)
}

func scoreVal(v float64) string { return domain.Fmt1(v) + "%" }

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func pctWidth(v float64) string {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return fmt.Sprintf("width:%.1f%%", v)
}

func delayText(planned, actual *time.Time) string {
	d, ok := domain.DelayDays(planned, actual)
	switch {
	case !ok:
		return "—"
	case d > 0:
		return fmt.Sprintf("متأخر %d يوم", d)
	case d < 0:
		return fmt.Sprintf("مبكر %d يوم", -d)
	}
	return "في الموعد"
}

// Option is a select option.
type Option struct {
	Value string
	Label string
}

// withQuery returns the current filters with one key replaced (for pagination links).
func withQuery(base string, q url.Values, key, val string) string {
	c := url.Values{}
	for k, v := range q {
		c[k] = v
	}
	c.Set(key, val)
	return base + "?" + c.Encode()
}

func initial(s string) string {
	for _, r := range s {
		return string(r)
	}
	return "؟"
}

func isActive(current, prefix string) bool {
	if prefix == "/" {
		return current == "/"
	}
	return len(current) >= len(prefix) && current[:len(prefix)] == prefix
}
