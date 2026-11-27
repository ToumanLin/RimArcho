// The looks editor (16 §2.5 Looks view, layer 2): one look, opened from a gallery card.
// Four numbered cards, each doing one thing: 1 which texture (the only place that selects the
// slot), 2 compare (the selected Mod beside what is in game; selects nothing), 3 override
// chain (the only place that selects the Mod), 4 operation (apply and undo for that Mod).
// The view model is view.LookDetail: the axes, each slot's chain, the provider in game, the
// targets an apply can name and their predictions all come from Go.
import { $, el, esc, api, T, Tf, say, errText, slideThumb } from "./state.js";
import { icon } from "./icons.js";
import { choiceControl } from "./choices.js";
import { wireSeparator } from "./separator.js";
import { texURL } from "./looksgrid.js";

const E = {
  ctx: null, shown: false,
  id: "", pos: -1,     // the look, and the selected Mod's position
  d: null,             // view.LookDetail
  slot: "", west: false, // the texture shown; west: the east slot drawn mirrored
  view: "side", zoom: "fit", layer: "texture", split: 50,
  target: 0,           // index into d.targets
  focus: 1, seq: 0,
  changed: false,      // a choice was written: the gallery reads again on the way back
  error: "",
};

const host = () => E.ctx.host;
const num = (n) => Number(n || 0).toLocaleString("en-US");
const bytes = (b) => (b >= 1048576 ? (b / 1048576).toFixed(1) + " MB" : b >= 1024 ? Math.round(b / 1024) + " KB" : b + " B");

// segSwitch: a small two-or-three-way switch with the sliding thumb (as the editor's details |
// tags). paint(on) names the open choice; the labels are English keys, translated when drawn.
export function segSwitch(cls, label, choices, pick) {
  const s = el("span", "seg sub lk-seg " + cls);
  s.setAttribute("role", "group");
  s.setAttribute("aria-label", label);
  s.append(el("span", "thumb"));
  for (const [value] of choices) {
    const b = el("button");
    b.type = "button";
    b.dataset.value = value;
    b.onclick = (e) => { e.stopPropagation(); pick(value); };
    s.append(b);
  }
  s.paint = (on) => {
    choices.forEach(([value, text], i) => {
      const b = s.querySelectorAll("button")[i], open = value === on;
      if (b.textContent !== T(text)) b.textContent = T(text);
      b.classList.toggle("on", open);
      b.setAttribute("aria-pressed", String(open));
    });
    const open = s.querySelector("button.on");
    if (s.isConnected && open && open.offsetWidth) slideThumb(s, open);
  };
  return s;
}

let viewSeg = null, zoomSeg = null, layerSeg = null;

export function initEditor(ctx) {
  E.ctx = ctx;
  const h = ctx.host;
  const card = (cls, n, body) => {
    const s = el("section", "area " + cls);
    s.dataset.area = "looks-e" + n;
    s.append(el("div", "head"), body);
    s.addEventListener("mousedown", () => markFocus(n), true);
    return s;
  };
  const which = el("div", "scroll lk-whichbody");
  which.tabIndex = -1;
  const cmp = el("div", "lk-cmpbody");
  cmp.tabIndex = -1;
  const chain = el("div", "scroll lk-chainbody");
  chain.tabIndex = -1;
  chain.setAttribute("role", "listbox");
  const right = el("div", "lk-ecol");
  right.append(card("lk-chain", 3, chain), card("lk-ops", 4, el("div", "lk-opsbody")));
  const s0 = el("div", "split lk-esplit"), s1 = el("div", "split lk-esplit");
  h.append(card("lk-which", 1, which), s0, card("lk-cmp", 2, cmp), s1, right);
  // the gaps between the three columns are splitters (16 §3.1 rule 10)
  const column = (gap, name, key, node, direction, fallback) => wireSeparator(gap, {
    key, direction,
    get: () => node.getBoundingClientRect().width,
    range: () => [160, Math.max(160, h.getBoundingClientRect().width - 520)],
    set: (v) => h.style.setProperty(name, v + "px"),
    reset: () => h.style.removeProperty(name),
    restore: (v) => { if (+v) h.style.setProperty(name, +v + "px"); },
    changed: () => layoutStages(),
  });
  column(s0, "--lk-e0", "ui.gui.looks.which", $(".lk-which", h), 1);
  column(s1, "--lk-e2", "ui.gui.looks.chain", right, -1);

  viewSeg = segSwitch("lk-viewseg", T("compare"), [["side", "side by side"], ["slider", "slider"]], (v) => { E.view = v; drawCompare(); });
  zoomSeg = segSwitch("lk-zoomseg", T("size"), [["fit", "fit"], ["2", "2x"], ["4", "4x"]], (v) => { E.zoom = v; drawCompare(); });
  layerSeg = segSwitch("lk-layerseg", T("texture or mask"), [["texture", "texture"], ["mask", "mask"]], (v) => { E.layer = v; drawCompare(); });

  which.addEventListener("click", (e) => {
    const t = e.target.closest("[data-slot]");
    if (t) setSlot(t.dataset.slot, t.dataset.west === "1");
  });
  chain.addEventListener("click", (e) => {
    const r = e.target.closest(".lk-cr[data-pos]");
    if (r) selectMod(+r.dataset.pos);
  });
  cmp.addEventListener("click", (e) => {
    const b = e.target.closest(".lk-show");
    if (b) api.LookShow(E.id, +b.dataset.pos, b.dataset.slot).catch((err) => say("fail", errText(err)));
  });
  let frame = 0;
  new ResizeObserver(() => { if (!frame) frame = requestAnimationFrame(() => { frame = 0; if (E.shown) layoutStages(); }); }).observe(cmp);
  ctx.way.addEventListener("click", (e) => { if (e.target.closest(".lk-back")) ctx.back(); });
}

