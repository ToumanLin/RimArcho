// Screens over the board (06 §4.2): 需要处理 (§7), recent changes (§4.1), the full detail
// (§6.3). esc returns; the top bar shows where you are.
import { S, $, el, esc, api, T, Tf, errText, say, cur } from "./state.js";
import { statusIcon } from "./icons.js";
import { blocks } from "./detail.js";
import * as pop from "./popups.js";

let ui = null;
export function initScreens(u) { ui = u; }

const GROUPS = ["this list", "missing dependencies", "conflicts", "last run", "needs your answer", "hidden mods", "this run"];

function openScreen(name) {
  S.screen = name;
  const sc = $("#screen");
  sc.hidden = false;
  sc.innerHTML = "";
  ui.renderWhere();
  ui.renderHints();
  return sc;
}

export function closeScreen() {
  if (!S.screen) return false;
  S.screen = null;
  $("#screen").hidden = true;
  $("#screen").innerHTML = "";
  ui.renderWhere();
  ui.renderHints();
  return true;
}

// ---- 需要处理 ----
const N = { items: [], cur: 0, open: new Set(), data: null };

// liveChanged redraws 需要处理 when the running game's errors changed (10 §3.4).
export function liveChanged() {
  if (S.screen === "needs" && N.data) {
    N.items = ((S.live && S.live.items) || []).concat(N.data.items || []);
    drawNeeds();
  }
}

export async function needs(keepCursor) {
  let data;
  try { data = await api.Needs(); } catch (e) { say("fail", errText(e)); return; }
  N.data = data;
  N.items = ((S.live && S.live.items) || []).concat(data.items || []);
  if (!keepCursor) N.cur = 0;
  N.cur = Math.min(N.cur, Math.max(N.items.length - 1, 0));
  drawNeeds();
}

function drawNeeds() {
  if (S.screen !== "needs") openScreen("needs");
  const sc = $("#screen");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = el("div", "wrap");
  sc.append(w);
  const d = N.data;
  const bar = el("div", "bar");
  bar.append(el("h2", "", d.count ? Tf("%d to handle", d.count) : T("nothing to handle")));
  let state = "";
  if (S.busy.analyze) state = T("checking the mods' files…");
  else if (d.never) state = T("the mods' files are not checked yet");
  else if (d.stale) state = T("the files changed since the last check");
  if (state) bar.append(el("span", "second", state));
  bar.append(el("span", "grow"));
  const a = el("button", "act", T("check the files again"));
  a.append(el("kbd", "", "A"));
  a.onclick = () => ui.cmd("analyze");
  const l = el("button", "act", T("choose a log"));
  l.append(el("kbd", "", "l"));
  l.onclick = chooseLog;
  bar.append(a, l);
  w.append(bar);
  if (!N.items.length) {
    w.append(el("p", "ok", "✓ " + T("nothing needs your attention")));
    return;
  }
  let g = -1;
  N.items.forEach((it, i) => {
    if (it.group !== g) {
      g = it.group;
      w.append(el("div", "grp", T(GROUPS[g])));
    }
    const row = el("div", "need" + (i === N.cur ? " cur" : "") + (it.dim ? " dimmed" : ""));
    const ic = el("span", "ic");
    ic.innerHTML = it.dim || !it.status ? "" : statusIcon(it.status);
    const body = el("div", "body");
    body.append(el("div", "", it.text));
    if (it.fold && N.open.has(it.text)) {
      const parts = el("div", "parts");
      for (const p of it.parts || []) parts.append(el("div", "", "· " + p));
      body.append(parts);
    }
    const acts = el("div", "acts");
    for (const act of it.actions || []) {
      const b = el("button", "act", act.label);
      b.append(el("kbd", "", act.key === "space" ? "space" : act.key));
      b.onclick = (e) => { e.stopPropagation(); N.cur = i; runAction(it, act); };
      acts.append(b);
    }
    body.append(acts);
    row.append(ic, body);
    row.onclick = () => { N.cur = i; drawNeeds(); };
    w.append(row);
  });
  sc.scrollTop = top;
  const on = $(".need.cur", sc);
  if (on) on.scrollIntoView({ block: "nearest" });
}

// runAction binds a shared action (view.Action) to what the page does.
async function runAction(it, act) {
  switch (act.kind) {
    case "sort": case "save":
      return ui.preview(act.kind);
    case "open-url":
      try { await api.OpenURL(act.url); say("ok", act.done); } catch (e) { say("fail", errText(e)); }
      return;
    case "view":
      try { pop.text(act.label, await api.Inspect(act.argv)); } catch (e) { say("fail", errText(e)); }
      return;
    case "list":
      return pop.text(act.title, act.lines);
    case "fold":
      N.open.has(it.text) ? N.open.delete(it.text) : N.open.add(it.text);
      return drawNeeds();
    case "choose-disable":
      return pop.choose(act.title, act.options.map((o) => o.label), (i) => doAction(act, i));
  }
  if (act.confirm) {
    // Not undone with u: ask first (06 §9, 08 §8).
    return pop.confirm(act.label, [act.warn || act.label], act.label, () => doAction(act, 0));
  }
  return doAction(act, 0);
}

