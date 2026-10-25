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
const SRC = { ws: ["steam", "工坊"], cmd: ["cmd", "SteamCMD"], git: ["git", "Git"], ll: ["globe", "LoversLab"], local: ["folder", "本地"] };
const src = (k) => `<span class="src">${ic(SRC[k][0])}${SRC[k][1]}</span>`;

// What waits for the player: sources RimArcho updates itself (Steam updates the Workshop).
const PENDING = [
  { id: "pf", n: "Performance Fish", s: "git", from: "a1b2c3d", fromD: "10月12日", to: "7f3e9a1", toD: "10月23日", track: "默认分支 main",
    used: ["主力殖民地", "轻量开荒"], kind: "commits",
    log: [["7f3e9a1", "Fix null ref in pawn cache after reload", "10月23日"], ["5c02b4e", "Support 1.6 hotfix 2", "10月21日"], ["e91d77a", "Bump version to 0.6.3", "10月21日"], ["0b3fa10", "Cleanup: remove unused patches", "10月18日"]],
    impact: "代码改了；会影响启动" },
  { id: "pl", n: "Prison Labor", s: "git", from: "v2.1.0", fromD: "9月30日", to: "v2.2.0", toD: "10月20日", track: "标签 v*",
    used: ["主力殖民地"], kind: "commits",
    log: [["v2.2.0", "Release 2.2.0: new work types, Chinese translation", "10月20日"]], impact: "Defs 和翻译改了" },
  { id: "bs", n: "Body Shape Patch", s: "ll", from: "2.3", fromD: "9月02日", to: "2.4", toD: "10月20日",
    used: ["战锤 40K"], kind: "files", page: "loverslab.com/files/file/18342-body-shape-patch",
    files: [["BodyShapePatch-2.4.zip", "12.4 MB", "10月20日", true], ["BodyShapePatch-2.4-legacy-1.5.zip", "11.9 MB", "10月20日", false]],
    note: "作者写的更新说明：适配 1.6；修复体型贴图偏移。", impact: "还没下载，下载后才知道改了什么" },
  { id: "ce", n: "Combat Extended", s: "cmd", from: "10月10日", fromD: "", to: "10月22日", toD: "", used: ["主力殖民地"], kind: "notes",
    note: "工坊更新说明：Fixed ammo loading for mortars; new ammo for VFE Mechanoids.", impact: "还没下载" },
];
// What Steam already changed since the current list was last played (modchanges tiers).
const CHANGES = [
  { id: "vef", n: "Vanilla Expanded Framework", t: 1, say: "代码改了：Harmony 补丁新增 3 个、改了 1 个", when: "10月23日", used: ["主力殖民地", "轻量开荒"],
    groups: [["代码 (.dll)", "改了 2 个"], ["Defs", "改了 6 个"], ["Patches", "新增 1 个"]] },
  { id: "har", n: "Humanoid Alien Races", t: 1, say: "需要的前置多了 Harmony 2.3；代码改了", when: "10月23日", used: ["主力殖民地"],
    groups: [["代码 (.dll)", "改了 1 个"], ["About", "前置新增 brrainz.harmony"]] },
  { id: "rr", n: "Real Ruins", t: 2, say: "支持的版本加了 1.6；Defs 改了 4 个", when: "10月22日", used: ["主力殖民地"], groups: [["Defs", "改了 4 个"], ["About", "支持 1.6"]] },
  { id: "dbh", n: "Dubs Bad Hygiene", t: 2, say: "Patches 改了 2 个，Defs 新增 1 个", when: "10月22日", used: ["主力殖民地", "轻量开荒"], groups: [["Patches", "改了 2 个"], ["Defs", "新增 1 个"]] },
  { id: "vfe", n: "Vanilla Furniture Expanded", t: 3, say: "贴图 +3.2 MB", when: "10月23日", used: ["主力殖民地"], groups: [["贴图", "+3.2 MB"]] },
  { id: "rs", n: "RimSaves", t: 3, say: "翻译改了 3 个", when: "10月22日", used: ["主力殖民地"], groups: [["翻译", "改了 3 个"]] },
];
const ARRIVALS = { add: ["Vanilla Gravship Expanded", "Allow Tool"], gone: ["Rimefeller"] };
const OLDER = [{ day: "10月21日", n: 7 }, { day: "10月19日", n: 3 }, { day: "10月15日", n: 12 }];
const TIER = { 1: "会影响启动", 2: "留意", 3: "只是内容" };
const SOURCES = [
  { k: "ws", c: 412, m: "Steam 自己下载更新", r: '<span class="warn">2 个已找不到</span>' },
  { k: "cmd", c: 6, m: "14:02 检查过", r: "1 个有更新", acct: "账户 rim_player" },
  { k: "git", c: 3, m: "14:02 检查过", r: "2 个有更新" },
  { k: "ll", c: 2, m: "14:02 检查过", r: "1 个有更新", acct: "已登录，11月30日到期" },
  { k: "local", c: 38, m: "放在 Mods 文件夹，没有来源", r: "" },
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
  const foot = (left, main) => `<div class="found-f"><span class="muted">${left}</span><button class="btn" data-do="clear">取消<kbd>esc</kbd></button><button class="btn primary" data-do="add">${main}<kbd>enter</kbd></button></div>`;
  if (kind === "bad") return `<div class="found${enter}"><div class="found-h"><span class="ico">${ic("info")}</span><b>认不出这个地址</b>
    <button class="btn x" data-do="clear">${ic("x")}</button><div class="meta">支持 GitHub / GitLab 仓库、LoversLab 的文件页、工坊链接或 ID、下载好的 zip</div></div></div>`;
  if (kind === "git") return `<div class="found${enter}">${head("git", "Performance Fish", `<span>${src("git")}</span><span>bbradson/Performance-Fish</span><span>最新提交 2 天前</span>`)}
    <div class="found-b">
      <div class="facts"><div class="f"><span class="ok">✓</span><span>找到 About/About.xml，包名 <code>bs.performance</code>，支持 1.5、1.6</span></div>
        <div class="f"><span class="warn">!</span><span>你已经有工坊那份 Performance Fish（主力殖民地在用）。两份同包名，游戏只加载一份；订阅后在列表里选用哪一份</span></div></div>
      <div class="vrow" style="display:flex;align-items:center;gap:10px"><span class="muted" style="font-size:12px">跟踪</span>
        <div class="seg">${["默认分支 main", "最新的标签", "固定在这个提交"].map((x, i) => `<button class="${(st.track || 0) === i ? "on" : ""}" data-track="${i}">${x}</button>`).join("")}</div></div>
    </div>${foot("克隆到 Mods 文件夹，以后检查更新时比较提交", "订阅")}</div>`;
  if (kind === "ll") {
    const f = st.file ?? 0;
    return `<div class="found${enter}">${head("globe", "Gene Tweaks", `<span>${src("ll")}</span><span>作者 Kaz</span><span>版本 1.2</span><span>10月18日更新</span>`)}
    <div class="found-b">
      <div class="note acc">${ic("git")}<span>这个页面链接了 GitHub 仓库 <code>kaz/gene-tweaks</code>。用 Git 订阅，以后更新不用再选文件。</span><button class="btn link" data-sample="git">改用 Git</button></div>
      <div class="choice">${[["GeneTweaks-1.2.zip", "4.1 MB", "10月18日"], ["GeneTweaks-1.2-nolore.zip", "3.6 MB", "10月18日"]].map(([n, s, d], i) =>
        `<button class="opt ${f === i ? "on" : ""}" data-file="${i}"><span class="rd"></span><span class="nm">${n}${i === 0 ? "<small>最大的那个</small>" : ""}</span><span class="ext">${s}  ${d}</span></button>`).join("")}</div>
      <div class="facts"><div class="f"><span class="ok">✓</span><span>已登录 LoversLab（记住我，11月30日到期）</span></div>
        <div class="f"><span class="muted">i</span><span>下载前会显示站点的下载协议；同意后才下载</span></div></div>
    </div>${foot("没登录也行：在浏览器下载 zip，拖到这里", "下载并订阅")}</div>`;
  }
  if (kind === "ws") {
    const w = st.via ?? 0;
    return `<div class="found${enter}">${head("steam", "Allow Tool", `<span>${src("ws")}</span><span>ID 2009463077</span><span>UnlimitedHugs</span><span>支持 1.6</span>`)}
    <div class="found-b"><div class="choice">
      <button class="opt ${w === 0 ? "on" : ""}" data-via="0"><span class="rd"></span><span class="nm">用 Steam 订阅<small>推荐</small></span><span class="ext ok">Steam 在运行</span><span class="ds">Steam 下载，以后自动更新；变化记在下面</span></button>
      <button class="opt ${w === 1 ? "on" : ""}" data-via="1"><span class="rd"></span><span class="nm">用 SteamCMD 下载</span><span class="ext">账户 rim_player</span><span class="ds">不经过 Steam 客户端；以后由 RimArcho 检查更新</span></button>
    </div></div>${foot("前置 HugsLib 已经装了", "订阅")}</div>`;
  }
  if (kind === "zip") return `<div class="found${enter}">${head("zip", "GeneTweaks-1.2.zip", `<span>4.1 MB</span><span>里面是 1 个 Mod：Gene Tweaks（kaz.genetweaks）</span>`)}
    <div class="found-b">
      <label class="omni" style="height:32px"><span class="muted" style="white-space:nowrap;font-size:12px">从哪下载的</span><input placeholder="粘贴 LoversLab 文件页（可不填）" value="${st.from ? "https://www.loverslab.com/files/file/20511-gene-tweaks/" : ""}" data-from></label>
      <div class="facts"><div class="f"><span class="muted">i</span><span>${st.from ? "填了页面：以后能检查更新，有新版时在这里提醒你" : "不填：装成本地 Mod，不检查更新"}</span></div></div>
    </div>${foot("解压到 Mods 文件夹", "添加")}</div>`;
  return "";
}

