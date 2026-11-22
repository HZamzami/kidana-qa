# CLAUDE.md

Kidana QA: maintenance quality system (Arabic, RTL). Work orders get **QA** (during work) and **QC** (after work) inspections, plus findings. See README.md and docs/ARCHITECTURE.md.

## Environment

* The host is Bluefin (immutable Fedora). **No Go/Node on the host**: run every tool through Docker via `just` (see Justfile).
* `just up` starts the stack with hot reload on :8080. Test with `just test` and lint with `just lint`.
* Bind mounts need `:z` (SELinux). Containers run as the host UID through `HOST_UID`/`HOST_GID`, which the Justfile exports.

## Rules

* After editing `.templ` or `internal/db/queries/*.sql`, run `just gen`. Generated files (`*_templ.go`, `internal/db/gen`) are committed.
* Schema changes always go in a new goose migration. Never edit an applied one.
* Business logic goes in `internal/domain` (pure, tested). Handlers in `internal/web` stay thin.
* Alpine uses its **CSP build**: register components in `web/static/js/app.js` and never use inline JS. Don't use htmx `hx-on` attributes either, because eval is disabled.
* UI text is Arabic. Put enum labels in `internal/domain/labels.go`. Counts go through `domain.Count`, percentages in text through `domain.Pct`/`PctVal`.
* Dates: show dd/mm/yyyy plus read-only Umm al-Qura Hijri (`@DateText`/`@DateInline`, `internal/dates`). Store ISO.
* The project skill `.claude/skills/kidana-qa` has the full design system and recipes. Verify UI work with `just ui-check`.
* Colours: use only the Kidana tokens in `web/styles/input.css` (lighter tints only).
