import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// ctrl+j (16 §2.16): opens Background tasks; again goes back to where it came from, or back in
// the page history when the page was opened another way.
async function setup() {
  const S = { screen: "home", screenSeq: 1 }, routes = [];
  let backs = 0;
  const tasks = await loadModules("./tasks.js", {
    "./feedbacksemantics.js": { progress() {} },
    "./fields.js": { pathContent: n => n },
    "./state.js": { S, $: () => null, el: (t, c, x) => new Element(t, c, x), api: { Tasks: async () => ({ rows: [] }), TasksSeen: async () => {}, Setting: async () => "" }, Tf: s => s, errText: String, say() {} },
    "./screens.js": { beginPageRead: name => (S.screen = name, { current: () => false, finish() {}, fail() {} }), page: () => new Element("div"), act: () => new Element("button"), area() {} },
    "./filterbar.js": { createBar: () => ({}) },
    "./popups.js": {},
    "./icons.js": { icon: {} },
    "./motion.js": { still() {} },
    "./datatable.js": { createTable: () => ({}), mergeColumns: () => [] },
    "./separator.js": { wireSeparator() {} },
  }, { document: { body: new Element("body"), activeElement: null } });
  // as navigation's go: a pack page sets the pack focus, any other page clears it
  const go = id => {
    routes.push(id);
    S.packFocus = id.startsWith("pack:") ? id.slice(5) : null;
    if (id === "tasks") return tasks.open(); // its page read shows the page
    S.screen = id.startsWith("pack:") ? "packs" : id === "plan" ? null : id;
  };
  tasks.initTasks({ go, back: () => { backs++; } });
  return { tasks, S, routes, go, backs: () => backs };
}

test("ctrl+j from a pack returns to that pack and lets the pack focus go", async () => {
  const h = await setup();
  h.S.screen = "packs"; h.S.packFocus = "Raid pack";
  await h.tasks.toggle(); await flush();
  assert.equal(h.S.screen, "tasks");
  assert.equal(h.S.packFocus, null, "openPage(\"tasks\") is not taken over by the pack");
  await h.tasks.toggle();
  assert.deepEqual(h.routes, ["tasks", "pack:Raid pack"]);
});

test("ctrl+j on a Tasks page opened another way goes back, not to an old page", async () => {
  const h = await setup();
  await h.tasks.toggle(); await flush(); await h.tasks.toggle(); // home → tasks → home
  h.go("settings"); await h.go("tasks"); await flush(); // the sidebar's foot line
  await h.tasks.toggle();
  assert.equal(h.backs(), 1);
  assert.deepEqual(h.routes, ["tasks", "home", "settings", "tasks"]);
});
