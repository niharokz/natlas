// sheet.js — the edit form (new / edit / read-only), toasts and undo.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('sheet', () => ({
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
      if (!r || !this.sheet.coll || this.sheet.coll.readonly) return [];
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
}));
