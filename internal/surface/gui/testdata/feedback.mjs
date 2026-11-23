import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// Feedback (16 §2.14): two panes with a dragging gap, both starting with a card, the buttons in
// the right card's head, and the template (or My reports) as the breadcrumb's second level.
async function setup(over = {}) {
  const screen = new Element("div"); screen.id = "screen";
  const S = { screen: "", screenSeq: 1 };
  const el = (tag, cls = "", text = "") => new Element(tag, cls, text);
  const $ = (sel) => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen\s+/, ""));
  const tpls = [
    { kind: "feature", label: "feature request", lead: "something RimArcho could do", attach: [], questions: [{ key: "want", label: "what you want to do", form: "lines" }] },
    { kind: "bug", label: "bug", lead: "RimArcho did something wrong", attach: ["log"], questions: [{ key: "did", label: "what you did", form: "lines" }] },
  ];
  const api = {
    FeedbackTemplates: async () => tpls,
    FeedbackReports: async () => ({ reports: [{ number: 7, title: "x", state: "open", replies: [] }] }),
    FeedbackAttach: async () => ({ send: true, basic: ["report: bug"], parts: [], sections: [] }),
    FeedbackCompose: async () => ({ body: "b", url: "u", full: true }),
    FeedbackCheck: async () => ({ reports: [] }),
    FeedbackChoices: async () => ({ diagnoses: [], mods: [] }),
    ...over,
  };
  const wired = [];
  let synced = 0;
  const button = (text, key, run, cls = "") => Object.assign(el("button", "btn " + cls, text), { onclick: run });
  const page = (cls) => el("div", "page " + cls);
  const m = await loadModules("./feedback.js", {
    "./state.js": { S, $, el, api: new Proxy(api, { get: (t, k) => t[k] || (async () => null) }), T: (s) => s, Tf: (s, ...a) => { let i = 0; return s.replace(/%[sd]/g, () => String(a[i++])); }, errText: (e) => e.message, say() {} },
    "./fields.js": { copy: async () => {}, pathContent: () => new Element("span") },
    "./icons.js": { icon: new Proxy({}, { get: () => "" }) },
    "./screens.js": { page, button, area() {}, beginPageRead: (name) => { S.screen = name; return { current: () => true, finish() {}, fail(e) { throw e; } }; } },
    "./separator.js": { wireSeparator: (h, o) => { wired.push([h, o]); return { refresh() {}, ready: Promise.resolve() }; } },
    "./shell.js": { sync: () => { synced++; } },
  }, { setTimeout: () => 0, clearTimeout() {} });
  return { m, screen, S, wired, synced: () => synced };
}

test("the two cards start on one line, with a dragging gap and the buttons inside the right card", async () => {
  const h = await setup();
  await h.m.open();
  await flush(); await flush();
  const lay = h.screen.querySelector(".fb-lay");
  const gap = h.screen.querySelector(".fb-lay .split");
  assert.ok(gap, "the gap between the cards is a splitter");
  assert.equal(h.wired.length, 1);
  assert.equal(h.wired[0][0], gap);
  assert.equal(h.wired[0][1].key, "ui.gui.feedback.side");
  for (const col of lay.children.filter((c) => c !== gap))
    assert.ok(col.children[0].classList.contains("card"), col.className + " starts with " + col.children[0].className);
  // the problems, the buttons and Archo are the right card's head, not loose above it
  assert.ok(h.screen.querySelector(".fb-main .fb-card .fb-bar .fb-acts .fb-send"));
  assert.ok(h.screen.querySelector(".fb-card .fb-bar .fb-add"));
});

test("the breadcrumb names the template shown, then My reports", async () => {
  const h = await setup();
  await h.m.open();
  await flush(); await flush();
  assert.equal(h.S.feedbackCrumb, "feature request");
  h.screen.querySelector('.fb-kind[data-kind="bug"]').onclick();
  assert.equal(h.S.feedbackCrumb, "bug");
  h.screen.querySelector('.fb-kind[data-kind="mine"]').onclick();
  assert.equal(h.S.feedbackCrumb, "my reports");
  assert.ok(h.synced() >= 3);
  // My reports is one card too, its note in the head
  assert.ok(h.screen.querySelector(".fb-main .fb-card .fb-mine-bar"));
});

test("an attached part's chip says its short name; the heading with the file leads its hover", async () => {
  const heading = "game log errors (Player.log of Nov 22 23:40)";
  const h = await setup({ FeedbackAttach: async () => ({ send: true, basic: ["report: bug"], parts: [{ key: "log", label: "game log", note: "" }], sections: [{ key: "log", heading, lines: ["E1 exception x3"] }, { key: "mod", heading: "the Mod Harmony", lines: ["harmony"] }] }) });
  await h.m.open();
  await flush(); await flush(); await flush();
  const chips = h.screen.querySelectorAll(".fb-chips .fb-chip");
  const log = chips.find((c) => c.textContent === "game log");
  assert.ok(log, "the log chip is short: " + chips.map((c) => c.textContent).join(" | "));
  assert.equal(log.title.split("\n")[0], heading);
  assert.ok(log.title.includes("E1 exception x3"));
  assert.ok(chips.some((c) => c.textContent === "the Mod Harmony"), "a section without a part keeps its heading");
});
