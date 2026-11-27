// Settings as RimArcho's two panes (16 §2.14 "Layout", 16 §7.3 rule 12): the groups at the
// left, each a row with its name, one muted summary line and a dot when something can be done
// (accent) or needs a look (amber); the picked group at the right, in one card whose head has
// its name and note. Every group's rows are drawn once and only the picked one is shown, so
// task updates and deep links find any row. A group of sections (Accounts and network…) draws
// each under its own heading, as the head draws the group. The gap drags (ui.gui.settings.side).
import { S, $, el, T } from "./state.js";
import { wireSeparator } from "./separator.js";
import { diskMeter } from "./membar.js";

const LAST = "archo.settings.group"; // the group last viewed (this window's localStorage)
const lastViewed = () => { try { return localStorage.getItem(LAST) || ""; } catch { return ""; } };
const remember = (key) => { try { localStorage.setItem(LAST, key); } catch { /* storage can be off */ } };

// groupOf is the group whose key, or one of whose sections' keys, is key: a key saved before
// the groups merged (network, storage, log…) finds the group that holds it now.
export function groupOf(groups, key) {
  return (groups || []).find(g => g.key === key || (g.sections || []).some(s => s.key === key));
}

// openingGroup: the first group that needs a look, else the last one viewed, else Interface.
export function openingGroup(groups) {
  const amber = groups.find(g => g.attention === "amber");
  if (amber) return amber.key;
  const last = groupOf(groups, lastViewed());
  if (last) return last.key;
  return groups.some(g => g.key === "interface") ? "interface" : groups[0]?.key || "";
}

// drawSettings lays the page out in w. body(box, g) draws group g's rows into box; it is
// called for every group. area(box, head, focus) gives the list's rows keys 1–9; crumbs
// redraws the breadcrumb (shell.js, passed in so this module does not import it).
let crumbs = () => {};
export function drawSettings(w, groups, shown, { body, area, crumbs: redraw }) {
  crumbs = redraw || (() => {});
  const grid = el("div", "st-lay");
  const side = el("section", "card xc-do st-list");
  side.setAttribute("role", "listbox");
  side.setAttribute("aria-label", T("settings"));
  const main = el("section", "card settings st-card");
  const head = el("div", "st-head");
  const scroll = el("div", "st-body");
  main.append(head, scroll);
  // the gap after the right card in the tab order: tab from the list goes into the right card
  const gap = el("div", "split st-split");
  grid.append(side, main, gap);
  const setSide = (v) => { if (v) grid.style.setProperty("--st-side", v + "px"); else grid.style.removeProperty("--st-side"); };
  const range = () => [220, Math.max(220, grid.clientWidth - 420 - 12)];
  wireSeparator(gap, {
    key: "ui.gui.settings.side", range,
    get: () => side.getBoundingClientRect().width,
    set: setSide, reset: () => setSide(0),
    restore: (v) => setSide(+v ? Math.max(range()[0], +v) : 0),
  });
  w.append(grid);
  for (const g of groups) {
    const r = groupRow(g, g.key === shown);
    // uninstall sits a little apart, as Feedback's "my reports"
    if (g.key === "uninstall") r.classList.add("st-apart");
    r.onclick = () => pick(g.key, { focus: false });
    side.append(r);
    // 1–9 pick the first nine groups; no number is drawn, so the names line up
    area(r, null, () => pick(g.key));
    const box = el("div", "st-group");
    box.dataset.group = g.key;
    box.hidden = g.key !== shown;
    if (g.sections?.length) {
      for (const s of g.sections) {
        const sec = el("div", "st-sec");
        sec.dataset.section = s.key;
        const h = el("div", "st-sec-head");
        h.append(el("div", "st-title", s.title));
        if (s.note) h.append(el("div", "group-note", s.note));
        sec.append(h);
        body(sec, s);
        box.append(sec);
      }
    } else body(box, g);
    scroll.append(box);
  }
  side.addEventListener("keydown", listKey);
  showHead(groups.find(g => g.key === shown));
  S.settingsGroups = groups;
}

