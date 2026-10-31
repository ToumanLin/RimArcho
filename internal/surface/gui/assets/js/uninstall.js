// 设置 › 卸载 RimArcho (06 §2.14): the dry run of every step (view.UninstallPage), the player's
// answers inline on the steps that have them (a checkbox, and where kept things go), one
// confirmation dialog, a dedicated progress page, then the same rows as the report.
// Plan/report controls keep their native keyboard behavior.
import { choiceControl } from "./choices.js";
import * as pop from "./popups.js";
import { copyablePath } from "./fields.js";
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button, pickFolder, beginPageRead } from "./screens.js";

const fresh = () => ({ keepLocked: false, lockedTo: "", removePacks: false, packsTo: "", removeSaves: false, savesTo: "", removeGames: false, restoreBackup: false });
const U = { choice: fresh(), page: null, running: false, prog: null, loading: false };
let events = 0;

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
  if (U.running) return drawProgress();
  exclusive();
  let read;
  if (fetch) {
    read = beginPageRead("uninstall", { replace: true });
    U.loading = true;
    try {
      const p = await api.UninstallPlan({ ...U.choice });
      if (!read.current()) return;
      U.page = p;
    } catch (e) { read.fail(e); return; }
    finally { if (read.current()) U.loading = false; }
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
  read?.finish();
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
  const sel = choiceControl(T("to %s's Mods folder").replace("%s", "…"), pg.envs, pg.lockedTo);
  sel.dataset.f = "lockedTo";
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
    const p = el("span", "p");
    p.append(copyablePath(el("span", "", it.path), it.path));
    if (it.to) p.append(" → ", copyablePath(el("span", "", it.to), it.to));
    li.append(p);
    const extra = [it.name, it.size, it.note, it.err].filter(Boolean).join("  ");
    if (extra) li.append(el("span", "x muted word-facts", extra));
    d.append(li);
  }
  return d;
}

function foot(pg) {
  const f = el("div", "un-foot");
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
  // Opens the single confirmation; execution only happens inside the dialog.
  const go = button(T("uninstall RimArcho…"), "", () => confirm(pg), "primary");
  go.disabled = U.loading || !!pg.blocked;
  f.append(go);
  return f;
}

function confirm(pg) {
  if (U.running || U.loading || pg.blocked || !pg.confirmation) return;
  const trigger = document.activeElement;
  const choice = { ...U.choice };
  pop.open("uninstall-confirm", T("uninstall RimArcho"), (box, popup) => {
    box.classList.add("un-confirm");
    box.setAttribute("role", "dialog");
    box.setAttribute("aria-modal", "true");
    box.setAttribute("aria-labelledby", "un-confirm-title");
    const title = el("h2", "ttl", T("uninstall RimArcho"));
    title.id = "un-confirm-title";
    const body = el("div", "body");
    body.tabIndex = 0;
    for (const [label, rows] of [["will remove or change", pg.confirmation.changes], ["will keep", pg.confirmation.kept]]) {
      const section = el("section", "un-summary");
      section.append(el("h3", "", T(label)));
      for (const row of rows || []) {
        const line = el("div", "un-summary-row");
        line.append(el("strong", "", row.title), el("span", "muted word-facts", `${row.count}  ${row.size}  ${row.method}`));
        for (const to of row.destinations || []) line.append(el("span", "muted", `→ ${to}`));
        section.append(line);
      }
      if (label === "will keep") section.append(el("p", "muted", pg.confirmation.untouched));
      body.append(section);
    }
    const buttons = el("div", "btns");
    const cancel = button(T("cancel"), "", () => pop.close());
    const go = button(T("uninstall"), "", () => { pop.close(); run(choice); }, "primary danger-go");
    buttons.append(cancel, go);
    box.append(title, body, buttons);
    popup.onclose = () => { if (trigger?.isConnected) trigger.focus(); };
    // Keep native button activation: Enter on the default Cancel must cancel.
    box.addEventListener("keydown", e => {
      e.stopPropagation();
      if (e.key === "Escape") { e.preventDefault(); pop.close(); }
      if (e.key === "Tab") {
        const nodes = [...box.querySelectorAll('button, [tabindex="0"]')];
        const i = nodes.indexOf(document.activeElement);
        nodes[(i + (e.shiftKey ? nodes.length - 1 : 1)) % nodes.length].focus();
        e.preventDefault();
      }
    });
    cancel.focus();
  });
}