// nav draws the sidebar and breadcrumb. on: "subs" or "add"; withAdd: show the 添加订阅 sub-page.
function nav(kidBadge, on = "subs", withAdd = false) {
  const it = (t, extra = "", cls = "") => `<button class="nav-item ${cls}"><span>${t}</span>${extra}</button>`;
  const chev = `<span class="chev">${ic("chev")}</span>`;
  const kid = (k, t, extra = "") => `<button class="nav-kid ${on === k ? "on" : ""}" data-page="${k}">${t}${extra}</button>`;
  $("#nav").innerHTML = `<div class="grp">在玩</div>${it("启动")}${it("能不能跑", '<span class="badge-n">2</span>')}${it("游戏日志")}
    <div class="grp">库</div>${it("Mod 库", chev, "open")}
    <div class="kids"><button class="nav-kid">全部</button>${kid("subs", "订阅与更新", kidBadge ? `<span class="badge-n acc">${kidBadge}</span>` : "")}${withAdd ? kid("add", "添加订阅") : ""}</div>
    ${it("列表", chev)}${it("整合包", chev)}${it("贴图优化")}${it("游戏")}`;
  $("#crumbs").innerHTML = `<span class="crumb">库</span><span class="sep">/</span><span class="crumb">Mod 库</span><span class="sep">/</span><span class="crumb last">${on === "add" ? "添加订阅" : "订阅与更新"}</span>`;
}
function hints(arr) { $("#hints").innerHTML = arr.map(([k, d]) => `<span><span class="k">${k}</span>${d}</span>`).join(""); }
let sayT;
function say(t) { $("#status").innerHTML = `<span class="ok">✓</span> ${t}`; clearTimeout(sayT); sayT = setTimeout(() => ($("#status").innerHTML = ""), 5000); }
function sourcesPopover(anchor) {
  const r = anchor.getBoundingClientRect();
  const v = $("#veil"); v.hidden = false;
  v.innerHTML = `<div class="popover" style="top:${r.bottom + 6}px;left:${Math.max(8, Math.min(r.left, innerWidth - 370))}px">
    ${SOURCES.map((s) => `<div class="srcrow">${ic(SRC[s.k][0])}<span>${SRC[s.k][1]}  <span class="muted">${s.c} 个</span></span><span class="r">${s.k === "ws" || s.k === "local" ? "" : '<button class="btn small">检查</button>'}</span>
      <span class="m">${[s.m, s.r, s.acct].filter(Boolean).join("，")}</span></div>`).join("")}
    <div class="srcrow"><span></span><span class="m">工坊 2 个已找不到：被删除或设为私有。<button class="btn link">看是哪两个</button></span></div>
    <div class="srcrow"><span></span><span class="m">账户在设置里管理 <button class="btn link">设置 › 账户 →</button></span></div></div>`;
}
function themeButton() {
  $("#theme").onclick = () => {
    const d = document.documentElement, dark = d.dataset.theme ? d.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    d.dataset.theme = dark ? "light" : "dark";
  };
}
