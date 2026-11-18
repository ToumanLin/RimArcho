import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// The Import page (16 §2.18): four ways, each only opening an existing flow.
async function setup({ found = [], envs = ["1.6", "1.5"] } = {}) {
  const screen = new Element("div"), S = { screen: "import", board: { envs } };
  const calls = [];
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const api = {
    DiscoverManagerImports: async () => found,
    PickImportFile: async () => "C:\\lists\\main.xml",
    PickPackFile: async () => "C:\\packs\\a.archopack",
    PackImport: async (path) => { calls.push(["pack", path]); return "imported pack a"; },
  };
  const ui = {
    go: (id) => calls.push(["go", id]), importManagers: () => calls.push(["managers"]),
    reload: async () => calls.push(["reload"]), renderWhere() {},
  };
  const deps = {
    "./state.js": { S, $: () => screen, el, api, T: s => s, Tf: (s, ...a) => s + " " + a.join(" "), errText: String, say: (k, t) => calls.push(["say", t]) },
    "./screens.js": {
      page: () => el("div"),
      button: (label, _, run) => { const b = el("button", "", label); b.onclick = run; return b; },
      beginPageRead: () => ({ finish() {} }),
    },
    "./exchange.js": { importFrom: (kind, src) => calls.push(["importFrom", kind, src]) },
  };
  const ways = await loadModules("./importways.js", deps);
  deps["./importways.js"] = ways;
  const hub = await loadModules("./importhub.js", deps);
  ways.initImportWays(ui); hub.initImportHub(ui);
  hub.open(); await flush();
  const button = (label) => screen.querySelectorAll("button").find(b => b.textContent === label);
  return { screen, calls, button, ways };
}

test("the page has the four ways and says what discovery found", async () => {
  const h = await setup({ found: [{ manager: "RimSort", kind: "list" }, { manager: "RimSort", kind: "rimsort-rules" }, { manager: "RimCrow", kind: "rimcrow" }] });
  assert.equal(h.screen.querySelectorAll(".way").length, 4);
  assert.match(h.screen.textContent, /found %s: %d RimSort 2/);
  assert.match(h.screen.textContent, /found %s: %d RimCrow 1/);
  assert.match(h.screen.textContent, /added: %s 1\.6, 1\.5/);
});

test("nothing found says so", async () => {
  const h = await setup();
  assert.match(h.screen.textContent, /No RimSort or RimCrow data found\./);
});

test("each way opens its existing flow", async () => {
  const h = await setup();
  h.button("add a game").onclick();
  h.button("import…").onclick();
  await h.button("choose a file…").onclick(); await flush();
  const field = h.screen.querySelectorAll("input")[0];
  field.value = " RC-abc ";
  h.button("open").onclick();
  await h.button("choose a pack…").onclick(); await flush();
  assert.deepEqual(h.calls, [
    ["go", "guide"], ["managers"], ["importFrom", "file", "C:\\lists\\main.xml"], ["importFrom", "text", "RC-abc"],
    ["pack", "C:\\packs\\a.archopack"], ["say", "imported pack a"], ["reload"], ["go", "packs"],
  ]);
});
