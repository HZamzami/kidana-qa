package domain

import "slices"

// WeightTotal is what the weights of a checklist, and the scorecard
// weights, must add up to: each weight then reads as a percentage.
const WeightTotal = 100

// WeightGap is how far a total is from WeightTotal: positive = still to
// add, negative = too much, 0 = right.
func WeightGap(weights ...int) int {
	sum := 0
	for _, w := range weights {
		sum += w
	}
	return WeightTotal - sum
}

// ChecklistScope is who a checklist applies to. No asset group and no
// systems = the general checklist of its stage.
type ChecklistScope struct {
	AssetGroupID *int64
	SystemIDs    []int64
}

// General reports whether the scope covers every work order.
func (s ChecklistScope) General() bool { return s.AssetGroupID == nil && len(s.SystemIDs) == 0 }

// ScopesOverlap reports whether two active checklists of the same stage
// would compete for the same work orders at the same level of detail, so
// which one is used would be unclear: the same asset group (or none) and
// the same systems (both none, or at least one in common). A narrower
// checklist beside a wider one is fine; the narrower one wins.
func ScopesOverlap(a, b ChecklistScope) bool {
	if (a.AssetGroupID == nil) != (b.AssetGroupID == nil) {
		return false
	}
	if a.AssetGroupID != nil && *a.AssetGroupID != *b.AssetGroupID {
		return false
	}
	if len(a.SystemIDs) == 0 || len(b.SystemIDs) == 0 {
		return len(a.SystemIDs) == len(b.SystemIDs)
	}
	for _, id := range a.SystemIDs {
		if slices.Contains(b.SystemIDs, id) {
			return true
		}
	}
	return false
}
