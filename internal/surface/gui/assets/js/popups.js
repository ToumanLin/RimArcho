import { closeChoiceMenu } from "./choices.js";
// Popups (16 §3.1 rule 5): the command palette, the tag bar, the position input, a
// confirmation, a text or choice panel, help. Each has a title, shows in the position
// line, closes with esc, and never nests: opening one closes the other.
import { pathContent, copyablePath } from "./fields.js";
import { S, $, el, esc, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { buildCommandPanel } from "./command-panel.js";

const MODALS = new Set(["url", "memory", "uninstall-confirm", "plan-rename", "ask"]);
let ui = null;
let modalFocus = null;
// Wire browser focus separately from popup construction, as with lists and history.
export function wireModalFocus() { modalFocus = modal; }

function modal(pop, p) {
  const title = pop.querySelector(".ttl"), body = pop.querySelector(".body");
  pop.setAttribute("role", "dialog");
  pop.setAttribute("aria-modal", "true");
  pop.removeAttribute("aria-label");
  if (title) { title.id = "popup-title"; pop.setAttribute("aria-labelledby", title.id); }
  if (body) { body.id = "popup-message"; pop.setAttribute("aria-describedby", body.id); }
  p.background = [...$("#app").children].filter(n => n.id !== "popup").map(n => [n, n.inert]);
  for (const [n] of p.background) n.inert = true;
  // Capture before local handlers (including the uninstall dialog's older Tab cycle).
  pop.addEventListener("keydown", e => {
    if (e.isComposing) return;
    if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && ["b", "k"].includes(e.key.toLowerCase())) return;
    // ctrl+\ shows the reference numbers (refnums.js, on the window) over a dialog too.
    if ((e.ctrlKey || e.metaKey) && !e.altKey && (e.code === "Backslash" || e.key === "\\" || e.key === "|")) return;
    e.stopPropagation();
    if (e.key === "Escape") { e.preventDefault(); close(); return; }
    if (e.key === "Tab") {
      const controls = [...pop.querySelectorAll('button, input, select, textarea, a[href], [tabindex]')]
        .filter(n => n.tabIndex >= 0 && !n.disabled && !n.closest("[inert]") && n.checkVisibility());
      const i = controls.indexOf(document.activeElement);
      e.preventDefault();
      // From the dialog itself (a confirmation's start), Tab goes to the first control, shift+Tab to the last.
      const next = i < 0 ? (e.shiftKey ? controls.length - 1 : 0) : (i + (e.shiftKey ? controls.length - 1 : 1)) % controls.length;
      (controls[next] || pop).focus();
      return;
    }
    if ((e.key === "Enter" || e.key === " ") && e.target.closest("button")) {
      e.preventDefault();
      if (!e.repeat) e.target.closest("button").click();
      return;
    }
    p.keys?.(e);
  }, true);
  p.restoreFocus = () => {
    const opener = p.opener;
    const target = opener?.isConnected && !opener.closest("[inert]") && opener.checkVisibility() && opener !== document.body && !opener.disabled ? opener :
      document.querySelector('#screen[role="main"], #board:not([inert])');
    target?.focus({ preventScroll: true, focusVisible: target === opener && p.openerFocusVisible });
  };
  // A confirmation starts on the dialog itself: Enter confirms, as its enter key cap says, and
  // no button is pre-selected; Tab reaches Cancel. A prompt starts in its field; the RAM warning
  // and uninstall on their Cancel, as they always have.
  if (p.kind === "confirm") pop.tabIndex = -1;
  const initial = p.kind === "confirm" ? pop : pop.querySelector('input:not([type=checkbox]):not([type=radio])') || pop.querySelector(".btns button");
  if (initial?.matches("input")) requestAnimationFrame(() => { if (S.popup === p) initial.focus({ preventScroll: true }); });
  else initial?.focus({ preventScroll: true, focusVisible: false });
}

export function initPopups(u) { ui = u; }

