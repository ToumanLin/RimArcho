// Shared headers and saved columns for data tables (16 §3.9).
import { el, api, T, Tf } from "./state.js";
import * as pop from "./popups.js";

export function mergeColumns(defs, saved) {
  let keep;
  try { keep = JSON.parse(saved || "[]"); } catch { keep = []; }
  if (!Array.isArray(keep)) keep = [];
  const by = new Map(defs.map((d) => [d.key, d])), out = [];
  for (const old of keep) {
    if (!old || typeof old !== "object") continue;
    const d = by.get(old.key || old.k);
    if (!d) continue;
    by.delete(d.key);
    const w = Number(old.w);
    out.push({ key: d.key, shown: !!d.fixed || ("shown" in old ? !!old.shown : !old.h),
      w: Number.isFinite(w) && w > 0 ? Math.max(d.key === "name" ? 120 : 40, Math.min(2000, w)) : d.w });
  }
  for (const d of by.values()) out.push({ key: d.key, shown: !!d.fixed || d.shown !== false, w: d.w });
  return out;
}

export function createTable({ key, columns, state = { cols: null, sort: "name", dir: 1 }, redraw, sort, focus }) {
  let root, head, reading;
  const defs = () => typeof columns === "function" ? columns() : columns;
  const def = (k) => defs().find((d) => d.key === k);
  const cols = () => state.cols || (state.cols = mergeColumns(defs(), ""));
  const visible = () => cols().filter((c) => c.shown);
  const save = () => api.SetSetting(key, JSON.stringify(cols())).catch(() => {});
  const changed = () => { save(); redraw(); };
  const focusTable = () => focus ? focus() : root.focus({ preventScroll: true });
  function layout() {
    const cs = visible();
    root.style.setProperty("--lib-cols", cs.map((c) => c.key === "name" ? `minmax(${c.w}px, 1fr)` : c.w + "px").join(" "));
    root.style.setProperty("--lib-minw", (cs.reduce((n, c) => n + c.w, 0) + 10 * Math.max(0, cs.length - 1) + 24) + "px");
  }
  function sortBy(k) {
    if (!def(k)) return;
    state.dir = state.sort === k ? -state.dir : 1;
    state.sort = k;
    if (sort) sort(); else redraw();
  }
  function size(k, delta) {
    const c = cols().find((c) => c.key === k);
    if (!c) return;
    c.w = delta === 0 ? def(k).w : Math.max(k === "name" ? 120 : 40, Math.min(2000, c.w + delta));
    changed();
  }
  function move(k, target, after = false) {
    const c = cols().find((c) => c.key === k), to = cols().find((c) => c.key === target);
    if (!c || !to || c === to) return;
    state.cols.splice(state.cols.indexOf(c), 1);
    state.cols.splice(state.cols.indexOf(to) + (after ? 1 : 0), 0, c);
    changed();
  }
  function menu(x, y, k = state.sort, keys = false) {
    const d = def(k);
    if (!d) return;
    const shift = (delta) => { const cs = visible(), to = cs[cs.findIndex((c) => c.key === k) + delta]; if (to) move(k, to.key, delta > 0); };
    const sortable = [{ key: k }, ...visible().filter((c) => c.key !== k)];
    const acts = sortable.map((c) => ({ label: Tf("sort by %s", T(def(c.key).title)),
      key: state.sort === c.key ? (state.dir * def(c.key).dir > 0 ? "▲" : "▼") : "", run: () => sortBy(c.key) }));
    acts.push({ sep: true },
      { label: Tf("move %s left", T(d.title)), run: () => shift(-1) },
      { label: Tf("move %s right", T(d.title)), run: () => shift(1) },
      { label: Tf("widen %s", T(d.title)), key: ">", run: () => size(k, 24) },
      { label: Tf("narrow %s", T(d.title)), key: "<", run: () => size(k, -24) },
      { label: Tf("reset width: %s", T(d.title)), key: "=", run: () => size(k, 0) }, { sep: true });
    for (const c of cols()) if (!def(c.key).fixed) acts.push({ label: T(def(c.key).title), key: c.shown ? "✓" : "", checked: c.shown, run: () => { c.shown = !c.shown; changed(); } });
    acts.push({ sep: true }, { label: T("reset the columns"), run: () => { state.cols = mergeColumns(defs(), ""); changed(); } });
    if (x == null) { const b = (head.querySelector(".th.on") || head).getBoundingClientRect(); x = b.left; y = b.bottom; }
    // the header opens it; esc goes back to the rows, where the keys work (16 §3.9)
    pop.menu(x, y, acts, keys, { name: T(d.title), opener: head.querySelector(`[data-k="${k}"]`) || root, back: focusTable });
  }
  function draw(box, th, options = {}) {
    root = box; head = th;
    root.setAttribute("role", options.role || root.getAttribute("role") || "table");
    // A table is named by its own visible heading; a page's only table needs none (the main region is named).
    if (options.labelledby) root.setAttribute("aria-labelledby", options.labelledby);
    if (options.multiselectable) root.setAttribute("aria-multiselectable", "true");
    if (options.count != null) count(options.count);
    root.setAttribute("aria-colcount", String(visible().length));
    head.setAttribute("role", "row");
    head.setAttribute("aria-rowindex", "1");
    layout();
    head.innerHTML = "";
    head.title = T("drag to move, right click: columns");
    head.dataset.columns = JSON.stringify([state.sort, state.dir, visible().map((c) => [c.key, T(def(c.key).title)])]);
    for (const c of visible()) {
      const d = def(c.key), active = state.sort === c.key;
      const cell = el("div", "th " + (d.cls || "") + (active ? " on" : "") + (d.num ? " r" : ""));
      cell.dataset.k = c.key;
      cell.id = key + "-column-" + c.key;
      cell.setAttribute("role", "columnheader");
      cell.tabIndex = -1;
      pop.menuOpener(cell);
      cell.setAttribute("aria-colindex", String(visible().indexOf(c) + 1));
      cell.setAttribute("aria-sort", active ? (state.dir * d.dir > 0 ? "ascending" : "descending") : "none");
      cell.append(el("span", "lbl", T(d.title)));
      if (active) {
        const arrow = el("span", "arrow", state.dir * d.dir > 0 ? "▲" : "▼");
        arrow.setAttribute("aria-hidden", "true");
        cell.append(arrow);
      }
      const rz = el("span", "rz");
      rz.ondblclick = (e) => { e.preventDefault(); e.stopPropagation(); size(c.key, 0); };
      cell.append(rz);
      cell.oncontextmenu = (e) => { e.preventDefault(); e.stopPropagation(); focusTable(); menu(e.clientX, e.clientY, c.key); };
      cell.onpointerdown = (e) => {
        if (e.button !== 0) return;
        focusTable();
        e.preventDefault();
        const c = cols().find((col) => col.key === d.key);
        const resizing = e.target === rz, x0 = e.clientX, w0 = c.w;
        let moving = false, target, after;
        // Keep resize clicks targeted at the edge so its double click can restore width (16 §3.9).
        (resizing ? rz : cell).setPointerCapture(e.pointerId);
        if (resizing) document.body.classList.add("col-resizing");
        const clear = () => {
          cell.onpointermove = cell.onpointerup = cell.onpointercancel = null;
          document.body.classList.remove("col-resizing");
          for (const t of head.querySelectorAll(".th")) t.classList.remove("dragging", "drop-before", "drop-after");
        };
        cell.onpointermove = (m) => {
          if (resizing) { c.w = Math.max(c.key === "name" ? 120 : 40, Math.min(2000, Math.round(w0 + m.clientX - x0))); layout(); return; }
          if (!moving && Math.abs(m.clientX - x0) < 5) return;
          moving = true;
          const cells = [...head.querySelectorAll(".th")];
          for (const t of cells) t.classList.remove("drop-before", "drop-after");
          cell.classList.add("dragging");
          const over = cells.find((t) => m.clientX < t.getBoundingClientRect().right) || cells.at(-1);
          target = over.dataset.k;
          const b = over.getBoundingClientRect(); after = m.clientX > b.left + b.width / 2;
          over.classList.add(after ? "drop-after" : "drop-before");
        };
        cell.onpointerup = () => { clear(); if (resizing) save(); else if (moving) move(c.key, target, after); else sortBy(c.key); };
        cell.onpointercancel = () => { if (resizing) { c.w = w0; layout(); } clear(); };
      };
      head.append(cell);
    }
    head.oncontextmenu = (e) => { e.preventDefault(); focusTable(); menu(e.clientX, e.clientY); };
  }
  function count(n) { root.setAttribute("aria-rowcount", String(n + 1)); }
  function row(node, index, identity) {
    if (identity != null) node.id = identity;
    node.setAttribute("role", "row");
    node.setAttribute("aria-rowindex", String(index + 2));
    const grid = ["grid", "treegrid"].includes(root.getAttribute("role"));
    [...node.children].forEach((cell, i) => {
      cell.setAttribute("role", grid ? "gridcell" : "cell");
      cell.setAttribute("aria-colindex", String(i + 1));
      if (cell.title === cell.textContent) cell.setAttribute("aria-description", "");
    });
    return node;
  }
  return {
    state, visible, draw, row, count, menu, size, move, sortBy, column: def,
    adopt(box, th) { root = box; head = th; layout(); },
    async load() {
      if (!reading) reading = api.Setting(key).catch(() => "").then((saved) => { state.cols = mergeColumns(defs(), saved); });
      await reading;
    },
    key(e) {
      if (!head || !root || root.hidden) return false;
      if (e.ctrlKey || e.metaKey || e.altKey) return false;
      if (e.key === "s") { e.preventDefault(); menu(undefined, undefined, state.sort, true); return true; }
      if (["<", ">", "="].includes(e.key)) { e.preventDefault(); size(state.sort, e.key === "=" ? 0 : e.key === ">" ? 24 : -24); return true; }
      return false;
    },
    sorted(rows) {
      const d = def(state.sort);
      return [...rows].sort((a, b) => Number(!!a.pending) - Number(!!b.pending) ||
        d.cmp(a, b) * d.dir * state.dir || (a.name || "").localeCompare(b.name || ""));
    },
  };
}
