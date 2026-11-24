package web

import (
	"context"
	"strconv"

	"github.com/HZamzami/kidana-quality/internal/domain"
	"github.com/HZamzami/kidana-quality/internal/views"
)

// lookupOptions loads the select options for sites, systems and contractors.
// activeOnly is used by forms; filters also show inactive values.
func (s *Server) lookupOptions(ctx context.Context, activeOnly bool) (sites, systems, contractors []views.Option, err error) {
	if activeOnly {
		ss, err := s.q.ListActiveSites(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, x := range ss {
			sites = append(sites, views.Option{Value: idStr(x.ID), Label: x.Name})
		}
		sy, err := s.q.ListActiveSystems(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, x := range sy {
			systems = append(systems, views.Option{Value: idStr(x.ID), Label: x.Name})
		}
		cs, err := s.q.ListActiveContractors(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, x := range cs {
			contractors = append(contractors, views.Option{Value: idStr(x.ID), Label: x.Name})
		}
		return sites, systems, contractors, nil
	}

	ss, err := s.q.ListSites(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, x := range ss {
		sites = append(sites, views.Option{Value: idStr(x.ID), Label: x.Name})
	}
	sy, err := s.q.ListSystems(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, x := range sy {
		systems = append(systems, views.Option{Value: idStr(x.ID), Label: x.Name})
	}
	cs, err := s.q.ListContractors(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, x := range cs {
		contractors = append(contractors, views.Option{Value: idStr(x.ID), Label: x.Name})
	}
	return sites, systems, contractors, nil
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
