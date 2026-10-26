// Day view layout: blocks sit in full-width rows. A row holds one to three
// blocks whose widths (in 12 columns, fractional) always add up to the full
// width, and the blocks of a row share its height. Each block has a minimum
// width and height. A row with a fit-to-content block takes that block's
// height.
//
// A block is picked up by pressing on any empty part of it. While it is
// dragged the layout stays still and a marker shows where it will go.
// Dropped on the middle of another block the two swap places, each taking
// the other's width and row height (as far as their minimums allow). Dropped on the
// left or right part (30%) of a block it goes beside it in that row (the
// row's blocks share the width again, the moved one keeping its width when it
// can); near the top or bottom edge of a row, or between rows, it gets a row
// of its own there. The row it left closes up to full width. The border
// between two blocks of a row moves both, the bottom border sets the row's
// height; both follow the pointer, catching at thirds and halves of the row.
// A locked (pinned) board can't be dragged or resized.
import { h } from './ui.js';
import { t } from './i18n.js';

export const COLS = 12;
const ROW = 30; // px per height unit
const GAP = 14; // px between blocks
const NARROW = 860; // below this width blocks simply stack
const MIN_W = 3; // default minimum width, in columns
const MIN_H = 5; // default minimum row height, in units
const MAX_H = 40;
const SNAP_W = [3, 4, 6, 8, 9]; // border positions (columns) a width catches at
const SNAP_PX = 10; // how close the pointer has to be for that

// Pressing on these never starts a drag: they have their own interaction.
const NO_DRAG = 'button, a, input, select, textarea, label, [contenteditable], .row, .sub-row, .al-row, .al-head, .al-bulk, ' +
  '.tl-wrap, .tl-mini, canvas, .shot, .chip, .segmented, .suggest, .sel-bar, .banner, .filter-wrap, .gi-edge, .empty .btn';

// A press on a scrollbar targets the scrolling element itself.
function onScrollbar(e) {
  const el = e.target;
  if (!(el instanceof HTMLElement)) return false;
  const r = el.getBoundingClientRect();
  return (el.scrollHeight > el.clientHeight && e.clientX > r.left + el.clientWidth) ||
    (el.scrollWidth > el.clientWidth && e.clientY > r.top + el.clientHeight);
}

const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v));
const unitPx = (u) => u * ROW + (u - 1) * GAP;
const copy = (rows) => rows.map((r) => ({ h: r.h, items: r.items.map((i) => ({ ...i })) }));

/**
 * Widths for the blocks of one row ([{ id, w }], w = wanted width): the wanted
 * widths scaled to the full width, each at least minW(id), or null if the
 * minimums don't fit (or there are more than three blocks).
 */
export function fitRow(items, minW) {
  if (!items.length || items.length > 3) return null;
  const mins = items.map((i) => minW(i.id));
  if (mins.reduce((n, m) => n + m, 0) > COLS + 1e-6) return null;
  const want = items.map((i) => Math.max(0.01, i.w || 0));
  // Blocks that would end up under their minimum get it; the others share
  // what is left in proportion.
  const fixed = new Set();
  for (;;) {
    const room = COLS - [...fixed].reduce((n, k) => n + mins[k], 0);
    const free = want.reduce((n, w, k) => (fixed.has(k) ? n : n + w), 0);
    const under = want.findIndex((w, k) => !fixed.has(k) && (w * room) / free < mins[k] - 1e-6);
    if (under < 0) return want.map((w, k) => (fixed.has(k) ? mins[k] : (w * room) / free));
    fixed.add(under);
  }
}

/** Fits every row to the full width, keeping the widths' proportions as far
 *  as it can; a row that can't be fitted is split into more rows. */
