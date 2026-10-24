// 列表管理 (06 §2.17): the sidebar's 列表 › 管理. On the left every list of the game, on the
// right the picked one: its game versions (04 §1.6), the Steam Workshop collections it
// follows (04 §9.6) and how to share it (export, pack). The export is a sub-page of its own
// (库 / 列表 / <列表> / 导出). Editing what a list holds stays in the editor (§2.5).
// Data: API.PlanManage (view.BuildPlanManage), API.PlanExport (view.BuildPlanExport).
import { pathContent } from "./fields.js";
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button, act, area, seg, beginPageRead } from "./screens.js";
import { icon } from "./icons.js";
import * as pop from "./popups.js";
import * as plans from "./plans.js";
import { wireSeparator } from "./separator.js";

let ui = null;
export function initPlanManage(u) { ui = u; }

const M = { data: null, sel: 0, open: new Set(), checked: new Set(), draft: null, created: 0 };
const X = { data: null, id: 0, from: "plans", format: "", info: null };
const FORMAT_KEY = "ui.gui.export.format";
const C = { seq: 0, page: null, slot: null, progress: null };

const ic = (name, cls = "ic") => { const s = el("span", cls); s.innerHTML = icon[name] || ""; return s; };
const sel = () => (M.data && M.data.sel) || null;

// ---- the page ----

// select picks the list the page opens on next (0: the current one).
export function select(id) { M.sel = id; }

