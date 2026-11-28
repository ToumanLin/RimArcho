import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush, deferred } from "./dom.mjs";

// arrays made inside the page's context compare as plain data
const plain = (x) => JSON.parse(JSON.stringify(x));

const info = (description = "old", refreshing = true, error = "") => ({
  steamcmd: "fake", games: "D:\\Games", refreshing, error,
  branches: [{ name: "public", description }, { name: "version-1.5", version: "1.5", description }],
});

// found is what the first run's "found" step confirms (view.FirstRunFound); none by default
async function setup({ folder = { done: true }, space = (path) => ({ path, free: 50e9 }), scan = async () => true, from, envs = [], found = null, lang = null } = {}) {
  const screen = new Element("div"), S = { screen: "guide", busy: {}, board: { env: envs[0], envs } }, chosen = [], handlers = {}, copies = [];
  let now = 0, calls = 0, response = info(), switchWay, pathEditor;
  const timers = [], delays = [], gameCalls = [], writes = [], went = [], added = [], settings = [], segs = [];
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const guide = await loadModules("./guide.js", {
    "./fields.js": { copyablePath: (n) => n, editablePath: (n) => n, pathContent: (n) => n },
    "./state.js": { S, $: () => screen, el, api: {
      GuideFind: async () => [], GuideAdd: async (games) => { writes.push("add"); added.push(games); return games.map((g) => g.name); }, GuideScan: (names) => { writes.push("scan"); return scan(names); }, GuideDownloadInfo: async () => { calls++; return await response; },
      GuideFirstRun: async () => (typeof found === "function" ? found() : found) || { games: [], managers: [], libraries: 1, folders: [] }, GuideLanguages: async () => lang,
      GuideHold: async () => {}, GuideChoices: async () => ({ updates: true, monitor: true }), GuideChoicesDone: async (names) => { if (names.length) writes.push("choices"); },
      ApplySetting: async (argv, value) => { settings.push([argv[2], value]); writes.push("setting"); return ["ok"]; },
      GuideCopyPlan: async (from, dir, name, mods) => ({ dir: dir || "D:\\Games\\RimWorld " + from + "-copy", name: name || from + "-copy", bytes: mods ? 5e9 : 1e9, modBytes: 4e9, free: 50e9, low: false }),
      GuideCopy: async (...args) => { copies.push(args); writes.push("copy"); return true; },
      GameConfigLists: async () => [],
      ManagedFolder: async () => folder, FolderSpace: async (path) => space(path), ChooseManagedFolder: async (path) => { chosen.push(path); writes.push("folder"); },
      // as view.GameAt: the version's first name not among the environments or pending
      GuideGame: async (path, pending) => {
        gameCalls.push([path, pending]);
        let name = "1.6-2";
        for (let i = 3; pending.includes(name); i++) name = "1.6-" + i;
        return { name, game: path, version: "1.6.4871", launch: "direct" };
      },
    }, T: s => s, Tf: s => s, errText: String, say() {} },
    "./screens.js": {
      page: () => el("div"),
      button: (label, _, action) => { const b = el("button", "", label); b.onclick = action; return b; },
      seg: (names, on, action) => { switchWay = action; segs.push({ names, on, action }); return el("div", "seg", names.join("|")); },
      chooseLocalPath: (value, done) => { pathEditor = { value, done }; }, beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }),
    },
    "./exchange.js": { exchange() {}, importFrom() {} },
    "./environment.js": { withEnvironmentSwitch: async (_, run) => { await run(); return true; } },
    "./api.js": { on(name, fn) { handlers[name] = fn; } },
  }, { Date: { now: () => now }, setTimeout: (fn, ms) => { timers.push({fn,ms}); delays.push(ms); } });
  const ui = { renderWhere() {}, go: (id) => went.push(id), reload: async () => {}, relang: async () => { writes.push("relang"); }, importFirstList: () => went.push("import:first") };
  guide.initGuide(ui);
  await guide.start(from);
  // the first run's download is a tile while no game is found (16 §2.15)
  const tile = (t) => screen.querySelectorAll(".guide-tile").find(b => b.textContent.startsWith(t));
  if (folder.done && !from && !found) { tile("download a version").onclick(); await flush(); }
  return {
    ui, screen, S, timers, delays, guide, chosen, gameCalls, writes, went, handlers, copies, added, settings, segs, tile, choosePath: (value) => pathEditor.done(value),
    button: (label) => screen.querySelectorAll("button").find(b => b.textContent === label),
    calls: () => calls, now: () => now, respond: value => { response = value; },
    switchWay: n => switchWay(n),
    changeDirectory(value) { screen.querySelectorAll("button").find(b => b.textContent === "change…").onclick(); pathEditor.done(value); },
    async tick() { assert.ok(timers.length); const {fn,ms} = timers.shift(); now += ms; fn(); await flush(); },
  };
}

