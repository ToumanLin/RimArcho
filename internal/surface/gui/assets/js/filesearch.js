// Mod library › Search files (06 §2.9, 03 §5.4–5.5).
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { beginPageRead, page, area, button, seg, hintsChanged } from "./screens.js";
import { copyablePath, copy } from "./fields.js";

const defaults = ["defs", "patches", "about"];
const X = { text: "", code: false, regex: false, case: false, word: false, def: false,
  kinds: new Set(defaults), mods: [], info: "", result: null, at: null, cursor: 0,
  rows: [], running: false, request: null, seq: 0, visit: 0, progress: "",
  timer: null, candidatesTimer: null, candidatesSeq: 0, picker: false, candidates: [], pick: 0 };
let root, queryCard, resultsCard, observer;
let begins = Promise.resolve();
const visible = () => S.screen === "search" && root?.isConnected;

export async function open() {
  leave();
  const read = beginPageRead("search", { label: "loading…", retry: open });
  const visit = ++X.visit;
  root = page("filesearch", "");
  queryCard = el("section", "card fs-query");
  resultsCard = el("section", "card fs-results");
  const title = el("h2", "", T("results"));
  resultsCard.append(title);
  area(queryCard, null, focusInput);
  area(resultsCard, title, focusResults);
  root.append(queryCard, resultsCard);
  $("#screen").replaceChildren(root);
  drawQuery(); drawResults(); drawHead();
  // The shared screen calls Go's Leave; observe its replacement to clear local timers too.
  observer = new MutationObserver(() => { if (!visible()) leave(); });
  observer.observe($("#screen"), { childList: true });
  read.finish();
  try {
    const info = await api.FileSearchInfo();
    if (!read.current() || visit !== X.visit) return;
    X.info = info;
    const scope = root.querySelector(".fs-default-scope");
    if (scope) scope.textContent = info;
  } catch (e) { if (read.current() && visit === X.visit) say("fail", errText(e)); }
}

function drawHead() {
  if (!visible()) return;
  hintsChanged();
  root.querySelector(":scope > .page-head")?.remove();
  const lead = X.running ? X.progress || T("searching…") : X.result?.summary || "";
  if (!lead && !X.running) return;
  const head = el("div", "page-head");
  const text = el("p", "lead");
  text.textContent = lead;
  text.setAttribute("aria-live", "polite");
  head.append(text);
  if (X.running) {
    const actions = el("div", "page-acts");
    actions.append(button(T("stop"), "esc", () => stop(), "quiet"));
    head.append(actions);
  }
  root.prepend(head);
}

function toggle(label, name, title, shortcut) {
  const b = button(label, "", () => change(() => { X[name] = !X[name]; }), "small fs-toggle");
  b.dataset.option = name;
  b.title = title + " (" + shortcut + ")";
  b.setAttribute("aria-label", title);
  b.setAttribute("aria-pressed", String(X[name] || X.def && (name === "case" || name === "word")));
  b.disabled = X.def && (name === "case" || name === "word");
  b.hidden = X.code && name !== "regex";
  return b;
}

