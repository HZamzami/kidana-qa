// Package appctx carries request-scoped values (current user, flash, path)
// between the HTTP layer and the templ views without an import cycle.
package appctx

import (
	"context"

	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/i18n"
)

type key int

const (
	userKey key = iota
	flashKey
	pathKey
	langKey
	uriKey
)

// Flash is a one-time message shown on the next page.
type Flash struct {
	Message string
	Tone    string // ok | warn | ""
}

func WithUser(ctx context.Context, u *gen.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// User returns the logged-in user, or nil.
func User(ctx context.Context) *gen.User {
	u, _ := ctx.Value(userKey).(*gen.User)
	return u
}

func WithFlash(ctx context.Context, f *Flash) context.Context {
	return context.WithValue(ctx, flashKey, f)
}

func FlashFrom(ctx context.Context) *Flash {
	f, _ := ctx.Value(flashKey).(*Flash)
	return f
}

func WithPath(ctx context.Context, p string) context.Context {
	return context.WithValue(ctx, pathKey, p)
}

func Path(ctx context.Context) string {
	p, _ := ctx.Value(pathKey).(string)
	return p
}

// Can reports whether the current user has one of the roles.
func Can(ctx context.Context, roles ...gen.UserRole) bool {
	u := User(ctx)
	if u == nil {
		return false
	}
	for _, r := range roles {
		if u.Role == r {
			return true
		}
	}
	return false
}

// CanEdit is true for admins and inspectors (viewers are read-only).
func CanEdit(ctx context.Context) bool {
	return Can(ctx, gen.UserRoleAdmin, gen.UserRoleInspector)
}

func IsAdmin(ctx context.Context) bool { return Can(ctx, gen.UserRoleAdmin) }

func WithLang(ctx context.Context, l i18n.Lang) context.Context {
	return context.WithValue(ctx, langKey, l)
}

// Lang is the UI language of the request (Arabic unless chosen otherwise).
func Lang(ctx context.Context) i18n.Lang {
	if l, ok := ctx.Value(langKey).(i18n.Lang); ok {
		return l
	}
	return i18n.AR
}

// T translates s into the request's language.
func T(ctx context.Context, s string) string { return i18n.T(Lang(ctx), s) }

// Tf translates a format string into the request's language and formats it.
func Tf(ctx context.Context, format string, args ...any) string {
	return i18n.Tf(Lang(ctx), format, args...)
}

// WithReturnURI stores the page address the language toggle returns to.
func WithReturnURI(ctx context.Context, uri string) context.Context {
	return context.WithValue(ctx, uriKey, uri)
}

// ReturnURI is the current page address for GET requests ("" otherwise).
func ReturnURI(ctx context.Context) string {
	s, _ := ctx.Value(uriKey).(string)
	return s
}
