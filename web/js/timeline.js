// Canvas timeline: stacked lanes of segments with zoom, pan, hover,
// range selection (with snapping), app icons, screenshot thumbnails and a
// 24-hour overview strip whose window can be dragged and resized. With
// opts.fill the lanes stretch or shrink to fill the host's height.
import { clock } from './fmt.js';
import { icon } from './ui.js';

const TICKS = [60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600];
const FONT = '"Segoe UI Variable Text", "Segoe UI", "Microsoft YaHei UI", system-ui, sans-serif';
const MINI_H = 44;
const MINI_GAP = 6; // .tl-mini margin-top

function lastStartingBefore(segs, t) {
  let lo = 0, hi = segs.length - 1, ans = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (segs[mid].s <= t) { ans = mid; lo = mid + 1; } else hi = mid - 1;
  }
  return ans;
}

function firstEndingAfter(segs, t) {
  let i = lastStartingBefore(segs, t);
  if (i < 0) return 0;
  while (i > 0 && segs[i - 1].e > t) i--;
  return segs[i].e > t ? i : i + 1;
}

function roundRect(ctx, x, y, w, h, r) {
  ctx.beginPath();
  if (ctx.roundRect) ctx.roundRect(x, y, w, h, Math.max(0, Math.min(r, w / 2, h / 2)));
  else ctx.rect(x, y, w, h);
}

