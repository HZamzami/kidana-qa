package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/excel"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// The work order API lets a CMMS (or any script) keep work orders in sync
// without Excel. It sits outside sessions and uses bearer tokens that admins
// create. Names (site, system, asset group, contractor) are matched like the
// Excel import, and new names are added.

type apiTokenKey struct{}

// requireAPIToken checks "Authorization: Bearer <token>".
func (s *Server) requireAPIToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			apiError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		t, err := s.q.UseAPIToken(r.Context(), auth.HashToken(strings.TrimSpace(token)))
		if err != nil {
			apiError(w, http.StatusUnauthorized, "invalid or revoked token")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), apiTokenKey{}, t.Name)))
	}
}

func apiError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiWorkOrder is the JSON shape of a work order.
type apiWorkOrder struct {
	Ref          string   `json:"ref"`
	Type         string   `json:"type"`
	Status       string   `json:"status"`
	Priority     string   `json:"priority"`
	Site         string   `json:"site"`
	Location     string   `json:"location"`
	System       string   `json:"system"`
	AssetGroup   string   `json:"asset_group"`
	Asset        string   `json:"asset"`
	Contractor   string   `json:"contractor"`
	Technician   string   `json:"technician"`
	Description  string   `json:"description"`
	ReportedOn   string   `json:"reported_on"`
	PlannedDate  string   `json:"planned_date,omitempty"`
	ActualDate   string   `json:"actual_date,omitempty"`
	DuringScore  *float64 `json:"during_score"`
	AfterScore   *float64 `json:"after_score"`
	AfterPassed  bool     `json:"after_passed"`
	OpenFindings int      `json:"open_findings"`
	UpdatedAt    string   `json:"updated_at"`
}

func toAPI(w gen.ListWorkOrdersAPIRow) apiWorkOrder {
	score := func(v float64) *float64 {
		if v < 0 {
			return nil
		}
		return &v
	}
	return apiWorkOrder{
		Ref: w.Ref, Type: string(w.Type), Status: string(w.Status), Priority: string(w.Priority), Site: w.Site,
		Location: w.Location, System: w.System, AssetGroup: w.AssetGroup, Asset: w.Asset, Contractor: w.Contractor,
		Technician: w.Technician, Description: w.Description, ReportedOn: w.ReportedOn.Format(time.DateOnly),
		PlannedDate: fmtDate(w.PlannedDate), ActualDate: fmtDate(w.ActualDate), DuringScore: score(w.DuringScore),
		AfterScore: score(w.AfterScore), AfterPassed: w.AfterPassed, OpenFindings: int(w.OpenFindings),
		UpdatedAt: w.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// apiListWorkOrders: GET /api/v1/work-orders?updated_since=RFC3339&limit=N
// Results are ordered by updated_at, so a client can page with the last
// updated_at it saw.
func (s *Server) apiListWorkOrders(w http.ResponseWriter, r *http.Request) {
	p := gen.ListWorkOrdersAPIParams{Lim: 500}
	if v := r.URL.Query().Get("updated_since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			apiError(w, http.StatusBadRequest, "updated_since must be RFC 3339, e.g. 2026-11-01T00:00:00Z")
			return
		}
		p.UpdatedSince = &t
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 500 {
		p.Lim = int32(n) //nolint:gosec // 1..500
	}
	rows, err := s.q.ListWorkOrdersAPI(r.Context(), p)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "server error")
		return
	}
	out := make([]apiWorkOrder, len(rows))
	for i, row := range rows {
		out[i] = toAPI(row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"work_orders": out})
}

// apiGetWorkOrder: GET /api/v1/work-orders/{ref}
func (s *Server) apiGetWorkOrder(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	rows, err := s.q.ListWorkOrdersAPI(r.Context(), gen.ListWorkOrdersAPIParams{Ref: &ref, Lim: 1})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "server error")
		return
	}
	if len(rows) == 0 {
		apiError(w, http.StatusNotFound, "work order not found")
		return
	}
	writeJSON(w, http.StatusOK, toAPI(rows[0]))
}

// apiPutBody is what a client sends to create or update a work order.
type apiPutBody struct {
	Type        string `json:"type"` // PM | WO
	Site        string `json:"site"`
	Location    string `json:"location"`
	System      string `json:"system"`
	AssetGroup  string `json:"asset_group"`
	Asset       string `json:"asset"`
	Contractor  string `json:"contractor"`
	Technician  string `json:"technician"`
	Priority    string `json:"priority"` // normal | high | emergency (Arabic labels accepted)
	Description string `json:"description"`
	ReportedOn  string `json:"reported_on"`
	PlannedDate string `json:"planned_date"`
	ActualDate  string `json:"actual_date"`
}

