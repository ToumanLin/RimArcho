// Import page drafts (16 §2.18, 07 §6–7). Example data only; nothing reads or writes files.
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const IC = {
  chev: '<path d="M6 3.5 10.5 8 6 12.5"/>', down: '<path d="M3.5 6 8 10.5 12.5 6"/>', x: '<path d="M4 4l8 8M12 4l-8 8"/>',
  theme: '<circle cx="8" cy="8" r="5.6"/><path d="M8 2.4v11.2A5.6 5.6 0 0 0 8 2.4z" fill="currentColor"/>',
  list: '<path d="M5.5 4h8M5.5 8h8M5.5 12h8"/><circle cx="2.8" cy="4" r=".6"/><circle cx="2.8" cy="8" r=".6"/><circle cx="2.8" cy="12" r=".6"/>',
  pack: '<path d="M2.4 5 8 2.4 13.6 5v6L8 13.6 2.4 11zM2.4 5 8 7.6 13.6 5M8 7.6v6"/>',
  move: '<path d="M2.5 8h9M8.5 4.5 12 8l-3.5 3.5"/><path d="M13.5 3v10"/>',
  file: '<path d="M4 1.8h5.2L12.4 5v9.2H4z"/><path d="M9 1.8V5h3.4"/>',
  paste: '<rect x="3.5" y="3" width="9" height="11" rx="1.5"/><path d="M6 3V2h4v1"/>',
  link: '<path d="M6.8 9.2 9.2 6.8M7.5 4.5l1-1a2.5 2.5 0 0 1 3.5 3.5l-1 1M8.5 11.5l-1 1A2.5 2.5 0 0 1 4 9l1-1"/>',
  save: '<path d="M3 2.6h7.8l2.6 2.6v8.2H2.6V3z"/><path d="M5 2.6v3.2h5V2.6M5 13.4V9h6v4.4"/>',
  tag: '<path d="M2.5 2.5h5l6 6-5 5-6-6z"/><circle cx="5.2" cy="5.2" r=".8"/>',
  note: '<path d="M3 2.6h10v8l-3 3H3z"/><path d="M10 13.6v-3h3"/>',
  rule: '<path d="M3 4h7M3 8h10M3 12h5"/><path d="m11 2.5 2 1.5-2 1.5"/>',
  env: '<rect x="2" y="2.8" width="12" height="8.4" rx="1.5"/><path d="M5.5 13.6h5M8 11.2v2.4"/>',
  ok: '<path d="M3.2 8.4l3 3 6.6-6.8"/>', warn: '<path d="M8 2.2l6.2 11H1.8z"/><path d="M8 6.4v3.2M8 11.6v.1"/>',
  drop: '<path d="M8 2.4v7.2M5.2 6.8 8 9.6l2.8-2.8"/><path d="M2.6 10.4v2.2a1 1 0 0 0 1 1h8.8a1 1 0 0 0 1-1v-2.2"/>',
  color: '<circle cx="8" cy="8" r="5.6"/><path d="M8 2.4a5.6 5.6 0 0 1 0 11.2z" fill="currentColor"/>',
  eye: '<path d="M1.8 8S4 3.6 8 3.6 14.2 8 14.2 8 12 12.4 8 12.4 1.8 8 1.8 8z"/><circle cx="8" cy="8" r="2"/>',
};
const ic = (n, cls = "") => `<svg class="i ${cls}" viewBox="0 0 16 16">${IC[n]}</svg>`;

// ---- example data: what discovery would find on one machine ----
// Each thing says what it becomes in RimArcho, so the page can say it in words.
const FOUND = [
  {
    id: "rimsort", name: "RimSort", mark: "RS", hue: 205, where: "%LOCALAPPDATA%\\RimSort", when: "used 2 days ago",
    things: [
      { id: "rs-lists", kind: "lists", icon: "list", n: 3, what: "saved lists", becomes: "3 new lists", items: [["Main colony", 214, 198], ["Medieval run", 156, 156], ["Test", 12, 9]] },
      { id: "rs-data", kind: "player", icon: "tag", n: 41, what: "notes and tags (instance Default)", becomes: "41 notes, 63 tags, 12 colours as coloured tags, 5 ignore-warnings marks as a tag", counts: [["notes", 41], ["tags", 63], ["colours", 12], ["ignore warnings", 5]] },
      { id: "rs-rules", kind: "rules", icon: "rule", n: 12, what: "your sorting rules", becomes: "12 load-order rules" },
    ],
  },
  {
    id: "rimcrow", name: "RimCrow", mark: "RC", hue: 18, where: "D:\\Tools\\RimCrow", when: "used today",
    things: [
      { id: "rc-profiles", kind: "configs", icon: "env", n: 3, what: "profiles", becomes: "a Game Config for each, with its own list", items: [["Vanilla+", "1.6", "ready"], ["Big modpack", "1.6", "ready"], ["Old 1.5", "1.5", "no 1.5 game here"]] },
      { id: "rc-data", kind: "player", icon: "tag", n: 172, what: "aliases, notes, groups and tags", becomes: "52 aliases, 120 notes, 9 groups as tags, 7 colours as coloured tags", counts: [["aliases", 52], ["notes", 120], ["groups", 9], ["colours", 7]] },
      { id: "rc-rules", kind: "rules", icon: "rule", n: 18, what: "rules and interlocks", becomes: "18 load-order rules" },
      { id: "rc-lists", kind: "lists", icon: "list", n: 6, what: "backup lists", becomes: "6 new lists", items: [["Latest_ModList", 230, 221], ["2026-11-01", 228, 221], ["…4 more", "", ""]] },
    ],
  },
  {
    id: "rimpy", name: "RimPy", mark: "RP", hue: 140, where: "%USERPROFILE%\\AppData\\LocalLow\\RimPy Mod Manager", when: "used 5 months ago",
    things: [
      { id: "rp-lists", kind: "lists", icon: "list", n: 4, what: "saved lists", becomes: "4 new lists", items: [["Colony", 188, 170], ["Royalty run", 140, 131], ["…2 more", "", ""]] },
    ],
  },
];

