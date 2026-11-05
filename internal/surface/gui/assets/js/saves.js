// Library › Saves (06 §2.11, 10 §4). Three cards, the gaps drag (06 §3.1 rule 10):
// left the colonies, each opening to its saves as a tree (search on top, `/`); middle the
// selected save: where it belongs, Can it load? with its quick fix and one row per problem,
// and its Mod list; right the playthrough, whose facts come with the full read of the save
// (10 §1.7 L1): skeletons until it is read. The listing reads every save's head, which can
// take a while on a cold library, so it arrives as a "saves" event (06 §8.4); the full read
// runs as a background task and its news arrives as "saves-read" events.
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { on } from "./api.js";
import { beginPageRead, page, area, act, button, hintsChanged } from "./screens.js";
import { wireSeparator } from "./separator.js";
import { pathContent } from "./fields.js";
import { icon } from "./icons.js";
import { shimmer } from "./motion.js";
import * as pop from "./popups.js";
import { getChoices } from "./credit.js";
import * as plans from "./plans.js";

let ui = null;
export function initSaves(u) { ui = u; }

// page: the listing; sel: the selected save's ref; open: the colonies shown open; q: the
// search, hits: the colonies whose Mods match it (from Go); detail: the middle and right
// cards' data (for detail.ref); ask: the inline confirmation shown (backup or recycle);
// renaming: the rename field is open; shown: how many of the Mod list's rows show (0: the
// first MODS; Show all adds STEP at a time, so a save of a thousand Mods stays within the
// page's size limits, 06 §11 A18).
const X = { page: null, sel: "", open: new Set(), q: "", hits: new Set(), detail: null, ask: null, renaming: false, shown: 0,
  listSeq: 0, detailSeq: 0, searchSeq: 0, busy: false };
let leftCard = null, midCard = null, rightCard = null, tree = null, input = null, timer = 0, searchTimer = 0;
const waiting = new Map(); // listing request → its resolve
const MODS = 24, STEP = 200;

on("saves", (ev) => {
  const w = waiting.get(ev?.request);
  if (!w) return;
  waiting.delete(ev.request);
  w(ev);
});
on("saves-read", (ev) => news(ev || {}));

// news is the full read's progress (06 §2.11 "Loading"): the status line while the page
// is open (never over a failure or a message with its undo), rows with their day and
// colonies regrouped, and the selected save's play facts once it is read.
function news(ev) {
  if (S.screen !== "saves" || !X.page) return;
  const st = $("#status"), quiet = !st || (st.dataset.kind !== "fail" && !st.querySelector("a"));
  if (ev.line && quiet) say("busy", ev.line);
  if (ev.done && (ev.error || quiet)) say(ev.error ? "fail" : "ok", ev.error || ev.text || "");
  if (ev.page) update(ev.page);
  if (X.sel && (ev.refs || []).includes(X.sel) && X.detail?.ref === X.sel && !X.detail.play && !X.detail.loading) refreshDetail();
}

// refreshDetail asks for the selected save's cards again after the full read; while the
// rename field or a confirmation is open only the right card is redrawn, so neither loses
// what the player is doing.
async function refreshDetail() {
  if (!X.renaming && !X.ask) { showDetail(0); return; }
  const ref = X.sel, n = ++X.detailSeq;
  let d;
  try { d = await api.SaveDetail(ref); } catch { return; }
  if (n !== X.detailSeq || S.screen !== "saves" || X.sel !== ref || X.detail?.ref !== ref) return;
  Object.assign(X.detail, { play: d.play, reading: d.reading, playNote: d.playNote, colony: d.colony });
  drawRight(true);
}

// update takes a newer listing from the full read: rows get their in-game day and colonies
// regroup by world, while the selection and the colonies shown open (by their saves) stay.
function update(p) {
  const shown = new Set(X.page.colonies.filter((c) => X.open.has(c.key)).flatMap((c) => c.saves.map((s) => s.ref)));
  const colony = colonyOf(X.sel)?.name;
  X.open = new Set();
  setPage(p);
  for (const c of p.colonies) if (c.saves.some((s) => shown.has(s.ref))) X.open.add(c.key);
  if (!leftCard?.isConnected) return;
  const lead = $("#screen .page.saves .page-head .lead");
  if (lead && p.lead) lead.textContent = p.lead;
  drawLeft();
  if (X.detail && !X.detail.loading && colonyOf(X.sel)?.name !== colony) refreshDetail();
}

// readList asks for the listing and waits for its event; a newer request makes it stale.
function readList() {
  // increasing across reloads of the page, as api.js's requests are
  const n = X.listSeq = Math.max(X.listSeq + 1, Date.now() * 1000);
  return new Promise((resolve, reject) => {
    waiting.set(n, (ev) => {
      if (ev.error === "stale" || n !== X.listSeq) return resolve(null);
      if (ev.error) return reject(new Error(ev.error));
      resolve(ev.page);
    });
    api.SavesRead(n, X.sel).then((ok) => { if (!ok) { waiting.delete(n); reject(new Error(T("the window is closing"))); } },
      (e) => { waiting.delete(n); reject(e); });
  });
}

export async function open() {
  const read = beginPageRead("saves", { retry: open, label: "reading the saves…" });
  let p;
  try { p = await readList(); } catch (e) { read.fail(e); return; }
  if (!p || !read.current()) return;
  setPage(p);
  build();
  read.finish();
  showDetail(0);
}

