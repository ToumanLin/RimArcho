// Popups (06 §3.1 rule 5): the command palette, the tag bar, the position input, a
// confirmation, a text or choice panel, help. Each has a title, shows in the position
// line, closes with esc, and never nests: opening one closes the other.
import { pathContent } from "./fields.js";
import { S, $, el, esc, api, T, Tf, errText, say } from "./state.js";

let ui = null;
export function initPopups(u) { ui = u; }

export function open(kind, title, build) {
  close();
  const box = $("#popup");
  box.innerHTML = "";
  const pop = el("div", "pop");
  box.append(pop);
  box.hidden = false;
  box.onmousedown = (e) => { if (e.target === box) close(); };
  S.popup = { kind, title, keys: null };
  build(pop, S.popup);
  ui.renderWhere();
}

export function close() {
  if (!S.popup) return;
  S.popup = null;
  const box = $("#popup");
  box.hidden = true;
  box.innerHTML = "";
  ui.renderWhere();
  ui.renderHeads();
}

// popupKey handles a key while a popup is open; true when it took it.
export function popupKey(e) {
  if (!S.popup) return false;
  if (e.key === "Escape") { close(); return true; }
  if (S.popup.keys) return S.popup.keys(e) !== false;
  return true;
}

// optionList: a list of options moved with the arrows (both pairs, 06 §3.3).
function optionList(pop, p, opts, render, pick, input) {
  const list = el("div", "opts");
  pop.append(list);
  p.sel = 0;
  p.opts = opts;
  const draw = () => {
    list.innerHTML = "";
    p.opts.forEach((o, i) => {
      const d = el("div", "opt" + (i === p.sel ? " on" : ""));
      render(d, o);
      d.onmousemove = () => { if (p.sel !== i) { p.sel = i; draw(); } };
      d.onclick = () => pick(o, i);
      list.append(d);
    });
    const on = list.children[p.sel];
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

// ---- the command palette (06 §3.5) ----
export function palette() {
  open("palette", T("command palette"), (pop, p) => {
    const inp = inputLine(pop, "❯", T("a command, a mod, a game"));
    const run = (o) => { close(); ui.palette(o); };
    optionList(pop, p, [], (d, o) => {
      d.append(el("span", "", o.name));
      if (o.kind === 0 && o.alias !== o.name) d.append(el("span", "dim", o.alias));
      d.append(el("span", "d", o.kind === 0 ? o.desc.split(" ")[0] : o.kind === 2 ? T("game") : o.kind === 3 ? T("list") : o.desc));
    }, run, inp);
    const refresh = async () => p.redraw(await api.Palette(inp.value));
    inp.addEventListener("input", refresh);
    inp.addEventListener("keydown", (e) => { if (e.ctrlKey && e.key === "u") { inp.value = ""; refresh(); e.preventDefault(); } });
    refresh();
  });
}

// ---- the tag bar (06 §3.8): pinned tags first, then by use; ✓ all have it, ~ some ----
export async function tags(items, after) {
  const ids = items.filter((x) => x.id).map((x) => x.id);
  if (!ids.length) return;
  let cands;
  try { cands = await api.TagCandidates(ids); } catch (e) { say("fail", errText(e)); return; }
  open("tags", T("tags"), (pop, p) => {
    pop.append(el("div", "ttl", Tf("tags of %s", items.length === 1 ? items[0].name : Tf("%d mods", items.length))));
    const inp = inputLine(pop, "✎", T("type a new tag, or pick one"));
    const chips = el("div", "chips");
    pop.append(chips);
    p.sel = -1;
    const draw = () => {
      chips.innerHTML = "";
      cands.forEach((c, i) => {
        const all = c.have === ids.length, some = c.have > 0 && !all;
        const b = el("button", "c" + (i === p.sel ? " on" : ""));
        if (all || some) b.append(el("span", "h", all ? "✓" : "~"));
        b.append(c.tag);
        b.dataset.envWrite = "tags";
        b.title = all ? T("enter removes it") : T("enter gives it to all");
        b.onclick = () => apply(c.tag, all);
        chips.append(b);
      });
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

// ---- the position input (06 §3.6) ----
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

// ---- a confirmation, for what u cannot undo (06 §6); text() shows lines with none ----
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
    if (cls) pop.classList.add(cls);
    pop.append(el("div", "ttl", title));
    const body = el("div", "body");
    body.append(content);
    pop.append(body);
    const btns = el("div", "btns");
    const no = el("button", "", (go ? T("cancel") : T("close")) + "  esc");
    no.onclick = close;
    btns.append(no);
    if (go) {
      const yes = el("button", "go", goLabel + "  enter");
      if (envWrite) yes.dataset.envWrite = envWrite;
      yes.onclick = () => { close(); go(); };
      btns.append(yes);
    }
    pop.append(btns);
    p.keys = (e) => {
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

// choose offers a few choices; pick gets the chosen index. other is a second action on
// the highlighted choice ({key, label, run}), shown under the list (the environment
// chooser's 删除环境).
export function choose(title, labels, pick, current = 0, other = null) {
  open("choose", title, (pop, p) => {
    pop.append(el("div", "ttl", title));
    optionList(pop, p, labels.map((l, i) => ({ l, i })), (d, o) => d.append(el("span", "word-facts", o.l)), (o) => { close(); pick(o.i); });
    if (other) {
      const btns = el("div", "btns");
      const b = el("button", "word-facts", other.label + "  " + (other.key === "Delete" ? "del" : other.key));
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

// help: the keys of the page it is asked on (06 §3.1 rule 4, view.KeyHelp): the plan
// editor and Mod 库存 share one table, then each page's own.
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

// ---- the context menu (06 §8.5): the same actions as the hint line ----
// An item {label, key, run, cls}; {sep: true} draws a hairline between groups. The keyboard
// works it as the mouse does: ↑↓ (or tab) move, enter runs, esc closes; it opens with the
// first item focused when the menu key or shift+F10 opened it (keys = true).
export function menu(x, y, actions, keys = false) {
  const m = $("#menu");
  m.innerHTML = "";
  for (const a of actions) {
    if (a.sep) { if (m.lastChild && m.lastChild.tagName !== "HR") m.append(el("hr")); continue; }
    const b = el("button", a.cls || "");
    b.append(el("span", "", a.label));
    if (a.key) b.append(el("span", "key", a.key));
    if (a.envWrite) b.dataset.envWrite = a.envWrite;
    b.onclick = () => { hideMenu(); a.run(); };
    m.append(b);
  }
  if (m.lastChild && m.lastChild.tagName === "HR") m.lastChild.remove();
  if (!m.childNodes.length) return;
  m.inert = false;
  m.hidden = false;
  const r = m.getBoundingClientRect();
  m.style.left = Math.max(6, Math.min(x, innerWidth - r.width - 6)) + "px";
  m.style.top = Math.max(6, Math.min(y, innerHeight - r.height - 6)) + "px";
  const back = document.activeElement;
  m.onkeydown = (e) => {
    const bs = [...m.querySelectorAll("button")];
    const i = bs.indexOf(document.activeElement);
    if (e.key === "ArrowDown" || e.key === "ArrowUp" || e.key === "Tab" || e.key === "Home" || e.key === "End") {
      e.preventDefault();
      const d = e.key === "ArrowUp" || (e.key === "Tab" && e.shiftKey) ? -1 : 1;
      const j = e.key === "Home" ? 0 : e.key === "End" ? bs.length - 1 : (i + d + bs.length) % bs.length;
      bs[j].focus();
    } else if (e.key === "Escape" || e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey)) {
      e.preventDefault();
      hideMenu();
      if (back && back.focus) back.focus();
    } else if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      if (i >= 0) bs[i].click();
    } else {
      // a key the menu shows runs its item
      const hit = actions.find((a) => !a.sep && a.key && a.key.split(" / ").some((k) => k.toLowerCase() === e.key.toLowerCase()));
      if (hit) { e.preventDefault(); bs[actions.filter((a) => !a.sep).indexOf(hit)].click(); }
    }
    e.stopPropagation();
  };
  if (keys) m.querySelector("button").focus();
  setTimeout(() => document.addEventListener("mousedown", hideMenuOutside, { once: true }), 0);
}
function hideMenuOutside(e) { if (!$("#menu").contains(e.target)) hideMenu(); else document.addEventListener("mousedown", hideMenuOutside, { once: true }); }
export function hideMenu() { const m = $("#menu"); m.inert = true; m.hidden = true; }
