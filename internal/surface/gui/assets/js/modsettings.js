// Mod library › Mod settings (16 §2.9 "Mod settings"): the current list's Mods that have
// settings, one row per settings file or HugsLib pack (its window's title, how many values
// it stores, its size), and on the right what the cursor's file stores. On/off, numbers, text
// and HugsLib choices change there while no game runs; the rest in the game. The table
// follows 16 §3.9; the gap between the cards drags (16 §3.1).
import { pathContent } from "./fields.js";
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { beginPageRead, page, area, act } from "./screens.js";
import { createTable } from "./datatable.js";
import { modSettingsColumns } from "./tabledefs.js";
import { wireSeparator } from "./separator.js";

// page: the table's read; rows: its rows as sorted; at: the cursor's file (its key), kept
// across reads and sorts; detail: the values on the right (for detail.key); seq: the newest
// values read (an older reply is dropped, 16 §3.1 rule 9).
const X = { cols: null, sort: "name", dir: 1, page: null, rows: [], at: "", detail: null, seq: 0 };
let tableCard = null, detailCard = null, rowEls = [], timer = 0, wanted = "";

// focusMod puts the cursor on a Mod's first file when the page opens next (a detail's
// settings row, 16 §4.1).
export function focusMod(instance) { wanted = instance || ""; }

const table = createTable({ key: "ui.gui.modsettings.columns", columns: modSettingsColumns, state: X,
  redraw: () => drawRows(), focus: () => focusTable(),
});

