# Domain: what the app models and why

## The problem

Kidana (شركة كدانة للتنمية والتطوير) runs maintenance at the holy sites (منى، عرفات، مزدلفة، مكة) through contractors. Its Operations Quality department (إدارة جودة العمليات, قطاع العمليات) must prove that the work is done **safely and correctly**. The earlier tool scored each job once against one mixed checklist and kept data only in the browser. This app splits the two questions the business really asks:

| | QA — رقابة أثناء التنفيذ | QC — فحص بعد التنفيذ |
|---|---|---|
| When | During the work, inspector on site with the contractor | After the work is finished |
| Question | Is the contractor working the right way? | Is the result right? |
| Typical frequency | Sampled; may happen several times on long jobs | Expected for every job; repeats after a failure |
| Seed checklist | Q1 safety (30, **critical**), Q2 procedure/checklist (20), Q3 approved materials (20), Q4 technician competence & tools (15), Q5 arrival/SLA (15) | C1 technical quality (35), C2 checklist & readings (15), C3 documentation & photos (15), C4 site cleanliness (10), C5 WO closed in CMMS (10), C6 on-time completion (15) |

Each stage's weights sum to 100. Admins can change them, and every change creates a new checklist **version**.

## Entities

```
work_orders 1──* inspections 1──* inspection_items
            1──* findings
inspections 1──* attachments (photos)
checklist_templates 1──* template_criteria
```

- **work_order**: the CMMS job. `ref` is unique (case-insensitive) and `type` is `PM` (صيانة وقائية) or `WO` (أمر عمل). It also holds site, system, asset, contractor, technician, priority, `planned_date` (PM schedule date or WO SLA due date), `actual_date` and `status`.
- **inspection**: `stage` (`qa`|`qc`) and `attempt` (1, 2, …; unique per WO and stage). It points to the exact `template_id` version it used and has a `status` of `draft` or `submitted`. `score`, `passed` and `critical_failed` are set on submit, and at most one draft per stage per WO is allowed.
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
- the latest submitted QC passed and there are 0 open findings → `accepted` (shown as **معتمد**);
- `accepted` but the rule no longer holds → `completed`;
- `open` with any submitted inspection → `in_progress`;
- otherwise the status stays as it is. Users may set open, in_progress, completed or cancelled by hand, never `accepted`.

**Delay** (`domain.DelayDays`): `actual_date − planned_date` in days, where positive means late. On-time rate is counted per work order.

**Dashboard insights** (`domain.Insights`): observations and recommendations, including QA-vs-QC gap insights and critical-safety alerts. Risk thresholds: weakest criterion < 85%, sites or contractors below 80%, overdue findings, repeat failures on the same asset (≥ 2, which suggests a root-cause analysis), on-time rate < 90%, and a QA−QC gap ≥ 10 points.

## Roles

| Role (`user_role`) | Label | Can |
|---|---|---|
| admin | مدير النظام | everything + settings + re-open submitted inspections |
| inspector | مدقق | create/edit work orders, run inspections, record and close findings, import/export |
| viewer | مشاهد | read only |

Check roles in the UI with `appctx.CanEdit(ctx)` / `appctx.IsAdmin(ctx)`, and enforce them on routes with `s.requireRole(...)` (see `web/server.go`).

## Glossary (use these exact Arabic terms)

| English | Arabic |
|---|---|
| work order | أمر عمل |
| preventive maintenance | صيانة وقائية |
| QA (during work) | رقابة أثناء التنفيذ |
| QC (after work) | فحص بعد التنفيذ |
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

- Whether QA and QC should be combined into one work-order score, and with what weighting. Today they are shown separately on purpose.
- Whether QA becomes mandatory for certain priorities or systems.
- Per-system checklists, e.g. an AC-specific QC. The schema allows adding this later.
