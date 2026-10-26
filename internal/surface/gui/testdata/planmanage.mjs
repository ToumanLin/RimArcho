import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, deferred, flush } from "./dom.mjs";

async function setup(overrides = {}) {
  const screen = new Element("div"), requests = [], polls = [], timers = [];
  const S = { screen: "plans", screenSeq: 1, envSeq: 0, board: { env: "game-a" }, shell: { plan: { id: 1 } } };
  const el = (tag, cls, text) => Object.assign(new Element(tag, cls, text), { addEventListener() {}, select() {} });
  const $ = selector => selector === "#screen" ? screen : screen.querySelector(selector);
  const detail = id => ({ id, name: "List " + id, page: "plan:" + id, versions: [], follows: [], facts: "", stale: false });
  const api = {
    PlanManage: async id => ({ sel: detail(id || 1), rows: [1, 2].map(id => ({ ...detail(id), meta: "" })) }),
    PlanLoadCost: id => { const d = deferred(); requests.push({ ...d, id }); return d.promise; },
    PlanLoadCostMeasure: request => { const d = deferred(); polls.push({ ...d, request }); return d.promise; },
    PlanSources: async id => ({ copy: "copy list", count: id * 10, versions: [{ from: "v" + id, label: "History " + id }], saves: [] }),
    CheckPlanName: async (base, name) => ({ free: base || "New list", problem: name === "taken" ? "name taken" : "" }),
    ...overrides,
  };
  const button = (label, key, onclick, cls) => Object.assign(el("button", cls, label), { onclick });
  const deps = {
    "./state.js": { S, $, el, api, T: s => s, Tf: s => s, errText: e => e.message, say() {} },
    "./screens.js": { openScreen: () => screen, page: cls => el("div", cls), button, act: button, area() {}, seg() {},
      beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }) },
    "./icons.js": { icon: {} },
    "./popups.js": {},
    "./plans.js": {},
    "./separator.js": { wireSeparator() {} },
    "./motion.js": { places: () => null, slideFrom() {} },
    "./exchange.js": { showImportSummary() {}, showImportContents() {} },
  };
  const pm = await loadModules("./planmanage.js", deps, { document: { activeElement: null }, setTimeout: f => timers.push(f), requestAnimationFrame: f => f() });
  const routes = [];
  const ui = { renderHints() {}, reload: async () => {}, go: async id => { routes.push(id); if (id === "plans") { S.screen = "plans"; await pm.open(); } else { S.screen = id; S.screenSeq++; } } };
  pm.initPlanManage(ui);
  return { pm, screen, requests, polls, timers, S, routes, api };
}

const result = (request, label) => ({
  request, loading: false, summary: label, detail: "baseline and mods", mods: "2 mods",
  parts: [{ category: "textures", files: "2 files", disk: "1 MB", ram: "0–1 MB", vram: "1–4 MB" }],
  top: [{ name: "Largest mod", ram: "1 MB", vram: "4 MB" }], notes: ["estimate notes"],
});

for (const rejected of [false, true]) test("changing lists ignores a late " + (rejected ? "error" : "result"), async () => {
  const h = await setup();
  await h.pm.open(1);
  const shimmer = h.screen.querySelector(".skel");
  assert.ok(shimmer, "renders before measurement completes");
  h.requests[0].resolve({ request: 1, loading: true, progress: "read 0 / 2 mods" });
  await flush();
  assert.ok(h.screen.textContent.includes("read 0 / 2 mods"));
  h.timers.shift()(); await flush();
  h.polls[0].resolve({ request: 1, loading: true, progress: "read 1 / 2 mods" });
  await flush();
  assert.equal(h.screen.querySelector(".skel"), shimmer, "progress preserves shimmer");
  h.timers.shift()(); await flush();
  await h.pm.open(2);
  h.requests[1].resolve(result(2, "new range")); await flush();
  if (rejected) h.polls[1].reject(new Error("old error"));
  else h.polls[1].resolve(result(1, "old range"));
  await flush();
  assert.ok(h.screen.textContent.includes("new range"));
  assert.ok(!h.screen.textContent.includes("old range"));
  assert.ok(!h.screen.textContent.includes("old error"));
  assert.equal(h.screen.querySelector(".skel"), null);
  const detail = h.screen.querySelector("details");
  assert.ok(detail);
  assert.equal(detail.open, false, "parts are collapsed initially");
  assert.ok(detail.textContent.includes("textures"));
  assert.ok(detail.textContent.includes("Largest mod"));
  assert.ok(detail.textContent.includes("estimate notes"));
});

