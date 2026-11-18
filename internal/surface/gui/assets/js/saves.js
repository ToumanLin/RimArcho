// Library › Saves (16 §2.11, 12 §4). Three cards numbered 1–3, the gaps drag (16 §3.1 rule
// 10): left the colonies, each opening to its saves as a tree, under the search's own card
// (`/`); middle the selected save: where it belongs, Can it load? with its quick fix and its
// rows by group (the file, the game, the Mod list), and the whole Mod list; right the
// playthrough, whose facts come with the full read of the save (12 §1.7 L1): skeletons
// until it is read. The listing reads every save's head, which can
// take a while on a cold library, so it arrives as a "saves" event (16 §8.4); the full read
// runs as a background task and its news arrives as "saves-read" events.
import { S, $, el, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { choiceControl } from "./choices.js";
import { on } from "./api.js";
import { beginPageRead, page, area, act, button, hintsChanged, openGameLog } from "./screens.js";
import { wireSeparator } from "./separator.js";
import { pathContent } from "./fields.js";
import { icon } from "./icons.js";
import { shimmer } from "./motion.js";
import * as pop from "./popups.js";
import { getChoices } from "./credit.js";
import { openLibraryInstance } from "./librarylink.js";
import { applyItem, focusLaunch } from "./home.js";

let ui = null;
export function initSaves(u) { ui = u; }

// page: the listing; sel: the selected save's ref; open: the colonies shown open; q: the
// search (colony and save names); owner: the Game Config filter ("" all, 12 §4.8); detail: the middle and right cards' data (for
// detail.ref); mode: what the player is editing, tied to a save (mode.ref): null when only
// viewing, {kind: "rename"} the save's name field, {kind: "colony"} the colony's (right
// card), {kind: "confirm", what: "backup" | "recycle", preview, running} the inline
// confirmation; viewing: the rows whose details View opened (by kind). built: the
// navigation (S.screenSeq) whose visit drew the page; visit: the one open() last reset for.
const X = { page: null, sel: "", open: new Set(), q: "", owner: "", detail: null, mode: null, viewing: new Set(),
  listSeq: 0, detailSeq: 0, askSeq: 0, busy: false, built: -1, visit: -1 };
let leftCard = null, midCard = null, rightCard = null, tree = null, input = null, timer = 0, mods = null;
const waiting = new Map(); // listing request → its resolve
const running = new Set(); // actions waiting for their reply: a second click does nothing
const ROW = 24; // a Mod list row's height: the list mounts only the rows in view (16 §11 A18)

// live: the page drawn in this visit is shown; news and redraws are for it only (a listing
// kept from an earlier visit is not).
const live = () => S.screen === "saves" && X.built === S.screenSeq && !!X.page;
// editing: the selected save has this editor open.
const editing = (kind) => X.mode?.kind === kind && X.mode.ref === X.sel;

on("saves", (ev) => {
  const w = waiting.get(ev?.request);
  if (!w) return;
  waiting.delete(ev.request);
  w(ev);
});
on("saves-read", (ev) => news(ev || {}));
// a list made from a save to analyse it: the save is checked again with what it indexed
on("analyze", (ev) => { if (ev?.done && live() && X.detail && !X.detail.loading) showDetail(0); });

// news is the full read's progress (16 §2.11 "Loading"): the status line while the page
// is open (never over a failure or a message with its undo), rows with their day and
// colonies regrouped, and the selected save's play facts once it is read.
function news(ev) {
  if (S.screen !== "saves") return;
  const st = $("#status"), quiet = !st || (st.dataset.kind !== "fail" && !st.querySelector("a"));
  if (ev.line && quiet) say("busy", ev.line);
  if (ev.done && (ev.error || quiet)) say(ev.error ? "fail" : "ok", ev.error || ev.text || "");
  if (!live()) return;
  if (ev.page) update(ev.page);
  if (X.sel && (ev.refs || []).includes(X.sel) && X.detail?.ref === X.sel && !X.detail.play && !X.detail.loading) showDetail(0);
  // read again for its content (an older parser's cache): ask for the content again
  else if (X.sel && (ev.refs || []).includes(X.sel) && content.ref === X.sel && content.data?.stale) loadContent(X.sel);
}

// update takes a newer listing from the full read: rows get their in-game day and colonies
// regroup by world, while the selection and the colonies shown open (by their saves) stay.
// A selected save the listing no longer has gives way to the first one, cards and all.
function update(p) {
  const shown = new Set(X.page.colonies.filter((c) => X.open.has(c.key)).flatMap((c) => c.saves.map((s) => s.ref)));
  const colony = colonyOf(X.sel)?.name, was = X.sel;
  X.open = new Set();
  setPage(p);
  for (const c of p.colonies) if (c.saves.some((s) => shown.has(s.ref))) X.open.add(c.key);
  if (!leftCard?.isConnected) return;
  const lead = $("#screen .page.saves .page-head .lead");
  if (lead && p.lead) lead.textContent = p.lead;
  drawLeft();
  if (X.sel !== was || X.detail && !X.detail.loading && colonyOf(X.sel)?.name !== colony) showDetail(0);
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

// open shows the page; want names a save to select (Can it run's last save). Coming back
// to the page starts from the listing: nothing of the last visit's cards, editors or
// confirmations stays (leave).
export async function open(want = "") {
  const read = beginPageRead("saves", { retry: () => open(want), label: "reading the saves…" });
  if (X.visit !== S.screenSeq) { X.visit = S.screenSeq; leave(); }
  if (want) selectSave(want);
  let p = null;
  // a newer listing asked for meanwhile makes this one stale: ask again, so this read ends
  while (!p) {
    try { p = await readList(); } catch (e) { read.fail(e); return; }
    if (!read.current()) return;
  }
  setPage(p);
  build();
  read.finish();
  showDetail(0);
}

// leave forgets the last visit (16 §2.11, 17 §7.3): the listing, the cards, the editor or
// confirmation open, the content and Over time's zoom, and any reply still on its way to
// them.
function leave() {
  clearTimeout(timer);
  mods?.destroy();
  mods = null;
  timeRO?.disconnect();
  timeRO = timeBox = contentBox = null;
  zoom.clear(); // kept per colony while the page is open
  content = freshContent("", content.seq + 1);
  X.page = null; X.detail = null; X.mode = null; X.viewing = new Set();
  X.detailSeq++; X.askSeq++;
  leftCard = midCard = rightCard = tree = input = null;
}

// reread reads the listing again behind the page shown (after an action, a game exit):
// the selection stays when its save is still there; want names a save to go to. true when
// the page now shows this listing.
async function reread(want = "") {
  let p;
  try { p = await readList(); } catch (e) { say("fail", errText(e)); return false; }
  if (!p || !live()) return false;
  if (want) selectSave(want);
  setPage(p);
  if (!leftCard?.isConnected) { build(); showDetail(0); return true; }
  drawLeft();
  showDetail(0);
  return true;
}

function setPage(p) {
  X.page = p;
  p.colonies ||= [];
  const all = p.colonies.flatMap((c) => c.saves);
  if (!all.some((s) => s.ref === X.sel)) selectSave(all[0]?.ref || "");
  const c = colonyOf(X.sel);
  if (c) X.open.add(c.key);
}

// selectSave makes ref the selected save: the cards, the editor or confirmation open and
// the replies on their way belong to the save before it, so they go.
function selectSave(ref) {
  if (ref === X.sel) return;
  X.sel = ref;
  X.detail = null; X.mode = null; X.viewing = new Set();
  X.detailSeq++; X.askSeq++;
}

const colonyOf = (ref) => X.page?.colonies.find((c) => c.saves.some((s) => s.ref === ref)) || null;
const rowOf = (ref) => X.page?.colonies.flatMap((c) => c.saves).find((s) => s.ref === ref) || null;

function build() {
  const p = X.page;
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = page("fill saves", p.lead || "", act(T("rescan"), "rescan", rescan));
  leftCard = midCard = rightCard = tree = input = null;
  X.built = S.screenSeq;
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
  // the search is a card of its own above the tree, unnumbered: `/` reaches it
  const leftCol = el("div", "sv-leftcol");
  const search = el("label", "card sv-find");
  input = el("input", "sv-q");
  input.type = "search";
  input.placeholder = T("search colonies and saves");
  input.setAttribute("aria-label", T("search colonies and saves"));
  input.value = X.q;
  input.autocomplete = "off"; input.spellcheck = false;
  input.oninput = () => setQuery(input.value);
  input.onkeydown = (e) => {
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); if (input.value) { input.value = ""; setQuery(""); } else focusTree(); }
    else if (e.key === "ArrowDown" || e.key === "Enter") { e.preventDefault(); e.stopPropagation(); focusTree(); }
    else e.stopPropagation(); // typing is not a page key
  };
  search.append(input, keyCaps("/"));
  leftCard = el("section", "card sv-left");
  const treeHead = el("div", "sv-cardhead");
  treeHead.append(el("span", "", T("colonies")));
  // every Game Config's saves are listed, whichever is current (12 §4.8); the filter only
  // narrows the tree, and shows when saves come from two or more configs or packs
  if (!(p.owners || []).includes(X.owner)) X.owner = "";
  if (p.owners?.length) {
    const sel = choiceControl(T("whose saves"), [{ value: "", label: T("all Game Configs") }, ...p.owners.map((o) => ({ value: o, label: o }))], X.owner);
    sel.classList.add("sv-owner-filter");
    sel.onchange = () => { X.owner = sel.value; drawLeft(); };
    treeHead.append(sel);
  }
  tree = el("div", "sv-tree");
  tree.setAttribute("role", "tree");
  tree.setAttribute("aria-label", T("colonies and their saves"));
  tree.tabIndex = 0;
  leftCard.append(treeHead, tree);
  leftCol.append(search, leftCard);
  const gap1 = el("div", "split sv-split");
  midCard = el("section", "card sv-mid");
  const gap2 = el("div", "split sv-split");
  rightCard = el("section", "card sv-right");
  grid.append(leftCol, gap1, midCard, gap2, rightCard);
  w.append(grid);
  sc.append(w);
  const setVar = (name) => (v) => { if (v) grid.style.setProperty(name, v + "px"); else grid.style.removeProperty(name); };
  const leftRange = () => [200, Math.max(200, Math.min(480, grid.clientWidth - 640))];
  wireSeparator(gap1, { key: "ui.gui.saves.left", range: leftRange,
    get: () => leftCol.getBoundingClientRect().width, set: setVar("--sv-left"), reset: () => setVar("--sv-left")(0),
    restore: (v) => setVar("--sv-left")(+v ? Math.max(leftRange()[0], Math.min(leftRange()[1], +v)) : 0) });
  const rightRange = () => [260, Math.max(260, grid.clientWidth - leftCol.getBoundingClientRect().width - 380)];
  wireSeparator(gap2, { key: "ui.gui.saves.right", direction: -1, range: rightRange,
    get: () => rightCard.getBoundingClientRect().width, set: setVar("--sv-right"), reset: () => setVar("--sv-right")(0),
    restore: (v) => setVar("--sv-right")(+v ? Math.max(rightRange()[0], Math.min(rightRange()[1], +v)) : 0) });
  area(leftCard, treeHead, () => focusTree());
  area(midCard, null, () => midCard.querySelector(".sv-body")?.focus({ preventScroll: true }));
  area(rightCard, null, () => rightCard.querySelector(".sv-body")?.focus({ preventScroll: true }));
  tree.onclick = (e) => {
    const s = e.target.closest("[data-save]");
    if (s) { pick(s.dataset.save); return; }
    const c = e.target.closest("[data-colony]");
    if (c) clickColony(c.dataset.colony);
  };
  tree.ondblclick = (e) => { if (e.target.closest("[data-save]")) startRename(); }; // keys: F2
  drawLeft();
  drawDetail();
}

