import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event } from "./dom.mjs";

// The list editor's head (16 §2.5): problems first; the tools as icons whose words are their
// hover and accessible name; save carries the unsaved count; launch last.
async function head(x = {}) {
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const m = await loadModules("./planhead.js", {
    "./state.js": { el, T: (s) => s, Tf: (f, ...a) => a.reduce((s, v) => s.replace(/%[ds]/, v), f), keyCaps: (k) => el("kbd", "", k) },
    "./icons.js": { icon: new Proxy({}, { get: () => "<svg/>" }) },
    "./fields.js": { pathContent: () => new Element("span") },
  });
  const calls = [];
  const acts = m.planActions({
    unsaved: 0, plan: { id: 7 }, current: null,
    problemsChip: () => el("button", "chip prob", "11 problems"),
    openNeeds: () => calls.push("needs"),
    rescanButton: () => { const b = el("button", "btn icon rescan"); b.setAttribute("aria-label", "rescan"); return b; },
    rename: (id) => calls.push("rename " + id), remove: (id) => calls.push("delete " + id), use: (id) => calls.push("use " + id),
    manage: () => calls.push("manage"), save: () => calls.push("save"), launch: () => calls.push("launch"),
    ...x,
  });
  return { acts, calls };
}

const label = (b) => b.attributes["aria-label"] || b.textContent;

test("problems, then the tools as icons, then save and launch", async () => {
  const { acts, calls } = await head({ unsaved: 32 });
  assert.equal(acts.length, 4);
  assert.ok(acts[0].classList.contains("prob"));
  const tools = acts[1];
  assert.ok(tools.classList.contains("ph-tools"));
  assert.deepEqual(tools.children.map(label).map((s) => s.split(";")[0]), ["rescan", "rename", "import/export", "delete"]);
  for (const b of tools.children) {
    assert.equal(b.textContent, "", "a tool shows no words: " + label(b));
    assert.ok(b.title || b.attributes["aria-label"], "its words are on hover");
  }
  // one fact, one place: the unsaved count is the save button's badge, nothing beside it
  const save = acts[2];
  assert.equal(save.querySelector(".unwritten").textContent, "32");
  assert.ok(save.title.startsWith("32 unsaved"));
  assert.equal(acts.filter((a) => a.classList.contains("unwritten")).length, 0);
  assert.equal(acts[3].dataset.envWrite, "launch");
  for (const b of [tools.children[1], tools.children[3], save, acts[3]]) b.onclick(event(""));
  assert.deepEqual(calls, ["rename 7", "delete 7", "save", "launch"]);
});

test("nothing unsaved: save says no changes and is off; another current list: set current", async () => {
  const { acts } = await head({ current: { name: "J&Y" } });
  const save = acts[2];
  assert.equal(save.disabled, true);
  assert.equal(save.querySelector(".unwritten"), null);
  assert.equal(acts[3].dataset.envWrite, "use-plan");
});
