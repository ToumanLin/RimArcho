// 设置 › 启动页背景 (06 §2.14): a small launch page that shows the choice as it will look,
// the dimming, blur, focus and rotation beside it, and the images by group below — the
// game's, each background mod's, the player's own. Every change is saved at once
// (API.SetBackground). The grid is reached with tab: arrows pick, enter uses, * stars
// (rotation picks among the starred), [ ] change the group, delete forgets a picked file.
// Thumbnails are the 480 px cached JPEGs (app.BackgroundImage), loaded as they scroll in.
import { el, api, T, Tf, errText, say } from "./state.js";

let P = null;   // API.Backgrounds: the setting and the groups
let tab = 0;    // the open group
let kb = 0;     // the grid's keyboard item
let box = null; // the section, redrawn in place

export async function bgSection(current = () => true) {
  let p;
  try { p = await api.Backgrounds(); } catch (e) { if (current()) say("fail", errText(e)); return el("div"); }
  if (!current()) return el("div");
  P = p;
  const i = P.groups.findIndex((g) => g.items.some((it) => it.key === P.setting.key));
  tab = i >= 0 ? i : 0;
  kb = Math.max(0, (P.groups[tab]?.items || []).findIndex((it) => it.key === P.setting.key));
  box = el("section", "bg-sec");
  draw();
  return box;
}

// Replace only the image choices after background extraction. Keep the live slider
// values, focus and scroll while the player edits settings (06 §2.14, 06 §3.1).
export async function refreshBackgrounds() {
  const section = box;
  if (!section?.isConnected || !P) return;
  const group = P.groups[tab], picked = group?.items[kb]?.key;
  let p;
  try { p = await api.Backgrounds(); } catch { return; }
  if (box !== section || !section.isConnected) return;
  const gridFocus = section.querySelector(".bg-grid")?.contains(document.activeElement);
  P.groups = p.groups;
  tab = Math.max(0, P.groups.findIndex((g) => g.name === group?.name && g.files === group?.files));
  kb = Math.max(0, P.groups[tab].items.findIndex((it) => it.key === picked));
  section.querySelector(".bg-tabs").replaceWith(tabs());
  section.querySelector(".bg-grid").replaceWith(grid());
  section.querySelector(".bg-pv-wrap").replaceWith(preview());
  if (gridFocus) section.querySelector(".bg-grid").focus({ preventScroll: true });
}

const B = () => P.setting;
const item = (key) => { for (const g of P.groups) for (const it of g.items) if (it.key === key) return [it, g]; return [null, null]; };

function draw(focusGrid = false) {
  box.innerHTML = "";
  const card = el("div", "bg-card");
  card.append(el("div", "name bg-title", T("launch page background")));
  const head = el("div", "bg-head");
  head.append(preview(), controls());
  card.append(head, el("div", "bg-rule"), tabs(), grid());
  card.append(el("div", "bg-note", T("background mods are found by name (\"background\" in it) and need not be in a list; add any other image under your images")));
  box.append(card);
  if (focusGrid) box.querySelector(".bg-grid").focus();
}

// preview: the launch page in small — the list down the left and the launch button — with
// the image under the same shade the page uses.
function preview() {
  const w = el("div", "bg-pv-wrap");
  const pv = el("div", "bg-pv");
  const [it, g] = item(B().key);
  const img = el("div", "img");
  if (it && it.thumb) {
    img.style.backgroundImage = `url("${it.thumb}")`;
    img.style.backgroundPosition = B().focus === "left" ? "left center" : B().focus === "right" ? "right center" : "center";
    const blur = B().blur * 0.25; // the preview is about a quarter of the window
    if (blur) { img.style.filter = `blur(${blur}px)`; img.style.inset = `-${blur * 2}px`; }
  } else pv.classList.add("plain");
  const shade = el("div", "shade");
  shade.style.setProperty("--dim", B().dim / 100);
  const ls = el("div", "ls");
  [78, 56, 64, 50].forEach((x, i) => { const b = el("i", i === 0 ? "c" : ""); b.style.width = x + "%"; ls.append(b, el("i", "s")); });
  pv.append(img, shade, ls, el("div", "ln"), el("div", "lb"));
  w.append(pv);
  const cap = el("div", "bg-cap word-facts");
  cap.append(el("span", "", it ? it.label : "—"));
  if (g && !g.files && g !== P.groups[0]) cap.append(el("span", "m",  Tf("from %s", g.name)));
  if (B().rotate && B().stars.length) cap.append(el("span", "m",  T("rotation is on: the launch page picks a starred one")));
  w.append(cap);
  return w;
}

function controls() {
  const c = el("div", "bg-ctl");
  c.append(slider(T("dim"), "dim", 80, (v) => v + "%"), slider(T("blur"), "blur", 20, (v) => String(v)));
  const fr = el("div", "bg-row");
  fr.append(el("span", "k", T("focus")));
  const seg = el("div", "bg-seg");
  for (const [v, l] of [["left", T("left")], ["center", T("center")], ["right", T("right")]]) {
    const b = el("button", v === B().focus ? "on" : "", l);
    b.onclick = () => save({ focus: v });
    seg.append(b);
  }
  fr.append(seg);
  const rr = el("label", "bg-row");
  rr.append(el("span", "k", T("rotation")));
  const cb = el("input");
  cb.type = "checkbox";
  cb.checked = B().rotate;
  cb.onchange = () => save({ rotate: cb.checked });
  const t = el("span", "tg word-facts");
  t.append(cb, el("span", "", T("a different one each time the window opens")));
  t.append(el("small", "", (B().stars.length ? Tf("among the %d starred", B().stars.length) : T("star some with *"))));
  rr.append(t);
  c.append(fr, rr);
  return c;
}

