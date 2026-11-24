package web

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
)

// TestContractorPortal: a contractor user sees only their own work, responds
// to a finding with a photo, the inspector rejects then verifies it, and
// each step notifies the other side. Also covers sampling and assignments.
func TestContractorPortal(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	conA := must(e.q.CreateContractor(ctx, gen.CreateContractorParams{Name: "مقاول أ"}))
	conB := must(e.q.CreateContractor(ctx, gen.CreateContractorParams{Name: "مقاول ب"}))
	hash, _ := auth.HashPassword(testPassword)
	must(e.q.CreateUser(ctx, gen.CreateUserParams{Email: "con@test.local", Name: "مقاول", Role: gen.UserRoleContractor, PasswordHash: hash, ContractorID: &conA.ID}))
	if _, err := e.q.CreateUser(ctx, gen.CreateUserParams{Email: "bad@test.local", Name: "x", Role: gen.UserRoleContractor, PasswordHash: hash}); err == nil {
		t.Fatal("a contractor user without a contractor must be refused by the database")
	}

	insp := e.login("inspector@test.local")
	sites, _ := e.q.ListActiveSites(ctx)
	systems, _ := e.q.ListActiveSystems(ctx)
	groups, _ := e.q.ListActiveAssetGroups(ctx)
	mkWO := func(ref string, con int64, priority string) gen.WorkOrder {
		res := e.post(insp, "/work-orders", url.Values{
			"type": {"WO"}, "ref": {ref}, "site_id": {idStr(sites[0].ID)}, "system_id": {idStr(systems[0].ID)},
			"asset_group_id": {idStr(groups[0].ID)}, "contractor_id": {idStr(con)}, "asset": {"PMP-1"}, "priority": {priority},
		})
		if res.StatusCode != http.StatusSeeOther {
			t.Fatalf("create %s: %d", ref, res.StatusCode)
		}
		return must(e.q.GetWorkOrderByRef(ctx, ref))
	}
	woA := mkWO("WO-A1", conA.ID, "emergency")
	woB := mkWO("WO-B1", conB.ID, "normal")

	// Sampling: emergency work orders get both inspections booked in the pool.
	as := must(e.q.ListAssignmentsByWorkOrder(ctx, woA.ID))
	if len(as) != 2 || as[0].AssigneeID != nil {
		t.Fatalf("emergency WO should have 2 pool assignments, got %d", len(as))
	}
	if res := e.post(insp, "/assignments/"+idStr(as[0].ID)+"/claim", nil); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("claim: %d", res.StatusCode)
	}
	if _, body := e.get(insp, "/my/tasks"); !strings.Contains(body, "WO-A1") {
		t.Fatal("claimed assignment missing from my tasks")
	}

	// Raise a finding on WO-A1 during an after-work inspection.
	res := e.post(insp, "/work-orders/"+idStr(woA.ID)+"/inspections", url.Values{"stage": {"after"}})
	ins := must(e.q.ListInspectionsByWorkOrder(ctx, woA.ID))
	if res.StatusCode != http.StatusSeeOther || len(ins) != 1 {
		t.Fatalf("start inspection: %d", res.StatusCode)
	}
	items := must(e.q.ListInspectionItems(ctx, ins[0].ID))
	e.post(insp, "/inspections/"+idStr(ins[0].ID)+"/findings", url.Values{
		"criterion_id": {idStr(items[0].CriterionID)}, "description": {"الأصل لا يعمل"}, "corrective_action": {"إصلاح وإعادة اختبار"},
	})
	fs := must(e.q.ListFindingsByWorkOrder(ctx, woA.ID))
	if len(fs) != 1 {
		t.Fatal("finding not created")
	}
	finding := fs[0]

	con := e.login("con@test.local")
	conUser := must(e.q.GetUserByEmail(ctx, "con@test.local"))
	if n := must(e.q.CountUnreadNotifications(ctx, conUser.ID)); n != 1 {
		t.Fatalf("contractor should be notified of the new finding, unread=%d", n)
	}

	// Scoping: own work order yes, the other contractor's no.
	if res, _ := e.get(con, "/work-orders/"+idStr(woA.ID)); res.StatusCode != http.StatusOK {
		t.Fatalf("own work order: %d", res.StatusCode)
	}
	for _, p := range []string{"/work-orders/" + idStr(woB.ID), "/work-orders/" + idStr(woB.ID) + "/print"} {
		if res, _ := e.get(con, p); res.StatusCode != http.StatusNotFound {
			t.Fatalf("other contractor's %s: %d", p, res.StatusCode)
		}
	}
	if _, body := e.get(con, "/work-orders"); !strings.Contains(body, "WO-A1") || strings.Contains(body, "WO-B1") {
		t.Fatal("list must show only the contractor's own work orders")
	}
	if _, body := e.get(con, "/work-orders?contractor="+idStr(conB.ID)); strings.Contains(body, "WO-B1") {
		t.Fatal("a contractor filter in the URL must not widen the scope")
	}
	for _, p := range []string{"/data", "/reports/print", "/reports/export.xlsx"} {
		if res, _ := e.get(con, p); res.StatusCode != http.StatusForbidden {
			t.Fatalf("contractor %s: %d", p, res.StatusCode)
		}
	}
	if res, _ := e.get(con, "/"); res.Header.Get("Location") != "/my/tasks" {
		t.Fatal("contractors start at their tasks")
	}
	if res := e.post(con, "/work-orders", url.Values{"ref": {"X"}}); res.StatusCode != http.StatusForbidden {
		t.Fatal("contractors can't create work orders")
	}

	// Respond: a photo is required.
	if res := e.multipart(con, "/findings/"+idStr(finding.ID)+"/respond", map[string]string{"response_note": "تم", "root_cause_category": "equipment"}, false); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("respond without photo: %d", res.StatusCode)
	}
	if res := e.multipart(con, "/findings/"+idStr(finding.ID)+"/respond", map[string]string{"response_note": "تم استبدال القطعة", "root_cause_category": "equipment"}, true); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("respond: %d", res.StatusCode)
	}
	f := must(e.q.GetFinding(ctx, finding.ID))
	if f.Status != gen.FindingStatusResponded || f.RootCauseCategory == nil || *f.RootCauseCategory != gen.RootCauseEquipment {
		t.Fatalf("after response: %s", f.Status)
	}
	if photos := must(e.q.ListAttachmentsByFinding(ctx, &finding.ID)); len(photos) != 1 || photos[0].Purpose != "response" {
		t.Fatal("response photo not stored")
	}
	inspUser := must(e.q.GetUserByEmail(ctx, "inspector@test.local"))
	if _, body := e.get(insp, "/my/tasks"); !strings.Contains(body, "تم استبدال القطعة") {
		t.Fatal("response must appear in the inspector's verification queue")
	}

	// The contractor can't verify their own response; the inspector rejects, then verifies.
	if res := e.post(con, "/findings/"+idStr(finding.ID)+"/close", nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("contractor close: %d", res.StatusCode)
	}
	if res := e.post(insp, "/findings/"+idStr(finding.ID)+"/reject", url.Values{"rejection_note": {"الصورة لا توضح الإصلاح"}}); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("reject: %d", res.StatusCode)
	}
	if f := must(e.q.GetFinding(ctx, finding.ID)); f.Status != gen.FindingStatusOpen || f.RejectionNote == "" {
		t.Fatal("reject must reopen with the reason")
	}
	e.multipart(con, "/findings/"+idStr(finding.ID)+"/respond", map[string]string{"response_note": "صورة أوضح", "root_cause_category": "equipment"}, true)
	if res := e.post(insp, "/findings/"+idStr(finding.ID)+"/close", nil); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("verify: %d", res.StatusCode)
	}
	if f := must(e.q.GetFinding(ctx, finding.ID)); f.Status != gen.FindingStatusClosed {
		t.Fatal("verify must close the finding")
	}
	notes := must(e.q.ListNotifications(ctx, gen.ListNotificationsParams{UserID: conUser.ID, Limit: 10}))
	kinds := map[string]bool{}
	for _, n := range notes {
		kinds[n.Kind] = true
	}
	if !kinds[notifyFindingRaised] || !kinds[notifyFindingRejected] || !kinds[notifyFindingVerified] {
		t.Fatalf("contractor notifications: %v", kinds)
	}
	if n := must(e.q.CountUnreadNotifications(ctx, inspUser.ID)); n < 2 {
		t.Fatalf("inspector should be notified of both responses, unread=%d", n)
	}

	// Opening a notification marks it read and follows its link.
	if res := e.post(con, "/notifications/"+idStr(notes[0].ID)+"/open", nil); res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/") {
		t.Fatalf("open notification: %d", res.StatusCode)
	}
	if res, body := e.get(con, "/notifications/bell"); res.StatusCode != http.StatusOK || !strings.Contains(body, `id="bell"`) {
		t.Fatal("bell partial")
	}
}

// multipart posts a form, optionally with one PNG in "photos".
func (e *testEnv) multipart(c *http.Client, path string, fields map[string]string, withPhoto bool) *http.Response {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if withPhoto {
		fw, _ := mw.CreateFormFile("photos", "fix.png")
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		img.Set(1, 1, color.RGBA{181, 150, 102, 255})
		_ = png.Encode(fw, img)
	}
	_ = mw.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.srv.URL+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return res
}
