package domain

import "time"

// SLATargets are days allowed by key: a priority (normal, high, emergency)
// for work orders, or finding_<severity> for findings.
type SLATargets map[string]int

// SLA states of a work order against its due date.
const (
	SLAMet      = "met"      // done on or before the due date
	SLABreached = "breached" // done late, or not done and past due
	SLAAtRisk   = "at_risk"  // not done, due today or tomorrow
	SLAOnTrack  = "on_track" // not done, due later
)

// DueFrom is the due date implied by a report date and a target key.
func DueFrom(reported time.Time, key string, t SLATargets) (time.Time, bool) {
	days, ok := t[key]
	if !ok {
		return time.Time{}, false
	}
	return reported.AddDate(0, 0, days), true
}

// SLAState compares completion with the due date ("" when there is no due date).
func SLAState(due, done *time.Time, today time.Time) string {
	switch {
	case due == nil:
		return ""
	case done != nil && !done.After(*due):
		return SLAMet
	case done != nil:
		return SLABreached
	case today.After(*due):
		return SLABreached
	case !today.Before(due.AddDate(0, 0, -1)):
		return SLAAtRisk
	}
	return SLAOnTrack
}

// SLARate is the share of finished-or-overdue work orders that met their
// target (percent), or nil when none are measurable yet.
func SLARate(states []string) *float64 {
	met, measured := 0, 0
	for _, s := range states {
		switch s {
		case SLAMet:
			met++
			measured++
		case SLABreached:
			measured++
		}
	}
	if measured == 0 {
		return nil
	}
	v := float64(met) / float64(measured) * 100
	return &v
}