test("refresh polling backs off to five seconds and stops at two minutes", async () => {
  const h = await setup();
  assert.match(h.screen.textContent, /refreshing branch list…/);
  for (let i = 0; h.timers.length; i++) { assert.ok(i < 40, "polling did not stop"); await h.tick(); }
  assert.deepEqual(h.delays.slice(0, 6), [250, 500, 1000, 2000, 4000, 5000]);
  assert.ok(Math.max(...h.delays) <= 5000);
  assert.equal(h.now(), 120000);
  assert.ok(h.calls() < 30);
  const calls = h.calls(); await flush(); assert.equal(h.calls(), calls);
  assert.equal(h.screen.querySelectorAll(".branch").length, 2);
  h.respond(info("fresh", false));
  h.guide.resume(); await flush();
  assert.match(h.screen.querySelector(".branches").textContent, /fresh/);
  assert.doesNotMatch(h.screen.textContent, /refreshing branch list…/);
});

test("the suggested game folder uses the games folder's own separator (Linux: /)", async () => {
  const h = await setup();
  h.respond({ ...info("fresh", false), games: "/home/player/Games" }); await h.tick();
  h.screen.querySelectorAll(".branch")[1].onclick();
  assert.ok(h.screen.textContent.includes("/home/player/Games/RimWorld 1.5"), h.screen.textContent);
  assert.doesNotMatch(h.screen.textContent, /Games\\RimWorld/);
});

test("refresh completion keeps selected branch and typed directory", async () => {
  const h = await setup();
  h.screen.querySelectorAll(".branch")[1].onclick();
  h.changeDirectory("D:/custom");
  h.respond(info("fresh", false)); await h.tick();
  assert.match(h.screen.querySelector(".branches").textContent, /fresh/);
  assert.match(h.screen.querySelector(".branch.on").textContent, /1.5/);
  assert.ok(h.screen.textContent.includes("D:/custom"));
  assert.equal(h.screen.querySelectorAll("input").length, 1, "only the account remains an input");
  assert.doesNotMatch(h.screen.textContent, /refreshing branch list…/);
  assert.equal(h.timers.length, 0);
});

test("failed refresh retains branches and shows its error", async () => {
  const h = await setup();
  h.respond(info("old", false, "offline")); await h.tick();
  assert.equal(h.screen.querySelectorAll(".branch").length, 2);
  assert.match(h.screen.textContent, /offline/);
  assert.doesNotMatch(h.screen.textContent, /refreshing branch list…/);
  assert.equal(h.timers.length, 0);
});

test("leaving the page or switching away stops polling", async () => {
  for (const way of ["page", "tab"]) {
    const h = await setup(); const calls = h.calls();
    if (way === "page") h.S.screen = "home"; else h.button("back").onclick();
    await h.tick();
    assert.equal(h.calls(), calls); assert.equal(h.timers.length, 0);
  }
});

test("a response arriving after navigation cannot redraw the page", async () => {
  const h = await setup(); const reply = deferred(); h.respond(reply.promise);
  await h.tick(); h.S.screen = "home";
  const before = h.screen.textContent;
  reply.resolve(info("late", false)); await flush();
  assert.equal(h.screen.textContent, before); assert.equal(h.timers.length, 0);
});

