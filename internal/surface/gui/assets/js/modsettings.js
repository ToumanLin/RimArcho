// Mod library › Mod settings (16 §2.9 "Mod settings"): the current list's Mods that have
// settings, one row per settings file or HugsLib pack (its window's title, how many values
// it stores, its size), and on the right what the cursor's file stores. On/off, numbers, text
// and HugsLib choices change there while no game runs; the rest in the game. The table
// follows 16 §3.9; the gap between the cards drags (16 §3.1).
import { pathContent } from "./fields.js";
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { choiceControl } from "./choices.js";
import { beginPageRead, page, area, act, seg } from "./screens.js";
import { createTable } from "./datatable.js";
import { modSettingsColumns, configFileColumns } from "./tabledefs.js";
import * as pop from "./popups.js";
import { wireSeparator } from "./separator.js";
import { infoMark } from "./needtext.js";

// page: the table's read; rows: its rows as sorted; at: the cursor's file (its key), kept
// across reads and sorts; detail: the values on the right (for detail.key); seq: the newest
// values read (an older reply is dropped, 16 §3.1 rule 9).
const X = { cols: null, sort: "name", dir: 1, page: null, rows: [], at: "", detail: null, seq: 0 };
// The Config folder's other views (16 §2.9 "All files"): view 0 is the list's Mods (X);
// 1 not in this list, 2 not installed (orphans), 3 every file. F holds their table: files
// as read, rows as filtered and sorted, the cursor and the selection (file names).
const VIEWS = ["", "unlisted", "orphan", "all"];
let view = 0;
const F = { cols: null, sort: "name", dir: 1, page: null, rows: [], at: "", sel: new Set(), anchor: "" };
let tableCard = null, detailCard = null, rowEls = [], timer = 0, wanted = "";

// focusMod puts the cursor on a Mod's first file when the page opens next (a detail's
// settings row, 16 §4.1).
export function focusMod(instance) { wanted = instance || ""; }

const table = createTable({ key: "ui.gui.modsettings.columns", columns: modSettingsColumns, state: X,
  redraw: () => drawRows(), focus: () => focusTable(),
});
const ftable = createTable({ key: "ui.gui.modsettings.files.columns", columns: configFileColumns, state: F,
  redraw: () => drawFiles(), focus: () => focusTable(),
});

export async function open() {
  if (view) return openFiles();
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
  const w = page("fill modsettings", lead, viewSwitch(), copyButton(), act(T("open the Config folder"), "export", () => openFolder(), "ctrl+o"));
  tableCard = detailCard = null; rowEls = [];
  if (!p.rows.length) {
    const empty = el("section", "card empty-state");
    empty.append(el("p", "", T("no Mod of this list has a settings file, and Archo Monitor recorded no settings window yet")),
      el("p", "muted", T("a Mod's settings file appears once its settings are changed in the game")));
    w.append(empty);
    sc.append(w);
    return;
  }
  layout(w, sc);
  drawRows();
  drawDetail(X.detail);
}

