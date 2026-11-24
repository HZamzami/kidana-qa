package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

func loginWithError(_ context.Context, msg string) templ.Component { return views.LoginPage("", msg) }

// ssoClient is set up on first use, so a slow or unreachable identity
// provider never delays the app's start.
type ssoClient struct {
	once     sync.Once
	err      error
	verifier *oidc.IDTokenVerifier
	config   oauth2.Config
}

func (s *Server) sso(ctx context.Context) (*ssoClient, error) {
	c := s.oidc
	c.once.Do(func() {
		p, err := oidc.NewProvider(ctx, s.cfg.OIDCIssuer)
		if err != nil {
			c.err = err
			return
		}
		c.verifier = p.Verifier(&oidc.Config{ClientID: s.cfg.OIDCClientID})
		c.config = oauth2.Config{
			ClientID: s.cfg.OIDCClientID, ClientSecret: s.cfg.OIDCClientSecret, RedirectURL: s.cfg.OIDCRedirectURL,
			Endpoint: p.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		}
	})
	if c.err != nil {
		// allow a retry on the next attempt
		s.oidc = &ssoClient{}
	}
	return c, c.err
}

// oidcStart sends the browser to the identity provider (PKCE + state + nonce).
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.cfg.OIDCEnabled() {
		s.notFound(w, r)
		return
	}
	c, err := s.sso(ctx)
	if err != nil {
		s.log.Error("oidc discovery", "err", err)
		s.render(w, r, http.StatusBadGateway, loginWithError(ctx, appctx.T(ctx, "تعذّر الاتصال بخدمة تسجيل الدخول الموحّد. حاول لاحقاً أو استخدم كلمة المرور.")))
		return
	}
	state, _ := auth.NewToken()
	nonce, _ := auth.NewToken()
	verifier := oauth2.GenerateVerifier()
	s.sm.Put(ctx, sessionOIDC, state+"|"+nonce+"|"+verifier)
	http.Redirect(w, r, c.config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

// oidcCallback signs in an existing, active user whose email matches. There
// is no automatic account creation: an admin adds users first.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fail := func(msg string) {
		s.render(w, r, http.StatusUnauthorized, loginWithError(ctx, msg))
	}
	if !s.cfg.OIDCEnabled() {
		s.notFound(w, r)
		return
	}
	saved := strings.SplitN(s.sm.PopString(ctx, sessionOIDC), "|", 3)
	if len(saved) != 3 || r.URL.Query().Get("state") != saved[0] || r.URL.Query().Get("code") == "" {
		fail(appctx.T(ctx, "انتهت محاولة تسجيل الدخول. ابدأ من جديد."))
		return
	}
	c, err := s.sso(ctx)
	if err != nil {
		fail(appctx.T(ctx, "تعذّر الاتصال بخدمة تسجيل الدخول الموحّد. حاول لاحقاً أو استخدم كلمة المرور."))
		return
	}
	tok, err := c.config.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(saved[2]))
	if err != nil {
		fail(appctx.T(ctx, "انتهت محاولة تسجيل الدخول. ابدأ من جديد."))
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := c.verifier.Verify(ctx, raw)
	if err != nil || idt.Nonce != saved[1] {
		fail(appctx.T(ctx, "انتهت محاولة تسجيل الدخول. ابدأ من جديد."))
		return
	}
	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"` // Entra ID puts the UPN here
	}
	_ = idt.Claims(&claims)
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if email == "" {
		email = strings.ToLower(strings.TrimSpace(claims.PreferredUsername))
	}

	u, err := s.q.GetUserByOIDCSubject(ctx, &idt.Subject)
	if errors.Is(err, pgx.ErrNoRows) && email != "" {
		u, err = s.q.GetUserByEmail(ctx, email)
		if err == nil {
			if u.OidcSubject != nil && *u.OidcSubject != idt.Subject {
				fail(appctx.T(ctx, "هذا الحساب مرتبط بهوية أخرى. راجع مدير النظام."))
				return
			}
			sub := idt.Subject
			if err := s.q.SetOIDCSubject(ctx, gen.SetOIDCSubjectParams{ID: u.ID, OidcSubject: &sub}); err != nil {
				s.serverError(w, r, err)
				return
			}
		}
	}
	if err != nil || !u.Active {
		fail(appctx.T(ctx, "لا يوجد حساب مفعّل بهذا البريد في النظام. اطلب من مدير النظام إضافتك."))
		return
	}
	// The identity provider enforces its own MFA, so the app's 2FA step is skipped.
	s.completeLogin(w, r, u, "login_sso")
}
