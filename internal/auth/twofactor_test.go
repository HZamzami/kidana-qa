package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestTOTP(t *testing.T) {
	setup, err := NewTOTP("a@b.c")
	if err != nil || setup.Secret == "" || !strings.HasPrefix(setup.QR, "data:image/png;base64,") {
		t.Fatalf("setup: %v %+v", err, setup.Secret)
	}
	now := time.Now()
	code, _ := totp.GenerateCode(setup.Secret, now)
	if !CheckTOTP(setup.Secret, code, now) {
		t.Fatal("current code must pass")
	}
	if !CheckTOTP(setup.Secret, code, now.Add(25*time.Second)) {
		t.Fatal("one step of drift must pass")
	}
	if CheckTOTP(setup.Secret, code, now.Add(5*time.Minute)) || CheckTOTP(setup.Secret, "12345", now) || CheckTOTP("", code, now) {
		t.Fatal("stale, short or secretless codes must fail")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, hashes := NewRecoveryCodes(8)
	if len(codes) != 8 || len(hashes) != 8 || len(codes[0]) != 11 || codes[0][5] != '-' {
		t.Fatalf("codes: %v", codes)
	}
	rest, ok := UseRecoveryCode(hashes, " "+strings.ToUpper(codes[3])+" ")
	if !ok || len(rest) != 7 {
		t.Fatal("a valid code (any case, spaces) must work once")
	}
	if _, ok := UseRecoveryCode(rest, codes[3]); ok {
		t.Fatal("a used code must not work again")
	}
}

func TestToken(t *testing.T) {
	tok, h := NewToken()
	if len(tok) < 40 || HashToken(tok) != h {
		t.Fatal("token hash mismatch")
	}
}
