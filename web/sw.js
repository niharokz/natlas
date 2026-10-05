/*
 * Natlas service worker.
 *
 *   App shell (page, CSS, JS, icons) ... cached per release, so the app opens
 *                                        instantly and works offline.
 *   GET /api/* .......................... network first; the last good answer is
 *                                        kept and served only when offline (marked
 *                                        with an X-Natlas-Offline header so the app
 *                                        shows "offline, read-only").
 *   Everything else (saves, login) ...... never touched.
 *
 * Every respondWith() resolves to a real Response - never undefined - so a
 * failure can only cost a file, never stop the app from starting.
 * Escape hatch: open the site with ?nosw to unregister this worker.
 */
const VERSION = '{{VERSION}}';
const SHELL = 'natlas-shell-' + VERSION;
const API = 'natlas-api';
const v = (p) => p + '?v=' + VERSION;
const SHELL_FILES = [
  '/', '/manifest.webmanifest',
  v('/assets/app.js'), v('/assets/natlas.css'), v('/assets/vendor/nss.min.css'), v('/assets/vendor/alpine.min.js'),
  v('/assets/icons/favicon.png'), v('/assets/icons/apple-touch-icon.png'),
  '/assets/icons/icon-192.png', '/assets/icons/icon-512.png', '/assets/icons/maskable-512.png',
];

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(SHELL).then((c) => c.addAll(SHELL_FILES)).catch(() => {}));
});

self.addEventListener('message', (event) => {
  if (event.data === 'skipWaiting') self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    // drop older releases, including v1's "natlas-static-*" caches
    for (const key of await caches.keys()) {
      if (key !== SHELL && key !== API) await caches.delete(key);
    }
    await self.clients.claim();
  })());
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (req.method !== 'GET' || url.origin !== location.origin) return;

  if (url.pathname.startsWith('/api/')) {
    if (url.pathname === '/api/login' || url.pathname === '/api/logout') return;
    event.respondWith(apiFirst(req));
    return;
  }
  if (req.mode === 'navigate') {
    event.respondWith(networkFirst(req, '/'));
    return;
  }
  if (url.pathname.startsWith('/assets/') || url.pathname === '/manifest.webmanifest') {
    event.respondWith(cacheFirst(req));
  }
});

async function apiFirst(req) {
  try {
    const res = await fetch(req);
    if (res.ok) {
      const copy = res.clone();
      caches.open(API).then((c) => c.put(req, copy)).catch(() => {});
    }
    return res;
  } catch (err) {
    const cached = await caches.match(req, { cacheName: API });
    if (cached) {
      const headers = new Headers(cached.headers);
      headers.set('X-Natlas-Offline', '1');
      return new Response(await cached.blob(), { status: cached.status, headers });
    }
    return new Response(JSON.stringify({ error: 'Offline and nothing saved for this screen yet.' }),
      { status: 503, headers: { 'Content-Type': 'application/json', 'X-Natlas-Offline': '1' } });
  }
}

async function networkFirst(req, fallbackKey) {
  try {
    const res = await fetch(req);
    if (res.ok) {
      const copy = res.clone();
      caches.open(SHELL).then((c) => c.put(fallbackKey, copy)).catch(() => {});
    }
    return res;
  } catch (err) {
    return (await caches.match(fallbackKey, { cacheName: SHELL })) || fetch(req);
  }
}

async function cacheFirst(req) {
  const cached = await caches.match(req);
  if (cached) return cached;
  try {
    const res = await fetch(req);
    if (res.ok && new URL(req.url).searchParams.has('v')) {
      const copy = res.clone();
      caches.open(SHELL).then((c) => c.put(req, copy)).catch(() => {});
    }
    return res;
  } catch (err) {
    return fetch(req);
  }
}