export function open(kind, title, build) {
  closeChoiceMenu();
  close();
  const opener = document.activeElement;
  const box = $("#popup");
  box.classList.remove("command-dock");
  box.innerHTML = "";
  const pop = el("div", "pop");
  box.append(pop);
  box.hidden = false;
  box.onmousedown = (e) => { if (e.target === box) close(); };
  const p = S.popup = { kind, title, keys: null, opener, openerFocusVisible: opener?.matches?.(":focus-visible") };
  build(pop, p);
  if (p.modal || MODALS.has(kind)) { p.modal = true; modalFocus?.(pop, p); }
  ui.renderWhere();
}

export function close() {
  if (!S.popup || S.popup.canClose?.() === false) return;
  const p = S.popup;
  for (const [n, inert] of p.background || []) n.inert = inert;
  p.onclose?.();
  S.popup = null;
  const box = $("#popup");
  box.hidden = true;
  box.classList.remove("command-dock");
  box.innerHTML = "";
  ui.renderWhere();
  ui.renderHeads();
  p.restoreFocus?.();
}

// popupKey handles a key while a popup is open; true when it took it.
export function popupKey(e) {
  if (!S.popup) return false;
  if (e.key === "Escape") { close(); return true; }
  if (S.popup.keys) return S.popup.keys(e) !== false;
  return true;
}

// linkOpener points the popup's opener at its list while it is open; closing puts back what it
// pointed at, so a row does not keep pointing at the next popup's list.
function linkOpener(p, id) {
  const o = p.opener;
  if (!o || o === document.body) return;
  const was = o.getAttribute("aria-controls"), prev = p.onclose;
  o.setAttribute("aria-controls", id);
  p.onclose = () => { if (was == null) o.removeAttribute("aria-controls"); else o.setAttribute("aria-controls", was); prev?.(); };
}

// optionList: a list of options moved with the arrows (both pairs, 16 §3.3).
function optionList(pop, p, opts, render, pick, input) {
  const list = el("div", "opts");
  list.id = "popup-options";
  list.setAttribute("role", "listbox");
  const title = pop.querySelector(".ttl");
  if (title) { title.id = "popup-options-title"; list.setAttribute("aria-labelledby", title.id); }
  const owner = input || list;
  if (input) {
    input.setAttribute("role", "combobox");
    input.setAttribute("aria-controls", list.id);
    input.setAttribute("aria-expanded", "true");
    input.setAttribute("aria-autocomplete", "list");
    if (title) input.setAttribute("aria-labelledby", title.id);
  } else list.tabIndex = -1;
  p.restoreFocus = () => { if (p.opener?.isConnected) p.opener.focus({ preventScroll: true, focusVisible: p.openerFocusVisible }); };
  linkOpener(p, list.id);
  pop.append(list);
  p.sel = 0;
  p.opts = opts;
  const draw = () => {
    list.innerHTML = "";
    p.opts.forEach((o, i) => {
      const d = el("div", "opt" + (i === p.sel ? " on" : ""));
      d.id = "popup-option-" + i;
      d.setAttribute("role", "option");
      d.setAttribute("aria-selected", String(i === p.sel));
      d.setAttribute("aria-posinset", String(i + 1));
      d.setAttribute("aria-setsize", String(p.opts.length));
      render(d, o);
      d.onmousemove = () => { if (p.sel !== i) { p.sel = i; draw(); } };
      d.onclick = () => pick(o, i);
      list.append(d);
    });
    const on = list.children[p.sel];
    if (on) owner.setAttribute("aria-activedescendant", on.id); else owner.removeAttribute("aria-activedescendant");
    if (on) on.scrollIntoView({ block: "nearest" });
  };
  p.redraw = (opts) => { if (opts) { p.opts = opts; p.sel = Math.min(p.sel, Math.max(opts.length - 1, 0)); } draw(); };
  p.keys = (e) => {
    if (["ArrowDown", "ArrowRight"].includes(e.key) || (e.key === "Tab" && !p.tab && !e.shiftKey) || (e.ctrlKey && e.key === "n")) {
      p.sel = Math.min(p.sel + 1, p.opts.length - 1); draw(); e.preventDefault(); return true;
    }
    if (["ArrowUp", "ArrowLeft"].includes(e.key) || (e.key === "Tab" && !p.tab && e.shiftKey) || (e.ctrlKey && e.key === "p")) {
      p.sel = Math.max(p.sel - 1, 0); draw(); e.preventDefault(); return true;
    }
    if (e.key === "Tab" && p.tab) { e.preventDefault(); p.tab(); return true; }
    if (e.key === "Enter") { e.preventDefault(); if (p.opts[p.sel]) pick(p.opts[p.sel], p.sel); return true; }
    if (input && document.activeElement !== input) input.focus();
    return false; // typing goes to the input
  };
  draw();
  if (!input) list.focus({ preventScroll: true, focusVisible: false });
}

