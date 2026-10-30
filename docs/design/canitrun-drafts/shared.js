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
const EVCLS = { "日志": "log", "社区数据": "community", "分享的日志": "corpus" };
const evLine = (ev) => (ev || []).map(([k, s]) => `<span class="ev-chip ${EVCLS[k.replace(/（.*/, "")] || ""}">${esc(k)}</span><span class="src">${esc(s)}</span>`).join("");

const PLAN = "主力殖民地", GAME = "1.6 Steam", LASTRUN = "10 月 28 日 21:04";

// The items. sec: where the page puts it in the candidates; now: where it is today, if different.
// step: what kind of thing the player does about it (draft C groups by this).
const ITEMS = [
  { id: "official", sec: "break", st: "err", step: "sort", title: "Core 和 DLC 要排在最前面",
    why: "官方内容排在 Mod 后面时，依赖它们的内容可能加载不了。", ev: [["推断", "游戏的官方内容顺序"]],
    acts: [{ l: "自动排序", k: "S" }], mods: ["Core", "Biotech", "Anomaly"] },
  { id: "order", sec: "break", st: "warn", step: "sort", title: "7 个 Mod 的顺序不对",
    why: "顺序不对可能让前置或部分内容无法生效。", ev: [["Mod 自己声明", "7 个 Mod 在 About.xml 里写的 loadAfter / loadBefore"]],
    acts: [{ l: "自动排序", k: "S" }, { l: "展开", k: "enter" }],
    fold: ["⟦Humanoid Alien Races⟧ 应该排在 ⟦Harmony⟧ 之后", "⟦Vanilla Factions Expanded - Mechanoids⟧ 应该排在 ⟦Vanilla Expanded Framework⟧ 之后",
      "⟦Prison Labor⟧ 应该排在 ⟦Humanoid Alien Races⟧ 之后", "⟦Real Ruins⟧ 应该排在 ⟦HugsLib⟧ 之后", "⟦Dubs Bad Hygiene⟧ 应该排在 ⟦Harmony⟧ 之后",
      "⟦RimHUD⟧ 应该排在 ⟦HugsLib⟧ 之后", "⟦Alpha Genes⟧ 应该排在 ⟦Vanilla Expanded Framework⟧ 之后"] },
  { id: "links", sec: "break", st: "err", step: "links", title: "游戏的 Mods 文件夹里有 2 个断掉的链接",
    why: "链接指向的文件夹已经不在了。启动列表之前要先修好。", ev: [["推断", "目标不存在：D:\\Mods\\Old\\…"]],
    acts: [{ l: "修复…", k: "R" }],
    links: [["1.6 Steam", "D:\\Steam\\steamapps\\common\\RimWorld", [["OldTextures", "D:\\Mods\\Old\\OldTextures", "目标不存在"], ["CE-Guns", "D:\\Mods\\Old\\CE-Guns", "目标不存在"]]]],
    otherGame: ["1.5 测试", "D:\\Games\\RimWorld-1.5", [["RimHUD-dev", "D:\\dev\\RimHUD", "指向未知位置，归属未确认"]]] },
  { id: "missing", sec: "break", st: "err", step: "subscribe", title: "列表里有 3 个 Mod 没有安装",
    why: "订阅并下载后会自动回到列表里的位置。", ev: [["推断", "列表里有，Mods 文件夹和工坊目录里都没有"]],
    acts: [{ l: "全部订阅…", k: "s" }, { l: "全部从列表中移除", k: "x" }, { l: "展开", k: "f" }],
    fold: ["⟦Vanilla Genetics Expanded⟧", "⟦Alpha Animals⟧", "⟦Character Editor⟧"] },
  { id: "dep", sec: "break", st: "err", step: "enable", title: "⟦Vanilla Factions Expanded - Mechanoids⟧ 需要 ⟦Vanilla Expanded Framework⟧，但它没有启用",
    why: "⟦Vanilla Furniture Expanded⟧ 也需要它。", ev: [["Mod 自己声明", "作者在 About.xml 中列出的前置"]],
    acts: [{ l: "启用 Vanilla Expanded Framework", k: "space" }], enables: "Vanilla Expanded Framework",
    mods: ["Vanilla Factions Expanded - Mechanoids", "Vanilla Furniture Expanded", "Vanilla Expanded Framework"] },
  { id: "patch", sec: "break", st: "warn", step: "enable", title: "⟦Dubs Bad Hygiene⟧ 的 6 个补丁不会生效",
    why: "它们针对的 ⟦Rimefeller⟧ 已安装但没有启用。", ev: [["推断", "补丁 1.6/Patches/Rimefeller.xml:4 没有找到目标"]],
    acts: [{ l: "启用 Rimefeller", k: "space" }, { l: "查看", k: "enter" }], enables: "Rimefeller" },
  { id: "incompat", sec: "break", st: "err", step: "choose", title: "⟦Combat Extended⟧ 与 ⟦Yayo's Combat 3⟧ 不兼容",
    why: "两个都改战斗系统，同时启用时游戏会报错或行为异常。", ev: [["Mod 自己声明", "Combat Extended 的 About.xml 写了 incompatibleWith"]],
    acts: [{ l: "选择停用哪一个…", k: "space" }], choices: ["停用 Combat Extended", "停用 Yayo's Combat 3"] },
  { id: "dup", sec: "break", st: "err", step: "choose", title: "同一个 Mod 启用了两份：⟦Allow Tool⟧",
    why: "工坊那份 10 月 20 日更新过；本地那份在 Mods/AllowTool-old。", ev: [["推断", "两份的 packageId 都是 unlimitedhugs.allowtool"]],
    acts: [{ l: "选择保留哪一份…", k: "space" }], choices: ["保留工坊那份", "保留本地那份"] },
  { id: "version", sec: "break", st: "warn", step: "look", title: "18 个 Mod 没有标明支持 1.6",
    why: "其中 15 个标明了 1.5，不含已忽略提醒的 2 个。多数仍能用，出错时先看这些。", ev: [["Mod 自己声明", "About.xml 的 supportedVersions"]],
    acts: [{ l: "查看列表", k: "enter" }, { l: "忽略这 18 个 Mod 的 1.6 提醒", k: "M" }],
    versions: [["标明了 1.5", ["Real Ruins", "Prison Labor", "Simple Sidearms", "RimHUD", "Alpha Genes", "Character Editor", "Dubs Mint Menus", "…还有 8 个"]], ["只标明了 1.4 或更早", ["Rimefeller", "Old Texture Pack", "CE Guns"]]] },

  // last run (Player.log)
  { id: "log-vfe", sec: "run", st: "err", step: "log", phase: "加载游戏时", title: "⟦Vanilla Furniture Expanded⟧ — 12 个错误",
    why: "贴图缺失 9 次（Things/Building/Furniture/VFE_DiningTable）；定义引用了不存在的内容 3 次", ev: [["日志", "Player.log  " + LASTRUN]],
    acts: [{ l: "打开游戏日志", k: "enter" }], relates: "dep",
    log: ["Could not load Texture2D at 'Things/Building/Furniture/VFE_DiningTable' in any active mod or in base resources.", "Config error in VFE_DiningTable: graphicData.texPath not found"] },
  { id: "log-rr", sec: "run", st: "warn", step: "log", phase: "加载游戏时", title: "⟦Real Ruins⟧ — 3 个警告",
    why: "定义引用了不存在的内容", ev: [["日志", "Player.log  " + LASTRUN]], acts: [{ l: "打开游戏日志", k: "enter" }],
    log: ["Could not resolve cross-reference to Verse.ThingDef named RR_ScatteredDebris (wanter=thingDef)"] },
  { id: "log-unk", sec: "run", st: "err", step: "log", phase: "读取存档时", title: "来源不明 — 2 个错误",
    why: "NullReferenceException 2 次；尚未确定来源", ev: [["日志", "Player.log  " + LASTRUN]], acts: [{ l: "打开游戏日志", k: "enter" }],
    log: ["Exception loading list element from XML: System.NullReferenceException", "  at Verse.ScribeExtractor.SaveableFromNode[T] (System.Xml.XmlNode subNode, System.Object[] ctorArgs)"] },
  { id: "log-pl", sec: "run", st: "err", step: "log", phase: "游玩时", title: "⟦Prison Labor⟧ — 41 个错误",
    why: "异常 41 次（PrisonLabor.WorkGiver_Supervise.JobOnThing）", ev: [["日志", "ArchoMonitor.log  10 月 28 日 21:31"]], acts: [{ l: "打开游戏日志", k: "enter" }],
    log: ["Exception in JobOnThing: System.NullReferenceException", "  at PrisonLabor.WorkGiver_Supervise.JobOnThing (Verse.Pawn pawn, Verse.Thing t, System.Boolean forced)"] },
  { id: "log-old", sec: "run", st: "hint", step: "old", old: true, title: "更早的日志  9 月 29 日  1.5", counted: false },

  // for you to confirm
  { id: "ident", sec: "confirm", st: "warn", step: "choose", title: "⟦Better Pawn Control⟧ 和 ⟦Better Pawn Control (Continued)⟧ 是同一个 Mod 吗？",
    why: "包名相同，文件夹不同；12 个文件完全一样。", ev: [],
    acts: [{ l: "是同一个", k: "s", eq: true }, { l: "是副本", k: "c", eq: true }, { l: "无关", k: "u", eq: true }], whyLink: "为什么这样问",
    cmp: [["名称", "Better Pawn Control (Continued)", "Better Pawn Control", false], ["包名", "VouLT.BetterPawnControl", "VouLT.BetterPawnControl", true],
      ["作者", "Vouli, Taranchuk", "Vouli", false], ["工坊编号", "3094618023", "1541460369", false], ["文件夹", "3094618023", "BetterPawnControl", false]] },
  { id: "extern", sec: "confirm", st: "warn", step: "choose", title: "ModsConfig.xml 在 RimArcho 之外被改了，而这个列表之后也改过",
    why: "10 月 28 日 22:10 被改过，可能是游戏里的 Mod 菜单。", ev: [["推断", "ModsConfig.xml  10 月 28 日 22:10"]],
    acts: [{ l: "看差异", k: "d" }, { l: "载入…", k: "L" }, { l: "忽略", k: "I" }], choices: ["载入游戏里的改动", "保留这个列表"] },
  { id: "packlinks", sec: "confirm", st: "warn", step: "packlinks", title: "整合包「战锤 40K」在 Mods/ 留了 14 个链接",
    why: "从 RimArcho 启动列表时会移除它们，也可以现在移除。", ev: [["推断", "Mods/ 里 14 个指向整合包的链接"]], acts: [{ l: "移除链接", k: "r" }] },

  // not counted: today dimmed inside 会让游戏出错
  { id: "combo", sec: "hint", now: "break", st: "hint", step: "hint", counted: false, title: "提示：⟦Vanilla Expanded Framework⟧ 和 ⟦Performance Optimizer⟧ 一起时常报错",
    why: "遇到报错时可以先停用其中一个。", ev: [["分享的日志（截至 2026-09）", "一起启用的 60 份日志里 31 份出错（52%）"]], acts: [{ l: "选择停用哪一个…", k: "space" }] },
  { id: "obs", sec: "hint", now: "break", st: "hint", step: "hint", counted: false, title: "提示：⟦Alpha Genes⟧ 可能缺少 ⟦Alpha Memes⟧",
    why: "Alpha Memes 没有安装。遇到相应报错时可以补上。", ev: [["分享的日志（截至 2026-09）", "缺了它时 40/50（80%）出错"]], acts: [{ l: "在工坊打开", k: "o" }] },
  { id: "known", sec: "hint", now: "break", st: "hint", step: "hint", counted: false, title: "提示：分享的日志里，这个版本的 ⟦Prison Labor⟧ 经常报错",
    why: "如果你也遇到了，可以检查更新或暂时停用它。", ev: [["分享的日志（截至 2026-09）", "这个版本出现在 112 份日志里，71 份有它的报错（63%）"]], acts: [{ l: "看这个错误", k: "enter" }] },
  { id: "weak", sec: "hint", now: "break", st: "hint", step: "sort", counted: false, title: "5 条较弱的顺序要求没满足",
    why: "这是作者或社区建议的顺序，分享的日志里违反它们并没有更常出错。", ev: [["社区数据", "RimSort 社区规则"]], acts: [{ l: "自动排序", k: "S" }, { l: "展开", k: "f" }] },
  { id: "cycle", sec: "hint", now: "break", st: "hint", step: "hint", counted: false, title: "排序会按优先级处理 3 个 Mod 的循环要求",
    why: "会放弃“⟦RimHUD⟧ 排在 ⟦Real Ruins⟧ 之后”这一条（社区数据）。", ev: [["社区数据", "RimSort 社区规则"]], acts: [{ l: "查看各条要求", k: "enter" }] },

  // hidden mods: today a section here; the candidates move them to Mod 库
  { id: "pin", sec: "hidden", st: "hint", step: "hidden", counted: false, title: "⟦RimHUD⟧ 锁定了版本，工坊那份之后更新过",
    why: "", ev: [["推断", "工坊 10 月 25 日更新"]], acts: [{ l: "查看差异", k: "d" }, { l: "更新锁定版本", k: "p" }, { l: "解除锁定…", k: "P" }] },
  { id: "residue", sec: "hidden", st: "hint", step: "hidden", counted: false, title: "⟦Dubs Mint Menus⟧ 被隐藏、已经退订，文件夹还留着",
    why: "", ev: [], acts: [{ l: "移到回收站…", k: "t" }] },
  { id: "hid", sec: "hidden", st: "hint", step: "hidden", counted: false, title: "⟦Simple Sidearms⟧ 被隐藏了（大概是别的 Mod 管理器做的）",
    why: "", ev: [], acts: [{ l: "保持隐藏", k: "i" }, { l: "恢复…", k: "U" }, { l: "移到回收站…", k: "t" }] },
];
for (const it of ITEMS) if (it.counted === undefined) it.counted = true;
const byId = Object.fromEntries(ITEMS.map((x) => [x.id, x]));

