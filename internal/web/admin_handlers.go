package web

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/http"
	"net/mail"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/i18n"
	"github.com/HZamzami/kidana-quality/internal/views"
)

func (s *Server) adminHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, views.AdminHome())
}

// ---------------------------------------------------------------- lookups

var lookupTitles = map[string]string{"sites": "المواقع", "systems": "المنظومات", "asset-groups": "مجموعات الأصول", "contractors": "المقاولون"}

func (s *Server) lookupData(r *http.Request, kind string) (views.LookupData, error) {
	ctx := r.Context()
	d := views.LookupData{Kind: kind, Title: lookupTitles[kind]}
	switch kind {
	case "sites":
		rows, err := s.q.ListSites(ctx)
		if err != nil {
			return d, err
		}
		for _, x := range rows {
			d.Rows = append(d.Rows, views.LookupRow{ID: x.ID, Name: x.Name, NameEn: x.NameEn, Sort: x.Sort, Active: x.Active})
		}
	case "systems":
		rows, err := s.q.ListSystems(ctx)
		if err != nil {
			return d, err
		}
		for _, x := range rows {
			d.Rows = append(d.Rows, views.LookupRow{ID: x.ID, Name: x.Name, NameEn: x.NameEn, Sort: x.Sort, Active: x.Active})
		}
	case "asset-groups":
		rows, err := s.q.ListAssetGroups(ctx)
		if err != nil {
			return d, err
		}
		for _, x := range rows {
			d.Rows = append(d.Rows, views.LookupRow{ID: x.ID, Name: x.Name, NameEn: x.NameEn, Sort: x.Sort, Active: x.Active})
		}
	case "contractors":
		rows, err := s.q.ListContractors(ctx)
		if err != nil {
			return d, err
		}
		for _, x := range rows {
			d.Rows = append(d.Rows, views.LookupRow{ID: x.ID, Name: x.Name, NameEn: x.NameEn, Active: x.Active, ContactName: x.ContactName, Phone: x.Phone})
		}
	}
	return d, nil
}

func (s *Server) lookupList(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if _, ok := lookupTitles[kind]; !ok {
		s.notFound(w, r)
		return
	}
	d, err := s.lookupData(r, kind)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.LookupPage(d))
}

func (s *Server) lookupCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kind := r.PathValue("kind")
	if _, ok := lookupTitles[kind]; !ok {
		s.notFound(w, r)
		return
	}
	f := newForm(r)
	name := f.Required("name", appctx.T(ctx, "الاسم"))
	nameEn := f.Str("name_en")
	f.MaxLen("name", 120)
	f.MaxLen("name_en", 120)
	sort := int32(f.Int("sort", 0)) //nolint:gosec // small admin-entered number
	var err error
	var id int64
	if f.Valid() {
		switch kind {
		case "sites":
			var x gen.Site
			x, err = s.q.CreateSite(ctx, gen.CreateSiteParams{Name: name, NameEn: nameEn, Sort: sort})
			id = x.ID
		case "systems":
			var x gen.System
			x, err = s.q.CreateSystem(ctx, gen.CreateSystemParams{Name: name, NameEn: nameEn, Sort: sort})
			id = x.ID
		case "asset-groups":
			var x gen.AssetGroup
			x, err = s.q.CreateAssetGroup(ctx, gen.CreateAssetGroupParams{Name: name, NameEn: nameEn, Sort: sort})
			id = x.ID
		case "contractors":
			var x gen.Contractor
			x, err = s.q.CreateContractor(ctx, gen.CreateContractorParams{Name: name, NameEn: nameEn, ContactName: f.Str("contact_name"), Phone: f.Str("phone")})
			id = x.ID
		}
		if isUniqueViolation(err) {
			f.Errors["name"] = appctx.T(ctx, "الاسم مسجل مسبقاً")
			err = nil
		}
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !f.Valid() {
		d, err := s.lookupData(r, kind)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		d.Errors, d.NewName = f.Errors, name
		s.render(w, r, http.StatusUnprocessableEntity, views.LookupPage(d))
		return
	}
	s.audit(ctx, s.q, kind, id, "created", map[string]any{"name": name})
	s.flash(ctx, appctx.T(ctx, "تمت الإضافة"), "ok")
	http.Redirect(w, r, "/admin/lookups/"+kind, http.StatusSeeOther)
}

