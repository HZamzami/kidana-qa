# Domain: what the app models and why

## The problem

Kidana (شركة كدانة للتنمية والتطوير) runs maintenance at the holy sites (منى، عرفات، مزدلفة، مكة) through contractors. Its Operations Quality department (إدارة جودة العمليات, قطاع العمليات) must prove that the work is done **safely and correctly**. The earlier tool scored each job once against one mixed checklist and kept data only in the browser. This app splits the two questions the business really asks:

| | During the work — فحص أثناء التنفيذ | After the work — فحص بعد التنفيذ |
|---|---|---|
| Code / enum | `during` (`domain.StageDuring`) | `after` (`domain.StageAfter`) |
| When | During the work, inspector on site with the contractor | After the work is finished |
| Question | Is the contractor working the right way? | Is the result right and safe? |
| Practice it follows | In-process inspection (ISO 9001 §8.5.1 controlled conditions; ISO 45001 safe systems of work) | Post-maintenance inspection and testing before release (ISO 9001 §8.6; EN 13306) |
| Typical frequency | Sampled; may happen several times on long jobs | Expected for every job; repeats after a failure |
| Seed checklist | D1 safety & permits (30, **critical**), D2 approved procedure & checklist (20), D3 approved materials (15), D4 technician competence & calibrated instruments (15), D5 protection of site, nearby assets & environment (10), D6 arrival/response time (10) | A1 functional test after maintenance (30, **critical**), A2 safe return to service — locks off, guards back (15, **critical**), A3 checklist & readings (15), A4 workmanship (15), A5 site cleanliness (10), A6 documentation & WO closed in CMMS (15) |

**Never call them QA/QC** in UI, code or docs: the business doesn't. Full names: «فحص أثناء التنفيذ» / «فحص بعد التنفيذ»; short (pills, columns): «أثناء التنفيذ» / «بعد التنفيذ» (`Stage.Label()` / `Stage.Short()`).

Each stage's weights sum to 100. Admins can change them, and every change creates a new checklist **version**. Timeliness is deliberately **not** an after-work criterion: on-time rate is measured from `planned_date`/`actual_date`, so scoring it too would count it twice.

## Entities

```
work_orders 1──* inspections 1──* inspection_items
            1──* findings
inspections 1──* attachments (photos)
checklist_templates 1──* template_criteria
```

- **work_order**: the CMMS job. `ref` is unique (case-insensitive) and `type` is `PM` (صيانة وقائية) or `WO` (أمر عمل). It also holds site, **system** (`systems`, shown as **المنظومة**: the facility or programme, e.g. منشأة الجمرات), **asset group** (`asset_groups`, shown as **مجموعة الأصل**: the kind of equipment, e.g. المبردات), asset, contractor, technician, **priority** (`normal`|`high`|`emergency` → عادي/عالي/طارئ, required, default `normal`), `planned_date` (PM schedule date or WO SLA due date), `actual_date` and `status`.
- **lookups**: `sites`, `systems`, `asset_groups` and `contractors` are independent admin-managed lists (Settings), each with `active` and `sort`. Excel import creates names it doesn't know. Systems and asset groups are deliberately not nested, so admins can organise both freely.
- **inspection**: `stage` (`during`|`after`) and `attempt` (1, 2, …; unique per WO and stage). It points to the exact `template_id` version it used and has a `status` of `draft` or `submitted`. `score`, `passed` and `critical_failed` are set on submit, and at most one draft per stage per WO is allowed.
- **inspection_item**: one row per criterion of the template, holding a `rating` (`C`/`P`/`N`/`NA`, or null for unrated) and a `note`.
- **finding** (NCR): `description`, `corrective_action`, `owner`, `due_date`, `severity` (`normal`|`critical`) and `status` (`open` → `in_progress` → `closed`, with a `closure_note`). It can link to an inspection and a criterion.
- **checklist_template**: `stage`, `version` (unique per stage) and `active` (exactly one active per stage).
- **attachment**: a photo on an inspection, optionally tied to a criterion, stored through `storage.Store`.
- **audit_log**: who did what, and when, for every mutation.

## Rules (all in `internal/domain`)

**Scoring** (`domain.Evaluate`), the formula Kidana already uses:
```
score  = Σ(weight × factor) ÷ Σ(weight of rated, applicable criteria) × 100   (1 decimal)
factor = C 1 · P 0.5 · N 0 · NA → excluded from numerator and denominator
passed = score ≥ 80 AND no critical criterion rated N
```
Grades (`domain.Grades`): ممتاز ≥ 90, جيد جداً ≥ 80, جيد ≥ 70, مقبول ≥ 60, ضعيف below.