function inputLine(pop, prompt, placeholder) {
  const row = el("div", "in");
  row.append(el("span", "pr", prompt || "❯"));
  const inp = el("input");
  inp.spellcheck = false;
  inp.placeholder = placeholder || "";
  row.append(inp);
  pop.append(row);
  setTimeout(() => inp.focus(), 0);
  return inp;
}

// ---- the command palette (16 §3.5) ----
export function palette() {
  if (S.popup?.kind === "palette") { close(); return; }
  hideMenu();
  open("palette", T("command palette"), (pop, p) => {
    $("#popup").classList.add("command-dock");
    buildCommandPanel(pop, p, { close, pick: ui.palette, reload: ui.commandDone || ui.reload, uninstall: ui.uninstallStarted });
  });
}

// ---- the tag bar (16 §3.8): pinned tags first, then by use; ✓ all have it, ~ some ----
export async function tags(items, after) {
  const ids = items.filter((x) => x.id).map((x) => x.id);
  if (!ids.length) return;
  let cands;
  try { cands = await api.TagCandidates(ids); } catch (e) { say("fail", errText(e)); return; }
  open("tags", T("tags"), (pop, p) => {
    pop.append(el("div", "ttl", Tf("tags of %s", items.length === 1 ? items[0].name : Tf("%d mods", items.length))));
    const inp = inputLine(pop, "✎", T("type a new tag, or pick one"));
    const chips = el("div", "chips");
    const title = pop.querySelector(".ttl"); title.id = "tag-title";
    chips.id = "tag-options"; chips.setAttribute("role", "listbox"); chips.setAttribute("aria-labelledby", title.id);
    inp.setAttribute("role", "combobox"); inp.setAttribute("aria-controls", chips.id);
    inp.setAttribute("aria-labelledby", title.id); inp.setAttribute("aria-expanded", String(cands.length > 0));
    inp.setAttribute("aria-autocomplete", "list");
    linkOpener(p, chips.id);
    p.restoreFocus = () => { if (p.opener?.isConnected) p.opener.focus({ preventScroll: true, focusVisible: p.openerFocusVisible }); };
    pop.append(chips);
    p.sel = -1;
    const draw = () => {
      chips.innerHTML = "";
      cands.forEach((c, i) => {
        const all = c.have === ids.length, some = c.have > 0 && !all;
        const b = el("button", "c" + (i === p.sel ? " on" : ""));
        b.id = "tag-option-" + i; b.setAttribute("role", "option");
        b.setAttribute("aria-selected", String(i === p.sel)); b.setAttribute("aria-checked", some ? "mixed" : String(all));
        b.setAttribute("aria-posinset", String(i + 1)); b.setAttribute("aria-setsize", String(cands.length));
        if (all || some) { const mark = el("span", "h", all ? "✓" : "~"); mark.setAttribute("aria-hidden", "true"); b.append(mark); }
        b.append(c.tag);
        b.dataset.envWrite = "tags";
        b.title = all ? T("enter removes it") : T("enter gives it to all");
        b.onclick = () => apply(c.tag, all);
        chips.append(b);
      });
      if (p.sel >= 0) inp.setAttribute("aria-activedescendant", "tag-option-" + p.sel); else inp.removeAttribute("aria-activedescendant");
    };
    const apply = async (tag, remove) => {
      close();
      try { say("ok", await api.SetTag(ids, tag, remove)); } catch (e) { say("fail", errText(e)); }
      ui.reload();
      if (after) after();
    };
    p.keys = (e) => {
      if (["ArrowRight", "ArrowDown"].includes(e.key)) { p.sel = Math.min(p.sel + 1, cands.length - 1); draw(); e.preventDefault(); return true; }
      if (["ArrowLeft", "ArrowUp"].includes(e.key)) { p.sel = Math.max(p.sel - 1, -1); draw(); if (p.sel < 0) inp.focus(); e.preventDefault(); return true; }
      if (e.key === "Enter") {
        e.preventDefault();
        if (p.sel >= 0) { const c = cands[p.sel]; apply(c.tag, c.have === ids.length); } else if (inp.value.trim()) apply(inp.value.trim(), false);
        return true;
      }
      if (p.sel >= 0 && e.key.length === 1) { p.sel = -1; draw(); inp.focus(); }
      return false;
    };
    draw();
  });
}

