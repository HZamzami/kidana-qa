# Kidana Quality — نظام جودة الصيانة

Quality system for Kidana's maintenance work orders. Every work order can be
inspected at two stages:

| Stage | Arabic | When | Checks |
|---|---|---|---|
| **During the work** | فحص أثناء التنفيذ | A Kidana inspector goes with the contractor **while the work is done** | Safety and permits (PPE, LOTO, PTW, risk assessment, **critical**), the approved procedure, approved materials, technician competence and calibrated instruments, protection of the site and environment, arrival/response time |
| **After the work** | فحص بعد التنفيذ | **After** the work is finished | Functional test after maintenance (**critical**), safe return to service (**critical**), checklist and readings, workmanship, site cleanliness, documentation and WO closed in the CMMS |

The business never calls these "QA"/"QC"; use the Arabic names above (short
form: «أثناء التنفيذ» / «بعد التنفيذ»).

Non-conformities become **findings** (ملاحظات) with a corrective action, owner
and due date. A work order is **accepted** automatically when its latest
after-work inspection passes and it has no open findings.

Each work order records a **المنظومة** (e.g. منشأة الجمرات، أبراج كدانة) and a
**مجموعة الأصل** (e.g. المبردات، المضخات). Both are admin-managed lists, as are
sites and contractors. Priority is عادي، عالي or طارئ.

The UI is **Arabic by default with a full English version** (toggle in the
header; pages switch to left-to-right). Lists and checklist items have
optional English names.

## What it does

* **Work orders** with المنظومة, مجموعة الأصل, asset, contractor, priority and a
  report date; the due date defaults from the priority's resolution target.
* **Inspections** on the phone, one item per card, with photos (location and
  time stamped, drawing on a copy), signatures, and **offline** use: saves made
  without a connection are kept on the phone and sent when it returns. The app
  is installable (PWA).
* **Findings** with corrective action, root cause, owner, evidence photos and
  repeat detection. **Contractor accounts** respond with a note and photo; the
  inspector verifies or sends it back.
* **«مهامي»**, **assignments** (manual, sampled by rules, automatic
  re-inspection after a failure) and **in-app notifications** with due-soon and
  overdue reminders.
* **Dashboard** with KPIs, criteria, monthly trend and insights; **contractor
  scorecards** with a weighted composite score and A4 reports; **change
  history** per work order and an admin audit log; search and saved views.
* **Asset register** with QR labels that open the asset's history.
* **PDF records** per work order (singly or by filter), findings lists,
  monthly reports, Excel export and import, and a **JSON API** for CMMS sync.
* **Security**: optional two-step verification, admin-issued one-time reset
  links, Microsoft (OIDC) sign-in when configured.

Stack: **Go** · **templ** · **HTMX** · **Alpine.js** (CSP build) · **Tailwind v4** · **PostgreSQL 18** · Docker.

---

## Quick start

Only **Docker** (with Compose v2) and **just** are needed on the host. Go, Node
and Postgres are not. This works on Fedora Atomic / Bluefin, macOS and Linux.

```sh
cp .env.example .env      # first-run admin account lives here
just up                   # builds the dev image, starts Postgres + the app
```

Open <http://localhost:8080> and log in with `ADMIN_EMAIL` / `ADMIN_PASSWORD`
from `.env` (default `admin@kidana.local` / `change-me-now`). Change the
password from the user menu.

For something to look at, `just seed` fills the dev database with about 150
realistic dummy work orders, inspections and findings (`just seed -reset`
replaces them). Demo inspectors log in as `insp1@demo.kidana.local` …
`insp4@…`, and contractor users as `contractor1@demo.kidana.local` …
`contractor6@…`, all with the password `demo-password`. The seeder refuses to
run in production.

The app hot-reloads: editing `.go`, `.templ`, `.sql` or `.css` files rebuilds
and restarts it (air). Refresh the browser to see changes.

### Everyday commands

| Command | What it does |
|---|---|
| `just up` / `just down` | Start / stop the stack (data is kept) |
| `just logs` | Follow app logs |
| `just test` | Unit + integration tests (uses the `kidana_quality_test` database) |
| `just lint` | golangci-lint, after regenerating code |
| `just gen` | Regenerate templ components and sqlc queries |
| `just seed` | Fill the dev database with dummy data (`-reset` wipes it first) |
| `just migration add_thing` | New SQL migration in `internal/db/migrations` |
| `just psql` | psql into the dev database |
| `just shell` | Shell inside the dev container |
| `just ui-check` | Playwright walkthrough of the whole UI (phone + desktop, all roles), with screenshots in `tmp/ui-check/` |
| `just image` | Build the production image |
| `just nuke` | Delete the stack **and all its data** |

