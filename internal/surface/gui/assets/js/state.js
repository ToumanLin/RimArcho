// The page's state and the helpers every part uses.
import { api, T, Tf } from "./api.js";

export const AREAS = ["Disabled", "Enabled", "Staging", "details"];
export const ROW_H = 28;

export const S = {
  start: null,  // Init(): language, text, commands
  board: null,  // Board(): the columns and the counts
  cols: [0, 1, 2].map(() => ({ items: [], view: [], cursor: 0, sel: new Set(), query: "", searching: false, others: null })),
  focus: 1,     // 0–2 the lists, 3 the detail (06 §13.3)
  anchor: -1,   // where shift+arrows started
  detail: null, // the shown Detail
  placed: null, // the mods space just placed: "m changes it" (06 §13.6)
  popup: null,  // the open popup (06 §13.2 rule 5)
  screen: null, // "needs" | "changes" | "full"
  busy: {},     // scan, analyze
  packBuild: null, // the last "pack" event of the build this window runs
  live: null,   // Live(): the running game and its errors (10 §3.4)
  history: null, // History(): unsaved steps and versions (08 §14)
};

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

export function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

export function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}

export function errText(e) {
  const m = (e && (e.message || e.error)) || String(e);
  return m.replace(/^error: /, "");
}

// ---- the status line (06 §9): one sentence, cleared by the next key ----
let undoLink = false;
export function say(kind, text, undo = false) {
  const s = $("#status");
  s.className = kind || "";
  s.textContent = text || "";
  undoLink = undo;
  if (undo && text) {
    const a = el("a", "", T("undo"));
    a.onclick = () => document.dispatchEvent(new CustomEvent("verb", { detail: "undo" }));
    s.appendChild(a);
  }
}
export function clearSay() {
  if (!S.busy.scan && !S.busy.analyze) say("", "");
}

// ---- columns ----
export const col = (k = S.focus) => S.cols[k];
export function cur(k = S.focus) {
  const c = S.cols[k];
  if (!c || c.cursor < 0 || c.cursor >= c.view.length) return null;
  return c.items[c.view[c.cursor]];
}

// selection: the selected rows of the focused list, else the cursor's row (06 §13.6).
export function selection() {
  if (S.focus > 2) return [];
  const c = col();
  if (c.sel.size) return c.items.filter((it) => c.sel.has(key(it)));
  const it = cur();
  return it ? [it] : [];
}
export const key = (it) => it.id || it.ref;
export const idsOf = (its) => its.filter((it) => it.id).map((it) => it.id);
export const refsOf = (its) => its.map((it) => it.ref);
export function named(its) {
  if (its.length === 1) return its[0].name;
  return S.start && S.start.lang === "zh" ? `${its.length} 个 Mod` : `${its.length} mods`;
}

// slideThumb moves a switch's raised thumb under its open choice (magpie's view switch):
// it slides from where it was instead of jumping. None open: it fades out.
export function slideThumb(box, on) {
  const th = box && box.querySelector(":scope > .thumb");
  if (!th) return;
  if (!on) { th.style.opacity = "0"; return; }
  const first = !th.style.width;
  if (first) th.style.transition = "none";
  th.style.opacity = "1";
  th.style.transform = `translateX(${on.offsetLeft}px)`;
  th.style.width = on.offsetWidth + "px";
  if (first) requestAnimationFrame(() => { th.style.transition = ""; });
}

export { api, T, Tf };
