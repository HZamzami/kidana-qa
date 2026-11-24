package web

import (
	"context"

	"github.com/HZamzami/kidana-quality/internal/db/gen"
)

// onPasswordResetRequested tells admins a user asked for a reset link.
func (s *Server) onPasswordResetRequested(ctx context.Context, u gen.User) {
	_ = ctx
	_ = u
}
