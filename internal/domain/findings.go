package domain

// RepeatWindowDays: a finding on the same asset and criterion as one raised
// within this many days is marked as a repeat (a root-cause signal).
const RepeatWindowDays = 90

// FindingAction is something a user can do to a finding.
type FindingAction string

const (
	FindingRespond FindingAction = "respond" // contractor: what was done, with photo proof
	FindingVerify  FindingAction = "verify"  // inspector accepts the response and closes
	FindingReject  FindingAction = "reject"  // inspector does not accept; back to open
	FindingClose   FindingAction = "close"   // inspector closes directly
	FindingReopen  FindingAction = "reopen"
	FindingEdit    FindingAction = "edit"
)

// FindingAllowed applies the finding workflow:
//
//	open / in_progress ──respond (contractor)──▶ responded ──verify──▶ closed
//	                                                 └──reject──▶ open
//	any open state ──close (staff)──▶ closed ──reopen (staff)──▶ open
func FindingAllowed(status string, a FindingAction, staff, contractor bool) bool {
	open := status == "open" || status == "in_progress"
	switch a {
	case FindingRespond:
		return contractor && open
	case FindingVerify, FindingReject:
		return staff && status == "responded"
	case FindingClose, FindingEdit:
		return staff && (open || status == "responded")
	case FindingReopen:
		return staff && status == "closed"
	}
	return false
}
