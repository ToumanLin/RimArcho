"use strict";
// Shared by the drafts: icons, sample data, the shell, small helpers. Sample data only.
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const IC = {
  err: '<circle cx="8" cy="8" r="6"/><path d="M8 4.8v3.8M8 11.1h.01"/>',
  warn: '<path d="M8 2.5 14 13H2z"/><path d="M8 6.6v3M8 11.4h.01"/>',
  hint: '<circle cx="8" cy="8" r="6"/><path d="M8 7.3v4M8 5h.01"/>',
  check: '<path d="M3.2 8.4 6.5 11.6 12.8 4.6"/>', x: '<path d="M4 4l8 8M12 4l-8 8"/>',
  chev: '<path d="M6 3.5 10.5 8 6 12.5"/>', down: '<path d="M3.5 6 8 10.5 12.5 6"/>',
  refresh: '<path d="M13 3.5v3h-3M3 12.5v-3h3"/><path d="M12.6 6.5A5 5 0 0 0 3.6 5.4M3.4 9.5a5 5 0 0 0 9 1.1"/>',
  log: '<path d="M4 1.8h5.5L12.5 5v9.2H4z"/><path d="M9.5 1.8V5h3M6 8h4.5M6 10.5h4.5"/>',
  wand: '<path d="M3 13 10.5 5.5M9 3.5l.5-1.5.5 1.5 1.5.5-1.5.5-.5 1.5-.5-1.5L7.5 4zM12.5 8l.4-1 .4 1 1 .4-1 .4-.4 1-.4-1-1-.4z"/>',
  arrow: '<path d="M3 8h10M9 4l4 4-4 4"/>', theme: '<circle cx="8" cy="8" r="5.6"/><path d="M8 2.4v11.2A5.6 5.6 0 0 0 8 2.4z" fill="currentColor"/>',
};
const ic = (n, cls = "") => `<svg class="i ${cls}" viewBox="0 0 16 16">${IC[n]}</svg>`;
const statusIcon = (st) => `<span class="ic ${st}">${ic(st)}</span>`;
// ⟦name⟧ marks a mod name: a link to its row in the editor, or the Workshop when not installed.
const nm = (s) => esc(s).replace(/⟦(.+?)⟧/g, '<span class="need-name" tabindex="0">$1</span>');
const plain = (s) => String(s).replace(/[⟦⟧]/g, "");
const EVCLS = { "log": "log", "community data": "community", "shared logs": "corpus" };
const evLine = (ev) => (ev || []).map(([k, s]) => `<span class="ev-chip ${EVCLS[k.replace(/ \(.*/, "")] || ""}">${esc(k)}</span><span class="src">${esc(s)}</span>`).join("");

const PLAN = "Main colony", GAME = "1.6 Steam", LASTRUN = "Oct 28 21:04";

