/*
 * Natlas web app - one Alpine component drives every screen.
 *
 * Nothing here knows about events, birthdays or inventory: the server sends
 * each plugin's definition (fields, list layout, actions) from plugin.yml and
 * this file renders it. Sections:
 *
 *   api ........ fetch wrapper (session expiry, offline, error messages)
 *   routing .... #/today, #/p/<plugin>/<collection>
 *   today ...... agenda + stats
 *   lists ...... search, filters, groups, summary, display formatting
 *   gestures ... swipe right = main action, swipe left = edit, long-press = select
 *   calendar ... month / week / schedule view for plugins with `view: calendar`
 *   document ... a Markdown note as numbered sections (`view: document`)
 *   charts ..... Today / Trends / Sleep / Body over a dated list (`charts:`)
 *   sheet ...... the edit form (new / edit / read-only)
 *   actions .... one-tap actions, badge menus, bulk edits, undo
 *   pwa ........ service worker, update prompt, theme
 *   phone ...... the Android app (window.NatlasNative): settings page, sync conflicts
 */

// A restored back/forward-cache page can start Alpine twice; reload instead.
window.addEventListener('pageshow', (e) => { if (e.persisted) location.reload(); });

const ICONS = {
  home: '<path d="M3 11l9-7 9 7"/><path d="M5 10v10h14V10"/>',
  calendar: '<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/>',
  wallet: '<rect x="3" y="6" width="18" height="14" rx="2"/><path d="M3 10h18M16 15h2"/>',
  gift: '<rect x="3" y="9" width="18" height="12" rx="1"/><path d="M12 9v12M3 13h18M12 9c-2-4-6-4-6-1s6 1 6 1zm0 0c2-4 6-4 6-1s-6 1-6 1z"/>',
  heart: '<path d="M12 20s-7-4.5-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.5-7 10-7 10z"/>',
  box: '<path d="M3 7l9-4 9 4v10l-9 4-9-4z"/><path d="M3 7l9 4 9-4M12 11v10"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  left: '<path d="M15 6l-6 6 6 6"/>',
  right: '<path d="M9 6l6 6-6 6"/>',
  link: '<path d="M14 4h6v6M20 4l-9 9M18 14v6H4V6h6"/>',
  book: '<path d="M4 5a2 2 0 0 1 2-2h14v15H6a2 2 0 0 0-2 2z"/><path d="M4 20a2 2 0 0 0 2 2h14v-4M8 7h8M8 11h6"/>',
  edit: '<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="M13 7l4 4"/>',
  toc: '<path d="M4 6h16M4 12h10M4 18h13"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1L7 17M17 7l2.1-2.1"/>',
  folder: '<path d="M3 6a1 1 0 0 1 1-1h5l2 2h9a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/>',
  list: '<path d="M9 6h11M9 12h11M9 18h11"/><circle cx="4.5" cy="6" r="1"/><circle cx="4.5" cy="12" r="1"/><circle cx="4.5" cy="18" r="1"/>',
  lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>',
  grid: '<rect x="4" y="4" width="7" height="7" rx="1.5"/><rect x="13" y="4" width="7" height="7" rx="1.5"/><rect x="4" y="13" width="7" height="7" rx="1.5"/><rect x="13" y="13" width="7" height="7" rx="1.5"/>',
  sync: '<path d="M20 12a8 8 0 0 1-13.7 5.7M4 12a8 8 0 0 1 13.7-5.7"/><path d="M18 3v4h-4M6 21v-4h4"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  search: '<circle cx="11" cy="11" r="6"/><path d="M20 20l-4.5-4.5"/>',
  refresh: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>',
  x: '<path d="M6 6l12 12M18 6L6 18"/>',
  more: '<circle cx="5" cy="12" r="1.3"/><circle cx="12" cy="12" r="1.3"/><circle cx="19" cy="12" r="1.3"/>',
  logout: '<path d="M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5L19 19M5 19l1.5-1.5M17.5 6.5L19 5"/>',
  moon: '<path d="M20 14A8 8 0 1 1 10 4a6 6 0 0 0 10 10z"/>',
  auto: '<circle cx="12" cy="12" r="8"/><path d="M12 4a8 8 0 0 1 0 16z" fill="currentColor"/>',
};

// Badge colours for common values; plugin.yml `tones:` can add more.
const TONES = {
  open: 'accent', pending: 'warn', closed: 'muted', done: 'ok', 'to do': 'warn',
  high: 'bad', medium: 'warn', low: 'muted', active: 'ok', cancelled: 'muted',
  premium: 'ok', excellent: 'ok', good: '', used: '', worn: 'warn', replace: 'bad',
};

// charts: parsed days per loaded list, and the points of each drawn chart (not reactive)
const CH = { days: new WeakMap(), pts: {} };

class ApiError extends Error {
  constructor(message, status, fields) { super(message); this.status = status; this.fields = fields || {}; }
}

