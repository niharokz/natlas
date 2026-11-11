// phone.js — the Android app (window.NatlasNative): settings page (#/phone), sync conflicts.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('phone', () => ({
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
}));
