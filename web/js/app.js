/*
 * Natlas web app - one Alpine component drives every screen.
 *
 * Nothing here knows about events, birthdays or inventory: the server sends
 * each plugin's definition (fields, list layout, actions) from plugin.yml and
 * the parts render it. Each file in js/ is one part, loaded in this order
 * (index.html), then merged here into the component `natlas`:
 *
 *   base.js ...... constants, ApiError, part()          (loaded first)
 *   core.js ...... state, api, routing
 *   today.js ..... Today screen, layout.yml, Customize
 *   lists.js ..... search, filters, groups, summary, formatting, dates
 *   interact.js .. gestures, selection & bulk, badge menu, actions, ask dialog
 *   calendar.js .. month / week / schedule (`view: calendar`)
 *   document.js .. Markdown note as sections (`view: document`)
 *   charts.js .... Today / Trends / Sleep / Body (`charts:`)
 *   sheet.js ..... edit form, toasts, undo
 *   phone.js ..... Android app settings (window.NatlasNative), sync conflicts
 *   shell.js ..... service worker, update prompt, theme, icons
 *   app.js ....... this file: merges the parts (loaded last, before Alpine)
 */
document.addEventListener('alpine:init', () => {
  Alpine.data('natlas', () => {
    const c = {};
    // property descriptors keep getters working; a later part never silently replaces an earlier key
    for (const p of NATLAS_PARTS) {
      const d = Object.getOwnPropertyDescriptors(p.make());
      for (const k of Object.keys(d)) if (k in c) console.warn(`natlas: ${p.name} redefines ${k}`);
      Object.defineProperties(c, d);
    }
    return c;
  });
});
