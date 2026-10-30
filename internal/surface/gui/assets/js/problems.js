// The list's problems inside the editor: the top bar's "N problems" button opens a panel
// with every item of 能不能跑, worst first, each with its fix, so the list can be put right
// without leaving it (06 §4.2). The panel follows S.board.problems: a list edit, a sort or
// the background check (the list-problems event) redraw it in place.
import { S, el, T, Tf, say, errText } from "./state.js";
import { api } from "./api.js";
import { statusIcon } from "./icons.js";
import { needText, needParts, needEvidence } from "./needtext.js";
import * as pop from "./popups.js";
import { runNeedAction, needActionWrite } from "./screens.js";

let ui = null;
export function initProblems(u) { ui = u; listen(); }

const box = el("div", "prob-pop");
box.setAttribute("role", "dialog");
let anchor = null;
const opened = new Set(); // sections shown in full
// view.Section* (06 §2.4); the last one is SectionElsewhere: the editor still lists those items
const SECTIONS = ["breaks the game", "worth knowing", "for you to confirm", "errors from the last run", "hidden mods"];
const FOLD_AT = 4; // a long section shows this many, then "show all"

const items = () => ((S.board && S.board.problems && S.board.problems.items) || []).filter((it) => it.counted || it.status === -1 || (it.actions || []).some((a) => a.kind === "version-list"));
const problemCount = (its) => its.filter((it) => it.counted && it.section !== 4).length;
export const isOpen = () => box.isConnected;

// autoFixable: the items one click can fix with no question (required mods that are
// installed but not enabled); CompleteDependencies enables them and sorts again.
const autoFixable = () => items().filter((it) => it.counted && (it.actions || []).some((a) => a.kind === "enable") && it.target);

// chip: the top bar's button, or null when the list has nothing to fix.
export function chip() {
  const p = S.board && S.board.problems;
  const its = items(), n = problemCount(its);
  if (!p || (!n && !its.some((it) => it.status === -1))) { if (isOpen()) close(); return null; }
  const b = el("button", "chip prob " + (n ? (p.severity === "error" ? "err" : "warn") : "hint") + (p.updating ? " updating" : "") + (isOpen() ? " on" : ""));
  b.append(el("span", "dot"), el("span", "t", n ? Tf("%d problems", n) : T("suggestions")), el("kbd", "", "p"));
  b.title = T(n ? "what is wrong with this list, and the fixes" : "suggestions") + T("; ") + "p";
  b.onclick = (e) => { e.stopPropagation(); toggle(b); };
  return b;
}

export function toggle(at) {
  if (isOpen()) return close();
  anchor = at || document.querySelector(".chip.prob");
  if (!anchor) return say("", T("nothing to fix in this list"));
  draw();
  anchor.classList.add("on");
}

export function close() {
  box.remove();
  if (anchor) anchor.classList.remove("on");
  anchor = null;
}

// refresh: the board changed; the open panel follows it, anchored to the new button.
let drawn = null; // the problems the open panel shows
export function refresh() {
  if (!isOpen()) return;
  const b = document.querySelector(".chip.prob");
  if (!b) return close();
  if (b !== anchor) { anchor = b; b.classList.add("on"); }
  if (drawn === S.board.problems) return place();
  draw();
}

function draw() {
  const scroll = box.querySelector(".pp-list")?.scrollTop || 0;
  box.innerHTML = "";
  drawn = S.board.problems;
  const its = items();
  const p = S.board.problems;
  const head = el("div", "pp-head");
  const lead = el("div", "pp-lead");
  lead.append(el("b", "", problemCount(its) ? Tf("%d problems in this list", problemCount(its)) : T("suggestions")));
  if (p.updating) lead.append(el("span", "muted", T("updating…")));
  head.append(lead);
  const acts = el("div", "pp-acts");
  const fix = autoFixable();
  if (fix.length) {
    const b = el("button", "btn small primary", Tf("fix %d automatically", fix.length));
    b.title = T("enables the required mods that are installed and sorts the list; u undoes it");
    b.dataset.envWrite = "enable";
    b.onclick = (e) => { e.stopPropagation(); fixAutomatically(); };
    acts.append(b);
  }
  const all = el("button", "btn small", T("open can it run"));
  all.onclick = (e) => { e.stopPropagation(); close(); ui.go("needs"); };
  acts.append(all);
  head.append(acts);
  box.append(head);

  const list = el("div", "pp-list");
  SECTIONS.forEach((title, si) => {
    const sec = its.filter((it) => it.section === si).sort((a, b) => b.status - a.status);
    if (!sec.length) return;
    const h = el("div", "pp-sec", T(sec.every((it) => it.status === -1) ? "suggestions" : title));
    const count = sec.filter((it) => it.counted).length;
    if (count) h.append(el("span", "num", String(count)));
    list.append(h);
    const shown = opened.has(si) || sec.length <= FOLD_AT + 1 ? sec : sec.slice(0, FOLD_AT);
    for (const it of shown) list.append(card(it));
    if (shown.length < sec.length) {
      const more = el("button", "pp-more", Tf("show all %d", sec.length));
      more.onclick = (e) => { e.stopPropagation(); opened.add(si); draw(); };
      list.append(more);
    }
  });
  box.append(list);
  if (!box.isConnected) document.body.append(box);
  place();
  list.scrollTop = scroll;
}