// The items. sec: where the page puts it in the candidates; now: where it is today, if different.
// step: what kind of thing the player does about it (draft C groups by this).
const ITEMS = [
  { id: "official", sec: "break", st: "err", step: "sort", title: "Core and the DLCs must come first in the list",
    why: "Content depending on official content may fail to load if it follows mods.", ev: [["inferred", "the game's official content order"]],
    acts: [{ l: "sort automatically", k: "S" }], mods: ["Core", "Biotech", "Anomaly"] },
  { id: "order", sec: "break", st: "warn", step: "sort", title: "7 mods are in the wrong order",
    why: "Wrong order can prevent dependencies or content from taking effect.", ev: [["declared by the mod", "loadAfter / loadBefore in 7 mods' About.xml"]],
    acts: [{ l: "sort automatically", k: "S" }, { l: "expand", k: "enter" }],
    fold: ["⟦Humanoid Alien Races⟧ should load after ⟦Harmony⟧", "⟦Vanilla Factions Expanded - Mechanoids⟧ should load after ⟦Vanilla Expanded Framework⟧",
      "⟦Prison Labor⟧ should load after ⟦Humanoid Alien Races⟧", "⟦Real Ruins⟧ should load after ⟦HugsLib⟧", "⟦Dubs Bad Hygiene⟧ should load after ⟦Harmony⟧",
      "⟦RimHUD⟧ should load after ⟦HugsLib⟧", "⟦Alpha Genes⟧ should load after ⟦Vanilla Expanded Framework⟧"] },
  { id: "links", sec: "break", st: "err", step: "links", title: "2 broken links in the game's Mods folder",
    why: "The target folders are gone. Repair the links before launching this list.", ev: [["inferred", "missing target: D:\\Mods\\Old\\…"]],
    acts: [{ l: "repair…", k: "R" }],
    links: [["1.6 Steam", "D:\\Steam\\steamapps\\common\\RimWorld", [["OldTextures", "D:\\Mods\\Old\\OldTextures", "target is missing"], ["CE-Guns", "D:\\Mods\\Old\\CE-Guns", "target is missing"]]]],
    otherGame: ["1.5 test", "D:\\Games\\RimWorld-1.5", [["RimHUD-dev", "D:\\dev\\RimHUD", "unknown target, ownership unconfirmed"]]] },
  { id: "missing", sec: "break", st: "err", step: "subscribe", title: "3 mods in the list are not installed",
    why: "After subscribing and downloading, they return to their positions in the list.", ev: [["inferred", "in the list, but not in Mods or Workshop folders"]],
    acts: [{ l: "subscribe to all…", k: "s" }, { l: "remove all from the list", k: "x" }, { l: "expand", k: "f" }],
    fold: ["⟦Vanilla Genetics Expanded⟧", "⟦Alpha Animals⟧", "⟦Character Editor⟧"] },
  { id: "dep", sec: "break", st: "err", step: "enable", title: "⟦Vanilla Factions Expanded - Mechanoids⟧ needs ⟦Vanilla Expanded Framework⟧, which is not enabled",
    why: "⟦Vanilla Furniture Expanded⟧ also needs it.", ev: [["declared by the mod", "dependencies declared by the author in About.xml"]],
    acts: [{ l: "enable Vanilla Expanded Framework", k: "space" }], enables: "Vanilla Expanded Framework",
    mods: ["Vanilla Factions Expanded - Mechanoids", "Vanilla Furniture Expanded", "Vanilla Expanded Framework"] },
  { id: "patch", sec: "break", st: "warn", step: "enable", title: "⟦Dubs Bad Hygiene⟧ : 6 patches will not take effect",
    why: "Their target ⟦Rimefeller⟧ is installed but not enabled.", ev: [["inferred", "patch 1.6/Patches/Rimefeller.xml:4 found no target"]],
    acts: [{ l: "enable Rimefeller", k: "space" }, { l: "see what", k: "enter" }], enables: "Rimefeller" },
  { id: "incompat", sec: "break", st: "err", step: "choose", title: "⟦Combat Extended⟧ is incompatible with ⟦Yayo's Combat 3⟧",
    why: "Both change combat; enabling both causes errors or abnormal behavior.", ev: [["declared by the mod", "Combat Extended declares incompatibleWith in About.xml"]],
    acts: [{ l: "choose which to disable…", k: "space" }], choices: ["disable Combat Extended", "disable Yayo's Combat 3"] },
  { id: "dup", sec: "break", st: "err", step: "choose", title: "two copies of the same mod enabled: ⟦Allow Tool⟧",
    why: "Workshop copy updated Oct 20; local copy in Mods/AllowTool-old.", ev: [["inferred", "both copies have packageId unlimitedhugs.allowtool"]],
    acts: [{ l: "choose which copy to keep…", k: "space" }], choices: ["keep the Workshop copy", "keep the local copy"] },
  { id: "version", sec: "break", st: "warn", step: "look", title: "18 mods do not claim support for 1.6",
    why: "15 list 1.5; excludes 2 ignored reminders. Most still work; check these first if errors occur.", ev: [["declared by the mod", "About.xml supportedVersions"]],
    acts: [{ l: "see the list", k: "enter" }, { l: "ignore 1.6 reminders for these 18 mods", k: "M" }],
    versions: [["list 1.5", ["Real Ruins", "Prison Labor", "Simple Sidearms", "RimHUD", "Alpha Genes", "Character Editor", "Dubs Mint Menus", "…8 more"]], ["list only 1.4 or earlier", ["Rimefeller", "Old Texture Pack", "CE Guns"]]] },

  // last run (Player.log)
  { id: "log-vfe", sec: "run", st: "err", step: "log", phase: "while loading the game", title: "⟦Vanilla Furniture Expanded⟧ — 12 errors",
    why: "9 missing textures (Things/Building/Furniture/VFE_DiningTable); 3 Def references to missing content", ev: [["log", "Player.log  " + LASTRUN]],
    acts: [{ l: "open game log", k: "enter" }], relates: "dep",
    log: ["Could not load Texture2D at 'Things/Building/Furniture/VFE_DiningTable' in any active mod or in base resources.", "Config error in VFE_DiningTable: graphicData.texPath not found"] },
  { id: "log-rr", sec: "run", st: "warn", step: "log", phase: "while loading the game", title: "⟦Real Ruins⟧ — 3 warnings",
    why: "Def references missing content", ev: [["log", "Player.log  " + LASTRUN]], acts: [{ l: "open game log", k: "enter" }],
    log: ["Could not resolve cross-reference to Verse.ThingDef named RR_ScatteredDebris (wanter=thingDef)"] },
  { id: "log-unk", sec: "run", st: "err", step: "log", phase: "while loading a save", title: "unknown source — 2 errors",
    why: "NullReferenceException twice; source not yet known", ev: [["log", "Player.log  " + LASTRUN]], acts: [{ l: "open game log", k: "enter" }],
    log: ["Exception loading list element from XML: System.NullReferenceException", "  at Verse.ScribeExtractor.SaveableFromNode[T] (System.Xml.XmlNode subNode, System.Object[] ctorArgs)"] },
  { id: "log-pl", sec: "run", st: "err", step: "log", phase: "while playing", title: "⟦Prison Labor⟧ — 41 errors",
    why: "41 exceptions (PrisonLabor.WorkGiver_Supervise.JobOnThing)", ev: [["log", "ArchoMonitor.log  Oct 28 21:31"]], acts: [{ l: "open game log", k: "enter" }],
    log: ["Exception in JobOnThing: System.NullReferenceException", "  at PrisonLabor.WorkGiver_Supervise.JobOnThing (Verse.Pawn pawn, Verse.Thing t, System.Boolean forced)"] },
  { id: "log-old", sec: "run", st: "hint", step: "old", old: true, title: "older log  Sep 29  1.5", counted: false },

  // for you to confirm
  { id: "ident", sec: "confirm", st: "warn", step: "choose", title: "Are ⟦Better Pawn Control⟧ and ⟦Better Pawn Control (Continued)⟧ the same mod?",
    why: "Same package ID, different folders; 12 identical files.", ev: [],
    acts: [{ l: "same", k: "s", eq: true }, { l: "a copy", k: "c", eq: true }, { l: "unrelated", k: "u", eq: true }], whyLink: "why this question?",
    cmp: [["name", "Better Pawn Control (Continued)", "Better Pawn Control", false], ["package ID", "VouLT.BetterPawnControl", "VouLT.BetterPawnControl", true],
      ["mod authors", "Vouli, Taranchuk", "Vouli", false], ["Workshop ID", "3094618023", "1541460369", false], ["comparison folder", "3094618023", "BetterPawnControl", false]] },
  { id: "extern", sec: "confirm", st: "warn", step: "choose", title: "ModsConfig.xml changed outside RimArcho, and this list changed afterwards",
    why: "Changed Oct 28 at 22:10, possibly in the game's Mod menu.", ev: [["inferred", "ModsConfig.xml  Oct 28 22:10"]],
    acts: [{ l: "see the changes", k: "d" }, { l: "load…", k: "L" }, { l: "ignore", k: "I" }], choices: ["use the outside change", "keep this list"] },
  { id: "packlinks", sec: "confirm", st: "warn", step: "packlinks", title: "pack Warhammer 40K left 14 links in Mods/",
    why: "Launching a list from RimArcho removes them; you can remove them now too.", ev: [["inferred", "14 links to the pack in Mods/"]], acts: [{ l: "remove the links", k: "r" }] },

  // not counted: today dimmed inside breaks the game
  { id: "combo", sec: "hint", now: "break", st: "hint", step: "hint", counted: false, title: "hint: ⟦Vanilla Expanded Framework⟧ and ⟦Performance Optimizer⟧ often error together",
    why: "If errors occur, try disabling one first.", ev: [["shared logs (through 2026-09)", "errors in 31 of 60 logs with both enabled (52%)"]], acts: [{ l: "choose which to disable…", k: "space" }] },
  { id: "obs", sec: "hint", now: "break", st: "hint", step: "hint", counted: false, title: "hint: ⟦Alpha Genes⟧ may need ⟦Alpha Memes⟧",
    why: "Alpha Memes is not installed. Add it if matching errors occur.", ev: [["shared logs (through 2026-09)", "errors in 40/50 (80%) without it"]], acts: [{ l: "open in Workshop", k: "o" }] },
  { id: "known", sec: "hint", now: "break", st: "hint", step: "hint", counted: false, title: "hint: this version of ⟦Prison Labor⟧ often errors in shared logs",
    why: "If it happens to you, check for updates or temporarily disable it.", ev: [["shared logs (through 2026-09)", "this version appears in 112 logs; 71 contain its errors (63%)"]], acts: [{ l: "see the error", k: "enter" }] },
  { id: "weak", sec: "hint", now: "break", st: "hint", step: "sort", counted: false, title: "5 weak order requirements unmet",
    why: "Suggested by authors or community; violating them did not cause more errors in shared logs.", ev: [["community data", "RimSort community rules"]], acts: [{ l: "sort automatically", k: "S" }, { l: "expand", k: "f" }] },
  { id: "cycle", sec: "hint", now: "break", st: "hint", step: "hint", counted: false, title: "sorting handles cyclic requirements of 3 mods by priority",
    why: "Drops the community rule: load ⟦RimHUD⟧ after ⟦Real Ruins⟧.", ev: [["community data", "RimSort community rules"]], acts: [{ l: "see each requirement", k: "enter" }] },

  // hidden mods: today a section here; the candidates move them to mod library
  { id: "pin", sec: "hidden", st: "hint", step: "hidden", counted: false, title: "⟦RimHUD⟧ version locked; Workshop copy updated since",
    why: "", ev: [["inferred", "Workshop updated Oct 25"]], acts: [{ l: "see the difference", k: "d" }, { l: "update the locked version", k: "p" }, { l: "unlock…", k: "P" }] },
  { id: "residue", sec: "hidden", st: "hint", step: "hidden", counted: false, title: "⟦Dubs Mint Menus⟧ hidden and unsubscribed; folder remains",
    why: "", ev: [], acts: [{ l: "move to the recycle bin…", k: "t" }] },
  { id: "hid", sec: "hidden", st: "hint", step: "hidden", counted: false, title: "⟦Simple Sidearms⟧ hidden (probably by another mod manager)",
    why: "", ev: [], acts: [{ l: "keep it hidden", k: "i" }, { l: "restore…", k: "U" }, { l: "move to the recycle bin…", k: "t" }] },
];
for (const it of ITEMS) if (it.counted === undefined) it.counted = true;
const byId = Object.fromEntries(ITEMS.map((x) => [x.id, x]));

