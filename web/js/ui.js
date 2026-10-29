// Small DOM toolkit: element builder, icons, dialogs, menus, tooltips and
// form controls shared by all views.
import { t } from './i18n.js';
import { hex, hashColor, dateKey, parseKey, weekStart } from './fmt.js';
import { iconURL, siteIconURL } from './api.js';

// Native append() turns null/false into text; skip them like h() does so
// views can pass optional children inline.
for (const proto of [Element.prototype, DocumentFragment.prototype]) {
  const native = proto.append;
  proto.append = function (...nodes) {
    native.apply(this, nodes.filter((n) => n !== null && n !== undefined && n !== false));
  };
}

export function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v === undefined || v === null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k === 'style' && typeof v === 'object') {
        for (const [sk, sv] of Object.entries(v)) {
          if (sk.startsWith('--')) el.style.setProperty(sk, sv);
          else el.style[sk] = sv;
        }
      }
      else if (k === 'html') el.innerHTML = v;
      else if (k === 'dataset') Object.assign(el.dataset, v);
      else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
      else if (v === true) el.setAttribute(k, '');
      else el.setAttribute(k, v);
    }
  }
  append(el, children);
  return el;
}

function append(el, children) {
  for (const c of children) {
    if (c === null || c === undefined || c === false) continue;
    if (Array.isArray(c)) append(el, c);
    else el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
}

export const $ = (sel, root = document) => root.querySelector(sel);
export const clear = (el) => { while (el.firstChild) el.firstChild.remove(); return el; };

// Lucide-style 24px stroke icons.
const P = {
  day: '<rect x="3" y="4" width="18" height="17" rx="2"/><path d="M3 9h18M8 2v4M16 2v4"/><path d="M7 13h4M7 17h7"/>',
  stats: '<path d="M3 3v18h18"/><path d="M7 15v3M11 10v8M15 6v12M19 12v6"/>',
  sheet: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M3 15h18M9 3v18"/>',
  tag: '<path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L3 13V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z"/><circle cx="7.5" cy="7.5" r="1.5"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  wand: '<path d="m15 4 1.5-1.5M20 9l1.5-1.5M15 9l-11.5 11.5M9.5 3.5 10 2M20.5 14l1.5.5M14 2.5v2M19.5 4.5l-1 1"/><path d="M15 9l1.5 1.5"/>',
  image: '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-5-5L5 21"/>',
  settings: '<path d="M12.2 2h-.4a2 2 0 0 0-2 2v.2a2 2 0 0 1-1 1.7l-.4.3a2 2 0 0 1-2 0l-.2-.1a2 2 0 0 0-2.7.7l-.2.4a2 2 0 0 0 .7 2.7l.2.1a2 2 0 0 1 1 1.7v.6a2 2 0 0 1-1 1.7l-.2.1a2 2 0 0 0-.7 2.7l.2.4a2 2 0 0 0 2.7.7l.2-.1a2 2 0 0 1 2 0l.4.3a2 2 0 0 1 1 1.7v.2a2 2 0 0 0 2 2h.4a2 2 0 0 0 2-2v-.2a2 2 0 0 1 1-1.7l.4-.3a2 2 0 0 1 2 0l.2.1a2 2 0 0 0 2.7-.7l.2-.4a2 2 0 0 0-.7-2.7l-.2-.1a2 2 0 0 1-1-1.7v-.6a2 2 0 0 1 1-1.7l.2-.1a2 2 0 0 0 .7-2.7l-.2-.4a2 2 0 0 0-2.7-.7l-.2.1a2 2 0 0 1-2 0l-.4-.3a2 2 0 0 1-1-1.7V4a2 2 0 0 0-2-2Z"/><circle cx="12" cy="12" r="3"/>',
  left: '<path d="m15 18-6-6 6-6"/>',
  right: '<path d="m9 18 6-6-6-6"/>',
  down: '<path d="m6 9 6 6 6-6"/>',
  up: '<path d="m18 15-6-6-6 6"/>',
  play: '<path d="M7 4v16l13-8Z"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="1.5"/>',
  pause: '<rect x="6" y="5" width="4" height="14" rx="1"/><rect x="14" y="5" width="4" height="14" rx="1"/>',
  timer: '<circle cx="12" cy="13" r="8"/><path d="M12 9v4l2 2M10 2h4"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  minus: '<path d="M5 12h14"/>',
  trash: '<path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>',
  edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
  paste: '<path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/><rect x="8" y="2" width="8" height="4" rx="1"/>',
  external: '<path d="M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
  folder: '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2Z"/>',
  file: '<path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5Z"/><path d="M14 2v6h6M8 13h8M8 17h5"/>',
  globe: '<circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 0 1 0 20M12 2a15 15 0 0 0 0 20"/>',
  monitor: '<rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/>',
  clock: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
  zoomIn: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5M11 8v6M8 11h6"/>',
  zoomOut: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5M8 11h6"/>',
  fit: '<path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3"/>',
  filter: '<path d="M22 3H2l8 9.5V19l4 2v-8.5Z"/>',
  download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>',
  upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  refresh: '<path d="M3 12a9 9 0 0 1 15-6.7L21 8M21 3v5h-5M21 12a9 9 0 0 1-15 6.7L3 16M8 16H3v5"/>',
  info: '<circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/>',
  alert: '<path d="m21.7 18-8-14a2 2 0 0 0-3.4 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.7-3Z"/><path d="M12 9v4M12 17h.01"/>',
  more: '<circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/>',
  camera: '<path d="M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3Z"/><circle cx="12" cy="13" r="3"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  moon: '<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>',
  keyboard: '<rect x="2" y="4" width="20" height="16" rx="2"/><path d="M6 8h.01M10 8h.01M14 8h.01M18 8h.01M6 12h.01M10 12h.01M14 12h.01M18 12h.01M7 16h10"/>',
  database: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.7 4 3 9 3s9-1.3 9-3V5M3 12c0 1.7 4 3 9 3s9-1.3 9-3"/>',
  puzzle: '<path d="M19.4 14.9c-.5.2-1 .1-1.4-.3l-1.6-1.6a2.4 2.4 0 1 0-3.4 3.4l1.6 1.6c.4.4.5 1 .3 1.4a2 2 0 0 1-1.6 1.3 2 2 0 0 1-1.4-.6L9.5 17.7a2 2 0 0 0-1.4-.6 2.4 2.4 0 1 1 0-4.8c.5 0 1-.2 1.4-.6l2.4-2.4c.4-.4.6-.9.6-1.4a2.4 2.4 0 1 1 4.8 0c0 .5.2 1 .6 1.4l2.4 2.4a2 2 0 0 1 .6 1.4 2 2 0 0 1-1.4 1.8Z"/>',
  sparkles: '<path d="m12 3-1.9 5.8a2 2 0 0 1-1.3 1.3L3 12l5.8 1.9a2 2 0 0 1 1.3 1.3L12 21l1.9-5.8a2 2 0 0 1 1.3-1.3L21 12l-5.8-1.9a2 2 0 0 1-1.3-1.3Z"/>',
  dollar: '<path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>',
  scissors: '<circle cx="6" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M20 4 8.1 15.9M14.5 14.5 20 20M8.1 8.1 12 12"/>',
  eyeOff: '<path d="M9.9 4.2A10 10 0 0 1 12 4c7 0 10 8 10 8a13 13 0 0 1-1.7 2.7M6.6 6.6A13.5 13.5 0 0 0 2 12s3 8 10 8a9.7 9.7 0 0 0 5.4-1.6M2 2l20 20M9.9 9.9a3 3 0 0 0 4.2 4.2"/>',
  app: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
  arrowUp: '<path d="M12 19V5M5 12l7-7 7 7"/>',
  arrowDown: '<path d="M12 5v14M19 12l-7 7-7-7"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>',
  power: '<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.8 0"/>',
  link: '<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>',
  menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  grip: '<circle cx="9" cy="6" r="1.2"/><circle cx="15" cy="6" r="1.2"/><circle cx="9" cy="12" r="1.2"/><circle cx="15" cy="12" r="1.2"/><circle cx="9" cy="18" r="1.2"/><circle cx="15" cy="18" r="1.2"/>',
  width: '<path d="M3 12h18M7 8l-4 4 4 4M17 8l4 4-4 4"/>',
  pin: '<path d="M12 17v5"/><path d="M9 10.8a2 2 0 0 1-1.1 1.8l-1.8.9A2 2 0 0 0 5 15.2v.8a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.8a2 2 0 0 0-1.1-1.8l-1.8-.9a2 2 0 0 1-1.1-1.8V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1Z"/>',
  layers: '<path d="m12 2 10 5-10 5L2 7Z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/>',
  hourglass: '<path d="M5 22h14M5 2h14M17 22v-4.2a2 2 0 0 0-.6-1.4L12 12l-4.4 4.4a2 2 0 0 0-.6 1.4V22M7 2v4.2a2 2 0 0 0 .6 1.4L12 12l4.4-4.4a2 2 0 0 0 .6-1.4V2"/>',
};

export function icon(name, size = 16, cls = '') {
  const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  s.setAttribute('viewBox', '0 0 24 24');
  s.setAttribute('width', size);
  s.setAttribute('height', size);
  s.setAttribute('fill', name === 'play' ? 'currentColor' : 'none');
  s.setAttribute('stroke', 'currentColor');
  s.setAttribute('stroke-width', '2');
  s.setAttribute('stroke-linecap', 'round');
  s.setAttribute('stroke-linejoin', 'round');
  s.setAttribute('aria-hidden', 'true');
  s.setAttribute('class', 'icon ' + cls);
  s.innerHTML = P[name] || P.info;
  return s;
}

export function button(label, opts = {}) {
  const b = h('button', {
    class: 'btn ' + (opts.variant ? 'btn-' + opts.variant : '') + (opts.small ? ' btn-sm' : '') + (label ? '' : ' btn-icon') + (opts.class ? ' ' + opts.class : ''),
    type: 'button', title: opts.title, onClick: opts.onClick, disabled: opts.disabled,
  }, opts.icon ? icon(opts.icon, opts.small ? 14 : 16) : null, label ? h('span', null, label) : null);
  return b;
}

/** App icon image, or a coloured letter tile when no icon was captured. */
export function appIcon(app, size = 18) {
  if (app && app.icon) {
    const img = h('img', { class: 'app-icon', src: iconURL(app.id, app.iconV), width: size, height: size, alt: '', loading: 'lazy', draggable: 'false' });
    img.addEventListener('error', () => img.replaceWith(appIcon({ ...app, icon: false }, size)), { once: true });
    return img;
  }
  const name = (app && app.name) || '?';
  return h('span', { class: 'app-icon letter', style: { width: size + 'px', height: size + 'px', background: hex(app && app.color), fontSize: Math.round(size * 0.55) + 'px' } }, name.trim().charAt(0).toUpperCase());
}

/** Lets the mouse wheel scroll el sideways, smoothly; at either end the page
 *  scrolls as usual. */
export function wheelScrollsX(el) {
  let target = null, raf = 0;
  const step = () => {
    const before = el.scrollLeft;
    const d = target - before;
    el.scrollLeft = Math.abs(d) < 2 ? target : before + d * 0.3;
    if (el.scrollLeft === target || el.scrollLeft === before) { el.scrollLeft = target; target = null; raf = 0; return; }
    raf = requestAnimationFrame(step);
  };
  el.addEventListener('wheel', (e) => {
    if (e.ctrlKey || e.shiftKey || Math.abs(e.deltaX) > Math.abs(e.deltaY)) return;
    const max = el.scrollWidth - el.clientWidth;
    if (max <= 0) return;
    const from = target ?? el.scrollLeft;
    const d = e.deltaY * (e.deltaMode === 1 ? 40 : e.deltaMode === 2 ? el.clientWidth : 1) * 1.5;
    const next = Math.max(0, Math.min(max, from + d));
    if (Math.abs(next - from) < 1) return; // at the end: the page scrolls
    e.preventDefault();
    target = next;
    if (!raf) raf = requestAnimationFrame(step);
  }, { passive: false });
}

/** Host name of a website key (a host or a URL) that can have an icon. */
export function siteHost(key) {
  let host = String(key || '').toLowerCase();
  if (/^[a-z][a-z0-9+.-]*:\/\//.test(host)) {
    try { host = new URL(host).hostname; } catch (_) { return ''; }
  }
  host = host.split(/[/:?#]/)[0];
  return /^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(host) && !/^[\d.]+$/.test(host) ? host : '';
}

/** Website icon (from the browsers' cache or the site), or a coloured globe;
 *  a file icon for files. */
export function kindIcon(kind, key, size = 16) {
  if (kind === 2) {
    const globe = () => h('span', { class: 'doc-icon', style: { background: hashColor(key || '') } }, icon('globe', size - 4));
    const host = siteHost(key);
    if (!host) return globe();
    const img = h('img', { class: 'site-icon', src: siteIconURL(host), width: size, height: size, alt: '', loading: 'lazy', draggable: 'false' });
    img.addEventListener('error', () => img.replaceWith(globe()), { once: true });
    return img;
  }
  return h('span', { class: 'doc-icon file' }, icon('file', size - 4));
}

// ---- Overlays -----------------------------------------------------------

let overlayRoot;
function root() {
  if (!overlayRoot) overlayRoot = document.getElementById('overlays');
  return overlayRoot;
}

const openModals = [];

export function modal({ title, body, actions = [], width = 460, onClose, className = '' }) {
  const close = (v) => {
    if (!wrap.isConnected) return;
    wrap.classList.add('closing');
    setTimeout(() => wrap.remove(), 120);
    openModals.splice(openModals.indexOf(api), 1);
    onClose && onClose(v);
  };
  const footer = actions.length
    ? h('div', { class: 'modal-actions' }, actions.map((a) => {
      const b = button(a.label, { variant: a.primary ? 'primary' : a.danger ? 'danger' : 'ghost', icon: a.icon, onClick: () => a.onClick ? a.onClick(close) : close(a.value) });
      if (a.left) b.classList.add('left');
      return b;
    }))
    : null;
  const box = h('div', { class: 'modal ' + className, role: 'dialog', 'aria-modal': 'true', style: { width: width + 'px' } },
    title ? h('div', { class: 'modal-head' }, h('h3', null, title), button('', { icon: 'x', variant: 'ghost', small: true, title: t('Close'), onClick: () => close() })) : null,
    h('div', { class: 'modal-body' }, body),
    footer);
  const wrap = h('div', { class: 'modal-wrap', onMousedown: (e) => { if (e.target === wrap) close(); } }, box);
  root().append(wrap);
  const api = { close, el: box, wrap };
  openModals.push(api);
  requestAnimationFrame(() => {
    const f = box.querySelector('[autofocus]') || box.querySelector('input, textarea, select');
    f && f.focus();
  });
  return api;
}

export function topModal() { return openModals[openModals.length - 1]; }

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') {
    if (closeMenu()) { e.stopPropagation(); return; }
    const m = topModal();
    if (m) { e.stopPropagation(); e.preventDefault(); m.close(); }
  }
}, true);

