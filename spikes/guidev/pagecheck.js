// Open /dev/pagecheck in guidev. Test real modules/DOM with delayed API replies and
// synthetic large logs; the mock runtime makes this page independent of user data.
import { handlers, emit } from "./pagecheck-runtime.js";
import { S, api } from "/js/state.js";
import * as screens from "/js/screens.js";
import * as library from "/js/library.js";
import * as lists from "/js/lists.js";
import * as tasks from "/js/tasks.js";
import * as textures from "/js/textures.js";
import * as home from "/js/home.js";
import * as packs from "/js/packs.js";
import * as guide from "/js/guide.js";
import * as uninstall from "/js/uninstall.js";
import * as full from "/js/fulldetail.js";
import { withEnvironmentSwitch } from "/js/environment.js";

const output = document.querySelector("#results"), results = [];
const assert = (ok, message) => { if (!ok) throw new Error(message); };
const settle = async () => { for (let i = 0; i < 5; i++) await new Promise(requestAnimationFrame); };
const key = (name) => screens.logsKey(new KeyboardEvent("keydown", { key: name, cancelable: true }));
const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; };
const visibleSelection = () => {
  const row = document.querySelector(".lrow.cur"), scroll = row?.closest(".scroll");
  if (!row || !scroll) return false;
  const r = row.getBoundingClientRect(), s = scroll.getBoundingClientRect();
  return r.top < s.bottom && r.bottom > s.top;
};
const ui = { renderWhere() {}, renderHints() {}, reload() {}, refreshEnvironment() {}, cmd() {}, go() {} };
screens.initScreens(ui);
library.initLibrary(ui);
tasks.initTasks(ui);
home.initHome(ui); packs.initPacks(ui); guide.initGuide(ui); full.initFull(ui);
handlers.set("Setting", () => "");
handlers.set("LogCluster", () => ["fixture detail"]);
handlers.set("LogPattern", () => null);

async function test(name, run) { await run(); results.push("PASS " + name); output.textContent = results.join("\n"); }

