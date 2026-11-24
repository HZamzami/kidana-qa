// Command seed fills a development database with realistic dummy data:
// inspectors, contractors and work orders spread over the last months, with
// submitted during/after inspections, findings and the statuses they imply.
// Scores come from domain.Evaluate and statuses from domain.NextWOStatus, so
// the data obeys the same rules as data entered through the app.
//
//	go run ./cmd/seed            # refuses if work orders already exist
//	go run ./cmd/seed -reset     # wipes work orders, contractors and demo users first
//
// It never runs when APP_ENV=production.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/config"
	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
)

const (
	demoDomain   = "demo.kidana.local"
	demoPassword = "demo-password"
)

func main() {
	n := flag.Int("n", 150, "number of work orders")
	reset := flag.Bool("reset", false, "delete existing work orders, contractors and demo users first")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log, *n, *reset); err != nil {
		log.Error("seed failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(log *slog.Logger, n int, reset bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.IsDev() {
		return errors.New("refusing to seed dummy data when APP_ENV=production")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	var existing int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM work_orders").Scan(&existing); err != nil {
		return err
	}
	if existing > 0 && !reset {
		return fmt.Errorf("the database already has %d work orders; run with -reset to replace them", existing)
	}

	hash, err := auth.HashPassword(demoPassword)
	if err != nil {
		return err
	}
	s := &seeder{rng: rand.New(rand.NewPCG(2026, 11)), today: today(), hash: hash} //nolint:gosec // dummy data, not security
	err = db.InTx(ctx, pool, func(tx pgx.Tx) error {
		if reset {
			// Targeted deletes, not TRUNCATE ... CASCADE: users reference
			// contractors, and a cascade would also empty users and checklists.
			for _, q := range []string{
				`DELETE FROM work_orders`, // inspections, findings, photos and assignments go with them
				`DELETE FROM audit_log`,
				`DELETE FROM report_runs`,
				`DELETE FROM users WHERE email LIKE '%@` + demoDomain + `'`,
				`DELETE FROM contractors WHERE id NOT IN (SELECT contractor_id FROM users WHERE contractor_id IS NOT NULL)`,
				`DELETE FROM assets`,
			} {
				if _, err := tx.Exec(ctx, q); err != nil {
					return err
				}
			}
		}
		s.tx, s.q = tx, gen.New(tx)
		if err := ensureAdmin(ctx, tx, cfg); err != nil {
			return err
		}
		return s.seed(ctx, n)
	})
	if err != nil {
		return err
	}
	log.Info("seeded dummy data",
		slog.Int("work_orders", n), slog.Int("inspections", s.inspections), slog.Int("findings", s.findings),
		slog.String("inspector_login", "insp1@"+demoDomain), slog.String("password", demoPassword))
	return nil
}

// ensureAdmin creates the ADMIN_* account when no admin exists. The server
// only does this for an empty users table, which the demo inspectors would
// otherwise prevent when seeding runs before the first start.
func ensureAdmin(ctx context.Context, tx pgx.Tx, cfg config.Config) error {
	var admins int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE role = 'admin'").Scan(&admins); err != nil || admins > 0 {
		return err
	}
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return errors.New("no admin exists: set ADMIN_EMAIL and ADMIN_PASSWORD in .env")
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	_, err = gen.New(tx).CreateUser(ctx, gen.CreateUserParams{
		Email: cfg.AdminEmail, Name: cfg.AdminName, Role: gen.UserRoleAdmin, PasswordHash: hash,
	})
	return err
}