export function confirmDialog(text, { title = t('Confirm'), ok = t('OK'), danger = false } = {}) {
  return new Promise((resolve) => {
    modal({
      title, body: h('p', { class: 'muted' }, text), width: 420,
      actions: [{ label: t('Cancel'), value: false }, { label: ok, primary: !danger, danger, value: true }],
      onClose: (v) => resolve(!!v),
    });
  });
}

export function promptDialog(title, value = '', { label = '', ok = t('Save') } = {}) {
  return new Promise((resolve) => {
    const input = h('input', { class: 'input', value, autofocus: true });
    let result = null;
    const m = modal({
      title, width: 420,
      body: h('label', { class: 'field' }, label ? h('span', null, label) : null, input),
      actions: [{ label: t('Cancel') }, { label: ok, primary: true, onClick: (close) => { result = input.value; close(); } }],
      onClose: () => resolve(result),
    });
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { result = input.value; m.close(); } });
  });
}

export function toast(msg, { type = 'info', timeout = 3200, action } = {}) {
  let host = document.getElementById('toasts');
  const el = h('div', { class: 'toast toast-' + type }, icon(type === 'error' ? 'alert' : type === 'ok' ? 'check' : 'info', 16), h('span', null, msg),
    action ? button(action.label, { small: true, variant: 'ghost', onClick: () => { action.onClick(); el.remove(); } }) : null);
  host.append(el);
  setTimeout(() => { el.classList.add('closing'); setTimeout(() => el.remove(), 200); }, timeout);
}

