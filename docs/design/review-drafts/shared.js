// Play review drafts (16 §2.11, 12 §3). Example data only; nothing reads or writes files.
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const IC = {
  chev: '<path d="M6 3.5 10.5 8 6 12.5"/>', down: '<path d="M3.5 6 8 10.5 12.5 6"/>', x: '<path d="M4 4l8 8M12 4l-8 8"/>',
  theme: '<circle cx="8" cy="8" r="5.6"/><path d="M8 2.4v11.2A5.6 5.6 0 0 0 8 2.4z" fill="currentColor"/>',
  search: '<circle cx="7" cy="7" r="4.2"/><path d="m10.2 10.2 3.3 3.3"/>',
};
const ic = (n, cls = "") => `<svg class="i ${cls}" viewBox="0 0 16 16">${IC[n]}</svg>`;

// ---- example data ----
// A category: key, label, the label for one.
const CATS = [
  ["buildings", "buildings"], ["weapons", "weapons"], ["apparel", "apparel"], ["items", "items"], ["terrain", "terrain"],
  ["animals", "animals"], ["races", "humanlike races"], ["factions", "factions"], ["research", "research projects"],
  ["events", "events"], ["quests", "quests"], ["genes", "xenotypes and genes"], ["culture", "culture"],
  ["abilities", "abilities"], ["hediffs", "health conditions"], ["appearance", "appearance"], ["biomes", "biomes"],
];
const CAT_LABEL = Object.fromEntries(CATS);
// The player's tags (07 §6), with the library's tag colours.
const TAGS = [["furniture", "orange"], ["decor", "pink"], ["combat", "blue"], ["races", "plum"], ["story", "violet"], ["world", "teal"], ["style", "grass"], ["QoL", "cyan"]];
const TAG_COLOR = Object.fromEntries(TAGS);