// reread reads the listing again behind the page shown (after an action, a game exit):
// the selection stays when its save is still there; select names a save to go to.
async function reread(select = "") {
  let p;
  try { p = await readList(); } catch (e) { say("fail", errText(e)); return; }
  if (!p || S.screen !== "saves") return;
  if (select) X.sel = select;
  setPage(p);
  if (!leftCard?.isConnected) { build(); showDetail(0); return; }
  drawLeft();
  showDetail(0);
}

function setPage(p) {
  X.page = p;
  p.colonies ||= [];
  const all = p.colonies.flatMap((c) => c.saves);
  if (!all.some((s) => s.ref === X.sel)) {
    X.sel = all[0]?.ref || "";
    X.detail = null;
  }
  const c = colonyOf(X.sel);
  if (c) X.open.add(c.key);
}

const colonyOf = (ref) => X.page?.colonies.find((c) => c.saves.some((s) => s.ref === ref)) || null;
const rowOf = (ref) => X.page?.colonies.flatMap((c) => c.saves).find((s) => s.ref === ref) || null;

function build() {
  const p = X.page;
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = page("fill saves", p.lead || "", act(T("rescan"), "rescan", () => { say("busy", T("reading the saves…")); reread().then(() => say("ok", T("saves read again"))); }));
  leftCard = midCard = rightCard = tree = input = null;
  if (!p.saves) {
    const empty = el("section", "card empty-state");
    empty.append(el("p", "", p.empty || T("no save in the Saves folders of your games and packs")));
    for (const f of p.folders || []) empty.append(pathContent(el("p", "sv-folder muted"), f, [f]));
    for (const e of p.errors || []) empty.append(el("p", "err", e));
    w.append(empty);
    sc.append(w);
    return;
  }
  const grid = el("div", "sv");
  leftCard = el("section", "card sv-left");
  const search = el("label", "sv-search");
  const si = el("span", "ic"); si.innerHTML = icon.search || "";
  input = el("input", "sv-q");
  input.type = "search";
  input.placeholder = T("search colonies, saves, Mods");
  input.setAttribute("aria-label", T("search colonies, saves, Mods"));
  input.value = X.q;
  input.autocomplete = "off"; input.spellcheck = false;
  input.oninput = () => setQuery(input.value);
  input.onkeydown = (e) => {
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); if (input.value) { input.value = ""; setQuery(""); } else focusTree(); }
    else if (e.key === "ArrowDown" || e.key === "Enter") { e.preventDefault(); e.stopPropagation(); focusTree(); }
    else e.stopPropagation(); // typing is not a page key
  };
  search.append(si, input, el("kbd", "", "/"));
  tree = el("div", "sv-tree");
  tree.setAttribute("role", "tree");
  tree.setAttribute("aria-label", T("colonies and their saves"));
  tree.tabIndex = 0;
  leftCard.append(search, tree);
  const gap1 = el("div", "split sv-split");
  midCard = el("section", "card sv-mid");
  const gap2 = el("div", "split sv-split");
  rightCard = el("section", "card sv-right");
  grid.append(leftCard, gap1, midCard, gap2, rightCard);
  w.append(grid);
  sc.append(w);
  const setVar = (name) => (v) => { if (v) grid.style.setProperty(name, v + "px"); else grid.style.removeProperty(name); };
  const leftRange = () => [200, Math.max(200, Math.min(480, grid.clientWidth - 640))];
  wireSeparator(gap1, { key: "ui.gui.saves.left", range: leftRange,
    get: () => leftCard.getBoundingClientRect().width, set: setVar("--sv-left"), reset: () => setVar("--sv-left")(0),
    restore: (v) => setVar("--sv-left")(+v ? Math.max(leftRange()[0], Math.min(leftRange()[1], +v)) : 0) });
  const rightRange = () => [260, Math.max(260, grid.clientWidth - leftCard.getBoundingClientRect().width - 380)];
  wireSeparator(gap2, { key: "ui.gui.saves.right", direction: -1, range: rightRange,
    get: () => rightCard.getBoundingClientRect().width, set: setVar("--sv-right"), reset: () => setVar("--sv-right")(0),
    restore: (v) => setVar("--sv-right")(+v ? Math.max(rightRange()[0], Math.min(rightRange()[1], +v)) : 0) });
  area(leftCard, null, () => focusTree());
  area(midCard, null, () => midCard.querySelector(".sv-body")?.focus({ preventScroll: true }));
  area(rightCard, null, () => rightCard.querySelector(".sv-body")?.focus({ preventScroll: true }));
  tree.onclick = (e) => {
    const s = e.target.closest("[data-save]");
    if (s) { pick(s.dataset.save); return; }
    const c = e.target.closest("[data-colony]");
    if (c) clickColony(c.dataset.colony);
  };
  tree.ondblclick = (e) => { if (e.target.closest("[data-save]")) startRename(); };
  drawLeft();
  drawDetail();
}

function focusTree() {
  if (!tree) return;
  tree.focus({ preventScroll: true });
  tree.querySelector(".sv-save.on")?.scrollIntoView({ block: "nearest" });
}

// ---- left: colonies and their saves (06 §2.11) ----