// open draws the page with the list id picked (0: the one picked before, else the current).
export async function open(id) {
  const costSeq = ++C.seq;
  C.page = C.slot = C.progress = null;
  if (id !== undefined) M.sel = id;
  const read = beginPageRead("plans");
  let d;
  try { d = await api.PlanManage(M.sel || 0); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  M.data = d;
  M.sel = d.sel ? d.sel.id : 0;
  draw();
  read.finish();
  if (d.sel && !M.draft) readCost(d.sel.id, costSeq);
  // a followed collection not read for ten minutes is read now; the page follows
  const s = d.sel;
  if (s && s.stale && !M.checked.has(s.id)) {
    M.checked.add(s.id);
    api.PlanCheck(s.id).then((changed) => { if (changed && S.screen === "plans" && M.sel === s.id) open(); }).catch(() => {});
  }
}

function draw() {
  const d = M.data;
  const sc = S.screen === "plans" ? $("#screen") : openScreen("plans");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const add = act(T("new list"), "plus", () => newList(), "n");
  add.classList.add("primary");
  const w = page("plans pm-page", "", act(T("import"), "import", () => ui.cmd("import"), "i"), add);
  const grid = el("div", "pm");
  const side = listCard(d), gap = el("div", "split pm-split");
  grid.append(side, gap, M.draft ? newForm() : detailCard(d.sel));
  const setSide = (v) => { if (v) grid.style.setProperty("--pm-side", v + "px"); else grid.style.removeProperty("--pm-side"); };
  wireSeparator(gap, {
    key: "ui.gui.plans.side", range: () => [270, 340],
    get: () => side.getBoundingClientRect().width,
    set: setSide, reset: () => setSide(0),
    restore: (v) => setSide(+v ? Math.max(270, Math.min(340, +v)) : 0),
  });
  w.append(grid);
  sc.append(w);
  sc.scrollTop = top;
  ui.renderHints();
}

function listCard(d) {
  const card = el("section", "card pm-list");
  const box = el("div", "scroll");
  if (M.draft) {
    const row = el("button", "pm-row on", T("new list"));
    row.onclick = () => $(".pm-new input")?.focus();
    box.append(row);
  }
  const rows = M.created ? d.rows.filter(r => r.id === M.created).concat(d.rows.filter(r => r.id !== M.created)) : d.rows;
  for (const r of rows) {
    const b = el("button", "pm-row" + (!M.draft && r.id === M.sel ? " on" : ""));
    const n = el("div", "n");
    n.append(el("b", "", r.name));
    if (r.current) n.append(el("span", "pill cur", T("current")));
    if (r.inGame) { const g = el("span", "gdot"); g.title = T("in the game"); n.append(g); }
    if (r.mark) { const m = el("span", "adot"); m.title = r.mark; n.append(m); }
    b.append(n, el("div", "m word-facts", r.meta));
    if (r.follows) {
      const f = ic("workshop", "ri" + (r.changed ? " changed" : ""));
      f.title = r.changed ? T("the collection changed") : T("follows Steam collections");
      b.append(f);
    }
    b.onclick = () => pick(r.id);
    b.ondblclick = () => ui.go(r.page);
    box.append(b);
  }
  card.append(box);
  area(card, null, () => { const on = card.querySelector(".pm-row.on"); if (on) on.focus({ preventScroll: true }); });
  requestAnimationFrame(() => { const on = card.querySelector(".pm-row.on"); if (on) on.scrollIntoView({ block: "nearest" }); });
  return card;
}

function pick(id) {
  if (id === M.sel && !M.draft) return;
  M.draft = null;
  M.sel = id;
  open();
}

function detailCard(p) {
  const card = el("section", "card pm-detail");
  if (!p) { card.append(el("div", "empty-state", T("pick a list"))); return card; }
  // the head: name, marks, facts; edit, make current, ⋯
  const head = el("div", "pm-head");
  const l = el("div", "l");
  const n = el("div", "n");
  n.append(el("h2", "", p.name));
  if (p.current) n.append(el("span", "pill cur", T("current")));
  if (p.mark) { const m = el("span", "adot"); m.title = p.mark; n.append(m); }
  const facts = el("div", "facts word-facts", p.facts);
  if (p.inGame) facts.append(el("span", "green", p.inGame));
  l.append(n, facts);
  const r = el("div", "r");
  r.append(button(T("edit"), "↵", () => ui.go(p.page)));
  if (!p.current) r.append(button(T("make current"), "", () => makeCurrent(p.id), "", "use-plan"));
  const more = el("button", "btn icon", "");
  more.append(ic("more"));
  more.title = T("more");
  more.onclick = (e) => { e.stopPropagation(); const b = more.getBoundingClientRect(); pop.menu(b.left, b.bottom + 4, menuOf(p)); };
  r.append(more);
  head.append(l, r);
  card.append(head);

  card.append(versions(p), costSection(p), follows(p), share(p));
  return card;
}

function menuOf(p) {
  const out = [{ label: T("rename…"), envWrite: "rename-plan", run: () => plans.renamePlan(p.id, stay) }];
  out.push({ label: T("copy as a new list…"), envWrite: "create-plan", run: () => newList(p) });
  out.push({ label: T("delete…"), envWrite: "remove-plan", run: () => plans.deletePlan(p.id, () => open(0)) });
  return out;
}

// stay: after a dialog the page shows the list it made or renamed.
const stay = (d) => open(+String(d.page || "").replace(/^plan:/, "") || 0);

// The draft is a selected row, abandoned by esc or another selection (06 §2.17).
export async function newList(p = null, from = "current") {
  M.draft = null;
  await ui.go("plans");
  if (S.screen !== "plans") return;
  if (p) await open(p.id);
  const source = sel()?.id || S.shell?.plan?.id;
  const read = beginPageRead("plans");
  try {
    const [sources, nm] = await Promise.all([api.PlanSources(source), api.CheckPlanName("", "", 0)]);
    if (!read.current()) return;
    M.draft = { source, sources, name: nm.free, from, previous: S.shell?.plan?.id };
    draw(); read.finish();
  } catch (e) { read.fail(e); }
}

function newForm() {
  const draft = M.draft;
  const card = el("section", "card pad pm-new");
  const form = el("form");
  const label = el("label", "xc-sec", T("name"));
  const name = el("input", "xc-input"); name.value = draft.name; name.spellcheck = false;
  label.append(name);
  const why = el("div", "plan-why");
  const create = button(T("create list"), "enter", () => form.requestSubmit(), "primary", "create-plan");
  create.type = "button";
  let seq = 0;
  async function check() {
    const n = ++seq, value = name.value;
    draft.name = value; create.disabled = true;
    try {
      const r = await api.CheckPlanName("", value, 0);
      if (n !== seq || M.draft !== draft) return false;
      why.textContent = r.problem; create.disabled = !!r.problem;
      return !r.problem;
    } catch (e) { why.textContent = errText(e); return false; }
  }
  name.oninput = check;
  form.append(label, why, el("div", "pm-sub", T("start from")));
  const groups = [["", T("an empty list"), null], ["current", draft.sources.copy, null],
    ["version", T("a version written to the game"), draft.sources.versions], ["save", T("a save file"), draft.sources.saves]];
  for (const [value, text, options] of groups) {
    const row = el("label", "xc-check");
    const radio = el("input"); radio.type = "radio"; radio.name = "start"; radio.value = value;
    radio.checked = draft.from === value; radio.disabled = !!options && !options.length;
    radio.onchange = () => { draft.from = value; };
    row.append(radio, el("span", "", text));
    if (options) {
      const select = el("select", "xc-input");
      options.forEach(o => { const op = el("option", "", o.label); op.value = o.from; select.append(op); });
      select.disabled = !options.length;
      select.onchange = () => { radio.checked = true; draft.from = value; draft[value] = select.value; };
      draft[value] ||= select.value; select.value = draft[value]; row.append(select);
    }
    form.append(row);
  }
  const imp = button(T("import a file, share code or collection…"), "", () => { M.draft = null; ui.cmd("import"); }, "link"); imp.type = "button";
  form.append(imp, el("div", "acts-row")); form.lastChild.append(create);
  let submitting = false;
  form.onkeydown = e => {
    if (e.key === "Enter" && !e.target.matches("button, select")) { e.preventDefault(); e.stopPropagation(); form.requestSubmit(); }
  };
  form.onsubmit = async e => {
    e.preventDefault();
    if (submitting || !(await check())) return;
    submitting = true; create.disabled = true;
    try {
      const from = draft.from === "version" || draft.from === "save" ? draft[draft.from] : draft.from;
      const r = await api.CreatePlan(draft.source, name.value, from);
      M.draft = null; M.created = +r.page.slice(5);
      await ui.reload(); await open(M.created);
      say("ok", r.text, async () => {
        try {
          if (draft.previous) await api.UsePlan(draft.previous);
          const t = await api.RemovePlan(+r.page.slice(5));
          await ui.reload(); await open(draft.previous || 0); say("ok", t.text);
        } catch (e) { say("fail", errText(e)); }
      });
    } catch (e) { why.textContent = errText(e); submitting = false; create.disabled = false; }
  };
  card.append(form);
  requestAnimationFrame(() => { if (M.draft === draft) { name.focus(); name.select(); } });
  return card;
}

// Leave a form behind when navigating to another sidebar page (06 §2.17).
export function abandon() { M.draft = null; M.created = 0; C.seq++; C.slot = null; }
export const selected = () => M.sel || S.shell?.plan?.id || 0;

async function makeCurrent(id) {
  if (await plans.openPlan(id)) open(id);
}

function section(title, right) {
  const s = el("div", "pm-sec");
  const h = el("div", "pm-sub");
  h.append(el("span", "", title));
  if (right) h.append(el("span", "r", right));
  s.append(h);
  return s;
}

// Measurements update this section only; polls leave the shimmer and page controls intact.
function costSection(p) {
  const s = section(T("Loaded memory (estimate)"));
  s.classList.add("pm-cost");
  C.slot = s;
  if (C.page && !C.page.loading) showCost(C.page, p.id);
  else {
    const pending = el("div", "skel");
    pending.setAttribute("aria-label", T("Loaded memory (estimate)"));
    pending.setAttribute("aria-busy", "true");
    pending.append(el("span", "bar"), el("span", "bar"));
    C.progress = el("div", "muted pm-note", C.page?.progress || "");
    C.progress.setAttribute("aria-live", "polite");
    s.append(pending, C.progress);
  }
  return s;
}

async function readCost(id, seq) {
  const epoch = S.screenSeq, env = S.envSeq;
  const current = () => seq === C.seq && S.screen === "plans" && S.screenSeq === epoch && S.envSeq === env && M.sel === id && !M.draft;
  const apply = p => {
    C.page = p;
    if (p.loading) { if (C.progress) C.progress.textContent = p.progress || ""; }
    else showCost(p, id);
  };
  try {
    const first = await api.PlanLoadCost(id);
    if (!current() || !first) return;
    apply(first);
    const poll = async () => {
      if (!current()) return;
      try {
        const p = await api.PlanLoadCostMeasure(first.request);
        if (!current() || !p) return;
        apply(p);
        if (p.loading) setTimeout(poll, 100);
      } catch (e) { if (current()) apply({ error: errText(e) }); }
    };
    if (first.loading) setTimeout(poll, 100);
  } catch (e) { if (current()) apply({ error: errText(e) }); }
}

function costTable(headers, rows) {
  const wrap = el("div", "scroll"), table = el("table", "pm-cost-table");
  const head = el("thead"), tr = el("tr");
  for (const title of headers) { const th = el("th", "", T(title)); th.setAttribute("scope", "col"); tr.append(th); }
  head.append(tr); table.append(head);
  const body = el("tbody");
  for (const values of rows) {
    const tr = el("tr");
    for (const value of values) tr.append(el("td", "", value));
    body.append(tr);
  }
  table.append(body); wrap.append(table);
  return wrap;
}

function showCost(p, id) {
  const s = C.slot;
  if (!s) return;
  s.replaceChildren(el("div", "pm-sub", T("Loaded memory (estimate)")));
  if (p.error) { s.append(el("div", "muted pm-note", p.error)); return; }
  s.append(el("div", "word-facts", p.summary), el("div", "muted pm-note", p.detail));
  const details = el("details"), summary = el("summary", "muted pm-note", T("what is counted"));
  details.open = M.open.has("cost:" + id);
  details.ontoggle = () => { if (details.open) M.open.add("cost:" + id); else M.open.delete("cost:" + id); };
  details.append(summary, el("div", "muted pm-note", p.mods));
  details.append(costTable(["category", "files", "size on disk", "RAM range", "VRAM range"],
    (p.parts || []).map(r => [r.category, r.files, r.disk, r.ram, r.vram])));
  if (p.top?.length) {
    details.append(el("div", "pm-sub", T("largest contributors (five by RAM and VRAM)")));
    details.append(costTable(["mod", "RAM upper bound", "VRAM upper bound"], p.top.map(m => [m.name, m.ram, m.vram])));
  }
  for (const note of p.notes || []) details.append(el("div", "muted pm-note", note));
  s.append(details);
}

// ---- 游戏版本 (04 §1.6): one small button per version, the allowed ones raised ----

function versions(p) {
  const s = section(T("game versions"));
  const row = el("div", "vrow");
  const strip = el("div", "vstrip");
  const on = p.versions.filter((c) => c.on).map((c) => c.v);
  for (const c of p.versions) {
    const b = el("button", "v" + (c.on ? " on" : ""), c.v);
    if (c.game) b.append(el("span", "cur"));
    b.dataset.envWrite = "plan-versions";
    b.title = c.tip;
    b.onclick = () => {
      const next = c.on ? on.filter((v) => v !== c.v) : on.concat([c.v]);
      if (!next.length) return say("fail", T("keep at least one version"));
      setVersions(p, next);
    };
    strip.append(b);
  }
  row.append(strip);
  if (p.custom) {
    const r = el("button", "btn link", T("reset to default"));
    r.dataset.envWrite = "plan-versions";
    r.title = Tf("default: %s", p.default);
    r.onclick = () => setVersions(p, []);
    row.append(r);
  }
  s.append(row);
  return s;
}

async function setVersions(p, vs) {
  const before = p.custom ? p.versions.filter((c) => c.on).map((c) => c.v) : [];
  let t;
  try { t = await api.PlanVersionsSet(p.id, vs); } catch (e) { return say("fail", errText(e)); }
  await open();
  // the undo puts the versions back (the list's own undo is for its mods)
  say("ok", t, async () => {
    try { say("ok", await api.PlanVersionsSet(p.id, before)); } catch (e) { return say("fail", errText(e)); }
    if (S.screen === "plans") open();
  });
}

// ---- 跟着 Steam 合集 (04 §9.6) ----

function follows(p) {
  const s = section(T("follows Steam collections"), p.follows.length ? String(p.follows.length) : "");
  for (const f of p.follows) s.append(collection(p, f));
  if (!p.follows.length) s.append(el("div", "muted pm-note", T("no collection yet")));
  const add = el("div", "addcoll");
  const inp = el("input", "xc-input");
  inp.placeholder = T("paste a Steam collection link to make this list follow it");
  inp.spellcheck = false;
  const go = el("button", "btn", T("follow"));
  go.dataset.envWrite = "plan-follow";
  go.onclick = async () => {
    const link = inp.value.trim();
    if (!link) return inp.focus();
    go.disabled = true;
    say("busy", T("reading the collection…"));
    let t;
    try { t = await api.PlanFollow(p.id, link, "add"); } catch (e) { go.disabled = false; return say("fail", errText(e)); }
    await open();
    say("ok", t);
  };
  inp.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); go.click(); } });
  add.append(inp, go);
  s.append(add);
  return s;
}

