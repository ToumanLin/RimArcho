// The full detail (06 §4.2): one page for a mod, opened by enter from the plan editor and
// from Mod 库存 › 全部. It puts together what the two lists know about it:
//   1 the mod: picture, name, author, its state, its tags, the description;
//   2 its copies (副本): each copy's state, place, size, version and note, and the selected
//     copy's actions (the six copy ops and the plan's, view.OpTable);
//   3 用在: the plans and packs that use each copy, and the current plan, with its key;
//   4 the field list (a click copies a value);
//   5 历史: the newest changes and the way to 变化;
//   6 技术信息: Defs, patches, Harmony, rules, every change (view.Sections, which other parts
//     of RimArcho add to: this page draws whatever sections come).
// ← → step to the previous / next mod of the list it came from, esc goes back there, 1–6
// focus the parts, the row menu and the copy keys act on the selected copy ([ ] pick it).
// The editor's detail column and Mod 库存's details are the short form of the same parts
// (detail.js renderDetail, library.js drawDetail).
import { S, $, el, api, T, Tf, errText, say, cur } from "./state.js";
import { closeScreen, page, seg, button, area, beginPageRead } from "./screens.js";
import { shimmer } from "./motion.js";
import { renderDetail } from "./detail.js";
import { fieldList } from "./fields.js";
import * as pop from "./popups.js";
import * as ops from "./ops.js";

let ui = null;
export function initFull(u) { ui = u; }

// F: where the page came from ("plan" or "library"), the mod (an instance id, or a copy
// ref for a subscribed item RimArcho never saw), and what was loaded for it.
const F = { from: "plan", id: "", env: "", d: null, row: null, secs: null, tab: 0, copy: "", seq: 0 };

// PLAN: the ops that are one plan step (u undoes them), as on Mod 库存.
const PLAN = new Set(["add", "take-out", "put-back", "stage", "use", "swap", "remove"]);

export async function full(from, id) {
  from = from || "plan";
  if (from === "plan") {
    const it = cur();
    if (!it || !it.id) return;
    id = it.id;
  }
  if (!id) return;
  if (F.id !== id || F.env !== S.board?.env) { F.tab = 0; F.copy = ""; F.d = F.row = F.secs = null; }
  F.env = S.board?.env;
  F.from = from;
  F.id = id;
  S.fullFrom = from;
  const n = ++F.seq;
  const read = beginPageRead("full");
  draw(true);
  let d = null, cp = null, secs = [];
  try {
    [d, cp, secs] = await Promise.all([
      api.Detail(id, false, false, false).catch(() => null),
      api.ModCopies(id),
      api.Sections(id).catch(() => []),
    ]);
  } catch (e) { read.fail(e); return; }
  if (n !== F.seq || !read.current()) return;
  F.d = d;
  F.row = cp && cp.row;
  S.fullName = fullName();
  if (cp && cp.opTable) ops.setTable(cp.opTable);
  F.secs = secs || [];
  const copies = (F.row && F.row.insts) || [];
  if (!copies.some((c) => c.ref === F.copy)) {
    const own = copies.find((c) => c.id === id) || copies.find((c) => c.inPlan) || copies[0];
    F.copy = own ? own.ref : "";
  }
  draw(false);
  read.finish();
  // the Workshop description once the cursor rested on the mod (06 §4.3 item 3)
  if (d && d.state === "not cached") {
    setTimeout(async () => {
      if (n !== F.seq || !read.current()) return;
      try { const f = await api.Detail(id, false, true, false); if (n === F.seq && read.current()) { F.d = f; draw(false); } } catch { /* the note stays */ }
    }, 400);
  }
}

const copies = () => (F.row && F.row.insts) || [];
const theCopy = () => copies().find((c) => c.ref === F.copy) || null;
export const fullName = () => (F.d && F.d.name) || (F.row && F.row.name) || "";