// ---- the position input (16 §3.6) ----
export function position(obj, from, done = null) {
  open("position", T("move to"), (pop, p) => {
    pop.append(el("div", "ttl", Tf("move %s to", obj.what)));
    const inp = inputLine(pop, "❯", T("a position, or a mod's name (tab: before / after)"));
    let before = false;
    const pick = (o) => { close(); done ? done(o.at) : ui.place(from, obj, o.at); };
    optionList(pop, p, [], (d, o) => {
      d.append(el("span", "", o.label));
      if (o.note) d.append(el("span", "d", o.note));
    }, pick, inp);
    const refresh = async () => {
      try { p.redraw(await api.PosOptions(obj.ids, inp.value, before)); } catch (e) { say("fail", errText(e)); }
    };
    p.tab = () => { before = !before; refresh(); };
    inp.addEventListener("input", () => { p.sel = 0; refresh(); });
    refresh();
  });
}

// ---- the tag a tag is merged into (16 §2.9): one of tags ({tag, n}), found by typing, or a new name ----
export function tagTarget(title, tags, pick) {
  open("tag-target", title, (pop, p) => {
    pop.append(el("div", "ttl", title));
    const inp = inputLine(pop, "❯", T("find a tag, or type a new name"));
    optionList(pop, p, [], (d, o) => {
      d.append(el("span", "", o.label));
      if (o.note) d.append(el("span", "d", o.note));
    }, (o) => { close(); pick(o.tag); }, inp);
    const fill = () => {
      const q = inp.value.trim(), ql = q.toLowerCase();
      const opts = tags.filter((t) => !ql || t.tag.toLowerCase().includes(ql)).map((t) => ({ tag: t.tag, label: t.tag, note: Tf("%d mods", t.n) }));
      if (q && !tags.some((t) => t.tag.toLowerCase() === ql)) opts.push({ tag: q, label: Tf("new tag %s", q), note: "" });
      p.sel = 0;
      p.redraw(opts);
    };
    inp.addEventListener("input", fill);
    fill();
  });
}

// ---- a confirmation, for what u cannot undo (16 §6); text() shows lines with none ----
// previewOk: the items of list (refs or ids, in the order the preview was asked for) that
// can go ahead; a preview leaves out the ones that cannot now (p.ok unset: all of them).
export function previewOk(p, list) {
  return Array.isArray(p && p.ok) ? list.filter((_, i) => p.ok.includes(i)) : list;
}

export function confirm(title, lines, goLabel, go, envWrite = "apply") {
  const pre = el("pre", "lines");
  pathContent(pre, (lines || []).join("\n"));
  panel(title, pre, goLabel, go, "", envWrite);
}

// Show the confirmation before starting disk/network checks. A closed or superseded
// dialog never receives a late preview, and Enter cannot bypass a pending check.
export async function libraryConfirm(kind, refs, label, done) {
  const body = el("div");
  const lines = el("pre", "lines", T("checking the selected mods…"));
  body.append(lines);
  const fast = el("input");
  fast.type = "checkbox";
  if (kind === "subscription-remove") {
    const choice = el("label");
    choice.append(fast, document.createTextNode(" " + T("quick delete (cannot be undone)")));
    body.append(choice, el("p", "muted", T("Quick delete skips the recycle bin. Deleted files cannot be restored.")));
  }
  let ready = false, go = refs;
  const apply = () => {
    if (!ready) return;
    if (fast.checked) {
      confirm(T("permanently delete these mods?"), [T("Quick delete skips the recycle bin. Deleted files cannot be restored."), ...go], T("permanently delete"), () => done(api.LibraryDo("subscription-remove-fast", go)));
    } else done(api.LibraryDo(kind, go));
  };
  panel(label, body, label, apply, "", "library");
  const dialog = S.popup;
  const yes = $("#popup .btns .go");
  yes.disabled = true;
  try {
    const preview = await api.LibraryPreview(kind, refs);
    if (S.popup !== dialog) return;
    pathContent(lines, (preview.lines || []).join("\n"));
    ready = !!preview.apply;
    go = previewOk(preview, refs);
    yes.disabled = !ready;
  } catch (e) {
    if (S.popup === dialog) lines.textContent = errText(e);
  }
}