test("a first run takes the default managed folder without asking (16 §2.15)", async () => {
  const h = await setup({ folder: { path: "C:\\RimArcho", free: 50e9, done: false } });
  assert.deepEqual(h.chosen, ["C:\\RimArcho"]);
  assert.equal(h.screen.querySelectorAll(".st").length, 3);
  assert.match(h.screen.querySelector(".st.cur").textContent, /found/);
});

test("a default managed folder that cannot be used starts on that step, which has no way to skip it", async () => {
  const button = (h, label) => h.screen.querySelectorAll("button").find(b => b.textContent === label);
  const h = await setup({ folder: { path: "C:\\RimArcho", free: 50e9, done: false, error: "not writable" }, space: (path) => ({ path, free: 2e9, low: true }) });
  assert.equal(h.screen.querySelectorAll(".st").length, 4);
  assert.match(h.screen.querySelector(".st.cur").textContent, /managed folders/);
  assert.ok(h.screen.textContent.includes("C:\\RimArcho"));
  assert.equal(button(h, "not now"), undefined);
  assert.equal(button(h, "back"), undefined);
  // another folder: its free space and the low-space warning; nothing is saved yet
  button(h, "change…").onclick(); h.choosePath("E:\\Archo"); await flush();
  assert.ok(h.screen.textContent.includes("E:\\Archo"));
  assert.match(h.screen.textContent, /Less than 10 GB/);
  assert.deepEqual(h.chosen, []);
  // confirming saves it and moves on to "found"; the step stays, done
  button(h, "use this folder").onclick(); await flush();
  assert.deepEqual(h.chosen, ["E:\\Archo"]);
  assert.match(h.screen.querySelector(".st.cur").textContent, /found/);
  assert.match(h.screen.querySelector(".st.done").textContent, /managed folders/);
  assert.ok(button(h, "back"), "the folder step can be revisited");
});

test("a managed folder already chosen has no folder step; the first run has three steps", async () => {
  const h = await setup();
  assert.deepEqual(h.screen.querySelectorAll(".st").map(s => s.textContent), ["1found", "2read the mods", "3first list"]);
  assert.match(h.screen.querySelector(".st.cur").textContent, /found/);
});

test("no game found: two tiles, where RimArcho looked, and the download has a way back", async () => {
  const h = await setup({ found: { games: [], managers: [], libraries: 2, folders: ["D:\\Games"] } });
  assert.equal(h.screen.querySelector(".guide-q").textContent, "No RimWorld found");
  assert.equal(h.screen.querySelectorAll(".guide-tile").length, 2);
  assert.match(h.screen.querySelector(".guide-tried").textContent, /looked in %s and %s/);
  h.tile("download a version").onclick(); await flush();
  assert.match(h.screen.textContent, /refreshing branch list…|asking Steam/);
  h.button("back").onclick(); await flush();
  assert.equal(h.screen.querySelector(".guide-q").textContent, "No RimWorld found");
});

test("two folders of one version added in one pass get different names", async () => {
  const h = await setup({ found: { games: [], managers: [], libraries: 1, folders: [] } });
  h.tile("choose its folder").onclick(); await h.choosePath("D:\\Game2"); await flush();
  h.button("add another folder").onclick(); await h.choosePath("D:\\Game3"); await flush();
  assert.deepEqual(h.gameCalls.map(([, pending]) => pending), [[], ["1.6-2"]]);
  // no name fields: the names are the versions (16 §2.15)
  assert.equal(h.screen.querySelectorAll(".game .field").length, 0);
  await h.button("add %d games and read Mods").onclick(); await flush();
  assert.deepEqual(plain(h.added[0].map(g => g.name)), ["1.6-2", "1.6-3"]);
});