const SEC = {
  break: { t: "breaks the game", cls: "sec-break" },
  run: { t: "errors from the last run", cls: "sec-run", r: LASTRUN },
  confirm: { t: "for you to confirm", cls: "sec-confirm" },
  hint: { t: "worth knowing", cls: "sec-hint" },
  hidden: { t: "hidden mods", cls: "sec-hidden" },
};

// shell draws the review strip, sidebar, breadcrumb and hints. page: which draft this is.
const DRAFTS = [["now", "what is there"], ["a", "A Two columns"], ["b", "B List + details"], ["c", "C By action"]];
function shell(page, badge, hints, status = "") {
  $("#review").innerHTML = `<b>can it run · drafts</b>${DRAFTS.map(([k, t]) => `<a href="${k}.html" class="${k === page ? "on" : ""}">${t}</a>`).join("")}
    <span class="sp"></span><span>example data; buttons perform no real actions</span><button class="rb" id="themeBtn">${ic("theme")} light / dark</button>`;
  $("#themeBtn").onclick = () => {
    const r = document.documentElement, dark = r.dataset.theme ? r.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    r.dataset.theme = dark ? "light" : "dark";
  };
  const it = (t, extra = "", on = false) => `<button class="nav-item ${on ? "on" : ""}"><span>${t}</span>${extra}</button>`;
  $("#nav").innerHTML = `<div class="grp">playing</div>${it("start")}${it("can it run", `<span class="badge-n" id="badge">${badge}</span>`, true)}${it("game log")}
    <div class="grp">library</div>${it("lists")}${it("packs")}${it("the library")}${it("texture optimisation")}${it("game")}
    <div class="grp">not played yet</div>${it("play review")}<div class="grp">worth playing</div>${it("play goals")}`;
  $("#top").innerHTML = `<span class="crumb">can it run</span><span class="chip">${PLAN}  ${GAME}</span>`;
  $("#hints").innerHTML = hints.map(([k, t]) => `<span><span class="k">${k}</span>${t}</span>`).join("");
  $("#status").textContent = status;
}
const setBadge = (n) => { const b = $("#badge"); if (b) { b.textContent = n; b.hidden = !n; } };

