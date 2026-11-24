package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// search looks across work orders, assets and findings. An exact work order
// number goes straight to that work order.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	d := views.SearchData{Q: q}
	if q == "" {
		s.render(w, r, http.StatusOK, views.SearchPage(d))
		return
	}
	if wo, err := s.q.GetWorkOrderByRef(ctx, q); err == nil && canSeeContractor(ctx, wo.ContractorID) {
		http.Redirect(w, r, "/work-orders/"+idStr(wo.ID), http.StatusSeeOther)
		return
	}
	scope := appctx.ContractorScope(ctx)
	var err error
	if d.WorkOrders, err = s.q.ListWorkOrders(ctx, gen.ListWorkOrdersParams{Q: &q, ContractorID: scope, Lim: 20}); err != nil {
		s.serverError(w, r, err)
		return
	}
	if scope == nil {
		if d.Assets, err = s.q.ListAssets(ctx, gen.ListAssetsParams{Q: &q, Lim: 20}); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	fs, err := s.q.ListFindings(ctx, gen.ListFindingsParams{Q: &q, ContractorID: scope})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(fs) > 20 {
		fs = fs[:20]
	}
	d.Findings = fs
	s.render(w, r, http.StatusOK, views.SearchPage(d))
}

// savedViewPages maps a saved view's page to its path.
var savedViewPages = map[string]string{"work-orders": "/work-orders", "findings": "/findings", "dashboard": "/"}

// savedViewCreate saves the current filters of a list under a name.
func (s *Server) savedViewCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := newForm(r)
	page, name := f.Str("page"), f.Str("name")
	base, ok := savedViewPages[page]
	if !ok {
		s.badRequest(w, r, appctx.T(ctx, "قيمة غير صالحة"))
		return
	}
	q, _ := url.ParseQuery(f.Str("query"))
	q.Del("page")
	if name == "" || len([]rune(name)) > 60 {
		s.flash(ctx, appctx.T(ctx, "اكتب اسماً للعرض (60 حرفاً كحد أقصى)"), "warn")
		http.Redirect(w, r, base+"?"+q.Encode(), http.StatusSeeOther)
		return
	}
	shared := f.Bool("shared") && appctx.CanEdit(ctx)
	if err := s.q.CreateSavedView(ctx, gen.CreateSavedViewParams{UserID: appctx.User(ctx).ID, Page: page, Name: name, Query: q.Encode(), Shared: shared}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.flash(ctx, appctx.T(ctx, "تم حفظ العرض"), "ok")
	http.Redirect(w, r, base+"?"+q.Encode(), http.StatusSeeOther)
}

func (s *Server) savedViewDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		s.notFound(w, r)
		return
	}
	if _, err := s.q.DeleteSavedView(ctx, gen.DeleteSavedViewParams{ID: id, UserID: appctx.User(ctx).ID}); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, backTo(r), http.StatusSeeOther)
}

// savedViews loads the views for a page (the user's own and shared ones).
func (s *Server) savedViews(r *http.Request, page string, q url.Values) views.ViewsBar {
	rows, err := s.q.ListSavedViews(r.Context(), gen.ListSavedViewsParams{UserID: appctx.User(r.Context()).ID, Page: page})
	if err != nil {
		s.log.Warn("saved views", "err", err)
	}
	q2 := url.Values{}
	for k, v := range q {
		if k != "page" {
			q2[k] = v
		}
	}
	return views.ViewsBar{Page: page, Base: savedViewPages[page], Query: q2.Encode(), Views: rows, Me: appctx.User(r.Context()).ID}
}