test("leaving the page ignores measurements and stops polling", async () => {
  const h = await setup();
  await h.pm.open(1);
  h.S.screen = "home"; h.S.screenSeq++;
  h.requests[0].resolve(result(1, "abandoned range")); await flush();
  assert.ok(!h.screen.textContent.includes("abandoned range"));
  assert.equal(h.timers.length, 0);
});

test("cancelling a new-list form resumes the selected list measurement", async () => {
  const h = await setup();
  await h.pm.open(1);
  await h.pm.newList();
  h.requests[0].resolve(result(1, "hidden range")); await flush();
  h.pm.key({ key: "Escape", preventDefault() {} }); await flush();
  assert.equal(h.requests.length, 2, "returning to the detail restarts its measurement");
  h.requests[1].resolve(result(2, "resumed range")); await flush();
  assert.ok(h.screen.textContent.includes("resumed range"));
  assert.equal(h.screen.querySelector(".skel"), null);
});

const label = b => b.text || b.children[0]?.textContent || "";
const buttonNamed = (screen, text) => screen.querySelectorAll("button").find(b => label(b) === text);
const inputName = async (h, value) => { const input = h.screen.querySelector(".pm-name"); input.value = value; input.oninput(); await flush(); };
const mode = async (h, value) => { h.screen.querySelectorAll(".pm-modes button").find(b => b.dataset.mode === value).onclick(); await flush(); };
const escape = async h => { h.pm.key({ key: "Escape", preventDefault() {} }); await flush(); };

test("the form starts empty and creates an empty list", async () => {
  const copies = [], h = await setup({ CreatePlan: async (...a) => { copies.push(a); return { page: "plan:3", text: "created" }; } });
  await h.pm.open(1); await h.pm.newList();
  assert.ok(h.screen.querySelector(".pm-modes .on").dataset.mode === "", "empty is the default start");
  assert.equal(h.screen.querySelector(".pm-name").value, "New list");
  assert.equal(buttonNamed(h.screen, "back to management"), undefined);
  assert.equal(buttonNamed(h.screen, "create only"), undefined);
  assert.equal(buttonNamed(h.screen, "continue later"), undefined);
  await buttonNamed(h.screen, "create and edit").onclick(); await flush();
  assert.deepEqual(copies, [[1, "New list", ""]]);
  assert.equal(h.routes.at(-1), "plan:3");
});

test("copy chooses another list and its own version without changing the current list", async () => {
  const copies = [], h = await setup({ CreatePlan: async (...a) => { copies.push(a); return { page: "plan:3", text: "created" }; } });
  await h.pm.open(1); await h.pm.newList(); await inputName(h, "My copy"); await mode(h, "current");
  const source = h.screen.querySelector(".pm-source"); source.value = "2"; source.onchange(); await flush();
  assert.equal(h.S.shell.plan.id, 1, "source browsing does not switch current list");
  assert.ok(h.screen.querySelector(".pm-version").textContent.includes("History 2"));
  assert.ok(!h.screen.querySelector(".pm-version").textContent.includes("History 1"));
  const version = h.screen.querySelector(".pm-version"); version.value = "v2"; version.onchange();
  await buttonNamed(h.screen, "create and edit").onclick();
  assert.deepEqual(copies, [[2, "My copy", "v2"]]);
  assert.equal(h.routes.at(-1), "plan:3");
});

test("a draft with its given name is dropped on leaving; a named one is kept", async () => {
  const h = await setup(); await h.pm.open(1);
  await h.pm.newList(); await escape(h);
  assert.equal(h.screen.querySelector(".pm-draft"), null, "esc drops an unnamed draft");
  await h.pm.newList(); h.pm.abandon();
  h.S.screen = "plans"; await h.pm.open();
  assert.equal(h.screen.querySelector(".pm-draft"), null, "leaving the page drops an unnamed draft");
  assert.equal(h.screen.querySelector(".pm-new"), null);
  await h.pm.newList(); await inputName(h, "New list");
  await escape(h);
  assert.equal(h.screen.querySelector(".pm-draft"), null, "typing the given name back is no name of one's own");
  await h.pm.newList(); await inputName(h, "Mine");
  h.screen.querySelectorAll(".pm-row").find(r => r.dataset.id === 2).onclick(); await flush();
  assert.ok(h.screen.querySelector(".pm-draft").textContent.includes("Mine"), "picking another list keeps a named draft");
});

