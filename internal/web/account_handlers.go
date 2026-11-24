package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// resetLinkTTL is how long an admin-issued reset link stays valid.
const resetLinkTTL = 24 * time.Hour

// ---------------------------------------------------------------- two-factor (optional)

func (s *Server) securityPage(w http.ResponseWriter, r *http.Request) {
	s.renderSecurity(w, r, http.StatusOK, views.SecurityData{})
}

func (s *Server) renderSecurity(w http.ResponseWriter, r *http.Request, status int, d views.SecurityData) {
	u := appctx.User(r.Context())
	d.Enabled, d.RecoveryLeft, d.SSO = u.TotpEnabled, len(u.RecoveryCodes), u.OidcSubject != nil
	s.render(w, r, status, views.SecurityPage(d))
}

// twoFAStart creates an authenticator secret and shows its QR code. The
// secret is kept in the session until the user confirms a code from it.
func (s *Server) twoFAStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := appctx.User(ctx)
	if u.TotpEnabled {
		http.Redirect(w, r, "/account/security", http.StatusSeeOther)
		return
	}
	setup, err := auth.NewTOTP(u.Email)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.sm.Put(ctx, sessionTOTP, setup.Secret)
	s.renderSecurity(w, r, http.StatusOK, views.SecurityData{Setup: &setup})
}

func (s *Server) twoFAConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := appctx.User(ctx)
	secret := s.sm.GetString(ctx, sessionTOTP)
	if secret == "" {
		http.Redirect(w, r, "/account/security", http.StatusSeeOther)
		return
	}
	if !auth.CheckTOTP(secret, r.PostFormValue("code"), time.Now()) {
		setup, err := auth.TOTPFromSecret(u.Email, secret)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		s.renderSecurity(w, r, http.StatusUnprocessableEntity, views.SecurityData{
			Setup: &setup, Error: appctx.T(ctx, "الرمز غير صحيح. تأكد من وقت الجوال وأدخل الرمز الحالي."),
		})
		return
	}
	codes, hashes := auth.NewRecoveryCodes(8)
	if err := s.q.SetTOTP(ctx, gen.SetTOTPParams{ID: u.ID, TotpSecret: secret, TotpEnabled: true, RecoveryCodes: hashes}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.sm.Remove(ctx, sessionTOTP)
	s.audit(ctx, s.q, "user", u.ID, "2fa_enabled", nil)
	u.TotpEnabled, u.RecoveryCodes = true, hashes
	s.renderSecurity(w, r, http.StatusOK, views.SecurityData{Codes: codes, Info: appctx.T(ctx, "تم تفعيل التحقق بخطوتين")})
}

// twoFADisable turns 2FA off after the password and a current code.
func (s *Server) twoFADisable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := appctx.User(ctx)
	ok, _ := auth.CheckPassword(u.PasswordHash, r.PostFormValue("password"))
	_, recovery := auth.UseRecoveryCode(u.RecoveryCodes, r.PostFormValue("code"))
	if !ok || (!auth.CheckTOTP(u.TotpSecret, r.PostFormValue("code"), time.Now()) && !recovery) {
		s.renderSecurity(w, r, http.StatusUnprocessableEntity, views.SecurityData{Error: appctx.T(ctx, "كلمة المرور أو الرمز غير صحيح")})
		return
	}
	if err := s.q.SetTOTP(ctx, gen.SetTOTPParams{ID: u.ID, TotpSecret: "", TotpEnabled: false, RecoveryCodes: []string{}}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "user", u.ID, "2fa_disabled", nil)
	s.flash(ctx, appctx.T(ctx, "تم إيقاف التحقق بخطوتين"), "")
	http.Redirect(w, r, "/account/security", http.StatusSeeOther)
}

// twoFARecovery replaces the recovery codes (after a current code).
func (s *Server) twoFARecovery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := appctx.User(ctx)
	if !u.TotpEnabled || !auth.CheckTOTP(u.TotpSecret, r.PostFormValue("code"), time.Now()) {
		s.renderSecurity(w, r, http.StatusUnprocessableEntity, views.SecurityData{Error: appctx.T(ctx, "الرمز غير صحيح")})
		return
	}
	codes, hashes := auth.NewRecoveryCodes(8)
	if err := s.q.SetRecoveryCodes(ctx, gen.SetRecoveryCodesParams{ID: u.ID, RecoveryCodes: hashes}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "user", u.ID, "2fa_recovery_codes", nil)
	s.renderSecurity(w, r, http.StatusOK, views.SecurityData{Codes: codes, Info: appctx.T(ctx, "رموز استرداد جديدة — الرموز السابقة لم تعد صالحة")})
}

