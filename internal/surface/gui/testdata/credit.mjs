import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules, deferred, flush } from "./dom.mjs";

// The Steam credit question (11 §1): asked only when the backend has a due batch and no
// other popup is open; concurrent checks share one; the answer goes back with the batch ids.
async function setup(prompt) {
  const calls = [], chosen = [], messages = [];
  const S = { popup: null };
  let gate = null;
  const api = {
    CreditPrompt: async () => { calls.push(["prompt"]); if (gate) await gate.promise; return prompt; },
    CreditAnswer: async (ids, replace) => { calls.push(["answer", ids, replace]); return { batchIDs: ids, state: replace ? "replacing" : "dismissed", taskID: replace ? "t1" : "" }; },
  };
  const deps = {
    "./state.js": { S, api, T: (s) => s, Tf: (s, ...a) => { let i = 0; return s.replace(/%[sd]/g, () => a[i++]); }, errText: (e) => e.message, say: (...a) => messages.push(a) },
    "./popups.js": { choose: (title, labels, pick) => { S.popup = { title }; chosen.push({ title, labels, pick }); } },
  };
  const mod = await loadModules("./credit.js", deps);
  return { mod, S, calls, chosen, messages, hold: () => { gate = deferred(); return gate; } };
}

const due = { batchIDs: [3, 5], modCount: 14, examples: ["Harmony", "Camera+"] };

test("the two subscribe-all choices say who credits and who manages, SteamCMD second", async () => {
  const { mod } = await setup(null);
  const c = mod.getChoices();
  assert.equal(c.length, 2);
  assert.match(c[0], /^from Steam .*credits the modder/);
  assert.match(c[1], /^from SteamCMD .*does not credit/);
});

test("nothing due: no question", async () => {
  const { mod, chosen } = await setup(null);
  await mod.checkCredit();
  assert.equal(chosen.length, 0);
});

test("a due batch asks once with its size and examples; replace answers with the batch ids", async () => {
  const { mod, chosen, calls, messages } = await setup(due);
  await mod.checkCredit();
  assert.equal(chosen.length, 1);
  assert.match(chosen[0].title, /credit the modders/);
  assert.match(chosen[0].title, /14 mods, such as Harmony, Camera\+/);
  assert.deepEqual([...chosen[0].labels], ["replace these SteamCMD mods with Steam subscriptions, little by little", "dismiss"]);
  await chosen[0].pick(0);
  await flush();
  assert.deepEqual(JSON.parse(JSON.stringify(calls.at(-1))), ["answer", [3, 5], true]);
  assert.equal(messages.at(-1)[0], "ok");
});

test("dismiss sends replace=false", async () => {
  const { mod, chosen, calls } = await setup(due);
  await mod.checkCredit();
  await chosen[0].pick(1);
  assert.deepEqual(JSON.parse(JSON.stringify(calls.at(-1))), ["answer", [3, 5], false]);
});

test("another popup open: wait, do not ask the backend or cover it", async () => {
  const { mod, S, chosen, calls } = await setup(due);
  S.popup = { title: "something else" };
  await mod.checkCredit();
  assert.equal(calls.length, 0);
  assert.equal(chosen.length, 0);
});

test("checks at the same time (focus and game exit) share one question", async () => {
  const { mod, chosen, calls, hold } = await setup(due);
  const g = hold();
  const a = mod.checkCredit(), b = mod.checkCredit();
  g.resolve();
  await Promise.all([a, b]);
  assert.equal(calls.filter((c) => c[0] === "prompt").length, 1);
  assert.equal(chosen.length, 1);
  // the question is open: a later check does not stack a second one
  await mod.checkCredit();
  assert.equal(chosen.length, 1);
});
