# CLAUDE.md

Kidana Quality: maintenance quality system (Arabic, RTL). Work orders get a **during-work** (فحص أثناء التنفيذ) and an **after-work** (فحص بعد التنفيذ) inspection, plus findings. Never call them QA/QC. Work orders carry المنظومة and مجموعة الأصل (admin-managed lists) and a priority of عادي/عالي/طارئ, and print to PDF one A4 record per work order. See README.md and docs/ARCHITECTURE.md.

## Environment

* The host is Bluefin (immutable Fedora). **No Go/Node on the host**: run every tool through Docker via `just` (see Justfile).
* `just up` starts the stack with hot reload on :8080. Test with `just test` and lint with `just lint`. `just seed` loads dummy data.
* Bind mounts need `:z` (SELinux). Containers run as the host UID through `HOST_UID`/`HOST_GID`, which the Justfile exports.

## Rules

* After editing `.templ` or `internal/db/queries/*.sql`, run `just gen`. Generated files (`*_templ.go`, `internal/db/gen`) are committed.
* Schema changes always go in a new goose migration. Never edit an applied one. (Exception: before the first production deploy, with the user's say-so, 00001/00002 may be rewritten; then `just nuke && just up && just seed`.)
* Business logic goes in `internal/domain` (pure, tested). Handlers in `internal/web` stay thin.
* Alpine uses its **CSP build**: register components in `web/static/js/app.js` and never use inline JS. Don't use htmx `hx-on` attributes either, because eval is disabled.
* UI text is Arabic. Put enum labels in `internal/domain/labels.go`. Counts go through `domain.Count`, percentages in text through `domain.Pct`/`PctVal`.
* Dates: show dd/mm/yyyy plus read-only Umm al-Qura Hijri (`@DateText`/`@DateInline`, `internal/dates`). Store ISO.
* The project skill `.claude/skills/kidana-quality` has the full design system and recipes. Verify UI work with `just ui-check`.
* Colours: use only the Kidana tokens in `web/styles/input.css` (lighter tints only).
