// Package views contains the templ components (pages, partials, components).
package views

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/i18n"
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

func delayText(ctx context.Context, planned, actual *time.Time) string {
	d, ok := domain.DelayDays(planned, actual)
	if !ok {
		return "—"
	}
	return domain.DelayPhrase(appctx.Lang(ctx), d)
}

// t translates a UI string into the request's language.
func t(ctx context.Context, s string) string { return appctx.T(ctx, s) }

// tf translates a format string and formats it.
func tf(ctx context.Context, format string, args ...any) string {
	return appctx.Tf(ctx, format, args...)
}

// count is domain.Count in the request's language.
func count(ctx context.Context, n int, w domain.Noun) string {
	return domain.CountIn(appctx.Lang(ctx), n, w)
}

// nm picks the English name of a record in the English UI when one is set.
func nm(ctx context.Context, ar, en string) string { return i18n.Pick(appctx.Lang(ctx), ar, en) }

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

// backArrow points toward the start of the line: → in Arabic, ← in English.
func backArrow(ctx context.Context) string {
	if appctx.Lang(ctx) == i18n.EN {
		return "←"
	}
	return "→"
}

// checklistName shows a checklist's name in the UI language. Names follow
// "<stage name> — الإصدار N" unless an admin typed something else, which is
// shown as entered.
func checklistName(ctx context.Context, name string) string {
	if appctx.Lang(ctx) != i18n.EN {
		return name
	}
	if base, ver, ok := strings.Cut(name, " — الإصدار "); ok {
		return t(ctx, base) + " — version " + ver
	}
	return t(ctx, name)
}
