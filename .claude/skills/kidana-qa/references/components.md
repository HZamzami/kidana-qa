# Component catalogue

Check here before writing markup. templ components live in `internal/views/`, and CSS component classes in `web/styles/input.css` (`@layer components`).

## CSS classes

| Class | What |
|---|---|
| `.card` | White surface, `border-ink-10`, 10px radius. Add padding yourself (`p-4`/`p-5`). |
| `.btn` + `.btn-primary` | Charcoal, for strong secondary actions (save, start QA). |
| `.btn` + `.btn-gold` | **The** primary CTA (submit inspection, export, start QC). One per view. |
| `.btn` + `.btn-outline` | Neutral actions (edit, back, view). |
| `.btn` + `.btn-ghost` | Tertiary actions (reset, cancel, menu triggers). |
| `.btn` + `.btn-danger` | Confirming a destructive action. |
| `.btn-lg` | Big phone buttons (`py-3 text-base rounded-xl`). |
| `.label`, `.input`, `.field-error` | Form primitives. `.input` also styles `select` and `textarea`. |
| `.pill` | Small rounded badge. Add colour utilities. |
| `.table` | Styled table (header fill, row hover in `gold-10`). Wrap it in `overflow-x-auto`. |
| `.num` | Tabular, LTR-isolated numbers. Use it on every score, date, ref and count. |
| `.rate` | Rating tile: a hidden radio plus a `span[data-k=C|P|N|NA]`, coloured when checked. |
| `.htmx-indicator` | Hidden until its request is in flight. |
| `.no-print`, `.print-avoid-break` | Print helpers. |
| `[x-cloak]` | Hidden until Alpine initialises. |

## Layout (`layout.templ`)

```templ
@App("عنوان الصفحة") {              // authenticated shell: header, nav, flash, footer
	@PageHeader("العنوان", "سطر توضيحي") {
		<a href="/x/new" class="btn btn-primary">＋ جديد</a>   // actions go here (optional)
	}
	...
}
@Base("title") { ... }              // bare document (login, print report)
@ErrorPage(404, "message")          // picks App or Base depending on login
```
To add a nav tab, edit the `nav` slice in `layout.templ`. Set `admin: true` to hide it from non-admins.

## Pills (`components.templ`)

| Component | Input | Renders |
|---|---|---|
| `@StagePill(stage)` | `"qa"`/`"qc"` | "QA · أثناء التنفيذ" (sky) / "QC · بعد التنفيذ" (gold) |
| `@TypePill(t)` | `"PM"`/`"WO"` | صيانة وقائية (green tint) / أمر عمل (ink tint) |
| `@WOStatusPill(status)` | wo_status | open, in_progress, completed, accepted, cancelled |
| `@FindingStatusPill(status, overdue)` | finding_status, bool | status pill + solid "متأخرة" when overdue |
| `@SeverityPill(sev)` | `"critical"` only | outlined "حرجة" |
| `@ResultPill(score *float64, passed *bool, critical bool)` | inspection result | "مسودة" / "86% · مطابق" / "62% · غير مطابق · بند حرج" |
| `@RatingPill(r)` | `"C"/"P"/"N"/"NA"/""` | rating label pill, dashed "غير مقيّم" for empty |
| `@ScoreBar(v float64, passed bool)` | list scores (`-1` = none) | value + mini bar |

## Dates

| Component | Use |
|---|---|
| `@DateText(t *time.Time)` | dd/mm/yyyy with the muted Hijri date under it; "—" for nil. Use for facts and table cells. |
| `@DateInline(t time.Time)` | "22/11/2026 · 12/06/1448 هـ" on one line, for meta lines. |
| `@HijriText(t time.Time)` | Hijri only ("12/06/1448 هـ"), with the long form as a tooltip. |
| `@dateEchoLine(iso string)` | Reference line under a date input. Needs a parent `x-data="dateEcho"` and the input's `x-on:change="echo"`. `TextField` does this for `type: "date"`. |

## Forms

```templ
@TextField("ref", "رقم أمر العمل", d.V.Ref, true, d.Errors, templ.Attributes{"maxlength": "60"})
@TextField("planned_date", "التاريخ المجدول", d.V.PlannedDate, false, d.Errors, templ.Attributes{"type": "date"})
@TextArea("description", "وصف العمل", d.V.Description, 3, d.Errors)
@SelectField("site_id", "الموقع", d.V.SiteID, d.Sites, "اختر الموقع", true, d.Errors)
@FilterSelect("site", "الموقع", d.Query.Get("site"), d.Sites)   // filter bars: first option "الكل"
@FieldError(errs, "name")
```
- Field ids are `f_<name>` (filters use `flt_<name>`), so tests and labels can target them.
- `Option{Value, Label}` is the select option type. Build lists with `s.lookupOptions(ctx, activeOnly)`, `priorityOptions()`, `woStatusOptions()`, `views.StageOptions` and `views.TypeOptions`.
- A form-level error banner: `if len(d.Errors) > 0 { <div role="alert" class="... bg-fail-10 ...">يرجى مراجعة الحقول المعلّمة.</div> }`.

