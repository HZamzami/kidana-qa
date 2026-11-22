# Recipes

Each recipe follows the same spine: **schema → query → `just gen` → domain rule + test → handler → view → test**. Skip the steps that don't apply.

---

## 1. Add a new page (read-only list or detail)

1. **Query** in `internal/db/queries/<area>.sql`:
   ```sql
   -- name: ListThings :many
   SELECT t.*, s.name AS site_name FROM things t JOIN sites s ON s.id = t.site_id
   WHERE (sqlc.narg(site_id)::bigint IS NULL OR t.site_id = sqlc.narg(site_id))
   ORDER BY t.created_at DESC LIMIT 500;
   ```
   Optional filters use `sqlc.narg(x)::type IS NULL OR …`. For joined nullable text, use `COALESCE(x, '')::text` so Go gets a `string`.
2. Run `just gen`.
3. **View** at `internal/views/things.templ`: a data struct, then `@App(...)` → `@PageHeader` → filter bar → `#things-results` card with table or `EmptyState`. Copy `findings.templ` (FindingsPage) as the template.
4. **Handler** in `internal/web/thing_handlers.go`:
   ```go
   func (s *Server) thingsList(w http.ResponseWriter, r *http.Request) {
       rows, err := s.q.ListThings(r.Context(), queryID(r, "site"))
       if err != nil { s.serverError(w, r, err); return }
       s.render(w, r, http.StatusOK, views.ThingsPage(views.ThingsData{Rows: rows, Query: r.URL.Query()}))
   }
   ```
   Return the full page every time; htmx's `hx-select` picks out the results fragment.
5. **Route** in `web/server.go` `routes()`: `mux.HandleFunc("GET /things", s.thingsList)`. Wrap it with `edit(...)` or `admin(...)` for restricted pages.
6. **Nav** (optional): add `{"/things", "الاسم", false}` to `nav` in `layout.templ`.
7. **Test**: add the path to the "Pages render" loop in `web_test.go`.

## 2. Add a field to work orders (full vertical slice)

Example: `building_no`.
1. Run `just migration add_building_no`, then write:
   ```sql
   -- +goose Up
   ALTER TABLE work_orders ADD COLUMN building_no text NOT NULL DEFAULT '';
   -- +goose Down
   ALTER TABLE work_orders DROP COLUMN building_no;
   ```
2. Add the column to `CreateWorkOrder` / `UpdateWorkOrder` in `queries/work_orders.sql` (and to `ListWorkOrders` if the table shows it). Run `just gen`.
3. In `web/workorder_handlers.go`, add it to `woInput`, `parseWO` (with `f.MaxLen`), and the create/update params. Prefill it in `workOrderEdit`.
4. In `views/workorders.templ`, add it to `WOFormValues`, the form (`@TextField`), and the detail `Fact`s.
5. Excel: add it to the template columns (`excel/template.go` `woColumns`), the parser (`parseWorkOrders` col map → `WorkOrderRow`), the import (`importWorkOrders`) and the export (`InspectionExport` + `inspectionsSheet`).
6. Tests: extend `TestTemplateRoundTrip` and the work-order part of `TestInspectionFlow`.

## 3. Change criteria or weights (no code)

Go to Admin → قوائم الفحص → "إصدار جديد من هذه". Edit or add rows, tick حرج for criteria whose failure must fail the whole inspection, then save with "تفعيل" ticked. Old inspections keep their version.

**Changing the seed** (only before the first deploy, or for a fresh install): edit `00002_seed.sql`. Weights per stage should sum to 100. Code prefixes are `Q` for QA and `C` for QC; the Excel export orders columns by them (`excel.SortCriteria`).

## 4. Add a new business rule

1. Write a pure function in `internal/domain/*.go`. Don't import DB or HTTP packages there.
2. Add a table-driven test next to it (see `scoring_test.go`).
3. Call it from the handler or service. If it affects work-order status, change `NextWOStatus` and its test. `refreshWorkOrderStatus` already runs after every relevant mutation.
4. Messages shown to users are Arabic strings returned from the domain function (see `ValidateSubmit`).

## 5. Add a dashboard KPI or chart

1. Add the field to `domain.Summary` and compute it in `Summarize` (with a test in `analytics_test.go`). Add an insight in `Insights` if it deserves a recommendation.
2. KPI: add `@kpi(label, value, sub, tone)` in `dashKPIs`. Keep the row at 6 tiles, or change the grid columns to match.
3. Chart: add a `<section class="card p-4 …">` to the bento grid with an `h2 font-semibold mb-4`, then `@groupBars` or `@barRow`, or a new SVG component drawn on the same 0–100 scale. Pick a column span so the grid has no holes.
4. Mirror it in the print report (`data.templ` `ReportPrint`) and the Excel summary (`excel/export.go` `summarySheet`) so all three agree.