// ---- opening, reading, leaving ----

export function open(id, pos) {
  Object.assign(E, { id, pos, d: null, slot: "", west: false, target: 0, changed: false, error: "", shown: true, focus: 1 });
  draw();
  reload();
}

export function show() { E.shown = true; draw(); }

// hide leaves the looks editor; true when a choice was written while it was open.
export function hide() {
  E.shown = false;
  E.seq++;
  E.ctx.way.replaceChildren();
  const changed = E.changed;
  E.changed = false;
  return changed;
}

export async function reload() {
  if (!E.shown || !E.id) return;
  const n = ++E.seq;
  let d;
  try { d = await E.ctx.reading(() => api.LookDetail(E.id, E.pos, E.ctx.scope())); } catch (e) {
    if (n === E.seq && E.shown) { E.error = errText(e); draw(); }
    return;
  }
  if (n !== E.seq || !E.shown) return;
  E.error = "";
  const open = d.slots.find((s) => s.slot === (E.slot || d.axes.open)) || d.slots[0];
  // no Mod selected yet (opened from the choices or a search): the one in game for the texture shown
  if (d.selected < 0 && open && open.inGame !== E.pos && d.providers.some((p) => p.pos === open.inGame)) {
    E.pos = open.inGame;
    return reload();
  }
  E.d = d;
  if (!d.slots.some((s) => s.slot === E.slot)) { E.slot = open ? open.slot : ""; E.west = false; }
  if (!hasMask()) E.layer = "texture";
  E.target = Math.max(0, Math.min(E.target, d.targets.length - 1));
  draw();
}

const slotNow = () => (E.d ? E.d.slots.find((s) => s.slot === E.slot) : null) || null;
const provider = (pos) => (E.d ? E.d.providers.find((p) => p.pos === pos) : null) || null;
const hasMask = () => !!E.d && E.d.slots.some((s) => s.chain.some((c) => c.mask));

function setSlot(slot, west = false) {
  if (!E.d || !E.d.slots.some((s) => s.slot === slot)) return;
  E.slot = slot;
  E.west = west;
  draw();
}

let picking = 0;
function selectMod(pos, soon = false) {
  if (!E.d || pos === E.pos) return;
  E.pos = pos;
  for (const r of host().querySelectorAll(".lk-cr[data-pos]")) r.classList.toggle("sel", +r.dataset.pos === pos);
  clearTimeout(picking);
  if (soon) picking = setTimeout(reload, 100); else reload();
}

// ---- drawing ----

function draw() {
  drawWay();
  drawWhich();
  drawCompare();
  drawChain();
  drawOps();
  markFocus(E.focus);
  E.ctx.ui.renderWhere();
}

// facing: the words for a slot's place in its look.
const FACING = { south: "south", east: "east", north: "north", west: "west", single: "single" };
const facingWord = (f) => (FACING[f] ? T(FACING[f]) : f || "");
function slotLabel(s) {
  if (!s) return "";
  if (E.d.axes.ground && s.slot === E.d.axes.ground.slot) return T("on the ground");
  if (!s.facing) return s.slot;
  return [s.body, facingWord(s.facing)].filter(Boolean).join(" ");
}

function drawWay() {
  const way = E.ctx.way, d = E.d;
  way.replaceChildren();
  if (!E.shown) return;
  const back = el("button", "lk-back", T("gallery"));
  back.type = "button";
  back.title = T("back to the gallery") + "  esc";
  way.append(back, el("span", "slash", "/"));
  if (!d) return;
  const ttl = el("b", "ttl", d.title);
  ttl.title = d.id;
  way.append(ttl, el("span", "meta", d.providers.length === 1 ? T("1 Mod") : Tf("%d Mods", d.providers.length)));
  if (d.kind === "worn") way.append(el("span", "meta", T("worn")));
  if (d.kind === "folder") way.append(el("span", "meta", T("folder")));
  const defs = d.defs || [];
  const used = el("span", "meta", defs.length ? Tf("used by %s", defs[0].label || defs[0].name) + (defs.length > 1 ? "  +" + (defs.length - 1) : "") : T("no def"));
  used.title = defs.length ? defs.map((x) => `${x.type} ${x.name} (${x.field})` + (x.mod ? ", " + x.mod : "")).join("\n") : T("no Def names this texture path: an interface icon, or loaded by code or a patch");
  way.append(used);
}