const lc = (s) => String(s || "").toLowerCase();
function saveMatch(c, s) { return !X.q || lc(c.name).includes(X.q) || X.hits.has(c.key) || lc(s.name).includes(X.q); }
function colonyMatch(c) { return !X.q || lc(c.name).includes(X.q) || X.hits.has(c.key) || c.saves.some((s) => lc(s.name).includes(X.q)); }

function setQuery(v) {
  X.q = lc(v.trim());
  X.hits = new Set();
  drawLeft();
  clearTimeout(searchTimer);
  if (!X.q) return;
  const n = ++X.searchSeq;
  // Mods in a colony's saves are matched in Go, where the heads are (a short rest first)
  searchTimer = setTimeout(async () => {
    let keys = [];
    try { keys = await api.SavesSearch(X.q); } catch { return; }
    if (n !== X.searchSeq || S.screen !== "saves") return;
    X.hits = new Set(keys || []);
    drawLeft();
  }, 120);
}

// mark highlights the search in a name.
function mark(text) {
  const box = el("span", "n");
  const i = X.q ? lc(text).indexOf(X.q) : -1;
  if (i < 0) { box.textContent = text; return box; }
  box.append(text.slice(0, i), el("mark", "sv-hit", text.slice(i, i + X.q.length)), text.slice(i + X.q.length));
  return box;
}

function drawLeft() {
  if (!tree || !X.page) return;
  const focused = tree.contains(document.activeElement) || document.activeElement === tree;
  tree.replaceChildren();
  const cols = X.page.colonies.filter(colonyMatch);
  if (!cols.length) { tree.append(el("div", "sv-none muted", T("nothing matches"))); return; }
  for (const c of cols) {
    const opened = X.open.has(c.key) || !!X.q;
    const row = el("button", "sv-col" + (opened ? " open" : ""));
    row.type = "button";
    row.tabIndex = -1;
    row.dataset.colony = c.key;
    row.id = "colony-row-" + encodeURIComponent(c.key);
    row.setAttribute("role", "treeitem");
    row.setAttribute("aria-expanded", String(opened));
    row.setAttribute("aria-level", "1");
    const chv = el("span", "chv"); chv.innerHTML = icon.chev;
    row.append(chv, mark(c.name), el("span", "k", String(c.saves.length)));
    row.setAttribute("aria-label", Tf("%s, %d saves", c.name, c.saves.length));
    tree.append(row);
    if (!opened) continue;
    const box = el("div", "sv-saves");
    box.setAttribute("role", "group");
    for (const s of c.saves.filter((s) => saveMatch(c, s))) {
      const r = el("button", "sv-save" + (s.ref === X.sel ? " on" : ""));
      r.type = "button";
      r.tabIndex = -1;
      r.dataset.save = s.ref;
      r.id = "save-row-" + encodeURIComponent(s.ref);
      r.setAttribute("role", "treeitem");
      r.setAttribute("aria-level", "2");
      r.setAttribute("aria-selected", String(s.ref === X.sel));
      r.append(mark(s.name), el("span", "d", s.when || ""));
      // the in-game day shows once the save is read; the date it was saved stays on hover
      r.title = [s.auto ? T("autosave") : "", s.saved || ""].filter(Boolean).join("  ");
      box.append(r);
    }
    tree.append(box);
  }
  const on = tree.querySelector(".sv-save.on");
  if (on) tree.setAttribute("aria-activedescendant", on.id); else tree.removeAttribute("aria-activedescendant");
  on?.scrollIntoView({ block: "nearest" });
  if (focused) tree.focus({ preventScroll: true });
}

// clickColony opens a colony and selects its latest save; clicking the open current
// colony folds it (06 §2.11).
function clickColony(key) {
  const c = X.page.colonies.find((x) => x.key === key);
  if (!c) return;
  const cur = colonyOf(X.sel);
  if (X.open.has(key) && cur === c) { X.open.delete(key); drawLeft(); return; }
  X.open.add(key);
  pick(c.saves[0].ref);
}

function pick(ref) {
  if (!ref) return;
  const c = colonyOf(ref);
  if (c) X.open.add(c.key);
  if (ref === X.sel) { drawLeft(); return; }
  X.sel = ref;
  X.ask = null; X.renaming = false; X.shown = 0;
  drawLeft();
  showDetail(60);
}

// moveSave goes to the previous or next save shown in the tree.
function moveSave(step) {
  const refs = [...(tree?.querySelectorAll("[data-save]") || [])].map((n) => n.dataset.save);
  if (!refs.length) return;
  const i = refs.indexOf(X.sel);
  pick(refs[Math.max(0, Math.min(refs.length - 1, i < 0 ? 0 : i + step))]);
}

// ---- middle and right: the selected save ----

// showDetail reads the selected save's check once the cursor rests (a held arrow reads
// only where it stops); the header shows at once from the tree's row.
function showDetail(delay) {
  clearTimeout(timer);
  const ref = X.sel;
  if (!ref) { X.detail = null; drawDetail(); return; }
  if (X.detail?.ref !== ref) {
    const r = rowOf(ref);
    X.detail = { ref, name: r?.name || "", colony: colonyOf(ref)?.name || "", loading: true, issues: [], mods: [], listOnly: [] };
    drawDetail();
  }
  const n = ++X.detailSeq;
  timer = setTimeout(async () => {
    let d;
    try { d = await api.SaveDetail(ref); } catch (e) { d = { ref, name: rowOf(ref)?.name || "", error: errText(e), issues: [], mods: [], listOnly: [] }; }
    if (n !== X.detailSeq || S.screen !== "saves" || X.sel !== ref) return;
    X.detail = d;
    drawDetail(true);
    hintsChanged();
  }, delay);
}