function draw(loading) {
  const sc = $("#screen");
  const top = loading ? 0 : sc.scrollTop;
  sc.innerHTML = "";
  const c = theCopy();
  const acts = [];
  if (c && (c.ops || []).includes("folder")) acts.push(button(T("open the folder"), "ctrl+o", () => runOp("folder")));
  if (c && (c.ops || []).includes("workshop")) acts.push(button(T("Workshop page"), "ctrl+w", () => runOp("workshop")));
  if (c && (c.ops || []).includes("steam")) acts.push(button(T("steam page"), "ctrl+s", () => runOp("steam")));
  const w = page("full mod-page", T("← → the previous / next mod · esc back · 1–6 its parts"), ...acts);
  sc.append(w);
  if (loading && !F.d) {
    const card = el("section", "card pad");
    card.append(shimmer(T("loading…"), 8));
    w.append(card);
    return;
  }
  const grid = el("div", "fd");
  const main = el("div", "fd-main");
  const side = el("div", "fd-side");
  grid.append(main, side);
  w.append(grid);

  // 1 the mod
  const mod = el("section", "card pad fd-mod");
  const modHead = el("div", "fd-h", T("the mod"));
  mod.append(modHead);
  const box = el("div", "fd-desc");
  if (F.d) {
    renderDetail(F.d, box, sc, true, { page: true, noHistory: true });
    // its state under the name, as in the editor's detail, and its tags
    const st = el("div", "fd-state " + (F.d.status > 1 ? "bad" : ""), F.d.statusText || "");
    const who = box.querySelector(".who") || box.querySelector("h1");
    if (who) who.after(st);
    const tags = (F.row && F.row.tags) || [];
    if (tags.length || (c && (c.ops || []).includes("tags"))) {
      const tg = el("div", "fd-tags");
      for (const t of tags) tg.append(el("span", "chip", t));
      if (c && (c.ops || []).includes("tags")) tg.append(button(T("tags") + " …", "g", () => runOp("tags"), "small"));
      st.after(tg);
    }
  } else box.append(el("h1", "", fullName()));
  mod.append(box);
  main.append(mod);
  area(mod, modHead);

  // 2 the copies
  const cs = el("section", "card fd-copies");
  const csHead = el("div", "fd-h", Tf("copies · %d", copies().length));
  cs.append(csHead);
  for (const x of copies()) cs.append(copyCard(x));
  if (!copies().length) cs.append(el("div", "empty-state", T("no copy on this machine")));
  side.append(cs);
  area(cs, csHead, () => { const b = cs.querySelector(".fd-copy.on"); if (b) b.focus(); });

  // 3 used in
  const us = el("section", "card pad fd-used");
  const usHead = el("div", "fd-h", T("used in"));
  us.append(usHead);
  for (const x of copies()) {
    const line = el("div", "fd-useline");
    line.append(el("span", "lbl", x.label), el("span", "x", (x.usedIn || []).join(" · ") || T("in no list or pack")));
    us.append(line);
  }
  if (c) {
    const now = el("div", "fd-now");
    now.append(el("span", "", c.inPlan ? T("in the current list") : T("not in the current list")));
    const sp = ops.keyed(c.ops || [], "space");
    if (sp) now.append(button(ops.op(sp).label, "space", () => runOp(sp), "small"));
    us.append(now);
  }
  side.append(us);
  area(us, usHead);

  // 4 the fields
  if (F.d && F.d.fields) {
    const fs = el("section", "card pad fd-fields");
    const fsHead = el("div", "fd-h", T("fields"));
    fs.append(fsHead, fieldList(F.d.fields, { tags: () => runOp("tags"), folder: () => runOp("folder") }));
    side.append(fs);
    area(fs, fsHead);
  }

  // 5 history
  const hs = el("section", "card pad fd-hist");
  const hsHead = el("div", "fd-h", T("history"));
  hs.append(hsHead);
  if (F.d && F.d.history) hs.append(el("div", "history", F.d.history));
  else hs.append(el("div", "muted", T("no changes recorded")));
  const all = el("button", "link", T("see all on the changes page"));
  all.onclick = () => document.dispatchEvent(new CustomEvent("changes-of", { detail: { id: F.id, name: fullName() } }));
  hs.append(all);
  side.append(hs);
  area(hs, hsHead);

  // 6 technical information: one tab per section
  const tech = el("section", "card fd-tech");
  const tHead = el("div", "fd-h", T("technical information"));
  tech.append(tHead);
  const secs = F.secs || [];
  if (secs.length) {
    F.tab = Math.min(F.tab, secs.length - 1);
    const names = secs.map((s) => s.title + (s.count >= 0 ? " · " + s.count : ""));
    const bar = el("div", "fd-tabs");
    bar.append(seg(names, F.tab, (i) => { F.tab = i; draw(false); }));
    tech.append(bar);
    const pre = el("pre", "lines");
    pre.textContent = (secs[F.tab].lines || []).join("\n");
    tech.append(pre);
  } else tech.append(el("div", "empty-state", F.secs ? T("nothing technical recorded") : T("loading…")));
  w.append(tech);
  area(tech, tHead);

  sc.scrollTop = top;
  ui.renderWhere();
  ui.renderHints();
}

