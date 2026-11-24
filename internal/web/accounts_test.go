package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/HZamzami/kidana-quality/internal/auth"
)

// TestTwoFactor enables optional 2FA, signs in with a code and with a
// recovery code (once only), and checks admins can turn it off.
func TestTwoFactor(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := e.login("inspector@test.local")

	if res := e.post(c, "/account/2fa/start", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("start 2fa: %d", res.StatusCode)
	}
	// The secret is only in the session until confirmed; read it from the page.
	_, body := e.get(c, "/account/security")
	if strings.Contains(body, "/account/2fa/confirm") {
		t.Fatal("a fresh page load must not keep the unconfirmed setup visible")
	}
	res := e.postBody(c, "/account/2fa/start", nil)
	secret := regexp.MustCompile(`dir="ltr">([A-Z2-7]{16,})</code>`).FindStringSubmatch(res)
	if secret == nil {
		t.Fatal("secret not shown")
	}
	if res := e.post(c, "/account/2fa/confirm", url.Values{"code": {"000000"}}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("a wrong code must be refused: %d", res.StatusCode)
	}
	code, _ := totp.GenerateCode(secret[1], time.Now())
	page := e.postBody(c, "/account/2fa/confirm", url.Values{"code": {code}})
	recovery := regexp.MustCompile(`>([a-z2-9]{5}-[a-z2-9]{5})<`).FindAllStringSubmatch(page, -1)
	if len(recovery) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(recovery))
	}
	if u := must(e.q.GetUserByEmail(ctx, "inspector@test.local")); !u.TotpEnabled || len(u.RecoveryCodes) != 8 {
		t.Fatal("2fa not saved")
	}

	// Password alone now leads to the second step, not into the app.
	c2 := e.client()
	if res := e.post(c2, "/login", url.Values{"email": {"inspector@test.local"}, "password": {testPassword}}); res.Header.Get("Location") != "/login/2fa" {
		t.Fatalf("expected the 2fa step, got %s", res.Header.Get("Location"))
	}
	if res, _ := e.get(c2, "/"); res.StatusCode != http.StatusSeeOther {
		t.Fatal("must not be signed in before the second step")
	}
	if res := e.post(c2, "/login/2fa", url.Values{"code": {"123456"}}); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong code: %d", res.StatusCode)
	}
	code, _ = totp.GenerateCode(secret[1], time.Now())
	if res := e.post(c2, "/login/2fa", url.Values{"code": {code}}); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/" {
		t.Fatalf("right code: %d", res.StatusCode)
	}
	if res, _ := e.get(c2, "/"); res.StatusCode != http.StatusOK {
		t.Fatal("not signed in after the second step")
	}

	// A recovery code works once.
	for i, want := range []int{http.StatusSeeOther, http.StatusUnauthorized} {
		c3 := e.client()
		e.post(c3, "/login", url.Values{"email": {"inspector@test.local"}, "password": {testPassword}})
		if res := e.post(c3, "/login/2fa", url.Values{"code": {recovery[0][1]}}); res.StatusCode != want {
			t.Fatalf("recovery code use %d: got %d, want %d", i+1, res.StatusCode, want)
		}
	}

	// Admin turns it off; the password alone is enough again.
	u := must(e.q.GetUserByEmail(ctx, "inspector@test.local"))
	admin := e.login("admin@test.local")
	e.post(admin, "/admin/users/"+idStr(u.ID)+"/2fa-reset", nil)
	e.login("inspector@test.local")
}

// TestPasswordResetLink covers «نسيت كلمة المرور»: the request reaches
// admins, the link works once, and old sessions are signed out.
func TestPasswordResetLink(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	old := e.login("viewer@test.local")

	if res := e.post(e.client(), "/forgot", url.Values{"email": {"viewer@test.local"}}); res.StatusCode != http.StatusOK {
		t.Fatalf("forgot: %d", res.StatusCode)
	}
	if res := e.post(e.client(), "/forgot", url.Values{"email": {"nobody@test.local"}}); res.StatusCode != http.StatusOK {
		t.Fatal("unknown emails must get the same answer")
	}
	admin := e.login("admin@test.local")
	_, users := e.get(admin, "/admin/users")
	if !strings.Contains(users, "viewer@test.local") || !strings.Contains(users, "/reset-link") {
		t.Fatal("open request not shown to admins")
	}
	u := must(e.q.GetUserByEmail(ctx, "viewer@test.local"))
	page := e.postBody(admin, "/admin/users/"+idStr(u.ID)+"/reset-link", nil)
	m := regexp.MustCompile(`/reset/([A-Za-z0-9_-]{40,})`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("reset link not shown")
	}
	if reqs := must(e.q.ListOpenPasswordResetRequests(ctx)); len(reqs) != 0 {
		t.Fatal("issuing a link must close the request")
	}
	link := "/reset/" + m[1]
	if res, _ := e.get(e.client(), link); res.StatusCode != http.StatusOK {
		t.Fatalf("open link: %d", res.StatusCode)
	}
	if res := e.post(e.client(), link, url.Values{"new": {"short"}, "confirm": {"short"}}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatal("short password must be refused")
	}
	if res := e.post(e.client(), link, url.Values{"new": {"brand-new-password"}, "confirm": {"brand-new-password"}}); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("reset: %d", res.StatusCode)
	}
	if res := e.post(e.client(), link, url.Values{"new": {"another-password-1"}, "confirm": {"another-password-1"}}); res.StatusCode != http.StatusGone {
		t.Fatal("a used link must not work again")
	}
	if res, _ := e.get(old, "/"); res.StatusCode != http.StatusSeeOther {
		t.Fatal("sessions from before the reset must be signed out")
	}
	if ok, _ := auth.CheckPassword(must(e.q.GetUserByEmail(ctx, "viewer@test.local")).PasswordHash, "brand-new-password"); !ok {
		t.Fatal("new password not saved")
	}
	if res, _ := e.get(e.client(), "/reset/not-a-real-token"); res.StatusCode != http.StatusGone {
		t.Fatal("unknown tokens must be refused")
	}
}

// postBody posts and returns the response body.
func (e *testEnv) postBody(c *http.Client, path string, form url.Values) string {
	e.t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var sb strings.Builder
	buf := make([]byte, 32<<10)
	for {
		n, err := res.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}