function drawDetail(keep = false) {
  drawMid(keep);
  drawRight(keep);
}

function body() {
  const b = el("div", "sv-body");
  b.tabIndex = 0;
  return b;
}

function drawMid(keep) {
  if (!midCard) return;
  const d = X.detail;
  const top = keep ? midCard.querySelector(".sv-body")?.scrollTop || 0 : 0;
  const focused = midCard.contains(document.activeElement) && !X.renaming ? document.activeElement : null;
  const focusKey = focused?.dataset?.key;
  midCard.replaceChildren();
  if (!d) { midCard.append(el("div", "empty-state", T("select a save"))); return; }
  const b = body();
  b.setAttribute("aria-label", d.name || T("the save"));
  // header: the name and the save's actions, then game, list and location
  const head = el("div", "sv-head");
  const row1 = el("div", "sv-row1");
  if (X.renaming && !d.loading && !d.error) row1.append(renameField(d));
  else {
    const h = el("h2", "sv-name", d.name);
    h.ondblclick = () => startRename();
    row1.append(h);
  }
  const acts = el("div", "sv-acts");
  const locked = d.locked || (d.loading ? T("reading…") : "");
  const add = (label, key, run, k, off = "") => {
    const b = button(label, k, run, "small", key);
    b.dataset.key = key;
    if (off) { b.disabled = true; b.title = off; } else if (k) b.title = k;
    acts.append(b);
    return b;
  };
  const noFile = d.loading || d.error ? T("reading…") : "";
  add(T("rename"), "rename", () => startRename(), "F2", locked || noFile);
  add(T("copy"), "copy", () => copySave(), "", noFile);
  add(T("back up"), "backup", () => askFiles("backup"), "", noFile);
  const rec = add(T("recycle…"), "recycle", () => askFiles("recycle"), "", locked || noFile);
  rec.classList.add("danger");
  row1.append(acts);
  head.append(row1);
  if (d.error) head.append(el("p", "err", d.error));
  if (!d.loading && !d.error) {
    const kv = el("dl", "sv-kv");
    const fact = (k, ...v) => { const dd = el("dd", ""); dd.append(...v); kv.append(el("dt", "", k), dd); return dd; };
    fact(T("game"), [d.game, d.env].filter(Boolean).join("  ") || "—");
    if (d.listKind) {
      const n = el("b", "", d.list);
      const dd = fact(d.listKind === "pack" ? T("pack") : T("list"), n);
      if (d.match) dd.append(el("span", "muted sv-match", "  " + d.match));
    } else fact(T("list"), el("span", "warn", T("no list or pack matches")));
    const loc = el("dd", "sv-loc");
    pathContent(loc, d.path, [d.path]);
    kv.append(el("dt", "", T("location")), loc);
    head.append(kv);
  }
  b.append(head);
  if (X.ask && X.ask.ref === d.ref) b.append(confirmBox());
  if (d.loading) {
    const sk = el("div", "sv-sect");
    sk.append(shimmer(T("checking the save…"), 4));
    b.append(sk);
    midCard.append(b);
    return;
  }
  if (!d.error) {
    b.append(loadSection(d));
    if (d.mods?.length) b.append(modSection(d));
  }
  midCard.append(b);
  b.scrollTop = top;
  if (focusKey) midCard.querySelector(`[data-key="${CSS.escape(focusKey)}"]`)?.focus({ preventScroll: true });
}

// loadSection is Can it load? (10 §4.3): the quick-fix bar, then only what needs attention.
function loadSection(d) {
  const s = el("div", "sv-sect");
  const h = el("h3", "", T("can it load?"));
  h.append(el("span", "r", d.against || ""));
  s.append(h);
  // green only when it loads with its list; no list is neither good nor bad
  const cls = d.level === "error" ? " bad" : d.level === "warning" ? " warn" : d.level === "writing" || !d.listKind ? " busy" : " ok";
  const bar = el("div", "sv-fixbar" + cls);
  bar.setAttribute("role", "group");
  bar.setAttribute("aria-label", d.verdict);
  bar.append(el("span", "t", d.verdict));
  if (d.quick) {
    const q = fixButton(d, d.quick, true);
    q.append(el("kbd", "", "↵"));
    bar.append(q);
  }
  s.append(bar);
  if (d.issues?.length) {
    const list = el("div", "sv-issues");
    for (const is of d.issues) {
      const row = el("div", "sv-iss " + (is.level || ""));
      row.setAttribute("role", "group");
      row.setAttribute("aria-label", is.head);
      const sign = el("span", "sign " + (is.sign === "−" ? "m" : is.sign === "+" ? "p" : "o"), is.sign || (is.level === "error" ? "✗" : is.level === "warning" ? "!" : "i"));
      sign.setAttribute("aria-hidden", "true");
      const head = el("span", "h", is.head);
      if (is.mods?.length > 1) head.title = is.mods.join("\n");
      const fixes = el("span", "f");
      for (const f of is.fixes || []) fixes.append(fixButton(d, f, false));
      row.append(sign, head, fixes, el("span", "c", is.consequence));
      list.append(row);
    }
    s.append(list);
  }
  return s;
}

