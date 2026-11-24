package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/HZamzami/kidana-quality/internal/db/gen"
)

func testPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := 0; x < 40; x++ {
		img.Set(x, 10, color.RGBA{76, 79, 83, 255})
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// TestFieldWork: location stamp, a photo with location, a drawn copy that
// keeps the original, signatures on submit, and all of it in the PDF record.
func TestFieldWork(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := e.login("inspector@test.local")
	sites, _ := e.q.ListActiveSites(ctx)
	systems, _ := e.q.ListActiveSystems(ctx)
	groups, _ := e.q.ListActiveAssetGroups(ctx)
	e.post(c, "/work-orders", url.Values{"type": {"WO"}, "ref": {"WO-F1"}, "site_id": {idStr(sites[0].ID)}, "system_id": {idStr(systems[0].ID)}, "asset_group_id": {idStr(groups[0].ID)}, "asset": {"CH-01"}})
	wo := must(e.q.GetWorkOrderByRef(ctx, "WO-F1"))
	if wo.AssetID == nil {
		t.Fatal("work order must be linked to the asset register")
	}
	if res, body := e.get(c, "/a/CH-01"); res.StatusCode != http.StatusSeeOther || body != "" && !strings.Contains(res.Header.Get("Location"), "/assets/") {
		t.Fatalf("QR link: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	if res, _ := e.get(c, "/assets/"+idStr(*wo.AssetID)+"/qr.png"); res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" {
		t.Fatal("asset QR image")
	}

	e.post(c, "/work-orders/"+idStr(wo.ID)+"/inspections", url.Values{"stage": {"after"}})
	in := must(e.q.ListInspectionsByWorkOrder(ctx, wo.ID))[0]
	if res := e.post(c, "/inspections/"+idStr(in.ID)+"/location", url.Values{"lat": {"21.4133"}, "lng": {"39.8933"}, "accuracy": {"12"}}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("location: %d", res.StatusCode)
	}
	if res := e.post(c, "/inspections/"+idStr(in.ID)+"/location", url.Values{"lat": {"999"}, "lng": {"0"}}); res.StatusCode != http.StatusBadRequest {
		t.Fatal("bad coordinates must be refused")
	}

	// A general photo with location, then a drawn copy of it.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("lat", "21.41")
	_ = mw.WriteField("lng", "39.89")
	fw, _ := mw.CreateFormFile("photo", "p.png")
	_, _ = fw.Write(testPNG())
	_ = mw.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.srv.URL+"/inspections/"+idStr(in.ID)+"/photos", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := c.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("photo upload: %v %d", err, res.StatusCode)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	photos := must(e.q.ListAttachmentsByInspection(ctx, &in.ID))
	if len(photos) != 1 || photos[0].Lat == nil {
		t.Fatal("photo location not stored")
	}
	body.Reset()
	mw = multipart.NewWriter(&body)
	fw, _ = mw.CreateFormFile("image", "a.png")
	_, _ = fw.Write(testPNG())
	_ = mw.Close()
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, e.srv.URL+"/attachments/"+idStr(photos[0].ID)+"/annotate", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if res, err := c.Do(req); err != nil || res.StatusCode != http.StatusSeeOther {
		t.Fatalf("annotate: %v %d", err, res.StatusCode)
	}

	// Rate all compliant and submit with the inspector's signature.
	for _, it := range must(e.q.ListInspectionItems(ctx, in.ID)) {
		e.post(c, "/inspections/"+idStr(in.ID)+"/items/"+idStr(it.CriterionID), url.Values{"rating": {"C"}})
	}
	sig := "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNG())
	if res := e.post(c, "/inspections/"+idStr(in.ID)+"/submit", url.Values{"signature_inspector": {sig}, "signature_rep": {"data:image/png;base64,bm9wZQ=="}, "rep_name": {"سالم"}}); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit: %d", res.StatusCode)
	}
	var sigs, drawn int
	for _, a := range must(e.q.ListAttachmentsByInspection(ctx, &in.ID)) {
		if a.Kind == "signature" {
			sigs++
		}
		if a.AnnotatedOf != nil {
			drawn++
		}
	}
	if sigs != 1 || drawn != 1 {
		t.Fatalf("signatures=%d (want 1; the broken one is ignored), drawn=%d", sigs, drawn)
	}
	if got := must(e.q.GetInspection(ctx, in.ID)); got.Lat == nil || got.Status != gen.InspectionStatusSubmitted {
		t.Fatal("inspection location or status wrong")
	}
	_, page := e.get(c, "/work-orders/"+idStr(wo.ID)+"/print?photos=1")
	if strings.Count(page, "/attachments/") < 2 || !strings.Contains(page, "21.41330") {
		t.Fatal("print must show the signature, photos and location")
	}
}
