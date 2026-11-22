package domain

import "testing"

func TestCount(t *testing.T) {
	cases := map[int]string{
		0: "0 فحص", 1: "1 فحص", 2: "فحصان", 3: "3 فحوصات", 10: "10 فحوصات",
		11: "11 فحصاً", 25: "25 فحصاً", 100: "100 فحص", 102: "102 فحص", 105: "105 فحوصات", 120: "120 فحصاً",
	}
	for n, want := range cases {
		if got := Count(n, NounInspection); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
	if got := Count(2, NounFinding); got != "ملاحظتان" {
		t.Errorf("dual finding = %q", got)
	}
}
