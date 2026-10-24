// 列表管理 (06 §2.17): the sidebar's 列表 › 管理. On the left every list of the game, on the
// right the picked one: its game versions (04 §1.6), the Steam Workshop collections it
// follows (04 §9.6) and how to share it (export, pack). The export is a sub-page of its own
// (库 / 列表 / <列表> / 导出). Editing what a list holds stays in the editor (§2.5).
// Data: API.PlanManage (view.BuildPlanManage), API.PlanExport (view.BuildPlanExport).
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button, act, area, seg, beginPageRead } from "./screens.js";
import { icon } from "./icons.js";
import * as pop from "./popups.js";
import * as plans from "./plans.js";
import { wireSeparator } from "./separator.js";
import { showImportSummary, showImportContents } from "./exchange.js";

let ui = null;
export function initPlanManage(u) { ui = u; }

const M = { data: null, sel: 0, open: new Set(), checked: new Set(), draft: null, creating: false, created: 0, env: null, sourceSeq: 0, importSeq: 0, form: null };
const games = new Map(); // session drafts belong to a game, not its current list
const game = () => S.board?.env || "";
function syncGame() {
  if (M.env === game()) return;
  if (M.env !== null) games.set(M.env, { draft: M.draft, creating: M.creating, sel: M.sel, created: M.created });
  Object.assign(M, games.get(game()) || { draft: null, creating: false, sel: 0, created: 0 });
  M.env = game(); M.sourceSeq++; M.importSeq++;
  if (M.draft) { M.draft.loading = false; M.draft.importing = false; }
}
const X = { data: null, id: 0, from: "plans", format: "", info: null };
const FORMAT_KEY = "ui.gui.export.format";
const C = { seq: 0, page: null, slot: null, progress: null };

const ic = (name, cls = "ic") => { const s = el("span", cls); s.innerHTML = icon[name] || ""; return s; };
const sel = () => (M.data && M.data.sel) || null;

// ---- the page ----

// select picks the list the page opens on next (0: the current one).
export function select(id) { syncGame(); M.sel = id; }

// open draws the page with the list id picked (0: the one picked before, else the current).
export async function open(id) {
  syncGame();
  if (M.draft && M.draft.sourceEnv !== S.envSeq) M.draft.loading = false;
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
  if (M.creating && M.draft && !M.draft.sources && !M.draft.loading) void readSources(M.draft);
  if (d.sel && !M.creating) readCost(d.sel.id, costSeq);
  // a followed collection not read for ten minutes is read now; the page follows
  const s = d.sel;
  if (s && s.stale && !M.creating && !M.checked.has(s.id)) {
    M.checked.add(s.id);
    api.PlanCheck(s.id).then((changed) => { if (changed && S.screen === "plans" && M.sel === s.id && !M.creating) open(); }).catch(() => {});
  }
}

function draw(focus = "") {
  const focused = document.activeElement;
  const fieldClass = ["pm-name", "pm-source", "pm-version", "pm-save"].find(c => focused?.classList.contains(c));
  const caret = fieldClass === "pm-name" ? [focused.selectionStart, focused.selectionEnd] : null;
  if (!focus && fieldClass) focus = "." + fieldClass;
  const d = M.data;
  const sc = S.screen === "plans" ? $("#screen") : openScreen("plans");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  S.listCreating = M.creating;
  const add = act(M.draft ? T("continue new list") : T("new list"), "plus", () => newList(), "n");
  if (!M.creating) add.classList.add("primary");
  const w = page("plans pm-page", "", act(T("import as a new list"), "import", () => newList(null, "import"), "i"), add);
  const grid = el("div", "pm");
  const side = listCard(d), gap = el("div", "split pm-split");
  grid.append(side, gap, M.creating && M.draft ? newForm() : detailCard(d.sel));
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
  ui.renderWhere?.();
  ui.renderHints();
  if (focus) requestAnimationFrame(() => { const field = $(focus); field?.focus(); if (field && caret && focus === ".pm-name") field.setSelectionRange(...caret); });
}