// panel is a confirmation whose body is an element (the pack build's steps); cls widens it.
export function panel(title, content, goLabel, go, cls = "", envWrite = "") {
  open("confirm", title, (pop, p) => {
    p.modal = !!go;
    if (cls) pop.classList.add(cls);
    pop.append(el("div", "ttl", title));
    const body = el("div", "body");
    body.append(content);
    pop.append(body);
    const btns = el("div", "btns");
    const no = el("button", "", go ? T("cancel") : T("close"));
    no.append(keyCaps("esc"));
    no.onclick = close;
    btns.append(no);
    if (go) {
      const yes = el("button", "go", goLabel);
      yes.append(keyCaps("enter"));
      if (envWrite) yes.dataset.envWrite = envWrite;
      yes.onclick = () => { close(); go(); };
      btns.append(yes);
    }
    pop.append(btns);
    p.keys = (e) => {
      // Enter on a focused button presses it (the modal handler); anywhere else it confirms.
      if (e.key === "Enter" && go) { e.preventDefault(); pop.querySelector(".btns .go").click(); }
      else if (e.key === "PageDown") body.scrollTop += body.clientHeight * 0.9;
      else if (e.key === "PageUp") body.scrollTop -= body.clientHeight * 0.9;
      else if (e.key === "ArrowDown") body.scrollTop += 40;
      else if (e.key === "ArrowUp") body.scrollTop -= 40;
      return true;
    };
  });
}

// text shows lines (an inspection, a list, help).
export function text(title, lines) { confirm(title, lines, "", null); }

// The player opens this comparison and can answer here; an answer replaces it
// with the existing confirmation, rather than nesting another popup.
export async function identity(question, run) {
  let pop, state;
  open("identity", T("why this question?"), (node, p) => {
    pop = node; state = p;
    pop.append(el("div", "body", T("loading…")));
  });
  try {
    const data = await api.IdentityWhy(question);
    if (S.popup !== state) return;
    pop.classList.add("identity-comparison");
    pop.innerHTML = "";
    const body = el("div", "body");
    body.append(el("p", "identity-headline", data.headline));
    const table = el("table", "identity-table");
    const head = el("thead"), headings = el("tr");
    for (const text of ["", T("newly found"), T("previously recorded")]) headings.append(el("th", "", text));
    head.append(headings); table.append(head);
    const rows = el("tbody");
    for (const row of data.rows || []) {
      const tr = el("tr", row.match ? "match" : "different");
      const label = el("th", "", row.label);
      const mark = el("span", "identity-mark", row.match ? "✓" : "≠");
      mark.title = T(row.match ? "matching values" : "different values");
      label.prepend(mark); tr.append(label);
      for (const [value, path] of [[row.new, row.newPath], [row.previous, row.previousPath]]) {
        const td = el("td"), text = el("span", "", value);
        if (path) copyablePath(text, path);
        td.append(text); tr.append(td);
      }
      rows.append(tr);
    }
    table.append(rows); body.append(table);
    if (data.files?.length) {
      body.append(el("div", "identity-files-label", T("matching files")));
      const files = el("ul", "identity-files");
      for (const file of data.files) files.append(el("li", "", file));
      if (data.more) files.append(el("li", "muted", Tf("and %d more files", data.more)));
      body.append(files);
    }
    pop.append(body);
    const btns = el("div", "btns");
    const leave = el("button", "identity-close", T("close"));
    leave.append(keyCaps("esc"));
    leave.onclick = close; btns.append(leave);
    const answers = el("div", "identity-answer-buttons");
    const pick = (action) => { close(); run(action); };
    for (const action of data.answers || []) {
      const b = el("button", "", action.label);
      b.title = action.title;
      b.dataset.envWrite = action.kind;
      b.onclick = () => pick(action);
      answers.append(b);
    }
    btns.append(answers); pop.append(btns);
    state.keys = (e) => {
      if (!e.ctrlKey && !e.altKey && !e.metaKey) {
        const action = (data.answers || []).find((a) => a.key === e.key);
        if (action) { e.preventDefault(); pick(action); return true; }
      }
      if (e.key === "PageDown") body.scrollTop += body.clientHeight * 0.9;
      if (e.key === "PageUp") body.scrollTop -= body.clientHeight * 0.9;
      return true;
    };
    leave.focus();
  } catch (e) {
    if (S.popup === state) { close(); say("fail", errText(e)); }
  }
}

