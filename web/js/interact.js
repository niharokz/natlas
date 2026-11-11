// interact.js — gestures (swipe, long-press), selection and bulk edits, badge menus, one-tap actions, the ask dialog.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('interact', () => ({
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
}));
