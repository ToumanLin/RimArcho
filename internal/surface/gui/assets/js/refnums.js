// Reference numbers (ctrl+\): every control, text block, image and chart on the page gets a
// thin red frame with a number at its top-left, such as "lib12", so a change can be asked
// for as "change lib12". The frames sit on their own layer above the page and take no
// clicks; the page's layout is untouched. The prefix names the page.
//
// A number belongs to an element for good: the first time an element is seen it takes the
// page's next number, which is remembered (GUI setting ui.gui.refnums), so the numbers do
// not move when another list is open or a button comes and goes; a page can show gaps. An
// element is known by its place in the page (ids and first classes of its ancestors) and,
// a control also by what it does (data-cmd, data-env-write, …) or its label without digits;
// alike text blocks and repeated rows by their position. ctrl+shift+\ numbers the page afresh.
//
// Each time the numbers are drawn they are also written to refnums/<prefix>.txt in the data
// directory (API.SaveRefNums): a line per number with its place and its text, the ones not
// on screen now included, so a number can be looked up in the code without the window.
// window.refnum("lib12") gives the element back (for the dev harness's console).
import { S, api } from "./state.js";

const PREFIX = {
  home: "hom", library: "lib", changes: "chg", subscribe: "sub", log: "log", needs: "run",
  plans: "lst", "plan-import": "imp", "plan-export": "exp", packs: "pak", build: "bld",
  textures: "tex", env: "env", settings: "set", tasks: "tsk", full: "ful", guide: "gui",
  uninstall: "del",
};
const EDITOR = "edt"; // the plan editor (S.screen is null)
const SETTING = "ui.gui.refnums";
const KEEP = 3000; // remembered elements per page at most (the oldest go first)

const CONTROL = "button, a[href], input:not([type=hidden]), select, textarea, summary, label:has(> input), [role=button], [role=tab], [role=menuitem], [role=option], [role=checkbox], [role=switch], [role=slider], [role=separator][tabindex], [contenteditable=true], .row, .lrow";
const MEDIA = "img, canvas, video, picture, svg";
const TEXT = "p, h1, h2, h3, h4, h5, h6, li, dt, dd, th, td, pre, blockquote, figcaption, legend, label";
// what an element does, when it says so: stable across its label's changes
const IDENT = ["data-cmd", "data-env-write", "data-id", "data-key", "data-step", "data-game", "data-area", "name", "aria-label"];

let layer = null, observer = null, queued = false, ids = new Map(), saveTimer = 0, saved = "";
// book: prefix → { next, seen: signature → [number, where, text] }
let book = null, bookDirty = false;
window.refnum = (id) => ids.get(id);

export function toggleRefNums() { layer ? off() : on(); }

async function on() {
  if (!book) {
    try { book = JSON.parse((await api.Setting(SETTING)) || "{}"); } catch { book = {}; }
  }
  if (layer) return;
  layer = document.createElement("div");
  layer.className = "refnums";
  document.body.append(layer);
  observer = new MutationObserver((records) => {
    if (records.some((r) => !layer.contains(r.target))) queue();
  });
  observer.observe(document.body, { subtree: true, childList: true, attributes: true, characterData: true });
  addEventListener("scroll", queue, true);
  addEventListener("resize", queue);
  draw();
}

function off() {
  observer.disconnect();
  removeEventListener("scroll", queue, true);
  removeEventListener("resize", queue);
  layer.remove();
  layer = observer = null;
}

function queue() {
  if (queued || !layer) return;
  queued = true;
  requestAnimationFrame(() => { queued = false; if (layer) draw(); });
}

// pick walks the page in order: a control, an image or a chart takes its whole subtree (the
// icon inside a button is part of the button); a text block is an element with text of its
// own, or one of the usual text tags, outside any block already taken.
function pick() {
  const out = [];
  const walk = (node) => {
    for (const c of node.children) {
      if (c === layer || !shown(c)) continue;
      if (c.matches(CONTROL) || (c.matches(MEDIA) && big(c))) { out.push(c); continue; }
      if (c.matches(TEXT) || ownText(c)) { out.push(c); if (!c.querySelector(CONTROL)) continue; }
      walk(c);
    }
  };
  walk(document.body);
  return out;
}

function shown(e) {
  if (e.hidden) return false;
  return e.checkVisibility ? e.checkVisibility({ opacityProperty: true, visibilityProperty: true }) : e.getClientRects().length > 0;
}

// Small inline svgs are icons next to text, not pictures of their own.
function big(e) {
  const r = e.getBoundingClientRect();
  return e.tagName !== "svg" || (r.width >= 40 && r.height >= 40);
}

function ownText(e) {
  for (const n of e.childNodes) if (n.nodeType === 3 && n.nodeValue.trim()) return true;
  return false;
}

// visible is the part of e not clipped away by its scrolling ancestors, or null.
function visible(e, clips) {
  let r = e.getBoundingClientRect();
  let box = { l: r.left, t: r.top, r: r.right, b: r.bottom };
  for (let p = e.parentElement; p && p !== document.body; p = p.parentElement) {
    let c = clips.get(p);
    if (c === undefined) {
      const st = getComputedStyle(p);
      c = /(auto|scroll|hidden|clip)/.test(st.overflow + st.overflowX + st.overflowY) ? p.getBoundingClientRect() : null;
      clips.set(p, c);
    }
    if (c) box = { l: Math.max(box.l, c.left), t: Math.max(box.t, c.top), r: Math.min(box.r, c.right), b: Math.min(box.b, c.bottom) };
    if (box.r - box.l < 2 || box.b - box.t < 2) return null;
  }
  box = { l: Math.max(box.l, 0), t: Math.max(box.t, 0), r: Math.min(box.r, innerWidth), b: Math.min(box.b, innerHeight) };
  return box.r - box.l < 2 || box.b - box.t < 2 ? null : box;
}