function collection(p, f) {
  const c = el("div", "coll");
  const t = el("div", "t");
  t.append(ic("workshop"), el("b", "", f.title), el("span", "muted", Tf("%d items", f.items)));
  const r = el("span", "r");
  const ext = el("button", "btn icon", "");
  ext.append(ic("ext"));
  ext.title = T("open in Steam");
  ext.onclick = () => api.OpenURL(f.link).catch((e) => say("fail", errText(e)));
  const un = el("button", "btn link", T("stop following"));
  un.dataset.envWrite = "plan-unfollow";
  un.onclick = async () => {
    let tx;
    try { tx = await api.PlanUnfollow(p.id, f.id); } catch (e) { return say("fail", errText(e)); }
    await open();
    say("ok", tx);
  };
  r.append(ext, un);
  t.append(r);
  c.append(t);

  const meta = el("div", "meta word-facts", f.meta);
  if (f.notInstalled) {
    meta.append(el("span", "", Tf("%d not installed", f.notInstalled)));
    const sub = el("a", "xc-link", T("subscribe in the Workshop"));
    sub.title = f.link;
    sub.onclick = (e) => { e.preventDefault(); api.OpenURL(f.link).catch((x) => say("fail", errText(x))); };
    meta.append(sub);
  }
  c.append(meta);

  if (f.changes) {
    const key = p.id + ":" + f.id;
    const chg = el("div", "chg");
    chg.append(el("b", "", Tf("the collection changed: %s", f.changes)));
    const see = el("button", "btn link", M.open.has(key) ? T("hide the changes") : T("see the changes"));
    see.onclick = () => { M.open.has(key) ? M.open.delete(key) : M.open.add(key); draw(); };
    const sync = el("button", "btn primary small", T("catch up"));
    sync.dataset.envWrite = "plan-sync";
    sync.onclick = async () => {
      sync.disabled = true;
      let tx;
      try { tx = await api.PlanSync(p.id, f.id); } catch (e) { sync.disabled = false; return say("fail", errText(e)); }
      M.open.delete(key);
      await ui.reload();
      await open();
      // one undoable edit of that list: the undo link is the current list's only
      say("ok", tx, p.current);
    };
    chg.append(see, el("span", "sp"), sync);
    c.append(chg);
    if (M.open.has(key)) {
      const diff = el("div", "diff");
      for (const a of f.added || []) diff.append(el("div", "a", a));
      for (const x of f.removed || []) diff.append(el("div", "d", x));
      c.append(diff);
    }
  } else if (f.checked) {
    const same = el("div", "same");
    same.append(ic("ok"), T("same as the collection"));
    c.append(same);
  }

  const mode = el("div", "follow");
  mode.append(el("span", "", T("when catching up:")));
  mode.append(seg([T("add new only"), T("also disable dropped")], f.mode === "follow" ? 1 : 0, async (i) => {
    try { await api.PlanFollowMode(p.id, f.id, i ? "follow" : "add"); } catch (e) { say("fail", errText(e)); }
    open();
  }));
  for (const b of mode.querySelectorAll("button")) b.dataset.envWrite = "plan-follow-mode";
  c.append(mode);
  return c;
}

