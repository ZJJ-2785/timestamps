// App shell: sidebar, routing, live status, stopwatch, away prompt, theme.
import { h, clear, icon, button, modal, tagInput, toast, appIcon, dropdown, toggle } from './ui.js';
import { t, setLang } from './i18n.js';
import { configure, dur, clock, today, resetColors } from './fmt.js';
import * as api from './api.js';
import { state, refreshLive, loadTags, onChange } from './state.js';
import { go, parse, onRoute } from './router.js';
import * as dayView from './views/day.js';
import * as statsView from './views/stats.js';
import * as sheetView from './views/timesheet.js';
import * as tagsView from './views/tags.js';
import * as searchView from './views/search.js';
import * as rulesView from './views/rules.js';
import * as shotsView from './views/shots.js';
import * as settingsView from './views/settings.js';
import { mod } from './platform.js';

const VIEWS = {
  day: { mod: dayView, icon: 'day', label: 'Day view' },
  stats: { mod: statsView, icon: 'stats', label: 'Statistics' },
  timesheet: { mod: sheetView, icon: 'sheet', label: 'Timesheets' },
  tags: { mod: tagsView, icon: 'tag', label: 'Tags' },
  search: { mod: searchView, icon: 'search', label: 'Search' },
  rules: { mod: rulesView, icon: 'sparkles', label: 'Autotag' },
  screenshots: { mod: shotsView, icon: 'image', label: 'Screenshots' },
  settings: { mod: settingsView, icon: 'settings', label: 'Settings' },
};

let current = null;
let currentKey = '';
let lastRoute = null;
const viewEl = document.getElementById('view');
const nav = document.getElementById('nav');
const liveEl = document.getElementById('live');
const liveRail = document.getElementById('live-rail');
const timerEl = document.getElementById('timer');