// 1.6 and 1.7 on one LocalLow, and a folder RimSort uses (16 §2.15)
const two = () => ({
  libraries: 2, folders: [],
  managers: [{ manager: "RimSort", path: "E:\\RimMods", mods: 27 }],
  games: [
    { name: "1.6", game: "D:\\S\\RimWorld", version: "1.6.4633 rev1261", source: "Steam", launch: "steam", userData: "C:\\LL\\RimWorld", workshop: "D:\\S\\ws", workshopMods: 1204, mods: 3, gameMods: 13 },
    { name: "1.7", game: "D:\\Games\\RimWorld 1.7", version: "1.7.5014 rev1300", source: "started directly", launch: "direct", userData: "c:\\ll\\rimworld", mods: 0, gameMods: 13 },
  ],
});

test("found: game cards, Mod folders and the second version's own saves by default", async () => {
  const h = await setup({ found: two() });
  assert.equal(h.screen.querySelector(".guide-q").textContent, "RimArcho found these. Untick what it should leave alone.");
  const cards = h.screen.querySelectorAll(".game");
  assert.equal(cards.length, 2);
  assert.match(cards[0].textContent, /RimWorld 1\.6.*4633 rev1261.*Steam/);
  assert.equal(cards[0].querySelectorAll(".saves").length, 0, "the first game asks nothing");
  assert.equal(cards[1].querySelector(".saves .seg").textContent, "shared with %s|its own");
  assert.equal(h.segs.find(s => s.names[1] === "its own").on, 1, "its own by default");
  // the Workshop folder, each game's Mods folder, the manager's folder (ticked)
  const rows = h.screen.querySelectorAll(".fbox .set");
  assert.deepEqual(rows.map(r => r.querySelector(".t").textContent), ["Steam Workshop", "Mods folder of %s", "Mods folder of %s", "used by %s"]);
  assert.ok(rows[3].querySelector("input").checked);
  await h.button("add %d games and read Mods").onclick(); await flush();
  const [g16, g17] = h.added[0];
  assert.deepEqual(plain([g16.saves, g16.gameList, g16.extra]), ["", false, ["E:\\RimMods"]]);
  assert.deepEqual(plain([g17.saves, g17.gameList, g17.extra]), ["own", true, []]);
});

test("found: shared saves and an unticked manager folder are what gets added", async () => {
  const h = await setup({ found: two() });
  h.segs.find(s => s.names[1] === "its own").action(0); await flush();
  const box = h.screen.querySelectorAll(".fbox .set")[3].querySelector("input");
  box.checked = false; box.onchange();
  await h.button("add %d games and read Mods").onclick(); await flush();
  const [g16, g17] = h.added[0];
  assert.deepEqual(plain([g16.extra, g17.saves, g17.gameList]), [[], "", false]);
});

test("found: unticking the first game leaves the second alone, with nothing to share", async () => {
  const h = await setup({ found: two() });
  const box = h.screen.querySelectorAll(".game")[0].querySelector("input");
  box.checked = false; box.onchange(); await flush();
  assert.equal(h.screen.querySelectorAll(".saves").length, 0);
  await h.button("add and read Mods").onclick(); await flush();
  assert.deepEqual(plain(h.added[0].map(g => [g.name, g.saves])), [["1.7", ""]]);
});

test("found: the language row names the system's language and applies a pick in place", async () => {
  const h = await setup({ found: two(), lang: { value: "auto", codes: ["en", "zh"], labels: ["English", "Chinese"], system: "English" } });
  const row = h.segs.find(s => s.names[0] === "follow system (%s)");
  assert.deepEqual([row.names.length, row.on], [3, 0]);
  await row.action(2); await flush();
  assert.deepEqual(h.settings, [["ui.lang", "zh"]]);
  assert.ok(h.writes.includes("relang"));
});