function fixButton(d, f, primary) {
  const env = { fix: "save-fix", subscribe: "save-fix", enable: "save-fix", mods: "save-fix", "disable-added": "save-fix", order: "save-fix", launch: "launch",
    recycle: "recycle", "new-list": "new-plan" }[f.action] || "";
  const b = button(f.label, "", () => runFix(d, f), primary ? "primary small" : "small sv-fix", env);
  b.dataset.key = "fix-" + f.action + "-" + (f.target || "");
  b.title = f.command;
  if (f.action === "recycle" && d.locked) { b.disabled = true; b.title = d.locked; }
  return b;
}

// modSection is the save's Mods in load order (10 §4.3 Mod list).
function modSection(d) {
  const s = el("div", "sv-sect");
  const h = el("h3", "", T("mod list"));
  const facts = [Tf("%d in load order", d.mods.length)];
  if (d.missing) facts.push(Tf("%d not installed", d.missing));
  if (d.moved) facts.push(Tf("%d in another order", d.moved));
  const r = el("span", "r", facts.join(T(", ")));
  h.append(r);
  s.append(h);
  const shown = d.mods.slice(0, Math.max(MODS, X.shown));
  const ol = el("ol", "sv-mods");
  ol.setAttribute("aria-label", T("mod list"));
  shown.forEach((m, i) => {
    const li = el("li", (m.missing ? "gone" : "") + (m.off ? " off" : ""));
    li.append(el("i", "", String(i + 1)), el("span", "n", m.name));
    li.title = m.packageId;
    if (m.missing) li.append(el("span", "tagx bad", T("not installed")));
    else if (m.off) li.append(el("span", "tagx", T("not in the list")));
    if (m.moved) { const mv = el("b", "mv", "↕"); mv.title = T("in another order in the list"); mv.setAttribute("aria-label", T("in another order in the list")); li.append(mv); }
    ol.append(li);
  });
  s.append(ol);
  const more = el("div", "sv-more");
  if (d.mods.length > MODS) {
    const left = d.mods.length - shown.length;
    const label = !left ? T("show fewer") : left <= STEP ? Tf("show all %d", d.mods.length) : Tf("show %d more (%d left)", STEP, left);
    const all = button(label, "", () => { X.shown = left ? shown.length + STEP : 0; drawMid(true); }, "link");
    all.dataset.key = "show-all";
    more.append(all);
  }
  if (d.newList) {
    const nl = button(d.newList.label, "", () => runFix(d, d.newList), "link", "new-plan");
    nl.dataset.key = "new-list";
    nl.title = d.newList.command;
    more.append(nl);
  }
  if (d.listOnly?.length) {
    const names = d.listOnly.slice(0, 3).join(T(", ")) + (d.listOnly.length > 3 ? " " + Tf("and %d more", d.listOnly.length - 3) : "");
    more.append(el("span", "muted", Tf("in the list, not in the save: %s", names)));
  }
  if (more.childNodes.length) s.append(more);
  return s;
}

// drawRight is the playthrough (10 §4.3 Play facts, §4.5; the draft's right column):
// the colony, its day and date, storyteller, difficulty and play time; the numbers at this
// save with their change since the colony's previous read save, the colonists; the
// newest letters; the colony over time. Skeletons while the save is read.
function drawRight(keep) {
  if (!rightCard) return;
  const d = X.detail;
  const top = keep ? rightCard.querySelector(".sv-body")?.scrollTop || 0 : 0;
  const focused = rightCard.contains(document.activeElement) ? document.activeElement.dataset?.key || "" : null;
  rightCard.replaceChildren();
  if (!d) return;
  const p = d.play;
  const b = body();
  b.setAttribute("aria-label", T("the playthrough"));
  const head = el("div", "sv-head");
  const row = el("div", "sv-row1");
  row.append(el("h2", "sv-name", d.colony || ""));
  if (p) row.append(el("span", "muted", [p.day, p.date].filter(Boolean).join("  ")));
  head.append(row);
  if (p) {
    const kv = el("dl", "sv-kv");
    for (const [k, v] of [[T("storyteller"), p.storyteller], [T("difficulty"), p.difficulty], [T("play time"), p.played]]) {
      if (v) kv.append(el("dt", "", k), el("dd", "", v));
    }
    if (kv.childNodes.length) head.append(kv);
  }
  b.append(head);
  if (d.loading) { rightCard.append(b); return; }
  if (p) b.append(...playSections(p));
  else if (d.reading) {
    const sk = el("div", "sv-sect");
    sk.append(shimmer(T("reading the save…"), 5));
    b.append(sk);
  } else {
    const s = el("div", "sv-sect");
    const kv = el("dl", "sv-kv");
    const c = colonyOf(d.ref);
    if (c) kv.append(el("dt", "", T("saves")), el("dd", "", String(c.saves.length)));
    if (d.saved) kv.append(el("dt", "", T("saved on")), el("dd", "", d.saved));
    if (d.size) kv.append(el("dt", "", T("size")), el("dd", "", d.size));
    s.append(kv);
    if (d.playNote) s.append(el("p", "sv-note muted", d.playNote));
    b.append(s);
  }
  rightCard.append(b);
  b.scrollTop = top;
  if (focused !== null) (focused && b.querySelector(`[data-key="${CSS.escape(focused)}"]`) || b).focus({ preventScroll: true });
}

