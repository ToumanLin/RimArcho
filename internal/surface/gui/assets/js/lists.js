// The three lists (06 §5): virtual rows, the cursor, selection, search, dragging.
import { S, $, $$, el, esc, col, cur, key, api, T, Tf, AREAS, ROW_H } from "./state.js";
import { sourceIcon, statusIcon } from "./icons.js";

const areas = () => $$("#board .area");
const rowsEl = (k) => $(`.area[data-area="${k}"] .rows`);

// ---- building the views ----

// setItems replaces a column's rows and keeps the cursor on the same mod.
export function setItems(k, items) {
  const c = S.cols[k];
  const keep = cur(k);
  c.items = items;
  const alive = new Set(items.map(key));
  for (const x of [...c.sel]) if (!alive.has(x)) c.sel.delete(x);
  return keep ? key(keep) : "";
}

// refilter applies the column's query (view.Query in Go) and keeps the cursor on keepKey.
export async function refilter(k, keepKey) {
  const c = S.cols[k];
  if (!c.query.trim()) {
    c.view = c.items.map((_, i) => i);
    c.others = null;
  } else {
    const r = await api.Filter(k, c.query);
    c.view = r.rows || [];
    c.others = r.counts;
  }
  c.cursor = Math.max(0, Math.min(c.cursor, c.view.length - 1));
  if (keepKey) {
    const i = c.view.findIndex((vi) => key(c.items[vi]) === keepKey);
    if (i >= 0) c.cursor = i;
  }
}

// ---- drawing ----

export function renderHeads() {
  for (const k of [0, 1, 2, 3]) {
    const a = $(`.area[data-area="${k}"]`);
    a.classList.toggle("focus", S.focus === k && !S.popup);
    const h = $(".head", a);
    if (k === 3) {
      h.innerHTML = `<span class="n">4</span><span class="t">${esc(T(AREAS[3]))}</span>`;
      continue;
    }
    const c = S.cols[k];
    const n = c.query.trim() ? `${c.view.length}/${c.items.length}` : `${c.items.length}`;
    h.innerHTML = `<span class="n">${k + 1}</span><span class="t">${esc(T(AREAS[k]))}</span><span class="num">${n}</span>` +
      (c.query && !c.searching ? `<span class="q">${esc(c.query)}</span>` : "");
    const s = $(".search", a);
    s.classList.toggle("on", c.searching || !!c.query);
    let o = $(".others", a);
    if (c.searching && c.others) {
      if (!o) { o = el("div", "others"); s.after(o); }
      const parts = [0, 1, 2].filter((x) => x !== k).map((x) => `${T(AREAS[x])} ${c.others[x]}`);
      o.textContent = Tf("other lists: %s · tab goes to the next list with matches", parts.join(" · "));
    } else if (o) o.remove();
  }
}

const pools = [new Map(), new Map(), new Map()];