document.addEventListener('alpine:init', () => {
  Alpine.data('natlas', () => ({
    // ── state ──────────────────────────────────────────────────────────
    ready: false, authed: false, user: '', busy: false,
    loginForm: { username: '', password: '' }, loginError: '',
    app: { title: 'Natlas', currency: '₹', locale: 'en-IN', plugins: [] },
    route: { view: 'today', plugin: null, coll: null },
    today: { agenda: {}, stats: [] }, todayLoaded: false,
    data: {},                         // "plugin/collection" -> {records, groups, options, note}
    offline: !navigator.onLine,
    search: '', filters: {}, showFilters: false, summaryBy: '', collapsed: {}, quick: '',
    selected: {}, menu: { open: false, x: 0, y: 0, options: [], current: '' },
    sheet: { coll: null, plugin: null, record: null, mode: 'edit', values: {}, initial: {}, errors: {}, error: '', conflict: false, saving: false },
    ask: { open: false, fields: [], values: {}, errors: {}, error: '', saving: false },
    // calendar view: anchor = an ISO day in the shown period, picked = the selected day
    cal: { view: 'month', anchor: '', picked: '', hidden: {}, days: 90 },
    // document view: active = id of the section at the top of the screen
    // charts (Health → Daily): tab, Trends metric/range, Sleep nights, Body range, tapped point
    ch: { tab: 'today', metric: '', range: '30', nights: 14, bodyRange: '90', tip: null },
    doc: { active: '', toc: false }, mdPreview: false, _mdCache: new Map(), _mermaid: null, phoneLog: '',
    toasts: [], moreOpen: false, theme: 'auto', updateReady: false,
    native: !!window.NatlasNative, phone: null,
    // device key (NATLAS_DEVICE_KEY): remembered = this browser stays signed in until logout
    device: { available: false, remembered: false, hours: 12, open: false, key: '', error: '', busy: false }, healthProgress: '', conflicts: { list: [], copies: 0, main: false, open: false, i: 0, busy: false },
    _press: null, _suppressClick: false, _undone: new Set(), _toastSeq: 0, _sw: null,

    get selecting() { return Object.keys(this.selected).length > 0; },

    async init() {
      this.theme = this._getPref('natlas_theme') || 'auto';
      this.ch.tab = this._getPref('natlas_ch_tab') || 'today';
      this.cal.view = this._getPref('natlas_cal_view') || (matchMedia('(min-width:700px)').matches ? 'month' : 'schedule');
      try { this.cal.hidden = JSON.parse(this._getPref('natlas_cal_hidden') || '{}') || {}; } catch (e) { this.cal.hidden = {}; }
      this.applyTheme();
      this.setupServiceWorker();
      if (this.native) { // events from the Android shell (folder picked, PIN set, …)
        window.natlasNativeEvent = (name, data) => this.onNative(name, data);
        if (this._getPref('natlas_motion') === 'off') document.documentElement.classList.add('no-motion');
      }
      window.addEventListener('hashchange', () => this.applyRoute());
      let spy = 0;
      window.addEventListener('scroll', () => { // highlight the section being read in the contents
        if (spy || !this.isDocument()) return;
        spy = requestAnimationFrame(() => { spy = 0; this.docSpy(); });
      }, { passive: true });
      window.addEventListener('popstate', () => {
        // the back button closes an open sheet instead of leaving the page
        if (this.$refs.sheet?.open && !history.state?.natlasSheet) this.$refs.sheet.close();
      });
      window.addEventListener('online', () => { this.offline = false; this.reload(); });
      window.addEventListener('offline', () => { this.offline = true; });
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible' && this.authed) this.reload(true);
      });
      try {
        const me = await this.api('GET', '/api/me');
        this.authed = me.authenticated;
        this.user = me.username || '';
        this.setDevice(me);
      } catch (e) { this.authed = false; }
      if (this.authed) await this.loadApp();
      this.ready = true;
    },

    async loadApp() {
      this.app = await this.api('GET', '/api/app');
      document.title = this.app.title;
      this.applyRoute();
    },

    // ── api ────────────────────────────────────────────────────────────

    async api(method, url, body) {
      if (method !== 'GET' && this.offline) throw new ApiError('You are offline - try again when connected.', 0);
      let res;
      try {
        res = await fetch(url, {
          method, credentials: 'same-origin',
          headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
          body: body !== undefined ? JSON.stringify(body) : undefined,
        });
      } catch (e) {
        this.offline = true;
        throw new ApiError('Cannot reach Natlas - check your connection.', 0);
      }
      if (res.headers.get('X-Natlas-Offline')) this.offline = true; // answered from the offline cache
      else if (method === 'GET' && this.offline && navigator.onLine) this.offline = false;
      const data = await res.json().catch(() => ({}));
      if (res.status === 401 && !url.endsWith('/api/login') && !url.endsWith('/api/device')) { // a wrong device key is not a lost session
        this.authed = false;
        throw new ApiError('Your session ended - please log in again.', 401);
      }
      if (!res.ok) throw new ApiError(data.error || res.statusText, res.status, data.fields);
      return data;
    },

    async login() {
      this.busy = true; this.loginError = '';
      try {
        const r = await this.api('POST', '/api/login', this.loginForm);
        this.user = r.username; this.authed = true; this.loginForm.password = '';
        this.api('GET', '/api/me').then(me => this.setDevice(me)).catch(() => {});
        await this.loadApp();
      } catch (e) { this.loginError = e.message; }
      this.busy = false;
    },

    setDevice(me) {
      this.device.available = !!me.device_key && !me.local;
      this.device.remembered = !!me.remembered;
      this.device.hours = me.session_hours || 12;
    },
    openDevice() {
      this.moreOpen = false;
      Object.assign(this.device, { open: true, key: '', error: '', busy: false });
      this.$nextTick(() => this.$refs.device.showModal());
    },
    async saveDevice() {
      this.device.busy = true; this.device.error = '';
      try {
        await this.api('POST', '/api/device', { key: this.device.key.trim() });
        this.device.remembered = true; this.device.key = '';
        this.toast('This device stays signed in until you log out');
      } catch (e) { this.device.error = e.message; }
      this.device.busy = false;
    },
    async forgetDevice() {
      this.device.busy = true;
      try {
        await this.api('DELETE', '/api/device');
        this.device.remembered = false;
        this.toast('Forgotten: this login now lasts ' + this.device.hours + ' hours');
      } catch (e) { this.device.error = e.message; }
      this.device.busy = false;
    },

    async logout() {
      await this.api('POST', '/api/logout', {}).catch(() => {});
      this.authed = false; this.moreOpen = false; this.data = {};
      if ('caches' in window) caches.delete('natlas-api').catch(() => {});
    },

    // ── routing ────────────────────────────────────────────────────────

    applyRoute() {
      const parts = location.hash.replace(/^#\/?/, '').split('/').map(decodeURIComponent);
      const prev = { ...this.route };
      let deep = null; // widget links: …/r/<id> opens a record, …/new a form, calendar/day/<date> a day
      if (parts[0] === 'p' && (parts[3] === 'r' || parts[3] === 'new' || parts[2] === 'day')) {
        deep = { kind: parts[2] === 'day' ? 'day' : parts[3], id: parts[2] === 'day' ? parts[3] : parts[4], coll: parts[2] };
        history.replaceState(history.state, '', '#/p/' + parts[1] + (parts[2] && parts[2] !== 'day' ? '/' + parts[2] : ''));
      }
      if (parts[0] === 'phone' && this.native) {
        this.route = { view: 'phone', plugin: null, coll: null };
        this.moreOpen = false;
        this.loadPhone();
        window.scrollTo(0, 0);
        return;
      }
      if (parts[0] === 'p' && this.plugin(parts[1])) {
        const p = this.plugin(parts[1]);
        let coll = p.collections.some(c => c.id === parts[2]) ? parts[2] : p.collections[0].id;
        if (p.view === 'calendar') coll = (p.collections.find(c => !c.readonly) || p.collections[0]).id; // adds go to the writable one
        if (p.view === 'document') coll = (p.collections.find(c => c.kind === 'list') || p.collections[0]).id;
        this.route = { view: 'plugin', plugin: p.id, coll };
      } else {
        this.route = { view: 'today', plugin: null, coll: null };
      }
      this.moreOpen = false;
      if (prev.plugin !== this.route.plugin || prev.coll !== this.route.coll || prev.view !== this.route.view) {
        this.search = ''; this.filters = {}; this.selected = {}; this.quick = '';
        this.showFilters = false;
        window.scrollTo(0, 0);
      }
      if (this.route.view === 'today') this.loadToday();
      else if (this.isCalendar()) {
        if (prev.plugin !== this.route.plugin) { this.cal.anchor = this.iso(new Date()); this.cal.picked = this.cal.anchor; this.cal.days = 90; }
        if (deep?.kind === 'day' && /^\d{4}-\d\d-\d\d$/.test(deep.id || '')) { this.cal.anchor = this.cal.picked = deep.id; if (this.cal.view === 'schedule') this.calSetView('month'); }
        this.loadCalendar().then(() => deep?.kind === 'new' && this.calNew(this.cal.picked));
      } else if (this.isDocument()) {
        this.doc.toc = false;
        this.loadCalendar(); // loads every collection of the plugin
      } else {
        const c = this.curColl();
        this.summaryBy = c.summary?.group_by?.[0] || '';
        if (c.charts) { this.ch.tip = null; this.chLoadGoals(); }
        this.load(this.route.plugin, this.route.coll).then(d => {
          if (deep?.kind === 'new') this.openNew(c);
          else if (deep?.kind === 'r') {
            const r = d?.records?.find(x => x._id === deep.id);
            if (r) this.openRecord(c, r); else this.toast('That record is gone - it may have been archived.', 'error');
          }
        });
      }
      if (deep?.kind === 'r' && (this.isCalendar() || this.isDocument())) {
        const c = this.collOf(this.route.plugin, deep.coll);
        this.load(this.route.plugin, deep.coll).then(d => { const r = d?.records?.find(x => x._id === deep.id); if (c && r) this.openRecord(c, r); });
      }
    },

    reload(quiet) {
      if (!this.authed) return;
      if (this.route.view === 'today') this.loadToday(!quiet);
      else if (this.route.view === 'phone') this.loadPhone();
      else if (this.isCalendar() || this.isDocument()) this.loadCalendar(true);
      else this.load(this.route.plugin, this.route.coll, true);
    },

    plugin(id) { return this.app.plugins.find(p => p.id === id); },
    collOf(pluginId, collId) { return this.plugin(pluginId)?.collections.find(c => c.id === collId); },
    curPlugin() { return this.plugin(this.route.plugin) || { id: '', title: '', collections: [] }; },
    curColl() { return this.collOf(this.route.plugin, this.route.coll) || { list: {}, fields: [], actions: [] }; },
    cur() { return this.data[this.route.plugin + '/' + this.route.coll]; },
    pluginOfColl(c) { return c ? this.app.plugins.find(p => p.collections.includes(c)) : null; },

    async load(pluginId, collId, force) {
      const key = pluginId + '/' + collId;
      if (this.data[key] && !force) { this._refresh(pluginId, collId); return this.data[key]; }
      return this._refresh(pluginId, collId);
    },

    async _refresh(pluginId, collId) {
      try {
        const d = await this.api('GET', `/api/p/${pluginId}/${collId}`);
        this.data[pluginId + '/' + collId] = d;
        return d;
      } catch (e) {
        if (e.status !== 401) this.toast(e.message, 'error');
      }
    },

    // ── today ──────────────────────────────────────────────────────────

    async loadToday(showToast) {
      try {
        this.today = await this.api('GET', '/api/dashboard');
        this.todayLoaded = true;
        this.loadConflicts();
      } catch (e) { if (showToast && e.status !== 401) this.toast(e.message, 'error'); }
    },

    todayTitle() {
      return new Date().toLocaleDateString(this.app.locale, { weekday: 'long', day: 'numeric', month: 'long' });
    },

    agendaSections() {
      const labels = { overdue: 'Overdue', today: 'Today', upcoming: 'Next 7 days' };
      return ['overdue', 'today', 'upcoming']
        .map(key => ({ key, label: labels[key], items: this.today.agenda?.[key] || [] }))
        .filter(s => s.items.length);
    },

    agendaSub(it) {
      const p = this.plugin(it.plugin);
      const when = it.days === undefined || it.days === null ? it.date : this.relDays(it.days);
      return [p?.title, when].filter(Boolean).join(' · ');
    },

    async openFromAgenda(it) {
      const c = this.collOf(it.plugin, it.collection);
      const d = await this.load(it.plugin, it.collection, true);
      const r = d?.records.find(x => x._id === it.id);
      if (r) this.openRecord(c, r);
      else this.toast('That record is gone - it may have been archived.', 'error');
    },

    async runAgendaAction(it, a) {
      const c = this.collOf(it.plugin, it.collection);
      const action = (c.actions || []).find(x => x.id === a.id);
      await this._doAction(c, { _id: it.id, _rev: it.rev, [c.list.title]: it.title }, action, {});
    },

    fmtWidget(w) {
      if (w.format === 'money') return this.money(w.value);
      return Number(w.value).toLocaleString(this.app.locale, { maximumFractionDigits: 2 });
    },


    // ── calendar ───────────────────────────────────────────────────────
    // Every collection of a `view: calendar` plugin is merged into one view;
    // read-only ones (a nightly sync) open read-only, the writable one takes adds.

    isCalendar() { return this.route.view === 'plugin' && this.curPlugin().view === 'calendar'; },

    loadCalendar(force) {
      const p = this.curPlugin();
      return Promise.all(p.collections.map(c => this.load(p.id, c.id, force)));
    },
    calLoaded() {
      const p = this.curPlugin();
      return p.collections.some(c => this.data[p.id + '/' + c.id]);
    },
    calNotes() {
      const p = this.curPlugin();
      return p.collections.map(c => this.data[p.id + '/' + c.id]?.note).filter(Boolean);
    },

    // calAll flattens every collection into {key, c, r, title, date, end, time, cat, cal}.
    calAll() {
      const p = this.curPlugin(), out = [];
      for (const c of p.collections) {
        const fDate = this.field(c, 'date'), fEnd = this.field(c, 'end');
        for (const r of this.data[p.id + '/' + c.id]?.records || []) {
          const d = this.parseDate(r.date, fDate?.format);
          if (!d) continue;
          const date = this.iso(d), e = r.end ? this.parseDate(r.end, fEnd?.format) : null;
          const end = e && this.iso(e) > date ? this.iso(e) : date;
          out.push({ key: c.id + '/' + r._id, c, r, date, end, title: this.text(r.title) || '(no title)',
            time: this.text(r.time), cat: this.text(r.category) || 'event', cal: this.text(r.calendar) || c.label,
            readonly: !!c.readonly });
        }
      }
      return out;
    },
    calEvents() {
      const q = this.search.trim().toLowerCase();
      return this.calAll().filter(e => !this.cal.hidden[e.cal] &&
        (!q || [e.title, e.cal, this.text(e.r.location), this.text(e.r.note)].some(t => t.toLowerCase().includes(q))));
    },
    // calCalendars lists the calendars for the show / hide chips
    calCalendars() {
      const counts = {};
      for (const e of this.calAll()) counts[e.cal] = (counts[e.cal] || 0) + 1;
      return Object.keys(counts).sort().map(name => ({ name, count: counts[name], on: !this.cal.hidden[name] }));
    },
    toggleCal(name) {
      this.cal.hidden = { ...this.cal.hidden, [name]: !this.cal.hidden[name] };
      if (!this.cal.hidden[name]) delete this.cal.hidden[name];
      this._setPref('natlas_cal_hidden', JSON.stringify(this.cal.hidden));
    },

    // calByDay buckets the shown events per ISO day (multi-day entries on every day).
    calByDay() {
      const by = {};
      for (const e of this.calEvents()) {
        const d = this._d(e.date), last = e.end;
        for (let i = 0; i < 400 && this.iso(d) <= last; i++, d.setDate(d.getDate() + 1)) (by[this.iso(d)] ??= []).push(e);
      }
      for (const list of Object.values(by)) list.sort((a, b) => (a.time ? 1 : 0) - (b.time ? 1 : 0) || a.time.localeCompare(b.time) || a.title.localeCompare(b.title));
      return by;
    },

    _d(iso) { const [y, m, d] = iso.split('-').map(Number); return new Date(y, m - 1, d); },
    _shift(iso, days) { const d = this._d(iso); d.setDate(d.getDate() + days); return this.iso(d); },
    _weekStart(iso) {
      const d = this._d(iso), first = this.curPlugin().week_start === 'sunday' ? 0 : 1;
      d.setDate(d.getDate() - ((d.getDay() - first + 7) % 7));
      return this.iso(d);
    },
    calWeekdays() {
      const start = this._weekStart(this.iso(new Date()));
      return [0, 1, 2, 3, 4, 5, 6].map(i => this._d(this._shift(start, i)).toLocaleDateString(this.app.locale, { weekday: 'short' }));
    },
    calDay(iso, by, month) {
      const d = this._d(iso), today = this.iso(new Date());
      return { iso, n: d.getDate(), events: by[iso] || [], today: iso === today, past: iso < today,
        out: month !== undefined && d.getMonth() !== month, picked: iso === this.cal.picked };
    },
    // calMonth returns the weeks (rows of 7 days) covering the anchor's month.
    calMonth() {
      const a = this._d(this.cal.anchor), month = a.getMonth(), by = this.calByDay();
      let day = this._weekStart(this.iso(new Date(a.getFullYear(), month, 1)));
      const weeks = [];
      do {
        weeks.push([0, 1, 2, 3, 4, 5, 6].map(i => this.calDay(this._shift(day, i), by, month)));
        day = this._shift(day, 7);
      } while (this._d(day).getMonth() === month);
      return weeks;
    },
    calMonthDays() { return this.calMonth().flat(); },
    calSub(e, iso) {
      return [this.calWhen(e, iso), e.cal, this.text(e.r.location)].filter(Boolean).join(' · ');
    },
    calWeek() {
      const start = this._weekStart(this.cal.anchor), by = this.calByDay();
      return [0, 1, 2, 3, 4, 5, 6].map(i => this.calDay(this._shift(start, i), by));
    },
    // calSchedule lists the days that have events, from the anchor on (Google's "Schedule").
    calSchedule() {
      const by = this.calByDay(), from = this.cal.anchor, to = this._shift(from, this.cal.days);
      return Object.keys(by).filter(d => d >= from && d < to).sort().map(iso => this.calDay(iso, by));
    },
    calPicked() { return this.calDay(this.cal.picked || this.cal.anchor, this.calByDay()); },

    calTitle() {
      const a = this._d(this.cal.anchor), L = this.app.locale;
      if (this.cal.view === 'month') return a.toLocaleDateString(L, { month: 'long', year: 'numeric' });
      if (this.cal.view === 'week') {
        const s = this._d(this._weekStart(this.cal.anchor)), e = this._d(this._shift(this.iso(s), 6));
        const same = s.getMonth() === e.getMonth();
        return s.toLocaleDateString(L, same ? { day: 'numeric' } : { day: 'numeric', month: 'short' }) + ' – ' +
          e.toLocaleDateString(L, { day: 'numeric', month: 'short', year: 'numeric' });
      }
      return 'From ' + a.toLocaleDateString(L, { day: 'numeric', month: 'short', year: 'numeric' });
    },
    calSetView(v) { this.cal.view = v; this.cal.days = 90; this._setPref('natlas_cal_view', v); },
    calMove(dir) {
      const v = this.cal.view;
      if (v === 'month') { const a = this._d(this.cal.anchor); this.cal.anchor = this.iso(new Date(a.getFullYear(), a.getMonth() + dir, 1)); }
      else this.cal.anchor = this._shift(this.cal.anchor, dir * (v === 'week' ? 7 : 30));
      this.cal.days = 90;
      if (v === 'month') {
        const p = this._d(this.cal.picked || this.cal.anchor), a = this._d(this.cal.anchor);
        if (p.getMonth() !== a.getMonth() || p.getFullYear() !== a.getFullYear()) this.cal.picked = this.cal.anchor;
      }
    },
    calToday() { this.cal.anchor = this.cal.picked = this.iso(new Date()); this.cal.days = 90; },
    // tap a day: first tap selects it, a second tap on the selected day adds an entry there
    calPick(day) {
      if (this.cal.picked === day.iso && this.calCanAdd()) return this.calNew(day.iso);
      this.cal.picked = day.iso;
    },
    calDateLabel(iso, long) {
      const d = this._d(iso);
      return d.toLocaleDateString(this.app.locale, long ? { weekday: 'long', day: 'numeric', month: 'long' } : { weekday: 'short', day: 'numeric', month: 'short' });
    },
    calTime(t) {
      const m = /^(\d{1,2}):(\d{2})/.exec(t || '');
      if (!m) return '';
      return new Date(2000, 0, 1, +m[1], +m[2]).toLocaleTimeString(this.app.locale, { hour: 'numeric', minute: '2-digit' });
    },
    calWhen(e, iso) {
      const parts = [];
      if (e.end !== e.date) parts.push(`${this.calDateLabel(e.date)} – ${this.calDateLabel(e.end)}` + (iso ? ` (day ${Math.round((this._d(iso) - this._d(e.date)) / 864e5) + 1})` : ''));
      parts.push(e.time ? this.calTime(e.time) : 'All day');
      return parts.join(' · ');
    },
    calCatClass(e) { return 'cat-' + String(e.cat).toLowerCase().replace(/[^a-z_]/g, ''); },

    calWritable() { return this.curPlugin().collections.find(c => !c.readonly) || null; },
    calCanAdd() { return !!this.calWritable() && !this.offline; },
    calNew(iso) {
      const c = this.calWritable();
      if (!c || this.offline) return;
      this.openNew(c);
      if (iso) this.sheet.values.date = iso;
    },
    calOpen(e) { this.openRecord(e.c, e.r); },

    // swipe left / right on the month or week grid changes the period
    calTouch(ev, phase) {
      const t = ev.changedTouches?.[0];
      if (!t) return;
      if (phase === 'start') { this._calX = t.clientX; this._calY = t.clientY; return; }
      const dx = t.clientX - (this._calX ?? t.clientX), dy = t.clientY - (this._calY ?? t.clientY);
      this._calX = null;
      if (Math.abs(dx) > 60 && Math.abs(dx) > 2 * Math.abs(dy)) this.calMove(dx < 0 ? 1 : -1);
    },

    // linkFor fills a collection's `link:` URL template from the record.
    linkFor(c, r) {
      if (!c?.link || !r) return '';
      return c.link.url.replace(/\{([^}]+)\}/g, (_, k) => encodeURIComponent(this.text(r[k])));
    },


    // ── document ───────────────────────────────────────────────────────
    // A Markdown note (format: markdown) shown like a handbook: a front record
    // (title, subtitle, reviewed, intro) and numbered "## " sections, each
    // editable on its own. Rendering: marked (GFM) + Obsidian-style callouts
    // (> [!note] Title) + ```mermaid diagrams (mermaid.js, loaded on demand).

    isDocument() { return this.route.view === 'plugin' && this.curPlugin().view === 'document'; },
    docColl(kind) { return this.curPlugin().collections.find(c => c.kind === kind); },
    docLoaded() { return !!this.data[this.curPlugin().id + '/' + this.docColl('list')?.id]; },
    docAll() { return this.data[this.curPlugin().id + '/' + this.docColl('list')?.id]?.records || []; },
    docFront() {
      const c = this.docColl('record');
      return c ? this.data[this.curPlugin().id + '/' + c.id]?.records?.[0] || null : null;
    },
    docSections() {
      const q = this.search.trim().toLowerCase();
      return this.docAll().map((r, i) => ({ r, n: String(i).padStart(2, '0') }))
        .filter(x => !q || (this.text(x.r.title) + '\n' + this.text(x.r.body)).toLowerCase().includes(q));
    },
    docGo(id) {
      this.doc.toc = false;
      document.getElementById('sec-' + id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      this.doc.active = id;
    },
    docSpy() {
      let active = '';
      for (const el of document.querySelectorAll('.doc-sec')) {
        if (el.getBoundingClientRect().top < 140) active = el.dataset.id; else break;
      }
      if (active !== this.doc.active) this.doc.active = active;
    },
    docEdit(r) { this.openRecord(this.docColl('list'), r); },
    docEditFront() {
      const c = this.docColl('record');
      if (c) this.openRecord(c, this.docFront() || { _id: 'record', _rev: '' });
    },
    // docAdd opens a new section; with a section given, it goes right after it
    docAdd(after) {
      const c = this.docColl('list');
      if (!c || this.offline) return;
      this.openNew(c);
      if (after) this.sheet.at = this.docAll().findIndex(r => r._id === after._id) + 1;
    },
    docMeta() {
      const f = this.docFront() || {}, out = [`${this.docAll().length} sections`];
      if (f.reviewed) out.push('reviewed ' + this.fmtDate(f.reviewed, ''));
      return out;
    },

    // md renders Markdown to HTML (cached by text).
    md(src) {
      src = this.text(src);
      if (!src) return '';
      let html = this._mdCache.get(src);
      if (html === undefined) {
        html = window.marked ? window.marked.parse(src, { gfm: true }) : src.replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
        if (this._mdCache.size > 300) this._mdCache.clear();
        this._mdCache.set(src, html);
      }
      return html;
    },
    // mdEnhance runs after the HTML is in place: callouts, wide tables, outside links, diagrams.
    mdEnhance(el) {
      if (!el) return;
      for (const bq of el.querySelectorAll('blockquote')) {
        const p = bq.firstElementChild;
        const m = p?.tagName === 'P' && /^\[!(\w+)\][+-]?\s*([^\n]*)/.exec(p.textContent);
        if (!m) continue;
        const kind = m[1].toLowerCase();
        const box = document.createElement('div');
        box.className = 'callout ' + ({ warning: 'warn', caution: 'warn', attention: 'warn', danger: 'bad', error: 'bad', bug: 'bad',
          tip: 'ok', success: 'ok', check: 'ok', done: 'ok', important: 'accent', info: 'accent' }[kind] || 'note');
        const head = document.createElement('p');
        head.className = 'callout-title';
        head.textContent = m[2] || kind[0].toUpperCase() + kind.slice(1);
        // the rest of the first paragraph (after its first line) stays as text
        const firstLine = p.innerHTML.split('\n');
        firstLine.shift();
        if (firstLine.join('').trim()) p.innerHTML = firstLine.join('\n'); else p.remove();
        box.append(head, ...bq.childNodes);
        bq.replaceWith(box);
      }
      for (const t of el.querySelectorAll('table')) {
        if (t.parentElement.classList.contains('table-wrap')) continue;
        const head = t.tHead; // "| | |" = a table without column titles
        if (head && ![...head.querySelectorAll('th')].some(th => th.textContent.trim())) head.remove();
        const w = document.createElement('div');
        w.className = 'table-wrap';
        t.replaceWith(w);
        w.append(t);
      }
      for (const a of el.querySelectorAll('a[href^="http"]')) { a.target = '_blank'; a.rel = 'noopener noreferrer'; }
      const diagrams = [];
      for (const code of el.querySelectorAll('pre > code.language-mermaid')) {
        const box = document.createElement('div');
        box.className = 'mermaid-box';
        box.dataset.src = code.textContent;
        code.parentElement.replaceWith(box);
        diagrams.push(box);
      }
      if (diagrams.length) this.drawDiagrams(diagrams);
    },
    async loadMermaid() {
      if (!this._mermaid) {
        this._mermaid = new Promise((ok, bad) => {
          const s = document.createElement('script');
          s.src = '/assets/vendor/mermaid.min.js?v=' + (document.querySelector('.app')?.dataset.version || '');
          s.onload = () => ok(window.mermaid);
          s.onerror = () => { this._mermaid = null; bad(new Error('diagram library did not load')); };
          document.head.append(s);
        });
      }
      return this._mermaid;
    },
    _dark() { return this.theme === 'dark' || (this.theme === 'auto' && !matchMedia('(prefers-color-scheme: light)').matches); },
    async drawDiagrams(boxes) {
      let mermaid;
      try { mermaid = await this.loadMermaid(); } catch (e) {
        for (const b of boxes) { b.textContent = b.dataset.src; b.classList.add('failed'); }
        return;
      }
      const css = getComputedStyle(document.documentElement), v = (k) => css.getPropertyValue(k).trim();
      mermaid.initialize({
        startOnLoad: false, securityLevel: 'strict', theme: 'base',
        fontFamily: v('--nss-sans') || 'system-ui, sans-serif',
        themeVariables: {
          darkMode: this._dark(), background: v('--nss-surface'), fontSize: '14px',
          primaryColor: v('--nss-surface-2'), primaryTextColor: v('--nss-fg'), primaryBorderColor: v('--nss-accent'),
          lineColor: v('--nss-muted'), secondaryColor: v('--nss-surface'), tertiaryColor: v('--nss-bg'),
          clusterBkg: v('--nss-bg'), clusterBorder: v('--nss-line'), edgeLabelBackground: v('--nss-surface'),
          noteBkgColor: v('--nss-surface-2'), noteTextColor: v('--nss-fg'), noteBorderColor: v('--nss-line'),
          actorBkg: v('--nss-surface-2'), actorBorder: v('--nss-accent'), actorTextColor: v('--nss-fg'),
          signalColor: v('--nss-muted'), signalTextColor: v('--nss-fg'),
          pie1: v('--nss-accent'), pie2: v('--nss-warn'), pie3: v('--nss-surface-2'), pie4: v('--nss-accent-2'),
          pie5: v('--nss-ok'), pie6: v('--nss-bad'), pieStrokeColor: v('--nss-bg'), pieOuterStrokeColor: v('--nss-line'),
          pieTitleTextColor: v('--nss-fg'), pieSectionTextColor: v('--nss-fg'), pieLegendTextColor: v('--nss-fg'),
          pieTitleTextSize: '16px', pieSectionTextSize: '13px', pieLegendTextSize: '13px',
          taskBkgColor: v('--nss-accent'), taskBorderColor: v('--nss-accent'), taskTextColor: v('--nss-fg'),
          taskTextLightColor: v('--nss-on-accent'), taskTextOutsideColor: v('--nss-fg'), sectionBkgColor: v('--nss-surface-2'),
          altSectionBkgColor: v('--nss-surface'), sectionBkgColor2: v('--nss-surface-2'), gridColor: v('--nss-line'),
          titleColor: v('--nss-fg'), milestoneBkgColor: v('--nss-warn'), milestoneBorderColor: v('--nss-warn'),
        },
      });
      for (const b of boxes) {
        if (!b.isConnected) continue;
        try {
          const { svg } = await mermaid.render('mmd-' + Math.random().toString(36).slice(2), b.dataset.src);
          b.innerHTML = svg;
          b.classList.remove('failed');
        } catch (e) {
          b.textContent = 'Diagram error: ' + (e?.message || e) + '\n\n' + b.dataset.src;
          b.classList.add('failed');
        }
      }
    },
    // redrawDiagrams re-colours every diagram on screen (after a theme change)
    redrawDiagrams() {
      const boxes = [...document.querySelectorAll('.mermaid-box')];
      if (boxes.length) this.drawDiagrams(boxes);
    },

    // ── lists ──────────────────────────────────────────────────────────

    activeFilterCount() {
      return Object.values(this.filters).filter(Boolean).length + (this.search.trim() ? 1 : 0);
    },
    clearFilters() { this.filters = {}; this.search = ''; },

    visibleRecords() {
      const d = this.cur();
      if (!d) return [];
      const c = this.curColl();
      const q = this.search.trim().toLowerCase();
      return d.records.filter(r => {
        for (const [k, v] of Object.entries(this.filters)) {
          if (v && this.text(r[k]) !== v) return false;
        }
        if (q) {
          const keys = c.list.search?.length ? c.list.search : [c.list.title];
          return keys.some(k => this.text(r[k]).toLowerCase().includes(q));
        }
        return true;
      });
    },

    groupedRecords() {
      const recs = this.visibleRecords();
      const groups = this.cur()?.groups;
      if (!groups?.length) return [{ label: '', collapsed: false, records: recs }];
      const byId = Object.fromEntries(recs.map(r => [r._id, r]));
      return groups
        .map(g => ({ label: g.label, collapsed: g.collapsed, records: g.ids.map(id => byId[id]).filter(Boolean) }))
        .filter(g => g.records.length);
    },

    isCollapsed(g) {
      if (this.activeFilterCount()) return false;
      const key = this.route.plugin + '/' + this.route.coll + '/' + g.label;
      return this.collapsed[key] ?? g.collapsed;
    },
    toggleGroup(g) {
      const key = this.route.plugin + '/' + this.route.coll + '/' + g.label;
      this.collapsed[key] = !this.isCollapsed(g);
    },

    summaryRows() {
      const c = this.curColl(), by = this.summaryBy, sums = c.summary?.sum || [];
      const rows = {};
      const total = { label: 'Total', count: 0, sums: {}, total: true };
      for (const r of this.visibleRecords()) {
        const label = this.text(r[by]) || '—';
        rows[label] ??= { label, count: 0, sums: {} };
        for (const row of [rows[label], total]) {
          row.count++;
          for (const k of sums) row.sums[k] = (row.sums[k] || 0) + (Number(r[k]) || 0);
        }
      }
      return [...Object.values(rows).sort((a, b) => a.label.localeCompare(b.label)), total];
    },

    field(c, key) { return c?.fields?.find(f => f.key === key); },
    fieldLabel(c, key) {
      if (!key) return '';
      const f = this.field(c, key);
      if (f) return f.label;
      const [base, part] = key.split('.');
      return (this.field(c, base)?.label || base) + (part ? ' ' + part : '');
    },
    visibleFields(c) {
      if (!c) return [];
      const isNew = this.sheet.coll === c && this.sheet.mode === 'new';
      // a read-only record in the sheet shows only the fields it has
      const r = c.readonly && this.sheet.coll === c ? this.sheet.record : null;
      return c.fields.filter(f => !f.hidden && !(isNew && f.readonly) && !(r && this.text(r[f.key]) === ''));
    },

    subtitle(c, r) {
      if (!c || !r) return '';
      return (c.list.subtitle || []).map(k => this.display(c, k, r)).filter(Boolean).join(' · ');
    },

    badges(c, r) {
      if (!c || !r) return [];
      return (c.list.badges || []).map(key => {
        const f = this.field(c, key);
        const text = this.display(c, key, r);
        if (!text) return null;
        let tone = f ? (f.tones?.[r[key]] ?? TONES[String(r[key]).toLowerCase()] ?? '') : '';
        if (key.endsWith('.days')) tone = r[key] === 0 ? 'warn' : r[key] <= 7 ? 'accent' : '';
        return { key, text, tone, editable: !!f && f.type === 'select' && !c.readonly && !f.readonly && !this.offline };
      }).filter(Boolean);
    },

    isMuted(c, r) {
      const first = this.badges(c, r)[0];
      return first && first.tone === 'muted';
    },

    // display formats one value for the list or the read-only view
    display(c, key, r) {
      const v = r?.[key];
      if (v === null || v === undefined || v === '') return '';
      if (key.endsWith('.days')) return this.relDays(v);
      if (key.endsWith('.age')) return 'turns ' + v;
      if (key.endsWith('.next')) return this.fmtDate(v, '');
      const f = this.field(c, key);
      switch (f?.type) {
        case 'date': return this.fmtDate(v, f.format);
        case 'time': return this.calTime(v) || this.text(v);
        case 'money': return this.money(v);
        case 'number': return Number(v).toLocaleString(this.app.locale, { maximumFractionDigits: 2 }) + (f.unit ? ' ' + f.unit : '');
        case 'bool': return v ? 'Yes' : 'No';
        case 'list': return Array.isArray(v) ? v.join(', ') : String(v);
        case 'anniversary': {
          const [y, m, d] = String(v).split('-').map(Number);
          if (!m || !d) return String(v);
          const label = new Date(2000, m - 1, d).toLocaleDateString(this.app.locale, { day: 'numeric', month: 'short' });
          return y ? `${label} ${y}` : label;
        }
      }
      return this.text(v);
    },
    fmtField(c, key, v) { return this.display(c, key, { [key]: v }) || '0'; },

    text(v) {
      if (v === null || v === undefined) return '';
      return Array.isArray(v) ? v.join(', ') : String(v);
    },
    money(v) {
      return this.app.currency + Number(v || 0).toLocaleString(this.app.locale, { maximumFractionDigits: 2 });
    },

    // ── dates ──────────────────────────────────────────────────────────

    // parseDate reads a stored date using its Go layout ("02 Jan 2006"); ISO always works.
    parseDate(value, layout) {
      const s = String(value || '').trim();
      let m = s.match(/^(\d{4})-(\d{2})-(\d{2})/);
      if (m) return new Date(+m[1], +m[2] - 1, +m[3]);
      if (!layout) return null;
      const order = [];
      const pattern = layout.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/2006|January|Jan|Monday|Mon|01|02|_2|1|2/g, (tok) => {
        order.push(tok);
        if (tok === '2006') return '(\\d{4})';
        if (/^(January|Jan|Monday|Mon)$/.test(tok)) return '([A-Za-z]+)';
        return '\\s?(\\d{1,2})';
      });
      m = s.match(new RegExp('^' + pattern + '$'));
      if (!m) return null;
      let y, mo, d;
      order.forEach((tok, i) => {
        const v = m[i + 1];
        if (tok === '2006') y = +v;
        else if (tok === 'January' || tok === 'Jan') mo = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'].indexOf(v.slice(0, 3).toLowerCase());
        else if (tok === '01' || tok === '1') mo = +v - 1;
        else if (tok === '02' || tok === '2' || tok === '_2') d = +v;
      });
      if (y === undefined || mo === undefined || mo < 0 || !d) return null;
      return new Date(y, mo, d);
    },
    iso(d) {
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    },
    daysFromToday(d) {
      const t = new Date(); t.setHours(0, 0, 0, 0);
      return Math.round((d - t) / 86400000);
    },
    relDays(n) {
      n = Number(n);
      if (n === 0) return 'today';
      if (n === 1) return 'tomorrow';
      if (n === -1) return 'yesterday';
      return n > 0 ? `in ${n} days` : `${-n} days ago`;
    },
    fmtDate(v, layout) {
      const d = this.parseDate(v, layout);
      if (!d) return String(v);
      const n = this.daysFromToday(d);
      if (n >= -1 && n <= 1) return this.relDays(n).replace(/^./, c => c.toUpperCase());
      if (n > 1 && n < 7) return d.toLocaleDateString(this.app.locale, { weekday: 'short', day: 'numeric', month: 'short' });
      const opts = { day: 'numeric', month: 'short' };
      if (d.getFullYear() !== new Date().getFullYear()) opts.year = 'numeric';
      return d.toLocaleDateString(this.app.locale, opts);
    },

    // ── gestures ───────────────────────────────────────────────────────

    pressStart(e, r) {
      if (e.pointerType === 'mouse' && e.button !== 0) return;
      if (e.target.closest('.badge, .act, .pick')) return;
      const row = e.currentTarget;
      this._press = { x: e.clientX, y: e.clientY, row, r, dx: 0, swiping: false, touch: e.pointerType !== 'mouse' };
      this._press.timer = setTimeout(() => {
        if (!this._press || this._press.swiping || this._press.long || this.curColl().readonly) return;
        this.toggleSelect(r);
        navigator.vibrate?.(15);
        this._press.long = true; // the click that ends this press must not toggle again
      }, 480);
    },
    pressMove(e) {
      const p = this._press;
      if (!p) return;
      const dx = e.clientX - p.x, dy = e.clientY - p.y;
      if (!p.swiping && Math.abs(dy) > 10 && Math.abs(dy) > Math.abs(dx)) { this.pressCancel(); return; }
      if (p.touch && !this.selecting && Math.abs(dx) > 12 && Math.abs(dx) > Math.abs(dy)) {
        clearTimeout(p.timer);
        p.swiping = true;
        p.dx = Math.max(-110, Math.min(110, dx));
        p.row.querySelector('.row-inner').style.transform = `translateX(${p.dx}px)`;
        p.row.classList.toggle('swipe-r', p.dx > 0);
        p.row.classList.toggle('swipe-l', p.dx < 0);
      }
    },
    pressEnd(e, r) {
      const p = this._press;
      this.pressCancel();
      if (p?.long) {
        this._suppressClick = true;
        setTimeout(() => { this._suppressClick = false; }, 0); // clears right after this press's click
        return;
      }
      if (!p || !p.swiping) return;
      this._suppressClick = true;
      setTimeout(() => { this._suppressClick = false; }, 50);
      const c = this.curColl();
      if (p.dx > 80) {
        const a = this.primaryAction(c, r);
        if (a) this.runAction(c, r, a);
      } else if (p.dx < -80) {
        this.openRecord(c, r);
      }
    },
    pressCancel() {
      const p = this._press;
      if (!p) return;
      clearTimeout(p.timer);
      const inner = p.row.querySelector('.row-inner');
      if (inner) inner.style.transform = '';
      p.row.classList.remove('swipe-r', 'swipe-l');
      this._press = null;
    },
    // Right-click selects on desktop. On phones a long-press also fires
    // contextmenu; the long-press timer already selected the row then.
    onContextMenu(r) {
      if (this._press?.long || this._press?.touch) return;
      this.toggleSelect(r);
    },
    rowClick(r) {
      if (this._suppressClick) { this._suppressClick = false; return; }
      if (this.selecting) this.toggleSelect(r);
      else this.openRecord(this.curColl(), r);
    },

    // ── selection & bulk ───────────────────────────────────────────────

    toggleSelect(r) {
      if (this.curColl().readonly || this.curColl().kind !== 'list') return;
      if (this.selected[r._id]) delete this.selected[r._id];
      else this.selected[r._id] = r;
      this.selected = { ...this.selected };
    },
    clearSelection() { this.selected = {}; },
    selectedCount() { return Object.keys(this.selected).length; },
    bulkActions() {
      // actions that apply to at least one selected record
      const picked = Object.values(this.selected);
      return (this.curColl().actions || []).filter(a => !a.move_to && !a.ask?.length && picked.some(r => r._actions?.includes(a.id)));
    },
    bulkFields() {
      const c = this.curColl();
      return (c.list.badges || []).map(k => this.field(c, k)).filter(f => f && f.type === 'select' && !f.readonly);
    },

    async bulkAction(a) {
      await this._bulk({ action: a.id }, a.label);
    },
    async bulkSet(key, value) {
      if (!value) return;
      await this._bulk({ values: { [key]: value } }, `${this.fieldLabel(this.curColl(), key)} → ${value}`);
    },
    async _bulk(body, label) {
      const c = this.curColl(), p = this.curPlugin();
      const items = Object.values(this.selected).map(r => ({ id: r._id, rev: r._rev }));
      try {
        const res = await this.api('POST', `/api/p/${p.id}/${c.id}/bulk`, { ...body, items });
        const ok = res.results.filter(x => !x.error);
        const skipped = res.results.length - ok.length;
        const undoItems = ok.map(x => ({ id: x.id, rev: x.record._rev, values: this._changedBack(c, x.before, x.record) }));
        this.selected = {};
        this.toast(`${label}: ${ok.length} updated` + (skipped ? `, ${skipped} skipped` : ''), skipped ? 'warn' : 'ok',
          ok.length ? () => this.api('POST', `/api/p/${p.id}/${c.id}/bulk`, { items: undoItems }) : null);
        await this.afterChange(p.id, c.id);
      } catch (e) { this.fail(e); }
    },

    // ── badge menu ─────────────────────────────────────────────────────

    openMenu(e, c, r, key) {
      const f = this.field(c, key);
      if (!f || f.type !== 'select' || c.readonly || f.readonly || this.offline) return;
      const rect = e.currentTarget.getBoundingClientRect();
      const options = this.optionsFor(c, f, r[key]);
      const height = options.length * 40 + 12;
      this.menu = {
        open: true, c, r, key, options, current: r[key],
        x: Math.max(8, Math.min(rect.left, window.innerWidth - 200)),
        y: rect.bottom + height > window.innerHeight - 8 ? Math.max(8, rect.top - height - 4) : rect.bottom + 4,
      };
    },
    async pickMenu(value) {
      const { c, r, key } = this.menu;
      this.menu.open = false;
      if (value === r[key]) return;
      await this.patch(c, r, { [key]: value }, `${this.fieldLabel(c, key)} → ${value}`);
    },

    optionsFor(c, f, current) {
      if (!c || !f) return [];
      const fromData = this.data[this.pluginOfColl(c)?.id + '/' + c.id]?.options?.[f.key];
      const list = [...(fromData || f.options || [])];
      if (current && !list.includes(current)) list.push(current);
      return list;
    },

    primaryAction(c, r) {
      if (!c || !r || c.readonly || this.offline) return null;
      return (c.actions || []).find(a => a.primary && r._actions?.includes(a.id)) || null;
    },

    // ── actions ────────────────────────────────────────────────────────

    async runAction(c, r, a) {
      if (a.confirm && !confirm(a.confirm)) return;
      if (a.ask?.length) return this.openAsk(c, r, a);
      await this._doAction(c, r, a, {});
    },

    async _doAction(c, r, a, values) {
      const p = this.pluginOfColl(c);
      try {
        const res = await this.api('POST', `/api/p/${p.id}/${c.id}/${encodeURIComponent(r._id)}/do/${a.id}`, { rev: r._rev, values });
        const title = this.text(r[c.list.title]) || 'record';
        let undo = null;
        if (!a.move_to) {
          const back = this._changedBack(c, res.before, res.record);
          undo = () => this.api('PATCH', `/api/p/${p.id}/${c.id}/${encodeURIComponent(res.record._id)}`, { rev: res.record._rev, values: back });
        }
        this.closeSheetQuiet();
        this.toast(`${a.label}: ${title}`, 'ok', undo);
        await this.afterChange(p.id, c.id);
        if (a.move_to) this.data[p.id + '/' + a.move_to] && this._refresh(p.id, a.move_to);
        return true;
      } catch (e) { this.fail(e, c); return false; }
    },

    async patch(c, r, values, label) {
      const p = this.pluginOfColl(c);
      try {
        const res = await this.api('PATCH', `/api/p/${p.id}/${c.id}/${encodeURIComponent(r._id)}`, { rev: r._rev, values });
        const back = this._changedBack(c, res.before, res.record);
        this.toast(label, 'ok', () => this.api('PATCH', `/api/p/${p.id}/${c.id}/${encodeURIComponent(res.record._id)}`, { rev: res.record._rev, values: back }));
        await this.afterChange(p.id, c.id);
      } catch (e) { this.fail(e, c); }
    },

    // _changedBack lists the old values of every field an edit changed (for Undo).
    _changedBack(c, before, after) {
      const back = {};
      for (const f of c.fields) {
        if (JSON.stringify(before?.[f.key] ?? null) !== JSON.stringify(after?.[f.key] ?? null)) {
          back[f.key] = before?.[f.key] ?? '';
        }
      }
      return back;
    },

    async afterChange(pluginId, collId) {
      if (this.route.view === 'today') await this.loadToday();
      await this._refresh(pluginId, collId);
    },

    fail(e, c) {
      if (e.status === 401) return;
      if (e.status === 409) {
        this.toast('Changed elsewhere (probably Taskmaster) - list reloaded, try again.', 'error');
        this.reload(true);
        return;
      }
      if (e.status === 422 && c) {
        const msgs = Object.entries(e.fields).map(([k, v]) => `${this.fieldLabel(c, k)} ${v}`);
        this.toast(msgs.join('; ') || e.message, 'error');
        return;
      }
      this.toast(e.message, 'error');
    },

    async quickAdd() {
      const c = this.curColl(), p = this.curPlugin();
      const text = this.quick.trim();
      if (!text) return;
      try {
        const res = await this.api('POST', `/api/p/${p.id}/${c.id}/quick`, { text });
        this.quick = '';
        const r = res.record;
        const when = c.quick_add.date && r[c.quick_add.date] ? ' · ' + this.display(c, c.quick_add.date, r) : '';
        this.toast(`Added: ${this.text(r[c.list.title])}${when}`, 'ok', () => this._delete(p, c, r));
        await this.afterChange(p.id, c.id);
      } catch (e) { this.fail(e, c); }
    },

    _delete(p, c, r) {
      return this.api('DELETE', `/api/p/${p.id}/${c.id}/${encodeURIComponent(r._id)}?rev=${r._rev}`);
    },

    // ── ask dialog ─────────────────────────────────────────────────────

    openAsk(c, r, a) {
      const p = this.pluginOfColl(c);
      const target = a.move_to ? this.collOf(p.id, a.move_to) : c;
      const fields = a.ask.map(k => this.field(target, k) || this.field(c, k)).filter(Boolean);
      this.ask = {
        open: true, c, r, action: a, fields, errors: {}, error: '', saving: false,
        title: this.text(r[c.list.title]),
        values: Object.fromEntries(fields.map(f => [f.key, ''])),
      };
      this.$nextTick(() => this.$refs.ask.showModal());
    },
    async submitAsk() {
      this.ask.saving = true; this.ask.errors = {}; this.ask.error = '';
      try {
        const { c, r, action } = this.ask;
        const p = this.pluginOfColl(c);
        const res = await this.api('POST', `/api/p/${p.id}/${c.id}/${encodeURIComponent(r._id)}/do/${action.id}`, { rev: r._rev, values: this.ask.values });
        this.$refs.ask.close();
        this.closeSheetQuiet();
        this.toast(`${action.label}: ${this.ask.title}`, 'ok');
        await this.afterChange(p.id, c.id);
        if (action.move_to) await this._refresh(p.id, action.move_to);
        void res;
      } catch (e) {
        if (e.status === 422) this.ask.errors = e.fields;
        else this.ask.error = e.message;
      }
      this.ask.saving = false;
    },

    // ── charts ─────────────────────────────────────────────────────────
    // A list with `charts:` (Health → Daily) gets Today / Trends / Sleep /
    // Body tabs drawn from its records, plus the plain list as "Days".
    // Charts are HTML (bars, labels) + SVG (lines); tap or hover a mark to read it.

    isCharts() { return this.route.view === 'plugin' && !!this.curColl().charts && !this.isCalendar() && !this.isDocument(); },
    chShowList() { return !this.curColl().charts || this.ch.tab === 'days'; },
    chTabs() {
      const ch = this.curColl().charts || {};
      return [['today', 'Today'], ['trends', 'Trends'], ch.sleep && ['sleep', 'Sleep'], ch.body?.length && ['body', 'Body'], ['days', 'Days']].filter(Boolean);
    },
    chSetTab(t) { this.ch.tab = t; this.ch.tip = null; this._setPref('natlas_ch_tab', t); },
    chLoadGoals() {
      const p = this.curPlugin(), g = this.curColl().charts?.goals;
      if (g) this.load(p.id, g);
    },

    // days with a parsed date, oldest first (cached per loaded list)
    chDays() {
      const c = this.curColl(), d = this.cur();
      if (!d?.records || !c.charts) return [];
      const key = Alpine.raw(d.records);
      if (CH.days.has(key)) return CH.days.get(key);
      const f = this.field(c, c.charts.date);
      const days = d.records.map(r => ({ r, date: this.parseDate(r[c.charts.date], f?.format) }))
        .filter(x => x.date).map(x => ({ ...x, iso: this.iso(x.date) }))
        .sort((a, b) => (a.iso < b.iso ? -1 : a.iso > b.iso ? 1 : 0));
      CH.days.set(key, days);
      return days;
    },
    chNum(v) { const n = v === null || v === undefined || v === '' ? NaN : Number(v); return Number.isFinite(n) ? n : null; },
    chMetric(field) { return [...(this.curColl().charts?.trends || []), ...(this.curColl().charts?.rings || []), ...(this.curColl().charts?.tiles || []), ...(this.curColl().charts?.body || [])].find(m => m.field === field) || { field, label: this.fieldLabel(this.curColl(), field), unit: this.field(this.curColl(), field)?.unit || '', decimals: 1, chart: 'bar' }; },
    chGoal(m) {
      const ch = this.curColl().charts;
      const rec = ch?.goals ? this.data[this.curPlugin().id + '/' + ch.goals]?.records?.[0] : null;
      const g = this.chNum(rec?.['goal_' + m.field]);
      return g > 0 ? g : (m.goal || 0);
    },
    chLast() { const d = this.chDays(); return d.length ? d[d.length - 1] : null; },
    chLatest(field) {
      const d = this.chDays();
      for (let i = d.length - 1; i >= 0; i--) { const v = this.chNum(d[i].r[field]); if (v !== null) return { v, iso: d[i].iso, date: d[i].date }; }
      return null;
    },
    // values of a field between two ISO days (inclusive)
    chRange(field, from, to) {
      return this.chDays().filter(x => x.iso >= from && x.iso <= to).map(x => ({ iso: x.iso, date: x.date, v: this.chNum(x.r[field]) })).filter(x => x.v !== null);
    },
    chAvg(list) { return list.length ? list.reduce((s, x) => s + x.v, 0) / list.length : null; },
    chShift(iso, days) { const d = this._d(iso); d.setDate(d.getDate() + days); return this.iso(d); },
    chEnd() { return this.chLast()?.iso || this.iso(new Date()); },

    // 7h 12m for hours, 9,840 for counts, 66.1 kg with decimals
    chFmt(m, v, noUnit) {
      if (v === null || v === undefined) return '—';
      if (m.unit === 'h') { const mins = Math.round(v * 60); return `${Math.floor(mins / 60)}h ${String(mins % 60).padStart(2, '0')}m`; }
      const d = m.decimals || 0;
      const s = Number(v).toLocaleString(this.app.locale, { minimumFractionDigits: d, maximumFractionDigits: d });
      if (noUnit || !m.unit) return s;
      return m.unit === '%' ? s + '%' : s + ' ' + m.unit;
    },
    chTick(v) {
      const a = Math.abs(v);
      if (a >= 1e5) return Math.round(v / 1e3) + 'K';
      if (a >= 1e3) return (v / 1e3).toFixed(a % 1e3 ? 1 : 0).replace(/\.0$/, '') + 'K';
      return String(Math.round(v * 10) / 10);
    },
    chDay(date, long) { return date.toLocaleDateString(this.app.locale, long ? { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' } : { weekday: 'short', day: 'numeric', month: 'short' }); },
    chWhen(iso) { return this.fmtDate(iso, ''); },

    // ── Today tab
    chRings() {
      return (this.curColl().charts?.rings || []).map(m => {
        const last = this.chLatest(m.field), goal = this.chGoal(m);
        const pct = last && goal ? last.v / goal : 0;
        return { m, last, goal, pct, svg: this.chRingSVG(Math.min(pct, 1), pct >= 1) };
      });
    },
    chRingSVG(p, done) {
      const r = 42, c = 2 * Math.PI * r, off = c * (1 - p);
      return `<svg viewBox="0 0 100 100" aria-hidden="true"><circle cx="50" cy="50" r="${r}" class="ring-track"/>` +
        `<circle cx="50" cy="50" r="${r}" class="ring-fill${done ? ' done' : ''}" stroke-dasharray="${c.toFixed(1)}" stroke-dashoffset="${off.toFixed(1)}" transform="rotate(-90 50 50)"/></svg>`;
    },
    chTiles() {
      return (this.curColl().charts?.tiles || []).map(m => {
        const last = this.chLatest(m.field);
        if (!last) return null;
        const prev = this.chAvg(this.chRange(m.field, this.chShift(last.iso, -30), this.chShift(last.iso, -1)));
        const delta = prev ? (last.v - prev) / prev * 100 : null;
        const spark = this.chRange(m.field, this.chShift(last.iso, -13), last.iso);
        let tone = '';
        if (delta !== null && Math.abs(delta) >= 1) tone = (delta < 0) === !!m.lower ? 'good' : 'bad';
        return { m, last, delta, tone, spark: this.chSpark(spark) };
      }).filter(Boolean);
    },
    chSpark(list) {
      if (list.length < 2) return '';
      const W = 84, H = 26, vs = list.map(x => x.v), lo = Math.min(...vs), hi = Math.max(...vs), span = hi - lo || 1;
      const t0 = this._d(list[0].iso), t1 = this._d(list[list.length - 1].iso), tw = (t1 - t0) || 1;
      const pts = list.map(x => [((this._d(x.iso) - t0) / tw) * (W - 6) + 3, H - 3 - ((x.v - lo) / span) * (H - 6)]);
      const [lx, ly] = pts[pts.length - 1];
      return `<svg class="spark" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" aria-hidden="true"><polyline points="${pts.map(p => p.map(n => n.toFixed(1)).join(',')).join(' ')}"/><circle cx="${lx.toFixed(1)}" cy="${ly.toFixed(1)}" r="3"/></svg>`;
    },
    // this week vs the week before, for the ring metrics
    chWeek() {
      const end = this.chEnd();
      return (this.curColl().charts?.rings || []).map(m => {
        const a = this.chAvg(this.chRange(m.field, this.chShift(end, -6), end));
        const b = this.chAvg(this.chRange(m.field, this.chShift(end, -13), this.chShift(end, -7)));
        const delta = a !== null && b ? (a - b) / b * 100 : null;
        return { m, a, b, delta, tone: delta === null || Math.abs(delta) < 1 ? '' : (delta < 0) === !!m.lower ? 'good' : 'bad' };
      }).filter(x => x.a !== null);
    },
    chDelta(d) { return d === null || d === undefined ? '' : (d >= 0 ? '▲ ' : '▼ ') + Math.abs(d).toFixed(Math.abs(d) < 10 ? 1 : 0) + '%'; },

    // ── Trends tab
    chTrendMetrics() { return (this.curColl().charts?.trends || []).filter(m => this.chLatest(m.field)); },
    chCurMetric() {
      const list = this.chTrendMetrics();
      return list.find(m => m.field === this.ch.metric) || list[0] || null;
    },
    chSpan(days) { // first ISO day of a range ending at the last data day
      const end = this.chEnd();
      if (days === 'all') return this.chDays()[0]?.iso || end;
      return this.chShift(end, -(Number(days) - 1));
    },
    // buckets: days up to a month, weeks up to ~4 months, months beyond
    chBuckets(m, from, to) {
      const list = this.chRange(m.field, from, to);
      const span = Math.round((this._d(to) - this._d(from)) / 864e5) + 1;
      const unit = span <= 31 ? 'day' : span <= 120 ? 'week' : 'month';
      const keyOf = (d) => {
        if (unit === 'day') return this.iso(d);
        if (unit === 'week') { const x = new Date(d); x.setDate(x.getDate() - ((x.getDay() + 6) % 7)); return this.iso(x); }
        return this.iso(new Date(d.getFullYear(), d.getMonth(), 1));
      };
      const out = [];
      for (let d = this._d(from); this.iso(d) <= to; d.setDate(d.getDate() + 1)) {
        const k = keyOf(d);
        if (!out.length || out[out.length - 1].key !== k) out.push({ key: k, start: new Date(d), vals: [] });
      }
      const idx = new Map(out.map((b, i) => [b.key, i]));
      for (const x of list) { const i = idx.get(keyOf(x.date)); if (i !== undefined) out[i].vals.push(x.v); }
      return { unit, buckets: out.map(b => ({ ...b, v: b.vals.length ? b.vals.reduce((s, v) => s + v, 0) / b.vals.length : null })) };
    },
    chTrendHTML() {
      const m = this.chCurMetric();
      if (!m) return '';
      const to = this.chEnd(), from = this.chSpan(this.ch.range);
      const { unit, buckets } = this.chBuckets(m, from, to);
      const lab = (b) => unit === 'month' ? b.start.toLocaleDateString(this.app.locale, { month: 'short', year: '2-digit' })
        : unit === 'week' ? 'Week of ' + b.start.toLocaleDateString(this.app.locale, { day: 'numeric', month: 'short' })
        : this.chDay(b.start);
      const axis = (b) => unit === 'month' ? b.start.toLocaleDateString(this.app.locale, { month: 'short' })
        : unit === 'week' ? b.start.toLocaleDateString(this.app.locale, { day: 'numeric', month: 'short' })
        : buckets.length <= 7 ? b.start.toLocaleDateString(this.app.locale, { weekday: 'short' }) : String(b.start.getDate());
      const per = unit === 'day' ? '' : ' a day (average)';
      const pts = buckets.map((b, i) => ({ v: b.v, axis: axis(b), tip: `${lab(b)} · ${b.v === null ? 'no data' : this.chFmt(m, b.v) + per}`, x: buckets.length > 1 ? i / (buckets.length - 1) : 0.5 }));
      const goal = this.chGoal(m);
      return m.chart === 'line' ? this.chLineHTML('trend', pts, { m, goal }) : this.chBarsHTML('trend', pts, { m, goal });
    },
    chTrendStats() {
      const m = this.chCurMetric();
      if (!m) return [];
      const to = this.chEnd(), from = this.chSpan(this.ch.range);
      const list = this.chRange(m.field, from, to);
      if (!list.length) return [];
      const days = Math.round((this._d(to) - this._d(from)) / 864e5) + 1;
      const avg = this.chAvg(list), goal = this.chGoal(m);
      const best = list.reduce((a, b) => (m.lower ? b.v < a.v : b.v > a.v) ? b : a);
      const worst = list.reduce((a, b) => (m.lower ? b.v > a.v : b.v < a.v) ? b : a);
      const out = [{ label: 'Average a day', value: this.chFmt(m, avg) }];
      if (m.total) out.push({ label: 'Total', value: this.chFmt({ ...m, unit: m.unit === 'h' ? 'h' : m.unit }, list.reduce((s, x) => s + x.v, 0)) });
      out.push({ label: m.lower ? 'Lowest' : 'Best', value: this.chFmt(m, best.v), sub: this.chDay(best.date) });
      out.push({ label: m.lower ? 'Highest' : 'Lowest', value: this.chFmt(m, worst.v), sub: this.chDay(worst.date) });
      out.push({ label: 'Days with data', value: `${list.length} of ${days}` });
      if (goal) {
        const hit = list.filter(x => (m.lower ? x.v <= goal : x.v >= goal)).length;
        out.push({ label: 'Goal met', value: `${hit} day${hit === 1 ? '' : 's'}`, sub: 'goal ' + this.chFmt(m, goal) });
      }
      if (this.ch.range !== 'all') {
        const prev = this.chAvg(this.chRange(m.field, this.chShift(from, -days), this.chShift(from, -1)));
        if (prev) {
          const d = (avg - prev) / prev * 100;
          out.push({ label: 'vs previous ' + (days === 365 ? 'year' : days + ' days'), value: this.chDelta(d), tone: Math.abs(d) < 1 ? '' : (d < 0) === !!m.lower ? 'good' : 'bad', sub: 'was ' + this.chFmt(m, prev) });
        }
      }
      return out;
    },

    // ── chart builders (return HTML; the container handles pointer events)
    chScale(vals, goal, zero) {
      const vs = vals.filter(v => v !== null);
      if (goal) vs.push(goal);
      if (!vs.length) return { lo: 0, hi: 1, ticks: [0, 1] };
      let lo = zero ? 0 : Math.min(...vs), hi = Math.max(...vs);
      if (hi === lo) { hi += 1; if (!zero) lo -= 1; }
      const raw = (hi - lo) / 3, pow = 10 ** Math.floor(Math.log10(raw)), step = [1, 2, 2.5, 5, 10].map(s => s * pow).find(s => s >= raw);
      lo = zero ? 0 : Math.floor(lo / step) * step;
      hi = Math.ceil(hi / step) * step;
      const ticks = [];
      for (let t = lo; t <= hi + step / 2; t += step) ticks.push(t);
      return { lo, hi, ticks };
    },
    chFrame(id, sc, inner, pts, opt) {
      const y = (v) => ((v - sc.lo) / (sc.hi - sc.lo)) * 100;
      const grid = sc.ticks.map(t => `<div class="ch-grid" style="bottom:${y(t).toFixed(2)}%"><span>${this.chTick(t)}</span></div>`).join('');
      const goal = opt.goal ? `<div class="ch-goal" style="bottom:${y(opt.goal).toFixed(2)}%"><span>Goal ${this.chFmt(opt.m, opt.goal, true)}</span></div>` : '';
      const every = Math.max(1, Math.ceil(pts.length / 7));
      const axis = pts.map((p, i) => i % every === 0 ? `<span style="left:${(opt.ax ? opt.ax(i) : p.x * 100).toFixed(2)}%">${this.esc(p.axis)}</span>` : '').join('');
      const sel = this.ch.tip?.id === id ? pts[this.ch.tip.i] : null;
      const read = sel ? sel.tip : (opt.hint || 'Tap the chart to see a day');
      return `<p class="ch-read${sel ? ' on' : ''}">${this.esc(read)}</p><div class="ch" data-ch="${id}"><div class="ch-plot">${grid}${goal}${inner}</div><div class="ch-axis">${axis}</div></div>`;
    },
    chBarsHTML(id, pts, opt) {
      CH.pts[id] = pts;
      const sc = this.chScale(pts.map(p => p.v), opt.goal, true);
      const sel = this.ch.tip?.id === id ? this.ch.tip.i : -1;
      const n = pts.length;
      const bars = pts.map((p, i) => {
        const h = p.v === null ? 0 : ((p.v - sc.lo) / (sc.hi - sc.lo)) * 100;
        const met = opt.goal && p.v !== null && (opt.m.lower ? p.v <= opt.goal : p.v >= opt.goal);
        return `<div class="ch-slot${i === sel ? ' sel' : ''}${sel >= 0 && i !== sel ? ' dim' : ''}" data-i="${i}" style="left:${(i / n * 100).toFixed(3)}%;width:${(100 / n).toFixed(3)}%"><i class="ch-bar${met ? ' met' : ''}" style="height:${h.toFixed(2)}%"></i></div>`;
      }).join('');
      return this.chFrame(id, sc, bars, pts, { ...opt, ax: (i) => (i + 0.5) / n * 100 });
    },
    chLineHTML(id, pts, opt) {
      CH.pts[id] = pts;
      const sc = this.chScale(pts.map(p => p.v), opt.goal, false);
      const y = (v) => 100 - ((v - sc.lo) / (sc.hi - sc.lo)) * 100;
      // break the line where a bucket has no data
      const runs = [[]];
      pts.forEach(p => { if (p.v === null) { if (runs[runs.length - 1].length) runs.push([]); } else runs[runs.length - 1].push(p); });
      const lines = runs.filter(r => r.length).map(r => r.length === 1
        ? `<circle cx="${(r[0].x * 1000).toFixed(1)}" cy="${y(r[0].v).toFixed(2)}" r="0.01"/>`
        : `<polyline points="${r.map(p => `${(p.x * 1000).toFixed(1)},${y(p.v).toFixed(2)}`).join(' ')}"/>`).join('');
      const sel = this.ch.tip?.id === id ? pts[this.ch.tip.i] : null;
      const last = [...pts].reverse().find(p => p.v !== null);
      const dot = (p, cls) => p && p.v !== null ? `<i class="ch-dot ${cls}" style="left:${(p.x * 100).toFixed(2)}%;top:${y(p.v).toFixed(2)}%"></i>` : '';
      const cross = sel ? `<i class="ch-cross" style="left:${(sel.x * 100).toFixed(2)}%"></i>` : '';
      const singles = runs.filter(r => r.length === 1).map(r => dot(r[0], 'lone')).join('');
      const svg = `<svg class="ch-line" viewBox="0 0 1000 100" preserveAspectRatio="none" aria-hidden="true">${lines}</svg>`;
      return this.chFrame(id, sc, svg + singles + cross + dot(sel || last, sel ? 'sel' : 'end'), pts, opt);
    },
    // pointer → nearest point
    chPick(e) {
      const box = e.target.closest('[data-ch]');
      if (!box) return;
      const id = box.dataset.ch, pts = CH.pts[id];
      if (!pts?.length) return;
      const slot = e.target.closest('[data-i]');
      let i;
      if (slot) i = +slot.dataset.i;
      else {
        const r = box.querySelector('.ch-plot').getBoundingClientRect();
        const x = (e.clientX - r.left) / r.width;
        let best = Infinity;
        pts.forEach((p, k) => { if (p.v !== null && Math.abs(p.x - x) < best) { best = Math.abs(p.x - x); i = k; } });
      }
      if (i === undefined || pts[i]?.v === null) return;
      if (this.ch.tip?.id !== id || this.ch.tip.i !== i) this.ch.tip = { id, i };
    },
    chLeave(e) { if (e.pointerType === 'mouse') this.ch.tip = null; },
    esc(s) { return String(s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])); },

    // ── Sleep tab
    chSleep() { return this.curColl().charts?.sleep || null; },
    chNights() {
      const s = this.chSleep();
      if (!s) return [];
      const to = this.chEnd(), from = this.chShift(to, -(this.ch.nights - 1));
      const m = (t) => { const x = /^(\d{1,2}):(\d{2})/.exec(t || ''); return x ? +x[1] * 60 + +x[2] : null; };
      const off = (t) => t === null ? null : (t - 1200 + 1440) % 1440; // minutes after 20:00
      return this.chDays().filter(x => x.iso >= from && x.iso <= to && this.chNum(x.r[s.hours]) !== null).reverse().map(x => {
        const a = off(m(x.r[s.start])), b = off(m(x.r[s.end]));
        const stages = s.stages.map((st, k) => ({ st, k, h: this.chNum(x.r[st.field]) || 0 })).filter(z => z.h > 0);
        const sum = stages.reduce((t, z) => t + z.h, 0);
        return { ...x, hours: this.chNum(x.r[s.hours]), a, b, ok: a !== null && b !== null && b > a && b <= 960, stages, sum,
          bed: x.r[s.start], wake: x.r[s.end] };
      });
    },
    chNightTip(n) {
      const s = this.chSleep(), hm = { unit: 'h' };
      const parts = [this.chDay(n.date), this.chFmt(hm, n.hours)];
      if (n.bed && n.wake) parts.push(`${this.calTime(n.bed)} – ${this.calTime(n.wake)}`);
      parts.push(...n.stages.map(z => `${s.stages[z.k].label} ${this.chFmt(hm, z.h)}`));
      return parts.join(' · ');
    },
    chSleepAxis() { return [0, 240, 480, 720, 960].map(t => ({ left: t / 9.6, label: new Date(2000, 0, 1, (20 + t / 60) % 24).toLocaleTimeString(this.app.locale, { hour: 'numeric' }) })); },
    chSleepStats() {
      const s = this.chSleep(), ns = this.chNights();
      if (!s || !ns.length) return [];
      const hm = { unit: 'h' }, avg = ns.reduce((t, n) => t + n.hours, 0) / ns.length;
      const timed = ns.filter(n => n.ok);
      const clock = (mins) => { const t = (Math.round(mins) + 1200) % 1440; return this.calTime(`${String(Math.floor(t / 60)).padStart(2, '0')}:${String(t % 60).padStart(2, '0')}`); };
      const out = [{ label: 'Average sleep', value: this.chFmt(hm, avg) }];
      const goal = this.chGoal({ field: s.hours, goal: s.goal });
      if (goal) { const hit = ns.filter(n => n.hours >= goal).length; out.push({ label: 'Goal met', value: `${hit} of ${ns.length} nights`, sub: 'goal ' + this.chFmt(hm, goal) }); }
      if (timed.length) {
        const ab = timed.reduce((t, n) => t + n.a, 0) / timed.length, bb = timed.reduce((t, n) => t + n.b, 0) / timed.length;
        const sd = Math.sqrt(timed.reduce((t, n) => t + (n.a - ab) ** 2, 0) / timed.length);
        out.push({ label: 'Average bedtime', value: clock(ab) }, { label: 'Average wake-up', value: clock(bb) },
          { label: 'Bedtime varies by', value: '± ' + Math.round(sd) + ' min', sub: sd <= 30 ? 'steady' : sd <= 60 ? 'a bit irregular' : 'irregular' });
      }
      s.stages.forEach((st, k) => {
        const hs = ns.map(n => n.stages.find(z => z.k === k)?.h).filter(h => h);
        if (hs.length) out.push({ label: 'Average ' + st.label.toLowerCase(), value: this.chFmt(hm, hs.reduce((t, h) => t + h, 0) / hs.length), sub: Math.round(hs.reduce((t, h) => t + h, 0) / hs.length / avg * 100) + '% of sleep', key: k });
      });
      const long = ns.reduce((a, b) => (b.hours > a.hours ? b : a)), short = ns.reduce((a, b) => (b.hours < a.hours ? b : a));
      out.push({ label: 'Longest night', value: this.chFmt(hm, long.hours), sub: this.chDay(long.date) }, { label: 'Shortest night', value: this.chFmt(hm, short.hours), sub: this.chDay(short.date) });
      return out;
    },

    // ── Body tab
    chBodyCards() {
      const to = this.chEnd(), from = this.chSpan(this.ch.bodyRange);
      return (this.curColl().charts?.body || []).map(m => {
        const list = this.chRange(m.field, from, to);
        const latest = this.chLatest(m.field);
        if (!latest) return null;
        const t0 = this._d(from), tw = (this._d(to) - t0) || 1;
        const pts = list.map(x => ({ v: x.v, x: (x.date - t0) / tw, axis: x.date.toLocaleDateString(this.app.locale, { day: 'numeric', month: 'short' }), tip: `${this.chDay(x.date, true)} · ${this.chFmt(m, x.v)}` }));
        const ch = list.length > 1 ? (list[list.length - 1].v - list[0].v) : null;
        const avg7 = this.chAvg(this.chRange(m.field, this.chShift(latest.iso, -6), latest.iso));
        return { m, latest, list, ch, avg7, lo: list.length ? Math.min(...list.map(x => x.v)) : null, hi: list.length ? Math.max(...list.map(x => x.v)) : null,
          tone: ch === null || Math.abs(ch) < 0.05 ? '' : (ch < 0) === !!m.lower ? 'good' : 'bad',
          html: list.length > 1 ? this.chLineHTML('body-' + m.field, pts, { m, hint: 'Tap the line to see a reading' }) : '' };
      }).filter(Boolean);
    },
    chSigned(m, v) { return v === null ? '—' : (v > 0 ? '+' : v < 0 ? '−' : '±') + this.chFmt(m, Math.abs(v)); },

    // ── sheet (edit form) ──────────────────────────────────────────────

    openRecord(c, r) {
      if (!r) return;
      this.sheet = {
        coll: c, plugin: this.pluginOfColl(c), record: r, mode: 'edit', errors: {}, error: '', conflict: false, saving: false,
        values: this._formValues(c, r),
      };
      this.mdPreview = false;
      this.sheet.initial = JSON.parse(JSON.stringify(this.sheet.values));
      this._showSheet();
    },
    openNew(c) {
      const values = {};
      for (const f of c.fields) {
        let v = f.default ?? '';
        if (f.type === 'date' && typeof v === 'string' && v.startsWith('today')) {
          const d = new Date(); d.setDate(d.getDate() + Number(v.slice(5) || 0)); v = this.iso(d);
        }
        values[f.key] = f.type === 'bool' ? !!v : String(v);
      }
      // a filter in use is a good default for the new record (e.g. Tag = run)
      for (const [k, v] of Object.entries(this.filters)) if (v && k in values) values[k] = v;
      this.sheet = { coll: c, plugin: this.pluginOfColl(c), record: null, mode: 'new', values, initial: {}, errors: {}, error: '', conflict: false, saving: false, at: null };
      this.mdPreview = false;
      this._showSheet();
    },
    _formValues(c, r) {
      const v = {};
      for (const f of c.fields) {
        const x = r[f.key];
        if (f.type === 'date') { const d = this.parseDate(x, f.format); v[f.key] = d ? this.iso(d) : (x ?? ''); }
        else if (f.type === 'list') v[f.key] = Array.isArray(x) ? x.join('\n') : (x ?? '');
        else if (f.type === 'bool') v[f.key] = !!x;
        else v[f.key] = x === null || x === undefined ? '' : String(x);
      }
      return v;
    },
    _showSheet() {
      history.pushState({ natlasSheet: true }, '');
      this.$nextTick(() => {
        this.$refs.sheet.showModal();
        if (this.sheet.mode === 'new') this.$refs.sheet.querySelector('input, textarea, select')?.focus();
      });
    },
    closeSheet() {
      if (history.state?.natlasSheet) history.back(); // popstate closes the dialog
      else this.$refs.sheet.close();
    },
    closeSheetQuiet() { if (this.$refs.sheet?.open) this.closeSheet(); },
    sheetClosed() {
      // the form stays rendered (hidden) until the next open, so Alpine never
      // evaluates it against an empty sheet
      if (history.state?.natlasSheet) history.back();
    },
    sheetTitle() {
      const c = this.sheet.coll;
      if (!c) return '';
      if (this.sheet.mode === 'new') {
        if (this.pluginOfColl(c)?.view === 'calendar') return 'New event';
        if (this.pluginOfColl(c)?.view === 'document') return 'New section';
        return 'New ' + (c.label.endsWith('s') ? c.label.slice(0, -1) : c.label).toLowerCase();
      }
      if (c.kind === 'record') return c.label;
      return this.text(this.sheet.record?.[c.list.title]) || c.label;
    },
    isLocked(f) { return !this.sheet.coll || this.sheet.coll.readonly || f.readonly || f.type === 'anniversary'; },
    inputType(f) {
      return { date: 'date', time: 'time', number: 'number', money: 'number', url: 'url' }[f.type] || 'text';
    },
    otherFields() {
      const c = this.sheet.coll, r = this.sheet.record || {};
      if (!c) return [];
      const known = new Set(c.fields.map(f => f.key).concat([c.id_field]));
      return Object.keys(r).filter(k => !known.has(k) && !k.startsWith('_') && !k.includes('.'))
        .map(k => ({ key: k, value: this.text(r[k]) || '—' }));
    },
    sheetWide() { return !!this.sheet.coll?.fields?.some(f => f.type === 'markdown'); },
    sheetActions() {
      const r = this.sheet.record;
      if (!r || !this.sheet.coll) return [];
      return (this.sheet.coll.actions || []).filter(a => r._actions?.includes(a.id));
    },

    async saveSheet() {
      const { coll: c, plugin: p, record: r, mode } = this.sheet;
      if (c.readonly) return;
      this.sheet.saving = true; this.sheet.errors = {}; this.sheet.error = '';
      try {
        if (mode === 'new') {
          const body = { values: this.sheet.values };
          if (Number.isInteger(this.sheet.at)) body.at = this.sheet.at; // "add section below"
          const res = await this.api('POST', `/api/p/${p.id}/${c.id}`, body);
          this.closeSheet();
          this.toast(`Added: ${this.text(res.record[c.list.title])}`, 'ok', () => this._delete(p, c, res.record));
        } else {
          const values = {};
          for (const [k, v] of Object.entries(this.sheet.values)) {
            if (JSON.stringify(v) !== JSON.stringify(this.sheet.initial[k])) values[k] = v;
          }
          if (!Object.keys(values).length) { this.closeSheet(); this.sheet.saving = false; return; }
          const res = await this.api('PATCH', `/api/p/${p.id}/${c.id}/${encodeURIComponent(r._id)}`, { rev: r._rev, values });
          this.closeSheet();
          const back = this._changedBack(c, res.before, res.record);
          this.toast('Saved', 'ok', () => this.api('PATCH', `/api/p/${p.id}/${c.id}/${encodeURIComponent(res.record._id)}`, { rev: res.record._rev, values: back }));
        }
        await this.afterChange(p.id, c.id);
      } catch (e) {
        if (e.status === 422) this.sheet.errors = e.fields;
        else if (e.status === 409) this.sheet.conflict = true;
        else if (e.status !== 401) this.sheet.error = e.message;
      }
      this.sheet.saving = false;
    },

    async reloadSheet() {
      const { coll: c, plugin: p, record: r } = this.sheet;
      const d = await this._refresh(p.id, c.id);
      const fresh = d?.records.find(x => x._id === r._id);
      if (!fresh) { this.sheet.error = 'This record no longer exists (deleted or archived).'; this.sheet.conflict = false; return; }
      this.sheet.record = fresh;
      this.sheet.values = this._formValues(c, fresh);
      this.sheet.initial = JSON.parse(JSON.stringify(this.sheet.values));
      this.sheet.conflict = false;
    },

    async deleteRecord() {
      const { coll: c, plugin: p, record: r } = this.sheet;
      const title = this.text(r[c.list.title]) || 'this record';
      if (!confirm(`Delete “${title}”?`)) return;
      try {
        const res = await this.api('DELETE', `/api/p/${p.id}/${c.id}/${encodeURIComponent(r._id)}?rev=${r._rev}`);
        this.closeSheet();
        this.toast(`Deleted: ${title}`, 'ok', () => this.api('POST', `/api/p/${p.id}/${c.id}`, { id: res.id, at: res.at, values: res.deleted }));
        await this.afterChange(p.id, c.id);
      } catch (e) {
        if (e.status === 409) this.sheet.conflict = true; else this.sheet.error = e.message;
      }
    },

    async moveRecord(dir) {
      const { coll: c, plugin: p, record: r } = this.sheet;
      try {
        await this.api('POST', `/api/p/${p.id}/${c.id}/${encodeURIComponent(r._id)}/move`, { dir });
        await this._refresh(p.id, c.id);
        this.toast(dir < 0 ? 'Moved up' : 'Moved down', 'ok');
      } catch (e) { this.sheet.error = e.message; }
    },

    // ── toasts & undo ──────────────────────────────────────────────────

    toast(text, type = 'ok', undo = null) {
      const id = ++this._toastSeq;
      // hasUndo is a plain flag: Alpine calls any function an expression returns,
      // so templates must never read t.undo directly
      this.toasts = [...this.toasts.slice(-2), { id, text, type, undo, hasUndo: !!undo }];
      setTimeout(() => this.dropToast(id), undo ? 6000 : type === 'error' ? 7000 : 3000);
    },
    dropToast(id) { this.toasts = this.toasts.filter(t => t.id !== id); },
    async undo(t) {
      if (this._undone.has(t.id)) return; // one tap = one undo
      this._undone.add(t.id);
      this.dropToast(t.id);
      try {
        await t.undo();
        this.toast('Undone', 'ok');
        this.reload(true);
        if (this.route.view !== 'today') this.loadToday();
      } catch (e) { this.fail(e); }
    },


    // ── phone (Android app) ────────────────────────────────────────────
    // Settings live in the app (not in the synced folder); NatlasNative is
    // the Kotlin bridge. Records changed on two devices are listed by
    // /api/conflicts and resolved here on any device.

    nativeCall(fn, ...args) {
      try { return window.NatlasNative?.[fn](...args); } catch (e) { this.toast(String(e.message || e), 'error'); }
    },
    loadPhone() {
      const raw = this.nativeCall('settings');
      if (raw) this.phone = JSON.parse(raw);
      this.loadConflicts();
    },
    setPhone(key, value) {
      const raw = this.nativeCall('set', key, typeof value === 'string' ? value : JSON.stringify(value));
      if (raw) this.phone = JSON.parse(raw);
    },
    phoneMove(id, dir) {
      const order = this.phone.plugins.map(p => p.id);
      const i = order.indexOf(id), j = i + dir;
      if (j < 0 || j >= order.length) return;
      [order[i], order[j]] = [order[j], order[i]];
      this.setPhone('order', order.join(','));
    },
    phoneToggle(id) {
      const on = this.phone.plugins.filter(p => p.on).map(p => p.id);
      const next = on.includes(id) ? on.filter(x => x !== id) : [...on, id];
      if (!next.length) { this.toast('Keep at least one list.', 'error'); return; }
      this.setPhone('enabled', next.join(','));
    },
    setMotion(on) {
      this._setPref('natlas_motion', on ? 'on' : 'off');
      document.documentElement.classList.toggle('no-motion', !on);
      this.setPhone('animations', on ? 'true' : 'false');
    },
    motionOn() { return this._getPref('natlas_motion') !== 'off'; },
    showLog() { this.phoneLog = this.nativeCall('log') || ''; },
    onNative(name, data) {
      if (name === 'settings') this.phone = data;
      if (name === 'toast') this.toast(data.text, data.type || 'ok');
      if (name === 'route') location.hash = data;
      if (name === 'refresh') this.reload(true);
      if (name === 'health') this.healthProgress = data.progress || '';
    },

    async loadConflicts() {
      try {
        const r = await this.api('GET', '/api/conflicts');
        this.conflicts = { ...this.conflicts, list: r.conflicts || [], copies: r.copies || 0, main: !!r.main };
      } catch (e) { /* older server: no conflicts endpoint */ }
    },
    openConflicts() {
      if (!this.conflicts.list.length) return;
      this.conflicts.i = 0;
      this.conflicts.open = true;
      this.$nextTick(() => this.$refs.conflicts?.showModal());
    },
    curConflict() { return this.conflicts.list[this.conflicts.i] || null; },
    async resolveConflict(keep) {
      const c = this.curConflict();
      if (!c) return;
      this.conflicts.busy = true;
      try {
        await this.api('POST', '/api/conflicts/' + encodeURIComponent(c.id), { keep });
        this.toast(keep === 'other' ? `Took the other device's “${c.title || c.record}”` : `Kept “${c.title || c.record}”`, 'ok');
        await this.loadConflicts();
        if (!this.conflicts.list.length) this.$refs.conflicts?.close();
        this.conflicts.i = Math.min(this.conflicts.i, Math.max(0, this.conflicts.list.length - 1));
        this.reload(true);
      } catch (e) { this.fail(e); }
      this.conflicts.busy = false;
    },

    // ── pwa & theme ────────────────────────────────────────────────────

    setupServiceWorker() {
      if (!('serviceWorker' in navigator)) return;
      if (this.native) { // the phone app serves itself from the phone: nothing to cache
        navigator.serviceWorker.getRegistrations().then(rs => rs.forEach(r => r.unregister())).catch(() => {});
        return;
      }
      if (new URLSearchParams(location.search).has('nosw')) {
        // escape hatch: https://natlas.example/?nosw removes the service worker and its caches
        navigator.serviceWorker.getRegistrations().then(rs => rs.forEach(r => r.unregister()));
        if ('caches' in window) caches.keys().then(ks => ks.forEach(k => caches.delete(k)));
        return;
      }
      navigator.serviceWorker.register('/sw.js').then(reg => {
        this._sw = reg;
        const check = () => { if (reg.waiting && navigator.serviceWorker.controller) this.updateReady = true; };
        check();
        reg.addEventListener('updatefound', () => {
          reg.installing?.addEventListener('statechange', check);
        });
      }).catch(() => {});
      let reloading = false;
      navigator.serviceWorker.addEventListener('controllerchange', () => {
        if (this._applying && !reloading) { reloading = true; location.reload(); }
      });
    },
    applyUpdate() {
      this._applying = true;
      if (this._sw?.waiting) this._sw.waiting.postMessage('skipWaiting');
      else location.reload();
    },

    cycleTheme() {
      this.theme = { auto: 'dark', dark: 'light', light: 'auto' }[this.theme] || 'auto';
      this._setPref('natlas_theme', this.theme);
      this.applyTheme();
      this.$nextTick(() => this.redrawDiagrams());
    },
    applyTheme() {
      const root = document.documentElement;
      if (this.theme === 'auto') root.removeAttribute('data-theme');
      else root.setAttribute('data-theme', this.theme);
      const dark = this.theme === 'dark' || (this.theme === 'auto' && !matchMedia('(prefers-color-scheme: light)').matches);
      document.querySelector('meta[name=theme-color]')?.setAttribute('content', dark ? '#070b0f' : '#f6f8f8');
      try { window.NatlasNative?.setBars(dark); } catch (e) { /* older shell */ }
    },
    _getPref(k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
    _setPref(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* private mode */ } },

    icon(name) {
      const body = ICONS[name];
      if (!body) return `<span class="emoji">${name ? String(name).slice(0, 2) : '•'}</span>`;
      return `<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;
    },
  }));
});
