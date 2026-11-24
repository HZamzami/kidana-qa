package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
)

// TestInsights: due date from priority, change history, search, saved views,
// scorecards and the admin pages behind them.
func TestInsights(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := e.login("inspector@test.local")
	admin := e.login("admin@test.local")
	sites, _ := e.q.ListActiveSites(ctx)
	systems, _ := e.q.ListActiveSystems(ctx)
	groups, _ := e.q.ListActiveAssetGroups(ctx)
	con := must(e.q.CreateContractor(ctx, gen.CreateContractorParams{Name: "مقاول"}))

	// An emergency WO without a due date is due the day after it was reported.
	e.post(c, "/work-orders", url.Values{
		"type": {"WO"}, "ref": {"WO-S1"}, "site_id": {idStr(sites[0].ID)}, "system_id": {idStr(systems[0].ID)},
		"asset_group_id": {idStr(groups[0].ID)}, "priority": {"emergency"}, "contractor_id": {idStr(con.ID)},
	})
	wo := must(e.q.GetWorkOrderByRef(ctx, "WO-S1"))
	if wo.PlannedDate == nil || !wo.PlannedDate.Equal(dates.Today().AddDate(0, 0, 1)) {
		t.Fatalf("due date from priority: %v", wo.PlannedDate)
	}

	// Editing records which fields changed; the history shows it.
	e.post(c, "/work-orders/"+idStr(wo.ID), url.Values{
		"type": {"WO"}, "ref": {"WO-S1"}, "site_id": {idStr(sites[0].ID)}, "system_id": {idStr(systems[0].ID)},
		"asset_group_id": {idStr(groups[0].ID)}, "priority": {"emergency"}, "status": {"open"}, "technician": {"أحمد"},
		"planned_date": {wo.PlannedDate.Format("2006-01-02")}, "contractor_id": {idStr(con.ID)},
	})
	hist := must(e.q.ListWorkOrderHistory(ctx, wo.ID))
	if len(hist) < 2 || !strings.Contains(string(hist[0].Details), `"technician"`) {
		t.Fatalf("history: %d entries, latest %s", len(hist), hist[0].Details)
	}
	if _, body := e.get(c, "/work-orders/"+idStr(wo.ID)); !strings.Contains(body, "أحمد") {
		t.Fatal("history not shown on the work order page")
	}
	if res, _ := e.get(admin, "/admin/audit?entity=work_order"); res.StatusCode != http.StatusOK {
		t.Fatal("audit page")
	}
	if res, _ := e.get(c, "/admin/audit"); res.StatusCode != http.StatusForbidden {
		t.Fatal("audit page is for admins")
	}

	// Search: an exact number jumps to the work order; partial text lists results.
	if res, _ := e.get(c, "/search?q=wo-s1"); res.Header.Get("Location") != "/work-orders/"+idStr(wo.ID) {
		t.Fatalf("exact search: %s", res.Header.Get("Location"))
	}
	if _, body := e.get(c, "/search?q=S1"); !strings.Contains(body, "WO-S1") {
		t.Fatal("partial search")
	}

	// Saved views.
	e.post(c, "/views", url.Values{"page": {"work-orders"}, "name": {"طارئ"}, "query": {"priority=emergency"}})
	if _, body := e.get(c, "/work-orders"); !strings.Contains(body, "priority=emergency") {
		t.Fatal("saved view chip missing")
	}
	if _, body := e.get(admin, "/work-orders"); strings.Contains(body, ">طارئ</a>") {
		t.Fatal("a private view must not show for others")
	}

	// SLA filter, scorecards, targets.
	if res, _ := e.get(c, "/work-orders?sla=at_risk"); res.StatusCode != http.StatusOK {
		t.Fatal("sla filter")
	}
	if _, body := e.get(c, "/contractors"); !strings.Contains(body, "مقاول") {
		t.Fatal("contractor ranking")
	}
	if res, _ := e.get(c, "/contractors/"+idStr(con.ID)); res.StatusCode != http.StatusOK {
		t.Fatal("scorecard")
	}
	if res, _ := e.get(c, "/reports/contractor/"+idStr(con.ID)); res.StatusCode != http.StatusOK {
		t.Fatal("contractor report")
	}
	e.post(admin, "/admin/sla", url.Values{"days_emergency": {"2"}, "w_after": {"50"}, "w_during": {"10"}, "w_sla": {"20"}, "w_findings": {"20"}})
	ts := must(e.q.ListSLATargets(ctx))
	for _, x := range ts {
		if x.Key == "emergency" && x.Days != 2 {
			t.Fatal("target not saved")
		}
	}
	if v := must(e.q.GetSetting(ctx, "scorecard_weights")); !strings.Contains(v, `"after":50`) {
		t.Fatalf("weights not saved: %s", v)
	}
}
