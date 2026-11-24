// Package appctx carries request-scoped values (current user, flash, path)
// between the HTTP layer and the templ views without an import cycle.
package appctx

import (
	"context"

	"github.com/HZamzami/kidana-quality/internal/db/gen"
)

type key int

const (
	userKey key = iota
	flashKey
	pathKey
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