// thumb: a slot's texture in a fixed box: the provider's file, or its format where no
// picture can be made of it.
function thumb(slot, pos, size, mirror = false) {
  const row = slot ? slot.chain.find((c) => c.pos === pos) : null;
  if (!row) return `<span class="lk-t none" style="width:${size}px;height:${size}px"></span>`;
  if (!row.file.preview) return `<span class="lk-t nopre" style="width:${size}px;height:${size}px">${esc(row.file.format)}</span>`;
  return `<span class="lk-t${mirror ? " mirror" : ""}" style="width:${size}px;height:${size}px"><img src="${esc(texURL(E.id, pos, slot.slot, 96, E.d.stamp))}" loading="lazy" decoding="async" alt=""></span>`;
}

const slotBy = (name) => E.d.slots.find((s) => s.slot === name) || null;
const count = (s) => `<span class="lk-c${s && s.contested ? " many" : ""}" title="${esc(s ? (s.chain.length === 1 ? T("1 Mod supplies it") : Tf("%d Mods supply it", s.chain.length)) : "")}">${s ? s.chain.length : 0}</span>`;

// bodyNow: the entry of axes.bodies the shown texture belongs to (null on the ground).
function bodyNow() {
  const ax = E.d.axes;
  if (ax.ground && ax.ground.slot === E.slot) return null;
  return (ax.bodies || []).find((b) => b.facings.some((f) => f.slot === E.slot)) || null;
}
// sameFacing: the entry of body b for the facing shown now, else its first.
function sameFacing(b) {
  const now = slotNow(), want = E.west ? "west" : now ? now.facing : "";
  return b.facings.find((f) => f.facing === want) || b.facings[0];
}

// drawWhich: the axes this look has (view.LookAxes): worn condition, body type, facings as
// tiles, a folder's files. Nothing else in the page changes the texture shown.
function drawWhich() {
  const card = $(".lk-which", host()), d = E.d;
  $(".head", card).innerHTML = `<span class="n">1</span><span class="t">${esc(T("which texture"))}</span>` + (d ? `<span class="num">${num(d.slots.length)}</span>` : "");
  const body = $(".lk-whichbody", card);
  if (!d) { body.replaceChildren(); return; }
  const ax = d.axes, out = [], here = bodyNow();
  const entry = (label, f, on, title = "") => {
    const s = slotBy(f.slot);
    return `<button type="button" class="lk-ent${on ? " on" : ""}" data-slot="${esc(f.slot)}"${f.mirrored ? ' data-west="1"' : ""} aria-pressed="${on}" title="${esc(title || label)}">` +
      `${thumb(s, s ? s.inGame : -1, 28, !!f.mirrored)}<span class="nm">${esc(label)}</span>${count(s)}</button>`;
  };
  const bodies = ax.bodies || [];
  if (ax.ground && bodies.length) {
    const worn = sameFacing(here || bodies[0]);
    out.push(`<div class="lk-axh">${esc(T("worn condition"))}</div><div class="lk-ax">` +
      entry(T("on the ground"), ax.ground, !here) + entry(T("worn"), worn, !!here) + `</div>`);
  }
  if (here && (bodies.length > 1 || bodies[0].body)) {
    out.push(`<div class="lk-axh">${esc(T("body type"))}</div><div class="lk-ax">` +
      bodies.map((b) => entry(b.body || T("any body"), sameFacing(b), b === here)).join("") + `</div>`);
  }
  if (here && here.facings.length) {
    out.push(`<div class="lk-axh">${esc(T("facing"))}</div><div class="lk-ori${here.facings.length < 3 ? " big" : ""}">` + here.facings.map((f) => {
      const s = slotBy(f.slot), on = f.slot === E.slot && !!f.mirrored === E.west;
      const sub = f.mirrored ? T("mirrored") : s ? (s.chain.length === 1 ? T("1 Mod") : Tf("%d Mods", s.chain.length)) : "";
      return `<button type="button" class="lk-tile${on ? " on" : ""}" data-slot="${esc(f.slot)}"${f.mirrored ? ' data-west="1"' : ""} aria-pressed="${on}" ` +
        `title="${esc(f.mirrored ? T("no Mod ships west: the game draws east mirrored") : f.slot)}">${thumb(s, s ? s.inGame : -1, here.facings.length < 3 ? 84 : 40, !!f.mirrored)}` +
        `<span class="l1">${esc(facingWord(f.facing))}</span><span class="l2${s && s.contested && !f.mirrored ? " many" : ""}">${esc(sub)}</span></button>`;
    }).join("") + `</div>`);
  }
  if ((ax.files || []).length) {
    out.push(`<div class="lk-axh">${esc(T("files"))}</div><div class="lk-ax">` +
      ax.files.map((f) => entry(f.slot, f, f.slot === E.slot)).join("") + `</div>`);
  }
  body.innerHTML = out.join("");
}