// ---- Context menu -------------------------------------------------------

let menuEl = null;
export function closeMenu() {
  if (!menuEl) return false;
  menuEl.remove();
  menuEl = null;
  return true;
}

export function menu(x, y, items) {
  closeMenu();
  menuEl = h('div', { class: 'menu', role: 'menu' }, items.filter(Boolean).map((it) => it.separator
    ? h('div', { class: 'menu-sep' })
    : h('button', {
      class: 'menu-item' + (it.danger ? ' danger' : ''), disabled: it.disabled, role: 'menuitem',
      onClick: () => { closeMenu(); it.onClick && it.onClick(); },
    }, it.icon ? icon(it.icon, 15) : h('span', { class: 'icon-space' }), h('span', { class: 'menu-label' }, it.label), it.hint ? h('kbd', null, it.hint) : null)));
  root().append(menuEl);
  const r = menuEl.getBoundingClientRect();
  menuEl.style.left = Math.min(x, innerWidth - r.width - 8) + 'px';
  menuEl.style.top = Math.min(y, innerHeight - r.height - 8) + 'px';
  setTimeout(() => {
    const off = (e) => { if (menuEl && !menuEl.contains(e.target)) { closeMenu(); document.removeEventListener('mousedown', off, true); } };
    document.addEventListener('mousedown', off, true);
  });
}

