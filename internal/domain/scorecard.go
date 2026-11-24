package domain

import "sort"

// ScoreWeights weight the parts of a contractor's composite score.
type ScoreWeights struct {
	After    int `json:"after"`    // average after-work score
	During   int `json:"during"`   // average during-work score
	SLA      int `json:"sla"`      // resolution targets met (%)
	Findings int `json:"findings"` // findings closed on time (%)
}

// DefaultScoreWeights apply until an admin changes them.
var DefaultScoreWeights = ScoreWeights{After: 40, During: 20, SLA: 20, Findings: 20}

// ScoreParts are a contractor's results, each 0–100 or -1 when unknown.
type ScoreParts struct {
	After, During, SLA, FindingsOnTime float64
}

// Composite is the weighted score out of 100. Unknown parts are left out
// and the remaining weights rescaled; -1 when nothing is known.
func Composite(p ScoreParts, w ScoreWeights) float64 {
	var sum, wsum float64
	for _, x := range []struct {
		v float64
		w int
	}{{p.After, w.After}, {p.During, w.During}, {p.SLA, w.SLA}, {p.FindingsOnTime, w.Findings}} {
		if x.v < 0 || x.w <= 0 {
			continue
		}
		sum += x.v * float64(x.w)
		wsum += float64(x.w)
	}
	if wsum == 0 {
		return -1
	}
	return sum / wsum
}

// Ranked is a contractor with its composite score and rank (1 = best).
type Ranked struct {
	ID    int64
	Score float64
	Rank  int
}

// Rank orders contractors by composite score, best first; those without a
// score come last, unranked (0).
func Rank(scores map[int64]float64) []Ranked {
	out := make([]Ranked, 0, len(scores))
	for id, s := range scores {
		out = append(out, Ranked{ID: id, Score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Score < 0) != (out[j].Score < 0) {
			return out[i].Score >= 0
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	for i := range out {
		if out[i].Score >= 0 {
			out[i].Rank = i + 1
		}
	}
	return out
}