export class Timeline {
  constructor(host, opts = {}) {
    this.opts = opts;
    this.host = host;
    this.wrap = document.createElement('div');
    this.wrap.className = 'tl-wrap';
    this.labels = document.createElement('div');
    this.labels.className = 'tl-labels';
    this.canvas = document.createElement('canvas');
    this.canvas.className = 'tl-canvas';
    this.mini = document.createElement('canvas');
    this.mini.className = 'tl-mini';
    this.wrap.append(this.canvas, this.labels);
    host.append(this.wrap, this.mini);
    this.labelW = 132;
    this.padR = 16;
    this.axisH = 36;
    this.gap = 8;
    this.lanes = [];
    this.from = 0;
    this.to = 86400;
    this.view = [0, 86400];
    this.sel = null;
    this.hover = null;
    this.mark = null;
    this.highlight = null;
    this.filter = '';
    this.now = 0;
    this.bounds = [];
    this.textW = new Map();
    this.images = new Map();
    this.drag = null;
    this.raf = 0;

    this.ro = new ResizeObserver(() => this.resize());
    this.ro.observe(host);
    this.canvas.addEventListener('wheel', (e) => this.onWheel(e), { passive: false });
    this.mini.addEventListener('wheel', (e) => this.onWheel(e, true), { passive: false });
    this.canvas.addEventListener('mousedown', (e) => this.onDown(e));
    this.canvas.addEventListener('mousemove', (e) => { if (!this.drag) this.onHoverMove(e); });
    this.canvas.addEventListener('mouseleave', () => { if (!this.drag) this.setHover(null); });
    this.canvas.addEventListener('dblclick', (e) => { const hit = this.hitAt(e); opts.onDblClick && opts.onDblClick(hit, e); });
    this.canvas.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      opts.onContext && opts.onContext(this.hitAt(e), e);
    });
    this.mini.addEventListener('mousedown', (e) => this.onMiniDown(e));
    this.mini.addEventListener('mousemove', (e) => this.onMiniHover(e));
    this.onMove = (e) => this.onDragMove(e);
    this.onUp = (e) => this.onDragUp(e);
    this.resize();
  }

  destroy() {
    this.ro.disconnect();
    window.removeEventListener('mousemove', this.onMove);
    window.removeEventListener('mouseup', this.onUp);
    cancelAnimationFrame(this.raf);
  }

  // ---- data & view ------------------------------------------------------

  setData({ from, to, lanes, now }, keepView = true) {
    const dayChanged = from !== this.from || to !== this.to;
    this.from = from;
    this.to = to;
    this.lanes = lanes;
    for (const l of lanes) if (l.base == null) l.base = l.h;
    this.now = now || 0;
    const b = [];
    for (const l of lanes) if (l.style !== 'shots') for (const s of l.segs) b.push(s.s, s.e);
    if (this.now) b.push(this.now);
    b.sort((x, y) => x - y);
    this.bounds = b;
    if (dayChanged || !keepView) this.view = this.defaultView();
    this.renderLabels();
    this.resize();
  }

  renderLabels() {
    this.labels.replaceChildren();
    let top = this.axisH;
    for (const l of this.lanes) {
      const el = document.createElement('div');
      el.className = 'tl-label';
      el.style.top = top + 'px';
      el.style.height = l.h + 'px';
      if (l.icon) el.append(icon(l.icon, 15));
      const span = document.createElement('span');
      span.textContent = l.label;
      el.append(span);
      this.labels.append(el);
      top += l.h + this.gap;
    }
  }

  /** Active hours with padding, or the whole day when there is no data. */
  defaultView() {
    const usage = this.lanes.find((l) => l.id === 'usage');
    const segs = (usage && usage.segs.length ? usage.segs : this.lanes.flatMap((l) => l.segs)).filter((s) => s.e > this.from && s.s < this.to);
    if (!segs.length) return [this.from, this.to];
    let s = Infinity, e = -Infinity;
    for (const x of segs) { if (x.s < s) s = x.s; if (x.e > e) e = x.e; }
    const pad = Math.max(900, (e - s) * 0.04);
    s = Math.max(this.from, Math.floor((s - pad) / 1800) * 1800);
    e = Math.min(this.to, Math.ceil((e + pad) / 1800) * 1800);
    if (e - s < 3600) { s = Math.max(this.from, s - 1800); e = Math.min(this.to, e + 1800); }
    return [s, e];
  }

  setView(s, e, animate = false) {
    const span = Math.max(60, Math.min(this.to - this.from, e - s));
    if (s < this.from) s = this.from;
    if (s + span > this.to) s = this.to - span;
    const target = [s, s + span];
    const done = () => this.opts.onView && this.opts.onView(this.view);
    if (!animate) {
      this.view = target;
      this.schedule();
      done();
      return;
    }
    const start = [...this.view], t0 = performance.now(), dur = 200;
    const step = (now) => {
      const k = Math.min(1, (now - t0) / dur), ease = 1 - Math.pow(1 - k, 3);
      this.view = [start[0] + (target[0] - start[0]) * ease, start[1] + (target[1] - start[1]) * ease];
      this.draw();
      if (k < 1) this.raf = requestAnimationFrame(step);
      else done();
    };
    cancelAnimationFrame(this.raf);
    this.raf = requestAnimationFrame(step);
  }

  zoomBy(f, center) {
    const [s, e] = this.view;
    const c = center ?? (this.sel ? (this.sel.s + this.sel.e) / 2 : (s + e) / 2);
    this.setView(c - (c - s) * f, c + (e - c) * f, true);
  }

  fit(full = false) {
    const [s, e] = full ? [this.from, this.to] : this.defaultView();
    this.setView(s, e, true);
  }

  zoomTo(s, e) {
    const pad = Math.max(60, (e - s) * 0.08);
    this.setView(s - pad, e + pad, true);
  }

  /** Scrolls (without zooming) so [s, e) is visible. */
  reveal(s, e) {
    const [vs, ve] = this.view;
    if (e > vs && s < ve) return; // already (partly) visible: don't move under the user
    const span = ve - vs;
    if (e - s > span) { this.zoomTo(s, e); return; }
    const c = (s + e) / 2;
    this.setView(c - span / 2, c + span / 2, true);
  }

  setSelection(sel, notify = true) {
    this.sel = sel && sel.e > sel.s ? { s: Math.max(this.from, sel.s), e: Math.min(this.to, sel.e) } : null;
    this.schedule();
    if (notify && this.opts.onSelect) this.opts.onSelect(this.sel);
  }

  /** fn(seg) returns true (emphasise), false (dim) or null (leave as is). */
  setHighlight(fn) { this.highlight = fn; this.schedule(); }
  /** Outlines one segment (e.g. the row hovered in the activity list). */
  setMark(seg) { if (seg !== this.mark) { this.mark = seg; this.schedule(); } }
  setFilter(text) { this.filter = (text || '').toLowerCase(); this.schedule(); }
  setNow(now) { this.now = now; this.schedule(); }

  // ---- geometry ---------------------------------------------------------

  /** With opts.fill, scales the lanes' heights to the room the host has. */
  fitLanes() {
    if (!this.opts.fill || !this.lanes.length) return;
    const base = this.lanes.reduce((n, l) => n + l.base, 0);
    const room = this.host.clientHeight - MINI_H - MINI_GAP - this.axisH - 2 - this.gap * this.lanes.length;
    const k = Math.max(0.5, Math.min(3, room / base));
    for (const l of this.lanes) l.h = Math.floor(l.base * k);
    this.renderLabels();
  }

  resize() {
    this.fitLanes();
    const w = this.host.clientWidth;
    let hgt = this.axisH;
    for (const l of this.lanes) hgt += l.h + this.gap;
    hgt += 2;
    const dpr = window.devicePixelRatio || 1;
    this.W = w;
    this.H = hgt;
    this.dpr = dpr;
    for (const [c, ch] of [[this.canvas, hgt], [this.mini, MINI_H]]) {
      c.style.width = w + 'px';
      c.style.height = ch + 'px';
      c.width = Math.round(w * dpr);
      c.height = Math.round(ch * dpr);
    }
    this.wrap.style.height = hgt + 'px';
    this.labels.style.width = this.labelW + 'px';
    this.draw();
  }

  get plotW() { return Math.max(10, this.W - this.labelW - this.padR); }
  x(t) { return this.labelW + ((t - this.view[0]) / (this.view[1] - this.view[0])) * this.plotW; }
  tAt(x) { return this.view[0] + ((x - this.labelW) / this.plotW) * (this.view[1] - this.view[0]); }
  mx(t) { return this.labelW + ((t - this.from) / (this.to - this.from)) * this.plotW; }

  laneAt(y) {
    let top = this.axisH;
    for (const l of this.lanes) {
      if (y >= top && y < top + l.h) return { lane: l, top };
      top += l.h + this.gap;
    }
    return null;
  }

  pos(e, el = this.canvas) {
    const r = el.getBoundingClientRect();
    return { x: e.clientX - r.left, y: e.clientY - r.top };
  }

  hitAt(e) {
    const { x, y } = this.pos(e);
    const t = this.tAt(x);
    if (x < this.labelW || x > this.labelW + this.plotW) return { t: null };
    if (y < this.axisH) return { t, axis: true };
    const la = this.laneAt(y);
    if (!la) return { t };
    const segs = la.lane.segs;
    const tol = ((la.lane.style === 'shots' ? 30 : 4) / this.plotW) * (this.view[1] - this.view[0]);
    let i = lastStartingBefore(segs, t);
    let seg = i >= 0 && segs[i].e > t ? segs[i] : null;
    if (!seg) {
      let best = Infinity;
      for (const j of [i, i + 1]) {
        const s = segs[j];
        if (!s) continue;
        const d = t < s.s ? s.s - t : t > s.e ? t - s.e : 0;
        if (d <= tol && d < best) { best = d; seg = s; }
      }
    }
    return { t, lane: la.lane, seg };
  }

  snap(t) {
    const b = this.bounds;
    if (!b.length) return Math.round(t);
    let lo = 0, hi = b.length - 1;
    while (lo < hi) { const m = (lo + hi) >> 1; if (b[m] < t) lo = m + 1; else hi = m; }
    let best = Math.round(t), bestD = 7;
    for (const j of [lo - 1, lo]) {
      if (j < 0 || j >= b.length) continue;
      const d = Math.abs(this.x(b[j]) - this.x(t));
      if (d < bestD) { bestD = d; best = b[j]; }
    }
    return best;
  }

  // ---- interaction ------------------------------------------------------

  onWheel(e, mini = false) {
    e.preventDefault();
    const { x } = this.pos(e, mini ? this.mini : this.canvas);
    const span = this.view[1] - this.view[0];
    if (e.shiftKey || Math.abs(e.deltaX) > Math.abs(e.deltaY)) {
      const d = (e.shiftKey ? e.deltaY : e.deltaX) * (span / this.plotW);
      this.setView(this.view[0] + d, this.view[1] + d);
      return;
    }
    const f = Math.exp(Math.max(-60, Math.min(60, e.deltaY)) * 0.004);
    const cx = Math.max(this.labelW, Math.min(this.labelW + this.plotW, x));
    const c = mini ? (this.view[0] + this.view[1]) / 2 : this.tAt(cx);
    this.setView(c - (c - this.view[0]) * f, c + (this.view[1] - c) * f);
  }

  onDown(e) {
    if (e.button === 2) return;
    const { x, y } = this.pos(e);
    const pan = e.button === 1 || y < this.axisH || x < this.labelW;
    let mode = pan ? 'pan' : 'pending';
    let edge = null;
    if (!pan && this.sel) {
      if (Math.abs(this.x(this.sel.s) - x) < 6) { mode = 'edge'; edge = 's'; }
      else if (Math.abs(this.x(this.sel.e) - x) < 6) { mode = 'edge'; edge = 'e'; }
    }
    this.drag = { mode, edge, x0: x, y0: y, view0: [...this.view], t0: this.snap(this.tAt(x)), shift: e.shiftKey, ev: e };
    window.addEventListener('mousemove', this.onMove);
    window.addEventListener('mouseup', this.onUp);
    e.preventDefault();
  }

  onDragMove(e) {
    const d = this.drag;
    if (!d) return;
    const { x } = this.pos(e);
    if (d.mode === 'pan') {
      const dt = ((x - d.x0) / this.plotW) * (d.view0[1] - d.view0[0]);
      this.setView(d.view0[0] - dt, d.view0[1] - dt);
      this.canvas.style.cursor = 'grabbing';
      return;
    }
    if (d.mode === 'pending' && Math.abs(x - d.x0) > 3) d.mode = 'select';
    const t = this.snap(Math.max(this.from, Math.min(this.to, this.tAt(x))));
    if (d.mode === 'select') {
      this.sel = { s: Math.min(d.t0, t), e: Math.max(d.t0, t) };
    } else if (d.mode === 'edge') {
      const other = d.edge === 's' ? this.sel.e : this.sel.s;
      this.sel = { s: Math.min(other, t), e: Math.max(other, t) };
    } else return;
    this.schedule();
    this.opts.onSelecting && this.opts.onSelecting(this.sel);
    this.setHover(null);
  }

  onDragUp(e) {
    const d = this.drag;
    this.drag = null;
    window.removeEventListener('mousemove', this.onMove);
    window.removeEventListener('mouseup', this.onUp);
    this.canvas.style.cursor = '';
    if (!d) return;
    if (d.mode === 'select' || d.mode === 'edge') {
      this.setSelection(this.sel);
      return;
    }
    if (d.mode === 'pending') {
      const hit = this.hitAt(d.ev);
      if (hit.seg && hit.lane.style !== 'shots') {
        const s = hit.seg;
        if (d.shift && this.sel) this.setSelection({ s: Math.min(this.sel.s, s.s), e: Math.max(this.sel.e, s.e) });
        else this.setSelection({ s: s.s, e: s.e });
      } else if (!hit.seg) {
        this.setSelection(null);
      }
      this.opts.onClickSeg && this.opts.onClickSeg(hit, e);
    }
  }

  onHoverMove(e) {
    const { x, y } = this.pos(e);
    if (this.sel && y >= this.axisH && (Math.abs(this.x(this.sel.s) - x) < 6 || Math.abs(this.x(this.sel.e) - x) < 6)) {
      this.canvas.style.cursor = 'ew-resize';
    } else if (y < this.axisH || x < this.labelW) {
      this.canvas.style.cursor = 'grab';
    } else {
      this.canvas.style.cursor = '';
    }
    const hit = this.hitAt(e);
    this.setHover(hit.seg ? hit : null, e);
  }

  setHover(hit, e) {
    const prev = this.hover && this.hover.seg;
    this.hover = hit;
    if ((hit && hit.seg) !== prev) this.schedule();
    this.opts.onHover && this.opts.onHover(hit, e);
  }

  miniGrip(x) {
    const v0 = this.mx(this.view[0]), v1 = this.mx(this.view[1]);
    if (Math.abs(x - v0) <= 7) return 's';
    if (Math.abs(x - v1) <= 7) return 'e';
    if (x > v0 && x < v1) return 'move';
    return null;
  }

  onMiniHover(e) {
    const g = this.miniGrip(this.pos(e, this.mini).x);
    this.mini.style.cursor = g === 's' || g === 'e' ? 'ew-resize' : g === 'move' ? 'grab' : 'pointer';
  }

  onMiniDown(e) {
    const toT = (cx) => this.from + ((cx - this.mini.getBoundingClientRect().left - this.labelW) / this.plotW) * (this.to - this.from);
    const grip = this.miniGrip(this.pos(e, this.mini).x);
    const span = this.view[1] - this.view[0];
    const t = toT(e.clientX);
    let offset = t - this.view[0];
    if (!grip) {
      offset = span / 2;
      this.setView(t - offset, t - offset + span, true);
    }
    const v0 = [...this.view];
    const move = (ev) => {
      const tt = Math.max(this.from, Math.min(this.to, toT(ev.clientX)));
      if (grip === 's') this.setView(Math.min(tt, v0[1] - 60), v0[1]);
      else if (grip === 'e') this.setView(v0[0], Math.max(tt, v0[0] + 60));
      else this.setView(tt - offset, tt - offset + span);
    };
    const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up); };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
    e.preventDefault();
  }

  // ---- drawing ----------------------------------------------------------

  schedule() {
    if (this.pending) return;
    this.pending = true;
    requestAnimationFrame(() => { this.pending = false; this.draw(); });
  }

  colors() {
    const cs = getComputedStyle(document.documentElement);
    const v = (n) => cs.getPropertyValue(n).trim();
    return {
      text: v('--text'), text2: v('--text-2'), text3: v('--text-3'), lane: v('--lane'), grid: v('--grid'), surface: v('--surface'),
      accent: v('--accent'), sel: v('--sel'), away: v('--away'), awayStrong: v('--away-strong'), active: v('--active'), now: v('--now'),
      theme: document.documentElement.dataset.theme,
    };
  }

  /** Cached image (app icon or thumbnail); null until loaded. */
  image(url) {
    let img = this.images.get(url);
    if (!img) {
      img = new Image();
      img.decoding = 'async';
      img.onload = () => { img.ready = true; this.schedule(); };
      img.onerror = () => { img.failed = true; };
      img.src = url;
      if (this.images.size > 600) this.images.clear();
      this.images.set(url, img);
    }
    return img.ready ? img : null;
  }

  measure(ctx, s) {
    let w = this.textW.get(s);
    if (w === undefined) {
      w = ctx.measureText(s).width;
      if (this.textW.size > 6000) this.textW.clear();
      this.textW.set(s, w);
    }
    return w;
  }

  fitText(ctx, s, max) {
    if (max <= 8) return '';
    if (this.measure(ctx, s) <= max) return s;
    let lo = 0, hi = s.length;
    while (lo < hi) {
      const m = (lo + hi + 1) >> 1;
      if (this.measure(ctx, s.slice(0, m) + '…') <= max) lo = m; else hi = m - 1;
    }
    return lo ? s.slice(0, lo) + '…' : '';
  }

  awayPattern(ctx, C) {
    const key = C.away + C.awayStrong + this.dpr;
    if (this._pat && this._patKey === key) return this._pat;
    const c = document.createElement('canvas');
    const n = Math.round(8 * this.dpr);
    c.width = c.height = n;
    const g = c.getContext('2d');
    g.fillStyle = C.away;
    g.fillRect(0, 0, n, n);
    g.strokeStyle = C.awayStrong;
    g.globalAlpha = 0.35;
    g.lineWidth = 1.5 * this.dpr;
    g.beginPath();
    g.moveTo(0, n); g.lineTo(n, 0);
    g.moveTo(-n / 2, n / 2); g.lineTo(n / 2, -n / 2);
    g.moveTo(n / 2, n * 1.5); g.lineTo(n * 1.5, n / 2);
    g.stroke();
    this._pat = ctx.createPattern(c, 'repeat');
    this._pat.setTransform(new DOMMatrix().scale(1 / this.dpr));
    this._patKey = key;
    return this._pat;
  }

  draw() {
    const ctx = this.canvas.getContext('2d');
    const { W, H, dpr } = this;
    if (!W) return;
    const C = this.colors();
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    const [vs, ve] = this.view;
    const span = ve - vs;
    const pw = this.plotW, L = this.labelW;
    const top0 = this.axisH;

    // Lanes.
    let top = top0;
    for (const lane of this.lanes) {
      ctx.fillStyle = C.lane;
      roundRect(ctx, L, top, pw, lane.h, 4);
      ctx.fill();
      top += lane.h + this.gap;
    }

    // Ticks and grid.
    let step = TICKS[TICKS.length - 1];
    for (const s of TICKS) { if ((s / span) * pw >= 84) { step = s; break; } }
    const minor = step >= 3600 ? step / 4 : step >= 600 ? step / 5 : step / 2;
    const tz = new Date(vs * 1000).getTimezoneOffset() * 60;
    ctx.font = `500 12px ${FONT}`;
    ctx.textBaseline = 'middle';
    ctx.textAlign = 'center';
    ctx.lineWidth = 1;
    for (let t = Math.ceil((vs - tz) / minor) * minor + tz; t <= ve; t += minor) {
      const x = Math.round(this.x(t)) + 0.5;
      const major = Math.abs((t - tz) % step) < 1;
      ctx.strokeStyle = C.grid;
      ctx.globalAlpha = major ? 0.9 : 0.4;
      ctx.beginPath();
      ctx.moveTo(x, major ? top0 - 8 : top0 - 4);
      ctx.lineTo(x, H);
      ctx.stroke();
      ctx.globalAlpha = 1;
      if (major) {
        ctx.fillStyle = C.text3;
        ctx.fillText(clock(t), x, 14);
      }
    }

    // Segments.
    top = top0;
    for (const lane of this.lanes) {
      ctx.save();
      ctx.beginPath();
      ctx.rect(L, top, pw, lane.h);
      ctx.clip();
      if (lane.style === 'shots') this.drawShots(ctx, lane, top, C);
      else this.drawLane(ctx, lane, top, C);
      ctx.restore();
      top += lane.h + this.gap;
    }
    this.opts.onDraw && this.opts.onDraw();

    // Hover / mark outlines.
    for (const [seg, width] of [[this.mark, 2], [this.hover && this.hover.seg, 1.5]]) {
      if (!seg) continue;
      const lane = seg.laneId ? this.lanes.find((l) => l.id === seg.laneId) : this.lanes.find((l) => l.segs.includes(seg));
      if (!lane || lane.style === 'shots') continue;
      const lt = this.laneTopOf(lane);
      const x0 = Math.max(L, this.x(seg.s)), x1 = Math.min(L + pw, this.x(seg.e));
      ctx.strokeStyle = C.text;
      ctx.lineWidth = width;
      roundRect(ctx, x0 - 0.5, lt + 2.5, Math.max(3, x1 - x0 + 1), lane.h - 5, 2.5);
      ctx.stroke();
    }
    ctx.lineWidth = 1;

    // Selection.
    if (this.sel) {
      const x0 = Math.max(L, this.x(this.sel.s)), x1 = Math.min(L + pw, this.x(this.sel.e));
      if (x1 > x0 - 1) {
        ctx.fillStyle = C.sel;
        ctx.fillRect(x0, top0 - 6, Math.max(1, x1 - x0), H - top0 + 6);
        ctx.strokeStyle = C.accent;
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(x0, top0 - 6); ctx.lineTo(x0, H);
        ctx.moveTo(x1, top0 - 6); ctx.lineTo(x1, H);
        ctx.stroke();
        ctx.fillStyle = C.accent;
        for (const x of [x0, x1]) { roundRect(ctx, x - 3, top0 - 10, 6, 10, 3); ctx.fill(); }
        ctx.lineWidth = 1;
      }
    }

    // Now indicator with a time label.
    if (this.now && this.now >= vs && this.now <= ve) {
      const x = Math.round(this.x(this.now)) + 0.5;
      ctx.strokeStyle = C.now;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.moveTo(x, top0 - 4);
      ctx.lineTo(x, H);
      ctx.stroke();
      ctx.font = `600 11px ${FONT}`;
      const label = clock(this.now);
      const w = this.measure(ctx, label) + 12;
      const lx = Math.min(Math.max(x - w / 2, L), L + pw - w);
      ctx.fillStyle = C.now;
      roundRect(ctx, lx, 4, w, 19, 9.5);
      ctx.fill();
      ctx.fillStyle = '#fff';
      ctx.textAlign = 'center';
      ctx.fillText(label, lx + w / 2, 14);
      ctx.lineWidth = 1;
    }
    this.drawMini(C);
  }

  laneTopOf(lane) {
    let top = this.axisH;
    for (const l of this.lanes) { if (l === lane) return top; top += l.h + this.gap; }
    return top;
  }

  drawLane(ctx, lane, top, C) {
    const [vs, ve] = this.view;
    const segs = lane.segs;
    const pad = lane.style === 'usage' ? 5 : 6;
    const y = top + pad, hh = lane.h - pad * 2;
    const filter = this.filter, hl = this.highlight;
    const L = this.labelW, R = this.labelW + this.plotW;
    const pat = lane.style === 'usage' ? this.awayPattern(ctx, C) : null;
    ctx.font = `600 13px ${FONT}`;
    ctx.textBaseline = 'middle';
    ctx.textAlign = 'left';

    // Consecutive segments of the same item (same app, page or tag) are
    // drawn as one block, so title changes don't fragment the lane.
    let run = null;
    const flush = () => {
      if (!run) return;
      let { x0, x1 } = run;
      let w = x1 - x0;
      if (w < 1.5) { const c = (x0 + x1) / 2; x0 = c - 0.75; x1 = c + 0.75; w = 1.5; }
      // A hairline gap separates neighbouring blocks.
      const g = w > 4 ? 0.75 : 0;
      ctx.globalAlpha = run.dim ? 0.15 : lane.style === 'auto' ? 0.5 : 1;
      if (pat && run.seg.state === 0) ctx.fillStyle = pat;
      else ctx.fillStyle = run.seg.color;
      roundRect(ctx, x0 + g, y, w - 2 * g, hh, Math.min(2.5, w / 3));
      ctx.fill();
      if (lane.style === 'auto' && w > 4) {
        ctx.globalAlpha = run.dim ? 0.2 : 0.95;
        ctx.strokeStyle = run.seg.color;
        ctx.setLineDash([4, 3]);
        roundRect(ctx, x0 + g + 0.5, y + 0.5, w - 2 * g - 1, hh - 1, 2);
        ctx.stroke();
        ctx.setLineDash([]);
      }
      const vx0 = Math.max(x0, L) + 7, vx1 = Math.min(x1, R) - 7;
      if (vx1 - vx0 > 18 && lane.style !== 'usage') {
        ctx.globalAlpha = run.dim ? 0.3 : 1;
        let tx = vx0;
        const ic = run.seg.icon ? this.image(run.seg.icon) : null;
        if (ic && vx1 - vx0 > 26) {
          const s = lane.h >= 56 ? 20 : 16;
          ctx.save();
          ctx.shadowColor = 'rgba(0,0,0,.25)';
          ctx.shadowBlur = 2;
          ctx.drawImage(ic, tx, top + (lane.h - s) / 2, s, s);
          ctx.restore();
          tx += s + 6;
        }
        const label = this.fitText(ctx, run.seg.label || '', vx1 - tx);
        if (label) {
          ctx.fillStyle = run.seg.ink || '#fff';
          ctx.fillText(label, tx, top + lane.h / 2 + 0.5);
        }
      } else if (lane.style === 'usage' && w > 70 && run.seg.state === 1 && run.seg.label) {
        ctx.globalAlpha = run.dim ? 0.3 : 0.95;
        ctx.font = `600 11px ${FONT}`;
        const label = this.fitText(ctx, run.seg.label, vx1 - vx0);
        ctx.fillStyle = '#fff';
        ctx.fillText(label, vx0, top + lane.h / 2 + 0.5);
        ctx.font = `600 13px ${FONT}`;
      }
      ctx.globalAlpha = 1;
      run = null;
    };
    for (let i = firstEndingAfter(segs, vs); i < segs.length && segs[i].s < ve; i++) {
      const sg = segs[i];
      const x0 = this.x(sg.s), x1 = this.x(sg.e);
      const hv = hl ? hl(sg, lane) : null;
      const dim = hv === false || (filter && !(sg.search || sg.label || '').toLowerCase().includes(filter));
      const key = sg.group ?? sg.label;
      if (run && run.key === key && run.dim === dim && x0 - run.x1 < 1.2) {
        if (x1 > run.x1) run.x1 = x1;
        continue;
      }
      // Sub-pixel slivers of different items in the same pixel: keep the first.
      if (run && x1 - x0 < 1 && x0 - run.x1 < 0 && run.x1 - run.x0 < 1.5) continue;
      flush();
      run = { x0, x1, key, dim, seg: sg };
    }
    flush();
  }

  drawShots(ctx, lane, top, C) {
    const [vs, ve] = this.view;
    const segs = lane.segs;
    const hov = this.hover && this.hover.seg;
    const th = lane.h - 8, tw = Math.round(th * 16 / 10);
    const i0 = firstEndingAfter(segs, vs - (tw / this.plotW) * (ve - vs));
    // Show thumbnails when there is room for them; otherwise ticks.
    let minGap = Infinity;
    for (let i = i0 + 1; i < segs.length && segs[i - 1].s < ve; i++) minGap = Math.min(minGap, this.x(segs[i].s) - this.x(segs[i - 1].s));
    const thumbs = minGap >= tw * 0.55 || segs.length === 1;
    let lastRight = -Infinity;
    for (let i = i0; i < segs.length && segs[i].s < ve + (tw / this.plotW) * (ve - vs); i++) {
      const sg = segs[i];
      const x = this.x(sg.s);
      if (thumbs && x - tw / 2 >= lastRight - tw * 0.45) {
        const x0 = Math.round(x - tw / 2), y0 = top + 4;
        const img = sg.thumb ? this.image(sg.thumb) : null;
        ctx.save();
        roundRect(ctx, x0, y0, tw, th, 3);
        ctx.clip();
        if (img) ctx.drawImage(img, x0, y0, tw, th);
        else { ctx.fillStyle = C.grid; ctx.fillRect(x0, y0, tw, th); }
        ctx.restore();
        ctx.strokeStyle = sg === hov ? C.accent : C.surface;
        ctx.lineWidth = sg === hov ? 2 : 1.5;
        roundRect(ctx, x0, y0, tw, th, 3);
        ctx.stroke();
        ctx.lineWidth = 1;
        lastRight = x0 + tw;
      } else {
        ctx.fillStyle = C.accent;
        ctx.globalAlpha = sg === hov ? 1 : 0.7;
        roundRect(ctx, Math.round(x) - 2, top + 8, 4, lane.h - 16, 2);
        ctx.fill();
        ctx.globalAlpha = 1;
      }
    }
  }

  drawMini(C) {
    const ctx = this.mini.getContext('2d');
    const { W, dpr } = this;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, MINI_H);
    const L = this.labelW, pw = this.plotW;
    const ty = 6, th = 22;
    ctx.fillStyle = C.lane;
    roundRect(ctx, L, ty, pw, th, 4);
    ctx.fill();
    ctx.save();
    roundRect(ctx, L, ty, pw, th, 4);
    ctx.clip();
    const usage = this.lanes.find((l) => l.id === 'usage');
    if (usage) {
      const pat = this.awayPattern(ctx, C);
      for (const s of usage.segs) {
        ctx.fillStyle = s.state === 0 ? pat : s.color;
        const x0 = this.mx(s.s);
        ctx.fillRect(x0, ty, Math.max(1, this.mx(s.e) - x0), th);
      }
    }
    ctx.restore();
    // Hour ticks and labels.
    ctx.font = `500 11px ${FONT}`;
    ctx.fillStyle = C.text3;
    ctx.textBaseline = 'middle';
    ctx.textAlign = 'center';
    for (let hr = 0; hr <= 24; hr++) {
      const x = Math.round(this.mx(this.from + hr * 3600)) + 0.5;
      if (hr % 3 === 0 && hr > 0 && hr < 24) ctx.fillText(String(hr).padStart(2, '0') + ':00', x, ty + th + 9);
    }
    ctx.fillStyle = 'rgba(0,0,0,0)';
    // Viewport window with grips.
    const v0 = this.mx(this.view[0]), v1 = this.mx(this.view[1]);
    ctx.fillStyle = C.sel;
    roundRect(ctx, v0, ty - 3, Math.max(6, v1 - v0), th + 6, 4);
    ctx.fill();
    ctx.strokeStyle = C.accent;
    ctx.lineWidth = 2;
    roundRect(ctx, v0, ty - 3, Math.max(6, v1 - v0), th + 6, 4);
    ctx.stroke();
    ctx.fillStyle = C.accent;
    for (const x of [v0, v1]) { roundRect(ctx, x - 2.5, ty + th / 2 - 6, 5, 12, 2.5); ctx.fill(); }
    ctx.lineWidth = 1;
    if (this.now >= this.from && this.now <= this.to) {
      ctx.fillStyle = C.now;
      ctx.fillRect(Math.round(this.mx(this.now)) - 1, ty - 3, 2, th + 6);
    }
  }
}
