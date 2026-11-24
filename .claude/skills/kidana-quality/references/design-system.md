# Design system

**Direction: "Style 1": clean, minimal and data-first** (the shadcn/ui and Linear family), dressed in Kidana's identity. Two context patterns sit on top:
- **Field pattern** (phone, used during inspections): one thing at a time, big tap targets, sticky status.
- **Bento pattern** (desktop dashboard): a grid of tiles in different sizes, KPIs first.

The source of truth is `web/styles/input.css` (Tailwind v4 `@theme` + a few `@layer components` classes).

## Colour tokens

Brand: primary **#4C4F53** (ink) and **#B59666** (gold); secondary **#627D4C** (green) and **#93BBCA** (sky).
**Brand rule: only lighter tints, never darker shades.** Hover states therefore go *lighter* (`hover:bg-ink-80`, `hover:bg-gold-80`), never darker.

| Token | Hex | Use |
|---|---|---|
| `ink` | #4C4F53 | Body text, primary buttons, table header fill in reports |
| `ink-80` | #707276 | Secondary text, labels |
| `ink-60` | #94959A | Tertiary text, hints, placeholders |
| `ink-30` | #C9CACB | Input borders, dashed upload tiles |
| `ink-10` | #EDEDEE | Card borders, dividers, bar tracks |
| `ink-5` | #F6F6F6 | Page background, table header, subtle fills |
| `gold` | #B59666 | **Primary accent**: main CTA (`btn-gold`), active nav underline, focus ring, progress, after-work accent |
| `gold-80/50/20/10` | #C4AB85 / #DACBB3 / #F0EAE0 / #F8F5F0 | Hover, link underlines, "partial" fill, row hover |
| `green` | #627D4C | **Pass**: مطابق, accepted, good insights |
| `green-80/50/20/10` | … | Pass pills and fills |
| `sky` | #93BBCA | **During-work accent**, focus outline, info, in-progress |
| `sky-60/30/15` | … | During-work pill fill, info fills |
| `fail` | #A65A4E | **Fail**: غير مطابق, overdue, critical. The one non-brand colour, a muted brick chosen to sit with the palette. Never use a pure red. |
| `fail-20/10` | #F2E1DD / #F9F0EE | Fail fills and borders |

**Semantic mapping (keep it consistent everywhere):**
- during the work → `sky` (`bg-sky-30` pill, `border-t-sky`, timeline dot `bg-sky`)
- after the work → `gold` (`bg-gold-20` pill, `border-t-gold`, dot `bg-gold`)
- priority → عادي `ink-5`, عالي `gold-20`, طارئ `fail-10` + `fail` text
- pass / compliant → `green-10` bg + `green` text
- partial → `gold-20` bg + `ink` text
- fail / non-compliant / open finding → `fail-10` bg + `fail` text
- N/A / unrated / draft → `ink-5` bg + `ink-60`/`ink-80` text
- critical → outlined pill `border border-fail text-fail`
- overdue → solid `bg-fail text-white` (the only solid fail fill, kept for urgency)

Grade donut order (`gradeColors`): green, sky, gold, gold-50, ink-60.

## Typography

- **Avenir Next Arabic** (self-hosted woff2, weights 400 / 500–600 / 700–900) with fallbacks of Segoe UI, Tahoma and Arial. `font-synthesis: none`, so only real weights are used.
- Base size is `text-sm` (14px) with `leading-relaxed`.

| Element | Classes |
|---|---|
| Page title | `text-xl font-semibold text-balance` |
| Section title | `font-semibold` (16px) or `text-lg font-semibold` on the phone flow |
| Label | `.label` (xs, medium, ink-80) |
| Secondary text | `text-sm text-ink-80` |
| Hints | `text-xs text-ink-60` |
| KPI value | `num text-2xl font-semibold` |
| Live score | `num text-xl font-semibold` |

