package domain

import "testing"

func TestDiff(t *testing.T) {
	d := Diff(map[string]string{"a": "1", "b": "2", "c": ""}, map[string]string{"a": "1", "b": "3", "c": "", "d": "new"})
	if len(d) != 2 || d["b"] != (Change{From: "2", To: "3"}) || d["d"] != (Change{To: "new"}) {
		t.Fatalf("diff: %+v", d)
	}
	if Diff(map[string]string{"a": "1"}, map[string]string{"a": "1"}) != nil {
		t.Fatal("no change must give nil")
	}
}
