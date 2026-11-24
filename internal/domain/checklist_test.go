package domain

import "testing"

func TestWeightGap(t *testing.T) {
	for _, c := range []struct {
		in   []int
		want int
	}{{[]int{40, 20, 20, 20}, 0}, {[]int{30, 20}, 50}, {[]int{60, 50}, -10}, {nil, 100}} {
		if got := WeightGap(c.in...); got != c.want {
			t.Errorf("WeightGap(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestScopesOverlap(t *testing.T) {
	g1, g2 := int64(1), int64(2)
	general := ChecklistScope{}
	camps := ChecklistScope{SystemIDs: []int64{10, 11, 12}}
	for _, c := range []struct {
		name string
		a, b ChecklistScope
		want bool
	}{
		{"two general lists", general, general, true},
		{"general beside camps", general, camps, false},
		{"camps beside one camp system", camps, ChecklistScope{SystemIDs: []int64{11}}, true},
		{"different systems", camps, ChecklistScope{SystemIDs: []int64{20}}, false},
		{"group beside general", ChecklistScope{AssetGroupID: &g1}, general, false},
		{"same group", ChecklistScope{AssetGroupID: &g1}, ChecklistScope{AssetGroupID: &g1}, true},
		{"different groups", ChecklistScope{AssetGroupID: &g1}, ChecklistScope{AssetGroupID: &g2}, false},
		{"group+camps vs group only", ChecklistScope{AssetGroupID: &g1, SystemIDs: []int64{10}}, ChecklistScope{AssetGroupID: &g1}, false},
	} {
		if got := ScopesOverlap(c.a, c.b); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		if got := ScopesOverlap(c.b, c.a); got != c.want {
			t.Errorf("%s (swapped): got %v", c.name, got)
		}
	}
	if !general.General() || camps.General() {
		t.Error("General()")
	}
}

func TestASCIIDigits(t *testing.T) {
	if got := ASCIIDigits(" ٤٠ "); got != "40" {
		t.Errorf("got %q", got)
	}
	if got := ASCIIDigits("۲۵"); got != "25" {
		t.Errorf("got %q", got)
	}
}
