package domain

import "fmt"

// Stage of an inspection relative to the work.
type Stage string

const (
	StageDuring Stage = "during" // during the work: how the contractor works
	StageAfter  Stage = "after"  // after the work: is the result correct
)

func (s Stage) Valid() bool { return s == StageDuring || s == StageAfter }

func (s Stage) Label() string {
	if s == StageDuring {
		return "فحص أثناء التنفيذ"
	}
	return "فحص بعد التنفيذ"
}

// Short is the compact badge text.
func (s Stage) Short() string {
	if s == StageDuring {
		return "أثناء التنفيذ"
	}
	return "بعد التنفيذ"
}

// WOStatus mirrors the wo_status enum.
type WOStatus string

const (
	WOOpen       WOStatus = "open"
	WOInProgress WOStatus = "in_progress"
	WOCompleted  WOStatus = "completed"
	WOAccepted   WOStatus = "accepted"
	WOCancelled  WOStatus = "cancelled"
)

func (s WOStatus) Label() string {
	switch s {
	case WOOpen:
		return "مفتوح"
	case WOInProgress:
		return "قيد التنفيذ"
	case WOCompleted:
		return "منجز — بانتظار الاعتماد"
	case WOAccepted:
		return "معتمد" // not "مقبول": that word is also a grade band (60–69)
	case WOCancelled:
		return "ملغى"
	}
	return string(s)
}

// ManualWOStatuses are the statuses a user may set directly. "accepted" is
// derived by NextWOStatus and never set by hand.
var ManualWOStatuses = []WOStatus{WOOpen, WOInProgress, WOCompleted, WOCancelled}

// AcceptanceInput summarises what NextWOStatus needs to know about a work order.
type AcceptanceInput struct {
	Current           WOStatus
	HasSubmittedAfter bool
	LatestAfterPassed bool // result of the most recent submitted after-work attempt
	OpenFindings      int
	HasSubmittedWork  bool // any submitted inspection (during or after)
}

// NextWOStatus applies the acceptance rule: a work order is accepted when its
// latest after-work inspection passed and it has no open findings. It also moves an open work
// order to "in progress" once an inspection was submitted, and steps an
// accepted work order back to "completed" if the rule no longer holds
// (e.g. a finding was re-opened).
func NextWOStatus(in AcceptanceInput) WOStatus {
	if in.Current == WOCancelled {
		return WOCancelled
	}
	if in.HasSubmittedAfter && in.LatestAfterPassed && in.OpenFindings == 0 {
		return WOAccepted
	}
	if in.Current == WOAccepted {
		return WOCompleted
	}
	if in.Current == WOOpen && in.HasSubmittedWork {
		return WOInProgress
	}
	return in.Current
}

// SubmitCheck validates an inspection before it is submitted.
type SubmitCheck struct {
	Result         Result
	FindingsLinked int // findings recorded on this inspection
	UnratedLabels  []string
}

// ValidateSubmit returns the user-facing reasons an inspection cannot be submitted.
func ValidateSubmit(c SubmitCheck) []string {
	var errs []string
	if len(c.UnratedLabels) > 0 {
		errs = append(errs, fmt.Sprintf("قيّم جميع المعايير (غير المقيّم: %s)", Count(len(c.UnratedLabels), NounItem)))
	}
	if !c.Result.Scored && len(c.UnratedLabels) == 0 {
		errs = append(errs, "لا يمكن أن تكون جميع البنود «لا ينطبق»")
	}
	if c.Result.Scored && !c.Result.Passed && c.FindingsLinked == 0 {
		errs = append(errs, "النتيجة غير مطابقة: سجّل ملاحظة واحدة على الأقل مع الإجراء التصحيحي المطلوب")
	}
	return errs
}