let seed = 7;
const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
const pick = (n) => Math.floor(rnd() * n);
// name, defName prefix, tags, [category, adds, share seen]…
const SPEC = [
  ["Vanilla Furniture Expanded - Props and Decor", "VFEPD", ["furniture", "decor"], ["buildings", 1639, .05]],
  ["Flooring Floors", "KS", ["decor"], ["terrain", 403, .005], ["buildings", 2, 0]],
  ["Alpha Props - Parks and Gardens", "AP", ["decor"], ["buildings", 190, 0], ["terrain", 6, 0]],
  ["Ancient Urban Ruins", "AUR", ["world", "story"], ["buildings", 271, 0], ["weapons", 35, 0], ["apparel", 10, .4], ["items", 75, .07], ["factions", 2, .5], ["quests", 5, .2], ["hediffs", 8, 0], ["biomes", 1, 0]],
  ["Vanilla Quests Expanded - Ancients", "VQEA", ["story"], ["buildings", 108, 0], ["genes", 33, .12], ["appearance", 27, .11], ["hediffs", 16, 0], ["quests", 6, .17], ["abilities", 6, .17], ["animals", 4, 0]],
  ["Vanilla Quests Expanded - Deadlife", "VQED", ["story"], ["buildings", 99, 0], ["appearance", 52, .04], ["weapons", 6, 0], ["apparel", 3, 1], ["quests", 2, .5], ["biomes", 1, 1]],
  ["Outland - More Floors", "Outland", ["decor"], ["terrain", 131, 0]],
  ["Garage Doors Expanded", "GDE", ["furniture"], ["buildings", 64, 0], ["research", 2, .5]],
  ["Small Furniture", "SF", ["furniture"], ["buildings", 51, .08], ["culture", 5, 0]],
  ["Greekcore Style", "Greekcore", ["style"], ["culture", 82, .1], ["buildings", 4, .25], ["terrain", 5, .2]],
  ["Vanilla Ideology Expanded - Sophian Style", "VIES", ["style"], ["culture", 86, .3]],
  ["Vanilla Weapons Expanded", "VWE", ["combat"], ["weapons", 46, .55], ["research", 3, 1], ["items", 4, .5]],
  ["Vanilla Armour Expanded", "VAE", ["combat"], ["apparel", 38, .4], ["research", 2, 1]],
  ["Laser Weapons: Re-Examined", "LWR", ["combat"], ["weapons", 2, 0], ["buildings", 2, 0], ["research", 1, 0]],
  ["Melee Animation", "AM", ["combat"], ["weapons", 1, 0], ["apparel", 3, 0], ["buildings", 1, 0], ["hediffs", 1, 0]],
  ["Alpha Animals", "AA", ["world"], ["animals", 94, .35], ["items", 31, .3], ["hediffs", 22, .2], ["events", 6, .5]],
  ["Vanilla Animals Expanded", "VAEx", ["world"], ["animals", 58, .6], ["items", 9, .7]],
  ["Alpha Biomes", "AB", ["world"], ["biomes", 9, .11], ["animals", 31, .1], ["terrain", 24, .08], ["buildings", 12, 0]],
  ["Vanilla Factions Expanded - Medieval", "VFEM", ["world", "story"], ["factions", 4, 1], ["weapons", 21, .6], ["apparel", 24, .5], ["buildings", 30, .3], ["research", 8, .75]],
  ["Vanilla Factions Expanded - Pirates", "VFEP", ["world", "story"], ["factions", 3, 1], ["weapons", 14, .5], ["apparel", 17, .35], ["genes", 12, .25], ["quests", 4, .25]],
  ["Kurin, the Three Tailed Fox", "Kurin", ["races"], ["races", 1, 1], ["apparel", 29, .6], ["weapons", 11, .45], ["appearance", 34, .5], ["factions", 2, 1], ["buildings", 6, .5]],
  ["Miho, the Celestial Fox", "Miho", ["races"], ["races", 1, 1], ["apparel", 41, .3], ["weapons", 18, .2], ["buildings", 22, .1], ["research", 9, .3], ["factions", 2, 1]],
  ["Ratkin", "Ratkin", ["races"], ["races", 1, 0], ["apparel", 36, 0], ["weapons", 12, 0], ["factions", 2, 0], ["appearance", 20, 0]],
  ["Vanilla Events Expanded", "VEE", ["story"], ["events", 38, .7], ["quests", 6, .3]],
  ["Go Explore!", "GE", ["story", "world"], ["events", 12, .4], ["quests", 9, .2]],
  ["Simple Warrants", "SW", ["story"], ["quests", 4, 0], ["events", 1, 0]],
  ["Vanilla Vehicles Expanded - Tier 3", "VVE", ["combat"], ["buildings", 13, 0], ["research", 11, 0]],
  ["Vanilla Furniture Expanded - Medical", "VFEMed", ["furniture"], ["buildings", 6, 0], ["hediffs", 1, 0]],
  ["Vanilla Nutrient Paste Expanded", "VNPE", ["furniture"], ["buildings", 8, 0]],
  ["Warehouse Storage", "Vin", ["furniture"], ["buildings", 12, .08]],
  ["Appliances Expanded", "AE", ["furniture"], ["buildings", 24, .04], ["items", 2, 0]],
  ["[T] MoreFloors", "TMF", ["decor"], ["terrain", 80, .03], ["items", 1, 1], ["buildings", 1, 0]],
  ["Architecture of the Rim", "AR", ["decor"], ["buildings", 17, 0]],
  ["Royal Carpets", "JF", ["decor"], ["buildings", 5, 0]],
  ["Dresses in the Rim", "DR", ["style"], ["apparel", 3, 0]],
  ["Layers: Capes and Armor", "LCA", ["style", "combat"], ["apparel", 3, 0]],
  ["Prisoner Jumpsuits", "PJ", [], ["apparel", 4, 0]],
  ["Character Editor", "SZ", ["QoL"], ["genes", 18, 0]],
  ["Vanilla Expanded Framework", "VEF", [], ["buildings", 2, 0], ["terrain", 6, 0], ["abilities", 3, 0], ["hediffs", 9, 0], ["events", 1, 0], ["items", 1, 0]],
  ["Mechanitor Encounters", "ME", ["story"], ["factions", 1, 1], ["hediffs", 47, .02], ["buildings", 8, 0], ["items", 4, .25], ["quests", 1, 1], ["abilities", 3, .33]],
  ["Progression: Gravship", "PG", [], ["research", 14, 0], ["factions", 1, 1]],
  ["One Stool", "OS", ["furniture"], ["buildings", 1, 1]],
  ["Beautiful Bodies", "BB", ["style"], ["appearance", 6, 1]],
  ["Gloomy Hair mk2", "GH", ["style"], ["appearance", 48, .9]],
  ["Dubs Bad Hygiene", "DBH", ["furniture", "QoL"], ["buildings", 44, .8], ["items", 5, 1], ["research", 9, 1], ["hediffs", 4, 1], ["terrain", 2, 1]],
  ["Rimefeller", "RF", ["furniture"], ["buildings", 31, .7], ["items", 9, .9], ["research", 6, .8]],
];
const FUNCTIONAL = ["Harmony", "HugsLib", "Performance Fish", "RocketMan", "Dubs Mint Menus", "Allow Tool", "Pick Up And Haul", "Common Sense", "Wall Light UI", "Camera+", "Replace Stuff", "Numbers", "RimHUD", "Better Workbench Management", "TD Enhancement Pack", "Simple Sidearms", "Trade UI Revised", "Smart Speed", "Achtung!", "Dubs Performance Analyzer", "Bulk Load For Transporters", "Blueprints", "Level Up!", "Tacticowl"];