// Light or dark mode, and the color theme picked for that mode (classic =
// none). A change redraws the current view with the new colors.
function applyTheme() {
  const s = state.settings || {};
  const pref = s.theme || 'system';
  const dark = pref === 'dark' || (pref === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  const root = document.documentElement;
  const palette = (dark ? s.darkTheme : s.lightTheme) || 'classic';
  try { localStorage.setItem('tsPalette', JSON.stringify({ light: s.lightTheme || 'classic', dark: s.darkTheme || 'classic' })); } catch (_) {}
  if (window.tsSetDark) window.tsSetDark(dark);
  if (root.dataset.theme === (dark ? 'dark' : 'light') && (root.dataset.palette || 'classic') === palette) return;
  const swap = () => {
    root.dataset.theme = dark ? 'dark' : 'light';
    if (palette === 'classic') delete root.dataset.palette; else root.dataset.palette = palette;
    resetColors();
    if (current && current.theme) current.theme();
    else if (lastRoute) { currentKey = ''; show(lastRoute); }
  };
  // Once the app is up, a new theme cross-fades in.
  if (document.startViewTransition && document.body.classList.contains('ready')) document.startViewTransition(swap);
  else swap();
}

// The app's logo: a ring that fills clockwise once a day and turns from
// green through yellow and orange to red, gray while paused (the same as the
// tray icon). Drawn in the sidebar and as the page icon.
const RING = [[0, [63, 178, 127]], [0.45, [242, 197, 61]], [0.75, [245, 146, 47]], [1, [229, 72, 77]]];
function ringColor(f) {
  for (let i = 1; i < RING.length; i++) {
    const [b, cb] = RING[i], [a, ca] = RING[i - 1];
    if (f <= b) return `rgb(${ca.map((v, k) => Math.round(v + (cb[k] - v) * ((f - a) / (b - a)))).join(',')})`;
  }
  return 'rgb(229,72,77)';
}
// Where the ring's round ends are centered, as angles clockwise from 12
// o'clock, for a ring of radius r and band width w (as ring.Draw): within the
// arc, which stops short of a full circle so the day doesn't read as over.
function ringEnds(f, r, w) {
  const a0 = w / 2 / r;
  const end = Math.min(f * 2 * Math.PI, 2 * Math.PI - Math.max(1.5, w / 2) / r);
  return [a0, Math.max(a0, end - a0)];
}
let logoKey = '';
function drawLogo() {
  const d = new Date();
  const f = Math.floor((d.getHours() * 60 + d.getMinutes()) / 5) / 288; // 5-minute steps
  const paused = state.live && state.live.state === 'paused';
  const key = f + '/' + paused;
  if (key === logoKey) return;
  logoKey = key;
  const color = paused ? 'rgb(139,147,167)' : ringColor(f);
  const track = 'rgba(128,134,148,0.43)';
  const r = 10.6, c = 2 * Math.PI * r, [a0, a1] = ringEnds(f, r, 4.8);
  const host = document.getElementById('brand-logo');
  if (host) {
    host.innerHTML = `<svg viewBox="0 0 28 28"><circle cx="14" cy="14" r="${r}" fill="none" stroke="${track}" stroke-width="4.8"/>` +
      (f > 0 ? `<circle cx="14" cy="14" r="${r}" fill="none" stroke="${color}" stroke-width="4.8" stroke-linecap="round" stroke-dasharray="${Math.max(0.01, (a1 - a0) * r).toFixed(2)} ${c.toFixed(2)}" stroke-dashoffset="${(-a0 * r).toFixed(2)}" transform="rotate(-90 14 14)"/>` : '') + '</svg>';
  }
  const cv = document.createElement('canvas');
  cv.width = cv.height = 64;
  const g = cv.getContext('2d');
  g.lineWidth = 11;
  g.strokeStyle = track;
  g.beginPath(); g.arc(32, 32, 24, 0, 2 * Math.PI); g.stroke();
  if (f > 0) {
    const [b0, b1] = ringEnds(f, 24, 11);
    g.strokeStyle = color; g.lineCap = 'round';
    g.beginPath(); g.arc(32, 32, 24, b0 - Math.PI / 2, Math.max(b1, b0 + 0.001) - Math.PI / 2); g.stroke();
  }
  const link = document.querySelector('link[rel="icon"]');
  if (link) link.href = cv.toDataURL('image/png');
}
setInterval(drawLogo, 30000);

// Easter egg: typing "dkhd" (outside a text box) switches the dkhd theme on
// for light and dark mode, and again switches back to the themes before it.
let eggKeys = '';
document.addEventListener('keydown', (e) => {
  if (e.ctrlKey || e.altKey || e.metaKey || e.key.length !== 1 || /INPUT|TEXTAREA|SELECT/.test(document.activeElement && document.activeElement.tagName)) return;
  eggKeys = (eggKeys + e.key.toLowerCase()).slice(-4);
  if (eggKeys !== 'dkhd' || !state.settings) return;
  eggKeys = '';
  const s = state.settings;
  let prev = null;
  try { prev = JSON.parse(localStorage.getItem('tsEggPrev') || 'null'); } catch (_) {}
  const on = s.lightTheme !== 'dkhd' || s.darkTheme !== 'dkhd';
  if (on) { try { localStorage.setItem('tsEggPrev', JSON.stringify({ light: s.lightTheme, dark: s.darkTheme })); localStorage.setItem('tsEgg', '1'); } catch (_) {} }
  const next = on ? { lightTheme: 'dkhd', darkTheme: 'dkhd' } : { lightTheme: (prev && prev.light) || 'classic', darkTheme: (prev && prev.dark) || 'classic' };
  Object.assign(s, next);
  applyTheme();
  api.post('settings', s).then((r) => { state.settings = r; }).catch(() => {});
  toast(on ? 'dkhd ✿' : t('Theme restored'), { timeout: 1600 });
});
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyTheme);

function resolveLang(s) {
  if (s.language && s.language !== 'auto') return s.language;
  return (navigator.language || 'en').toLowerCase().startsWith('zh') ? 'zh' : 'en';
}

function renderNav(active) {
  clear(nav);
  const item = (key, v) => h('a', { href: '#/' + key + (key === 'day' ? '/' + today() : ''), class: 'nav-item' + (key === active ? ' on' : ''), 'data-tip': t(v.label), 'aria-label': t(v.label) }, icon(v.icon, 18), h('span', null, t(v.label)));
  for (const [key, v] of Object.entries(VIEWS)) if (key !== 'settings') nav.append(item(key, v));
  const s = document.getElementById('nav-settings');
  clear(s);
  s.append(item('settings', VIEWS.settings));
}

// ---- collapsed sidebar ----------------------------------------------------