func (s *Server) lookupUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kind := r.PathValue("kind")
	id, ok := pathID(r, "id")
	if _, known := lookupTitles[kind]; !known || !ok {
		s.notFound(w, r)
		return
	}
	f := newForm(r)
	name, nameEn := f.Str("name"), f.Str("name_en")
	if name == "" || len([]rune(name)) > 120 || len([]rune(nameEn)) > 120 {
		s.flash(ctx, appctx.T(ctx, "الاسم مطلوب (120 حرفاً كحد أقصى)"), "warn")
		http.Redirect(w, r, "/admin/lookups/"+kind, http.StatusSeeOther)
		return
	}
	sort := int32(f.Int("sort", 0)) //nolint:gosec // small admin-entered number
	active := f.Bool("active")
	var err error
	switch kind {
	case "sites":
		_, err = s.q.UpdateSite(ctx, gen.UpdateSiteParams{ID: id, Name: name, NameEn: nameEn, Sort: sort, Active: active})
	case "systems":
		_, err = s.q.UpdateSystem(ctx, gen.UpdateSystemParams{ID: id, Name: name, NameEn: nameEn, Sort: sort, Active: active})
	case "asset-groups":
		_, err = s.q.UpdateAssetGroup(ctx, gen.UpdateAssetGroupParams{ID: id, Name: name, NameEn: nameEn, Sort: sort, Active: active})
	case "contractors":
		_, err = s.q.UpdateContractor(ctx, gen.UpdateContractorParams{ID: id, Name: name, NameEn: nameEn, ContactName: f.Str("contact_name"), Phone: f.Str("phone"), Active: active})
	}
	switch {
	case isUniqueViolation(err):
		s.flash(ctx, appctx.T(ctx, "الاسم مسجل مسبقاً"), "warn")
	case err != nil:
		s.dbError(w, r, err)
		return
	default:
		s.audit(ctx, s.q, kind, id, "updated", map[string]any{"name": name, "active": active})
		s.flash(ctx, appctx.T(ctx, "تم الحفظ"), "ok")
	}
	http.Redirect(w, r, "/admin/lookups/"+kind, http.StatusSeeOther)
}

// ---------------------------------------------------------------- users

func (s *Server) usersPage(w http.ResponseWriter, r *http.Request, status int, d views.UsersData) {
	users, err := s.q.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d.Users = users
	lk, err := s.lookupOptions(r.Context(), false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d.Contractors = lk.Contractors
	if d.Requests, err = s.q.ListOpenPasswordResetRequests(r.Context()); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, status, views.UsersPage(d))
}

func (s *Server) usersList(w http.ResponseWriter, r *http.Request) {
	s.usersPage(w, r, http.StatusOK, views.UsersData{})
}

func (s *Server) userCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := newForm(r)
	var d views.UsersData
	d.New.Name = f.Required("name", appctx.T(ctx, "الاسم"))
	d.New.Email = strings.ToLower(f.Required("email", appctx.T(ctx, "البريد الإلكتروني")))
	d.New.Role = f.Str("role")
	f.MaxLen("name", 80)
	if d.New.Email != "" {
		if _, err := mail.ParseAddress(d.New.Email); err != nil {
			f.Errors["email"] = appctx.T(ctx, "بريد إلكتروني غير صالح")
		}
	}
	role := gen.UserRole(d.New.Role)
	if !role.Valid() {
		f.Errors["role"] = appctx.T(ctx, "اختر الصلاحية")
	}
	// A contractor user belongs to exactly one contractor; staff to none.
	contractor := f.OptID("contractor_id")
	d.New.ContractorID = f.Str("contractor_id")
	if role == gen.UserRoleContractor && contractor == nil {
		f.Errors["contractor_id"] = appctx.T(ctx, "اختر المقاول لهذا الحساب")
	}
	if role != gen.UserRoleContractor {
		contractor = nil
	}
	password := r.PostFormValue("password")
	generated := false
	if password == "" {
		password, generated = tempPassword(), true
	} else if len([]rune(password)) < auth.MinPasswordLength {
		f.Errors["password"] = appctx.T(ctx, "10 أحرف على الأقل")
	}
	if !f.Valid() {
		d.Errors = f.Errors
		s.usersPage(w, r, http.StatusUnprocessableEntity, d)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	u, err := s.q.CreateUser(ctx, gen.CreateUserParams{Email: d.New.Email, Name: d.New.Name, Role: role, PasswordHash: hash, ContractorID: contractor})
	if isUniqueViolation(err) {
		d.Errors = map[string]string{"email": appctx.T(ctx, "البريد مسجل لمستخدم آخر")}
		s.usersPage(w, r, http.StatusUnprocessableEntity, d)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "user", u.ID, "created", map[string]any{"role": role})
	info := appctx.T(ctx, "تمت إضافة ") + u.Name + "."
	if generated {
		info += appctx.T(ctx, " كلمة المرور المؤقتة: ") + password + appctx.T(ctx, " — سلّمها للمستخدم واطلب تغييرها بعد الدخول. لن تظهر مرة أخرى.")
	}
	s.usersPage(w, r, http.StatusOK, views.UsersData{Info: info})
}