const MODS = [];
for (const [name, prefix, tags, ...cats] of SPEC) {
  const m = { id: prefix, name, prefix, tags, cats: [], adds: 0, seen: 0, colonies: 1 + pick(10), saves: 0 };
  m.saves = m.colonies * (2 + pick(22));
  for (const [key, adds, share] of cats) {
    const seen = Math.min(adds, Math.round(adds * share));
    m.cats.push({ key, adds, seen });
    m.adds += adds; m.seen += seen;
  }
  MODS.push(m);
}
FUNCTIONAL.forEach((name, i) => MODS.push({ id: "fn" + i, name, prefix: "", tags: i % 3 ? ["QoL"] : [], cats: [], adds: 0, seen: 0, colonies: 3 + pick(18), saves: 40 + pick(160), functional: true }));
const COLONIES = [["", "all colonies", 209], ["jy", "Fort Ember", 48], ["tundra", "Tundra Base", 31], ["fox", "Fox Den", 22]];

const share = (m) => (m.adds ? m.seen / m.adds : 0);
const state = (m) => (m.functional ? "functional" : m.seen === 0 ? "never" : m.seen < m.adds ? "part" : "whole");
const STATES = [["never", "never seen"], ["part", "partly seen"], ["whole", "all seen"], ["functional", "no content to see"]];
const STATE_LABEL = Object.fromEntries(STATES);
// names not seen of one category of a Mod, made up from its prefix
const WORDS = ["Lamp", "Bench", "Fountain", "Planter", "Arch", "Trellis", "Hedge", "Pier", "Column", "Statue", "Table", "Shelf", "Rug", "Banner", "Gate", "Tile", "Path", "Bridge", "Sofa", "Stove"];
function names(m, c, seenToo = false) {
  const out = [];
  for (let i = 0; i < c.adds; i++) {
    const round = Math.floor(i / WORDS.length);
    out.push({ n: `${m.prefix}_${WORDS[i % WORDS.length]}${round ? round : ""}`, seen: i < c.seen });
  }
  return seenToo ? out : out.filter((x) => !x.seen);
}

// ---- pieces every draft uses ----
// meter: the share seen as a bar; an empty bar is amber-edged, a full one green.
function meter(seen, adds, w = 64) {
  const p = adds ? seen / adds : 0;
  const cls = seen === 0 ? "none" : seen >= adds ? "full" : "";
  return `<span class="meter ${cls}" style="width:${w}px" title="${seen} of ${adds} seen"><i style="width:${seen ? Math.max(4, Math.round(p * 100)) : 0}%"></i></span>`;
}
const frac = (seen, adds) => `<span class="frac"><b>${seen}</b><span class="of">/ ${adds.toLocaleString()}</span></span>`;
const tagChip = (t) => `<span class="tag" style="--c: var(--tag-${TAG_COLOR[t] || "blue"})">${esc(t)}</span>`;
const where = (m) => `${m.colonies} ${m.colonies === 1 ? "colony" : "colonies"}, ${m.saves} saves`;
// catLine: "buildings 0/271 · weapons 0/35 · …", the least seen first
function catLine(m, max = 4) {
  const cs = [...m.cats].sort((a, b) => b.adds - a.adds);
  return cs.slice(0, max).map((c) => `<span class="cl ${c.seen === 0 ? "none" : c.seen >= c.adds ? "full" : ""}">${CAT_LABEL[c.key]} <b>${c.seen}/${c.adds}</b></span>`).join("") + (cs.length > max ? `<span class="cl more">+${cs.length - max}</span>` : "");
}
// detail: one Mod in full (the content form is decided after the organisation is; this is
// the plain form every draft shares for now).
function detailHTML(m, onlyCat = "") {
  if (!m) return `<div class="empty">Select a Mod</div>`;
  if (m.functional) return `<div class="dh"><h2>${esc(m.name)}</h2><div class="sub">${m.tags.map(tagChip).join("")}<span class="muted">${where(m)}</span></div></div>
    <p class="note">It adds nothing a save can show (interface, performance, fixes): saves cannot tell whether it was used.</p>`;
  const cats = m.cats.filter((c) => !onlyCat || c.key === onlyCat);
  return `<div class="dh"><h2>${esc(m.name)}</h2><div class="sub">${m.tags.map(tagChip).join("")}<span class="muted">${where(m)}</span></div>
      <div class="big">${meter(m.seen, m.adds, 180)}${frac(m.seen, m.adds)}<span class="muted">seen</span><button class="btn small line" data-toast="would open it in the Mod library" style="margin-left:auto">Open in Mod library</button></div></div>
    ${cats.map((c) => { const ns = names(m, c); return `<section class="dc"><h3>${CAT_LABEL[c.key]} ${meter(c.seen, c.adds, 48)}${frac(c.seen, c.adds)}</h3>
      ${ns.length ? `<p class="names">${ns.slice(0, 36).map((x) => `<span>${esc(x.n)}</span>`).join("")}${ns.length > 36 ? `<button class="btn link" data-toast="would show all ${ns.length}">${ns.length - 36} more</button>` : ""}</p>` : `<p class="names muted">all seen</p>`}</section>`; }).join("")}`;
}