export function normalize(rows, minW) {
  const out = [];
  for (const r of rows) {
    let items = r.items;
    while (items.length) {
      let k = Math.min(3, items.length), fit = null;
      for (; k > 0 && !fit; k--) {
        const part = items.slice(0, k);
        const sum = part.reduce((n, i) => n + i.w, 0) || 1;
        fit = fitRow(part.map((i) => ({ id: i.id, w: (i.w * COLS) / sum })), minW);
        if (fit) break;
      }
      if (!fit) { k = 1; fit = [COLS]; }
      out.push({ h: r.h, items: items.slice(0, k).map((it, i) => ({ id: it.id, w: fit[i] })) });
      items = items.slice(k);
    }
  }
  return out;
}

export class Board {
  /**
   * blocks: [{ id, el, row, w, h, minW, minH, autoHeight }] in their default
   *   rows (row index) and order; h is the default row height, minW and minH
   *   the smallest width (columns) and height (units) it may have.
   * layout: saved [{ id, x, y, w, h }] (y = row; may be empty or stale)
   * onChange(layout) is called after every drag or resize.
   */
  constructor(host, blocks, layout, onChange) {
    this.host = host;
    this.onChange = onChange;
    this.blocks = new Map(blocks.map((b) => [b.id, b]));
    this.minW = (id) => this.blocks.get(id).minW || MIN_W;
    const x = {};
    this.defaults = this.fromItems(blocks.map((b) => {
      const it = { id: b.id, x: x[b.row] || 0, y: b.row, w: b.w, h: b.h || 0 };
      x[b.row] = it.x + b.w;
      return it;
    }));
    this.hidden = new Set();
    this.items = new Map();
    this.edges = new Map();
    this.preview = null; // rows shown while dragging or resizing
    this.dragId = null;
    this.locked = false;
    host.classList.add('grid');
    this.holder = h('div', { class: 'gi-holder', hidden: true });
    this.marker = h('div', { class: 'gi-drop', hidden: true }, h('span', { class: 'gi-drop-label' }));
    host.append(this.holder, this.marker);
    for (const b of blocks) {
      const el = h('div', { class: 'gi' + (b.autoHeight ? ' auto' : ''), dataset: { id: b.id } }, b.el);
      const edges = { e: h('div', { class: 'gi-edge e', hidden: true }), s: h('div', { class: 'gi-edge s', hidden: true }) };
      for (const [dir, edge] of Object.entries(edges)) {
        edge.addEventListener('pointerdown', (e) => this.startResize(e, b.id, edge, dir));
        el.append(edge);
      }
      el.addEventListener('pointerdown', (e) => this.press(e, b.id));
      host.append(el);
      this.items.set(b.id, el);
      this.edges.set(b.id, edges);
      if (b.autoHeight) new ResizeObserver(() => this.render()).observe(b.el);
    }
    const saved = this.fromItems(layout);
    this.rows = saved.length ? saved : copy(this.defaults);
    this.ro = new ResizeObserver(() => this.render());
    this.ro.observe(host);
    this.render();
    // Animate moves only after the first placement.
    setTimeout(() => host.classList.add('ready'), 60);
  }

  /** Rows from saved items: items with the same y form a row. Blocks the
   *  layout doesn't have get a row of their own at the end. */
  fromItems(items) {
    const seen = new Set();
    const ok = [];
    for (const it of items || []) {
      if (!this.blocks.has(it.id) || seen.has(it.id) || ![it.x, it.y, it.w].every(Number.isFinite)) continue;
      seen.add(it.id);
      ok.push(it);
    }
    if (!ok.length) return [];
    ok.sort((a, b) => a.y - b.y || a.x - b.x);
    const rows = [];
    let y = null;
    for (const it of ok) {
      if (y !== it.y) rows.push({ h: it.h, items: [] });
      y = it.y;
      rows[rows.length - 1].items.push({ id: it.id, w: it.w });
    }
    for (const b of this.blocks.values()) if (!seen.has(b.id)) rows.push({ h: b.h, items: [{ id: b.id, w: COLS }] });
    return this.tidy(rows);
  }

  isAuto(row) { return row.items.some((i) => this.blocks.get(i.id).autoHeight); }

  /** The smallest height every block of a row allows. */
  minH(row) { return Math.max(...row.items.map((i) => this.blocks.get(i.id).minH || MIN_H)); }

