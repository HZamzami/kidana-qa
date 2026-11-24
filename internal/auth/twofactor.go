package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"image/png"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Issuer names the app in authenticator apps.
const Issuer = "Kidana Quality"

// TOTPSetup is a new, not yet confirmed authenticator secret.
type TOTPSetup struct {
	Secret string // base32, also shown for manual entry
	QR     string // data: URI of a PNG QR code for authenticator apps
}

// NewTOTP creates a secret for account (the user's email).
func NewTOTP(account string) (TOTPSetup, error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: Issuer, AccountName: account})
	if err != nil {
		return TOTPSetup{}, err
	}
	img, err := key.Image(240, 240)
	if err != nil {
		return TOTPSetup{}, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return TOTPSetup{}, err
	}
	return TOTPSetup{Secret: key.Secret(), QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

// TOTPFromSecret rebuilds the setup (QR) for an existing secret, e.g. to
// show the QR again after a wrong confirmation code.
func TOTPFromSecret(account, secret string) (TOTPSetup, error) {
	u := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + Issuer + ":" + account}
	u.RawQuery = url.Values{"secret": {secret}, "issuer": {Issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
	key, err := otp.NewKeyFromURL(u.String())
	if err != nil {
		return TOTPSetup{}, err
	}
	img, err := key.Image(240, 240)
	if err != nil {
		return TOTPSetup{}, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return TOTPSetup{}, err
	}
	return TOTPSetup{Secret: secret, QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

// CheckTOTP validates a 6-digit code, allowing one 30-second step of clock drift.
func CheckTOTP(secret, code string, now time.Time) bool {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if secret == "" || len(code) != 6 {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{
		Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// NewRecoveryCodes returns n one-time codes to show the user once, and their
// hashes to store.
func NewRecoveryCodes(n int) (codes, hashes []string) {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789" // no look-alikes
	for range n {
		b := make([]byte, 10)
		_, _ = rand.Read(b)
		var sb strings.Builder
		for i, c := range b {
			if i == 5 {
				sb.WriteByte('-')
			}
			sb.WriteByte(alphabet[int(c)%len(alphabet)])
		}
		codes = append(codes, sb.String())
		hashes = append(hashes, HashToken(sb.String()))
	}
	return codes, hashes
}

// UseRecoveryCode checks code against the stored hashes. On a match it
// returns the remaining hashes, without the used one.
func UseRecoveryCode(hashes []string, code string) (remaining []string, ok bool) {
	h := HashToken(strings.ToLower(strings.TrimSpace(code)))
	for i, stored := range hashes {
		if subtle.ConstantTimeCompare([]byte(stored), []byte(h)) == 1 {
			remaining = append(append([]string{}, hashes[:i]...), hashes[i+1:]...)
			return remaining, true
		}
	}
	return hashes, false
}

// NewToken returns a random URL-safe token and the hash to store.
func NewToken() (token, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken is the stored form of a token or recovery code.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
