"use strict";
// Shared by both drafts: icons, sample data, the shell, the paste box's recognition.
// Sample data only; nothing reads or writes files.
const $ = (s, r = document) => r.querySelector(s);
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const IC = {
  plus: '<path d="M8 3v10M3 8h10"/>',
  git: '<circle cx="4.5" cy="4" r="1.6"/><circle cx="4.5" cy="12" r="1.6"/><circle cx="11.5" cy="6.5" r="1.6"/><path d="M4.5 5.6v4.8M11.5 8.1c0 2-2.2 2.4-5.6 3"/>',
  steam: '<circle cx="8" cy="8" r="5.8"/><circle cx="10" cy="6.3" r="1.6"/><path d="M2.5 9.6l3.6 1.5"/>',
  cmd: '<rect x="2" y="3" width="12" height="10" rx="1.6"/><path d="M4.8 6.4 6.8 8l-2 1.6M8.5 10h3"/>',
  globe: '<circle cx="8" cy="8" r="5.8"/><path d="M2.2 8h11.6M8 2.2c1.7 1.6 2.5 3.5 2.5 5.8S9.7 12.2 8 13.8M8 2.2C6.3 3.8 5.5 5.7 5.5 8s.8 4.2 2.5 5.8"/>',
  folder: '<path d="M2.5 4.5a1 1 0 0 1 1-1h3l1.5 1.5h4.5a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1z"/>',
  refresh: '<path d="M13 3.5v3h-3M3 12.5v-3h3"/><path d="M12.6 6.5A5 5 0 0 0 3.6 5.4M3.4 9.5a5 5 0 0 0 9 1.1"/>',
  check: '<path d="M3.2 8.4 6.5 11.6 12.8 4.6"/>', x: '<path d="M4 4l8 8M12 4l-8 8"/>',
  chev: '<path d="M6 3.5 10.5 8 6 12.5"/>', down: '<path d="M3.5 6 8 10.5 12.5 6"/>',
  ext: '<path d="M9.5 2.5h4v4M13.5 2.5 7.5 8.5M12 9.5v3.5a.5.5 0 0 1-.5.5h-8a.5.5 0 0 1-.5-.5v-8a.5.5 0 0 1 .5-.5H7"/>',
  warn: '<path d="M8 2.5 14 13H2z"/><path d="M8 6.5v3M8 11.4h.01"/>', info: '<circle cx="8" cy="8" r="5.8"/><path d="M8 7.2v4M8 5h.01"/>',
  zip: '<path d="M4 1.8h5.5L12.5 5v9.2H4z"/><path d="M9.5 1.8V5h3M7 4h1M7 6h1M7 8h1M6.5 10h2v2h-2z"/>',
  link: '<path d="M6.8 9.2a2.6 2.6 0 0 0 3.7 0l2-2a2.6 2.6 0 0 0-3.7-3.7l-.7.7M9.2 6.8a2.6 2.6 0 0 0-3.7 0l-2 2a2.6 2.6 0 0 0 3.7 3.7l.7-.7"/>',
  dl: '<path d="M8 2.5v8M4.5 7 8 10.5 11.5 7M3 13.5h10"/>', commit: '<circle cx="8" cy="8" r="2.4"/><path d="M2 8h3.6M10.4 8H14"/>',
  clock: '<circle cx="8" cy="8" r="5.8"/><path d="M8 4.8V8l2.2 1.4"/>', user: '<circle cx="8" cy="5.5" r="2.6"/><path d="M3 13.5c.6-2.6 2.6-4 5-4s4.4 1.4 5 4"/>',
  inbox: '<path d="M2.5 9 4.4 3.5h7.2L13.5 9v3.5a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1z"/><path d="M2.5 9h3l.8 1.6h3.4l.8-1.6h3"/>',
};
const ic = (n) => `<svg class="i" viewBox="0 0 16 16">${IC[n]}</svg>`;
const SRC = { ws: ["steam", "workshop"], cmd: ["cmd", "SteamCMD"], git: ["git", "Git"], ll: ["globe", "LoversLab"], local: ["folder", "local"] };
const src = (k) => `<span class="src">${ic(SRC[k][0])}${SRC[k][1]}</span>`;