test("found: a language pick rereads the game cards' chips and keeps the picks", async () => {
  let chip = "started directly";
  const h = await setup({ found: () => { const f = two(); f.games[1].source = chip; return f; }, lang: { value: "auto", codes: ["en", "ja"], labels: ["English", "Japanese"], system: "English" } });
  const box = h.screen.querySelectorAll(".game")[0].querySelector("input");
  box.checked = false; box.onchange(); await flush();
  chip = "started directly (ja)";
  await h.segs.find(s => s.names[0] === "follow system (%s)").action(2); await flush();
  const cards = h.screen.querySelectorAll(".game");
  assert.ok(cards[1].textContent.includes("started directly (ja)"));
  assert.equal(cards[0].querySelector("input").checked, false, "the unticked game stays unticked");
});

for (const way of ["throws", "refuses"]) test(`a first scan that ${way} gives back the scan flag and ends the step`, async () => {
  const h = await setup({ scan: async () => { if (way === "throws") throw "scan failed"; return false; }, found: { games: [], managers: [], libraries: 1, folders: [] } });
  h.tile("choose its folder").onclick(); await h.choosePath("D:\\Game2"); await flush();
  await h.button("add and read Mods").onclick(); await flush();
  assert.ok(!h.S.busy.scan, "no scan runs");
  assert.match(h.screen.textContent, way === "throws" ? /scan failed/ : /a scan is already running/);
  if (way === "throws") assert.ok(h.button("rescan"));
});

test("read the mods: the two switches write their settings at once, and next releases the checks", async () => {
  const h = await setup({ found: two() });
  await h.button("add %d games and read Mods").onclick(); await flush();
  assert.match(h.screen.textContent, /while you wait/);
  const [updates, monitor] = h.segs.filter(s => s.names.join() === "on,off").slice(-2);
  await updates.action(1); await flush();
  const [, monitor2] = h.segs.filter(s => s.names.join() === "on,off").slice(-2);
  await monitor2.action(1); await flush();
  assert.deepEqual(h.settings, [["updates.auto", "off"], ["monitor.default", "off"]]);
  assert.equal(monitor.on, 0);
});

test("the preview shows the first run from finding the game and writes nothing", async () => {
  const h = await setup({ from: "preview", folder: { path: "C:\\RimArcho", free: 50e9, done: false } });
  // a first run takes the default folder, so the preview has no folder step and chooses none
  assert.equal(h.screen.querySelectorAll(".st").length, 3);
  assert.deepEqual(h.chosen, []);
  assert.match(h.screen.textContent, /nothing you choose here is saved/);
  assert.match(h.screen.querySelector(".st.cur").textContent, /found/);
  h.tile("choose its folder").onclick(); await h.choosePath("D:\\Game2"); await flush();
  await h.button("add and read Mods").onclick(); await flush();
  // the scan plays out on timers
  assert.match(h.screen.querySelector(".st.cur").textContent, /read the mods/);
  assert.match(h.screen.querySelector(".guide-q").textContent, /Reading your Mods/);
  // the switches show, and a pick writes nothing
  const updates = h.segs.filter(s => s.names.join() === "on,off").at(-2);
  await updates.action(1); await flush();
  for (let i = 0; h.timers.length; i++) { assert.ok(i < 40, "the made-up scan did not end"); await h.tick(); }
  assert.match(h.screen.textContent, /✓/);
  // the heading no longer says reading once the scan has ended (VM 2026-11-22)
  assert.equal(h.screen.querySelector(".guide-q").textContent, "Your Mods are read");
  // where the first list comes from is Import (16 §2.15): a preview ends here
  await h.button("next").onclick(); await flush();
  assert.deepEqual(h.writes, []);
  assert.deepEqual(h.went, ["settings"]);
});

test("the preview's download plays out without SteamCMD", async () => {
  const h = await setup({ from: "preview" });
  h.respond({ games: "", offer: { bytes: 1e6, dir: "C:\\x" } });
  h.tile("download a version").onclick(); await flush();
  h.screen.querySelectorAll("button").find(b => b.textContent === "download SteamCMD").onclick(); await flush();
  assert.equal(h.screen.querySelectorAll(".branch").length, 3);
  await h.screen.querySelectorAll("button").find(b => b.textContent === "download and add").onclick(); await flush();
  for (let i = 0; h.timers.length; i++) { assert.ok(i < 60, "the made-up download did not end"); await h.tick(); }
  assert.match(h.screen.querySelector(".st.cur").textContent, /read the mods/);
  assert.match(h.screen.textContent, /✓/);
  assert.deepEqual(h.writes, []);
});