function groupRow(g, on) {
  const r = el("button", "xc-dorow st-row" + (on ? " on" : ""));
  r.type = "button";
  r.dataset.group = g.key;
  r.setAttribute("role", "option");
  r.setAttribute("aria-selected", String(on));
  r.tabIndex = on ? 0 : -1;
  const l = el("div", "l"), t = el("div", "name");
  t.append(el("span", "nt", g.title));
  if (g.attention === "accent" || g.attention === "amber") {
    const d = el("span", "st-dot " + g.attention);
    d.setAttribute("aria-hidden", "true");
    t.append(d);
  }
  // the summary's facts, 12 px apart (16 §7.3 rule 5: no middle dots)
  const facts = el("div", "sub st-facts");
  for (const f of g.summary || []) facts.append(el("span", "", f));
  l.append(t, facts);
  if (g.disk) l.append(diskMeter(g.disk));
  r.append(l);
  r.title = (g.summary || []).join("  ");
  return r;
}

// pick shows group key at the right; the row keeps or takes the focus.
export function pick(key, { focus = true, keepScroll = false } = {}) {
  const sc = $("#screen");
  const g = groupOf(S.settingsGroups, key);
  if (!sc || !g) return;
  key = g.key;
  for (const r of sc.querySelectorAll(".st-row")) {
    const on = r.dataset.group === key;
    r.classList.toggle("on", on);
    r.setAttribute("aria-selected", String(on));
    r.tabIndex = on ? 0 : -1;
    if (on && focus) r.focus({ preventScroll: true });
  }
  for (const b of sc.querySelectorAll(".st-group")) b.hidden = b.dataset.group !== key;
  for (const x of sc.querySelectorAll(".st-landed")) x.classList.remove("st-landed");
  const body = sc.querySelector(".st-body");
  if (body && !keepScroll && S.settingsGroup !== key) body.scrollTop = 0;
  showHead(g);
  sc.querySelector(`.st-row[data-group="${key}"]`)?.scrollIntoView({ block: "nearest" });
}

// showHead: the right card's head names the group and says its note; the breadcrumb follows.
function showHead(g) {
  const head = $("#screen .st-head");
  if (!head || !g) return;
  head.replaceChildren(el("div", "st-title", g.title));
  if (g.note) head.append(el("div", "group-note", g.note));
  S.settingsGroup = g.key;
  remember(g.key);
  if (S.settingsCrumb !== g.title) { S.settingsCrumb = g.title; crumbs(); }
}

// ↑ / ↓ in the list change the group; home / end go to the first and last.
function listKey(e) {
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  const rows = [...e.currentTarget.querySelectorAll(".st-row")];
  const at = rows.findIndex(r => r.classList.contains("on"));
  const to = { ArrowDown: at + 1, ArrowUp: at - 1, Home: 0, End: rows.length - 1 }[e.key];
  if (to === undefined) return;
  e.preventDefault(); e.stopPropagation();
  const r = rows[Math.max(0, Math.min(rows.length - 1, to))];
  if (r) pick(r.dataset.group);
}

// reveal brings a row of any group into view: its group is picked, the row scrolled to and
// marked until another group is picked (deep links from other pages and the command panel).
export function reveal(row) {
  const box = row?.closest(".st-group");
  if (!box) return false;
  pick(box.dataset.group, { focus: false, keepScroll: true });
  row.classList.add("st-landed");
  row.scrollIntoView({ block: "center" });
  return true;
}

// revealSetting finds the first row matching selector on the Settings page and reveals it;
// the first button in it takes the focus.
export function revealSetting(selector) {
  const row = document.querySelector(selector)?.closest("[data-setting-row]");
  if (!reveal(row)) return null;
  row.querySelector("button, input, select")?.focus({ preventScroll: true });
  return row;
}