const SEC = {
  break: { t: "会让游戏出错", cls: "sec-break" },
  run: { t: "上次运行的报错", cls: "sec-run", r: LASTRUN },
  confirm: { t: "要你确认", cls: "sec-confirm" },
  hint: { t: "可以留意", cls: "sec-hint" },
  hidden: { t: "隐藏的 Mod", cls: "sec-hidden" },
};

// shell draws the review strip, sidebar, breadcrumb and hints. page: which draft this is.
const DRAFTS = [["now", "现在"], ["a", "A 两列"], ["b", "B 列表 + 详情"], ["c", "C 按要做的事"]];
function shell(page, badge, hints, status = "") {
  $("#review").innerHTML = `<b>能不能跑 · 草案</b>${DRAFTS.map(([k, t]) => `<a href="${k}.html" class="${k === page ? "on" : ""}">${t}</a>`).join("")}
    <span class="sp"></span><span>示例数据，按钮不会做任何事</span><button class="rb" id="themeBtn">${ic("theme")} 浅色 / 深色</button>`;
  $("#themeBtn").onclick = () => {
    const r = document.documentElement, dark = r.dataset.theme ? r.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    r.dataset.theme = dark ? "light" : "dark";
  };
  const it = (t, extra = "", on = false) => `<button class="nav-item ${on ? "on" : ""}"><span>${t}</span>${extra}</button>`;
  $("#nav").innerHTML = `<div class="grp">在玩</div>${it("启动")}${it("能不能跑", `<span class="badge-n" id="badge">${badge}</span>`, true)}${it("游戏日志")}
    <div class="grp">库</div>${it("列表")}${it("整合包")}${it("Mod 库")}${it("贴图优化")}${it("游戏")}
    <div class="grp">没玩到</div>${it("游玩回顾")}<div class="grp">值得玩</div>${it("游玩目标")}`;
  $("#top").innerHTML = `<span class="crumb">能不能跑</span><span class="chip">${PLAN}　${GAME}</span>`;
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
  if (n) { e.stopPropagation(); toast(`去编辑器里的 ${n.textContent}`); }
}, true);
document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeMenu(); });

