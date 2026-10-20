// Popups (06 §13.2 rule 5): the command palette, the tag bar, the position input, a
// confirmation, a text or choice panel, help. Each has a title, shows in the position
// line, closes with esc, and never nests: opening one closes the other.
import { S, $, el, esc, api, T, Tf, errText, say } from "./state.js";

let ui = null;
export function initPopups(u) { ui = u; }

function open(kind, title, build) {
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

// optionList: a list of options moved with the arrows (both pairs, 06 §13.4).
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

// ---- the command palette (06 §8) ----
export function palette() {
  open("palette", T("command palette"), (pop, p) => {
    const inp = inputLine(pop, "❯", T("a command, a mod, an environment"));
    const run = (o) => { close(); ui.palette(o); };
    optionList(pop, p, [], (d, o) => {
      d.append(el("span", "", o.name));
      if (o.kind === 0 && o.alias !== o.name) d.append(el("span", "dim", o.alias));
      d.append(el("span", "d", o.kind === 0 ? o.desc.split(" ")[0] : o.kind === 2 ? T("environment") : o.desc));
    }, run, inp);
    const refresh = async () => p.redraw(await api.Palette(inp.value));
    inp.addEventListener("input", refresh);
    inp.addEventListener("keydown", (e) => { if (e.ctrlKey && e.key === "u") { inp.value = ""; refresh(); e.preventDefault(); } });
    refresh();
  });
}

// ---- the tag bar (06 §12.3): pinned tags first, then by use; ✓ all have it, ~ some ----
export async function tags(items) {
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
        b.title = all ? T("enter removes it") : T("enter gives it to all");
        b.onclick = () => apply(c.tag, all);
        chips.append(b);
      });
    };
    const apply = async (tag, remove) => {
      close();
      try { say("ok", await api.SetTag(ids, tag, remove)); } catch (e) { say("fail", errText(e)); }
      ui.reload();
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

// ---- the position input (06 §13.7) ----
export function position(obj, from) {
  open("position", T("move to"), (pop, p) => {
    pop.append(el("div", "ttl", Tf("move %s to", obj.what)));
    const inp = inputLine(pop, "❯", T("a position, or a mod's name (tab: before / after)"));
    let before = false;
    const pick = (o) => { close(); ui.place(from, obj, o.at); };
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

// ---- a preview with a confirmation (sort, save, launch; 06 §9) ----
export function confirm(title, lines, goLabel, go) {
  open("confirm", title, (pop, p) => {
    pop.append(el("div", "ttl", title));
    const body = el("div", "body");
    const pre = el("pre", "lines");
    pre.textContent = (lines || []).join("\n");
    body.append(pre);
    pop.append(body);
    const btns = el("div", "btns");
    const no = el("button", "", T("cancel") + "  esc");
    no.onclick = close;
    btns.append(no);
    if (go) {
      const yes = el("button", "go", goLabel + "  enter");
      yes.onclick = () => { close(); go(); };
      btns.append(yes);
    }
    pop.append(btns);
    p.keys = (e) => {
      if (e.key === "Enter" && go) { e.preventDefault(); close(); go(); }
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

// choose offers a few choices; pick gets the chosen index.
export function choose(title, labels, pick, current = 0) {
  open("choose", title, (pop, p) => {
    pop.append(el("div", "ttl", title));
    optionList(pop, p, labels.map((l, i) => ({ l, i })), (d, o) => d.append(o.l), (o) => { close(); pick(o.i); });
    p.sel = current;
    p.redraw();
  });
}

// ask reads one line (the archo command line).
export function ask(title, placeholder, done) {
  open("ask", title, (pop, p) => {
    pop.append(el("div", "ttl", title));
    const inp = inputLine(pop, "❯", placeholder);
    p.keys = (e) => {
      if (e.key === "Enter") { e.preventDefault(); const v = inp.value.trim(); close(); if (v) done(v); return true; }
      return false;
    };
  });
}

// help: every key (06 §13.2 rule 4).
export function help() {
  open("help", T("all keys"), (pop) => {
    pop.append(el("div", "ttl", T("the arrows only move; keys that change things act at once, u undoes")));
    const body = el("div", "body help");
    const rows = [
      ["↑ ↓", T("move the cursor (scroll in the details)")], ["← → · 1–4", T("go to an area")],
      ["PgUp PgDn", T("scroll the details from anywhere")], ["shift+↑↓ · shift+click", T("select a range")],
      ["ctrl+space · ctrl+click", T("select or unselect one")], ["esc", T("clear the selection, close a popup")],
      ["space · double-click", T("enable / disable / put back")], ["t", T("stage; in staging, put back")],
      ["m", T("move to a position or after a mod")], ["J K · alt+J K", T("move down / up 1 (10)")],
      ["g · #", T("tags")], ["enter", T("full details")], ["o", T("Workshop page")],
      ["S · w · L", T("sort · save · launch (preview first)")], ["u · ctrl+z", T("undo")], ["ctrl+r · ctrl+y", T("redo")],
      ["/ · ctrl+f", T("search the list")], ["shift+1–5", T("go to a page (the tabs at the top)")], ["c", T("recent changes")],
      ["R · A", T("rescan · check the files again")], ["E", T("switch environment")], ["ctrl+k", T("command palette")],
      [T("drag"), T("between lists: enable, disable, stage; within Enabled: reorder")], [T("right click"), T("what you can do with it")],
    ];
    for (const [k, v] of rows) body.append(el("b", "", k), el("span", "", v));
    pop.append(body);
  });
}

// ---- the context menu (08 §6): the same actions as the hint line ----
export function menu(x, y, actions) {
  const m = $("#menu");
  m.innerHTML = "";
  for (const a of actions) {
    const b = el("button");
    b.append(el("span", "", a.label));
    if (a.key) b.append(el("span", "key", a.key));
    b.onclick = () => { hideMenu(); a.run(); };
    m.append(b);
  }
  m.hidden = false;
  const r = m.getBoundingClientRect();
  m.style.left = Math.min(x, innerWidth - r.width - 6) + "px";
  m.style.top = Math.min(y, innerHeight - r.height - 6) + "px";
  setTimeout(() => document.addEventListener("mousedown", hideMenuOutside, { once: true }), 0);
}
function hideMenuOutside(e) { if (!$("#menu").contains(e.target)) hideMenu(); else document.addEventListener("mousedown", hideMenuOutside, { once: true }); }
export function hideMenu() { $("#menu").hidden = true; }