// renderRows draws the visible rows of one list (virtual: only what is on screen).
export function renderRows(k) {
  const box = rowsEl(k);
  if (!box) return;
  const c = S.cols[k];
  let pad = box.firstElementChild;
  if (!pad || !pad.classList.contains("pad")) {
    box.innerHTML = "";
    pad = el("div", "pad");
    box.appendChild(pad);
    pools[k].clear();
  }
  pad.style.height = c.view.length * ROW_H + "px";
  let empty = $(".empty", box);
  if (!c.view.length) {
    if (!empty) box.appendChild(empty = el("div", "empty"));
    empty.textContent = c.query ? T("nothing matches") : [T("every installed mod is enabled"), T("nothing enabled yet"), T("staging is empty · t puts the selected mods here")][k];
    // Staging is empty: its place shows the history (08 §14).
    if (k === 2 && !c.query && S.history && S.history.length > 1) empty.append(historyPanel());
  } else if (empty) empty.remove();
  const first = Math.max(0, Math.floor(box.scrollTop / ROW_H) - 4);
  const last = Math.min(c.view.length, first + Math.ceil(box.clientHeight / ROW_H) + 8);
  const pool = pools[k];
  for (const [i, d] of pool) if (i < first || i >= last) { d.remove(); pool.delete(i); }
  for (let i = first; i < last; i++) {
    const it = c.items[c.view[i]];
    let d = pool.get(i);
    const sig = rowSig(k, i, it);
    if (d && d.dataset.sig === sig) continue;
    if (!d) { d = el("div", "row"); box.appendChild(d); pool.set(i, d); }
    d.dataset.sig = sig;
    d.dataset.i = i;
    d.style.top = i * ROW_H + "px";
    d.draggable = !!it.id || k === 1;
    const st = S.board && S.board.status ? rowStatus(it) : 0;
    d.className = "row" + (i === c.cursor ? " cur" : "") + (c.sel.has(key(it)) ? " sel" : "") + (it.dim || it.missing ? " dimmed" : "");
    const tags = it.tags && it.tags.length ? `<span class="g">${esc(it.tags.join(" · "))}</span>` : "";
    d.innerHTML = `<span class="m"></span>` + (k === 1 ? `<span class="p">${it.pos || ""}</span>` : "") +
      `<span class="k">${it.missing ? "" : sourceIcon(it.kind)}</span><span class="t">${esc(it.name)}</span>${tags}` +
      (it.unsaved ? `<span class="u" title="${esc(T("not saved yet"))}"></span>` : "") + `<span class="s">${statusIcon(st)}</span>`;
    d.title = it.name;
  }
}

// onHistory runs when a history row is clicked (main.js sets it).
export let onHistory = () => {};
export function setOnHistory(fn) { onHistory = fn; }

// historyPanel: unsaved steps around "now", then the saved versions (08 §14).
function historyPanel() {
  const p = el("div", "hist");
  p.append(el("div", "hh", T("history")));
  let versions = false;
  for (const h of S.history) {
    if (h.kind === "version" && !versions) {
      versions = true;
      p.append(el("div", "hh", T("saved versions")));
    }
    const r = el("div", "h " + h.kind, h.text);
    if (h.kind !== "now") {
      r.title = h.kind === "version" ? T("load this version into the list") : T("go back to here");
      r.onclick = (e) => { e.stopPropagation(); onHistory(h); };
    }
    p.append(r);
  }
  return p;
}

function rowSig(k, i, it) {
  const c = S.cols[k];
  return [key(it), it.pos, it.unsaved, it.dim, (it.tags || []).join(","), i === c.cursor, c.sel.has(key(it)), rowStatus(it), it.name].join("|");
}

// rowStatus mirrors view.RowStatus (missing entries are errors, unloadable rows dim).
export function rowStatus(it) {
  let s = (S.board && S.board.status && S.board.status[it.id]) || 0;
  if (it.missing) s = 4;
  if (!s && it.dim) s = 1;
  return s;
}

export function renderLists() {
  renderHeads();
  for (const k of [0, 1, 2]) renderRows(k);
}

// ensureVisible scrolls a list so its cursor row shows.
export function ensureVisible(k = S.focus) {
  const box = rowsEl(k);
  if (!box) return;
  const c = S.cols[k];
  const y = c.cursor * ROW_H;
  if (y < box.scrollTop) box.scrollTop = y;
  else if (y + ROW_H > box.scrollTop + box.clientHeight) box.scrollTop = y + ROW_H - box.clientHeight;
}

// ---- wiring: scrolling, clicks, drags ----

