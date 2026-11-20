import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// The Import page (16 §2.18): the kinds column, the layout switch, no export, and drops.
async function setup(api = {}, { layout = "" } = {}) {
  const screen = new Element("div"); screen.id = "screen";
  const S = { screen: "", screenSeq: 1, shell: { plan: { id: 1 }, plans: [{ id: 1, name: "Main" }] }, cols: [] };
  const el = (tag, cls = "", text = "") => new Element(tag, cls, text);
  const $ = (sel) => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen\s+/, ""));
  const calls = [], said = [];
  const base = {
    ExchangeInfo: async () => ({ game: { path: "C:/g/ModsConfig.xml", mods: 3 }, dialogs: true }),
    ImportSurvey: async () => [],
    CheckPlanName: async () => ({ free: "x", problem: "" }),
    ...api,
  };
  const recorded = new Proxy(base, {
    get(t, k) { return (...a) => { calls.push([k, ...a]); return k in t ? t[k](...a) : Promise.resolve(null); }; },
  });
  const button = (text, key, run, cls = "") => Object.assign(el("button", "btn " + cls, text), { onclick: run });
  const page = (cls, lead) => { const w = el("div", cls); w.append(el("p", "lead", lead)); return w; };
  const docEl = new Element("html"), body = new Element("body"), listeners = {};
  const document = { documentElement: docEl, body, createElement: (t) => new Element(t), querySelector: (s) => body.querySelector(s), addEventListener: (t, f) => { listeners[t] = f; } };
  if (layout) docEl.dataset.importLayout = layout;
  const m = await loadModules("./exchange.js", {
    "./feedbacksemantics.js": { fieldError() {} },
    "./state.js": {
      factText: (e, t) => { e.text = [].concat(t).map((x) => x?.text ?? x).join("  "); return e; },
      S, $, el, api: recorded, T: (s) => s, Tf: (s, ...a) => { let i = 0; return s.replace(/%[sd]/g, () => String(a[i++])); },
      errText: (e) => e.message, say: (k, t) => said.push([k, t]),
    },
    "./screens.js": { page, button, area() {}, beginPageRead: (name) => { S.screen = name; return { current: () => true, finish() {}, fail(e) { throw e; } }; } },
    "./icons.js": { icon: new Proxy({}, { get: () => "" }) },
    "./popups.js": { confirm() {}, choose() {}, ask() {} },
    "./credit.js": { getChoices: () => [] },
    "./motion.js": { still: () => true },
    "./membar.js": {
      splitBar: () => el("div", "sbar-row"), diffMarks: () => el("span", "dmarks"), stateMark: () => el("span", "smark"),
      visTip: (n, t) => { n.setAttribute("aria-label", t); return n; },
    },
  }, { document, location: { search: "" }, URLSearchParams, navigator: {}, setTimeout: () => 0 });
  m.initExchange({ selectedList: () => 1, renderWhere() {}, renderHints() {}, go() {}, reload: async () => {}, resumeGuide() {} });
  return { m, screen, calls, said, listeners, S, body };
}

const fileDrop = (name, { dir = false, text = "" } = {}) => ({
  dataTransfer: {
    types: ["Files"],
    items: [{ kind: "file", getAsFile: () => ({ name, text: async () => text, slice() { return this; } }), webkitGetAsEntry: () => ({ isDirectory: dir }) }],
    files: [{ name }],
  },
  defaultPrevented: false,
  preventDefault() { this.defaultPrevented = true; },
});

test("dropKind names lists, packs, managers' data and folders, and nothing else", async () => {
  const { m } = await setup();
  const cases = {
    "Medieval run.xml": "list", "ModsConfig.xml": "list", "a.rml": "list", "Vanilla+.archolist.json": "list", "list.txt": "list", "Autosave-1.rws": "list",
    "Medieval.archopack": "pack", "D:\\Packs\\Medieval.ARCHOPACK": "pack",
    "aux_metadata.db": "manager", "rimcrow.db": "manager", "userRules.json": "manager",
    "notes.docx": "", "picture.png": "", "": "",
  };
  for (const [name, want] of Object.entries(cases)) assert.equal(m.dropKind(name), want, name);
  assert.equal(m.dropKind("RimSort", true), "folder");
});