func (s *Server) userUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	f := newForm(r)
	name, email := f.Str("name"), strings.ToLower(f.Str("email"))
	role := gen.UserRole(f.Str("role"))
	active := f.Bool("active")
	contractor := f.OptID("contractor_id")
	if role != gen.UserRoleContractor {
		contractor = nil
	}
	if _, err := mail.ParseAddress(email); err != nil || name == "" || !role.Valid() || (role == gen.UserRoleContractor && contractor == nil) {
		s.flash(ctx, appctx.T(ctx, "بيانات غير صالحة: تحقق من الاسم والبريد والصلاحية"), "warn")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	me := appctx.User(ctx)
	if me.ID == id && (!active || role != gen.UserRoleAdmin) {
		s.flash(ctx, appctx.T(ctx, "لا يمكنك إلغاء تفعيل حسابك أو إزالة صلاحية المدير عن نفسك"), "warn")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	_, err := s.q.UpdateUser(ctx, gen.UpdateUserParams{ID: id, Email: email, Name: name, Role: role, Active: active, ContractorID: contractor})
	switch {
	case isUniqueViolation(err):
		s.flash(ctx, appctx.T(ctx, "البريد مسجل لمستخدم آخر"), "warn")
	case err != nil:
		s.dbError(w, r, err)
		return
	default:
		s.audit(ctx, s.q, "user", id, "updated", map[string]any{"role": role, "active": active})
		s.flash(ctx, appctx.T(ctx, "تم حفظ المستخدم"), "ok")
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (s *Server) userResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	u, err := s.q.GetUser(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	pw := tempPassword()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{ID: id, PasswordHash: hash}); err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.q.BumpSessionVersion(ctx, id); err != nil { // sign the user out everywhere
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "user", id, "password_reset", nil)
	s.usersPage(w, r, http.StatusOK, views.UsersData{
		Info: appctx.T(ctx, "كلمة المرور المؤقتة الجديدة لـ ") + u.Name + ": " + pw + appctx.T(ctx, " — سلّمها للمستخدم. لن تظهر مرة أخرى."),
	})
}

func tempPassword() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// ---------------------------------------------------------------- checklists

func (s *Server) checklistsList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.q.ListTemplates(ctx)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	// One entry per checklist: its active version (else the newest), with
	// the other versions underneath. Rows come newest first per family.
	var fams []views.ChecklistFamily
	idx := map[int64]int{}
	for _, row := range rows {
		i, ok := idx[row.FamilyID]
		if !ok {
			idx[row.FamilyID] = len(fams)
			fams = append(fams, views.ChecklistFamily{Current: row})
			continue
		}
		f := &fams[i]
		if row.Active && !f.Current.Active {
			f.Older = append(f.Older, f.Current)
			f.Current = row
		} else {
			f.Older = append(f.Older, row)
		}
	}
	s.render(w, r, http.StatusOK, views.ChecklistsPage(fams))
}

func (s *Server) checklistShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	t, err := s.q.GetTemplate(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	crits, err := s.q.ListCriteria(ctx, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	scope, err := s.checklistScopeText(ctx, t)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ChecklistShow(t, crits, scope))
}

// checklistScopeText describes who a checklist applies to.
func (s *Server) checklistScopeText(ctx context.Context, t gen.ChecklistTemplate) (string, error) {
	systems, err := s.q.ListTemplateSystems(ctx, t.ID)
	if err != nil {
		return "", err
	}
	l := appctx.Lang(ctx)
	var parts []string
	if t.AssetGroupID != nil {
		g, err := s.q.GetAssetGroup(ctx, *t.AssetGroupID)
		if err != nil {
			return "", err
		}
		parts = append(parts, appctx.T(ctx, "مجموعة الأصل: ")+i18n.Pick(l, g.Name, g.NameEn))
	}
	if len(systems) > 0 {
		names := make([]string, len(systems))
		for i, sy := range systems {
			names[i] = i18n.Pick(l, sy.Name, sy.NameEn)
		}
		parts = append(parts, appctx.T(ctx, "المنظومات: ")+strings.Join(names, appctx.T(ctx, "، ")))
	}
	if len(parts) == 0 {
		return appctx.T(ctx, "عامة — لكل أوامر العمل التي ليس لها قائمة خاصة"), nil
	}
	return strings.Join(parts, " · "), nil
}

const blankCriterionRows = 3

// checklistBaseName drops the "— الإصدار N" suffix the app adds.
func checklistBaseName(name string) string {
	base, _, _ := strings.Cut(name, " — الإصدار ")
	return strings.TrimSpace(base)
}

func criteriaRows(crits []gen.TemplateCriterion, blanks int) []views.CriterionInput {
	rows := make([]views.CriterionInput, 0, len(crits)+blanks)
	for _, c := range crits {
		rows = append(rows, views.CriterionInput{Code: c.Code, Label: c.Label, Description: c.Description, LabelEn: c.LabelEn, DescriptionEn: c.DescriptionEn, Weight: strconv.Itoa(int(c.Weight)), Critical: c.IsCritical})
	}
	for i := 0; i < blanks; i++ {
		rows = append(rows, views.CriterionInput{})
	}
	return rows
}

// checklistNew starts a new checklist (its own scope, version 1), either
// blank or copying the items of an existing one: /admin/checklists/new?stage=after&from=ID.
func (s *Server) checklistNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stage := r.URL.Query().Get("stage")
	if !gen.InspectionStage(stage).Valid() {
		stage = string(gen.InspectionStageAfter)
	}
	d := views.ChecklistFormData{Stage: stage, Activate: true}
	fromQ := r.URL.Query().Get("from")
	if fromQ == "" { // default: the stage's general checklist
		if g, err := s.q.GetActiveTemplate(ctx, gen.InspectionStage(stage)); err == nil {
			fromQ = idStr(g.ID)
		}
	}
	if from, err := strconv.ParseInt(fromQ, 10, 64); err == nil && from > 0 {
		crits, err := s.q.ListCriteria(ctx, from)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		d.Rows = criteriaRows(crits, blankCriterionRows)
	}
	if len(d.Rows) == 0 {
		d.Rows = criteriaRows(nil, 6)
	}
	if err := s.checklistScopes(ctx, &d); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ChecklistForm(d))
}