/** Dropdown anchored under an element, or beside it with side 'right'. */
export function dropdown(anchor, items, side) {
  const r = anchor.getBoundingClientRect();
  if (side === 'right') menu(r.right + 8, r.top, items);
  else menu(r.left, r.bottom + 4, items);
}

// ---- Tooltip ------------------------------------------------------------

let tipEl = null;
export function showTip(x, y, content) {
  if (!tipEl) {
    tipEl = h('div', { class: 'tip' });
    root().append(tipEl);
  }
  if (typeof content === 'string') tipEl.innerHTML = content;
  else { clear(tipEl); tipEl.append(content); }
  tipEl.style.display = 'block';
  const r = tipEl.getBoundingClientRect();
  let left = x + 14, top = y + 16;
  if (left + r.width > innerWidth - 8) left = x - r.width - 14;
  if (top + r.height > innerHeight - 8) top = y - r.height - 12;
  tipEl.style.left = Math.max(8, left) + 'px';
  tipEl.style.top = Math.max(8, top) + 'px';
}
export function hideTip() { if (tipEl) tipEl.style.display = 'none'; }

// ---- Controls -----------------------------------------------------------

export function toggle(checked, onChange, label) {
  const input = h('input', { type: 'checkbox', checked, onChange: (e) => onChange(e.target.checked) });
  return h('label', { class: 'switch' }, input, h('span', { class: 'switch-track' }), label ? h('span', { class: 'switch-label' }, label) : null);
}