// ---- 分享: the newest pack made from it, export and create a pack ----

function share(p) {
  const s = section(T("share"));
  if (p.pack) {
    const r = el("div", "packrow");
    r.append(ic("packs"), el("b", "", p.pack.name), el("span", "meta word-facts", p.pack.facts));
    const o = el("button", "btn link r", T("open"));
    o.onclick = () => ui.go("pack:" + p.pack.name);
    r.append(o);
    s.append(r);
  }
  const acts = el("div", "acts-row");
  acts.append(act(T("export…"), "export", () => exportOf(p.id, "plans"), "e"), act(T("create a pack…"), "packs", () => buildOf(p), "B"));
  s.append(acts);
  return s;
}

// buildOf opens 编译成整合包 (§2.6) for the list: it becomes current first (no game file is
// written), and esc comes back here.
async function buildOf(p) {
  if (!p.current && !(await plans.openPlan(p.id))) return;
  S.buildFrom = "plans";
  ui.go("build");
}

// ---- keys and hints ----

function step(k) {
  const all = (M.data && M.data.rows) || [];
  const rows = M.created ? all.filter(r => r.id === M.created).concat(all.filter(r => r.id !== M.created)) : all;
  const i = rows.findIndex((r) => r.id === M.sel);
  if (!rows.length) return;
  let j = i;
  if (k === "ArrowDown" || k === "j") j = Math.min(rows.length - 1, i + 1);
  else if (k === "ArrowUp" || k === "k") j = Math.max(0, i - 1);
  else if (k === "Home") j = 0;
  else if (k === "End") j = rows.length - 1;
  pick(rows[j].id);
}

