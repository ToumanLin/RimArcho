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