let toastT;
function toast(msg) {
  const t = $("#toast");
  t.textContent = msg; t.classList.add("on");
  clearTimeout(toastT); toastT = setTimeout(() => t.classList.remove("on"), 1800);
}
// menu: the ⋯ menu of an item's other actions.
function menu(anchor, acts, onPick) {
  closeMenu();
  const m = document.createElement("div");
  m.className = "menu"; m.id = "menu";
  m.innerHTML = acts.map((a, i) => `<button data-i="${i}">${esc(a.l)}<kbd>${esc(a.k)}</kbd></button>`).join("");
  document.body.append(m);
  const r = anchor.getBoundingClientRect();
  m.style.top = Math.min(r.bottom + 4, innerHeight - m.offsetHeight - 8) + "px";
  m.style.left = Math.max(8, r.right - m.offsetWidth) + "px";
  m.onclick = (e) => { const b = e.target.closest("button"); if (b) { closeMenu(); onPick(acts[+b.dataset.i]); } };
  setTimeout(() => addEventListener("pointerdown", outside), 0);
}
function outside(e) { if (!e.target.closest("#menu")) closeMenu(); }
function closeMenu() { $("#menu")?.remove(); removeEventListener("pointerdown", outside); }
document.addEventListener("click", (e) => {
  const n = e.target.closest(".need-name");
  if (n) { e.stopPropagation(); toast(`go to the editor's ${n.textContent}`); }
}, true);
document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeMenu(); });

