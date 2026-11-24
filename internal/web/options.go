package web

import (
	"context"
	"strconv"

	"github.com/HZamzami/kidana-quality/internal/appctx"
	"github.com/HZamzami/kidana-quality/internal/i18n"

	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// lookupSet holds the select options for the admin-managed lists.
type lookupSet struct {
	Sites, Systems, AssetGroups, Contractors []views.Option
}

// lookupOptions loads the select options for sites, systems, asset groups and
// contractors. activeOnly is used by forms; filters also show inactive values.
func (s *Server) lookupOptions(ctx context.Context, activeOnly bool) (lookupSet, error) {
	var l lookupSet
	lang := appctx.Lang(ctx)
	opt := func(id int64, ar, en string) views.Option {
		return views.Option{Value: idStr(id), Label: i18n.Pick(lang, ar, en)}
	}
	sites, systems, groups, contractors := s.q.ListSites, s.q.ListSystems, s.q.ListAssetGroups, s.q.ListContractors
	if activeOnly {
		sites, systems, groups, contractors = s.q.ListActiveSites, s.q.ListActiveSystems, s.q.ListActiveAssetGroups, s.q.ListActiveContractors
	}
	ss, err := sites(ctx)
	if err != nil {
		return l, err
	}
	for _, x := range ss {
		l.Sites = append(l.Sites, opt(x.ID, x.Name, x.NameEn))
	}
	sy, err := systems(ctx)
	if err != nil {
		return l, err
	}
	for _, x := range sy {
		l.Systems = append(l.Systems, opt(x.ID, x.Name, x.NameEn))
	}
	ag, err := groups(ctx)
	if err != nil {
		return l, err
	}
	for _, x := range ag {
		l.AssetGroups = append(l.AssetGroups, opt(x.ID, x.Name, x.NameEn))
	}
	cs, err := contractors(ctx)
	if err != nil {
		return l, err
	}
	for _, x := range cs {
		l.Contractors = append(l.Contractors, opt(x.ID, x.Name, x.NameEn))
	}
	return l, nil
}

func priorityOptions() []views.Option {
	out := make([]views.Option, 0, len(domain.PriorityOrder))
	for _, p := range domain.PriorityOrder {
		out = append(out, views.Option{Value: p, Label: domain.PriorityLabels[p]})
	}
	return out
}

func woStatusOptions(includeAccepted bool) []views.Option {
	var out []views.Option
	for _, st := range []domain.WOStatus{domain.WOOpen, domain.WOInProgress, domain.WOCompleted, domain.WOAccepted, domain.WOCancelled} {
		if st == domain.WOAccepted && !includeAccepted {
			continue
		}
		out = append(out, views.Option{Value: string(st), Label: st.Label()})
	}
	return out
}

func idStr(id int64) string { return strconv.FormatInt(id, 10) }
