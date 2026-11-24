// Kidana Quality — client behaviour.
// Alpine runs in its CSP build (no eval), so components are registered here
// with Alpine.data() and templates only reference their properties/methods.
// htmx handles server round-trips; Alpine handles local UI state.

document.addEventListener("alpine:init", () => {
  // ------------------------------------------------------------------ inspection flow
  // Reads criteria weights from data-criteria (JSON) and keeps a live score.
  // The server recomputes the score on save; this is only a preview.
  Alpine.data("inspection", () => ({
    step: 0,
    total: 0,
    criteria: [], // [{id, weight, critical, rating}]

    init() {
      this.criteria = JSON.parse(this.$el.dataset.criteria || "[]");
      this.total = this.criteria.length;
      const firstUnrated = this.criteria.findIndex((c) => !c.rating);
      this.step = firstUnrated === -1 ? this.total : firstUnrated; // all rated → review
      this.$watch("step", (v) => {
        window.scrollTo({ top: 0, behavior: "smooth" });
        // Re-render the review step from the server so it reflects autosaved ratings.
        if (v >= this.total) htmx.trigger("#review-body", "refresh");
      });
    },

    // Called from a rating radio's change event (data-criterion on the input).
    rate(ev) {
      const id = Number(ev.target.dataset.criterion);
      const c = this.criteria.find((x) => x.id === id);
      if (c) c.rating = ev.target.value;
    },

    get result() {
      let earned = 0, possible = 0, rated = 0, criticalFailed = false;
      for (const c of this.criteria) {
        if (!c.rating) continue;
        rated++;
        if (c.critical && c.rating === "N") criticalFailed = true;
        const f = { C: 1, P: 0.5, N: 0 }[c.rating];
        if (f === undefined) continue;
        possible += c.weight;
        earned += c.weight * f;
      }
      const score = possible ? Math.round((earned / possible) * 1000) / 10 : null;
      return { score, rated, criticalFailed, passed: score !== null && score >= 80 && !criticalFailed };
    },
    get scoreText() { return this.result.score === null ? "—" : this.result.score + "%"; },
    get ratedText() { return this.result.rated + " / " + this.total; },
    get isPass() { return this.result.score !== null && this.result.passed; },
    get isFail() { return this.result.score !== null && !this.result.passed; },
    get criticalFailed() { return this.result.criticalFailed; },
    get progressWidth() { return "width:" + Math.round((this.result.rated / Math.max(this.total, 1)) * 100) + "%"; },
    get onReview() { return this.step >= this.total; },
    get atStart() { return this.step === 0; },
    get stepText() { return this.onReview ? "المراجعة والإرسال" : "البند " + (this.step + 1) + " من " + this.total; },

    next() { if (this.step < this.total) this.step++; },
    prev() { if (this.step > 0) this.step--; },
    goReview() { this.step = this.total; },
    goTo(ev) { this.step = Number(ev.currentTarget.dataset.step); },
  }));

  // One criterion card inside the inspection flow; visible when it is the current step.
  Alpine.data("criterionCard", () => ({
    index: 0,
    init() { this.index = Number(this.$el.dataset.index); },
    get visible() { return this.step === this.index; },
  }));

  // Review panel visibility.
  Alpine.data("reviewPanel", () => ({
    get visible() { return this.onReview; },
  }));

  // ------------------------------------------------------------------ dates
  // Under a native date input, shows the chosen date as dd/mm/yyyy plus the
  // Umm al-Qura Hijri date (read-only). Native inputs display in the browser's
  // own locale format, so this line is the consistent reference.
  const hijriFmt = (() => {
    try {
      return new Intl.DateTimeFormat("en-u-ca-islamic-umalqura-nu-latn", { day: "2-digit", month: "2-digit", year: "numeric", timeZone: "UTC" });
    } catch (_) { return null; }
  })();
  // Gregorian dd/mm/yyyy in g, Umm al-Qura digits dd/mm/yyyy in h ("هـ" is in the markup).
  Alpine.data("dateEcho", () => ({
    g: "",
    h: "",
    init() {
      const input = this.$el.querySelector("input[type=date]");
      if (input) this.format(input.value);
    },
    echo(ev) { this.format(ev.target.value); },
    format(iso) {
      if (!iso) { this.g = ""; this.h = ""; return; }
      const [y, m, d] = iso.split("-");
      this.g = d + "/" + m + "/" + y;
      this.h = "";
      if (hijriFmt) {
        // formatToParts gives clean digits without bidi control marks.
        const parts = hijriFmt.formatToParts(new Date(Date.UTC(+y, +m - 1, +d, 12)));
        const get = (t) => (parts.find((p) => p.type === t) || {}).value || "";
        const hy = get("year").replace(/\D/g, "");
        if (hy) this.h = get("day") + "/" + get("month") + "/" + hy;
      }
    },
  }));

  // ------------------------------------------------------------------ generic helpers
  // Simple disclosure (menus, inline forms).
  Alpine.data("disclosure", () => ({
    open: false,
    init() { this.open = this.$el.dataset.open === "true"; },
    toggle() { this.open = !this.open; },
    close() { this.open = false; },
  }));

  // Confirmation step built into the page (no window.confirm in the flow).
  Alpine.data("confirmAction", () => ({
    asking: false,
    ask() { this.asking = true; },
    cancel() { this.asking = false; },
  }));
});

// ------------------------------------------------------------------ htmx glue
// Show a toast when the server sends an HX-Trigger: {"toast": {"message": "...", "tone": "ok"}}
document.body.addEventListener("toast", (e) => {
  const el = document.getElementById("toast");
  if (!el) return;
  const { message, tone } = e.detail || {};
  el.textContent = message || "";
  el.dataset.tone = tone || "";
  el.hidden = false;
  clearTimeout(el._t);
  el._t = setTimeout(() => { el.hidden = true; }, 3500);
});

// Treat 422 responses as swappable (validation errors re-render the form).
document.body.addEventListener("htmx:beforeSwap", (e) => {
  if (e.detail.xhr.status === 422) {
    e.detail.shouldSwap = true;
    e.detail.isError = false;
  }
});