async function doAction(act, option) {
  try { say("ok", await api.DoAction(act, option), act.kind === "enable" || act.kind === "disable" || act.kind === "choose-disable"); } catch (e) { say("fail", errText(e)); }
  await ui.reload();
  if (S.screen === "needs") needs(true);
}

export async function chooseLog() {
  const logs = await api.Logs();
  if (!logs || !logs.length) return say("", T("no Player.log found"));
  pop.choose(T("analyze which log?"), logs, async (i) => {
    try { say("ok", await api.SelectLog(logs[i])); } catch (e) { say("fail", errText(e)); }
    needs();
  });
}

export function needsKey(e) {
  const k = e.key;
  if (k === "ArrowDown" || k === "j") N.cur = Math.min(N.cur + 1, N.items.length - 1);
  else if (k === "ArrowUp" || k === "k") N.cur = Math.max(N.cur - 1, 0);
  else if (k === "PageDown") N.cur = Math.min(N.cur + 10, N.items.length - 1);
  else if (k === "PageUp") N.cur = Math.max(N.cur - 10, 0);
  else if (k === "Home") N.cur = 0;
  else if (k === "End") N.cur = N.items.length - 1;
  else if (k === "A") return ui.cmd("analyze");
  else if (k === "l") return chooseLog();
  else {
    const it = N.items[N.cur];
    const name = k === " " ? "space" : k === "Enter" ? "enter" : k;
    const act = it && (it.actions || []).find((a) => a.key === name);
    if (act) { e.preventDefault(); runAction(it, act); }
    return;
  }
  e.preventDefault();
  drawNeeds();
}

// the analysis' progress and end refresh the screen
export function analyzeEvent(ev) {
  if (S.screen === "needs" && ev.done) needs(true);
}

// ---- recent changes (06 §4.1) ----
const C = { scans: [], cur: 0, rows: [], open: new Map() };

export async function changes() {
  try { C.scans = (await api.Changes()) || []; } catch (e) { say("fail", errText(e)); return; }
  C.cur = 0;
  drawChanges();
}

function drawChanges() {
  if (S.screen !== "changes") openScreen("changes");
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = el("div", "wrap");
  sc.append(w);
  const bar = el("div", "bar");
  bar.append(el("h2", "", T("recent changes")));
  w.append(bar);
  if (!C.scans.length) { w.append(el("p", "dim", T("no scan yet (R scans)"))); return; }
  C.rows = [];
  for (const s of C.scans) {
    w.append(el("div", "scan", Tf("scan %s", s.time) + "  ·  " + s.summary));
    for (const e of s.events || []) {
      const i = C.rows.length;
      C.rows.push(e);
      const r = el("div", "ev" + (i === C.cur ? " cur" : ""));
      r.append(el("span", "", e.mod), el("span", "dim", "·"), el("span", "second", e.text));
      if (e.files) r.append(el("span", "dim", C.open.has(e.id) ? "▾" : "▸"));
      r.onclick = () => { C.cur = i; toggleFiles(e); };
      w.append(r);
      if (C.open.has(e.id)) w.append(el("div", "ev files", C.open.get(e.id).join("\n")));
    }
  }
  const on = $(".ev.cur", sc);
  if (on) on.scrollIntoView({ block: "nearest" });
}

async function toggleFiles(e) {
  if (C.open.has(e.id)) C.open.delete(e.id);
  else if (e.files) C.open.set(e.id, (await api.FileDiff(e.id)) || []);
  drawChanges();
}

export function changesKey(e) {
  const k = e.key;
  if (k === "ArrowDown" || k === "j") C.cur = Math.min(C.cur + 1, C.rows.length - 1);
  else if (k === "ArrowUp" || k === "k") C.cur = Math.max(C.cur - 1, 0);
  else if (k === "Enter" && C.rows[C.cur]) return toggleFiles(C.rows[C.cur]);
  else return;
  e.preventDefault();
  drawChanges();
}

// ---- the full detail (06 §6.3): ←→ the previous / next mod, tab the sub-sections ----
const F = { tab: 0, secs: null, id: "" };