export function key(e) {
  const k = e.key;
  const kk = e.shiftKey && /^[a-z]$/.test(k) ? k.toUpperCase() : k;
  if (S.screen === "plan-export") return exportKey(e);
  if (M.draft) {
    if (k === "Escape") { e.preventDefault(); M.draft = null; open(); }
    return;
  }
  const p = sel();
  if (document.activeElement?.closest(".pm-detail") && ["Enter", "ArrowUp", "ArrowDown", "Home", "End"].includes(k)) return;
  if (["ArrowDown", "ArrowUp", "j", "k", "Home", "End"].includes(k)) { e.preventDefault(); return step(k); }
  if (!p) return;
  const map = {
    Enter: () => ui.go(p.page), n: () => newList(), i: () => ui.cmd("import"), e: () => exportOf(p.id, "plans"),
    B: () => buildOf(p), F2: () => plans.renamePlan(p.id, stay), Delete: () => plans.deletePlan(p.id, () => open(0)),
  };
  if (map[kk]) { e.preventDefault(); map[kk](); }
}

export function hintList() {
  if (M.draft && S.screen === "plans") return [{ key: "enter", label: T("create list") }, { key: "esc", label: T("cancel") }];
  if (S.screen === "plan-export") {
    return [{ key: "↑↓", label: T("format") }, { key: "ctrl+s", label: T("save as a file…") }, { key: "ctrl+c", label: T("copy") },
      { key: "esc", label: T("cancel") }, { key: "?", label: T("all keys") }];
  }
  return [{ key: "↑↓", label: T("pick a list") }, { key: "enter", label: T("edit") }, { key: "n", label: T("new list") }, { key: "i", label: T("import") },
    { key: "e", label: T("export") }, { key: "B", label: T("create a pack") }, { key: "?", label: T("all keys") }];
}

