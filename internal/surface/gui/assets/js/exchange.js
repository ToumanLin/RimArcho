// Import and export of the mod list (04 §9): to and from RimSort, RimCrow, RimWorld's
// own .rml, share codes, Rentry and Workshop collections. An import is previewed first —
// what is installed, what is missing and where to get it, what the enabled list becomes —
// and applied as one undo step; nothing here writes ModsConfig.xml (that stays save).
import { factText, S, $, el, api, T, Tf, errText, say } from "./state.js";
import { page, button, beginPageRead } from "./screens.js";

let ui = null;
export function initExchange(u) {
  ui = u;
  // File drops anywhere in the window enter the same import flow (06 §2.17).
  document.addEventListener("dragover", e => { if (e.dataTransfer?.types.includes("Files")) e.preventDefault(); });
  document.addEventListener("drop", async e => {
    if (e.defaultPrevented) return;
    const f = e.dataTransfer?.files?.[0];
    if (!f) return;
    e.preventDefault();
    if (S.screen !== "plan-import") await exchange();
    readFile(f);
  });
}

let info = null;
let pasted = "";
let session = null;
let previewSeq = 0;

// Import is a sub-page, including when entered from the first-run guide (06 §2.17).
export async function importFrom(kind, src) {
  await exchange();
  if (S.screen === "plan-import") return preview(kind, src);
}

export async function exchange() {
  const from = S.screen === "plan-import" ? session?.from : S.screen || "plan";
  const target = from === "plans" ? ui.selectedList() : S.shell?.plan?.id || 0;
  const read = beginPageRead("plan-import", { replace: true });
  session = { from, target, name: S.shell?.plans?.find(p => p.id === target)?.name || "", epoch: S.screenSeq };
  ++previewSeq;
  try { info = await api.ExchangeInfo(); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  const sc = $("#screen"); sc.innerHTML = "";
  const w = page("plans pm-import", "", button(T("cancel"), "esc", cancel));
  const body = el("section", "card pad xc-body");
  const result = el("section", "card pad xc-preview"); result.hidden = true;
  w.append(body, result); sc.append(w);
  importSource(body);
  body.append(el("div", "xc-hint", T("or drop a file here")));
  read.finish(); ui.renderWhere(); ui.renderHints();
}

export function cancel() {
  ++previewSeq;
  const from = session?.from || "plans";
  ui.afterImport = null;
  return from === "guide" ? ui.resumeGuide() : ui.go(from);
}
export function key(e) { if (e.key === "Escape") { e.preventDefault(); cancel(); } }
export const hintList = () => [{ key: "tab", label: T("import destination") }, { key: "enter", label: T("import") }, { key: "esc", label: T("cancel") }];

// ---- import, step 1: where the list comes from ----

function importSource(body) {
  body.append(el("div", "xc-note", T("RimArcho, RimSort, RimCrow and RimWorld lists, share codes, packageIds or workshop links. Nothing changes before you confirm.")));

  // a file: the window's open dialog, a typed path, or a dropped file
  const file = section(body, T("from a file"));
  const row = el("div", "xc-row");
  const path = el("input", "xc-input");
  path.placeholder = T("a file path: .xml .rml .json .txt, a save (.rws)");
  path.spellcheck = false;
  const read = el("button", "btn", T("read"));
  read.onclick = () => path.value.trim() && preview("file", path.value.trim());
  path.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); read.click(); } });
  {
    const pick = el("button", "btn primary", T("choose a file…"));
    pick.onclick = async () => {
      let f = "";
      try { f = await api.PickImportFile(); } catch (e) { return say("fail", errText(e)); }
      if (f) { path.value = f; preview("file", f); }
      else if (!info.dialogs) fileInput.click();
    };
    const fileInput = el("input"); fileInput.type = "file"; fileInput.hidden = true;
    fileInput.accept = ".xml,.rml,.json,.txt,.rws";
    fileInput.onchange = () => { if (fileInput.files[0]) readFile(fileInput.files[0]); };
    row.append(pick, fileInput);
  }
  row.append(path, read);



  // pasted text: a list, a share code, a Rentry or collection link
  const paste = section(body, T("paste"));
  const ta = el("textarea", "xc-text");
  ta.spellcheck = false;
  ta.placeholder = T("a list from RimSort or RimCrow, a share code (RC-…), a Rentry link, or a Steam Workshop collection link");
  ta.value = pasted;
  ta.oninput = () => { pasted = ta.value; };
  const btns = el("div", "xc-row end");
  const clip = el("button", "btn", T("paste from the clipboard"));
  clip.onclick = async () => {
    let t = "";
    try { t = await api.Clipboard(); } catch { /* the page asks the browser */ }
    if (!t && navigator.clipboard) { try { t = await navigator.clipboard.readText(); } catch { /* not allowed */ } }
    if (!t) return say("", T("the clipboard has no text"));
    ta.value = pasted = t;
  };
  const go = el("button", "btn primary", T("preview"));
  go.onclick = () => ta.value.trim() && preview("text", ta.value);
  btns.append(el("span", "xc-hint", T("links are read from Steam or Rentry when you press preview")), clip, go);
  paste.append(ta, row, btns);
  file.remove();
  requestAnimationFrame(() => { if (S.screen === "plan-import") ta.focus(); });
}