Prefer an IDE? `.devcontainer/devcontainer.json` opens the same container in
VS Code or JetBrains with Go, templ, Tailwind and htmx extensions.

## Roles

| Role | Can |
|---|---|
| مدير النظام (admin) | Everything, plus settings: checklists, sites, المنظومات, مجموعات الأصول, contractors, users; re-open submitted inspections |
| مدقق (inspector) | Create work orders, run during/after inspections, record, verify and close findings, assign and claim inspections, import/export |
| مشاهد (viewer) | Read-only |
| مقاول (contractor) | Only their own contractor's work orders, inspections and findings: respond to findings with photos, see their scorecard |

## Scoring

```
score = Σ(weight × factor) ÷ Σ(weight of applicable criteria) × 100
factor: مطابق 1 · مطابق جزئياً 0.5 · غير مطابق 0 · لا ينطبق excluded
```

An inspection **passes** at ≥ 80% **and** when no *critical* criterion (safety
during the work; the functional test and safe return to service after it) is
rated غير مطابق. Grades: ممتاز ≥ 90, جيد جداً ≥ 80, جيد ≥ 70,
مقبول ≥ 60, ضعيف below.

Checklists are **versioned**. Editing one creates a new version, so old
inspections keep the criteria and weights they were scored with.

## Data in and out

* **Work order PDF**: every row in the work order list has a print action, the
  detail page has «طباعة», and «طباعة النتائج PDF» prints every work order
  matching the current filters (up to 300), one A4 record per page with page
  numbers: verdict, scores, details, both checklists, findings and a
  sign-off block. Save as PDF from the browser's print dialog.
* **Export**: Excel (summary with logo, inspections with every rating,
  findings, and the active checklists with the scoring rules), with the
  current dashboard filters, including pass/fail. There is also a
  print-optimised A4 report with charts and the open findings (save as PDF
  from the browser).
* **Import**: the work-order template, for bulk loading from the CMMS. Its
  date columns are real dd/mm/yyyy date cells, validated like the drop-downs.

## Settings to confirm with the business

All are admin-editable; the seeded values are defaults:

* **Sampling rules** (Settings → قواعد العينة): after-work inspection for 100%
  of emergency and high work orders and 50% of normal; during-work for 100% of
  emergency and 20% of preventive maintenance.
* **Resolution targets** (Settings → زمن الحل والتقييم): emergency 1 day, high
  3, normal 7; critical findings 2 days, normal 7.
* **Scorecard weights**: after-work 40, during-work 20, resolution targets 20,
  findings closed on time 20.
* **Root cause categories** for findings (in code: `domain.RootCauseLabels`).

## Work order API

Admins create tokens under Settings → الربط مع الأنظمة (shown once, stored
hashed). Then, with `Authorization: Bearer <token>`:

* `PUT /api/v1/work-orders/{ref}`: create or update by number (JSON: type,
  priority, site, system, asset_group, asset, contractor, technician,
  description, reported_on, planned_date, actual_date). Names are matched
  like the Excel import; new ones are added.
* `GET /api/v1/work-orders/{ref}`: status, scores and open findings.
* `GET /api/v1/work-orders?updated_since=<RFC 3339>&limit=N`: changes, oldest
  first.

## Production

```sh
cp .env.example .env.prod   # set POSTGRES_PASSWORD, ADMIN_*, SESSION_SECURE=true
docker compose -f compose.prod.yaml --env-file .env.prod up -d --build
```

* Runs as a non-root distroless image. Migrations run automatically on start.
* Microsoft sign-in: set `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`
  and `OIDC_REDIRECT_URL` (see `.env.example`). Users must already exist with
  the same email.
* There is no email: notifications are in-app, and password resets are links
  an admin creates and hands over.
* Put a TLS reverse proxy (Caddy/nginx) in front; the app listens on 127.0.0.1:8080.
* The `backup` service writes a nightly `pg_dump` to `./backups` and keeps 14
  days. Copy these off the server. Photos live in the `uploads` volume, so back
  that up too.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how the code is organised.

## Dates

Gregorian dates are shown as **dd/mm/yyyy**, with the **Hijri (Umm al-Qura)** date
alongside, read-only, on screens, in the print report and in Excel exports.
Date inputs keep the native picker, and a dd/mm/yyyy + Hijri line under each one
confirms the chosen date whatever the browser's locale. Imports accept
dd/mm/yyyy, yyyy-mm-dd, Excel dates and Arabic-Indic digits.

## Notes

* The brand font (Avenir Next Arabic) is self-hosted from `web/static/fonts`.
  Confirm Kidana's font licence covers web embedding before going live.
