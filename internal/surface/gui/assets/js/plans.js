// Plans in the window (04 §1.3, 06 §2.1, §2.5): each plan is a sidebar entry whose page is
// its editor. Opening one only shows it: the current list (the one launched) changes when
// the player makes another current, writes it to the game or launches it from its editor.
// New (empty, a copy, a version, an import) opens management; delete asks for confirmation.
// Every verb is a view call through the API (view.NewPlan, UsePlan, OpenPlan, RenamePlan,
// RemovePlan, ImportAsPlan).
import { fieldError } from "./feedbacksemantics.js";
import { S, $, el, api, T, Tf, errText, say, keyCaps } from "./state.js";
import * as pop from "./popups.js";

let ui = null;
export function initPlans(u) { ui = u; }

const current = () => (S.shell && S.shell.plan) || null;
const planOf = (id) => ((S.shell && S.shell.plans) || []).find((p) => p.id === id) || null;

// openPlan opens a plan when it is not open (the current list stays); true once its list
// is the board's.
export async function openPlan(id) {
  const cur = current();
  if (cur && cur.id === id) return true;
  try { await api.OpenPlan(id); } catch (e) { say("fail", errText(e)); return false; }
  forget();
  await ui.reload();
  return true;
}

// usePlan makes a plan current — the one launched — and opens it; no game file is written.
export async function usePlan(id) {
  const p = planOf(id), was = current();
  if (p && p.current && was && was.id === id) return true;
  let d;
  try { d = await api.UsePlan(id); } catch (e) { say("fail", errText(e)); return false; }
  if (!was || was.id !== id) forget();
  await ui.reload();
  say("ok", d.text);
  return true;
}

// forget: the board shows another list now; the cursors, searches and selections were
// the old list's.
function forget() {
  S.cols.forEach((c, k) => {
    c.query = ""; c.cursor = 0; c.sel.clear(); c.searching = false;
    const inp = $(`.area[data-area="${k}"] .search input`);
    if (inp) inp.value = "";
  });
}

// done: after a dialog's verb the window shows the plan's page, or what after opens (the
// list management page stays where it is, 06 §2.17).
async function done(p, after) {
  let d;
  try { d = await p; } catch (e) { say("fail", errText(e)); return null; }
  pop.close();
  forget();
  await ui.reload();
  if (after) await after(d);
  else await ui.go(d.page);
  say("ok", d.text);
  return d;
}

// nameField is a name input with what is wrong with it underneath (checked as typed).
function nameField(box, value, self) {
  const row = el("div", "in");
  row.append(el("span", "pr", T("name")));
  const inp = el("input");
  inp.spellcheck = false;
  inp.value = value || "";
  row.append(inp);
  const why = el("div", "plan-why");
  fieldError(inp, why);
  box.append(row, why);
  let seq = 0;
  const check = async () => {
    const n = ++seq;
    const r = await api.CheckPlanName("", inp.value, self || 0).catch(() => null);
    if (n === seq && r) why.textContent = r.problem;
  };
  inp.addEventListener("input", check);
  setTimeout(() => { inp.focus(); inp.select(); }, 0);
  return { inp, check, bad: () => why.textContent !== "" };
}

// New lists live in the management page's right column (06 §2.17).
export function newPlan(from = "") { pop.close(); return ui.newList(from); }

// renamePlan: the rename dialog; the history stays.
export function renamePlan(id, after = null) {
  const p0 = planOf(id);
  if (!p0) return;
  pop.open("plan-rename", T("rename the list"), (box, p) => {
    box.classList.add("plan-pop");
    box.append(el("div", "ttl", Tf("rename %s", p0.name)));
    const f = nameField(box, p0.name, id);
    const btns = el("div", "btns");
    const no = el("button", "", T("cancel"));
    no.append(keyCaps("esc"));
    no.onclick = pop.close;
    const yes = el("button", "go", T("rename"));
    yes.append(keyCaps("enter"));
    yes.dataset.envWrite = "rename-plan";
    const go = async () => { await f.check(); if (f.bad()) return f.inp.focus(); done(api.RenamePlan(id, f.inp.value), after); };
    yes.onclick = go;
    btns.append(no, yes);
    box.append(btns);
    p.keys = (e) => { if (e.key === "Enter") { e.preventDefault(); yes.click(); return true; } return false; };
  });
}

// deletePlan asks first (it cannot be undone); the last plan cannot go.
export async function deletePlan(id, after = null) {
  let warn;
  try { warn = await api.RemovePlanWarn(id); } catch (e) { return say("fail", errText(e)); }
  pop.confirm(T("delete the list"), [warn], T("delete"), () => done(api.RemovePlan(id), after));
}

// actions are what can be done with a plan: the sidebar entry's context menu, the
// editor's "plan" menu and the palette's plan chooser.
export function actions(id) {
  const p = planOf(id);
  if (!p) return [];
  const out = [];
  if (!p.open) out.push({ label: T("open"), run: () => ui.go(p.page) });
  if (!p.current) out.push({ label: T("make current"), envWrite: "use-plan", run: () => usePlan(id) });
  out.push({ label: T("rename…"), envWrite: "rename-plan", run: () => renamePlan(id) });
  out.push({ label: T("duplicate"), envWrite: "create-plan", run: () => ui.duplicateList(id) });
  out.push({ label: T("new / import…"), envWrite: "new-plan", run: () => newPlan() });
  out.push({ label: T("delete…"), envWrite: "remove-plan", run: () => deletePlan(id) });
  return out;
}

// chooser: the palette's "plans" — go to a plan, or manage the current one.
export function chooser() {
  const ps = (S.shell && S.shell.plans) || [];
  const cur = current();
  const labels = ps.map((p) => p.name + (p.current ? "  " + T("current") : "") + (p.open ? "  " + T("editing") : "") + (p.conflict ? "  " + p.conflictText : ""));
  const runs = ps.map((p) => () => ui.go(p.page));
  const extra = [[T("new / import…"), () => newPlan()]];
  if (cur) extra.push([Tf("rename %s…", cur.name), () => renamePlan(cur.id)], [Tf("delete %s…", cur.name), () => deletePlan(cur.id)]);
  for (const [l, r] of extra) { labels.push(l); runs.push(r); }
  pop.choose(T("lists"), labels, (i) => runs[i](), Math.max(0, ps.findIndex((p) => p.open)));
}

// menuAt opens a plan's actions as a context menu.
export function menuAt(id, x, y, opener, keys = false) { pop.menu(x, y, actions(id), keys, { name: planOf(id)?.name, opener }); }