function drawQuery() {
  const old = document.activeElement;
  const focus = queryCard.contains(old) ? old?.dataset.focus : null;
  const selection = old?.tagName === "INPUT" ? [old.selectionStart, old.selectionEnd] : null;
  queryCard.replaceChildren();
  const top = el("div", "fs-input-row");
  top.append(el("span", "anum", "1"));
  const input = el("input", "fs-input");
  input.type = "text";
  input.placeholder = T("text to find");
  input.setAttribute("aria-label", T("text to find"));
  input.dataset.focus = "query";
  input.value = X.text;
  input.oninput = () => { X.text = input.value; };
  input.onkeydown = e => {
    if (e.isComposing) return;
    if (shortcuts(e)) { e.stopPropagation(); return; }
    if (e.key === "Enter") { consume(e); run(); }
    else if (e.key === "Escape") { consume(e); if (X.running) stop(); else { input.blur(); queryCard.focus(); } }
    else if (e.key === "ArrowDown" && !e.altKey && !e.ctrlKey && !e.metaKey) { consume(e); focusResults(); }
  };
  top.append(input,
    toggle("Aa", "case", T("case sensitive"), "alt+c"),
    toggle("ab", "word", T("whole word"), "alt+w"),
    toggle(".*", "regex", T("regular expression"), "alt+r"),
    toggle("Def", "def", T("Def name"), "alt+d"));
  queryCard.tabIndex = -1;
  queryCard.append(top);
  const kinds = el("div", "fs-options");
  kinds.append(el("span", "muted fs-label", T("kind")));
  kinds.append(seg([T("files"), T("C# code")], X.code ? 1 : 0,
    i => change(() => { X.code = i === 1; }), ["alt+1", "alt+2"]));
  if (!X.code) {
    const choices = [["defs", "Defs"], ["patches", T("patches")], ["about", "About"],
      ["languages", T("translations")], ["textures", T("textures")],
      ["sounds", T("sounds")], ["assemblies", T("assemblies")]];
    for (const [kind, label] of choices) {
      const b = button(label, "", () => change(() => {
        if (X.kinds.has(kind)) X.kinds.delete(kind); else X.kinds.add(kind);
      }), "small fs-kind");
      b.dataset.focus = kind;
      b.setAttribute("aria-pressed", String(X.kinds.has(kind)));
      kinds.append(b);
    }
  }
  queryCard.append(kinds);
  const scope = el("div", "fs-options fs-scope");
  scope.append(el("span", "muted fs-label", T("scope")));
  if (!X.mods.length) scope.append(el("span", "chip muted fs-default-scope", X.info));
  for (const mod of X.mods) {
    const chip = el("span", "chip fs-mod-chip");
    chip.append(document.createTextNode(mod.name));
    const remove = el("button", "", "×");
    remove.setAttribute("aria-label", T("remove") + " " + mod.name);
    remove.onclick = () => change(() => { X.mods = X.mods.filter(m => m.id !== mod.id); });
    chip.append(remove); scope.append(chip);
  }
  if (X.picker) {
    const picker = el("div", "fs-picker");
    const name = el("input", "fs-mod-input");
    name.placeholder = T("mod name");
    name.setAttribute("aria-label", T("mod name"));
    name.setAttribute("role", "combobox");
    name.setAttribute("aria-expanded", "true");
    name.setAttribute("aria-controls", "fs-candidates");
    name.setAttribute("aria-autocomplete", "list");
    name.dataset.focus = "mod";
    name.oninput = () => candidates(name.value);
    name.onkeydown = e => {
      if (e.isComposing) return;
      if (e.key === "Escape") { consume(e); closePicker(); }
      else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        consume(e);
        X.pick = Math.max(0, Math.min(X.candidates.length - 1, X.pick + (e.key === "ArrowDown" ? 1 : -1)));
        drawCandidates();
      } else if (e.key === "Enter") { consume(e); addMod(X.candidates[X.pick]); }
    };
    picker.append(name, el("div", "fs-candidates"));
    picker.lastChild.id = "fs-candidates";
    picker.lastChild.setAttribute("role", "listbox");
    scope.append(picker);
    queueMicrotask(() => { if (name.isConnected) name.focus(); });
    candidates("");
  } else {
    const add = button(T("+ Mod"), "", () => { X.picker = true; drawQuery(); }, "small");
    add.dataset.focus = "add";
    scope.append(add);
  }
  queryCard.append(scope);
  if (focus && !X.picker) {
    const next = queryCard.querySelector(`[data-focus="${focus}"]`);
    next?.focus({ preventScroll: true });
    if (selection && next?.tagName === "INPUT") next.setSelectionRange(...selection);
  }
}

function change(apply) {
  apply(); drawQuery();
  if (X.text.trim()) run();
  else if (X.running) stop(false);
}

function closePicker() {
  X.picker = false; ++X.candidatesSeq;
  clearTimeout(X.candidatesTimer); X.candidatesTimer = null;
  drawQuery(); queryCard.querySelector('[data-focus="add"]')?.focus();
}

function candidates(text) {
  const n = ++X.candidatesSeq;
  clearTimeout(X.candidatesTimer);
  X.candidates = []; X.pick = 0; drawCandidates();
  X.candidatesTimer = setTimeout(async () => {
    X.candidatesTimer = null;
    try {
      const mods = await api.ModCandidates(text);
      if (n !== X.candidatesSeq || !visible() || !X.picker) return;
      X.candidates = (mods || []).filter(m => m.id && !X.mods.some(x => x.id === m.id)).slice(0, 6);
      X.pick = 0; drawCandidates();
    } catch (e) { if (n === X.candidatesSeq && visible()) say("fail", errText(e)); }
  }, 120);
}

