import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules } from "./dom.mjs";

// An outside `plan use` (CLI, MCP) while the window shows Can it run or Launch: on the
// focus the board is read again and, because the open list moved, the visible page is
// drawn again (VM round 2026-11-23: Can it run stayed one switch behind).
test("an outside list switch redraws the visible page on focus", async () => {
  const { lookAgain } = await loadModules("./outsidechange.js", {}, {});
  const S = { screen: "needs", board: { env: "main", stamp: "1" }, shell: { plan: { id: 1, current: true } } };
  let stamp = "1", redraws = 0;
  const reload = async () => { S.board = { env: "main", stamp }; S.shell = { plan: { id: next, current: true } }; return true; };
  const redraw = async () => { redraws++; };
  let next = 1;
  assert.equal(await lookAgain(S, async () => stamp, reload, redraw), false, "nothing changed: nothing read");
  stamp = "2"; next = 2;
  assert.equal(await lookAgain(S, async () => stamp, reload, redraw), true);
  assert.equal(redraws, 1, "the open list moved: the page is drawn again");
  stamp = "3";
  await lookAgain(S, async () => stamp, reload, redraw);
  assert.equal(redraws, 1, "another change on the same list: the board only");
  S.screen = null; stamp = "4"; next = 3;
  await lookAgain(S, async () => stamp, reload, redraw);
  assert.equal(redraws, 1, "the editor is the board itself");
  S.screen = "guide"; stamp = "5"; next = 1;
  await lookAgain(S, async () => stamp, reload, redraw);
  assert.equal(redraws, 1, "the guide is not restarted");
  S.screen = "home"; stamp = "6"; next = 2;
  await lookAgain(S, async () => stamp, async () => undefined, redraw);
  assert.equal(redraws, 1, "a read that did not happen redraws nothing");
});