test("draft name, source and version survive browsing, leaving, and switching games", async () => {
  const h = await setup(); await h.pm.open(1); await h.pm.newList(); await inputName(h, "Draft A"); await mode(h, "current");
  const version = h.screen.querySelector(".pm-version"); version.value = "v1"; version.onchange();
  await escape(h);
  assert.equal(h.screen.querySelector(".pm-new"), null);
  assert.ok(h.screen.querySelector(".pm-draft").textContent.includes("Draft A"));
  await h.pm.newList();
  assert.equal(h.screen.querySelector(".pm-name").value, "Draft A");
  assert.equal(h.screen.querySelector(".pm-version").value, "v1");
  h.pm.abandon(); h.S.screen = "home"; h.S.screenSeq++;
  h.S.board.env = "game-b"; h.S.envSeq++; h.S.screen = "plans"; await h.pm.open(1);
  assert.equal(h.screen.querySelector(".pm-draft"), null, "other game never sees draft A");
  await h.pm.newList(); await inputName(h, "Draft B");
  h.S.board.env = "game-a"; h.S.envSeq++; await h.pm.open();
  assert.equal(h.screen.querySelector(".pm-name").value, "Draft A");
  assert.equal(h.screen.querySelector(".pm-version").value, "v1");
});

test("late source replies never replace a newer selection", async () => {
  const pending = [], h = await setup({ PlanSources: id => { const d = deferred(); pending.push({ id, ...d }); return d.promise; } });
  await h.pm.open(1); const initial = h.pm.newList(); await flush();
  await inputName(h, "Keep input"); await mode(h, "current");
  const source = h.screen.querySelector(".pm-source"); source.value = "2"; source.onchange();
  pending[1].resolve({ count: 20, versions: [{ from: "v2", label: "Latest source" }], saves: [] }); await flush();
  pending[0].resolve({ count: 10, versions: [{ from: "v1", label: "Stale source" }], saves: [] }); await initial;
  assert.equal(h.screen.querySelector(".pm-name").value, "Keep input");
  assert.equal(h.screen.querySelector(".pm-source").value, "2");
  assert.ok(h.screen.querySelector(".pm-version").textContent.includes("Latest source"));
  assert.ok(!h.screen.textContent.includes("Stale source"));
});

test("import: the main button previews first, then creates; changed content previews again", async () => {
  const previews = [], creates = [], h = await setup({
    ImportPreviewFor: async (...args) => { previews.push(args); return { request: 42, summary: "2 mods", installed: [], missing: [] }; },
    ImportAsPlanPreview: async (...args) => { creates.push(args); return { page: "plan:3", text: "created" }; },
  });
  await h.pm.open(1); await h.pm.newList(); await inputName(h, "My own name"); await mode(h, "import");
  assert.equal(h.screen.querySelector(".pm-import-path"), null, "no typed path");
  assert.equal(buttonNamed(h.screen, "preview").disabled, true, "nothing to preview yet");
  let text = h.screen.querySelector("textarea"); text.value = "mod.a\nmod.b"; text.oninput();
  assert.equal(buttonNamed(h.screen, "create and edit"), undefined);
  await buttonNamed(h.screen, "preview").onclick(); await flush();
  assert.equal(h.screen.querySelector(".pm-name").value, "My own name");
  assert.equal(buttonNamed(h.screen, "create and edit").disabled, false, "the same button now creates");
  await escape(h); await h.pm.newList();
  assert.equal(h.screen.querySelector("textarea").value, "mod.a\nmod.b");
  text = h.screen.querySelector("textarea"); text.value = "mod.c"; text.oninput();
  assert.equal(buttonNamed(h.screen, "create and edit"), undefined, "changed content is previewed again");
  await buttonNamed(h.screen, "preview").onclick(); await flush();
  await buttonNamed(h.screen, "create and edit").onclick(); await flush();
  assert.deepEqual(creates, [["My own name", 42, true]]);
  assert.deepEqual(previews.at(-1), [1, "text", "mod.c"]);
  assert.equal(h.routes.at(-1), "plan:3");
});