export async function open() {
  const read = beginPageRead("mod-settings", { retry: open });
  let p;
  try { [p] = await Promise.all([api.ModSettings(), table.load()]); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  X.page = p;
  p.rows ||= [];
  if (wanted) {
    const r = table.sorted(p.rows).find((x) => x.instance === wanted);
    if (r) X.at = r.key;
    wanted = "";
  }
  if (!p.rows.some((r) => r.key === X.at)) { X.at = ""; X.detail = null; }
  build();
  read.finish();
  showDetail(0);
}

function build() {
  const p = X.page;
  const sc = $("#screen");
  sc.innerHTML = "";
  const mods = new Set(p.rows.map((r) => r.instance)).size;
  const lead = p.rows.length ? Tf("%d mods of %s have settings", mods, p.plan) : "";
  const w = page("fill modsettings", lead, act(T("open the Config folder"), "export", () => openFolder(), "ctrl+o"));
  tableCard = detailCard = null; rowEls = [];
  if (!p.rows.length) {
    const empty = el("section", "card empty-state");
    empty.append(el("p", "", T("no Mod of this list has a settings file, and Archo Monitor recorded no settings window yet")),
      el("p", "muted", T("a Mod's settings file appears once its settings are changed in the game")));
    w.append(empty);
    sc.append(w);
    return;
  }
  const grid = el("div", "ms");
  tableCard = el("section", "card data-table ms-table");
  tableCard.tabIndex = 0;
  const gap = el("div", "split ms-split");
  detailCard = el("section", "card ms-detail");
  grid.append(tableCard, gap, detailCard);
  w.append(grid);
  sc.append(w);
  const setSide = (v) => { if (v) grid.style.setProperty("--ms-side", v + "px"); else grid.style.removeProperty("--ms-side"); };
  const range = () => [320, Math.max(320, grid.clientWidth - 300)];
  wireSeparator(gap, {
    key: "ui.gui.modsettings.side", range,
    get: () => tableCard.getBoundingClientRect().width,
    set: setSide, reset: () => setSide(0),
    restore: (v) => setSide(+v ? Math.max(range()[0], Math.min(range()[1], +v)) : 0),
  });
  area(tableCard, null, () => focusTable());
  area(detailCard, null, () => detailCard.querySelector(".ms-body")?.focus({ preventScroll: true }));
  drawRows();
  drawDetail(X.detail);
}

function focusTable() { tableCard?.focus({ preventScroll: true }); }

function drawRows() {
  if (!tableCard || !X.page) return;
  X.rows = table.sorted(X.page.rows);
  if (!X.rows.some((r) => r.key === X.at)) X.at = X.rows[0]?.key || "";
  tableCard.innerHTML = "";
  const head = el("div", "lib-th");
  tableCard.append(head);
  table.draw(tableCard, head, { role: "grid", count: X.rows.length });
  rowEls = [];
  X.rows.forEach((r, i) => {
    const row = el("div", "data-row" + (r.key === X.at ? " cur" : "") + (r.exists ? "" : " ms-defaults"));
    row.dataset.key = r.key;
    for (const c of table.visible()) {
      const cell = el("span", "cell" + (c.key === "name" ? " n" : "") + (table.column(c.key).num ? " num" : ""), String(r[c.key] ?? "—"));
      cell.title = cell.textContent;
      row.append(cell);
    }
    row.setAttribute("aria-selected", String(r.key === X.at));
    row.onclick = () => { focusTable(); moveTo(i); };
    table.row(row, i);
    rowEls.push(row);
    tableCard.append(row);
  });
  rowEls[X.rows.findIndex((r) => r.key === X.at)]?.scrollIntoView({ block: "nearest" });
}

// moveTo puts the cursor on row i; the values follow once it rests (a held arrow reads
// only where it stops).
function moveTo(i) {
  if (!X.rows.length) return;
  i = Math.max(0, Math.min(X.rows.length - 1, i));
  const key = X.rows[i].key;
  if (key === X.at) return;
  X.at = key;
  for (const row of rowEls) {
    const on = row.dataset.key === key;
    row.classList.toggle("cur", on);
    row.setAttribute("aria-selected", String(on));
  }
  rowEls[i]?.scrollIntoView({ block: "nearest" });
  showDetail(80);
}

function showDetail(delay) {
  clearTimeout(timer);
  const r = X.rows.find((x) => x.key === X.at);
  if (!r) { drawDetail(null); return; }
  if (X.detail?.key !== r.key) {
    X.detail = { key: r.key, name: r.name, title: r.title === "—" ? "" : r.title, file: r.file, loading: true, values: [], notes: [] };
    drawDetail(X.detail);
  }
  const n = ++X.seq;
  timer = setTimeout(async () => {
    let d;
    try { d = await api.ModSettingsFile(r.instance, r.class); } catch (e) { d = { key: r.key, name: r.name, file: r.file, error: errText(e), values: [], notes: [] }; }
    if (n !== X.seq || S.screen !== "mod-settings" || X.at !== r.key) return;
    const same = X.detail?.key === d.key && !X.detail.loading;
    X.detail = d;
    drawDetail(d, same);
  }, delay);
}

// drawDetail draws the values into the same card (its area number stays); keep: the same
// file read again keeps its scroll.
function drawDetail(d, keep = false) {
  if (!detailCard) return;
  const top = keep ? detailCard.querySelector(".ms-body")?.scrollTop || 0 : 0;
  const focused = detailCard.contains(document.activeElement);
  detailCard.replaceChildren();
  if (!d) { detailCard.append(el("div", "empty-state", T("select a settings file"))); return; }
  const head = el("div", "ms-dhead");
  head.append(el("h2", "word-facts", d.name));
  const facts = el("div", "facts");
  if (d.kind === "hugslib") facts.append(el("span", "word-facts", d.title ? Tf("HugsLib: %s", d.title) : "HugsLib"));
  else if (d.title) facts.append(el("span", "word-facts", Tf("settings window: %s", d.title)));
  if (d.type) facts.append(el("span", "ms-type", d.type));
  if (d.stored) facts.append(el("span", "", Tf("%d stored values", d.stored)));
  if (facts.childNodes.length) head.append(facts);
  head.append(d.path ? pathContent(el("div", "ms-path"), d.path, [d.path]) : el("div", "ms-path muted", d.file || ""));
  const body = el("div", "ms-body");
  body.tabIndex = 0;
  body.setAttribute("aria-label", T("stored values"));
  if (d.loading) body.append(el("p", "ms-note muted", T("reading…")));
  if (d.error) body.append(el("p", "ms-note err", d.error));
  // the game's window as Archo Monitor saw it, then what the file stores
  if (d.window) {
    body.append(el("h3", "ms-sub", Tf("in the game's window (%s)", d.window.seen)));
    const kv = el("dl", "ms-values ms-window");
    for (const c of d.window.controls) {
      if (c.kind === "label") { kv.append(el("dt", "ms-head", c.label)); continue; }
      const dt = el("dt", "", c.kind === "button" ? "" : c.label || "—");
      const dd = el("dd", "ms-v " + c.kind, c.kind === "button" ? "[" + c.label + "]" : c.text);
      dd.title = dd.textContent;
      kv.append(dt, dd);
    }
    body.append(kv);
    if (!d.window.full) body.append(el("p", "ms-note muted", T("the window draws more than Archo Monitor keeps")));
    if (d.values?.length || d.exists) body.append(el("h3", "ms-sub", T("stored in the file")));
  }
  const editing = d.editable && !d.loading && !X.page?.locked;
  if (d.editable && X.page?.locked) body.append(el("p", "ms-note ms-locked", X.page.locked));
  if (d.values?.length) {
    const kv = el("dl", "ms-values");
    for (const v of d.values) {
      const dd = el("dd", "ms-v " + v.kind + (v.fixed ? " fixed" : "") + (v.atDefault ? " at-default" : ""));
      if (editing && v.edit) dd.append(editor(d, v));
      else dd.append(document.createTextNode(v.text));
      if (v.default) dd.append(el("span", "ms-def", Tf("default %s", v.default)));
      if (editing && v.edit && v.stored) dd.append(resetButton(d, v));
      dd.title = v.text + (v.default ? "  " + Tf("default %s", v.default) : "");
      const dt = el("dt", v.name ? "ms-title" : "", v.key);
      dt.title = [v.name, v.help].filter(Boolean).join("\n") || v.key;
      kv.append(dt, dd);
    }
    body.append(kv);
  }
  for (const n of d.notes || []) body.append(el("p", "ms-note muted", n));
  detailCard.append(head, body);
  body.scrollTop = top;
  if (focused) body.focus({ preventScroll: true });
}

// editor is a value's control: a switch for on/off, a list for a choice, else the value as a
// button that turns into a field (Enter saves, Esc leaves it).
function editor(d, v) {
  const name = v.name || v.key;
  const label = Tf("change %s", v.key);
  if (v.edit === "bool") {
    const on = v.value.toLowerCase() === "true";
    const b = el("button", "ms-edit ms-switch" + (on ? " on" : ""), v.text);
    b.type = "button";
    b.setAttribute("role", "switch");
    b.setAttribute("aria-checked", String(on));
    b.setAttribute("aria-label", v.key);
    b.dataset.name = name;
    b.onclick = () => change(d, v, on ? "False" : "True");
    return b;
  }
  if (v.edit === "choice") {
    const s = el("select", "ms-edit ms-choice");
    s.setAttribute("aria-label", v.key);
    s.dataset.name = name;
    for (const c of v.choices || []) {
      const o = el("option", "", c);
      o.value = c;
      s.append(o);
    }
    s.value = v.value;
    s.onchange = () => change(d, v, s.value);
    return s;
  }
  const b = el("button", "ms-edit ms-field", v.text);
  b.type = "button";
  b.title = label;
  b.setAttribute("aria-label", label);
  b.dataset.name = name;
  b.onclick = () => {
    const input = el("input", "ms-input");
    input.type = "text";
    input.inputMode = v.edit === "text" ? "text" : "decimal";
    input.value = v.value;
    input.setAttribute("aria-label", v.key);
    input.dataset.name = name;
    let done = false;
    const leave = () => { if (!done) { done = true; input.replaceWith(b); b.focus({ preventScroll: true }); } };
    input.onkeydown = (e) => {
      if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); done = true; change(d, v, input.value); }
      else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); leave(); }
    };
    input.onblur = () => leave();
    b.replaceWith(input);
    input.focus();
    input.select();
  };
  return b;
}