  /** Full-width rows with heights the blocks allow (0 for fit-to-content rows). */
  tidy(rows) {
    return normalize(rows.filter((r) => r.items.length), this.minW).map((r) => {
      if (this.isAuto(r)) return { ...r, h: 0 };
      const def = Math.max(...r.items.map((i) => this.blocks.get(i.id).h || 0));
      return { ...r, h: clamp(r.h || def || 10, this.minH(r), MAX_H) };
    });
  }

  visibleRows(rows = this.rows) {
    return this.tidy(rows.map((r) => ({ h: r.h, items: r.items.filter((i) => !this.hidden.has(i.id)) })));
  }

  /** Puts hidden blocks back (each in a row of its own where it was). */
  withHidden(rows) {
    const out = copy(rows);
    this.rows.forEach((r, i) => {
      for (const it of r.items) {
        if (this.hidden.has(it.id)) out.splice(Math.min(i, out.length), 0, { h: r.h, items: [{ id: it.id, w: COLS }] });
      }
    });
    return this.tidy(out);
  }

  current() {
    const out = [];
    this.rows.forEach((r, y) => {
      let x = 0;
      for (const it of r.items) { out.push({ id: it.id, x, y, w: it.w, h: r.h }); x += it.w; }
    });
    return out;
  }

  reset() {
    this.rows = copy(this.defaults);
    this.render();
    this.onChange(this.current());
  }

  setLocked(locked) {
    this.locked = !!locked;
    this.host.classList.toggle('locked', this.locked);
    this.render();
  }

  setHidden(id, hidden) {
    if (hidden === this.hidden.has(id)) return;
    if (hidden) this.hidden.add(id); else this.hidden.delete(id);
    this.render();
  }

  get colW() { return (this.host.clientWidth - GAP * (COLS - 1)) / COLS; }

  /** Pixel boxes of every block and row. */
  geometry(rows) {
    const cw = this.colW;
    const g = { rows: [], rects: new Map(), height: 0 };
    let top = 0;
    for (const r of rows) {
      const auto = r.items.find((i) => this.blocks.get(i.id).autoHeight);
      const hh = auto ? this.blocks.get(auto.id).el.offsetHeight || unitPx(10) : unitPx(r.h);
      let x = 0;
      const items = r.items.map((it) => {
        const rect = { id: it.id, left: x * (cw + GAP), top, width: it.w * cw + (it.w - 1) * GAP, height: hh };
        x += it.w;
        g.rects.set(it.id, rect);
        return rect;
      });
      g.rows.push({ top, bottom: top + hh, items });
      top += hh + GAP;
    }
    g.height = Math.max(0, top - GAP);
    return g;
  }

  /** Whether the border after block i of a row can move. */
  canMove(row, i) {
    const a = row.items[i], b = row.items[i + 1];
    return !!b && a.w + b.w > this.minW(a.id) + this.minW(b.id) + 0.05;
  }

  render() {
    const rows = this.preview || this.visibleRows();
    const narrow = this.host.clientWidth < NARROW;
    this.host.classList.toggle('narrow', narrow);
    const shown = new Set(rows.flatMap((r) => r.items.map((i) => i.id)));
    for (const [id, el] of this.items) el.hidden = !shown.has(id);
    if (narrow) {
      // Stack in reading order; no dragging.
      let top = 0;
      for (const r of rows) {
        for (const it of r.items) {
          const el = this.items.get(it.id);
          const auto = this.blocks.get(it.id).autoHeight;
          el.style.transform = `translate(0px, ${top}px)`;
          el.style.width = '100%';
          el.style.height = auto ? '' : unitPx(r.h || this.blocks.get(it.id).h || 10) + 'px';
          top += el.offsetHeight + GAP;
        }
      }
      this.host.style.height = Math.max(0, top - GAP) + 'px';
      this.holder.hidden = true;
      return;
    }
    const g = this.geometry(rows);
    this.geo = g;
    rows.forEach((r, ri) => r.items.forEach((it, i) => {
      const edges = this.edges.get(it.id);
      edges.e.hidden = this.locked || !!this.dragId || !this.canMove(r, i);
      edges.s.hidden = this.locked || !!this.dragId || this.isAuto(r);
      if (it.id === this.dragId) return;
      const el = this.items.get(it.id);
      const p = g.rects.get(it.id);
      el.style.transform = `translate(${p.left}px, ${p.top}px)`;
      el.style.width = p.width + 'px';
      el.style.height = this.blocks.get(it.id).autoHeight ? '' : p.height + 'px';
    }));
    const hp = this.dragId && g.rects.get(this.dragId);
    this.holder.hidden = !hp;
    if (hp) {
      this.holder.style.transform = `translate(${hp.left}px, ${hp.top}px)`;
      this.holder.style.width = hp.width + 'px';
      this.holder.style.height = hp.height + 'px';
    }
    this.host.style.height = g.height + 'px';
  }