// layout puts the table card, the gap and the right card on the page.
function layout(w, sc) {
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
  if (d.exists && d.kind !== "hugslib" && !d.loading && !X.page?.locked) {
    const b = el("button", "btn quiet ms-resetmod", T("reset to defaults…"));
    b.type = "button";
    b.title = T("move this Mod's settings files to the recycle bin: the game starts it with its defaults");
    b.dataset.envWrite = "mod-settings";
    b.onclick = () => resetMod(d);
    const box = el("div", "ms-dacts");
    box.append(b);
    head.append(box);
  }
  const body = el("div", "ms-body");
  body.tabIndex = 0;
  body.setAttribute("aria-label", T("stored values"));
  if (d.loading) body.append(el("p", "ms-note muted", T("reading…")));
  if (d.error) body.append(el("p", "ms-note err", d.error));
  // the game's window as Archo Monitor saw it, then what the file stores
  if (d.window) {
    body.append(el("h3", "ms-sub", Tf("in the game's window (%s)", d.window.seen)));
    if (d.window.older) body.append(el("p", "ms-note muted", T("changed since: the values stored in the file are the current ones")));
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
  // how the values work: behind an ⓘ after them, not lines read every time (16 §2.9)
  if (d.about?.length && d.values?.length) { const p = el("p", "ms-note muted"); p.append(infoMark(d.about.join("\n"))); body.append(p); }
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
    const s = choiceControl(v.key, (v.choices || []).map((c) => ({ value: c, label: c })), v.value);
    s.classList.add("ms-edit", "ms-choice");
    s.dataset.name = name;
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

// ---- views, cleanup, reset and copy (16 §2.9 "All files", "Cleanup", "Copy from another Game Config") ----

function setView(i) {
  if (i === view) return;
  view = i;
  F.sel.clear();
  F.at = "";
  open();
}

function viewSwitch() {
  return seg([T("this list"), T("not in this list"), T("not installed"), T("all files")], view, setView, ["alt+1", "alt+2", "alt+3", "alt+4"]);
}

function copyButton() {
  return act(T("copy settings from…"), "import", () => copyFrom(), "", "mod-settings");
}

async function openFiles() {
  const read = beginPageRead("mod-settings", { retry: open });
  let p;
  try { [p] = await Promise.all([api.ModSettingsFiles(), ftable.load()]); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  F.page = p;
  p.rows ||= [];
  const sc = $("#screen");
  sc.innerHTML = "";
  const shown = filesShown();
  let bytes = 0;
  for (const r of shown) bytes += r.bytes;
  const lead = shown.length === 1 ? Tf("%d file, %s", 1, sizeText(bytes)) : Tf("%d files, %s", shown.length, sizeText(bytes));
  const w = page("fill modsettings", lead, viewSwitch(), copyButton(), act(T("open the Config folder"), "export", () => openFolder(), "ctrl+o"));
  tableCard = detailCard = null; rowEls = [];
  if (!shown.length) {
    const empty = el("section", "card empty-state");
    empty.append(el("p", "", view === 2 ? T("no orphaned settings file: every Mod settings file belongs to an installed Mod") : T("no such file in the Config folder")));
    w.append(empty);
    sc.append(w);
    read.finish();
    return;
  }
  layout(w, sc);
  for (const k of [...F.sel]) if (!shown.some((r) => r.key === k)) F.sel.delete(k);
  drawFiles();
  drawSelection();
  read.finish();
}

function filesShown() {
  const want = VIEWS[view];
  return (F.page?.rows || []).filter((r) => want === "all" || r.status === want);
}

function sizeText(b) {
  if (b < 1024) return b + " B";
  if (b < 1024 * 1024) return (b / 1024).toFixed(b < 10240 ? 1 : 0) + " KB";
  return (b / 1024 / 1024).toFixed(1) + " MB";
}

function drawFiles() {
  if (!tableCard || !F.page) return;
  F.rows = ftable.sorted(filesShown());
  if (!F.rows.some((r) => r.key === F.at)) F.at = F.rows[0]?.key || "";
  tableCard.innerHTML = "";
  const head = el("div", "lib-th");
  tableCard.append(head);
  ftable.draw(tableCard, head, { role: "grid", count: F.rows.length });
  rowEls = [];
  F.rows.forEach((r, i) => {
    const on = F.sel.has(r.key);
    const row = el("div", "data-row" + (r.key === F.at ? " cur" : "") + (on ? " sel" : "") + (r.recyclable ? "" : " ms-defaults"));
    row.dataset.key = r.key;
    for (const c of ftable.visible()) {
      const cell = el("span", "cell" + (c.key === "name" ? " n" : "") + (ftable.column(c.key).num ? " num" : ""), String(r[c.key] ?? "—"));
      cell.title = cell.textContent;
      row.append(cell);
    }
    row.setAttribute("aria-selected", String(on));
    row.onclick = (e) => { focusTable(); clickFile(i, e); };
    ftable.row(row, i);
    rowEls.push(row);
    tableCard.append(row);
  });
  rowEls[F.rows.findIndex((r) => r.key === F.at)]?.scrollIntoView({ block: "nearest" });
}

// clickFile: a plain click selects only the clicked file, ctrl toggles it, shift adds the
// range from the last click (as the Mod library's rows).
function clickFile(i, e) {
  const r = F.rows[i];
  if (!r) return;
  if (e?.shiftKey && F.anchor) {
    const a = F.rows.findIndex((x) => x.key === F.anchor);
    const [lo, hi] = a < i ? [a, i] : [i, a];
    if (!e.ctrlKey && !e.metaKey) F.sel.clear();
    for (let k = Math.max(0, lo); k <= hi; k++) if (F.rows[k].recyclable) F.sel.add(F.rows[k].key);
  } else if (e?.ctrlKey || e?.metaKey) {
    if (F.sel.has(r.key)) F.sel.delete(r.key); else if (r.recyclable) F.sel.add(r.key);
    F.anchor = r.key;
  } else {
    F.sel.clear();
    if (r.recyclable) F.sel.add(r.key);
    F.anchor = r.key;
  }
  F.at = r.key;
  drawFiles();
  drawSelection();
}

function moveFile(i) {
  if (!F.rows.length) return;
  i = Math.max(0, Math.min(F.rows.length - 1, i));
  F.at = F.rows[i].key;
  F.anchor = F.at;
  for (const row of rowEls) row.classList.toggle("cur", row.dataset.key === F.at);
  rowEls[i]?.scrollIntoView({ block: "nearest" });
  drawSelection();
}

// picked is what the recycle button takes: the selection, else the cursor's file.
function picked() {
  if (F.sel.size) return F.rows.filter((r) => F.sel.has(r.key));
  const r = F.rows.find((x) => x.key === F.at);
  return r?.recyclable ? [r] : [];
}

function drawSelection() {
  if (!detailCard) return;
  detailCard.replaceChildren();
  const cur = F.rows.find((x) => x.key === F.at);
  const head = el("div", "ms-dhead");
  const body = el("div", "ms-body");
  body.tabIndex = 0;
  if (cur) {
    head.append(el("h2", "word-facts", cur.name));
    const facts = el("div", "facts");
    facts.append(el("span", "word-facts", cur.state), el("span", "", cur.size), el("span", "", Tf("last written %s", cur.modified)));
    if (cur.mod !== "—") facts.append(el("span", "word-facts", cur.mod));
    head.append(facts);
  }
  const go = picked();
  if (F.page?.locked) body.append(el("p", "ms-note ms-locked", F.page.locked));
  else if (go.length) {
    let bytes = 0;
    for (const r of go) bytes += r.bytes;
    const b = el("button", "btn ms-recycle", go.length === 1 ? Tf("move %d file (%s) to the recycle bin…", 1, sizeText(bytes)) : Tf("move %d files (%s) to the recycle bin…", go.length, sizeText(bytes)));
    b.type = "button";
    b.dataset.envWrite = "mod-settings";
    b.onclick = () => recycle(go);
    body.append(b);
  }
  if (cur && !cur.recyclable) body.append(el("p", "ms-note muted", T("a game file, not a Mod's settings: RimArcho leaves it alone")));
  body.append(el("p", "ms-note muted", T("not installed: no installed Mod has this file's folder name, for example a Mod that was removed; a Mod that comes back would read its old values again")),
    el("p", "ms-note muted", T("click selects, ctrl+click and shift+click add, space toggles, ctrl+a selects all")));
  detailCard.append(head, body);
}

function recycle(rows) {
  if (!rows.length || F.page?.locked) return;
  const lines = rows.map((r) => r.name + "  " + r.size + "  " + r.state);
  pop.confirm(rows.length === 1 ? Tf("move %d settings file to the recycle bin?", 1) : Tf("move %d settings files to the recycle bin?", rows.length),
    [T("the game reads these files at start; a Mod whose file goes starts with its defaults"), "", ...lines],
    T("move to the recycle bin"), async () => {
      try { say("ok", await api.RecycleModSettings(rows.map((r) => r.key))); } catch (e) { say("fail", errText(e)); }
      F.sel.clear();
      openFiles();
    }, "mod-settings");
}

function resetMod(d) {
  const r = X.rows.find((x) => x.key === d.key);
  if (!r) return;
  const files = X.rows.filter((x) => x.instance === r.instance && x.exists && !x.class.startsWith("hugslib:")).map((x) => x.file + "  " + x.size);
  pop.confirm(Tf("reset %s to its defaults?", r.name),
    [T("its settings files go to the recycle bin; the game starts the Mod with its defaults (HugsLib settings stay: reset them value by value)"), "", ...files],
    T("reset to defaults"), async () => {
      try { say("ok", await api.ResetModSettings(r.instance)); } catch (e) { say("fail", errText(e)); }
      X.detail = null;
      open();
    }, "mod-settings");
}

// copyFrom: choose a Game Config, then (on the list's view) every Mod or the cursor's, then
// the preview.
async function copyFrom() {
  let envs;
  try { envs = await api.ModSettingsSources(); } catch (e) { say("fail", errText(e)); return; }
  if (!envs.length) { say("fail", T("no other Game Config to copy settings from")); return; }
  pop.choose(T("copy settings from which Game Config?"), envs, (i) => {
    const from = envs[i];
    const r = view === 0 ? X.rows.find((x) => x.key === X.at) : null;
    if (!r) { previewCopy(from, [], ""); return; }
    pop.choose(T("copy which settings?"), [T("every Mod of this list"), Tf("only %s", r.name)], (j) => previewCopy(from, j ? [r.instance] : [], j ? r.name : ""));
  });
}

async function previewCopy(from, instances, name) {
  let v;
  try { v = await api.CopyModSettingsFrom(from, instances, true); } catch (e) { say("fail", errText(e)); return; }
  const title = name ? Tf("copy the settings of %s from %s?", name, from) : Tf("copy this list's settings from %s?", from);
  if (!v.copies) { pop.text(title, [v.message, "", ...v.lines]); return; }
  pop.confirm(title, [v.message, "", ...v.lines], T("copy"), async () => {
    try { say("ok", (await api.CopyModSettingsFrom(from, instances, false)).message); } catch (e) { say("fail", errText(e)); }
    X.detail = null;
    open();
  }, "mod-settings");
}

function fileKey(e) {
  const k = e.key;
  if (ftable.key(e)) return true;
  if ((e.ctrlKey || e.metaKey) && !e.altKey && k.toLowerCase() === "a") {
    e.preventDefault();
    for (const r of F.rows) if (r.recyclable) F.sel.add(r.key);
    drawFiles(); drawSelection();
    return true;
  }
  if (e.ctrlKey || e.metaKey || e.altKey || !F.rows.length) return false;
  const i = F.rows.findIndex((r) => r.key === F.at);
  if (k === " ") {
    e.preventDefault();
    const r = F.rows[i];
    if (r && F.sel.has(r.key)) F.sel.delete(r.key); else if (r?.recyclable) F.sel.add(r.key);
    drawFiles(); drawSelection();
    return true;
  }
  if (k === "Delete") { e.preventDefault(); recycle(picked()); return true; }
  const step = Math.max(1, Math.floor((tableCard?.clientHeight || 300) / 30) - 1);
  const to = { ArrowDown: i + 1, j: i + 1, ArrowUp: i - 1, k: i - 1, PageDown: i + step, PageUp: i - step, Home: 0, End: F.rows.length - 1 }[k];
  if (to === undefined) return false;
  e.preventDefault();
  if (!tableCard?.contains(document.activeElement)) focusTable();
  moveFile(to);
  return true;
}

async function openFolder() {
  try { await api.OpenModSettingsFolder(); } catch (e) { say("fail", errText(e)); }
}

// key: the page's keys (16 §3.3): arrows move the table's cursor; in the values card they
// scroll it (the browser's own scrolling). true when taken.
export function key(e) {
  const k = e.key;
  if ((e.ctrlKey || e.metaKey) && !e.altKey && k.toLowerCase() === "o") { e.preventDefault(); openFolder(); return true; }
  if (e.altKey && !e.ctrlKey && !e.metaKey && /^[1-4]$/.test(k)) { e.preventDefault(); setView(+k - 1); return true; }
  if (document.activeElement?.closest?.(".ms-detail")) return false;
  if (view) return fileKey(e);
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
  if (view) {
    return [{ key: "↑↓", label: T("move") }, { key: "space", label: T("select") }, { key: "del", label: T("move to the recycle bin") },
      { key: "alt+1–4", label: T("views") }, { key: "ctrl+o", label: T("open the Config folder") }, { key: "?", label: T("all keys") }];
  }
  return [{ key: "↑↓", label: T("move") }, { key: "1 2", label: T("the table / the values") }, { key: "ctrl+o", label: T("open the Config folder") },
    { key: "alt+1–4", label: T("views") }, { key: "s", label: T("sort and columns") }, { key: "?", label: T("all keys") }];
}
