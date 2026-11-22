// What RimArcho did (16 §2.16, 13 §1.3): the operation log, a view of the Background tasks
// page. Visual first: each row is its time, an icon for the operation, the subject and a
// result mark; failures and refusals stand out. Chips filter by result and by origin. The
// path, the reason and where to get an item back are shown only when a row is opened. Never
// an undo: the recycle bin and the list's history keep their own routes.
import { $, el, api, T, Tf, errText } from "./state.js";
import { beginPageRead, area } from "./screens.js";
import { icon } from "./icons.js";

const O = { page: null, result: "", origin: "", open: new Set(), cur: 0 };
let els = null;

// the result marks: ✓ done, ✕ failed, ⊘ refused, ◐ partly done, a clock for unfinished
const MARK = { ok: "ok", failed: "err", refused: "refused", partial: "warn", started: "vers" };

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
  const chips = el("div", "op-chips");
  const rows = el("div", "op-rows");
  rows.setAttribute("role", "list");
  rows.tabIndex = -1;
  root.append(head, chips, rows);
  sc.append(root);
  els = { root, head, chips, rows, back };
  area(root, null, () => rows.focus());
  chips.addEventListener("click", (e) => {
    const b = e.target.closest("[data-f]");
    if (!b) return;
    const [k, v] = b.dataset.f.split(":");
    O[k] = O[k] === v ? "" : v;
    O.cur = 0;
    draw();
  });
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
  return rows.filter((r) => (!O.result || (O.result === "bad" ? r.bad : r.result === O.result)) && (!O.origin || r.origin === O.origin));
}

function chip(key, val, label, n, ic, cls = "") {
  const b = el("button", "op-chip " + cls + (O[key] === val ? " on" : ""));
  b.dataset.f = key + ":" + val;
  b.setAttribute("aria-pressed", String(O[key] === val));
  if (ic) { const i = el("span", "mk"); i.innerHTML = icon[ic] || ""; b.append(i); }
  b.append(el("span", ic ? "vis-hidden" : "", label), el("span", "num", String(n)));
  b.title = label;
  return b;
}

function draw() {
  if (!els || !O.page) return;
  const p = O.page;
  els.head.innerHTML = "";
  const t = el("span", "t", T("What RimArcho did"));
  t.id = "ops-heading";
  els.head.append(el("span", "anum", "1"), t, el("span", "num", String(p.count)));
  if (p.failed) { const b = el("span", "badge-n red num", String(p.failed)); b.title = Tf("%d failed or refused", p.failed); els.head.append(b); }
  const kept = el("span", "q", p.kept);
  els.head.append(kept);
  const back = el("button", "btn quiet op-back", T("back to the tasks"));
  back.onclick = () => els.back();
  els.head.append(back);

  els.chips.innerHTML = "";
  if (p.count) {
    const res = el("div", "op-chipset");
    res.setAttribute("aria-label", T("result"));
    if (p.failed) res.append(chip("result", "bad", T("failed or refused"), p.failed, "err", "bad"));
    for (const f of p.results || []) res.append(chip("result", f.key, f.label, f.count, MARK[f.key], "r-" + f.key));
    const org = el("div", "op-chipset");
    org.setAttribute("aria-label", T("origin"));
    for (const f of p.origins || []) org.append(chip("origin", f.key, f.label, f.count, "", "o-" + f.key));
    els.chips.append(res, org);
  }

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
  const mk = el("span", "op-mk r-" + r.result);
  mk.innerHTML = icon[MARK[r.result]] || "";
  mk.title = r.resultText;
  mk.append(el("span", "vis-hidden", r.resultText));
  b.append(el("span", "tm num", r.time), ic, what, sub);
  if (r.steps && r.steps.length) { const n = el("span", "op-n num", String(r.steps.length)); n.title = Tf("%d steps", r.steps.length); b.append(n); }
  if (r.origin === "agent" || r.origin === "mcp") { const o = el("span", "op-org o-" + r.origin, r.originText); b.append(o); }
  b.append(mk);
  b.title = r.what + T("; ") + r.resultText + T("; ") + r.originText;
  e.append(b);
  if (open) e.append(more(r));
  return e;
}

// more: what only an opened row shows — the path, the reason, where to get it back, the steps
function more(r) {
  const m = el("div", "op-more");
  const line = (k, v, cls = "") => { if (!v) return; const d = el("div", "op-kv " + cls); d.append(el("span", "k", k), el("span", "v", v)); m.append(d); };
  line(T("what"), r.what + T("; ") + r.resultText);
  line(T("from"), r.originText);
  line(T("path"), r.target, "path");
  line(T("why"), r.detail, r.bad ? "bad" : "");
  if (r.back) { const d = el("div", "op-back-to"); const i = el("span", "mk"); i.innerHTML = icon.ok; d.append(i, el("span", "", r.back)); m.append(d); }
  if (r.steps && r.steps.length) {
    const st = el("div", "op-steps");
    for (const s of r.steps) {
      const x = el("div", "op-step r-" + s.result + (s.bad ? " bad" : ""));
      const i = el("span", "op-ic"); i.innerHTML = icon[s.icon] || icon.log;
      const k = el("span", "op-mk r-" + s.result); k.innerHTML = icon[MARK[s.result]] || ""; k.title = s.resultText;
      x.append(i, el("span", "op-sub", [s.what, s.subject].filter(Boolean).join(T("; "))), k);
      x.title = [s.target, s.detail].filter(Boolean).join("\n");
      st.append(x);
      if (s.bad && s.detail) st.append(el("div", "op-step-why", s.detail));
    }
    m.append(st);
  }
  return m;
}

// ---- keys: ↑↓ move, enter opens, f failures only, esc clears the filter or goes back ----
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
  if (k === "f") { O.result = O.result === "bad" ? "" : "bad"; O.cur = 0; draw(); return done(); }
  if (k === "Escape") {
    if (O.result || O.origin) { O.result = O.origin = ""; draw(); return done(); }
    els.back();
    return done();
  }
  return false;
}

export function where() {
  const list = shown();
  let s = `${T("What RimArcho did")}    ${list.length ? O.cur + 1 : 0} / ${list.length}`;
  if (O.result || O.origin) s += "  " + T("filtered");
  return s;
}

export function hintList() {
  return [{ key: "↑↓", label: T("move") }, { key: "enter", label: T("open") }, { key: "f", label: T("failed or refused") }, { key: "esc", label: T("back to the tasks") }, { key: "?", label: T("all keys") }];
}

export function reset() { els = null; }
export const opsRoot = () => (els ? els.root : null);
export const has = () => !!$("#screen .op-log");
