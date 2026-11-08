import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules } from "./dom.mjs";

// A status badge's popover (16 §4.2) must not open during a row drag (16 §8.5): it would
// cover the rows under the pointer, so the drop there would do nothing.
async function setup() {
  const body = new Element("body"), rows = new Element("div", "rows");
  const row = new Element("div", "row"), badge = new Element("span", "s");
  row.dataset.i = "0"; badge.append(new Element("svg"));
  row.append(badge); rows.append(row); body.append(rows);
  const el = (tag, cls = "", text = "") => new Element(tag, cls, text);
  const S = { board: { reasons: { m1: [{ status: 2, text: "90 patches do nothing" }] } } };
  const q = await loadModules("./badgepop.js", {
    "./state.js": { S, el, T: (s) => s, factText: () => "" },
    "./icons.js": { statusIcon: () => "" },
    "./needtext.js": { needText: (tag, cls, text) => el(tag, cls, text), needEvidence: () => el("div") },
  }, {
    document: { body, addEventListener() {} }, innerHeight: 800,
    setTimeout: () => 0, clearTimeout() {},
  });
  q.wireBadges(1, rows, () => ({ id: "m1" }), () => 2);
  const fire = (type, more = {}) => rows.dispatchEvent({ type, target: badge, button: 0, ...more });
  const popover = () => body.querySelector(".badge-pop");
  return { q, fire, popover };
}

test("hovering a badge shows its popover, but not while a row drag runs", async () => {
  const h = await setup();
  h.fire("mouseover");
  assert.ok(h.popover(), "hover shows the popover");
  h.q.hide(true);
  h.q.rowDrag(true);
  h.fire("mouseover");
  assert.equal(h.popover(), null, "no popover during a drag");
  h.q.rowDrag(false);
  h.fire("mouseover");
  assert.ok(h.popover(), "hover works again after the drop");
});

test("starting a drag closes an open popover, a pinned one too", async () => {
  const h = await setup();
  h.fire("mouseover");
  h.q.rowDrag(true);
  assert.equal(h.popover(), null);
  h.q.rowDrag(false);
  h.fire("mousedown"); // the press that starts a drag may land on a badge and pin it
  assert.ok(h.popover()?.classList.contains("pinned"));
  h.q.rowDrag(true);
  assert.equal(h.popover(), null);
  h.q.rowDrag(false);
});
