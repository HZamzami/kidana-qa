// Package web is the HTTP layer: routing, middleware and handlers.
package web

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/config"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/storage"
	"github.com/HZamzami/kidana-quality/internal/views"
	webassets "github.com/HZamzami/kidana-quality/web"
)

type Server struct {
	cfg     config.Config
	log     *slog.Logger
	pool    *pgxpool.Pool
	q       *gen.Queries
	sm      *scs.SessionManager
	store   storage.Store
	limiter *auth.LoginLimiter
	oidc    *ssoClient
}

func New(cfg config.Config, log *slog.Logger, pool *pgxpool.Pool, store storage.Store) *Server {
	sm := scs.New()
	sm.Store = pgxstore.New(pool)
	sm.Lifetime = cfg.SessionLifetime
	sm.IdleTimeout = 4 * time.Hour
	sm.Cookie.Name = "kidana_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = cfg.SessionSecure

	if cfg.OIDCEnabled() {
		views.SSOLabel = cfg.OIDCLabel
	}
	return &Server{
		cfg:     cfg,
		log:     log,
		pool:    pool,
		q:       gen.New(pool),
		sm:      sm,
		store:   store,
		limiter: auth.NewLoginLimiter(time.Minute, 5),
		oidc:    &ssoClient{},
	}
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	staticFS, _ := fs.Sub(webassets.Static, "static")
	static := http.StripPrefix("/static/", http.FileServerFS(staticFS))

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz)
	root.Handle("GET /static/", cacheStatic(static, s.cfg.IsDev()))
	root.HandleFunc("GET /sw.js", s.serviceWorker)
	root.HandleFunc("GET /manifest.webmanifest", s.manifest)
	// Work order API (bearer tokens, no session).
	root.HandleFunc("GET /api/v1/work-orders", s.requireAPIToken(s.apiListWorkOrders))
	root.HandleFunc("GET /api/v1/work-orders/{ref}", s.requireAPIToken(s.apiGetWorkOrder))
	root.HandleFunc("PUT /api/v1/work-orders/{ref}", s.requireAPIToken(s.apiPutWorkOrder))

	// Everything below uses sessions.
	app := http.NewServeMux()
	app.HandleFunc("GET /login", s.loginPage)
	app.HandleFunc("POST /login", s.loginSubmit)
	app.HandleFunc("POST /lang", s.setLang)
	app.HandleFunc("GET /offline", s.offlinePage)
	app.HandleFunc("GET /login/2fa", s.twoFactorPage)
	app.HandleFunc("POST /login/2fa", s.twoFactorSubmit)
	app.HandleFunc("GET /forgot", s.forgotPage)
	app.HandleFunc("POST /forgot", s.forgotSubmit)
	app.HandleFunc("GET /reset/{token}", s.resetPage)
	app.HandleFunc("POST /reset/{token}", s.resetSubmit)
	app.HandleFunc("GET /auth/oidc", s.oidcStart)
	app.HandleFunc("GET /auth/oidc/callback", s.oidcCallback)

	authed := http.NewServeMux()
	s.routes(authed)
	app.Handle("/", s.requireUser(authed))

	root.Handle("/", s.sm.LoadAndSave(s.loadUser(app)))

	// CSRF: reject cross-origin unsafe requests using Sec-Fetch-Site / Origin.
	csrf := http.NewCrossOriginProtection()

	var h http.Handler = root
	h = csrf.Handler(h)
	h = securityHeaders(h)
	h = s.recoverPanics(h)
	h = s.logRequests(h)
	return h
}