// copyCard: one copy — its label and state, size · version · folder, its note, and for the
// selected copy its actions. A click selects it.
function copyCard(x) {
  const on = x.ref === F.copy;
  const b = el("div", "fd-copy" + (on ? " on" : ""));
  b.tabIndex = -1;
  const top = el("div", "top");
  top.append(el("span", "lbl", x.label));
  if (x.state) top.append(el("span", "chip st " + (x.code || ""), x.state));
  if (x.inPlan) top.append(el("span", "chip cur", T("in the current list")));
  b.append(top);
  b.append(el("div", "facts", [x.size, x.version, x.name].filter(Boolean).join(" · ")));
  if (x.note || x.auto) b.append(el("div", "note" + (x.note ? "" : " auto"), x.note || x.auto));
  if (on) {
    const acts = el("div", "acts");
    for (const code of ops.sorted((x.ops || []).filter((k) => ops.op(k).detail || k === "note"))) {
      const o = ops.op(code);
      const btn = el("button", "btn small" + (o.danger ? " danger" : ""), o.label);
      if (o.key) btn.append(el("kbd", "", o.key));
      btn.onclick = (e) => { e.stopPropagation(); runOp(code); };
      acts.append(btn);
    }
    if (acts.childNodes.length) b.append(acts);
  }
  b.onclick = () => { if (F.copy !== x.ref) { F.copy = x.ref; draw(false); } };
  return b;
}

// runOp runs an op on the selected copy, as Mod 库存 does (06 §3.5): the plan's ops at once
// (u undoes), the ones that change files after their dry run.
async function runOp(code) {
  const c = theCopy();
  if (!c) return;
  const o = ops.op(code);
  const done = async (p, undo) => {
    try { const t = await p; say("ok", typeof t === "string" ? t : t && t.text, undo); } catch (e) { say("fail", errText(e)); }
    await ui.reload();
    if (S.screen === "full") await full(F.from, F.id);
  };
  switch (code) {
    case "folder": return api.OpenFolder(c.id).then(() => say("ok", T("opened the folder")), (e) => say("fail", errText(e)));
    case "steam": return api.OpenSteamWorkshop(c.id || c.ref).then((t) => say("ok", t), (e) => say("fail", errText(e)));
    case "workshop":
      if (c.id) return api.OpenWorkshop(c.id).then((t) => say("ok", t), (e) => say("", errText(e)));
      return api.OpenURL(c.page).then(() => say("ok", T("opened the Workshop page")), (e) => say("fail", errText(e)));
    case "tags": return pop.tags([{ id: c.id, name: fullName() }], () => full(F.from, F.id));
    case "note": return editNote(c);
    case "enable-at": case "move-to": {
      const from = code === "move-to" ? 1 : (c.ops || []).includes("put-back") ? 2 : 0;
      return pop.position({ ids: [c.id], what: fullName() }, from, (at) => done(api.Place(from, [c.id], at), true));
    }
  }
  if (PLAN.has(code)) return done(api.LibraryDo(code, [c.ref]), true);
  if (!o.confirm) return done(api.LibraryDo(code, [c.ref]), false);
  let p;
  try { p = await api.LibraryPreview(code, [c.ref]); } catch (e) { return say("fail", errText(e)); }
  const go = code === "textures" ? () => done(api.TexturesRun("", 100, [c.ref]), false) : () => done(api.LibraryDo(code, [c.ref]), false);
  pop.confirm(p.title, p.lines, o.label, p.apply ? go : null);
}

