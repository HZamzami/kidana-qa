package web

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/dates"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/excel"
	"github.com/HZamzami/kidana-quality/internal/i18n"
	"github.com/HZamzami/kidana-quality/internal/storage"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// findingsFilter reads the findings list filters. Contractor users are
// always limited to their own contractor.
func findingsFilter(r *http.Request) gen.ListFindingsParams {
	q := r.URL.Query()
	p := gen.ListFindingsParams{
		SiteID: queryID(r, "site"), ContractorID: queryID(r, "contractor"), Q: queryStr(r, "q"),
		SystemID: queryID(r, "system"), AssetGroupID: queryID(r, "group"), OwnerUserID: queryID(r, "owner"),
		RepeatOnly: q.Get("repeat") == "1",
	}
	if scope := appctx.ContractorScope(r.Context()); scope != nil {
		p.ContractorID = scope
	}
	switch q.Get("scope") {
	case "", "open":
		p.OpenOnly = true
	case "overdue":
		p.OverdueOnly = true
	case "closed", "responded", "in_progress":
		st := gen.FindingStatus(q.Get("scope"))
		p.Status = &st
	}
	if sev := gen.FindingSeverity(q.Get("severity")); sev.Valid() {
		p.Severity = &sev
	}
	if st := gen.InspectionStage(q.Get("stage")); st.Valid() {
		p.Stage = &st
	}
	return p
}

func (s *Server) findingsList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.q.ListFindings(ctx, findingsFilter(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	lk, err := s.lookupOptions(ctx, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.FindingsPage(views.FindingsListData{
		Rows: rows, Query: r.URL.Query(), Sites: lk.Sites, Systems: lk.Systems, AssetGroups: lk.AssetGroups,
		Contractors: lk.Contractors, Today: time.Now().Format(time.DateOnly),
	}))
}

func findingValues(f gen.GetFindingRow) views.FindingFormValues {
	v := views.FindingFormValues{
		Description: f.Description, CorrectiveAction: f.CorrectiveAction, Owner: f.Owner,
		DueDate: fmtDate(f.DueDate), Severity: string(f.Severity), Status: string(f.Status), RootCause: f.RootCause,
	}
	if f.OwnerUserID != nil {
		v.OwnerUserID = idStr(*f.OwnerUserID)
	}
	if f.RootCauseCategory != nil {
		v.RootCauseCategory = string(*f.RootCauseCategory)
	}
	return v
}

// loadFinding gets a finding the current user may see (contractors only
// their own contractor's), answering 404 otherwise.
func (s *Server) loadFinding(w http.ResponseWriter, r *http.Request) (gen.GetFindingRow, bool) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return gen.GetFindingRow{}, false
	}
	f, err := s.q.GetFinding(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return f, false
	}
	if !canSeeContractor(r.Context(), f.WoContractorID) {
		s.notFound(w, r)
		return f, false
	}
	return f, true
}

// canSeeContractor is false when a contractor user looks at another
// contractor's record.
func canSeeContractor(ctx context.Context, contractorID *int64) bool {
	scope := appctx.ContractorScope(ctx)
	return scope == nil || (contractorID != nil && *contractorID == *scope)
}

func (s *Server) findingEditData(ctx context.Context, f gen.GetFindingRow, v views.FindingFormValues, errs map[string]string) (views.FindingEditData, error) {
	d := views.FindingEditData{F: f, V: v, Errors: errs}
	photos, err := s.q.ListAttachmentsByFinding(ctx, &f.ID)
	if err != nil {
		return d, err
	}
	d.Photos = photos
	if f.WoContractorID != nil && appctx.CanEdit(ctx) {
		users, err := s.q.ListContractorUsers(ctx, f.WoContractorID)
		if err != nil {
			return d, err
		}
		for _, u := range users {
			d.Owners = append(d.Owners, views.Option{Value: idStr(u.ID), Label: u.Name})
		}
	}
	return d, nil
}

func (s *Server) renderFinding(w http.ResponseWriter, r *http.Request, status int, f gen.GetFindingRow, v views.FindingFormValues, errs map[string]string) {
	d, err := s.findingEditData(r.Context(), f, v, errs)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, status, views.FindingEdit(d))
}

func (s *Server) findingEdit(w http.ResponseWriter, r *http.Request) {
	f, ok := s.loadFinding(w, r)
	if !ok {
		return
	}
	s.renderFinding(w, r, http.StatusOK, f, findingValues(f), nil)
}

