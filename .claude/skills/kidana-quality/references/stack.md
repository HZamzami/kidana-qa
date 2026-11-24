# Stack, environment and gotchas

## Versions (pinned)

| What | Version | Where pinned |
|---|---|---|
| Go | 1.27.4 | `go.mod`, `Dockerfile` `GO_VERSION` |
| templ | v0.3.1118 | `go.mod`, `Dockerfile`, CI |
| sqlc | 1.31.1 | `Dockerfile`, CI |
| goose | v3.30.1 | `go.mod`, `Dockerfile` |
| air | v1.67.4 | `Dockerfile` |
| Tailwind (standalone CLI, v4) | v4.3.5 | `Dockerfile` |
| golangci-lint | 2.15.0 | `Dockerfile`, CI |
| PostgreSQL | 18.7-alpine | `compose*.yaml`, CI |
| htmx | 2.0.11 | vendored `web/static/js/htmx.min.js` |
| Alpine (CSP build) | 3.17.5 | vendored `web/static/js/alpine-csp.min.js` |
| pgx v5, scs v2 (+pgxstore), excelize v2, x/crypto (argon2id), go-hijri v1.0.2 (Umm al-Qura) | see `go.mod` | |
| Playwright (UI check) | python v1.55.0-noble image + pip `playwright==1.55.0`, `openpyxl==3.1.5` | `Justfile` `ui-check` |

The JS libraries are vendored, not loaded from a CDN, because site networks may be offline and the CSP is `script-src 'self'`. To upgrade one, download the exact version into `web/static/js/` and update this table.

## Dev loop

- The host is Bluefin-dx (immutable Fedora) with Docker and `just`. **No Go or Node on the host.**
- `just up` builds the `dev` target and runs Postgres plus the app. air rebuilds on `.go/.templ/.sql/.css/.js` changes (its build step runs `templ generate`, then Tailwind, then `go build`).
- `just gen | test | lint | fmt | psql | shell | migration <name> | migrate-status | image | nuke`.
- Logs: `just logs`. The app logs `slog` text in dev and JSON in production.
- The first admin comes from `.env` (`ADMIN_EMAIL` / `ADMIN_PASSWORD`) and is created only when the users table is empty.

### Gotchas

- **SELinux**: bind mounts need `:z` (already set in compose). Without it you get "permission denied" inside the container.
- **File ownership**: containers run as `${HOST_UID}:${HOST_GID}`, which the Justfile exports. Calling `docker compose` directly without them falls back to 1000:1000.
- **Go caches**: `GOCACHE=/go-cache`, `GOMODCACHE=/go-mod`, `GOPATH=/go-mod/gopath`, all on named volumes made world-writable in the image. If you see "permission denied" on `/go/pkg/sumdb`, GOPATH isn't set.
- **Postgres 18 volume path** is `/var/lib/postgresql`, not `/var/lib/postgresql/data`.
- **Test DB**: `kidana_quality_test` is created by `docker/postgres/init/01-test-db.sql`, but only when the volume is first created. After `just nuke` it comes back automatically.
- **Templ + Alpine**: write `x-on:click` / `x-bind:class`, not `@click` / `:class`. Conditional boolean attributes use `checked?={ cond }`.
- **Alpine CSP**: the expression parser handles simple expressions and object literals, but no arbitrary JS. If something doesn't react, move it into a getter.
- **excelize pictures need `_ "image/png"`** registered (done in `excel/export.go`), otherwise `AddPicture` fails with "image: unknown format".
- **HTTP headers are Latin-1**: any Arabic in `HX-Trigger` etc. must be `\uXXXX`-escaped (`asciiJSON` in `web/helpers.go`, covered by a test).
- **`templ fmt` can crash** on this codebase (templ v0.3.1118 imports bug). `templ generate` is what matters; format Go with `gofmt`.
- **htmx config** is set in a `<meta name="htmx-config">`: `includeIndicatorStyles:false` (inline styles), `allowEval:false`, `allowScriptTags:false`.

## Project layout

See `docs/ARCHITECTURE.md`. Key entry points:
- `cmd/server/main.go`: wiring, migrations, bootstrap admin, graceful shutdown.
- `internal/web/server.go`: middleware chain and **all routes**.
- `internal/web/helpers.go`: `render`, `redirect`, `flash`, `toast`, error responses, the `form` parsing helper, `audit`.
- `internal/web/service.go`: `refreshWorkOrderStatus`.

## Security model

- Sessions use scs in Postgres (`sessions` table): HttpOnly, SameSite=Lax, Secure in production. The token is renewed on login and on password change.
- Passwords are argon2id (OWASP parameters). Login is rate-limited per IP+email (5 per minute), and unknown users take the same time as known ones (dummy hash).
- CSRF is handled by `http.CrossOriginProtection` (Sec-Fetch-Site/Origin). Plain same-origin forms and htmx need no tokens.
- CSP: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'` (inline styles are only for computed widths), plus `frame-ancestors 'none'`, nosniff and a strict Referrer-Policy.
- Authorisation: `requireUser` wraps every app route, and `requireRole` guards edit and admin routes. Views hide controls with `appctx.CanEdit` / `IsAdmin`, but the server is always the authority.
- Uploads: the type is sniffed (jpeg/png/webp/gif only), size is limited by `MAX_UPLOAD_MB`, files get random keys, path traversal is blocked, and they're served only to logged-in users.
- Everything is audit-logged.

## Production

- `docker compose -f compose.prod.yaml --env-file .env.prod up -d --build` runs a distroless nonroot image with embedded assets and migrations applied on start.
- A nightly `pg_dump` goes to `./backups` with 14 days kept. Back up the `uploads` volume too.
- Put a TLS reverse proxy in front and set `SESSION_SECURE=true`. If you add one, extend `clientIP` to trust X-Forwarded-For from that proxy only.