// ---- shell ----
const DRAFTS = [["h", "H Three panes"], ["g", "G One colony, three questions"], ["e", "E Shelf and album"], ["f", "F Highlights"], ["a", "A By kind"], ["b", "B By tags"], ["c", "C Table"], ["d", "D Map"]];
function shell(page, hints) {
  $("#review").innerHTML = `<b>play review · organisation drafts</b>${DRAFTS.map(([k, t]) => `<a href="${k}.html" class="${k === page ? "on" : ""}">${t}</a>`).join("")}
    <span class="sp"></span><span>example data; buttons perform no real actions</span><button class="rb" id="themeBtn">${ic("theme")} light / dark</button>`;
  $("#themeBtn").onclick = () => {
    const r = document.documentElement, dark = r.dataset.theme ? r.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    r.dataset.theme = dark ? "light" : "dark";
  };
  const it = (t, on = false, cls = "") => `<button class="nav-item ${cls} ${on ? "on" : ""}"><span>${t}</span></button>`;
  $("#nav").innerHTML = `<div class="grp">playing</div>${it("start")}${it("can it run")}${it("game log")}
    <div class="grp">library</div>${it("lists")}${it("packs")}${it("saves")}<div class="sub-guide">${it("all saves", false, "grp-sub")}${it("play review", true, "grp-sub")}</div>${it("mod library")}${it("texture optimisation")}${it("game")}`;
  $("#top").innerHTML = `<span class="crumb muted">library</span><span class="faint">/</span><span class="crumb muted">saves</span><span class="faint">/</span><span class="crumb">play review</span>`;
  const n = MODS.filter((m) => !m.functional);
  $("#head").innerHTML = `<p class="lead">What your Mods add and how much of it your saves show: <b>21 colonies</b>, 209 saves, ${MODS.length} Mods</p>
    <div class="page-acts"><select class="sel" id="colony">${COLONIES.map(([k, t, s]) => `<option value="${k}">${esc(t)}${k ? ` (${s})` : ""}</option>`).join("")}</select></div>`;
  $("#hints").innerHTML = hints.map(([k, t]) => `<span><span class="k">${k}</span>${t}</span>`).join("");
  void n;
}
let toastT;
function toast(msg) {
  const t = $("#toast");
  t.textContent = msg; t.classList.add("on");
  clearTimeout(toastT); toastT = setTimeout(() => t.classList.remove("on"), 1800);
}
document.addEventListener("click", (e) => { const b = e.target.closest("[data-toast]"); if (b) toast(b.dataset.toast); });