export function select(options, value, onChange, attrs = {}) {
  const s = h('select', { class: 'input select', ...attrs, onChange: (e) => onChange(e.target.value) },
    options.map(([v, l]) => h('option', { value: v, selected: String(v) === String(value) }, l)));
  return s;
}

// Where each set of options last had its highlight, so a control that is
// rebuilt (as range bars are on every change) slides from there.
const segLast = new Map();

export function segmented(options, value, onChange) {
  const wrap = h('div', { class: 'segmented', role: 'tablist' });
  const pill = h('span', { class: 'seg-pill' });
  const key = options.map((o) => o[0]).join('|');
  // The highlight is a pill behind the buttons that slides to the selected one.
  const place = () => {
    const on = wrap.querySelector('button.on');
    if (!on || !on.offsetWidth) return;
    const to = { x: on.offsetLeft, w: on.offsetWidth };
    const from = segLast.get(key);
    segLast.set(key, to);
    pill.style.transition = 'none';
    if (from) { pill.style.transform = `translateX(${from.x}px)`; pill.style.width = from.w + 'px'; }
    wrap.classList.add('has-pill');
    pill.getBoundingClientRect();
    pill.style.transition = '';
    pill.style.transform = `translateX(${to.x}px)`;
    pill.style.width = to.w + 'px';
  };
  const render = (v) => {
    clear(wrap);
    wrap.append(pill);
    options.forEach(([val, label, ic]) => wrap.append(h('button', {
      type: 'button', role: 'tab', class: String(val) === String(v) ? 'on' : '', 'aria-selected': String(val) === String(v),
      onClick: () => { render(val); onChange(val); },
    }, ic ? icon(ic, 14) : null, label ? h('span', null, label) : null)));
    requestAnimationFrame(place);
  };
  render(value);
  wrap.set = render;
  return wrap;
}