// checklistNewVersion edits a checklist: a new version with the same scope.
func (s *Server) checklistNewVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	t, err := s.q.GetTemplate(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	crits, err := s.q.ListCriteria(ctx, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := views.ChecklistFormData{Stage: string(t.Stage), Name: checklistBaseName(t.Name), Activate: true, Family: t.FamilyID, Rows: criteriaRows(crits, blankCriterionRows)}
	if d.ScopeText, err = s.checklistScopeText(ctx, t); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ChecklistForm(d))
}

func (s *Server) checklistCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := newForm(r)
	d := views.ChecklistFormData{Stage: f.Str("stage"), Name: f.Str("name"), Activate: f.Bool("activate"), Family: int64(f.Int("family", 0))}
	var scope domain.ChecklistScope
	if d.Family > 0 {
		// Editing: stage and scope come from the checklist, not the form.
		latest, err := s.q.GetFamilyLatest(ctx, d.Family)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		d.Stage = string(latest.Stage)
		scope.AssetGroupID = latest.AssetGroupID
		systems, err := s.q.ListTemplateSystems(ctx, latest.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		for _, sy := range systems {
			scope.SystemIDs = append(scope.SystemIDs, sy.ID)
		}
		if d.ScopeText, err = s.checklistScopeText(ctx, latest); err != nil {
			s.serverError(w, r, err)
			return
		}
	} else {
		scope.AssetGroupID = f.OptID("asset_group_id")
		d.AssetGroupID = f.Str("asset_group_id")
		for _, v := range r.PostForm["system_id"] {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 && !slices.Contains(scope.SystemIDs, id) {
				scope.SystemIDs = append(scope.SystemIDs, id)
				d.SystemIDs = append(d.SystemIDs, v)
			}
		}
		if err := s.checklistScopes(ctx, &d); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	stage := gen.InspectionStage(d.Stage)
	if !stage.Valid() {
		s.badRequest(w, r, appctx.T(ctx, "مرحلة غير صالحة"))
		return
	}
	n := f.Int("rows", 0)
	if n < 0 || n > 60 {
		s.badRequest(w, r, appctx.T(ctx, "عدد بنود غير صالح"))
		return
	}
	type crit struct {
		code, label, desc string
		labelEn, descEn   string
		weight            int
		critical          bool
	}
	var crits []crit
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		in := views.CriterionInput{
			Code: f.Str(fmt.Sprintf("code_%d", i)), Label: f.Str(fmt.Sprintf("label_%d", i)),
			Description: f.Str(fmt.Sprintf("desc_%d", i)), Weight: domain.ASCIIDigits(f.Str(fmt.Sprintf("weight_%d", i))),
			LabelEn: f.Str(fmt.Sprintf("label_en_%d", i)), DescriptionEn: f.Str(fmt.Sprintf("desc_en_%d", i)),
			Critical: f.Bool(fmt.Sprintf("critical_%d", i)),
		}
		d.Rows = append(d.Rows, in)
		if in.Code == "" && in.Label == "" {
			continue
		}
		row := i + 1
		wt, err := strconv.Atoi(in.Weight)
		var rowErrs []string
		if in.Code == "" || in.Label == "" {
			rowErrs = append(rowErrs, appctx.T(ctx, "الرمز والبند مطلوبان"))
		}
		if err != nil || wt < 1 || wt > 100 {
			rowErrs = append(rowErrs, appctx.T(ctx, "الوزن رقم من 1 إلى 100"))
		}
		if in.Code != "" && seen[strings.ToUpper(in.Code)] {
			rowErrs = append(rowErrs, appctx.T(ctx, "الرمز ")+in.Code+appctx.T(ctx, " مكرر"))
		}
		if in.Code != "" {
			seen[strings.ToUpper(in.Code)] = true
		}
		if len(rowErrs) > 0 {
			d.Errors = append(d.Errors, appctx.Tf(ctx, "الصف %d: %s", row, strings.Join(rowErrs, appctx.T(ctx, "، "))))
			continue
		}
		crits = append(crits, crit{in.Code, in.Label, in.Description, in.LabelEn, in.DescriptionEn, wt, in.Critical})
	}
	if d.Name == "" {
		d.Errors = append(d.Errors, appctx.T(ctx, "اسم القائمة مطلوب"))
	}
	if len(crits) == 0 {
		d.Errors = append(d.Errors, appctx.T(ctx, "أضف بنداً واحداً على الأقل"))
	} else if len(d.Errors) == 0 {
		weights := make([]int, len(crits))
		for i, c := range crits {
			weights[i] = c.weight
		}
		if msg := weightProblem(ctx, weights...); msg != "" {
			d.Errors = append(d.Errors, msg)
		}
	}
	if d.Activate {
		active, err := s.q.ListActiveScopes(ctx, stage)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		for _, a := range active {
			if a.FamilyID != d.Family && domain.ScopesOverlap(scope, domain.ChecklistScope{AssetGroupID: a.AssetGroupID, SystemIDs: a.SystemIds}) {
				d.Errors = append(d.Errors, appctx.Tf(ctx, "يوجد تداخل مع القائمة المفعّلة «%s» لنفس المنظومة أو مجموعة الأصل. لتغيير بنودها افتح «تعديل» عليها، أو اختر نطاقاً آخر، أو احفظ دون تفعيل.", checklistBaseName(a.Name)))
				break
			}
		}
	}
	if len(d.Errors) > 0 {
		s.render(w, r, http.StatusUnprocessableEntity, views.ChecklistForm(d))
		return
	}
	var newID int64
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		id, err := q.NewTemplateID(ctx)
		if err != nil {
			return err
		}
		family, ver := id, int32(1)
		if d.Family > 0 {
			family = d.Family
			if ver, err = q.NextTemplateVersion(ctx, family); err != nil {
				return err
			}
		}
		t, err := q.CreateTemplate(ctx, gen.CreateTemplateParams{
			ID: id, Stage: stage, Version: ver, Name: appctx.Tf(ctx, "%s — الإصدار %d", checklistBaseName(d.Name), ver),
			CreatedBy: currentUserID(ctx), AssetGroupID: scope.AssetGroupID, FamilyID: family,
		})
		if err != nil {
			return err
		}
		for _, sys := range scope.SystemIDs {
			if err := q.AddTemplateSystem(ctx, gen.AddTemplateSystemParams{TemplateID: t.ID, SystemID: sys}); err != nil {
				return err
			}
		}
		for i, c := range crits {
			if _, err := q.CreateCriterion(ctx, gen.CreateCriterionParams{
				TemplateID: t.ID, Code: c.code, Label: c.label, Description: c.desc, LabelEn: c.labelEn, DescriptionEn: c.descEn,
				Weight: int32(c.weight), IsCritical: c.critical, Sort: int32(i + 1), //nolint:gosec // validated 1..100, ≤60 rows
			}); err != nil {
				return err
			}
		}
		if d.Activate {
			if err := q.DeactivateFamily(ctx, family); err != nil {
				return err
			}
			if err := q.ActivateTemplate(ctx, t.ID); err != nil {
				return err
			}
		}
		newID = t.ID
		s.audit(ctx, q, "checklist_template", t.ID, "created", map[string]any{"stage": stage, "version": ver, "activated": d.Activate, "new_checklist": d.Family == 0})
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if d.Family == 0 {
		s.flash(ctx, appctx.T(ctx, "تم إنشاء القائمة الجديدة"), "ok")
	} else {
		s.flash(ctx, appctx.T(ctx, "تم حفظ الإصدار الجديد"), "ok")
	}
	http.Redirect(w, r, "/admin/checklists/"+idStr(newID), http.StatusSeeOther)
}