// Add Game Config (S.guideFrom "env"): the current config's choices follow a switch.
async function addingSetup({ accept = true } = {}) {
  const screen = new Element("div"), S = { screen: "guide", busy: {}, board: { env: "A", envs: ["A", "B"] } };
  const listCalls = [], added = [];
  const games = () => [
    { name: "1.6-2", game: "D:\A", version: "1.6", launch: "steam", env: "A", known: true, current: S.board.env === "A" },
    { name: "1.5", game: "D:\B", version: "1.5", launch: "steam", env: "B", known: true, current: S.board.env === "B" },
  ];
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const Tf = (f, ...a) => { let i = 0; return f.replace(/%[sd]/g, () => a[i++]); };
  let accepting = accept;
  const guide = await loadModules("./guide.js", {
    "./fields.js": { copyablePath: (n) => n, editablePath: (n) => n, pathContent: (n) => n },
    "./state.js": { S, $: () => screen, el, api: {
      GuideFind: async () => games(), GuideAdd: async (gs) => { added.push(...gs); return gs.map((g) => g.name); },
      GameConfigLists: async (from) => { listCalls.push(from); return ["list of " + from]; },
      GuideScan: async () => true, GuideDownloadInfo: async () => info(),
    }, T: s => s, Tf, errText: String, say() {} },
    "./screens.js": {
      page: () => el("div"),
      button: (label, _, action) => { const b = el("button", "", label); b.onclick = action; return b; },
      seg: (names) => el("div", "seg", names.join("|")),
      chooseLocalPath() {}, beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }),
    },
    "./exchange.js": { exchange() {}, importFrom() {} },
    "./environment.js": { withEnvironmentSwitch: async (_, run) => { await run(); return accepting; } },
    "./api.js": { on() {} },
  }, { Option: class extends Element { constructor(text, value) { super("option", "", text); this.value = value; } }, setTimeout() {} });
  const ui = { renderWhere() {}, reload: async () => {}, useEnv: async () => {}, go() {} };
  guide.initGuide(ui);
  await guide.start("env");
  const button = (label) => screen.querySelectorAll("button").find((b) => b.textContent === label);
  const radios = () => screen.querySelectorAll("input").filter((i) => i.type === "radio");
  return { screen, S, guide, listCalls, added, button, radios, accept: (v) => { accepting = v; } };
}

test("Add Game Config rereads the current config after a switch", async () => {
  const h = await addingSetup();
  assert.deepEqual(h.listCalls, ["A"]);
  assert.match(h.screen.textContent, /copy the settings of A/);
  h.S.board.env = "B";
  await h.guide.envChanged(); await flush();
  assert.deepEqual(h.listCalls, ["A", "B"]);
  assert.match(h.screen.textContent, /copy the settings of B/);
  assert.match(h.screen.textContent, /list of B/);
  assert.doesNotMatch(h.screen.textContent, /list of A/);
  // the previous config's game gives way to the new one's, which copies B
  assert.ok(h.radios()[1].checked);
  await h.button("add Game Config").onclick(); await flush();
  assert.equal(h.added[0].like, "B");
  // a game picked by hand stays picked across a switch
  const k = await addingSetup();
  k.radios()[1].onchange(); await flush();
  k.S.board.env = "C"; await k.guide.envChanged(); await flush();
  assert.ok(k.radios()[1].checked);
});

test("a game folder other than the current config's starts with game defaults", async () => {
  const h = await addingSetup();
  h.radios()[1].onchange(); await flush();
  assert.doesNotMatch(h.screen.textContent, /copy the settings of/);
  assert.match(h.screen.textContent, /A uses another game folder/);
  await h.button("add Game Config").onclick(); await flush();
  assert.equal(h.added[0].game, "D:\B");
  assert.equal(h.added[0].like, "", "env add --like would refuse another game folder");
  // the current config's own game copies its settings
  const k = await addingSetup();
  await k.button("add Game Config").onclick(); await flush();
  assert.equal(k.added[0].like, "A");
});