// Collapsed, the sidebar is a rail of icons; an item's name shows beside it
// on hover. The width animates only when toggled, not when the app opens.
const root = document.documentElement;
let navTimer = 0;
function collapsed() { return root.classList.contains('nav-collapsed'); }
function labelCollapse() {
  const b = document.getElementById('collapse'), label = collapsed() ? t('Expand sidebar') : t('Collapse sidebar');
  b.dataset.tip = label;
  b.setAttribute('aria-label', label);
  b.title = collapsed() ? '' : label;
}
function toggleNav() {
  root.classList.add('nav-anim');
  root.classList.toggle('nav-collapsed');
  clearTimeout(navTimer);
  navTimer = setTimeout(() => root.classList.remove('nav-anim'), 400);
  try { localStorage.setItem('ts.nav', collapsed() ? '1' : ''); } catch (_) { /* ignore */ }
  labelCollapse();
  hideRailTip();
}

let railTip = null;
let railQuiet = null; // clicked: no name for it until the mouse moves on (its menu is open)
function hideRailTip() {
  if (!railTip) return;
  railTip.remove();
  railTip = null;
}
function railTips(side) {
  side.addEventListener('mouseover', (e) => {
    const el = collapsed() && e.target.closest('[data-tip]');
    if (el !== railQuiet) railQuiet = null;
    if (!el || el === railQuiet) return hideRailTip();
    if (railTip && railTip._for === el) return;
    hideRailTip();
    const r = el.getBoundingClientRect();
    railTip = h('div', { class: 'rail-tip' }, el.dataset.tip);
    railTip._for = el;
    railTip.style.left = side.getBoundingClientRect().right + 8 + 'px';
    railTip.style.top = r.top + r.height / 2 + 'px';
    document.body.append(railTip);
  });
  side.addEventListener('mouseleave', () => { railQuiet = null; hideRailTip(); });
  side.addEventListener('mousedown', (e) => { railQuiet = e.target.closest('[data-tip]'); hideRailTip(); });
}

function show(route) {
  lastRoute = route;
  const v = VIEWS[route.view] ? route.view : 'day';
  const key = v + '/' + route.arg + '?' + JSON.stringify(route.params);
  if (key === currentKey) return;
  currentKey = key;
  if (current && current.destroy) current.destroy();
  clear(viewEl);
  viewEl.scrollTop = 0;
  renderNav(v);
  const params = { ...route.params, arg: route.arg, date: route.arg };
  const host = h('div');
  viewEl.append(host);
  current = VIEWS[v].mod.mount(host, params) || {};
  document.title = 'Timestamps · ' + t(VIEWS[v].label);
  if (route.params.away) setTimeout(() => promptAway(), 300);
}

// ---- live status card ----------------------------------------------------

// What the live card and the timers last showed: they animate in only when
// that changes, not on every refresh.
let liveShown = '', timerShown = '';

function renderLive() {
  const l = state.live;
  clear(liveEl);
  if (!l) return;
  const shown = l.state + '/' + (l.state === 'active' ? l.appId : '');
  const enter = liveShown !== '' && shown !== liveShown ? ' enter' : '';
  liveShown = shown;
  const labels = { active: t('Tracking'), away: t('Away'), paused: t('Paused'), schedule: t('Outside schedule'), off: t('Off|state') };
  const markOff = { label: t('Mark as off'), icon: 'power', onClick: () => api.post('off').then(refreshLive) };
  const resume = { label: t('Resume tracking'), icon: 'play', onClick: () => api.post('resume').then(refreshLive) };
  const pauses = [
    { label: t('Pause for 15 minutes'), icon: 'pause', onClick: () => api.post('pause', { minutes: 15 }).then(refreshLive) },
    { label: t('Pause for 1 hour'), icon: 'pause', onClick: () => api.post('pause', { minutes: 60 }).then(refreshLive) },
    { label: t('Pause until I resume'), icon: 'pause', onClick: () => api.post('pause', { minutes: 0 }).then(refreshLive) },
  ];
  const head = h('div', { class: 'live-head' }, h('span', { class: 'live-dot ' + l.state }), h('span', { class: 'live-state' }, labels[l.state] || l.state),
    h('div', { class: 'spacer' }),
    l.state === 'active' ? button('', { icon: 'power', small: true, variant: 'ghost', title: markOff.label, onClick: markOff.onClick }) : null,
    button('', { icon: l.state === 'paused' ? 'play' : 'pause', small: true, variant: 'ghost', title: l.state === 'paused' ? resume.label : t('Pause tracking'), onClick: (e) => {
      if (l.state === 'paused') resume.onClick();
      else dropdown(e.currentTarget, pauses);
    } }));
  liveEl.append(head);
  const app = l.state === 'active' && l.app ? { id: l.appId, icon: !!l.iconV, iconV: l.iconV, name: l.app } : null;
  let detail = '';
  if (app) {
    liveEl.append(h('div', { class: 'live-app' + enter }, appIcon(app, 20),
      h('div', { class: 'live-text' }, h('div', { class: 'live-name' }, l.app), h('div', { class: 'live-title', title: l.title }, l.doc || l.title || ''))));
    detail = l.app;
  } else if (l.state === 'paused' && l.pausedTill) {
    detail = t('Resumes at {0}', clock(l.pausedTill));
    liveEl.append(h('div', { class: 'live-title' + enter }, detail));
  } else if ((l.state === 'away' || l.state === 'off') && l.since) {
    detail = t('Since {0}', clock(l.since));
    liveEl.append(h('div', { class: 'live-title' + enter }, detail));
  }

  // Collapsed, the card is its dot on the app in use; its actions are in its
  // menu.
  clear(liveRail);
  liveRail.className = 'rail-live' + (app ? '' : ' bare');
  liveRail.append(...(app ? [appIcon(app, 20)] : []), h('span', { class: 'live-dot ' + l.state }));
  const tip = [labels[l.state] || l.state, detail].filter(Boolean).join(' · ');
  liveRail.dataset.tip = tip;
  liveRail.setAttribute('aria-label', tip);
  if (railTip && railTip._for === liveRail) railTip.textContent = tip;
  liveRail.onclick = () => dropdown(liveRail, l.state === 'paused' ? [resume] : l.state === 'active' ? [markOff, { separator: true }, ...pauses] : pauses, 'right');
}