// playSections draws the full read's facts (L1).
function playSections(p) {
  const out = [];
  const at = el("div", "sv-sect");
  const h = el("h3", "", T("at this save"));
  h.append(el("span", "r", p.since ? Tf("change since %s", p.since) : T("no earlier save of this colony is read")));
  at.append(h);
  if (p.note) at.append(el("p", "sv-note warn", p.note));
  const grid = el("div", "sv-stats");
  for (const st of p.stats || []) {
    const tile = el("div", "sv-stat");
    // paragraphs, so each number gets its reference number (ctrl+\)
    tile.append(el("p", "n", st.value), el("p", "l", st.label));
    if (st.change) tile.append(el("p", "d" + (st.down ? " neg" : st.change === "±0" ? " same" : ""), st.change));
    grid.append(tile);
  }
  at.append(grid);
  if (p.colonists?.length) {
    const ul = el("ul", "sv-people");
    ul.setAttribute("aria-label", T("colonists"));
    for (const c of p.colonists) {
      const li = el("li", "sv-person" + (c.dead ? " dead" : "") + (c.slave ? " slave" : ""), c.name);
      if (c.dead) { li.setAttribute("aria-label", Tf("%s, dead", c.name)); li.title = T("dead"); }
      else if (c.slave) { li.setAttribute("aria-label", Tf("%s, slave", c.name)); li.title = T("slave"); }
      ul.append(li);
    }
    if (p.more) ul.append(el("li", "sv-person more", Tf("and %d more", p.more)));
    at.append(ul);
  }
  out.push(at);
  if (p.events?.length) {
    const s = el("div", "sv-sect");
    const eh = el("h3", "", T("events"));
    eh.append(el("span", "r", T("newest first")));
    const ul = el("ul", "sv-events");
    for (const e of p.events) {
      const li = el("li", e.death ? "death" : "");
      li.append(el("span", "t", Tf("day %d", e.day)), el("span", "x", e.text));
      ul.append(li);
    }
    s.append(eh, ul);
    out.push(s);
  }
  if (p.timeline) {
    const s = el("div", "sv-sect");
    const th = el("h3", "", T("over time"));
    th.append(el("span", "r", T("a point opens its save")));
    s.append(th, chart(p.timeline));
    const dead = (p.colonists || []).filter((c) => c.dead).map((c) => c.name);
    const legend = el("p", "sv-legend");
    if (p.timeline.wealth) legend.append(el("span", "w", T("wealth")));
    if (p.timeline.people) legend.append(el("span", "p", T("colonists")));
    if (p.timeline.deaths?.length) legend.append(el("span", "x", T("deaths")));
    const names = dead.join(T(", ")) + (p.dead > dead.length ? " " + Tf("and %d more", p.dead - dead.length) : "");
    legend.append(el("span", "muted", p.dead ? Tf("%d dead: %s", p.dead, names) : T("no colonist has died")));
    s.append(legend);
    out.push(s);
  }
  return out;
}

const SVG = "http://www.w3.org/2000/svg";
function svgEl(tag, attrs, text) {
  const n = document.createElementNS(SVG, tag);
  for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, String(v));
  if (text != null) n.textContent = text;
  return n;
}

// kilo is wealth on the chart's hover text: 850, 12.4k, 1.2M.
function kilo(v) {
  if (v < 1000) return String(Math.round(v));
  if (v < 1e4) return (Math.round(v / 100) / 10) + "k";
  if (v < 1e6) return Math.round(v / 1000) + "k";
  return (Math.round(v / 1e5) / 10) + "M";
}

// chart is the colony over in-game days (10 §4.5): this save's wealth (line and area) and
// colonists (dashed) from its own history, a mark per death letter, and a point per read
// save of the colony; a point opens its save. The tree reaches every save by keys, so the
// points are for the mouse; the chart reads as one image.
function chart(tl) {
  const w = Math.max(240, (rightCard?.clientWidth || 360) - 34), h = 120, L = 12, R = 12, TOP = 10, B = 20;
  const days = Math.max(1, tl.days), wealth = tl.wealth || [], people = tl.people || [];
  const maxW = Math.max(1, ...wealth, ...tl.points.map((p) => p.wealth));
  const maxP = Math.max(1, ...people);
  const x = (d) => L + (Math.min(d, days) / days) * (w - L - R);
  const y = (v, max) => h - B - (v / max) * (h - B - TOP);
  const along = (vals, max) => vals.map((v, i) => `${x(tl.upto * i / Math.max(1, vals.length - 1)).toFixed(1)},${y(v, max).toFixed(1)}`).join(" ");
  const svg = svgEl("svg", { class: "sv-chart", viewBox: `0 0 ${w} ${h}`, role: "img",
    "aria-label": Tf("wealth and colonists over %d days, %d saves", days, tl.points.length) });
  svg.append(svgEl("line", { class: "ax", x1: L, x2: w - R, y1: h - B, y2: h - B }));
  for (const t of [...new Set([0, 0.25, 0.5, 0.75, 1].map((f) => Math.round(f * days)))]) {
    svg.append(svgEl("text", { x: x(t), y: h - 5, "text-anchor": t === 0 ? "start" : t === days ? "end" : "middle" }, Tf("day %d", t)));
  }
  for (const d of tl.deaths || []) svg.append(svgEl("line", { class: "ev", x1: x(d), x2: x(d), y1: TOP, y2: h - B }));
  if (wealth.length > 1) {
    const line = along(wealth, maxW);
    svg.append(svgEl("polygon", { class: "area", points: `${x(0)},${h - B} ${line} ${x(tl.upto)},${h - B}` }));
    svg.append(svgEl("polyline", { class: "ln", points: line }));
  }
  if (people.length > 1) svg.append(svgEl("polyline", { class: "pop", points: along(people, maxP) }));
  for (const pt of tl.points) {
    const on = pt.ref === X.sel, off = pt.wealth < 0;
    const cx = x(pt.day), cy = off ? h - B : y(pt.wealth, maxW), r = pt.auto ? 3.5 : 5;
    if (on) svg.append(svgEl("circle", { class: "ring", cx, cy, r: r + 4 }));
    const c = svgEl("circle", { class: "pt" + (pt.auto ? " auto" : "") + (off ? " off" : "") + (on ? " sel" : ""), cx, cy, r });
    c.dataset.save = pt.ref;
    c.dataset.key = "point-" + pt.ref;
    c.append(svgEl("title", {}, [pt.name, Tf("day %d", pt.day), off ? "" : Tf("wealth %s", kilo(pt.wealth))].filter(Boolean).join("  ")));
    svg.append(c);
  }
  svg.onclick = (e) => { const c = e.target.closest?.("[data-save]"); if (c && c.dataset.save !== X.sel) pick(c.dataset.save); };
  return svg;
}