## 6. htmx partial update (autosave, inline add, swap)

- **Autosave** with no response body: `hx-post=... hx-trigger="change" hx-swap="none"`; the handler returns `204`. Send feedback with `toast(w, "…", "ok")`.
- **Re-render a fragment**: give the component a stable `id`, set `hx-target="#that-id" hx-swap="outerHTML"`, and have the handler render just that component (see `PhotoStrip`).
- **Validation errors inside a fragment**: render the fragment with `errs` and status `422`; `app.js` makes htmx swap it.
- **After a POST that changes the page**, use a normal form submit plus `http.Redirect(…, 303)` and `s.flash`. Use `redirect(w, r, url)` when the request may come from htmx (it sends `HX-Redirect`).
- **File uploads**: `hx-encoding="multipart/form-data"`, `http.MaxBytesReader`, `ParseMultipartForm`, then `s.store.Save` (sniffs the type and enforces the limit).
- **Never** use `hx-on` or inline `<script>`. CSP and `allowEval:false` block them.

## 7. Add an Alpine component (CSP build)

```js
// web/static/js/app.js, inside the alpine:init listener
Alpine.data("thingPicker", () => ({
  selected: null,
  init() { this.selected = this.$el.dataset.initial || null; },
  pick(ev) { this.selected = ev.currentTarget.dataset.value; },
  get hasSelection() { return this.selected !== null; },
}));
```
```templ
<div x-data="thingPicker" data-initial={ v }>
	<button type="button" data-value="a" x-on:click="pick">أ</button>
	<p x-show="hasSelection" x-cloak>…</p>
</div>
```
- Use `x-on:` and `x-bind:` (not `@` or `:`) so templ parses the attributes cleanly.
- Pass server data through `data-*` attributes (JSON for lists), never inline script.
- Keep expressions to property and getter names. Put logic in getters and methods.

## 8. Add an Excel export column or import format

- Export: `excel.Report(excel.ReportInput{...})` builds summary (with logo), inspections, findings and criteria sheets. Add the field to `excel.InspectionExport` (or `FindingExport`), append to `headers` and `vals` in the sheet builder, and fill it in `web/data_handlers.go` `exportExcel`. Write dates as `time.Time` with `st.date` (dd/mm/yyyy), never as text.
- Import: `excel.Parse` finds the header row by its titles in the first 10 rows. For a new import format, add a detector, a `parseX` function and a `KindX`, then handle the new kind in `web.importExcel`. Normalise text with `excel.Norm`, and parse values with `ParseDate`, `ParseRating`, `ParseType` and `ParsePriority`. Imports run in one transaction and report `Created`, `Skipped` and `Warnings` (with row numbers, in Arabic).

## 9. Tests

- **Domain/excel**: table-driven unit tests, no DB.
- **HTTP**: `internal/web/web_test.go`. `setup(t)` resets the test schema, migrates, and creates `admin@`, `inspector@` and `viewer@test.local` (password `test-password-123`). Use `e.login(email)`, then `e.post(c, path, url.Values{…})` / `e.get(c, path)`, and assert status codes and DB state through `e.q`.
- Run them with `just test` (all) or `just test -run TestInspectionFlow ./internal/web/`.

## 10. Visual check (no browser on the host)

`just ui-check` (stack must be running) runs `scripts/ui_check.py` in a Playwright container. It walks the whole system like a user: login and its failure, work order create/validate/duplicate/edit, the QA phone flow with photo upload and a bad file, a critical fail with its finding, a refused submit, QC attempts 1 and 2, the findings lifecycle, list filters, the dashboard, the print report, the Excel template, the work order import (plus an unrecognised file), export, admin lookups/users/checklist validation, password errors, 404, and the viewer role.

- Screenshots go to `tmp/ui-check/NN-name.png` at **390px** (phone) and **1366px** (desktop) widths.
- At the end it prints **failed steps** and **console/network problems**. Both must say `none`; 4xx lines from deliberate error tests are filtered out.
- When you add a screen or flow, add a `@step` section to `scripts/ui_check.py`, then actually open the screenshots and check RTL, dates, numbers and empty space.
- It leaves `DEMO-*` / `IMP-*` records in the dev DB on purpose. Run `just nuke && just up` for a clean slate.
