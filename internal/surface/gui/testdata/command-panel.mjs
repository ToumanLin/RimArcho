import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, deferred, flush } from "./dom.mjs";

async function setup(opts = {}) {
  const root = new Element("div"), previous = new Element("button"), document = { activeElement: previous };
  root.append(previous);
  let restored = 0, reloaded = 0, copied = "", resolveStatus = null;
  previous.focus = () => { restored++; document.activeElement = previous; };
  const S = { popup: null, cols: [], focus: 0 }, p = {}, pop = new Element("div"), runs = [];
  let status = { sequence: 0 }, histories = [];
  const palettes = new Map(), asked = [], picked = [];
  const api = {
    Palette: async (text, page, parts) => { asked.push([text, page, parts]); if (opts.found) return opts.found; if (palettes.has(text)) return palettes.get(text).promise; if (text.includes("--")) return null; return [{ id: "home", name: "Launch", kind: 0, alias: "home" }]; },
    CommandStatus: async () => resolveStatus ? resolveStatus.promise : status,
    RunCLI: async text => { runs.push(text); return status = { sequence: 1, running: true }; },
    CommandComplete: async () => ({ candidates: [{ insert: '"Two words"', label: "Two words" }], from: 9, to: 11 }),
    CommandHistory: async (...args) => { histories.push(args); return { at: 0, text: "version", draft: args[2] }; },
  };
  const timers = [];
  const el = (tag, cls, text) => {
    const n = new Element(tag, cls, text); n.value = "";
    n.focus = () => { document.activeElement = n; };
    n.setSelectionRange = (a, b) => { n.selectionStart = a; n.selectionEnd = b; };
    return n;
  };
  const m = await loadModules("./command-panel.js", {
    "./state.js": { S, el, api, T: s => s, errText: e => e.message, factText() {}, keyCaps: k => el("kbd", "", k) },
    "./fields.js": { copy: async text => { copied = text; } },
  }, { document, setTimeout: (fn, ms) => { if (!ms) fn(); else timers.push(fn); } });
  S.popup = p;
  m.buildCommandPanel(pop, p, { close: () => { p.onclose(); S.popup = null; }, pick: o => picked.push(o), reload: () => reloaded++, page: opts.page, parts: opts.parts });
  await flush();
  const input = pop.querySelector("input"), list = pop.querySelector(".opts"), output = pop.querySelector("pre");
  return { input, list, output, p, S, runs, timers, palettes, histories, asked, picked,
    copy: () => pop.querySelector(".command-head").children[1].onclick(), copied: () => copied,
    restored: () => restored, reloads: () => reloaded,
    status: r => { status = r; },
    text: async value => { input.value = value; input.selectionStart = value.length; input.oninput(); await flush(); },
    key: async (key, more) => { p.keys(event(key, more)); await flush(); },
  };
}
test("completion replaces the quoted word, history restores draft, stale search is ignored", async () => {
  const s = await setup();
  await s.text("plan use Tw"); await s.key("Tab");
  assert.equal(s.input.value, 'plan use "Two words" ');
  await s.key("Enter");
  assert.equal(s.runs[0], 'plan use "Two words" ');
  await s.text("draft"); await s.key("ArrowUp");
  assert.equal(s.input.value, "version"); assert.equal(s.histories[0][2], "draft");
  const old = deferred(); s.palettes.set("old", old);
  s.input.value = "old"; s.input.oninput();
  await s.text("new"); old.resolve([{ name: "STALE", kind: 0 }]); await flush();
  assert.equal(s.list.textContent.includes("STALE"), false);
  s.p.onclose(); assert.equal(s.restored(), 1);
});
test("CLI execution keeps raw output and exit code, copies the entire result", async () => {
  const s = await setup(); await s.text("list --active"); await s.key("Enter");
  assert.deepEqual(s.runs, ["list --active"]);
  s.status({ sequence: 1, running: false, output: "#raw\n<literal>\n", code: 3 });
  s.timers.shift()(); await flush();
  assert.equal(s.output.textContent, "#raw\n<literal>\n");
  await s.copy(); assert.equal(s.copied(), "#raw\n<literal>\n");
  assert.equal(s.reloads(), 1);
});
test("16 §3.5: the page and its parts go to the ranking; a part goes there; no name twice", async () => {
  let went = 0;
  const parts = [{ name: "interface", key: "2", run: () => went++ }];
  const found = [
    { kind: 0, id: "analyze", name: "check again", alias: "check again", own: true },
    { kind: 4, id: "0", name: "interface", own: true },
    { kind: 0, id: "sort", name: "trier", alias: "sort" },
  ];
  const s = await setup({ page: "settings", parts, found });
  assert.deepEqual(s.asked[0], ["", "settings", ["interface"]]);
  const rows = s.list.children;
  assert.equal(rows.length, 3);
  // the English name is not repeated beside itself; a translated one keeps it; a part shows its key
  assert.equal(rows[0].children.length, 1);
  assert.equal(rows[1].children[1].tagName.toLowerCase(), "kbd"); assert.equal(rows[1].children[1].textContent, "2");
  assert.equal(rows[2].children[1].textContent, "sort");
  // a hairline after the page's own entries
  assert.equal(rows[1].className.includes("own-end"), true); assert.equal(rows[0].className.includes("own-end"), false);
  assert.equal(rows[0].className.includes(" on"), true);
  await s.key("n", { ctrlKey: true }); await s.key("Enter");
  assert.equal(went, 1); assert.equal(s.picked.length, 0);
});
