package web

import (
	"context"

	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/domain"
)

// refreshWorkOrderStatus applies the acceptance rule after anything that can
// change it (inspection submitted/reopened, finding created/closed/reopened).
func (s *Server) refreshWorkOrderStatus(ctx context.Context, q *gen.Queries, woID int64) error {
	f, err := q.AcceptanceFacts(ctx, woID)
	if err != nil {
		return err
	}
	next := domain.NextWOStatus(domain.AcceptanceInput{
		Current:           domain.WOStatus(f.Status),
		HasSubmittedAfter: f.HasSubmittedAfter,
		LatestAfterPassed: f.LatestAfterPassed,
		OpenFindings:      int(f.OpenFindings),
		HasSubmittedWork:  f.HasSubmitted,
	})
	if string(next) == string(f.Status) {
		return nil
	}
	if err := q.SetWorkOrderStatus(ctx, gen.SetWorkOrderStatusParams{ID: woID, Status: gen.WoStatus(next)}); err != nil {
		return err
	}
	s.audit(ctx, q, "work_order", woID, "status_auto", map[string]any{"from": f.Status, "to": next})
	return nil
}
