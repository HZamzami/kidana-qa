package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/config"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/storage"
)

// Integration tests run against TEST_DATABASE_URL (see compose.yaml / CI).
// They are skipped when it is not set.

type testEnv struct {
	t    *testing.T
	srv  *httptest.Server
	pool *pgxpool.Pool
	q    *gen.Queries
}

const testPassword = "test-password-123"

func setup(t *testing.T) *testEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Fresh schema every run (test database only): migrations + seed data.
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	q := gen.New(pool)
	hash, _ := auth.HashPassword(testPassword)
	for _, u := range []gen.CreateUserParams{
		{Email: "admin@test.local", Name: "مدير", Role: gen.UserRoleAdmin, PasswordHash: hash},
		{Email: "inspector@test.local", Name: "مدقق", Role: gen.UserRoleInspector, PasswordHash: hash},
		{Email: "viewer@test.local", Name: "مشاهد", Role: gen.UserRoleViewer, PasswordHash: hash},
	} {
		if _, err := q.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	store, err := storage.NewDisk(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Env: "development", SessionLifetime: time.Hour, MaxUploadBytes: 1 << 20}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(cfg, log, pool, store).Handler())
	t.Cleanup(srv.Close)
	return &testEnv{t: t, srv: srv, pool: pool, q: q}
}

// client returns an HTTP client that keeps cookies and does not follow redirects.
func (e *testEnv) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (e *testEnv) login(email string) *http.Client {
	c := e.client()
	res := e.post(c, "/login", url.Values{"email": {email}, "password": {testPassword}})
	if res.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("login %s: status %d", email, res.StatusCode)
	}
	return c
}

func (e *testEnv) post(c *http.Client, path string, form url.Values) *http.Response {
	e.t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return res
}

func (e *testEnv) get(c *http.Client, path string) (*http.Response, string) {
	e.t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.srv.URL+path, nil)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	return res, string(b)
}

func TestAuthAndGuards(t *testing.T) {
	e := setup(t)

	res, _ := e.get(e.client(), "/")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("anonymous dashboard: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	if res := e.post(e.client(), "/login", url.Values{"email": {"admin@test.local"}, "password": {"wrong"}}); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", res.StatusCode)
	}

	viewer := e.login("viewer@test.local")
	if res, _ := e.get(viewer, "/"); res.StatusCode != http.StatusOK {
		t.Fatalf("viewer dashboard: %d", res.StatusCode)
	}
	if res := e.post(viewer, "/work-orders", url.Values{"ref": {"X"}}); res.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer create work order: %d", res.StatusCode)
	}
	if res, _ := e.get(viewer, "/admin/users"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer admin: %d", res.StatusCode)
	}

	// Cross-site form posts are rejected (CSRF protection).
	admin := e.login("admin@test.local")
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.srv.URL+"/admin/lookups/sites", strings.NewReader("name=evil"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://evil.example")
	res, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site post: %d", res.StatusCode)
	}

	// Logout ends the session.
	e.post(admin, "/logout", nil)
	if res, _ := e.get(admin, "/"); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("after logout: %d", res.StatusCode)
	}
}

