// 设置 › 卸载 RimArcho (06 §2.14): the dry run of every step (view.UninstallPage), the player's
// answers inline on the steps that have them (a checkbox, and where kept things go), one
// button that runs it, the progress, then the same rows as the report. Everything is a
// native control, so tab, space and enter do what the mouse does.
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button, pickFolder, beginPageRead } from "./screens.js";

const fresh = () => ({ keepLocked: false, lockedTo: "", removePacks: false, packsTo: "", removeSaves: false, savesTo: "", removeGames: false, restoreBackup: false });
const U = { choice: fresh(), page: null, running: false, prog: null, loading: false };

// locked: while it runs, and once it has run without a failure, no other page opens (every
// command is refused after an uninstall).
export function locked() { return U.running || !!(U.page && U.page.ran && !U.page.failed); }

export async function open() {
  if (S.screen !== "uninstall" && !locked()) { U.choice = fresh(); U.page = null; }
  return draw(!locked());
}

// answer sets one answer and draws the plan again, keeping the focus on the same control.
async function answer(k, v) {
  U.choice[k] = v;
  const f = document.activeElement && document.activeElement.dataset.f;
  await draw(true);
  if (f) { const n = $(`#screen [data-f="${f}"]`); if (n) n.focus(); }
}

async function draw(fetch) {
  if (fetch) {
    const read = beginPageRead("uninstall", { replace: true });
    U.loading = true;
    try {
      const p = await api.UninstallPlan({ ...U.choice });
      if (!read.current()) return;
      U.page = p;
    } catch (e) { read.fail(e); return; }
    finally { if (read.current()) { U.loading = false; read.finish(); } }
  }
  if (S.screen !== "uninstall") return;
  const pg = U.page;
  if (!pg) return;
  const sc = S.screen === "uninstall" ? $("#screen") : openScreen("uninstall");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("uninstall", pg.ran ? "" : T("what RimArcho put on this machine, and what happens to each on uninstall"));
  sc.append(w);
  if (pg.ran) w.append(el("p", "un-result " + (pg.failed ? "err" : "ok"), pg.result));
  else if (pg.blocked) w.append(el("p", "un-result err", Tf("can't uninstall now: %s", pg.blocked)));
  const card = el("section", "card settings uninstall");
  for (const s of pg.steps || []) card.append(stepRow(pg, s));
  w.append(card);
  w.append(foot(pg));
  sc.scrollTop = top;
}

function stepRow(pg, s) {
  const box = el("div", "un-step");
  const row = el("div", "set");
  const l = el("div", "l");
  l.append(el("div", "name", s.title));
  l.append(el("div", "sub word-facts", s.text + (s.size ? "  " + s.size : "")));
  const r = el("div", "r");
  if (s.toggle && !pg.ran) {
    const lab = el("label", "un-toggle");
    const cb = el("input");
    cb.type = "checkbox";
    cb.checked = !!s.on;
    cb.dataset.f = s.toggle;
    cb.disabled = U.running;
    cb.onchange = () => answer(s.toggle, cb.checked);
    lab.append(cb, el("span", "", s.label));
    r.append(lab);
  }
  row.append(l, r);
  box.append(row);
  // where kept things go
  if (!pg.ran) {
    if (s.key === "locked" && U.choice.keepLocked && (pg.envs || []).length > 1) box.append(envPick(pg));
    if (s.key === "packs" && !U.choice.removePacks && (s.items || []).some((it) => it.to)) box.append(folderPick("packsTo", pg.packsTo));
    if (s.key === "saves" && !U.choice.removeSaves) box.append(folderPick("savesTo", pg.savesTo));
  }
  box.append(items(s));
  return box;
}

