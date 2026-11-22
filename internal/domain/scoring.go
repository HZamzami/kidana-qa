// Package domain holds the business rules of the QA/QC system. It has no I/O
// and no dependencies on the database or HTTP layers, so it is easy to test.
package domain

import (
	"math"
	"time"
)

// PassThreshold is the minimum score (percent) for an inspection to pass.
const PassThreshold = 80.0

// Rating is the result recorded for one criterion.
type Rating string

const (
	RatingCompliant    Rating = "C"  // مطابق
	RatingPartial      Rating = "P"  // مطابق جزئياً
	RatingNonCompliant Rating = "N"  // غير مطابق
	RatingNA           Rating = "NA" // لا ينطبق
)

// Ratings in display order.
var Ratings = []Rating{RatingCompliant, RatingPartial, RatingNonCompliant, RatingNA}

// Valid reports whether r is a known rating.
func (r Rating) Valid() bool {
	switch r {
	case RatingCompliant, RatingPartial, RatingNonCompliant, RatingNA:
		return true
	}
	return false
}

// Factor is the share of a criterion's weight earned by the rating.
// ok is false for "not applicable", which is excluded from the calculation.
func (r Rating) Factor() (factor float64, ok bool) {
	switch r {
	case RatingCompliant:
		return 1, true
	case RatingPartial:
		return 0.5, true
	case RatingNonCompliant:
		return 0, true
	}
	return 0, false
}

// Label is the Arabic display label.
func (r Rating) Label() string {
	switch r {
	case RatingCompliant:
		return "مطابق"
	case RatingPartial:
		return "مطابق جزئياً"
	case RatingNonCompliant:
		return "غير مطابق"
	case RatingNA:
		return "لا ينطبق"
	}
	return ""
}

// ShortLabel is a compact label for tight layouts.
func (r Rating) ShortLabel() string {
	if r == RatingPartial {
		return "جزئي"
	}
	return r.Label()
}

// Grade band: a score of at least Min gets Label.
type Grade struct {
	Min   float64
	Label string
}

// Grades from best to worst (the bands Kidana already uses).
var Grades = []Grade{
	{90, "ممتاز"},
	{80, "جيد جداً"},
	{70, "جيد"},
	{60, "مقبول"},
	{0, "ضعيف"},
}

// GradeFor returns the grade label for a score.
func GradeFor(score float64) string {
	for _, g := range Grades {
		if score >= g.Min {
			return g.Label
		}
	}
	return Grades[len(Grades)-1].Label
}

// Item is one criterion as it takes part in scoring.
type Item struct {
	Weight   int
	Critical bool
	Rating   Rating // empty = not rated yet
}

// Result of evaluating an inspection.
type Result struct {
	Score          float64 // percent, valid only when Scored
	Scored         bool    // false when nothing applicable was rated
	Grade          string
	Passed         bool
	CriticalFailed bool // a critical criterion was rated non-compliant
	Rated          int  // criteria with any rating, including NA
	Total          int
}

// Complete reports whether every criterion has a rating.
func (r Result) Complete() bool { return r.Total > 0 && r.Rated == r.Total }

// Evaluate scores a set of rated criteria:
//
//	score = Σ(weight × factor) ÷ Σ(weight of applicable criteria) × 100
//
// An inspection passes when score ≥ PassThreshold and no critical criterion failed.
func Evaluate(items []Item) Result {
	res := Result{Total: len(items)}
	var earned, possible float64
	for _, it := range items {
		if it.Rating == "" || !it.Rating.Valid() {
			continue
		}
		res.Rated++
		if it.Critical && it.Rating == RatingNonCompliant {
			res.CriticalFailed = true
		}
		f, ok := it.Rating.Factor()
		if !ok {
			continue
		}
		possible += float64(it.Weight)
		earned += float64(it.Weight) * f
	}
	if possible == 0 {
		return res
	}
	res.Scored = true
	res.Score = math.Round(earned/possible*1000) / 10 // one decimal
	res.Grade = GradeFor(res.Score)
	res.Passed = res.Score >= PassThreshold && !res.CriticalFailed
	return res
}

// DelayDays returns actual − planned in whole days (positive = late).
func DelayDays(planned, actual *time.Time) (days int, ok bool) {
	if planned == nil || actual == nil {
		return 0, false
	}
	p := time.Date(planned.Year(), planned.Month(), planned.Day(), 0, 0, 0, 0, time.UTC)
	a := time.Date(actual.Year(), actual.Month(), actual.Day(), 0, 0, 0, 0, time.UTC)
	return int(a.Sub(p).Hours() / 24), true
}
