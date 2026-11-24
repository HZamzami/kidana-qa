package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// maxSignatureBytes bounds a drawn signature (a small PNG).
const maxSignatureBytes = 300 << 10

// inspectionLocation records where the inspector was when the inspection
// was opened (proof of presence). It is set once per inspection.
func (s *Server) inspectionLocation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, ok := s.draftInspection(w, r)
	if !ok {
		return
	}
	lat, lng, acc, ok := parseGeo(r)
	if !ok {
		s.badRequest(w, r, appctx.T(ctx, "قيمة غير صالحة"))
		return
	}
	if err := s.q.SetInspectionLocation(ctx, gen.SetInspectionLocationParams{ID: in.ID, Lat: &lat, Lng: &lng, Accuracy: &acc}); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseGeo reads lat, lng and accuracy (metres) from the form.
func parseGeo(r *http.Request) (lat, lng float64, acc float32, ok bool) {
	var err1, err2 error
	lat, err1 = strconv.ParseFloat(r.FormValue("lat"), 64)
	lng, err2 = strconv.ParseFloat(r.FormValue("lng"), 64)
	a, _ := strconv.ParseFloat(r.FormValue("accuracy"), 32)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, 0, false
	}
	return lat, lng, float32(a), true
}

// geoFields are the optional location and device time sent with a photo.
func geoFields(r *http.Request) (lat, lng *float64, acc *float32, at *time.Time) {
	if la, lo, a, ok := parseGeo(r); ok {
		lat, lng, acc = &la, &lo, &a
	}
	if ms, err := strconv.ParseInt(r.FormValue("captured_at"), 10, 64); err == nil && ms > 0 {
		t := time.UnixMilli(ms)
		if d := time.Since(t); d > -time.Hour && d < 7*24*time.Hour { // ignore clocks that are far off
			at = &t
		}
	}
	return lat, lng, acc, at
}

// saveSignatures stores the signatures drawn on the review step. Both are
// optional: the contractor's representative may not be on site.
func (s *Server) saveSignatures(ctx context.Context, r *http.Request, in gen.GetInspectionRow) error {
	for _, sig := range []struct{ field, role, name string }{
		{"signature_inspector", "inspector", in.InspectorName},
		{"signature_rep", "contractor_rep", strings.TrimSpace(r.PostFormValue("rep_name"))},
	} {
		png, ok := decodeSignature(r.PostFormValue(sig.field))
		if !ok {
			continue
		}
		key, size, ctype, err := s.store.Save(ctx, bytes.NewReader(png))
		if err != nil {
			return err
		}
		name := sig.name
		if len([]rune(name)) > 80 {
			name = string([]rune(name)[:80])
		}
		if _, err := s.q.CreateAttachment(ctx, gen.CreateAttachmentParams{
			InspectionID: &in.ID, StorageKey: key, Filename: sig.role + ".png", ContentType: ctype, SizeBytes: size,
			UploadedBy: currentUserID(ctx), Kind: "signature", SignerRole: sig.role, SignerName: name,
		}); err != nil {
			_ = s.store.Delete(ctx, key)
			return err
		}
	}
	return nil
}

// decodeSignature accepts a canvas PNG data URL.
func decodeSignature(v string) ([]byte, bool) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(v, prefix) || len(v) > maxSignatureBytes*4/3+len(prefix) {
		return nil, false
	}
	b, err := base64.StdEncoding.DecodeString(v[len(prefix):])
	if err != nil || len(b) < 100 || !bytes.HasPrefix(b, []byte("\x89PNG")) {
		return nil, false
	}
	return b, true
}

// ---------------------------------------------------------------- photo annotation

// annotatePage shows a photo on a canvas to draw on (arrows, circles).
func (s *Server) annotatePage(w http.ResponseWriter, r *http.Request) {
	a, in, ok := s.editablePhoto(w, r)
	if !ok {
		return
	}
	s.render(w, r, http.StatusOK, views.AnnotatePage(a, in.ID))
}

// annotateSave stores the drawn copy next to the original, which is kept.
func (s *Server) annotateSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		s.badRequest(w, r, appctx.T(ctx, "الملف كبير جداً أو غير صالح"))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	orig, in, ok := s.editablePhoto(w, r)
	if !ok {
		return
	}
	file, _, err := r.FormFile("image")
	if err != nil {
		s.badRequest(w, r, appctx.T(ctx, "لم يتم اختيار صورة"))
		return
	}
	defer func() { _ = file.Close() }()
	key, size, ctype, err := s.store.Save(ctx, file)
	if err != nil {
		s.badRequest(w, r, appctx.T(ctx, "الملف ليس صورة مدعومة (JPG أو PNG أو WEBP)"))
		return
	}
	a, err := s.q.CreateAttachment(ctx, gen.CreateAttachmentParams{
		InspectionID: orig.InspectionID, CriterionID: orig.CriterionID, StorageKey: key, Filename: "annotated-" + orig.Filename,
		ContentType: ctype, SizeBytes: size, UploadedBy: currentUserID(ctx), Kind: "photo",
		Lat: orig.Lat, Lng: orig.Lng, Accuracy: orig.Accuracy, CapturedAt: orig.CapturedAt, AnnotatedOf: &orig.ID,
	})
	if err != nil {
		_ = s.store.Delete(ctx, key)
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "attachment", a.ID, "annotated", map[string]any{"original": orig.ID})
	http.Redirect(w, r, "/inspections/"+idStr(in.ID), http.StatusSeeOther)
}

// editablePhoto loads a photo on a draft inspection.
func (s *Server) editablePhoto(w http.ResponseWriter, r *http.Request) (gen.Attachment, gen.GetInspectionRow, bool) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return gen.Attachment{}, gen.GetInspectionRow{}, false
	}
	a, err := s.q.GetAttachment(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return a, gen.GetInspectionRow{}, false
	}
	if a.InspectionID == nil || a.Kind != "photo" {
		s.notFound(w, r)
		return a, gen.GetInspectionRow{}, false
	}
	in, err := s.q.GetInspection(ctx, *a.InspectionID)
	if err != nil {
		s.dbError(w, r, err)
		return a, in, false
	}
	if in.Status != gen.InspectionStatusDraft {
		s.badRequest(w, r, appctx.T(ctx, "تم إرسال هذا الفحص ولا يمكن تعديل صوره."))
		return a, in, false
	}
	return a, in, true
}
