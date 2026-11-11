// calendar.js — month / week / schedule view for plugins with `view: calendar`.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('calendar', () => ({
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
      const whole = /^\{([^}]+)\}$/.exec(c.link.url); // "{url}": the field is the link
      const u = whole ? this.text(r[whole[1]]) : c.link.url.replace(/\{([^}]+)\}/g, (_, k) => encodeURIComponent(this.text(r[k])));
      return /^https?:\/\//.test(u) ? u : '';
    },
}));