// panes: the selected Mod's file and what stands beside it: the one in game, or, when the
// selected Mod is the one in game, the one it covers (the last other Mod of the chain).
function panes() {
  const s = slotNow();
  if (!s) return null;
  const sel = E.d.selected;
  const a = { pos: sel, row: s.chain.find((c) => c.pos === sel) || null, role: "sel", inGame: s.inGame === sel };
  let b = null;
  if (s.inGame !== sel) b = { pos: s.inGame, row: s.chain.find((c) => c.pos === s.inGame) || null, role: "game", byChoice: !!s.byChoice };
  else {
    const others = s.chain.filter((c) => c.pos !== sel);
    if (others.length) b = { pos: others[others.length - 1].pos, row: others[others.length - 1], role: "covered" };
  }
  return { s, a, b };
}

// shownFile: what a pane draws for the layer chosen: the texture, or its mask (the game's
// own choice of mask for the pane in game, view.LookSlot.MaskInGame).
function shownFile(s, p) {
  if (!p || !p.row) return null;
  if (E.layer !== "mask") return { pos: p.pos, slot: s.slot, file: p.row.file };
  if (p.role === "game") {
    const r = s.maskInGame >= 0 ? s.chain.find((c) => c.pos === s.maskInGame) : null;
    return r && r.mask ? { pos: r.pos, slot: s.slot + "_mask", file: r.mask, from: r.pos !== p.pos ? r.pos : -1 } : { none: true };
  }
  return p.row.mask ? { pos: p.pos, slot: s.slot + "_mask", file: p.row.mask } : { none: true };
}

function picture(f) {
  if (!f) return "";
  if (f.none) return `<div class="lk-nothing">${esc(T("no mask for this texture"))}</div>`;
  if (!f.file.preview) return `<div class="lk-nothing"><b>${esc(f.file.format)}</b><span>${esc(T("no preview of this format"))}</span></div>`;
  return `<div class="lk-box${E.west ? " mirror" : ""}"><img src="${esc(texURL(E.id, f.pos, f.slot, E.zoom === "fit" ? 512 : 0, E.d.stamp))}" decoding="async" alt=""></div>`;
}

function tag(p, right) {
  const m = provider(p.pos), name = m ? m.name : "";
  if (p.role === "sel") return `<span class="lk-vtag"><span class="nm">${esc(name)}</span>${p.inGame ? `<b class="ig">${esc(T("in game"))}</b>` : ""}</span>`;
  if (p.role === "covered") return `<span class="lk-vtag cov${right ? " r" : ""}"><span class="nm">${esc(name)}</span><span>${esc(T("covered"))}</span></span>`;
  return `<span class="lk-vtag${right ? " r" : ""}"><b class="ig">${esc(T("in game"))}</b>${p.byChoice ? `<span>${esc(T("your choice"))}</span>` : ""}</span>`;
}

// info: under a texture, left-aligned: the Mod, the file's path on its own wrapping line (never
// cut), then its size in pixels, format and bytes and show in explorer.
function info(s, p, f) {
  const m = provider(p.pos);
  const head = `<div class="l1"><span class="nm" title="${esc(m ? m.name : "")}">${esc(m ? m.name : "")}</span><span class="no">#${m ? m.no : ""}</span>`;
  if (!f || f.none || !p.row) {
    return `<div class="lk-info">${head}</div><div class="l3"><span>${esc(!p.row ? T("no file for this texture") : T("no mask for this texture"))}</span></div></div>`;
  }
  const file = f.file, same = file.sameAs >= 0 ? provider(file.sameAs) : null, from = f.from >= 0 ? provider(f.from) : null;
  return `<div class="lk-info">${head}` +
    (same ? `<span class="lk-nt" title="${esc(same.name)}">${esc(Tf("same file as #%d", same.no))}</span>` : "") +
    (from ? `<span class="lk-nt warn" title="${esc(from.name)}">${esc(Tf("mask from #%d", from.no))}</span>` : "") + `</div>` +
    `<div class="l2">${esc(file.rel)}${E.west ? ` <span class="mirn">${esc(T("mirrored for west"))}</span>` : ""}</div>` +
    `<div class="l3"><span>${file.w}×${file.h}</span><span class="lk-chip">${esc(file.format)}</span>` +
    (file.alt ? `<span class="lk-chip" title="${esc(Tf("this Mod also ships it as %s", file.alt))}">+${esc(file.alt)}</span>` : "") +
    `<span>${esc(bytes(file.bytes))}</span><button type="button" class="btn small lk-show" data-pos="${f.pos}" data-slot="${esc(f.slot)}">${esc(T("show in explorer"))}</button></div></div>`;
}