// choose offers a few choices; pick gets the chosen index. other is a second action on
// the highlighted choice ({key, label, run}), shown under the list (the environment
// chooser's remove game).
export function choose(title, labels, pick, current = 0, other = null) {
  open("choose", title, (pop, p) => {
    pop.append(el("div", "ttl", title));
    optionList(pop, p, labels.map((l, i) => ({ l, i })), (d, o) => d.append(el("span", "word-facts", o.l)), (o) => { close(); pick(o.i); });
    if (other) {
      const btns = el("div", "btns");
      const b = el("button", "", other.label);
      b.append(keyCaps(other.key === "Delete" ? "del" : other.key));
      if (other.envWrite) b.dataset.envWrite = other.envWrite;
      const run = () => { const i = p.opts[p.sel] && p.opts[p.sel].i; close(); if (i !== undefined) other.run(i); };
      b.onclick = run;
      btns.append(b);
      pop.append(btns);
      const keys = p.keys;
      p.keys = (e) => {
        if (e.key === other.key) { e.preventDefault(); b.click(); return true; }
        return keys(e);
      };
    }
    p.sel = current;
    p.redraw();
  });
}

// ask reads one line (the archo command line).
export function ask(title, placeholder, done, value = null) {
  open("ask", title, (pop, p) => {
    pop.append(el("div", "ttl", title));
    const inp = inputLine(pop, "❯", placeholder);
    // value: an edit (a note): it starts from the text, and an empty line is an answer too
    if (value != null) { inp.value = value; inp.setSelectionRange(value.length, value.length); }
    p.keys = (e) => {
      if (e.key === "Enter") { e.preventDefault(); const v = inp.value.trim(); close(); if (v || value != null) done(v); return true; }
      return false;
    };
  });
}

// help: the keys of the page it is asked on (16 §3.1 rule 4, view.KeyHelp): the plan
// editor and Mod library share one table, then each page's own.
export async function help(page = "plan") {
  let rows = [];
  try { rows = await api.Keys(page === "full" ? "plan" : page); } catch { rows = []; }
  open("help", T("all keys"), (pop) => {
    pop.append(el("div", "ttl", T("the arrows only move; keys that change things act at once, u undoes")));
    const body = el("div", "body help");
    for (const r of rows) body.append(el("b", "word-facts", r.keys), el("span", "word-facts", r.what));
    pop.append(body);
  });
}

