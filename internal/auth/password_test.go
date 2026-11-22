package auth

import (
	"testing"
	"time"
)

func TestHashAndCheck(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := CheckPassword(h, "correct horse battery"); err != nil || !ok {
		t.Fatalf("correct password rejected: %v %v", ok, err)
	}
	if ok, _ := CheckPassword(h, "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := CheckPassword("not-a-hash", "x"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
	if CheckPasswordOrDummy(nil, "x") {
		t.Fatal("missing user must never authenticate")
	}
}

func TestLoginLimiter(t *testing.T) {
	l := NewLoginLimiter(time.Hour, 3)
	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("attempt %d blocked", i+1)
		}
	}
	if l.Allow("k") {
		t.Fatal("4th attempt should be blocked")
	}
	if !l.Allow("other") {
		t.Fatal("other key should be independent")
	}
}
