package web

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/i18n"
)

const (
	sessionUserID = "userID"
	sessionVer    = "sessionVersion" // users.session_version at login
	sessionTwoFA  = "pending2fa"     // user id that passed the password step
	sessionTOTP   = "totpSetup"      // unconfirmed authenticator secret
	sessionOIDC   = "oidcState"      // state|nonce|verifier for Microsoft sign-in
	sessionFlash  = "flash"
	sessionFlashT = "flashTone"
)

// statusRecorder captures the status code for access logs.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz" {
			return
		}
		s.log.Info("http",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("dur", time.Since(start)),
			slog.Bool("htmx", isHTMX(r)),
		)
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				s.log.Error("panic", slog.Any("value", v), slog.String("stack", string(debug.Stack())))
				w.Header().Set("Connection", "close")
				http.Error(w, appctx.T(r.Context(), "حدث خطأ غير متوقع"), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets a strict CSP. Scripts are only our own files; Alpine
// uses its CSP build, so no 'unsafe-eval'. Inline style attributes are
// allowed for small dynamic widths (progress bars).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; "+
				"font-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'self'; object-src 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(self), geolocation=(), microphone=()")
		next.ServeHTTP(w, r)
	})
}

// loadUser puts the logged-in user (if any), the request path and any flash
// message into the request context.
func (s *Server) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := appctx.WithPath(r.Context(), r.URL.Path)
		if r.Method == http.MethodGet && !isHTMX(r) {
			ctx = appctx.WithReturnURI(ctx, r.URL.RequestURI())
		}
		if id := s.sm.GetInt64(ctx, sessionUserID); id != 0 {
			u, err := s.q.GetUser(ctx, id)
			switch {
			case err == nil && u.Active && int64(u.SessionVersion) == s.sm.GetInt64(ctx, sessionVer):
				ctx = appctx.WithUser(ctx, &u)
			case err == nil || errors.Is(err, pgx.ErrNoRows):
				// deactivated, deleted, or signed out everywhere (password changed): drop the session
				_ = s.sm.Destroy(ctx)
			default:
				s.serverError(w, r, err)
				return
			}
		}
		ctx = appctx.WithLang(ctx, requestLang(r, appctx.User(ctx)))
		if r.Method == http.MethodGet && !isHTMX(r) {
			if msg := s.sm.PopString(ctx, sessionFlash); msg != "" {
				ctx = appctx.WithFlash(ctx, &appctx.Flash{Message: msg, Tone: s.sm.PopString(ctx, sessionFlashT)})
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireUser redirects anonymous users to the login page.
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if appctx.User(r.Context()) == nil {
			redirect(w, r, "/login")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// requireRole wraps a handler so only the given roles may use it.
func (s *Server) requireRole(roles ...gen.UserRole) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !appctx.Can(r.Context(), roles...) {
				s.forbidden(w, r)
				return
			}
			next(w, r)
		}
	}
}

const langCookie = "lang"

// requestLang picks the UI language: the lang cookie (set by the toggle),
// then the user's saved choice, then Arabic.
func requestLang(r *http.Request, u *gen.User) i18n.Lang {
	if c, err := r.Cookie(langCookie); err == nil && i18n.Valid(c.Value) {
		return i18n.Lang(c.Value)
	}
	if u != nil {
		return i18n.Parse(u.Lang)
	}
	return i18n.AR
}
