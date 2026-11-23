import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush, event } from "./dom.mjs";

// A synthetic plan after a sort (16 §6.1): each kind has one Mod; applying a kind removes it
// from what the plan still needs, undo puts the last one back.
function plan(kinds = ["use-installed", "replace", "enable", "subscribe"], unfixable = []) {
  const applied = [], calls = [];
  const item = {
    "use-installed": { change: "copy", name: "One" },
    replace: { change: "replace", name: "New", old: "Old" },
    enable: { change: "add", name: "Dep", for: ["User"] },
    subscribe: { change: "get", name: "Fork", workshopId: "7002" },
  };
  const api = {
    SortSteps: async (past) => {
      calls.push(["SortSteps", [...past]]);
      const steps = kinds.filter((k) => !applied.includes(k)).map((k) => ({ kind: k, items: [{ ...item[k] }], workshopIds: k === "subscribe" ? ["7002"] : undefined }));
      const current = steps.findIndex((s) => !past.includes(s.kind));
      if (current >= 0 && steps[current].kind !== "subscribe") { steps[current].preview = true; steps[current].items[0].at = 3; }
      return { steps, current, unfixable };
    },
    ApplySortStep: async (kind) => { calls.push(["ApplySortStep", kind]); applied.push(kind); return { text: "did " + kind, undo: true }; },
    Undo: async () => { calls.push(["Undo"]); applied.pop(); return "undid"; },
    Subscribe: async (o) => { calls.push(["Subscribe", o]); applied.push("subscribe"); return "queued"; },
  };
  return { api, calls, applied };
}

async function setup(p) {
  const S = { popup: null }, said = [];
  const popup = new Element("div");
  class Node extends Element { click() { if (!this.disabled) this.onclick?.(event("")); } }
  const el = (tag, cls, text) => new Node(tag, cls, text);
  const mod = await loadModules("./sortsteps.js", {
    "./state.js": { S, el, api: p.api, T: (s) => s, Tf: (f, ...a) => a.reduce((s, v) => s.replace(/%[ds]/, v), f), errText: (e) => e.message, say: (...a) => said.push(a), keyCaps: (k) => el("kbd", "", k) },
    "./icons.js": { icon: {} },
    "./popups.js": {
      open: (kind, title, build) => { popup.replaceChildren(); S.popup = { kind, title }; build(popup, S.popup); },
      close: () => { S.popup = null; popup.replaceChildren(); },
    },
    "./credit.js": { getChoices: () => ["from Steam", "from SteamCMD"] },
  });
  mod.initSortSteps({ edited: async (call) => { try { return await call(); } catch { return null; } } });
  return { mod, S, popup, said };
}

const chips = (h) => h.popup.querySelectorAll(".ss-chip").map((c) => c.dataset.kind + ":" + c.className.split(" ")[1]);
const button = (h, cls) => h.popup.querySelector(".btns " + cls);
const settle = async () => { for (let i = 0; i < 5; i++) await flush(); };

test("nothing to fix opens nothing", async () => {
  const h = await setup(plan([]));
  await h.mod.offerAfterSort({ text: "sorted", undo: true });
  assert.equal(h.S.popup, null);
});

test("only what cannot be fixed still opens, with close as the only way on", async () => {
  const h = await setup(plan([], [{ change: "none", name: "tester.lost", why: "no known source" }]));
  await h.mod.offerAfterSort({ text: "sorted", undo: true });
  assert.equal(h.S.popup.kind, "sort-steps");
  assert.match(h.popup.querySelector(".ss-unfix").textContent, /1 cannot be fixed here/);
  assert.equal(button(h, ".go").textContent, "closeenter");
});

test("one step at a time: each shows its changes and applies alone", async () => {
  const p = plan(), h = await setup(p);
  await h.mod.offerAfterSort({ text: "sorted: 2 moved", undo: true });
  assert.deepEqual(chips(h), ["sort:done", "use-installed:now", "replace:later", "enable:later", "subscribe:later"]);
  const card = h.popup.querySelector(".ss-card");
  assert.equal(card.dataset.kind, "use-installed");
  assert.match(card.textContent, /use the installed copies for entries with no folder/);
  assert.equal(card.querySelector(".ss-at").textContent, "#3");
  assert.equal(button(h, ".go").textContent, "use copies1enter");
  assert.deepEqual(p.calls, [["SortSteps", []]]);

  button(h, ".go").onclick(); await settle();
  assert.deepEqual(p.calls.slice(1), [["ApplySortStep", "use-installed"], ["SortSteps", ["use-installed"]]]);
  assert.deepEqual(chips(h), ["sort:done", "use-installed:done", "replace:now", "enable:later", "subscribe:later"]);
  const card2 = h.popup.querySelector(".ss-card");
  assert.equal(card2.querySelector(".ss-old").textContent, "Old");
  assert.equal(card2.querySelector(".ss-mod").textContent, "New");
});

