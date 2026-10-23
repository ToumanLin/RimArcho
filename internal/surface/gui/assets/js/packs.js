// 整合包 (06 §2.6, §2.7, 08 §5): the packs (the sidebar's 整合包), each pack's own page
// (its sub-entries), and 编译成整合包, the plan's sub-page: on the left the name and the
// things building does, on the right the summary and 编译, disabled with the reason when
// something stops the build (G14); the builds cut short below. A pack's list alone is
// exported from its page as a RimArcho list (E).
// Data: API.Packs (view.PackRows, view.UnfinishedRows), API.Pack (view.BuildPackPage),
// API.BuildPage (view.BuildPageOf), API.PackDryRun (archo pack build --dry-run).
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button, act, area, beginPageRead } from "./screens.js";
import * as pop from "./popups.js";

let ui = null;
export function initPacks(u) { ui = u; }

let packPoll = 0;
const L = { rows: [], un: [], others: 0, cur: 0, els: [] }; // the list page
let P = null; // the pack page shown: view.PackPage
const B = { page: null, tex: 0, name: "", nameErr: "", seq: 0, loading: false }; // the build page

// ---- the list (整合包) ----

async function loadList(current = () => true) {
  const l = await api.Packs();
  if (!current()) return;
  L.rows = (l && l.rows) || []; L.others = (l && l.others) || 0; L.un = (l && l.unfinished) || [];
  // This window's build before its child wrote its first state.
  if (l && l.building && !L.un.some((u) => u.name === l.building)) L.un.unshift({ name: l.building, running: true, facts: T("checking the list…") });
  for (const u of L.un) if (u.running && S.packBuild && S.packBuild.name === u.name) u.progress = S.packBuild;
}

