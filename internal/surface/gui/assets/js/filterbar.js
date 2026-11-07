// The filter bar (16 §2.5, §2.9): one component for the plan editor and Mod library > All.
// One row: the query (grey completion, Tab takes it) and a "filter" toggle (also `\`).
// Clicked conditions edit the same query. Open, it shows lists, tags, source and content:
// the values most used first with their counts, then a button to expand the rest. A left
// click (enter, space) keeps a value, a right click (x) leaves it out, again drops it; the
// current list and game's own version are marked. The keyboard reaches all of it: `\` opens the bar with the
// focus on its first value, ↑↓ go between groups, ←→ between values, a letter types into
// the main query with that group key, esc closes it.
//
// The page gives: data() → { groups, conds, complete } of its last answer; change(search,
// filters) when either changed; and the place it lives in. The bar keeps search, filters
// and open; the page reads them.
import { el, T, Tf, api, keyCaps } from "./state.js";
import { wireSeparator } from "./separator.js";
import { suggestions, queryContext, setQueryFilter } from "./searchsuggest.js";
const LINE = 24; // common values before the expand button
let nextBar = 0;
export function createBar(opts) {
  const b = {
    search: "",
    filters: [],
    open: false,
    opts
  };
  const root = el("div", "fbar" + (opts.cls ? " " + opts.cls : ""));
  const row = el("div", "fbar-row");
  const box = el("div", "fbar-search");
  const inp = el("input");
  inp.spellcheck = false;
  // placeholder: a string, or a function so it follows a language change
  const placeholder = () => (typeof opts.placeholder === "function" ? opts.placeholder() : opts.placeholder) || T("search");
  inp.placeholder = placeholder();
  const ghost = el("span", "fbar-ghost");
  ghost.append(el("span", "typed"), el("span", "sfx"));
  ghost.setAttribute("aria-hidden", "true");
  box.append(inp, ghost);
  const popup = el("div", "fbar-suggestions");
  const listID = "filter-options-" + ++nextBar;
  popup.hidden = true;
  box.append(popup);
  inp.setAttribute("role", "combobox");
  inp.setAttribute("aria-autocomplete", "both");
  const clear = el("button", "fbar-clear", "×");
  clear.title = T("clear");
  clear.onclick = () => {
    b.clear();
    inp.focus();
  };
  let suggestOpen = false,
    choice = 0,
    candidates = [],
    composing = false;
  const tog = el("button", "fbar-tog");
  tog.title = T("filter") + T("; ") + "\\";
  const panel = el("div", "fbar-panel");
  panel.hidden = true;
  row.append(box, clear, tog);
  // Drag the gap to share the column with the details; double click restores 35%.
  const gap = el("div", "fbar-gap");
  gap.title = T("drag to resize, double click: back to the default");
  root.append(row, panel, gap);
  b.el = root;
  b.input = inp;
  const expanded = new Set(); // group keys whose remaining values are visible
  let cur = {
    g: 0,
    v: 0
  }; // the panel's focused value
  let wantFocus = false;
  const data = () => opts.data() || {};
  const groups = () =>
    (data().groups || [])
      .filter((g) => g.key !== "location")
      .sort((a, b) => {
        const order = ["files", "lists", "tag", "source", "content"];
        return (order.includes(a.key) ? order.indexOf(a.key) : 9) - (order.includes(b.key) ? order.indexOf(b.key) : 9);
      });
  // Derive completion from the active query span without changing stored conditions.
  function showGhost() {
    candidates = suggestions(
      groups().filter((g) => !g.target),
      inp.value,
      inp.selectionStart ?? inp.value.length,
      opts.bare
    );
    choice = Math.min(choice, Math.max(0, candidates.length - 1));
    const visible = suggestOpen && !composing && document.activeElement === inp;
    if (visible) inp.setAttribute("aria-controls", listID); else inp.removeAttribute("aria-controls");
    popup.hidden = !visible;
    inp.setAttribute("aria-expanded", String(visible));
    popup.replaceChildren();
    if (visible) {
      const h = el("div", "fbar-suggestion-head");
      h.append(
        el(
          "span",
          "",
          T(
            opts.bare || queryContext(inp.value, inp.selectionStart ?? inp.value.length).text.includes(":")
              ? "available values"
              : "available keys"
          )
        ),
        keyCaps("↑↓ tab")
      );
      popup.append(h);
      const list = el("div", "fbar-suggestion-list");
      list.id = listID;
      list.setAttribute("role", "listbox");
      candidates.forEach((c, i) => {
        const button = el("button", "fbar-suggestion" + (i === choice ? " on" : ""));
        button.setAttribute("role", "option");
        button.id = listID + "-" + i;
        button.setAttribute("aria-selected", String(i === choice));
        button.tabIndex = -1;
        button.append(el("code", "", c.token));
        if (c.label) {
          button.append(el("span", "", c.label));
        }
        button.onmousedown = (e) => e.preventDefault();
        button.onclick = () => accept(i);
        list.append(button);
      });
      if (!candidates.length) {
        list.append(el("span", "second", T("no suggestions")));
      }
      popup.append(list);
    }
    if (visible && candidates.length) {
      inp.setAttribute("aria-activedescendant", listID + "-" + choice);
    } else {
      inp.removeAttribute("aria-activedescendant");
    }
    const ctx = queryContext(inp.value, inp.selectionStart ?? inp.value.length);
    const token = candidates[choice]?.token || "";
    const sfx = token.toLowerCase().startsWith(ctx.text.toLowerCase()) ? token.slice(ctx.text.length) : "";
    const atEnd = inp.selectionStart === inp.value.length && inp.selectionEnd === inp.value.length;
    ghost.hidden = !(visible && sfx && atEnd);
    if (!ghost.hidden) {
      ghost.firstChild.textContent = inp.value;
      ghost.lastChild.textContent = sfx;
      ghost.style.transform = `translateX(${-inp.scrollLeft}px)`;
    }
    inp.placeholder = !ghost.hidden ? "" : placeholder();
    clear.hidden = !inp.value;
  }
  b.showGhost = showGhost;
  let timer = 0;
  // Commit input once and distinguish panel edits from ordinary typing.
  function change(text, show = false, reason = "input") {
    clearTimeout(timer);
    inp.value = b.search = text;
    b.filters = [];
    suggestOpen = show;
    choice = 0;
    opts.change(false, reason);
    showGhost();
  }
  function accept(i) {
    const c = candidates[i];
    if (!c) {
      return;
    }
    const ctx = queryContext(inp.value, inp.selectionStart ?? inp.value.length);
    const suffix = inp.value.slice(ctx.end),
      space = !c.key && !suffix.startsWith(" ") ? " " : "";
    const text = inp.value.slice(0, ctx.start) + c.token + space + suffix;
    const caret = ctx.start + c.token.length + space.length;
    inp.value = text;
    inp.setSelectionRange(caret, caret);
    change(text, c.key);
  }
  inp.oninput = () => {
    choice = 0;
    suggestOpen = true;
    showGhost();
    clearTimeout(timer);
    if (!composing) {
      timer = setTimeout(() => change(inp.value, document.activeElement === inp), 90);
    }
  };
  inp.onfocus = inp.onclick = () => {
    suggestOpen = true;
    choice = 0;
    showGhost();
  };
  inp.onblur = () => {
    suggestOpen = false;
    showGhost();
  };
  inp.onscroll = showGhost;
  inp.addEventListener("compositionstart", () => {
    composing = true;
    showGhost();
  });
  inp.addEventListener("compositionend", () => {
    composing = false;
    change(inp.value, true);
  });
  inp.onkeydown = (e) => {
    e.stopPropagation();
    if (composing || e.isComposing) {
      return;
    }
    if (opts.key?.(e)) {
      return;
    }
    if (e.key === "Tab" && !e.shiftKey && suggestOpen && candidates.length) {
      e.preventDefault();
      accept(choice);
      return;
    }
    if ((e.key === "ArrowDown" || e.key === "ArrowUp") && candidates.length) {
      e.preventDefault();
      suggestOpen = true;
      choice = (choice + (e.key === "ArrowDown" ? 1 : candidates.length - 1)) % candidates.length;
      showGhost();
      popup.querySelector(".fbar-suggestion.on")?.scrollIntoView({
        block: "nearest"
      });
      return;
    }
    if (e.key === "Escape") {
      e.preventDefault();
      if (suggestOpen) {
        suggestOpen = false;
        showGhost();
        return;
      }
      if (inp.value) {
        change("");
      }
      inp.blur();
      opts.leave?.();
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault();
      change(inp.value);
      if (opts.submit) {
        opts.submit();
      } else {
        inp.blur();
        opts.leave?.();
      }
      return;
    }
    if (e.key === "\\") {
      e.preventDefault();
      if (opts.toggle) {
        opts.toggle();
      } else {
        b.toggle();
      }
      return;
    }
    if (["ArrowLeft", "ArrowRight", "Home", "End"].includes(e.key)) {
      setTimeout(showGhost);
    }
  };
  tog.onclick = () => b.toggle();
  b.set = (f, on, g) => {
    const target = g?.target && opts.targets?.[g.target];
    if (target) {
      return target.set(f, on);
    }
    change(setQueryFilter(inp.value, f, on), false, "filter");
  };
  b.setText = (text) => {
    inp.value = b.search = text;
    showGhost();
  };
  b.type = (text) => {
    change(text, true);
    inp.focus();
    inp.setSelectionRange(text.length, text.length);
    showGhost();
  };
  b.clear = () => change("");
  b.active = () => !!(b.search.trim() || b.filters.length);
  // null follows the default 35% share; a number is a saved height, 0 fills the column.
  b.h = null;
  const KEY = opts.splitKey;
  const colH = () =>
    root.isConnected ? (opts.column ? opts.column() : root.parentElement)?.getBoundingClientRect().height || 0 : 0;
  const height = () => (b.h === null ? Math.max(120, colH() * 0.35) : b.h || colH() - 34);
  const savedHeight = () => (b.h === null ? "" : String(b.h));
  function place() {
    root.classList.toggle("fill", b.open && b.h === 0);
    if (b.h === null) {
      root.style.setProperty("--fbar-h", "max(120px, 35%)");
    } else if (b.h) {
      root.style.setProperty("--fbar-h", b.h + "px");
    } else {
      root.style.removeProperty("--fbar-h");
    }
  }
  // setH keeps at least the bar and a few values, and leaves what is below its title and a
  // few rows; dragged near the bottom it fills the column again
  b.setH = (h, save = true) => {
    const max = colH() - 34;
    b.h = h === null ? null : !h || h > max - 40 ? 0 : Math.max(120, Math.round(h));
    place();
    if (save && KEY) {
      api.SetSetting(KEY, savedHeight()).catch(() => {});
    }
    opts.resized && opts.resized();
    separator.refresh();
  };
  // A page whose card holds more than the bar (Search files) owns the gap below the card:
  // it passes noGap and its own nudge for ctrl+↑↓.
  b.nudge = (d) => (opts.nudge ? opts.nudge(d) : b.setH(height() + d));
  const separator = opts.noPanel || opts.noGap
    ? {
        refresh() {}
      }
    : wireSeparator(gap, {
        axis: "y",
        key: KEY,
        enabled: () => b.open,
        range: () => [120, colH() - 34],
        get: height,
        set: (h) => b.setH(h, false),
        reset: () => b.setH(null, false),
        serialize: savedHeight,
        restore: (v) => {
          b.h = v === null || v === undefined || v === "" || !Number.isFinite(+v) || +v < 0 ? null : +v;
          place();
        }
      });
  b.fills = () => b.open && b.h === 0;
  b.toggle = (v = !b.open) => {
    if (opts.noPanel) {
      return;
    }
    b.open = v;
    if (v) {
      wantFocus = true;
    }
    draw();
    opts.change(true);
    if (v) {
      suggestOpen = false;
      showGhost();
    } else if (root.contains(document.activeElement)) {
      document.activeElement.blur();
      opts.leave && opts.leave();
    }
  };
  // the open bar folds the moment the focus leaves it: a click elsewhere, tab, a key that
  // moves the focus (16 §3.1 rule 8); what it filters stays in force on its one line
  const left = (t) =>
    b.open &&
    root.isConnected &&
    t instanceof Node &&
    t.isConnected &&
    // focusRoot may be several elements: a card and the gap that resizes it
    ![].concat(opts.focusRoot?.() || root).some((r) => r?.contains(t)) &&
    !(t.closest && t.closest("#menu"));
  let pointerOutside = false;
  const down = (e) => {
    pointerOutside = left(e.target);
  };
  const mouse = (e) => {
    pointerOutside = false;
    if (left(e.target)) {
      b.toggle(false);
    }
  };
  const focus = (e) => {
    if (!pointerOutside && left(e.target)) {
      b.toggle(false);
    }
  };
  const outsideEvent = opts.deferOutside ? "click" : "mousedown";
  if (opts.deferOutside) {
    document.addEventListener("mousedown", down, true);
  }
  document.addEventListener(outsideEvent, mouse, !opts.deferOutside);
  document.addEventListener("focusin", focus);
  b.destroy = () => {
    clearTimeout(timer);
    document.removeEventListener("mousedown", down, true);
    document.removeEventListener(outsideEvent, mouse, !opts.deferOutside);
    document.removeEventListener("focusin", focus);
  };
  b.focus = () => {
    inp.focus();
    inp.select();
  };
  b.focused = () => root.contains(document.activeElement);
  const available = (g) => (g.values || []).filter((v) => g.noCounts || v.count > 0 || v.on);
  const lineOf = (g) => {
    const vals = available(g);
    return expanded.has(g.key) ? vals : vals.slice(0, LINE).concat(vals.slice(LINE).filter((v) => v.on));
  };
  // Rebuild panel values while preserving the focused group and value.
  function draw() {
    const d = data();
    tog.replaceChildren(el("span", "", T("filter")), keyCaps("\\"));
    const count = opts.count?.();
    if (count) {
      tog.insertBefore(el("span", "fbar-count", String(count)), tog.lastChild);
    }
    tog.hidden = !!opts.noPanel;
    gap.hidden = !!(opts.noPanel || opts.noGap);
    clear.hidden = !inp.value;
    tog.classList.toggle("on", b.open);
    panel.hidden = !b.open;
    place();
    root.classList.toggle("open", b.open);
    if (!b.open) {
      return;
    }
    if (opts.width) {
      root.style.setProperty("--fbar-w", opts.width() + "px");
    }
    const keep = panel.contains(document.activeElement) ? document.activeElement.dataset.at : null;
    panel.innerHTML = "";
    const ordered = groups();
    ordered.forEach((g) => {
      const gi = (d.groups || []).indexOf(g);
      if (!["files", "lists", "tag", "source", "content"].includes(g.key)) {
        return;
      }
      const vals = lineOf(g);
      if (!vals.length) {
        return;
      }
      const t = el("span", "fbar-gt", g.title);
      const line = el("span", "fbar-line");
      line.dataset.g = gi;
      vals.forEach((v, vi) => {
        const x = el("button", "fbar-v" + (v.on === 1 ? " on" : v.on === -1 ? " ex" : "") + (v.cur ? " cur" : ""));
        x.append(el("span", "l", v.label));
        if (v.cur) {
          x.append(el("span", "curv", T(g.key === "lists" ? "current list" : "this game")));
        }
        if (!g.noCounts) {
          x.append(el("span", "n", String(v.count)));
        }
        x.title = v.label + "\n" + T("left click: only these, right click or x: leave these out");
        x.dataset.at = gi + ":" + vi;
        x.tabIndex = -1;
        x.onclick = () => b.set(v.filter, v.on === 1 ? 0 : 1, g);
        x.oncontextmenu = (e) => {
          e.preventDefault();
          b.set(v.filter, v.on === -1 ? 0 : -1, g);
        };
        x.onkeydown = (e) => valueKey(e, gi, vi, v);
        line.append(x);
      });
      const more = available(g).length - vals.length;
      if (more > 0 || (expanded.has(g.key) && available(g).length > LINE)) {
        const f = el("button", "fbar-v fbar-more", more > 0 ? Tf("%d more…", more) : T("show fewer"));
        f.dataset.at = gi + ":more";
        f.setAttribute("aria-expanded", String(expanded.has(g.key)));
        f.onclick = () => {
          expanded.has(g.key) ? expanded.delete(g.key) : expanded.add(g.key);
          draw();
        };
        f.onkeydown = (e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            e.stopPropagation();
            f.onclick();
          } else if (!["x", "X", "-", "Delete"].includes(e.key)) {
            valueKey(e, gi, vals.length, null);
          } else {
            e.stopPropagation();
          }
        };
        line.append(f);
      }
      panel.append(t, line);
    });
    if (wantFocus && panel.querySelector(".fbar-v")) {
      wantFocus = false;
      cur = {
        g: 0,
        v: 0
      };
      focusValue();
      return;
    }
    if (keep) {
      const n =
        panel.querySelector(`[data-at="${keep}"]`) || panel.querySelector(`[data-at^="${keep.split(":")[0]}:"]`);
      if (n) {
        n.focus({
          preventScroll: true
        });
      }
    }
  }
  b.draw = () => {
    draw();
    showGhost();
  };
  function lines() {
    return [...panel.querySelectorAll(".fbar-line")];
  }
  function focusValue(g = cur.g, v = cur.v) {
    const ls = lines();
    if (!ls.length) {
      return;
    }
    g = Math.max(0, Math.min(ls.length - 1, g));
    const items = [...ls[g].querySelectorAll(".fbar-v")];
    v = Math.max(0, Math.min(items.length - 1, v));
    cur = {
      g,
      v
    };
    items[v].focus();
  }
  // Route keyboard edits to the input owned by the focused group.
  function valueKey(e, gi, vi, v) {
    if (opts.key?.(e)) {
      return;
    }
    const k = e.key;
    const li = lines().findIndex((l) => +l.dataset.g === gi);
    if (k === "ArrowLeft" || k === "ArrowRight") {
      e.preventDefault();
      e.stopPropagation();
      return focusValue(li, vi + (k === "ArrowRight" ? 1 : -1));
    }
    // ctrl+↑↓: the gap (what a drag does)
    if ((e.ctrlKey || e.metaKey) && (k === "ArrowUp" || k === "ArrowDown")) {
      e.preventDefault();
      e.stopPropagation();
      return b.nudge(k === "ArrowDown" ? 40 : -40);
    }
    if (k === "ArrowUp" || k === "ArrowDown") {
      e.preventDefault();
      e.stopPropagation();
      return focusValue(li + (k === "ArrowDown" ? 1 : -1), vi);
    }
    if (k === "Enter" || k === " ") {
      e.preventDefault();
      e.stopPropagation();
      cur = {
        g: li,
        v: vi
      };
      return b.set(v.filter, v.on === 1 ? 0 : 1, (data().groups || [])[gi]);
    }
    if (k === "x" || k === "X" || k === "-" || k === "Delete") {
      e.preventDefault();
      e.stopPropagation();
      cur = {
        g: li,
        v: vi
      };
      return b.set(v.filter, v.on === -1 ? 0 : -1, (data().groups || [])[gi]);
    }
    if (k === "Escape" || k === "\\") {
      e.preventDefault();
      e.stopPropagation();
      return b.toggle(false);
    }
    if (k === "/") {
      e.preventDefault();
      e.stopPropagation();
      return b.focus();
    }
    // Typing from a group continues in the one query input.
    if (k.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault();
      e.stopPropagation();
      const g = (data().groups || [])[gi];
      const target = g.target && opts.targets?.[g.target];
      if (target) {
        const prefix = target.input.value.trimEnd();
        target.type((prefix ? prefix + " " : "") + k);
        return;
      }
      const prefix = inp.value.trimEnd();
      change((prefix ? prefix + " " : "") + (g.key === "tag" ? "tags" : g.key) + ":" + k, true);
      inp.focus();
      inp.setSelectionRange(inp.value.length, inp.value.length);
      showGhost();
      return;
    }
    e.stopPropagation();
  }
  b.cur = () => cur;
  draw();
  return b;
}