func (s *Server) findingUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cur, ok := s.loadFinding(w, r)
	if !ok {
		return
	}
	if !domain.FindingAllowed(string(cur.Status), domain.FindingEdit, appctx.CanEdit(ctx), false) {
		s.badRequest(w, r, appctx.T(ctx, "الملاحظة مغلقة. أعد فتحها أولاً."))
		return
	}
	f := newForm(r)
	v := views.FindingFormValues{
		Description: f.Required("description", appctx.T(ctx, "وصف الملاحظة")), CorrectiveAction: f.Required("corrective_action", appctx.T(ctx, "الإجراء التصحيحي")),
		Owner: f.Str("owner"), OwnerUserID: f.Str("owner_user_id"), DueDate: f.Str("due_date"), Severity: f.Str("severity"),
		Status: f.Str("status"), RootCauseCategory: f.Str("root_cause_category"), RootCause: f.Str("root_cause"),
	}
	due := f.OptDate("due_date")
	f.MaxLen("description", 1500)
	f.MaxLen("corrective_action", 1000)
	f.MaxLen("owner", 80)
	f.MaxLen("root_cause", 1000)
	sev, st := gen.FindingSeverity(v.Severity), gen.FindingStatus(v.Status)
	if !sev.Valid() {
		f.Errors["severity"] = appctx.T(ctx, "قيمة غير صالحة")
	}
	if cur.Status == gen.FindingStatusResponded {
		st, v.Status = gen.FindingStatusResponded, string(gen.FindingStatusResponded) // verify or reject instead
	} else if st != gen.FindingStatusOpen && st != gen.FindingStatusInProgress {
		f.Errors["status"] = appctx.T(ctx, "قيمة غير صالحة")
	}
	owner := f.OptID("owner_user_id")
	if owner != nil && !s.isContractorUser(ctx, *owner, cur.WoContractorID) {
		f.Errors["owner_user_id"] = appctx.T(ctx, "قيمة غير صالحة")
	}
	var rc *gen.RootCause
	if c := gen.RootCause(v.RootCauseCategory); c.Valid() {
		rc = &c
	}
	if !f.Valid() {
		s.renderFinding(w, r, http.StatusUnprocessableEntity, cur, v, f.Errors)
		return
	}
	if _, err := s.q.UpdateFinding(ctx, gen.UpdateFindingParams{
		ID: cur.ID, Description: v.Description, CorrectiveAction: v.CorrectiveAction, Owner: v.Owner,
		DueDate: due, Severity: sev, Status: st, OwnerUserID: owner, RootCauseCategory: rc, RootCause: v.RootCause,
	}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "finding", cur.ID, "updated", findingDiff(cur, v))
	s.flash(ctx, appctx.T(ctx, "تم حفظ الملاحظة"), "ok")
	http.Redirect(w, r, "/findings/"+idStr(cur.ID)+"/edit", http.StatusSeeOther)
}

// findingDiff records which fields an edit changed, for the change history.
func findingDiff(cur gen.GetFindingRow, v views.FindingFormValues) map[string]any {
	b := findingValues(cur)
	return domain.Diff(map[string]string{
		"description": b.Description, "corrective_action": b.CorrectiveAction, "owner": b.Owner, "owner_user": b.OwnerUserID,
		"due_date": b.DueDate, "severity": b.Severity, "status": b.Status,
		"root_cause_category": b.RootCauseCategory, "root_cause": b.RootCause,
	}, map[string]string{
		"description": v.Description, "corrective_action": v.CorrectiveAction, "owner": v.Owner, "owner_user": v.OwnerUserID,
		"due_date": v.DueDate, "severity": v.Severity, "status": v.Status,
		"root_cause_category": v.RootCauseCategory, "root_cause": v.RootCause,
	})
}

func (s *Server) isContractorUser(ctx context.Context, userID int64, contractorID *int64) bool {
	if contractorID == nil {
		return false
	}
	u, err := s.q.GetUser(ctx, userID)
	return err == nil && u.ContractorID != nil && *u.ContractorID == *contractorID
}

