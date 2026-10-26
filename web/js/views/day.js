// Day view: timeline lanes, live updates, tagging and summaries.
import { h, clear, icon, button, appIcon, kindIcon, siteHost, wheelScrollsX, menu, showTip, hideTip, pickDate, toast, confirmDialog, segmented, empty } from '../ui.js';
import { t } from '../i18n.js';
import { dur, durLong, clock, fmtDateLong, today, addDays, hex, hashColor, mutedColor, escapeHTML, shortURL, baseName, dirName, pct, pageTitle, stripBrowser, baseDomain } from '../fmt.js';
import * as api from '../api.js';
import { Timeline } from '../timeline.js';
import { ActivityList } from '../activitylist.js';
import { Board } from '../grid.js';
import { state, loadTags } from '../state.js';
import { debounce } from '../ui.js';
import { openTagDialog } from '../tagdialog.js';
import { openLightbox } from './shots.js';
import { go } from '../router.js';
import { mod, keys, byOS } from '../platform.js';

let clipboardTag = null;

const BROWSERS = new Set(['chrome.exe', 'msedge.exe', 'brave.exe', 'opera.exe', 'vivaldi.exe', 'chromium.exe', 'browser.exe', '360se.exe',
  '360chrome.exe', 'qqbrowser.exe', 'sogouexplorer.exe', '2345explorer.exe', 'liebao.exe', 'centbrowser.exe', 'arc.exe', 'thorium.exe',
  'iexplore.exe', 'firefox.exe', 'waterfox.exe', 'librewolf.exe', 'floorp.exe', 'zen.exe',
  // macOS apps are named after their bundle
  'safari', 'safari technology preview', 'google chrome', 'google chrome beta', 'google chrome dev', 'google chrome canary', 'microsoft edge',
  'microsoft edge beta', 'microsoft edge dev', 'microsoft edge canary', 'brave browser', 'brave browser beta', 'brave browser nightly', 'vivaldi',
  'chromium', 'arc', 'opera', 'opera gx', 'thorium', 'orion', 'firefox', 'firefox developer edition', 'firefox nightly', 'waterfox', 'librewolf',
  'floorp', 'zen', 'zen browser']);

function cssVar(n) { return getComputedStyle(document.documentElement).getPropertyValue(n).trim(); }

// Text colour that stays readable on a segment colour.
const inkCache = new Map();
function inkFor(color) {
  let v = inkCache.get(color);
  if (v) return v;
  const c = document.createElement('canvas').getContext('2d');
  c.fillStyle = color;
  const m = /^#([0-9a-f]{6})$/i.exec(c.fillStyle);
  let lum = 0.4;
  if (m) {
    const n = parseInt(m[1], 16);
    const r = (n >> 16) / 255, g = ((n >> 8) & 255) / 255, b = (n & 255) / 255;
    lum = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  }
  v = lum > 0.62 ? '#1a1d26' : '#ffffff';
  inkCache.set(color, v);
  return v;
}

