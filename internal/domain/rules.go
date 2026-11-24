package domain

import (
	"github.com/HZamzami/kidana-quality/internal/i18n"
)

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
	Lang           i18n.Lang // language of the messages (Arabic when empty)
}

// ValidateSubmit returns the user-facing reasons an inspection cannot be submitted.
func ValidateSubmit(c SubmitCheck) []string {
	var errs []string
	if len(c.UnratedLabels) > 0 {
		errs = append(errs, i18n.Tf(c.Lang, "قيّم جميع المعايير (غير المقيّم: %s)", CountIn(c.Lang, len(c.UnratedLabels), NounItem)))
	}
	if !c.Result.Scored && len(c.UnratedLabels) == 0 {
		errs = append(errs, i18n.T(c.Lang, "لا يمكن أن تكون جميع البنود «لا ينطبق»"))
	}
	if c.Result.Scored && !c.Result.Passed && c.FindingsLinked == 0 {
		errs = append(errs, i18n.T(c.Lang, "النتيجة غير مطابقة: سجّل ملاحظة واحدة على الأقل مع الإجراء التصحيحي المطلوب"))
	}
	return errs
}

// Verdict is the overall result of a work order, as shown on its printed
// record. Tone is one of good, bad, warn or neutral.
type Verdict struct {
	Label string
	Tone  string
}

// VerdictInput summarises what WOVerdict needs to know about a work order.
type VerdictInput struct {
	Status            WOStatus
	HasSubmittedAfter bool
	LatestAfterPassed bool
	OpenFindings      int
}

// WOVerdict explains where a work order stands against the acceptance rule:
// accepted, failed and awaiting re-inspection, passed with findings still
// open, or not yet inspected after the work.
func WOVerdict(in VerdictInput) Verdict {
	switch {
	case in.Status == WOCancelled:
		return Verdict{"ملغى", "neutral"}
	case in.Status == WOAccepted:
		return Verdict{"معتمد", "good"}
	case in.HasSubmittedAfter && !in.LatestAfterPassed:
		return Verdict{"غير مطابق — بانتظار إعادة الفحص", "bad"}
	case in.HasSubmittedAfter && in.OpenFindings > 0:
		return Verdict{"مطابق — بانتظار إغلاق الملاحظات", "warn"}
	}
	return Verdict{"بانتظار الفحص بعد التنفيذ", "neutral"}
}