// today is midnight UTC of the current Riyadh date, like DATE values from Postgres.
func today() time.Time {
	y, m, d := time.Now().In(dates.Riyadh).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type lookup struct {
	ID   int64
	Name string
}

type checklist struct {
	TemplateID int64
	Criteria   []gen.TemplateCriterion
}

type contractor struct {
	ID      int64
	Name    string
	Quality float64 // chance a criterion is fully compliant
	User    int64   // the contractor's portal user
}

type seeder struct {
	tx    pgx.Tx // for backdating timestamps the queries set to now()
	q     *gen.Queries
	rng   *rand.Rand
	today time.Time
	hash  string

	sites, systems, groups []lookup
	contractors            []contractor
	inspectors             []int64
	lists                  map[gen.InspectionStage]checklist

	inspections, findings int
}

func (s *seeder) seed(ctx context.Context, n int) error {
	if err := s.loadReference(ctx); err != nil {
		return err
	}
	if err := s.createPeople(ctx); err != nil {
		return err
	}
	// Oldest first, so the newest work orders are listed first in the app.
	start := s.today.AddDate(0, 0, -180)
	seq := map[gen.WoType]int{}
	for i := range n {
		planned := start.AddDate(0, 0, i*185/n+s.rng.IntN(5))
		if err := s.workOrder(ctx, planned, seq); err != nil {
			return err
		}
	}
	// Last month's report, as the scheduler would have made it.
	first := time.Date(s.today.Year(), s.today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	if _, err := s.q.CreateReportRun(ctx, gen.CreateReportRunParams{Kind: "monthly", Period: first}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

func (s *seeder) loadReference(ctx context.Context) error {
	sites, err := s.q.ListActiveSites(ctx)
	if err != nil {
		return err
	}
	for _, x := range sites {
		s.sites = append(s.sites, lookup{x.ID, x.Name})
	}
	systems, err := s.q.ListActiveSystems(ctx)
	if err != nil {
		return err
	}
	for _, x := range systems {
		s.systems = append(s.systems, lookup{x.ID, x.Name})
	}
	groups, err := s.q.ListActiveAssetGroups(ctx)
	if err != nil {
		return err
	}
	for _, x := range groups {
		s.groups = append(s.groups, lookup{x.ID, x.Name})
	}
	if len(s.sites) == 0 || len(s.systems) == 0 || len(s.groups) == 0 {
		return errors.New("sites, systems and asset groups must exist (see migration 00002)")
	}
	s.lists = map[gen.InspectionStage]checklist{}
	for _, st := range []gen.InspectionStage{gen.InspectionStageDuring, gen.InspectionStageAfter} {
		t, err := s.q.GetActiveTemplate(ctx, st)
		if err != nil {
			return fmt.Errorf("active %s checklist: %w", st, err)
		}
		cs, err := s.q.ListCriteria(ctx, t.ID)
		if err != nil {
			return err
		}
		s.lists[st] = checklist{TemplateID: t.ID, Criteria: cs}
	}
	return nil
}

func (s *seeder) createPeople(ctx context.Context) error {
	for i, name := range []string{"عبدالله الحربي", "فهد الغامدي", "سعد القحطاني", "ماجد الزهراني"} {
		u, err := s.q.CreateUser(ctx, gen.CreateUserParams{
			Email: fmt.Sprintf("insp%d@%s", i+1, demoDomain), Name: name, Role: gen.UserRoleInspector, PasswordHash: s.hash,
		})
		if err != nil {
			return err
		}
		s.inspectors = append(s.inspectors, u.ID)
	}
	for _, c := range []struct {
		name, nameEn, contact, phone string
		quality                      float64
	}{
		{"شركة الأنظمة المتكاملة للصيانة", "Integrated Systems Maintenance Co.", "خالد العتيبي", "0551234567", 0.90},
		{"مؤسسة الريان للمقاولات", "Al-Rayyan Contracting Est.", "أحمد الشهري", "0559876543", 0.82},
		{"شركة البيئة الحديثة", "Modern Environment Co.", "ناصر المالكي", "0504567890", 0.76},
		{"مجموعة الإتقان الفنية", "Al-Itqan Technical Group", "تركي الدوسري", "0532345678", 0.86},
		{"شركة الصيانة الشاملة", "Comprehensive Maintenance Co.", "يوسف البقمي", "0546543210", 0.70},
		{"مؤسسة أركان التشغيل", "Arkan Operations Est.", "سلطان العمري", "0567890123", 0.80},
	} {
		x, err := s.q.CreateContractor(ctx, gen.CreateContractorParams{Name: c.name, NameEn: c.nameEn, ContactName: c.contact, Phone: c.phone})
		if err != nil {
			return err
		}
		u, err := s.q.CreateUser(ctx, gen.CreateUserParams{
			Email: fmt.Sprintf("contractor%d@%s", len(s.contractors)+1, demoDomain), Name: c.contact,
			Role: gen.UserRoleContractor, PasswordHash: s.hash, ContractorID: &x.ID,
		})
		if err != nil {
			return err
		}
		s.contractors = append(s.contractors, contractor{x.ID, c.name, c.quality, u.ID})
	}
	return nil
}

// groupsBySystem pairs systems with plausible asset groups: a system whose
// name contains the key draws from those groups (matched by name prefix, so
// edited lists still work); any other system draws from every group.
var groupsBySystem = map[string][]string{
	"المصاعد":       {"المصاعد"},
	"الحريق":        {"شبكات الرشاشات", "لوحات إنذار الحريق", "المضخات"},
	"مياه الشرب":    {"المضخات", "خزانات المياه"},
	"دورات المياه":  {"المضخات", "خزانات المياه", "الإنارة", "الأبواب والنوافذ"},
	"الجهد الحراري": {"المضخات", "وحدات مناولة الهواء", "وحدات التكييف المنفصلة"},
	"تظليل":         {"الإنارة", "المضخات"},
	"مخيمات":        {"وحدات التكييف المنفصلة", "الإنارة", "لوحات التوزيع", "شبكات الرشاشات"},
	"أبراج":         {"المبردات", "وحدات مناولة الهواء", "المصاعد", "لوحات التوزيع", "الإنارة"},
	"مباني":         {"المبردات", "وحدات مناولة الهواء", "وحدات التكييف المنفصلة", "لوحات التوزيع"},
	"امن":           {"كاميرات المراقبة", "لوحات إنذار الحريق"},
	"قطار":          {"لوحات التوزيع", "المولدات", "كاميرات المراقبة"},
	"الجمرات":       {"المصاعد", "الإنارة", "وحدات مناولة الهواء", "كاميرات المراقبة"},
	"محطة":          {"المضخات", "المولدات", "لوحات التوزيع"},
}

var assetPrefix = map[string]string{
	"المبردات": "CH", "وحدات مناولة": "AHU", "وحدات التكييف": "SPL", "لوحات التوزيع": "DB", "المولدات": "GEN",
	"المضخات": "PMP", "خزانات": "TNK", "شبكات": "SPR", "لوحات إنذار": "FAP", "المصاعد": "LFT", "الإنارة": "LGT",
	"كاميرات": "CAM", "الأبواب": "DR",
}

var pmTasks = []string{
	"صيانة وقائية دورية وفق الخطة الشهرية",
	"فحص وتنظيف وقياس القراءات التشغيلية",
	"صيانة ربع سنوية واستبدال المرشحات",
	"اختبار تشغيلي وشد التوصيلات",
}

var woTasks = []string{
	"بلاغ: الوحدة لا تعمل بالكفاءة المطلوبة",
	"بلاغ: صوت غير طبيعي أثناء التشغيل",
	"بلاغ: تسرب ظاهر حول المعدة",
	"بلاغ: انقطاع متكرر وفصل القاطع",
	"بلاغ: عطل في لوحة التحكم وإنذار مستمر",
}

var technicians = []string{"محمد علي", "رامي حسن", "عمر خان", "سامر يوسف", "جمال أحمد", "وليد محمود", "أنور شريف", "حسين عباس"}

func (s *seeder) pick(xs []lookup) lookup { return xs[s.rng.IntN(len(xs))] }

func (s *seeder) groupFor(system string) lookup {
	for key, names := range groupsBySystem {
		if !strings.Contains(system, key) {
			continue
		}
		var match []lookup
		for _, g := range s.groups {
			for _, n := range names {
				if strings.HasPrefix(g.Name, n) {
					match = append(match, g)
				}
			}
		}
		if len(match) > 0 {
			return s.pick(match)
		}
	}
	return s.pick(s.groups)
}

func (s *seeder) assetCode(group, site string) string {
	code := "AST"
	for prefix, c := range assetPrefix {
		if strings.HasPrefix(group, prefix) {
			code = c
		}
	}
	siteCode := map[string]string{"منى": "MN", "عرفات": "AR", "مزدلفة": "MZ", "مكة المكرمة": "MK"}[site]
	if siteCode == "" {
		siteCode = "HQ"
	}
	return fmt.Sprintf("%s-%s-%03d", code, siteCode, 1+s.rng.IntN(240))
}

func (s *seeder) chance(p float64) bool { return s.rng.Float64() < p }

func (s *seeder) workOrder(ctx context.Context, planned time.Time, seq map[gen.WoType]int) error {
	typ := gen.WoTypePM
	if s.chance(0.45) {
		typ = gen.WoTypeWO
	}
	seq[typ]++
	priority := gen.WoPriorityNormal
	if typ == gen.WoTypeWO {
		switch r := s.rng.Float64(); {
		case r < 0.15:
			priority = gen.WoPriorityEmergency
		case r < 0.45:
			priority = gen.WoPriorityHigh
		}
	} else if s.chance(0.1) {
		priority = gen.WoPriorityHigh
	}
	site := s.pick(s.sites)
	system := s.pick(s.systems)
	group := s.groupFor(system.Name)
	c := s.contractors[s.rng.IntN(len(s.contractors))]
	task := pmTasks[s.rng.IntN(len(pmTasks))]
	if typ == gen.WoTypeWO {
		task = woTasks[s.rng.IntN(len(woTasks))]
	}

	// Work planned in the last week may not be done yet.
	var actual *time.Time
	if planned.Before(s.today.AddDate(0, 0, -7)) || s.chance(0.4) {
		delay := []int{-1, 0, 0, 0, 0, 1, 1, 2, 3, 5, 8}[s.rng.IntN(11)]
		a := planned.AddDate(0, 0, delay)
		if a.After(s.today) {
			a = s.today
		}
		actual = &a
	}
	if planned.After(s.today) {
		actual = nil
	}

	wo, err := s.q.CreateWorkOrder(ctx, gen.CreateWorkOrderParams{
		Ref: fmt.Sprintf("%s-%d-%04d", typ, planned.Year(), 1000+seq[typ]), Type: typ,
		SiteID: site.ID, Location: fmt.Sprintf("مخيم %d — الوحدة %d", 100+s.rng.IntN(400), 1+s.rng.IntN(8)),
		SystemID: system.ID, AssetGroupID: group.ID, Asset: s.assetCode(group.Name, site.Name),
		ContractorID: &c.ID, Technician: technicians[s.rng.IntN(len(technicians))], Priority: priority,
		Description: task, PlannedDate: &planned, ActualDate: actual,
	})
	if err != nil {
		return err
	}
	// Registered a few days before it was due, as it would be in the CMMS.
	registered := planned.AddDate(0, 0, -1-s.rng.IntN(6)).Add(8*time.Hour + time.Duration(s.rng.IntN(9*60))*time.Minute)
	if _, err := s.tx.Exec(ctx, "UPDATE work_orders SET created_at = $2::timestamptz, updated_at = $2::timestamptz, reported_on = ($2::timestamptz)::date WHERE id = $1", wo.ID, registered); err != nil {
		return err
	}

	if actual == nil {
		// Not done yet: sometimes inspected while the work is under way.
		if !planned.After(s.today) && s.chance(0.35) {
			if _, err := s.inspect(ctx, wo.ID, gen.InspectionStageDuring, 1, planned, c); err != nil {
				return err
			}
		}
		if err := s.assign(ctx, wo.ID, gen.InspectionStageAfter, planned.AddDate(0, 0, 3), gen.AssignmentReasonSampling); err != nil {
			return err
		}
		return s.finish(ctx, wo.ID, false)
	}
	if s.chance(0.03) {
		return s.q.SetWorkOrderStatus(ctx, gen.SetWorkOrderStatusParams{ID: wo.ID, Status: gen.WoStatusCancelled})
	}
	if s.chance(0.55) {
		day := planned
		if actual.After(planned) {
			day = planned.AddDate(0, 0, s.rng.IntN(int(actual.Sub(planned).Hours()/24)+1))
		}
		if _, err := s.inspect(ctx, wo.ID, gen.InspectionStageDuring, 1, day, c); err != nil {
			return err
		}
	}
	if s.chance(0.1) {
		// done, after-work inspection still pending
		if err := s.assign(ctx, wo.ID, gen.InspectionStageAfter, actual.AddDate(0, 0, 2), gen.AssignmentReasonManual); err != nil {
			return err
		}
		return s.finish(ctx, wo.ID, true)
	}
	day := minDate(actual.AddDate(0, 0, s.rng.IntN(3)), s.today)
	passed := false
	for attempt := 1; attempt <= 3; attempt++ {
		var err error
		if passed, err = s.inspect(ctx, wo.ID, gen.InspectionStageAfter, attempt, day, c); err != nil {
			return err
		}
		next := day.AddDate(0, 0, 3+s.rng.IntN(5))
		if passed || next.After(s.today) || s.chance(0.2) {
			break // passed, too recent to re-inspect, or the contractor has not fixed it yet
		}
		day = next
		c.Quality = min(c.Quality+0.08, 0.97) // the re-inspection follows a fix
	}
	if !passed {
		if err := s.assign(ctx, wo.ID, gen.InspectionStageAfter, day.AddDate(0, 0, 7), gen.AssignmentReasonReinspection); err != nil {
			return err
		}
	}
	return s.finish(ctx, wo.ID, true)
}

func minDate(a, b time.Time) time.Time {
	if a.After(b) {
		return b
	}
	return a
}

// rate picks a rating; quality is the chance of full compliance.
func (s *seeder) rate(c gen.TemplateCriterion, quality float64) domain.Rating {
	if c.IsCritical {
		quality = min(quality+0.12, 0.99) // safety items are watched closely and rarely fail outright
	}
	r := s.rng.Float64()
	switch {
	case !c.IsCritical && r < 0.04:
		return domain.RatingNA
	case r < quality:
		return domain.RatingCompliant
	case r < quality+(1-quality)*0.6:
		return domain.RatingPartial
	}
	return domain.RatingNonCompliant
}

var findingText = map[string][2]string{
	"D1": {"لا يوجد عزل وإقفال (LOTO) على مصدر الطاقة أثناء العمل", "إيقاف العمل فوراً وتطبيق العزل والإقفال قبل الاستئناف"},
	"D2": {"تخطي خطوات من قائمة الفحص المعتمدة للأصل", "إعادة تنفيذ الخطوات الناقصة وفق الإجراء المعتمد"},
	"D3": {"استخدام قطعة غيار غير مطابقة للمواصفات", "استبدال القطعة بأخرى معتمدة وتقديم شهادة المطابقة"},
	"D4": {"جهاز القياس منتهي المعايرة", "استخدام جهاز معاير وإعادة أخذ القراءات"},
	"D5": {"منطقة العمل غير معزولة بالحواجز واللوحات التحذيرية", "عزل منطقة العمل وتركيب اللوحات التحذيرية"},
	"D6": {"تأخر وصول الفريق عن الموعد المجدول", "الالتزام بالجدول وتقديم خطة لتحسين زمن الاستجابة"},
	"A1": {"الأصل لا يعمل ضمن القيم التصميمية بعد الصيانة", "معالجة العطل وإعادة الاختبار التشغيلي بحضور المفتش"},
	"A2": {"لم تُعد الأغطية والحمايات إلى مكانها بعد العمل", "إعادة تركيب الحمايات والملصقات قبل تسليم الأصل"},
	"A3": {"قراءات تشغيلية ناقصة في قائمة الفحص", "استكمال القراءات وإرفاقها في أمر العمل"},
	"A4": {"توصيلات غير مثبتة بإحكام وتشطيب غير مكتمل", "إحكام التوصيلات واستكمال التشطيب"},
	"A5": {"مخلفات متروكة في موقع العمل", "إزالة المخلفات وتنظيف الموقع"},
	"A6": {"لم تُرفق صور قبل وبعد ولم يُغلق أمر العمل في النظام", "إرفاق الصور وإغلاق أمر العمل في نظام إدارة الصيانة"},
}

// inspect creates and submits one inspection, raising findings when it fails.
func (s *seeder) inspect(ctx context.Context, woID int64, stage gen.InspectionStage, attempt int, day time.Time, c contractor) (bool, error) {
	list := s.lists[stage]
	inspector := s.inspectors[s.rng.IntN(len(s.inspectors))]
	in, err := s.q.CreateInspection(ctx, gen.CreateInspectionParams{
		WorkOrderID: woID, Stage: stage, Attempt: int32(attempt), TemplateID: list.TemplateID, //nolint:gosec // attempt ≤ 3
		InspectorID: inspector, InspectedOn: day,
	})
	if err != nil {
		return false, err
	}
	if err := s.q.CreateInspectionItems(ctx, in.ID); err != nil {
		return false, err
	}
	items := make([]domain.Item, len(list.Criteria))
	ratings := make([]domain.Rating, len(list.Criteria))
	for i, cr := range list.Criteria {
		r := s.rate(cr, c.Quality)
		ratings[i] = r
		items[i] = domain.Item{Weight: int(cr.Weight), Critical: cr.IsCritical, Rating: r}
		gr := gen.Rating(r)
		if _, err := s.q.SetItemRating(ctx, gen.SetItemRatingParams{InspectionID: in.ID, CriterionID: cr.ID, Rating: &gr}); err != nil {
			return false, err
		}
	}
	res := domain.Evaluate(items)
	var bad []int
	if !res.Passed {
		// Every failed inspection carries at least one finding (the submit rule):
		// one per non-compliant item, else one on the weakest item.
		for i, r := range ratings {
			if r == domain.RatingNonCompliant {
				bad = append(bad, i)
			}
		}
		if len(bad) == 0 {
			bad = []int{weakest(ratings)}
		}
	} else if s.chance(0.2) {
		// A pass can still carry a minor finding on a partly compliant item.
		for i, r := range ratings {
			if r == domain.RatingPartial {
				bad = append(bad, i)
				break
			}
		}
	}
	for _, i := range bad {
		cr := list.Criteria[i]
		critical := cr.IsCritical && ratings[i] == domain.RatingNonCompliant
		if err := s.finding(ctx, woID, in.ID, cr, day, inspector, c, critical); err != nil {
			return false, err
		}
	}
	score, passed := res.Score, res.Passed
	if err := s.q.SubmitInspection(ctx, gen.SubmitInspectionParams{
		ID: in.ID, Score: &score, Passed: &passed, CriticalFailed: res.CriticalFailed,
	}); err != nil {
		return false, err
	}
	s.inspections++
	return passed, nil
}

// weakest returns the index of the lowest-rated applicable item.
func weakest(ratings []domain.Rating) int {
	idx, worst := 0, 2.0
	for i, r := range ratings {
		if f, ok := r.Factor(); ok && f < worst {
			idx, worst = i, f
		}
	}
	return idx
}

func (s *seeder) finding(ctx context.Context, woID, inID int64, cr gen.TemplateCriterion, day time.Time, by int64, c contractor, critical bool) error {
	var ref string
	if err := s.tx.QueryRow(ctx, "SELECT ref FROM work_orders WHERE id = $1", woID).Scan(&ref); err != nil {
		return err
	}
	text, ok := findingText[cr.Code]
	if !ok {
		text = [2]string{"عدم مطابقة في بند: " + cr.Label, "معالجة الملاحظة وإبلاغ المفتش لإعادة التحقق"}
	}
	severity := gen.FindingSeverityNormal
	if critical {
		severity = gen.FindingSeverityCritical
	}
	due := day.AddDate(0, 0, 7)
	if critical {
		due = day.AddDate(0, 0, 2)
	}
	f, err := s.q.CreateFinding(ctx, gen.CreateFindingParams{
		WorkOrderID: woID, InspectionID: &inID, CriterionID: &cr.ID, Severity: severity,
		Description: text[0], CorrectiveAction: text[1], Owner: c.Name, DueDate: &due, CreatedBy: &by,
	})
	if err != nil {
		return err
	}
	s.findings++
	// Older findings are mostly closed; recent ones are still being worked on.
	age := int(s.today.Sub(day).Hours() / 24)
	closed := false
	switch {
	case age > 30 && s.chance(0.8), age > 10 && s.chance(0.45):
		closed = true
		if err := s.q.CloseFinding(ctx, gen.CloseFindingParams{ID: f.ID, ClosureNote: "تم التحقق من تنفيذ الإجراء التصحيحي في الموقع", ClosedBy: &by}); err != nil {
			return err
		}
		closed := minDate(day.AddDate(0, 0, 1+s.rng.IntN(9)), s.today).Add(10 * time.Hour)
		_, err = s.tx.Exec(ctx, "UPDATE findings SET closed_at = $2, created_at = $3, updated_at = $2 WHERE id = $1", f.ID, closed, day.Add(9*time.Hour))
	case s.chance(0.35):
		// The contractor answered; the inspector has yet to verify.
		rc := []gen.RootCause{gen.RootCauseEquipment, gen.RootCauseProcedure, gen.RootCausePeople, gen.RootCauseMaterials}[s.rng.IntN(4)]
		_, err = s.q.RespondFinding(ctx, gen.RespondFindingParams{
			ID: f.ID, ResponseNote: "تمت المعالجة حسب الإجراء التصحيحي، والصور مرفقة في نظام المقاول.", RespondedBy: &c.User,
			RootCauseCategory: &rc, RootCause: "",
		})
		if err == nil {
			s.note(ctx, by, "finding_responded", map[string]string{"ref": ref}, "/findings/"+strconv.FormatInt(f.ID, 10)+"/edit")
		}
	case s.chance(0.3):
		_, err = s.q.UpdateFinding(ctx, gen.UpdateFindingParams{
			ID: f.ID, Description: f.Description, CorrectiveAction: f.CorrectiveAction, Owner: f.Owner,
			DueDate: f.DueDate, Severity: f.Severity, Status: gen.FindingStatusInProgress,
		})
	}
	if err == nil && !closed {
		s.note(ctx, c.User, "finding_raised", map[string]string{"ref": ref, "desc": f.Description}, "/findings/"+strconv.FormatInt(f.ID, 10)+"/edit")
	}
	return err
}

// finish sets the status the app would reach: "completed" once the work is
// done, then the acceptance rule decides.
func (s *seeder) finish(ctx context.Context, woID int64, done bool) error {
	if done {
		if err := s.q.SetWorkOrderStatus(ctx, gen.SetWorkOrderStatusParams{ID: woID, Status: gen.WoStatusCompleted}); err != nil {
			return err
		}
	}
	f, err := s.q.AcceptanceFacts(ctx, woID)
	if err != nil {
		return err
	}
	next := domain.NextWOStatus(domain.AcceptanceInput{
		Current: domain.WOStatus(f.Status), HasSubmittedAfter: f.HasSubmittedAfter, LatestAfterPassed: f.LatestAfterPassed,
		OpenFindings: int(f.OpenFindings), HasSubmittedWork: f.HasSubmitted,
	})
	return s.q.SetWorkOrderStatus(ctx, gen.SetWorkOrderStatusParams{ID: woID, Status: gen.WoStatus(next)})
}

// assign books an inspection: half go to an inspector (who is notified),
// the rest wait in the pool.
func (s *seeder) assign(ctx context.Context, woID int64, stage gen.InspectionStage, due time.Time, reason gen.AssignmentReason) error {
	var who *int64
	if reason == gen.AssignmentReasonReinspection || s.chance(0.5) {
		id := s.inspectors[s.rng.IntN(len(s.inspectors))]
		who = &id
	}
	a, err := s.q.CreateAssignment(ctx, gen.CreateAssignmentParams{
		WorkOrderID: woID, Stage: stage, AssigneeID: who, DueDate: &due, Reason: reason,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil || who == nil {
		return err
	}
	var ref string
	if err := s.tx.QueryRow(ctx, "SELECT ref FROM work_orders WHERE id = $1", woID).Scan(&ref); err != nil {
		return err
	}
	kind := "assigned"
	if reason == gen.AssignmentReasonReinspection {
		kind = "reinspection"
	}
	s.note(ctx, *who, kind, map[string]string{"ref": ref, "stage": string(a.Stage), "due": dates.DMY(due)}, "/work-orders/"+strconv.FormatInt(woID, 10))
	return nil
}

// note adds an in-app notification for the demo.
func (s *seeder) note(ctx context.Context, user int64, kind string, params map[string]string, link string) {
	b, _ := json.Marshal(params)
	_ = s.q.CreateNotification(ctx, gen.CreateNotificationParams{UserID: user, Kind: kind, Params: b, Link: link})
}
