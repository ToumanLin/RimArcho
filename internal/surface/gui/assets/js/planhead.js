// The list editor's head actions (16 §2.5), in three groups: what the list has (problems,
// its "an outside version has not been loaded" mark); the tools, as icons with their words on
// hover and as their accessible names (rescan, rename, import / export: the list's page in
// all lists, delete); and saving, then set current or launch. Save carries how many entries
// are not saved to the game yet as its badge, so that count is shown once. Undo sits beside
// sort in the enabled list's head.
import { el, T, Tf, keyCaps } from "./state.js";
import { icon } from "./icons.js";

// planActions builds the actions; x brings what they act with: the open plan and the one
// launched (S.shell), the unsaved count, the problems chip, the rescan button, and the verbs.
export function planActions(x) {
  const u = x.unsaved || 0;
  const plan = x.plan;
  const btn = (label, k, run, cls = "", envWrite = "") => {
    const b = el("button", "btn " + cls, label);
    if (envWrite) b.dataset.envWrite = envWrite;
    if (k) { b.append(keyCaps(k)); b.title = k; }
    b.onclick = (e) => { e.stopPropagation(); run(); };
    return b;
  };
  const out = [x.problemsChip()];
  if (plan && plan.conflict) {
    const m = el("button", "chip mark");
    m.append(el("span", "dot"), el("span", "t", plan.conflictText));
    m.title = plan.conflictText + T("; ") + T("can it run › needs your answer");
    m.onclick = (e) => { e.stopPropagation(); x.openNeeds(); };
    out.push(m);
  }
  // the tools: an icon each, the word on hover and as the accessible name
  const tool = (ic, words, run, cls = "", envWrite = "") => {
    const b = btn("", "", run, "icon " + cls, envWrite);
    const s = el("span", "ic");
    s.innerHTML = icon[ic];
    b.append(s);
    b.title = words;
    b.setAttribute("aria-label", words);
    return b;
  };
  const tools = el("span", "ph-tools");
  tools.append(x.rescanButton());
  if (plan) tools.append(tool("edit", T("rename"), () => x.rename(plan.id), "", "rename-plan"));
  tools.append(tool("io", T("import/export") + T("; ") + T("the list's page in all lists: import, export, share"), x.manage));
  if (plan) tools.append(tool("bin", T("delete"), () => x.remove(plan.id), "danger", "remove-plan"));
  out.push(tools);
  // save: how many entries are not saved yet is its badge (one fact, one place)
  const save = btn(u ? T("save") : T("no changes"), "w", x.save, u ? "hot" : "", "save");
  save.disabled = !u;
  if (u) {
    const n = el("b", "unwritten", String(u));
    save.insertBefore(n, save.querySelector("kbd"));
    save.title = Tf("%d unsaved", u) + T("; ") + T("different from the last saved list; launching or saving saves these changes to the game") + T("; ") + "w";
    save.setAttribute("aria-label", T("save changes") + ", " + Tf("%d unsaved", u));
    x.bump?.(n, u);
  } else x.bump?.(null, 0);
  out.push(save);
  // another list is the one launched (07 §1.3): this one can be made current first
  if (plan && x.current) {
    const c = btn(T("set current"), "", () => x.use(plan.id), "primary", "use-plan");
    c.title = Tf("the game is launched with %s; this list is only open here", x.current.name) + T("; ") + T("no game file is written");
    out.push(c);
  } else {
    const l = btn(T("launch"), "L", x.launch, "primary", "launch");
    l.title = T("starts the game with this list") + T("; ") + "L";
    out.push(l);
  }
  return out;
}
