import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, deferred, event, flush } from "./dom.mjs";

// The load-order rules fold: Enter twice while the rule is on its way adds it once.
test("enter twice in the rule form saves the rule once", async () => {
  const area = new Element("div", "rulesarea"), bar = new Element("div", "hbar"), body = new Element("div", "rulesbody");
  area.append(bar, body);
  const saves = [], reply = deferred();
  const api = {
    Rules: async () => ({ rows: [], kinds: [{ kind: "first", label: "%s loads first", two: false }] }),
    SaveRule: (...args) => { saves.push(args); return reply.promise; },
  };
  const rules = await loadModules("./rules.js", {
    "./choices.js": { choiceControl: () => new Element("div") },
    "./state.js": { $: (s, root) => s === ".rulesarea" ? area : (root || area).querySelector(s), el: (t, c, x) => new Element(t, c, x), api, T: s => s, errText: String, say() {} },
    "./icons.js": { icon: {} },
    "./screens.js": { seg: () => new Element("div") },
  }, { document: { activeElement: null }, requestAnimationFrame: () => {}, setTimeout, clearTimeout });
  rules.initRules({ reload() {} });
  await rules.refreshRules();
  rules.startRule({ id: "mod-a", name: "Mod A" }); await flush();
  const note = body.querySelector(".rule-note");
  note.onkeydown(event("Enter")); note.onkeydown(event("Enter")); await flush();
  assert.equal(saves.length, 1);
  reply.resolve("added"); await flush();
});
