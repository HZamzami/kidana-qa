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
`insp4@…` with the password `demo-password`. The seeder refuses to run in
production.

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
| مدقق (inspector) | Create work orders, run during/after inspections, record and close findings, import/export |
| مشاهد (viewer) | Read-only |

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

## Production

```sh
cp .env.example .env.prod   # set POSTGRES_PASSWORD, ADMIN_*, SESSION_SECURE=true
docker compose -f compose.prod.yaml --env-file .env.prod up -d --build
```

* Runs as a non-root distroless image. Migrations run automatically on start.
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