// ---- picture-first pieces (drafts E and F) ----
// Stand-ins for real art: a Mod's cover would be its Preview.png, an item's tile its
// texture. Here a cover is a gradient from the name, a tile a glyph of its kind.
const hue = (str) => { let h = 0; for (const ch of str) h = (h * 31 + ch.charCodeAt(0)) % 360; return h; };
const GLYPH = {
  buildings: '<path d="M3 13V7l5-4 5 4v6zM6.5 13v-3.5h3V13"/>', weapons: '<path d="M12.5 3.5 6 10M4.5 9l2.5 2.5M4 12l1.5-1.5"/>',
  apparel: '<path d="M5.5 3 3 5l1.5 2L5.5 6.5V13h5V6.5L11.5 7 13 5l-2.5-2a2.5 2 0 0 1-5 0z"/>', items: '<path d="M3 5.5 8 3l5 2.5v5L8 13l-5-2.5zM3 5.5 8 8l5-2.5M8 8v5"/>',
  terrain: '<path d="M3 3h10v10H3zM3 8h10M8 3v10"/>', animals: '<circle cx="5" cy="6" r="1.2"/><circle cx="8" cy="4.6" r="1.2"/><circle cx="11" cy="6" r="1.2"/><path d="M5.5 11.5c0-2 1-3 2.5-3s2.5 1 2.5 3c0 1-1 1.2-2.5 1.2S5.5 12.500 5.500 11.500z"/>',
  races: '<circle cx="8" cy="5.5" r="2.4"/><path d="M3.5 13c.5-2.6 2.300-3.800 4.500-3.800s4 1.200 4.500 3.800"/>', factions: '<path d="M4 13V3M4 3.500h7.500L10 6l1.500 2.500H4"/>',
  research: '<path d="M6.500 3h3M7 3v3.500L4 12.500h8L9 6.500V3"/>', events: '<path d="M9 2.500 4.500 9H8l-1 4.500L11.500 7H8z"/>', quests: '<path d="M4 2.800h8v10.400H4zM6 5.500h4M6 8h4M6 10.500h2.500"/>',
  genes: '<path d="M5.500 2.500c0 4 5 4 5 8M10.500 2.500c0 4-5 4-5 8M6.200 4.500h3.600M6.200 9h3.600"/>', culture: '<path d="M3 13h10M4.500 13V7M8 13V7M11.500 13V7M3 7l5-4 5 4z"/>',
  abilities: '<path d="M8 2.500l1.600 3.600 3.900.400-2.900 2.600.9 3.900L8 11l-3.500 2 .9-3.900L2.500 6.500l3.900-.400z"/>', hediffs: '<path d="M8 3.500v9M3.500 8h9"/>',
  appearance: '<path d="M4 9c0-3 1.500-5.500 4-5.500s4 2.500 4 5.500M5 9.500c0 2 1.300 3.500 3 3.500s3-1.500 3-3.500"/>', biomes: '<path d="M2.500 12.500 6 6l2.500 4 1.500-2 3.500 4.500z"/>',
};
const glyph = (k) => `<svg class="g" viewBox="0 0 16 16">${GLYPH[k] || GLYPH.items}</svg>`;
// ring: the share seen as a ring; nothing seen is an empty amber ring, all seen a green one
function ring(seen, adds, d = 28) {
  const p = adds ? seen / adds : 0, r = d / 2 - 3, c = 2 * Math.PI * r;
  const col = seen === 0 ? "var(--amber)" : seen >= adds ? "var(--green)" : "var(--accent)";
  return `<svg class="ring" width="${d}" height="${d}" viewBox="0 0 ${d} ${d}"><circle cx="${d / 2}" cy="${d / 2}" r="${r}" fill="none" stroke="${seen === 0 ? "color-mix(in srgb, var(--amber) 45%, transparent)" : "var(--line)"}" stroke-width="3"/>
    <circle cx="${d / 2}" cy="${d / 2}" r="${r}" fill="none" stroke="${col}" stroke-width="3" stroke-linecap="round" stroke-dasharray="${Math.max(seen ? 2 : 0, p * c)} ${c}" transform="rotate(-90 ${d / 2} ${d / 2})"/></svg>`;
}
const initials = (name) => name.replace(/[^A-Za-z0-9 ]/g, "").split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0].toUpperCase()).join("");
function cover(m, extra = "") {
  const h = hue(m.name);
  return `<span class="cover ${state(m)}" style="--h:${h}">${extra}<span class="ini">${initials(m.name)}</span></span>`;
}
// tiles: a Mod's things of one kind as small pictures; seen in colour, not seen as outlines
function tiles(m, c, max = 0, onlyUnseen = false) {
  let ns = names(m, c, true);
  if (onlyUnseen) ns = ns.filter((x) => !x.seen);
  const cut = max ? ns.slice(0, max) : ns;
  return cut.map((x) => `<span class="it ${x.seen ? "seen" : ""}" style="--h:${hue(m.name)}" title="${esc(x.n.replace(/^[^_]*_/, "").replace(/(\d+)$/, " $1"))}">${glyph(c.key)}</span>`).join("") + (max && ns.length > max ? `<span class="it more">+${ns.length - max}</span>` : "");
}