// ---- stopwatch & countdown ------------------------------------------------

function renderTimers() {
  const tm = state.timers;
  clear(timerEl);
  if (!tm) return;
  const now = Math.floor(Date.now() / 1000);
  const offset = (tm.now || now) - now;
  const sw = tm.stopwatch, cd = tm.countdown;
  const shown = sw ? 'sw' : cd ? 'cd' : '';
  timerEl.classList.toggle('enter', shown !== '' && shown !== timerShown);
  timerShown = shown;
  // A running timer is a card; collapsed, it is a small clock whose actions
  // are in its menu.
  const rail = (cls, ico, tip, items) => {
    const time = h('span', { class: 'rail-clock-time' });
    const b = h('button', { type: 'button', class: 'rail-clock' + cls, 'data-tip': tip, 'aria-label': tip, onClick: () => dropdown(b, items, 'right') }, icon(ico, 16), time);
    timerEl.append(b);
    return { b, time };
  };
  const short = (s) => dur(Math.max(0, s), 'clock').replace(/^0:/, '');
  if (sw) {
    const stop = async () => { await api.post('stopwatch/stop'); await refreshLive(); loadTags(); toast(t('Stopwatch saved as a tag'), { type: 'ok' }); };
    const discard = async () => { await api.post('stopwatch/stop', { discard: true }); refreshLive(); };
    const el = h('span', { class: 'timer-clock' }, dur(now + offset - sw.start, 'clock'));
    timerEl.append(h('div', { class: 'timer running' },
      h('div', { class: 'timer-row' }, icon('timer', 16), el, h('div', { class: 'spacer' }),
        button('', { icon: 'stop', small: true, variant: 'primary', title: t('Stop and save'), onClick: stop }),
        button('', { icon: 'x', small: true, variant: 'ghost', title: t('Discard'), onClick: discard })),
      h('button', { class: 'timer-tags', type: 'button', onClick: editStopwatch }, sw.tags || t('Add tags…'))));
    const r = rail('', 'timer', sw.tags || t('Stopwatch'), [
      { label: t('Stop and save'), icon: 'stop', onClick: stop },
      { label: sw.tags || t('Add tags…'), icon: 'tag', onClick: editStopwatch },
      { separator: true },
      { label: t('Discard'), icon: 'x', danger: true, onClick: discard },
    ]);
    timerEl._tick = () => {
      const s = Math.floor(Date.now() / 1000) + offset - sw.start;
      el.textContent = dur(s, 'clock');
      r.time.textContent = short(s);
    };
    timerEl._tick();
  } else if (cd) {
    const cancel = async () => { await api.post('countdown/cancel'); refreshLive(); };
    const el = h('span', { class: 'timer-clock' });
    const total = cd.end - cd.start;
    const bar = h('div', { class: 'timer-progress' }, h('div'));
    timerEl.append(h('div', { class: 'timer running cd' },
      h('div', { class: 'timer-row' }, icon('hourglass', 16), el, h('div', { class: 'spacer' }),
        button('', { icon: 'x', small: true, variant: 'ghost', title: t('Cancel countdown'), onClick: cancel })),
      cd.label || cd.tags ? h('div', { class: 'timer-tags' }, cd.label || cd.tags) : null, bar));
    const r = rail(' cd', 'hourglass', cd.label || cd.tags || t('Countdown'), [{ label: t('Cancel countdown'), icon: 'x', danger: true, onClick: cancel }]);
    timerEl._tick = () => {
      const nowS = Math.floor(Date.now() / 1000) + offset, p = Math.min(1, (nowS - cd.start) / total);
      el.textContent = dur(Math.max(0, cd.end - nowS), 'clock');
      r.time.textContent = short(cd.end - nowS);
      bar.firstChild.style.width = p * 100 + '%';
      r.b.style.setProperty('--p', p);
    };
    timerEl._tick();
  } else {
    timerEl._tick = null;
    const item = (ico, label, onClick) => h('button', { type: 'button', class: 'nav-item', 'data-tip': label, 'aria-label': label, onClick }, icon(ico, 18), h('span', null, label));
    timerEl.append(h('div', { class: 'timer-buttons' }, item('timer', t('Stopwatch'), startStopwatch), item('hourglass', t('Countdown'), startCountdown)));
  }
}
setInterval(() => timerEl._tick && timerEl._tick(), 1000);

