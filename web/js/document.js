// document.js — a Markdown note as numbered sections (`view: document`): contents, edit, move, Mermaid.
// One part of the Alpine component; js/app.js merges every part. ARCHITECTURE.md → Web app.
part('document', () => ({
    // ── document ───────────────────────────────────────────────────────
    // A Markdown note (format: markdown) shown like a handbook: a front record
    // (title, subtitle, reviewed, intro) and numbered "## " sections, each
    // editable on its own. Rendering: marked (GFM) + Obsidian-style callouts
    // (> [!note] Title) + ```mermaid diagrams (mermaid.js, loaded on demand).

    isDocument() { return this.route.view === 'plugin' && this.curPlugin().view === 'document'; },
    docColl(kind) { return this.curPlugin().collections.find(c => c.kind === kind); },
    docLoaded() { return !!this.data[this.curPlugin().id + '/' + this.docColl('list')?.id]; },
    docAll() { return this.data[this.curPlugin().id + '/' + this.docColl('list')?.id]?.records || []; },
    docFront() {
      const c = this.docColl('record');
      return c ? this.data[this.curPlugin().id + '/' + c.id]?.records?.[0] || null : null;
    },
    docSections() {
      const q = this.search.trim().toLowerCase();
      return this.docAll().map((r, i) => ({ r, n: String(i).padStart(2, '0') }))
        .filter(x => !q || (this.text(x.r.title) + '\n' + this.text(x.r.body)).toLowerCase().includes(q));
    },
    docGo(id) {
      this.doc.toc = false;
      document.getElementById('sec-' + id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      this.doc.active = id;
    },
    docSpy() {
      let active = '';
      for (const el of document.querySelectorAll('.doc-sec')) {
        if (el.getBoundingClientRect().top < 140) active = el.dataset.id; else break;
      }
      if (active !== this.doc.active) this.doc.active = active;
    },
    docEdit(r) { this.openRecord(this.docColl('list'), r); },
    docEditFront() {
      const c = this.docColl('record');
      if (c) this.openRecord(c, this.docFront() || { _id: 'record', _rev: '' });
    },
    // docAdd opens a new section; with a section given, it goes right after it
    docAdd(after) {
      const c = this.docColl('list');
      if (!c || this.offline) return;
      this.openNew(c);
      if (after) this.sheet.at = this.docAll().findIndex(r => r._id === after._id) + 1;
    },
    docMeta() {
      const f = this.docFront() || {}, out = [`${this.docAll().length} sections`];
      if (f.reviewed) out.push('reviewed ' + this.fmtDate(f.reviewed, ''));
      return out;
    },

    // md renders Markdown to HTML (cached by text).
    md(src) {
      src = this.text(src);
      if (!src) return '';
      let html = this._mdCache.get(src);
      if (html === undefined) {
        html = window.marked ? window.marked.parse(src, { gfm: true }) : src.replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
        if (this._mdCache.size > 300) this._mdCache.clear();
        this._mdCache.set(src, html);
      }
      return html;
    },
    // mdEnhance runs after the HTML is in place: callouts, wide tables, outside links, diagrams.
    mdEnhance(el) {
      if (!el) return;
      for (const bq of el.querySelectorAll('blockquote')) {
        const p = bq.firstElementChild;
        const m = p?.tagName === 'P' && /^\[!(\w+)\][+-]?\s*([^\n]*)/.exec(p.textContent);
        if (!m) continue;
        const kind = m[1].toLowerCase();
        const box = document.createElement('div');
        box.className = 'callout ' + ({ warning: 'warn', caution: 'warn', attention: 'warn', danger: 'bad', error: 'bad', bug: 'bad',
          tip: 'ok', success: 'ok', check: 'ok', done: 'ok', important: 'accent', info: 'accent' }[kind] || 'note');
        const head = document.createElement('p');
        head.className = 'callout-title';
        head.textContent = m[2] || kind[0].toUpperCase() + kind.slice(1);
        // the rest of the first paragraph (after its first line) stays as text
        const firstLine = p.innerHTML.split('\n');
        firstLine.shift();
        if (firstLine.join('').trim()) p.innerHTML = firstLine.join('\n'); else p.remove();
        box.append(head, ...bq.childNodes);
        bq.replaceWith(box);
      }
      for (const t of el.querySelectorAll('table')) {
        if (t.parentElement.classList.contains('table-wrap')) continue;
        const head = t.tHead; // "| | |" = a table without column titles
        if (head && ![...head.querySelectorAll('th')].some(th => th.textContent.trim())) head.remove();
        const w = document.createElement('div');
        w.className = 'table-wrap';
        t.replaceWith(w);
        w.append(t);
      }
      for (const a of el.querySelectorAll('a[href^="http"]')) { a.target = '_blank'; a.rel = 'noopener noreferrer'; }
      const diagrams = [];
      for (const code of el.querySelectorAll('pre > code.language-mermaid')) {
        const box = document.createElement('div');
        box.className = 'mermaid-box';
        box.dataset.src = code.textContent;
        code.parentElement.replaceWith(box);
        diagrams.push(box);
      }
      if (diagrams.length) this.drawDiagrams(diagrams);
    },
    async loadMermaid() {
      if (!this._mermaid) {
        this._mermaid = new Promise((ok, bad) => {
          const s = document.createElement('script');
          s.src = '/assets/vendor/mermaid.min.js?v=' + (document.querySelector('.app')?.dataset.version || '');
          s.onload = () => ok(window.mermaid);
          s.onerror = () => { this._mermaid = null; bad(new Error('diagram library did not load')); };
          document.head.append(s);
        });
      }
      return this._mermaid;
    },
    _dark() { return this.theme === 'dark' || (this.theme === 'auto' && !matchMedia('(prefers-color-scheme: light)').matches); },
    async drawDiagrams(boxes) {
      let mermaid;
      try { mermaid = await this.loadMermaid(); } catch (e) {
        for (const b of boxes) { b.textContent = b.dataset.src; b.classList.add('failed'); }
        return;
      }
      const css = getComputedStyle(document.documentElement), v = (k) => css.getPropertyValue(k).trim();
      mermaid.initialize({
        startOnLoad: false, securityLevel: 'strict', theme: 'base',
        fontFamily: v('--nss-sans') || 'system-ui, sans-serif',
        themeVariables: {
          darkMode: this._dark(), background: v('--nss-surface'), fontSize: '14px',
          primaryColor: v('--nss-surface-2'), primaryTextColor: v('--nss-fg'), primaryBorderColor: v('--nss-accent'),
          lineColor: v('--nss-muted'), secondaryColor: v('--nss-surface'), tertiaryColor: v('--nss-bg'),
          clusterBkg: v('--nss-bg'), clusterBorder: v('--nss-line'), edgeLabelBackground: v('--nss-surface'),
          noteBkgColor: v('--nss-surface-2'), noteTextColor: v('--nss-fg'), noteBorderColor: v('--nss-line'),
          actorBkg: v('--nss-surface-2'), actorBorder: v('--nss-accent'), actorTextColor: v('--nss-fg'),
          signalColor: v('--nss-muted'), signalTextColor: v('--nss-fg'),
          pie1: v('--nss-accent'), pie2: v('--nss-warn'), pie3: v('--nss-surface-2'), pie4: v('--nss-accent-2'),
          pie5: v('--nss-ok'), pie6: v('--nss-bad'), pieStrokeColor: v('--nss-bg'), pieOuterStrokeColor: v('--nss-line'),
          pieTitleTextColor: v('--nss-fg'), pieSectionTextColor: v('--nss-fg'), pieLegendTextColor: v('--nss-fg'),
          pieTitleTextSize: '16px', pieSectionTextSize: '13px', pieLegendTextSize: '13px',
          taskBkgColor: v('--nss-accent'), taskBorderColor: v('--nss-accent'), taskTextColor: v('--nss-fg'),
          taskTextLightColor: v('--nss-on-accent'), taskTextOutsideColor: v('--nss-fg'), sectionBkgColor: v('--nss-surface-2'),
          altSectionBkgColor: v('--nss-surface'), sectionBkgColor2: v('--nss-surface-2'), gridColor: v('--nss-line'),
          titleColor: v('--nss-fg'), milestoneBkgColor: v('--nss-warn'), milestoneBorderColor: v('--nss-warn'),
        },
      });
      for (const b of boxes) {
        if (!b.isConnected) continue;
        try {
          const { svg } = await mermaid.render('mmd-' + Math.random().toString(36).slice(2), b.dataset.src);
          b.innerHTML = svg;
          b.classList.remove('failed');
        } catch (e) {
          b.textContent = 'Diagram error: ' + (e?.message || e) + '\n\n' + b.dataset.src;
          b.classList.add('failed');
        }
      }
    },
    // redrawDiagrams re-colours every diagram on screen (after a theme change)
    redrawDiagrams() {
      const boxes = [...document.querySelectorAll('.mermaid-box')];
      if (boxes.length) this.drawDiagrams(boxes);
    },
}));
