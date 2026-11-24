package web

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if appctx.User(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, views.LoginPage("", ""))
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := strings.TrimSpace(strings.ToLower(r.PostFormValue("email")))
	password := r.PostFormValue("password")

	if !s.limiter.Allow(clientIP(r) + "|" + email) {
		s.render(w, r, http.StatusTooManyRequests, views.LoginPage(email, "محاولات كثيرة. انتظر دقيقة ثم حاول مرة أخرى."))
		return
	}

	var hash *string
	u, err := s.q.GetUserByEmail(ctx, email)
	switch {
	case err == nil && u.Active:
		hash = &u.PasswordHash
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		s.serverError(w, r, err)
		return
	}
	if !auth.CheckPasswordOrDummy(hash, password) {
		s.render(w, r, http.StatusUnauthorized, views.LoginPage(email, "البريد الإلكتروني أو كلمة المرور غير صحيحة."))
		return
	}

	// New session token on login prevents session fixation.
	if err := s.sm.RenewToken(ctx); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.sm.Put(ctx, sessionUserID, u.ID)
	s.audit(appctx.WithUser(ctx, &u), s.q, "user", u.ID, "login", nil)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.sm.Destroy(r.Context()); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) passwordPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, views.PasswordPage(nil))
}

func (s *Server) passwordSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := appctx.User(ctx)
	f := newForm(r)
	current := r.PostFormValue("current")
	next := r.PostFormValue("new")

	if ok, _ := auth.CheckPassword(u.PasswordHash, current); !ok {
		f.Errors["current"] = "كلمة المرور الحالية غير صحيحة"
	}
	if len([]rune(next)) < auth.MinPasswordLength {
		f.Errors["new"] = "10 أحرف على الأقل"
	}
	if next != r.PostFormValue("confirm") {
		f.Errors["confirm"] = "كلمتا المرور غير متطابقتين"
	}
	if !f.Valid() {
		s.render(w, r, http.StatusUnprocessableEntity, views.PasswordPage(f.Errors))
		return
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{ID: u.ID, PasswordHash: hash}); err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.sm.RenewToken(ctx)
	s.audit(ctx, s.q, "user", u.ID, "password_changed", nil)
	s.flash(ctx, "تم تغيير كلمة المرور", "ok")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// clientIP uses the direct peer address. If the app runs behind a reverse
// proxy, configure the proxy to set X-Forwarded-For and extend this.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
