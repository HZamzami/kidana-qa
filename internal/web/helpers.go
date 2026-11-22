package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/HZamzami/kidana-qa/internal/appctx"
	"github.com/HZamzami/kidana-qa/internal/db/gen"
	"github.com/HZamzami/kidana-qa/internal/views"
)

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// render writes a templ component with the given status.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		s.log.Error("render", slog.String("path", r.URL.Path), slog.Any("err", err))
	}
}

// redirect works for both normal and htmx requests.
func redirect(w http.ResponseWriter, r *http.Request, url string) {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (s *Server) flash(ctx context.Context, msg, tone string) {
	s.sm.Put(ctx, sessionFlash, msg)
	s.sm.Put(ctx, sessionFlashT, tone)
}

// toast asks the client to show a toast after an htmx swap.
// HTTP header values are read as Latin-1 by browsers, so every non-ASCII
// character is escaped (\uXXXX) or Arabic text arrives garbled.
func toast(w http.ResponseWriter, msg, tone string) {
	b, _ := json.Marshal(map[string]any{"toast": map[string]string{"message": msg, "tone": tone}})
	w.Header().Set("HX-Trigger", asciiJSON(b))
}

func asciiJSON(b []byte) string {
	var sb strings.Builder
	for _, r := range string(b) {
		if r < 0x80 {
			sb.WriteRune(r)
			continue
		}
		for _, u := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&sb, "\\u%04x", u)
		}
	}
	return sb.String()
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("server error", slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Any("err", err))
	s.render(w, r, http.StatusInternalServerError, views.ErrorPage(http.StatusInternalServerError, "حدث خطأ غير متوقع. حاول مرة أخرى، وإن تكرر الخطأ أبلغ مدير النظام."))
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusNotFound, views.ErrorPage(http.StatusNotFound, "الصفحة أو السجل المطلوب غير موجود."))
}

func (s *Server) forbidden(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusForbidden, views.ErrorPage(http.StatusForbidden, "ليست لديك صلاحية لهذا الإجراء."))
}

func (s *Server) badRequest(w http.ResponseWriter, r *http.Request, msg string) {
	s.render(w, r, http.StatusBadRequest, views.ErrorPage(http.StatusBadRequest, msg))
}

// dbError maps "no rows" to 404 and everything else to 500.
func (s *Server) dbError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		s.notFound(w, r)
		return
	}
	s.serverError(w, r, err)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

func currentUserID(ctx context.Context) *int64 {
	if u := appctx.User(ctx); u != nil {
		id := u.ID
		return &id
	}
	return nil
}

// audit records a change. Failures are logged but never block the request.
func (s *Server) audit(ctx context.Context, q *gen.Queries, entity string, id int64, action string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	b, _ := json.Marshal(details)
	if err := q.InsertAudit(ctx, gen.InsertAuditParams{
		UserID: currentUserID(ctx), Entity: entity, EntityID: id, Action: action, Details: b,
	}); err != nil {
		s.log.Warn("audit", slog.Any("err", err))
	}
}

// ---------------------------------------------------------------- form parsing

// form collects values and validation errors for one submitted form.
type form struct {
	r      *http.Request
	Errors map[string]string
}

func newForm(r *http.Request) *form { return &form{r: r, Errors: map[string]string{}} }

func (f *form) Str(name string) string { return strings.TrimSpace(f.r.PostFormValue(name)) }

func (f *form) Required(name, label string) string {
	v := f.Str(name)
	if v == "" {
		f.Errors[name] = "حقل «" + label + "» مطلوب"
	}
	return v
}

func (f *form) MaxLen(name string, n int) {
	if len([]rune(f.Str(name))) > n {
		f.Errors[name] = "النص أطول من المسموح (" + strconv.Itoa(n) + " حرفاً)"
	}
}

// OptID parses an optional positive id ("" → nil).
func (f *form) OptID(name string) *int64 {
	v := f.Str(name)
	if v == "" {
		return nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		f.Errors[name] = "قيمة غير صالحة"
		return nil
	}
	return &id
}

func (f *form) ReqID(name, label string) int64 {
	id := f.OptID(name)
	if id == nil {
		if _, bad := f.Errors[name]; !bad {
			f.Errors[name] = "حقل «" + label + "» مطلوب"
		}
		return 0
	}
	return *id
}

func (f *form) Int(name string, def int) int {
	v := f.Str(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		f.Errors[name] = "أدخل رقماً صحيحاً"
		return def
	}
	return n
}

// OptDate parses an optional yyyy-mm-dd date.
func (f *form) OptDate(name string) *time.Time {
	v := f.Str(name)
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		f.Errors[name] = "تاريخ غير صالح"
		return nil
	}
	return &t
}

func (f *form) Bool(name string) bool {
	v := f.Str(name)
	return v == "on" || v == "true" || v == "1"
}

func (f *form) Valid() bool { return len(f.Errors) == 0 }

// ---------------------------------------------------------------- query parsing

func queryID(r *http.Request, name string) *int64 {
	id, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return &id
}

func queryStr(r *http.Request, name string) *string {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return nil
	}
	return &v
}

func queryDate(r *http.Request, name string) *time.Time {
	t, err := time.Parse(time.DateOnly, r.URL.Query().Get(name))
	if err != nil {
		return nil
	}
	return &t
}