- **Numbers** always use `.num`, which gives tabular figures plus `direction:ltr; unicode-bidi:isolate`, so `85.5%`, dates and WO refs never get reordered by RTL. Put `.num` on the **inline number only**, never on a block that also holds Arabic, or the block flips LTR and hugs the left edge.
- **Percentages inside Arabic sentences** go through `domain.Pct` / `domain.PctVal`, which wrap the value in U+2066…U+2069 isolates. A bare `"%s%%"` renders as "%50".
- Scores use one decimal (`scoreVal`, `domain.Pct`).

## Dates (Gregorian + Hijri)

Kidana works in both calendars:
- **Gregorian is shown as dd/mm/yyyy.** The **Hijri date (Umm al-Qura, the official Saudi calendar) is shown alongside, read-only**. It is never entered or stored.
- Components: `@DateText(*time.Time)` stacks dd/mm/yyyy over the muted Hijri date (detail facts, table cells); `@DateInline(time.Time)` gives "22/11/2026 · 12/06/1448 هـ" (meta lines); `@HijriText(time.Time)` renders the Hijri date alone. `dateTime(t)` gives "22/11/2026 15:44" (Riyadh time).
- Only the digits are LTR (`.num`), and "هـ" stays outside the number span. Hovering shows the long form ("12 جمادى الآخرة 1448 هـ").
- **Date inputs** stay native `type="date"` (a good picker on phones; ISO value). Their display format follows the *browser* locale, so every date input gets a reference line underneath: "22/11/2026 — 12/06/1448 هـ". `TextField` adds it automatically for `type: "date"`; elsewhere, wrap the input in `x-data="dateEcho"`, add `x-on:change="echo"`, and put `@dateEchoLine(iso)` after it.
- Go: `internal/dates` provides `DMY`, `DMYTime`, `Hijri`, `HijriDigits`, `HijriLong` and `Both`. It is valid for 1356–1500 AH; outside that range the Hijri part is just omitted.
- Excel: date cells use the `dd/mm/yyyy` format with an extra read-only Hijri column. Template date columns are **text**, so Excel's locale can't swap day and month; `excel.ParseDate` reads dd/mm/yyyy, ISO, Excel serials and Arabic-Indic digits.

## Arabic counting

Use `domain.Count(n, domain.NounX)`, never `fmt.Sprintf("%d فحص")`. Nouns available: `NounInspection` (فحص/فحصان/فحوصات/فحصاً), `NounFinding`, `NounWorkOrder`, `NounItem`, `NounDay`, `NounRating`, `NounCase`, `NounUser`, `NounEntry` and `NounAttempt`. Add new nouns to `domain/count.go`. When a sentence reads awkwardly with a count, use the label form: "الملاحظات المفتوحة: 3".

## Shape, spacing, elevation

- Radius: cards use `rounded-card` (10px), controls `rounded-lg`, big phone buttons and rating tiles `rounded-xl`, and pills `rounded-full`.
- Borders do the separating, not shadows. Cards are `border border-ink-10` with no shadow. Shadows appear only on floating things: menus (`shadow-lg`) and the toast.
- Spacing scale: page `py-6`, section gaps `gap-4`/`gap-5`/`mt-5`, card padding `p-4`/`p-5`, form grids `gap-4`. Lay siblings out with `gap`, not margins.
- Container is `mx-auto max-w-7xl px-4 sm:px-6`. The phone inspection flow is narrower: `max-w-2xl`.
- Use card styling **by role**: a card is a separate object. Don't nest cards inside cards; use `rounded-lg border` or tinted boxes inside a card instead.

## Layout patterns

**App shell** (`views.App`): a white header with the colour logo and the title block; a tab nav underneath with a gold 2px underline on the active tab, scrolling horizontally on phones. The content goes in `<main>` and the flash banner sits at the top.

**Page header** (`views.PageHeader(title, subtitle)`): the title and subtitle on the start side, with actions as children on the end side (`ms-auto`).