// ---- the context menu (16 §8.5): the same actions as the hint line ----
// An item {label, key, run, cls, checked?, disabled?, current?}; {sep: true} draws a hairline between groups. The keyboard
// works it as the mouse does: ↑↓ (or tab) move, enter runs, esc closes; it opens with the
// first item focused when the menu key or shift+F10 opened it (keys = true).
// Link the visible opener before opening too, so agents can discover its menu.
export function menuOpener(node) {
  node.setAttribute("aria-haspopup", "menu");
  node.setAttribute("aria-expanded", "false");
  node.setAttribute("aria-controls", "menu");
  return node;
}
let menuFocus = null;
// back: where esc returns, an element or a function (the opener by default).
export function menu(x, y, actions, keys = false, { name, opener = document.activeElement, back = opener } = {}) {
  closeChoiceMenu();
  hideMenu();
  const m = $("#menu");
  m.innerHTML = "";
  m.tabIndex = -1;
  m.setAttribute("role", "menu");
  m.setAttribute("aria-label", name || "");
  for (const a of actions) {
    if (a.sep) { if (m.lastChild && m.lastChild.tagName !== "HR") { const hr = el("hr"); hr.setAttribute("role", "separator"); m.append(hr); } continue; }
    const b = el("button", a.cls || "");
    b.setAttribute("role", a.checked == null ? "menuitem" : "menuitemcheckbox");
    if (a.checked != null) b.setAttribute("aria-checked", String(a.checked));
    if (a.current) b.setAttribute("aria-current", "page");
    b.disabled = !!a.disabled;
    b.append(el("span", "", a.label));
    if (a.key) {
      const key = el("span", "key", a.key);
      if (a.checked != null) key.setAttribute("aria-hidden", "true");
      b.append(key);
    }
    if (a.envWrite) b.dataset.envWrite = a.envWrite;
    b.onclick = () => { hideMenu(true); a.run(); };
    m.append(b);
  }
  if (m.lastChild && m.lastChild.tagName === "HR") m.lastChild.remove();
  if (!m.childNodes.length) return;
  m.inert = false;
  m.hidden = false;
  const r = m.getBoundingClientRect();
  m.style.left = Math.max(6, Math.min(x, innerWidth - r.width - 6)) + "px";
  m.style.top = Math.max(6, Math.min(y, innerHeight - r.height - 6)) + "px";
  menuFocus = { opener, back, visible: opener?.matches?.(":focus-visible") };
  if (opener) { menuOpener(opener); opener.setAttribute("aria-expanded", "true"); }
  m.onkeydown = (e) => {
    const bs = [...m.querySelectorAll("button:not(:disabled)")];
    const i = bs.indexOf(document.activeElement);
    if (e.key === "ArrowDown" || e.key === "ArrowUp" || e.key === "Tab" || e.key === "Home" || e.key === "End") {
      e.preventDefault();
      const d = e.key === "ArrowUp" || (e.key === "Tab" && e.shiftKey) ? -1 : 1;
      const j = e.key === "Home" ? 0 : e.key === "End" ? bs.length - 1 : (i + d + bs.length) % bs.length;
      bs[j]?.focus();
    } else if (e.key === "Escape" || e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey)) {
      e.preventDefault();
      hideMenu(true);
    } else if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      if (i >= 0) bs[i].click();
    } else {
      // a key the menu shows runs its item
      const hit = actions.find((a) => !a.sep && a.key && a.key.split(" / ").some((k) => k.toLowerCase() === e.key.toLowerCase()));
      if (hit && !hit.disabled) { e.preventDefault(); m.querySelectorAll("button")[actions.filter((a) => !a.sep).indexOf(hit)].click(); }
    }
    e.stopPropagation();
  };
  if (keys) {
    const first = m.querySelector("button:not(:disabled)");
    if (first) first.focus(); else m.focus({ focusVisible: false });
  }
  setTimeout(() => document.addEventListener("mousedown", hideMenuOutside, { once: true }), 0);
}
function hideMenuOutside(e) { if (!$("#menu").contains(e.target)) hideMenu(); else document.addEventListener("mousedown", hideMenuOutside, { once: true }); }
export function hideMenu(restore = false) {
  const m = $("#menu"), focus = menuFocus;
  m.inert = true; m.hidden = true;
  menuFocus = null;
  focus?.opener?.setAttribute("aria-expanded", "false");
  if (!restore || !focus?.back) return;
  if (typeof focus.back === "function") focus.back();
  else if (focus.back.isConnected) focus.back.focus({ preventScroll: true, focusVisible: focus.visible });
}

