// charts.js — charts over a dated list (`charts:`, Health → Daily): Today, Trends, Sleep, Body tabs.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('charts', () => ({
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
}));