function slider(label, key, max, fmt) {
  const r = el("label", "bg-row");
  r.append(el("span", "k", label));
  const s = el("input");
  s.type = "range";
  s.min = 0; s.max = max; s.value = B()[key];
  const v = el("span", "v num", fmt(B()[key]));
  s.oninput = () => { v.textContent = fmt(+s.value); B()[key] = +s.value; livePreview(); };
  s.onchange = () => save({ [key]: +s.value });
  r.append(s, v);
  return r;
}

// livePreview restyles the preview while a slider moves (saving waits for the release).
function livePreview() {
  const pv = box.querySelector(".bg-pv");
  pv.querySelector(".shade").style.setProperty("--dim", B().dim / 100);
  const img = pv.querySelector(".img");
  const blur = B().blur * 0.25;
  img.style.filter = blur ? `blur(${blur}px)` : "";
  img.style.inset = blur ? `-${blur * 2}px` : "0";
}

function tabs() {
  const t = el("div", "bg-tabs");
  P.groups.forEach((g, i) => {
    const b = el("button", "bg-tab" + (i === tab ? " on" : ""));
    b.append(el("span", "", g.name), el("em", "num", g.files ? (g.items.length ? String(g.items.length) : "+") : String(g.items.length)));
    b.onclick = () => { tab = i; kb = 0; draw(); };
    t.append(b);
  });
  return t;
}

function grid() {
  const g = P.groups[tab];
  const d = el("div", "bg-grid");
  d.tabIndex = 0;
  g.items.forEach((it, i) => d.append(tile(it, i, g)));
  if (g.files) {
    const add = el("button", "bg-tile add");
    add.append(el("div", "th", "+"), el("div", "t", T("add an image…")));
    add.onclick = addFile;
    d.append(add);
  }
  d.onkeydown = gridKey;
  return d;
}

function tile(it, i, g) {
  const sel = it.key === B().key;
  const d = el("div", "bg-tile" + (sel ? " sel" : "") + (i === kb ? " kb" : ""));
  const th = el("div", "th" + (it.thumb ? "" : " plain"));
  if (it.thumb) {
    const im = el("img");
    im.loading = "lazy";
    im.decoding = "async";
    im.src = it.thumb;
    im.alt = "";
    th.append(im);
  }
  if (it.key !== "plain") {
    const st = el("button", "star" + (B().stars.includes(it.key) ? " on" : ""), "★");
    st.title = T("star: rotation picks among the starred") + T("; ") + "*";
    st.onclick = (e) => { e.stopPropagation(); star(it.key); };
    th.append(st);
  }
  if (g.files) {
    const x = el("button", "rm", "×");
    x.title = T("forget this image (the file stays)") + T("; ") + "delete";
    x.onclick = (e) => { e.stopPropagation(); forget(it.key); };
    th.append(x);
  }
  d.append(th, el("div", "t", (sel ? "✓ " : "") + it.label));
  d.title = it.label;
  d.onclick = () => { kb = i; save({ key: it.key }); };
  return d;
}

function gridKey(e) {
  const g = P.groups[tab];
  const n = g.items.length;
  const cols = getComputedStyle(e.currentTarget).gridTemplateColumns.split(" ").length || 1;
  const k = e.key;
  const move = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols }[k];
  if (move) {
    e.preventDefault(); e.stopPropagation();
    if (n) { kb = Math.min(n - 1, Math.max(0, kb + move)); draw(true); }
    return;
  }
  if (k === "[" || k === "]") {
    e.preventDefault(); e.stopPropagation();
    tab = (tab + (k === "]" ? 1 : P.groups.length - 1)) % P.groups.length;
    kb = 0;
    return draw(true);
  }
  const it = g.items[kb];
  if (k === "Enter") { e.preventDefault(); e.stopPropagation(); if (it) save({ key: it.key }, true); else if (g.files) addFile(); return; }
  if (k === "*" && it && it.key !== "plain") { e.preventDefault(); e.stopPropagation(); star(it.key, true); return; }
  if ((k === "Delete" || k === "Backspace") && it && g.files) { e.preventDefault(); e.stopPropagation(); forget(it.key, true); }
}

function star(key, focus = false) {
  const s = B().stars;
  save({ stars: s.includes(key) ? s.filter((x) => x !== key) : [...s, key] }, focus);
}

async function save(change, focus = false) {
  Object.assign(B(), change);
  draw(focus);
  try { await api.SetBackground(B()); } catch (e) { say("fail", errText(e)); }
}

async function addFile() {
  let path = "";
  if (!P.dialogs) {
    path = (window.prompt(T("the image's path (PNG or JPG)")) || "").trim();
    if (!path) return;
  }
  say("", "…");
  try {
    const key = await api.AddBackgroundFile(path);
    if (!key) return say("", "");
    await reload(key);
    say("ok", T("background set"));
  } catch (e) { say("fail", errText(e)); }
}

async function forget(key, focus = false) {
  try { await api.RemoveBackgroundFile(key); } catch (e) { return say("fail", errText(e)); }
  await reload(B().key === key ? null : B().key, focus);
}

async function reload(key, focus = false) {
  P = await api.Backgrounds();
  const i = P.groups.findIndex((g) => g.items.some((it) => it.key === (key || P.setting.key)));
  if (i >= 0 && key) { tab = i; kb = P.groups[i].items.findIndex((it) => it.key === key); }
  kb = Math.max(0, Math.min(kb, P.groups[tab].items.length - 1));
  draw(focus);
}
