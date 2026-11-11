// lists.js — list screens: search, filters, groups, summary, display formatting, dates.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('lists', () => ({
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
          if (v && (Array.isArray(r[k]) ? !r[k].map(String).includes(v) : this.text(r[k]) !== v)) return false;
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
}));