export function wireLists(ui) {
  for (const k of [0, 1, 2]) {
    const box = rowsEl(k);
    box.addEventListener("scroll", () => renderRows(k), { passive: true });
    new ResizeObserver(() => renderRows(k)).observe(box);
    box.addEventListener("mousedown", (e) => {
      const r = e.target.closest(".row");
      if (!r || e.button !== 0) return;
      ui.click(k, +r.dataset.i, e);
    });
    box.addEventListener("dblclick", (e) => {
      const r = e.target.closest(".row");
      if (r) ui.verb("toggle");
    });
    box.addEventListener("contextmenu", (e) => {
      e.preventDefault();
      const r = e.target.closest(".row");
      if (!r) return;
      const i = +r.dataset.i;
      const c = S.cols[k];
      if (S.focus !== k || !c.sel.has(key(c.items[c.view[i]]))) ui.click(k, i, { shiftKey: false, ctrlKey: false });
      ui.menu(e.clientX, e.clientY);
    });
    wireDrag(ui, k, box);
    const input = $(`.area[data-area="${k}"] .search input`);
    input.placeholder = T("search: words, tag:, author:, type:, has:, -not");
    input.addEventListener("input", () => ui.search(k, input.value));
    input.addEventListener("keydown", (e) => ui.searchKey(k, e, input));
    input.addEventListener("blur", () => { S.cols[k].searching = false; renderHeads(); });
  }
}

// Dragging is another way to run a verb (08 §6): it acts on drop and u undoes it.
let drag = null;
const tip = el("div", "drop-tip");

function wireDrag(ui, k, box) {
  box.addEventListener("dragstart", (e) => {
    const r = e.target.closest(".row");
    if (!r) return;
    const c = S.cols[k];
    const it = c.items[c.view[+r.dataset.i]];
    if (!c.sel.has(key(it)) || S.focus !== k) ui.click(k, +r.dataset.i, { shiftKey: false, ctrlKey: false });
    const its = ui.selectionItems();
    drag = { from: k, items: its };
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", its.map((x) => x.name).join("\n"));
  });
  box.addEventListener("dragend", () => { drag = null; clearDrop(); });
  const area = box.closest(".area");
  area.addEventListener("dragover", (e) => {
    if (!drag) return;
    e.preventDefault();
    clearDrop();
    area.classList.add("drop-target");
    const t = dropTarget(k, box, e);
    if (t.row) t.row.classList.add(t.before ? "drop-before" : "drop-after");
    tip.textContent = t.label;
    tip.style.left = e.clientX + 14 + "px";
    tip.style.top = e.clientY + 10 + "px";
    if (!tip.isConnected) document.body.appendChild(tip);
  });
  area.addEventListener("dragleave", (e) => { if (!area.contains(e.relatedTarget)) clearDrop(); });
  area.addEventListener("drop", (e) => {
    if (!drag) return;
    e.preventDefault();
    const t = dropTarget(k, box, e);
    const d = drag;
    drag = null;
    clearDrop();
    ui.drop(d.from, k, d.items, t.at);
  });
}

function clearDrop() {
  $$(".drop-before, .drop-after, .drop-target").forEach((x) => x.classList.remove("drop-before", "drop-after", "drop-target"));
  tip.remove();
}

// dropTarget: in Enabled the place between two rows ("position N" in the list without the
// moving mods); elsewhere the list itself.
function dropTarget(k, box, e) {
  const label = [T("disable"), "", T("stage")][k];
  if (k !== 1) return { label: drag.from === k ? "" : label };
  const c = S.cols[1];
  const y = e.clientY - box.getBoundingClientRect().top + box.scrollTop;
  let i = Math.max(0, Math.min(Math.round(y / ROW_H), c.view.length));
  const moving = new Set(drag.items.map(key));
  // the list position before view row i
  const posAt = (vi) => c.items[c.view[vi]].pos;
  let p = i < c.view.length ? posAt(i) : (c.view.length ? posAt(c.view.length - 1) + 1 : 1);
  const before = c.items.filter((x) => moving.has(key(x)) && x.pos && x.pos < p).length;
  const at = Math.max(1, p - before);
  const rowEl = box.querySelector(`.row[data-i="${i < c.view.length ? i : c.view.length - 1}"]`);
  return { at, row: rowEl, before: i < c.view.length, label: Tf("position %d", at) };
}
