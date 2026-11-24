package domain

import (
	"hash/fnv"
	"strings"
)

// SamplingRule says what share of matching work orders get an inspection of
// a stage. An empty Type or Priority matches any.
type SamplingRule struct {
	Stage    Stage
	Type     string // PM | WO | ""
	Priority string // normal | high | emergency | ""
	Percent  int
}

// specificity ranks rules: type and priority, then one of them, then neither.
func (r SamplingRule) specificity() int {
	n := 0
	if r.Type != "" {
		n++
	}
	if r.Priority != "" {
		n++
	}
	return n
}

func (r SamplingRule) matches(woType, priority string) bool {
	return (r.Type == "" || r.Type == woType) && (r.Priority == "" || r.Priority == priority)
}

// Sampled is a deterministic draw: the same work order and stage always give
// the same answer, so re-importing or editing never changes the sample.
func Sampled(ref string, stage Stage, percent int) bool {
	if percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(ref) + "|" + string(stage)))
	return int(h.Sum32()%100) < percent
}

// SampleStages returns the stages a new work order should be inspected at.
// For each stage the most specific matching rule decides; ties take the
// higher percentage.
func SampleStages(rules []SamplingRule, ref, woType, priority string) []Stage {
	var out []Stage
	for _, stage := range []Stage{StageDuring, StageAfter} {
		best, found := SamplingRule{}, false
		for _, r := range rules {
			if r.Stage != stage || !r.matches(woType, priority) {
				continue
			}
			if !found || r.specificity() > best.specificity() ||
				(r.specificity() == best.specificity() && r.Percent > best.Percent) {
				best, found = r, true
			}
		}
		if found && Sampled(ref, stage, best.Percent) {
			out = append(out, stage)
		}
	}
	return out
}
