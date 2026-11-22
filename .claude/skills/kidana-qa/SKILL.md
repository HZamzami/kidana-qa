---
name: kidana-qa
description: How to build in the Kidana QA maintenance-quality app (Go + templ + HTMX + Alpine CSP + Tailwind v4 + Postgres, Arabic RTL, Kidana brand). Use for ANY work in this repo — new pages or screens, UI components, styling/theming, dashboard KPIs or charts, inspection/checklist/criteria changes, work-order or finding features, Excel import/export, migrations and queries, tests — and whenever someone asks what the app is, how its QA/QC model works, or how to make something "look like the rest of the app".
---

# Kidana QA

A quality system for Kidana's maintenance contractors. Every **work order** (أمر عمل) or preventive maintenance job (صيانة وقائية) can be inspected twice:

- **QA — رقابة أثناء التنفيذ**: a Kidana inspector goes with the contractor *while* the work happens and checks *how* it is done (safety/LOTO/PPE, procedure, materials, technician competence, arrival on time).
- **QC — فحص بعد التنفيذ**: *after* the work, checks the *result* (technical quality, readings, photos, cleanliness, CMMS closure, on-time completion).

Each failed check becomes a **finding** (ملاحظة) with a corrective action, owner and due date. A work order is **accepted** (معتمد) automatically when its latest QC passes and no findings are open. Read `references/domain.md` before changing any rule, criterion or status.

## What the app does today (feature map)

| Area | Route(s) | Highlights |
|---|---|---|
| Dashboard | `/` | KPIs (QA and QC averages, pass rate, open/overdue findings, on-time rate), bento charts, insights. Filters: stage, type, site, system, contractor, **result (pass/fail)**, date range |
| Work orders | `/work-orders`, `/work-orders/{id}` | HTMX search and filters; detail page with facts, a QA → QC timeline and findings. **Save and add another**, **copy as new** (`/work-orders/new?from=ID`), **admin delete** (only before any inspection is submitted) |
| Inspections | `/inspections/{id}` | Phone-first stepper: one criterion per card, autosave, live score, photos per item, inline findings, server-rendered review and submit; read-only summary after submit; admins can re-open |
| Findings | `/findings` | Open/overdue/closed views; edit, close with a note, re-open |
| Data & reports | `/data`, `/reports/print`, `/reports/export.xlsx` | **Print report** (A4: KPIs, the four charts, insights, tables, open findings, inspection log, signatures); **Excel export** (summary with logo, inspections, findings, criteria sheet); work order **import template** with real date cells. All follow the dashboard filters |
| Settings (admin) | `/admin/...` | Versioned checklists, sites, systems, contractors, users (temporary passwords) |

Everything shows dates as dd/mm/yyyy with the Umm al-Qura Hijri date alongside.

## The look in one paragraph

Clean, quiet, data-first ("Style 1": shadcn/Linear-like minimalism) with Kidana's palette. That means white cards with hairline `ink-10` borders on an `ink-5` page, charcoal text, **gold for the primary call to action and accents**, olive green for pass, and muted brick (`fail`) for fail. The app is RTL Arabic set in Avenir Next Arabic, with tabular LTR-isolated numbers. Phone screens use the **field pattern**: one criterion per card, large 48px+ tap targets, a sticky live score. The dashboard uses a **bento** grid. Brand rule: **only lighter tints of the brand colours, never darker shades.** Full system: `references/design-system.md`.

## Before you start

1. **The host has no Go or Node.** Everything runs through Docker via `just` (`just up`, `just gen`, `just test`, `just lint`). See `references/stack.md` for the dev environment and its gotchas.
2. Find the closest existing screen or component and copy its patterns. Inventing a new one is the last resort. Catalogue: `references/components.md`.
3. For any change that touches data, follow the order in `references/recipes.md`: migration → query → `just gen` → domain rule + test → handler → view → test.

## Non-negotiables

- **Business rules live in `internal/domain`** as pure functions with table-driven tests. Handlers stay thin.
- **SQL lives in `internal/db/queries/*.sql`** and goes through sqlc. Schema changes always get a new goose migration; never edit one that has been applied.
- **Alpine is the CSP build.** Register components with `Alpine.data()` in `web/static/js/app.js`. Templates reference properties or methods only, with no inline JS. Don't use `hx-on`, because htmx eval is disabled.
- **Arabic UI copy**, written in the user's language: short, direct, active ("حفظ", "إرسال الفحص", "تم إغلاق الملاحظة"). Enum labels go in `internal/domain/labels.go` and nowhere else.
- **Use Kidana tokens only** (`ink`, `gold`, `green`, `sky`, plus `fail` for semantics). No raw hex values in templates except where SVG needs a literal colour.
- **Dates**: display Gregorian as **dd/mm/yyyy** with the **Umm al-Qura Hijri** date alongside (read-only). Use `@DateText`, `@DateInline` and `@HijriText` in views and `internal/dates` in Go. Store and submit ISO only.
- **Numbers inside Arabic text**: put percentages through `domain.Pct` / `domain.PctVal` (bidi-isolated, so you never get "%50"), and counted nouns through `domain.Count(n, domain.NounX)` for correct agreement (فحصان، 5 فحوصات، 12 فحصاً).
- **Forms validate on the server.** On error, re-render with status `422` and `Errors map[string]string`.
- **Every mutation** runs in `db.InTx` when it touches more than one row, calls `s.audit(...)`, and calls `s.refreshWorkOrderStatus(...)` if it can change acceptance.
- After editing `.templ` or `.sql`, run `just gen`. Generated files are committed.

## Reference map

| Need | Read |
|---|---|
| What QA/QC/findings/acceptance mean, scoring, criteria, roles, glossary | `references/domain.md` |
| Colours, type, spacing, layout patterns, RTL, copy tone, do/don't | `references/design-system.md` |
| Existing templ components and CSS classes, with usage | `references/components.md` |
| Step-by-step: new page, new field, new criterion/stage, KPI/chart, htmx partial, Alpine component, Excel column, tests | `references/recipes.md` |
| Docker/just dev loop, project layout, security model, gotchas | `references/stack.md` |

## Definition of done

- `just gen` leaves no diff, and `just test` and `just lint` pass.
- `just ui-check` (Playwright walkthrough, both widths, all roles) reports no failed steps and no console errors. Look at the screenshots in `tmp/ui-check/` for anything you touched.
- The screen works at **390px phone** and **desktop** widths, with no horizontal page scroll (only tables scroll, inside `overflow-x-auto`).
- Empty states, validation errors and the viewer role (read-only) are all handled.
- Text is Arabic and RTL-correct, and numbers use `.num`.