// findingClose closes a finding directly, or verifies a contractor's response.
func (s *Server) findingClose(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 4*s.cfg.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		s.badRequest(w, r, appctx.T(ctx, "الملف كبير جداً أو غير صالح"))
		return
	}
	cur, ok := s.loadFinding(w, r)
	if !ok {
		return
	}
	if !domain.FindingAllowed(string(cur.Status), domain.FindingClose, appctx.CanEdit(ctx), false) {
		s.badRequest(w, r, appctx.T(ctx, "لا يمكن إغلاق هذه الملاحظة الآن"))
		return
	}
	f := newForm(r)
	verifying := cur.Status == gen.FindingStatusResponded
	note := f.Str("closure_note")
	if note == "" && !verifying {
		f.Errors["closure_note"] = appctx.Tf(ctx, "حقل «%s» مطلوب", appctx.T(ctx, "كيف تمت المعالجة"))
	}
	if note == "" {
		note = appctx.T(ctx, "تم التحقق من رد المقاول")
	}
	f.MaxLen("closure_note", 1000)
	if !f.Valid() {
		s.renderFinding(w, r, http.StatusUnprocessableEntity, cur, findingValues(cur), f.Errors)
		return
	}
	if err := s.saveFindingPhotos(ctx, r, cur.ID, "closure"); err != nil {
		s.photoError(w, r, err, cur)
		return
	}
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.CloseFinding(ctx, gen.CloseFindingParams{ID: cur.ID, ClosureNote: note, ClosedBy: currentUserID(ctx)}); err != nil {
			return err
		}
		action := "closed"
		if verifying {
			action = "verified"
			s.notify(ctx, q, s.contractorUserIDs(ctx, cur.WoContractorID), findingNote(notifyFindingVerified, cur, ""))
		}
		s.audit(ctx, q, "finding", cur.ID, action, nil)
		return s.refreshWorkOrderStatus(ctx, q, cur.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "تم إغلاق الملاحظة"), "ok")
	http.Redirect(w, r, "/work-orders/"+idStr(cur.WorkOrderID), http.StatusSeeOther)
}

// findingRespond is the contractor's answer: what was done, the root cause,
// and at least one photo as proof. It goes to the inspector to verify.
func (s *Server) findingRespond(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 4*s.cfg.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		s.badRequest(w, r, appctx.T(ctx, "الملف كبير جداً أو غير صالح"))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	cur, ok := s.loadFinding(w, r)
	if !ok {
		return
	}
	if !domain.FindingAllowed(string(cur.Status), domain.FindingRespond, false, appctx.IsContractor(ctx)) {
		s.badRequest(w, r, appctx.T(ctx, "لا يمكن الرد على هذه الملاحظة الآن"))
		return
	}
	f := newForm(r)
	note := f.Required("response_note", appctx.T(ctx, "ما الذي تم؟"))
	f.MaxLen("response_note", 1500)
	f.MaxLen("root_cause", 1000)
	var rc *gen.RootCause
	if c := gen.RootCause(f.Str("root_cause_category")); c.Valid() {
		rc = &c
	} else {
		f.Errors["root_cause_category"] = appctx.T(ctx, "اختر السبب الجذري")
	}
	if len(r.MultipartForm.File["photos"]) == 0 {
		f.Errors["photos"] = appctx.T(ctx, "أرفق صورة واحدة على الأقل تثبت المعالجة")
	}
	v := findingValues(cur)
	v.ResponseNote, v.RootCause = note, f.Str("root_cause")
	if rc != nil {
		v.RootCauseCategory = string(*rc)
	}
	if !f.Valid() {
		s.renderFinding(w, r, http.StatusUnprocessableEntity, cur, v, f.Errors)
		return
	}
	if err := s.saveFindingPhotos(ctx, r, cur.ID, "response"); err != nil {
		s.photoError(w, r, err, cur)
		return
	}
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.RespondFinding(ctx, gen.RespondFindingParams{
			ID: cur.ID, ResponseNote: note, RespondedBy: currentUserID(ctx), RootCauseCategory: rc, RootCause: v.RootCause,
		})
		if err != nil || n == 0 {
			return err
		}
		s.audit(ctx, q, "finding", cur.ID, "responded", nil)
		var to []int64
		if cur.CreatedBy != nil {
			to = append(to, *cur.CreatedBy)
		}
		s.notify(ctx, q, to, findingNote(notifyFindingResponded, cur, ""))
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "تم إرسال الرد للتحقق"), "ok")
	http.Redirect(w, r, "/findings/"+idStr(cur.ID)+"/edit", http.StatusSeeOther)
}