// covered: another layer of the page lies over e at its centre and near each corner (the
// editor stays under the other pages). Elements that take no clicks cannot be hit-tested.
function covered(e, box) {
  if (getComputedStyle(e).pointerEvents === "none") return false;
  const w = box.r - box.l, h = box.b - box.t;
  const points = [[0.5, 0.5], [0.15, 0.2], [0.85, 0.2], [0.15, 0.8], [0.85, 0.8]];
  return !points.some(([fx, fy]) => {
    const top = document.elementFromPoint(box.l + w * fx, box.t + h * fy);
    return top && (top === e || e.contains(top) || top.contains(e));
  });
}

// where: the element's place by its ancestors' ids and classes, from the page (or the
// shell part it is in) down, such as "#screen .page.home .hm-hero .hm-facts > span".
// first: only each element's first class (the later ones are usually states: on, hot).
function where(e, first = false) {
  const part = (n) => n.id ? "#" + n.id : n.classList.length ? "." + (first ? n.classList[0] : [...n.classList].join(".")) : "";
  const out = [];
  for (let n = e; n && n !== document.body; n = n.parentElement) {
    const p = part(n);
    if (p) out.unshift(p);
    if (n.id) break;
  }
  out.push("> " + e.tagName.toLowerCase());
  return out.join(" ");
}

const textOf = (e) => (e.getAttribute("aria-label") || e.textContent || e.value || e.title || "").replace(/\s+/g, " ").trim();
const ident = (e) => { for (const a of IDENT) { const v = e.getAttribute(a); if (v) return a + "=" + v; } return ""; };

// signatures gives each picked element its lasting name: its place, then what it does or
// its text (digits dropped: counts change); among many alike siblings (rows), its position.
function signatures(els) {
  const seen = new Map();
  return els.map((e) => {
    const p = e.parentElement, id = ident(e);
    // rows: many siblings of the same kind (a list's rows, a table's cells)
    const repeated = !id && p && [...p.children].filter((c) => c.tagName === e.tagName && c.classList[0] === e.classList[0]).length >= 8;
    // a text block is known by its place alone (its words are content: a mod's name); a
    // control by what it does, else its label
    const label = e.matches(CONTROL) ? textOf(e).replace(/\d+/g, "").toLowerCase().slice(0, 32) : "";
    const s = where(e, true) + "|" + (repeated ? "#" : id || label);
    const n = seen.get(s) || 0;
    seen.set(s, n + 1);
    return n ? s + "|" + (n + 1) : s;
  });
}

function draw() {
  const prefix = S.screen ? PREFIX[S.screen] || S.screen.slice(0, 3) : EDITOR;
  const page = book[prefix] || (book[prefix] = { next: 1, seen: {} });
  const clips = new Map();
  const frag = document.createDocumentFragment();
  const els = pick().map((e) => [e, visible(e, clips)]).filter(([e, box]) => box && !covered(e, box));
  const sigs = signatures(els.map(([e]) => e));
  ids = new Map();
  els.forEach(([e, box], i) => {
    let rec = page.seen[sigs[i]];
    if (!rec) { rec = page.seen[sigs[i]] = [page.next++, "", ""]; bookDirty = true; }
    const w = where(e), t = textOf(e).slice(0, 80);
    if (rec[1] !== w || rec[2] !== t) { rec[1] = w; rec[2] = t; bookDirty = true; }
    const id = prefix + rec[0];
    ids.set(id, e);
    const f = document.createElement("div");
    f.className = "refnum";
    f.style.cssText = `left:${box.l}px;top:${box.t}px;width:${box.r - box.l}px;height:${box.b - box.t}px`;
    const tag = document.createElement("span");
    tag.textContent = id;
    f.append(tag);
    frag.append(f);
  });
  layer.replaceChildren(frag);
  clearTimeout(saveTimer);
  saveTimer = setTimeout(save, 400, prefix);
}

function save(prefix) {
  if (!layer) return;
  const page = book[prefix];
  const entries = Object.entries(page.seen);
  if (entries.length > KEEP) {
    entries.sort((a, b) => a[1][0] - b[1][0]);
    for (const [s] of entries.slice(0, entries.length - KEEP)) delete page.seen[s];
    bookDirty = true;
  }
  if (bookDirty) { bookDirty = false; api.SetSetting(SETTING, JSON.stringify(book)).catch(() => {}); }
  const now = new Set([...ids.keys()]);
  const lines = Object.values(page.seen).sort((a, b) => a[0] - b[0]).map(([n, w, t]) =>
    `${prefix}${n}\t${now.has(prefix + n) ? "shown" : "not shown now"}\t${w}\t${JSON.stringify(t)}`);
  const out = `# ${prefix} (${S.screen || "editor"})  ${innerWidth}x${innerHeight}\n` + lines.join("\n") + "\n";
  if (out === saved) return;
  saved = out;
  api.SaveRefNums(prefix, out).catch(() => {});
}

// renumber forgets the page's numbers: the next drawing numbers it afresh in page order.
function renumber() {
  if (!layer || !book) return;
  const prefix = S.screen ? PREFIX[S.screen] || S.screen.slice(0, 3) : EDITOR;
  book[prefix] = { next: 1, seen: {} };
  bookDirty = true;
  draw();
}

// Capturing on the window, so fields that keep their own keys still let it through.
addEventListener("keydown", (e) => {
  if ((e.code === "Backslash" || e.key === "\\" || e.key === "|") && (e.ctrlKey || e.metaKey) && !e.altKey) {
    e.preventDefault();
    e.stopPropagation();
    if (e.shiftKey) renumber();
    else toggleRefNums();
  }
}, true);
