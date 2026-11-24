package web

import (
	"net/http"
	"net/url"
	"time"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/i18n"
)

// setLang switches the UI language. It works before login (login page
// toggle), remembers the choice in a cookie and, when logged in, on the user.
func (s *Server) setLang(w http.ResponseWriter, r *http.Request) {
	l := i18n.Parse(r.PostFormValue("lang"))
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows SESSION_SECURE (off for local http)
		Name: langCookie, Value: string(l), Path: "/", MaxAge: 365 * 24 * 3600,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SessionSecure,
		Expires: time.Now().AddDate(1, 0, 0),
	})
	if u := appctx.User(r.Context()); u != nil {
		if err := s.q.SetUserLang(r.Context(), gen.SetUserLangParams{ID: u.ID, Lang: string(l)}); err != nil {
			s.log.Warn("save language", "err", err)
		}
	}
	http.Redirect(w, r, backTo(r), http.StatusSeeOther)
}

// backTo returns the same-origin page the request came from, or "/".
func backTo(r *http.Request) string {
	if next := r.PostFormValue("next"); len(next) > 0 && next[0] == '/' && (len(next) == 1 || next[1] != '/') {
		return next
	}
	ref, err := url.Parse(r.Referer())
	if err != nil || ref.Host != r.Host || ref.Path == "" {
		return "/"
	}
	if ref.RawQuery != "" {
		return ref.Path + "?" + ref.RawQuery
	}
	return ref.Path
}