// findingReject sends a response back to the contractor with the reason.
func (s *Server) findingReject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cur, ok := s.loadFinding(w, r)
	if !ok {
		return
	}
	if !domain.FindingAllowed(string(cur.Status), domain.FindingReject, appctx.CanEdit(ctx), false) {
		s.badRequest(w, r, appctx.T(ctx, "لا يمكن رفض الرد الآن"))
		return
	}
	f := newForm(r)
	reason := f.Required("rejection_note", appctx.T(ctx, "سبب عدم القبول"))
	f.MaxLen("rejection_note", 1000)
	if !f.Valid() {
		s.renderFinding(w, r, http.StatusUnprocessableEntity, cur, findingValues(cur), f.Errors)
		return
	}
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.RejectFindingResponse(ctx, gen.RejectFindingResponseParams{ID: cur.ID, RejectionNote: reason}); err != nil {
			return err
		}
		s.audit(ctx, q, "finding", cur.ID, "response_rejected", map[string]any{"reason": reason})
		s.notify(ctx, q, s.contractorUserIDs(ctx, cur.WoContractorID), findingNote(notifyFindingRejected, cur, reason))
		return s.refreshWorkOrderStatus(ctx, q, cur.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "أُعيدت الملاحظة للمقاول"), "")
	http.Redirect(w, r, "/findings/"+idStr(cur.ID)+"/edit", http.StatusSeeOther)
}

// findingPhotoUpload adds evidence photos to an open finding (staff).
func (s *Server) findingPhotoUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 4*s.cfg.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		s.badRequest(w, r, appctx.T(ctx, "الملف كبير جداً أو غير صالح"))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	cur, ok := s.loadFinding(w, r)
	if !ok {
		return
	}
	if cur.Status == gen.FindingStatusClosed {
		s.badRequest(w, r, appctx.T(ctx, "الملاحظة مغلقة. أعد فتحها أولاً."))
		return
	}
	if err := s.saveFindingPhotos(ctx, r, cur.ID, "evidence"); err != nil {
		s.photoError(w, r, err, cur)
		return
	}
	s.flash(ctx, appctx.T(ctx, "تمت إضافة الصور"), "ok")
	http.Redirect(w, r, "/findings/"+idStr(cur.ID)+"/edit", http.StatusSeeOther)
}

// saveFindingPhotos stores every file in the "photos" field (up to 8).
func (s *Server) saveFindingPhotos(ctx context.Context, r *http.Request, findingID int64, purpose string) error {
	if r.MultipartForm == nil {
		return nil
	}
	files := r.MultipartForm.File["photos"]
	if len(files) > 8 {
		files = files[:8]
	}
	for _, fh := range files {
		if err := s.saveFindingPhoto(ctx, fh, findingID, purpose); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) saveFindingPhoto(ctx context.Context, fh *multipart.FileHeader, findingID int64, purpose string) error {
	file, err := fh.Open()
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	key, size, ctype, err := s.store.Save(ctx, file)
	if err != nil {
		return err
	}
	name := filepath.Base(fh.Filename)
	if len(name) > 200 {
		name = name[:200]
	}
	a, err := s.q.CreateAttachment(ctx, gen.CreateAttachmentParams{
		FindingID: &findingID, StorageKey: key, Filename: name, ContentType: ctype, SizeBytes: size,
		UploadedBy: currentUserID(ctx), Purpose: purpose,
	})
	if err != nil {
		_ = s.store.Delete(ctx, key)
		return err
	}
	s.audit(ctx, s.q, "attachment", a.ID, "uploaded", map[string]any{"finding": findingID, "purpose": purpose})
	return nil
}

func (s *Server) photoError(w http.ResponseWriter, r *http.Request, err error, cur gen.GetFindingRow) {
	ctx := r.Context()
	msg := appctx.T(ctx, "تعذّر رفع الصورة: ") + err.Error()
	if errors.Is(err, storage.ErrUnsupportedType) {
		msg = appctx.T(ctx, "الملف ليس صورة مدعومة (JPG أو PNG أو WEBP)")
	}
	s.renderFinding(w, r, http.StatusUnprocessableEntity, cur, findingValues(cur), map[string]string{"photos": msg})
}

// findingsBulk applies one change to the selected findings (staff).
func (s *Server) findingsBulk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = r.ParseForm()
	var ids []int64
	for _, v := range r.PostForm["ids"] {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	f := newForm(r)
	p := gen.BulkUpdateFindingsParams{Ids: ids, DueDate: f.OptDate("due_date"), MarkInProgress: f.Bool("in_progress")}
	if o := f.Str("owner"); o != "" {
		p.Owner = &o
	}
	if len(ids) == 0 || (p.DueDate == nil && p.Owner == nil && !p.MarkInProgress) {
		s.flash(ctx, appctx.T(ctx, "اختر ملاحظة واحدة على الأقل وتعديلاً واحداً"), "warn")
		http.Redirect(w, r, backTo(r), http.StatusSeeOther)
		return
	}
	n, err := s.q.BulkUpdateFindings(ctx, p)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "finding", 0, "bulk_updated", map[string]any{"ids": ids, "due_date": fmtDate(p.DueDate), "owner": p.Owner, "in_progress": p.MarkInProgress})
	s.flash(ctx, appctx.Tf(ctx, "تم تحديث %d ملاحظة", n), "ok")
	http.Redirect(w, r, backTo(r), http.StatusSeeOther)
}