// list: every pack of the environment, one row each; enter (or a click) opens its page.
export async function list() {
  const read = beginPageRead("packs", { replace: !!P });
  S.packFocus = null; P = null;
  try { await loadList(read.current); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  const sc = S.screen === "packs" && !S.packFocus ? $("#screen") : openScreen("packs");
  sc.innerHTML = "";
  const w = page("packs", T("a pack copies every mod of the list as it is now and plays with its own saves"),
    act(T("create a pack"), "packs", () => ui.go("build"), "B"));
  sc.append(w);
  for (const u of L.un) w.append(buildCard(u));
  const card = el("section", "card");
  L.els = [];
  if (!L.rows.length) {
    card.append(el("div", "empty-state", T("no packs yet: a pack fixes the current list — every mod as it is now, without the files of other game versions — and plays with its own saves")));
  }
  L.cur = Math.min(L.cur, Math.max(L.rows.length - 1, 0));
  L.rows.forEach((r, i) => {
    const row = el("div", "pack link" + (i === L.cur ? " cur" : ""));
    const left = el("div", "l");
    left.append(el("div", "name", r.name), el("div", "sub", r.facts + " · " + r.played));
    row.append(left, el("span", "go", "→"));
    row.onclick = () => ui.go("pack:" + r.name);
    L.els.push(row);
    card.append(row);
  });
  w.append(card);
  if (L.others) w.append(el("div", "muted note", Tf("%d packs belong to other games; switch the game to see them", L.others)));
  ui.renderHints();
  pollWhileBuilding(list);
  read.finish();
}

function markList() {
  L.els.forEach((r, i) => r.classList.toggle("cur", i === L.cur));
  if (L.els[L.cur]) L.els[L.cur].scrollIntoView({ block: "nearest" });
}

// ---- one pack (a sub-entry of 整合包) ----

export async function pack(name) {
  const read = beginPageRead("packs", { replace: !P || P.name !== name });
  S.packFocus = name; P = null;
  let p;
  try { p = await api.Pack(name); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  P = p;
  const sc = S.screen === "packs" && S.packFocus === name ? $("#screen") : openScreen("packs");
  sc.innerHTML = "";
  const play = act(T("play"), "play", () => launch(), "L");
  play.classList.add("primary");
  const w = page("packs pack-page", P.facts.join(" · "), button(T("export"), "e", () => exportPack(false)),
    button(T("export the list"), "E", () => exportPack(true)), button(T("move to the recycle bin"), "Del", () => removePack()), play);
  sc.append(w);
  const sec = el("section", "sec");
  sec.append(el("h2", "label", T("the mods, in load order")));
  const card = el("div", "card pack-mods");
  for (const m of P.mods) {
    const r = el("div", "pm");
    r.append(el("span", "n", m.name), el("span", "x muted", [m.note, m.size].filter(Boolean).join(" · ")));
    card.append(r);
  }
  sec.append(card);
  w.append(sec, el("div", "muted note dir", T("folder") + ": " + P.dir));
  ui.renderHints(); // the list and a pack's page share the screen: their keys differ
  read.finish();
}

async function launch() {
  if (!P) return;
  await launchPack(P.name);
}

// launchPack plays a pack (here and on 启动): when Mods/ holds another version of one of its
// mods that can move into the lock library, one confirmation lists the moves, then it goes
// on (08 §5.3).
export async function launchPack(name) {
  let o;
  try { o = await api.PackLaunch(name, false); } catch (e) { return say("fail", errText(e)); }
  if (o.moves) {
    pop.confirm(T("move the other versions into the lock library"), o.moves, T("move and go on"), async () => {
      say("busy", T("moving into the lock library…"));
      let o2;
      try { o2 = await api.PackLaunch(name, true); } catch (e) { return say("fail", errText(e)); }
      say(o2.refused ? "fail" : "ok", o2.text);
      if (o2.refused && o2.refused.length > 1) pop.text(T("launch"), o2.refused);
      ui.reload();
    });
    return;
  }
  say(o.refused ? "fail" : "ok", o.text);
  if (o.refused && o.refused.length > 1) pop.text(T("launch"), o.refused);
  ui.reload();
}

// exportPack writes the pack to a file, or with list only its list (a RimArcho list: each
// mod with its version, for sharing): the save dialog in the window, a typed path in the
// development harness.
async function exportPack(list) {
  if (!P) return;
  const name = P.name;
  try {
    const t = await api.PackExport(name, "", list);
    if (t) say("ok", t);
  } catch (e) {
    pop.ask(list ? T("export the list") : T("export the pack"), T("the new file") + " · " + name + (list ? ".archolist.json" : ".archopack"), async (file) => {
      if (!file) return;
      try { say("ok", await api.PackExport(name, file, list)); } catch (e2) { say("fail", errText(e2)); }
    });
  }
}

function removePack() {
  if (!P) return;
  const name = P.name;
  pop.confirm(T("move to the recycle bin"), [Tf("pack %s goes to the recycle bin; you can restore it from there", name)], T("move to the recycle bin"), async () => {
    try { say("ok", await api.PackRemove(name)); } catch (e) { return say("fail", errText(e)); }
    await ui.reload();
    ui.go("packs");
  });
}

// ---- builds running or cut short ----

function progressBox(p) {
  const w = el("div", "prog");
  const bar = el("div", "bar" + (p.error ? " bad" : ""));
  const fill = el("i");
  fill.style.width = Math.round((p.frac || 0) * 100) + "%";
  bar.append(fill);
  w.append(bar, el("div", "ptext" + (p.error ? " bad" : ""), p.error || p.text || ""));
  return w;
}

// buildCard is a build: running (here or in another window) or cut short (delete what it
// copied — RimArcho's own unfinished copy, 08 §5.2 — or build it again).
function buildCard(u) {
  const card = el("section", "card build" + (u.running ? "" : " unfinished"));
  card.dataset.build = u.name;
  const head = el("div", "pack");
  const left = el("div", "l");
  left.append(el("div", "name", u.running ? Tf("creating pack %s", u.name) : u.name), el("div", "sub", u.facts || ""));
  const right = el("div", "r");
  if (u.running) {
    right.append(button(T("stop"), "", async () => {
      try { say("ok", await api.PackStop(u.name)); } catch (e) { say("fail", errText(e)); }
    }));
  } else {
    right.append(button(T("create again"), "", () => startBuild(u.name)));
    right.append(button(T("delete what it copied"), "", () => pop.confirm(T("delete what it copied"), [Tf("the unfinished pack %s and what it copied (%s) are deleted; your mods are not touched", u.name, u.copied || "?")], T("delete what it copied"), async () => {
      say("busy", T("deleting what it copied…"));
      try { say("ok", await api.PackRemoveUnfinished(u.name)); } catch (e) { say("fail", errText(e)); }
      redraw();
    }), "danger"));
  }
  head.append(left, right);
  card.append(head);
  if (u.running) {
    const body = el("div", "progwrap");
    body.append(progressBox(u.progress || { frac: 0, text: "" }));
    card.append(body);
  }
  return card;
}

// A build this window does not watch (started before a restart, or elsewhere): look again
// while it runs.
function pollWhileBuilding(again) {
  clearTimeout(packPoll);
  if (L.un.some((u) => u.running && !(S.packBuild && S.packBuild.name === u.name))) {
    const screen = S.screen, epoch = S.screenSeq, focus = S.packFocus;
    packPoll = setTimeout(() => { if (S.screen === screen && S.screenSeq === epoch && S.packFocus === focus && !S.popup) again(); }, 1500);
  }
}

function redraw() {
  if (S.screen === "build") return build(true);
  if (S.screen === "packs") return S.packFocus ? pack(S.packFocus) : list();
}

// packEvent is a "pack" event: the progress of the build this window started, and its end
// (a finished build opens its page).
export function packEvent(ev) {
  S.packBuild = ev.done ? null : ev;
  if (ev.done) {
    say(ev.ok ? "ok" : "fail", ev.ok ? ev.text : ev.error);
    ui.reload();
    if (ev.ok && S.screen === "build") return ui.go("pack:" + ev.name);
    return redraw();
  }
  if (S.screen === "build") return drawBuild();
  if (S.screen !== "packs" || S.packFocus) { say("busy", Tf("creating pack %s", ev.name) + " · " + ev.text); return; }
  const card = document.querySelector(`.card.build[data-build="${CSS.escape(ev.name)}"]`);
  if (!card) return list();
  const body = card.querySelector(".progwrap");
  if (body) { body.innerHTML = ""; body.append(progressBox(ev)); }
  card.querySelector(".sub").textContent = T("creating") + " · " + ev.text;
}

// ---- 编译成整合包 (the plan's sub-page) ----

// build opens the page: the check is quick, the sizes need every mod's folder walked, so
// the page shows at once and the sizes follow. again: a redraw keeps the name typed.
export async function build(again = false) {
  const seq = ++B.seq;
  if (!again) { B.page = null; B.tex = 0; B.name = ""; B.nameErr = ""; }
  const read = beginPageRead("build", { replace: !again, label: "checking the list…" });
  B.loading = true;
  try {
    const [p] = await Promise.all([
      B.page || api.BuildPage(B.tex || 0),
      loadList(read.current).catch(() => { if (read.current()) L.un = []; }),
    ]);
    if (!read.current() || seq !== B.seq) return;
    B.page = p;
    if (!B.name) B.name = p.name;
    const error = await api.PackNameCheck(B.name);
    if (!read.current() || seq !== B.seq) return;
    B.nameErr = error;
  } catch (e) { if (read.current()) B.loading = false; read.fail(e); return; }
  B.loading = false;
  drawBuild();
  pollWhileBuilding(() => build(true));
  read.finish();
}

// setTex changes the textures choice: the page is measured again, since the files
// converted, the size after and the peak all depend on it.
async function setTex(tex) {
  if (B.loading || B.tex === tex) return;
  return measureBuild(tex);
}

async function measureBuild(tex) {
  B.tex = tex;
  const seq = ++B.seq;
  const read = beginPageRead("build", { replace: true, label: "checking the list…" });
  B.loading = true;
  let p;
  try { p = await api.BuildPage(tex); } catch (e) { if (read.current()) B.loading = false; read.fail(e); return; }
  if (!read.current() || seq !== B.seq) return;
  B.page = p;
  B.loading = false;
  drawBuild();
  read.finish();
}

// nextTex: the D key steps through the choices.
function nextTex(d) {
  const cs = (B.page && B.page.texChoices) || [];
  if (!cs.length) return;
  const i = cs.findIndex((c) => c.value === B.tex);
  setTex(cs[(i + d + cs.length) % cs.length].value);
}

const building = () => !!(S.packBuild || L.un.some((u) => u.running));

function drawBuild() {
  const p = B.page;
  if (!p || S.screen !== "build" || B.loading) return;
  const sc = S.screen === "build" ? $("#screen") : openScreen("build");
  const focusName = document.activeElement && document.activeElement.id === "pk-name";
  sc.innerHTML = "";
  const w = page("build", T("a pack locks every mod's version: later Workshop updates do not change it, and it has its own saves and settings; the list stays as it is"),
    button(T("cancel"), "esc", () => ui.go("plan")));
  const cols = el("div", "bd-cols");
  const left = el("div", "bd-left");

  // the name
  const basics = el("section", "sec");
  const basicsH = el("h2", "label", T("the pack"));
  basics.append(basicsH);
  area(basics, basicsH);
  const bc = el("div", "card bd-basics");
  const nameRow = el("label", "bd-row");
  const name = el("input", "field");
  name.id = "pk-name";
  name.spellcheck = false;
  name.value = B.name;
  name.oninput = async () => {
    B.name = name.value;
    const epoch = S.screenSeq, seq = B.seq;
    const n = B.name, err = await api.PackNameCheck(n);
    if (n !== B.name || epoch !== S.screenSeq || seq !== B.seq || S.screen !== "build") return;
    B.nameErr = err;
    syncButton();
  };
  name.onkeydown = (e) => {
    if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); go(); }
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); name.blur(); }
  };
  nameRow.append(el("span", "k", T("name")), name);
  bc.append(nameRow, el("div", "bd-err", ""));
  basics.append(bc);

  // the four things building does; while it builds, where it is
  const steps = el("section", "sec");
  const stepsH = el("h2", "label", T("what creating does"));
  steps.append(stepsH);
  area(steps, stepsH);
  const sc2 = el("div", "card bd-steps");
  // A build's progress (view.ProgressOf: 0 check, 2 copy, 3 textures, 4 write, 5 done) marks
  // the steps it is at; copying also locks the versions (origin.json in each copy).
  const NOW = { 0: ["check"], 2: ["trim", "lock"], 3: ["textures"], 4: ["place"], 5: [] };
  const now = S.packBuild ? NOW[S.packBuild.step] || ["check"] : null;
  const first = now ? (now.length ? p.steps.findIndex((s) => now.includes(s.key)) : p.steps.length) : -1;
  p.steps.forEach((s, i) => {
    let cls = "bd-step" + (s.level ? " " + s.level : "");
    const done = now && !now.includes(s.key) && (i < first || (s.key === "lock" && S.packBuild.step >= 3));
    if (now) cls += done ? " done" : now.includes(s.key) ? " cur" : " wait";
    const r = el("div", cls);
    const n = el("span", "n", done ? "✓" : s.level === "bad" ? "!" : String(i + 1));
    const body = el("div", "b");
    body.append(el("div", "t", s.title), el("div", "x", s.text));
    if (s.more && s.more.length) {
      const ul = el("ul");
      for (const m of s.more) ul.append(el("li", "", m));
      body.append(ul);
    }
    r.append(n, body);
    if (s.kind === "textures" && p.texChoices) {
      // the choice itself, a native select: tab reaches it, ↑↓ change it
      const sel = el("select", "field bd-tex");
      sel.id = "pk-tex";
      for (const c of p.texChoices) {
        const o = el("option", "", c.label);
        o.value = String(c.value);
        sel.append(o);
      }
      sel.value = String(B.tex);
      sel.onchange = () => setTex(+sel.value);
      sel.onkeydown = (e) => { if (e.key === "Escape") { e.stopPropagation(); sel.blur(); } };
      body.insertBefore(sel, body.children[1]);
    }
    if (s.go) r.append(button(T("install"), "", () => ui.go(s.go), "primary"));
    if (s.needs) r.append(button(T("fix them"), "", () => ui.go("needs"), "primary"));
    sc2.append(r);
  });
  if (S.packBuild) {
    // this window's build: where it is, and 停止 (what it copied is removed)
    const pw = el("div", "progwrap bd-prog");
    const name = S.packBuild.name;
    pw.append(progressBox(S.packBuild), button(T("stop"), "", async () => {
      try { say("ok", await api.PackStop(name)); } catch (e) { say("fail", errText(e)); }
    }));
    sc2.append(pw);
  }
  steps.append(sc2);
  left.append(basics, steps);

  // the summary and 编译
  const side = el("aside", "card bd-side");
  side.append(el("div", "h", B.name || p.plan));
  const kv = el("div", "bd-kv");
  const row = (k, v) => kv.append(el("span", "k", k), v instanceof Node ? v : el("span", "v", v));
  row(T("game"), p.env);
  row(T("mods"), el("span", "v num", String(p.mods)));
  if (p.before) {
    const v = el("span", "v");
    v.append(p.before + " → ", el("b", "", p.after));
    row(T("size"), v);
  } else row(T("size"), el("span", "v muted", T("worked out once the problems are fixed")));
  row(T("saves"), p.saves);
  row(T("location"), el("span", "v path", p.dir));
  side.append(kv);
  const go1 = el("button", "btn primary bd-go");
  go1.append(T("create pack"), el("kbd", "", "B"));
  go1.onclick = () => go();
  const why = el("div", "bd-why");
  const dry = button(T("see the list first, write nothing"), "V", () => dryRun());
  dry.classList.add("bd-dry");
  side.append(go1, why, dry);
  cols.append(left, side);
  w.append(cols);

  // builds cut short (or running in another window) below
  const un = L.un.filter((u) => !(S.packBuild && S.packBuild.name === u.name));
  if (un.length) {
    const s = el("section", "sec");
    s.append(el("h2", "label", T("unfinished packs")));
    for (const u of un) s.append(buildCard(u));
    w.append(s);
  }
  sc.append(w);
  syncButton();
  if (focusName) { const n = $("#pk-name"); n.focus(); n.setSelectionRange(n.value.length, n.value.length); }
}

