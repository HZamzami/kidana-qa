package web

import (
	"bytes"
	"context"
	"image/png"
	"net/http"
	"net/url"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// assetLabelCap bounds a QR label sheet print.
const assetLabelCap = 600

func assetFilter(r *http.Request, lim int32) gen.ListAssetsParams {
	return gen.ListAssetsParams{
		Q: queryStr(r, "q"), SiteID: queryID(r, "site"), SystemID: queryID(r, "system"), AssetGroupID: queryID(r, "group"),
		ActiveOnly: r.URL.Query().Get("all") != "1", Lim: lim,
	}
}

func (s *Server) assetsList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.q.ListAssets(ctx, assetFilter(r, 500))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	lk, err := s.lookupOptions(ctx, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.AssetsPage(views.AssetsData{
		Rows: rows, Query: r.URL.Query(), Sites: lk.Sites, Systems: lk.Systems, AssetGroups: lk.AssetGroups,
	}))
}

func (s *Server) assetShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	a, err := s.q.GetAsset(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	scope := appctx.ContractorScope(ctx)
	wos, err := s.q.ListWorkOrdersByAsset(ctx, gen.ListWorkOrdersByAssetParams{AssetID: &id, ContractorID: scope})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if scope != nil && len(wos) == 0 {
		s.notFound(w, r) // contractors see only assets they work on
		return
	}
	fs, err := s.q.ListFindingsByAsset(ctx, gen.ListFindingsByAssetParams{AssetID: &id, ContractorID: scope})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := views.AssetData{A: a, WorkOrders: wos, Findings: fs, Link: s.assetLink(r, a.Code)}
	if appctx.CanEdit(ctx) {
		lk, err := s.lookupOptions(ctx, true)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		d.Sites, d.Systems, d.AssetGroups = lk.Sites, lk.Systems, lk.AssetGroups
	}
	s.render(w, r, http.StatusOK, views.AssetPage(d))
}

func (s *Server) assetUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	f := newForm(r)
	if err := s.q.UpdateAsset(ctx, gen.UpdateAssetParams{
		ID: id, Name: f.Str("name"), NameEn: f.Str("name_en"), SiteID: f.OptID("site_id"), SystemID: f.OptID("system_id"),
		AssetGroupID: f.OptID("asset_group_id"), Location: f.Str("location"), Active: f.Bool("active"),
	}); err != nil {
		s.dbError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "asset", id, "updated", nil)
	s.flash(ctx, appctx.T(ctx, "تم الحفظ"), "ok")
	http.Redirect(w, r, "/assets/"+idStr(id), http.StatusSeeOther)
}

// assetByCode is where a scanned QR label lands.
func (s *Server) assetByCode(w http.ResponseWriter, r *http.Request) {
	a, err := s.q.GetAssetByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	http.Redirect(w, r, "/assets/"+idStr(a.ID), http.StatusSeeOther)
}

// assetQR serves the QR code (PNG) that opens the asset page.
func (s *Server) assetQR(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	a, err := s.q.GetAsset(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	img, err := qrPNG(s.assetLink(r, a.Code), 360)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(img)
}

// assetLabels prints QR labels for the filtered assets (A4, 3 × 8).
func (s *Server) assetLabels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.q.ListAssets(ctx, assetFilter(r, assetLabelCap))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if id := queryID(r, "id"); id != nil {
		kept := rows[:0]
		for _, a := range rows {
			if a.ID == *id {
				kept = append(kept, a)
			}
		}
		rows = kept
	}
	q := r.URL.Query()
	q.Del("id")
	s.render(w, r, http.StatusOK, views.AssetLabels(views.AssetLabelsData{Rows: rows, Back: "/assets?" + q.Encode()}))
}

// assetLink is the URL a QR label encodes.
func (s *Server) assetLink(r *http.Request, code string) string {
	return baseURL(r) + "/a/" + url.PathEscape(code)
}

func qrPNG(content string, size int) ([]byte, error) {
	code, err := qr.Encode(content, qr.M, qr.Auto)
	if err != nil {
		return nil, err
	}
	code, err = barcode.Scale(code, size, size)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, code); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// linkAsset ties a work order to the asset register by its asset code,
// creating the asset the first time a code is seen.
func (s *Server) linkAsset(ctx context.Context, q *gen.Queries, wo gen.WorkOrder) error {
	if wo.Asset == "" {
		return q.SetWorkOrderAsset(ctx, gen.SetWorkOrderAssetParams{ID: wo.ID})
	}
	id, err := q.EnsureAsset(ctx, gen.EnsureAssetParams{
		Code: wo.Asset, SiteID: &wo.SiteID, SystemID: &wo.SystemID, AssetGroupID: &wo.AssetGroupID, Location: wo.Location,
	})
	if err != nil {
		return err
	}
	return q.SetWorkOrderAsset(ctx, gen.SetWorkOrderAssetParams{ID: wo.ID, AssetID: &id})
}