function startStopwatch() {
  const ti = tagInput({ names: state.tagNames, combos: state.tagCombos, autofocus: true, onEnter: () => start() });
  const notes = h('input', { class: 'input', placeholder: t('Notes (optional)') });
  const start = async () => { await api.post('stopwatch/start', { tags: ti.value, notes: notes.value }); m.close(); refreshLive(); };
  const m = modal({
    title: t('Start stopwatch'), width: 460,
    body: h('div', { class: 'form' }, h('label', { class: 'field' }, h('span', null, t('What are you working on?')), ti), h('label', { class: 'field' }, h('span', null, t('Notes')), notes),
      h('p', { class: 'hint' }, t('When you stop it, the elapsed time is saved as a tag.'))),
    actions: [{ label: t('Cancel') }, { label: t('Start'), primary: true, icon: 'play', onClick: start }],
  });
}

function editStopwatch() {
  const sw = state.timers.stopwatch;
  const ti = tagInput({ value: sw.tags, names: state.tagNames, combos: state.tagCombos, autofocus: true });
  const notes = h('input', { class: 'input', value: sw.notes || '', placeholder: t('Notes (optional)') });
  modal({
    title: t('Stopwatch'), width: 460,
    body: h('div', { class: 'form' }, h('label', { class: 'field' }, h('span', null, t('Tags')), ti), h('label', { class: 'field' }, h('span', null, t('Notes')), notes)),
    actions: [{ label: t('Cancel') }, { label: t('Save'), primary: true, onClick: async (close) => { await api.post('stopwatch/update', { tags: ti.value, notes: notes.value }); close(); refreshLive(); } }],
  });
}

function startCountdown() {
  let mins = 25;
  const custom = h('input', { class: 'input num', type: 'number', min: 1, max: 600, value: 25 });
  custom.addEventListener('input', () => (mins = +custom.value || 25));
  const presets = h('div', { class: 'chips' }, [5, 10, 15, 25, 45, 60, 90].map((m) => h('button', { class: 'chip', type: 'button', onClick: () => { mins = m; custom.value = m; } }, m + ' min')));
  const label = h('input', { class: 'input', placeholder: t('Label (optional)') });
  const ti = tagInput({ names: state.tagNames, combos: state.tagCombos, placeholder: t('Tag the time when it ends (optional)') });
  modal({
    title: t('Start countdown'), width: 460,
    body: h('div', { class: 'form' }, presets, h('label', { class: 'field' }, h('span', null, t('Minutes')), custom), h('label', { class: 'field' }, h('span', null, t('Label')), label), h('label', { class: 'field' }, h('span', null, t('Tags')), ti)),
    actions: [{ label: t('Cancel') }, { label: t('Start'), primary: true, icon: 'play', onClick: async (close) => { await api.post('countdown/start', { secs: Math.round(mins * 60), tags: ti.value, label: label.value }); close(); refreshLive(); } }],
  });
}

