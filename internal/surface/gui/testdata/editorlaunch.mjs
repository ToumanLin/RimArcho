import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules } from "./dom.mjs";

// The editor's launch (16 §2.5). VM 2026-11-22: with the game running, L on another list made
// that list current and asked about RAM the running game was using; Launch's ▶ is disabled
// then, so the editor refuses first and changes nothing.
async function setup({ live = {}, open = { id: 2, current: false }, severity = "", unsaved = 0, busy = "" } = {}) {
  const calls = [];
  const S = { live, shell: { plan: open }, board: { unsaved } };
  let proceed;
  const { editorLauncher } = await loadModules("./editorlaunch.js", {}, {});
  const launch = editorLauncher({
    S, T: (s) => s,
    say: (kind, text) => calls.push(["say", kind, text]),
    unsavedConfirm: (count, go) => { calls.push(["unsaved confirm", count]); proceed = go; },
    usePlan: async (id) => { calls.push(["use", id]); return true; },
    homeMemory: async () => { calls.push(["memory"]); return { token: "t", assessment: { ramSeverity: severity } }; },
    memoryConfirm: () => calls.push(["confirm"]),
    run: async (id) => { calls.push(["run", id]); },
    busy: () => busy,
  });
  return {
    launch, calls, S,
    cancel: () => { proceed = null; },
    confirm: () => proceed(),
  };
}

test("a running game is refused before the open list becomes current or RAM is asked about", async () => {
  for (const live of [{ running: true }, { blocked: "Game Config 1.6 runs on this installation" }]) {
    const h = await setup({ live, severity: "severe", unsaved: 2 });
    await h.launch();
    assert.deepEqual(h.calls, [["say", "fail", live.blocked || "the game is running: close it first"]]);
  }
});

test("with no game running, another list becomes current, then the RAM check, then the launch", async () => {
  const h = await setup();
  await h.launch();
  assert.deepEqual(h.calls, [["use", 2], ["memory"], ["say", "busy", "starting…"], ["run", "launch"]]);
  const severe = await setup({ open: { id: 1, current: true }, severity: "severe" });
  await severe.launch();
  assert.deepEqual(severe.calls, [["memory"], ["confirm"]]);
});

test("a busy window refuses launch before asking about unsaved changes", async () => {
  const h = await setup({ busy: "switching Game Config", unsaved: 2 });
  await h.launch();
  assert.deepEqual(h.calls, [["say", "busy", "switching Game Config"]]);
});

test("unsaved changes prompt before switching lists, checking RAM or writing to the game", async () => {
  const h = await setup({ unsaved: 2 });
  await h.launch();
  h.cancel();
  assert.deepEqual(h.calls, [["unsaved confirm", 2]]);

  await h.launch();
  assert.deepEqual(h.calls, [["unsaved confirm", 2], ["unsaved confirm", 2]]);
  h.S.board.unsaved = 3;
  await h.confirm();
  assert.deepEqual(h.calls, [
    ["unsaved confirm", 2], ["unsaved confirm", 2],
    ["use", 2], ["memory"], ["say", "busy", "starting…"], ["run", "launch"],
  ]);
});

test("confirming unsaved changes rechecks whether the game is running", async () => {
  const h = await setup({ unsaved: 1 });
  await h.launch();
  h.S.live.running = true;
  await h.confirm();
  assert.deepEqual(h.calls, [["unsaved confirm", 1], ["say", "fail", "the game is running: close it first"]]);
});
