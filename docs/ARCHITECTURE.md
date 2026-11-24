# Architecture

A server-rendered Go application. HTML comes from templ components; HTMX swaps
fragments for interactions (filters, autosave, uploads); Alpine handles purely
local UI state (inspection stepper, live score preview, menus, signature pad,
photo drawing, offline queue). There is no SPA. The only JSON is the small
token-protected work order API under `/api/v1` for CMMS sync.

```
cmd/server/            main: config → db pool → migrations → first admin → scheduler + HTTP server
cmd/seed/              dummy data for development (`just seed`), never in production
internal/
  config/              environment variables (12-factor), incl. optional OIDC
  i18n/                UI language: Arabic source strings → English catalog (gettext style)
  domain/              business rules, no I/O: scoring, acceptance, verdict, finding workflow,
                       sampling, SLA, trends, scorecards, diffs, insights, labels
  db/
    migrations/        goose SQL migrations (embedded, run at startup)
    queries/           SQL for sqlc
    gen/               sqlc output (generated, committed)
  dates/               dd/mm/yyyy + Umm al-Qura Hijri formatting (display only)
  auth/                argon2id passwords, login rate limiter, TOTP, recovery codes, tokens
  storage/             photo storage interface + local disk implementation
  excel/               export workbook, work order import template and parsing
  appctx/              request context values shared by web and views (user, flash, path)
  views/               templ components (pages, partials, components)
  web/                 routes, middleware, handlers
web/
  styles/input.css     Tailwind v4 source with Kidana brand tokens (@theme)
  static/              fonts, logos, icons, vendored htmx + Alpine (CSP build), app.js,
                       sw.js (service worker), manifest.webmanifest, built app.css
```

## Domain model

```
work_orders (site, system = المنظومة, asset_group = مجموعة الأصل, asset_id → assets, contractor,
             priority normal|high|emergency, reported_on, planned_date = due date)
            1──* inspections (stage during|after, attempt n, template version, lat/lng) 1──* inspection_items
            1──* findings (status open|in_progress|responded|closed, root cause, owner_user, repeat_of)
            1──* inspection_assignments (stage, assignee or pool, reason manual|sampling|reinspection)
inspections / findings 1──* attachments (kind photo|signature, lat/lng, annotated_of)
checklist_templates (family_id, version within family, active, optional asset group) 1──* template_criteria
            *──* systems via checklist_template_systems (scope; none = general)
users (role admin|inspector|viewer|contractor, contractor_id, lang, totp, session_version)
notifications · sampling_rules · sla_targets · saved_views · report_runs · settings · api_tokens · audit_log
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
* Every change is written to `audit_log`; edits record the fields that
  changed (`domain.Diff`) for the history screens.
* **Language**: middleware puts the language in the request context (cookie,
  then the user's choice, then Arabic). Templates wrap every string in
  `t(ctx, "…")`; `TestCatalogComplete` fails when an Arabic string in the code
  has no English entry in `internal/i18n/catalog_en.go`.
* **Contractor scoping**: `appctx.ContractorScope` limits every list, detail,
  print and photo to the contractor user's own contractor (others → 404).
* **Scheduler** (`web.RunScheduler`, hourly): due-soon and overdue reminders
  and the monthly report run; dedupe keys make reruns safe.
* **Offline**: `sw.js` keeps visited inspection/work order/task pages; `app.js`
  queues failed inspection saves in IndexedDB and replays them in order.
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
* `internal/web/*_test.go`: integration tests over real HTTP and Postgres (`TEST_DATABASE_URL`): auth, role guards, CSRF, the during → after → acceptance flow, language switch, 2FA and reset links, the contractor portal, field work (signatures, location, drawing), insights, the API and scoped checklists.

## Future extensions (designed for, not built)

* Email or WhatsApp notifications: add a sender next to `s.notify` (notifications already store a kind and parameters per reader).
* S3/MinIO photo storage: implement `storage.Store`.
* Server-side PDF via a Gotenberg container that renders `/reports/print` or `/work-orders/print` (for emailing records without a browser).
* A combined work-order score (during + after weighting) once the business defines it.