function drawCandidates() {
  const box = root?.querySelector(".fs-candidates");
  if (!box) return;
  box.replaceChildren();
  X.candidates.forEach((m, i) => {
    const b = button(m.name, "", () => addMod(m), "fs-candidate" + (i === X.pick ? " cur" : ""));
    b.id = "fs-candidate-" + i;
    b.setAttribute("role", "option");
    b.setAttribute("aria-selected", String(i === X.pick));
    b.tabIndex = -1;
    box.append(b);
  });
  const input = root.querySelector(".fs-mod-input");
  if (X.candidates.length) input?.setAttribute("aria-activedescendant", "fs-candidate-" + X.pick);
  else input?.removeAttribute("aria-activedescendant");
}

function addMod(mod) {
  if (!mod) return;
  X.mods.push({ id: mod.id, name: mod.name });
  closePicker();
  if (X.text.trim()) run();
}

function request(pageNumber) {
  const kinds = [...X.kinds];
  return { pattern: X.text.trim(), code: X.code, regex: X.regex,
    case: !X.code && (X.case || X.def), word: !X.code && (X.word || X.def), def: !X.code && X.def,
    kinds: kinds.length === defaults.length && defaults.every(k => X.kinds.has(k)) ? [] : kinds,
    mods: X.mods.map(m => m.id), page: pageNumber };
}

async function run(pageNumber = 1) {
  if (!visible()) return;
  const q = request(pageNumber);
  if (!q.pattern || q.code && !q.mods.length || !q.code && !X.kinds.size) {
    if (X.running) stop(false);
    if (q.pattern && X.result) { X.result = null; X.at = null; drawResults(); drawHead(); }
    say("fail", !q.pattern ? T("type what to look for") : q.code ?
      T("C# code searches only named mods: add one with + Mod first") : T("choose a file kind"));
    return;
  }
  // a refusal (or failure) of an earlier search is not about this one
  if ($("#status")?.classList.contains("fail")) say("", "");
  const seq = ++X.seq, visit = X.visit, screenSeq = S.screenSeq;
  const current = () => visible() && seq === X.seq && visit === X.visit && screenSeq === S.screenSeq;
  clearTimeout(X.timer); X.timer = null;
  X.running = true; X.request = null; X.progress = "";
  drawHead();
  try {
    const n = await (begins = begins.catch(() => {}).then(() => api.BeginFileSearch()));
    if (!current()) { api.StopFileSearch(n).catch(() => {}); return; }
    X.request = n;
    poll(n, current);
    const result = await api.FileSearch(q, n);
    if (!current() || n !== X.request) return;
    X.result = result; X.at = q; X.cursor = 0;
    drawResults();
  } catch (e) {
    if (!current()) return;
    if (!/context cancel(?:ed|led)/i.test(errText(e))) say("fail", errText(e));
  } finally {
    if (current()) {
      X.running = false; X.request = null;
      clearTimeout(X.timer); X.timer = null;
      drawHead();
    }
  }
}

function poll(n, current) {
  X.timer = setTimeout(async () => {
    X.timer = null;
    if (!current() || !X.running || n !== X.request) return;
    try {
      const progress = await api.FileSearchProgress(n);
      if (!current() || !X.running || n !== X.request) return;
      if (progress !== X.progress) { X.progress = progress; drawHead(); }
    } catch { /* the search itself reports errors */ }
    if (current() && X.running && n === X.request) poll(n, current);
  }, 300);
}

function stop(announce = true) {
  const n = X.request;
  ++X.seq; X.running = false; X.request = null;
  clearTimeout(X.timer); X.timer = null;
  if (n !== null) api.StopFileSearch(n).catch(e => { if (visible()) say("fail", errText(e)); });
  drawHead();
  if (announce) say("ok", T("stopped"));
}

export function leave() {
  ++X.seq; ++X.visit; ++X.candidatesSeq;
  clearTimeout(X.timer); clearTimeout(X.candidatesTimer);
  X.timer = X.candidatesTimer = null;
  if (X.request !== null) api.StopFileSearch(X.request).catch(() => {});
  X.running = false; X.request = null; X.picker = false;
  observer?.disconnect(); observer = null;
}

