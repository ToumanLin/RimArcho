// Mod library › updates (06 §2.9): one list and a detail. The list is what waits for the player
// to update (Git, download pages, SteamCMD, Workshop items Steam has not downloaded), then
// installed copies removed from the Workshop, then what already changed (one row per mod; earlier
// days). A scope bar above it picks the current list or every mod; one line under it sums
// up the sources. Mod library › add a subscription is its own page (openAdd). Data: API.SubscriptionsPage
// (view.BuildSubscriptions, the history from modchanges.Build).
import { S, $, el, api, T, Tf, say, errText } from "./state.js";
import { pathContent } from "./fields.js";
import * as pop from "./popups.js";
import { icon } from "./icons.js";
import { wireSeparator } from "./separator.js";
import { addForm, pageUpdate } from "./subscribe.js";
import { libraryLink, openLibraryInstance } from "./librarylink.js";

let ui = null;
// the page helpers of screens.js (given at init: screens.js re-exports this module)
let beginPageRead, page, area, button, act, seg;
// library: every mod (else the current list); of: only one mod's Lineage ({id, name});
// sel: the selected item's key; days: earlier days opened; raw: file lists read (by row);
// landing: focus the list after the next draw.
const V = { data: null, library: false, of: null, sel: "", days: new Set(), raw: new Map(), landing: false, items: [] };
let pasted = "";

export function initSubscriptions(u, helpers) {
  ui = u;
  ({ beginPageRead, page, area, button, act, seg } = helpers);
}

// listenPaste: a paste on updates (not in a field) takes the address to add a subscription. Added
// with the page's first visit.
let listening = false;
function listenPaste() {
  if (listening) return;
  listening = true;
  document.addEventListener("paste", (e) => {
    if (S.screen !== "changes" || e.target?.closest?.("input, textarea, [contenteditable]")) return;
    const text = e.clipboardData?.getData("text") || "";
    if (!text.trim()) return;
    e.preventDefault();
    pasted = text.trim();
    ui.go("subscribe");
  });
}

const SRC_ICON = { workshop: "workshop", steamcmd: "workshop", git: "git", page: "ext", local: "local" };
const TIER = { 1: "launch", 2: "watch", 3: "content" };
const rowKey = (r) => r.id + "|" + (r.before || 0) + "|" + (r.after || 0);
const hasFiles = (r) => r.before && r.after && r.before !== r.after;
const ic = (name, cls = "ic") => { const s = el("span", cls); s.innerHTML = icon[name] || ""; return s; };

// ---- the page's state and data ----

// changesFiltered: the page shows one mod's Lineage (the hint line offers esc).
export function changesFiltered() { return S.screen === "changes" && !!V.of; }

// changesPlan: the next visit shows the current list since it was last played (the launch
// page's line opens it so).
export function changesPlan() { V.library = false; V.of = null; V.sel = ""; V.landing = true; }