async function readFile(f) {
  const active = session;
  try {
    const blob = /\.rws$/i.test(f.name) ? f.slice(0, 4 << 20) : f;
    const text = await blob.text();
    if (active === session && S.screen === "plan-import") preview("text", text, f.name);
  } catch (e) { say("fail", errText(e)); }
}

// ---- import, step 2: the preview ----

async function preview(kind, src, fileName) {
  const body = $("#screen .xc-preview");
  const active = session, seq = ++previewSeq;
  const current = () => S.screen === "plan-import" && session === active && S.screenSeq === active.epoch && seq === previewSeq;
  if (!body) return;
  say("busy", kind === "text" && /^\s*\S+\s*$/.test(src) && /steamcommunity|rentry|^\s*\d{6,}\s*$/.test(src) ? T("reading the link…") : T("reading the list…"));
  let p;
  body.hidden = false; body.textContent = T("reading the list…");
  try { p = await api.ImportPreviewFor(active.target, kind, src); }
  catch (e) { if (current()) { body.textContent = errText(e); say("fail", errText(e)); } return; }
  if (!current()) return;
  say("", "");
  if (fileName) p.source = fileName;
  body.innerHTML = "";

  showImportSummary(body, p, !!(p.sourcePath || fileName));

  // A destination is selected first; only the final import button changes data (06 §2.17).
  const ways = el("fieldset", "xc-sec");
  ways.append(el("legend", "xc-sec-t", T("import destination")));
  let destination = "new", busy = false;
  const name = el("input", "xc-input"); name.spellcheck = false;
  const base = p.name || (p.source || "").replace(/^.*[\\/]/, "").replace(/\.[^.]*$/, "");
  const free = await api.CheckPlanName(base, "", 0).catch(() => ({ free: base }));
  if (!current()) return;
  name.value = free.free;
  const why = el("div", "plan-why");
  const submit = button(T("import"), "enter", perform, "primary", "import-apply");
  submit.title = T("does not write game files");
  const options = [["new", T("import as a new list"), Tf("%d mods; the current list stays as it is", p.total)],
    ["replace", Tf("replace %s", active.name), p.replace.text], ["merge", T("add the ones you lack"), p.merge.text]];
  for (const [value, title, text] of options) {
    const row = el("label", "xc-check");
    const radio = el("input"); radio.type = "radio"; radio.name = "destination"; radio.checked = value === destination;
    radio.onchange = () => { destination = value; name.disabled = value !== "new"; checkName(); };
    row.append(radio, el("span", "", title), el("span", "xc-note", text)); ways.append(row);
  }
  let nameSeq = 0;
  async function checkName() {
    const n = ++nameSeq;
    if (destination !== "new") { why.textContent = ""; submit.disabled = busy; return true; }
    submit.disabled = true;
    try {
      const r = await api.CheckPlanName("", name.value, 0);
      if (!current() || n !== nameSeq) return false;
      why.textContent = r.problem; submit.disabled = busy || !!r.problem; return !r.problem;
    } catch (e) { if (current()) why.textContent = errText(e); return false; }
  }
  name.oninput = checkName;
  name.onkeydown = e => { if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); perform(); } };
  ways.append(name, why); body.append(ways);
  let follow = true;
  async function perform() {
    if (busy || !(await checkName()) || !current()) return;
    busy = true; submit.disabled = true;
    try {
      await api.ImportFollow(follow);
      const r = destination === "new" ? await api.ImportAsPlan(name.value) : await api.ImportApplyTo(active.target, destination === "merge");
      if (!current()) return;
      for (const c of S.cols) { c.sel.clear(); c.query = ""; c.cursor = 0; }
      const after = active.from === "guide" ? ui.afterImport : null;
      ui.afterImport = null;
      await ui.reload();
      if (after) return after();
      const id = destination === "new" ? +r.page.slice(5) : active.target;
      await ui.manageLists(id);
      say("ok", destination === "new" ? r.text : r, destination !== "new" ? async () => {
        try { await api.UsePlan(id); const t = await api.Undo(); await ui.reload(); await ui.manageLists(id); say("ok", t); }
        catch (e) { say("fail", errText(e)); }
      } : false);
    } catch (e) { if (current()) { say("fail", errText(e)); busy = false; checkName(); } }
  }
  checkName();
  // a Steam Workshop collection: the list can follow it (04 §9.6); ticked by default
  if (p.collection) {
    const lb = el("label", "xc-check");
    const cb = el("input");
    cb.type = "checkbox";
    cb.checked = true;
    cb.onchange = () => { follow = cb.checked; };
    lb.append(cb, el("span", "", T("follow this collection from now on")), el("span", "xc-hint", T("RimArcho tells you when it changes; catching up is one undoable step")));
    body.append(lb);
  }
  body.append(submit);
  body.onkeydown = e => {
    if (e.key === "Enter" && !e.target.matches("button, a, select")) { e.preventDefault(); e.stopPropagation(); perform(); }
  };

  showImportContents(body, p);
}

