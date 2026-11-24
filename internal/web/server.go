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

	return &Server{
		cfg:     cfg,
		log:     log,
		pool:    pool,
		q:       gen.New(pool),
		sm:      sm,
		store:   store,
		limiter: auth.NewLoginLimiter(time.Minute, 5),
	}
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	staticFS, _ := fs.Sub(webassets.Static, "static")
	static := http.StripPrefix("/static/", http.FileServerFS(staticFS))

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz)
	root.Handle("GET /static/", cacheStatic(static, s.cfg.IsDev()))

	// Everything below uses sessions.
	app := http.NewServeMux()
	app.HandleFunc("GET /login", s.loginPage)
	app.HandleFunc("POST /login", s.loginSubmit)
	app.HandleFunc("POST /lang", s.setLang)

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
	admin := s.requireRole(gen.UserRoleAdmin)

	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /account/password", s.passwordPage)
	mux.HandleFunc("POST /account/password", s.passwordSubmit)

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

	// inspections
	mux.HandleFunc("GET /inspections/{id}", s.inspectionShow)
	mux.HandleFunc("GET /inspections/{id}/review", s.inspectionReview)
	mux.HandleFunc("POST /inspections/{id}/items/{cid}", edit(s.inspectionItemSave))
	mux.HandleFunc("POST /inspections/{id}/header", edit(s.inspectionHeaderSave))
	mux.HandleFunc("POST /inspections/{id}/findings", edit(s.inspectionFindingCreate))
	mux.HandleFunc("POST /inspections/{id}/photos", edit(s.inspectionPhotoUpload))
	mux.HandleFunc("POST /inspections/{id}/submit", edit(s.inspectionSubmit))
	mux.HandleFunc("POST /inspections/{id}/delete", edit(s.inspectionDelete))
	mux.HandleFunc("POST /inspections/{id}/reopen", admin(s.inspectionReopen))

	// findings
	mux.HandleFunc("GET /findings", s.findingsList)
	mux.HandleFunc("GET /findings/{id}/edit", edit(s.findingEdit))
	mux.HandleFunc("POST /findings/{id}", edit(s.findingUpdate))
	mux.HandleFunc("POST /findings/{id}/close", edit(s.findingClose))
	mux.HandleFunc("POST /findings/{id}/reopen", edit(s.findingReopen))
	mux.HandleFunc("POST /findings/{id}/delete", edit(s.findingDelete))

	// attachments
	mux.HandleFunc("GET /attachments/{id}", s.attachmentGet)
	mux.HandleFunc("POST /attachments/{id}/delete", edit(s.attachmentDelete))

	// reports & data
	mux.HandleFunc("GET /reports/print", s.reportPrint)
	mux.HandleFunc("GET /reports/export.xlsx", s.exportExcel)
	mux.HandleFunc("GET /data", s.dataPage)
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
