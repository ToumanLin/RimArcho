// Plans in the window (04 §1.3, 06 §2.1, §2.5): each plan is a sidebar entry whose page is
// its editor. Opening one switches the current plan (no game file is written); new (empty,
// a copy, a version, an import), rename and delete are dialogs. Every verb is a view
// call through the API (view.NewPlan, UsePlan, RenamePlan, RemovePlan, ImportAsPlan).
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import * as pop from "./popups.js";

let ui = null;
export function initPlans(u) { ui = u; }

const current = () => (S.shell && S.shell.plan) || null;
const planOf = (id) => ((S.shell && S.shell.plans) || []).find((p) => p.id === id) || null;

// openPlan makes a plan current when it is not; true once its list is the board's.
export async function openPlan(id) {
  const cur = current();
  if (cur && cur.id === id) return true;
  let d;
  try { d = await api.UsePlan(id); } catch (e) { say("fail", errText(e)); return false; }
  forget();
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

async function done(p) {
  let d;
  try { d = await p; } catch (e) { say("fail", errText(e)); return null; }
  pop.close();
  forget();
  await ui.reload();
  await ui.go(d.page);
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

// newPlan: the new-plan dialog (04 §1.3): a name and where the list comes from.
export async function newPlan(from = "") {
  let froms, nm;
  try { [froms, nm] = await Promise.all([api.PlanFroms(), api.CheckPlanName("", "", 0)]); } catch (e) { return say("fail", errText(e)); }
  const opts = froms.concat([{ from: "import", label: T("from an import: a file, a share code, a link…") }]);
  let sel = Math.max(0, opts.findIndex((o) => o.from === from));
  pop.open("plan-new", T("new list"), (box, p) => {
    box.classList.add("plan-pop");
    box.append(el("div", "ttl", T("new list")));
    const f = nameField(box, nm.free, 0);
    box.append(el("div", "label", T("start from")));
    const list = el("div", "opts plan-from");
    box.append(list);
    const draw = () => {
      list.innerHTML = "";
      opts.forEach((o, i) => {
        const d = el("div", "opt" + (i === sel ? " on" : ""), o.label);
        d.onclick = () => { sel = i; draw(); f.inp.focus(); };
        d.ondblclick = go;
        list.append(d);
      });
      const on = list.children[sel];
      if (on) on.scrollIntoView({ block: "nearest" });
    };
    const btns = el("div", "btns");
    const no = el("button", "", T("cancel") + "  esc");
    no.onclick = pop.close;
    const yes = el("button", "go", T("create") + "  enter");
    yes.onclick = () => go();
    btns.append(no, yes);
    box.append(btns, el("div", "plan-note", T("the game's list stays as it is until you write or launch")));
    async function go() {
      const o = opts[sel];
      if (o.from === "import") {
        // the import popup reads and previews the list; "as a new list" applies it
        pop.close();
        return ui.cmd("import");
      }
      await f.check();
      if (f.bad()) return f.inp.focus();
      done(api.NewPlan(f.inp.value, o.from));
    }
    p.keys = (e) => {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); sel = Math.max(0, Math.min(opts.length - 1, sel + (e.key === "ArrowDown" ? 1 : -1))); draw(); return true; }
      if (e.key === "Enter") { e.preventDefault(); go(); return true; }
      return false; // typing goes to the name
    };
    draw();
  });
}

// renamePlan: the rename dialog; the history stays.
export function renamePlan(id) {
  const p0 = planOf(id);
  if (!p0) return;
  pop.open("plan-rename", T("rename the list"), (box, p) => {
    box.classList.add("plan-pop");
    box.append(el("div", "ttl", Tf("rename %s", p0.name)));
    const f = nameField(box, p0.name, id);
    const btns = el("div", "btns");
    const no = el("button", "", T("cancel") + "  esc");
    no.onclick = pop.close;
    const yes = el("button", "go", T("rename") + "  enter");
    const go = async () => { await f.check(); if (f.bad()) return f.inp.focus(); done(api.RenamePlan(id, f.inp.value)); };
    yes.onclick = go;
    btns.append(no, yes);
    box.append(btns);
    p.keys = (e) => { if (e.key === "Enter") { e.preventDefault(); go(); return true; } return false; };
  });
}

// deletePlan asks first (it cannot be undone); the last plan cannot go.
export async function deletePlan(id) {
  let warn;
  try { warn = await api.RemovePlanWarn(id); } catch (e) { return say("fail", errText(e)); }
  pop.confirm(T("delete the list"), [warn], T("delete"), () => done(api.RemovePlan(id)));
}

// actions are what can be done with a plan: the sidebar entry's context menu, the
// editor's "plan" menu and the palette's plan chooser.
export function actions(id) {
  const p = planOf(id);
  if (!p) return [];
  const out = [];
  if (!p.current) out.push({ label: T("open"), run: () => ui.go(p.page) });
  out.push({ label: T("rename…"), run: () => renamePlan(id) });
  if (p.current) out.push({ label: T("copy as a new list…"), run: () => newPlan("current") });
  out.push({ label: T("new list…"), run: () => newPlan() });
  out.push({ label: T("delete…"), run: () => deletePlan(id) });
  return out;
}

// chooser: the palette's "plans" — go to a plan, or manage the current one.
export function chooser() {
  const ps = (S.shell && S.shell.plans) || [];
  const cur = current();
  const labels = ps.map((p) => p.name + (p.current ? "  · " + T("editing") : "") + (p.conflict ? "  · " + p.conflictText : ""));
  const runs = ps.map((p) => () => ui.go(p.page));
  const extra = [[T("new list…"), () => newPlan()]];
  if (cur) extra.push([Tf("rename %s…", cur.name), () => renamePlan(cur.id)], [Tf("delete %s…", cur.name), () => deletePlan(cur.id)]);
  for (const [l, r] of extra) { labels.push(l); runs.push(r); }
  pop.choose(T("lists"), labels, (i) => runs[i](), Math.max(0, ps.findIndex((p) => p.current)));
}

// menuAt opens a plan's actions as a context menu.
export function menuAt(id, x, y) { pop.menu(x, y, actions(id)); }
