// The row's status badge says what it is (16 §4.2): hovering it shows the items behind the
// icon (S.board.reasons, worst first); a click pins the popover so its text can be read and
// selected; a second click on the badge, esc, a click elsewhere or scrolling the list closes
// it. The same information is in the details (enter), so the keyboard reaches it there.
import { S, el, T } from "./state.js";
import { statusIcon } from "./icons.js";
import { needText, needEvidence } from "./needtext.js";

const box = el("div", "badge-pop");
box.setAttribute("role", "tooltip");
let shown = null; // { k, key } of the row whose badge the popover is for
let pinned = false;
let hideTimer = 0;
let dragging = false; // a row drag is running (16 §8.5): no popover may cover the drop rows
// run(item, action) and write(action) come from Can it run (screens.js): the pinned popover
// offers an item's actions as that page does.
let runNeed = null, needWrite = () => "";
export function setNeedRunner(run, write) { runNeed = run; needWrite = write; }

// reasonsOf: the items behind a row's icon; a missing entry or an unloadable mod without
// one says what the icon itself means.
function reasonsOf(it, st) {
  const rs = (it.id && S.board && S.board.reasons && S.board.reasons[it.id]) || [];
  if (rs.length) return rs;
  if (it.missing) return [{ status: st, text: T("not installed") }];
  if (st === 1) return [{ status: st, text: T("the game cannot load it") }];
  return [];
}

function draw(it, st) {
  box.innerHTML = "";
  for (const r of reasonsOf(it, st)) {
    const line = el("div", "bp-item");
    const ic = el("span", "bp-ic s" + r.status);
    ic.innerHTML = statusIcon(r.status);
    const body = el("div", "bp-body");
    body.append(needText("div", "bp-t", r.text, r.content?.text, () => hide(true), it.id));
    if (r.why) body.append(needText("div", "bp-why", r.why, r.content?.why, () => hide(true), it.id));
    if (r.evidence) body.append(needEvidence(r.evidence));
    if (pinned && runNeed && r.actions && r.actions.length) {
      const acts = el("div", "bp-acts");
      const answers = r.actions.some((a) => a.kind === "answer");
      r.actions.forEach((a, i) => {
        const why = answers && a.kind !== "answer";
        const b = el("button", why ? "btn identity-why" : "btn small" + (i === 0 && !answers ? " primary" : ""), a.label);
        if (a.title) b.title = a.title;
        const w = needWrite(a);
        if (w) b.dataset.envWrite = w;
        b.onclick = (e) => { e.stopPropagation(); hide(true); runNeed(r, a); };
        (why ? body : acts).append(b);
      });
      body.append(acts);
    }
    line.append(ic, body);
    box.append(line);
  }
  box.classList.toggle("pinned", pinned);
  if (!pinned) box.append(el("div", "bp-hint", T("click to keep it open")));
}

function place(anchor) {
  if (!box.isConnected) document.body.append(box);
  const a = anchor.getBoundingClientRect();
  const w = box.offsetWidth, h = box.offsetHeight;
  let x = a.right - w, y = a.bottom + 6;
  if (x < 8) x = 8;
  if (y + h > innerHeight - 8) y = Math.max(8, a.top - h - 6);
  box.style.left = x + "px";
  box.style.top = y + "px";
}

function show(k, it, st, anchor) {
  clearTimeout(hideTimer);
  shown = { k, key: rowKey(it) };
  draw(it, st);
  if (!box.firstChild) return hide(true);
  place(anchor);
}

export function hide(force = false) {
  clearTimeout(hideTimer);
  if (pinned && !force) return;
  pinned = false;
  shown = null;
  box.remove();
}

const rowKey = (it) => it.id || it.ref;

// rowDrag: lists.js says when a row drag starts and ends. Starting closes the popover, even
// a pinned one (the press that starts a drag may have pinned it), and while the drag runs
// passing over a badge shows nothing, so the rows under the pointer stay a drop target.
export function rowDrag(on) {
  dragging = on;
  if (on) hide(true);
}

// wireBadges hooks one list's rows: item(i) is the row's item, status(it) its icon.
export function wireBadges(k, rows, item, status) {
  listen();
  const target = (e) => {
    const s = e.target.closest && e.target.closest(".row .s");
    if (!s || !s.firstChild) return null;
    const r = s.closest(".row");
    const it = item(+r.dataset.i);
    return it ? { s, it, st: status(it) } : null;
  };
  rows.addEventListener("mouseover", (e) => {
    const t = target(e);
    if (!t || pinned || dragging) return;
    show(k, t.it, t.st, t.s);
  });
  rows.addEventListener("mouseout", (e) => {
    if (pinned || !target(e)) return;
    if (box.contains(e.relatedTarget)) return;
    hideTimer = setTimeout(() => hide(), 120);
  });
  // mousedown, not click: the row redraws when the press selects it, so the badge under
  // the mouse is a new element by the time a click would land
  rows.addEventListener("mousedown", (e) => {
    const t = e.button === 0 && target(e);
    if (!t) return;
    const same = shown && shown.k === k && shown.key === rowKey(t.it);
    if (pinned && same) return hide(true);
    pinned = true;
    show(k, t.it, t.st, t.s);
  }, true); // before the row's own mousedown redraws it
  rows.addEventListener("scroll", () => hide(true), { passive: true });
}

// listen hooks the popover and the page once, when the first list is wired (the module
// also loads where there is no list, e.g. Can it run setting the runner).
let listening = false;
function listen() {
  if (listening) return;
  listening = true;
  box.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") e.stopPropagation();
  });
  box.addEventListener("mouseleave", () => { if (!pinned) hideTimer = setTimeout(() => hide(), 120); });
  box.addEventListener("mouseenter", () => clearTimeout(hideTimer));
  // a click outside the popover and outside any badge closes a pinned one
  document.addEventListener("mousedown", (e) => {
    if (!pinned || box.contains(e.target) || (e.target.closest && e.target.closest(".row .s"))) return;
    hide(true);
  }, true);
  // esc closes a pinned popover before anything else takes the key
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape" || !box.isConnected) return;
    if (!pinned) return hide(true);
    e.preventDefault();
    e.stopPropagation();
    hide(true);
  }, true);
}
