// shell.js — service worker and update prompt, theme, preferences, icons.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('shell', () => ({
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
