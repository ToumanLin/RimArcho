"use strict";
// Shared by the save drafts: sample data, the shell and the analysis blocks every draft reuses.
// Sample data only; buttons show a toast and never touch files.
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const IC = {
  err: '<circle cx="8" cy="8" r="6"/><path d="M8 4.8v3.8M8 11.1h.01"/>',
  warn: '<path d="M8 2.5 14 13H2z"/><path d="M8 6.6v3M8 11.4h.01"/>',
  ok: '<path d="M3.2 8.4 6.5 11.6 12.8 4.6"/>', hint: '<circle cx="8" cy="8" r="6"/><path d="M8 7.3v4M8 5h.01"/>',
  chev: '<path d="M6 3.5 10.5 8 6 12.5"/>', down: '<path d="M3.5 6 8 10.5 12.5 6"/>', x: '<path d="M4 4l8 8M12 4l-8 8"/>',
  play: '<path d="M5 3.5v9l7-4.5z"/>', log: '<path d="M4 1.8h5.5L12.5 5v9.2H4z"/><path d="M9.5 1.8V5h3M6 8h4.5M6 10.5h4.5"/>',
  theme: '<circle cx="8" cy="8" r="5.6"/><path d="M8 2.4v11.2A5.6 5.6 0 0 0 8 2.4z" fill="currentColor"/>',
  search: '<circle cx="7" cy="7" r="4.2"/><path d="m10.2 10.2 3.3 3.3"/>',
};
const ic = (n, cls = "") => `<svg class="i ${cls}" viewBox="0 0 16 16">${IC[n]}</svg>`;
const stIcon = (st) => `<span class="st-ic ${st}">${ic({ ok: "ok", warn: "warn", broken: "err" }[st] || "hint")}</span>`;
const kw = (n) => (n >= 1000 ? (n / 1000).toFixed(2) + " M" : n + " k");