// changes shows the page; of: {id, name} narrows it to that mod's Lineage, null shows every
// mod, undefined keeps what it shows.
export async function changes(of) {
  listenPaste();
  if (S.screen !== "changes") { V.raw.clear(); }
  if (of?.id) V.landing = true;
  if (of !== undefined && (of && of.id) !== (V.of && V.of.id)) { V.of = of; V.sel = ""; V.days.clear(); }
  const read = beginPageRead("changes");
  let p;
  try { p = await api.SubscriptionsPage(V.library, V.of ? V.of.id : ""); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  V.data = p;
  draw();
  read.finish();
}

// shown: the pending updates the scope shows, and how many it hides.
function shownPending() {
  const all = V.data?.pending || [];
  if (V.of) return { rows: all.filter((r) => r.id === V.of.id || r.name === V.of.name), hidden: 0 };
  if (V.library) return { rows: all, hidden: 0 };
  const rows = all.filter((r) => r.inPlan);
  return { rows, hidden: all.length - rows.length };
}

function shownGone() {
  const all = V.data?.gone || [];
  if (V.of) return all.filter(r => r.id === V.of.id);
  return V.library ? all : all.filter(r => r.inPlan);
}

// items: every row of the list, in order; each knows what its detail shows.
function buildItems() {
  const h = V.data?.history || {}, out = [];
  for (const r of shownPending().rows) out.push({ key: "p:" + r.ref, t: "pending", r });
  for (const r of shownGone()) out.push({ key: "gone:" + r.id, t: "gone", r });
  if (!V.of) {
    for (const r of h.rows || []) out.push({ key: "r:now:" + rowKey(r), t: "change", r });
    if (h.arrivals) out.push({ key: "g:now", t: "group", g: h.arrivals });
  }
  for (const d of h.older || []) {
    out.push({ key: "d:" + d.day, t: "day", d });
    if (!V.of && !V.days.has(d.day)) continue;
    for (const r of d.rows || []) out.push({ key: "r:" + d.day + ":" + rowKey(r), t: "change", r, day: d.day });
    if (d.arrivals) out.push({ key: "g:" + d.day, t: "group", g: d.arrivals, day: d.day });
  }
  out.push({ key: "sources", t: "sources" });
  return out;
}

const selected = () => V.items.find((it) => it.key === V.sel) || null;

// ---- drawing ----

function draw() {
  const sc = $("#screen");
  const focusInList = !!document.activeElement?.closest?.(".subs-list");
  const keepTop = $("#screen .subs-list .scroll")?.scrollTop || 0;
  V.items = buildItems();
  if (!selected()) V.sel = V.items.find((it) => it.t !== "day")?.key || V.items[0]?.key || "";
  sc.innerHTML = "";
  const w = page("changes fill subs-page", T("First what waits for you to update, then what Steam already updated."),
    act(T("check for updates"), "rescan", checkAll, "K"),
    act(T("add a subscription"), "plus", () => ui.go("subscribe"), "+"));
  const grid = el("div", "subs");
  const left = el("div", "subs-left");
  const list = listCard();
  left.append(scopeBar(), list);
  const gap = el("div", "split subs-split");
  const detail = detailCard();
  grid.append(left, gap, detail);
  const setSide = (v) => { if (v) grid.style.setProperty("--subs-side", v + "px"); else grid.style.removeProperty("--subs-side"); };
  wireSeparator(gap, {
    key: "ui.gui.changes.side", range: () => [300, 560],
    get: () => left.getBoundingClientRect().width,
    set: setSide, reset: () => setSide(0),
    restore: (v) => setSide(+v ? Math.max(300, Math.min(560, +v)) : 0),
  });
  area(list, null, () => focusSel());
  area(detail, null, () => detail.querySelector("button, input")?.focus({ preventScroll: true }));
  w.append(grid);
  sc.append(w);
  const scroll = list.querySelector(".scroll");
  if (scroll) scroll.scrollTop = keepTop;
  if (V.landing || focusInList) { focusSel(); V.landing = false; }
  ui.renderWhere?.();
  ui.renderHints();
}

function focusSel() {
  const row = $(`#screen .subs-list [data-key="${CSS.escape(V.sel)}"]`);
  if (row) { row.focus({ preventScroll: true }); row.scrollIntoView({ block: "nearest" }); }
}

// scopeBar: list "…" / every mod and the start of the range; or the one-mod condition.
function scopeBar() {
  const bar = el("div", "card subs-scope");
  const h = V.data?.history || {};
  if (V.of) {
    const b = el("button", "lib-cond");
    b.append(el("span", "", Tf("only %s", V.of.name)), el("span", "x", "×"));
    b.title = T("esc shows every mod");
    b.onclick = () => changes(null).then(() => ui.renderHints());
    bar.append(b);
    return bar;
  }
  const names = [h.plan ? Tf("list “%s”", h.plan) : T("the current list"), T("every mod")];
  bar.append(seg(names, V.library ? 1 : 0, (i) => { if ((i === 1) !== V.library) toggleScope(); }, ["a", "a"]));
  if (h.since) bar.append(el("span", "since muted word-facts", h.since));
  return bar;
}

function toggleScope() {
  if (V.of) return;
  V.library = !V.library;
  V.sel = "";
  changes();
}

// listCard: waiting for you to update, already updated, earlier days; the sources line under them.
function listCard() {
  const card = el("section", "card subs-list");
  const box = el("div", "scroll");
  const { rows, hidden } = shownPending();
  const head = (title, n, right) => {
    const h = el("div", "subs-h");
    const t = el("h3", "", title);
    if (n !== undefined) t.append(el("span", "n", String(n)));
    h.append(t);
    if (right) { const r = el("span", "r"); r.append(right); h.append(r); }
    box.append(h);
  };
  const all = button(T("update all"), "U", () => updateRows(rows), "primary small", "subscription-update");
  all.disabled = !rows.length;
  head(T("waiting for you to update"), rows.length, all);
  if (!rows.length) box.append(el("div", "subs-empty muted", V.of ? T("no update waits for this mod") : T("everything is up to date")));
  if (hidden) {
    const more = el("div", "subs-hidden muted");
    more.append(el("span", "", Tf("%d more in other lists", hidden)), button(T("show all mods"), "", () => toggleScope(), "link"));
    box.append(more);
  }
  let section = "";
  for (const it of V.items) {
    if (it.t === "sources") continue;
    if (it.t === "gone") {
      if (section !== "gone") { section = "gone"; head(T("won't update any more"), shownGone().length); }
      box.append(row(it)); continue;
    }
    if (it.t === "pending") { box.append(row(it)); continue; }
    if (it.t !== "day" && !it.day && section !== "now") {
      section = "now";
      head(T("already updated"), (V.data.history.rows || []).length, el("span", "muted", T("by Steam")));
    }
    if (it.t === "day" && section !== "older") {
      section = "older";
      const h = el("div", "subs-h quiet"); h.append(el("h3", "", T("earlier"))); box.append(h);
    }
    box.append(row(it));
  }
  if (!V.of && !(V.data.history.rows || []).length && !V.data.history.arrivals && section !== "now") {
    const h = el("div", "subs-h"); h.append(el("h3", "", T("already updated"))); box.append(h);
    box.append(el("div", "subs-empty muted", V.data.history.noScan ? T("no scan yet (R scans)") : T("nothing changed")));
  }
  card.append(box);
  card.append(sourcesLine());
  return card;
}

// row: one item of the list (a button: tab and the arrows reach it).
function row(it) {
  const b = el("button", "subs-row" + (it.key === V.sel ? " sel" : "") + (it.day ? " indent" : ""));
  b.type = "button";
  b.dataset.key = it.key;
  const name = el("span", "nm word-facts"), meta = el("span", "m word-facts"), right = el("span", "ri");
  if (it.t === "pending") {
    const r = it.r;
    name.textContent = r.name;
    meta.append(ic(SRC_ICON[r.kind] || "local"), el("span", "", r.source), el("span", "", r.installed + " → " + r.available));
    if (r.kind === "page") right.append(el("span", "pill", T("choose a file")));
  } else if (it.t === "gone") {
    name.textContent = it.r.name;
    meta.textContent = T("removed from the Workshop");
  } else if (it.t === "change") {
    name.textContent = it.r.mod;
    meta.textContent = it.r.text;
    right.append(el("span", "tier " + (TIER[it.r.tier] || ""), it.r.tierText));
  } else if (it.t === "group") {
    name.textContent = it.g.text;
    name.classList.add("plain");
    if ((it.g.rows || []).length > 1) meta.textContent = it.g.rows.map(r => r.mod).join(T(", "));
  } else {
    name.textContent = it.d.day;
    meta.textContent = Tf("%d mods", (it.d.rows || []).length + ((it.d.arrivals && it.d.arrivals.rows) || []).length);
    right.append(el("span", "chev", V.of || V.days.has(it.d.day) ? "▾" : "▸"));
  }
  b.append(name, meta, right);
  b.onclick = () => { pick(it.key); if (it.t === "day" && !V.of) toggleDay(it.d.day); };
  b.onfocus = () => { if (V.sel !== it.key) pick(it.key, true); };
  return b;
}

// pick selects an item: the list marks it, the detail follows (without a redraw of the list).
function pick(key, fromFocus = false) {
  if (V.sel === key && !fromFocus) return;
  V.sel = key;
  $("#screen .subs-list")?.querySelectorAll(".subs-row").forEach((r) => r.classList.toggle("sel", r.dataset.key === key));
  const card = $("#screen .subs-detail");
  if (card) fillDetail(card);
  ui.renderHints();
}

function toggleDay(day) {
  V.days.has(day) ? V.days.delete(day) : V.days.add(day);
  draw();
  focusSel();
}

// The saved source summary is a list item; its detail uses the full right column.
function sourcesLine() {
  const foot = el("div", "subs-foot");
  const b = el("button", "subs-row subs-srcsum" + (V.sel === "sources" ? " sel" : ""));
  b.type = "button"; b.dataset.key = "sources";
  b.append(ic("rescan"), el("span", "word-facts", V.data?.summary?.line || T("sources not checked yet")));
  b.onclick = () => pick("sources");
  b.onfocus = () => { if (V.sel !== "sources") pick("sources", true); };
  foot.append(b);
  return foot;
}

function sourcesDetail(card) {
  detailHead(card, T("Update checks"), []);
  const box = el("div", "scroll subs-sources");
  for (const s of V.data?.sources || []) {
    const r = el("div", "subs-src");
    const l = el("div", "l");
    const n = el("div", "n");
    n.append(ic(SRC_ICON[s.key] || "local"), el("b", "", s.name), el("span", "muted", Tf("%d mods", s.count)));
    l.append(n, el("div", "muted word-facts", [s.checked || T("not checked yet"), s.result].filter(Boolean).join("  ")));
    if (s.account && ["steamcmd", "page"].includes(s.key)) {
      const a = el("div", "word-facts acct");
      a.append(el("span", "", s.account), button(T("accounts") + " →", "enter", () => goAccounts(), "link"));
      l.append(a);
    }
    for (const g of s.failures || []) {
      for (const item of g.items) l.append(el("div", "word-facts", item));
    }
    const check = button(T("check"), "enter", async () => {
      try { say("ok", await api.CheckSubscriptionSource(s.key)); }
      catch (e) { say("fail", errText(e)); }
    }, "small");
    check.disabled = !s.count && s.key !== "local";
    r.append(l, check);
    box.append(r);
  }
  card.append(box);
}

async function goAccounts() {
  await ui.go("settings");
  const heading = document.querySelector('[data-setting-action^="account "]')?.closest("[data-area]");
  heading?.scrollIntoView({ block: "start" });
  heading?.querySelector("button, input")?.focus({ preventScroll: true });
}

// ---- the detail ----

function detailCard() {
  const card = el("section", "card subs-detail");
  fillDetail(card);
  return card;
}

// fillDetail draws the selected item into the detail card (the same card: its area number stays).
function fillDetail(card) {
  card.replaceChildren();
  const it = selected();
  if (!it) { card.append(el("div", "empty-state", V.data?.history?.noScan ? T("no scan yet (R scans)") : T("nothing here yet"))); return; }
  if (it.t === "pending") pendingDetail(card, it.r);
  else if (it.t === "change") changeDetail(card, it.r);
  else if (it.t === "group") groupDetail(card, it.g);
  else if (it.t === "gone") goneDetail(card, it.r);
  else if (it.t === "sources") sourcesDetail(card);
  else dayDetail(card, it.d);
}

function detailHead(card, title, facts, ...acts) {
  const head = el("div", "subs-dhead");
  const l = el("div", "l");
  l.append(el("h2", "word-facts", title));
  const f = el("div", "facts");
  for (const x of facts) if (x) f.append(typeof x === "string" ? el("span", "word-facts", x) : x);
  l.append(f);
  const r = el("div", "r");
  for (const a of acts) if (a) r.append(a);
  head.append(l, r);
  card.append(head);
}

function section(body, title, right) {
  const h = el("div", "subs-sub");
  h.append(el("span", "", title));
  if (right) { const r = el("span", "r"); r.append(right); h.append(r); }
  const s = el("div", "subs-sec");
  body.append(h, s);
  return s;
}

function usedIn(body, used) {
  if (!used || !used.length) return;
  const s = section(body, T("used in"));
  const pills = el("div", "subs-pills");
  const plan = V.data?.history?.plan;
  for (const u of used) pills.append(el("span", "pill" + (u === plan ? " on" : ""), u));
  s.append(pills);
}

function pendingDetail(card, r) {
  const src = el("span", "src");
  src.append(ic(SRC_ICON[r.kind] || "local"), el("span", "", r.source));
  const upd = button(r.kind === "page" ? T("choose the new files below") : T("update"), "u", () => updateRows([r]), "primary", "subscription-update");
  if (r.kind === "page") upd.disabled = true;
  detailHead(card, r.name, [el("span", "pill", T("waiting for you to update")), src, r.installed + " → " + r.available], upd);
  const body = el("div", "scroll");
  const s = section(body, T("this update"));
  const kv = el("div", "subs-kv");
  const add = (k, v) => { if (v) kv.append(el("span", "", k), el("span", "word-facts", v)); };
  add(T("installed"), [r.installed, r.installedAt].filter(Boolean).join("  "));
  add(T("available"), r.available);
  add(T("track"), r.tracking);
  add(T("last check"), r.checked);
  s.append(kv);
  if (r.kind === "workshop") s.append(el("div", "muted subs-note", T("Steam downloads this update; RimArcho asks it to.")));
  if (r.kind === "page") {
    const f = section(body, T("which file"));
    f.append(pageUpdate(r.ref, () => changes()));
  }
  usedIn(body, r.usedIn);
  const acts = el("div", "subs-acts");
  if (r.opens) acts.append(button(r.opens === "compare" ? T("compare on the website") : r.opens === "notes" ? T("Workshop change notes") : T("open the source page"), "ctrl+w", () => openSource(r), "line small"));
  acts.append(button(T("lock this version first"), "p", () => lock(r.ref), "small", "pin"));
  acts.append(button(T("show in the mod library"), "l", () => openLibraryInstance(r.id), "small"));
  const keep = section(body, T("keep the old one"));
  keep.append(el("div", "muted subs-note", T("Updating replaces this copy. To keep the old one, lock this version first.")), acts);
  card.append(body);
}

function goneDetail(card, r) {
  detailHead(card, r.name, []);
  const body = el("div", "scroll subs-gone");
  body.append(el("p", "word-facts", T("The author deleted the Workshop page or made it private; your installed copy still works, but Steam will never update it.")),
    el("p", "word-facts", T("After unsubscribing, you cannot download it again; lock this version to keep it.")));
  const acts = el("div", "subs-acts");
  acts.append(button(T("lock this version"), "p", () => lock(r.ref), "small", "pin"),
    button(T("show in the mod library"), "l", () => openLibraryInstance(r.id), "small"));
  body.append(acts);
  card.append(body);
}

function changeDetail(card, r) {
  const facts = [el("span", "tier " + (TIER[r.tier] || ""), r.tierText)];
  const notes = r.workshopId ? button(T("Workshop change notes"), "ctrl+w", () => changeAct(r, "workshop"), "line") : null;
  detailHead(card, r.mod, facts, notes);
  const body = el("div", "scroll");
  const s = section(body, T("what it means"));
  s.append(el("div", "subs-say word-facts", r.text));
  if (r.tier === 1) {
    const n = el("div", "subs-alert");
    n.append(ic("warn"), el("span", "word-facts", T("Look at Can it run before launching: this change can stop the game from loading.")));
    s.append(n);
  }
  const details = (r.details || []);
  const k = rowKey(r);
  const files = hasFiles(r) ? button(V.raw.has(k) ? T("hide the files") : T("every file"), "i", () => rawFiles(r), "small") : null;
  if (details.length || files) {
    const d = section(body, T("what changed"), files);
    for (const line of details) d.append(el("div", "subs-line word-facts", line));
    if (V.raw.has(k)) { const pre = el("pre", "lines files"); pathContent(pre, V.raw.get(k).join("\n")); d.append(pre); }
  }
  const acts = (r.actions || []).filter((a) => a.kind !== "workshop");
  if (acts.length) {
    const a = section(body, T("what you can do"));
    const box = el("div", "subs-acts");
    for (const x of acts) box.append(button(x.label, x.key, () => changeAct(r, x.kind), "line small", ["deps"].includes(x.kind) ? "" : x.kind));
    a.append(box);
  }
  card.append(body);
}

function groupDetail(card, g) {
  detailHead(card, g.text, []);
  const body = el("div", "scroll");
  const s = section(body, T("mods"));
  for (const r of g.rows || []) {
    const line = el("div", "subs-line");
    line.append(r.present ? libraryLink(r.id, r.mod) : el("span", "word-facts", r.mod), el("span", "muted word-facts", r.text));
    s.append(line);
  }
  card.append(body);
}

function dayDetail(card, d) {
  const rows = d.rows || [];
  detailHead(card, d.day, [Tf("%d mods", rows.length + ((d.arrivals && d.arrivals.rows) || []).length)]);
  const body = el("div", "scroll");
  const s = section(body, T("mods"));
  for (const r of rows) {
    const line = el("div", "subs-line");
    line.append(el("span", "tier " + (TIER[r.tier] || ""), r.tierText), el("b", "word-facts", r.mod), el("span", "muted word-facts", r.text));
    s.append(line);
  }
  if (d.arrivals) s.append(el("div", "subs-line muted word-facts", d.arrivals.text));
  card.append(body);
}

// ---- actions ----

// checkAll queues a check of every source that has mods.
async function checkAll() {
  const keys = (V.data?.sources || []).filter((s) => s.count && s.key !== "local").map((s) => s.key);
  if (!keys.length) return say("fail", T("no subscriptions to check"));
  try { for (const k of keys) await api.CheckSubscriptionSource(k); say("ok", Tf("queued %d update checks", keys.length)); }
  catch (e) { say("fail", errText(e)); }
}

// updateRows previews, then updates (one confirmation for all of them). A download page needs
// the player's file choice: it is skipped, and its detail shows the choice.
async function updateRows(items) {
  const pages = items.filter((r) => r.kind === "page");
  items = items.filter((r) => r.kind !== "page");
  if (!items.length) {
    if (pages.length) { pick("p:" + pages[0].ref); say("fail", T("choose the new files first")); }
    return;
  }
  const previews = [];
  try { for (const r of items) previews.push(r.kind === "workshop" ? await api.WorkshopUpdatePreview(r.ref) : await api.LibraryPreview("subscription-update", [r.ref])); }
  catch (e) { return say("fail", errText(e)); }
  const lines = previews.flatMap((p, i) => [items[i].name, ...(p.lines || [])]);
  if (pages.length) lines.push(Tf("%d download pages need a file first; they are not updated now", pages.length));
  pop.confirm(T("update subscription"), lines, T("update subscription"), previews.every((p) => p.apply) ? async () => {
    try { for (const r of items) say("ok", r.kind === "workshop" ? await api.WorkshopUpdate(r.ref) : await api.LibraryDo("subscription-update", [r.ref])); }
    catch (e) { say("fail", errText(e)); }
  } : null);
}

async function openSource(r) {
  try { say("ok", await api.OpenSubscriptionSource(r.ref)); } catch (e) { say("fail", errText(e)); }
}

// lock: a copy in the lock library, the dry run first (as on Mod library, 06 §6).
async function lock(ref) {
  let p;
  try { p = await api.LibraryPreview("pin", [ref]); } catch (e) { return say("fail", errText(e)); }
  pop.confirm(p.title, p.lines, T("lock this one"), p.apply ? async () => {
    try { say("ok", await api.LibraryDo("pin", [ref])); } catch (e) { say("fail", errText(e)); }
    await ui.reload();
    changes();
  } : null);
}

async function rawFiles(r) {
  const epoch = S.screenSeq, k = rowKey(r);
  if (V.raw.has(k)) V.raw.delete(k);
  else {
    let lines;
    try { lines = (await api.ChangeFiles(r.before, r.after)) || []; } catch (e) { if (S.screen === "changes" && S.screenSeq === epoch) say("fail", errText(e)); return; }
    if (S.screen !== "changes" || S.screenSeq !== epoch) return;
    V.raw.set(k, lines);
  }
  pick(V.sel, true);
}

// changeAct runs a change row's action (each also has its key).
async function changeAct(r, kind) {
  switch (kind) {
    case "workshop":
      try { say("ok", await api.OpenChangeNotes(r.workshopId)); } catch (e) { say("fail", errText(e)); }
      return;
    case "deps": return ui.go("needs");
    case "sort": return ui.run("sort");
    case "lock": return lock(r.ref);
  }
}

// ---- keys and hints ----

export function changesHints() {
  const it = selected();
  const out = [{ key: "↑↓", label: T("move") }];
  if (["pending", "gone"].includes(it?.t)) out.push({ key: "p", label: T("lock this version") }, { key: "l", label: T("show in the mod library") });
  if (it?.t === "sources" || it?.t === "group") out.push({ key: "tab", label: T("the next field or button") }, { key: "enter", label: T("open") });
  if (it?.t === "pending") out.push({ key: "u", label: T("update") }, ...(it.r.opens ? [{ key: "ctrl+w", label: T("open the source page") }] : []));
  if (it?.t === "change") {
    if (hasFiles(it.r)) out.push({ key: "i", label: T("files") });
    out.push({ key: "ctrl+o", label: T("open the folder") });
    for (const a of it.r.actions || []) out.push({ key: a.key, label: a.label });
  }
  if (it?.t === "day") out.push({ key: "enter", label: T("open") });
  out.push({ key: "U", label: T("update all") }, { key: "K", label: T("check for updates") });
  if (!V.of) out.push({ key: "a", label: V.library ? T("the current list") : T("every mod") });
  out.push({ key: "+", label: T("add a subscription") });
  return out;
}

function move(k) {
  const n = V.items.length;
  if (!n) return false;
  const i = Math.max(0, V.items.findIndex((it) => it.key === V.sel));
  let j = i;
  if (k === "ArrowDown" || k === "j") j = Math.min(i + 1, n - 1);
  else if (k === "ArrowUp" || k === "k") j = Math.max(i - 1, 0);
  else if (k === "Home") j = 0;
  else if (k === "End") j = n - 1;
  else if (k === "PageDown") j = Math.min(i + 10, n - 1);
  else if (k === "PageUp") j = Math.max(i - 10, 0);
  else return false;
  pick(V.items[j].key);
  focusSel();
  return true;
}

// changesKey: the page's keys (every action on it has one).
export function changesKey(e) {
  const k = e.key, ctrl = e.ctrlKey || e.metaKey;
  const done = () => e.preventDefault();
  if (k === "Escape") {
    if (V.of) { done(); return changes(null).then(() => ui.renderHints()); }
    return;
  }
  if (k === "Tab" || e.target?.closest?.("input, select, textarea, details")) return;
  if (k === "+" && !ctrl) { done(); return ui.go("subscribe"); }
  if (k === "K" && !ctrl) { done(); return checkAll(); }
  if (k === "U" && !ctrl) { done(); return updateRows(shownPending().rows); }
  if (k === "a" && !ctrl && !V.of) { done(); return toggleScope(); }
  if (k === "R" && !ctrl) { done(); return ui.cmd("rescan"); }
  if (!ctrl && !e.altKey && !e.target?.closest?.(".subs-detail") && move(k)) return done();
  const it = selected();
  if (!it) return;
  if (ctrl) {
    const lk = k.toLowerCase();
    if (lk === "o" && it.t === "change") { done(); return api.OpenFolder(it.r.id).then(() => say("ok", T("opened the folder")), (x) => say("fail", errText(x))); }
    if (lk === "w" && it.t === "change" && it.r.workshopId) { done(); return changeAct(it.r, "workshop"); }
    if (lk === "w" && it.t === "pending" && it.r.opens) { done(); return openSource(it.r); }
    return;
  }
  if (["pending", "gone"].includes(it.t) && k === "l") { done(); return openLibraryInstance(it.r.id); }
  if (it.t === "gone" && k === "p") { done(); return lock(it.r.ref); }
  if (["sources", "group"].includes(it.t) && k === "Enter" && e.target?.closest?.(".subs-list")) {
    done(); return $("#screen .subs-detail")?.querySelector("button:not(:disabled)")?.focus();
  }
  if (it.t === "pending") {
    if (k === "u") { done(); return updateRows([it.r]); }
    if (k === "p") { done(); return lock(it.r.ref); }
    return;
  }
  if (it.t === "day" && k === "Enter" && !V.of) { done(); return toggleDay(it.d.day); }
  if (it.t !== "change") return;
  const has = (kind) => (it.r.actions || []).some((a) => a.kind === kind);
  if (k === "i" && hasFiles(it.r)) { done(); return rawFiles(it.r); }
  if (k === "S" && has("sort")) { done(); return changeAct(it.r, "sort"); }
  if (k === "D" && has("deps")) { done(); return changeAct(it.r, "deps"); }
  if (k === "p" && has("lock")) { done(); return changeAct(it.r, "lock"); }
}

// ---- Mod library › add a subscription ----

// openAdd shows the add page; a paste on updates arrives filled in.
export async function openAdd() {
  const read = beginPageRead("subscribe");
  let data = null;
  try { data = await api.AddSubscriptionInfo(); } catch { /* the page works without the account lines */ }
  if (!read.current()) return;
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = page("subscribe-page", T("Paste an address: RimArcho recognises what it is, where to download it and how to check it for updates."));
  const form = addForm({ data, back: () => ui.go("changes"), done: () => {} });
  w.append(form.el);
  sc.append(w);
  const text = pasted;
  pasted = "";
  if (text) form.setText(text);
  form.focus();
  ui.renderHints();
  read.finish();
}

export function addHints() {
  return [{ key: "ctrl+v", label: T("paste an address") }, { key: "enter", label: T("subscribe") }, { key: "esc", label: T("clear, then back to updates") }];
}