// TestInspectionFlow walks a work order through a failed QA (critical
// criterion), a passing QC and the closing of the finding.
func TestInspectionFlow(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := e.login("inspector@test.local")

	sites, _ := e.q.ListActiveSites(ctx)
	systems, _ := e.q.ListActiveSystems(ctx)
	res := e.post(c, "/work-orders", url.Values{
		"type": {"WO"}, "ref": {"WO-1001"}, "site_id": {idStr(sites[0].ID)}, "system_id": {idStr(systems[0].ID)},
		"planned_date": {"2026-11-01"}, "actual_date": {"2026-11-02"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create WO: %d", res.StatusCode)
	}
	wo, err := e.q.GetWorkOrderByRef(ctx, "WO-1001")
	if err != nil {
		t.Fatal(err)
	}
	// "save and add another" keeps the context, copy prefills the rest
	res = e.post(c, "/work-orders", url.Values{"type": {"PM"}, "ref": {"WO-1002"}, "site_id": {idStr(sites[0].ID)}, "system_id": {idStr(systems[0].ID)}, "asset": {"GEN-1"}, "next": {"new"}})
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(loc, "/work-orders/new?keep=ctx&from=") {
		t.Fatalf("save and add another: %d %s", res.StatusCode, loc)
	}
	if _, body := e.get(c, res.Header.Get("Location")); strings.Contains(body, "GEN-1") {
		t.Fatal("keep=ctx must not copy the asset")
	}
	wo2, _ := e.q.GetWorkOrderByRef(ctx, "WO-1002")
	if _, body := e.get(c, "/work-orders/new?from="+idStr(wo2.ID)); !strings.Contains(body, "GEN-1") || strings.Contains(body, "WO-1002") {
		t.Fatal("copy must prefill the asset but not the number")
	}
	// admin-only delete; refused once an inspection is submitted (checked below)
	if res := e.post(c, "/work-orders/"+idStr(wo2.ID)+"/delete", nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("inspector delete: %d", res.StatusCode)
	}
	admin := e.login("admin@test.local")
	if res := e.post(admin, "/work-orders/"+idStr(wo2.ID)+"/delete", nil); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("admin delete: %d", res.StatusCode)
	}
	if _, err := e.q.GetWorkOrder(ctx, wo2.ID); err == nil {
		t.Fatal("work order not deleted")
	}

	// duplicate ref is rejected
	if res := e.post(c, "/work-orders", url.Values{"type": {"WO"}, "ref": {"wo-1001"}, "site_id": {idStr(sites[0].ID)}, "system_id": {idStr(systems[0].ID)}}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate ref: %d", res.StatusCode)
	}

	start := func(stage string) int64 {
		res := e.post(c, "/work-orders/"+idStr(wo.ID)+"/inspections", url.Values{"stage": {stage}})
		if res.StatusCode != http.StatusSeeOther {
			t.Fatalf("start %s: %d", stage, res.StatusCode)
		}
		loc := res.Header.Get("Location")
		var id int64
		for _, in := range must(e.q.ListInspectionsByWorkOrder(ctx, wo.ID)) {
			if "/inspections/"+idStr(in.ID) == loc {
				id = in.ID
			}
		}
		if id == 0 {
			t.Fatalf("start %s: unexpected redirect %s", stage, loc)
		}
		return id
	}
	rateAll := func(inspID int64, rating func(it gen.ListInspectionItemsRow) string) {
		for _, it := range must(e.q.ListInspectionItems(ctx, inspID)) {
			res := e.post(c, "/inspections/"+idStr(inspID)+"/items/"+idStr(it.CriterionID), url.Values{"rating": {rating(it)}, "note": {"ok"}})
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("rate %s: %d", it.Code, res.StatusCode)
			}
		}
	}

	// QA: the critical safety criterion fails → inspection fails and is flagged critical.
	qa := start("qa")
	rateAll(qa, func(it gen.ListInspectionItemsRow) string {
		if it.IsCritical {
			return "N"
		}
		return "C"
	})
	if res := e.post(c, "/inspections/"+idStr(qa)+"/submit", nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("submit failed QA without finding should be refused: %d", res.StatusCode)
	}
	if res := e.post(c, "/inspections/"+idStr(qa)+"/findings", url.Values{"description": {"لا يوجد LOTO"}, "corrective_action": {"إيقاف العمل وتطبيق العزل"}, "critical": {"on"}}); res.StatusCode != http.StatusOK {
		t.Fatalf("add finding: %d", res.StatusCode)
	}
	if res := e.post(c, "/inspections/"+idStr(qa)+"/submit", nil); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit QA: %d", res.StatusCode)
	}
	in := must(e.q.GetInspection(ctx, qa))
	if in.Passed == nil || *in.Passed || !in.CriticalFailed || in.Score == nil || *in.Score != 70 {
		t.Fatalf("QA result: passed=%v critical=%v score=%v", in.Passed, in.CriticalFailed, in.Score)
	}
	if st := must(e.q.GetWorkOrder(ctx, wo.ID)).Status; st != gen.WoStatusInProgress {
		t.Fatalf("after QA: status %s", st)
	}
	// Submitted inspections are read-only.
	items := must(e.q.ListInspectionItems(ctx, qa))
	if res := e.post(c, "/inspections/"+idStr(qa)+"/items/"+idStr(items[0].CriterionID), url.Values{"rating": {"C"}}); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("edit submitted: %d", res.StatusCode)
	}

	// QC passes, but the open QA finding blocks acceptance.
	qc := start("qc")
	rateAll(qc, func(gen.ListInspectionItemsRow) string { return "C" })
	if res := e.post(c, "/inspections/"+idStr(qc)+"/submit", nil); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit QC: %d", res.StatusCode)
	}
	if st := must(e.q.GetWorkOrder(ctx, wo.ID)).Status; st == gen.WoStatusAccepted {
		t.Fatal("must not be accepted while a finding is open")
	}

	// Closing the finding accepts the work order.
	fs := must(e.q.ListFindingsByWorkOrder(ctx, wo.ID))
	if len(fs) != 1 {
		t.Fatalf("findings: %d", len(fs))
	}
	if res := e.post(c, "/findings/"+idStr(fs[0].ID)+"/close", url.Values{"closure_note": {"تم تطبيق العزل والتوعية"}}); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("close finding: %d", res.StatusCode)
	}
	if st := must(e.q.GetWorkOrder(ctx, wo.ID)).Status; st != gen.WoStatusAccepted {
		t.Fatalf("after closing finding: status %s", st)
	}

	if res := e.post(admin, "/work-orders/"+idStr(wo.ID)+"/delete", nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete with submitted inspections must be refused: %d", res.StatusCode)
	}

	// Pages render with data.
	for _, p := range []string{"/", "/work-orders", "/work-orders/" + idStr(wo.ID), "/inspections/" + idStr(qa), "/findings?scope=all", "/reports/print", "/data", "/?result=fail", "/reports/print?result=pass&stage=qc"} {
		if res, body := e.get(c, p); res.StatusCode != http.StatusOK || !strings.Contains(body, "</html>") {
			t.Fatalf("GET %s: %d", p, res.StatusCode)
		}
	}
	res2, _ := e.get(c, "/reports/export.xlsx")
	if res2.StatusCode != http.StatusOK || !strings.Contains(res2.Header.Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("export: %d %s", res2.StatusCode, res2.Header.Get("Content-Type"))
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
