// base.js — loaded first: shared constants (icons, badge tones, chart state, ApiError)
// and part(), with which every js/*.js file adds its piece of the Alpine component.


// A restored back/forward-cache page can start Alpine twice; reload instead.
window.addEventListener('pageshow', (e) => { if (e.persisted) location.reload(); });

const ICONS = {
  home: '<path d="M3 11l9-7 9 7"/><path d="M5 10v10h14V10"/>',
  calendar: '<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/>',
  wallet: '<rect x="3" y="6" width="18" height="14" rx="2"/><path d="M3 10h18M16 15h2"/>',
  gift: '<rect x="3" y="9" width="18" height="12" rx="1"/><path d="M12 9v12M3 13h18M12 9c-2-4-6-4-6-1s6 1 6 1zm0 0c2-4 6-4 6-1s-6 1-6 1z"/>',
  heart: '<path d="M12 20s-7-4.5-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.5-7 10-7 10z"/>',
  box: '<path d="M3 7l9-4 9 4v10l-9 4-9-4z"/><path d="M3 7l9 4 9-4M12 11v10"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  left: '<path d="M15 6l-6 6 6 6"/>',
  right: '<path d="M9 6l6 6-6 6"/>',
  link: '<path d="M14 4h6v6M20 4l-9 9M18 14v6H4V6h6"/>',
  book: '<path d="M4 5a2 2 0 0 1 2-2h14v15H6a2 2 0 0 0-2 2z"/><path d="M4 20a2 2 0 0 0 2 2h14v-4M8 7h8M8 11h6"/>',
  edit: '<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="M13 7l4 4"/>',
  toc: '<path d="M4 6h16M4 12h10M4 18h13"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1L7 17M17 7l2.1-2.1"/>',
  folder: '<path d="M3 6a1 1 0 0 1 1-1h5l2 2h9a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/>',
  list: '<path d="M9 6h11M9 12h11M9 18h11"/><circle cx="4.5" cy="6" r="1"/><circle cx="4.5" cy="12" r="1"/><circle cx="4.5" cy="18" r="1"/>',
  lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>',
  grid: '<rect x="4" y="4" width="7" height="7" rx="1.5"/><rect x="13" y="4" width="7" height="7" rx="1.5"/><rect x="4" y="13" width="7" height="7" rx="1.5"/><rect x="13" y="13" width="7" height="7" rx="1.5"/>',
  sync: '<path d="M20 12a8 8 0 0 1-13.7 5.7M4 12a8 8 0 0 1 13.7-5.7"/><path d="M18 3v4h-4M6 21v-4h4"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  search: '<circle cx="11" cy="11" r="6"/><path d="M20 20l-4.5-4.5"/>',
  refresh: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>',
  x: '<path d="M6 6l12 12M18 6L6 18"/>',
  more: '<circle cx="5" cy="12" r="1.3"/><circle cx="12" cy="12" r="1.3"/><circle cx="19" cy="12" r="1.3"/>',
  logout: '<path d="M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5L19 19M5 19l1.5-1.5M17.5 6.5L19 5"/>',
  moon: '<path d="M20 14A8 8 0 1 1 10 4a6 6 0 0 0 10 10z"/>',
  bookmark: '<path d="M6 3h12v18l-6-4-6 4z"/>',
  pin: '<path d="M12 21s-7-6.5-7-12a7 7 0 0 1 14 0c0 5.5-7 12-7 12z"/><circle cx="12" cy="9" r="2.5"/>',
  auto: '<circle cx="12" cy="12" r="8"/><path d="M12 4a8 8 0 0 1 0 16z" fill="currentColor"/>',
};

// Badge colours for common values; plugin.yml `tones:` can add more.
const TONES = {
  open: 'accent', pending: 'warn', closed: 'muted', done: 'ok', 'to do': 'warn',
  high: 'bad', medium: 'warn', low: 'muted', active: 'ok', cancelled: 'muted',
  premium: 'ok', excellent: 'ok', good: '', used: '', worn: 'warn', replace: 'bad',
};

// charts: parsed days per loaded list, and the points of each drawn chart (not reactive)
const CH = { days: new WeakMap(), pts: {} };

class ApiError extends Error {
  constructor(message, status, fields) { super(message); this.status = status; this.fields = fields || {}; }
}

// part(name, make): registers one piece of the component; make() returns its state and methods.
const NATLAS_PARTS = [];
function part(name, make) { NATLAS_PARTS.push({ name, make }); }