function card(it) {
  const row = el("div", "pp-item" + (it.dim ? " muted" : ""));
  const ic = el("span", "pp-ic s" + it.status);
  ic.innerHTML = statusIcon(it.status);
  const body = el("div", "pp-body");
  const t = needText("div", "pp-t", it.title || it.text, it.title ? it.content?.title : it.content?.text, close);
  body.append(t);
  if (it.why) body.append(needText("div", "pp-why", it.why, it.content?.why, close));
  if (it.status === -1 && it.evidence) body.append(needEvidence(it.evidence));
  if (it.fold && it.parts?.length) {
    const fold = el("details", "pp-parts");
    fold.append(el("summary", "", T("show which")), needParts(it, close));
    body.append(fold);
  }
  const acts = (it.actions || []).filter((a) => a.kind !== "fold");
  if (acts.length) {
    const right = el("div", "pp-acts");
    const answers = acts.some((a) => a.kind === "answer");
    if (answers) {
      for (const a of acts.filter((a) => a.kind !== "answer")) body.append(actButton(it, a, "btn identity-why"));
    }
    const shown = answers ? acts.filter((a) => a.kind === "answer") : acts.slice(0, 1);
    for (const a of shown) right.append(actButton(it, a, answers ? "btn small" : "btn small primary"));
    if (!answers && acts.length > 1) {
      const more = el("button", "btn small more", "⋯");
      more.title = acts.slice(1).map((x) => x.label).join(T("; "));
      more.onclick = (e) => {
        e.stopPropagation();
        const r = more.getBoundingClientRect();
        pop.menu(r.left, r.bottom + 4, acts.slice(1).map((x) => ({ label: x.label, envWrite: needActionWrite(x), run: () => runNeedAction(it, x) })));
      };
      right.append(more);
    }
    body.append(right);
  }
  row.append(ic, body);
  return row;
}

function actButton(it, a, cls) {
  const b = el("button", cls, a.label);
  if (a.title) b.title = a.title;
  const w = needActionWrite(a);
  if (w) b.dataset.envWrite = w;
  b.onclick = (e) => { e.stopPropagation(); runNeedAction(it, a); };
  return b;
}

async function fixAutomatically() {
  try {
    await ui.edited(() => api.CompleteDependencies(), true);
  } catch (e) { say("fail", errText(e)); }
}

function place() {
  if (!anchor) return;
  const a = anchor.getBoundingClientRect();
  const w = Math.min(480, innerWidth - 16);
  box.style.width = w + "px";
  let x = Math.min(a.right - w, innerWidth - w - 8);
  if (x < 8) x = 8;
  box.style.left = x + "px";
  box.style.top = (a.bottom + 6) + "px";
  box.style.maxHeight = Math.max(200, innerHeight - a.bottom - 20) + "px";
}

let listening = false;
function listen() {
  if (listening) return;
  listening = true;
  box.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") e.stopPropagation();
  });
  // a click outside the panel (and outside its menu and any dialog) closes it
  document.addEventListener("mousedown", (e) => {
    if (!isOpen() || box.contains(e.target)) return;
    const t = e.target.closest && e.target.closest(".chip.prob, #menu, #popup");
    if (!t) close();
  }, true);
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape" || !isOpen() || S.popup || !document.querySelector("#menu")?.hidden) return;
    e.preventDefault();
    e.stopPropagation();
    close();
  }, true);
  addEventListener("resize", () => place());
}

// event: the background check finished for this list (list-problems): the newer answer
// replaces the board's problems, and the row icons follow (view.BuildNeeds' Status/Reasons).
export function event(p) {
  const b = S.board;
  if (!b || !p) return false;
  const plan = S.shell && S.shell.plan && S.shell.plan.id;
  // environment is the game's id (the board's own problems carry the same id)
  const env = b.problems && b.problems.environment;
  if (p.environment && env && p.environment !== env) return false;
  if (plan && p.plan && p.plan !== plan) return false;
  if (b.problems && b.problems.sequence > p.sequence) return false;
  b.problems = p;
  b.status = p.status || {};
  b.reasons = p.reasons || {};
  b.needs = p.count;
  return true;
}