// A player-requested temporary decision panel (16 §3.1 rule 7).
export async function problem(ref, run, actionWrite) {
  open("problem", T("see the problem and fixes"), (box) => {
    box.append(el("div", "ttl", T("checking the problem…")));
  });
  let request = S.popup;
  const env = S.envSeq;
  const current = () => S.popup === request && S.envSeq === env;
  const draw = (detail) => {
    open("problem", detail.title, (box, p) => {
      box.classList.add("problem-panel");
      box.append(el("div", "ttl", detail.title));
      const body = el("div", "body");
      if (detail.meaning) body.append(el("p", "", detail.meaning));
      for (const note of detail.notes || []) body.append(el("p", "", note));
      if (detail.searching) body.append(el("p", "muted", T("searching installed mods…")));
      for (const [label, lines] of [[T("affected content"), detail.affected], [T("problem evidence"), detail.evidence]]) {
        if (!lines?.length) continue;
        body.append(el("h3", "", label));
        const list = el("ul");
        lines.forEach((line, i) => {
          const li = el("li", "", line);
          if (lines === detail.affected && detail.locations?.[i]) li.append(el("span", "muted", "  " + detail.locations[i]));
          list.append(li);
        });
        body.append(list);
      }
      box.append(body);
      const btns = el("div", "btns");
      for (const [i, act] of (detail.actions || []).entries()) {
        const b = el("button", detail.primary && i === 0 ? "go" : "", act.label);
        const write = actionWrite(act);
        if (write) b.dataset.envWrite = write;
        b.onclick = () => { close(); run(act); };
        btns.append(b);
      }
      const back = el("button", "", T("close"));
      back.append(keyCaps("esc"));
      back.onclick = close;
      btns.append(back);
      box.append(btns);
      const buttons = [...btns.querySelectorAll("button")];
      buttons[0]?.focus();
      p.keys = (e) => {
        if (["Tab", "ArrowLeft", "ArrowRight"].includes(e.key)) {
          e.preventDefault();
          const i = buttons.indexOf(document.activeElement);
          const step = e.shiftKey || e.key === "ArrowLeft" ? -1 : 1;
          buttons[(i + step + buttons.length) % buttons.length].focus();
        } else if (e.key === "Enter" || e.key === " ") {
          if (buttons.includes(document.activeElement)) { e.preventDefault(); document.activeElement.click(); }
        } else if (e.key === "ArrowDown") body.scrollTop += 40;
        else if (e.key === "ArrowUp") body.scrollTop -= 40;
        else if (e.key === "PageDown") body.scrollTop += body.clientHeight;
        else if (e.key === "PageUp") body.scrollTop -= body.clientHeight;
        return true;
      };
    });
    request = S.popup;
  };
  try {
    const detail = await api.ProblemDetail(ref);
    if (!current()) return;
    draw(detail);
    if (!detail.searching) return;
    const lookup = await api.BeginProblemProviders();
    const cancel = () => { api.CancelProblemProviders(lookup).catch(() => {}); };
    if (!current()) { cancel(); return; }
    request.onclose = cancel;
    const updated = await api.ProblemProviders(ref, lookup);
    if (!current()) { cancel(); return; }
    request.onclose = null;
    cancel();
    draw(updated);
  } catch (e) {
    if (current()) { close(); say("fail", errText(e)); }
  }
}

// B1: separate needed/available values; Enter follows focus, initially Cancel.
export function memoryConfirm(memory, go) {
  open("memory", T("RAM may be insufficient"), (pop, p) => {
    pop.classList.add("memory-confirm");
    pop.append(el("div", "ttl", T("RAM may be insufficient")));
    const values = el("div", "memory-values");
    for (const [label, value] of [["loading RAM needed", memory.needed], ["RAM available now", memory.available]]) {
      const cell = el("div");
      cell.append(el("div", "muted", T(label)), el("strong", "num", value));
      values.append(cell);
    }
    const body = el("div", "body");
    body.append(values, el("p", "", T("close other apps or reduce the list before launching.")));
    pop.append(body);
    const buttons = el("div", "btns");
    const cancel = el("button", "memory-cancel", T("cancel"));
    const yes = el("button", "go", T("launch anyway"));
    yes.dataset.envWrite = "launch";
    cancel.onclick = close;
    let accepted = false;
    yes.onclick = () => { if (accepted || S.popup !== p) return; accepted = true; close(); go(); };
    buttons.append(cancel, yes); pop.append(buttons);
    cancel.focus();
    p.keys = e => {
      if (["Tab", "ArrowLeft", "ArrowRight"].includes(e.key)) {
        e.preventDefault();
        (document.activeElement === cancel ? yes : cancel).focus();
      } else if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        (document.activeElement === yes ? yes : cancel).click();
      }
      return true;
    };
  });
}