// ---- the export sub-page (库 / 列表 / <列表> / 导出) ----

// exportOf opens the export of list id; from is where esc goes back to: "plans" (this
// page) or "editor" (the plan editor's e).
export async function exportOf(id, from = "plans") {
  X.id = id; X.from = from;
  S.planExport = { id, from };
  return exportPage();
}

export const exportPlan = () => X.id;
export const exportFrom = () => X.from;

async function exportPage() {
  const read = beginPageRead("plan-export");
  if (!X.format) X.format = await api.Setting(FORMAT_KEY).catch(() => "") || "";
  if (!read.current()) return;
  let d, info;
  try { [d, info] = await Promise.all([api.PlanExport(X.id, X.format), X.info ? X.info : api.ExchangeInfo()]); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  X.data = d; X.info = info; X.format = d.format;
  drawExport();
  read.finish();
}

const fmtOf = () => X.data.formats.find((f) => f.id === X.data.format) || X.data.formats[0];
const extOf = (f) => (f.file.match(/(\.archolist\.json|-ModList\.xml|-workshop\.md|\.[^.]+)$/) || [""])[0].replace(/^-/, "");

function drawExport() {
  const d = X.data;
  const sc = S.screen === "plan-export" ? $("#screen") : openScreen("plan-export");
  sc.innerHTML = "";
  const w = page("plans pm-export", d.lead, button(T("cancel"), "esc", back));
  const grid = el("div", "pm-x");
  const left = el("section", "card pad");
  left.append(el("div", "pm-sub", T("format")));
  const list = el("div", "fmts");
  d.formats.forEach((f, i) => {
    const b = el("button", "fmt" + (f.id === d.format ? " on" : ""));
    b.append(el("span", "rd"));
    const nm = el("span", "nm", f.name);
    if (i === 0) nm.append(el("small", "", T("recommended")));
    b.append(nm, el("span", "ext", extOf(f)), el("span", "ds", f.desc));
    b.onclick = () => choose(f.id);
    list.append(b);
  });
  left.append(list);
  area(left, null, () => { const on = left.querySelector(".fmt.on"); if (on) on.focus({ preventScroll: true }); });

  const right = el("section", "card pad");
  const h = el("div", "pm-sub");
  h.append(el("span", "", T("content")), el("span", "r word-facts", d.facts));
  const pre = el("pre", "prev", d.text);
  const out = el("div", "xc-out");
  const acts = el("div", "acts-row");
  const f = fmtOf();
  if (f.id === "rimcrow") {
    const c = button(T("copy the share code"), "", () => copy(out));
    c.classList.add("primary");
    acts.append(c);
  } else {
    const s = button(T("save as a file…"), "", () => save(out));
    s.classList.add("primary");
    acts.append(s, button(T("copy"), "", () => copy(out)));
  }
  right.append(h, pre, acts, out);
  area(right, null);
  grid.append(left, right);
  w.append(grid);
  sc.append(w);
  ui.renderHints();
}

async function choose(id) {
  if (id === X.format) return;
  X.format = id;
  document.querySelectorAll("#screen .fmt").forEach((b, i) => b.classList.toggle("on", X.data.formats[i].id === id));
  api.SetSetting(FORMAT_KEY, id).catch(() => {});
  let d;
  try { d = await api.PlanExport(X.id, id); } catch (e) { return say("fail", errText(e)); }
  if (S.screen !== "plan-export" || X.format !== id) return;
  X.data = d;
  drawExport();
}

function back() {
  S.planExport = null;
  if (X.from === "editor") return ui.go("plan");
  return ui.go("plans");
}

function result(out, ok, text, paths = []) { pathContent(out, text, paths); out.className = "xc-out " + (ok ? "ok" : "err"); say(ok ? "ok" : "fail", text, false, paths); }

// save: the save dialog in the window (it asked before replacing a file), a typed path in
// the development harness (nothing is overwritten).
async function save(out) {
  const f = fmtOf();
  const write = async (path, overwrite) => {
    try { result(out, true, await api.PlanExportSave(X.id, f.id, path, overwrite), [path]); } catch (e) { result(out, false, errText(e)); }
  };
  if (X.info && X.info.dialogs) {
    let path = "";
    try { path = await api.PickExportFile(f.id); } catch (e) { return say("fail", errText(e)); }
    if (path) write(path, true);
    return;
  }
  const dir = X.info && X.info.dir ? X.info.dir + (X.info.dir.includes("/") ? "/" : "\\") : "";
  pop.ask(T("save as a file…"), T("the new file"), (path) => { if (path) write(path, false); }, dir + f.file);
}

async function copy(out) {
  let c;
  try { c = await api.PlanExportCopy(X.id, X.format); } catch (e) { return result(out, false, errText(e)); }
  let ok = c.copied;
  if (!ok && navigator.clipboard) { try { await navigator.clipboard.writeText(c.text); ok = true; } catch { /* shown below */ } }
  if (ok) return result(out, true, c.line);
  out.className = "xc-out";
  out.textContent = T("copy it from here:");
  say("", T("copy it from here:"));
  const pre = $("#screen .pm-export pre.prev");
  if (pre) { const r = document.createRange(); r.selectNodeContents(pre); const s = getSelection(); s.removeAllRanges(); s.addRange(r); }
}

function exportKey(e) {
  const k = e.key;
  if (k === "Escape") { e.preventDefault(); return back(); }
  if (!X.data) return;
  if (k === "ArrowDown" || k === "ArrowUp" || k === "j" || k === "k") {
    e.preventDefault();
    const fs = X.data.formats;
    const i = fs.findIndex((f) => f.id === X.format); // the one picked, before its text is in
    const j = Math.max(0, Math.min(fs.length - 1, i + (k === "ArrowDown" || k === "j" ? 1 : -1)));
    return choose(fs[j].id);
  }
  const out = $("#screen .pm-export .xc-out");
  if ((e.ctrlKey || e.metaKey) && k === "s" && fmtOf().id !== "rimcrow") { e.preventDefault(); return save(out); }
  if ((e.ctrlKey || e.metaKey) && k === "c" && !String(getSelection())) { e.preventDefault(); return copy(out); }
}