function envPick(pg) {
  const q = el("div", "un-ask");
  const sel = el("select");
  sel.dataset.f = "lockedTo";
  for (const n of pg.envs) {
    const o = el("option", "", n);
    o.value = n;
    if (n === pg.lockedTo) o.selected = true;
    sel.append(o);
  }
  sel.onchange = () => answer("lockedTo", sel.value);
  const [a, b] = T("to %s's Mods folder").split("%s");
  q.append(el("span", "muted", a), sel, el("span", "muted", b || ""));
  return q;
}

function folderPick(k, value) {
  const q = el("div", "un-ask");
  const inp = el("input", "field");
  inp.value = value || "";
  inp.spellcheck = false;
  inp.dataset.f = k;
  const set = () => { if (inp.value.trim() !== (value || "")) answer(k, inp.value.trim()); };
  inp.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); set(); } };
  inp.onchange = set;
  q.append(el("span", "muted", T("move to")), inp, button(T("choose…"), "", () => pickFolder(inp.value, (p) => answer(k, p))));
  return q;
}

// items: the folders and files of a step, folded (summary is a native control: enter opens it).
function items(s) {
  const d = el("details", "un-items");
  const n = (s.items || []).length;
  const bad = (s.items || []).filter((it) => it.err).length;
  d.append(el("summary", bad ? "err word-facts" : "word-facts", bad ? `${n}    ${T("failed")} ${bad}` : String(n)));
  if (bad) d.open = true;
  for (const it of s.items || []) {
    const li = el("div", "un-item" + (it.err ? " bad" : ""));
    const mark = it.err ? "✗" : it.done ? "✓" : it.later ? "…" : "";
    li.append(el("span", "mk " + (it.err ? "err" : "ok"), mark), el("span", "vb", it.verb));
    const p = el("span", "p", it.path + (it.to ? " → " + it.to : ""));
    p.title = p.textContent;
    li.append(p);
    const extra = [it.name, it.size, it.note, it.err].filter(Boolean).join("  ");
    if (extra) li.append(el("span", "x muted word-facts", extra));
    d.append(li);
  }
  return d;
}

function foot(pg) {
  const f = el("div", "un-foot");
  if (U.running) {
    const p = U.prog || {};
    const w = el("div", "prog");
    const bar = el("div", "bar");
    const fill = el("i");
    fill.style.width = (p.n ? Math.round(100 * p.i / p.n) : 0) + "%";
    bar.append(fill);
    w.append(bar, el("div", "ptext", [T("uninstalling…"), p.step, p.n ? `${p.i} / ${p.n}` : ""].filter(Boolean).join(" ")));
    f.append(w);
    return f;
  }
  if (pg.ran && !pg.failed) {
    f.append(button(T("close RimArcho"), "", async () => {
      if (!(await api.Quit().catch(() => false))) say("", T("close the window: RimArcho deletes its data and the program once it has closed"));
    }, "primary"));
    return f;
  }
  if (pg.ran) {
    f.append(button(T("uninstall RimArcho…"), "", () => { U.page = null; draw(true); }));
    return f;
  }
  // The one confirmation: this button, under the whole plan.
  const go = button(T("uninstall RimArcho"), "", run, "primary danger-go");
  go.disabled = U.loading || !!pg.blocked;
  f.append(go);
  return f;
}

async function run() {
  if (U.running || U.loading) return;
  try { await api.UninstallRun(U.choice); } catch (e) { say("fail", errText(e)); return draw(true); }
  U.running = true;
  U.prog = null;
  draw(false);
}

// event is an "uninstall" event: progress, then the report.
export function event(ev) {
  if (!ev) return;
  if (!ev.done) {
    U.prog = ev;
    const t = $("#screen .un-foot .ptext");
    const i = $("#screen .un-foot .bar i");
    if (t) t.textContent = [T("uninstalling…"), ev.step, `${ev.i} / ${ev.n}`].join(" ");
    if (i) i.style.width = Math.round(100 * ev.i / ev.n) + "%";
    return;
  }
  U.running = false;
  if (ev.error) { say("fail", ev.error); return draw(true); }
  U.page = ev.page;
  if (S.screen === "uninstall") draw(false);
}
