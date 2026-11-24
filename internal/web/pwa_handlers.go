package web

import (
	"net/http"

	"github.com/HZamzami/kidana-quality/internal/views"
	webassets "github.com/HZamzami/kidana-quality/web"
)

// serviceWorker serves sw.js from the site root so it can control every page.
func (s *Server) serviceWorker(w http.ResponseWriter, r *http.Request) {
	b, err := webassets.Static.ReadFile("static/sw.js")
	if err != nil {
		s.notFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	_, _ = w.Write(b)
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request) {
	b, err := webassets.Static.ReadFile("static/manifest.webmanifest")
	if err != nil {
		s.notFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(b)
}

// offlinePage is shown by the service worker for pages not kept offline.
func (s *Server) offlinePage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, views.OfflinePage())
}