func (s *Server) routes(mux *http.ServeMux) {
	edit := s.requireRole(gen.UserRoleAdmin, gen.UserRoleInspector)
	staff := s.requireRole(gen.UserRoleAdmin, gen.UserRoleInspector, gen.UserRoleViewer) // not contractors
	contractor := s.requireRole(gen.UserRoleContractor)
	admin := s.requireRole(gen.UserRoleAdmin)

	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /account/password", s.passwordPage)
	mux.HandleFunc("POST /account/password", s.passwordSubmit)
	mux.HandleFunc("GET /account/security", s.securityPage)
	mux.HandleFunc("POST /account/2fa/start", s.twoFAStart)
	mux.HandleFunc("POST /account/2fa/confirm", s.twoFAConfirm)
	mux.HandleFunc("POST /account/2fa/disable", s.twoFADisable)
	mux.HandleFunc("POST /account/2fa/recovery", s.twoFARecovery)

	// dashboard
	mux.HandleFunc("GET /{$}", s.dashboard)

	// work orders
	mux.HandleFunc("GET /work-orders", s.workOrdersList)
	mux.HandleFunc("GET /work-orders/new", edit(s.workOrderNew))
	mux.HandleFunc("POST /work-orders", edit(s.workOrderCreate))
	mux.HandleFunc("GET /work-orders/print", s.workOrdersPrint)
	mux.HandleFunc("GET /work-orders/{id}", s.workOrderShow)
	mux.HandleFunc("GET /work-orders/{id}/print", s.workOrderPrint)
	mux.HandleFunc("GET /work-orders/{id}/edit", edit(s.workOrderEdit))
	mux.HandleFunc("POST /work-orders/{id}", edit(s.workOrderUpdate))
	mux.HandleFunc("POST /work-orders/{id}/inspections", edit(s.inspectionStart))
	mux.HandleFunc("POST /work-orders/{id}/delete", admin(s.workOrderDelete))
	mux.HandleFunc("POST /work-orders/{id}/assignments", edit(s.assignmentCreate))

	// search and saved views
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("POST /views", s.savedViewCreate)
	mux.HandleFunc("POST /views/{id}/delete", s.savedViewDelete)

	// contractor performance
	mux.HandleFunc("GET /contractors", s.contractorsPage)
	mux.HandleFunc("GET /contractors/{id}", s.contractorPage)
	mux.HandleFunc("GET /reports/contractor/{id}", s.contractorReport)

	// assets (QR labels open /a/{code})
	mux.HandleFunc("GET /assets", s.assetsList)
	mux.HandleFunc("GET /assets/labels", staff(s.assetLabels))
	mux.HandleFunc("GET /assets/{id}", s.assetShow)
	mux.HandleFunc("POST /assets/{id}", edit(s.assetUpdate))
	mux.HandleFunc("GET /assets/{id}/qr.png", s.assetQR)
	mux.HandleFunc("GET /a/{code}", s.assetByCode)

	// tasks, assignments and notifications
	mux.HandleFunc("GET /my/tasks", s.tasksPage)
	mux.HandleFunc("POST /assignments/{id}/claim", edit(s.assignmentClaim))
	mux.HandleFunc("POST /assignments/{id}", edit(s.assignmentUpdate))
	mux.HandleFunc("POST /assignments/{id}/cancel", edit(s.assignmentCancel))
	mux.HandleFunc("GET /notifications", s.notificationsPage)
	mux.HandleFunc("GET /notifications/bell", s.notificationsBell)
	mux.HandleFunc("POST /notifications/{id}/open", s.notificationOpen)
	mux.HandleFunc("POST /notifications/read-all", s.notificationsReadAll)

	// inspections
	mux.HandleFunc("GET /inspections/{id}", s.inspectionShow)
	mux.HandleFunc("GET /inspections/{id}/review", s.inspectionReview)
	mux.HandleFunc("POST /inspections/{id}/items/{cid}", edit(s.inspectionItemSave))
	mux.HandleFunc("POST /inspections/{id}/header", edit(s.inspectionHeaderSave))
	mux.HandleFunc("POST /inspections/{id}/findings", edit(s.inspectionFindingCreate))
	mux.HandleFunc("POST /inspections/{id}/photos", edit(s.inspectionPhotoUpload))
	mux.HandleFunc("POST /inspections/{id}/location", edit(s.inspectionLocation))
	mux.HandleFunc("GET /attachments/{id}/annotate", edit(s.annotatePage))
	mux.HandleFunc("POST /attachments/{id}/annotate", edit(s.annotateSave))
	mux.HandleFunc("POST /inspections/{id}/submit", edit(s.inspectionSubmit))
	mux.HandleFunc("POST /inspections/{id}/delete", edit(s.inspectionDelete))
	mux.HandleFunc("POST /inspections/{id}/reopen", admin(s.inspectionReopen))

	// findings
	mux.HandleFunc("GET /findings", s.findingsList)
	mux.HandleFunc("GET /findings/export.xlsx", s.findingsExport)
	mux.HandleFunc("GET /findings/print", s.findingsPrint)
	mux.HandleFunc("POST /findings/bulk", edit(s.findingsBulk))
	mux.HandleFunc("POST /findings/{id}/respond", contractor(s.findingRespond))
	mux.HandleFunc("POST /findings/{id}/reject", edit(s.findingReject))
	mux.HandleFunc("POST /findings/{id}/photos", edit(s.findingPhotoUpload))
	mux.HandleFunc("GET /findings/{id}/edit", s.findingEdit)
	mux.HandleFunc("POST /findings/{id}", edit(s.findingUpdate))
	mux.HandleFunc("POST /findings/{id}/close", edit(s.findingClose))
	mux.HandleFunc("POST /findings/{id}/reopen", edit(s.findingReopen))
	mux.HandleFunc("POST /findings/{id}/delete", edit(s.findingDelete))

	// attachments
	mux.HandleFunc("GET /attachments/{id}", s.attachmentGet)
	mux.HandleFunc("POST /attachments/{id}/delete", edit(s.attachmentDelete))

	// reports & data
	mux.HandleFunc("GET /reports/print", staff(s.reportPrint))
	mux.HandleFunc("GET /reports/export.xlsx", staff(s.exportExcel))
	mux.HandleFunc("GET /data", staff(s.dataPage))
	mux.HandleFunc("GET /data/work-orders-template.xlsx", edit(s.workOrderTemplate))
	mux.HandleFunc("POST /data/import", edit(s.importExcel))

	// admin
	mux.HandleFunc("GET /admin", admin(s.adminHome))
	mux.HandleFunc("GET /admin/lookups/{kind}", admin(s.lookupList))
	mux.HandleFunc("POST /admin/lookups/{kind}", admin(s.lookupCreate))
	mux.HandleFunc("POST /admin/lookups/{kind}/{id}", admin(s.lookupUpdate))
	mux.HandleFunc("GET /admin/users", admin(s.usersList))
	mux.HandleFunc("POST /admin/users", admin(s.userCreate))
	mux.HandleFunc("POST /admin/users/{id}", admin(s.userUpdate))
	mux.HandleFunc("POST /admin/users/{id}/password", admin(s.userResetPassword))
	mux.HandleFunc("POST /admin/users/{id}/reset-link", admin(s.adminResetLink))
	mux.HandleFunc("POST /admin/users/{id}/2fa-reset", admin(s.adminReset2FA))
	mux.HandleFunc("GET /admin/audit", admin(s.auditPage))
	mux.HandleFunc("GET /admin/api", admin(s.apiTokensPage))
	mux.HandleFunc("POST /admin/api", admin(s.apiTokenCreate))
	mux.HandleFunc("POST /admin/api/{id}/revoke", admin(s.apiTokenRevoke))
	mux.HandleFunc("GET /admin/sla", admin(s.slaPage))
	mux.HandleFunc("POST /admin/sla", admin(s.slaSave))
	mux.HandleFunc("GET /admin/sampling", admin(s.samplingPage))
	mux.HandleFunc("POST /admin/sampling", admin(s.samplingCreate))
	mux.HandleFunc("POST /admin/sampling/{id}", admin(s.samplingUpdate))
	mux.HandleFunc("GET /admin/checklists", admin(s.checklistsList))
	mux.HandleFunc("GET /admin/checklists/{id}", admin(s.checklistShow))
	mux.HandleFunc("GET /admin/checklists/{id}/new-version", admin(s.checklistNewVersion))
	mux.HandleFunc("POST /admin/checklists", admin(s.checklistCreate))

	mux.HandleFunc("/", s.notFound)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.pool.Ping(r.Context()); err != nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func cacheStatic(next http.Handler, dev bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dev {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		next.ServeHTTP(w, r)
	})
}