// syncButton: 编译 is enabled only when nothing stops the build; the line under it says
// what does, and the check's problems lead to 能不能跑 (G14).
function syncButton() {
  const p = B.page;
  const go1 = $(".bd-go"), why = $(".bd-why"), err = $(".bd-err"), head = $(".bd-side .h");
  if (!go1 || !p) return;
  if (head) head.textContent = B.name.trim() || p.plan;
  err.textContent = B.nameErr || "";
  let reason = "", toNeeds = false, toGo = "";
  if (!p.ok) { reason = p.why; toNeeds = p.toNeeds; toGo = p.toGo || ""; }
  else if (S.packBuild) reason = Tf("creating pack %s", S.packBuild.name);
  else if (building()) reason = T("another pack is being created; wait for it to finish");
  else if (B.nameErr) reason = B.nameErr;
  go1.disabled = !!reason;
  why.classList.toggle("busy", !!S.packBuild && p.ok);
  why.innerHTML = "";
  why.hidden = !reason;
  if (reason) {
    if (toNeeds || toGo) {
      const a = el("button", "bd-link", reason + " →");
      a.title = toGo ? T("texture optimisation") : T("can it run");
      a.onclick = () => ui.go(toGo || "needs");
      why.append(a);
    } else why.append(el("span", "", reason));
  }
}

