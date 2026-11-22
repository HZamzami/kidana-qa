// Package config reads settings from the environment (12-factor style).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env             string // development | production
	HTTPAddr        string
	DatabaseURL     string
	UploadDir       string
	MaxUploadBytes  int64
	SessionSecure   bool
	SessionLifetime time.Duration

	// First-run admin, created only when there are no users.
	AdminEmail    string
	AdminName     string
	AdminPassword string
}

func (c Config) IsDev() bool { return c.Env != "production" }

func Load() (Config, error) {
	c := Config{
		Env:           get("APP_ENV", "development"),
		HTTPAddr:      get("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		UploadDir:     get("UPLOAD_DIR", "./data/uploads"),
		AdminEmail:    os.Getenv("ADMIN_EMAIL"),
		AdminName:     get("ADMIN_NAME", "مدير النظام"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	mb, err := strconv.Atoi(get("MAX_UPLOAD_MB", "10"))
	if err != nil || mb <= 0 {
		errs = append(errs, fmt.Errorf("MAX_UPLOAD_MB: invalid value"))
	}
	c.MaxUploadBytes = int64(mb) << 20

	c.SessionSecure, err = strconv.ParseBool(get("SESSION_SECURE", strconv.FormatBool(c.Env == "production")))
	if err != nil {
		errs = append(errs, fmt.Errorf("SESSION_SECURE: %w", err))
	}
	c.SessionLifetime, err = time.ParseDuration(get("SESSION_LIFETIME", "12h"))
	if err != nil {
		errs = append(errs, fmt.Errorf("SESSION_LIFETIME: %w", err))
	}
	if c.Env == "production" && c.AdminPassword == "change-me-now" {
		errs = append(errs, errors.New("ADMIN_PASSWORD must be changed from the example value in production"))
	}
	return c, errors.Join(errs...)
}

func get(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