// Shared by the standalone destination flow and the new-list form's inline source.
// sourceIsPath: the source is a file the player picked, so it copies on a click (06 §3.1 rule 12).
export function showImportSummary(body, p, sourceIsPath = !!p.sourcePath) {
  const top = el("div", "xc-sum");
  top.append(el("div", "xc-big", p.summary));
  const facts = [sourceIsPath && p.source ? { text: p.source, paths: [p.source] } : p.source];
  if (p.name) facts.push(Tf("list: %s", p.name));
  if (p.game) facts.push("RimWorld " + p.game);
  if (p.allowed) facts.push(Tf("suggested game versions: %s", p.allowed));
  top.append(factText(el("div", "xc-note"), facts.filter(Boolean)));
  if (p.gameNote) top.append(el("div", "xc-note warn", p.gameNote));
  body.append(top);
}

export function showImportContents(body, p) {
  if (p.missing.length) {
    const sec = section(body, Tf("not installed: %d", p.missing.length));
    const list = el("div", "xc-list");
    for (const r of p.missing) {
      const d = el("div", "xc-mod");
      const n = el("div", "n");
      n.append(el("span", "", r.name));
      if (r.package && r.package !== r.name) n.append(el("span", "dim", r.package));
      d.append(n);
      if (r.instead) {
        const i = el("div", "xc-instead", Tf("replaced by %s", r.instead));
        if (r.insteadUrl) i.append(link(T("Workshop page"), r.insteadUrl));
        d.append(i);
      }
      if (r.url) d.append(link(T("subscribe"), r.url, true));
      else d.append(el("span", "dim", T("not on the Workshop list")));
      list.append(d);
    }
    sec.append(list);
  }
  if (p.installed.length) {
    // a RimArcho list names each mod's version: the ones installed in another version are
    // marked, and the list opens to show them
    const det = el("details", "xc-sec");
    let sum = Tf("installed: %d (%d enabled now)", p.installed.length, p.installed.filter((r) => r.enabled).length);
    if (p.other) { sum += "  " + Tf("%d in another version", p.other); det.open = true; }
    det.append(el("summary", "word-facts", sum));
    const list = el("div", "xc-list");
    p.installed.forEach((r, i) => {
      const d = el("div", "xc-mod");
      d.append(el("span", "pos", String(i + 1)), el("span", r.enabled ? "" : "accent", r.name));
      if (r.other) d.append(el("span", "chip mark", T("another version")));
      if (!r.enabled) d.append(el("span", "dim", T("not enabled now")));
      list.append(d);
    });
    det.append(list);
    body.append(det);
  }
}

function link(text, url, button) {
  const a = el(button ? "button" : "a", button ? "btn small" : "xc-link", text);
  a.title = url;
  a.onclick = (e) => { e.preventDefault(); api.OpenURL(url).catch((x) => say("fail", errText(x))); };
  return a;
}

function section(body, title) {
  const s = el("div", "xc-sec");
  s.append(el("div", "xc-sec-t", title)); body.append(s); return s;
}