test("an add superseded by another switch leaves the button usable", async () => {
  const h = await addingSetup({ accept: false });
  await h.button("add Game Config").onclick(); await flush();
  assert.equal(h.button("add Game Config").disabled, false);
});

test("copying a game folder: the plan, the copy's events, then reading its mods", async () => {
  const button = (h, label) => h.screen.querySelectorAll("button").find(b => b.textContent === label);
  const h = await setup({ from: "env", envs: ["1.6", "1.5"] });
  h.switchWay(2); await flush();
  assert.ok(h.screen.textContent.includes("D:\\Games\\RimWorld 1.6-copy"));
  assert.match(h.screen.textContent, /copies %s; free: %s/);
  // the Mods folder too: the plan is asked again
  const mods = h.screen.querySelectorAll("button").find(b => b.getAttribute("aria-label") === "the game's Mods folder");
  mods.value = "yes"; mods.onchange(); await flush();
  await button(h, "copy and add").onclick(); await flush();
  assert.deepEqual(h.copies, [["1.6", "D:\\Games\\RimWorld 1.6-copy", "", true, false]]);
  h.handlers.copy({ bytes: 5e8, total: 1e9 }); await flush();
  assert.match(h.screen.textContent, /copying the game…/);
  h.handlers.copy({ end: true, ok: true, env: "1.6-copy" }); await flush();
  assert.match(h.screen.querySelector(".st.cur").textContent, /read the mods/);
  assert.deepEqual(h.writes, ["copy", "scan"]);
});

// VM round 2A 2026-11-23: opened again after a copy, the way kept its pick and asked no plan,
// so "copy and add" stayed off with no size line and no reason.
test("the copy way opened again from the Game Config page after a copy asks a new plan", async () => {
  const button = (h, label) => h.screen.querySelectorAll("button").find(b => b.textContent === label);
  const h = await setup({ from: "env", envs: ["1.6", "1.5"] });
  h.switchWay(2); await flush();
  await button(h, "copy and add").onclick(); await flush();
  h.handlers.copy({ end: true, ok: true, env: "1.6-copy" }); await flush();
  // the Game Config page's "copy the game folder…" opens the way directly
  h.S.guideWay = 2;
  await h.guide.start("env"); await flush();
  assert.match(h.screen.textContent, /copies %s; free: %s/);
  assert.equal(button(h, "copy and add").disabled, false);
});

test("the first run has no copy way, a preview with a Game Config neither (16 §2.15)", async () => {
  for (const h of [await setup(), await setup({ from: "preview", envs: ["1.6"] })]) {
    assert.doesNotMatch(h.screen.textContent, /copy a game folder|copy and add/);
  }
});

test("after the scan the first run hands over to Import, which ends the guide", async () => {
  const h = await setup({ found: { games: [], managers: [], libraries: 1, folders: [] } });
  assert.ok(h.S.firstRun, "the sidebar waits for the guide's end");
  h.tile("choose its folder").onclick(); await h.choosePath("D:\\Game2"); await flush();
  await h.button("add and read Mods").onclick(); await flush();
  h.handlers.guide?.({ done: true, ok: true });
  h.guide.guideEvent({ done: true, ok: true }); await flush();
  const next = h.button("next");
  assert.ok(next && !next.disabled, "the scan ended");
  await next.onclick(); await flush();
  // step 2's choices hold from here (API.GuideChoicesDone), then Import
  assert.deepEqual(h.writes, ["add", "scan", "choices"]);
  assert.deepEqual(h.went, ["import:first"]);
  await h.ui.afterImport(""); await flush();
  assert.deepEqual(h.went, ["import:first", "home"]);
  assert.ok(!h.S.firstRun, "the sidebar fills in");
});