// linkRows: the grouped broken links (16 §2.4), one game per group.
function linkRows(groups) {
  return `<div class="links">${groups.map(([g, path, rows]) => `<div><div class="game"><b>${esc(g)}</b>${esc(path)}</div>
    ${rows.map(([f, to, why]) => `<div class="link-row"><span>${esc(f)}</span><span class="muted">→</span><span class="p" title="${esc(to)}">${esc(to)}</span><span class="muted">${esc(why)}</span><span class="tag ${why.includes("unconfirmed") ? "keep" : ""}">${why.includes("unconfirmed") ? "keep" : "will delete"}</span></div>`).join("")}</div>`).join("")}</div>`;
}

// needHTML: one item as the page draws it today (16 §2.4): icon, bold title, a plain why,
// the evidence line under the cursor, one main button on the right and the rest in ⋯.
// opts.links: the link groups to show; opts.open: expanded merged items.
function needHTML(it, i, opts = {}) {
  if (it.old) return `<div class="need old" data-i="${i}">${esc(it.title)}</div>`;
  const dim = !it.counted, acts = it.acts || [];
  let right = "";
  if (acts.some((a) => a.eq)) right = acts.map((a, j) => `<button class="btn line small" data-a="${j}">${esc(a.l)}</button>`).join("");
  else if (acts.length) {
    right = `<button class="btn ${dim ? "line" : "primary"} small" data-a="0">${esc(acts[0].l)}</button>`;
    if (acts.length > 1) right += `<button class="btn more small" data-more="1" title="other actions">⋯</button>`;
  }
  const open = opts.open && opts.open.has(it.id);
  const fold = open && it.fold ? `<ul class="fold">${it.fold.map((f) => `<li>${nm(f)}</li>`).join("")}</ul>` : "";
  const links = it.links ? linkRows(opts.links ? opts.links(it) : it.links) : "";
  const whyLink = it.whyLink ? `<div><button class="btn link" data-why="1">${esc(it.whyLink)}</button></div>` : "";
  return `<div class="need ${dim ? "dim" : ""}" data-i="${i}">
    <div class="top">${statusIcon(it.st)}<div class="body"><div class="title">${nm(it.title)}</div>${it.why && !it.links ? `<div class="why">${nm(it.why)}</div>` : ""}${whyLink}${fold}${opts.extra || ""}</div>
    <div class="acts">${right}</div></div>
    ${it.ev && it.ev.length ? `<div class="ev">${evLine(it.ev)}</div>` : ""}${links}</div>`;
}

