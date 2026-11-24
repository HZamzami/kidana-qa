---
name: kidana-quality
description: How to build in the Kidana Quality maintenance-quality app (Go + templ + HTMX + Alpine CSP + Tailwind v4 + Postgres, Arabic RTL, Kidana brand). Use for ANY work in this repo — new pages or screens, UI components, styling/theming, dashboard KPIs or charts, inspection/checklist/criteria changes, work-order or finding features, Excel import/export, migrations and queries, tests — and whenever someone asks what the app is, how its during/after inspection model works, how printing, translation, the contractor portal, offline mode or the API work, or how to make something "look like the rest of the app".
---

# Kidana Quality

A quality system for Kidana's maintenance contractors. Every **work order** (أمر عمل) or preventive maintenance job (صيانة وقائية) can be inspected twice:

- **فحص أثناء التنفيذ** (during the work, `during`): a Kidana inspector goes with the contractor *while* the work happens and checks *how* it is done (safety and permits, the approved procedure, materials, technician competence and calibrated instruments, protection of the site, arrival on time).
- **فحص بعد التنفيذ** (after the work, `after`): checks the *result* (functional test, safe return to service, readings, workmanship, cleanliness, documentation and CMMS closure).

**Never write "QA"/"QC"** anywhere (UI, code, docs): the business doesn't use those words. Short labels are «أثناء التنفيذ» / «بعد التنفيذ».

Each failed check becomes a **finding** (ملاحظة) with a corrective action, owner and due date. A work order is **accepted** (معتمد) automatically when its latest after-work inspection passes and no findings are open. Work orders carry a **المنظومة** and a **مجموعة الأصل** (independent admin-managed lists) and a priority of عادي / عالي / طارئ. Read `references/domain.md` before changing any rule, criterion or status.

## What the app does today (feature map)

| Area | Route(s) | Highlights |
|---|---|---|
| Dashboard | `/` | KPIs, criteria per stage, by site/grade/contractor, **monthly trend**, insights, saved views. Filters: stage, type, site, `system`, `group`, contractor, result, dates. Contractors are redirected to their tasks |
| My tasks | `/my/tasks` | Assigned inspections, responses awaiting my verification, the claimable pool; contractors see findings awaiting their response |
| Work orders | `/work-orders`, `/work-orders/{id}` | Filters incl. priority and `sla` (breached/at risk), saved views; detail with facts, SLA pill, report date, timeline, **assignments card**, findings, **history**. Save-and-add, copy, admin delete |
| Work order print | `/work-orders/{id}/print`, `/work-orders/print?…` | A4 record: verdict, scores, details, checklists, findings, signatures; «تضمين الصور» adds photos |
| Inspections | `/inspections/{id}` | Phone stepper with autosave, photos (location/time; draw on a copy at `/attachments/{id}/annotate`), findings, signatures on review, location stamp; **works offline** (queue + service worker) |
| Findings | `/findings`, `/findings/{id}/edit` | Filters (scope incl. «بانتظار التحقق», severity, stage, system, group, contractor, repeats), bulk changes, Excel/PDF; respond (contractor) → verify/reject (inspector); root cause, owner user, evidence photos, repeat tag |
| Assets | `/assets`, `/assets/{id}`, `/a/{code}` | Register linked from work orders; history, repeat failures, QR image and A4 label sheets |
| Contractors | `/contractors`, `/contractors/{id}`, `/reports/contractor/{id}` | Ranking by composite score, scorecard with trend and weakest criteria, A4 report |
| Search | `/search` | Work orders, assets, findings; exact ref redirects. Header search box |
| Notifications | `/notifications` (+ bell) | In-app only; rendered in the reader's language |
| Data & reports | `/data`, `/reports/print`, `/reports/export.xlsx` | Print report, Excel export, import template, **monthly report runs** |
| Settings (admin) | `/admin/...` | Checklists (optionally scoped to a system/asset group), sites, systems, asset groups, contractors, users (incl. contractor accounts, reset links, 2FA reset), sampling rules, SLA targets and scorecard weights, audit log, API tokens |
| Account | `/account/security`, `/forgot`, `/reset/{token}`, `/login/2fa`, `/auth/oidc` | Optional TOTP 2FA, admin-issued reset links, OIDC sign-in |
| API | `/api/v1/work-orders[/{ref}]` | Bearer-token JSON for CMMS sync (list changes, get, PUT upsert) |

