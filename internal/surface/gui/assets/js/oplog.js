// What RimArcho did (16 §2.16, 13 §1.3): the operation log, a view of the Background tasks
// page. Visual first: each row is its time, an icon for the operation and the subject, with
// one fixed column at the right that holds a mark only when something went wrong. One
// toggle shows only the problems. Who did it, the steps, the path, the reason and where to
// get an item back are shown only when a row is opened. Never an undo: the recycle bin and
// the list's history keep their own routes.
import { $, el, api, T, Tf } from "./state.js";
import { beginPageRead, area } from "./screens.js";
import { icon } from "./icons.js";

const O = { page: null, bad: false, open: new Set(), cur: 0 };
let els = null;

// the marks of what went wrong: ✕ failed, ⊘ refused, ◐ partly done, a clock for unfinished;
// a row that went well has none
const MARK = { failed: "err", refused: "refused", partial: "warn", started: "vers" };

export async function load(sc, back, read = null) {
  read ||= beginPageRead("tasks", { retry: () => load(sc, back) });
  let p;
  try { p = await api.Operations(); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  O.page = p;
  if (!els || !document.body.contains(els.root)) build(sc, back);
  draw();
  read.finish();
}

function build(sc, back) {
  const root = el("section", "area op-log");
  const head = el("div", "head");
  const rows = el("div", "op-rows");
  rows.setAttribute("role", "list");
  rows.tabIndex = -1;
  root.append(head, rows);
  sc.append(root);
  els = { root, head, rows, back };
  area(root, null, () => rows.focus());
  rows.addEventListener("click", (e) => {
    const r = e.target.closest(".op-row");
    if (!r) return;
    toggle(+r.dataset.id);
  });
}

function toggle(id) {
  O.open.has(id) ? O.open.delete(id) : O.open.add(id);
  const list = shown();
  O.cur = Math.max(0, list.findIndex((r) => r.id === id));
  draw();
}

export function shown() {
  const rows = (O.page && O.page.rows) || [];
  return O.bad ? rows.filter((r) => r.bad) : rows;
}

function problems() {
  O.bad = !O.bad;
  O.cur = 0;
  draw();
}

// mark: the one right-hand column, kept empty when the row went well
function mark(x) {
  const m = el("span", "op-mk" + (x.mark ? " r-" + x.mark : ""));
  if (!x.mark) return m;
  m.innerHTML = icon[MARK[x.mark]] || "";
  const step = x.mark === x.result ? null : (x.steps || []).find((s) => s.bad);
  m.title = step ? step.resultText : x.resultText;
  m.append(el("span", "vis-hidden", m.title));
  return m;
}

function draw() {
  if (!els || !O.page) return;
  const p = O.page;
  els.head.innerHTML = "";
  const t = el("span", "t", T("What RimArcho did"));
  t.id = "ops-heading";
  const n = el("span", "num", String(p.count));
  t.title = n.title = p.kept;
  els.head.append(el("span", "anum", "1"), t, n);
  if (p.failed) {
    // the one filter: only what failed, was refused or partly done
    const b = el("button", "op-problems" + (O.bad ? " on" : ""));
    b.setAttribute("aria-pressed", String(O.bad));
    b.title = Tf("%d failed or refused", p.failed);
    const i = el("span", "mk");
    i.innerHTML = icon.err;
    b.append(i, el("span", "", T("problems")), el("span", "num", String(p.failed)));
    b.onclick = problems;
    els.head.append(b);
  } else O.bad = false;
  const back = el("button", "btn quiet op-back", T("back to the tasks"));
  back.onclick = () => els.back();
  els.head.append(back);

  const list = shown();
  const box = els.rows;
  box.innerHTML = "";
  box.setAttribute("aria-labelledby", "ops-heading");
  if (!list.length) {
    box.append(el("div", "empty-state", p.count ? T("no row matches the filter") : p.empty));
    return;
  }
  O.cur = Math.min(O.cur, list.length - 1);
  let day = "";
  list.forEach((r, i) => {
    if (r.day !== day) { day = r.day; box.append(el("div", "group op-day", day)); }
    box.append(row(r, i === O.cur));
  });
  const c = box.querySelector(".op-row.cur");
  if (c) c.scrollIntoView({ block: "nearest" });
}

function row(r, cur) {
  const open = O.open.has(r.id);
  const e = el("div", "op-item" + (r.bad ? " bad" : "") + (open ? " open" : ""));
  const b = el("div", "op-row r-" + r.result + (cur ? " cur" : ""));
  b.dataset.id = r.id;
  b.setAttribute("role", "listitem");
  b.setAttribute("aria-expanded", String(open));
  const ic = el("span", "op-ic");
  ic.innerHTML = icon[r.icon] || icon.log;
  ic.title = r.what;
  const sub = el("span", "op-sub", r.subject || r.what);
  const what = el("span", "vis-hidden", r.what);
  b.append(el("span", "tm num", r.time), ic, what, sub, mark(r));
  b.title = r.what + T("; ") + r.resultText + T("; ") + r.originText;
  e.append(b);
  if (open) e.append(more(r));
  return e;
}

// more: what only an opened row shows — what and its result, who did it, the path, the
// reason, where to get it back, the steps
function more(r) {
  const m = el("div", "op-more");
  const line = (k, v, cls = "") => { if (!v) return; const d = el("div", "op-kv " + cls); d.append(el("span", "k", k), el("span", "v", v)); m.append(d); };
  line(T("what"), r.what + T("; ") + r.resultText);
  line(T("from"), r.originText, r.origin === "agent" || r.origin === "mcp" ? "agent" : "");
  line(T("path"), r.target, "path");
  line(T("why"), r.detail, r.bad ? "bad" : "");
  if (r.back) { const d = el("div", "op-back-to"); const i = el("span", "mk"); i.innerHTML = icon.ok; d.append(i, el("span", "", r.back)); m.append(d); }
  if (r.steps && r.steps.length) {
    const st = el("div", "op-steps");
    st.setAttribute("aria-label", Tf("%d steps", r.steps.length));
    for (const s of r.steps) {
      const x = el("div", "op-step r-" + s.result + (s.bad ? " bad" : ""));
      const i = el("span", "op-ic"); i.innerHTML = icon[s.icon] || icon.log;
      x.append(i, el("span", "op-sub", [s.what, s.subject].filter(Boolean).join(T("; "))), mark(s));
      x.title = [s.target, s.detail].filter(Boolean).join("\n");
      st.append(x);
      if (s.bad && s.detail) st.append(el("div", "op-step-why", s.detail));
    }
    m.append(st);
  }
  return m;
}

// ---- keys: ↑↓ move, enter opens, f problems only, esc clears the filter or goes back ----
export function key(e) {
  if (!O.page) return false;
  const k = e.key, list = shown();
  const done = () => { e.preventDefault(); return true; };
  const go = (j) => { O.cur = Math.max(0, Math.min(list.length - 1, j)); draw(); return done(); };
  if (k === "ArrowDown") return go(O.cur + 1);
  if (k === "ArrowUp") return go(O.cur - 1);
  if (k === "Home") return go(0);
  if (k === "End") return go(list.length - 1);
  if ((k === "Enter" || k === " ") && list[O.cur]) { toggle(list[O.cur].id); return done(); }
  if (k === "f") { if (O.page.failed) problems(); return done(); }
  if (k === "Escape") {
    if (O.bad) { O.bad = false; draw(); return done(); }
    els.back();
    return done();
  }
  return false;
}

export function where() {
  const list = shown();
  let s = `${T("What RimArcho did")}    ${list.length ? O.cur + 1 : 0} / ${list.length}`;
  if (O.bad) s += "  " + T("filtered");
  return s;
}

export function hintList() {
  return [{ key: "↑↓", label: T("move") }, { key: "enter", label: T("open") }, { key: "f", label: T("problems") }, { key: "esc", label: T("back to the tasks") }, { key: "?", label: T("all keys") }];
}

export function reset() { els = null; }
export const opsRoot = () => (els ? els.root : null);
export const has = () => !!$("#screen .op-log");