// What waits for the player: sources RimArcho updates itself (Steam updates the Workshop).
const PENDING = [
  { id: "pf", n: "Performance Fish", s: "git", from: "a1b2c3d", fromD: "10/12", to: "7f3e9a1", toD: "10/23", track: "default branch main",
    used: ["Main colony", "Light start"], kind: "commits",
    log: [["7f3e9a1", "Fix null ref in pawn cache after reload", "10/23"], ["5c02b4e", "Support 1.6 hotfix 2", "10/21"], ["e91d77a", "Bump version to 0.6.3", "10/21"], ["0b3fa10", "Cleanup: remove unused patches", "10/18"]],
    impact: "code changed; affects launch" },
  { id: "pl", n: "Prison Labor", s: "git", from: "v2.1.0", fromD: "09/30", to: "v2.2.0", toD: "10/20", track: "tag v*",
    used: ["Main colony"], kind: "commits",
    log: [["v2.2.0", "Release 2.2.0: new work types, Chinese translation", "10/20"]], impact: "Defs and translations changed" },
  { id: "bs", n: "Body Shape Patch", s: "ll", from: "2.3", fromD: "09/02", to: "2.4", toD: "10/20",
    used: ["Warhammer 40K"], kind: "files", page: "loverslab.com/files/file/18342-body-shape-patch",
    files: [["BodyShapePatch-2.4.zip", "12.4 MB", "10/20", true], ["BodyShapePatch-2.4-legacy-1.5.zip", "11.9 MB", "10/20", false]],
    note: "Author notes: support 1.6; fix body texture offsets.", impact: "not downloaded yet; changes known after download" },
  { id: "ce", n: "Combat Extended", s: "cmd", from: "10/10", fromD: "", to: "10/22", toD: "", used: ["Main colony"], kind: "notes",
    note: "Workshop change notes: Fixed ammo loading for mortars; new ammo for VFE Mechanoids.", impact: "not downloaded yet" },
];
// What Steam already changed since the current list was last played (modchanges tiers).
const CHANGES = [
  { id: "vef", n: "Vanilla Expanded Framework", t: 1, say: "code changed: 3 Harmony patches added, 1 changed", when: "10/23", used: ["Main colony", "Light start"],
    groups: [["code (.dll)", "2 changed"], ["Defs", "6 changed"], ["Patches", "1 added"]] },
  { id: "har", n: "Humanoid Alien Races", t: 1, say: "new dependency Harmony 2.3; code changed", when: "10/23", used: ["Main colony"],
    groups: [["code (.dll)", "1 changed"], ["About", "dependency brrainz.harmony added"]] },
  { id: "rr", n: "Real Ruins", t: 2, say: "1.6 support added; 4 Defs changed", when: "10/22", used: ["Main colony"], groups: [["Defs", "4 changed"], ["About", "supports 1.6"]] },
  { id: "dbh", n: "Dubs Bad Hygiene", t: 2, say: "2 Patches changed, 1 Def added", when: "10/22", used: ["Main colony", "Light start"], groups: [["Patches", "2 changed"], ["Defs", "1 added"]] },
  { id: "vfe", n: "Vanilla Furniture Expanded", t: 3, say: "textures +3.2 MB", when: "10/23", used: ["Main colony"], groups: [["textures", "+3.2 MB"]] },
  { id: "rs", n: "RimSaves", t: 3, say: "3 translations changed", when: "10/22", used: ["Main colony"], groups: [["translation", "3 changed"]] },
];
const ARRIVALS = { add: ["Vanilla Gravship Expanded", "Allow Tool"], gone: ["Rimefeller"] };
const OLDER = [{ day: "10/21", n: 7 }, { day: "10/19", n: 3 }, { day: "10/15", n: 12 }];
const TIER = { 1: "affects launch", 2: "worth a look", 3: "content only" };
const SOURCES = [
  { k: "ws", c: 412, m: "Steam downloads updates itself", r: '<span class="warn">2 no longer available</span>' },
  { k: "cmd", c: 6, m: "checked at 14:02", r: "1 update available", acct: "account rim_player" },
  { k: "git", c: 3, m: "checked at 14:02", r: "2 updates available" },
  { k: "ll", c: 2, m: "checked at 14:02", r: "1 update available", acct: "signed in, expires Nov 30" },
  { k: "local", c: 38, m: "in Mods folder, no source", r: "" },
];