function drawResults() {
  const heading = resultsCard.querySelector(":scope > h2");
  resultsCard.replaceChildren(heading);
  resultsCard.tabIndex = -1;
  resultsCard.setAttribute("aria-label", T("results"));
  X.rows = [];
  const p = X.result;
  if (!p) return;
  for (const mod of p.mods || []) {
    const head = el("div", "fs-mod-head");
    head.append(el("span", "fs-position muted", mod.pos || ""));
    const link = el("a", "fs-mod-name", mod.name);
    link.href = "#";
    link.onclick = async e => {
      e.preventDefault();
      const visit = X.visit;
      try {
        const fullPage = await import("./fulldetail.js");
        if (visible() && visit === X.visit) fullPage.full("search", mod.id);
      } catch (error) { if (visible()) say("fail", errText(error)); }
    };
    head.append(link, el("span", "muted fs-count", Tf("%d matches", mod.count)));
    resultsCard.append(head);
    for (const file of mod.files || []) {
      const row = resultRow(mod, file, "fs-file");
      const path = el("span", "fs-path");
      path.textContent = file.rel;
      copyablePath(path, file.path);
      path.addEventListener("click", () => setCursor(X.rows.findIndex(r => r.node === row), false));
      row.append(path);
      if (file.count !== mod.count) row.append(el("span", "muted fs-count", Tf("%d matches", file.count)));
      if (file.more) row.append(el("span", "muted", Tf("+%d matches not shown", file.more)));
      resultsCard.append(row);
      for (const line of file.lines || []) {
        const r = resultRow(mod, file, "fs-line");
        r.append(el("span", "muted fs-line-number", line.line || ""));
        const content = el("div", "fs-line-content");
        if (line.role || line.within) {
          const label = el("span", "muted fs-context");
          label.textContent = [line.role, line.within].filter(Boolean).join("  ");
          content.append(label);
        }
        const text = el("span", "fs-text");
        highlight(text, line.text, X.at);
        content.append(text); r.append(content); resultsCard.append(r);
      }
    }
  }
  if (!X.rows.length) {
    resultsCard.append(el("p", "muted fs-empty", T("nothing found")));
    if (p.hint) resultsCard.append(el("p", "muted fs-note", p.hint));
  }
  for (const note of p.notes || []) resultsCard.append(el("p", "muted fs-note", note));
  if (p.page > 1 || p.more) {
    const paging = el("div", "fs-paging");
    const prev = button(T("previous page"), "[", () => run(p.page - 1));
    const next = button(T("next page"), "]", () => run(p.page + 1));
    prev.disabled = p.page <= 1; next.disabled = !p.more;
    paging.append(prev, el("span", "muted", Tf("page %d", p.page)), next);
    resultsCard.append(paging);
  }
  setCursor(Math.min(X.cursor, X.rows.length - 1), false);
  hintsChanged();
}

function resultRow(mod, file, cls) {
  const node = el("div", "fs-row " + cls);
  const i = X.rows.length;
  node.tabIndex = -1;
  node.onclick = () => { setCursor(i, false); node.focus({ preventScroll: true }); };
  node.onfocus = () => setCursor(i, false);
  X.rows.push({ node, modID: mod.id, path: file.path });
  return node;
}

function highlight(node, text, q) {
  text = String(text ?? "");
  if (!q?.pattern) { node.textContent = text; return; }
  const ranges = [];
  const whole = q.def || q.word;
  const boundary = (a, b) => !whole || (!/[\w]/.test(text[a - 1] || "") && !/[\w]/.test(text[b] || ""));
  if (q.regex && !q.def) {
    try {
      const re = new RegExp(q.pattern, q.case ? "g" : "gi");
      for (const m of text.matchAll(re)) if (m[0].length && boundary(m.index, m.index + m[0].length)) ranges.push([m.index, m.index + m[0].length]);
    } catch { /* Go uses RE2; a pattern may not be supported by JavaScript. */ }
  } else {
    const source = q.case || q.def ? text : text.toLowerCase();
    const pattern = q.case || q.def ? q.pattern : q.pattern.toLowerCase();
    for (let at = 0; at <= source.length - pattern.length;) {
      const start = source.indexOf(pattern, at);
      if (start < 0) break;
      const end = start + pattern.length;
      if (boundary(start, end)) ranges.push([start, end]);
      at = end;
    }
  }
  let at = 0;
  for (const [a, b] of ranges) {
    node.append(document.createTextNode(text.slice(at, a)));
    const mark = el("mark"); mark.textContent = text.slice(a, b); node.append(mark); at = b;
  }
  node.append(document.createTextNode(text.slice(at)));
}

