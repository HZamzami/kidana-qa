package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/storage"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// inspectionStart creates a draft during or after inspection for a work order
// (or resumes the existing draft of that stage).
func (s *Server) inspectionStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	woID, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	stage := gen.InspectionStage(r.PostFormValue("stage"))
	if !stage.Valid() {
		s.badRequest(w, r, appctx.T(ctx, "مرحلة غير صالحة"))
		return
	}
	wo, err := s.q.GetWorkOrder(ctx, woID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if wo.Status == gen.WoStatusCancelled {
		s.badRequest(w, r, appctx.T(ctx, "لا يمكن فحص أمر عمل ملغى"))
		return
	}
	existing, err := s.q.ListInspectionsByWorkOrder(ctx, woID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, in := range existing {
		if in.Stage == stage && in.Status == gen.InspectionStatusDraft {
			http.Redirect(w, r, "/inspections/"+idStr(in.ID), http.StatusSeeOther)
			return
		}
	}

	var inspID int64
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		tpl, err := q.GetActiveTemplate(ctx, stage)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errNoTemplate
			}
			return err
		}
		attempt, err := q.NextInspectionAttempt(ctx, gen.NextInspectionAttemptParams{WorkOrderID: woID, Stage: stage})
		if err != nil {
			return err
		}
		in, err := q.CreateInspection(ctx, gen.CreateInspectionParams{
			WorkOrderID: woID, Stage: stage, Attempt: attempt, TemplateID: tpl.ID,
			InspectorID: appctx.User(ctx).ID, InspectedOn: today(),
		})
		if err != nil {
			return err
		}
		if err := q.CreateInspectionItems(ctx, in.ID); err != nil {
			return err
		}
		inspID = in.ID
		s.audit(ctx, q, "inspection", in.ID, "started", map[string]any{"work_order": woID, "stage": stage, "attempt": attempt})
		return nil
	})
	if errors.Is(err, errNoTemplate) {
		s.badRequest(w, r, appctx.T(ctx, "لا توجد قائمة فحص مفعّلة لهذه المرحلة. فعّل قائمة من الإعدادات."))
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/inspections/"+idStr(inspID), http.StatusSeeOther)
}

var errNoTemplate = errors.New("no active checklist template")

func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// loadInspection gathers everything the inspection page needs.
func (s *Server) loadInspection(ctx context.Context, id int64) (views.InspectionData, error) {
	var d views.InspectionData
	in, err := s.q.GetInspection(ctx, id)
	if err != nil {
		return d, err
	}
	wo, err := s.q.GetWorkOrder(ctx, in.WorkOrderID)
	if err != nil {
		return d, err
	}
	items, err := s.q.ListInspectionItems(ctx, id)
	if err != nil {
		return d, err
	}
	findings, err := s.q.ListFindingsByInspection(ctx, &id)
	if err != nil {
		return d, err
	}
	photos, err := s.q.ListAttachmentsByInspection(ctx, &id)
	if err != nil {
		return d, err
	}

	type crit struct {
		ID       int64  `json:"id"`
		Weight   int32  `json:"weight"`
		Critical bool   `json:"critical"`
		Rating   string `json:"rating"`
	}
	crits := make([]crit, 0, len(items))
	for _, it := range items {
		c := crit{ID: it.CriterionID, Weight: it.Weight, Critical: it.IsCritical}
		if it.Rating != nil {
			c.Rating = string(*it.Rating)
		}
		crits = append(crits, c)
	}
	cj, _ := json.Marshal(crits)

	d = views.InspectionData{
		I: in, WO: wo, Items: items, Findings: findings, Photos: photos,
		Result:       evaluateItems(items),
		Editable:     in.Status == gen.InspectionStatusDraft && appctx.CanEdit(ctx),
		CriteriaJSON: string(cj),
		Today:        time.Now().Format(time.DateOnly),
	}
	return d, nil
}