// adminReset2FA turns off a user's 2FA (lost phone and recovery codes).
func (s *Server) adminReset2FA(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := s.q.SetTOTP(ctx, gen.SetTOTPParams{ID: id, TotpSecret: "", TotpEnabled: false, RecoveryCodes: []string{}}); err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "user", id, "2fa_reset_by_admin", nil)
	s.flash(ctx, appctx.T(ctx, "تم إيقاف التحقق بخطوتين لهذا المستخدم"), "ok")
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// ---------------------------------------------------------------- password reset (no email)

func (s *Server) forgotPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, views.ForgotPage(false, ""))
}

// forgotSubmit records a reset request for admins. The answer is the same
// whether or not the email exists, so it can't be used to probe accounts.
func (s *Server) forgotSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := strings.TrimSpace(strings.ToLower(r.PostFormValue("email")))
	if !s.limiter.Allow(clientIP(r) + "|forgot") {
		s.render(w, r, http.StatusTooManyRequests, views.ForgotPage(false, appctx.T(ctx, "محاولات كثيرة. انتظر دقيقة ثم حاول مرة أخرى.")))
		return
	}
	u, err := s.q.GetUserByEmail(ctx, email)
	switch {
	case err == nil && u.Active:
		if err := s.q.CreatePasswordResetRequest(ctx, u.ID); err != nil {
			s.serverError(w, r, err)
			return
		}
		s.audit(appctx.WithUser(ctx, &u), s.q, "user", u.ID, "password_reset_requested", nil)
		s.onPasswordResetRequested(ctx, u)
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ForgotPage(true, ""))
}

// adminResetLink issues a one-time link for a user and shows it once.
func (s *Server) adminResetLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	u, err := s.q.GetUser(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	token, hash := auth.NewToken()
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.CreatePasswordReset(ctx, gen.CreatePasswordResetParams{
			UserID: id, TokenHash: hash, ExpiresAt: time.Now().Add(resetLinkTTL), CreatedBy: currentUserID(ctx),
		}); err != nil {
			return err
		}
		return q.HandlePasswordResetRequests(ctx, gen.HandlePasswordResetRequestsParams{UserID: id, HandledBy: currentUserID(ctx)})
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "user", id, "password_reset_link", nil)
	s.usersPage(w, r, http.StatusOK, views.UsersData{
		ResetLink: baseURL(r) + "/reset/" + token, ResetFor: u.Name,
	})
}

func (s *Server) resetPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.validReset(r); !ok {
		s.render(w, r, http.StatusGone, views.ResetPage(r.PathValue("token"), nil, true))
		return
	}
	s.render(w, r, http.StatusOK, views.ResetPage(r.PathValue("token"), nil, false))
}

func (s *Server) resetSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reset, ok := s.validReset(r)
	if !ok {
		s.render(w, r, http.StatusGone, views.ResetPage(r.PathValue("token"), nil, true))
		return
	}
	f := newForm(r)
	next := r.PostFormValue("new")
	if len([]rune(next)) < auth.MinPasswordLength {
		f.Errors["new"] = appctx.T(ctx, "10 أحرف على الأقل")
	}
	if next != r.PostFormValue("confirm") {
		f.Errors["confirm"] = appctx.T(ctx, "كلمتا المرور غير متطابقتين")
	}
	if !f.Valid() {
		s.render(w, r, http.StatusUnprocessableEntity, views.ResetPage(r.PathValue("token"), f.Errors, false))
		return
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	used := false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.UsePasswordReset(ctx, reset.ID)
		if err != nil || n == 0 {
			return err
		}
		used = true
		if err := q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{ID: reset.UserID, PasswordHash: hash}); err != nil {
			return err
		}
		return q.BumpSessionVersion(ctx, reset.UserID) // sign out everywhere
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !used {
		s.render(w, r, http.StatusGone, views.ResetPage(r.PathValue("token"), nil, true))
		return
	}
	s.audit(ctx, s.q, "user", reset.UserID, "password_reset_used", nil)
	s.flash(ctx, appctx.T(ctx, "تم تعيين كلمة المرور الجديدة. سجّل الدخول بها."), "ok")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// validReset finds an unused, unexpired reset for the token in the path.
func (s *Server) validReset(r *http.Request) (gen.GetPasswordResetRow, bool) {
	reset, err := s.q.GetPasswordReset(r.Context(), auth.HashToken(r.PathValue("token")))
	if err != nil || reset.UsedAt != nil || time.Now().After(reset.ExpiresAt) || !reset.Active {
		return reset, false
	}
	return reset, true
}

// baseURL is the app's address as the browser sees it, for links an admin
// copies to a user.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