// linkRows: the grouped broken links (06 §2.4), one game per group.
function linkRows(groups) {
  return `<div class="links">${groups.map(([g, path, rows]) => `<div><div class="game"><b>${esc(g)}</b>${esc(path)}</div>
    ${rows.map(([f, to, why]) => `<div class="link-row"><span>${esc(f)}</span><span class="muted">→</span><span class="p" title="${esc(to)}">${esc(to)}</span><span class="muted">${esc(why)}</span><span class="tag ${why.includes("未确认") ? "keep" : ""}">${why.includes("未确认") ? "保留" : "将删除"}</span></div>`).join("")}</div>`).join("")}</div>`;
}

// needHTML: one item as the page draws it today (06 §2.4): icon, bold title, a plain why,
// the evidence line under the cursor, one main button on the right and the rest in ⋯.
// opts.links: the link groups to show; opts.open: expanded merged items.
function needHTML(it, i, opts = {}) {
  if (it.old) return `<div class="need old" data-i="${i}">${esc(it.title)}</div>`;
  const dim = !it.counted, acts = it.acts || [];
  let right = "";
  if (acts.some((a) => a.eq)) right = acts.map((a, j) => `<button class="btn line small" data-a="${j}">${esc(a.l)}</button>`).join("");
  else if (acts.length) {
    right = `<button class="btn ${dim ? "line" : "primary"} small" data-a="0">${esc(acts[0].l)}</button>`;
    if (acts.length > 1) right += `<button class="btn more small" data-more="1" title="其他动作">⋯</button>`;
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
function bindRows(root, list, setCur, onAct = (it, a) => toast(`会执行：${a.l}`)) {
  root.addEventListener("click", (e) => {
    const row = e.target.closest("[data-i]");
    if (!row || !root.contains(row)) return;
    const i = +row.dataset.i, it = list[i];
    setCur(i);
    const b = e.target.closest("button");
    if (!b || !it) return;
    if (b.dataset.more) menu(b, it.acts.slice(1), (a) => onAct(it, a));
    else if (b.dataset.why) toast("打开“为什么这样问”的比较层");
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
