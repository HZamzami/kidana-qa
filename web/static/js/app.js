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

  // Work order print page: opens the print dialog once fonts and images are
  // ready (data-auto="1"), and backs the toolbar's print button.
  Alpine.data("printPage", () => ({
    init() {
      if (this.$el.dataset.auto !== "1") return;
      const images = [...document.images].filter((i) => !i.complete)
        .map((i) => new Promise((ok) => { i.onload = i.onerror = ok; }));
      Promise.all([document.fonts.ready, ...images]).then(() => setTimeout(() => window.print(), 250));
    },
    print() { window.print(); },
  }));

  // Row selection for bulk actions in a list: checkboxes carry
  // x-on:change="recount", the header box x-on:change="toggleAll".
  Alpine.data("bulkSelect", () => ({
    count: 0,
    boxes() { return [...this.$root.querySelectorAll('input[name="ids"]')]; },
    recount() { this.count = this.boxes().filter((b) => b.checked).length; },
    toggleAll(ev) { for (const b of this.boxes()) b.checked = ev.target.checked; this.recount(); },
    get selectedText() { return (document.documentElement.lang === "en" ? "Selected: " : "المحدد: ") + this.count; },
  }));

  // ---------------------------------------------------------------- field work
  // Proof of presence: asks for the location once when an inspection opens,
  // stores it on the inspection, and keeps it for photos taken afterwards.
  Alpine.data("geoStamp", () => ({
    init() {
      if (!navigator.geolocation) return;
      navigator.geolocation.getCurrentPosition((pos) => {
        const c = pos.coords;
        window.__geo = { lat: c.latitude, lng: c.longitude, accuracy: Math.round(c.accuracy) };
        if (this.$el.dataset.done === "1") return;
        const body = new FormData();
        body.append("lat", c.latitude); body.append("lng", c.longitude); body.append("accuracy", Math.round(c.accuracy));
        fetch(this.$el.dataset.url, { method: "POST", body, credentials: "same-origin" }).catch(() => {});
      }, () => {}, { enableHighAccuracy: true, timeout: 15000, maximumAge: 60000 });
    },
  }));

  // A canvas to sign on with a finger or mouse; the PNG goes into a hidden input.
  Alpine.data("signaturePad", () => ({
    drawing: false,
    init() {
      const c = this.$refs.pad;
      const fit = () => { const r = c.getBoundingClientRect(); c.width = r.width * 2; c.height = r.height * 2; this.ctx2d(); };
      requestAnimationFrame(fit);
      const at = (e) => { const r = c.getBoundingClientRect(); return [(e.clientX - r.left) * 2, (e.clientY - r.top) * 2]; };
      c.addEventListener("pointerdown", (e) => { this.drawing = true; c.setPointerCapture(e.pointerId); const [x, y] = at(e); this.g.beginPath(); this.g.moveTo(x, y); });
      c.addEventListener("pointermove", (e) => { if (!this.drawing) return; const [x, y] = at(e); this.g.lineTo(x, y); this.g.stroke(); });
      c.addEventListener("pointerup", () => { this.drawing = false; this.$refs.out.value = c.toDataURL("image/png"); });
    },
    ctx2d() { this.g = this.$refs.pad.getContext("2d"); this.g.lineWidth = 4; this.g.lineCap = "round"; this.g.lineJoin = "round"; this.g.strokeStyle = "#4C4F53"; },
    clear() { const c = this.$refs.pad; this.g.clearRect(0, 0, c.width, c.height); this.$refs.out.value = ""; },
  }));

  // Draw on a photo (circles, arrows) and save the drawn copy; the original stays.
  Alpine.data("photoAnnotate", () => ({
    saving: false, error: false, strokes: [],
    init() {
      const c = this.$refs.canvas;
      this.img = new Image();
      this.img.onload = () => { c.width = this.img.naturalWidth; c.height = this.img.naturalHeight; this.redraw(); };
      this.img.src = this.$el.dataset.src;
      const at = (e) => { const r = c.getBoundingClientRect(); return [(e.clientX - r.left) * c.width / r.width, (e.clientY - r.top) * c.height / r.height]; };
      let cur = null;
      c.addEventListener("pointerdown", (e) => { c.setPointerCapture(e.pointerId); cur = [at(e)]; this.strokes.push(cur); });
      c.addEventListener("pointermove", (e) => { if (!cur) return; cur.push(at(e)); this.redraw(); });
      c.addEventListener("pointerup", () => { cur = null; });
    },
    redraw() {
      const c = this.$refs.canvas, g = c.getContext("2d");
      g.drawImage(this.img, 0, 0);
      g.strokeStyle = "#A65A4E"; g.lineWidth = Math.max(4, c.width / 150); g.lineCap = "round"; g.lineJoin = "round";
      for (const s of this.strokes) { g.beginPath(); s.forEach(([x, y], i) => (i ? g.lineTo(x, y) : g.moveTo(x, y))); g.stroke(); }
    },
    undo() { this.strokes.pop(); this.redraw(); },
    save() {
      this.saving = true; this.error = false;
      this.$refs.canvas.toBlob((blob) => {
        const body = new FormData();
        body.append("image", blob, "annotated.jpg");
        fetch(this.$el.dataset.url, { method: "POST", body, credentials: "same-origin" })
          .then((r) => { if (!r.ok) throw new Error(r.status); window.location.href = this.$el.dataset.back; })
          .catch(() => { this.error = true; this.saving = false; });
      }, "image/jpeg", 0.88);
    },
  }));

  // Offline: banner with the number of saves waiting, and the submit guard.
  Alpine.data("offlineStatus", () => ({
    pending: 0, online: navigator.onLine,
    init() {
      const sync = () => { this.pending = window.kqQueue ? window.kqQueue.count : 0; this.online = navigator.onLine; };
      window.addEventListener("kq:queue", sync); window.addEventListener("online", sync); window.addEventListener("offline", sync);
      sync();
    },
    get visible() { return !this.online || this.pending > 0; },
    get text() {
      const en = document.documentElement.lang === "en";
      if (!this.online) return en ? `Offline — saves are kept on this phone and sent when the connection returns (${this.pending})` : `غير متصل — تُحفظ التعديلات على الجوال وتُرسل عند عودة الاتصال (${this.pending})`;
      return en ? `Sending saved changes… (${this.pending})` : `جارٍ إرسال التعديلات المحفوظة… (${this.pending})`;
    },
  }));
  Alpine.data("offlineGuard", () => ({
    pending: 0, online: navigator.onLine,
    init() {
      const sync = () => { this.pending = window.kqQueue ? window.kqQueue.count : 0; this.online = navigator.onLine; };
      window.addEventListener("kq:queue", sync); window.addEventListener("online", sync); window.addEventListener("offline", sync);
      sync();
    },
    get blocked() { return !this.online || this.pending > 0; },
    get blockedText() {
      return document.documentElement.lang === "en"
        ? "Submit is available once you're online and every change has been sent."
        : "يتاح الإرسال عند عودة الاتصال وإرسال كل التعديلات المحفوظة.";
    },
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

// ------------------------------------------------------------------ offline queue
// Inspection saves (ratings, notes, photos) that fail for lack of a
// connection are kept in IndexedDB and replayed in order when it returns.
(() => {
  const DB = "kidana-queue", STORE = "requests";
  const open = () => new Promise((ok, fail) => {
    const req = indexedDB.open(DB, 1);
    req.onupgradeneeded = () => req.result.createObjectStore(STORE, { keyPath: "id", autoIncrement: true });
    req.onsuccess = () => ok(req.result); req.onerror = () => fail(req.error);
  });
  const tx = async (mode, fn) => { const db = await open(); return new Promise((ok, fail) => {
    const t = db.transaction(STORE, mode); const r = fn(t.objectStore(STORE)); t.oncomplete = () => ok(r && r.result); t.onerror = () => fail(t.error);
  }); };
  const queue = window.kqQueue = { count: 0 };
  const announce = async () => { queue.count = await tx("readonly", (s) => s.count()) || 0; window.dispatchEvent(new Event("kq:queue")); };
  const queueable = (path) => /^\/inspections\/\d+\/(items\/\d+|header|photos)$/.test(path);

  queue.add = async (path, form) => {
    const entries = [];
    for (const [k, v] of form.entries()) entries.push([k, v]); // File/Blob values store natively
    await tx("readwrite", (s) => s.add({ path, entries, at: Date.now() }));
    announce();
  };
  let replaying = false;
  queue.replay = async () => {
    if (replaying || !navigator.onLine) return;
    replaying = true;
    try {
      const all = await tx("readonly", (s) => s.getAll()) || [];
      for (const item of all) {
        const body = new FormData();
        for (const [k, v] of item.entries) body.append(k, v);
        let res;
        try { res = await fetch(item.path, { method: "POST", body, credentials: "same-origin" }); } catch { break; } // still offline
        if (res.ok || (res.status >= 400 && res.status < 500)) await tx("readwrite", (s) => s.delete(item.id)); // 4xx: drop, it can never succeed
        else break;
      }
    } finally { replaying = false; announce(); }
  };

  // Catch htmx saves that never reached the server.
  document.body.addEventListener("htmx:sendError", (e) => {
    const cfg = e.detail.requestConfig || {};
    if (cfg.verb !== "post" || !queueable(cfg.path)) return;
    const form = cfg.formData instanceof FormData ? cfg.formData : new FormData(e.detail.elt.closest("form") || undefined);
    queue.add(cfg.path, form);
  });
  // Photos carry the location and device time when known.
  document.body.addEventListener("htmx:configRequest", (e) => {
    if (!/\/photos$/.test(e.detail.path)) return;
    const p = e.detail.parameters;
    p.captured_at = String(Date.now());
    if (window.__geo) { p.lat = window.__geo.lat; p.lng = window.__geo.lng; p.accuracy = window.__geo.accuracy; }
  });
  window.addEventListener("online", () => queue.replay());
  setInterval(() => queue.replay(), 20000);
  announce().then(() => queue.replay());

  // Installable app + offline pages.
  if ("serviceWorker" in navigator) navigator.serviceWorker.register("/sw.js").catch(() => {});
  // Signing out clears pages kept for offline use on this device.
  document.addEventListener("submit", (e) => {
    if (e.target.matches('form[action="/logout"]') && window.caches) caches.keys().then((ks) => ks.forEach((k) => caches.delete(k)));
  });
})();