Dev data: `just seed` (or `just seed -reset`) loads ~150 realistic work orders with inspections, findings, assignments, contractor users (`contractor1@demo.kidana.local`) and notifications; never in production.

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
- **Every UI string is translatable.** Wrap it: `t(ctx, "…")` / `tf(ctx, "…%s", x)` in templates, `appctx.T`/`Tf` in handlers, `i18n.T(l, …)` in Excel/domain. Add the English to `internal/i18n/catalog_en.go` (`TestCatalogComplete` fails otherwise). Names with an English column render with `nm(ctx, ar, en)`. Use logical CSS (`ms/me/ps/pe/start/end`) and `backArrow(ctx)` so English (LTR) works.
- **Contractor scoping.** Contractor users see only their own contractor's records: apply `appctx.ContractorScope` to list queries and `canSeeContractor` to detail/print/file routes (answer 404). Staff-only pages use the `staff` route guard.
- **Notify, don't email.** Use `s.notify(ctx, q, users, note{kind, link, dedupe, params})`; add the message template for a new kind in `views.NotificationText`.
- **Use Kidana tokens only** (`ink`, `gold`, `green`, `sky`, plus `fail` for semantics). No raw hex values in templates except where SVG needs a literal colour.
- **Dates**: display Gregorian as **dd/mm/yyyy** with the **Umm al-Qura Hijri** date alongside (read-only). Use `@DateText`, `@DateInline` and `@HijriText` in views and `internal/dates` in Go. Store and submit ISO only.
- **Numbers inside Arabic text**: put percentages through `domain.Pct` / `domain.PctVal` (bidi-isolated, so you never get "%50"), and counted nouns through `count(ctx, n, domain.NounX)` (views) or `domain.CountIn(lang, …)` for correct agreement in both languages (فحصان، 5 فحوصات، 12 فحصاً / 5 inspections).
- **Forms validate on the server.** On error, re-render with status `422` and `Errors map[string]string`.
- **Every mutation** runs in `db.InTx` when it touches more than one row, calls `s.audit(...)`, and calls `s.refreshWorkOrderStatus(...)` if it can change acceptance.
- After editing `.templ` or `.sql`, run `just gen`. Generated files are committed.

## Reference map

| Need | Read |
|---|---|
| What the during/after stages, findings, acceptance and verdict mean, scoring, criteria, lookups, roles, glossary | `references/domain.md` |
| Colours, type, spacing, layout patterns, RTL, copy tone, do/don't | `references/design-system.md` |
| Existing templ components and CSS classes, with usage | `references/components.md` |
| Step-by-step: new page, new field, new criterion/stage, KPI/chart, htmx partial, Alpine component, Excel column, tests | `references/recipes.md` |
| Docker/just dev loop, project layout, security model, gotchas | `references/stack.md` |

## Definition of done

- `just gen` leaves no diff, and `just test` and `just lint` pass.
- `just ui-check` (Playwright walkthrough, both widths, all roles) reports no failed steps and no console errors. Look at the screenshots in `tmp/ui-check/` for anything you touched.
- The screen works in **Arabic and English** (`lang` cookie `en`), and the catalog test passes.
- The screen works at **390px phone** and **desktop** widths, with no horizontal page scroll (only tables scroll, inside `overflow-x-auto`; the A4 print preview scrolls inside `.sheets`).
- Empty states, validation errors and the viewer role (read-only) are all handled.
- Text is Arabic and RTL-correct, and numbers use `.num`.
