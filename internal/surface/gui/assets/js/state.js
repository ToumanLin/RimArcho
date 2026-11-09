// The page's state and the helpers every part uses.
import { progress, clearStatusError } from "./feedbacksemantics.js";
import { api, T, Tf } from "./api.js";
import { statusIn } from "./motion.js";
import { pathContent } from "./fields.js";

export const AREAS = ["Disabled", "Enabled", "Staging", "details"];
export const ROW_H = 28;

export const S = {
  start: null,  // Init(): language, text, commands
  board: null,  // Board(): the columns and the counts
  cols: [0, 1, 2].map(() => ({ items: [], view: [], cursor: 0, sel: new Set(), query: "", searching: false, others: null })),
  focus: 1,     // 0–2 the lists, 3 the detail (16 §3.2)
  anchor: -1,   // where shift+arrows started
  detail: null, // the shown Detail
  placed: null, // the mods space just placed: "m changes it" (16 §3.5)
  popup: null,  // the open popup (16 §3.1 rule 5)
  screen: "home", // start on the launch page; null is the plan editor
  screenSeq: 0, // late reads from a page that was left must not reopen it
  envSeq: 0,   // latest game request, including reads started before it (16 §3.1)
  busy: {},     // scan, analyze, env
  packBuild: null, // the last "pack" event of the build this window runs
  live: null,   // Live(): the running game and its errors (16 §2.1)
  history: null, // History(): unsaved steps and versions (16 §2.5)
};

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

// keyCaps draws a key label as key caps, never as plain words beside a label (16 §7.3 rule 13):
// "ctrl+o ctrl+w" is two caps, and enter shows ↵.
const KEY_TEXT = { enter: "enter ↵" };
export function keyCaps(keys) {
  const parts = String(keys).split(" ").filter(Boolean);
  const cap = (k) => el("kbd", "", KEY_TEXT[k] || k);
  if (parts.length === 1) return cap(parts[0]);
  const s = el("span", "keys");
  s.append(...parts.map(cap));
  return s;
}

export function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) {
    if (e.classList.contains("word-facts")) factText(e, text);
    else if (["INPUT", "TEXTAREA", "SELECT", "OPTION", "BUTTON", "A", "KBD"].includes(e.tagName)) e.textContent = text;
    else pathContent(e, text);
  }
  return e;
}

// View models separate facts with two terminal cells; the GUI gives each fact its own span.
export function factText(node, ...values) {
  node.classList.add("word-facts");
  node.replaceChildren();
  for (const v of values.flat().filter(v => v != null && v !== "")) {
    const tmp = pathContent(document.createElement("span"), v.text ?? v, v.paths || []);
    for (const child of [...tmp.childNodes]) {
      if (child.nodeType === Node.TEXT_NODE) {
        for (const part of child.textContent.split(/ {2,}/).filter(Boolean)) {
          const span = document.createElement("span");
          span.textContent = part;
          node.append(span);
        }
      } else node.append(child);
    }
  }
  return node;
}

export function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}

export function errText(e) {
  const m = (e && (e.message || e.error)) || String(e);
  return m.replace(/^error: /, "");
}

// ---- the status line (16 §6): one sentence, cleared by the next key ----
let undoLink = false;
export function say(kind, text, undo = false, paths = []) {
  const s = $("#status");
  s.setAttribute("role", kind === "fail" ? "alert" : "status");
  s.setAttribute("aria-live", kind === "fail" ? "assertive" : "polite");
  s.setAttribute("aria-atomic", "true");
  s.setAttribute("aria-busy", kind === "busy" ? "true" : "false");
  // Repeating an unchanged outcome must not deliver the same message twice.
  if (s.dataset.kind === (kind || "") && s.dataset.message === (text || "") && undoLink === undo) return;
  clearStatusError();
  s.dataset.kind = kind || "";
  s.dataset.message = text || "";
  s.className = kind || "";
  // One line (style.css #status): the sentence in its own box, cut with an ellipsis when it
  // is too long; its line breaks become gaps. A wrapped sentence used to be clipped in the
  // middle of its lines. The whole text stays in the live region, so a screen reader still
  // hears all of it, and is the hover text.
  const msg = factText(el("span", "msg"), { text: String(text || "").replace(/\s*\n\s*/g, "  "), paths });
  s.replaceChildren(...(text ? [msg] : []));
  if (text) s.title = text; else s.removeAttribute("title");
  if (kind === "busy" && s.firstElementChild) {
    const label = s.firstElementChild;
    // the busy text itself is the progressbar, with the count it shows (16 §11 A11)
    const count = /(\d+)\s*\/\s*(\d+)/.exec(text);
    progress(label, label, count ? +count[1] : undefined, count ? +count[2] : 100);
  }
  undoLink = undo;
  if (undo && text) {
    const a = el("a", "", T("undo"));
    a.setAttribute("role", "button");
    a.tabIndex = -1;
    // undo is true (the current list's undo) or the function that undoes this one thing
    a.onclick = typeof undo === "function" ? () => undo() : () => document.dispatchEvent(new CustomEvent("verb", { detail: "undo" }));
    s.appendChild(a);
  }
  statusIn(s);
}
// Page-level edits can supply their own undo (16 §2.17, 16 §6).
export function undoStatus() {
  if (typeof undoLink !== "function") return false;
  const undo = undoLink; say("", ""); undo(); return true;
}
export function clearSay() {
  if (!S.busy.scan && !S.busy.analyze && !S.busy.env) say("", "");
}

// busyStart shows the start before the reply (16 §11 A11); a refused start takes back only its own line.
// The work's events end it: S.busy[name] goes false and its outcome takes the line.
export async function busyStart(name, text, request, changed = () => {}) {
  if (S.busy[name]) return false;
  S.busy[name] = true;
  say("busy", text);
  changed();
  let ok = false;
  try { ok = await request(); } catch (e) { S.busy[name] = false; say("fail", errText(e)); changed(); return false; }
  if (!ok && S.busy[name]) {
    S.busy[name] = false;
    if ($("#status").dataset.message === text) say("", "");
    changed();
  }
  return ok;
}

// ---- columns ----
export const col = (k = S.focus) => S.cols[k];
export function cur(k = S.focus) {
  const c = S.cols[k];
  if (!c || c.cursor < 0 || c.cursor >= c.view.length) return null;
  return c.items[c.view[c.cursor]];
}

// selection: the selected rows of the focused list, else the cursor's row (16 §3.5).
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
  return Tf("%d mods", its.length);
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