function go() {
  const p = B.page;
  if (!p || B.loading || !p.ok || B.nameErr || building()) return;
  startBuild(B.name.trim());
}

async function startBuild(name) {
  try { say("ok", await api.PackBuild(name, B.tex || 0)); } catch (e) { return say("fail", errText(e)); }
  redraw();
}

// dryRun is 先看清单，不写磁盘: the text of archo pack build --dry-run.
async function dryRun() {
  if (B.loading) return;
  const name = (B.name || "").trim() || (B.page && B.page.name) || "";
  try { pop.text(T("see the list first, write nothing"), await api.PackDryRun(name, B.tex || 0)); } catch (e) { say("fail", errText(e)); }
}

// ---- keys ----

// packsKey: the list (↑↓, enter, B), a pack's page (L, e, E, Del) and the build page (B,
// V, D, esc back to the plan).
export function packsKey(e) {
  const k = e.key;
  const kk = e.shiftKey && /^[a-z]$/.test(k) ? k.toUpperCase() : k;
  if (S.screen === "build") {
    if (kk === "B" || (k === "Enter" && e.ctrlKey)) { e.preventDefault(); return go(); }
    if (kk === "V") { e.preventDefault(); return dryRun(); }
    if (kk === "D") { e.preventDefault(); return nextTex(1); }
    if (k === "n" || k === "/") { e.preventDefault(); const n = $("#pk-name"); if (n) n.focus(); return; }
    if (k === "Escape") { e.preventDefault(); return ui.go("plan"); }
    return;
  }
  if (S.packFocus) {
    if (kk === "L") { e.preventDefault(); return launch(); }
    if (k === "e") { e.preventDefault(); return exportPack(false); }
    if (kk === "E") { e.preventDefault(); return exportPack(true); }
    if (k === "Delete") { e.preventDefault(); return removePack(); }
    return;
  }
  if (k === "ArrowDown" || k === "ArrowUp" || k === "j" || k === "k") {
    e.preventDefault();
    if (!L.rows.length) return;
    L.cur = Math.max(0, Math.min(L.rows.length - 1, L.cur + (k === "ArrowDown" || k === "j" ? 1 : -1)));
    return markList();
  }
  if (k === "Home" || k === "End") { e.preventDefault(); L.cur = k === "Home" ? 0 : Math.max(L.rows.length - 1, 0); return markList(); }
  if (k === "Enter" && L.rows[L.cur]) { e.preventDefault(); return ui.go("pack:" + L.rows[L.cur].name); }
  if (kk === "B") { e.preventDefault(); return ui.go("build"); }
}

export function hintList() {
  const tail = [{ key: "shift+1–8", label: T("pages") }, { key: "?", label: T("all keys") }];
  if (S.screen === "build") {
    return [{ key: "B", label: T("create pack") }, { key: "V", label: T("see the list first") }, { key: "D", label: T("textures") },
      { key: "esc", label: T("back to the list") }, ...tail];
  }
  if (S.packFocus) {
    return [{ key: "L", label: T("play") }, { key: "e", label: T("export") }, { key: "E", label: T("export the list") }, { key: "Del", label: T("move to the recycle bin") },
      { key: "tab", label: T("next pack") }, ...tail];
  }
  return [{ key: "↑↓", label: T("move") }, { key: "enter", label: T("open") }, { key: "B", label: T("create a pack") }, { key: "tab", label: T("first pack") }, ...tail];
}