func evaluateItems(items []gen.ListInspectionItemsRow) domain.Result {
	di := make([]domain.Item, len(items))
	for i, it := range items {
		di[i] = domain.Item{Weight: int(it.Weight), Critical: it.IsCritical}
		if it.Rating != nil {
			di[i].Rating = domain.Rating(*it.Rating)
		}
	}
	return domain.Evaluate(di)
}

func (s *Server) inspectionShow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	d, err := s.loadInspection(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.InspectionPage(d))
}

// inspectionReview re-renders the review step (htmx).
func (s *Server) inspectionReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	d, err := s.loadInspection(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ReviewBody(d))
}

// draftInspection loads an inspection and checks it can still be edited.
func (s *Server) draftInspection(w http.ResponseWriter, r *http.Request) (gen.GetInspectionRow, bool) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return gen.GetInspectionRow{}, false
	}
	in, err := s.q.GetInspection(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return in, false
	}
	if in.Status != gen.InspectionStatusDraft {
		w.Header().Set("HX-Refresh", "true")
		s.badRequest(w, r, appctx.T(r.Context(), "تم إرسال هذا الفحص ولا يمكن تعديله."))
		return in, false
	}
	return in, true
}

func (s *Server) inspectionItemSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, ok := s.draftInspection(w, r)
	if !ok {
		return
	}
	cid, ok := pathID(r, "cid")
	if !ok {
		s.notFound(w, r)
		return
	}
	f := newForm(r)
	var rating *gen.Rating
	if v := f.Str("rating"); v != "" {
		rt := gen.Rating(v)
		if !rt.Valid() {
			s.badRequest(w, r, appctx.T(ctx, "تقييم غير صالح"))
			return
		}
		rating = &rt
	}
	note := f.Str("note")
	if len([]rune(note)) > 1000 {
		note = string([]rune(note)[:1000])
	}
	n, err := s.q.SetItemRating(ctx, gen.SetItemRatingParams{InspectionID: in.ID, CriterionID: cid, Rating: rating})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if n == 0 {
		s.notFound(w, r)
		return
	}
	if _, err := s.q.SetItemNote(ctx, gen.SetItemNoteParams{InspectionID: in.ID, CriterionID: cid, Note: note}); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) inspectionHeaderSave(w http.ResponseWriter, r *http.Request) {
	in, ok := s.draftInspection(w, r)
	if !ok {
		return
	}
	f := newForm(r)
	on := f.OptDate("inspected_on")
	if on == nil {
		on = &in.InspectedOn
	}
	if on.After(today()) {
		toast(w, appctx.T(r.Context(), "تاريخ الفحص لا يمكن أن يكون في المستقبل"), "warn")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	notes := f.Str("notes")
	if len([]rune(notes)) > 2000 {
		notes = string([]rune(notes)[:2000])
	}
	if err := s.q.UpdateInspectionHeader(r.Context(), gen.UpdateInspectionHeaderParams{ID: in.ID, Notes: notes, InspectedOn: *on}); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// inspectionFindingCreate records a finding on a draft inspection and returns
// the re-rendered finding box for that criterion (or the general box).
func (s *Server) inspectionFindingCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, ok := s.draftInspection(w, r)
	if !ok {
		return
	}
	f := newForm(r)
	cid := f.OptID("criterion_id")
	desc := f.Required("description", appctx.T(ctx, "وصف الملاحظة"))
	action := f.Required("corrective_action", appctx.T(ctx, "الإجراء التصحيحي"))
	owner := f.Str("owner")
	due := f.OptDate("due_date")
	f.MaxLen("description", 1500)
	f.MaxLen("corrective_action", 1000)
	f.MaxLen("owner", 80)
	sev := gen.FindingSeverityNormal
	if f.Bool("critical") {
		sev = gen.FindingSeverityCritical
	}

	critical := false
	if cid != nil {
		items, err := s.q.ListInspectionItems(ctx, in.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		found := false
		for _, it := range items {
			if it.CriterionID == *cid {
				found, critical = true, it.IsCritical
			}
		}
		if !found {
			s.badRequest(w, r, appctx.T(ctx, "بند غير موجود في هذا الفحص"))
			return
		}
	}

	if f.Valid() {
		err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
			q := s.q.WithTx(tx)
			fd, err := q.CreateFinding(ctx, gen.CreateFindingParams{
				WorkOrderID: in.WorkOrderID, InspectionID: &in.ID, CriterionID: cid, Severity: sev,
				Description: desc, CorrectiveAction: action, Owner: owner, DueDate: due, CreatedBy: currentUserID(ctx),
			})
			if err != nil {
				return err
			}
			s.audit(ctx, q, "finding", fd.ID, "created", map[string]any{"inspection": in.ID})
			return s.refreshWorkOrderStatus(ctx, q, in.WorkOrderID)
		})
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		toast(w, appctx.T(ctx, "تم تسجيل الملاحظة"), "ok")
	}

	all, err := s.q.ListFindingsByInspection(ctx, &in.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var mine []gen.ListFindingsByInspectionRow
	for _, fd := range all {
		if (cid == nil && fd.CriterionID == nil) || (cid != nil && fd.CriterionID != nil && *fd.CriterionID == *cid) {
			mine = append(mine, fd)
		}
	}
	status := http.StatusOK
	var errs map[string]string
	if !f.Valid() {
		status, errs = http.StatusUnprocessableEntity, f.Errors
	}
	s.render(w, r, status, views.FindingBox(in.ID, cid, critical, mine, errs))
}

func (s *Server) inspectionPhotoUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, ok := s.draftInspection(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		s.badRequest(w, r, appctx.T(ctx, "الملف كبير جداً أو غير صالح"))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	f := newForm(r)
	cid := f.OptID("criterion_id")

	file, hdr, err := r.FormFile("photo")
	if err != nil {
		s.badRequest(w, r, appctx.T(ctx, "لم يتم اختيار صورة"))
		return
	}
	defer func() { _ = file.Close() }()

	key, size, ctype, err := s.store.Save(ctx, file)
	switch {
	case errors.Is(err, storage.ErrUnsupportedType):
		toast(w, appctx.T(ctx, "الملف ليس صورة مدعومة (JPG أو PNG أو WEBP)"), "warn")
	case err != nil:
		toast(w, appctx.T(ctx, "تعذّر رفع الصورة: ")+err.Error(), "warn")
	default:
		name := filepath.Base(hdr.Filename)
		if len(name) > 200 {
			name = name[:200]
		}
		a, err := s.q.CreateAttachment(ctx, gen.CreateAttachmentParams{
			InspectionID: &in.ID, CriterionID: cid, StorageKey: key, Filename: name,
			ContentType: ctype, SizeBytes: size, UploadedBy: currentUserID(ctx),
		})
		if err != nil {
			_ = s.store.Delete(ctx, key)
			s.serverError(w, r, err)
			return
		}
		s.audit(ctx, s.q, "attachment", a.ID, "uploaded", map[string]any{"inspection": in.ID})
	}
	s.renderPhotoStrip(w, r, in.ID, cid)
}

func (s *Server) renderPhotoStrip(w http.ResponseWriter, r *http.Request, inspectionID int64, cid *int64) {
	photos, err := s.q.ListAttachmentsByInspection(r.Context(), &inspectionID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var mine []gen.Attachment
	for _, p := range photos {
		if (cid == nil && p.CriterionID == nil) || (cid != nil && p.CriterionID != nil && *p.CriterionID == *cid) {
			mine = append(mine, p)
		}
	}
	s.render(w, r, http.StatusOK, views.PhotoStrip(inspectionID, cid, mine, true))
}

func (s *Server) inspectionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, ok := s.draftInspection(w, r)
	if !ok {
		return
	}
	d, err := s.loadInspection(ctx, in.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var unrated []string
	for _, it := range d.Items {
		if it.Rating == nil {
			unrated = append(unrated, it.Code)
		}
	}
	if errs := domain.ValidateSubmit(domain.SubmitCheck{
		Result: d.Result, FindingsLinked: len(d.Findings), UnratedLabels: unrated, Lang: appctx.Lang(ctx),
	}); len(errs) > 0 {
		d.Errors = errs
		s.render(w, r, http.StatusUnprocessableEntity, views.InspectionPage(d))
		return
	}

	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		score, passed := d.Result.Score, d.Result.Passed
		if err := q.SubmitInspection(ctx, gen.SubmitInspectionParams{
			ID: in.ID, Score: &score, Passed: &passed, CriticalFailed: d.Result.CriticalFailed,
		}); err != nil {
			return err
		}
		s.audit(ctx, q, "inspection", in.ID, "submitted", map[string]any{"score": score, "passed": passed})
		return s.refreshWorkOrderStatus(ctx, q, in.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	msg := appctx.T(ctx, "تم إرسال الفحص — النتيجة ") + domain.Fmt1(d.Result.Score) + "% "
	if d.Result.Passed {
		s.flash(ctx, msg+appctx.T(ctx, "(مطابق)"), "ok")
	} else {
		s.flash(ctx, msg+appctx.T(ctx, "(غير مطابق). تابع الملاحظات المفتوحة، ثم سجّل فحصاً جديداً بعد المعالجة."), "warn")
	}
	http.Redirect(w, r, "/work-orders/"+idStr(in.WorkOrderID), http.StatusSeeOther)
}

func (s *Server) inspectionDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, ok := s.draftInspection(w, r)
	if !ok {
		return
	}
	photos, err := s.q.ListAttachmentsByInspection(ctx, &in.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		// Findings recorded on a discarded draft go with it.
		fs, err := q.ListFindingsByInspection(ctx, &in.ID)
		if err != nil {
			return err
		}
		for _, f := range fs {
			if err := q.DeleteFinding(ctx, f.ID); err != nil {
				return err
			}
		}
		if _, err := q.DeleteDraftInspection(ctx, in.ID); err != nil {
			return err
		}
		s.audit(ctx, q, "inspection", in.ID, "draft_deleted", map[string]any{"work_order": in.WorkOrderID})
		return s.refreshWorkOrderStatus(ctx, q, in.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, p := range photos {
		_ = s.store.Delete(ctx, p.StorageKey)
	}
	s.flash(ctx, appctx.T(ctx, "تم حذف المسودة"), "")
	http.Redirect(w, r, "/work-orders/"+idStr(in.WorkOrderID), http.StatusSeeOther)
}

func (s *Server) inspectionReopen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	in, err := s.q.GetInspection(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.ReopenInspection(ctx, id); err != nil {
			return err
		}
		s.audit(ctx, q, "inspection", id, "reopened", nil)
		return s.refreshWorkOrderStatus(ctx, q, in.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "أُعيد فتح الفحص كمسودة"), "")
	http.Redirect(w, r, "/inspections/"+idStr(id), http.StatusSeeOther)
}

// ---------------------------------------------------------------- attachments

func (s *Server) attachmentGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	a, err := s.q.GetAttachment(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	f, err := s.store.Open(ctx, a.StorageKey)
	if err != nil {
		s.notFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, "", a.CreatedAt, f)
}

func (s *Server) attachmentDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	a, err := s.q.GetAttachment(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if a.InspectionID == nil {
		s.badRequest(w, r, appctx.T(ctx, "لا يمكن حذف هذا المرفق من هنا"))
		return
	}
	in, err := s.q.GetInspection(ctx, *a.InspectionID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	if in.Status != gen.InspectionStatusDraft {
		s.badRequest(w, r, appctx.T(ctx, "تم إرسال هذا الفحص ولا يمكن تعديل صوره."))
		return
	}
	if err := s.q.DeleteAttachment(ctx, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.Delete(ctx, a.StorageKey)
	s.audit(ctx, s.q, "attachment", id, "deleted", map[string]any{"inspection": in.ID})
	s.renderPhotoStrip(w, r, in.ID, a.CriterionID)
}
