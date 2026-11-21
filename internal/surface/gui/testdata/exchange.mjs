import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// The Import page (16 §2.18): the ways in, the order comparison, the bar over a result, no export, and drops.
async function setup(api = {}) {
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
  const m = await loadModules("./exchange.js", {
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
    "./separator.js": { wireSeparator() {} },
    "./api.js": { on() {} },
    "./membar.js": { splitBar: () => el("div", "sbar-row"), diffMarks: () => el("span", "dmarks") },
  }, { document, location: { search: "" }, URLSearchParams, navigator: {}, setTimeout: () => 0 });
  const went = [];
  m.initExchange({ selectedList: () => 1, renderWhere() {}, renderHints() {}, go: (p) => went.push(["go", p]), addWith: (t) => went.push(["addWith", t]), newList: (f) => went.push(["newList", f]), reload: async () => {}, resumeGuide() {} });
  return { m, screen, calls, said, listeners, S, body, went };
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
const row = (key, extra = {}) => ({ key, name: key.toUpperCase(), ...extra });

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

test("the page has no export; the left column lists the ways in, two of them open their own forms", async () => {
  const h = await setup();
  await h.m.exchange();
  await flush();
  assert.deepEqual(h.calls.map((c) => c[0]).filter((k) => /export|^Packs$/i.test(k)), []);
  assert.ok(!/export/i.test(h.screen.textContent), h.screen.textContent);
  assert.deepEqual(h.screen.querySelectorAll(".xc-dorow").map((b) => b.dataset.do), ["game", "list", "subs", "pack", "db"]);
  h.screen.querySelector('.xc-dorow[data-do="list"] .btn').onclick();
  h.screen.querySelector('.xc-dorow[data-do="subs"] .btn').onclick();
  assert.deepEqual(h.went, [["newList", "import"], ["go", "subscribe"]]);
  // empty, the right is the drop pane and says so
  assert.ok(h.screen.querySelector(".xc-preview").hidden);
  assert.match(h.screen.querySelector(".xc-empty").textContent, /Drop/);
});

test("reading the game's list shows it next to yours, in order, with what is removed, added and moved", async () => {
  const preview = { summary: "s", format: "ModsConfig.xml", total: 4, missing: [{ name: "Gone", url: "https://steamcommunity.com/sharedfiles/filedetails/?id=42" }], installed: [], replace: { added: 1, text: "1 added" }, merge: { text: "" },
    current: [row("core"), row("a"), row("b"), row("old")], incoming: [row("core"), row("b"), row("a"), row("gone", { missing: true })] };
  const h = await setup({ ImportPreviewFor: async () => preview });
  await h.m.exchange();
  h.screen.querySelector('.xc-dorow[data-do="game"] .btn').onclick();
  await flush(); await flush(); await flush();
  assert.deepEqual(h.calls.find((c) => c[0] === "ImportPreviewFor").slice(2), ["file", "C:/g/ModsConfig.xml"]);
  const counts = Object.fromEntries(h.screen.querySelectorAll(".ld-k").map((k) => [k.className.match(/k-(\w+)/)[1], k.querySelector(".num").textContent]));
  assert.deepEqual(counts, { gone: "1", added: "1", moved: "1", same: "2" });
  assert.deepEqual(h.screen.querySelectorAll(".ld-col.r .ld-row").map((r) => r.className.match(/s-(\w+)/)[1]), ["same", "shift", "moved", "added"]);
  assert.ok(h.screen.querySelector(".ld-col.r .ld-row.miss"), "the Mod not installed is marked");
  assert.ok(h.screen.querySelector('.xc-dorow[data-do="game"]').classList.contains("on"));
  // the game's list applies to yours as its filled button; what is not installed can be subscribed at once
  assert.match(h.screen.querySelector(".xc-acts .btn.primary").textContent, /apply to Main/);
  assert.match(h.screen.querySelector(".xc-miss").textContent, /subscribe all 1/);
});

test("a result puts its counts and buttons in the bar over it, with Archo; empty, there is no bar", async () => {
  const preview = { summary: "s", format: "ModsConfig.xml", total: 1, missing: [], installed: [], replace: { added: 1 }, merge: {}, current: [row("a")], incoming: [row("b")] };
  const h = await setup({ ImportPreviewFor: async () => preview });
  await h.m.exchange();
  assert.equal(h.screen.querySelector(".xc-lay").dataset.showing, undefined);
  h.screen.querySelector('.xc-dorow[data-do="game"] .btn').onclick();
  await flush(); await flush(); await flush();
  assert.equal(h.screen.querySelector(".xc-lay").dataset.showing, "list");
  assert.ok(h.screen.querySelector(".xc-main .xc-bar .ld-legend"));
  assert.ok(h.screen.querySelector(".xc-bar .xc-acts"));
  assert.ok(!h.screen.querySelector(".xc-preview .xc-acts"));
  assert.ok(h.screen.querySelector(".xc-preview .ld-grid"));
});

// a manager with a list and notes, imported before, so its groups open with it
const MGR = () => [{ name: "RimSort", imported: 1, groups: [
  { kind: "lists", path: "C:/l", parts: [{ id: "L1", state: "new", default: true, name: "Medieval run", list: { mods: 2 } }] },
  { kind: "player", path: "C:/aux.db", parts: [{ id: "N1", part: "notes", state: "new", default: true, new: 2, same: 1 }] }] }];

test("one row is shown at a time: reading the game's list drops the manager picked, and back", async () => {
  const preview = { summary: "s", format: "ModsConfig.xml", total: 1, missing: [], installed: [], replace: { added: 1 }, merge: {}, current: [row("a")], incoming: [row("b")] };
  const h = await setup({ ImportSurvey: async () => MGR(), ImportPreviewFor: async () => preview });
  await h.m.exchange();
  await flush(); await flush();
  const card = () => h.screen.querySelector(".xc-mrow");
  const game = () => h.screen.querySelector('.xc-dorow[data-do="game"]');
  card().onclick();
  assert.ok(card().classList.contains("on"));
  game().querySelector(".btn").onclick();
  await flush(); await flush(); await flush();
  assert.ok(game().classList.contains("on"));
  assert.ok(!card().classList.contains("on"), "the manager is no longer picked");
  card().onclick();
  assert.ok(card().classList.contains("on"));
  assert.ok(!game().classList.contains("on"));
});

test("view shows what a part holds without bringing it over; back returns to the tree as it was", async () => {
  const preview = { summary: "s", format: "RimSort list", total: 2, missing: [], installed: [], replace: { added: 1 }, merge: {}, current: [row("a")], incoming: [row("a"), row("b")] };
  const views = { N1: { items: [{ part: "notes", mod: "Alpha", value: "a note", had: false }, { part: "notes", mod: "Beta", value: "old", had: true }] }, L1: { list: "C:/l/Medieval run.xml", items: [] } };
  const h = await setup({ ImportSurvey: async () => MGR(), ManagerPartView: async (id) => views[id], ImportPreviewFor: async () => preview });
  await h.m.exchange();
  await flush(); await flush();
  h.screen.querySelector(".xc-mrow").onclick();
  const viewOf = (label) => h.screen.querySelectorAll(".xc-tn.lv2").find((r) => r.textContent.includes(label)).querySelector(".xc-view");
  viewOf("notes").onclick();
  await flush(); await flush();
  assert.deepEqual(h.calls.find((c) => c[0] === "ManagerPartView").slice(1), ["N1"]);
  assert.deepEqual(h.screen.querySelectorAll(".xc-item").map((r) => r.textContent), ["Alphaa note", "Betaold"]);
  assert.ok(h.screen.querySelector(".xc-item.had"));
  assert.ok(!h.calls.some((c) => c[0] === "StartMigration"));
  const back = () => h.screen.querySelectorAll(".xc-acts .btn").find((b) => b.textContent === "back");
  back().onclick();
  assert.ok(h.screen.querySelector(".xc-tree"));
  assert.ok(h.screen.querySelector(".xc-mrow").classList.contains("on"), "the picks are kept");
  // a list is compared as any list, with only a way back
  viewOf("Medieval run").onclick();
  await flush(); await flush(); await flush(); await flush();
  assert.deepEqual(h.calls.find((c) => c[0] === "ImportPreviewFor").slice(2), ["file", "C:/l/Medieval run.xml"]);
  assert.ok(h.screen.querySelector(".ld-grid"));
  assert.deepEqual(h.screen.querySelectorAll(".xc-acts .btn").map((b) => b.textContent), ["back"]);
  assert.ok(h.screen.querySelector(".xc-mrow").classList.contains("on"));
  back().onclick();
  assert.ok(h.screen.querySelector(".xc-tree"));
});

test("orderDiff keeps the longest common order and marks the rest as moved, in blocks", async () => {
  const m = await loadModules("./listdiff.js", { "./state.js": { el: (t, c = "", x = "") => new Element(t, c, x), T: (s) => s, Tf: (s) => s, factText: (e) => e }, "./icons.js": { icon: {} } });
  const keys = (s) => s.split("").map((k) => ({ key: k, name: k }));
  const d = m.orderDiff(keys("abcdefghijz"), keys("abghcdefixj"));
  assert.equal(d.right.map((r) => r.key + ":" + r.st).join(" "), "a:same b:same g:moved h:moved c:shift d:shift e:shift f:shift i:same x:added j:shift");
  assert.equal(d.right[2].block, d.right[3].block, "g and h move together");
  assert.equal(d.left.at(-1).st, "gone");
  assert.deepEqual([d.counts.gone, d.counts.added, d.counts.moved, d.counts.same], [1, 1, 2, 8]);
  // a long unchanged run folds, keeping one row at its head; the end of a list keeps none
  const long = keys("abcdefghij");
  const f = m.folded(m.orderDiff(long, long).left, "l", true, new Set());
  assert.equal(f.map((x) => x.fold ? "~" + x.fold : x.row.key).join(" "), "a ~9");
  assert.equal(m.folded(m.orderDiff(long, long).left, "l", true, new Set(["l0"])).length, 10, "an opened run stays open");
});

test("an unknown dropped file is refused on the right, not ignored", async () => {
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

test("a dropped list is read and compared", async () => {
  const h = await setup({ ImportPreviewFor: async () => ({ summary: "s", total: 0, missing: [], installed: [], replace: { same: true }, merge: {}, current: [], incoming: [] }) });
  await h.m.exchange();
  await h.listeners.drop(fileDrop("Medieval run.xml", { text: "<ModsConfigData/>" }));
  await flush();
  const call = h.calls.find((c) => c[0] === "ImportPreviewFor");
  assert.deepEqual(call.slice(2), ["text", "<ModsConfigData/>"]);
  assert.equal(h.screen.querySelector(".xc-preview").dataset.mode, "list");
});

test("a dropped Mod folder or zip, or a link, goes to add a subscription to be confirmed there", async () => {
  const kinds = { "D:/Downloads/Colony Tweaks": "mod", "D:/RimSort": "folder" };
  const h = await setup({ DropFolderKind: async (p) => kinds[p] });
  await h.m.dropped({ name: "Colony Tweaks.zip", path: "D:/Downloads/Colony Tweaks.zip" });
  await h.m.dropped({ name: "Colony Tweaks", path: "D:/Downloads/Colony Tweaks", dir: true });
  await h.m.dropped({ text: "https://github.com/Example/colony" });
  await h.m.dropped({ text: "not a link" });
  assert.deepEqual(h.went, [["addWith", "D:/Downloads/Colony Tweaks.zip"], ["addWith", "D:/Downloads/Colony Tweaks"], ["addWith", "https://github.com/Example/colony"]]);
  assert.ok(!h.calls.some((c) => c[0] === "Subscribe"), "nothing is installed or subscribed without the player's yes");
  // a folder with no Mod in it is another manager's data, on the Import page
  await h.m.dropped({ name: "RimSort", path: "D:/RimSort", dir: true });
  assert.equal(h.went.length, 3);
  assert.equal(h.S.screen, "plan-import");
  // a browser drop gives no path: a zip is answered on the Import page
  await h.listeners.drop(fileDrop("Colony.zip"));
  assert.equal(h.screen.querySelector(".xc-preview").dataset.mode, "refused");
  assert.match(h.screen.querySelector(".xc-preview").textContent, /choose zip/);
  // a link dragged from a browser
  await h.listeners.drop({ dataTransfer: { types: ["text/uri-list"], items: [], files: [], getData: (t) => t === "text/uri-list" ? "# comment\r\nhttps://www.loverslab.com/files/file/1-x/" : "" }, defaultPrevented: false, preventDefault() {} });
  assert.deepEqual(h.went.at(-1), ["addWith", "https://www.loverslab.com/files/file/1-x/"]);
});

test("a dropped pack or folder without a path asks for the button; with a path it is added", async () => {
  const h = await setup({ PackUpdateOffer: async () => null, StartPackImport: async () => 7, TaskByID: async () => ({ state: "run" }) });
  await h.m.exchange();
  await h.listeners.drop(fileDrop("Medieval.archopack"));
  assert.equal(h.screen.querySelector(".xc-preview").dataset.mode, "refused");
  assert.ok(!h.calls.some((c) => c[0] === "StartPackImport"));
  await h.listeners.drop(fileDrop("RimSort", { dir: true }));
  assert.match(h.screen.querySelector(".xc-preview").textContent, /add…/);
  await h.m.dropped({ name: "Medieval.archopack", path: "D:/Packs/Medieval.archopack" });
  await flush();
  assert.deepEqual(h.calls.find((c) => c[0] === "StartPackImport").slice(1), ["D:/Packs/Medieval.archopack", false]);
});

test("a dropped folder with no manager's data in it is refused, not left silent", async () => {
  const h = await setup({ ManagerFileKind: async () => "folder", ImportSurvey: async () => [] });
  await h.m.exchange();
  await h.m.dropped({ name: "Screenshots", dir: true, path: "D:/Screenshots" });
  await flush();
  const res = h.screen.querySelector(".xc-preview");
  assert.equal(res.dataset.mode, "refused");
  assert.match(res.textContent, /no RimSort, RimCrow or RimPy data/);
});

test("a drop a form already took is left to it", async () => {
  const h = await setup();
  await h.m.exchange();
  const e = fileDrop("notes.docx"); e.defaultPrevented = true;
  await h.listeners.drop(e);
  assert.ok(h.screen.querySelector(".xc-preview").hidden);
});
