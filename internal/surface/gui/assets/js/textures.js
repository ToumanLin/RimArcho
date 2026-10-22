// 库 › 贴图优化 (08 §5.2, 06 §2.1): the current plan's (or every mod's) PNG textures, what
// RimArcho converted and what went stale, what converting at a scale would take and save,
// then the run in the background ("textures" events), stop, and take everything back. The
// encoder (todds) is downloaded only after the player saw where from and how big.
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button } from "./screens.js";
import * as pop from "./popups.js";

const X = { scope: "plan", scale: 100, page: null, prog: null };

export async function open() { return draw(true); }

async function load() {
  try { X.page = await api.Textures(X.scope, X.scale); } catch (e) { say("fail", errText(e)); return false; }
  X.prog = X.page.running || X.prog;
  if (!X.page.running && X.prog && !X.prog.done) X.prog = null;
  return true;
}

// event is a "textures" event: the run's progress, then its end.
export function event(ev) {
  if (ev.done) {
    X.prog = null;
    say(ev.ok ? "ok" : "fail", ev.ok ? ev.text : ev.error);
    if (S.screen === "textures") draw(true);
    return;
  }
  X.prog = ev;
  if (S.screen !== "textures") { say("busy", T("converting textures") + " · " + ev.text); return; }
  const f = $("#screen .tex-foot");
  if (f) f.replaceWith(foot(X.page));
}

function setRow(card, name, sub, ...right) {
  const row = el("div", "set");
  const l = el("div", "l");
  l.append(el("div", "name", name));
  if (sub) for (const s of [].concat(sub)) if (s) l.append(el("div", "sub", s));
  const r = el("div", "r");
  for (const x of right) if (x) r.append(x);
  row.append(l, r);
  card.append(row);
  return row;
}

// select: a choice that measures again; f names it, so a redraw keeps the focus on it.
function select(f, values, labels, value, onchange) {
  const s = el("select");
  s.dataset.f = f;
  values.forEach((v, i) => {
    const o = el("option", "", labels[i]);
    o.value = String(v);
    if (String(v) === String(value)) o.selected = true;
    s.append(o);
  });
  s.onchange = () => onchange(s.value);
  s.disabled = !!X.prog;
  return s;
}

async function draw(fetch) {
  if (fetch && !(await load())) return;
  const p = X.page;
  if (!p) return;
  const sc = S.screen === "textures" ? $("#screen") : openScreen("textures");
  const top = sc.scrollTop;
  const focused = document.activeElement && document.activeElement.dataset && document.activeElement.dataset.f;
  sc.innerHTML = "";
  const w = page("textures", T("the game loads a .dds in place of the .png of the same name: no PNG to decode at start, and opaque textures take half the video memory"));
  sc.append(w);

  // the encoder
  w.append(el("div", "label section", T("encoder")));
  const enc = el("section", "card settings");
  setRow(enc, T("the DDS encoder"), p.toolOk ? null : [p.offer, p.offerTo && "→ " + p.offerTo], p.toolOk ? el("span", "muted", p.tool) : null,
    p.toolOk ? null : button(T("download todds…"), "", () => install(p), "primary"));
  if (!p.toolOk) enc.querySelector(".name").after(el("div", "sub warn", p.tool));
  w.append(enc);

  // what is there
  w.append(el("div", "label section", T("what is there")));
  const now = el("section", "card settings");
  setRow(now, T("mods"), Tf("%d mods", p.mods),
    select("scope", ["plan", "all"], [T("the current plan"), T("every installed mod")], X.scope, (v) => { X.scope = v; draw(true); }));
  setRow(now, "PNG", p.png);
  if (p.ours) {
    setRow(now, T("RimArcho's DDS"), [p.ours, p.staleText], button(T("remove every converted texture…"), "", () => clean(), "danger"));
  }
  w.append(now);

  // what converting does
  w.append(el("div", "label section", T("what converting does")));
  const est = el("section", "card settings");
  setRow(est, T("scale"), T("scaling shrinks them further; never below 128 px, masks stay full size"),
    select("scale", p.scales, p.labels, X.scale, (v) => { X.scale = Number(v); draw(true); }));
  const facts = [...(p.estFacts || [])];
  if (p.upToDate) facts.push(Tf("%d up to date", p.upToDate));
  if (p.leftAlone) facts.push(Tf("%d left alone (a DDS of the mod's own, or another format of the same name)", p.leftAlone));
  setRow(est, T("convert"), facts);
  setRow(est, "", T("the PNGs stay; RimArcho writes a .dds next to each and can take them all back"));
  w.append(est);
  w.append(foot(p));

  // the mods, the biggest first
  if ((p.rows || []).length) {
    const t = el("section", "card tex-rows");
    const head = el("div", "tr th");
    for (const h of [T("mod"), "PNG", T("to convert"), T("RimArcho's DDS"), T("stale"), T("own DDS")]) head.append(el("span", "", h));
    t.append(head);
    for (const r of p.rows.slice(0, 200)) {
      const tr = el("div", "tr" + (r.stale ? " stale" : ""));
      tr.append(el("span", "n", r.name), el("span", "", r.png), el("span", "", r.convert ? String(r.convert) : ""), el("span", "", r.dds || ""),
        el("span", r.stale ? "warn" : "", r.stale ? String(r.stale) : ""), el("span", "muted", r.own ? String(r.own) : ""));
      t.append(tr);
    }
    w.append(t);
  }
  sc.scrollTop = top;
  if (focused) { const n = sc.querySelector(`[data-f="${focused}"]`); if (n) n.focus(); }
}

// foot: the run's progress and stop, or the convert button.
function foot(p) {
  const f = el("div", "tex-foot");
  if (X.prog) {
    const w = el("div", "prog");
    const bar = el("div", "bar");
    const fill = el("i");
    fill.style.width = Math.round((X.prog.frac || 0) * 100) + "%";
    bar.append(fill);
    w.append(bar, el("div", "ptext", X.prog.text || T("converting textures")));
    f.append(w, button(T("stop"), "", async () => {
      try { say("ok", await api.TexturesStop()); } catch (e) { say("fail", errText(e)); }
    }));
    return f;
  }
  const go = button(T("convert"), "", () => convert(p), "primary");
  go.disabled = !p.toolOk || !p.convert;
  f.append(go);
  return f;
}

function convert(p) {
  const lines = [...(p.estFacts || []), T("the PNGs stay; RimArcho writes a .dds next to each and can take them all back")];
  pop.confirm(T("convert textures to DDS"), lines, T("convert"), async () => {
    try { say("ok", await api.TexturesRun(X.scope, X.scale, [])); } catch (e) { say("fail", errText(e)); return; }
    X.prog = { text: T("converting textures"), frac: 0 };
    draw(false);
  });
}

function install(p) {
  pop.confirm(T("download todds…"), [p.offer, "→ " + p.offerTo], T("download todds…"), async () => {
    say("busy", T("download todds…"));
    try { say("ok", await api.TexturesInstall()); } catch (e) { say("fail", errText(e)); }
    draw(true);
  });
}

function clean() {
  pop.confirm(T("remove every converted texture…"), [T("the DDS RimArcho wrote are deleted; the PNGs stay and the game uses them again")], T("remove every converted texture…"), async () => {
    try { say("ok", await api.TexturesClean(X.scope)); } catch (e) { say("fail", errText(e)); }
    draw(true);
  });
}
