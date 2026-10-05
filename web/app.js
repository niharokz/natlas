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
 *   sheet ...... the edit form (new / edit / read-only)
 *   actions .... one-tap actions, badge menus, bulk edits, undo
 *   pwa ........ service worker, update prompt, theme
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
    toasts: [], moreOpen: false, theme: 'auto', updateReady: false,
    _press: null, _suppressClick: false, _undone: new Set(), _toastSeq: 0, _sw: null,

    get selecting() { return Object.keys(this.selected).length > 0; },

    async init() {
      this.theme = this._getPref('natlas_theme') || 'auto';
      this.applyTheme();
      this.setupServiceWorker();
      window.addEventListener('hashchange', () => this.applyRoute());
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
      if (res.status === 401 && !url.endsWith('/api/login')) {
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
        await this.loadApp();
      } catch (e) { this.loginError = e.message; }
      this.busy = false;
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
      if (parts[0] === 'p' && this.plugin(parts[1])) {
        const p = this.plugin(parts[1]);
        const coll = p.collections.some(c => c.id === parts[2]) ? parts[2] : p.collections[0].id;
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
      else {
        const c = this.curColl();
        this.summaryBy = c.summary?.group_by?.[0] || '';
        this.load(this.route.plugin, this.route.coll);
      }
    },

    reload(quiet) {
      if (!this.authed) return;
      if (this.route.view === 'today') this.loadToday(!quiet);
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
      return c.fields.filter(f => !f.hidden && !(isNew && f.readonly));
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

    // ── sheet (edit form) ──────────────────────────────────────────────

    openRecord(c, r) {
      if (!r) return;
      this.sheet = {
        coll: c, plugin: this.pluginOfColl(c), record: r, mode: 'edit', errors: {}, error: '', conflict: false, saving: false,
        values: this._formValues(c, r),
      };
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
      this.sheet = { coll: c, plugin: this.pluginOfColl(c), record: null, mode: 'new', values, initial: {}, errors: {}, error: '', conflict: false, saving: false };
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
      if (this.sheet.mode === 'new') return 'New ' + (c.label.endsWith('s') ? c.label.slice(0, -1) : c.label).toLowerCase();
      if (c.kind === 'record') return c.label;
      return this.text(this.sheet.record?.[c.list.title]) || c.label;
    },
    isLocked(f) { return !this.sheet.coll || this.sheet.coll.readonly || f.readonly || f.type === 'anniversary'; },
    inputType(f) {
      return { date: 'date', number: 'number', money: 'number', url: 'url' }[f.type] || 'text';
    },
    otherFields() {
      const c = this.sheet.coll, r = this.sheet.record || {};
      if (!c) return [];
      const known = new Set(c.fields.map(f => f.key).concat([c.id_field]));
      return Object.keys(r).filter(k => !known.has(k) && !k.startsWith('_') && !k.includes('.'))
        .map(k => ({ key: k, value: this.text(r[k]) || '—' }));
    },
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
          const res = await this.api('POST', `/api/p/${p.id}/${c.id}`, { values: this.sheet.values });
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

    // ── pwa & theme ────────────────────────────────────────────────────

    setupServiceWorker() {
      if (!('serviceWorker' in navigator)) return;
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
    },
    applyTheme() {
      const root = document.documentElement;
      if (this.theme === 'auto') root.removeAttribute('data-theme');
      else root.setAttribute('data-theme', this.theme);
      const dark = this.theme === 'dark' || (this.theme === 'auto' && !matchMedia('(prefers-color-scheme: light)').matches);
      document.querySelector('meta[name=theme-color]')?.setAttribute('content', dark ? '#070b0f' : '#f6f8f8');
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
