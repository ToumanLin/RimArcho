// The plan editor's 排序规则 (06 §2.5, 04 §6.1): a fold above 历史 in the left column. Rules
// are global (every list and game shares them); the fold shows those with a mod enabled in
// the current list (只看当前列表, the default) or all of them, and its bar counts what it
// shows. Each row has 编辑 and 删除; the form adds or changes one in place (no popup,
// 06 §3.1 rule 7). r opens the form for the mod at the cursor. Removing is at once, with
// 撤销 in the status line (u right after, too); rules are not list edits, so they are not in
// the list's undo stack.
import { $, el, api, T, errText, say } from "./state.js";
import { icon } from "./icons.js";
import { seg } from "./screens.js";

const R = { data: null, open: false, all: false, form: null, seq: 0, cands: [], at: 0, candSeq: 0 };
try {
  R.open = localStorage.getItem("archo.rules.open") === "1";
  R.all = localStorage.getItem("archo.rules.scope") === "all";
} catch { /* folded, the current list */ }
let ui = null;

const area = () => $(".rulesarea");

function setAll(v) {
  R.all = v;
  try { localStorage.setItem("archo.rules.scope", v ? "all" : "list"); } catch { /* not kept */ }
  draw();
  $(".rules-scope button.on", area())?.focus({ preventScroll: true });
}

// the rules the fold shows: a rule being edited stays until the form closes
const shown = () => (R.data ? R.data.rows : []).filter((r) => R.all || r.inList || (R.form && R.form.id === r.id));

export function initRules(u) {
  ui = u;
  const a = area();
  $(".hbar", a).addEventListener("click", () => setOpen(!R.open));
  // Keys typed in the fold are the fold's: the editor's verbs (space, letters) must not
  // act on the lists while a button or a field here has the focus.
  $(".rulesbody", a).addEventListener("keydown", (e) => {
    if (e.key === "Tab") return;
    if (e.key === "Escape") {
      e.preventDefault(); e.stopPropagation();
      if (R.cands.length) { R.cands = []; drawCands(); return; }
      if (R.form) return closeForm();
      document.activeElement?.blur();
      return;
    }
    e.stopPropagation();
  });
}

function setOpen(v) {
  R.open = v;
  try { localStorage.setItem("archo.rules.open", v ? "1" : "0"); } catch { /* not kept */ }
  draw();
  if (v) refreshRules();
}

// refreshRules reads the rules again (the editor's reloads, after a change).
export async function refreshRules() {
  const n = ++R.seq;
  let d;
  try { d = await api.Rules(); } catch { return; }
  if (n !== R.seq) return;
  R.data = d;
  draw();
}

// startRule is r (and the fold's 加一条规则): the form for a new rule about mod it.
export function startRule(it) {
  if (!it || !it.id) return say("", T("choose a mod in the lists first"));
  R.form = { id: 0, kind: "after", a: { ref: it.id, name: it.name }, b: null, note: "" };
  if (!R.open) setOpen(true); else draw();
  focusForm();
}

function editRule(r) {
  R.form = { id: r.id, kind: r.kind, a: r.a, b: r.b || null, note: r.note || "", target: r.target, text: r.text };
  draw();
  focusForm();
}

function closeForm() {
  R.form = null; R.cands = [];
  draw();
  document.activeElement?.blur();
}

function focusForm() {
  requestAnimationFrame(() => {
    const f = $(".rulesbody .rule-form", area());
    if (!f) return;
    (f.querySelector(".rule-other:not([hidden])") || f.querySelector(".rule-note")).focus();
  });
}

const kinds = () => (R.data && R.data.kinds) || [];
const kindOf = (k) => kinds().find((x) => x.kind === k);
// a kind's label in the form: the row's sentence without the first mod, "…" for the second
const kindLabel = (k) => k.label.replace("%s", "").replace("%s", "…").trim();

function draw() {
  const a = area();
  if (!a) return;
  a.classList.toggle("open", R.open);
  const rows = shown();
  const n = rows.length;
  const bar = $(".hbar", a);
  bar.replaceChildren(el("span", "t", T("load-order rules")));
  if (n) bar.append(el("span", "num", String(n)));
  const chev = el("span", "fold-chev"); chev.innerHTML = icon.chev; bar.append(chev);
  const body = $(".rulesbody", a);
  // a redraw (the editor reloads behind the form) keeps the field being typed in
  const was = document.activeElement && body.contains(document.activeElement) ? document.activeElement : null;
  const focusAt = was ? [was.className, was.selectionStart, was.selectionEnd] : null;
  body.replaceChildren();
  if (!R.open || !R.data) return;
  const scope = seg([T("only the current list"), T("all load-order rules")], R.all ? 1 : 0, (i) => setAll(i === 1));
  scope.classList.add("rules-scope");
  body.append(scope);
  if (R.form && !R.form.id) body.append(form());
  for (const r of rows) body.append(R.form && R.form.id === r.id ? form() : row(r));
  if (!R.form) {
    const add = el("button", "btn small quiet rule-add", T("add a rule"));
    add.prepend(Object.assign(el("span", "ic"), { innerHTML: icon.plus }));
    add.append(el("kbd", "", "r"));
    add.title = T("for the mod at the cursor") + "  r";
    add.onclick = () => startRule(ui.ruleTarget());
    body.append(add);
  }
  const again = focusAt && focusAt[0] && body.querySelector("." + focusAt[0].split(" ")[0]);
  if (again) {
    again.focus();
    if (again.tagName === "INPUT" && focusAt[1] != null) again.setSelectionRange(focusAt[1], focusAt[2]);
  }
  if (R.cands.length) drawCands();
}

