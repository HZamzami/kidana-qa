package domain

import (
	"fmt"
	"sort"
	"strings"
)

// InspectionRow is one submitted inspection with its work order context.
type InspectionRow struct {
	ID             int64
	WorkOrderID    int64
	Stage          Stage
	Score          float64
	Passed         bool
	CriticalFailed bool
	Type           string // PM | WO
	Site           string
	System         string
	Contractor     string
	Asset          string
	DelayDays      *int // work order actual − planned
}

// ItemRow is one rated criterion of a submitted inspection.
type ItemRow struct {
	InspectionID int64
	Code         string
	Label        string
	Rating       Rating
	Critical     bool
}

// FindingStats are counts of findings in the dashboard scope.
type FindingStats struct {
	Open, Overdue, OpenCritical int
}

// Group is a per-site / per-contractor / per-system aggregate.
type Group struct {
	Name     string
	Count    int
	Avg      float64
	PassRate float64
	Fails    int
}

// CriterionStat is the compliance of one criterion across inspections.
type CriterionStat struct {
	Code       string
	Label      string
	Critical   bool
	N          int
	Compliance float64 // average factor × 100
	Fails      int
}

// GradeCount is the number of inspections in one grade band.
type GradeCount struct {
	Label string
	Count int
}

// Summary is everything the dashboard, report and Excel summary show.
type Summary struct {
	Total, QACount, QCCount int
	AvgScore, AvgQA, AvgQC  *float64
	PassCount, FailCount    int
	PassRate                *float64
	CriticalFails           int
	WorkOrders              int
	OnTimeRate, AvgDelay    *float64
	LateCount               int
	Findings                FindingStats
	BySite, ByContractor    []Group
	BySystem                []Group
	Criteria                []CriterionStat
	Grades                  []GradeCount
	RepeatFailAssets        []AssetFails
}

// AssetFails counts failed inspections on one asset.
type AssetFails struct {
	Asset string
	Fails int
}

// Summarize aggregates submitted inspections.
func Summarize(rows []InspectionRow, items []ItemRow, fs FindingStats) Summary {
	S := Summary{Total: len(rows), Findings: fs}
	var all, qa, qc []float64
	woDelay := map[int64]*int{}
	failsByAsset := map[string]int{}

	for _, r := range rows {
		all = append(all, r.Score)
		if r.Stage == StageQA {
			S.QACount++
			qa = append(qa, r.Score)
		} else {
			S.QCCount++
			qc = append(qc, r.Score)
		}
		if r.Passed {
			S.PassCount++
		} else {
			S.FailCount++
			if r.Asset != "" {
				failsByAsset[r.Asset]++
			}
		}
		if r.CriticalFailed {
			S.CriticalFails++
		}
		woDelay[r.WorkOrderID] = r.DelayDays
	}
	S.AvgScore, S.AvgQA, S.AvgQC = avg(all), avg(qa), avg(qc)
	if S.Total > 0 {
		v := float64(S.PassCount) / float64(S.Total) * 100
		S.PassRate = &v
	}

	S.WorkOrders = len(woDelay)
	var timed, onTime int
	var lateSum float64
	for _, d := range woDelay {
		if d == nil {
			continue
		}
		timed++
		if *d <= 0 {
			onTime++
		} else {
			S.LateCount++
			lateSum += float64(*d)
		}
	}
	if timed > 0 {
		v := float64(onTime) / float64(timed) * 100
		S.OnTimeRate = &v
	}
	if S.LateCount > 0 {
		v := lateSum / float64(S.LateCount)
		S.AvgDelay = &v
	}

	S.BySite = groupBy(rows, func(r InspectionRow) string { return r.Site })
	S.ByContractor = groupBy(rows, func(r InspectionRow) string { return r.Contractor })
	S.BySystem = groupBy(rows, func(r InspectionRow) string { return r.System })
	S.Criteria = criteriaStats(items)

	for _, g := range Grades {
		S.Grades = append(S.Grades, GradeCount{Label: g.Label})
	}
	for _, r := range rows {
		label := GradeFor(r.Score)
		for i := range S.Grades {
			if S.Grades[i].Label == label {
				S.Grades[i].Count++
			}
		}
	}

	for a, n := range failsByAsset {
		if n >= 2 {
			S.RepeatFailAssets = append(S.RepeatFailAssets, AssetFails{a, n})
		}
	}
	sort.Slice(S.RepeatFailAssets, func(i, j int) bool { return S.RepeatFailAssets[i].Fails > S.RepeatFailAssets[j].Fails })
	return S
}