## Content blocks

```templ
@EmptyState("لا توجد ملاحظات", "تلميح اختياري") { <a class="btn btn-primary mt-4">…</a> }
<dl class="grid gap-x-6 gap-y-4 grid-cols-2 md:grid-cols-4">
	@Fact("الموقع") { { d.WO.SiteName } }
</dl>
```

## Dashboard pieces (`dashboard.templ`)

| Component | Use |
|---|---|
| `@kpi(label, value, sub, tone)` | KPI tile. tone: `""`, `"sky"`, `"gold"`, `"green"` or `"fail"` (the dot colour). |
| `@barRow(label, value, sub, critical)` | One 0–100 bar row (green ≥ 80, gold below). |
| `@groupBars([]domain.Group, max int)` | barRows for a site/contractor/system grouping, capped at `max` with an "و N أخرى" line. |
| `@criteriaCard(stage, []domain.CriterionStat, wide)` | One stage's criterion compliance (use `S.CriteriaFor(stage)`). `wide` = full width with two columns. |
| `@compactBarRow(domain.CriterionStat)` | One-line criterion row (stacks on phones); tooltip has counts. |
| `@gradeDonut([]domain.GradeCount)` | SVG donut with legend. |

Option lists for filters and forms: `views.StageOptions` (qa/qc), `views.TypeOptions` (PM/WO), `views.ResultOptions` (pass/fail).

## Print report pieces (`data.templ`)

| Component | Use |
|---|---|
| `@ReportPrint(ReportData)` | The A4 page: KPIs, charts, insights, groups, criteria, open findings, inspection log, signatures. |
| `@reportKPI(label, value, sub)` | Compact bordered KPI tile. |
| `@reportChart(title) { … }` | Bordered tile wrapping any dashboard chart component (`groupBars`, `criteriaRows`, `gradeDonut`). Use it so print and screen charts stay identical. |
| `@criteriaRows([]domain.CriterionStat)` | Compact criteria list for a report tile. |
| `@reportGroup(title, []domain.Group)` | Site/contractor table with pass/fail-tinted averages. |

## Work order actions (`workorders.templ`)

| Action | How |
|---|---|
| Save and add another | Second submit button `name="next" value="new"` (new form only). The handler redirects to `/work-orders/new?keep=ctx&from=ID`, which keeps type, site, system and contractor. |
| Copy as new | Link to `/work-orders/new?from=ID`; it prefills everything except the number and dates. |
| Delete (admin) | Shown only when `appctx.IsAdmin(ctx) && d.deletable()` (no submitted inspection), behind `confirmAction`. The server re-checks and refuses otherwise. |

## Inspection flow (`inspection.templ`)

| Component | Use |
|---|---|
| `@PhotoStrip(inspectionID, criterionID *int64, photos, editable)` | Thumbnails + camera upload tile. Re-renders itself after upload or delete. |
| `@FindingBox(inspectionID, criterionID *int64, critical, findings, errs)` | Findings on a criterion + collapsible add form. Swaps into the nearest `div[id^=finding-box]`. |
| `@ReviewBody(d)` | Final step. Fetched again from `/inspections/{id}/review` whenever the stepper reaches review. |
| `@ratingIcon(r)` | Inline SVG check / half / cross / dash. |

## Alpine components (`web/static/js/app.js`, CSP build)

| `x-data` | Provides |
|---|---|
| `inspection` | `step`, `total`, `criteria` (from `data-criteria` JSON); `rate(ev)`, `next()`, `prev()`, `goTo(ev)` (reads `data-step`), `goReview()`; getters `scoreText`, `ratedText`, `isPass`, `isFail`, `criticalFailed`, `progressWidth`, `onReview`, `atStart`, `stepText` |
| `criterionCard` | `visible` (nested in `inspection`, reads `data-index`) |
| `reviewPanel` | `visible` |
| `disclosure` | `open`, `toggle()`, `close()`. Starts open when `data-open="true"`. |
| `confirmAction` | `asking`, `ask()`, `cancel()` for in-page confirmations |
| `dateEcho` | `g` (dd/mm/yyyy), `h` (Umm al-Qura digits via `Intl` `formatToParts`); `echo(ev)` |

Global htmx glue in `app.js`: the `toast` event (from HX-Trigger) shows `#toast`, and `422` responses are swapped so validation errors render.

## Go view helpers (`views/helpers.go`, `dashboard.templ`)

`u(format, args…) templ.SafeURL` · `path(format, args…) string` · `asset(path)` (cache-busted) · `idStr`, `itoa` · `isoDate(*time.Time)` (comparisons and values only) · `dmy(t)`, `dateTime(t)` · `scoreVal(float64)` · `orDash(s)` · `pctWidth(v)` (inline width style) · `delayText(planned, actual)` · `withQuery(base, q, key, val)` (pagination) · `tern(cond, a, b)` · `initial(name)`.