function listCard(d) {
  const card = el("section", "card pm-list");
  const box = el("div", "scroll");
  if (M.draft) {
    const row = el("button", "pm-row pm-draft" + (M.creating ? " on" : ""));
    const n = el("div", "n");
    n.append(el("b", "draft-name", M.draft.name || T("new list")), el("span", "pill", T("draft")));
    row.append(n, el("div", "m", T("not created; continue any time")));
    row.onclick = () => resume();
    box.append(row);
  }
  const rows = M.created ? d.rows.filter(r => r.id === M.created).concat(d.rows.filter(r => r.id !== M.created)) : d.rows;
  for (const r of rows) {
    const b = el("button", "pm-row" + (!M.creating && r.id === M.sel ? " on" : ""));
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
  if (id === M.sel && !M.creating) return;
  M.creating = false; S.listCreating = false; M.importSeq++;
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
  r.append(button(T("copy as a new list…"), "", () => newList(p), "", "create-plan"));
  r.append(button(T("edit"), "↵", () => ui.go(p.page), "primary"));
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

// A single session draft per game. Navigation never discards its inputs.
export async function newList(p = null, from = "current") {
  if (S.screen !== "plans" || !M.data) await ui.go("plans");
  if (S.screen !== "plans") return;
  if (M.draft) {
    if (from === "import") M.draft.from = "import";
    resume();
    if (p) say("", T("a draft is already open; you can change its source list"));
    return;
  }
  const source = p?.id || sel()?.id || S.shell?.plan?.id;
  const draft = M.draft = { env: game(), source, sources: null, name: "", from, version: "current", save: "", import: { text: "", path: "", follow: true }, previous: S.shell?.plan?.id };
  M.creating = true;
  ui.pageEntering?.();
  draw(".pm-name"); ui.pageEntered?.();
  const pending = readSources(draft);
  try {
    const nm = await api.CheckPlanName(p ? Tf("%s copy", p.name) : "", "", 0);
    if (M.draft === draft && game() === draft.env && !draft.nameTouched) {
      draft.name = nm.free;
      if (S.screen === "plans" && M.creating) {
        const field = $(".pm-name"); if (field) field.value = draft.name;
        const row = $(".pm-draft .draft-name"); if (row) row.textContent = draft.name;
        draft.nameError = "";
        // The form revalidates on submit; keep its current controls and focus intact.
        if (!draft.importing) draw();
      }
    }
  } catch (e) { if (M.draft === draft) draft.error = errText(e); }
  await pending;
}

function resume() {
  M.importSeq++; if (M.draft) M.draft.importing = false;
  M.creating = true; ui.pageEntering?.();
  draw(".pm-name"); ui.pageEntered?.();
  if (!M.draft.sources && !M.draft.loading) void readSources(M.draft);
}

export function backToManagement() {
  if (!M.creating) return false;
  ui.pageEntering?.();
  M.creating = false; S.listCreating = false; M.importSeq++;
  if (M.draft) M.draft.importing = false;
  void open(); say("", T("draft kept; continue it from the list"));
  return true;
}

export const historyState = () => ({ creating: M.creating, selected: M.sel });
export function restoreHistory(state) {
  syncGame();
  M.creating = !!state?.creating && !!M.draft;
  if (state?.selected) M.sel = state.selected;
}

async function readSources(draft) {
  const n = ++M.sourceSeq, id = draft.source, env = S.envSeq;
  draft.loading = true; draft.sourceEnv = env; draft.sourceError = "";
  try {
    const sources = await api.PlanSources(id);
    if (M.draft !== draft || n !== M.sourceSeq || env !== S.envSeq || game() !== draft.env) return;
    draft.sources = sources; draft.loading = false;
    if (!sources.versions.some(v => v.from === draft.version)) draft.version = "current";
    if (!sources.saves.some(v => v.from === draft.save)) draft.save = sources.saves[0]?.from || "";
  } catch (e) {
    if (M.draft !== draft || n !== M.sourceSeq || env !== S.envSeq) return;
    draft.loading = false; draft.sourceError = errText(e);
  }
  if (S.screen === "plans" && M.creating && (draft.from === "current" || draft.from === "save")) draw();
}

function newForm() {
  const draft = M.draft;
  const card = el("section", "card pad pm-new");
  const head = el("div", "pm-head");
  const title = el("div"); title.append(el("h2", "", T("new list")), el("div", "pm-note", T("draft kept")));
  head.append(title, button(T("back to management"), "esc", backToManagement)); card.append(head);
  const form = el("form");
  M.form = form;
  const fields = el("fieldset", "pm-fields"); fields.disabled = !!draft.submitting;
  const label = el("label", "pm-field", T("new list name"));
  const name = el("input", "xc-input pm-name"); name.value = draft.name; name.spellcheck = false;
  label.append(name);
  const why = el("div", "plan-why", draft.error || ""); why.setAttribute("role", "alert");
  const create = button(T("create and edit"), "enter", () => submit(true), "primary", "create-plan");
  const only = button(T("create only"), "", () => submit(false), "", "create-plan");
  const active = () => M.draft === draft && M.form === form && M.creating && S.screen === "plans" && draft.env === game();
  const updateButtons = () => {
    const sourceOK = draft.from === "import" ? !!draft.import.preview && !draft.importing : draft.from === "" || !!draft.sources && !draft.loading && (draft.from !== "save" || !!draft.save);
    create.disabled = only.disabled = !!draft.submitting || !name.value.trim() || !!draft.nameError || !sourceOK;
  };
  let seq = 0;
  async function check() {
    const n = ++seq, value = name.value;
    create.disabled = only.disabled = true;
    try {
      const r = await api.CheckPlanName("", value, 0);
      if (n !== seq || !active()) return false;
      draft.nameError = r.problem || ""; why.textContent = draft.nameError; updateButtons();
      return !r.problem;
    } catch (e) { if (active()) { draft.nameError = errText(e); why.textContent = draft.nameError; } return false; }
  }
  name.oninput = () => {
    draft.name = name.value; draft.nameTouched = true; draft.error = "";
    const row = $(".pm-draft .draft-name"); if (row) row.textContent = draft.name || T("new list");
    void check();
  };
  fields.append(label, why, el("div", "pm-sub", T("start from")));
  const modes = el("div", "pm-modes"); modes.setAttribute("role", "group"); modes.setAttribute("aria-label", T("start from"));
  for (const [value, text] of [["current", T("copy a list")], ["", T("an empty list")], ["save", T("from a save")], ["import", T("import")]]) {
    const b = button(text, "", () => { M.importSeq++; draft.importing = false; draft.from = value; draft.error = ""; draw(); $(".pm-modes .on")?.focus(); });
    b.type = "button"; b.dataset.mode = value;
    b.classList.toggle("on", draft.from === value); b.setAttribute("aria-pressed", String(draft.from === value)); modes.append(b);
  }
  fields.append(modes);
  function choice(text, cls, options, value, change) {
    const field = el("label", "pm-field", text), select = el("select", "xc-input " + cls);
    for (const o of options) { const op = el("option", "", o.label); op.value = o.from; select.append(op); }
    select.value = value; select.onchange = () => change(select.value); field.append(select); fields.append(field); return select;
  }
  if (draft.from === "current") {
    choice(T("source list"), "pm-source", M.data.rows.map(r => ({ from: String(r.id), label: r.name })), String(draft.source), id => {
      draft.source = +id; draft.version = "current"; draft.sources = null; draft.sourceError = "";
      void readSources(draft); draw(".pm-source");
    });
    if (draft.sources) {
      choice(T("based on which version"), "pm-version", [{ from: "current", label: T("current editing content (including changes not written to the game)") }, ...draft.sources.versions], draft.version, v => { draft.version = v; draw(".pm-version"); });
      const summary = el("div", "pm-source-summary");
      summary.append(el("div", "", draft.version === "current" ? Tf("%d enabled mods; loading order is kept", draft.sources.count) : draft.sources.versions.find(v => v.from === draft.version)?.label));
      summary.append(el("div", "pm-note", T("a separate list with its own history; game versions are copied")));
      if (!draft.sources.versions.length) summary.append(el("div", "pm-note", T("this list has no history versions yet")));
      fields.append(summary);
    }
  } else if (draft.from === "save" && draft.sources) {
    if (draft.sources.saves.length) choice(T("choose a save"), "pm-save", draft.sources.saves, draft.save, v => { draft.save = v; });
    else fields.append(el("div", "pm-note", T("no saves found; choose a file under import")));
  } else if (draft.from === "import") {
    fields.append(inlineImport(draft, active, updateButtons));
  }
  if (draft.from !== "import" && draft.from !== "") {
    if (draft.loading) fields.append(el("div", "pm-note", T("reading source list…")));
    if (draft.sourceError) { fields.append(el("div", "plan-why", draft.sourceError), button(T("retry"), "", () => { void readSources(draft); draw(); })); }
  }
  const bottom = el("div", "pm-create-bottom");
  bottom.append(el("div", "pm-note", T("the new list becomes current; game files change only when writing or launching")));
  const actions = el("div", "acts-row");
  const discard = button(T("discard draft"), "", () => { const kept = draft; M.draft = null; backToManagement(); say("", T("draft discarded"), () => { if (game() !== kept.env || M.draft) return; M.draft = kept; return ui.go("plans").then(resume); }); });
  actions.append(discard, button(T("continue later"), "", backToManagement), only, create);
  bottom.append(actions); fields.append(bottom); form.append(fields);
  for (const b of form.querySelectorAll("button")) b.type = "button";
  form.onkeydown = e => {
    if (e.key === "Enter" && e.target === name) { e.preventDefault(); e.stopPropagation(); void submit(true); }
  };
  form.onsubmit = e => { e.preventDefault(); void submit(true); };
  async function submit(edit) {
    if (draft.submitting) return;
    draft.submitting = true; fields.disabled = true;
    const env = S.envSeq, epoch = S.screenSeq;
    const visible = () => active() && env === S.envSeq && epoch === S.screenSeq;
    try {
      if (!(await check()) || !visible()) return;
      const from = draft.from === "save" ? draft.save : draft.from === "current" ? draft.version : draft.from;
      if (draft.from === "current" && !draft.sources || draft.from === "save" && !draft.save || draft.from === "import" && !draft.import.preview) return;
      const r = draft.from === "import" ? await api.ImportAsPlanPreview(draft.name, draft.import.preview.request, draft.import.follow) : await api.CreatePlan(draft.source, draft.name, from);
      const stillHere = visible();
      if (M.draft === draft) {
        M.draft = null; M.created = +r.page.slice(5); M.creating = false; S.listCreating = false;
      } else if (games.get(draft.env)?.draft === draft) {
        Object.assign(games.get(draft.env), { draft: null, creating: false, created: +r.page.slice(5) });
      }
      if (env !== S.envSeq || game() !== draft.env) return;
      await ui.reload();
      if (stillHere && S.screen === "plans" && epoch === S.screenSeq) { if (edit) await ui.go(r.page); else await open(M.created); }
      say("ok", r.text, !edit ? async () => {
        try {
          if (draft.previous) await api.UsePlan(draft.previous);
          const t = await api.RemovePlan(+r.page.slice(5));
          await ui.reload(); await open(draft.previous || 0); say("ok", t.text);
        } catch (e) { say("fail", errText(e)); }
      } : false);
    } catch (e) { if (visible()) { draft.error = errText(e); why.textContent = draft.error; } }
    finally { draft.submitting = false; if (active()) { fields.disabled = false; updateButtons(); } }
  }
  card.append(form);
  updateButtons();
  return card;
}

function inlineImport(draft, active, updateButtons) {
  const box = el("div", "pm-inline-import"), imp = draft.import;
  const input = el("label", "pm-field", T("file, share code or collection"));
  const text = el("textarea", "xc-text"); text.value = imp.text; text.spellcheck = false;
  text.placeholder = T("a list from RimSort or RimCrow, a share code (RC-…), a Rentry link, or a Steam Workshop collection link");
  input.append(text); box.append(input);
  const status = el("div", "pm-note", draft.importing ? T("reading the list…") : ""); status.setAttribute("role", "status");
  const error = el("div", "plan-why", imp.error || ""); error.setAttribute("role", "alert");
  const preview = el("div", "pm-inline-preview");
  function invalidate() {
    M.importSeq++; draft.importing = false; imp.preview = null; imp.error = "";
    preview.replaceChildren(); status.textContent = ""; error.textContent = ""; updateButtons();
  }
  text.oninput = () => { imp.text = text.value; invalidate(); };
  const actions = el("div", "acts-row");
  const file = el("input"); file.type = "file"; file.hidden = true; file.accept = ".xml,.rml,.json,.txt,.rws";
  const path = el("input", "xc-input pm-import-path"); path.value = imp.path; path.spellcheck = false;
  path.placeholder = T("a file path: .xml .rml .json .txt, a save (.rws)");
  path.setAttribute("aria-label", T("from a file"));
  path.oninput = () => { imp.path = path.value; invalidate(); };
  async function read(kind, value, fileName) {
    if (!value.trim() || !active()) return;
    const n = ++M.importSeq, env = S.envSeq;
    const current = () => active() && n === M.importSeq && env === S.envSeq;
    imp.preview = null; imp.error = ""; draft.importing = true;
    preview.replaceChildren(); error.textContent = ""; status.textContent = T("reading the list…"); updateButtons();
    try {
      const p = await api.ImportPreviewFor(selected(), kind, value);
      if (!current()) return;
      if (fileName) p.source = fileName;
      imp.preview = p; imp.kind = kind; imp.value = value; draft.importing = false;
      status.textContent = ""; renderPreview(); updateButtons();
    } catch (e) {
      if (!current()) return;
      draft.importing = false; imp.error = errText(e); error.textContent = imp.error; status.textContent = ""; updateButtons();
    }
  }
  async function readFile(f) {
    invalidate();
    const n = ++M.importSeq;
    draft.importing = true; updateButtons();
    try {
      const value = await (/\.rws$/i.test(f.name) ? f.slice(0, 4 << 20) : f).text();
      if (!active() || n !== M.importSeq) return;
      imp.text = value; text.value = value; imp.path = ""; path.value = "";
      await read("text", value, f.name);
    } catch (e) { if (active() && n === M.importSeq) { draft.importing = false; error.textContent = errText(e); updateButtons(); } }
  }
  file.onchange = () => { if (file.files[0]) void readFile(file.files[0]); };
  actions.append(button(T("choose a file…"), "", async () => {
    const epoch = S.envSeq;
    try {
      const info = await api.ExchangeInfo();
      if (!active() || epoch !== S.envSeq) return;
      if (!info.dialogs) { file.click(); return; }
      const picked = await api.PickImportFile();
      if (picked && active() && epoch === S.envSeq) { imp.path = picked; path.value = picked; await read("file", picked); }
    } catch (e) { if (active()) error.textContent = errText(e); }
  }), button(T("preview"), "", () => read("text", text.value)), file);
  const pathRow = el("div", "xc-row");
  pathRow.append(path, button(T("read"), "", () => read("file", path.value)));
  box.append(actions, pathRow, status, error, preview);
  box.addEventListener("dragover", e => { if (e.dataTransfer?.types.includes("Files")) e.preventDefault(); });
  box.addEventListener("drop", e => { const f = e.dataTransfer?.files?.[0]; if (f) { e.preventDefault(); e.stopPropagation(); void readFile(f); } });
  function renderPreview() {
    preview.replaceChildren();
    const p = imp.preview; if (!p) return;
    showImportSummary(preview, p);
    if (p.collection) {
      const label = el("label", "xc-check"), follow = el("input"); follow.type = "checkbox"; follow.checked = imp.follow;
      follow.onchange = () => { imp.follow = follow.checked; };
      label.append(follow, el("span", "", T("follow this collection from now on"))); preview.append(label);
    }
    showImportContents(preview, p);
  }
  renderPreview();
  return box;
}

// Leave only transient reads behind; the session draft survives ordinary navigation.
export function abandon() { M.importSeq++; if (M.draft) M.draft.importing = false; C.seq++; C.slot = null; M.form = null; }
export const selected = () => { syncGame(); return M.sel || S.shell?.plan?.id || 0; };

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
  const current = () => seq === C.seq && S.screen === "plans" && S.screenSeq === epoch && S.envSeq === env && M.sel === id && !M.creating;
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
  if (M.creating) {
    if (k === "Escape") { e.preventDefault(); backToManagement(); }
    return;
  }
  const p = sel();
  if (document.activeElement?.closest(".pm-detail") && ["Enter", "ArrowUp", "ArrowDown", "Home", "End"].includes(k)) return;
  if (["ArrowDown", "ArrowUp", "j", "k", "Home", "End"].includes(k)) { e.preventDefault(); return step(k); }
  if (!p) return;
  const map = {
    Enter: () => ui.go(p.page), n: () => newList(), i: () => newList(null, "import"), e: () => exportOf(p.id, "plans"),
    B: () => buildOf(p), F2: () => plans.renamePlan(p.id, stay), Delete: () => plans.deletePlan(p.id, () => open(0)),
  };
  if (map[kk]) { e.preventDefault(); map[kk](); }
}

export function hintList() {
  if (M.creating && S.screen === "plans") return [{ key: "enter", label: T("create and edit") }, { key: "esc", label: T("back to management") }];
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

function result(out, ok, text) { out.textContent = text; out.className = "xc-out " + (ok ? "ok" : "err"); say(ok ? "ok" : "fail", text); }

// save: the save dialog in the window (it asked before replacing a file), a typed path in
// the development harness (nothing is overwritten).
async function save(out) {
  const f = fmtOf();
  const write = async (path, overwrite) => {
    try { result(out, true, await api.PlanExportSave(X.id, f.id, path, overwrite)); } catch (e) { result(out, false, errText(e)); }
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