// ---- sample data ----
// A colony is the group of saves that share one game (save-file seed + world), not an object of its own.
// Everything shown at colony level is read from its latest save; older saves show only how they differ.
const NAMES = ["Engie", "Hollis", "Mara", "Pike", "Rook", "Sable", "Tamsin", "Ves", "Wren", "Yule", "Ozzie", "Bram"];
const COLONIES = [
  {
    id: "tundra", name: "Tundra Hollow", ver: "1.6.4633", env: "1.6 · Steam", list: "Medieval run", listMatch: "exact",
    folder: "Saves · shared by 1.6 · Steam", story: "Cassandra Classic · Strive to survive", biome: "Boreal forest", playH: 46, dead: ["Ozzie"],
    uses: [
      { m: "Core + Royalty + Ideology + Biotech", o: "everything vanilla", rm: "—" },
      { m: "Vanilla Expanded Framework", o: "3 world components, 41 comps on things", rm: "breaks load", lv: "broken" },
      { m: "Dubs Bad Hygiene", o: "46 buildings, needs on 11 colonists", rm: "loses 46 buildings", lv: "warn" },
      { m: "Medieval Overhaul", o: "1 faction, 212 items, 3 research projects", rm: "loses faction and items", lv: "warn" },
      { m: "Vanilla Furniture Expanded – Art", o: "14 sculptures", rm: "already gone", lv: "warn", gone: true },
      { m: "RimHUD, Camera+, 31 more", o: "nothing in the save (interface only)", rm: "safe", lv: "ok" },
    ],
    saves: [
      { id: "t1", file: "Tundra Hollow.rws", day: 431, date: "14 Aprimay 5502", saved: "Nov 3 · 21:04", size: "27.4 MB", col: 11, wealth: 184, animals: 23, mods: 214, research: 64, raids: 22, st: "warn",
        say: "1 Mod is no longer installed: 14 sculptures will vanish", diff: { gone: ["Vanilla Furniture Expanded – Art"], added: ["RimHUD"], moved: 4 },
        lastLoad: { when: "Nov 3 · 20:12", errors: 0 } },
      { id: "t2", file: "Autosave-1.rws", auto: 1, day: 428, date: "11 Aprimay 5502", saved: "Nov 3 · 20:51", size: "27.3 MB", col: 11, wealth: 181, animals: 23, mods: 214, research: 63, raids: 22, st: "warn",
        say: "Same Mods as the latest save", diff: { gone: ["Vanilla Furniture Expanded – Art"], added: ["RimHUD"], moved: 4 } },
      { id: "t3", file: "Autosave-2.rws", auto: 1, day: 420, date: "3 Aprimay 5502", saved: "Nov 3 · 20:02", size: "27.0 MB", col: 11, wealth: 176, animals: 21, mods: 214, research: 62, raids: 21, st: "warn",
        say: "Same Mods as the latest save", diff: { gone: ["Vanilla Furniture Expanded – Art"], added: ["RimHUD"], moved: 4 } },
      { id: "t4", file: "Autosave-3.rws", auto: 1, day: 412, date: "10 Jugust 5501", saved: "Nov 2 · 23:41", size: "9.1 MB", col: 11, wealth: 0, animals: 0, mods: 214, research: 0, raids: 0, st: "broken", trunc: 1,
        say: "Cut off: no </savegame>. Written while the game crashed at 23:41", diff: { gone: [], added: [], moved: 0 } },
      { id: "t5", file: "Before the siege.rws", day: 390, date: "8 Jugust 5501", saved: "Nov 2 · 22:10", size: "25.8 MB", col: 12, wealth: 162, animals: 19, mods: 214, research: 58, raids: 20, st: "warn",
        say: "Same Mods as the latest save · Ozzie still alive here", diff: { gone: ["Vanilla Furniture Expanded – Art"], added: ["RimHUD"], moved: 4 } },
      { id: "t6", file: "Autosave-4.rws", auto: 1, day: 360, date: "8 Jugust 5501", saved: "Nov 1 · 19:30", size: "24.1 MB", col: 10, wealth: 141, animals: 17, mods: 214, research: 52, raids: 18, st: "warn",
        say: "Same Mods as the latest save", diff: { gone: ["Vanilla Furniture Expanded – Art"], added: ["RimHUD"], moved: 4 } },
      { id: "t7", file: "Tundra Hollow early.rws", day: 120, date: "5 Septober 5500", saved: "Oct 18 · 22:47", size: "11.6 MB", col: 5, wealth: 31, animals: 6, mods: 198, research: 21, raids: 6, st: "ok",
        say: "16 Mods were added since; adding is safe", diff: { gone: [], added: ["Dubs Bad Hygiene", "Medieval Overhaul", "14 more"], moved: 9 } },
    ],
    events: [["day 402", "Ozzie died (siege, mechanoid lance)"], ["day 388", "Medieval Overhaul siege repelled"], ["day 301", "Married: Mara and Pike"], ["day 140", "Ship part crashed (psychic droner)"]],
  },
  {
    id: "ashfall", name: "Ashfall Outpost", ver: "1.6.4633", env: "1.6 · Steam", list: "Biotech heavy", listMatch: "close",
    folder: "Saves · shared by 1.6 · Steam", story: "Randy Random · Adventure story", biome: "Arid shrubland", playH: 19, dead: [], prefix: "Ashfall",
    uses: [
      { m: "Core + Biotech", o: "everything vanilla", rm: "—" },
      { m: "Kurin HAR", o: "3 colonists of the Kurin race, 1 faction, 2 hediffs", rm: "already gone", lv: "broken", gone: true },
      { m: "Humanoid Alien Races", o: "race framework for 3 colonists", rm: "breaks load", lv: "broken" },
      { m: "Vanilla Genetics Expanded", o: "28 genes on 6 colonists", rm: "loses genes", lv: "warn" },
      { m: "17 more", o: "nothing in the save", rm: "safe", lv: "ok" },
    ],
    saves: [
      { id: "a1", file: "Ashfall#§#Ashfall Outpost.rws", day: 212, date: "2 Decembary 5501", saved: "Nov 2 · 22:31", size: "18.2 MB", col: 9, wealth: 96, animals: 4, mods: 88, research: 41, raids: 11, st: "broken", aliens: 3,
        say: "Kurin HAR is gone: 3 colonists would be lost, and the save uses a class no installed Mod has",
        diff: { gone: ["Kurin HAR"], added: [], moved: 0 }, refs: 6, cls: "Kurin_HAR.CompKurinTail",
        lastLoad: { when: "Nov 2 · 22:40", errors: 41, sample: "Could not resolve reference to object with loadID Thing_Human1043 of type Verse.Pawn" } },
      { id: "a2", file: "Ashfall#§#Autosave-1.rws", auto: 1, day: 206, date: "11 Septober 5501", saved: "Nov 2 · 22:02", size: "18.0 MB", col: 9, wealth: 94, animals: 4, mods: 88, research: 41, raids: 11, st: "broken", aliens: 3,
        say: "Same Mods as the latest save", diff: { gone: ["Kurin HAR"], added: [], moved: 0 }, refs: 6, cls: "Kurin_HAR.CompKurinTail" },
      { id: "a3", file: "Ashfall#§#Landing.rws", day: 2, date: "2 Aprimay 5500", saved: "Oct 27 · 20:15", size: "4.3 MB", col: 3, wealth: 12, animals: 0, mods: 87, research: 0, raids: 0, st: "broken", aliens: 1,
        say: "Kurin HAR is gone: 1 colonist would be lost", diff: { gone: ["Kurin HAR"], added: ["Vanilla Genetics Expanded"], moved: 0 }, refs: 2, cls: "Kurin_HAR.CompKurinTail" },
    ],
    events: [["day 190", "Mechanitor gestated 2 militors"], ["day 77", "Waster pirates raid"], ["day 2", "Landed with 3 colonists (1 Kurin)"]],
  },
  {
    id: "glacier", name: "Glacier test", ver: "1.5.4409", env: "1.5 test", list: "pack · Vanilla 1.5", listMatch: "exact",
    folder: "packs/Vanilla 1.5/userdata/Saves (isolated)", story: "Phoebe Chillax · Peaceful", biome: "Ice sheet", playH: 3, dead: [],
    uses: [{ m: "Core + Royalty", o: "everything vanilla", rm: "—" }, { m: "4 more", o: "nothing in the save", rm: "safe", lv: "ok" }],
    saves: [
      { id: "g1", file: "Glacier test.rws", day: 38, date: "13 Jugust 5500", saved: "Oct 30 · 16:20", size: "6.2 MB", col: 4, wealth: 22, animals: 1, mods: 6, research: 12, raids: 1, st: "ok",
        say: "Matches the pack exactly", diff: { gone: [], added: [], moved: 0 }, lastLoad: { when: "Oct 30 · 15:02", errors: 0 } },
      { id: "g2", file: "Autosave-1.rws", auto: 1, day: 30, date: "5 Jugust 5500", saved: "Oct 30 · 15:48", size: "5.9 MB", col: 4, wealth: 20, animals: 1, mods: 6, research: 10, raids: 1, st: "ok",
        say: "Matches the pack exactly", diff: { gone: [], added: [], moved: 0 } },
    ],
    events: [["day 21", "Cold snap"]],
  },
  {
    id: "sandy", name: "Sandy Reach", ver: "1.4.3901", env: "no 1.4 game", list: null, orphan: 1,
    folder: "Saves · shared by 1.6 · Steam", story: "Cassandra Classic · Rough", biome: "Desert", playH: 61, dead: ["Hollis", "Rook"],
    uses: [{ m: "31 of 88 Mods", o: "not installed now", rm: "—", lv: "broken" }],
    saves: [
      { id: "s1", file: "Sandy Reach.rws", day: 860, date: "15 Decembary 5503", saved: "Mar 9, 2025", size: "41.0 MB", col: 14, wealth: 612, animals: 40, mods: 88, research: 100, raids: 47, st: "broken",
        say: "Made in 1.4; no game here runs 1.4, and 31 of its 88 Mods are not installed", diff: { gone: ["31 Mods"], added: [], moved: 0 } },
    ],
    events: [["day 840", "Ship launched? No: the save ends before launch"]],
  },
];
COLONIES.forEach((c) => c.saves.forEach((s) => (s.c = c)));
const latest = (c) => c.saves[0];
const findSave = (id) => COLONIES.flatMap((c) => c.saves).find((s) => s.id === id);
const shownName = (s) => s.file.replace(/^.*#§#/, "").replace(/\.rws$/, "");

// ---- shell ----
const DRAFTS = [["a", "A Colony page + timeline"], ["b-first", "B first draft"], ["b", "B Three columns (revised)"], ["c", "C One list, verdict in the row"]];
function shell(page, crumb, hints) {
  $("#review").innerHTML = `<b>saves · drafts</b>${DRAFTS.map(([k, t]) => `<a href="${k}.html" class="${k === page ? "on" : ""}">${t}</a>`).join("")}
    <span class="sp"></span><span>example data; buttons perform no real actions</span><button class="rb" id="themeBtn">${ic("theme")} light / dark</button>`;
  $("#themeBtn").onclick = () => {
    const r = document.documentElement, dark = r.dataset.theme ? r.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    r.dataset.theme = dark ? "light" : "dark";
  };
  const it = (t, on = false, cls = "") => `<button class="nav-item ${cls} ${on ? "on" : ""}"><span>${t}</span></button>`;
  $("#nav").innerHTML = `<div class="grp">playing</div>${it("launch")}${it("can it run")}${it("game log")}
    <div class="grp">library</div>${it("lists")}${it("packs")}${it("saves", true)}${it("mod library")}${it("texture optimisation")}${it("games")}
    <div class="grp">not played yet</div>${it("play review")}
    <div class="grp">worth playing</div>${it("play goals")}`;
  $("#top").innerHTML = crumb;
  $("#hints").innerHTML = hints.map(([k, t]) => `<span><span class="k">${k}</span>${t}</span>`).join("");
}
let toastT;
function toast(msg) {
  const t = $("#toast");
  t.textContent = msg; t.classList.add("on");
  clearTimeout(toastT); toastT = setTimeout(() => t.classList.remove("on"), 1800);
}
document.addEventListener("click", (e) => { const b = e.target.closest("[data-toast]"); if (b) toast(b.dataset.toast); });

// ---- colony facts: shown once, never repeated per save ----
function colonyChips(c) {
  const list = c.list ? `<span class="chip"><span class="k">list</span> ${esc(c.list)}${c.listMatch === "close" ? ' <span class="warn">· 1 Mod differs</span>' : ""}</span>`
    : `<span class="chip"><span class="k">list</span> <span class="warn">matches no list</span></span>`;
  return `<div class="chips"><span class="chip"><span class="k">game</span> ${esc(c.ver)}</span><span class="chip"><span class="k">environment</span> ${esc(c.env)}</span>${list}
    <span class="chip"><span class="k">folder</span> ${esc(c.folder)}</span><span class="chip"><span class="k">story</span> ${esc(c.story)}</span></div>`;
}

// ---- the save's verdict: one sentence answering "can I keep playing this?" ----
const VERDICT = { ok: "Loads with the current list", warn: "Loads, but something will be lost", broken: "Will not load cleanly" };
function verdictHTML(s, opts = {}) {
  const acts = [];
  if (s.st === "broken" && s.trunc) acts.push(`<button class="btn line small" data-toast="would recycle ${esc(shownName(s))}">Move to Recycle Bin</button>`);
  else if (s.diff.gone.length && !s.c.orphan) acts.push(`<button class="btn primary small" data-toast="would subscribe to ${esc(s.diff.gone.join(", "))} and enable it in ${esc(s.c.list)}">Subscribe ${s.diff.gone.length === 1 ? esc(s.diff.gone[0]) : s.diff.gone.length + " Mods"}</button>`);
  if (s.c.orphan) acts.push(`<button class="btn primary small" data-toast="would create a list from this save's 88 Mods">Make a list from this save</button>`);
  if (!s.trunc && s.st !== "broken") acts.push(`<button class="btn line small" data-toast="would launch ${esc(s.c.list)}">${ic("play")} Launch</button>`);
  return `<div class="verdict ${s.st}">${stIcon(s.st)}<div><div class="t">${VERDICT[s.st]}</div><div class="s">${esc(s.say)}</div></div>
    ${opts.noActs ? "" : `<div class="acts">${acts.join("")}</div>`}</div>`;
}

// ---- technical: file, references, classes, Mods vs list, last load ----
function checksHTML(s) {
  const c = s.c, row = (st, l, v, sub = "") => `<div class="check">${stIcon(st)}<span class="l">${l}</span><span class="v">${v}${sub ? `<span class="sub">${sub}</span>` : ""}</span></div>`;
  const out = [];
  out.push(s.trunc ? row("broken", "File", "<b>Cut off</b> at 9.1 MB of ~27 MB", "No <code>&lt;/savegame&gt;</code>. The game log shows a crash at 23:41 while saving.")
    : row("ok", "File", `Complete · ${esc(s.size)}`, "XML well-formed, ends with <code>&lt;/savegame&gt;</code>"));
  if (!s.trunc) {
    out.push(s.refs ? row("broken", "References", `<b>${s.refs} references</b> point to objects that will not exist`, "Objects owned by Kurin HAR; the game reports “Could not resolve reference”")
      : row("ok", "References", "Every loadID reference resolves"));
    out.push(s.cls ? row("broken", "Code", `<b>1 class</b> no installed Mod provides`, `<code>${esc(s.cls)}</code> · from Kurin HAR`)
      : row("ok", "Code", "Every Class=\"…\" exists in installed assemblies"));
  }
  const d = s.diff, parts = [];
  if (d.gone.length) parts.push(`<b class="${s.st === "broken" ? "err" : "warn"}">${d.gone.length === 1 ? "1 Mod missing" : esc(d.gone.join(", ")) + " missing"}</b>${d.gone.length === 1 ? ": " + esc(d.gone[0]) : ""}`);
  if (d.added.length) parts.push(`${d.added.length > 2 ? 16 : d.added.length} added (${esc(d.added.slice(0, 2).join(", "))}${d.added.length > 2 ? "…" : ""})`);
  if (d.moved) parts.push(`${d.moved} reordered`);
  const listName = c.list ? `“${esc(c.list)}”` : "no list";
  out.push(row(d.gone.length ? (s.st === "broken" ? "broken" : "warn") : "ok", "Mods", `${s.mods} in the save · vs ${listName}: ${parts.join(" · ") || "identical"}`,
    d.gone.length && !c.orphan ? "Missing Mods lose what they own; see “What the save uses”." : d.added.length ? "Added Mods are normally safe mid-playthrough." : ""));
  if (s.lastLoad) out.push(row(s.lastLoad.errors ? "broken" : "ok", "Last load", s.lastLoad.errors ? `<b>${s.lastLoad.errors} errors</b> when loaded ${esc(s.lastLoad.when)}` : `Loaded ${esc(s.lastLoad.when)} without errors`,
    s.lastLoad.sample ? `<code>${esc(s.lastLoad.sample)}</code> <button class="btn link" data-toast="would open the game log at this load">${ic("log")} game log</button>` : "from the game log (Monitor context.save)"));
  else out.push(row("none", "Last load", '<span class="muted">Not loaded since RimArcho started watching</span>'));
  return `<div class="checks">${out.join("")}</div>`;
}

// ---- which Mods the save really uses, and what removing each one costs ----
function usesHTML(c) {
  return `<table class="uses">${c.uses.map((u) => `<tr><td>${esc(u.m)}${u.gone ? ' <span class="tagx broken">not installed</span>' : ""}</td><td class="o">${esc(u.o)}</td>
    <td class="rm ${u.lv === "broken" ? "err" : u.lv === "warn" ? "warn" : u.lv === "ok" ? "ok" : "muted"}">${u.rm === "—" || u.gone ? "" : "remove → "}${esc(u.rm)}</td></tr>`).join("")}</table>`;
}

// ---- play: stats with change since the previous save, people, events ----
function playHTML(s, opts = {}) {
  const c = s.c, i = c.saves.indexOf(s), prev = c.saves.slice(i + 1).find((p) => !p.trunc);
  if (s.trunc) return `<div class="muted">Unreadable past 9.1 MB; play facts come from the other saves.</div>`;
  const dlt = (k, f = (v) => v) => (prev ? (s[k] - prev[k] ? `<div class="d ${s[k] < prev[k] ? "neg" : ""}">${s[k] > prev[k] ? "+" : ""}${f(s[k] - prev[k])}</div>` : '<div class="d muted">±0</div>') : "");
  const st = (n, l, d = "") => `<div class="stat"><div class="n">${n}</div><div class="l">${l}</div>${d}</div>`;
  const pool = [...NAMES.filter((n) => !c.dead.includes(n)), ...c.dead], ppl = pool.slice(0, s.col).map((n, k) => `<span class="person ${k < (s.aliens || 0) ? "alien" : ""}"><i></i>${n}</span>`).join("")
    + (i === 0 ? c.dead.map((n) => `<span class="person dead"><i></i>${n}</span>`).join("") : "");
  return `<div class="stats">${st(`${kw(s.wealth)}`, "wealth", dlt("wealth", (v) => kw(v)))}${st(s.col, "colonists", dlt("col"))}${opts.noDay ? "" : st(`day ${s.day}`, esc(s.date))}
      ${st(s.animals, "animals", dlt("animals"))}${st(`${s.research}<small>%</small>`, "research", dlt("research", (v) => v + "%"))}${st(s.raids, "raids survived", dlt("raids"))}</div>
    ${opts.noPeople ? "" : `<div class="people">${ppl}</div>`}
    ${opts.noEvents ? "" : `<ul class="events">${c.events.filter((e) => +e[0].slice(4) <= s.day).slice(0, 4).map((e) => `<li><span>${e[0]}</span>${esc(e[1])}</li>`).join("")}</ul>`}`;
}

// ---- timeline: wealth over in-game days; one dot per save ----
function timelineSVG(c, selId, w = 640, h = 120) {
  const ss = c.saves.filter((s) => !s.trunc).slice().reverse(), maxD = Math.max(...c.saves.map((s) => s.day)) || 1, maxW = Math.max(...ss.map((s) => s.wealth)) || 1;
  const x = (d) => 24 + (d / maxD) * (w - 48), y = (v) => h - 22 - (v / maxW) * (h - 40);
  const line = ss.map((s) => `${x(s.day)},${y(s.wealth)}`).join(" ");
  const pop = ss.map((s) => `${x(s.day)},${y((s.col / 14) * maxW)}`).join(" ");
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => Math.round(f * maxD));
  const deaths = c.events.filter((e) => /died/.test(e[1])).map((e) => +e[0].slice(4));
  const dots = c.saves.map((s) => {
    const cy = s.trunc ? h - 22 : y(s.wealth), r = s.auto ? 4 : 5.5, sel = s.id === selId;
    return `${sel ? `<circle class="ring" cx="${x(s.day)}" cy="${cy}" r="${r + 4}"/>` : ""}<circle class="pt ${s.auto ? "auto" : ""} ${s.st} ${sel ? "sel" : ""}" data-save="${s.id}" cx="${x(s.day)}" cy="${cy}" r="${r}"><title>${esc(shownName(s))} · day ${s.day}</title></circle>`;
  }).join("");
  return `<svg class="timeline" viewBox="0 0 ${w} ${h}" style="height:${h}px">
    <line class="ax" x1="24" x2="${w - 24}" y1="${h - 22}" y2="${h - 22}"/>
    ${ticks.map((t) => `<text x="${x(t)}" y="${h - 6}" text-anchor="middle">day ${t}</text>`).join("")}
    ${deaths.map((d) => `<line class="ev" x1="${x(d)}" x2="${x(d)}" y1="10" y2="${h - 22}"><title>death</title></line>`).join("")}
    <polygon class="area" points="${x(ss[0].day)},${h - 22} ${line} ${x(ss.at(-1).day)},${h - 22}"/>
    <polyline class="ln" points="${line}"/><polyline class="pop" points="${pop}"/>${dots}
    <text x="24" y="12">— wealth   ┄ colonists   │ death</text></svg>`;
}
function sparkSVG(c, w = 64, h = 18) {
  const ss = c.saves.filter((s) => !s.trunc).slice().reverse();
  if (ss.length < 2) return `<svg class="spark" width="${w}" height="${h}"></svg>`;
  const maxD = ss.at(-1).day || 1, maxW = Math.max(...ss.map((s) => s.wealth)) || 1;
  return `<svg class="spark" width="${w}" height="${h}"><polyline points="${ss.map((s) => `${(s.day / maxD) * (w - 2) + 1},${h - 2 - (s.wealth / maxW) * (h - 4)}`).join(" ")}"/></svg>`;
}

// ---- save actions (shared) ----
function saveActs(s) {
  return `<button class="btn small" data-toast="would rename ${esc(shownName(s))} inline">Rename</button><button class="btn small" data-toast="would copy to ${esc(shownName(s))} (copy).rws">Copy</button>
    <button class="btn small" data-toast="would back up to backups/saves/">Back up</button><button class="btn small" data-toast="would ask, then move to the Recycle Bin">Recycle…</button>`;
}
