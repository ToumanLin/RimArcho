// Open /dev/pagecheck in guidev. Test real modules/DOM with delayed API replies and
// synthetic large logs; the mock runtime makes this page independent of user data.
import { handlers } from "./pagecheck-runtime.js";
import { S } from "/js/state.js";
import * as screens from "/js/screens.js";
import * as library from "/js/library.js";
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
const ui = { renderWhere() {}, renderHints() {}, reload() {}, cmd() {}, go() {} };
screens.initScreens(ui);
library.initLibrary(ui);
home.initHome(ui); packs.initPacks(ui); guide.initGuide(ui); full.initFull(ui);
handlers.set("Setting", () => "");
handlers.set("LogCluster", () => ["fixture detail"]);
handlers.set("LogPattern", () => null);

async function test(name, run) { await run(); results.push("PASS " + name); output.textContent = results.join("\n"); }

try {
  const backgrounds = { setting: { key: "plain", stars: [], files: [] }, groups: [{ name: "game", items: [{ key: "plain", label: "plain" }] }] };
  const buildPreview = { name: "fixture pack", plan: "fixture plan", steps: [], mods: 0, ok: true };
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
    ["build", "BuildPage", () => packs.build(), buildPreview],
    ["guide", "GuideFind", () => guide.start(), []],
    ["uninstall", "UninstallPlan", () => uninstall.open(), { steps: [] }],
    ["full", "ModCopies", () => full.full("library", "fixture"), { row: null }],
  ];
  for (const [screen, method, open, payload] of pages) {
    await test(method + " shows loading immediately; late success/failure cannot navigate", async () => {
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
      handlers.set(method, () => payload);
      await open();
      assert(S.screen === screen && !document.querySelector('#screen[aria-busy="true"]'), method + " did not finish rendering");
      assert(!document.querySelector('.page-loading'), method + " left a loading placeholder");
      assert(!screens.pageLoading(), method + " left object shortcuts suspended");
    });
  }
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
  await test("build keyboard cannot run a stale preview while remeasuring", async () => {
    handlers.set("BuildPage", () => buildPreview); await packs.build();
    const d = deferred(); let builds = 0;
    handlers.set("BuildPage", () => d.promise); handlers.set("PackBuild", () => { builds++; });
    packs.packsKey(new KeyboardEvent("keydown", { key: "T", cancelable: true })); await Promise.resolve();
    packs.packsKey(new KeyboardEvent("keydown", { key: "B", cancelable: true })); await Promise.resolve();
    assert(builds === 0, "build used a stale preview");
    screens.openScreen("other"); d.resolve(buildPreview); await settle();
    assert(S.screen === "other", "option response reopened the build page");
  });
  await test("environment switch shows progress immediately and rejects repeat clicks", async () => {
    const d = deferred(); let calls = 0;
    const pending = withEnvironmentSwitch("1.5", () => { calls++; return d.promise; });
    const app = document.querySelector("#app");
    assert(document.querySelector('#env-progress [role="progressbar"]') && app.inert, "no immediate progress or input lock");
    assert(document.querySelector("#env-progress").textContent.includes("1.5"), "progress does not name the destination");
    assert(await withEnvironmentSwitch("1.6", () => { calls++; }) === false, "repeat click started another switch");
    await settle();
    assert(calls === 1 && document.querySelector("#env-progress"), "progress disappeared while switch was pending");
    d.resolve(); await pending;
    assert(!S.busy.env && !app.inert && !document.querySelector("#env-progress"), "switch did not release progress and input");
  });
  await test("environment progress stays through destination refresh", async () => {
    const reply = deferred(), refresh = deferred();
    const pending = withEnvironmentSwitch("1.5", async () => { await reply.promise; await refresh.promise; });
    await settle(); reply.resolve(); await settle();
    assert(document.querySelector("#env-progress") && S.busy.env, "progress ended before destination refresh");
    refresh.resolve(); await pending;
    assert(!document.querySelector("#env-progress"), "progress did not finish after refresh");
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
    assert(S.screen === "library" && document.querySelector("#screen").textContent.includes("loading"), "inventory did not show loading immediately");
    screens.openScreen("other"); d.resolve({}); await pending;
    assert(S.screen === "other", "late inventory reply reopened inventory");
  });
  await test("texture re-entry ignores old success and failure", async () => {
    const old = deferred(), fresh = deferred();
    handlers.set("Textures", () => old.promise);
    const p1 = textures.open(); await Promise.resolve();
    assert(S.screen === "textures" && document.querySelector("#screen").textContent.includes("reading textures"), "textures did not show loading immediately");
    screens.closeScreen(); handlers.set("Textures", () => fresh.promise);
    const p2 = textures.open(); await Promise.resolve();
    old.resolve({}); await p1;
    assert(document.querySelector("#screen").textContent.includes("reading textures"), "old reply replaced new loading state");
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