func (s *Server) findingReopen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cur, ok := s.loadFinding(w, r)
	if !ok {
		return
	}
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.ReopenFinding(ctx, cur.ID); err != nil {
			return err
		}
		s.audit(ctx, q, "finding", cur.ID, "reopened", nil)
		return s.refreshWorkOrderStatus(ctx, q, cur.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "أُعيد فتح الملاحظة"), "")
	http.Redirect(w, r, "/findings/"+idStr(cur.ID)+"/edit", http.StatusSeeOther)
}

func (s *Server) findingDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cur, ok := s.loadFinding(w, r)
	if !ok {
		return
	}
	if cur.InspectionID != nil {
		in, err := s.q.GetInspection(ctx, *cur.InspectionID)
		if err == nil && in.Status == gen.InspectionStatusSubmitted && cur.Status != gen.FindingStatusClosed {
			// Deleting it could make a failed inspection look unsupported; keep the record.
			s.badRequest(w, r, appctx.T(ctx, "هذه الملاحظة مرتبطة بفحص مرسل. أغلقها بدلاً من حذفها."))
			return
		}
	}
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteFinding(ctx, cur.ID); err != nil {
			return err
		}
		s.audit(ctx, q, "finding", cur.ID, "deleted", map[string]any{"work_order": cur.WorkOrderID})
		return s.refreshWorkOrderStatus(ctx, q, cur.WorkOrderID)
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "تم حذف الملاحظة"), "")
	http.Redirect(w, r, "/work-orders/"+idStr(cur.WorkOrderID), http.StatusSeeOther)
}

// findingNote builds a finding notification. Each response, rejection and
// verification is its own event, so there is no dedupe key.
func findingNote(kind string, f gen.GetFindingRow, reason string) note {
	return note{
		kind: kind, link: "/findings/" + idStr(f.ID) + "/edit",
		params: map[string]string{"ref": f.WoRef, "desc": clip(f.Description, 80), "reason": clip(reason, 120)},
	}
}

// findingsForExport converts the filtered list for Excel.
func findingsForExport(ctx context.Context, rows []gen.ListFindingsRow) []excel.FindingExport {
	l := appctx.Lang(ctx)
	out := make([]excel.FindingExport, 0, len(rows))
	for _, f := range rows {
		crit := ""
		if f.CriterionCode != "" {
			crit = f.CriterionCode + " - " + i18n.Pick(l, f.CriterionLabel, f.CriterionLabelEn)
		}
		out = append(out, excel.FindingExport{
			ID: f.ID, Ref: f.WoRef, Site: i18n.Pick(l, f.SiteName, f.SiteNameEn), Contractor: i18n.Pick(l, f.ContractorName, f.ContractorNameEn),
			Criterion: crit, Severity: string(f.Severity), Status: string(f.Status), Description: f.Description,
			CorrectiveAction: f.CorrectiveAction, Owner: ownerOf(f.OwnerUserName, f.Owner), ClosureNote: f.ClosureNote,
			DueDate: f.DueDate, CreatedAt: f.CreatedAt, ClosedAt: f.ClosedAt,
		})
	}
	return out
}

func ownerOf(user, text string) string {
	if user != "" {
		return user
	}
	return text
}

// findingsExport downloads the filtered findings as Excel.
func (s *Server) findingsExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.q.ListFindings(ctx, findingsFilter(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	book, err := excel.FindingsWorkbook(appctx.Lang(ctx), findingsForExport(ctx, rows))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.sendWorkbook(w, r, book, "Kidana-Quality-Findings_"+time.Now().Format(time.DateOnly)+".xlsx")
}

// findingsPrint shows the filtered findings as an A4 landscape list.
func (s *Server) findingsPrint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.q.ListFindings(ctx, findingsFilter(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	q := r.URL.Query()
	d := views.FindingsPrintData{
		Rows: rows, Printed: time.Now().In(dates.Riyadh), Scope: s.describeWOFilter(ctx, q), Back: "/findings?" + q.Encode(),
	}
	if u := appctx.User(ctx); u != nil {
		d.PrintedBy = u.Name
	}
	s.render(w, r, http.StatusOK, views.FindingsPrint(d))
}