function drawCompare() {
  const card = $(".lk-cmp", host()), d = E.d, head = $(".head", card), body = $(".lk-cmpbody", card);
  if (!head.firstChild) {
    head.innerHTML = `<span class="n">2</span><span class="t">${esc(T("compare"))}</span>`;
    const acts = el("span", "acts");
    acts.append(viewSeg, zoomSeg, layerSeg);
    head.append(acts);
  }
  $(".t", head).textContent = T("compare");
  if (E.error) { body.innerHTML = `<div class="lk-nothing bad">${esc(E.error)}</div>`; return; }
  const ps = d ? panes() : null;
  viewSeg.hidden = !ps || !ps.b || !ps.a.row;
  layerSeg.hidden = !hasMask();
  viewSeg.paint(E.view);
  zoomSeg.paint(E.zoom);
  layerSeg.paint(E.layer);
  if (!ps) { body.innerHTML = d ? `<div class="lk-nothing">${esc(T("this look has no texture"))}</div>` : ""; return; }
  const { s, a, b } = ps, fa = shownFile(s, a), fb = shownFile(s, b), who = provider(a.pos);
  const none = `<div class="lk-nothing">${esc(Tf("%s has no file for this texture", who ? who.name : ""))}</div>`;
  const view = (f, p, right) => `<div class="lk-vpw"><div class="lk-vp" tabindex="-1">${!p.row ? none : f.none || !f.file.preview ? picture(f) : `<div class="lk-stage">${picture(f)}</div>`}</div>${tag(p, right)}</div>`;
  if (b && a.row && E.view === "slider" && fa && fb && !fa.none && !fb.none && fa.file.preview && fb.file.preview) {
    body.className = "lk-cmpbody one";
    body.innerHTML = `<div class="lk-pane"><div class="lk-vpw"><div class="lk-vp" tabindex="-1"><div class="lk-stage"><div class="lk-sl">` +
      `${picture(fb)}<div class="top" style="clip-path:inset(0 ${100 - E.split}% 0 0)">${picture(fa)}</div>` +
      `<div class="hit"></div><button type="button" class="knob" role="slider" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${E.split}" aria-label="${esc(T("where the two textures meet"))}" style="left:${E.split}%"></button>` +
      `</div></div></div>${tag(a)}${tag(b, true)}</div><div class="lk-two">${info(s, a, fa)}${info(s, b, fb)}</div></div>`;
    wireSlider(body);
  } else {
    const shape = (fa && fa.file) || (fb && fb.file);
    const wide = b && shape && shape.w / Math.max(1, shape.h) > 1.3;
    body.className = "lk-cmpbody" + (b ? (wide ? " rows" : "") : " one");
    body.innerHTML = `<div class="lk-pane">${view(fa, a)}${info(s, a, fa)}</div>` + (b ? `<div class="lk-pane">${view(fb, b, false)}${info(s, b, fb)}</div>` : "");
  }
  layoutStages();
}

// layoutStages gives both textures one box: the texture's shape, as large as fits (fit), or
// two or four times that; a zoomed box scrolls, and the two scroll together.
function layoutStages() {
  const body = $(".lk-cmpbody", host());
  if (!body || !E.d) return;
  const vps = [...body.querySelectorAll(".lk-vp")].filter((v) => v.querySelector(".lk-stage"));
  if (!vps.length) return;
  const ps = panes(), s = ps && ps.s;
  const f = ps && ((ps.a.row && shownFile(s, ps.a)) || shownFile(s, ps.b));
  const file = f && f.file ? f.file : null;
  const ar = file && file.w && file.h ? file.w / file.h : 1, k = { fit: 1, 2: 2, 4: 4 }[E.zoom] || 1;
  const w = Math.min(...vps.map((v) => v.clientWidth)) - 16, h = Math.min(...vps.map((v) => v.clientHeight)) - 16;
  if (w <= 0 || h <= 0) return;
  let bw = Math.min(w, h * ar), bh = bw / ar;
  bw = Math.floor(bw * k); bh = Math.floor(bh * k);
  for (const v of vps) {
    v.classList.toggle("zoomed", k > 1);
    const st = v.querySelector(".lk-stage");
    st.style.width = bw + "px";
    st.style.height = bh + "px";
    st.classList.toggle("px", !!file && bw > file.w);
    if (k > 1) { v.scrollLeft = (v.scrollWidth - v.clientWidth) / 2; v.scrollTop = (v.scrollHeight - v.clientHeight) / 2; }
  }
  if (vps.length === 2) {
    let lock = false;
    const tie = (a, b) => { a.onscroll = () => { if (lock) return; lock = true; b.scrollLeft = a.scrollLeft; b.scrollTop = a.scrollTop; requestAnimationFrame(() => { lock = false; }); }; };
    tie(vps[0], vps[1]); tie(vps[1], vps[0]);
  }
}