test("import names an unnamed list after the imported one", async () => {
  const h = await setup({
    ImportPreviewFor: async () => ({ request: 1, name: "Shared colony", summary: "", installed: [], missing: [] }),
    CheckPlanName: async (base, name) => ({ free: base || "New list", problem: "" }),
  });
  await h.pm.open(1); await h.pm.newList(); await mode(h, "import");
  const text = h.screen.querySelector("textarea"); text.value = "RC-1"; text.oninput();
  await buttonNamed(h.screen, "preview").onclick(); await flush();
  assert.equal(h.screen.querySelector(".pm-name").value, "Shared colony");
  await escape(h);
  assert.equal(h.screen.querySelector(".pm-draft"), null, "the suggested name is not the player's");
});

test("leaving during import preview ignores the late result and permits retry", async () => {
  const d = deferred(), h = await setup({ ImportPreviewFor: () => d.promise });
  await h.pm.open(1); await h.pm.newList(); await inputName(h, "Keep me"); await mode(h, "import");
  const text = h.screen.querySelector("textarea"); text.value = "mod.a"; text.oninput();
  const read = buttonNamed(h.screen, "preview").onclick();
  h.pm.closeForm(); await flush(); d.resolve({ request: 7 }); await read;
  await h.pm.newList();
  assert.equal(h.screen.querySelector(".pm-name").value, "Keep me");
  assert.equal(buttonNamed(h.screen, "preview").disabled, false, "the preview can run again");
  assert.ok(!h.screen.textContent.includes("reading the list…"));
});

test("name validation and a double submission preserve the draft and create at most once", async () => {
  const d = deferred(), copies = [], h = await setup({ CreatePlan: (...args) => { copies.push(args); return d.promise; } });
  await h.pm.open(1); await h.pm.newList(); await inputName(h, "taken");
  assert.equal(buttonNamed(h.screen, "create and edit").disabled, true);
  await buttonNamed(h.screen, "create and edit").onclick(); assert.equal(copies.length, 0);
  await inputName(h, "Good name");
  const first = buttonNamed(h.screen, "create and edit").onclick();
  const second = buttonNamed(h.screen, "create and edit").onclick(); await flush();
  assert.equal(copies.length, 1); d.resolve({ page: "plan:3", text: "created" }); await Promise.all([first, second]);
});

test("the detail shows rename, duplicate, delete and edit; double-clicking the title renames", async () => {
  const renames = [], h = await setup({ RenamePlan: async (...a) => { renames.push(a); return { page: "plan:1", text: "renamed" }; } });
  await h.pm.open(1);
  const names = h.screen.querySelectorAll(".pm-detail .pm-head .r button").map(label);
  assert.deepEqual(names, ["rename", "duplicate", "delete", "edit"]);
  h.screen.querySelector(".pm-title").ondblclick(); await flush();
  const input = h.screen.querySelector(".pm-title-input");
  assert.equal(input.value, "List 1");
  input.value = "Colony"; input.oninput();
  input.onkeydown({ key: "Enter", preventDefault() {}, stopPropagation() {} }); await flush();
  assert.deepEqual(renames, [[1, "Colony"]]);
  assert.equal(h.screen.querySelector(".pm-title-input"), null);
});

test("esc in the title puts the name back", async () => {
  const renames = [], h = await setup({ RenamePlan: async (...a) => { renames.push(a); return { page: "plan:1", text: "" }; } });
  await h.pm.open(1); h.pm.startRename(1); await flush();
  const input = h.screen.querySelector(".pm-title-input"); input.value = "Other";
  input.onkeydown({ key: "Escape", preventDefault() {}, stopPropagation() {} }); await flush();
  assert.deepEqual(renames, []);
  assert.equal(h.screen.querySelector(".pm-title").textContent, "List 1");
});

test("duplicate copies the list as it is now and offers its name for editing", async () => {
  const copies = [], h = await setup({
    CreatePlan: async (...a) => { copies.push(a); return { page: "plan:3", text: "created" }; },
    CheckPlanName: async (base) => ({ free: base, problem: "" }),
  });
  await h.pm.open(1);
  await buttonNamed(h.screen, "duplicate").onclick(); await flush();
  assert.deepEqual(copies, [[1, "%s copy", "current"]]);
  assert.ok(h.screen.querySelector(".pm-title-input"), "the copy's name is up for editing");
});