// checklistDeactivate stops a specific checklist; its work orders go back
// to the general one. The general checklist of a stage can't be stopped.
func (s *Server) checklistDeactivate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	t, err := s.q.GetTemplate(ctx, id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	systems, err := s.q.ListTemplateSystems(ctx, t.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if t.AssetGroupID == nil && len(systems) == 0 {
		s.flash(ctx, appctx.T(ctx, "القائمة العامة لا تُوقف، فهي تُستخدم لكل أمر عمل ليس له قائمة خاصة. عدّل بنودها بدلاً من ذلك."), "warn")
		http.Redirect(w, r, "/admin/checklists", http.StatusSeeOther)
		return
	}
	if err := s.q.DeactivateFamily(ctx, t.FamilyID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "checklist_template", t.ID, "deactivated", nil)
	s.flash(ctx, appctx.T(ctx, "أُوقفت القائمة. الفحوصات الجديدة لهذه الأوامر تستخدم القائمة العامة."), "ok")
	http.Redirect(w, r, "/admin/checklists", http.StatusSeeOther)
}

// weightProblem explains, in plain words, weights that don't add up to 100.
func weightProblem(ctx context.Context, weights ...int) string {
	gap := domain.WeightGap(weights...)
	switch {
	case gap > 0:
		return appctx.Tf(ctx, "مجموع الأوزان %d من 100. أضف %d إلى بند أو أكثر ليصبح المجموع 100.", domain.WeightTotal-gap, gap)
	case gap < 0:
		return appctx.Tf(ctx, "مجموع الأوزان %d من 100. أنقص %d من بند أو أكثر ليصبح المجموع 100.", domain.WeightTotal-gap, -gap)
	}
	return ""
}

// checklistScopes fills the system and asset group choices of the checklist form.
func (s *Server) checklistScopes(ctx context.Context, d *views.ChecklistFormData) error {
	lk, err := s.lookupOptions(ctx, true)
	if err != nil {
		return err
	}
	d.Systems, d.AssetGroups = lk.Systems, lk.AssetGroups
	return nil
}