function setSplit(x, body) {
  E.split = Math.max(0, Math.min(100, Math.round(x)));
  const top = $(".lk-sl .top", body), knob = $(".lk-sl .knob", body);
  if (top) top.style.clipPath = `inset(0 ${100 - E.split}% 0 0)`;
  if (knob) { knob.style.left = E.split + "%"; knob.setAttribute("aria-valuenow", String(E.split)); }
}
function wireSlider(body) {
  const hit = $(".lk-sl .hit", body), knob = $(".lk-sl .knob", body);
  const at = (ev) => { const r = hit.getBoundingClientRect(); setSplit(100 * (ev.clientX - r.left) / Math.max(1, r.width), body); };
  hit.onpointerdown = (ev) => { hit.setPointerCapture(ev.pointerId); at(ev); hit.onpointermove = at; };
  hit.onpointerup = () => { hit.onpointermove = null; };
  // the knob is a slider: its own arrows move it, as a native one's do
  knob.onkeydown = (ev) => {
    const d = { ArrowLeft: -2, ArrowRight: 2, Home: -100, End: 100 }[ev.key];
    if (!d) return;
    ev.preventDefault(); ev.stopPropagation();
    setSplit(E.split + d, body);
  };
}

const CHOICE = { applied: "applied", "not needed": "not needed", asleep: "asleep", broken: "broken" };

// drawChain: the Mods that supply the look, in load order; a row only selects its Mod. Then
// _Archo_Looks with this look's choices and their states.
function drawChain() {
  const card = $(".lk-chain", host()), d = E.d, body = $(".lk-chainbody", card);
  $(".head", card).innerHTML = `<span class="n">3</span><span class="t" id="lk-chain-t">${esc(T("override chain"))}</span><span class="q">${esc(T("load order"))}</span>` +
    (d ? `<span class="num">${esc(d.providers.length === 1 ? T("1 Mod") : Tf("%d Mods", d.providers.length))}</span>` : "");
  body.setAttribute("aria-labelledby", "lk-chain-t");
  if (!d) { body.replaceChildren(); return; }
  const s = slotNow(), top = body.scrollTop, total = d.slots.length;
  const rows = d.providers.map((p) => {
    const has = !!s && s.chain.some((c) => c.pos === p.pos), game = !!s && s.inGame === p.pos, sel = p.pos === d.selected;
    const supplies = !has ? T("not this texture") : p.slots.length === total ? (total === 1 ? T("this texture") : Tf("all %d textures", total)) : Tf("%d of %d textures", p.slots.length, total);
    return `<div class="lk-cr${sel ? " sel" : ""}" role="option" aria-selected="${sel}" id="lk-cr-${p.pos}" data-pos="${p.pos}" title="${esc(p.name)}">` +
      `${thumb(s, p.pos, 32, E.west)}<div class="who"><div class="nm">${esc(p.name)}</div>` +
      `<div class="ln2"><span class="pos">#${p.no}</span><span class="sub">${esc(supplies)}</span></div></div>` +
      `<span class="act">${game ? `<b class="ig">${esc(T("in game"))}</b>` : ""}</span></div>`;
  }).join("");
  const choices = d.choices || [];
  const lines = choices.length ? choices.map((c) => {
    const what = c.slot === "all" ? T("entire look") : slotLabel(slotBy(c.slot)) || c.slot;
    const state = c.overruled ? T("overruled by another choice") : T(CHOICE[c.state] || c.state);
    return `<div class="ln2"><span class="dec">${esc(Tf("%s from %s", what, c.mod || "—"))}</span><span class="lk-chip">${esc(E.ctx.scopeWord(c.scope))}</span>` +
      `<span class="st ${c.state === "broken" ? "bad" : c.state === "asleep" || c.overruled ? "dim" : ""}">${esc(state)}</span></div>`;
  }).join("") : `<div class="ln2"><span class="sub">${esc(T("no choice: the load order decides"))}</span></div>`;
  const ov = E.ctx.overview(), by = s && s.byChoice;
  body.innerHTML = (d.mixed ? `<p class="lk-warn">${icon.warn}<span>${esc(T("in game now: the look takes textures from more than one Mod"))}</span></p>` : "") +
    rows + `<div class="lk-sep"></div>` +
    `<div class="lk-cr gen">${by ? thumb(s, s.inGame, 32, E.west) : `<span class="lk-t none" style="width:32px;height:32px"></span>`}` +
    `<div class="who"><div class="nm lk-code">${esc((ov && ov.generated.name) || "_Archo_Looks")}</div>${lines}</div><span class="act"></span></div>`;
  body.scrollTop = top;
  const at = $(".lk-cr.sel", body);
  if (at) body.setAttribute("aria-activedescendant", at.id); else body.removeAttribute("aria-activedescendant");
}