// What one file or pasted text can be, and what it becomes (07 §7.2, 10 §4).
const SOURCES = [
  { k: "list", icon: "list", t: "A Mod list", from: "RimSort, RimCrow, RimPy, RimWorld (.rml), ModsConfig.xml, a RimArcho list, plain packageIds", becomes: "a list, or changes to one of yours" },
  { k: "save", icon: "save", t: "A save's Mods", from: "a .rws save file", becomes: "a list with the Mods that save used; the save itself stays where it is" },
  { k: "code", icon: "link", t: "A link or a code", from: "a Steam collection, a Rentry page, a RimCrow share code (RC-…)", becomes: "a list; a collection can be followed" },
  { k: "pack", icon: "pack", t: "A pack", from: "an .archopack file", becomes: "a pack you can launch, with its Mods" },
  { k: "manager", icon: "move", t: "Another manager's data", from: "RimSort's aux_metadata.db or userRules.json, RimCrow's mod_manager.db", becomes: "notes, tags, rules and, from RimCrow, Game Configs" },
];

// Simulated recognition of an input (what the real page would read).
const SAMPLES = {
  list: { icon: "list", kind: "A Mod list from RimSort", name: "Main colony.xml", facts: "214 Mods  ·  198 installed  ·  16 not installed  ·  RimWorld 1.6", dest: true },
  save: { icon: "save", kind: "The Mods of a save", name: "Fort Ember.rws", facts: "186 Mods in its load order  ·  180 installed  ·  6 not installed", dest: true },
  code: { icon: "link", kind: "A Steam collection", name: "Vanilla Expanded – all", facts: "97 Mods  ·  61 installed  ·  36 to subscribe", dest: true, follow: true },
  pack: { icon: "pack", kind: "A pack", name: "Ratkin + 4 races.archopack", facts: "38 Mods, 2.0 GB  ·  checked: every file matches", pack: true },
  manager: { icon: "move", kind: "RimSort's notes and tags", name: "aux_metadata.db (instance Default)", facts: "41 notes  ·  63 tags  ·  12 colours  ·  5 ignore-warnings marks", manager: true },
};

const LISTS = ["default", "Main colony", "Medieval run"];

// ---- shell ----
const DRAFTS = [["a", "A By what you get"], ["b", "B Moving in first"], ["c", "C Drop anything"], ["d", "D Inbox"]];
function shell(page, hints, lead) {
  $("#review").innerHTML = `<b>import · page drafts</b>${DRAFTS.map(([k, t]) => `<a href="${k}.html" class="${k === page ? "on" : ""}">${t}</a>`).join("")}
    <span class="sp"></span><span>example data; buttons perform no real actions</span><button class="rb" id="themeBtn">${ic("theme")} light / dark</button>`;
  $("#themeBtn").onclick = () => {
    const r = document.documentElement, dark = r.dataset.theme ? r.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    r.dataset.theme = dark ? "light" : "dark";
  };
  const it = (t, on = false) => `<button class="nav-item ${on ? "on" : ""}"><span>${t}</span></button>`;
  $("#nav").innerHTML = `<div class="grp">playing</div>${it("start")}${it("can it run")}${it("game log")}
    <div class="grp">saves</div>${it("all saves")}${it("play review")}
    <div class="grp">library</div>${it("lists")}${it("packs")}${it("mod library")}${it("texture optimisation")}${it("Game Config")}${it("import", true)}`;
  $("#top").innerHTML = `<span class="crumb muted">library</span><span class="faint">/</span><span class="crumb">import</span><span class="chip">main  1.6</span>`;
  $("#head").innerHTML = lead ? `<p class="lead">${lead}</p>` : "";
  $("#hints").innerHTML = hints.map(([k, t]) => `<span><span class="k">${k}</span>${t}</span>`).join("");
}
let toastT;
function toast(msg) {
  const t = $("#toast");
  t.textContent = msg; t.classList.add("on");
  clearTimeout(toastT); toastT = setTimeout(() => t.classList.remove("on"), 2000);
}
document.addEventListener("click", (e) => { const b = e.target.closest("[data-toast]"); if (b) toast(b.dataset.toast); });

// pieces shared by the drafts
const mark = (m, size = 32) => `<span class="mgr" style="--h:${m.hue};width:${size}px;height:${size}px;font-size:${Math.round(size * 0.36)}px">${m.mark}</span>`;
const destRadios = (name, follow) => `<fieldset class="dest"><legend>it becomes</legend>
  <label><input type="radio" name="${name}" checked><span><b>a new list</b><small>your current list stays as it is</small></span></label>
  <label><input type="radio" name="${name}"><span><b>the content of a list you have</b><small><select class="sel">${LISTS.map((l) => `<option>${l}</option>`).join("")}</select>  14 removed, 22 added, 9 moved; undoable</small></span></label>
  <label><input type="radio" name="${name}"><span><b>only the Mods your list is missing</b><small>added at the end of the current list</small></span></label>
  ${follow ? `<label class="chk"><input type="checkbox" checked><span><b>follow this collection</b><small>RimArcho tells you when it changes</small></span></label>` : ""}</fieldset>`;
