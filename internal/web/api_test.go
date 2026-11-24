package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/HZamzami/kidana-quality/internal/db/gen"
)

func (e *testEnv) api(method, path, token, body string) (int, map[string]any) {
	e.t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), method, e.srv.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return res.StatusCode, out
}

// TestWorkOrderAPI: tokens, create/update by ref, read back, list changes.
func TestWorkOrderAPI(t *testing.T) {
	e := setup(t)
	admin := e.login("admin@test.local")
	page := e.postBody(admin, "/admin/api", url.Values{"name": {"CMMS"}})
	m := regexp.MustCompile(`readonly value="([A-Za-z0-9_-]{40,})"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("token not shown once")
	}
	token := m[1]

	if code, _ := e.api("GET", "/api/v1/work-orders", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := e.api("GET", "/api/v1/work-orders", "nope", ""); code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", code)
	}
	body := `{"type":"WO","priority":"high","site":"منى","system":"منظومة جديدة","asset_group":"المضخات","asset":"PMP-9","contractor":"مقاول API","description":"تسرب"}`
	code, out := e.api("PUT", "/api/v1/work-orders/API-1", token, body)
	if code != http.StatusCreated || out["ref"] != "API-1" || out["planned_date"] == "" {
		t.Fatalf("create: %d %v", code, out)
	}
	code, out = e.api("PUT", "/api/v1/work-orders/api-1", token, strings.Replace(body, `"high"`, `"emergency"`, 1))
	if code != http.StatusOK || out["priority"] != "emergency" {
		t.Fatalf("update: %d %v", code, out)
	}
	if code, out := e.api("PUT", "/api/v1/work-orders/API-2", token, `{"type":"X"}`); code != http.StatusUnprocessableEntity || out["problems"] == nil {
		t.Fatalf("validation: %d %v", code, out)
	}
	if code, _ := e.api("PUT", "/api/v1/work-orders/API-2", token, `{"unknown":1}`); code != http.StatusBadRequest {
		t.Fatal("unknown fields must be refused")
	}
	if code, out := e.api("GET", "/api/v1/work-orders/API-1", token, ""); code != http.StatusOK || out["system"] != "منظومة جديدة" {
		t.Fatalf("get: %d %v", code, out)
	}
	if code, _ := e.api("GET", "/api/v1/work-orders/NOPE", token, ""); code != http.StatusNotFound {
		t.Fatal("missing ref")
	}
	code, out = e.api("GET", "/api/v1/work-orders?updated_since=2000-01-01T00:00:00Z", token, "")
	if list, _ := out["work_orders"].([]any); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: %d %v", code, out)
	}
	if code, _ := e.api("GET", "/api/v1/work-orders?updated_since=yesterday", token, ""); code != http.StatusBadRequest {
		t.Fatal("bad updated_since")
	}
	// Revoked tokens stop working.
	toks := must(e.q.ListAPITokens(context.Background()))
	e.post(admin, "/admin/api/"+idStr(toks[0].ID)+"/revoke", nil)
	if code, _ := e.api("GET", "/api/v1/work-orders", token, ""); code != http.StatusUnauthorized {
		t.Fatal("revoked token still works")
	}
}

// TestScopedChecklists: a checklist for one system is used for that system's
// work orders, and the general one for the rest.
func TestScopedChecklists(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	admin := e.login("admin@test.local")
	general := must(e.q.GetActiveTemplate(ctx, gen.InspectionStageAfter))
	systems, _ := e.q.ListActiveSystems(ctx)
	form := url.Values{"stage": {"after"}, "name": {"قائمة الجمرات"}, "activate": {"on"}, "rows": {"1"},
		"system_id": {idStr(systems[0].ID)}, "code_0": {"J1"}, "label_0": {"بند خاص"}, "weight_0": {"100"}}
	if res := e.post(admin, "/admin/checklists", form); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create scoped checklist: %d", res.StatusCode)
	}
	if still := must(e.q.GetActiveTemplate(ctx, gen.InspectionStageAfter)); still.ID != general.ID {
		t.Fatal("activating a scoped checklist must keep the general one active")
	}
	c := e.login("inspector@test.local")
	sites, _ := e.q.ListActiveSites(ctx)
	groups, _ := e.q.ListActiveAssetGroups(ctx)
	for i, sys := range []int64{systems[0].ID, systems[1].ID} {
		ref := "SC-" + idStr(int64(i))
		e.post(c, "/work-orders", url.Values{"type": {"PM"}, "ref": {ref}, "site_id": {idStr(sites[0].ID)}, "system_id": {idStr(sys)}, "asset_group_id": {idStr(groups[0].ID)}})
		wo := must(e.q.GetWorkOrderByRef(ctx, ref))
		e.post(c, "/work-orders/"+idStr(wo.ID)+"/inspections", url.Values{"stage": {"after"}})
		in := must(e.q.ListInspectionsByWorkOrder(ctx, wo.ID))[0]
		items := must(e.q.ListInspectionItems(ctx, in.ID))
		if i == 0 && (len(items) != 1 || items[0].Code != "J1") {
			t.Fatalf("scoped system must use its checklist, got %d items", len(items))
		}
		if i == 1 && in.TemplateID != general.ID {
			t.Fatal("other systems must use the general checklist")
		}
	}
}