try {
  // Use the product's editor markup so the virtual-row checks exercise its real DOM.
  const markup = new DOMParser().parseFromString(await (await fetch("/")).text(), "text/html");
  document.querySelector("#app").prepend(markup.querySelector("#board"));
  const editor = () => lists.openEditor(async (read) => {
    const workspace = await api.Workspace();
    if (!read.current()) return false;
    read.commit(() => { S.board = workspace.board; S.shell = workspace.shell; });
    for (let k = 0; k < 3; k++) {
      lists.setItems(k, workspace.board.cols[k]);
      await lists.refilter(k);
      if (!read.current()) return false;
    }
    return true;
  }, false, () => { for (let k = 0; k < 3; k++) lists.renderRows(k); });
  const workspace = { board: { env: "fixture", cols: [[], [], []] }, shell: { plan: { id: 1 } } };
  const texturePage = { scales: [100], labels: ["100%"], rows: [] };
  const backgrounds = { setting: { key: "plain", stars: [], files: [] }, groups: [{ name: "game", items: [{ key: "plain", label: "plain" }] }] };
  const buildPreview = { name: "fixture pack", plan: "fixture plan", tex: 0,
    texChoices: [0, 100, 50, 75].map((value) => ({ value, label: String(value) })),
    steps: [{ key: "trim", title: "trim", text: "fixture trim" }, { key: "textures", title: "textures", kind: "textures", text: "fixture textures" }],
    before: "100 MB", after: "80 MB", mods: 0, ok: true };
  handlers.set("BuildPageResult", () => buildPreview);
  handlers.set("HomeBackground", () => ({})); handlers.set("Backgrounds", () => backgrounds);
  handlers.set("Packs", () => ({ rows: [] })); handlers.set("PackNameCheck", () => "");
  handlers.set("Detail", () => null); handlers.set("Sections", () => []);
  const pages = [
    ["home", "Home", () => home.home(), { rows: [], featured: -1 }],
    ["needs", "Needs", () => screens.needs(), { items: [] }],
    ["changes", "Changes", () => screens.changes(null), { rows: [] }],
    ["settings", "Settings", () => screens.settings(), []],
    ["env", "Environment", () => screens.environment(), { envs: [], groups: [] }],
    ["packs", "Packs", () => packs.list(), { rows: [] }],
    ["packs", "Pack", () => packs.pack("fixture"), { name: "fixture", facts: [], mods: [] }],
    ["build", "BuildPage", () => packs.build(), 1],
    ["guide", "GuideFind", () => guide.start(), []],
    ["uninstall", "UninstallPlan", () => uninstall.open(), { steps: [] }],
    ["full", "ModCopies", () => full.full("library", "fixture"), { row: null }],
    [null, "Workspace", editor, workspace],
    ["library", "Library", () => library.open(), { rows: [], columns: [] }],
    ["tasks", "Tasks", () => tasks.open(), { rows: [] }],
    ["textures", "Textures", () => textures.open(), texturePage],
    ["log", "LogPage", () => screens.gameLog("fixture", true), { path: "fixture", files: [], rows: [] }],
  ];
  for (const [screen, method, open, payload] of pages) {
    const remaining = ["Workspace", "Library", "Tasks", "Textures", "LogPage"].includes(method);
    await test(method + " shows loading immediately; late success/failure cannot navigate" + (remaining ? "; failure can retry" : ""), async () => {
      for (const fail of [false, true]) {
        screens.openScreen("other");
        const d = deferred(); handlers.set(method, () => d.promise);
        const pending = open(); await Promise.resolve();
        assert(S.screen === screen, method + " waited before navigation");
        assert(document.querySelector('#screen [role="status"]') || document.querySelector('#screen[aria-busy="true"]'), method + " has no feedback");
        screens.openScreen("other");
        if (fail) d.reject(new Error("late " + method)); else d.resolve(payload);
        await pending;
        assert(S.screen === "other" && !document.querySelector('#screen[aria-busy="true"]'), method + " overwrote destination");
      }
      if (remaining) {
        screens.openScreen("other");
        handlers.set(method, () => { throw new Error("fixture " + method + " failed"); });
        await open();
        assert(S.screen === screen && document.querySelector("#screen").textContent.includes("fixture " + method + " failed"), method + " hid the read error");
        assert(!document.querySelector('#screen[aria-busy="true"]'), method + " left loading after failure");
        const retry = document.querySelector("#screen .page-read-error button");
        assert(retry, method + " has no retry action");
        const retryReply = deferred(); handlers.set(method, () => retryReply.promise);
        retry.click(); await Promise.resolve();
        assert(document.querySelector('#screen[aria-busy="true"] .skel'), method + " retry did not show loading");
        retryReply.resolve(payload); await settle();
        assert(!screens.pageLoading() && !document.querySelector('#screen[aria-busy="true"]'), method + " retry did not finish");
      }
      handlers.set(method, () => payload);
      await open();
      assert(S.screen === screen && !document.querySelector('#screen[aria-busy="true"]'), method + " did not finish rendering");
      assert(!document.querySelector('.page-loading'), method + " left a loading placeholder");
      assert(!screens.pageLoading(), method + " left object shortcuts suspended");
    });
    if (!remaining) continue;
    await test(method + " refresh keeps content; only the latest same-page read applies", async () => {
      const sc = document.querySelector("#screen"), shown = sc.firstChild;
      const old = deferred(), next = deferred();
      handlers.set(method, () => old.promise);
      const first = open(); await Promise.resolve();
      assert(sc.firstChild === shown && !screens.pageLoading(), method + " replaced content during refresh");
      handlers.set(method, () => next.promise);
      const second = open(); await Promise.resolve();
      next.resolve(payload); await second;
      const newest = sc.firstChild;
      old.resolve(payload); await first;
      assert(sc.firstChild === newest && !sc.hasAttribute("aria-busy"), method + " installed the old same-page reply");
    });
    await test(method + " rejects results from the previous environment and plan", async () => {
      for (const field of ["env", "plan"]) {
        const old = deferred(), next = deferred();
        handlers.set(method, () => old.promise);
        const first = open(); await Promise.resolve();
        if (field === "env") S.board = { ...S.board, env: "next-" + method };
        else S.shell = { ...S.shell, plan: { id: (S.shell?.plan?.id || 0) + 1 } };
        handlers.set(method, () => next.promise);
        const second = open(); await Promise.resolve();
        assert(screens.pageLoading(), method + " kept old object controls");
        const placeholder = document.querySelector("#screen").firstChild;
        const status = document.querySelector("#status").textContent;
        old.reject(new Error("obsolete " + field)); await first;
        assert(document.querySelector("#screen").firstChild === placeholder && document.querySelector("#status").textContent === status, method + " installed an obsolete error");
        next.resolve(payload); await second;
        assert(!screens.pageLoading(), method + " did not complete the new context");
      }
    });
  }
  await test("editor and inventory hide old virtual rows during object switches, including game switching", async () => {
    const oldMod = { id: 17, name: "old editor mod" };
    handlers.set("Workspace", () => ({ ...workspace, board: { ...workspace.board, cols: [[oldMod], [], []] } }));
    await editor();
    assert(document.querySelector("#board .row"), "editor fixture has no virtual rows");
    const d = deferred(); handlers.set("Workspace", () => d.promise);
    S.board = { ...S.board, env: "other game" }; S.busy.env = "other game";
    const reading = editor();
    lists.renderRows(0);
    assert(document.querySelector("#board").inert && !document.querySelector("#board .row"), "old editor rows survived the object switch");
    d.resolve(workspace); await reading; S.busy.env = false;
    assert(!document.querySelector("#board").inert, "accepted editor stayed inert");
    handlers.set("Library", () => ({ rows: [{ id: 1, name: "old inventory mod", insts: [], copies: 0 }], columns: [{ key: "name", title: "mod", shown: true }] }));
    await library.open();
    assert(document.querySelector("#screen .lib-row"), "inventory fixture has no virtual rows");
    const libReply = deferred(); handlers.set("Library", () => libReply.promise);
    S.shell = { ...S.shell, plan: { id: 101 } }; S.busy.env = "other game";
    const inventory = library.open();
    dispatchEvent(new Event("resize"));
    assert(screens.pageLoading() && !document.querySelector("#screen .lib-row"), "old inventory rows survived the object switch");
    libReply.resolve({ rows: [], columns: [] }); await inventory; S.busy.env = false;
  });
  await test("same-page replies use only the latest request, including after close/re-entry", async () => {
    screens.openScreen("other");
    const old = deferred(), next = deferred();
    handlers.set("Environment", () => old.promise);
    const p1 = screens.environment(); await Promise.resolve();
    screens.closeScreen();
    handlers.set("Environment", () => next.promise);
    const p2 = screens.environment(); await Promise.resolve();
    next.resolve({ envs: [{ name: "new environment", game: "fixture", current: true }], groups: [] }); await p2;
    old.resolve({ envs: [{ name: "old environment", game: "fixture" }], groups: [] }); await p1;
    assert(document.querySelector('#screen').textContent.includes("new environment") && !document.querySelector('#screen').textContent.includes("old environment"), "old environment reply won");
  });
  await test("failed page load shows an error and can retry", async () => {
    screens.openScreen("other"); handlers.set("Environment", () => { throw new Error("fixture read failed"); });
    await screens.environment();
    assert(document.querySelector('#screen').textContent.includes("fixture read failed") && !document.querySelector('#screen[aria-busy]'), "failure left a spinner");
    assert(screens.pageLoading(), "failed initial read permits old object shortcuts");
    handlers.set("Environment", () => ({ envs: [], groups: [] })); await screens.environment();
    assert(!document.querySelector('#screen').textContent.includes("fixture read failed"), "retry kept old error");
  });
  await test("changing environment or plan replaces old page controls while loading", async () => {
    const board = S.board, shell = S.shell;
    S.board = { env: "first" }; S.shell = { plan: { id: 1 } };
    handlers.set("Environment", () => ({ envs: [{ name: "old context", current: true }], groups: [] }));
    await screens.environment();
    const d = deferred(); handlers.set("Environment", () => d.promise);
    S.board = { env: "second" }; S.shell = { plan: { id: 2 } };
    const pending = screens.environment(); await Promise.resolve();
    assert(screens.pageLoading() && !document.querySelector('#screen').textContent.includes("old context"), "old environment controls survived the switch");
    d.resolve({ envs: [{ name: "new context", current: true }], groups: [] }); await pending;
    assert(document.querySelector('#screen').textContent.includes("new context"), "destination did not load");
    S.board = board; S.shell = shell;
  });
  await test("build keeps inputs responsive and only the latest texture measurement wins", async () => {
    handlers.set("BuildPage", () => 1);
    handlers.set("BuildPageResult", () => buildPreview);
    await packs.build();
    const old = deferred(), fresh = deferred(); let builds = 0;
    handlers.set("BuildPage", (tex) => tex);
    handlers.set("BuildPageResult", (id) => id === 100 ? old.promise : fresh.promise);
    handlers.set("PackBuild", () => { builds++; });
    const name = document.querySelector("#pk-name"), select = document.querySelector("#pk-tex");
    name.value = "my typed pack"; await name.oninput();
    select.focus(); select.value = "100"; const first = select.onchange();
    await settle();
    assert(select.isConnected && select.value === "100" && document.activeElement === select, "choice was replaced or lost focus");
    assert(name.isConnected && name.value === "my typed pack", "name was cleared while measuring");
    assert(document.querySelectorAll(".bd-step .skel").length === 2 && document.querySelector("#pk-size .skel"), "measured facts have no shimmer");
    assert(document.querySelector(".bd-go").disabled && !document.querySelector(".page-loading"), "measurement replaced the page or enabled build");
    packs.packsKey(new KeyboardEvent("keydown", { key: "B", cancelable: true }));
    name.onkeydown(new KeyboardEvent("keydown", { key: "Enter", cancelable: true }));
    await Promise.resolve(); assert(builds === 0, "build used a stale preview");
    select.value = "50"; const second = select.onchange(); await settle();
    assert(select.value === "50" && select.isConnected, "second choice was blocked");
    fresh.resolve({ ...buildPreview, tex: 50, after: "new size" }); await second;
    assert(document.querySelector("#pk-tex").value === "50" && document.querySelector("#pk-size").textContent.includes("new size"), "latest measurement missing");
    old.resolve({ ...buildPreview, tex: 100, after: "stale size" }); await first;
    assert(document.querySelector("#pk-tex").value === "50" && !document.querySelector("#screen").textContent.includes("stale size"), "late measurement overwrote the choice");
    assert(document.querySelector("#pk-name").value === "my typed pack" && !document.querySelector(".bd-go").disabled, "fresh preview lost name or stayed disabled");
    assert(!document.querySelector("#screen .skel"), "fresh preview kept shimmer");
    const late = deferred(); handlers.set("BuildPageResult", () => late.promise);
    const nextSelect = document.querySelector("#pk-tex"); nextSelect.value = "75"; const pending = nextSelect.onchange();
    await settle(); screens.openScreen("other"); late.reject(new Error("late build failure")); await pending;
    assert(S.screen === "other" && !document.querySelector('#screen[aria-busy]'), "late failure reopened the build page");
  });
  await test("environment switch keeps navigation live and only the latest result applies", async () => {
    const old = deferred(), next = deferred(); let selected = "";
    const button = document.createElement("button"); button.className = "hm-go";
    document.querySelector("#app").append(button);
    const p1 = withEnvironmentSwitch("1.5", async (current) => { await old.promise; if (current()) selected = "1.5"; });
    assert(!document.querySelector("#app").inert && !document.querySelector("#env-progress"), "switch locked the window");
    assert(button.disabled && button.title.includes("1.5"), "launch action has no reason");
    screens.openScreen("env");
    assert(S.screen === "env", "navigation was blocked");
    const p2 = withEnvironmentSwitch("1.6", async (current) => { await next.promise; if (current()) selected = "1.6"; });
    old.resolve(); await p1;
    assert(S.busy.env === "1.6" && button.disabled && button.title.includes("1.6"), "old completion unlocked the newer switch");
    next.resolve(); await p2;
    assert(selected === "1.6" && !S.busy.env && !button.disabled, "newest result did not win");
    button.remove();
  });
  await test("environment progress stays through destination refresh", async () => {
    const reply = deferred(), refresh = deferred();
    const pending = withEnvironmentSwitch("1.5", async () => { await reply.promise; await refresh.promise; });
    reply.resolve(); await settle();
    assert(S.busy.env === "1.5" && !document.querySelector("#app").inert, "progress ended before destination refresh");
    refresh.resolve(); await pending;
    assert(!S.busy.env, "progress did not finish after refresh");
  });
  await test("environment API waits for its matching event, even before acknowledgement", async () => {
    handlers.set("UseEnv", (request, name) => {
      emit("environment", { request, text: name });
      return true;
    });
    assert((await api.UseEnv("fixture")).text === "fixture", "event was lost before acknowledgement");
    const pending = withEnvironmentSwitch("1.6", async () => {
      try { await api.Run("save"); throw new Error("write was allowed"); }
      catch (e) { assert(e.message.includes("1.6"), "write rejection has no destination"); }
    });
    await pending;
  });
  await test("pre-switch page results cannot overwrite the destination", async () => {
    screens.openScreen("other");
    const old = deferred(), switchEnd = deferred();
    handlers.set("Environment", () => old.promise);
    const read = screens.environment();
    const switching = withEnvironmentSwitch("1.6", () => switchEnd.promise);
    old.resolve({ envs: [{ name: "obsolete destination", game: "fixture" }], groups: [] });
    await read;
    assert(!document.querySelector("#screen").textContent.includes("obsolete destination"), "old page read was installed");
    switchEnd.resolve(); await switching;
    handlers.set("Environment", () => ({ envs: [], groups: [] }));
  });
  await test("failed environment switch restores focus and input", async () => {
    const app = document.querySelector("#app"), button = document.createElement("button");
    app.append(button); button.focus();
    try {
      await withEnvironmentSwitch("1.5", async () => { throw new Error("fixture switch failed"); });
      throw new Error("switch failure was swallowed");
    } catch (error) { assert(error.message === "fixture switch failed", error.message); }
    assert(!app.inert && !app.hasAttribute("aria-busy") && !S.busy.env, "failed switch left input locked");
    assert(!document.querySelector("#env-progress") && document.activeElement === button, "failed switch left progress or lost focus");
    button.remove();
  });
  await test("inventory opens before its reply and ignores late navigation", async () => {
    const d = deferred(); handlers.set("Library", () => d.promise);
    const pending = library.open();
    assert(S.screen === "library" && document.querySelector("#screen .skel"), "inventory did not show loading immediately");
    screens.openScreen("other"); d.resolve({}); await pending;
    assert(S.screen === "other", "late inventory reply reopened inventory");
  });
  await test("texture re-entry ignores old success and failure", async () => {
    const old = deferred(), fresh = deferred();
    handlers.set("Textures", () => old.promise);
    const p1 = textures.open(); await Promise.resolve();
    assert(S.screen === "textures" && document.querySelector('#screen [role="status"]')?.getAttribute("aria-label") === "reading textures…", "textures did not show loading immediately");
    screens.closeScreen(); handlers.set("Textures", () => fresh.promise);
    const p2 = textures.open(); await Promise.resolve();
    old.resolve({}); await p1;
    assert(document.querySelector("#screen .page-loading .skel"), "old reply replaced new loading state");
    screens.openScreen("other"); fresh.reject(new Error("late failure")); await p2;
    assert(S.screen === "other", "late texture failure reopened page");
  });
  await test("log opens immediately and ignores late replies", async () => {
    const d = deferred(); handlers.set("LogPage", () => d.promise);
    const pending = screens.gameLog("", true);
    assert(S.screen === "log" && document.querySelector('#screen [role="status"]')?.getAttribute("aria-label") === "reading the log…", "log did not show loading immediately");
    screens.openScreen("other"); d.resolve({ path: "old" }); await pending;
    assert(S.screen === "other", "late log reply reopened log");
  });
  await test("environment switch clears log selection and ignores the previous reply", async () => {
    S.board = { env: "old" };
    handlers.set("LogPage", () => ({ path: "old/Player.log", files: [], rows: [] }));
    await screens.gameLog(undefined, true);
    const old = deferred(); handlers.set("LogPage", () => old.promise);
    const pending = screens.gameLog(); await Promise.resolve();
    const seen = [];
    S.board = { env: "new" };
    handlers.set("LogPage", (path) => { seen.push(path); return { path: "new/Player.log", files: [], rows: [] }; });
    await screens.logEnvironmentChanged();
    old.resolve({ path: "old/Player.log", files: [], rows: [] }); await pending;
    await screens.gameLog();
    assert(seen[0] === "" && seen[1] === "new/Player.log", "old log selection survived the switch or late reply");
    S.board = { env: "empty" };
    handlers.set("LogPage", (path) => { assert(path === "", "empty environment was given the old path"); return { path: "", files: [], rows: [] }; });
    await screens.logEnvironmentChanged();
    assert(document.querySelector("#screen").textContent.includes("no Player.log found"), "empty environment displays previous log");
  });
  await test("switching environments clears details even when both share the same log path", async () => {
    let detail = "old environment detail";
    handlers.set("LogCluster", () => [detail]);
    handlers.set("LogPage", () => ({ path: "shared/Player.log", files: [], rows: [{ class: "fixture", text: "shared error", count: 1, cluster: "E0" }] }));
    S.board = { env: "shared-one" };
    await screens.logEnvironmentChanged(); await new Promise((resolve) => setTimeout(resolve, 140)); await settle();
    assert(document.querySelector(".detail-card").textContent.includes(detail), "initial detail did not load");
    detail = "new environment detail"; S.board = { env: "shared-two" };
    await screens.logEnvironmentChanged(); await new Promise((resolve) => setTimeout(resolve, 140)); await settle();
    assert(document.querySelector(".detail-card").textContent.includes(detail), "shared path reused another environment's attribution");
    handlers.set("LogCluster", () => ["fixture detail"]);
  });
  const rows = Array.from({ length: 10000 }, (_, i) => ({ class: "fixture", text: "Error " + i + " long text ".repeat(i % 4), count: i + 1, cluster: "E" + i }));
  const entries = Array.from({ length: 5000 }, (_, i) => ({ key: "m" + i, text: "Monitor " + i, details: "line\n".repeat(30), count: 1 }));
  handlers.set("LogPage", () => ({ path: "fixture", files: [], rows, monitor: { title: "Monitor", facts: "fixture", entries } }));
  await test("10,000 log rows keep DOM small, Home/End reveal selected row", async () => {
    await screens.gameLog("fixture", true); await settle();
    assert(document.querySelectorAll(".lrow").length < 60, "all log rows were mounted");
    key("End"); await settle();
    assert(document.querySelector(".lrow.cur")?.textContent.includes("Error 9999") && visibleSelection(), "End selection is not visible");
    key("Home"); await settle();
    assert(document.querySelector(".lrow.cur")?.textContent.includes("Error 0") && visibleSelection(), "Home selection is not visible");
  });
  await test("5,000 monitor entries support variable-height expansion and navigation", async () => {
    key("ArrowRight"); await settle();
    assert(document.querySelectorAll(".lrow").length < 60, "all monitor entries were mounted");
    key("Enter"); await settle();
    const first = document.querySelector(".lrow.cur");
    const next = document.querySelector('[data-virtual-index="1"]');
    assert(first.classList.contains("open") && next.getBoundingClientRect().top >= first.getBoundingClientRect().bottom - 1, "expanded rows overlap");
    key("End"); await settle();
    assert(document.querySelector(".lrow.cur")?.textContent.includes("Monitor 4999") && visibleSelection(), "last monitor entry not visible");
    key("Home"); await settle();
    assert(visibleSelection(), "first expanded monitor entry not visible");
    key("Enter"); await settle();
    assert(!document.querySelector(".lrow.cur").classList.contains("open"), "monitor entry did not collapse");
    screens.closeScreen(); await settle();
    assert(document.querySelectorAll("[data-virtual-index]").length === 0, "virtual rows survived page close");
  });
  output.dataset.result = "pass";
  const preview = document.createElement("button");
  preview.textContent = "Preview environment switching animation";
  preview.onclick = () => withEnvironmentSwitch("RimWorld 1.5", () => new Promise((resolve) => setTimeout(resolve, 5000)));
  output.append(document.createElement("br"), preview);
} catch (error) {
  output.textContent = results.join("\n") + "\nFAIL " + error.stack;
  output.dataset.result = "fail";
}