test("skip moves on without a change; a skipped chip takes the step up again", async () => {
  const p = plan(), h = await setup(p);
  await h.mod.offerAfterSort({ text: "sorted", undo: true });
  button(h, ".ss-skip").onclick(); await settle();
  assert.deepEqual(chips(h), ["sort:done", "use-installed:skipped", "replace:now", "enable:later", "subscribe:later"]);
  assert.ok(!p.calls.some((c) => c[0] === "ApplySortStep"));
  assert.deepEqual(p.calls.at(-1), ["SortSteps", ["use-installed"]]);
  h.popup.querySelector(".ss-chip.skipped").onclick(); await settle();
  assert.deepEqual(chips(h), ["sort:done", "use-installed:now", "replace:later", "enable:later", "subscribe:later"]);
});

test("undo takes back the latest step only, which becomes current again", async () => {
  const p = plan(), h = await setup(p);
  await h.mod.offerAfterSort({ text: "sorted", undo: true });
  // the sort is the latest step at first
  assert.equal(h.popup.querySelectorAll(".ss-undo").length, 1);
  assert.equal(h.popup.querySelector(".ss-undo").closest(".ss-chip").dataset.kind, "sort");
  button(h, ".go").onclick(); await settle();
  button(h, ".go").onclick(); await settle();
  assert.deepEqual(chips(h), ["sort:done", "use-installed:done", "replace:done", "enable:now", "subscribe:later"]);
  assert.equal(h.popup.querySelector(".ss-undo").closest(".ss-chip").dataset.kind, "replace");
  h.popup.querySelector(".ss-undo").onclick(); await settle();
  assert.deepEqual(p.calls.filter((c) => c[0] === "Undo").length, 1);
  assert.deepEqual(chips(h), ["sort:done", "use-installed:done", "replace:now", "enable:later", "subscribe:later"]);
  assert.equal(h.popup.querySelector(".ss-undo").closest(".ss-chip").dataset.kind, "use-installed");
});

test("undoing the sort marks it undone and keeps the steps", async () => {
  const p = plan(), h = await setup(p);
  await h.mod.offerAfterSort({ text: "sorted", undo: true });
  h.popup.querySelector(".ss-undo").onclick(); await settle();
  assert.deepEqual(chips(h)[0], "sort:undone");
  assert.equal(h.popup.querySelectorAll(".ss-undo").length, 0);
  assert.equal(h.popup.querySelector(".ss-card").dataset.kind, "use-installed");
});

test("the Workshop step offers Steam and SteamCMD and subscribes its IDs; then all is done", async () => {
  const p = plan(["subscribe"]), h = await setup(p);
  await h.mod.offerAfterSort({ text: "sorted", undo: true });
  const go = button(h, ".go");
  assert.equal(go.textContent, "get with Steam1enter");
  assert.equal(go.title, "from Steam");
  go.onclick(); await settle();
  assert.equal(JSON.stringify(p.calls.find((c) => c[0] === "Subscribe")[1]), JSON.stringify({ kind: "workshop", url: "7002", steamcmd: false, batch: true }));
  assert.match(h.popup.querySelector(".ss-card").textContent, /all steps done/);
  assert.equal(button(h, ".go").textContent, "closeenter");
});

test("keys: enter applies, s skips", async () => {
  const p = plan(), h = await setup(p);
  await h.mod.offerAfterSort({ text: "sorted", undo: true });
  h.S.popup.keys(event("s")); await settle();
  h.S.popup.keys(event("Enter")); await settle();
  assert.deepEqual(p.calls.filter((c) => c[0] === "ApplySortStep"), [["ApplySortStep", "replace"]]);
});

test("stripOf orders chips by step and marks only the latest undoable one", () => {
  return setup(plan()).then(({ mod }) => {
    const flow = {
      sort: { state: "done", text: "sorted" }, skipped: new Set(["replace"]), busy: false,
      done: [{ kind: "sort", undo: true }, { kind: "enable", n: 2, text: "enabled 2 mods", undo: true }],
      data: { steps: [{ kind: "replace", items: [{}] }, { kind: "subscribe", items: [{}, {}] }], current: 1 },
    };
    const s = mod.stripOf(flow);
    assert.equal(JSON.stringify(s.map((c) => [c.kind, c.state, c.n, c.undo])), JSON.stringify([
      ["sort", "done", 0, false], ["replace", "skipped", 1, false], ["enable", "done", 2, true], ["subscribe", "now", 2, false]]));
    assert.equal(JSON.stringify(mod.pastOf(flow)), JSON.stringify(["enable", "replace"]));
  });
});