function modName(m) {
  if (!m.id) return el("span", "rule-gone", m.name);
  const b = el("button", "rule-mod", m.name);
  b.title = T("show it in the list");
  b.onclick = (e) => { e.stopPropagation(); ui.palette({ kind: 1, id: m.id }); };
  return b;
}

function row(r) {
  const d = el("div", "rule");
  d.dataset.id = r.id;
  const t = el("div", "rule-t");
  for (const p of r.parts) t.append(p.mod === 1 ? modName(r.a) : p.mod === 2 ? modName(r.b) : p.text);
  d.append(t);
  if (r.note) d.append(el("div", "rule-n", r.note));
  d.title = "r:" + r.id + "  " + r.created;
  const acts = el("span", "rule-acts");
  const edit = el("button", "btn small quiet", T("edit"));
  edit.onclick = () => editRule(r);
  const del = el("button", "btn small quiet danger", T("delete"));
  del.onclick = () => remove(r.id);
  acts.append(edit, del);
  d.append(acts);
  return d;
}

function form() {
  const f = R.form;
  const box = el("div", "rule-form");
  const line = el("div", "rule-line");
  if (f.kind === "ignore") {
    line.append(el("span", "rule-fixed", f.text));
  } else {
    line.append(el("span", "rule-a", f.a.name));
    const sel = el("select", "rule-kind");
    for (const k of kinds()) {
      const o = el("option", "", kindLabel(k));
      o.value = k.kind;
      if (k.kind === f.kind) o.selected = true;
      sel.append(o);
    }
    sel.onchange = () => { f.kind = sel.value; draw(); focusForm(); };
    line.append(sel);
    const two = kindOf(f.kind)?.two;
    const bw = el("span", "rule-b");
    const inp = el("input", "rule-other");
    inp.spellcheck = false;
    inp.placeholder = T("the other mod");
    inp.value = f.b ? f.b.name : (f.typed || "");
    inp.hidden = !two;
    inp.oninput = () => { f.b = null; f.typed = inp.value; candidates(inp.value); };
    inp.onkeydown = (e) => {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        if (R.cands.length) { R.at = (R.at + (e.key === "ArrowDown" ? 1 : -1) + R.cands.length) % R.cands.length; drawCands(); }
      } else if ((e.key === "Enter" || e.key === "Tab" && !e.shiftKey) && R.cands.length) {
        e.preventDefault(); e.stopPropagation(); pick(R.cands[R.at]);
      } else if (e.key === "Enter") { e.preventDefault(); save(); }
    };
    bw.append(inp, el("div", "rule-cands"));
    line.append(bw);
  }
  box.append(line);
  const note = el("input", "rule-note");
  note.spellcheck = false;
  note.placeholder = T("note (optional)");
  note.value = f.note;
  note.oninput = () => { f.note = note.value; };
  note.onkeydown = (e) => { if (e.key === "Enter") { e.preventDefault(); save(); } };
  box.append(note);
  const acts = el("div", "rule-form-acts");
  const cancel = el("button", "btn small quiet", T("cancel"));
  cancel.append(el("kbd", "", "esc"));
  cancel.onclick = closeForm;
  const ok = el("button", "btn small primary", f.id ? T("save") : T("add"));
  ok.append(el("kbd", "", "enter"));
  ok.onclick = save;
  acts.append(cancel, ok);
  box.append(acts);
  return box;
}

let candTimer = 0;
function candidates(text) {
  clearTimeout(candTimer);
  candTimer = setTimeout(async () => {
    const n = ++R.candSeq;
    let list = [];
    try { list = text.trim() ? await api.RuleCandidates(text, R.form ? R.form.a.ref : "") : []; } catch { list = []; }
    if (n !== R.candSeq || !R.form) return;
    R.cands = list || []; R.at = 0;
    drawCands();
  }, 80);
}

function drawCands() {
  const box = $(".rulesbody .rule-cands", area());
  if (!box) return;
  box.replaceChildren();
  R.cands.forEach((c, i) => {
    const b = el("button", "rule-cand" + (i === R.at ? " on" : ""), c.name);
    if (c.pos) b.append(el("span", "p", String(c.pos)));
    b.tabIndex = -1;
    b.onmousedown = (e) => { e.preventDefault(); pick(c); };
    box.append(b);
  });
}

function pick(c) {
  if (!R.form || !c) return;
  R.form.b = { ref: c.ref, name: c.name, id: c.id };
  R.cands = [];
  const inp = $(".rulesbody .rule-other", area());
  if (inp) inp.value = c.name;
  drawCands();
  $(".rulesbody .rule-note", area())?.focus();
}

async function save() {
  const f = R.form;
  if (!f) return;
  const two = kindOf(f.kind)?.two;
  if (two && !f.b) {
    if (R.cands.length) return pick(R.cands[R.at]);
    return say("fail", T("choose the other mod"));
  }
  const spec = f.kind === "ignore" ? { kind: "ignore", target: f.target, note: f.note, a: "", b: "" }
    : { kind: f.kind, a: f.a.ref, b: two ? f.b.ref : "", target: "", note: f.note };
  let text;
  try { text = await api.SaveRule(f.id, spec); } catch (e) { return say("fail", errText(e)); }
  R.form = null; R.cands = [];
  say("ok", text);
  await refreshRules();
  ui.reload();
}

async function remove(id) {
  let text;
  try { text = await api.RemoveRule(id); } catch (e) { return say("fail", errText(e)); }
  if (R.form && R.form.id === id) R.form = null;
  say("ok", text, () => restore(id));
  await refreshRules();
  ui.reload();
}

async function restore(id) {
  try { say("ok", await api.RestoreRule(id)); } catch (e) { return say("fail", errText(e)); }
  await refreshRules();
  ui.reload();
}