async function run(choice) {
  if (U.running || U.loading) return;
  // Lock before the request: progress/report events can beat its acknowledgement.
  U.running = true;
  U.prog = null;
  drawProgress();
  try { await api.UninstallRun(choice); }
  catch (e) { U.running = false; exclusive(); say("fail", errText(e)); openScreen("uninstall"); return draw(true); }
}

// Keep page navigation out of the tab order. Target its controls, so custom
// window buttons added alongside navigation still reach the native Go guard.
function exclusive() {
  const lock = locked();
  document.body.classList.toggle("uninstall-exclusive", lock);
  for (const selector of ["#nav", "#navtoggle", "#nav-settings", "#tasks-line", "#side-split", "#crumbs", "#env", "#hints", "#planhead"]) {
    const node = $(selector);
    if (node) node.inert = lock;
  }
}

function drawProgress() {
  const sc = S.screen === "uninstall-progress" ? $("#screen") : openScreen("uninstall-progress");
  exclusive();
  if (!sc.querySelector(".un-progress")) {
    sc.replaceChildren();
    const w = page("uninstall", "");
    const card = el("section", "card un-progress");
    card.setAttribute("aria-live", "polite");
    card.append(el("p", "ptext", T("uninstalling…")));
    const progress = el("progress");
    progress.setAttribute("aria-label", T("uninstalling…"));
    card.append(progress);
    const notice = el("p", "un-close-notice", T("uninstalling; you can close RimArcho when it finishes"));
    notice.hidden = true;
    card.append(notice);
    const details = el("details", "card un-progress-steps");
    details.append(el("summary", "", T("all steps")), el("ol"));
    w.classList.add("un-progress-page");
    w.append(card, details);
    sc.append(w);
    // Drop focus from the old red button or navigation.
    card.tabIndex = -1;
    card.focus();
  }
  const p = U.prog || {};
  // The bar shows the items; the text names the step only, so the two counts never mix.
  sc.querySelector(".ptext").textContent = [T("uninstalling…"), p.step].filter(Boolean).join(" ");
  const progress = sc.querySelector("progress");
  if (p.n) { progress.max = p.n; progress.value = Math.max(0, p.i - 1); }
  else progress.removeAttribute("value");
  const steps = p.steps || U.page?.steps || [];
  const list = sc.querySelector(".un-progress-steps ol");
  const signature = JSON.stringify(steps.map(s => [s.key, s.title]));
  if (list.dataset.steps !== signature) {
    list.dataset.steps = signature;
    list.replaceChildren(...steps.map(s => {
      const row = el("li");
      row.append(el("span", "", s.title), el("span", "muted"));
      return row;
    }));
  }
  const current = steps.findIndex(s => s.key === p.stepKey);
  // Done steps get a check, later ones are muted; only the current step and the two
  // that run after closing carry a second line.
  [...list.children].forEach((row, i) => {
    const later = ["data", "program"].includes(steps[i].key);
    const state = later ? "later" : i === current ? "current" : i < current ? "done" : "pending";
    row.className = state;
    row.lastChild.textContent = state === "later" ? T("after closing") : state === "current" ? T("in progress") : "";
    row.lastChild.hidden = !row.lastChild.textContent;
    if (state === "current") row.setAttribute("aria-current", "step");
    else row.removeAttribute("aria-current");
  });
}

// The progress page says it once; the status line only when that page isn't up yet.
export function closeBlocked() {
  if (U.running) {
    drawProgress();
    $(".un-close-notice").hidden = false;
    return;
  }
  say("busy", T("uninstalling; you can close RimArcho when it finishes"));
}

// Restore progress/report from Go after a WebView reload; events already received
// take precedence over an older in-flight snapshot.
export async function restore() {
  const before = events;
  const status = await api.UninstallStatus();
  if (before === events && (status.running || status.done)) event(status);
  return U.running || !!U.page?.ran;
}

// Progress and completion share the same shape as UninstallStatus().
export function event(ev) {
  if (!ev) return;
  events++;
  if (!ev.done) {
    U.running = true;
    U.prog = ev;
    drawProgress();
    return;
  }
  U.running = false;
  U.page = ev.page || null;
  exclusive();
  openScreen("uninstall");
  if (ev.error) say("fail", ev.error);
  if (U.page) draw(false);
  else draw(true);
}
