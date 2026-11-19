import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules } from "./dom.mjs";

// A Game Config switch takes its own "switching to …" line off the status bar when it ends
// (16 §6). The first-run guide's switch used to leave it there for minutes (B4 screenshot 06).
async function load() {
  const status = new Element("div");
  status.dataset = {};
  const S = { busy: {}, envSeq: 0, screenSeq: 0 };
  const say = (kind, text) => { status.dataset.kind = kind; status.dataset.message = text; };
  const body = new Element("body");
  body.matches = () => false;
  const env = await loadModules("./environment.js", {
    "./state.js": { S, Tf: (f, a) => f.replace("%s", a), say, $: sel => sel === "#status" ? status : null },
    "./api.js": { setEnvironmentGuard() {} },
  }, { document: { body }, Node: Element, MutationObserver: class { observe() {} disconnect() {} } });
  return { env, status, S, say };
}

test("the switch's line goes when the switch ends", async () => {
  const { env, status, S } = await load();
  let during;
  assert.equal(await env.withEnvironmentSwitch("1.6", async () => { during = status.dataset.message; }), true);
  assert.equal(during, "switching to 1.6…");
  assert.equal(status.dataset.message, "", "the line is cleared");
  assert.equal(S.busy.env, false);
});

test("a message put there during the switch stays", async () => {
  const { env, status, say } = await load();
  await env.withEnvironmentSwitch("1.6", async () => { say("busy", "scanning 3/10"); });
  assert.equal(status.dataset.message, "scanning 3/10");
});