// ---- actions (10 §4.4) ----

async function runFix(d, f) {
  switch (f.action) {
    case "recycle": return askFiles("recycle");
    case "open-save": return pick(f.target);
    case "new-list": return newList(d);
    case "launch": return launch(d);
    case "fix": return fix(d, [], false, false, true);
    case "enable": case "subscribe": return fix(d, [f.target], false, false, false);
    case "mods": return fix(d, f.target.split(" "), false, false, false); // a folded row: all of its Mods
    case "disable-added": return fix(d, [], true, false, false);
    case "order": return fix(d, [], false, true, false);
  }
}

// fix applies the save's fixes to its list (one undo step), then subscribes to the
// Workshop Mods the fix added, after the usual Steam / SteamCMD choice; with then launch,
// the list becomes current and launches through the usual launch, with its refusals.
async function fix(d, mods, disableAdded, order, thenLaunch) {
  if (X.busy) return;
  X.busy = true;
  let r;
  try { r = await api.SaveFix(d.ref, mods, disableAdded, order); }
  catch (e) { X.busy = false; say("fail", errText(e)); return; }
  X.busy = false;
  // u and the status line's undo take the edit back, and the save is checked again
  say("ok", r.text, r.undo ? async () => { await ui.verb("undo"); if (S.screen === "saves") showDetail(0); } : false);
  await ui?.reload?.();
  if (S.screen === "saves") showDetail(0);
  const ids = r.workshopIds || [];
  // without downloads it launches now; with them the list becomes current once they are
  // queued, and the player launches when they are in (06 §2.11)
  const after = async () => {
    if (!thenLaunch) return;
    if (!ids.length) { if (await plans.usePlan(r.plan)) await ui.run("launch"); return; }
    if (await plans.usePlan(r.plan)) say("ok", Tf("%s is current; launch it once the %d Mods are downloaded", d.list, ids.length));
  };
  if (!ids.length) return after();
  pop.choose(Tf("get %d mods", ids.length), getChoices(), async (i) => {
    try { say("ok", await api.Subscribe({ kind: "workshop", url: ids.join(" "), steamcmd: i === 1, batch: true })); }
    catch (e) { say("fail", errText(e)); return; }
    await after();
  });
}

// launch makes the save's list current and launches it through the usual launch.
async function launch(d) {
  if (!d.listId) return;
  if (await plans.usePlan(d.listId)) await ui.run("launch");
}

async function newList(d) {
  let r;
  try { r = await api.SaveNewList(d.ref); } catch (e) { say("fail", errText(e)); return; }
  say("ok", r.text);
  await ui?.reload?.();
  if (r.page) ui.go(r.page);
}

async function copySave() {
  const d = X.detail;
  if (!d || d.loading) return;
  let r;
  try { r = await api.SaveCopy(d.ref); } catch (e) { say("fail", errText(e)); return; }
  say("ok", r.text);
  await reread(r.ref);
}

// askFiles shows the inline confirmation of Back up or Recycle (06 §6): the files and
// sizes first; only the confirm button acts.
async function askFiles(kind) {
  const d = X.detail;
  if (!d || d.loading) return;
  if (kind === "recycle" && d.locked) return say("fail", d.locked);
  let r;
  try { r = await (kind === "backup" ? api.SaveBackup([d.ref], false) : api.SaveTrash([d.ref], false)); }
  catch (e) { say("fail", errText(e)); return; }
  if (X.sel !== d.ref || S.screen !== "saves") return;
  X.ask = { ref: d.ref, kind, preview: r.preview };
  drawMid(true);
  midCard?.querySelector(".sv-ask .btn.go")?.focus({ preventScroll: true });
}