  /** Shows where a drop goes: a bar between blocks or rows, or a block
   *  highlighted to be replaced; null hides it. */
  showMark(m) {
    const mk = this.marker;
    mk.hidden = !m;
    if (!m) return;
    mk.className = 'gi-drop gi-drop-' + m.kind;
    mk.style.transform = `translate(${m.left}px, ${m.top}px)`;
    mk.style.width = m.width + 'px';
    mk.style.height = m.height + 'px';
    // A label on a bar at the left or right edge goes inside the board.
    const label = mk.firstChild;
    label.textContent = m.label;
    const edge = m.width < 20 && (m.left < 80 ? 'l' : m.left > this.host.clientWidth - 80 ? 'r' : '');
    label.style.left = edge === 'l' ? '12px' : edge === 'r' ? 'auto' : '';
    label.style.right = edge === 'r' ? '12px' : '';
    label.style.transform = edge ? 'translateY(-50%)' : '';
  }

  // ---- dragging ---------------------------------------------------------

  /** A press on an empty part of a block becomes a drag once it moves. */
  press(e, id) {
    if (e.button !== 0 || this.locked || this.host.classList.contains('narrow')) return;
    if (e.target.closest(NO_DRAG) || onScrollbar(e)) return;
    const sx = e.clientX, sy = e.clientY;
    const move = (ev) => {
      if (Math.hypot(ev.clientX - sx, ev.clientY - sy) < 5) return;
      stop();
      this.startDrag(e, id, ev);
    };
    const stop = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', stop);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', stop);
  }

  startDrag(e, id, firstMove) {
    const el = this.items.get(id);
    try { el.setPointerCapture(e.pointerId); } catch (_) { /* best effort */ }
    window.getSelection && window.getSelection().removeAllRanges();
    const rows = this.visibleRows();
    const ri = rows.findIndex((r) => r.items.some((i) => i.id === id));
    const src = rows[ri];
    const me = { id, w: src.items.find((i) => i.id === id).w, h: src.h || this.blocks.get(id).h || 10 };
    // The layout without the block: its row closes up (or goes away).
    const base = this.tidy(rows.map((r, i) => (i === ri ? { h: r.h, items: r.items.filter((x) => x.id !== id) } : r)));
    // Where it goes (rows and indexes in `base`): a new row before row `at`;
    // into row `row` before block `index` (join); or swapped with that block
    // (replace).
    const fitted = (r, items) => {
      const fit = fitRow(items, this.minW);
      if (!fit) return false;
      r.items = items.map((x, i) => ({ id: x.id, w: fit[i] }));
      return true;
    };
    const build = (t) => {
      const out = copy(base);
      if (t.type === 'new') {
        out.splice(t.at, 0, { h: me.h, items: [{ id, w: COLS }] });
        return this.tidy(out);
      }
      const r = out[t.row];
      if (!r) return null;
      if (t.type === 'join') {
        if (r.items.length >= 3) return null;
        // The moved block keeps its width if it can; the others share the rest.
        const want = me.w < COLS ? me.w : COLS / (r.items.length + 1);
        const items = r.items.map((x) => ({ id: x.id, w: (x.w * (COLS - want)) / COLS }));
        items.splice(t.index, 0, { id, w: want });
        return fitted(r, items) ? this.tidy(out) : null;
      }
      // Swap: the two trade places in the layout as it was; each slot keeps
      // its width and row height.
      const other = r.items[t.index].id;
      const swapped = copy(rows);
      for (const row of swapped) for (const x of row.items) x.id = x.id === id ? other : x.id === other ? id : x.id;
      return this.tidy(swapped);
    };
    const rowOf = (bid) => base.findIndex((r) => r.items.some((x) => x.id === bid));
    const unchanged = JSON.stringify(rows);
    // The layout stays as it is until the drop; this.geo holds its boxes.
    // Base index of a new row put just before row p of the layout.
    const newBefore = (p) => {
      const rs = this.geo.rows;
      if (p >= rs.length) return base.length;
      const o = rs[p].items.find((x) => x.id !== id);
      return o ? rowOf(o.id) : ri;
    };
    const hbar = (y) => ({ kind: 'line', left: 0, top: y - 3, width: this.host.clientWidth, height: 6, label: t('New row') });
    const vbar = (x, r) => ({ kind: 'line', left: Math.max(0, x - 3), top: r.top, width: 6, height: r.bottom - r.top, label: t('Side by side') });
    // The drop under the pointer (host coordinates): { next, mark } where
    // next is the resulting layout and mark what shows it; null for none.
    const dropAt = (lx, ly) => {
      const rs = this.geo.rows;
      let cands;
      if (!rs.length) return null;
      const p = rs.findIndex((r) => ly <= r.bottom);
      if (p < 0) {
        cands = [{ t: { type: 'new', at: base.length }, mark: hbar(rs[rs.length - 1].bottom + GAP / 2) }];
      } else if (ly < rs[p].top) {
        cands = [{ t: { type: 'new', at: newBefore(p) }, mark: hbar(p ? (rs[p - 1].bottom + rs[p].top) / 2 : rs[0].top - GAP / 2) }];
      } else {
        const r = rs[p];
        const over = r.items.find((x) => lx < x.left + x.width + GAP / 2) || r.items[r.items.length - 1];
        if (over.id === id) return null; // its own place
        const above = { t: { type: 'new', at: newBefore(p) }, mark: hbar(p ? (rs[p - 1].bottom + r.top) / 2 : r.top - GAP / 2) };
        const below = { t: { type: 'new', at: newBefore(p + 1) }, mark: hbar(r.bottom + GAP / 2) };
        const zone = Math.min(56, (r.bottom - r.top) * 0.2);
        if (ly < r.top + zone) cands = [above];
        else if (ly > r.bottom - zone) cands = [below];
        else {
          const b = rowOf(over.id);
          const index = base[b].items.findIndex((x) => x.id === over.id);
          const left = { t: { type: 'join', row: b, index }, mark: vbar(over.left - GAP / 2, r) };
          const right = { t: { type: 'join', row: b, index: index + 1 }, mark: vbar(over.left + over.width + GAP / 2, r) };
          const repl = { t: { type: 'replace', row: b, index }, mark: { kind: 'box', left: over.left, top: r.top, width: over.width, height: r.bottom - r.top, label: t('Swap') } };
          const f = (lx - over.left) / over.width;
          const row = ly < (r.top + r.bottom) / 2 ? above : below;
          cands = f < 0.3 ? [left, repl, row] : f > 0.7 ? [right, repl, row] : [repl, f < 0.5 ? left : right, row];
        }
      }
      for (const c of cands) {
        const next = build(c.t);
        if (next) return JSON.stringify(next) === unchanged ? null : { next, mark: c.mark };
      }
      return null;
    };

    // Where it was grabbed, on the block as shown (it may still be moving).
    const start = el.getBoundingClientRect();
    const offX = e.clientX - start.left, offY = e.clientY - start.top;
    // The picked-up block shrinks around the pointer so what is under it
    // stays visible.
    const scale = Math.max(0.25, Math.min(0.7, 420 / start.width, 300 / start.height));
    el.style.transformOrigin = `${offX}px ${offY}px`;
    const sc = this.host.closest('.main');
    let px = firstMove.clientX, py = firstMove.clientY;
    let drop = null;
    this.dragId = id;
    el.classList.add('dragging');
    document.body.classList.add('grid-dragging');
    this.render();

    const place = () => {
      const hr = this.host.getBoundingClientRect();
      const lx = px - hr.left, ly = py - hr.top;
      el.style.transform = `translate(${lx - offX}px, ${ly - offY}px) scale(${scale})`;
      drop = dropAt(lx, ly);
      this.showMark(drop && drop.mark);
    };
    let timer = 0;
    const tick = () => {
      if (sc && Math.hypot(px - e.clientX, py - e.clientY) > 30) {
        const r = sc.getBoundingClientRect();
        const v = py < r.top + 60 ? -16 : py > r.bottom - 60 ? 16 : 0;
        if (v) { sc.scrollTop += v; place(); }
      }
      timer = setTimeout(tick, 16);
    };
    tick();
    place();
    const move = (ev) => { px = ev.clientX; py = ev.clientY; place(); };
    const end = () => {
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', end);
      el.removeEventListener('pointercancel', end);
      clearTimeout(timer);
      el.classList.remove('dragging');
      document.body.classList.remove('grid-dragging');
      this.dragId = null;
      this.showMark(null);
      if (drop) this.commit(drop.next);
      else this.render();
      // Grow back from the pointer only after it has been placed.
      setTimeout(() => { el.style.transformOrigin = ''; }, 260);
    };
    el.addEventListener('pointermove', move);
    el.addEventListener('pointerup', end);
    el.addEventListener('pointercancel', end);
  }

  /**
   * Border resize: the right border moves the border shared with the next
   * block of the row (both change width); the bottom border sets the row
   * height. Both follow the pointer within the blocks' minimums; a border
   * catches at thirds and halves of the row.
   */
  startResize(e, id, handle, dir) {
    if (e.button !== 0 || this.locked || this.host.classList.contains('narrow')) return;
    e.preventDefault();
    e.stopPropagation();
    try { handle.setPointerCapture(e.pointerId); } catch (_) { /* best effort */ }
    const el = this.items.get(id);
    const rows = this.visibleRows();
    const ri = rows.findIndex((r) => r.items.some((i) => i.id === id));
    const i = rows[ri].items.findIndex((x) => x.id === id);
    const top = this.geo.rows[ri].top;
    this.preview = rows;
    el.classList.add('resizing');
    document.body.classList.add('grid-resizing', 'resize-' + dir);
    const move = (ev) => {
      const hr = this.host.getBoundingClientRect();
      const lx = ev.clientX - hr.left, ly = ev.clientY - hr.top;
      const next = copy(this.preview);
      const row = next[ri];
      if (dir === 'e') {
        const a = row.items[i], b = row.items[i + 1];
        const sum = a.w + b.w;
        const left = row.items.slice(0, i).reduce((n, x) => n + x.w, 0);
        const step = this.colW + GAP;
        let col = (lx + GAP / 2) / step; // pointer, in columns
        const snap = SNAP_W.find((c) => Math.abs(c - col) * step < SNAP_PX);
        if (snap !== undefined) col = snap;
        a.w = clamp(col - left, this.minW(a.id), sum - this.minW(b.id));
        b.w = sum - a.w;
      } else {
        row.h = clamp((ly - top + GAP) / (ROW + GAP), this.minH(row), MAX_H);
      }
      this.preview = next;
      this.render();
    };
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('pointercancel', end);
      el.classList.remove('resizing');
      document.body.classList.remove('grid-resizing', 'resize-' + dir);
      const done = this.preview;
      this.preview = null;
      this.commit(done);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
  }

  /** Stores the visible rows (hidden blocks keep a row where they were). */
  commit(rows) {
    const before = JSON.stringify(this.current());
    this.rows = this.withHidden(rows);
    this.render();
    if (JSON.stringify(this.current()) !== before) this.onChange(this.current());
  }
}
