package domain

import (
	"fmt"
	"testing"
)

func TestSampled(t *testing.T) {
	if Sampled("WO-1", StageAfter, 0) || !Sampled("WO-1", StageAfter, 100) {
		t.Fatal("0% never, 100% always")
	}
	if Sampled("WO-7", StageAfter, 50) != Sampled("wo-7", StageAfter, 50) {
		t.Fatal("the draw must be stable and case-insensitive")
	}
	hits := 0
	for i := range 2000 {
		if Sampled(fmt.Sprintf("PM-2026-%04d", i), StageDuring, 20) {
			hits++
		}
	}
	if hits < 300 || hits > 500 {
		t.Fatalf("20%% of 2000 should be about 400, got %d", hits)
	}
}

func TestSampleStages(t *testing.T) {
	rules := []SamplingRule{
		{Stage: StageAfter, Priority: "emergency", Percent: 100},
		{Stage: StageAfter, Percent: 0},
		{Stage: StageDuring, Type: "PM", Percent: 0},
		{Stage: StageDuring, Type: "PM", Priority: "emergency", Percent: 100},
	}
	got := SampleStages(rules, "PM-1", "PM", "emergency")
	if len(got) != 2 {
		t.Fatalf("emergency PM: the specific rules (100%%) must win, got %v", got)
	}
	if got := SampleStages(rules, "PM-1", "PM", "normal"); len(got) != 0 {
		t.Fatalf("normal PM: generic 0%% rules apply, got %v", got)
	}
	if got := SampleStages(nil, "WO-1", "WO", "high"); len(got) != 0 {
		t.Fatal("no rules, no sampling")
	}
}
