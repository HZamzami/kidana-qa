package web

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
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
			d.Rows = append(d.Rows, views.LookupRow{ID: x.ID, Name: x.Name, Sort: x.Sort, Active: x.Active})
		}
	case "systems":
		rows, err := s.q.ListSystems(ctx)
		if err != nil {
			return d, err
		}
		for _, x := range rows {
			d.Rows = append(d.Rows, views.LookupRow{ID: x.ID, Name: x.Name, Sort: x.Sort, Active: x.Active})
		}
	case "asset-groups":
		rows, err := s.q.ListAssetGroups(ctx)
		if err != nil {
			return d, err
		}
		for _, x := range rows {
			d.Rows = append(d.Rows, views.LookupRow{ID: x.ID, Name: x.Name, Sort: x.Sort, Active: x.Active})
		}
	case "contractors":
		rows, err := s.q.ListContractors(ctx)
		if err != nil {
			return d, err
		}
		for _, x := range rows {
			d.Rows = append(d.Rows, views.LookupRow{ID: x.ID, Name: x.Name, Active: x.Active, ContactName: x.ContactName, Phone: x.Phone})
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
	name := f.Required("name", "الاسم")
	f.MaxLen("name", 120)
	sort := int32(f.Int("sort", 0)) //nolint:gosec // small admin-entered number
	var err error
	var id int64
	if f.Valid() {
		switch kind {
		case "sites":
			var x gen.Site
			x, err = s.q.CreateSite(ctx, gen.CreateSiteParams{Name: name, Sort: sort})
			id = x.ID
		case "systems":
			var x gen.System
			x, err = s.q.CreateSystem(ctx, gen.CreateSystemParams{Name: name, Sort: sort})
			id = x.ID
		case "asset-groups":
			var x gen.AssetGroup
			x, err = s.q.CreateAssetGroup(ctx, gen.CreateAssetGroupParams{Name: name, Sort: sort})
			id = x.ID
		case "contractors":
			var x gen.Contractor
			x, err = s.q.CreateContractor(ctx, gen.CreateContractorParams{Name: name, ContactName: f.Str("contact_name"), Phone: f.Str("phone")})
			id = x.ID
		}
		if isUniqueViolation(err) {
			f.Errors["name"] = "الاسم مسجل مسبقاً"
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
	s.flash(ctx, "تمت الإضافة", "ok")
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
	name := f.Str("name")
	if name == "" || len([]rune(name)) > 120 {
		s.flash(ctx, "الاسم مطلوب (120 حرفاً كحد أقصى)", "warn")
		http.Redirect(w, r, "/admin/lookups/"+kind, http.StatusSeeOther)
		return
	}
	sort := int32(f.Int("sort", 0)) //nolint:gosec // small admin-entered number
	active := f.Bool("active")
	var err error
	switch kind {
	case "sites":
		_, err = s.q.UpdateSite(ctx, gen.UpdateSiteParams{ID: id, Name: name, Sort: sort, Active: active})
	case "systems":
		_, err = s.q.UpdateSystem(ctx, gen.UpdateSystemParams{ID: id, Name: name, Sort: sort, Active: active})
	case "asset-groups":
		_, err = s.q.UpdateAssetGroup(ctx, gen.UpdateAssetGroupParams{ID: id, Name: name, Sort: sort, Active: active})
	case "contractors":
		_, err = s.q.UpdateContractor(ctx, gen.UpdateContractorParams{ID: id, Name: name, ContactName: f.Str("contact_name"), Phone: f.Str("phone"), Active: active})
	}
	switch {
	case isUniqueViolation(err):
		s.flash(ctx, "الاسم مسجل مسبقاً", "warn")
	case err != nil:
		s.dbError(w, r, err)
		return
	default:
		s.audit(ctx, s.q, kind, id, "updated", map[string]any{"name": name, "active": active})
		s.flash(ctx, "تم الحفظ", "ok")
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
	s.render(w, r, status, views.UsersPage(d))
}

func (s *Server) usersList(w http.ResponseWriter, r *http.Request) {
	s.usersPage(w, r, http.StatusOK, views.UsersData{})
}

func (s *Server) userCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := newForm(r)
	var d views.UsersData
	d.New.Name = f.Required("name", "الاسم")
	d.New.Email = strings.ToLower(f.Required("email", "البريد الإلكتروني"))
	d.New.Role = f.Str("role")
	f.MaxLen("name", 80)
	if d.New.Email != "" {
		if _, err := mail.ParseAddress(d.New.Email); err != nil {
			f.Errors["email"] = "بريد إلكتروني غير صالح"
		}
	}
	role := gen.UserRole(d.New.Role)
	if !role.Valid() {
		f.Errors["role"] = "اختر الصلاحية"
	}
	password := r.PostFormValue("password")
	generated := false
	if password == "" {
		password, generated = tempPassword(), true
	} else if len([]rune(password)) < auth.MinPasswordLength {
		f.Errors["password"] = "10 أحرف على الأقل"
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
	u, err := s.q.CreateUser(ctx, gen.CreateUserParams{Email: d.New.Email, Name: d.New.Name, Role: role, PasswordHash: hash})
	if isUniqueViolation(err) {
		d.Errors = map[string]string{"email": "البريد مسجل لمستخدم آخر"}
		s.usersPage(w, r, http.StatusUnprocessableEntity, d)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "user", u.ID, "created", map[string]any{"role": role})
	info := "تمت إضافة " + u.Name + "."
	if generated {
		info += " كلمة المرور المؤقتة: " + password + " — سلّمها للمستخدم واطلب تغييرها بعد الدخول. لن تظهر مرة أخرى."
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
	if _, err := mail.ParseAddress(email); err != nil || name == "" || !role.Valid() {
		s.flash(ctx, "بيانات غير صالحة: تحقق من الاسم والبريد والصلاحية", "warn")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	me := appctx.User(ctx)
	if me.ID == id && (!active || role != gen.UserRoleAdmin) {
		s.flash(ctx, "لا يمكنك إلغاء تفعيل حسابك أو إزالة صلاحية المدير عن نفسك", "warn")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	_, err := s.q.UpdateUser(ctx, gen.UpdateUserParams{ID: id, Email: email, Name: name, Role: role, Active: active})
	switch {
	case isUniqueViolation(err):
		s.flash(ctx, "البريد مسجل لمستخدم آخر", "warn")
	case err != nil:
		s.dbError(w, r, err)
		return
	default:
		s.audit(ctx, s.q, "user", id, "updated", map[string]any{"role": role, "active": active})
		s.flash(ctx, "تم حفظ المستخدم", "ok")
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
	s.audit(ctx, s.q, "user", id, "password_reset", nil)
	s.usersPage(w, r, http.StatusOK, views.UsersData{
		Info: "كلمة المرور المؤقتة الجديدة لـ " + u.Name + ": " + pw + " — سلّمها للمستخدم. لن تظهر مرة أخرى.",
	})
}

func tempPassword() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// ---------------------------------------------------------------- checklists

func (s *Server) checklistsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListTemplates(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ChecklistsPage(rows))
}

func (s *Server) checklistShow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	t, err := s.q.GetTemplate(r.Context(), id)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	crits, err := s.q.ListCriteria(r.Context(), id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ChecklistShow(t, crits))
}

const blankCriterionRows = 3

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
	next, err := s.q.NextTemplateVersion(ctx, t.Stage)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	name := strings.TrimSpace(strings.Split(t.Name, " — ")[0])
	d := views.ChecklistFormData{Stage: string(t.Stage), Name: fmt.Sprintf("%s — الإصدار %d", name, next), Activate: true}
	for _, c := range crits {
		d.Rows = append(d.Rows, views.CriterionInput{Code: c.Code, Label: c.Label, Description: c.Description, Weight: strconv.Itoa(int(c.Weight)), Critical: c.IsCritical})
	}
	for i := 0; i < blankCriterionRows; i++ {
		d.Rows = append(d.Rows, views.CriterionInput{})
	}
	s.render(w, r, http.StatusOK, views.ChecklistForm(d))
}

func (s *Server) checklistCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := newForm(r)
	d := views.ChecklistFormData{Stage: f.Str("stage"), Name: f.Str("name"), Activate: f.Bool("activate")}
	stage := gen.InspectionStage(d.Stage)
	if !stage.Valid() {
		s.badRequest(w, r, "مرحلة غير صالحة")
		return
	}
	n := f.Int("rows", 0)
	if n < 0 || n > 60 {
		s.badRequest(w, r, "عدد بنود غير صالح")
		return
	}
	type crit struct {
		code, label, desc string
		weight            int
		critical          bool
	}
	var crits []crit
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		in := views.CriterionInput{
			Code: f.Str(fmt.Sprintf("code_%d", i)), Label: f.Str(fmt.Sprintf("label_%d", i)),
			Description: f.Str(fmt.Sprintf("desc_%d", i)), Weight: f.Str(fmt.Sprintf("weight_%d", i)),
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
			rowErrs = append(rowErrs, "الرمز والبند مطلوبان")
		}
		if err != nil || wt < 1 || wt > 100 {
			rowErrs = append(rowErrs, "الوزن رقم من 1 إلى 100")
		}
		if in.Code != "" && seen[strings.ToUpper(in.Code)] {
			rowErrs = append(rowErrs, "الرمز "+in.Code+" مكرر")
		}
		if in.Code != "" {
			seen[strings.ToUpper(in.Code)] = true
		}
		if len(rowErrs) > 0 {
			d.Errors = append(d.Errors, fmt.Sprintf("الصف %d: %s", row, strings.Join(rowErrs, "، ")))
			continue
		}
		crits = append(crits, crit{in.Code, in.Label, in.Description, wt, in.Critical})
	}
	if d.Name == "" {
		d.Errors = append(d.Errors, "اسم القائمة مطلوب")
	}
	if len(crits) == 0 {
		d.Errors = append(d.Errors, "أضف بنداً واحداً على الأقل")
	}
	if len(d.Errors) > 0 {
		s.render(w, r, http.StatusUnprocessableEntity, views.ChecklistForm(d))
		return
	}
	var newID int64
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		ver, err := q.NextTemplateVersion(ctx, stage)
		if err != nil {
			return err
		}
		t, err := q.CreateTemplate(ctx, gen.CreateTemplateParams{Stage: stage, Version: ver, Name: d.Name, CreatedBy: currentUserID(ctx)})
		if err != nil {
			return err
		}
		for i, c := range crits {
			if _, err := q.CreateCriterion(ctx, gen.CreateCriterionParams{
				TemplateID: t.ID, Code: c.code, Label: c.label, Description: c.desc,
				Weight: int32(c.weight), IsCritical: c.critical, Sort: int32(i + 1), //nolint:gosec // validated 1..100, ≤60 rows
			}); err != nil {
				return err
			}
		}
		if d.Activate {
			if err := q.DeactivateStageTemplates(ctx, stage); err != nil {
				return err
			}
			if err := q.ActivateTemplate(ctx, t.ID); err != nil {
				return err
			}
		}
		newID = t.ID
		s.audit(ctx, q, "checklist_template", t.ID, "created", map[string]any{"stage": stage, "version": ver, "activated": d.Activate})
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, "تم حفظ الإصدار الجديد", "ok")
	http.Redirect(w, r, "/admin/checklists/"+idStr(newID), http.StatusSeeOther)
}
