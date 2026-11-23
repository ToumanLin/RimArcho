import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush, event } from "./dom.mjs";

// Saves › Play review (16 §2.11): the page's request lifecycle and what a newer part of the
// review keeps.
async function setup() {
  const S = { screen: null, screenSeq: 0 };
  const document = { body: new Element("body"), activeElement: null };
  Element.prototype.focus = function () { document.activeElement = this; };
  const screen = new Element("div"); screen.id = "screen";
  const status = new Element("div"); status.id = "status";
  const root = new Element("div"); root.append(screen, status);
  const say = (kind, text) => { status.dataset.kind = kind || ""; status.dataset.message = text || ""; };
  const $ = (s) => s === "#screen" ? screen : s === "#status" ? status : root.querySelector(s.replace(/^#screen /, ""));
  const asked = [], marks = [];
  let handler = null;
  const el = (t, c, x) => new Element(t, c, x);
  const review = await loadModules("./review.js", {
    "./state.js": { S, $, el, api: { SaveReviewRead: async (n, colony) => { asked.push({ n, colony }); return true; }, Detail: async () => ({}), SaveReviewMark: async (lib, v) => { marks.push([lib, v]); return "marked"; } }, T: (s) => s, Tf: (s, ...a) => s + " " + a.join("/"), say, factText: (n) => n },
    "./api.js": { on: (name, fn) => { if (name === "review") handler = fn; } },
    "./screens.js": {
      beginPageRead: (name) => { if (S.screen !== name) S.screenSeq++; S.screen = name; const seq = S.screenSeq; return { current: () => S.screen === name && S.screenSeq === seq, finish() {}, fail(e) { throw e; } }; },
      page: (cls, lead) => { const w = el("div", "page " + cls), head = el("div", "page-head"); if (lead) head.append(el("p", "lead", lead)); w.append(head); return w; },
      area() {},
      button: (label, _k, fn, cls) => { const b = el("button", cls, label); b.onclick = fn; return b; },
    },
    "./icons.js": { icon: { chev: "" } },
    "./detail.js": { detailFrame: () => ({ head() {}, detail() {}, settle: async () => {}, show() {} }) },
    "./separator.js": { wireSeparator() {} },
    "./choices.js": { choiceControl: () => el("select") },
    "./librarylink.js": { openLibraryInstance() {} },
    "./tagcolor.js": { tagColors: () => ({}), tagCSS: () => "" },
  }, { document, localStorage: { getItem: () => null, setItem() {} } });
  const send = (ev) => handler(ev);
  const leave = () => { S.screen = "saves"; S.screenSeq++; };
  return { review, S, screen, status, asked, marks, send, document, leave };
}

const mod = (name, library, more = {}) => ({ name, packageId: "tester.hats", library, sum: "1 of 2 seen", where: "1 colony, 1 save", adds: 2, seen: 1, size: 2, part: 1, tip: "",
  cats: [{ label: "apparel", adds: 2, seen: 1, names: ["Helm"] }, { label: "weapons", adds: 3, seen: 0, names: ["Bow", "Axe", "Pike"] }], ...more });
const pageOf = (more = {}) => ({ lead: "2 saves", colony: "c1", colonies: [{ key: "c1", name: "Fort", line: "" }], all: "1 colony", done: 1, total: 2,
  groups: [{ key: "rest", label: "rest", hint: "", mods: [mod("Hats", "copy-a")] }], ...more });
const find = (root, cls, test = () => true) => root.querySelectorAll("." + cls).find(test);

test("a review stopped by leaving the page ends, and takes its progress off the status line", async () => {
  const h = await setup();
  const opened = h.review.open();
  const n = h.asked.at(-1).n;
  h.send({ request: n, partial: true, page: pageOf() });
  assert.equal(h.status.dataset.kind, "busy");
  h.leave();
  h.send({ request: n, error: "stale" });
  await opened;
  assert.equal(h.status.dataset.kind, "", "the busy indicator went with the review");
});

test("a review superseded by another colony's leaves the newer progress alone", async () => {
  const h = await setup();
  const opened = h.review.open();
  const n = h.asked.at(-1).n;
  h.send({ request: n, page: pageOf({ colonies: [{ key: "c1", name: "Fort", line: "" }, { key: "c2", name: "Camp", line: "" }] }) });
  await opened;
  find(h.screen, "rv-col", (b) => b.dataset.colony === "c2").onclick();
  const m = h.asked.at(-1).n;
  assert.notEqual(m, n);
  h.send({ request: m, partial: true, page: pageOf({ colony: "c2" }) });
  h.send({ request: n, error: "stale" }); // a late end of the old one changes nothing
  assert.equal(h.status.dataset.kind, "busy");
  h.send({ request: m, page: pageOf({ colony: "c2", done: 2 }) });
  await flush();
  assert.equal(h.status.dataset.kind, "");
});

test("coming back starts afresh: the colony and the Mod picked are forgotten", async () => {
  const h = await setup();
  let opened = h.review.open();
  h.send({ request: h.asked.at(-1).n, page: pageOf({ colonies: [{ key: "c1", name: "Fort", line: "" }, { key: "c2", name: "Camp", line: "" }] }) });
  await opened;
  find(h.screen, "rv-col", (b) => b.dataset.colony === "c2").onclick();
  h.leave();
  opened = h.review.open();
  assert.equal(h.asked.at(-1).colony, "", "the next visit asks for the default colony");
  h.send({ request: h.asked.at(-1).n, page: pageOf() });
  await opened;
});

test("each part keeps the fold, the open categories, their scroll and the focus; the lead and note follow", async () => {
  const h = await setup();
  const opened = h.review.open();
  const n = h.asked.at(-1).n;
  h.send({ request: n, partial: true, page: pageOf() });
  const why = find(h.screen, "rv-why");
  why.open = true; why.dispatchEvent({ type: "toggle" });
  find(h.screen, "rv-cath", (b) => b.dataset.cat === "weapons").onclick();
  find(h.screen, "rv-cats").scrollTop = 120;
  assert.equal(h.document.activeElement.dataset.cat, "weapons");
  h.send({ request: n, partial: true, page: pageOf({ lead: "5 saves", note: "1 save could not be read and are not counted", done: 2 }) });
  assert.equal(find(h.screen, "rv-why").open, true, "the not exact line stays open");
  assert.equal(find(h.screen, "rv-cats").scrollTop, 120);
  const focused = h.document.activeElement;
  assert.ok(focused.isConnected && focused.dataset.cat === "weapons", "the focus stays on the category");
  assert.equal(find(h.screen, "rv-cath", (b) => b.dataset.cat === "weapons").getAttribute("aria-expanded"), "true");
  assert.equal(find(h.screen, "lead").textContent, "5 saves");
  const note = find(h.screen, "sv-note");
  assert.equal(note.hidden, false);
  assert.match(note.textContent, /could not be read/);
  h.send({ request: n, page: pageOf({ lead: "5 saves" }) });
  await opened;
  assert.equal(find(h.screen, "sv-note").hidden, true);
});

test("an input method's Enter in the find box stays in the box", async () => {
  const h = await setup();
  const opened = h.review.open();
  h.send({ request: h.asked.at(-1).n, page: pageOf() });
  await opened;
  const box = find(h.screen, "sv-q");
  box.focus();
  box.onkeydown(event("Enter", { isComposing: true }));
  assert.equal(h.document.activeElement, box);
  box.onkeydown(event("Enter"));
  assert.ok(h.document.activeElement.className.includes("rv-cols"));
});

test("two copies of one packageId are two tiles, picked one at a time", async () => {
  const h = await setup();
  const opened = h.review.open();
  h.send({ request: h.asked.at(-1).n, page: pageOf({ groups: [{ key: "rest", label: "rest", hint: "", mods: [mod("Hats", "copy-a"), mod("Hats", "copy-b")] }] }) });
  await opened;
  const tiles = h.screen.querySelectorAll(".rv-tile");
  assert.equal(tiles.length, 2);
  tiles[1].onclick();
  const on = h.screen.querySelectorAll(".rv-tile").filter((b) => b.classList.contains("on"));
  assert.deepEqual(on.map((b) => b.dataset.mod), ["copy-b"]);
});

// B4 screenshot 66: VEF was marked functional and left the map, and the details showed
// Anomaly, the page's first Mod in data order, with no tile marked. The pick moves to the
// tile that took its place, and the details and the mark button follow it.
test("a Mod marked functional leaves the map and the pick moves to the next tile", async () => {
  const h = await setup();
  const three = [mod("Anomaly", "dlc"), mod("Ideology", "ide"), mod("VEF", "vef"), mod("Hats", "hats")];
  const opened = h.review.open();
  h.send({ request: h.asked.at(-1).n, page: pageOf({ groups: [{ key: "rest", label: "rest", hint: "", mods: three }] }) });
  await opened;
  find(h.screen, "rv-tile", (b) => b.dataset.mod === "vef").onclick();
  const named = () => find(h.screen, "word-facts").textContent;
  assert.equal(named(), "VEF");
  find(h.screen, "editor-full-link", (b) => b.dataset.key === "review-mark-vef").onclick();
  await flush();
  h.send({ request: h.asked.at(-1).n, page: pageOf({ groups: [{ key: "rest", label: "rest", hint: "", mods: three.filter((m) => m.library !== "vef") }] }) });
  await flush();
  assert.deepEqual(h.marks, [["vef", "functional"]]);
  const on = h.screen.querySelectorAll(".rv-tile").filter((b) => b.classList.contains("on")).map((b) => b.dataset.mod);
  assert.deepEqual(on, ["hats"], "the next tile is picked");
  assert.equal(named(), "Hats", "the details follow the picked tile");
  assert.ok(find(h.screen, "editor-full-link", (b) => b.dataset.key === "review-mark-hats"), "the mark button acts on it");
});

// VM 2026-11-22: Ideology, 36 seen (memes, looks, abilities: what is picked from) and none of
// its meet categories, sat under never seen. By progress puts it under picked from, some in
// use; a Mod with nothing seen stays under never seen, one only set up under only set up.
test("a mixed Mod seen only in what is picked from is not under never seen", async () => {
  const h = await setup();
  const opened = h.review.open();
  const meet = [
    mod("Ideology", "ide", { part: 0, size: 37, seen: 36, group: "seen" }),
    mod("Quiet", "quiet", { part: 0, size: 5, seen: 0, group: "never-seen" }),
    mod("Planned", "plan", { part: 0, size: 3, seen: 0, group: "configured" }),
  ];
  h.send({ request: h.asked.at(-1).n, page: pageOf({ groups: [{ key: "meet", label: "meet", hint: "", mods: meet }] }) });
  await opened;
  const under = {};
  for (const sec of h.screen.querySelectorAll(".rv-grp")) {
    const label = sec.querySelector("h3 .l").textContent;
    for (const t of sec.querySelectorAll(".rv-tile")) under[t.dataset.mod] = label;
  }
  assert.deepEqual(under, { ide: "picked from, some in use", quiet: "never seen in the saves", plan: "only set up" });
});

// 16 §2.11: the page's line says what the map shows, with the bars' colours beside it, not
// the counts the colonies card has ("1 colony, 1 save" read as a riddle, 2026-11-23).
test("the page's line has a legend of the bar colours", async () => {
  const h = await setup();
  const opened = h.review.open();
  const n = h.asked.at(-1).n;
  h.send({ request: n, page: pageOf({ lead: "how much of each Mod's content has appeared in the colony" }) });
  await opened;
  const legend = find(h.screen, "rv-legend");
  assert.ok(legend, "no legend");
  assert.deepEqual(legend.querySelectorAll(".k").map((k) => ["none", "some", "whole"].find((c) => k.classList.contains(c))), ["none", "some", "whole"]);
  assert.match(legend.textContent, /none yet.*some.*all/);
  assert.equal(find(h.screen, "lead").textContent, "how much of each Mod's content has appeared in the colony");
});