// apiPutWorkOrder: PUT /api/v1/work-orders/{ref} creates or updates by ref.
func (s *Server) apiPutWorkOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ref := strings.TrimSpace(r.PathValue("ref"))
	var b apiPutBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	var problems []string
	typ, ok := excel.ParseType(b.Type)
	if !ok || typ == "" {
		problems = append(problems, "type must be PM or WO")
	}
	pri, ok := excel.ParsePriority(b.Priority)
	if !ok {
		problems = append(problems, "priority must be normal, high or emergency")
	}
	if ref == "" || len(ref) > 60 {
		problems = append(problems, "ref is required (60 characters max)")
	}
	for name, v := range map[string]string{"site": b.Site, "system": b.System, "asset_group": b.AssetGroup} {
		if strings.TrimSpace(v) == "" {
			problems = append(problems, name+" is required")
		}
	}
	date := func(field, v string) *time.Time {
		if v == "" {
			return nil
		}
		t, ok := excel.ParseDate(v)
		if !ok || t == nil {
			problems = append(problems, field+" must be a date (yyyy-mm-dd)")
		}
		return t
	}
	reported, planned, actual := date("reported_on", b.ReportedOn), date("planned_date", b.PlannedDate), date("actual_date", b.ActualDate)
	if len(problems) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation failed", "problems": problems})
		return
	}

	created := false
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		lr := &lookupResolver{q: q, ctx: ctx, cache: map[string]int64{}}
		siteID, err := lr.id("site", b.Site)
		if err != nil {
			return err
		}
		systemID, err := lr.id("system", b.System)
		if err != nil {
			return err
		}
		groupID, err := lr.id("asset_group", b.AssetGroup)
		if err != nil {
			return err
		}
		conID, err := lr.optional("contractor", b.Contractor)
		if err != nil {
			return err
		}
		in := woInput{Type: gen.WoType(typ), Priority: gen.WoPriority(pri), PlannedDate: planned, ReportedOn: reported}
		due := s.dueDate(ctx, in)
		var wo gen.WorkOrder
		existing, err := q.GetWorkOrderByRef(ctx, ref)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			created = true
			wo, err = q.CreateWorkOrder(ctx, gen.CreateWorkOrderParams{
				Ref: ref, Type: gen.WoType(typ), SiteID: siteID, Location: b.Location, SystemID: systemID, AssetGroupID: groupID,
				Asset: b.Asset, ContractorID: conID, Technician: b.Technician, Priority: gen.WoPriority(pri),
				Description: b.Description, PlannedDate: due, ActualDate: actual, ReportedOn: reported,
			})
		case err == nil:
			wo, err = q.UpdateWorkOrderAPI(ctx, gen.UpdateWorkOrderAPIParams{
				ID: existing.ID, Type: gen.WoType(typ), SiteID: siteID, Location: b.Location, SystemID: systemID, AssetGroupID: groupID,
				Asset: b.Asset, ContractorID: conID, Technician: b.Technician, Priority: gen.WoPriority(pri),
				Description: b.Description, PlannedDate: due, ActualDate: actual, ReportedOn: reported,
			})
		}
		if err != nil {
			return err
		}
		if err := s.linkAsset(ctx, q, wo); err != nil {
			return err
		}
		action := "updated"
		if created {
			action = "created"
			if err := s.sampleNewWorkOrder(ctx, q, wo); err != nil {
				return err
			}
		}
		token, _ := ctx.Value(apiTokenKey{}).(string)
		s.audit(ctx, q, "work_order", wo.ID, action, map[string]any{"via": "api", "token": token})
		return nil
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "server error")
		return
	}
	rows, err := s.q.ListWorkOrdersAPI(ctx, gen.ListWorkOrdersAPIParams{Ref: &ref, Lim: 1})
	if err != nil || len(rows) == 0 {
		apiError(w, http.StatusInternalServerError, "server error")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, toAPI(rows[0]))
}

// ---------------------------------------------------------------- tokens (admin)

func (s *Server) apiTokensPage(w http.ResponseWriter, r *http.Request) {
	s.renderAPITokens(w, r, "")
}

func (s *Server) renderAPITokens(w http.ResponseWriter, r *http.Request, newToken string) {
	rows, err := s.q.ListAPITokens(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.APITokensPage(rows, newToken, baseURL(r)))
}

func (s *Server) apiTokenCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" || len([]rune(name)) > 80 {
		s.flash(ctx, appctx.T(ctx, "اكتب اسماً للمفتاح (80 حرفاً كحد أقصى)"), "warn")
		http.Redirect(w, r, "/admin/api", http.StatusSeeOther)
		return
	}
	token, hash := auth.NewToken()
	id, err := s.q.CreateAPIToken(ctx, gen.CreateAPITokenParams{Name: name, TokenHash: hash, CreatedBy: currentUserID(ctx)})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "api_token", id, "created", map[string]any{"name": name})
	s.renderAPITokens(w, r, token)
}

func (s *Server) apiTokenRevoke(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := s.q.RevokeAPIToken(ctx, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(ctx, s.q, "api_token", id, "cancelled", nil)
	s.flash(ctx, appctx.T(ctx, "أُلغي المفتاح"), "")
	http.Redirect(w, r, "/admin/api", http.StatusSeeOther)
}