test("the page has no export: no export call, no share section, five import kinds", async () => {
  const h = await setup();
  await h.m.exchange();
  await flush();
  assert.deepEqual(h.calls.map((c) => c[0]).filter((k) => /export|^Packs$/i.test(k)), []);
  const text = h.screen.textContent;
  assert.ok(!/export|share/i.test(text), text);
  assert.deepEqual(h.screen.querySelectorAll(".xc-kind").map((b) => b.dataset.kind), ["game", "file", "paste", "pack", "managers"]);
});

test("the layout follows data-import-layout, 2col by default", async () => {
  for (const [layout, want] of [["", "2col"], ["3col", "3col"], ["2col", "2col"], ["bogus", "2col"]]) {
    const h = await setup({}, { layout });
    await h.m.exchange();
    assert.equal(h.screen.querySelector(".xc-lay").dataset.layout, want, layout);
  }
});

test("picking the game's list reads it at once and the preview compares it with the current list", async () => {
  const preview = { summary: "s", format: "ModsConfig.xml", total: 3, missing: [{ name: "Gone" }], installed: [{ name: "A", enabled: true }, { name: "B", enabled: false }, { name: "C", enabled: true, other: true }], replace: { added: 1, text: "1 added" }, merge: { text: "" } };
  const h = await setup({ ImportPreviewFor: async () => preview });
  await h.m.exchange();
  h.screen.querySelector('.xc-kind[data-kind="file"]').onclick();
  h.screen.querySelector('.xc-kind[data-kind="game"]').onclick();
  await flush(); await flush();
  assert.deepEqual(h.calls.find((c) => c[0] === "ImportPreviewFor").slice(2), ["file", "C:/g/ModsConfig.xml"]);
  const counts = h.screen.querySelectorAll(".ccounts .cn").map((c) => c.getAttribute("aria-label"));
  assert.deepEqual(counts, ["1 already in your list", "1 installed, not in your list", "1 in another version", "1 not installed"]);
});

test("an unknown dropped file is refused in the preview, not ignored", async () => {
  const h = await setup();
  await h.m.exchange();
  const e = fileDrop("Colony notes.docx");
  await h.listeners.drop(e);
  assert.ok(e.defaultPrevented);
  assert.equal(h.screen.querySelector(".xc-preview").dataset.mode, "refused");
  assert.ok(h.screen.querySelector(".xc-preview").textContent.includes("Colony notes.docx"));
  assert.ok(h.said.some(([k]) => k === "fail"));
  assert.ok(!h.calls.some((c) => c[0] === "ImportPreviewFor"));
});

test("a dropped list is read and previewed; its kind is picked", async () => {
  const h = await setup({ ImportPreviewFor: async () => ({ summary: "s", total: 0, missing: [], installed: [], replace: { same: true }, merge: {} }) });
  await h.m.exchange();
  await h.listeners.drop(fileDrop("Medieval run.xml", { text: "<ModsConfigData/>" }));
  await flush();
  const call = h.calls.find((c) => c[0] === "ImportPreviewFor");
  assert.deepEqual(call.slice(2), ["text", "<ModsConfigData/>"]);
  assert.ok(h.screen.querySelector(".xc-kind.on").dataset.kind === "file");
});

test("a dropped pack or folder without a path asks for the dialog; with a path it is added", async () => {
  const h = await setup({ PackUpdateOffer: async () => null, StartPackImport: async () => 7, TaskByID: async () => ({ state: "run" }) });
  await h.m.exchange();
  await h.listeners.drop(fileDrop("Medieval.archopack"));
  assert.equal(h.screen.querySelector(".xc-preview").dataset.mode, "refused");
  assert.ok(!h.calls.some((c) => c[0] === "StartPackImport"));
  await h.listeners.drop(fileDrop("RimSort", { dir: true }));
  assert.equal(h.screen.querySelector(".xc-kind.on").dataset.kind, "managers");
  await h.m.dropped({ name: "Medieval.archopack", path: "D:/Packs/Medieval.archopack" });
  await flush();
  assert.deepEqual(h.calls.find((c) => c[0] === "StartPackImport").slice(1), ["D:/Packs/Medieval.archopack", false]);
});

test("a drop a form already took is left to it", async () => {
  const h = await setup();
  await h.m.exchange();
  const e = fileDrop("notes.docx"); e.defaultPrevented = true;
  await h.listeners.drop(e);
  assert.ok(h.screen.querySelector(".xc-preview").hidden);
});