function resetButton(d, v) {
  const b = el("button", "ms-reset", T("reset"));
  b.type = "button";
  b.title = T("remove the stored value: the Mod uses its default");
  b.setAttribute("aria-label", Tf("reset %s to its default", v.key));
  b.onclick = () => change(d, v, "", true);
  return b;
}

// change writes one value (16 §2.9): the reply's file replaces the values, and the row's
// count follows; a refusal reads the file again.
async function change(d, v, value, reset = false) {
  const r = X.rows.find((x) => x.key === d.key);
  if (!r) return;
  const name = v.name || v.key;
  let res;
  try {
    res = await api.SetModSetting(r.instance, r.class, name, value, v.stored ? v.value : "", reset);
  } catch (e) {
    say("fail", errText(e));
    X.detail = null;
    showDetail(0);
    return;
  }
  say("ok", res.message, false, res.backup ? [res.backup] : []);
  if (res.detail && X.at === d.key) {
    X.detail = res.detail;
    drawDetail(res.detail, true);
    detailCard?.querySelector(`[data-name="${CSS.escape(name)}"]`)?.focus({ preventScroll: true });
  }
  if (res.detail?.exists !== undefined) {
    r.exists = res.detail.exists || res.detail.stored > 0;
    r.count = res.detail.stored;
    r.values = String(res.detail.stored);
    drawRows();
  }
}

async function openFolder() {
  try { await api.OpenModSettingsFolder(); } catch (e) { say("fail", errText(e)); }
}

// key: the page's keys (16 §3.3): arrows move the table's cursor; in the values card they
// scroll it (the browser's own scrolling). true when taken.
export function key(e) {
  const k = e.key;
  if ((e.ctrlKey || e.metaKey) && !e.altKey && k.toLowerCase() === "o") { e.preventDefault(); openFolder(); return true; }
  if (document.activeElement?.closest?.(".ms-detail")) return false;
  if (table.key(e)) return true;
  if (e.ctrlKey || e.metaKey || e.altKey || !X.rows.length) return false;
  const i = X.rows.findIndex((r) => r.key === X.at);
  const step = Math.max(1, Math.floor((tableCard?.clientHeight || 300) / 30) - 1);
  const to = { ArrowDown: i + 1, j: i + 1, ArrowUp: i - 1, k: i - 1, PageDown: i + step, PageUp: i - step, Home: 0, End: X.rows.length - 1 }[k];
  if (to === undefined) return false;
  e.preventDefault();
  if (!tableCard?.contains(document.activeElement)) focusTable();
  moveTo(to);
  return true;
}

export function hintList() {
  return [{ key: "↑↓", label: T("move") }, { key: "1 2", label: T("the table / the values") }, { key: "ctrl+o", label: T("open the Config folder") },
    { key: "s", label: T("sort and columns") }, { key: "shift+1–8", label: T("pages") }, { key: "?", label: T("all keys") }];
}