function setCursor(i, scroll = true) {
  X.rows[X.cursor]?.node.classList.remove("cur");
  X.cursor = Math.max(0, i);
  const row = X.rows[X.cursor]?.node;
  row?.classList.add("cur");
  if (scroll && row) { row.focus({ preventScroll: true }); row.scrollIntoView({ block: "nearest" }); }
}
function focusResults() {
  if (X.rows.length) setCursor(X.cursor);
  else resultsCard?.focus();
}
export function focusInput() {
  const input = root?.querySelector(".fs-input");
  input?.focus(); input?.select();
}
function consume(e) { e.preventDefault(); e.stopPropagation(); }

function shortcuts(e) {
  const k = e.key.toLowerCase(), ctrl = e.ctrlKey || e.metaKey;
  if (ctrl && !e.altKey && k === "f" || !ctrl && !e.altKey && e.key === "/" && e.target.tagName !== "INPUT") {
    e.preventDefault(); focusInput(); return true;
  }
  if (!e.altKey || ctrl || e.shiftKey) return false;
  if (k === "1" || k === "2") { e.preventDefault(); change(() => { X.code = k === "2"; }); return true; }
  const name = { c: "case", w: "word", r: "regex", d: "def" }[k];
  if (!name || X.code && name !== "regex" || X.def && (name === "case" || name === "word")) return false;
  e.preventDefault(); change(() => { X[name] = !X[name]; }); return true;
}

export function key(e) {
  if (shortcuts(e)) return true;
  const k = e.key, ctrl = e.ctrlKey || e.metaKey;
  if (k === "Escape") {
    if (X.picker) closePicker(); else if (X.running) stop(); else return false;
    e.preventDefault(); return true;
  }
  if (e.altKey) return false;
  const row = X.rows[X.cursor];
  if (ctrl && k.toLowerCase() === "c" && row) {
    if (globalThis.getSelection()?.toString()) return false;
    e.preventDefault(); copy(row.path, row.node).catch(error => say("fail", errText(error))); return true;
  }
  if ((k === "Enter" && !ctrl || ctrl && k.toLowerCase() === "o") && row) {
    // Native controls (Mod links, copyable paths and paging) keep their own activation.
    if (k === "Enter" && e.target.closest?.("button, a, .path-copy")) return false;
    e.preventDefault();
    api.OpenSearchFolder(row.modID, row.path).then(() => { if (visible()) say("ok", T("opened the folder")); },
      error => { if (visible()) say("fail", errText(error)); });
    return true;
  }
  if (ctrl || e.shiftKey) return false;
  if ((k === "[" || k === "]") && X.result) {
    e.preventDefault();
    if (k === "[" && X.result.page > 1) run(X.result.page - 1);
    if (k === "]" && X.result.more) run(X.result.page + 1);
    return true;
  }
  if (!row) return false;
  let i = X.cursor;
  const step = Math.max(1, Math.floor(($("#screen").clientHeight - 100) / (row.node.getBoundingClientRect().height || 28)));
  if (k === "ArrowDown" || k === "j") i++;
  else if (k === "ArrowUp" || k === "k") i--;
  else if (k === "PageDown") i += step;
  else if (k === "PageUp") i -= step;
  else if (k === "Home") i = 0;
  else if (k === "End") i = X.rows.length - 1;
  else return false;
  e.preventDefault(); setCursor(Math.max(0, Math.min(X.rows.length - 1, i))); return true;
}

export function hintList() {
  const hints = [{ key: "/ ctrl+f", label: T("search") }];
  if (X.running) hints.push({ key: "esc", label: T("stop") });
  if (X.rows.length) hints.push({ key: "↑↓ j k", label: T("move the cursor") },
    { key: "enter ctrl+o", label: T("open the folder") }, { key: "ctrl+c", label: T("copy the path") });
  if (X.result && (X.result.page > 1 || X.result.more)) hints.push({ key: "[ ]", label: T("previous / next page") });
  hints.push({ key: "?", label: T("all keys") });
  return hints;
}