// ---- away prompt ----------------------------------------------------------

let awayOpen = false;
async function promptAway(specific) {
  if (awayOpen) return;
  await refreshLive();
  const list = state.aways || [];
  const a = specific || list[list.length - 1];
  if (!a) return;
  awayOpen = true;
  const ti = tagInput({ names: state.tagNames, combos: state.tagCombos, autofocus: true, onEnter: () => save() });
  const notes = h('input', { class: 'input', placeholder: t('Notes (optional)') });
  let billable = false;
  const recents = h('div', { class: 'chips' }, state.recentTags.slice(0, 8).map((r) => h('button', { class: 'chip', type: 'button', onClick: () => { ti.value = r; ti.input.focus(); } }, r)));
  const quick = h('div', { class: 'chips' }, [t('Meeting'), t('Break'), t('Lunch'), t('Phone call')].map((r) => h('button', { class: 'chip ghost', type: 'button', onClick: () => { ti.value = r; ti.input.focus(); } }, r)));
  let saved = false;
  const save = async () => {
    if (!ti.value.trim()) { ti.input.focus(); return; }
    await api.post('aways/resolve', { s: a.s, e: a.e, tags: ti.value, notes: notes.value, billable });
    saved = true;
    loadTags();
    m.close();
  };
  const m = modal({
    title: t('Welcome back'), width: 500,
    body: h('div', { class: 'form' },
      h('div', { class: 'away-range' }, icon('clock', 18), h('div', null, h('b', null, `${clock(a.s)} – ${clock(a.e)}`), h('span', { class: 'muted' }, t('You were away for {0}', dur(a.e - a.s))))),
      h('label', { class: 'field' }, h('span', null, t('What were you doing?')), ti),
      state.recentTags.length ? recents : null, quick,
      h('label', { class: 'field' }, h('span', null, t('Notes')), notes),
      toggle(false, (v) => (billable = v), t('Billable'))),
    actions: [{ label: t('Ignore'), onClick: async (close) => { await api.post('aways/resolve', { s: a.s, e: a.e }); close(); } }, { label: t('Save tag'), primary: true, icon: 'tag', onClick: save }],
    onClose: () => { awayOpen = false; refreshLive(); if (saved && current && current.refresh) current.refresh({ type: 'tags' }); },
  });
}
window.__showAway = promptAway;

// ---- boot ---------------------------------------------------------------

async function boot() {
  const settings = await api.get('settings');
  state.settings = settings;
  setLang(resolveLang(settings));
  configure(settings);
  applyTheme();
  document.getElementById('brand-name').textContent = 'Timestamps';
  drawLogo();
  await Promise.all([refreshLive(), loadTags()]);
  renderLive();
  renderTimers();
  onChange((what) => {
    if (what === 'live') { renderLive(); renderTimers(); drawLogo(); }
    if (what === 'settings') { configure(state.settings); applyTheme(); }
  });
  onRoute(show);
  if (!location.hash || location.hash === '#/' || location.hash === '#') go('day/' + today(), true);
  show(parse());

  let liveTm = 0;
  api.events((ev) => {
    if (ev.type === 'away' && state.settings.awayPrompt) setTimeout(() => promptAway(), 400);
    if (['seg', 'state', 'timers', 'away'].includes(ev.type)) {
      clearTimeout(liveTm);
      liveTm = setTimeout(refreshLive, 250);
    }
    if (ev.type === 'tags') loadTags();
    current && current.refresh && current.refresh(ev);
  });
  setInterval(refreshLive, 20000);

  document.addEventListener('keydown', (e) => {
    if (mod(e) && e.key.toLowerCase() === 'k') { e.preventDefault(); go('search'); }
    if (e.key === 'F5' && parse().view !== 'day') e.preventDefault();
  });
  document.getElementById('collapse').onclick = toggleNav;
  labelCollapse();
  railTips(document.querySelector('.sidebar'));
  window.__navigate = (route) => go(route.replace(/^\/+/, ''));
  document.body.classList.add('ready');
}

boot().catch((err) => {
  viewEl.append(h('div', { class: 'boot-error' }, icon('alert', 20), h('div', null, h('b', null, 'Timestamps could not start'), h('div', { class: 'muted' }, err.message))));
});
