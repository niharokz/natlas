// today.js — the Today screen (agenda, numbers), layout.yml (menu order, Today blocks) and the Customize page.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('today', () => ({
    // ── today ──────────────────────────────────────────────────────────

    async loadToday(showToast) {
      try {
        this.today = await this.api('GET', '/api/dashboard');
        this.todayLoaded = true;
        for (const b of this.layout.home) if (b.type === 'list') this.load(b.plugin, b.coll, true);
        this.loadConflicts();
      } catch (e) { if (showToast && e.status !== 401) this.toast(e.message, 'error'); }
    },

    todayTitle() {
      return new Date().toLocaleDateString(this.app.locale, { weekday: 'long', day: 'numeric', month: 'long' });
    },

    agendaSections(b) {
      const labels = { overdue: 'Overdue', today: 'Today', upcoming: 'Next 7 days' };
      return (b?.sections?.length ? b.sections : ['overdue', 'today', 'upcoming'])
        .map(key => ({ key, label: labels[key], items: this.today.agenda?.[key] || [] }))
        .filter(s => s.items.length);
    },

    // ── layout: menu order and Today blocks (layout.yml, shared by web and phone) ──

    menuPlugins() {
      const order = this.layout.menu || [];
      const pos = id => { const i = order.indexOf(id); return i < 0 ? 999 : i; };
      return this.app.plugins.filter(p => !this.layout.hidden?.includes(p.id)).sort((a, b) => pos(a.id) - pos(b.id));
    },
    glanceStats(b) { return (this.today.stats || []).filter(s => !b.plugin || s.plugin === b.plugin); },
    // blockRows: records of a list block (only its group, matching its search), first `limit`.
    blockRows(b) {
      const d = this.data[b.plugin + '/' + b.coll], c = this.collOf(b.plugin, b.coll);
      if (!d || !c) return [];
      let recs = d.records;
      if (b.group) {
        const g = d.groups?.find(x => x.label === b.group);
        const ids = new Set(g ? g.ids : []);
        recs = recs.filter(r => ids.has(r._id));
      }
      const q = (b.search || '').toLowerCase();
      if (q) {
        const keys = c.list.search?.length ? c.list.search : [c.list.title];
        recs = recs.filter(r => keys.some(k => this.text(r[k]).toLowerCase().includes(q)));
      }
      return recs.slice(0, b.limit || 6);
    },
    blockTitle(b) {
      if (b.title) return b.title;
      if (b.type === 'list') return this.collOf(b.plugin, b.coll)?.label || b.plugin;
      if (b.type === 'glance') return b.plugin ? (this.plugin(b.plugin)?.title || b.plugin) + ' at a glance' : 'At a glance';
      if (b.type === 'agenda') return b.sections?.length ? 'Agenda: ' + b.sections.join(', ') : 'Agenda';
      return 'Note';
    },
    listColls() { // every list collection, for list blocks
      return this.app.plugins.flatMap(p => p.collections.filter(c => c.kind === 'list' && p.view !== 'document')
        .map(c => ({ key: p.id + '/' + c.id, plugin: p.id, coll: c.id, label: p.title + (p.collections.length > 1 ? ' › ' + c.label : '') })));
    },

    // ── Customize (#/customize) ──
    czStart() {
      const l = JSON.parse(JSON.stringify(this.layout));
      this.cz = { menu: this.app.plugins.map(p => p.id).sort((a, b) => l.menu.indexOf(a) - l.menu.indexOf(b)),
        hidden: l.hidden || [], home: l.home || [], open: -1, adding: '', saving: false };
    },
    czMove(list, i, dir) {
      const j = i + dir;
      if (j < 0 || j >= list.length) return;
      [list[i], list[j]] = [list[j], list[i]];
      if (this.cz.open === i) this.cz.open = j; else if (this.cz.open === j) this.cz.open = i;
    },
    czToggle(id) {
      const h = this.cz.hidden;
      this.cz.hidden = h.includes(id) ? h.filter(x => x !== id) : [...h, id];
    },
    czAdd(type) {
      if (!type) return;
      const first = this.listColls()[0] || {};
      const b = { agenda: { type, sections: [] }, glance: { type, plugin: '' }, note: { type, title: '', text: '' },
        list: { type, plugin: first.plugin, coll: first.coll, title: '', group: '', search: '', limit: 6 } }[type];
      this.cz.home.push(b);
      this.cz.open = this.cz.home.length - 1;
      this.cz.adding = '';
      if (type === 'list') this.load(b.plugin, b.coll);
    },
    czSetColl(b, key) {
      [b.plugin, b.coll] = key.split('/');
      b.group = '';
      this.load(b.plugin, b.coll);
    },
    czGroups(b) { return (this.data[b.plugin + '/' + b.coll]?.groups || []).map(g => g.label); },
    czSection(b, key) {
      const s = b.sections || [];
      b.sections = s.includes(key) ? s.filter(x => x !== key) : ['overdue', 'today', 'upcoming'].filter(x => x === key || s.includes(x));
    },
    async czSave(reset) {
      this.cz.saving = true;
      try {
        const body = reset ? { reset: true } : { menu: this.cz.menu, hidden: this.cz.hidden, home: this.cz.home };
        this.layout = await this.api('PUT', '/api/layout', body);
        this.toast(reset ? 'Back to the default layout' : 'Layout saved');
        if (reset) this.czStart(); else location.hash = '#/today';
      } catch (e) { this.toast(e.message, 'error'); }
      if (this.cz) this.cz.saving = false;
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
}));