export function empty(iconName, title, text, action) {
  return h('div', { class: 'empty' }, h('div', { class: 'empty-icon' }, icon(iconName, 22)), h('div', { class: 'empty-title' }, title),
    text ? h('div', { class: 'empty-text' }, text) : null, action || null);
}

/** Tag entry with autocompletion on the last comma-separated part. */
export function tagInput({ value = '', names = [], combos = [], placeholder = t('Tags, e.g. Client, Project'), onEnter, autofocus } = {}) {
  const input = h('input', { class: 'input', value, placeholder, autofocus, spellcheck: 'false', autocomplete: 'off' });
  const list = h('div', { class: 'suggest' });
  const wrap = h('div', { class: 'tag-input' }, input, list);
  let items = [], active = -1;
  const render = () => {
    clear(list);
    const v = input.value;
    const parts = v.split(',');
    const last = parts.pop().trim().toLowerCase();
    const prefix = parts.length ? parts.map((p) => p.trim()).filter(Boolean).join(', ') + ', ' : '';
    const seen = new Set();
    items = [];
    if (!prefix) {
      combos.filter((c) => c.toLowerCase().includes(last) && c !== v).slice(0, 6).forEach((c) => { seen.add(c); items.push({ label: c, value: c, combo: true }); });
    }
    names.filter((n) => n.toLowerCase().includes(last) && !seen.has(prefix + n) && (prefix + n) !== v).slice(0, 8)
      .forEach((n) => items.push({ label: n, value: prefix + n }));
    items = items.slice(0, 10);
    if (!items.length || document.activeElement !== input) { list.style.display = 'none'; return; }
    active = Math.min(active, items.length - 1);
    items.forEach((it, i) => list.append(h('div', {
      class: 'suggest-item' + (i === active ? ' on' : ''),
      onMousedown: (e) => { e.preventDefault(); pick(it); },
    }, icon(it.combo ? 'tag' : 'plus', 13), h('span', null, it.label))));
    list.style.display = 'block';
  };
  const pick = (it) => { input.value = it.value; active = -1; render(); input.dispatchEvent(new Event('input')); };
  input.addEventListener('input', () => { active = -1; render(); });
  input.addEventListener('focus', render);
  input.addEventListener('blur', () => setTimeout(() => (list.style.display = 'none'), 100));
  input.addEventListener('keydown', (e) => {
    if (list.style.display === 'block' && items.length) {
      if (e.key === 'ArrowDown') { active = (active + 1) % items.length; render(); e.preventDefault(); return; }
      if (e.key === 'ArrowUp') { active = (active - 1 + items.length) % items.length; render(); e.preventDefault(); return; }
      if ((e.key === 'Enter' || e.key === 'Tab') && active >= 0) { pick(items[active]); e.preventDefault(); return; }
    }
    if (e.key === 'Enter' && onEnter) { e.preventDefault(); onEnter(input.value); }
  });
  wrap.input = input;
  Object.defineProperty(wrap, 'value', { get: () => input.value, set: (v) => { input.value = v; } });
  return wrap;
}