**Filter bar**: an even **grid**, never a wrapping flex row (wrapping splits groups like a date range and leaves stragglers). Pattern: `card p-3 grid grid-cols-2 md:grid-cols-4 xl:grid-cols-<n> items-end gap-3`, where `n` is the number of cells on wide screens; a search box takes `col-span-2`. Keep related fields adjacent (from/to dates last, together). Cells are `min-w-0` divs (`FilterSelect` already is) and compact controls use `h-9`. Selects use "الكل" as the empty option. **"إعادة ضبط الفلاتر" lives in the page header actions**, not in the bar. A `<noscript>` apply button stays for no-JS. htmx re-renders only the results:
`hx-get=<same url> hx-trigger="change, input from:#search delay:350ms" hx-target="#results" hx-select="#results" hx-swap="outerHTML" hx-push-url="true"`.

**Control height**: every single-line `.input` (text, date, number, select) is `h-10` app-wide so mixed controls line up in any row; use `h-9` for compact bars and inline table edits. Textareas and file inputs are exempt. Never align controls with ad-hoc padding.

**List**: a `card overflow-hidden` wrapping `overflow-x-auto` wrapping `table.table`. The first column links (underline `decoration-gold-50`, hover `decoration-gold`), status pills sit in a trailing column, and a footer row shows the count and pagination. If there are no rows, show `EmptyState(title, hint)`.

**Detail**: a breadcrumb (`text-ink-80`, `/` separators in `ink-30`), a header card with a `Fact` grid (`dl` with `grid-cols-2 md:grid-cols-4`), and then a `lg:grid-cols-5` split of main (3) and side (2).

**Timeline** (inspections on a work order): an `ol` with `border-s-2 border-ink-10`, a dot per item coloured by stage, and each item a clickable card.

**Print record (A4)**: follow `print.templ` for anything printed per record. One record per page; identity first (logo, title, ref large and LTR), then the answer (verdict and scores) before the detail, then details as a bordered 4-column fact grid, checklists side by side, findings, sign-off, and a footer with print time and printer. Sizes in pt: body 8.5, labels 7–7.5, section titles 9.5, ref 19. Colour is only a second signal: every rating and verdict also has a word and a shape, so black-and-white copies stay readable. Use light tints for fills, full-contrast text, and `print-avoid-break` on sections. Check the output as a real PDF (Playwright `page.pdf`), not only on screen.

**Field pattern (inspection flow, phone first)**:
- A sticky header card holds the WO ref, stage pill, site/system/asset line, a **live score box** that tints green or fail, a step text, the rated count, a gold progress bar, and a critical-fail banner.
- Below it, a horizontal strip of criterion-code chips lets the inspector jump between steps.
- Each criterion gets one card: code chip, `text-lg` title, description, weight pill and critical pill, then the **2×2 rating grid** (`.rate` tiles, min-height 48px, icon plus label), a note textarea, a photo strip with a camera tile, and an inline finding form.
- Big previous/next buttons (`btn-lg`) close each card.
- The last step is the server-rendered review: result, the list of ratings, date and notes, general photos and findings, errors, a gold full-width submit button, and a delete-draft action behind an in-page confirm.
- Autosave on `change` via htmx (`hx-swap="none"`) means there is no save button per item.

**Dashboard layout (must scale with data)**: KPI row (`grid-cols-2 md:grid-cols-3 xl:grid-cols-6`, each a `kpi` tile with a coloured dot, label, value and sub-line), then:
1. **Criteria, one card per stage** (`criteriaCard`) in `lg:grid-cols-2`: during and after side by side at similar heights. When the stage filter is set, only that card shows (`lg:col-span-2`) and its rows flow into two columns.
2. **Three equal-height cards** (`md:grid-cols-3`, stretched): by site, grade donut, by contractor. Lists are capped with `groupBars(groups, 6)` plus an "و N أخرى" line.
3. **Insights**, full width.

Never let one card's height be driven by an unbounded list next to fixed-height cards. That is what produced the empty bento gaps. Split it (by stage), cap it (top N), or make it compact. Test with realistic data volumes, not an empty database.

**Compact rows** (`compactBarRow`, criteria): label, bar and value on one line from `sm` up, with counts in the `title` tooltip. On phones (no hover) the full label wraps beside the value and the bar runs full width below. In print, labels wrap instead of truncating. Copy only the "hover for details" hint at `sm:` and up.

