// core.js — state, the API wrapper (session expiry, offline, errors) and routing (#/today, #/p/<plugin>/<coll>, #/customize).
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('core', () => ({
    // ── state ──────────────────────────────────────────────────────────
    ready: false, authed: false, user: '', busy: false,
    loginForm: { username: '', password: '' }, loginError: '',
    app: { title: 'Natlas', currency: '₹', locale: 'en-IN', plugins: [] },
    route: { view: 'today', plugin: null, coll: null },
    today: { agenda: {}, stats: [] }, todayLoaded: false,
    // layout.yml: menu order, hidden lists, Today blocks; cz = the Customize draft
    layout: { menu: [], hidden: [], home: [{ type: 'agenda' }, { type: 'glance' }] }, cz: null, demo: false,
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
      this.layout = await this.api('GET', '/api/layout').catch(() => this.layout);
      if (this.demo) for (const p of this.app.plugins) for (const c of p.collections) c.readonly = true; // the demo is read-only
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
        this.demo = !!r.demo;
        this.api('GET', '/api/me').then(me => this.setDevice(me)).catch(() => {});
        await this.loadApp();
      } catch (e) { this.loginError = e.message; }
      this.busy = false;
    },

    setDevice(me) {
      this.demo = !!me.demo;
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
      this.authed = false; this.moreOpen = false; this.data = {}; this.demo = false; this.cz = null;
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
      if (parts[0] === 'customize') {
        this.route = { view: 'customize', plugin: null, coll: null };
        this.moreOpen = false;
        this.czStart();
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
}));