function targetLabel(t) {
  if (t.kind === "all") return T("entire look");
  if (t.kind === "worn") return T("all worn textures");
  if (t.kind === "ground") return T("texture on the ground");
  if (t.kind === "body") return Tf("body type %s", t.body);
  return Tf("only %s", [t.body, t.facing ? facingWord(t.facing) : (t.slots || [])[0]].filter(Boolean).join(" "));
}

// drawOps: the operation card: apply the selected Mod's textures in game, for what the
// dropdown names, with Go's prediction under it; undo this look's choice in the scope.
function drawOps() {
  const card = $(".lk-ops", host()), d = E.d, body = $(".lk-opsbody", card), scope = E.ctx.scope();
  $(".head", card).innerHTML = `<span class="n">4</span><span class="t">${esc(T("operation"))}</span>` +
    (d ? `<span class="q" title="${esc(d.id)}">${esc(Tf("for %s", d.title))}</span><span class="num">${esc(scope === "all" ? T("for all lists") : T("for this list"))}</span>` : "");
  body.replaceChildren();
  if (!d) return;
  const m = provider(d.selected), ov = E.ctx.overview();
  const sel = el("div", "lk-osel");
  sel.append(el("span", "k", T("selected")), el("span", "v", m ? m.name : "—"));
  if (m) sel.lastChild.title = m.name;
  body.append(sel);

  const op = el("div", "lk-op");
  op.append(el("div", "oh", T("apply in game")));
  const row = el("div", "or");
  const t = d.targets[E.target] || d.targets[0];
  const pick = choiceControl(T("apply in game"), d.targets.map((x, i) => ({ value: String(i), label: targetLabel(x) })), String(E.target));
  pick.classList.add("lk-target");
  pick.onchange = () => { E.target = +pick.value; drawOps(); $(".lk-target", host())?.focus(); };
  const p = t && t.preview, out = p ? p.outcome : "none";
  const go = el("button", "btn primary lk-apply", T("apply"));
  go.type = "button";
  go.dataset.envWrite = "looks";
  go.disabled = !m || !t || out === "none" || out === "inGame";
  go.onclick = () => apply();
  row.append(pick, go);
  op.append(row);
  const say1 = (text, cls) => {
    const w = el("p", "ow " + cls);
    if (cls === "warn") w.innerHTML = icon.warn;
    w.append(el("span", "", text));
    op.append(w);
  };
  if (!m) say1(T("select a Mod in the override chain"), "quiet");
  else if (out === "none") say1(Tf("%s has no texture for it.", m.name), "warn");
  else if (out === "inGame") say1(T("The game already shows this."), "quiet");
  else if (out === "overruled") say1(T("A choice for this list decides these textures: one for all lists would not show."), "warn");
  else if (out === "mixed") say1(p.text || T("Mixed result: some textures of the look still come from another Mod."), "warn");
  if (d.kind === "folder" && m) say1(T("A folder's files are merged across Mods: this replaces the files both Mods name alike, and files only other Mods add stay."), "quiet");
  if (ov && ov.gameRunning) say1(T("A game is running: the choice is stored now, and _Archo_Looks is rebuilt at the next write to the game."), "warn");
  body.append(op);

  const last = el("div", "lk-op");
  const line = el("div", "or");
  const mine = (d.choices || []).some((c) => c.scope === scope), other = (d.choices || []).some((c) => c.scope !== scope);
  line.append(el("span", "otx", mine ? Tf("this look has a choice for %s", E.ctx.scopeWord(scope))
    : other ? Tf("no choice for %s; there is one for %s", E.ctx.scopeWord(scope), E.ctx.scopeWord(scope === "all" ? "plan" : "all")) : T("no choice made for this look")));
  const undo = el("button", "btn lk-undo", T("undo"));
  undo.type = "button";
  undo.dataset.envWrite = "looks";
  undo.disabled = !d.canUndo;
  undo.onclick = () => undoChoice();
  line.append(undo);
  last.append(line);
  body.append(last);
}

function apply() {
  const d = E.d, t = d.targets[E.target], m = provider(d.selected);
  if (!t || !m) return;
  const scope = E.ctx.scope(), id = d.id, slots = t.slots || [];
  E.ctx.gate({
    preview: () => api.LooksApplyPreview([id], m.ref, slots, scope),
    title: T("apply this look in game?"),
    facts: () => [[T("look"), d.title], [T("from"), m.name], [T("what"), targetLabel(t)], [T("for"), E.ctx.scopeWord(scope)]],
    run: async () => {
      let r;
      try { r = await api.LooksApply(id, m.ref, slots, scope); } catch (e) { return say("fail", errText(e)); }
      E.changed = true;
      E.ctx.wrote(r);
      reload();
    },
  });
}