// rescan reads the saves again; "read again" only when this listing is the one shown (a
// failure says itself, a newer listing has its own news).
async function rescan() {
  const busy = T("reading the saves…");
  say("busy", busy);
  if (await reread()) say("ok", T("saves read again"));
  else if ($("#status")?.textContent === busy) say("", "");
}

// refocus moves the focus where fn puts it when the focused control went with a redraw,
// never away from where the player moved it.
function refocus(fn) {
  const a = document.activeElement;
  if (!a || a === document.body || !a.isConnected) fn();
}

function focusTree() {
  if (!tree) return;
  tree.focus({ preventScroll: true });
  tree.querySelector(".sv-save.on")?.scrollIntoView({ block: "nearest" });
}

// ---- left: colonies and their saves (16 §2.11) ----

const lc = (s) => String(s || "").toLowerCase();
function saveMatch(c, s) { return !X.q || lc(c.name).includes(X.q) || lc(s.name).includes(X.q); }
function colonyMatch(c) { return (!X.owner || c.owner === X.owner) && colonyMatchQ(c); }
function colonyMatchQ(c) { return !X.q || lc(c.name).includes(X.q) || (c.game && lc(c.game).includes(X.q)) || c.saves.some((s) => lc(s.name).includes(X.q)); }

function setQuery(v) {
  X.q = lc(v.trim());
  drawLeft();
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
    row.append(chv, mark(c.name));
    if (X.page.owners?.length && c.owner) row.append(el("span", "o", c.owner));
    row.append(el("span", "k", String(c.saves.length)));
    row.setAttribute("aria-label", Tf("%s, %d saves", c.name, c.saves.length));
    if (c.game) row.title = Tf("its saves name it %s", c.game);
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
// colony folds it (16 §2.11).
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
  selectSave(ref);
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
// only where it stops); the header shows at once from the tree's row. The reply redraws
// the cards except one holding an open name field, which keeps what the player typed and
// its focus; that card is drawn when the field closes.
function showDetail(delay) {
  clearTimeout(timer);
  const ref = X.sel;
  if (!ref) { X.detail = null; drawDetail(); return; }
  if (X.detail?.ref !== ref) {
    const r = rowOf(ref);
    X.detail = { ref, name: r?.name || "", colony: colonyOf(ref)?.name || "", loading: true, issues: [], mods: [] };
    drawDetail();
  }
  const n = ++X.detailSeq;
  timer = setTimeout(async () => {
    let d;
    try { d = await api.SaveDetail(ref); } catch (e) { d = { ref, name: rowOf(ref)?.name || "", error: errText(e), issues: [], mods: [] }; }
    if (n !== X.detailSeq || !live() || X.sel !== ref) return;
    X.detail = d;
    if (!editing("rename")) drawMid(true);
    else fillTime(d); // the card keeps the name field; Over time is filled in place
    if (!editing("colony")) drawRight(true);
    hintsChanged();
    if (!d.error) loadContent(ref);
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

// anum is a card's number (1–3, the keys that focus it), as other pages put it before a
// heading (screens.js area).
function anum(card) {
  return el("span", "anum", card?.dataset.area || "");
}

function drawMid(keep) {
  if (!midCard) return;
  const d = X.detail;
  const top = keep ? midCard.querySelector(".sv-body")?.scrollTop || 0 : 0;
  const focused = midCard.contains(document.activeElement) && X.mode?.kind !== "rename" ? document.activeElement : null;
  const focusKey = focused?.dataset?.key;
  mods?.destroy();
  mods = null;
  timeRO?.disconnect();
  timeRO = timeBox = null;
  midCard.replaceChildren();
  if (!d) { midCard.append(el("div", "empty-state", T("select a save"))); return; }
  const b = body();
  b.setAttribute("aria-label", d.name || T("the save"));
  // header: the name and the save's actions, then game, list and location
  const head = el("div", "sv-head");
  const row1 = el("div", "sv-row1");
  row1.append(anum(midCard));
  if (X.mode?.kind === "rename" && X.mode.ref === d.ref && !d.loading && !d.error) row1.append(renameField(d, X.mode));
  else {
    const h = el("h2", "sv-name", d.name);
    h.ondblclick = () => startRename(); // keys: F2
    row1.append(h);
  }
  const acts = el("div", "sv-acts");
  const locked = d.locked || (d.loading ? T("reading…") : "");
  const add = (label, key, run, k, off = "", env = "") => {
    const b = button(label, k, run, "small", env);
    b.dataset.key = key;
    if (off) { b.disabled = true; b.title = off; } else if (k) b.title = k;
    acts.append(b);
    return b;
  };
  const noFile = d.loading || d.error ? T("reading…") : "";
  add(T("rename"), "rename", () => startRename(), "F2", locked || noFile);
  add(T("copy"), "copy", () => copySave(), "", noFile);
  add(T("back up"), "backup", () => askFiles("backup"), "", noFile);
  if (d.newList) add(T("make a list"), "new-list", () => newList(d, false), "", "", "new-plan").title = d.newList.command;
  if (!noFile) chooseButton(d, false, add);
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
    const open = button(T("open location"), "", () => api.OpenSaveFolder(d.ref).catch((e) => say("fail", errText(e))), "link sv-open");
    open.dataset.key = "open-folder";
    open.title = d.folder;
    loc.append(open);
    kv.append(el("dt", "", T("location")), loc);
    // whose Saves folder, when the game line does not say it (a pack's, a shared folder)
    if (d.owner) fact(T("saves of"), d.owner);
    if (d.noCloud) loc.append(el("span", "muted sv-match", T("Steam Cloud does not sync this folder")));
    head.append(kv);
  }
  b.append(head);
  if (X.mode?.kind === "confirm" && X.mode.ref === d.ref) b.append(confirmBox(X.mode));
  if (d.loading) {
    const sk = el("div", "sv-sect");
    sk.append(shimmer(T("checking the save…"), 4));
    b.append(sk);
    midCard.append(b);
    return;
  }
  if (!d.error) {
    b.append(loadSection(d));
    b.append(timeBox = el("div", "sv-sect sv-time"));
    if (d.mods?.length || d.changes?.length) b.append(modSection(d));
  }
  midCard.append(b);
  if (timeBox) fillTime(d);
  const list = b.querySelector(".sv-mods");
  if (list && !d.error) mods = modRows(b, list, modShown(d));
  b.scrollTop = top;
  mods?.draw();
  if (focusKey) midCard.querySelector(`[data-key="${CSS.escape(focusKey)}"]`)?.focus({ preventScroll: true });
}

const GROUPS = [["file", "the file"], ["load", "the last load"], ["game", "game changes"], ["mods", "Mod list changes"]];

// loadSection is Can it load? (12 §4.3): the quick-fix bar, then only what needs
// attention, under the file, the game and the Mod list.
function loadSection(d) {
  const s = el("div", "sv-sect");
  const h = el("h3", "", T("can it load?"));
  h.append(el("span", "r", d.switch ? "" : d.against || ""));
  s.append(h);
  // in another Game Config's or a pack's Saves folder: the game started now does not list
  // it (12 §4.8); Copy to … puts the file into the current config's folder
  if (d.unseen) {
    const u = el("div", "sv-unseen");
    u.append(el("span", "t", d.unseen));
    if (d.copyHere) {
      const b = button(Tf("copy to the saves of %s", d.copyHere), "", () => copyHere(d), "small", "save-copy-to");
      b.dataset.key = "copy-here";
      b.title = T("copies only the save file; Mods, lists and settings stay as they are");
      u.append(b);
    }
    s.append(u);
  }
  // The save's own game and list, when the Launch page applied others: how it loads with
  // them, and Switch and launch; the bar and the rows below are then the current ones'.
  if (d.switch) {
    const w = d.switch;
    const sw = el("div", "sv-fixbar sv-switch" + levelClass(w.level));
    sw.setAttribute("role", "group");
    sw.setAttribute("aria-label", w.text);
    sw.append(el("span", "t", w.text));
    const b = fixButton(d, { action: "switch", label: w.label }, true);
    b.append(keyCaps("enter"));
    sw.append(b);
    s.append(sw, el("p", "sv-cur", d.against));
  }
  // green only when it loads with its list; no list is neither good nor bad
  const cls = d.level === "writing" || (!d.againstKind && !d.level) ? " busy" : levelClass(d.level);
  const bar = el("div", "sv-fixbar" + cls);
  bar.setAttribute("role", "group");
  bar.setAttribute("aria-label", d.verdict);
  bar.append(el("span", "t", d.verdict));
  if (d.quick) {
    const q = fixButton(d, d.quick, true);
    q.append(keyCaps("enter"));
    bar.append(q);
  }
  s.append(bar);
  for (const [g, title] of GROUPS) {
    const rows = (d.issues || []).filter((is) => (is.group || "mods") === g);
    if (!rows.length) continue;
    s.append(el("h4", "sv-group", T(title)));
    const list = el("div", "sv-issues");
    for (const is of rows) list.append(issueRow(d, is));
    s.append(list);
  }
  return s;
}

function levelClass(level) { return level === "error" ? " bad" : level === "warning" ? " warn" : " ok"; }

function issueRow(d, is) {
  const row = el("div", "sv-iss " + (is.level || ""));
  row.setAttribute("role", "group");
  row.setAttribute("aria-label", is.head);
  const sign = el("span", "sign " + (is.sign === "−" ? "m" : is.sign === "+" ? "p" : "o"), is.sign || (is.level === "error" ? "✗" : is.level === "warning" ? "!" : "i"));
  sign.setAttribute("aria-hidden", "true");
  const head = el("span", "h", is.head);
  if (is.title) head.title = is.title;
  const fixes = el("span", "f");
  for (const f of is.fixes || []) fixes.append(fixButton(d, f, false));
  const open = X.viewing.has(is.kind);
  if (is.details) {
    const v = button(open ? T("hide") : T("view"), "", () => {
      if (X.viewing.has(is.kind)) X.viewing.delete(is.kind); else X.viewing.add(is.kind);
      drawMid(true);
    }, "small sv-fix");
    v.dataset.key = "view-" + is.kind;
    v.setAttribute("aria-expanded", String(open));
    fixes.append(v);
  }
  row.append(sign, head, fixes, el("span", "c", is.consequence));
  if (open && is.details) row.append(detailsBox(is.details));
  return row;
}

// detailsBox is View's list (16 §2.11): each Mod with its packageId, then what loading
// without them loses, by kind.
function detailsBox(x) {
  const box = el("div", "sv-details");
  const ul = el("ul", "sv-dmods");
  for (const m of x.mods || []) {
    const li = el("li", "");
    li.append(el("span", "n", m.name), el("code", "", m.packageId));
    if (m.note) li.append(el("span", "muted", m.note));
    ul.append(li);
  }
  box.append(ul);
  for (const l of x.lost || []) {
    const p = el("p", "sv-lost");
    p.append(el("b", "", l.kind), " " + l.names.join(T(", ")) + (l.more ? " " + Tf("and %d more", l.more) : ""));
    box.append(p);
  }
  return box;
}

function fixButton(d, f, primary) {
  const env = { fix: "save-fix", subscribe: "save-fix", enable: "save-fix", mods: "save-fix", "disable-added": "save-fix", order: "save-fix",
    recycle: "recycle", "new-list": "new-plan", "new-list-analyse": "new-plan" }[f.action] || "";
  const b = button(f.label, "", () => runFix(d, f), primary ? "primary small" : "small sv-fix", env);
  b.dataset.key = "fix-" + f.action + "-" + (f.target || "");
  b.title = f.action === "switch" ? T("make this game and list current, then open the Launch page") : f.command;
  if (f.action === "recycle" && d.locked) { b.disabled = true; b.title = d.locked; }
  return b;
}

// The Mod list (12 §4.3, 16 §2.11) shows only how the list the save is checked against
// differs from the save: the list's Mods the save lacks (green +) and the save's Mods the
// list lacks (red −), order ignored. Past MOD_FOLD changes the rest fold behind Show all;
// Show the full list unfolds the save's whole list in load order with the changes marked,
// then the added Mods. A save with no list or pack has no changes: its list is folded
// whole. The choice stays while the same save is selected.
const MOD_FOLD = 12;
let modOpen = { ref: "", mode: "" }; // mode: "changes" (all of them) or "full"

function modMode(d) { return modOpen.ref === d.ref ? modOpen.mode : ""; }
function modItems(d, mode) {
  if (mode === "full" || !d.againstKind) return [...(d.mods || []), ...(d.changes || []).filter((m) => m.change === "added")];
  return d.changes || [];
}
function modShown(d) {
  const mode = modMode(d), all = modItems(d, mode);
  if (mode) return all;
  return d.againstKind ? all.slice(0, MOD_FOLD) : [];
}

// modSection is the heading, what the changes are against, the rows (mounted as the card
// scrolls, modRows) and the folds.
function modSection(d) {
  const s = el("div", "sv-sect");
  const mode = modMode(d), shown = modShown(d);
  const changes = d.changes || [], mods = d.mods || [];
  const h = el("h3", "", d.againstKind ? T("mod changes") : T("mod list"));
  h.append(el("span", "r", d.againstKind ? d.against : Tf("this save's %d Mods", mods.length)));
  s.append(h);
  if (d.againstKind) {
    const added = changes.filter((m) => m.change === "added").length;
    if (!changes.length) s.append(el("p", "sv-none", T("the list has the same Mods as this save")));
    else {
      const counts = el("p", "sv-mcount");
      if (added) counts.append(el("span", "add", Tf("%d added", added)));
      if (changes.length - added) counts.append(el("span", "del", Tf("%d removed", changes.length - added)));
      s.append(counts);
    }
  }
  const list = el("div", "sv-mods" + (mode === "full" || !d.againstKind ? " full" : ""));
  list.setAttribute("role", "list");
  list.setAttribute("aria-label", mode === "full" || !d.againstKind ? T("mod list") : T("mod changes"));
  list.style.height = shown.length * ROW + "px";
  s.append(list);
  const folds = el("p", "sv-folds");
  const fold = (label, to, key, open) => {
    const b = button(label, "", () => { modOpen = { ref: d.ref, mode: to }; drawMid(true); }, "link sv-fold");
    b.dataset.key = key;
    b.setAttribute("aria-expanded", String(open));
    folds.append(b);
  };
  if (d.againstKind && changes.length > MOD_FOLD && mode !== "full") {
    if (mode === "changes") fold(T("show fewer"), "", "mod-fold", true);
    else fold(Tf("show all %d changes", changes.length), "changes", "mod-fold", false);
  }
  if (mods.length) {
    if (mode === "full") fold(d.againstKind ? T("show only the changes") : T("show fewer"), "", "mod-full", true);
    else fold(Tf("show the full list (%d Mods)", mods.length), "full", "mod-full", false);
  }
  if (folds.childElementCount) s.append(folds);
  return s;
}

// modRows keeps the Mod list's rows in view mounted, plus a few on each side: a save of a
// thousand Mods stays within the page's size limits (16 §11 A18).
function modRows(scroll, list, all) {
  const shown = new Map();
  const full = list.classList.contains("full"); // order marks only in the full list
  let frame = 0;
  const row = (i) => {
    const m = all[i];
    const add = m.change === "added";
    const r = el("div", "r" + (m.change ? (add ? " add" : " del") : "") + (m.missing ? " gone" : ""));
    r.setAttribute("role", "listitem");
    r.setAttribute("aria-setsize", String(all.length));
    r.setAttribute("aria-posinset", String(i + 1));
    r.style.top = i * ROW + "px";
    // a change has its sign; in the full list the others have their place in the save
    const sign = m.change ? el("i", "sg", add ? "+" : "−") : el("i", "", String(i + 1));
    if (m.change) {
      sign.title = add ? T("in the list, not in this save") : T("in this save, not in the list");
      sign.setAttribute("aria-label", sign.title);
    }
    r.append(sign);
    if (m.library) {
      const n = el("button", "n lk", m.name);
      n.type = "button";
      n.title = m.packageId + "  " + T("show in the mod library");
      n.onclick = () => openLibraryInstance(m.library);
      r.append(n);
    } else {
      const n = el("span", "n", m.name);
      n.title = m.packageId;
      r.append(n);
    }
    if (m.missing) r.append(el("span", "tagx bad", T("not installed")));
    if (m.moved && full) { const mv = el("b", "mv", "↕"); mv.title = T("in another order in the list"); mv.setAttribute("aria-label", T("in another order in the list")); r.append(mv); }
    return r;
  };
  const draw = () => {
    frame = 0;
    if (!list.isConnected) return;
    const at = list.getBoundingClientRect().top - scroll.getBoundingClientRect().top + scroll.scrollTop;
    const from = Math.max(0, Math.floor((scroll.scrollTop - at) / ROW) - 8);
    const to = Math.min(all.length, Math.ceil((scroll.scrollTop + scroll.clientHeight - at) / ROW) + 8);
    for (const [i, r] of shown) if (i < from || i >= to) { r.remove(); shown.delete(i); }
    for (let i = from; i < to; i++) if (!shown.has(i)) { const r = row(i); shown.set(i, r); list.append(r); }
  };
  const later = () => { if (!frame) frame = requestAnimationFrame(draw); };
  scroll.addEventListener("scroll", later, { passive: true });
  const ro = new ResizeObserver(later);
  ro.observe(scroll);
  return { draw, destroy() { cancelAnimationFrame(frame); ro.disconnect(); scroll.removeEventListener("scroll", later); } };
}

// drawRight is the colony and what its save needs (16 §2.11 "Right"): the colony, its day
// and date, storyteller, difficulty and play time; then the save's Mods by what removing
// one costs (12 §4.7). Skeletons while the save is read.
function drawRight(keep) {
  if (!rightCard) return;
  const d = X.detail;
  if (keep && d && !d.loading && redrawAroundFind(d)) return;
  const top = keep ? rightCard.querySelector(".sv-body")?.scrollTop || 0 : 0;
  const focused = rightCard.contains(document.activeElement) ? document.activeElement.dataset?.key || "" : null;
  rightCard.replaceChildren();
  if (!d) return;
  const b = body();
  b.setAttribute("aria-label", T("the colony and what its save needs"));
  b.append(rightHead(d));
  if (d.loading) { rightCard.append(b); return; }
  b.append(contentBox = contentSection(d));
  rightCard.append(b);
  b.scrollTop = top;
  if (focused !== null) (focused && b.querySelector(`[data-key="${CSS.escape(focused)}"]`) || b).focus({ preventScroll: true });
}

// redrawAroundFind redraws the right card around the find box while it has the focus:
// the box itself stays in the page, so its text, caret and an IME composition in progress
// survive a redraw (a saves-read event, an analysis, a group folded). false: draw it anew.
function redrawAroundFind(d) {
  const old = contentBox?.isConnected && contentBox.querySelector('[data-key="need-find"]');
  if (!old || document.activeElement !== old || content.ref !== d.ref) return false;
  const head = rightCard.querySelector(".sv-head");
  if (!head) return false;
  head.replaceWith(rightHead(d));
  return redrawContent();
}

// rightHead is the right card's header: the colony's name and its play facts.
function rightHead(d) {
  const p = d.play;
  const head = el("div", "sv-head");
  const row = el("div", "sv-row1");
  row.append(anum(rightCard));
  const colEdit = X.mode?.kind === "colony" && X.mode.ref === d.ref && !!d.colonyKey;
  if (colEdit) row.append(colonyField(d, X.mode));
  else {
    // the colony's name in RimArcho (12 §4.1): double-click or Rename names it; the
    // name its saves give it stays on hover
    const h = el("h2", "sv-name", d.colony || "");
    if (d.colonyGame) h.title = Tf("its saves name it %s", d.colonyGame);
    if (d.colonyKey) h.ondblclick = () => startColonyRename(); // keys: the rename button beside the name
    row.append(h);
  }
  if (p) row.append(el("span", "muted", [p.day, p.date].filter(Boolean).join("  ")));
  if (d.colonyKey && !colEdit) {
    const rn = button(T("rename"), "", () => startColonyRename(), "small sv-colrename");
    rn.dataset.key = "rename-colony";
    rn.title = T("name this colony in RimArcho; the save files keep their names");
    row.append(rn);
  }
  if (d.colonyKey && !d.loading && !d.error) chooseButton(d, true, (label, key, run, k, off, env) => {
    const b = button(label, "", run, "small sv-colrename", env);
    b.dataset.key = key;
    row.append(b);
    return b;
  });
  head.append(row);
  if (p) {
    const kv = el("dl", "sv-kv");
    for (const [k, v] of [[T("storyteller"), p.storyteller], [T("difficulty"), p.difficulty], [T("play time"), p.played]]) {
      if (v) kv.append(el("dt", "", k), el("dd", "", v));
    }
    if (kv.childNodes.length) head.append(kv);
  }
  if (p?.note) head.append(el("p", "sv-note warn", p.note));
  return head;
}

// ---- right: what the save needs of its Mods (12 §4.7) ----

// content: the selected save's content (API.SaveContent) once asked for; q: the find
// box's text; open: the verdict groups unfolded; all: the groups listing every Mod, not the
// first NEED_FOLD; opened: the Mods (by packageId, "?" for the Defs of no installed Mod)
// whose names are shown.
const freshContent = (ref, seq) => ({ ref, seq, data: null, error: "", q: "", open: new Set(["breaks", "loses"]), all: new Set(), opened: new Set() });
let content = freshContent("", 0);
// contentBox is the section in the right card, redrawn on its own (redrawContent) so a
// colony name being typed in the card's header stays.
let contentBox = null;

// The find box is kept when the new section has one too: the nodes around it are
// replaced, it is not (moving a focused input blurs it and ends an IME composition).
function redrawContent() {
  const d = X.detail;
  if (!d || !contentBox?.isConnected) { drawRight(true); return false; }
  const key = contentBox.contains(document.activeElement) ? document.activeElement.dataset?.key : "";
  const n = contentSection(d);
  const old = contentBox.querySelector('[data-key="need-find"]');
  const nf = n.querySelector('[data-key="need-find"]');
  if (old && nf && content.ref === d.ref) {
    const kids = [...n.childNodes], at = kids.indexOf(nf);
    while (old.previousSibling) old.previousSibling.remove();
    while (old.nextSibling) old.nextSibling.remove();
    old.before(...kids.slice(0, at));
    old.after(...kids.slice(at + 1));
    // the new section's handler reads its own (detached) box: read the kept one
    const fill = nf.oninput;
    old.oninput = () => { nf.value = old.value; fill(); };
    contentBox.className = n.className;
  } else {
    contentBox.replaceWith(n);
    contentBox = n;
  }
  if (key && contentBox.querySelector(`[data-key="${CSS.escape(key)}"]`) !== document.activeElement) {
    contentBox.querySelector(`[data-key="${CSS.escape(key)}"]`)?.focus({ preventScroll: true });
  }
  return true;
}
const NEED_FOLD = 8;

// loadContent asks for the save's content after its check: attributing thousands of Defs
// is a separate call, so the cards above never wait for it.
async function loadContent(ref) {
  if (content.ref !== ref) content = freshContent(ref, content.seq);
  const n = ++content.seq;
  let c, err = "";
  try { c = await api.SaveContent(ref); } catch (e) { err = errText(e); }
  if (n !== content.seq || content.ref !== ref || X.sel !== ref || S.screen !== "saves") return;
  content.data = c || null;
  content.error = err;
  redrawContent();
}

// contentSection says what the save needs of each of its Mods, whatever list it is checked
// against: a bar for how the Mods split, a find box, then the Mods by what removing one
// costs, heaviest group first.
function contentSection(d) {
  const s = el("div", "sv-sect sv-content");
  const c = content.ref === d.ref ? content.data : null;
  const h = el("h3", "", T("what this save needs"));
  s.append(h);
  if (content.ref === d.ref && content.error) { s.append(el("p", "err", content.error)); return s; }
  if (!c || c.stale || (!c.read && d.reading)) {
    s.append(shimmer(c?.stale ? T("reading the save for its content…") : d.reading ? T("reading the save…") : T("finding the Mods of its content…"), 5));
    return s;
  }
  const nd = c.needs;
  if (!c.read || !nd) {
    // why it is not read (it failed, the game is writing it) before "not read yet"
    s.append(el("p", "sv-note muted", d.playNote || c.note || T("the save is not read in full yet")));
    return s;
  }
  h.append(el("span", "r", Tf("%d Mods", nd.mods)));
  if (c.note) s.append(el("p", "sv-note warn", c.note));
  const groups = nd.groups || [];
  const bar = el("div", "sv-nbar");
  bar.setAttribute("role", "img");
  bar.setAttribute("aria-label", groups.map((g) => Tf("%d %s", g.mods.length, g.label)).join(T(", ")));
  for (const g of groups) {
    const i = el("i", "k-" + g.key);
    i.style.flex = String(g.mods.length);
    i.title = Tf("%d %s", g.mods.length, g.label);
    bar.append(i);
  }
  s.append(bar);
  const find = el("input", "sv-nfind");
  find.type = "search";
  find.placeholder = T("find a Mod: can it be removed?");
  find.setAttribute("aria-label", find.placeholder);
  find.dataset.key = "need-find";
  find.value = content.q;
  const list = el("div", "sv-needs");
  list.setAttribute("aria-label", T("the save's Mods by what removing one costs"));
  // typing redraws only the groups, so the box keeps its focus and caret
  find.oninput = () => { content.q = find.value; fillNeeds(list, d, groups); };
  s.append(find, list);
  fillNeeds(list, d, groups);
  if (nd.game) s.append(el("p", "sv-note muted", Tf("part of the game, always needed: %s", nd.game)));
  if (c.unknown) {
    // Defs no installed Mod of the save defines (12 §1.5)
    const u = c.unknown, open = content.opened.has("?");
    const p = el("p", "sv-note muted", Tf("%d Defs in the save come from no installed Mod", u.count) + " ");
    p.title = T("no installed Mod of this save defines them: a Mod update removed them, or their Mod is not installed");
    const tg = button(open ? T("hide the names") : T("show the names"), "", () => { if (open) content.opened.delete("?"); else content.opened.add("?"); redrawContent(); }, "link sv-fold");
    tg.dataset.key = "need-unknown";
    tg.setAttribute("aria-expanded", String(open));
    p.append(tg);
    s.append(p);
    if (open) {
      const names = u.names || [];
      s.append(el("p", "sv-lost", names.join(T(", ")) + (u.count > names.length ? " " + Tf("and %d more", u.count - names.length) : "")));
    }
  }
  // what a save cannot tell (12 §2.3): said, folded, so no verdict reads as more than it is
  const lim = el("details", "sv-limits");
  lim.append(el("summary", "", T("what a save cannot tell")));
  const ul = el("ul");
  for (const l of [T("a Def in the save means it was present, not how much it was played"), ...(c.limits || [])]) ul.append(el("li", "", l));
  lim.append(ul);
  s.append(lim);
  return s;
}

// fillNeeds draws the verdict groups; with text in the find box, only the Mods it matches,
// every group unfolded.
function fillNeeds(list, d, groups) {
  list.replaceChildren();
  const q = lc(content.q.trim());
  for (const g of groups) {
    const mods = q ? g.mods.filter((m) => lc(m.name).includes(q) || lc(m.packageId).includes(q)) : g.mods;
    if (q && !mods.length) continue;
    const open = !!q || content.open.has(g.key);
    const sec = el("section", "sv-need k-" + g.key);
    const hd = el("button", "gh");
    hd.type = "button";
    hd.dataset.key = "need-" + g.key;
    hd.setAttribute("aria-expanded", String(open));
    const chv = el("span", "chv" + (open ? " open" : "")); chv.innerHTML = icon.chev;
    hd.append(el("i", "dot"), el("span", "l", g.label), el("span", "k", String(mods.length)), chv);
    hd.onclick = () => {
      if (q) return; // a search shows every group it matches
      if (content.open.has(g.key)) content.open.delete(g.key); else content.open.add(g.key);
      redrawContent();
    };
    sec.append(hd, el("p", "hint", g.hint));
    if (open) {
      const ul = el("ul", "sv-nmods");
      const cut = q || content.all.has(g.key) ? mods.length : Math.min(mods.length, NEED_FOLD);
      for (const m of mods.slice(0, cut)) ul.append(needMod(m));
      sec.append(ul);
      const folds = el("p", "sv-folds");
      if (mods.length > cut) {
        const more = button(Tf("%d more", mods.length - cut), "", () => { content.all.add(g.key); redrawContent(); }, "link sv-fold");
        more.dataset.key = "need-all-" + g.key;
        folds.append(more);
      }
      if (g.key === "unchecked") {
        // analysing a list made from the save reads its Mods; the card asks again when
        // the analysis is done
        const go = button(T("make a list from this save and analyse it"), "", () => newList(d, true), "link sv-fold", "new-plan");
        go.dataset.key = "content-analyse";
        folds.append(go);
      }
      if (folds.childElementCount) sec.append(folds);
    }
    list.append(sec);
  }
  if (!list.childElementCount) list.append(el("p", "sv-none", q ? T("no Mod of this save matches") : T("this save names no Mod besides the game")));
}

// needMod is one Mod with what it holds in the save; opening it shows the names.
function needMod(m) {
  const li = el("li", "sv-nmod");
  const open = content.opened.has(m.packageId);
  const more = (m.parts || []).length > 0;
  // a row of text, not a button, so its name and summary can be selected; the chevron
  // opens it and the name is the link to the library
  const row = el("div", "h");
  if (more) {
    const tg = el("button", "tg" + (open ? " open" : ""));
    tg.type = "button";
    tg.innerHTML = icon.chev;
    tg.dataset.key = "need-mod-" + m.packageId;
    tg.title = open ? T("hide what it holds") : T("show what it holds");
    tg.setAttribute("aria-label", tg.title + T(": ") + m.name);
    tg.setAttribute("aria-expanded", String(open));
    tg.onclick = () => {
      if (open) content.opened.delete(m.packageId); else content.opened.add(m.packageId);
      redrawContent();
    };
    row.append(tg);
  }
  let name;
  if (m.library) {
    name = el("a", "n lk", m.name);
    name.setAttribute("role", "link");
    name.tabIndex = 0;
    name.dataset.key = "need-lib-" + m.packageId;
    name.title = m.packageId + "  " + T("show in the mod library");
    // a drag over the name selects it; a plain click follows it
    name.onclick = () => { if (String(getSelection()) === "") openLibraryInstance(m.library); };
    name.onkeydown = (e) => { if (e.key === "Enter") openLibraryInstance(m.library); };
  } else {
    name = el("span", "n", m.name);
    name.title = m.packageId;
  }
  const sum = el("span", "c", m.sum || "");
  if (m.sum) sum.title = m.sum; // the line may be cut short
  row.append(name, sum);
  li.append(row);
  if (open && more) {
    const box = el("div", "sv-details");
    for (const x of m.parts || []) {
      const p = el("p", "sv-lost");
      const names = x.names || [];
      p.append(el("b", "", x.label), names.length ? " " + names.join(T(", ")) + (x.more ? " " + Tf("and %d more", x.more) : "") : "");
      box.append(p);
    }
    li.append(box);
  }
  return li;
}

// ---- middle: the colony over time (12 §4.5) ----

// timeBox is Over time's section in the middle card, drawn on its own so the full read's
// news redraws it without the rest of the card (a rename in progress stays). zoom is how
// many times the card's width the chart is drawn, kept per colony: a long colony's saves
// sit a few pixels apart until zoomed in.
let timeBox = null, timeRO = null;
const zoom = new Map(); // colony key → zoom, while the page is open
const ZOOM_MAX = 64;

function zoomOf() {
  return zoom.get(colonyOf(X.sel)?.key || "") || 1;
}

// fillTime draws Over time: skeletons while the save is read, nothing when it has no
// timeline. at keeps a place of the chart under the same pixel after a zoom: ratio is the
// place along the whole chart, px where it was in view.
function fillTime(d, at = null) {
  const s = timeBox;
  if (!s || !s.isConnected || d !== X.detail) return;
  const tl = d.play?.timeline;
  s.replaceChildren();
  s.hidden = !tl && !d.reading;
  if (s.hidden) return;
  const h = el("h3", "", T("over time"));
  s.append(h);
  if (!tl) { s.append(shimmer(T("reading the save…"), 2)); return; }
  const z = zoomOf();
  const r = el("span", "r sv-zoom");
  r.append(el("span", "", T("a point opens its save")));
  const zb = (label, key, title, to, off) => {
    const b = button(label, "", () => setZoom(to(), zoomAnchor()), "small sv-zb");
    b.dataset.key = key;
    b.title = title;
    b.setAttribute("aria-label", title);
    if (off) b.disabled = true;
    r.append(b);
  };
  zb("−", "zoom-out", T("zoom out (ctrl+wheel)"), () => zoomOf() / 2, z <= 1);
  zb("+", "zoom-in", T("zoom in (ctrl+wheel)"), () => zoomOf() * 2, z >= ZOOM_MAX);
  zb(T("fit"), "zoom-fit", T("show the whole colony"), () => 1, z <= 1);
  h.append(r);
  const sc = el("div", "sv-chartscroll" + (z > 1 ? " zoomed" : ""));
  const width = s.clientWidth - 32;
  sc.append(chart(tl, Math.max(240, width) * z));
  s.append(sc);
  const p = d.play;
  const dead = (p.colonists || []).filter((c) => c.dead).map((c) => c.name);
  const legend = el("p", "sv-legend");
  if (tl.wealth) legend.append(el("span", "w", T("wealth")));
  if (tl.people) legend.append(el("span", "p", T("colonists")));
  if (tl.deaths?.length) legend.append(el("span", "x", T("deaths")));
  const names = dead.join(T(", ")) + (p.dead > dead.length ? " " + Tf("and %d more", p.dead - dead.length) : "");
  legend.append(el("span", "muted", p.dead ? Tf("%d dead: %s", p.dead, names) : T("no colonist has died")));
  s.append(legend);
  const sel = sc.querySelector(".pt.sel");
  // the selected save stays under its pixel; points sit inside a margin, so a ratio of the
  // width would drift them (12 px a step at the right edge)
  if (at?.selPx !== undefined && sel) sc.scrollLeft = +sel.getAttribute("cx") - at.selPx;
  else if (at) sc.scrollLeft = at.ratio * sc.scrollWidth - at.px;
  else if (z > 1) {
    // the selected save in view
    if (sel) sc.scrollLeft = +sel.getAttribute("cx") - sc.clientWidth / 2;
  }
  wireZoom(sc);
  // the card's width changes with the window and the gaps: draw again at the new width
  timeRO?.disconnect();
  let w0 = s.clientWidth, frame = 0;
  timeRO = new ResizeObserver(() => {
    if (s.clientWidth === w0 || frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      w0 = s.clientWidth;
      const c = s.querySelector(".sv-chartscroll");
      if (c) fillTime(X.detail, { ratio: (c.scrollLeft + c.clientWidth / 2) / c.scrollWidth, px: c.clientWidth / 2 });
    });
  });
  timeRO.observe(s);
}

// zoomAnchor: the buttons and keys zoom around the selected save while it is in view, else
// around the middle (VM 2026-11-09: from the whole colony, + lost the newest save off the
// right edge).
function zoomAnchor() {
  const c = timeBox?.querySelector(".sv-chartscroll"), p = c?.querySelector(".pt.sel");
  if (!p) return 0.5;
  const f = (+p.getAttribute("cx") - c.scrollLeft) / c.clientWidth;
  return f >= 0 && f <= 1 ? f : 0.5;
}

// setZoom draws the chart at z, keeping the place at frac of the view (0 left, 1 right)
// where it was.
function setZoom(z, frac) {
  z = Math.max(1, Math.min(ZOOM_MAX, z));
  const old = zoomOf();
  if (Math.abs(z - old) < 0.01) return;
  const sc = timeBox?.querySelector(".sv-chartscroll");
  const px = (sc?.clientWidth || 0) * frac;
  const ratio = sc ? (sc.scrollLeft + px) / sc.scrollWidth : 0.5;
  const p = sc?.querySelector(".pt.sel");
  const selPx = p ? +p.getAttribute("cx") - sc.scrollLeft : NaN;
  zoom.set(colonyOf(X.sel)?.key || "", z < 1.05 ? 1 : z);
  const keep = timeBox?.contains(document.activeElement) ? document.activeElement.dataset?.key : "";
  fillTime(X.detail, Math.abs(selPx - px) < 1 ? { selPx } : { ratio, px });
  const f = keep && timeBox?.querySelector(`[data-key="${CSS.escape(keep)}"]`);
  if (f && !f.disabled) f.focus({ preventScroll: true });
  else if (keep) timeBox?.querySelector(".sv-zb:not(:disabled)")?.focus({ preventScroll: true });
}

// wireZoom: ctrl+wheel zooms around the pointer; dragging a zoomed chart pans it; + − 0
// zoom while the section has the focus.
function wireZoom(sc) {
  sc.addEventListener("wheel", (e) => {
    if (!e.ctrlKey) return;
    e.preventDefault();
    const r = sc.getBoundingClientRect();
    setZoom(zoomOf() * (e.deltaY < 0 ? 1.25 : 0.8), (e.clientX - r.left) / r.width);
  }, { passive: false });
  let drag = null;
  sc.onpointerdown = (e) => {
    if (e.button !== 0 || zoomOf() <= 1 || e.target.closest?.("[data-save]")) return;
    drag = { x: e.clientX, left: sc.scrollLeft, id: e.pointerId };
    sc.setPointerCapture(e.pointerId);
    sc.classList.add("drag");
  };
  sc.onpointermove = (e) => { if (drag && e.pointerId === drag.id) sc.scrollLeft = drag.left - (e.clientX - drag.x); };
  sc.onpointerup = sc.onpointercancel = (e) => { if (drag && e.pointerId === drag.id) { drag = null; sc.classList.remove("drag"); } };
  timeBox.onkeydown = (e) => {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    const to = e.key === "+" || e.key === "=" ? zoomOf() * 2 : e.key === "-" ? zoomOf() / 2 : e.key === "0" ? 1 : 0;
    if (!to) return;
    e.preventDefault(); e.stopPropagation();
    setZoom(to, zoomAnchor());
  };
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

// dayStep is the days between the axis's labels: 1, 2, 5, 10, 20, 50… so that labels
// stand about 90 px apart.
function dayStep(days, width) {
  const raw = days / Math.max(1, Math.floor(width / 90));
  const e = Math.pow(10, Math.floor(Math.log10(Math.max(1, raw))));
  for (const m of [1, 2, 5, 10]) if (m * e >= raw) return m * e;
  return 10 * e;
}

// chart is the colony over in-game days (12 §4.5), w pixels wide: this save's wealth (line
// and area) and colonists (dashed) from its own history, a mark per death letter, and a
// point per read save of the colony; a point opens its save. The tree reaches every save
// by keys, so the points are for the mouse; the chart reads as one image.
function chart(tl, w) {
  const h = 140, L = 12, R = 12, TOP = 10, B = 20;
  const days = Math.max(1, tl.days), wealth = tl.wealth || [], people = tl.people || [];
  const maxW = Math.max(1, ...wealth, ...tl.points.map((p) => p.wealth));
  const maxP = Math.max(1, ...people);
  const x = (d) => L + (Math.min(d, days) / days) * (w - L - R);
  const y = (v, max) => h - B - (v / max) * (h - B - TOP);
  const along = (vals, max) => vals.map((v, i) => `${x(tl.upto * i / Math.max(1, vals.length - 1)).toFixed(1)},${y(v, max).toFixed(1)}`).join(" ");
  const svg = svgEl("svg", { class: "sv-chart", width: w, height: h, viewBox: `0 0 ${w} ${h}`, role: "img",
    "aria-label": Tf("wealth and colonists over %d days, %d saves", days, tl.points.length) });
  svg.append(svgEl("line", { class: "ax", x1: L, x2: w - R, y1: h - B, y2: h - B }));
  const step = dayStep(days, w - L - R);
  const ticks = [];
  for (let t = 0; t <= days; t += step) ticks.push(t);
  // the last day, unless a label stands too close to it
  if (x(days) - x(ticks[ticks.length - 1]) > 60) ticks.push(days);
  for (const t of ticks) {
    const end = x(days) - x(t) < 30;
    svg.append(svgEl("line", { class: "tk", x1: x(t), x2: x(t), y1: h - B, y2: h - B + 3 }));
    svg.append(svgEl("text", { x: x(t), y: h - 5, "text-anchor": t === 0 ? "start" : end ? "end" : "middle" }, Tf("day %d", t)));
  }
  for (const d of tl.deaths || []) svg.append(svgEl("line", { class: "ev", x1: x(d), x2: x(d), y1: TOP, y2: h - B }));
  if (wealth.length > 1) {
    const line = along(wealth, maxW);
    svg.append(svgEl("polygon", { class: "area", points: `${x(0)},${h - B} ${line} ${x(tl.upto)},${h - B}` }));
    svg.append(svgEl("polyline", { class: "ln", points: line }));
  }
  if (people.length > 1) svg.append(svgEl("polyline", { class: "pop", points: along(people, maxP) }));
  // saves of one day sit side by side, the last on the day, so each can be clicked once
  // zoomed (VM 2026-11-09: saves minutes apart covered each other at every zoom)
  const left = new Map(), count = new Map();
  for (const pt of tl.points) count.set(pt.day, (count.get(pt.day) || 0) + 1);
  for (const pt of tl.points) {
    const n = count.get(pt.day) - 1;
    count.set(pt.day, n);
    left.set(pt, n);
  }
  // the selected point last, so a neighbour never covers it
  for (const pt of [...tl.points].sort((p, q) => (p.ref === X.sel) - (q.ref === X.sel))) {
    const on = pt.ref === X.sel, off = pt.wealth < 0;
    const cx = Math.max(L, x(pt.day) - left.get(pt) * 11), cy = off ? h - B : y(pt.wealth, maxW), r = pt.auto ? 3.5 : 5;
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

// ---- actions (12 §4.4) ----

async function runFix(d, f) {
  switch (f.action) {
    case "recycle": return askFiles("recycle");
    case "open-save": return pick(f.target);
    case "open-log": return openGameLog(f.target, f.cluster);
    case "new-list": return newList(d, false);
    case "new-list-analyse": return newList(d, true);
    case "go-launch": return goLaunch();
    case "switch": return switchTo(d.switch);
    case "enable": case "subscribe": return fix(d, [f.target], false, false);
    case "mods": return fix(d, f.target.split(" "), false, false); // a folded row: all of its Mods
    case "disable-added": return fix(d, [], true, false);
    case "order": return fix(d, [], false, true);
  }
}

// fix applies the save's fixes to the list it is checked against (one undo step), then
// subscribes to the Workshop Mods the fix added, after the usual Steam / SteamCMD choice.
// It never launches: the game starts only from the Launch page (16 §2.11).
async function fix(d, mods, disableAdded, order) {
  if (X.busy) return;
  X.busy = true;
  let r;
  try { r = await api.SaveFix(d.ref, mods, disableAdded, order); }
  catch (e) { X.busy = false; say("fail", errText(e)); return; }
  X.busy = false;
  // u and the status line's undo take the edit back, and the save is checked again
  say("ok", r.text, r.undo ? async () => { await ui.verb("undo"); if (live()) showDetail(0); } : false);
  await ui?.reload?.();
  if (live()) showDetail(0);
  const ids = r.workshopIds || [];
  if (!ids.length) return;
  pop.choose(Tf("get %d mods", ids.length), getChoices(), async (i) => {
    try { say("ok", await api.Subscribe({ kind: "workshop", url: ids.join(" "), steamcmd: i === 1, batch: true })); }
    catch (e) { say("fail", errText(e)); }
  });
}

// The game starts only from the Launch page (16 §2.11): Launch opens it with its button
// focused; Switch and launch applies the save's own game and list or pack there first.
async function goLaunch() {
  await ui.go("home");
  await focusLaunch();
}

async function switchTo(w) {
  if (!w) return;
  await ui.go("home");
  if (await applyItem({ kind: w.kind, envName: w.env, plan: w.plan || 0, name: w.pack || "" })) await focusLaunch();
}

// newList makes a list from the save and opens it; with analyse it starts the list's
// analysis instead and stays, and the save is checked again when it is done.
async function newList(d, analyse) {
  await once("new-list", async () => {
    let r;
    try { r = await api.SaveNewList(d.ref, analyse); } catch (e) { say("fail", errText(e)); return; }
    say(analyse ? "busy" : "ok", r.text);
    await ui?.reload?.();
    if (r.page) ui.go(r.page);
  });
}

// once runs an action unless the same one still waits for its reply (a double click or a
// second enter makes no second copy or list).
async function once(key, run) {
  if (running.has(key)) return;
  running.add(key);
  try { await run(); } finally { running.delete(key); }
}

// chooseButton is Check on Can it run (12 §4.8): this save, or (colony) its colony's newest
// save, becomes what Can it run checks for the current Game Config; pressed again, Can it
// run goes back to the last loaded save.
function chooseButton(d, colony, add) {
  const kind = colony ? "colony" : "save";
  const on = d.chosen === kind;
  const label = on ? "✓ " + T("on Can it run") : colony ? T("check this colony") : T("check this save");
  // the choice is the current Game Config's: it waits while the window switches configs
  const b = add(label, "choose-" + kind, () => choose(d, colony, on), "", "", "save-choice");
  b.setAttribute("aria-pressed", String(on));
  b.title = on ? T("press to check the last loaded save again")
    : colony ? T("Can it run checks this colony's newest save, for the current Game Config") : T("Can it run checks this save, for the current Game Config");
  return b;
}

async function choose(d, colony, clear) {
  await once("choose", async () => {
    let text;
    try { text = await api.SaveChoose(d.ref, colony, clear); } catch (e) { say("fail", errText(e)); return; }
    say("ok", text);
    if (live() && X.detail?.ref === d.ref) {
      X.detail.chosen = clear ? "" : colony ? "colony" : "save";
      drawDetail(true);
    }
  });
}

async function copyHere(d) {
  await once("copy-here", async () => {
    let r;
    try { r = await api.SaveCopyTo(d.ref); } catch (e) { say("fail", errText(e)); return; }
    say("ok", r.text);
    await reread(X.sel === d.ref ? r.ref : "");
  });
}

async function copySave() {
  const d = X.detail;
  if (!d || d.loading) return;
  await once("copy", async () => {
    let r;
    try { r = await api.SaveCopy(d.ref); } catch (e) { say("fail", errText(e)); return; }
    say("ok", r.text);
    // the copy is selected, unless the player went to another save meanwhile
    await reread(X.sel === d.ref ? r.ref : "");
  });
}

// askFiles shows the inline confirmation of Back up or Recycle (16 §6): the files and
// sizes first; only the confirm button acts. The last one asked for is the one shown.
async function askFiles(what) {
  const d = X.detail;
  if (!d || d.loading) return;
  if (what === "recycle" && d.locked) return say("fail", d.locked);
  const n = ++X.askSeq;
  let r;
  try { r = await (what === "backup" ? api.SaveBackup([d.ref], false, "") : api.SaveTrash([d.ref], false, "")); }
  catch (e) { if (n === X.askSeq) say("fail", errText(e)); return; }
  if (n !== X.askSeq || X.sel !== d.ref || !live()) return;
  X.mode = { kind: "confirm", ref: d.ref, what, preview: r.preview };
  drawMid(true);
  midCard?.querySelector(".sv-ask .btn.go")?.focus({ preventScroll: true });
}

// focusFileAction puts the focus back on the button that asked for the confirmation.
function focusFileAction(what) {
  midCard?.querySelector(`[data-key="${what === "backup" ? "backup" : "recycle"}"]`)?.focus({ preventScroll: true });
}

function confirmBox(a) {
  const p = a.preview;
  const box = el("div", "sv-ask" + (a.what === "recycle" ? " danger" : ""));
  box.setAttribute("role", "group");
  box.setAttribute("aria-label", p.title);
  box.append(el("p", "t", p.title));
  const ul = el("ul", "files");
  for (const l of p.lines || []) ul.append(el("li", "", l));
  box.append(ul, el("p", "muted", p.to));
  const btns = el("div", "btns");
  const go = button(p.button, "", () => doFiles(a), "small go primary" + (a.what === "recycle" ? " danger-go" : ""), a.what === "recycle" ? "recycle" : "");
  go.dataset.key = "confirm";
  if (a.running) go.disabled = true;
  const no = button(T("cancel"), "", () => { if (X.mode === a) X.mode = null; drawMid(true); focusFileAction(a.what); }, "small");
  no.dataset.key = "cancel";
  box.onkeydown = (e) => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); no.click(); } };
  btns.append(go, no);
  box.append(btns);
  return box;
}

// doFiles backs up or recycles what the confirmation listed: the preview's token binds the
// confirm to those files as they were, so a save that changed since is refused and the
// confirmation goes.
async function doFiles(a) {
  // the confirm button stays until the reply: a second click must not back up or recycle again
  if (a.running) return;
  a.running = true;
  const token = a.preview?.token || "";
  let r;
  try { r = await (a.what === "backup" ? api.SaveBackup([a.ref], true, token) : api.SaveTrash([a.ref], true, token)); }
  catch (e) {
    if (X.mode === a) X.mode = null;
    say("fail", errText(e));
    if (live()) { drawMid(true); refocus(() => focusFileAction(a.what)); }
    return;
  }
  if (X.mode === a) X.mode = null;
  say(r.failed ? "fail" : "ok", r.text);
  if (!live()) return;
  if (a.what === "recycle") { if (await reread()) refocus(focusTree); }
  else { drawMid(true); refocus(() => focusFileAction(a.what)); }
}

function startRename() {
  const d = X.detail;
  if (!d || d.loading || d.error) return;
  if (d.locked) return say("fail", d.locked);
  if (!editing("rename")) {
    X.mode = { kind: "rename", ref: d.ref };
    drawMid(true);
  }
  const inp = midCard?.querySelector(".sv-rename input");
  inp?.focus();
  inp?.select();
}

// renameField is the inline rename (16 §2.17): duplicates are checked while typing; enter
// or leaving saves, esc cancels. mine is the mode that opened it: once the player selects
// another save the field is no longer theirs, and a rename still saving leaves that
// selection alone.
function renameField(d, mine) {
  const box = el("div", "sv-rename");
  const inp = el("input", "xc-input");
  inp.value = d.name; inp.spellcheck = false;
  inp.setAttribute("aria-label", T("rename the save"));
  const why = el("div", "plan-why");
  why.setAttribute("role", "alert");
  let seq = 0, done = false, busy = false;
  const check = async () => {
    const n = ++seq;
    const p = await api.SaveNameProblem(d.ref, inp.value).catch(() => "");
    if (n === seq) why.textContent = p;
    return n === seq && !p;
  };
  const stop = () => {
    if (done) return;
    done = true;
    const mineNow = X.mode === mine;
    if (mineNow) X.mode = null;
    if (!live() || !inp.isConnected) return;
    drawMid(true);
    if (mineNow) midCard?.querySelector('[data-key="rename"]')?.focus({ preventScroll: true });
  };
  const commit = async () => {
    if (done || busy) return;
    const name = inp.value.trim();
    if (!name || name === d.name) return stop();
    busy = true; // leaving the field while it saves (its blur, its removal) saves once
    try {
      if (!(await check())) {
        if (inp.isConnected) inp.focus(); else if (why.textContent) say("fail", why.textContent);
        return;
      }
      let r;
      try { r = await api.SaveRename(d.ref, name); }
      catch (e) { why.textContent = errText(e); if (inp.isConnected) inp.focus(); else say("fail", errText(e)); return; }
      done = true;
      const stayed = X.mode === mine;
      if (stayed) X.mode = null;
      say("ok", r.text);
      // the renamed save stays selected under its new name, unless the player went to
      // another save meanwhile (leaving the field is what saved it)
      if (await reread(X.sel === d.ref ? r.ref : "") && stayed) refocus(focusTree);
    } finally { busy = false; }
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

function startColonyRename() {
  const d = X.detail;
  if (!d?.colonyKey) return;
  if (!editing("colony")) {
    X.mode = { kind: "colony", ref: d.ref };
    drawRight(true);
  }
  const inp = rightCard?.querySelector(".sv-rename input");
  inp?.focus();
  inp?.select();
}

// colonyField names the colony in place: enter or leaving saves, esc cancels, and an empty
// name gives the colony back its saves' name. mine is the mode that opened it (renameField).
function colonyField(d, mine) {
  const box = el("div", "sv-rename");
  const inp = el("input", "xc-input");
  inp.value = d.colony || ""; inp.spellcheck = false; inp.maxLength = 80;
  inp.placeholder = d.colonyGame || "";
  inp.setAttribute("aria-label", T("rename the colony"));
  inp.dataset.key = "rename-colony"; // a redraw after saving puts the focus on Rename
  const why = el("div", "plan-why");
  why.setAttribute("role", "alert");
  let done = false, busy = false;
  const stop = () => {
    if (done) return;
    done = true;
    const mineNow = X.mode === mine;
    if (mineNow) X.mode = null;
    if (!live() || !inp.isConnected) return;
    drawRight(true);
    if (mineNow) rightCard?.querySelector('[data-key="rename-colony"]')?.focus({ preventScroll: true });
  };
  const commit = async () => {
    if (done || busy) return;
    const name = inp.value.trim();
    if (name === d.colony) return stop();
    busy = true;
    try {
      const text = await api.ColonyRename(d.colonyKey, name);
      done = true;
      if (X.mode === mine) X.mode = null;
      say("ok", text);
      await reread();
    } catch (e) { why.textContent = errText(e); if (inp.isConnected) inp.focus(); else say("fail", errText(e)); }
    finally { busy = false; }
  };
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
// the saves are read again (12 §1.7).
export function liveChanged(gameRuns) {
  if (!live()) return;
  if (!gameRuns) reread(); else showDetail(0);
}

// key: the page's keys (16 §2.11). true when taken.
export function key(e) {
  const k = e.key;
  if (e.ctrlKey || e.metaKey || e.altKey) return false;
  if (k === "/") { e.preventDefault(); input?.focus(); input?.select(); return true; }
  if (k === "F2") { e.preventDefault(); startRename(); return true; }
  if (editing("confirm") && k === "Escape") { e.preventDefault(); const what = X.mode.what; X.mode = null; drawMid(true); focusFileAction(what); return true; }
  if (k === "Escape" && X.q) { e.preventDefault(); if (input) input.value = ""; setQuery(""); return true; }
  // in the middle and right cards the arrows scroll them, and enter presses the focused button
  const inCards = document.activeElement?.closest?.(".sv-mid, .sv-right");
  if (k === "Enter" && !inCards) {
    const q = primary(X.detail);
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

// primary is what enter runs: the Switch line's action, else the bar's.
function primary(d) {
  if (!d) return null;
  return d.switch ? { action: "switch", label: d.switch.label } : d.quick;
}

export function hintList() {
  const q = primary(X.detail);
  const out = [{ key: "/", label: T("search") }, { key: "↑↓", label: T("previous / next save") }, { key: "←→", label: T("fold the colony") }];
  if (q) out.push({ key: "enter", label: q.label });
  out.push({ key: "F2", label: T("rename") }, { key: "1–3", label: T("the cards") }, { key: "?", label: T("all keys") });
  return out;
}
