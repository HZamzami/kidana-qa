# Architecture

A server-rendered Go application. HTML comes from templ components; HTMX swaps
fragments for interactions (filters, autosave, uploads); Alpine handles purely
local UI state (inspection stepper, live score preview, menus). There is no SPA
and no JSON API.

```
cmd/server/            main: config → db pool → migrations → first admin → HTTP server
cmd/seed/              dummy data for development (`just seed`), never in production
internal/
  config/              environment variables (12-factor)
  domain/              business rules, no I/O: scoring, acceptance rule, submit validation,
                       dashboard aggregation and insights, Arabic labels
  db/
    migrations/        goose SQL migrations (embedded, run at startup)
    queries/           SQL for sqlc
    gen/               sqlc output (generated, committed)
  dates/               dd/mm/yyyy + Umm al-Qura Hijri formatting (display only)
  auth/                argon2id passwords, login rate limiter
  storage/             photo storage interface + local disk implementation
  excel/               export workbook, work order import template and parsing
  appctx/              request context values shared by web and views (user, flash, path)
  views/               templ components (pages, partials, components)
  web/                 routes, middleware, handlers
web/
  styles/input.css     Tailwind v4 source with Kidana brand tokens (@theme)
  static/              fonts, logos, vendored htmx + Alpine (CSP build), app.js, built app.css
```

## Domain model

```
work_orders (site, system = المنظومة, asset_group = مجموعة الأصل, contractor, priority normal|high|emergency)
            1──* inspections (stage during|after, attempt n, template version) 1──* inspection_items (rating, note)
            1──* findings (description, corrective action, owner, due date, status)
inspections 1──* attachments (photos, optionally tied to a criterion)
checklist_templates (stage, version, active) 1──* template_criteria (code, weight, is_critical)
```

* **The during-work inspection is optional and can repeat** (sampling; several
  visits on long jobs). **The after-work inspection can repeat** (attempt 2, 3, …
  after a failure).
* Sites, systems (المنظومات), asset groups (مجموعات الأصول) and contractors are
  independent admin-managed lookup tables. Excel import creates missing names.
* `work_orders.status` is maintained by `refreshWorkOrderStatus` (web/service.go)
  using `domain.NextWOStatus`. It runs after every change that can affect
  acceptance: inspection submitted/re-opened/deleted, finding
  created/closed/re-opened/deleted, work order edited.
* Scores are computed by `domain.Evaluate` and stored on submit. The client's
  live score in `app.js` mirrors the formula for preview only.
* Every change is written to `audit_log`.
* **Printing**: `/work-orders/{id}/print` and `/work-orders/print?<list filters>`
  render A4 records (`views/print.templ`, `web/print_handlers.go`). The list and
  the filtered print share `woFilter`, so a print always matches the screen. Data
  loads in four batch queries; `domain.WOVerdict` gives the overall result.
  The browser's print-to-PDF keeps Arabic shaping and the brand font, with no
  server-side PDF engine.

## Conventions

* **SQL first**: add a query to `internal/db/queries/*.sql`, then run `just gen`. Don't hand-write SQL in Go.
* **Schema changes**: run `just migration name`, then write Up and Down. Never edit a migration that has shipped.
* **Business rules** belong in `internal/domain` with table-driven tests. Handlers stay thin.
* **Arabic labels** for enum values live in `domain/labels.go` so the UI, Excel and reports agree.
* **Forms**: validate on the server and re-render with `422` and field errors. htmx swaps 422 responses (see `app.js`).
* **Alpine** runs in its CSP build, which has no `eval`. Register components with `Alpine.data()` in `web/static/js/app.js` and keep template expressions simple.
* **Security**: CSP blocks inline scripts. Go's `CrossOriginProtection` handles CSRF. Sessions are stored in Postgres (scs) with the token renewed on login. Uploads are type-sniffed and size-limited.
* **Brand**: use only the Kidana tokens from `input.css`. Per the brand rule, use lighter tints only, never darker shades. `fail` (muted brick) is the single colour added for semantics.

## Testing

* `internal/domain`, `internal/dates`, `internal/excel`, `internal/auth`, `internal/storage`: pure unit tests.
* `just ui-check`: a Playwright walkthrough of every screen and role (`scripts/ui_check.py`).
* `internal/web/web_test.go`: integration tests over real HTTP and Postgres (`TEST_DATABASE_URL`). They cover auth, role guards, CSRF and the full during → after → acceptance flow.

## Future extensions (designed for, not built)

* Per-system or per-asset-group checklists: add `system_id` / `asset_group_id` to `checklist_templates` and pick the template by it + stage.
* SSO (Microsoft Entra ID) via OIDC alongside local accounts.
* S3/MinIO photo storage: implement `storage.Store`.
* Server-side PDF via a Gotenberg container that renders `/reports/print` or `/work-orders/print` (for emailing records without a browser).
* A combined work-order score (during + after weighting) once the business defines it.