func groupBy(rows []InspectionRow, key func(InspectionRow) string) []Group {
	idx := map[string]*struct {
		scores []float64
		pass   int
	}{}
	var order []string
	for _, r := range rows {
		k := key(r)
		if k == "" {
			k = "غير محدد"
		}
		g, ok := idx[k]
		if !ok {
			g = &struct {
				scores []float64
				pass   int
			}{}
			idx[k] = g
			order = append(order, k)
		}
		g.scores = append(g.scores, r.Score)
		if r.Passed {
			g.pass++
		}
	}
	out := make([]Group, 0, len(order))
	for _, k := range order {
		g := idx[k]
		n := len(g.scores)
		out = append(out, Group{
			Name: k, Count: n, Avg: *avg(g.scores),
			PassRate: float64(g.pass) / float64(n) * 100, Fails: n - g.pass,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Avg > out[j].Avg })
	return out
}

func criteriaStats(items []ItemRow) []CriterionStat {
	type acc struct {
		CriterionStat
		sum float64
	}
	idx := map[string]*acc{}
	var order []string
	for _, it := range items {
		f, ok := it.Rating.Factor()
		if !ok {
			continue
		}
		k := it.Code + "|" + it.Label
		a, exists := idx[k]
		if !exists {
			a = &acc{CriterionStat: CriterionStat{Code: it.Code, Label: it.Label, Critical: it.Critical}}
			idx[k] = a
			order = append(order, k)
		}
		a.N++
		a.sum += f * 100
		if it.Rating == RatingNonCompliant {
			a.Fails++
		}
	}
	out := make([]CriterionStat, 0, len(order))
	for _, k := range order {
		a := idx[k]
		a.Compliance = a.sum / float64(a.N)
		out = append(out, a.CriterionStat)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func avg(xs []float64) *float64 {
	if len(xs) == 0 {
		return nil
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	v := s / float64(len(xs))
	return &v
}

// Insight is one observation or recommendation shown on the dashboard.
type Insight struct {
	Tone string // good | info | risk
	Text string
}

// Pct formats a percentage with one decimal. The value is wrapped in Unicode
// left-to-right isolates so "50%" is not displayed as "%50" inside Arabic text.
func Pct(v *float64) string {
	if v == nil {
		return "—"
	}
	return PctVal(*v)
}

// PctVal is Pct for a plain value.
func PctVal(v float64) string { return "\u2066" + Fmt1(v) + "%\u2069" }

// Fmt1 formats a number with at most one decimal.
func Fmt1(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}

// Insights derives patterns and recommendations, including QA/QC gaps and
// critical-criteria alerts.
func Insights(S Summary) []Insight {
	if S.Total == 0 {
		return []Insight{{"info", "لا توجد فحوصات مرسلة ضمن النطاق المحدد بعد. ابدأ بفتح أمر عمل وتسجيل رقابة أثناء التنفيذ أو فحص بعد التنفيذ."}}
	}
	var out []Insight
	if S.PassRate != nil {
		if *S.PassRate >= 90 {
			out = append(out, Insight{"good", fmt.Sprintf("نسبة المطابقة %s — أداء مستقر فوق المستهدف. يوصى بالحفاظ على وتيرة التدقيق الحالية.", Pct(S.PassRate))})
		} else {
			tone := "info"
			if *S.PassRate < 70 {
				tone = "risk"
			}
			out = append(out, Insight{tone, fmt.Sprintf("نسبة المطابقة %s (غير المطابق %d من %s). يوصى بزيادة عينة التدقيق على الجهات الأقل أداءً.", Pct(S.PassRate), S.FailCount, Count(S.Total, NounInspection))})
		}
	}
	if S.CriticalFails > 0 || S.Findings.OpenCritical > 0 {
		out = append(out, Insight{"risk", fmt.Sprintf("خطر سلامة: إخفاق في بند حرج في %s، والملاحظات الحرجة المفتوحة: %d. يوصى بإيقاف العمل غير الآمن فوراً وتصعيدها لإدارة السلامة.", Count(S.CriticalFails, NounInspection), S.Findings.OpenCritical)})
	}
	weak := make([]CriterionStat, len(S.Criteria))
	copy(weak, S.Criteria)
	sort.Slice(weak, func(i, j int) bool { return weak[i].Compliance < weak[j].Compliance })
	if len(weak) > 0 && weak[0].Compliance < 85 {
		w := weak[0]
		out = append(out, Insight{"risk", fmt.Sprintf("أضعف معيار: «%s» بنسبة مطابقة %s (عدم المطابقة: %s). يوصى بخطة تحسين مركّزة وتوعية الفنيين على هذا البند.", w.Label, PctVal(w.Compliance), Count(w.Fails, NounCase))})
	}
	var lowSites []string
	for _, g := range S.BySite {
		if g.Avg < PassThreshold {
			lowSites = append(lowSites, fmt.Sprintf("%s (%s)", g.Name, PctVal(g.Avg)))
		}
	}
	if len(lowSites) > 0 {
		out = append(out, Insight{"risk", fmt.Sprintf("مواقع دون حد المطابقة (%s): %s.", PctVal(PassThreshold), strings.Join(lowSites, "، "))})
	}
	var lowCon []string
	for _, g := range S.ByContractor {
		if g.Avg < PassThreshold && g.Name != "غير محدد" {
			lowCon = append(lowCon, fmt.Sprintf("%s (%s)", g.Name, PctVal(g.Avg)))
		}
	}
	if len(lowCon) > 0 {
		out = append(out, Insight{"risk", fmt.Sprintf("مقاولون دون المستهدف: %s. يوصى بتفعيل بنود الجزاءات/الإنذار حسب العقد ومتابعة خطة تصحيحية.", strings.Join(lowCon, "، "))})
	}
	if S.Findings.Overdue > 0 {
		out = append(out, Insight{"risk", fmt.Sprintf("ملاحظات تجاوزت تاريخ الإغلاق المستهدف: %d. يوصى بمتابعة أسبوعية مع مسؤولي المعالجة.", S.Findings.Overdue)})
	}
	if S.OnTimeRate != nil && *S.OnTimeRate < 90 {
		delay := "—"
		if S.AvgDelay != nil {
			delay = Fmt1(*S.AvgDelay)
		}
		out = append(out, Insight{"info", fmt.Sprintf("الالتزام بالموعد %s — متوسط التأخير %s يوم، وأوامر العمل المتأخرة: %d. راجع توزيع الموارد وجدولة الصيانة الوقائية.", Pct(S.OnTimeRate), delay, S.LateCount)})
	}
	if len(S.RepeatFailAssets) > 0 {
		parts := make([]string, 0, len(S.RepeatFailAssets))
		for _, a := range S.RepeatFailAssets {
			parts = append(parts, fmt.Sprintf("%s (%d)", a.Asset, a.Fails))
		}
		out = append(out, Insight{"risk", fmt.Sprintf("إخفاق متكرر على أصول: %s. يوصى بتحليل السبب الجذري (RCA).", strings.Join(parts, "، "))})
	}
	if S.AvgQA != nil && S.AvgQC != nil {
		diff := *S.AvgQA - *S.AvgQC
		if diff >= 10 {
			out = append(out, Insight{"info", fmt.Sprintf("أداء أثناء التنفيذ (%s) أعلى من نتيجة الفحص بعد التنفيذ (%s): العمل يُنفَّذ بطريقة سليمة لكن النتيجة النهائية أضعف — راجع جودة المخرجات والتوثيق.", Pct(S.AvgQA), Pct(S.AvgQC))})
		} else if diff <= -10 {
			out = append(out, Insight{"info", fmt.Sprintf("نتيجة الفحص بعد التنفيذ (%s) أعلى من الرقابة أثناء التنفيذ (%s): النتائج مقبولة لكن طريقة العمل تحتاج ضبطاً — ركّز على السلامة والإجراءات.", Pct(S.AvgQC), Pct(S.AvgQA))})
		}
	}
	return out
}