// Example inputs for the paste box.
const SAMPLES = {
  git: "https://github.com/bbradson/Performance-Fish",
  ll: "https://www.loverslab.com/files/file/20511-gene-tweaks/",
  ws: "https://steamcommunity.com/sharedfiles/filedetails/?id=2009463077",
  zip: "C:\\Users\\me\\Downloads\\GeneTweaks-1.2.zip",
};
// detect: what a pasted text is. Git hosts, LoversLab file pages, Workshop links or bare IDs, local zips.
function detect(t) {
  t = t.trim();
  if (!t) return null;
  if (/^https?:\/\/(github\.com|gitlab\.com|codeberg\.org)\/[^/]+\/[^/]+/.test(t) || /\.git$/.test(t)) return "git";
  if (/loverslab\.com\/files\/file\//.test(t)) return "ll";
  if (/steamcommunity\.com\/(sharedfiles|workshop)\/filedetails/.test(t) || /^\d{6,12}$/.test(t)) return "ws";
  if (/\.(zip|7z|rar)$/i.test(t) || /^[A-Za-z]:\\/.test(t)) return "zip";
  return "bad";
}
// found: the recognised item, with the one decision it needs and its evidence.
function found(kind, st) {
  // only a newly recognised item fades in; redraws while choosing do not
  const enter = kind !== found.last ? " enter" : ""; found.last = kind;
  const head = (icon, name, meta) => `<div class="found-h"><span class="ico">${ic(icon)}</span><b>${name}</b>
    <button class="btn x" data-do="clear" title="esc">${ic("x")}</button><div class="meta">${meta}</div></div>`;
  const foot = (left, main) => `<div class="found-f"><span class="muted">${left}</span><button class="btn" data-do="clear">cancel<kbd>esc</kbd></button><button class="btn primary" data-do="add">${main}<kbd>enter</kbd></button></div>`;
  if (kind === "bad") return `<div class="found${enter}"><div class="found-h"><span class="ico">${ic("info")}</span><b>unrecognized address</b>
    <button class="btn x" data-do="clear">${ic("x")}</button><div class="meta">Supports GitHub / GitLab repos, LoversLab file pages, Workshop links or IDs, downloaded zips</div></div></div>`;
  if (kind === "git") return `<div class="found${enter}">${head("git", "Performance Fish", `<span>${src("git")}</span><span>bbradson/Performance-Fish</span><span>latest commit 2 days ago</span>`)}
    <div class="found-b">
      <div class="facts"><div class="f"><span class="ok">✓</span><span>found About/About.xml, package ID <code>bs.performance</code>, supports 1.5, 1.6</span></div>
        <div class="f"><span class="warn">!</span><span>You already have the Workshop Performance Fish (used by Main colony). Both have the same package ID; the game loads one. Choose a copy in the list after subscribing</span></div></div>
      <div class="vrow" style="display:flex;align-items:center;gap:10px"><span class="muted" style="font-size:12px">tracking</span>
        <div class="seg">${["default branch main", "latest tag", "stay on this commit"].map((x, i) => `<button class="${(st.track || 0) === i ? "on" : ""}" data-track="${i}">${x}</button>`).join("")}</div></div>
    </div>${foot("clone into Mods; compare commits when checking updates", "subscribe to it")}</div>`;
  if (kind === "ll") {
    const f = st.file ?? 0;
    return `<div class="found${enter}">${head("globe", "Gene Tweaks", `<span>${src("ll")}</span><span>author Kaz</span><span>version 1.2</span><span>updated Oct 18</span>`)}
    <div class="found-b">
      <div class="note acc">${ic("git")}<span>This page links GitHub repository <code>kaz/gene-tweaks</code>. Subscribe with Git to avoid choosing files for future updates.</span><button class="btn link" data-sample="git">use Git instead</button></div>
      <div class="choice">${[["GeneTweaks-1.2.zip", "4.1 MB", "10/18"], ["GeneTweaks-1.2-nolore.zip", "3.6 MB", "10/18"]].map(([n, s, d], i) =>
        `<button class="opt ${f === i ? "on" : ""}" data-file="${i}"><span class="rd"></span><span class="nm">${n}${i === 0 ? "<small>the largest one</small>" : ""}</span><span class="ext">${s}  ${d}</span></button>`).join("")}</div>
      <div class="facts"><div class="f"><span class="ok">✓</span><span>signed in to LoversLab (remember me, expires Nov 30)</span></div>
        <div class="f"><span class="muted">i</span><span>the site's download terms appear before downloading; agree to proceed</span></div></div>
    </div>${foot("No login needed: download the zip in your browser and drop it here", "download and subscribe")}</div>`;
  }
  if (kind === "ws") {
    const w = st.via ?? 0;
    return `<div class="found${enter}">${head("steam", "Allow Tool", `<span>${src("ws")}</span><span>ID 2009463077</span><span>UnlimitedHugs</span><span>supports 1.6</span>`)}
    <div class="found-b"><div class="choice">
      <button class="opt ${w === 0 ? "on" : ""}" data-via="0"><span class="rd"></span><span class="nm">subscribe with Steam<small>recommended</small></span><span class="ext ok">Steam is running</span><span class="ds">Steam downloads and updates automatically; changes appear below</span></button>
      <button class="opt ${w === 1 ? "on" : ""}" data-via="1"><span class="rd"></span><span class="nm">downloaded with SteamCMD</span><span class="ext">account rim_player</span><span class="ds">without the Steam client; RimArcho checks updates later</span></button>
    </div></div>${foot("dependency HugsLib is already installed", "subscribe to it")}</div>`;
  }
  if (kind === "zip") return `<div class="found${enter}">${head("zip", "GeneTweaks-1.2.zip", `<span>4.1 MB</span><span>contains 1 mod: Gene Tweaks (kaz.genetweaks)</span>`)}
    <div class="found-b">
      <label class="omni" style="height:32px"><span class="muted" style="white-space:nowrap;font-size:12px">downloaded from</span><input placeholder="paste LoversLab file page (optional)" value="${st.from ? "https://www.loverslab.com/files/file/20511-gene-tweaks/" : ""}" data-from></label>
      <div class="facts"><div class="f"><span class="muted">i</span><span>${st.from ? "with a page: check for updates later and notify here" : "without it: install as local mod, no update checks"}</span></div></div>
    </div>${foot("extract into Mods folder", "add")}</div>`;
  return "";
}

// nav draws the sidebar and breadcrumb. on: "subs" or "add"; withAdd: show the add subscription sub-page.
function nav(kidBadge, on = "subs", withAdd = false) {
  const it = (t, extra = "", cls = "") => `<button class="nav-item ${cls}"><span>${t}</span>${extra}</button>`;
  const chev = `<span class="chev">${ic("chev")}</span>`;
  const kid = (k, t, extra = "") => `<button class="nav-kid ${on === k ? "on" : ""}" data-page="${k}">${t}${extra}</button>`;
  $("#nav").innerHTML = `<div class="grp">playing</div>${it("start")}${it("can it run", '<span class="badge-n">2</span>')}${it("game log")}
    <div class="grp">library</div>${it("the library", chev, "open")}
    <div class="kids"><button class="nav-kid">all</button>${kid("subs", "subscriptions and updates", kidBadge ? `<span class="badge-n acc">${kidBadge}</span>` : "")}${withAdd ? kid("add", "add a subscription") : ""}</div>
    ${it("lists", chev)}${it("packs", chev)}${it("texture optimisation")}${it("game")}`;
  $("#crumbs").innerHTML = `<span class="crumb">library</span><span class="sep">/</span><span class="crumb">the library</span><span class="sep">/</span><span class="crumb last">${on === "add" ? "add a subscription" : "subscriptions and updates"}</span>`;
}
function hints(arr) { $("#hints").innerHTML = arr.map(([k, d]) => `<span><span class="k">${k}</span>${d}</span>`).join(""); }
let sayT;
function say(t) { $("#status").innerHTML = `<span class="ok">✓</span> ${t}`; clearTimeout(sayT); sayT = setTimeout(() => ($("#status").innerHTML = ""), 5000); }
function sourcesPopover(anchor) {
  const r = anchor.getBoundingClientRect();
  const v = $("#veil"); v.hidden = false;
  v.innerHTML = `<div class="popover" style="top:${r.bottom + 6}px;left:${Math.max(8, Math.min(r.left, innerWidth - 370))}px">
    ${SOURCES.map((s) => `<div class="srcrow">${ic(SRC[s.k][0])}<span>${SRC[s.k][1]}  <span class="muted">${s.c} items</span></span><span class="r">${s.k === "ws" || s.k === "local" ? "" : '<button class="btn small">check</button>'}</span>
      <span class="m">${[s.m, s.r, s.acct].filter(Boolean).join(", ")}</span></div>`).join("")}
    <div class="srcrow"><span></span><span class="m">2 Workshop items unavailable: deleted or private.<button class="btn link">see which two</button></span></div>
    <div class="srcrow"><span></span><span class="m">manage accounts in settings <button class="btn link">settings › accounts →</button></span></div></div>`;
}
function themeButton() {
  $("#theme").onclick = () => {
    const d = document.documentElement, dark = d.dataset.theme ? d.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    d.dataset.theme = dark ? "light" : "dark";
  };
}