// editNote: the copy's note in place (n): enter saves, shift+enter breaks the line, esc
// leaves it.
function editNote(c) {
  pop.ask(T("the copy's note"), T("write a note"), async (text) => {
    try { say("ok", await api.LibrarySetNote(c.ref, text)); } catch (e) { say("fail", errText(e)); }
    await full(F.from, F.id);
  }, c.note || c.auto || "");
}

// back is esc: the list the page came from, where it was.
export function back() {
  if (F.from === "library") return ui.go("library");
  closeScreen();
  ui.detailFollow && ui.detailFollow(true);
}

// fullKey: ← → the previous / next mod of that list; ↑↓ PgUp PgDn scroll; tab the
// technical sections; [ ] the copy; the row menu and the copy keys act on that copy.
export async function fullKey(e) {
  const k = e.key;
  const sc = $("#screen");
  const ctrl = e.ctrlKey || e.metaKey;
  if (k === "ArrowLeft" || k === "ArrowRight") {
    e.preventDefault();
    const d = k === "ArrowRight" ? 1 : -1;
    if (F.from === "library") {
      const lib = await import("./library.js");
      const id = lib.stepCopy(d);
      if (id) return full("library", id);
      return;
    }
    // a row with no mod behind it (listed but not installed) is passed over
    for (let i = 0; i < 50; i++) {
      const before = cur();
      await ui.step(d);
      const it = cur();
      if (it === before) return; // the end of the list
      if (it && it.id) return full("plan");
    }
    return;
  }
  if (k === "Tab" && (F.secs || []).length) { e.preventDefault(); const n = F.secs.length; F.tab = (F.tab + (e.shiftKey ? n - 1 : 1)) % n; return draw(false); }
  if (k === "[" || k === "]") {
    e.preventDefault();
    const cs = copies();
    const i = cs.findIndex((x) => x.ref === F.copy);
    if (cs.length) { F.copy = cs[(i + (k === "]" ? 1 : cs.length - 1)) % cs.length].ref; draw(false); }
    return;
  }
  const c = theCopy();
  if (ops.menuKey(e) && c) {
    e.preventDefault();
    const b = (document.querySelector("#screen .fd-copy.on") || sc).getBoundingClientRect();
    return ops.openMenu(b.left + 24, b.top + 24, c.ops || [], runOp, true);
  }
  if (ctrl && c && ["o", "w", "s"].includes(k.toLowerCase())) { e.preventDefault(); return runOp({ o: "folder", w: "workshop", s: "steam" }[k.toLowerCase()]); }
  if (!ctrl && !e.altKey && c) {
    const name = k === " " ? "space" : k === "Delete" ? "Del" : k;
    const code = ops.keyed(c.ops || [], name);
    if (code) { e.preventDefault(); return runOp(code); }
  }
  if (k === "ArrowDown" || k === "j") sc.scrollTop += 40;
  else if (k === "ArrowUp" || k === "k") sc.scrollTop -= 40;
  else if (k === "PageDown") sc.scrollTop += sc.clientHeight * 0.9;
  else if (k === "PageUp") sc.scrollTop -= sc.clientHeight * 0.9;
  else if (k === "Home") sc.scrollTop = 0;
  else if (k === "End") sc.scrollTop = sc.scrollHeight;
  else return;
  e.preventDefault();
}

// fullSection is the technical section shown, for the position line.
export function fullSection() {
  if (S.screen !== "full" || !F.secs || !F.secs.length) return "";
  return F.secs[F.tab].title;
}

// fullHints: the page's hint line.
export function fullHints() {
  const c = theCopy();
  const out = [{ key: "← →", label: T("previous / next mod") }, { key: "esc", label: T("back") }];
  if (c) {
    const sp = ops.keyed(c.ops || [], "space");
    if (sp) out.push({ key: "space", label: ops.op(sp).label });
    if (copies().length > 1) out.push({ key: "[ ]", label: T("another copy") });
    out.push({ key: "F10", label: T("menu") });
  }
  out.push({ key: "1–6", label: T("its parts") }, { key: "?", label: T("all keys") });
  return out;
}