async function undoChoice() {
  const d = E.d;
  if (!d || !d.canUndo) return;
  let r;
  try { r = await api.LooksUndo(d.id, E.ctx.scope()); } catch (e) { return say("fail", errText(e)); }
  E.changed = true;
  E.ctx.wrote(r);
  reload();
}

// ---- focus and keys (16 §3.5) ----

function markFocus(n) {
  E.focus = n;
  [".lk-which", ".lk-cmp", ".lk-chain", ".lk-ops"].forEach((c, i) => $(c, host())?.classList.toggle("focus", i + 1 === n));
}

function focusCard(n) {
  markFocus(n);
  const h = host();
  if (n === 1) $(".lk-whichbody", h).focus({ preventScroll: true });
  else if (n === 2) $(".lk-cmpbody", h).focus({ preventScroll: true });
  else if (n === 3) $(".lk-chainbody", h).focus({ preventScroll: true });
  else ($(".lk-target", h) || $(".lk-opsbody", h)).focus({ preventScroll: true });
  E.ctx.ui.renderHints();
}

const step = (list, at, d) => (list.length ? list[(Math.max(0, at) + d + list.length) % list.length] : null);

// moveFacing / moveGroup: the arrows of cards 1 and 2 walk the axes; they only change what
// is shown.
function moveFacing(d) {
  const b = bodyNow();
  if (!b || b.facings.length < 2) return false;
  const at = b.facings.findIndex((f) => f.slot === E.slot && !!f.mirrored === E.west), f = step(b.facings, at, d);
  setSlot(f.slot, !!f.mirrored);
  return true;
}
function moveGroup(d) {
  const ax = E.d.axes;
  if ((ax.files || []).length) {
    const f = step(ax.files, ax.files.findIndex((x) => x.slot === E.slot), d);
    setSlot(f.slot);
    return true;
  }
  const b = bodyNow(), list = [...(ax.ground && (ax.bodies || []).length ? [null] : []), ...(ax.bodies || [])];
  if (list.length < 2) return false;
  const next = step(list, list.indexOf(b), d);
  if (!next) setSlot(ax.ground.slot);
  else { const f = sameFacing(next); setSlot(f.slot, !!f.mirrored); }
  return true;
}

export function key(e) {
  const k = e.key, ctrl = e.ctrlKey || e.metaKey;
  const take = () => { e.preventDefault(); return true; };
  if (k === "Escape") { E.ctx.back(); return take(); }
  if (!ctrl && !e.altKey && "1234".includes(k) && k.length === 1) { focusCard(+k); return take(); }
  if (!E.d || !k.startsWith("Arrow")) return k === "Enter" || k === " " || k === "Home" || k === "End" || k.startsWith("Page");
  const d = k === "ArrowDown" || k === "ArrowRight" ? 1 : -1, vertical = k === "ArrowDown" || k === "ArrowUp";
  if (E.focus === 4) return true;
  if (E.focus === 3) {
    if (!vertical) return take();
    const list = E.d.providers, next = step(list, list.findIndex((p) => p.pos === E.pos), d);
    if (next) { selectMod(next.pos, true); $(`#lk-cr-${next.pos}`, host())?.scrollIntoView({ block: "nearest" }); }
    return take();
  }
  if (E.focus === 2 && E.zoom !== "fit") {
    for (const v of host().querySelectorAll(".lk-vp.zoomed")) { if (vertical) v.scrollTop += d * 40; else v.scrollLeft += d * 40; }
    return take();
  }
  if (vertical) moveGroup(d); else moveFacing(d);
  return take();
}

export function hintList() {
  if (E.focus === 3) return [
    { key: "↑↓", label: T("select a Mod") }, { key: "1–4", label: T("cards") }, { key: "esc", label: T("gallery") }, { key: "?", label: T("all keys") },
  ];
  return [
    { key: "←→", label: T("facing") }, { key: "↑↓", label: T("body type or file") }, { key: "1–4", label: T("cards") },
    { key: "esc", label: T("gallery") }, { key: "?", label: T("all keys") },
  ];
}

export function where() {
  const d = E.d, s = slotNow();
  return `${T("looks")} › ${d ? d.title : "…"}` + (s ? "    " + (E.west ? T("west") + " " + T("mirrored") : slotLabel(s)) : "");
}

export function helpRows() {
  return [
    ["1–4", T("focus a card: which texture, compare, override chain, operation")],
    ["←→", T("the next or previous facing")],
    ["↑↓", T("the next or previous body type or file; in the override chain: select a Mod")],
    ["tab", T("reach the switches, the list of what to apply, apply and undo")],
    ["esc", T("back to the gallery")],
    ["w  L  u", T("save, launch, undo the list's last edit")],
  ];
}
