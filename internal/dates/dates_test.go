package dates

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func TestDMY(t *testing.T) {
	if got := DMY(d("2026-11-02")); got != "02/11/2026" {
		t.Fatalf("got %s", got)
	}
}

// Reference dates from the official Umm al-Qura calendar.
func TestHijriUmmAlQura(t *testing.T) {
	cases := map[string][3]int{
		"2026-02-18": {1, 9, 1447}, // 1 Ramadan 1447
		"2025-06-26": {1, 1, 1447}, // 1 Muharram 1447
		"2024-03-11": {1, 9, 1445}, // 1 Ramadan 1445
		"2023-07-19": {1, 1, 1445}, // 1 Muharram 1445
	}
	for g, want := range cases {
		dd, mm, yy, ok := HijriParts(d(g))
		if !ok || dd != want[0] || mm != want[1] || yy != want[2] {
			t.Errorf("%s → %d/%d/%d (ok=%v), want %v", g, dd, mm, yy, ok, want)
		}
	}
	if got := HijriLong(d("2026-02-18")); got != "1 رمضان 1447 هـ" {
		t.Errorf("HijriLong = %q", got)
	}
	if got := Hijri(d("1900-01-01")); got != "" {
		t.Errorf("out of range should be empty, got %q", got)
	}
}