function confirmBox() {
  const a = X.ask, p = a.preview;
  const box = el("div", "sv-ask" + (a.kind === "recycle" ? " danger" : ""));
  box.setAttribute("role", "group");
  box.setAttribute("aria-label", p.title);
  box.append(el("p", "t", p.title));
  const ul = el("ul", "files");
  for (const l of p.lines || []) ul.append(el("li", "", l));
  box.append(ul, el("p", "muted", p.to));
  const btns = el("div", "btns");
  const go = button(p.button, "", () => doFiles(a), "small go primary" + (a.kind === "recycle" ? " danger-go" : ""), a.kind === "recycle" ? "recycle" : "");
  go.dataset.key = "confirm";
  const no = button(T("cancel"), "", () => { X.ask = null; drawMid(true); midCard?.querySelector(`[data-key="${a.kind === "backup" ? "backup" : "recycle"}"]`)?.focus({ preventScroll: true }); }, "small");
  no.dataset.key = "cancel";
  box.onkeydown = (e) => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); no.click(); } };
  btns.append(go, no);
  box.append(btns);
  return box;
}

async function doFiles(a) {
  X.ask = null;
  let r;
  try { r = await (a.kind === "backup" ? api.SaveBackup([a.ref], true) : api.SaveTrash([a.ref], true)); }
  catch (e) { say("fail", errText(e)); drawMid(true); return; }
  say(r.failed ? "fail" : "ok", r.text);
  if (a.kind === "recycle") await reread();
  else drawMid(true);
}

function startRename() {
  const d = X.detail;
  if (!d || d.loading || d.error) return;
  if (d.locked) return say("fail", d.locked);
  X.renaming = true;
  drawMid(true);
  const inp = midCard?.querySelector(".sv-rename input");
  inp?.focus();
  inp?.select();
}

// renameField is the inline rename (06 §2.17): duplicates are checked while typing; enter
// or leaving saves, esc cancels.
function renameField(d) {
  const box = el("div", "sv-rename");
  const inp = el("input", "xc-input");
  inp.value = d.name; inp.spellcheck = false;
  inp.setAttribute("aria-label", T("rename the save"));
  const why = el("div", "plan-why");
  why.setAttribute("role", "alert");
  let seq = 0, done = false;
  const check = async () => {
    const n = ++seq;
    const p = await api.SaveNameProblem(d.ref, inp.value).catch(() => "");
    if (n === seq) why.textContent = p;
    return n === seq && !p;
  };
  const stop = () => { if (done) return; done = true; X.renaming = false; drawMid(true); midCard?.querySelector('[data-key="rename"]')?.focus({ preventScroll: true }); };
  const commit = async () => {
    if (done) return;
    const name = inp.value.trim();
    if (!name || name === d.name) return stop();
    if (!(await check())) { inp.focus(); return; }
    done = true;
    try {
      const r = await api.SaveRename(d.ref, name);
      X.renaming = false;
      say("ok", r.text);
      await reread(r.ref);
    } catch (e) { done = false; why.textContent = errText(e); inp.focus(); }
  };
  inp.oninput = () => { void check(); };
  inp.onkeydown = (e) => {
    if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); void commit(); }
    else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); stop(); }
    else e.stopPropagation();
  };
  inp.onblur = () => { if (!done) void commit(); };
  box.append(inp, why);
  return box;
}

// liveChanged follows the game: a running game locks rename and recycle; when it exits
// the saves are read again (10 §1.7).
export function liveChanged(running) {
  if (S.screen !== "saves" || !X.page) return;
  if (!running) reread(); else showDetail(0);
}

// key: the page's keys (06 §2.11). true when taken.
export function key(e) {
  const k = e.key;
  if (e.ctrlKey || e.metaKey || e.altKey) return false;
  if (k === "/") { e.preventDefault(); input?.focus(); input?.select(); return true; }
  if (k === "F2") { e.preventDefault(); startRename(); return true; }
  if (X.ask && k === "Escape") { e.preventDefault(); X.ask = null; drawMid(true); return true; }
  if (k === "Escape" && X.q) { e.preventDefault(); if (input) input.value = ""; setQuery(""); return true; }
  // in the middle and right cards the arrows scroll them, and enter presses the focused button
  const inCards = document.activeElement?.closest?.(".sv-mid, .sv-right");
  if (k === "Enter" && !inCards) {
    const q = X.detail?.quick;
    if (q && !X.detail.loading) { e.preventDefault(); runFix(X.detail, q); return true; }
    return false;
  }
  if (inCards) return false;
  if (k === "ArrowDown" || k === "ArrowUp") { e.preventDefault(); if (document.activeElement !== tree) tree?.focus({ preventScroll: true }); moveSave(k === "ArrowDown" ? 1 : -1); return true; }
  const c = colonyOf(X.sel);
  if (k === "ArrowLeft" && c) { e.preventDefault(); X.open.delete(c.key); drawLeft(); return true; }
  if (k === "ArrowRight" && c) { e.preventDefault(); X.open.add(c.key); drawLeft(); return true; }
  return false;
}

export function hintList() {
  const q = X.detail?.quick;
  const out = [{ key: "/", label: T("search") }, { key: "↑↓", label: T("previous / next save") }, { key: "←→", label: T("fold the colony") }];
  if (q) out.push({ key: "enter", label: q.label });
  out.push({ key: "F2", label: T("rename") }, { key: "1–3", label: T("the cards") }, { key: "?", label: T("all keys") });
  return out;
}