// bindRows: clicks on rows and their buttons. rows carry data-i (an index into list).
// onAct(it, act) runs an action; default: a toast.
function bindRows(root, list, setCur, onAct = (it, a) => toast(`would run: ${a.l}`)) {
  root.addEventListener("click", (e) => {
    const row = e.target.closest("[data-i]");
    if (!row || !root.contains(row)) return;
    const i = +row.dataset.i, it = list[i];
    setCur(i);
    const b = e.target.closest("button");
    if (!b || !it) return;
    if (b.dataset.more) menu(b, it.acts.slice(1), (a) => onAct(it, a));
    else if (b.dataset.why) toast("open the why-this-question comparison");
    else if (b.dataset.a !== undefined) onAct(it, it.acts[+b.dataset.a]);
  });
}

// keyNav: ↑↓ (j k) move, ← → jump to the previous / next group, enter the main action.
// groupOf(i) says which group row i is in.
function keyNav(len, get, set, groupOf, enter) {
  document.addEventListener("keydown", (e) => {
    if (e.target.closest("input, textarea") || e.ctrlKey || e.altKey || e.metaKey) return;
    const cur = get(), n = len();
    if (["ArrowDown", "j"].includes(e.key)) set(Math.min(n - 1, cur + 1));
    else if (["ArrowUp", "k"].includes(e.key)) set(Math.max(0, cur - 1));
    else if (e.key === "Home") set(0);
    else if (e.key === "End") set(n - 1);
    else if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
      const g = groupOf(cur), d = e.key === "ArrowRight" ? 1 : -1;
      let i = cur;
      if (d > 0) { while (i < n && groupOf(i) === g) i++; if (i < n) set(i); }
      else { while (i > 0 && groupOf(i - 1) === g) i--; if (i > 0) { const pg = groupOf(i - 1); while (i > 0 && groupOf(i - 1) === pg) i--; set(i); } }
    } else if (e.key === "Enter" && enter) enter(cur);
    else return;
    e.preventDefault();
  });
}
const counted = (list) => list.filter((x) => x.counted).length;