export function mount(el, params) {
  const date = params.arg || today();
  const isToday = () => date === today();
  el.className = 'view day-view';

  let data = null, lanes = [], liveNow = 0, sel = null, summaryTab = 'apps', docTab = 'sites';
  const expanded = new Set();

  // ---- header ----
  const dateBtn = h('button', { class: 'btn date-btn', type: 'button', title: t('Pick a date') }, icon('day', 16), h('span', null, fmtDateLong(date)));
  dateBtn.onclick = async () => { const d = await pickDate(dateBtn, date); if (d) go('day/' + d); };
  const kpis = h('div', { class: 'kpis' });
  const resetBtn = button('', { icon: 'layers', variant: 'ghost', title: t('Reset layout'), onClick: () => board.reset() });
  const pinBtn = button('', { icon: 'pin', variant: 'ghost', onClick: () => setPinned(!(state.settings && state.settings.dayGridPinned)) });
  // A pinned layout can't be dragged, resized or reset.
  const showPinned = (on) => {
    pinBtn.classList.toggle('on', on);
    pinBtn.title = on ? t('Unpin layout') : t('Pin layout');
    resetBtn.disabled = on;
  };
  const setPinned = (on) => {
    if (!state.settings) return;
    state.settings.dayGridPinned = on;
    board.setLocked(on);
    showPinned(on);
    api.post('settings', state.settings).catch((err) => toast(err.message, { type: 'error' }));
  };
  showPinned(!!(state.settings && state.settings.dayGridPinned));
  const header = h('header', { class: 'toolbar' },
    h('div', { class: 'date-nav' },
      button('', { icon: 'left', title: t('Previous day') + ' (F7)', onClick: () => go('day/' + addDays(date, -1)) }),
      dateBtn,
      button('', { icon: 'right', title: t('Next day') + ' (F8)', onClick: () => go('day/' + addDays(date, 1)), disabled: date >= today() }),
      date !== today() ? button(t('Today'), { variant: 'ghost', onClick: () => go('day/' + today()) }) : null),
    kpis,
    h('div', { class: 'spacer' }),
    h('div', { class: 'btn-group' },
      button('', { icon: 'zoomOut', title: t('Zoom out') + ' (-)', onClick: () => tl.zoomBy(1.6) }),
      button('', { icon: 'fit', title: t('Fit active hours') + ' (0)', onClick: () => tl.fit() }),
      button('24h', { title: t('Whole day') + ' (Shift+0)', onClick: () => tl.fit(!(data && tl.view[0] <= data.from && tl.view[1] >= data.to)), class: 'btn-text' }),
      button('', { icon: 'zoomIn', title: t('Zoom in') + ' (+)', onClick: () => tl.zoomBy(1 / 1.6) })),
    pinBtn,
    resetBtn);

  // ---- timeline ----
  const tlHost = h('div', { class: 'tl-host' });
  const selBar = h('div', { class: 'sel-bar', hidden: true });
  const banner = h('div', { class: 'banner', hidden: true });
  const tlCard = h('section', { class: 'card tl-card' }, banner, tlHost, selBar);

  // ---- summaries ----
  const appsCard = h('section', { class: 'card list-card' });
  const docsCard = h('section', { class: 'card list-card' });
  const tagsCard = h('section', { class: 'card list-card' });
  const shotsCard = h('section', { class: 'card shots-card' });
  const listHost = h('div', { class: 'blk-fill' });
  const boardEl = h('div');
  el.append(header, boardEl);
  const saveLayout = debounce((layout) => {
    if (!state.settings) return;
    state.settings.dayGrid = layout;
    api.post('settings', state.settings).catch((err) => toast(err.message, { type: 'error' }));
  }, 400);
  // Full-width rows of 12 columns; h is the default row height in grid
  // units, minW and minH the smallest size. The timeline's lanes fill its
  // height.
  const board = new Board(boardEl, [
    { id: 'timeline', el: tlCard, row: 0, w: 12, h: 10, minW: 6, minH: 7 },
    { id: 'activities', el: listHost, row: 1, w: 8, h: 13, minW: 5 },
    { id: 'docs', el: docsCard, row: 1, w: 4, h: 13 },
    { id: 'apps', el: appsCard, row: 2, w: 6, h: 10 },
    { id: 'tags', el: tagsCard, row: 2, w: 6, h: 10 },
    { id: 'shots', el: shotsCard, row: 3, w: 12, h: 5, minH: 4 },
  ], (state.settings && state.settings.dayGrid) || [], saveLayout);
  board.setHidden('shots', true);
  board.setLocked(state.settings && state.settings.dayGridPinned);

  const tl = new Timeline(tlHost, {
    fill: true,
    onSelect: (s) => { sel = s; renderSelBar(); renderSummaries(); list.setSelection(s); },
    onSelecting: (s) => { sel = s; renderSelBar(); },
    onHover: (hit, e) => {
      list.hoverSeg(hit && hit.seg);
      if (!hit || !hit.seg || !e) { hideTip(); return; }
      showTip(e.clientX, e.clientY, tooltip(hit.seg));
    },
    onView: (v) => scheduleListRange(v),
    onDraw: () => positionSelBar(),
    onDblClick: (hit) => {
      if (!hit || !hit.seg) return;
      if (hit.seg.kind === 'tag') editTag(hit.seg.tag);
      else if (hit.seg.kind === 'shot') openShot(hit.seg.ts);
      else tagRange(hit.seg.s, hit.seg.e);
    },
    onClickSeg: (hit) => {
      if (!hit || !hit.seg) return;
      if (hit.seg.kind === 'shot') { openShot(hit.seg.ts); return; }
      const tab = { app: 'apps', doc: 'docs', tag: 'tags', usage: 'usage' }[hit.seg.kind];
      if (tab) { list.showTab(tab); list.pick(hit.seg); }
    },
    onContext: (hit, e) => contextMenu(hit, e),
  });

  const list = new ActivityList(listHost, {
    onPick: (seg) => { tl.setSelection({ s: seg.s, e: seg.e }); tl.reveal(seg.s, seg.e); },
    onHover: (r) => tl.setMark(r && r.segs ? { s: r.s, e: r.e, laneId: list.tab } : r),
    onOpen: (r) => (r.segs ? tagMany(r.segs) : r.kind === 'tag' ? editTag(r.tag) : tagRange(r.s, r.e)),
    onContext: (seg, e) => contextMenu({ seg, t: null, lane: null }, e),
    onTag: (segs) => tagMany(segs),
    onDeleteTags: async (segs) => {
      if (!(await confirmDialog(t('Delete {0} tags?', segs.length), { danger: true, ok: t('Delete') }))) return;
      await api.post('tags/delete', { ids: segs.map((x) => x.tag.id) });
      load();
    },
    onFilter: (q) => applyFilter(q),
  });

  let listRaf = 0;
  function scheduleListRange(v) {
    cancelAnimationFrame(listRaf);
    listRaf = requestAnimationFrame(() => list.setRange(v[0], v[1]));
  }

  function openShot(ts) {
    const list = data.shots;
    openLightbox(list, Math.max(0, list.indexOf(ts)));
  }

  function tooltip(sg) {
    const time = `<div class="tip-time">${clock(sg.s, true)} – ${clock(sg.e, true)} · ${durLong(sg.e - sg.s)}</div>`;
    switch (sg.kind) {
      case 'app':
        return `<div class="tip-title">${escapeHTML(sg.app.name)}</div>${sg.title ? `<div class="tip-sub">${escapeHTML(sg.title)}</div>` : ''}${time}`;
      case 'doc': {
        const d = sg.doc;
        const sub = d.kind === 2 ? escapeHTML(d.key) : escapeHTML(d.key);
        return `<div class="tip-title">${escapeHTML(d.kind === 2 ? sg.page || d.grp : baseName(d.key))}</div><div class="tip-sub mono">${sub}</div>${time}`;
      }
      case 'tag':
        return `<div class="tip-title">${escapeHTML(sg.tag.tags)}${sg.tag.billable ? ' <span class="pill">$</span>' : ''}</div>${sg.tag.notes ? `<div class="tip-sub">${escapeHTML(sg.tag.notes)}</div>` : ''}${time}`;
      case 'auto':
        return `<div class="tip-title">${escapeHTML(sg.label)}</div><div class="tip-sub">${t('Suggested by autotag rules')}</div>${time}`;
      case 'usage':
        return `<div class="tip-title">${sg.state ? t('Active') : t('Away')}</div>${time}`;
      case 'shot':
        return `<img class="tip-shot" src="${api.shotURL(sg.ts, true)}" alt=""><div class="tip-time">${clock(sg.ts, true)}</div>`;
    }
    return time;
  }

  // ---- data ----
  function buildLanes() {
    const d = data;
    const C = { active: cssVar('--active'), away: cssVar('--away') };
    const tagSegs = d.tags.map((tg) => {
      const c = d.tagMap[tg.tags] && d.tagMap[tg.tags].color ? hex(d.tagMap[tg.tags].color) : hashColor(tg.tags);
      return { s: tg.s, e: tg.e, color: c, ink: inkFor(c), label: tg.tags, group: 't' + tg.id, search: tg.tags + ' ' + tg.notes, kind: 'tag', tag: tg };
    });
    const usage = d.usage.map(([s, e, st]) => ({ s, e, state: st, color: st ? C.active : C.away, kind: 'usage', group: 'u' + st, label: st ? t('Active') : t('Away') }));
    const apps = d.apps.map(([s, e, a, ti]) => {
      const app = d.appMap[a] || { id: a, name: '?', color: '888888' };
      const title = d.titleMap[ti] || '';
      const c = hex(app.color);
      return { s, e, color: c, ink: inkFor(c), label: app.name, group: 'a' + a, icon: app.icon ? api.iconURL(a, app.iconV) : null, search: app.name + ' ' + title, kind: 'app', app, title };
    });
    const pages = pageTitles(apps);
    const mergeSub = !state.settings || state.settings.mergeSubdomains !== false;
    let ai = 0;
    const docs = d.docs.map(([s, e, id, a]) => {
      const doc = d.docMap[id] || { kind: 1, key: '?', name: '?', grp: '?' };
      // The page title is the browser window title at the time (imported
      // ManicTime data has URLs only).
      let page = '';
      if (doc.kind === 2) {
        const mid = (s + e) / 2;
        while (ai < apps.length && apps[ai].e <= mid) ai++;
        const as = apps[ai];
        if (as && as.s <= mid) page = pages(as);
        if (!page && doc.name && doc.name !== doc.key) page = doc.name;
      }
      const c = hashColor(doc.grp || doc.key);
      const site = doc.kind === 2 ? (mergeSub ? baseDomain(doc.grp) : doc.grp) : '';
      const sc = doc.kind === 2 ? siteColor(site) : c;
      const host = doc.kind === 2 ? siteHost(site) : '';
      return { s, e, color: sc, ink: inkFor(sc), label: doc.kind === 2 ? site : doc.name, group: doc.kind === 2 ? 'w' + site : 'f' + doc.key, icon: host ? api.siteIconURL(host) : null, search: doc.key + ' ' + doc.name + ' ' + page, kind: 'doc', doc, page, site, appId: a };
    });
    lanes = [
      { id: 'tags', label: t('Tags'), icon: 'tag', h: 50, segs: tagSegs, style: 'tags' },
    ];
    if (d.auto && d.auto.length) {
      lanes.push({ id: 'auto', label: t('Autotags'), icon: 'sparkles', h: 36, style: 'auto', segs: d.auto.map((a) => { const c = hashColor(a.tags); return { s: a.s, e: a.e, color: c, ink: inkFor(c), label: a.tags, group: 'x' + a.tags, kind: 'auto', auto: a }; }) });
    }
    lanes.push(
      { id: 'usage', label: t('Computer'), icon: 'monitor', h: 34, segs: usage, style: 'usage' },
      { id: 'apps', label: t('Applications'), icon: 'app', h: 62, segs: apps, style: 'bar' },
      { id: 'docs', label: t('Documents'), icon: 'file', h: 50, segs: docs, style: 'bar' });
    if (d.shots.length) lanes.push({ id: 'shots', label: t('Screenshots'), icon: 'image', h: 84, style: 'shots', segs: d.shots.map((ts) => ({ s: ts, e: ts + 1, kind: 'shot', ts, thumb: api.shotURL(ts, true) })) });
  }

  // A website's color comes from its icon; sites without one get a quiet gray.
  function siteColor(site) {
    const c = data && data.siteColors && data.siteColors[site];
    return c ? hex(c) : mutedColor(site);
  }

  // Returns seg -> cleaned page title for browser windows. A trailing
  // " - <profile>" shared by most titles of a browser is dropped too.
  function pageTitles(apps) {
    const byApp = new Map();
    for (const sg of apps) {
      if (!BROWSERS.has(sg.app.key) || !sg.title) continue;
      let m = byApp.get(sg.app.id);
      if (!m) byApp.set(sg.app.id, (m = { n: 0, tails: new Map() }));
      const p = stripBrowser(sg.title);
      const i = p.lastIndexOf(' - ');
      m.n++;
      if (i > 0) m.tails.set(p.slice(i), (m.tails.get(p.slice(i)) || 0) + 1);
    }
    const profile = new Map();
    for (const [id, m] of byApp) {
      for (const [tail, n] of m.tails) if (m.n >= 4 && n / m.n >= 0.5) profile.set(id, tail);
    }
    const cache = new Map();
    return (sg) => {
      if (!BROWSERS.has(sg.app.key) || !sg.title) return '';
      let v = cache.get(sg.title);
      if (v === undefined) {
        v = pageTitle(sg.title);
        const tail = profile.get(sg.app.id);
        if (tail && v.endsWith(tail) && v.length > tail.length) v = v.slice(0, -tail.length);
        cache.set(sg.title, v);
      }
      return v;
    };
  }

  // Extend the open segments to "now" between refreshes so the view is live.
  function applyLive() {
    if (!data || !data.live || !isToday()) return;
    const now = Math.floor(Date.now() / 1000);
    const prev = liveNow || data.live.now;
    liveNow = now;
    if (data.live.state === 'active') {
      for (const id of ['usage', 'apps', 'docs']) {
        const l = lanes.find((x) => x.id === id);
        if (!l || !l.segs.length) continue;
        const last = l.segs[l.segs.length - 1];
        if (last.e >= prev - 15 && (id !== 'docs' || data.live.docSeg)) last.e = now;
      }
    }
    tl.setNow(now);
  }

  let filterInited = false;
  async function load(keepView = true) {
    try {
      data = await api.day(date);
    } catch (err) {
      toast(err.message, { type: 'error' });
      return;
    }
    liveNow = 0;
    buildLanes();
    tl.setData({ from: data.from, to: data.to, lanes, now: isToday() ? Math.floor(Date.now() / 1000) : 0 }, keepView);
    applyLive();
    list.setLanes(lanes, data.from, data.to);
    list.setRange(tl.view[0], tl.view[1]);
    if (params.filter && !filterInited) { filterInited = true; list.setFilter(params.filter); }
    renderKpis();
    renderSummaries();
    renderShots();
    renderBanner();
  }

  // ---- header pieces ----
  function totals(rs, re) {
    let active = 0, away = 0, first = 0, last = 0;
    for (const [s, e, st] of data.usage) {
      const a = Math.max(s, rs), b = Math.min(e, re);
      if (b <= a) continue;
      if (st) { active += b - a; if (!first || a < first) first = a; if (b > last) last = b; } else away += b - a;
    }
    let tagged = 0;
    for (const tg of data.tags) tagged += Math.max(0, Math.min(tg.e, re) - Math.max(tg.s, rs));
    return { active, away, first, last, tagged };
  }

  function renderKpis() {
    clear(kpis);
    const tot = totals(data.from, liveNow || data.to);
    const chip = (cls, label, value, title) => h('div', { class: 'kpi ' + cls, title }, h('span', { class: 'kpi-label' }, label), h('span', { class: 'kpi-value' }, value));
    kpis.append(
      chip('k-active', t('Active'), dur(tot.active)),
      chip('k-away', t('Away'), dur(tot.away)),
      tot.first ? chip('k-span', t('Span'), `${clock(tot.first)} – ${tot.last >= data.to ? '24:00' : clock(tot.last)}`, t('First and last activity')) : null,
      chip('k-tag', t('Tagged'), dur(tot.tagged)));
  }

  function renderBanner() {
    const aways = (data && data.aways) || [];
    if (!aways.length || !isToday()) { banner.hidden = true; return; }
    const a = aways[aways.length - 1];
    clear(banner);
    banner.hidden = false;
    banner.append(icon('clock', 16),
      h('span', null, t('You were away {0} – {1} ({2}).', clock(a.s), clock(a.e), dur(a.e - a.s))),
      button(t('Tag it'), { small: true, variant: 'primary', onClick: () => window.__showAway && window.__showAway(a) }),
      button(t('Dismiss'), { small: true, variant: 'ghost', onClick: async () => { await api.post('aways/resolve', { s: a.s, e: a.e }); data.aways = aways.filter((x) => x !== a); renderBanner(); } }));
  }

  function renderSelBar() {
    if (!sel) { selBar.hidden = true; return; }
    selBar.hidden = false;
    clear(selBar);
    selBar.append(
      h('span', { class: 'sel-range' }, `${clock(sel.s, true)} – ${clock(sel.e, true)}`),
      h('span', { class: 'sel-dur' }, durLong(sel.e - sel.s)),
      button(t('Tag'), { icon: 'tag', small: true, variant: 'primary', title: 'E', onClick: () => tagRange(sel.s, sel.e) }),
      button('', { icon: 'trash', small: true, variant: 'ghost', title: t('Clear tags in selection') + ' (Del)', onClick: clearTags }),
      button('', { icon: 'zoomIn', small: true, variant: 'ghost', title: t('Zoom to selection'), onClick: () => tl.zoomTo(sel.s, sel.e) }),
      button('', { icon: 'x', small: true, variant: 'ghost', title: t('Clear selection') + ' (Esc)', onClick: () => tl.setSelection(null) }));
    positionSelBar();
  }

  // Keeps the selection bar above the selected range while the timeline is
  // panned or zoomed; hidden when the range is out of view.
  function positionSelBar() {
    if (!sel || selBar.hidden) return;
    const L = tl.labelW, R = tl.labelW + tl.plotW;
    const x0 = Math.max(L, tl.x(sel.s)), x1 = Math.min(R, tl.x(sel.e));
    if (x1 < L || x0 > R || x1 < x0) { selBar.style.visibility = 'hidden'; return; }
    selBar.style.visibility = '';
    const w = selBar.offsetWidth || 360;
    const x = tlHost.offsetLeft + (x0 + x1) / 2;
    selBar.style.left = Math.max(8, Math.min(tlCard.clientWidth - w - 8, x - w / 2)) + 'px';
    // Once the timeline scrolls up out of view the bar stays at the top of
    // the window, so the selection can still be tagged from further down.
    const mr = scroller ? scroller.getBoundingClientRect() : { top: 0 };
    const cr = tlCard.getBoundingClientRect();
    const pinned = cr.top < mr.top;
    selBar.classList.toggle('pinned', pinned);
    selBar.style.top = pinned ? mr.top - cr.top + 10 + 'px' : '';
  }
  const scroller = el.closest('.main');
  const onScroll = () => positionSelBar();
  scroller && scroller.addEventListener('scroll', onScroll, { passive: true });

  // ---- summaries ----
  function rangeNow() {
    if (sel) return [sel.s, sel.e];
    return [data.from, data.to];
  }

  function collect() {
    const [rs, re] = rangeNow();
    const apps = new Map(), sites = new Map(), files = new Map(), tags = new Map();
    const clipLen = (s, e) => Math.max(0, Math.min(e, re) - Math.max(s, rs));
    for (const sg of lanes.find((l) => l.id === 'apps').segs) {
      const d = clipLen(sg.s, sg.e);
      if (!d) continue;
      let a = apps.get(sg.app.id);
      if (!a) apps.set(sg.app.id, (a = { app: sg.app, secs: 0, titles: new Map() }));
      a.secs += d;
      a.titles.set(sg.title, (a.titles.get(sg.title) || 0) + d);
    }
    for (const sg of lanes.find((l) => l.id === 'docs').segs) {
      const d = clipLen(sg.s, sg.e);
      if (!d) continue;
      const doc = sg.doc;
      if (doc.kind === 2) {
        let s = sites.get(sg.site);
        if (!s) sites.set(sg.site, (s = { grp: sg.site, secs: 0, pages: new Map() }));
        s.secs += d;
        const p = s.pages.get(doc.key) || { doc, secs: 0, title: sg.page };
        p.secs += d;
        s.pages.set(doc.key, p);
      } else {
        const f = files.get(doc.key) || { doc, secs: 0 };
        f.secs += d;
        files.set(doc.key, f);
      }
    }
    for (const tg of data.tags) {
      const d = clipLen(tg.s, tg.e);
      if (d) tags.set(tg.tags, (tags.get(tg.tags) || 0) + d);
    }
    const tot = totals(rs, Math.min(re, liveNow || re));
    return { apps: [...apps.values()].sort((a, b) => b.secs - a.secs), sites: [...sites.values()].sort((a, b) => b.secs - a.secs), files: [...files.values()].sort((a, b) => b.secs - a.secs), tags: [...tags.entries()].sort((a, b) => b[1] - a[1]), tot };
  }

  function cardHead(title, ic, extra) {
    const [rs, re] = rangeNow();
    return h('div', { class: 'card-head' }, icon(ic, 16), h('h3', null, title),
      sel ? h('span', { class: 'pill accent', title: t('Showing the selected range') }, `${clock(rs)}–${clock(re)}`) : null,
      h('div', { class: 'spacer' }), extra || null);
  }

  function bar(secs, max, color) {
    return h('div', { class: 'bar' }, h('div', { class: 'bar-fill', style: { width: Math.max(1.5, (secs / max) * 100) + '%', background: color } }));
  }

  function listRows(rows, limitKey) {
    const limit = expanded.has(limitKey) ? rows.length : 40;
    const out = rows.slice(0, limit);
    if (rows.length > limit) {
      out.push(h('button', { class: 'more-btn', type: 'button', onClick: () => { expanded.add(limitKey); renderSummaries(); } }, t('Show all ({0})', rows.length)));
    }
    return out;
  }

  function hoverHighlight(fn) {
    return {
      onMouseenter: () => tl.setHighlight(fn),
      onMouseleave: () => tl.setHighlight(null),
    };
  }

  // Rebuilding a panel (e.g. expanding a row) must not jump its list back
  // to the top; switching the Websites/Files tab does start at the top.
  let shownDocTab = docTab;
  function renderSummaries() {
    if (!data) return;
    const scrollOf = (card) => { const l = card.querySelector('.list'); return l ? l.scrollTop : 0; };
    const keep = [[appsCard, scrollOf(appsCard)], [docsCard, shownDocTab === docTab ? scrollOf(docsCard) : 0], [tagsCard, scrollOf(tagsCard)]];
    shownDocTab = docTab;
    renderSummaryCards();
    for (const [card, top] of keep) { const l = card.querySelector('.list'); if (l) l.scrollTop = top; }
  }

  function renderSummaryCards() {
    const c = collect();
    // Applications
    clear(appsCard);
    const appTotal = c.apps.reduce((n, a) => n + a.secs, 0);
    appsCard.append(cardHead(t('Applications'), 'app', h('span', { class: 'muted' }, dur(appTotal))));
    const list = h('div', { class: 'list' });
    if (!c.apps.length) list.append(empty('app', t('No application activity'), sel ? t('Nothing was tracked in the selected range.') : null));
    const max = c.apps.length ? c.apps[0].secs : 1;
    list.append(...listRows(c.apps.map((a) => {
      const key = 'app:' + a.app.id;
      const open = expanded.has(key);
      const row = h('div', { class: 'row' + (open ? ' open' : ''), ...hoverHighlight((sg) => sg.kind === 'app' ? sg.app.id === a.app.id : sg.kind === 'doc' && sg.appId ? sg.appId === a.app.id : null) },
        h('button', { class: 'row-main', type: 'button', onClick: () => { open ? expanded.delete(key) : expanded.add(key); renderSummaries(); } },
          appIcon(a.app, 18), h('span', { class: 'row-name' }, a.app.name), bar(a.secs, max, hex(a.app.color)),
          h('span', { class: 'row-pct' }, pct(a.secs, appTotal)), h('span', { class: 'row-dur' }, dur(a.secs))));
      if (open) {
        const titles = [...a.titles.entries()].sort((x, y) => y[1] - x[1]);
        row.append(h('div', { class: 'sub-list' }, titles.slice(0, 60).map(([title, secs]) => h('div', { class: 'sub-row', title, ...hoverHighlight((sg) => sg.kind === 'app' ? sg.app.id === a.app.id && sg.title === title : null) },
          h('span', { class: 'row-name' }, title || t('(no title)')), h('span', { class: 'row-dur' }, dur(secs))))));
      }
      return row;
    }), 'apps'));
    appsCard.append(list);

    // Documents
    clear(docsCard);
    const tabs = segmented([['sites', t('Websites') + ` ${c.sites.length || ''}`, 'globe'], ['files', t('Files') + ` ${c.files.length || ''}`, 'file']], docTab, (v) => { docTab = v; renderSummaries(); });
    docsCard.append(cardHead(t('Documents'), 'file', tabs));
    const dl = h('div', { class: 'list' });
    if (docTab === 'sites') {
      if (!c.sites.length) dl.append(empty('globe', t('No websites'), t('Websites are read from the browser address bar, or from the optional browser extension.')));
      const smax = c.sites.length ? c.sites[0].secs : 1;
      dl.append(...listRows(c.sites.map((s) => {
        const key = 'site:' + s.grp;
        const open = expanded.has(key);
        const row = h('div', { class: 'row' + (open ? ' open' : ''), ...hoverHighlight((sg) => sg.kind === 'doc' ? sg.doc.kind === 2 && sg.site === s.grp : null) },
          h('button', { class: 'row-main', type: 'button', onClick: () => { open ? expanded.delete(key) : expanded.add(key); renderSummaries(); } },
            kindIcon(2, s.grp, 18), h('span', { class: 'row-name' }, s.grp), bar(s.secs, smax, siteColor(s.grp)), h('span', { class: 'row-dur' }, dur(s.secs))));
        if (open) {
          const pages = [...s.pages.values()].sort((x, y) => y.secs - x.secs);
          row.append(h('div', { class: 'sub-list' }, pages.slice(0, 80).map((p) => h('div', { class: 'sub-row', title: p.doc.key },
            h('span', { class: 'row-name' }, h('span', null, p.title || (p.doc.name && p.doc.name !== p.doc.key ? p.doc.name : shortURL(p.doc.key))), h('small', { class: 'mono' }, shortURL(p.doc.key))),
            button('', { icon: 'external', small: true, variant: 'ghost', title: t('Open in browser'), onClick: () => api.post('open', { target: p.doc.key }) }),
            h('span', { class: 'row-dur' }, dur(p.secs))))));
        }
        return row;
      }), 'sites'));
    } else {
      if (!c.files.length) dl.append(empty('file', t('No files'), t(byOS('Files are detected from the open document, the window title and Spotlight.', 'Files are detected from the window title, Office/WPS, Explorer and Windows Recent items.'))));
      const fmax = c.files.length ? c.files[0].secs : 1;
      dl.append(...listRows(c.files.map((f) => {
        const full = /^[a-zA-Z]:[\\/]|^\\\\/.test(f.doc.key);
        return h('div', { class: 'row', ...hoverHighlight((sg) => sg.kind === 'doc' ? sg.doc.key === f.doc.key : null) },
          h('div', { class: 'row-main', title: f.doc.key },
            kindIcon(1, f.doc.key, 18),
            h('span', { class: 'row-name' }, h('span', null, f.doc.name), full ? h('small', { class: 'mono' }, dirName(f.doc.key)) : null),
            full ? button('', { icon: 'folder', small: true, variant: 'ghost', title: t('Show in folder'), onClick: (e) => { e.stopPropagation(); api.post('reveal', { path: f.doc.key }).catch(() => toast(t('File not found'), { type: 'error' })); } }) : null,
            bar(f.secs, fmax, hashColor(f.doc.grp || f.doc.key)), h('span', { class: 'row-dur' }, dur(f.secs))));
      }), 'files'));
    }
    docsCard.append(dl);

    // Tags
    clear(tagsCard);
    tagsCard.append(cardHead(t('Tags'), 'tag', button(t('Tag'), { icon: 'plus', small: true, variant: 'ghost', title: t('Tag the selection or the whole day'), onClick: () => { const [rs, re] = rangeNow(); tagRange(rs, Math.min(re, liveNow || re)); } })));
    const tlst = h('div', { class: 'list' });
    const tot = c.tot;
    const tagTotal = c.tags.reduce((n, [, s]) => n + s, 0);
    tlst.append(h('div', { class: 'tag-summary' },
      h('div', null, h('span', { class: 'muted' }, t('Active')), h('b', null, dur(tot.active))),
      h('div', null, h('span', { class: 'muted' }, t('Tagged')), h('b', null, dur(tagTotal))),
      h('div', null, h('span', { class: 'muted' }, t('Away')), h('b', null, dur(tot.away)))));
    if (!c.tags.length) tlst.append(empty('tag', t('No tags yet'), t('Drag across the timeline and press E to tag what you were working on.')));
    const tmax = c.tags.length ? c.tags[0][1] : 1;
    for (const [tags, secs] of c.tags) {
      const color = data.tagMap[tags] && data.tagMap[tags].color ? hex(data.tagMap[tags].color) : hashColor(tags);
      tlst.append(h('div', { class: 'row', ...hoverHighlight((sg) => sg.kind === 'tag' ? sg.tag.tags === tags : null) },
        h('div', { class: 'row-main' }, h('span', { class: 'dot', style: { background: color } }), h('span', { class: 'row-name' }, tags), bar(secs, tmax, color), h('span', { class: 'row-pct' }, pct(secs, tagTotal)), h('span', { class: 'row-dur' }, dur(secs)))));
    }
    tagsCard.append(tlst);
  }

  function renderShots() {
    const shots = data.shots;
    board.setHidden('shots', !shots.length);
    if (!shots.length) return;
    clear(shotsCard);
    const strip = h('div', { class: 'shot-strip' });
    wheelScrollsX(strip);
    shots.forEach((ts, i) => strip.append(h('button', { class: 'shot', type: 'button', onClick: () => openLightbox(shots, i) }, h('img', { src: api.shotURL(ts, true), loading: 'lazy', alt: '' }), h('span', null, clock(ts)))));
    shotsCard.append(h('div', { class: 'card-head' }, icon('image', 16), h('h3', null, t('Screenshots')), h('span', { class: 'muted' }, shots.length), h('div', { class: 'spacer' }),
      button(t('View all'), { small: true, variant: 'ghost', onClick: () => go('screenshots/' + date) })), strip);
    requestAnimationFrame(() => { strip.scrollLeft = strip.scrollWidth; });
  }

  // ---- actions ----
  async function tagRange(s, e, preset = {}) {
    if (e <= s) return;
    if (await openTagDialog({ s, e, ...preset })) { await load(); }
  }

  async function editTag(tg) {
    if (await openTagDialog(tg)) await load();
  }

  async function clearTags() {
    if (!sel) return;
    await api.post('tags/clear', { s: sel.s, e: sel.e });
    await load();
    toast(t('Tags cleared'));
  }

  function tagAt(time) {
    return data.tags.find((tg) => tg.s <= time && tg.e > time);
  }

  async function pasteTag(s, e) {
    if (!clipboardTag) return;
    await api.post('tags/add', { s, e, ...clipboardTag });
    loadTags();
    await load();
  }

  function contextMenu(hit, e) {
    hideTip();
    const items = [];
    const seg = hit && hit.seg;
    if (seg && (!sel || seg.s < sel.s || seg.e > sel.e) && seg.kind !== 'shot') {
      tl.setSelection({ s: seg.s, e: seg.e });
    }
    const range = sel;
    if (range) {
      items.push({ label: t('Tag selection…'), icon: 'tag', hint: 'E', onClick: () => tagRange(range.s, range.e) });
      if (clipboardTag) items.push({ label: t('Paste tag') + ` "${clipboardTag.tags}"`, icon: 'paste', hint: keys('Ctrl+V'), onClick: () => pasteTag(range.s, range.e) });
      items.push({ label: t('Clear tags in selection'), icon: 'trash', hint: 'Del', onClick: clearTags });
      items.push({ label: t('Zoom to selection'), icon: 'zoomIn', onClick: () => tl.zoomTo(range.s, range.e) });
    }
    if (seg && seg.kind === 'tag') {
      items.push({ separator: true },
        { label: t('Edit tag…'), icon: 'edit', onClick: () => editTag(seg.tag) },
        { label: t('Copy tag'), icon: 'copy', hint: keys('Ctrl+C'), onClick: () => { clipboardTag = { tags: seg.tag.tags, notes: seg.tag.notes, billable: seg.tag.billable }; toast(t('Tag copied')); } },
        hit.t ? { label: t('Split here'), icon: 'scissors', onClick: async () => { await api.post('tags/split', { id: seg.tag.id, at: Math.round(hit.t) }); load(); } } : null,
        { label: seg.tag.billable ? t('Mark not billable') : t('Mark billable'), icon: 'dollar', hint: 'B', onClick: async () => { await api.post('tags/update', { ...seg.tag, billable: !seg.tag.billable }); load(); } },
        { label: t('Delete tag'), icon: 'trash', danger: true, onClick: async () => { await api.post('tags/delete', { ids: [seg.tag.id] }); load(); } });
    }
    if (seg && seg.kind === 'auto') {
      items.push({ separator: true }, { label: t('Apply this autotag'), icon: 'sparkles', onClick: async () => { await api.post('tags/add', { s: seg.s, e: seg.e, tags: seg.auto.tags, billable: seg.auto.billable }); loadTags(); load(); } });
    }
    if (lanes.some((l) => l.id === 'auto')) {
      items.push({ label: t('Apply all autotags for this day'), icon: 'sparkles', onClick: async () => {
        const r = await api.post('autotag/apply', { from: date, to: date, overwrite: false });
        toast(t('{0} tags added', r.count)); loadTags(); load();
      } });
    }
    if (seg && seg.kind === 'doc') {
      const d = seg.doc;
      items.push({ separator: true });
      if (d.kind === 2) items.push({ label: t('Open in browser'), icon: 'external', onClick: () => api.post('open', { target: d.key }) });
      else if (/^[a-zA-Z]:[\\/]|^\\\\/.test(d.key)) {
        items.push({ label: t('Open file'), icon: 'external', onClick: () => api.post('open', { target: d.key }).catch(() => toast(t('File not found'), { type: 'error' })) });
        items.push({ label: t('Show in folder'), icon: 'folder', onClick: () => api.post('reveal', { path: d.key }).catch(() => toast(t('File not found'), { type: 'error' })) });
      }
      items.push({ label: d.kind === 2 ? t('Copy URL') : t('Copy path'), icon: 'copy', onClick: () => navigator.clipboard.writeText(d.key) });
    }
    if (seg && seg.kind === 'app') {
      items.push({ separator: true },
        { label: t('Copy window title'), icon: 'copy', onClick: () => navigator.clipboard.writeText(seg.title) },
        { label: t('Filter by {0}', seg.app.name), icon: 'filter', onClick: () => list.setFilter(seg.app.name) });
    }
    if (seg && seg.kind === 'shot') items.push({ label: t('Open screenshot'), icon: 'image', onClick: () => openShot(seg.ts) });
    if (!items.length) items.push({ label: t('Drag across the timeline to select time'), disabled: true });
    menu(e.clientX, e.clientY, items.filter((x, i, arr) => x && !(x.separator && (i === 0 || arr[i - 1].separator))));
  }

  // ---- filter ----
  function applyFilter(q) {
    tl.setFilter(q);
  }

  // Tags each checked activity, merging neighbours into one tag.
  async function tagMany(segs) {
    const ranges = segs.map((x) => [x.s, x.e]).sort((a, b) => a[0] - b[0]);
    const merged = [];
    for (const r of ranges) {
      const last = merged[merged.length - 1];
      if (last && r[0] <= last[1] + 1) last[1] = Math.max(last[1], r[1]);
      else merged.push([...r]);
    }
    if (await openTagDialog({ s: merged[0][0], e: merged[merged.length - 1][1], ranges: merged }, { title: t('Tag {0} activities', segs.length) })) {
      list.checked.clear();
      load();
    }
  }

  // ---- keyboard ----
  function onKey(e) {
    if (document.querySelector('.modal-wrap, .lightbox')) return;
    const typing = /INPUT|TEXTAREA|SELECT/.test(document.activeElement && document.activeElement.tagName);
    if (mod(e) && e.key.toLowerCase() === 'f') { e.preventDefault(); list.focusFilter(); return; }
    if (typing) return;
    const k = e.key;
    if (k === 'F7' || (k === 'ArrowLeft' && !mod(e))) { e.preventDefault(); go('day/' + addDays(date, -1)); }
    else if (k === 'F8' || (k === 'ArrowRight' && !mod(e))) { e.preventDefault(); if (date < today()) go('day/' + addDays(date, 1)); }
    else if (k === 'F5') { e.preventDefault(); if (date !== today()) go('day/' + today()); else load(); }
    else if ((k === 'e' || k === 'E' || k === 'Enter') && sel) { e.preventDefault(); tagRange(sel.s, sel.e); }
    else if ((k === 'Delete' || k === 'Backspace') && sel) { e.preventDefault(); clearTags(); }
    else if (k === 'Escape') tl.setSelection(null);
    else if (k === '+' || k === '=') tl.zoomBy(1 / 1.6);
    else if (k === '-' || k === '_') tl.zoomBy(1.6);
    else if (k === '0' || k === ')') tl.fit(e.shiftKey);
    else if (mod(e) && k.toLowerCase() === 'c' && sel) {
      const tg = tagAt((sel.s + sel.e) / 2);
      if (tg) { clipboardTag = { tags: tg.tags, notes: tg.notes, billable: tg.billable }; toast(t('Tag copied')); }
    } else if (mod(e) && k.toLowerCase() === 'v' && sel) { e.preventDefault(); pasteTag(sel.s, sel.e); }
    else if ((k === 'b' || k === 'B') && sel) {
      const tg = tagAt((sel.s + sel.e) / 2);
      if (tg) api.post('tags/update', { ...tg, billable: !tg.billable }).then(load);
    }
  }
  document.addEventListener('keydown', onKey);

  const ticker = setInterval(() => {
    if (!isToday() || !data) return;
    applyLive();
    if (Date.now() / 1000 > data.to) go('day/' + today(), true);
  }, 1000);
  const kpiTicker = setInterval(() => { if (data && isToday()) { renderKpis(); if (!sel) renderSummaries(); } }, 30000);

  load(false);

  let reloadTimer = 0;
  return {
    refresh(ev) {
      if (!ev || ev.type === 'settings') { load(); return; }
      if (['seg', 'tags', 'shot', 'state', 'away', 'timers', 'icons'].includes(ev.type) && (isToday() || ev.type === 'tags' || ev.type === 'icons')) {
        clearTimeout(reloadTimer);
        reloadTimer = setTimeout(() => load(), ev.type === 'seg' ? 1200 : ev.type === 'icons' ? 1500 : 150);
      }
    },
    theme() { if (data) { buildLanes(); tl.setData({ from: data.from, to: data.to, lanes, now: tl.now }); list.setLanes(lanes, data.from, data.to); renderSummaries(); } },
    destroy() {
      document.removeEventListener('keydown', onKey);
      scroller && scroller.removeEventListener('scroll', onScroll);
      clearInterval(ticker);
      clearInterval(kpiTicker);
      tl.destroy();
      list.destroy();
      hideTip();
    },
  };
}
