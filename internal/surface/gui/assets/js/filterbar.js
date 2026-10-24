// The filter bar (06 §2.5, §2.9): one component for the plan editor and Mod 库存 › 全部.
// One row: the search box (grey completion, tab or → takes it), the filters in force as
// removable chips, and a "筛选" toggle (also `\`). Open, it shows the groups one line each:
// the values most used first with their counts, then a box to type in for the rest. A left
// click (enter, space) keeps a value, a right click (x) leaves it out, again drops it; the
// game's own version is marked. The keyboard reaches all of it: `\` opens the bar with the
// focus on its first value, ↑↓ go between groups, ←→ between values, a letter types into
// the group's box, esc closes it.
//
// The page gives: data() → { groups, conds, complete } of its last answer; change(search,
// filters) when either changed; and the place it lives in. The bar keeps search, filters
// and open; the page reads them.
import { el, T, Tf, api } from "./state.js";
import { wireSeparator } from "./separator.js";

const LINE = 24; // values of a group before its box (open, the bar has its column's height)

export function createBar(opts) {
  const b = { search: "", filters: [], open: false, opts };
  const root = el("div", "fbar" + (opts.cls ? " " + opts.cls : ""));
  const row = el("div", "fbar-row");
  const box = el("div", "fbar-search");
  const inp = el("input");
  inp.spellcheck = false;
  inp.placeholder = opts.placeholder || T("search");
  const ghost = el("span", "fbar-ghost");
  ghost.append(el("span", "typed"), el("span", "sfx"));
  box.append(inp, ghost);
  const chips = el("div", "fbar-chips");
  const tog = el("button", "fbar-tog");
  tog.title = T("filter") + T("; ") + "\\";
  const panel = el("div", "fbar-panel");
  panel.hidden = true;
  row.append(box, chips, tog);
  // the gap under the open bar: drag it to share the column with what is below (the list,
  // the details); double click, or the bar's first state, gives the bar the whole column
  const gap = el("div", "fbar-gap");
  gap.title = T("drag: share the column, double click: the filter takes it all");
  root.append(row, panel, gap);
  b.el = root;
  b.input = inp;
  const finds = {}; // group key → typed text in its box
  let cur = { g: 0, v: 0 }; // the panel's focused value
  let wantFocus = false;

  const data = () => opts.data() || {};
  const showGhost = () => {
    const sfx = data().complete && inp.value === b.search ? data().complete : "";
    const atEnd = document.activeElement === inp && inp.selectionStart === inp.value.length && inp.selectionEnd === inp.value.length;
    ghost.hidden = !(sfx && atEnd && inp.scrollLeft === 0);
    if (!ghost.hidden) { ghost.firstChild.textContent = inp.value; ghost.lastChild.textContent = sfx; }
  };
  b.showGhost = showGhost;
  let timer = 0;
  inp.oninput = () => { ghost.hidden = true; clearTimeout(timer); timer = setTimeout(() => { b.search = inp.value; opts.change(); }, 90); };
  inp.onblur = inp.onscroll = showGhost;
  inp.onkeydown = (e) => {
    e.stopPropagation();
    const d = data();
    if ((e.key === "Tab" && !e.shiftKey || e.key === "ArrowRight") && !ghost.hidden && d.complete) {
      e.preventDefault();
      inp.value += d.complete;
      b.search = inp.value;
      opts.change();
      return;
    }
    if (e.key === "Tab") return;
    if (e.key === "Escape") {
      e.preventDefault();
      if (inp.value) { inp.value = ""; b.search = ""; opts.change(); }
      inp.blur();
      opts.leave && opts.leave();
      return;
    }
    if (e.key === "Enter" || e.key === "ArrowDown") { e.preventDefault(); inp.blur(); opts.leave && opts.leave(); return; }
    // backspace in an empty box takes the last filter off
    if (e.key === "Backspace" && !inp.value && b.filters.length) { e.preventDefault(); b.filters.pop(); opts.change(); return; }
    if (e.key === "\\") { e.preventDefault(); b.toggle(); return; }
    if (e.key === "ArrowLeft" || e.key === "Home" || e.key === "End") setTimeout(showGhost);
  };
  tog.onclick = () => b.toggle();

  // set: on 1 keeps the value, -1 leaves it out, 0 drops it
  b.set = (f, on) => {
    b.filters = b.filters.filter((x) => x !== f && x !== "-" + f);
    if (on === 1) b.filters.push(f);
    if (on === -1) b.filters.push("-" + f);
    opts.change();
  };
  b.clear = () => { b.filters = []; b.search = ""; inp.value = ""; opts.change(); };
  b.active = () => !!(b.search.trim() || b.filters.length);
  // h: the open bar's height in px; 0 fills the column (what is below shrinks to its title)
  b.h = 0;
  const KEY = opts.splitKey;
  const colH = () => root.isConnected ? (opts.column ? opts.column() : root.parentElement)?.getBoundingClientRect().height || 0 : 0;
  function place() {
    root.classList.toggle("fill", b.open && !b.h);
    if (b.h) root.style.setProperty("--fbar-h", b.h + "px");
    else root.style.removeProperty("--fbar-h");
  }
  // setH keeps at least the bar and a few values, and leaves what is below its title and a
  // few rows; dragged near the bottom it fills the column again
  b.setH = (h, save = true) => {
    const max = colH() - 34;
    b.h = !h || h > max - 40 ? 0 : Math.max(120, Math.round(h));
    place();
    if (save && KEY) api.SetSetting(KEY, b.h ? String(b.h) : "").catch(() => {});
    opts.resized && opts.resized();
    separator.refresh();
  };
  b.nudge = (d) => b.setH((b.h || colH() - 34) + d);
  const separator = wireSeparator(gap, {
    axis: "y", key: KEY, enabled: () => b.open,
    range: () => [120, colH() - 34],
    get: () => b.h || colH() - 34,
    set: (h) => b.setH(h, false), reset: () => b.setH(0, false),
    serialize: () => b.h ? String(b.h) : "",
    restore: (v) => { b.h = +v || 0; place(); },
  });
  b.fills = () => b.open && !b.h;

  b.toggle = (v = !b.open) => {
    b.open = v;
    draw();
    opts.change(true);
    if (v) wantFocus = true; // once the groups are in (draw)
    else if (root.contains(document.activeElement)) { document.activeElement.blur(); opts.leave && opts.leave(); }
  };
  // the open bar folds the moment the focus leaves it: a click elsewhere, tab, a key that
  // moves the focus (06 §3.1 rule 8); what it filters stays in force on its one line
  const left = (t) => b.open && root.isConnected && t instanceof Node && !root.contains(t) && !(t.closest && t.closest("#menu"));
  document.addEventListener("mousedown", (e) => { if (left(e.target)) b.toggle(false); }, true);
  document.addEventListener("focusin", (e) => { if (left(e.target)) b.toggle(false); });
  b.focus = () => { inp.focus(); inp.select(); };
  b.focused = () => root.contains(document.activeElement);

  const lineOf = (g) => {
    const q = (finds[g.key] || "").toLowerCase();
    let vals = (g.values || []).filter((v) => v.count > 0 || v.on);
    if (q) vals = (g.values || []).filter((v) => v.label.toLowerCase().includes(q) || v.filter.toLowerCase().includes(q));
    else vals = vals.slice(0, LINE).concat(vals.slice(LINE).filter((v) => v.on));
    return vals;
  };

  function draw() {
    const d = data();
    // the chips: what is in force, each removable
    chips.innerHTML = "";
    for (const c of d.conds || []) {
      const x = el("button", "fbar-chip" + (c.exclude ? " ex" : ""));
      x.append(el("span", "", c.label), el("span", "x", "×"));
      x.title = c.label + (c.exclude ? T("; ") + T("left out") : "");
      x.onclick = () => b.set(c.filter.replace(/^-/, ""), 0);
      chips.append(x);
    }
    const n = (d.conds || []).length;
    tog.innerHTML = "";
    tog.append(el("span", "", T("filter")));
    if (n) tog.append(el("span", "c", String(n)));
    tog.append(el("kbd", "", "\\"));
    tog.classList.toggle("on", b.open);
    panel.hidden = !b.open;
    place();
    root.classList.toggle("open", b.open);
    if (!b.open) return;
    if (opts.width) root.style.setProperty("--fbar-w", opts.width() + "px");
    const keep = panel.contains(document.activeElement) ? document.activeElement.dataset.at : null;
    panel.innerHTML = "";
    (d.groups || []).forEach((g, gi) => {
      const vals = lineOf(g);
      if (!vals.length && !finds[g.key] && !(g.values || []).length) return;
      const t = el("span", "fbar-gt", g.title);
      const line = el("span", "fbar-line");
      line.dataset.g = gi;
      vals.forEach((v, vi) => {
        const x = el("button", "fbar-v" + (v.on === 1 ? " on" : v.on === -1 ? " ex" : "") + (v.cur ? " cur" : ""));
        x.append(el("span", "l", v.label));
        if (v.cur) x.append(el("span", "curv", T("this game")));
        x.append(el("span", "n", String(v.count)));
        x.title = v.label + "\n" + T("left click: only these, right click or x: leave these out");
        x.dataset.at = gi + ":" + vi;
        x.tabIndex = -1;
        x.onclick = () => b.set(v.filter, v.on === 1 ? 0 : 1);
        x.oncontextmenu = (e) => { e.preventDefault(); b.set(v.filter, v.on === -1 ? 0 : -1); };
        x.onkeydown = (e) => valueKey(e, gi, vi, v);
        line.append(x);
      });
      const more = (g.values || []).filter((v) => v.count > 0 || v.on).length - vals.length;
      const f = el("input", "fbar-find");
      f.placeholder = more > 0 && !finds[g.key] ? Tf("%d more…", more) : T("type to find…");
      f.value = finds[g.key] || "";
      f.spellcheck = false;
      f.dataset.at = gi + ":f";
      f.oninput = () => { finds[g.key] = f.value; draw(); };
      f.onkeydown = (e) => findKey(e, gi, f, vals);
      line.append(f);
      panel.append(t, line);
    });
    if (wantFocus && panel.querySelector(".fbar-v")) { wantFocus = false; cur = { g: 0, v: 0 }; focusValue(); return; }
    if (keep) {
      const n = panel.querySelector(`[data-at="${keep}"]`) || panel.querySelector(`[data-at^="${keep.split(":")[0]}:"]`);
      if (n) { n.focus(); if (n.tagName === "INPUT") n.setSelectionRange(n.value.length, n.value.length); }
    }
  }
  b.draw = () => { draw(); showGhost(); };

  function lines() { return [...panel.querySelectorAll(".fbar-line")]; }
  function focusValue(g = cur.g, v = cur.v) {
    const ls = lines();
    if (!ls.length) return;
    g = Math.max(0, Math.min(ls.length - 1, g));
    const items = [...ls[g].querySelectorAll(".fbar-v, .fbar-find")];
    v = Math.max(0, Math.min(items.length - 1, v));
    cur = { g, v };
    items[v].focus();
  }
  function valueKey(e, gi, vi, v) {
    const k = e.key;
    const li = lines().findIndex((l) => +l.dataset.g === gi);
    if (k === "ArrowLeft" || k === "ArrowRight") { e.preventDefault(); e.stopPropagation(); return focusValue(li, vi + (k === "ArrowRight" ? 1 : -1)); }
    // ctrl+↑↓: the gap (what a drag does)
    if ((e.ctrlKey || e.metaKey) && (k === "ArrowUp" || k === "ArrowDown")) { e.preventDefault(); e.stopPropagation(); return b.nudge(k === "ArrowDown" ? 40 : -40); }
    if (k === "ArrowUp" || k === "ArrowDown") { e.preventDefault(); e.stopPropagation(); return focusValue(li + (k === "ArrowDown" ? 1 : -1), vi); }
    if (k === "Enter" || k === " ") { e.preventDefault(); e.stopPropagation(); cur = { g: li, v: vi }; return b.set(v.filter, v.on === 1 ? 0 : 1); }
    if (k === "x" || k === "X" || k === "-" || k === "Delete") { e.preventDefault(); e.stopPropagation(); cur = { g: li, v: vi }; return b.set(v.filter, v.on === -1 ? 0 : -1); }
    if (k === "Escape" || k === "\\") { e.preventDefault(); e.stopPropagation(); return b.toggle(false); }
    if (k === "/" ) { e.preventDefault(); e.stopPropagation(); return b.focus(); }
    // a letter: into the group's box
    if (k.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault(); e.stopPropagation();
      const g = (data().groups || [])[gi];
      finds[g.key] = (finds[g.key] || "") + k;
      draw();
      const f = panel.querySelector(`[data-at="${gi}:f"]`);
      if (f) { f.focus(); f.setSelectionRange(f.value.length, f.value.length); }
      return;
    }
    e.stopPropagation();
  }
  function findKey(e, gi, f, vals) {
    e.stopPropagation();
    const k = e.key;
    const li = lines().findIndex((l) => +l.dataset.g === gi);
    const g = (data().groups || [])[gi];
    if (k === "Enter" && vals.length) { e.preventDefault(); finds[g.key] = ""; b.set(vals[0].filter, vals[0].on === 1 ? 0 : 1); return; }
    if (k === "Escape") { e.preventDefault(); if (f.value) { finds[g.key] = ""; draw(); focusValue(li, 0); } else b.toggle(false); return; }
    if (k === "ArrowUp" || k === "ArrowDown") { e.preventDefault(); return focusValue(li + (k === "ArrowDown" ? 1 : -1), 0); }
    if (k === "ArrowLeft" && f.selectionStart === 0) { e.preventDefault(); return focusValue(li, vals.length - 1); }
  }
  b.cur = () => cur;
  draw();
  return b;
}