**Charts**: server-rendered and scaled to 0–100%. Bars are `barRow` (track `bg-ink-10 h-2 rounded-full`, fill green ≥ 80 else gold, the value in `.num` at the end); the donut is SVG circles with `stroke-dasharray`. No chart JS library.

**Print report** (`/reports/print`): A4, white, gold rule under the header, a 3-column KPI grid, the four dashboard charts (same components as the screen, in `reportChart` tiles), insights, site/contractor and criteria tables, the **open findings** table, the inspection log, and signature lines (أعدّه / راجعه / اعتمده). Use `.no-print` and `.print-avoid-break`. Print CSS sets `print-color-adjust: exact` so bars and pills keep their colours.

## RTL rules

- `<html lang="ar" dir="rtl">`. Use **logical** utilities only: `ms-/me-/ps-/pe-/start-/end-/border-s/text-start`. Never use `ml-`, `left-` or `text-left`.
- Arrows point to the right for "back" (`→`) in RTL.
- Inputs for emails and phone numbers get `dir="ltr"`; mixed refs get `dir="auto"`.
- Horizontal bars fill from the start side (right), which happens naturally under `dir=rtl`.

## States and feedback

- **Focus**: a global `:focus-visible` outline in `sky`, and inputs get `focus:border-gold focus:ring-gold-20`.
- **Invalid field**: `aria-invalid="true"` gives a fail border and ring, plus `.field-error` text under the field linked via `aria-describedby`.
- **Form error summary**: a `role="alert"` box at the top in `bg-fail-10 border-fail-20 text-fail`.
- **Flash** (after a redirect): `s.flash(ctx, msg, "ok"|"warn"|"")`. **Toast** (after an htmx call): `toast(w, msg, tone)`, which travels in the HX-Trigger header ASCII-escaped. Never write Arabic into an HTTP header directly, because browsers decode headers as Latin-1 and it turns into mojibake.
- **Destructive actions**: use the in-page `confirmAction` Alpine component, never `window.confirm`. Use `btn-danger` for the confirming button.
- **Loading**: put `.htmx-indicator` text next to the control.
- **Read-only** (viewer, or a submitted inspection): hide edit controls entirely; don't just disable them.

## Copy tone (Arabic)

- Write the way a knowledgeable colleague would talk: short and direct. Name things by what users recognise (أمر العمل، الملاحظة), not system terms.
- Buttons say exactly what happens: "حفظ", "بدء فحص بعد التنفيذ", "إرسال الفحص", "إغلاق الملاحظة". The toast or flash then confirms in the past tense: "تم إرسال الفحص — النتيجة 86% (مطابق)".
- Errors say what is wrong and how to fix it, without apologies: "حقل «الموقع» مطلوب", "قيّم جميع المعايير (2 بند غير مقيّم)".
- Field labels quote names with «…».
- Use Western digits (0–9) in the UI, which matches the CMMS. Parsing accepts Arabic-Indic digits on import.

## Do / Don't

- **Do** put gold on one primary action per view. Everything else is `btn-outline` or `btn-ghost`.
- **Do** encode state in form as well as colour: pill text plus colour, and icons on the rating tiles.
- **Do** keep dashboards summary-first: KPIs, then charts, then insights, then details.
- **Don't** add new colours, gradients, glassmorphism or heavy shadows.
- **Don't** darken brand colours, and don't use pure red or emoji.
- **Don't** put `rounded-*` plus a shadow on everything; flat bordered cards are the house style.
- **Don't** let the page scroll sideways on phones. Only tables may scroll, inside `overflow-x-auto`.
- **Don't** hard-code Arabic enum labels in templates. Use `domain.*Labels` or the stage/status methods.
- **Don't** reuse a word with two meanings next to each other. Grade "مقبول" (60–69) vs. status "معتمد" is why grades are shown as "التقدير: مقبول".
- **Don't** let native number spinners show: they overlap digits in RTL. `input[type=number]` hides them globally.
