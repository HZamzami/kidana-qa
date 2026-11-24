// Command server runs the Kidana Quality web application.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HZamzami/kidana-quality/internal/auth"
	"github.com/HZamzami/kidana-quality/internal/config"
	"github.com/HZamzami/kidana-quality/internal/db"
	"github.com/HZamzami/kidana-quality/internal/db/gen"
	"github.com/HZamzami/kidana-quality/internal/storage"
	"github.com/HZamzami/kidana-quality/internal/views"
	"github.com/HZamzami/kidana-quality/internal/web"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("fatal", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.IsDev() {
		log = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	views.AssetVersion = version
	if version == "dev" {
		views.AssetVersion = time.Now().Format("150405")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	if err := bootstrapAdmin(ctx, log, pool, cfg); err != nil {
		return err
	}

	store, err := storage.NewDisk(cfg.UploadDir, cfg.MaxUploadBytes)
	if err != nil {
		return err
	}

	app := web.New(cfg, log, pool, store)
	go app.RunScheduler(ctx) // reminders and scheduled reports

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second, // photo uploads on slow site networks
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", cfg.HTTPAddr), slog.String("env", cfg.Env), slog.String("version", version))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// bootstrapAdmin creates the first admin from ADMIN_EMAIL / ADMIN_PASSWORD
// when the users table is empty.
func bootstrapAdmin(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, cfg config.Config) error {
	q := gen.New(pool)
	n, err := q.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		log.Warn("no users exist; set ADMIN_EMAIL and ADMIN_PASSWORD to create the first admin")
		return nil
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	if _, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email: cfg.AdminEmail, Name: cfg.AdminName, Role: gen.UserRoleAdmin, PasswordHash: hash,
	}); err != nil {
		return err
	}
	log.Info("created first admin user", slog.String("email", cfg.AdminEmail))
	return nil
}