export async function full() {
  const it = cur();
  if (!it || !it.id) return;
  if (F.id !== it.id) { F.tab = 0; F.secs = null; F.id = it.id; }
  openScreen("full");
  drawFull();
  try { F.secs = await api.Sections(it.id); } catch (e) { F.secs = []; say("fail", errText(e)); }
  if (S.screen === "full" && F.id === it.id) drawFull();
}

function drawFull() {
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = el("div", "wrap");
  sc.append(w);
  const d = S.detail;
  const bar = el("div", "bar");
  bar.append(el("h2", "", d ? d.name : ""));
  bar.append(el("span", "second", T("← → the previous / next mod · tab the sections")));
  w.append(bar);
  const tabs = el("div", "tabs");
  const names = [T("summary"), ...(F.secs || []).map((s) => s.title + (s.count >= 0 ? " · " + s.count : ""))];
  names.forEach((n, i) => {
    const b = el("button", i === F.tab ? "on" : "", n);
    b.onclick = () => { F.tab = i; drawFull(); };
    tabs.append(b);
  });
  w.append(tabs);
  if (F.tab === 0) {
    const src = $("#detail");
    w.append(src.cloneNode(true));
    // re-bind the links of the copy through the original's handlers
    w.querySelectorAll(".link").forEach((a, i) => { const o = src.querySelectorAll(".link")[i]; if (o) a.onclick = o.onclick; });
  } else if (F.secs) {
    const s = F.secs[F.tab - 1];
    const pre = el("pre", "lines");
    pre.textContent = (s.lines || []).join("\n");
    w.append(pre);
  } else w.append(el("p", "dim", T("loading…")));
  ui.renderWhere();
}

export function fullKey(e) {
  const n = 1 + (F.secs ? F.secs.length : 0);
  if (e.key === "Tab") { F.tab = (F.tab + (e.shiftKey ? n - 1 : 1)) % n; drawFull(); e.preventDefault(); return; }
  if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
    e.preventDefault();
    ui.step(e.key === "ArrowRight" ? 1 : -1).then(() => full());
    return;
  }
  const sc = $("#screen");
  if (e.key === "ArrowDown") sc.scrollTop += 40;
  else if (e.key === "ArrowUp") sc.scrollTop -= 40;
  else if (e.key === "PageDown" || e.key === " ") sc.scrollTop += sc.clientHeight * 0.9;
  else if (e.key === "PageUp") sc.scrollTop -= sc.clientHeight * 0.9;
  else return;
  e.preventDefault();
}

export function fullSection() {
  if (S.screen !== "full" || !F.secs || F.tab === 0) return "";
  return F.secs[F.tab - 1].title;
}

// ---- settings (10 §7) ----
// Every row is a value and the command that changes it (view.SettingsPage); the page runs
// only those commands (API.ApplySetting).

export async function settings() {
  let groups;
  try { groups = (await api.Settings()) || []; } catch (e) { say("fail", errText(e)); return; }
  if (S.screen !== "settings") openScreen("settings");
  const sc = $("#screen");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = el("div", "wrap settings");
  sc.append(w);
  const bar = el("div", "bar");
  bar.append(el("h2", "", T("settings")));
  w.append(bar);
  const run = async (argv, value, after) => {
    try {
      const out = await api.ApplySetting(argv, value || "");
      say("ok", (out && out[0]) || T("done"));
    } catch (e) { say("fail", errText(e)); }
    if (after) return after();
    await ui.reload();
    settings();
  };
  for (const g of groups) {
    w.append(el("div", "grp", g.title));
    for (const it of g.items || []) {
      const row = el("div", "set");
      const left = el("div", "l");
      left.append(el("div", "", it.label));
      if (it.note) left.append(el("div", "dim small", it.note));
      const right = el("div", "r");
      if (it.edit === "choice") {
        const s = el("select");
        for (const c of it.choices) {
          const o = el("option", "", c);
          o.value = c;
          if (c === it.value) o.selected = true;
          s.append(o);
        }
        // A new language needs the page's text again: reload after it is set.
        s.onchange = () => run(it.argv, s.value, it.argv[2] === "ui.lang" ? () => location.reload() : null);
        right.append(s);
      } else if (it.edit) {
        const inp = el("input");
        inp.value = it.value || "";
        inp.spellcheck = false;
        const ok = el("button", "act", T("apply"));
        ok.onclick = () => run(it.argv, inp.value.trim());
        inp.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); ok.click(); } };
        right.append(inp, ok);
      } else if (it.value) {
        right.append(el("span", "second", it.value));
      }
      for (const a of it.actions || []) {
        const b = el("button", "act", a.label);
        b.onclick = () => a.confirm ? pop.confirm(a.label, [a.confirm], a.label, () => run(a.argv)) : run(a.argv);
        right.append(b);
      }
      row.append(left, right);
      w.append(row);
    }
  }
  sc.scrollTop = top;
}
