// Open /dev/pagecheck in guidev. Test real modules/DOM with delayed API replies and
// synthetic large logs; the mock runtime makes this page independent of user data.
import { handlers, emit } from "./pagecheck-runtime.js";
import { S, api, T, Tf } from "/js/state.js";
import * as screens from "/js/screens.js";
import * as library from "/js/library.js";
import * as lists from "/js/lists.js";
import * as tasks from "/js/tasks.js";
import * as textures from "/js/textures.js";
import * as home from "/js/home.js";
import * as packs from "/js/packs.js";
import * as guide from "/js/guide.js";
import * as uninstall from "/js/uninstall.js";
import * as feedback from "/js/feedback.js";
import * as full from "/js/fulldetail.js";
import { setText } from "/js/api.js";
import * as ops from "/js/ops.js";
import * as pop from "/js/popups.js";
import * as nav from "/js/navigation.js";
import * as planManage from "/js/planmanage.js";
import { initExchange } from "/js/exchange.js";
import { withEnvironmentSwitch } from "/js/environment.js";

const output = document.querySelector("#results"), results = [];
const assert = (ok, message) => { if (!ok) throw new Error(message); };
const settle = async () => { for (let i = 0; i < 5; i++) await new Promise(requestAnimationFrame); };
const key = (name) => {
  const target = document.querySelector("#screen");
  target.addEventListener("keydown", screens.logsKey, { once: true });
  target.dispatchEvent(new KeyboardEvent("keydown", { key: name, bubbles: true, cancelable: true }));
};
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
  handlers.set("Backgrounds", () => backgrounds);
  await test("compact accounts use an inline keyboard login and ignore late completion", async () => {
    const accountGroups = [{ title: "Accounts", items: [
      { label: "SteamCMD", note: "mod downloads available, not logged in",
        actions: [{ label: "log in", argv: ["account", "steamcmd", "login"], input: "known-player" }] },
      { label: "LoversLab", note: "unavailable, not logged in",
        actions: [{ label: "log in", argv: ["account", "loverslab", "login"], confirm: "Site consent" }] },
    ] }];
    handlers.set("Settings", () => accountGroups); handlers.set("Tasks", () => ({ rows: [] }));
    let actions = 0, selected = ""; const pending = deferred();
    handlers.set("SteamLogin", name => { actions++; selected = name; return pending.promise; });
    await screens.settings(); await settle();
    assert(document.querySelectorAll("[data-setting-row]").length === 2 && actions === 0, "settings ran account authentication");
    assert(!document.querySelector("#screen input, #screen .apply-setting, iframe, webview"), "account field or login surface is visible on entry");
    const login = document.querySelector('[data-setting-action="account steamcmd login"]');
    login.focus(); login.click();
    let input = document.querySelector(".account-login input");
    assert(input.value === "known-player" && document.activeElement === input, "known name not prefilled/focused");
    input.dispatchEvent(new KeyboardEvent("keydown", {key:"Escape", bubbles:true}));
    assert(!document.querySelector(".account-login") && document.activeElement === login && actions === 0, "escape did not cancel login");
    login.click(); input = document.querySelector(".account-login input"); input.value = "fixture-player";
    input.dispatchEvent(new KeyboardEvent("keydown", {key:"Enter", bubbles:true})); await settle();
    assert(actions === 1 && selected === "fixture-player" && login.disabled, "enter did not start the player login once");
    screens.openScreen("other"); pending.resolve(["account action completed"]); await settle();
    assert(S.screen === "other", "late login completion reopened settings");
    handlers.set("Settings", () => []); handlers.delete("SteamLogin");
  });
  await test("inline Steam login rejects anonymous and site helper failures remain visible", async () => {
    const popup = document.createElement("div"); popup.id = "popup"; popup.hidden = true;
    document.querySelector("#app").append(popup); pop.initPopups({ renderWhere() {}, renderHeads() {} });
    const steam = { label: "SteamCMD", actions: [{ label: "log in", argv: ["account", "steamcmd", "login"], input: "" }] };
    handlers.set("Settings", () => [{ title: "Accounts", items: [steam, { label: "LoversLab", note: "unavailable, not logged in",
      actions: [{ label: "log in", argv: ["account", "loverslab", "login"], confirm: "Site consent" }] }] }]);
    let actions = 0;
    handlers.set("SteamLogin", () => { actions++; });
    handlers.set("AccountAction", () => { actions++; throw Error("controller create failed 0x80004005"); });
    await screens.settings(); await settle();
    document.querySelector('[data-setting-action="account steamcmd login"]').click();
    const input = document.querySelector(".account-login input");
    for (const value of ["", "anonymous", "ANONYMOUS", "匿名"]) {
      input.value = value; input.dispatchEvent(new KeyboardEvent("keydown", {key:"Enter", bubbles:true})); await settle();
      assert(document.activeElement === input && actions === 0 && popup.hidden, "anonymous login launched or field not focused");
      assert(document.querySelector("#status").textContent === T("enter a Steam account name first"), "missing name prompt");
    }
    input.dispatchEvent(new KeyboardEvent("keydown", {key:"Escape", bubbles:true}));
    document.querySelector('[data-setting-action="account loverslab login"]').click(); await settle();
    assert(actions === 0 && !popup.hidden, "site login skipped first-time consent");
    popup.querySelector(".btns .go").click(); await settle();
    assert(actions === 1 && popup.hidden, "consent did not call AccountAction");
    assert(document.querySelector("#status").textContent === "controller create failed 0x80004005", "helper reason disappeared");
    assert(!document.querySelector('[data-setting-action="account loverslab login"]').disabled, "failed login cannot be retried");
    handlers.set("Settings", () => []); handlers.delete("AccountAction"); handlers.delete("SteamLogin"); popup.remove();
  });
  await test("tool installation confirms source, destination and size before starting", async () => {
    const popup = document.createElement("div"); popup.id = "popup"; popup.hidden = true;
    document.querySelector("#app").append(popup); pop.initPopups({ renderWhere() {}, renderHeads() {} });
    let installs = 0;
    handlers.set("ApplySetting", () => { installs++; return ["queued"]; });
    for (const [name, bytes] of [["steamcmd", -1], ["todds", 12345], ["wezterm", 45678]]) {
      handlers.set("Settings", () => [{ title: "External tools", items: [{ label: name, actions: [
        { label: "Install tool", argv: ["deps", "install", name], confirm: "Download tool?" }] }] }]);
      handlers.set("ToolInstallOffer", () => ({ url: "https://fixture.invalid/" + name + ".zip", dir: "C:/fixture/" + name, bytes }));
      await screens.settings(); await settle();
      const before = installs;
      document.querySelector('[data-setting-action="deps install ' + name + '"]').click(); await settle();
      assert(installs === before && !popup.hidden, "download began without confirmation");
      const text = popup.textContent;
      const addresses = [...popup.querySelectorAll("[title]")].map(n => n.title).join("\n");
      assert(addresses.includes("https://fixture.invalid/" + name + ".zip") && addresses.includes("C:/fixture/" + name), "missing URL or directory");
      assert(text.includes(bytes > 0 ? Tf("size: %s bytes", bytes) : T("size unknown")), "missing size");
      popup.querySelector(".btns .go").click(); await settle();
      assert(installs === before + 1, "confirmed installation did not run");
    }
    handlers.set("Settings", () => []); handlers.delete("ApplySetting"); handlers.delete("ToolInstallOffer"); popup.remove();
  });
  handlers.set("HomeBackground", () => ({})); handlers.set("Backgrounds", () => backgrounds);
  handlers.set("Packs", () => ({ rows: [] })); handlers.set("PackNameCheck", () => "");
  handlers.set("Detail", () => null); handlers.set("Sections", () => []);
  await test("needs link rows group games and reuse repair preview with full path tooltips", async () => {
    const popup = document.createElement("div");
    popup.id = "popup"; popup.hidden = true; document.querySelector("#app").append(popup);
    pop.initPopups({ renderWhere() {}, renderHeads() {} });
    const groups = ["1.6", "1.7"].map(version => ({ game: "RimWorld " + version, path: "D:/games/" + version,
      rows: [{ name: "_Archo_env_old_" + version, path: "D:/games/" + version + "/Mods/_Archo_env_old_" + version,
        target: "D:/very long library folder/".repeat(8) + "KnownMod", reason: "target missing", action: "keep" }] }));
    const preview = groups.map(g => ({ ...g, rows: g.rows.map(r => ({ ...r, action: "will delete" })) }));
    handlers.set("Needs", () => ({ items: [{ title: "Links", text: "Links", why: "raw evidence must be hidden", section: 0,
      counted: true, status: 2, linkGroups: groups, actions: [{ kind: "repair-links", label: "Repair", confirm: "Confirm",
      refs: ["fixture-preview"], linkGroups: preview }] }] }));
    await screens.needs(); await settle();
    assert(document.querySelectorAll("#screen .link-game").length === 2, "games are not grouped");
    assert(document.querySelectorAll("#screen .link-row").length === 2, "missing link rows");
    assert(!document.querySelector("#screen").textContent.includes("raw evidence"), "raw paragraph rendered");
    const target = document.querySelector("#screen .link-row .link-path:nth-child(3)");
    assert(target.dataset.path === groups[0].rows[0].target && target.title.includes(groups[0].rows[0].target), "full target lost");
    assert(target.scrollWidth <= target.clientWidth + 1, "long path overflows row");
    await screens.runNeedAction({}, { kind: "repair-links", label: "Repair", confirm: "Confirm", linkGroups: preview }); await settle();
    assert(document.querySelectorAll("#popup .link-game").length === 2, "preview lost game groups");
    assert([...document.querySelectorAll("#popup .link-tag")].every(n => n.textContent === "will delete"), "preview action tag is wrong");
    assert(document.querySelector("#popup .link-row .link-path:nth-child(3)").dataset.path === groups[0].rows[0].target, "preview lost full path");
    pop.close(); popup.remove();
    handlers.set("Needs", () => ({ items: [] }));
  });
  await test("subscriptions: one list and a detail; scope bar; sources line; update preview; days; one-mod and home landings; add page", async () => {
    const sourceKeys = ["workshop", "steamcmd", "git", "page", "local"];
    const change = (id, tier, text) => ({ id, ref: "#" + id, mod: id.toUpperCase(), tier, tierText: ["", "affects launch", "worth a look", "content only"][tier], text, details: [text + " in detail"], before: 1, after: 2, workshopId: "123", actions: [{ kind: "lock", label: "lock this version", key: "p" }] });
    const data = { sources: sourceKeys.map(key => ({ key, name: key, count: key === "local" ? 3 : 1, checked: "10:00", result: "cached", account: ["steamcmd", "page"].includes(key) ? "offline account" : "" })),
      summary: { line: "checked 4 sources, the last at 10:00, 1 check failed", failures: 1 },
      pending: [{ id: "git-id", ref: "git-ref", name: "Git fixture", kind: "git", source: "Git", installed: "abc", available: "def", usedIn: ["Colony"], inPlan: true, tracking: "the default branch", opens: "compare" },
        { id: "page-id", ref: "page-ref", name: "Page fixture", kind: "page", source: "download page", installed: "1", available: "2", usedIn: ["Other"] }],
      recent: [],
      history: { plan: "Colony", since: "since last played", rows: [change("vef", 1, "code changed"), change("tex", 3, "textures +3 MB")],
        arrivals: { text: "1 new mod: NEW", rows: [{ ...change("new", 3, "found in the Mods folder"), present: true }] }, older: [{ day: "Oct 21", rows: [change("old", 2, "Defs changed")] }] } };
    data.sources[0].failures = [{ label: "connection timed out", items: ["Fixture: connection timed out"] }];
    let reads = [], checks = [], previews = [], updates = [], went = [];
    handlers.set("SubscriptionsPage", (library, of) => { reads.push([library, of]); return data; });
    handlers.set("CheckSubscriptionSource", kind => { checks.push(kind); return "queued"; });
    // updates preview through their own call (subscriptions.js updateRows), then run as a library task
    handlers.set("SubscriptionUpdatePreview", (refs) => { previews.push(["subscription-update", refs]); return { title: "preview", apply: true, lines: ["abc → def"] }; });
    handlers.set("LibraryDo", (kind, refs) => { updates.push([kind, refs]); return "queued"; });
    handlers.set("SubscriptionFilePage", () => "");
    handlers.set("ChangeFiles", () => ["~ Defs/a.xml"]);
    const popup = document.createElement("div"); popup.id = "popup"; popup.hidden = true; document.querySelector("#app").append(popup);
    const go = ui.go; ui.go = id => { went.push(id); return id === "changes" ? screens.changes(null) : undefined; };
    const rows = () => [...document.querySelectorAll(".subs-row")];
    const sel = () => document.querySelector(".subs-row.sel");
    const detail = () => document.querySelector(".subs-detail").textContent;
    try {
      await screens.changes(null); await settle();
      assert(checks.length === 0, "sources checked on entry");
      assert(document.querySelector(".subs-scope .seg") && document.querySelector(".subs-scope").textContent.includes("since last played"), "scope bar");
      assert(rows().filter(r => r.dataset.key.startsWith("p:")).length === 1, "the list scope shows only the current list's updates");
      assert(document.querySelector(".subs-hidden")?.textContent.includes("1"), "hidden updates are not counted");
      assert(sel()?.dataset.key === "p:git-ref" && detail().includes("Git fixture") && detail().includes("the default branch"), "first update not selected and detailed");
      assert(document.querySelector(".subs-foot").textContent.includes("checked 4 sources") && !document.querySelector(".subs-failed"), "sources line");
      for (const n of [1, 2]) { screens.areaKey(new KeyboardEvent("keydown", { key: String(n), cancelable: true })); assert(document.activeElement.closest("[data-area]")?.dataset.area === String(n), "area key " + n); }
      screens.areaKey(new KeyboardEvent("keydown", { key: "1", cancelable: true }));
      key("ArrowDown"); await settle();
      assert(sel()?.dataset.key.startsWith("r:now:vef") && detail().includes("VEF") && detail().includes("affects launch"), "arrow did not move to the change row");
      assert(document.activeElement === sel(), "focus did not follow the selection");
      key("i"); await settle();
      key("ArrowUp"); key("u"); await settle();
      assert(previews.length === 1 && updates.length === 0 && S.popup?.kind === "confirm", "update skipped preview/confirmation");
      popup.querySelector(".btns .go").click(); await settle();
      assert(updates[0]?.[1]?.[0] === "git-ref", "wrong update target");
      key("a"); await settle();
      assert(reads.at(-1)[0] === true && rows().filter(r => r.dataset.key.startsWith("p:")).length === 2, "all mods did not show every update");
      document.querySelector('[data-key="p:page-ref"]').click(); await settle();
      assert(document.querySelector(".subs-detail .subscribe-update"), "download page update has no file choice");
      assert(document.querySelector(".subs-detail .subs-dhead .btn.primary").disabled, "download page update can run without files");
      document.querySelector(".subs-srcsum").click(); await settle();
      assert(document.querySelectorAll(".subs-detail .subs-src").length === 5 && !document.querySelector(".subs-list .subs-sources"), "checks did not open in the detail");
      assert(detail().includes("Fixture: connection timed out"), "failure names and reasons missing");
      [...document.querySelectorAll(".subs-src")][2].querySelector(".btn.small").click(); await settle();
      assert(checks.join() === "git", "check did not queue the git source");
      key("Escape"); await settle();
      assert(sel()?.dataset.key === "sources", "esc changed the selected source summary");
      key("K"); await settle();
      assert(checks.slice(1).join() === "workshop,steamcmd,git,page", "check for updates did not queue every source");
      document.querySelector('[data-key="d:Oct 21"]').click(); await settle();
      assert(rows().some(r => r.classList.contains("indent")), "the day did not open");
      emit("tasks", { ended: { kind: "source-check" } }); await settle();
      assert(sel()?.dataset.key === "d:Oct 21", "a task refresh lost the selection");
      key("+");
      assert(went.at(-1) === "subscribe", "+ did not go to Add a subscription");
      await screens.changes({ id: "git-id", name: "Git fixture" }); await settle();
      assert(reads.at(-1)[1] === "git-id" && document.querySelector(".subs-scope .lib-cond") && screens.changesFiltered(), "one-mod view lost");
      assert(document.activeElement.closest(".subs-list"), "one-mod view did not land in the list");
      key("Escape"); await settle();
      assert(reads.at(-1)[1] === "" && !screens.changesFiltered(), "esc did not show every mod");
      handlers.set("Home", () => ({ rows: [{ kind: "plan", envName: "fixture", env: "fixture", plan: 1, page: "plan:1", name: "Colony", current: true }], featured: 0, changes: { title: "current list", text: "since last played", count: 1 } }));
      await home.home(); document.querySelector(".hm-actline").click(); await settle();
      assert(reads.at(-1).join() === "false," && document.activeElement.closest(".subs-list"), "home lost the current-list landing");
      await screens.openAdd(); await settle();
      const box = document.querySelector(".subscribe-omni input");
      assert(box && box === document.activeElement, "Add a subscription did not focus the paste box");
      assert(document.querySelectorAll(".subscribe-tile").length === 3, "source cards missing");
    } finally { ui.go = go; }
    pop.close(); popup.remove();
    handlers.set("Home", () => ({ rows: [], featured: -1 }));
    handlers.set("SubscriptionsPage", () => ({ sources: [], pending: [], history: { rows: [] } }));
  });
  handlers.set("FeedbackCompose", () => ({ body: "fixture", url: "https://github.com/ToumanLin/RimArcho/issues/new", full: true }));
  const pages = [
    ["home", "Home", () => home.home(), { rows: [], featured: -1 }],
    ["needs", "Needs", () => screens.needs(), { items: [] }],
    ["changes", "SubscriptionsPage", () => screens.changes(null), { sources: [], pending: [], history: { rows: [] } }],
    ["settings", "Settings", () => screens.settings(), []],
    ["env", "Environment", () => screens.environment(), { envs: [], groups: [] }],
    ["packs", "Packs", () => packs.list(), { rows: [] }],
    ["packs", "Pack", () => packs.pack("fixture"), { name: "fixture", facts: [], mods: [] }],
    ["build", "BuildPage", () => packs.build(), 1],
    ["guide", "GuideFind", () => guide.start(), []],
    ["uninstall", "UninstallPlan", () => uninstall.open(), { steps: [] }],
    ["feedback", "FeedbackAttach", () => feedback.open(), { basic: [], parts: [], sections: [] }],
    ["full", "ModCopies", () => full.full("library", "fixture"), { row: null }],
    [null, "Workspace", editor, workspace],
    ["library", "Library", () => library.open(), { rows: [], columns: [] }],
    ["tasks", "Tasks", () => tasks.open(), { rows: [] }],
    ["textures", "TexturesInfo", () => textures.open(), texturePage],
    ["log", "LogPage", () => screens.gameLog("fixture", true), { path: "fixture", files: [], rows: [] }],
  ];
  for (const [screen, method, open, payload] of pages) {
    const remaining = ["Workspace", "Library", "Tasks", "TexturesInfo", "LogPage"].includes(method);
    await test(method + " shows loading immediately; late success/failure cannot navigate" + (remaining ? "; failure can retry" : ""), async () => {
      for (const fail of [false, true]) {
        screens.openScreen("other");
        const d = deferred(); handlers.set(method, () => d.promise);
        const pending = open(); await Promise.resolve();
        assert(S.screen === screen, method + " waited before navigation");
        assert(document.querySelector('#screen [role="progressbar"]') || document.querySelector('#screen[aria-busy="true"]'), method + " has no feedback");
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
  // The launch page's numbers are focus keys; applying a row must keep the column (16 §2.3).
  await test("home 1/2/3 focus columns, arrows apply within their scope, Enter launches", async () => {
    const original = { ...ui }, live = S.live;
    const rows = [
      { kind: "plan", envName: "game-a", plan: 1, name: "list-a1", page: "plan:1", current: true },
      { kind: "plan", envName: "game-a", plan: 2, name: "list-a2", page: "plan:2" },
      { kind: "pack", envName: "game-a", name: "pack-a" },
      { kind: "plan", envName: "game-b", plan: 3, name: "list-b1", page: "plan:3" },
      { kind: "plan", envName: "game-b", plan: 4, name: "list-b2", page: "plan:4" },
    ].map((r) => ({ ...r, env: r.envName + " · 1.7", mods: 10, level: "ok", state: "ready" }));
    const fixture = { rows, featured: 0 }, applied = [], opened = [], launched = [];
    const press = (key, extra = {}) => home.homeKey(new KeyboardEvent("keydown", { key, cancelable: true, ...extra }));
    const focused = (n) => document.activeElement?.closest(".hm-step")?.dataset.step === String(n);
    handlers.set("Home", () => fixture);
    handlers.set("HomeApply", async (request, kind, env, plan, name) => {
      fixture.featured = rows.findIndex((r) => r.kind === kind && r.envName === env && (kind === "plan" ? r.plan === plan : r.name === name));
      applied.push(rows[fixture.featured].name);
      await new Promise(requestAnimationFrame);
      emit("environment", { request, text: "applied fixture" });
      return true;
    });
    Object.assign(ui, { reload() {}, refreshEnvironment: () => home.home({ keep: true }), go: (page) => opened.push(page), run: (op) => launched.push(op) });
    S.live = null;
    try {
      await home.home();
      for (const n of [1, 2, 3]) {
        press(String(n));
        const step = document.querySelector(".hm-s" + n), number = step.querySelector("h3 b");
        assert(focused(n), n + " did not focus its column");
        assert(number.textContent === "0" + n, "number changed");
        const probe = document.createElement("span"); probe.style.color = "var(--accent)"; step.append(probe);
        assert(getComputedStyle(number).color === getComputedStyle(probe).color, "focused number is not accent");
        probe.remove();
      }
      assert(document.activeElement.matches(".hm-go") && !applied.length && !launched.length, "focus key applied or launched a row");
      for (const modifier of ["ctrlKey", "altKey", "metaKey", "shiftKey"]) {
        press("1", { [modifier]: true }); assert(focused(3), modifier + "+1 focused a column");
      }
      const move = async (key, name, column) => {
        press(key); await settle();
        assert(applied.at(-1) === name, key + " applied " + applied.at(-1) + ", expected " + name);
        assert(focused(column), key + " lost column focus after redraw");
      };
      press("1");
      await move("ArrowDown", "list-b1", 1);
      await move("ArrowUp", "list-a1", 1);
      press("2");
      await move("ArrowDown", "list-a2", 2);
      await move("ArrowDown", "pack-a", 2);
      await move("ArrowUp", "list-a2", 2);
      await move("ArrowRight", "list-b1", 2);
      await move("ArrowLeft", "list-a2", 2);
      press("Enter"); assert(opened.at(-1) === "plan:2" && !launched.length, "Enter outside 03 did not edit");
      press("3"); await move("ArrowUp", "list-a1", 3);
      assert(document.activeElement.matches(".hm-go"), "03 did not return focus to launch after switching");
      press("Enter"); await settle();
      assert(launched.length === 1 && launched[0] === "launch", "Enter on 03 did not launch exactly once");
      S.live = { running: true }; home.liveChanged(); press("3"); press("Enter");
      assert(focused(3) && launched.length === 1 && opened.length === 1, "disabled 03 launched or opened the list");
      const hints = home.hintList();
      assert(hints.map((h) => h.key).join(" ") === "1–3 ↑↓ ←→ enter L shift+1–8 ?", "startup hint keys differ from 16 §2.3");
    } finally {
      Object.assign(ui, original); delete ui.run; S.live = live;
      handlers.set("Home", () => ({ rows: [], featured: -1 })); handlers.delete("HomeApply");
      screens.openScreen("other");
    }
  });
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
  await test("build counts explicitly, keeps inputs responsive, and rejects stale texture facts", async () => {
    handlers.set("BuildPage", () => 1);
    handlers.set("BuildPageResult", () => ({ ...buildPreview, toolOk: true }));
    await packs.build();
    const old = deferred(), fresh = deferred(); let builds = 0, counts = 0;
    handlers.set("BuildPage", (tex) => { counts++; return tex; });
    handlers.set("BuildPageResult", (id) => id === 100 ? old.promise : fresh.promise);
    handlers.set("PackBuild", () => { builds++; });
    document.querySelector("#pk-name").value = "my typed pack"; await document.querySelector("#pk-name").oninput();
    let select = document.querySelector("#pk-tex");
    select.focus(); select.value = "100"; select.onchange(); await settle();
    select = document.querySelector("#pk-tex");
    assert(select.value === "100" && document.activeElement === select && counts === 0, "choice lost focus or counted without a request");
    const countButton = () => [...document.querySelectorAll('.bd-step[data-key="textures"] button')].find(b => b.textContent === T("count the textures"));
    countButton().click(); await settle();
    assert(select.isConnected && select.value === "100" && document.activeElement === select, "count replaced the choice or lost focus");
    assert(document.querySelector("#pk-name").value === "my typed pack", "name was cleared while counting");
    assert(document.querySelectorAll(".bd-step .skel").length === 2 && document.querySelector("#pk-size .skel"), "counted facts have no shimmer");
    assert(document.querySelector(".bd-go").disabled && !document.querySelector(".page-loading"), "count replaced the page or enabled build");
    packs.packsKey(new KeyboardEvent("keydown", { key: "B", cancelable: true }));
    await Promise.resolve(); assert(builds === 0, "build used a pending count");
    select.value = "50"; select.onchange(); await settle();
    assert(document.querySelector("#pk-tex").value === "50", "choice was blocked during counting");
    old.resolve({ ...buildPreview, toolOk: true, tex: 100, after: "stale size" }); await settle();
    assert(document.querySelector("#pk-tex").value === "50" && !document.querySelector("#pk-size").textContent.includes("stale size"), "old count supplied facts for another choice");
    countButton().click(); await settle();
    fresh.resolve({ ...buildPreview, toolOk: true, tex: 50, after: "new size" }); await settle();
    assert(document.querySelector("#pk-size").textContent.includes("new size"), "explicit fresh count missing");
    assert(document.querySelector("#pk-name").value === "my typed pack" && !document.querySelector(".bd-go").disabled, "fresh count lost name or stayed disabled");
    assert(!document.querySelector("#screen .skel"), "fresh count kept shimmer");
    const late = deferred(); handlers.set("BuildPageResult", () => late.promise);
    select = document.querySelector("#pk-tex"); select.value = "75"; select.onchange();
    countButton().click(); await settle();
    screens.openScreen("other"); late.reject(new Error("late build failure")); await settle();
    assert(S.screen === "other" && !document.querySelector('#screen[aria-busy]'), "late failure reopened the build page");
  });
  await test("environment switch keeps navigation live and only the latest result applies", async () => {
    const old = deferred(), next = deferred(); let selected = "";
    const button = document.createElement("button"); button.className = "hm-go"; button.dataset.envWrite = "launch";
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
  // Attribute guards must survive translated text, icons and redraws (16 §2.3, 16 §3.1).
  await test("Chinese environment actions disable by code, navigation stays live, and state restores", async () => {
    const start = S.start, lang = document.documentElement.lang, live = S.live;
    const host = document.createElement("section"), menu = document.createElement("div"), popup = document.createElement("div");
    menu.id = "menu"; menu.hidden = true; popup.id = "popup"; popup.hidden = true;
    host.append(menu, popup); document.querySelector("#app").append(host);
    pop.initPopups({ renderWhere() {}, renderHeads() {} });
    const text = { "write to the game": "写入游戏", "launch the game": "启动游戏", undo: "撤销", redo: "重做", "switching to %s": "正在切换到 %s" };
    setText(text); document.documentElement.lang = "zh"; S.live = null;
    const table = [
      { code: "enable", label: "启用", key: "space", group: 1 },
      { code: "tags", label: "标签", key: "g", group: 2 },
      { code: "folder", label: "打开文件夹", key: "ctrl+o", group: 3 },
      { code: "pin", label: "锁定这个版本", group: 4 },
      { code: "delete", label: "移到回收站", key: "Del", group: 5 },
    ];
    S.start = { ...S.start, ops: table }; ops.setTable(table);
    const writes = ["save", "launch", "undo", "redo"].map((code, i) => {
      const b = screens.button("① ▶ " + T(["write to the game", "launch the game", "undo", "redo"][i]), "", () => {}, "", code);
      host.append(b); return b;
    });
    writes[0].title = "w"; writes[1].title = ""; writes[3].disabled = true; writes[3].title = "没有可重做的操作";
    const navigation = screens.button(T("write to the game"), "", () => {}); navigation.title = "导航"; host.append(navigation);
    const before = writes.map(b => ({ disabled: b.disabled, title: b.getAttribute("title") }));
    const finish = deferred(), pending = withEnvironmentSwitch("游戏乙", () => finish.promise);
    const allDisabled = buttons => buttons.forEach(b => assert(b.disabled && b.title === "正在切换到 游戏乙", "unguarded Chinese action " + b.dataset.envWrite));
    try {
      allDisabled(writes);
      assert(!navigation.disabled && navigation.title === "导航", "matching navigation text was disabled");
      const newButton = screens.rescanButton(); host.append(newButton); writes.push(newButton);
      await settle(); allDisabled([newButton]);
      ops.openMenu(0, 0, table.map(o => o.code), () => { throw new Error("guarded menu key ran"); });
      await settle();
      allDisabled([...menu.querySelectorAll("[data-env-write]")]);
      assert(menu.querySelectorAll("[data-env-write]").length === 4 && !menu.querySelector("button:not([data-env-write])").disabled, "op groups were classified incorrectly");
      menu.dispatchEvent(new KeyboardEvent("keydown", { key: "g", bubbles: true, cancelable: true }));
      let confirms = 0;
      pop.confirm("删除", [], "删除", () => confirms++);
      await settle();
      allDisabled([...popup.querySelectorAll("[data-env-write]")]);
      pop.popupKey(new KeyboardEvent("keydown", { key: "Enter", cancelable: true }));
      assert(!confirms && S.popup, "confirmation keyboard bypassed disabled");
      pop.close(); pop.hideMenu();
      handlers.set("Home", () => ({ rows: [{ kind: "plan", envName: "游戏乙", env: "游戏乙", plan: 1, page: "plan:1", name: "方案", current: true }], featured: 0 }));
      await home.home(); await settle();
      const launch = document.querySelector(".hm-go"); allDisabled([launch]);
      home.homeKey(new KeyboardEvent("keydown", { key: "L", cancelable: true }));
      for (const [method, args] of [["Run", ["save"]], ["Run", ["launch"]], ["Run", ["sort"]], ["Undo", []], ["Redo", []], ["Scan", []], ["Analyze", []], ["RunOp", ["delete", []]], ["LibraryDo", ["enable", []]]]) {
        let called = false; handlers.set(method, () => { called = true; });
        try {
          let reason = "";
          try { await api[method](...args); } catch (error) { reason = error.message; }
          assert(!called && reason === "正在切换到 游戏乙", method + " shortcut escaped API guard");
        } finally { handlers.delete(method); }
      }
      writes[0].disabled = false; writes[0].title = "late validation";
      await settle(); allDisabled([writes[0]]);
      finish.resolve(); await pending;
      before.forEach((old, i) => assert(writes[i].disabled === old.disabled && writes[i].getAttribute("title") === old.title, "disabled/title not restored for " + writes[i].dataset.envWrite));
      assert(!newButton.disabled && !launch.disabled && !launch.hasAttribute("title"), "redrawn actions stayed disabled");
      assert(!navigation.disabled && navigation.title === "导航", "navigation state changed");
      try { await withEnvironmentSwitch("游戏丙", async () => { throw new Error("fixture failure"); }); } catch (error) { assert(error.message === "fixture failure", error.message); }
      before.forEach((old, i) => assert(writes[i].disabled === old.disabled && writes[i].getAttribute("title") === old.title, "failure lost original state"));
    } finally {
      finish.resolve(); await pending; pop.close(); host.remove();
      S.start = start; S.live = live; ops.setTable(start?.ops || table); setText(start?.text); document.documentElement.lang = lang;
      handlers.set("Home", () => ({ rows: [], featured: -1 })); screens.openScreen("other");
    }
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
    handlers.set("TexturesInfo", () => old.promise);
    const p1 = textures.open(); await Promise.resolve();
    assert(S.screen === "textures" && document.querySelector('#screen [role="progressbar"]')?.getAttribute("aria-label") === "loading…", "textures did not show loading immediately");
    screens.closeScreen(); handlers.set("TexturesInfo", () => fresh.promise);
    const p2 = textures.open(); await Promise.resolve();
    old.resolve({}); await p1;
    assert(document.querySelector("#screen .page-loading .skel"), "old reply replaced new loading state");
    screens.openScreen("other"); fresh.reject(new Error("late failure")); await p2;
    assert(S.screen === "other", "late texture failure reopened page");
  });
  await test("log opens immediately and ignores late replies", async () => {
    const d = deferred(); handlers.set("LogPage", () => d.promise);
    const pending = screens.gameLog("", true);
    assert(S.screen === "log" && document.querySelector('#screen [role="progressbar"]')?.getAttribute("aria-label") === "reading the log…", "log did not show loading immediately");
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
  // G26: the production router, page completion hooks and window input listeners.
  Object.assign(ui, { go: nav.go, pageEntering: nav.pageEntering, pageEntered: nav.pageEntered,
    render() {}, renderHeads() {}, detailFollow() {}, selectedList: () => 1,
    reload: async (_, read) => { read?.commit(() => { S.shell.plan = { id: 1 }; }); return true; } });
  nav.initNavigation(ui); nav.wireHistory(); pop.initPopups(ui);
  planManage.initPlanManage(ui); initExchange(ui);
  for (const id of ["menu", "popup"]) {
    const node = document.createElement("div"); node.id = id; node.hidden = true;
    document.querySelector("#app").append(node);
  }
  S.board = { env: "navigation", envs: [{ name: "navigation" }] };
  S.shell = { plan: { id: 1 }, plans: [{ id: 1 }] };
  S.start = { ...S.start, text: {} }; setText({});
  handlers.set("Home", () => ({ rows: [], featured: -1 }));
  handlers.set("Needs", () => ({ items: [] }));
  handlers.set("Settings", () => []);
  handlers.set("Environment", () => ({ envs: [], groups: [] }));
  handlers.set("Tasks", () => ({ rows: [] }));
  handlers.set("ExchangeInfo", () => ({}));
  handlers.set("Packs", () => ({ rows: [] }));
  handlers.set("Pack", (name) => ({ name, facts: [], mods: [] }));
  handlers.set("GuideFind", () => []);
  const input = (name) => {
    const e = new KeyboardEvent("keydown", { key: name, altKey: name.startsWith("Arrow"), bubbles: true, cancelable: true });
    (document.activeElement || document).dispatchEvent(e);
    assert(e.defaultPrevented, name + " was not prevented");
  };
  const side = (type, button) => {
    const e = new MouseEvent(type, { button, bubbles: true, cancelable: true });
    document.dispatchEvent(e);
    assert(e.defaultPrevented, type + " side button was not prevented");
  };
  let writes = 0;
  const writeMethods = ["Run", "RunOp", "HistoryGo", "ApplyImport", "PackBuild", "Uninstall", "Undo", "Redo"];
  for (const method of writeMethods) handlers.set(method, () => { writes++; throw new Error("navigation wrote data: " + method); });
  await test("page history A→B→C, two back, forward, duplicate and branch truncation", async () => {
    await nav.go("home"); await nav.go("needs"); await nav.go("settings");
    await nav.travel(-1); assert(S.screen === "needs", "first back missed B");
    await nav.travel(-1); assert(S.screen === "home", "second back missed A");
    await nav.travel(1); assert(S.screen === "needs", "forward missed B");
    await nav.go("needs"); await nav.travel(1);
    assert(S.screen === "settings", "same-page navigation cleared forward history");
    await nav.travel(-1); await nav.go("env"); await nav.travel(1);
    assert(S.screen === "env", "new navigation did not clear forward");
    await nav.travel(-1); assert(S.screen === "needs", "duplicate visit was pushed");
  });
  await test("same-page home refresh during back preserves forward history for keys and mouse", async () => {
    const payload = { rows: [], featured: -1 };
    for (const mouse of [false, true]) for (const order of ["original", "refresh", "forward"]) {
      handlers.set("Home", () => payload);
      await nav.go("home"); await nav.go("needs");
      const original = deferred(), refresh = deferred();
      let reads = 0, refreshed;
      handlers.set("Home", () => ++reads === 1 ? original.promise : refresh.promise);
      // HomeBackground can announce newly prepared art while Home is still reading.
      handlers.set("HomeBackground", () => {
        if (reads === 1) queueMicrotask(() => { refreshed = home.home({ keep: true }); });
        return { url: "/fixture-background.jpg", dim: 30 };
      });
      if (mouse) { side("mousedown", 3); side("mouseup", 3); side("auxclick", 3); }
      else input("ArrowLeft");
      for (let i = 0; reads < 2 && i < 30; i++) await Promise.resolve();
      assert(reads === 2 && S.screen === "home", "back did not receive a same-page refresh");
      if (order === "original") {
        original.resolve(payload);
        for (let i = 0; i < 30; i++) await Promise.resolve();
        assert(S.screen === "home", "superseded traversal skipped the pending refresh");
        refresh.resolve(payload); await refreshed; await settle();
      } else if (order === "refresh") {
        refresh.resolve(payload); await refreshed;
        original.resolve(payload); await settle();
      }
      if (mouse) { side("mousedown", 4); side("mouseup", 4); side("auxclick", 4); }
      else input("ArrowRight");
      assert(S.screen === "needs", "refresh removed the forward destination: " + order);
      if (order === "forward") {
        original.resolve(payload); refresh.resolve(payload); await refreshed;
      }
      await settle();
      assert(S.screen === "needs", "late home refresh took over forward navigation");
      handlers.set("Home", () => payload); handlers.set("HomeBackground", () => ({}));
      await nav.travel(-1); assert(S.screen === "home", "refresh added a new home visit");
      await nav.travel(1); assert(S.screen === "needs", "forward history did not survive a second traversal");
    }
  });
  await test("back waits for the latest same-page refresh instead of cancelling its replay", async () => {
    const payload = { rows: [], featured: -1 };
    await nav.go("home"); await nav.go("needs");
    const original = deferred(), latest = deferred();
    let reads = 0;
    handlers.set("Home", () => ++reads === 1 ? original.promise : latest.promise);
    let done = false;
    const back = nav.travel(-1).then(() => { done = true; });
    const refresh = home.home({ keep: true });
    original.resolve(payload);
    await settle();
    assert(reads === 2 && !done && S.screen === "home", "same-page refresh cancelled the pending replay");
    latest.resolve(payload); await refresh; await back;
    await nav.travel(1); assert(S.screen === "needs", "refresh truncated forward history");
    handlers.set("Home", () => payload);
  });
  await test("back then immediate forward preserves a still-loading destination for keys and mouse", async () => {
    for (const mouse of [false, true]) {
      await nav.go("home");
      const pending = deferred(); handlers.set("Needs", () => pending.promise);
      const entering = nav.go("needs");
      if (mouse) { side("mousedown", 3); side("mouseup", 3); side("auxclick", 3); }
      else input("ArrowLeft");
      assert(S.screen === "home", "back waited for the pending destination");
      if (mouse) { side("mousedown", 4); side("mouseup", 4); side("auxclick", 4); }
      else input("ArrowRight");
      assert(S.screen === "needs", "pending destination was lost to forward navigation");
      pending.resolve({ items: [] }); await entering; await settle();
      assert(S.screen === "needs", "late read took over the pending destination");
      handlers.set("Needs", () => ({ items: [] }));
      await nav.travel(-1); assert(S.screen === "home", "pending destination added a duplicate visit");
      await nav.travel(1); assert(S.screen === "needs", "pending destination was not retained");
    }
  });
  await test("side mouseup and auxclick take one step, alt arrows work in inputs, URL stays local", async () => {
    await nav.go("home"); await nav.go("needs"); await nav.go("settings");
    const url = location.href;
    side("mousedown", 3); assert(S.screen === "settings", "mousedown navigated");
    side("mouseup", 3); await settle(); assert(S.screen === "needs", "mouseup did not go back");
    side("auxclick", 3); await settle(); assert(S.screen === "needs", "auxclick repeated mouseup");
    side("mousedown", 4); side("mouseup", 4); side("auxclick", 4); await settle();
    assert(S.screen === "settings", "forward side button failed");
    side("auxclick", 3); await settle(); assert(S.screen === "needs", "auxclick-only back failed");
    const field = document.createElement("input"); document.querySelector("#app").append(field); field.focus();
    input("ArrowLeft"); await settle(); assert(S.screen === "home", "alt+left in input failed");
    input("ArrowRight"); await settle(); assert(S.screen === "needs", "alt+right failed");
    input("BrowserBack"); await settle(); assert(S.screen === "home", "BrowserBack failed");
    input("BrowserForward"); await settle(); assert(S.screen === "needs", "BrowserForward failed");
    field.remove();
    assert(location.href === url && location.origin === new URL(url).origin, "history changed the dev/Wails address");
  });
  await test("back closes a menu or popup, then full details use Backspace return", async () => {
    await nav.go("home"); await nav.go("needs");
    const menu = document.querySelector("#menu");
    pop.menu(0, 0, [{ label: "fixture", run() { writes++; } }]);
    await nav.travel(-1); assert(menu.hidden && S.screen === "needs", "back passed through menu");
    pop.open("help", "fixture", () => {});
    await nav.travel(-1); assert(!S.popup && S.screen === "needs", "back passed through popup");
    handlers.set("Library", () => ({ rows: [], columns: [] }));
    await nav.go("library");
    await full.full("library", "fixture");
    await nav.travel(1); assert(S.screen === "full", "forward at the end left full details");
    await nav.travel(-1); assert(S.screen === "library", "full detail did not return to library");
    await nav.travel(-1); assert(S.screen === "needs", "full detail was added to page history");
  });
  await test("pack subpages remain distinct; deleted packs and plans are skipped", async () => {
    await nav.go("home"); await nav.go("pack:first"); await nav.go("pack:second");
    await nav.travel(-1); assert(S.packFocus === "first", "pack history lost its name");
    await nav.travel(1); assert(S.packFocus === "second", "pack forward lost its name");
    handlers.set("Pack", () => { throw new Error("pack removed"); });
    await nav.travel(-1); assert(S.screen === "home", "unavailable packs were not skipped");
    handlers.set("Pack", (name) => ({ name, facts: [], mods: [] }));
    await nav.go("plan:1"); await nav.go("settings");
    S.shell.plans = [];
    await nav.travel(-1); assert(S.screen === "home", "deleted plan was not skipped");
    S.shell.plans = [{ id: 1 }];
  });
  await test("direct import and task entry points record completion, expired guide sessions cannot reopen", async () => {
    await nav.go("home"); await nav.go("plan-import");
    const field = document.querySelector("#screen textarea"); field.value = "unsaved input";
    await tasks.toggle(); assert(S.screen === "tasks", "tasks did not open");
    await nav.travel(-1); assert(S.screen === "plan-import" && !S.popup, "import was not restored as normal navigation");
    await nav.travel(-1); assert(S.screen === "home", "import return missing");
    await nav.go("guide");
    const oldSession = guide.historySession();
    // Reopening the guide also invalidates the old entry without executing any data action.
    await nav.go("guide");
    assert(guide.historySession() !== oldSession, "guide session was reused");
    await nav.go("settings"); await nav.travel(-1);
    assert(S.screen === "guide", "active guide was not resumable");
    await nav.travel(-1); assert(S.screen === "home", "expired guide entry was not skipped");
  });
  await test("finishing a guide invalidates its page history entry", async () => {
    await nav.go("home");
    handlers.set("GuideFind", () => [{ name: "fixture game", game: "fixture", version: "1.6" }]);
    handlers.set("GuideAdd", (request) => { emit("environment", { request, names: ["fixture game"] }); return true; });
    handlers.set("GuideScan", () => true);
    await nav.go("guide");
    document.querySelector(".guide-foot .primary").click(); await settle();
    guide.guideEvent({ done: true, ok: true });
    [...document.querySelectorAll(".guide-foot button")].find(b => b.textContent === "next").click();
    [...document.querySelectorAll(".guide-foot button")].find(b => b.textContent === "skip").click(); await settle();
    assert(S.screen === "home" && guide.historySession() === null, "guide did not finish");
    await nav.travel(-1); assert(S.screen === "home", "completed guide reopened from history");
    handlers.set("GuideFind", () => []);
  });
  await test("export keeps its list id and return context; unavailable visits and other games are skipped", async () => {
    const seen = [];
    handlers.set("PlanExport", (id) => { seen.push(id); return { format: "json", formats: [{ id: "json", file: "list.json" }], text: "fixture" }; });
    await nav.go("home"); await planManage.exportOf(1, "editor"); await nav.go("settings");
    await nav.travel(-1);
    assert(S.screen === "plan-export" && S.planExport.id === 1 && S.planExport.from === "editor" && seen.length === 2, "export context was lost");
    await nav.go("needs");
    handlers.set("PlanExport", () => { throw new Error("gone"); });
    await nav.travel(-1); assert(S.screen === "home", "failed export did not skip to home");
    await nav.go("pack:gone"); await nav.go("settings");
    handlers.set("Pack", () => { throw new Error("gone"); });
    // All older visits belong to a different game and cannot be revisited.
    S.board.env = "different game";
    await nav.travel(-1); assert(S.screen === "settings", "history switched games");
    S.board.env = "navigation";
    handlers.set("Pack", (name) => ({ name, facts: [], mods: [] }));
  });
  await test("late success and failure cannot take over a newer history navigation", async () => {
    await nav.go("home"); await nav.go("needs"); await nav.go("settings");
    for (const fail of [false, true]) {
      const delayed = deferred(); handlers.set("Needs", () => delayed.promise);
      const old = nav.travel(-1);
      assert(S.screen === "needs", "history did not enter before reading");
      const newer = nav.travel(-1); await newer;
      assert(S.screen === "home", "second back waited for old load");
      if (fail) delayed.reject(new Error("stale failure")); else delayed.resolve({ items: [] });
      await old; assert(S.screen === "home", "stale load reopened B");
      handlers.set("Needs", () => ({ items: [] }));
      await nav.travel(1); await nav.travel(1);
      assert(S.screen === "settings", "stale completion corrupted forward history");
    }
    const ordinary = deferred(); handlers.set("Needs", () => ordinary.promise);
    await nav.go("home"); const pending = nav.go("needs");
    await nav.travel(-1); assert(S.screen === "home", "back skipped the source of an ordinary pending page");
    ordinary.resolve({ items: [] }); await pending;
    await nav.travel(1); assert(S.screen === "needs", "back discarded the explicitly requested pending destination");
    handlers.set("Needs", () => ({ items: [] }));
    await nav.go("needs"); await nav.go("settings");
    const delayed = deferred(); handlers.set("Needs", () => delayed.promise);
    const old = nav.travel(-1); await nav.go("env"); delayed.resolve({ items: [] }); await old;
    await nav.travel(1); assert(S.screen === "env", "new route did not supersede pending back");
    handlers.set("Needs", () => ({ items: [] }));
  });
  await test("history cap stops after 49 back steps and navigation performs no data actions", async () => {
    for (let i = 0; i < 55; i++) await nav.go(i % 2 ? "needs" : "home");
    let moved = 0;
    for (let i = 0; i < 55; i++) { const before = S.screen; await nav.travel(-1); if (before !== S.screen) moved++; }
    assert(moved === 49, "history did not cap at 50 entries: " + moved);
    assert(writes === 0, "page navigation performed data actions");
  });
  await test("completed uninstall locks both history directions without actions", async () => {
    uninstall.event({ done: true, page: { ran: true, failed: false } });
    const before = S.screen;
    await nav.travel(-1); await nav.travel(1); input("BrowserBack"); await settle();
    assert(S.screen === before && writes === 0, "history escaped uninstall lock");
  });
  for (const method of writeMethods) handlers.delete(method);
  await (await import("./feedbackcheck.js")).checkFeedback({ handlers, test, assert, deferred });
  output.dataset.result = "pass";
  const preview = document.createElement("button");
  preview.textContent = "Preview environment switching animation";
  preview.onclick = () => withEnvironmentSwitch("RimWorld 1.5", () => new Promise((resolve) => setTimeout(resolve, 5000)));
  output.append(document.createElement("br"), preview);
} catch (error) {
  output.textContent = results.join("\n") + "\nFAIL " + error.stack;
  output.dataset.result = "fail";
}