**Submit validation** (`domain.ValidateSubmit`):
- every criterion must be rated;
- they cannot all be "لا ينطبق";
- a failed result needs at least one finding on that inspection.

**Acceptance** (`domain.NextWOStatus`, applied by `web.refreshWorkOrderStatus`):
- `cancelled` is sticky;
- the latest submitted after-work inspection passed and there are 0 open findings → `accepted` (shown as **معتمد**);
- `accepted` but the rule no longer holds → `completed`;
- `open` with any submitted inspection → `in_progress`;
- otherwise the status stays as it is. Users may set open, in_progress, completed or cancelled by hand, never `accepted`.

**Verdict** (`domain.WOVerdict`), shown on the printed record: ملغى (cancelled) · معتمد (accepted) · غير مطابق — بانتظار إعادة الفحص (latest after-work failed) · مطابق — بانتظار إغلاق الملاحظات (passed, findings open) · بانتظار الفحص بعد التنفيذ (not inspected after yet).

**Delay** (`domain.DelayDays`): `actual_date − planned_date` in days, where positive means late. On-time rate is counted per work order.

**Dashboard insights** (`domain.Insights`): observations and recommendations, including during-vs-after gap insights and critical-safety alerts. Risk thresholds: weakest criterion < 85%, sites or contractors below 80%, overdue findings, repeat failures on the same asset (≥ 2, which suggests a root-cause analysis), on-time rate < 90%, and a during−after gap ≥ 10 points.

**Report scope**: the dashboard, print report and Excel export share one set of filters (stage, type, site, system, asset group, contractor, result, date range; query keys `system` and `group`). Inspection-level filters apply to scores and charts. Finding counts and the open-findings table follow the work-order filters (site, system, asset group, contractor, type) and always show findings that are open *now*.

## Roles

| Role (`user_role`) | Label | Can |
|---|---|---|
| admin | مدير النظام | everything + settings + re-open submitted inspections |
| inspector | مدقق | create/edit work orders, run inspections, record and close findings, import/export |
| viewer | مشاهد | read only |

**Deleting**: only admins can delete a work order, and only before any inspection is submitted (a mistaken entry). After that it is a quality record: set it to `cancelled` instead. Draft inspections and findings can be deleted by editors; findings on submitted inspections must be closed, not deleted. There is no "clear all".

Check roles in the UI with `appctx.CanEdit(ctx)` / `appctx.IsAdmin(ctx)`, and enforce them on routes with `s.requireRole(...)` (see `web/server.go`).

## Glossary (use these exact Arabic terms)

| English | Arabic |
|---|---|
| work order | أمر عمل |
| preventive maintenance | صيانة وقائية |
| during-work inspection | فحص أثناء التنفيذ (short: أثناء التنفيذ). Not "رقابة", not QA |
| after-work inspection | فحص بعد التنفيذ (short: بعد التنفيذ). Not QC |
| system (facility / programme) | المنظومة (plural المنظومات) |
| asset group | مجموعة الأصل (plural مجموعات الأصول). Replaces the old "النظام" |
| priority: normal / high / emergency | الأولوية: عادي / عالي / طارئ |
| inspection | فحص |
| criterion / item | بند / معيار |
| compliant / partial / non-compliant / N/A | مطابق / مطابق جزئياً (جزئي) / غير مطابق / لا ينطبق |
| finding | ملاحظة |
| corrective action | الإجراء التصحيحي |
| owner (remediation) | مسؤول المعالجة |
| target close date | تاريخ الإغلاق المستهدف |
| critical | حرج / حرجة |
| accepted (work-order status) | معتمد. Not "مقبول", which is the 60–69 grade band |
| attempt | محاولة |
| inspector | المدقق |
| contractor / technician | المقاول / الفني |
| site / detailed location | الموقع / الموقع التفصيلي |
| asset | الأصل / المعدة |
| checklist | قائمة الفحص |
| score / grade | النتيجة / التقدير |

## Open business questions (don't decide them silently)

- Whether the during and after scores should be combined into one work-order score, and with what weighting. Today they are shown separately on purpose.
- Whether the during-work inspection becomes mandatory for certain priorities (e.g. طارئ) or systems.
- Per-system or per-asset-group checklists (e.g. an AC-specific after-work list). The schema allows adding this later.