/** Calendar popover; resolves with a YYYY-MM-DD key or null. */
export function pickDate(anchor, value, { marks } = {}) {
  return new Promise((resolve) => {
    closeMenu();
    let cur = parseKey(value);
    cur.setDate(1);
    const pop = h('div', { class: 'menu calendar' });
    const draw = () => {
      clear(pop);
      const title = cur.toLocaleDateString(document.documentElement.lang || 'en', { year: 'numeric', month: 'long' });
      pop.append(h('div', { class: 'cal-head' },
        button('', { icon: 'left', small: true, variant: 'ghost', onClick: () => { cur.setMonth(cur.getMonth() - 1); draw(); } }),
        h('span', null, title),
        button('', { icon: 'right', small: true, variant: 'ghost', onClick: () => { cur.setMonth(cur.getMonth() + 1); draw(); } })));
      const grid = h('div', { class: 'cal-grid' });
      const ws = weekStart();
      for (let i = 0; i < 7; i++) {
        const d = new Date(2024, 0, 7 + ((i + ws) % 7));
        grid.append(h('span', { class: 'cal-dow' }, d.toLocaleDateString(document.documentElement.lang || 'en', { weekday: 'narrow' })));
      }
      const first = new Date(cur);
      const offset = (first.getDay() - ws + 7) % 7;
      first.setDate(1 - offset);
      const todayKey = dateKey(new Date());
      for (let i = 0; i < 42; i++) {
        const d = new Date(first);
        d.setDate(first.getDate() + i);
        const k = dateKey(d);
        grid.append(h('button', {
          type: 'button',
          class: 'cal-day' + (d.getMonth() !== cur.getMonth() ? ' other' : '') + (k === value ? ' on' : '') + (k === todayKey ? ' today' : '') + (marks && marks.has(k) ? ' mark' : ''),
          onClick: () => { closeMenu(); resolve(k); },
        }, d.getDate()));
      }
      pop.append(grid, h('div', { class: 'cal-foot' }, button(t('Today'), { small: true, variant: 'ghost', onClick: () => { closeMenu(); resolve(todayKey); } })));
    };
    draw();
    menuEl = pop;
    root().append(pop);
    const r = anchor.getBoundingClientRect();
    const pr = pop.getBoundingClientRect();
    pop.style.left = Math.max(8, Math.min(r.left, innerWidth - pr.width - 8)) + 'px';
    pop.style.top = r.bottom + 6 + 'px';
    setTimeout(() => {
      const off = (e) => {
        if (menuEl === pop && !pop.contains(e.target)) { closeMenu(); resolve(null); document.removeEventListener('mousedown', off, true); }
      };
      document.addEventListener('mousedown', off, true);
    });
  });
}

/** Debounce helper. */
export function debounce(fn, ms) {
  let tm;
  return (...a) => { clearTimeout(tm); tm = setTimeout(() => fn(...a), ms); };
}
