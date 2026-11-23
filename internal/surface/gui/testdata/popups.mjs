import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules } from "./dom.mjs";

// A popup that refuses to close (the first-run folder dialog while it works) is not replaced:
// ctrl+k over it used to swap in the palette, skip the dialog's onclose and leave #app inert.
test("opening over a popup that refuses to close keeps it", async () => {
  const app = new Element("div"), popup = new Element("div"), page = new Element("main");
  app.id = "app"; popup.id = "popup"; app.append(page, popup);
  const S = {};
  const pop = await loadModules("./popups.js", {
    "./choices.js": { closeChoiceMenu() {} },
    "./fields.js": { pathContent: n => n, copyablePath: n => n },
    "./state.js": { S, $: s => s === "#app" ? app : s === "#popup" ? popup : app.querySelector(s), el: (t, c, x) => new Element(t, c, x), api: {}, Tf: s => s, errText: String, say() {} },
    "./command-panel.js": { buildCommandPanel() {} },
  }, { document: { activeElement: null } });
  pop.initPopups({ renderWhere() {}, renderHeads() {} });
  let busy = true, closed = 0;
  pop.open("ask", "install RimArcho", (box, p) => { p.canClose = () => !busy; p.onclose = () => closed++; });
  const dialog = S.popup;
  pop.open("palette", "command palette", () => {});
  assert.equal(S.popup, dialog, "the busy dialog stays");
  assert.equal(closed, 0);
  busy = false;
  pop.open("palette", "command palette", () => {});
  assert.equal(S.popup.kind, "palette");
  assert.equal(closed, 1, "a dialog that may close runs its onclose");
});

// The arrows, Home and End pass over disabled choices; a disabled current choice is not
// selected when the list opens (F19).
test("the keys skip disabled options", async () => {
  const app = new Element("div"), popup = new Element("div");
  app.id = "app"; popup.id = "popup"; app.append(popup);
  const S = {}, picked = [];
  const pop = await loadModules("./popups.js", {
    "./choices.js": { closeChoiceMenu() {} },
    "./fields.js": { pathContent: n => n, copyablePath: n => n },
    "./state.js": { S, $: s => s === "#app" ? app : s === "#popup" ? popup : app.querySelector(s), el: (t, c, x) => new Element(t, c, x), api: {}, Tf: s => s, errText: String, say() {} },
    "./command-panel.js": { buildCommandPanel() {} },
  }, { document: { activeElement: null }, setTimeout() {} });
  pop.initPopups({ renderWhere() {}, renderHeads() {} });
  const off = (name) => ({ name, disabled: true });
  pop.choose("pick", [off("a"), "b", off("c"), "d", off("e")], (i) => picked.push(i), 0);
  const p = S.popup, key = (k) => p.keys({ key: k, preventDefault() {} });
  assert.equal(p.sel, 1, "the disabled current choice moves to the next enabled one");
  key("ArrowDown"); assert.equal(p.sel, 3);
  key("ArrowDown"); assert.equal(p.sel, 3, "no enabled option below: it stays");
  key("ArrowUp"); assert.equal(p.sel, 1);
  key("ArrowUp"); assert.equal(p.sel, 1);
  key("End"); assert.equal(p.sel, 3);
  key("Home"); assert.equal(p.sel, 1);
  key("Enter"); assert.deepEqual(picked, [1]);
});
