// Kidana Quality service worker: makes the app installable and keeps the
// pages an inspector opened usable without a connection. Saves made offline
// are queued by app.js (IndexedDB) and sent when the connection returns.
const VERSION = "kq-v1";
const SHELL = [
  "/offline",
  "/static/css/app.css",
  "/static/js/app.js",
  "/static/js/htmx.min.js",
  "/static/js/alpine-csp.min.js",
  "/static/img/kidana-logo.png",
  "/static/img/icon-192.png",
  "/static/fonts/avenir-next-arabic-400.woff2",
  "/static/fonts/avenir-next-arabic-500.woff2",
  "/static/fonts/avenir-next-arabic-700.woff2",
];
// Pages kept for offline use once visited.
const PAGES = /^\/(inspections\/\d+|work-orders\/\d+|my\/tasks|findings\/\d+\/edit)?$/;

self.addEventListener("install", (e) => {
  e.waitUntil(caches.open(VERSION).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (e) => {
  e.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
    .then(() => self.clients.claim()));
});

self.addEventListener("fetch", (e) => {
  const req = e.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;

  // Static files: cache first (they are versioned with ?v=), refresh in the background.
  if (url.pathname.startsWith("/static/")) {
    e.respondWith(caches.open(VERSION).then(async (c) => {
      const hit = await c.match(req, { ignoreSearch: true });
      const net = fetch(req).then((res) => { if (res.ok) c.put(req, res.clone()); return res; }).catch(() => hit);
      return hit || net;
    }));
    return;
  }

  // Pages: network first; a copy of the pages above is kept for offline use.
  if (req.mode === "navigate") {
    e.respondWith(fetch(req).then((res) => {
      if (res.ok && PAGES.test(url.pathname) && !res.redirected) {
        const copy = res.clone();
        caches.open(VERSION).then((c) => c.put(req, copy));
      }
      return res;
    }).catch(async () => (await caches.match(req)) || (await caches.match("/offline"))));
  }
});
