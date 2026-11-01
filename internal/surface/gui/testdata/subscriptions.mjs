import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event } from "./dom.mjs";

async function setup(overrides = {}) {
  const doc = new Element("html"), screen = new Element("div"), calls = [], opened = [], confirms = [];
  screen.id = "screen"; doc.append(screen);
  doc.dispatchEvent = e => opened.push(e.detail);
  const el = (tag, cls = "", text = "") => {
    const n = new Element(tag, cls, text);
    n.focus = () => { doc.activeElement = n; n.onfocus?.(); };
    return n;
  };
  const data = {
    summary: { line: "checked 4 sources, the last at 10:42, 1 check failed", failures: 1 },
    sources: [
      { key: "workshop", name: "Steam Workshop", count: 2, checked: "10:42", result: "1 check failed", failures: [{ label: "timed out", items: ["Network Mod: timed out"] }] },
      { key: "steamcmd", name: "SteamCMD", count: 1, result: "everything is up to date", account: "not logged in" },
      { key: "git", name: "Git", count: 1, result: "everything is up to date" },
      { key: "page", name: "LoversLab", count: 1, result: "everything is up to date", account: "logged in" },
      { key: "local", name: "Local", count: 0, result: "not checked yet" },
    ],
    pending: [{ id: "pending-copy", ref: "#pending", name: "Same name", kind: "git", source: "Git", installed: "old", available: "new", inPlan: true }],
    gone: [{ id: "gone-copy", ref: "#gone", name: "Same name", inPlan: true }, { id: "other-copy", ref: "#other", name: "Other list", inPlan: false }],
    history: { rows: [], arrivals: { text: "1 new mod: Same name", rows: [{ id: "arrival-copy", mod: "Same name", text: "found in the Mods folder", present: true }, { id: "removed", mod: "Gone local", text: "removed from the Mods folder" }] }, older: [] },
    ...overrides,
  };
  const S = { screen: "changes", screenSeq: 1 };
  const api = {
    SubscriptionsPage: async () => { calls.push("read"); return data; },
    CheckSubscriptionSource: async key => { calls.push(key); return "queued"; },
    LibraryPreview: async (code, refs) => { calls.push([code, ...refs]); return { apply: true, lines: ["dry run"] }; },
  };
  const button = (label, key, fn, cls = "") => {
    const b = el("button", "btn " + cls, label); if (key) b.append(el("kbd", "", key));
    b.onclick = () => fn(); return b;
  };
  const deps = {
    "./state.js": { S, $: s => doc.querySelector(s), el, api, T: s => s, Tf: (s, ...a) => { let i = 0; return s.replace(/%[sd]/g, () => a[i++]); }, say() {}, errText: e => e.message },
    "./fields.js": { pathContent() {} }, "./popups.js": { confirm: (...a) => confirms.push(a) },
    "./icons.js": { icon: {} }, "./separator.js": { wireSeparator() {} },
    "./subscribe.js": { addForm() {}, pageUpdate() {} },
  };
  const mod = await loadModules("./subscriptions.js", deps, { document: doc, CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init.detail; } }, CSS: { escape: s => s } });
  mod.initSubscriptions({ renderHints() {}, renderWhere() {} }, {
    beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }),
    page: () => el("div"), area() {}, button, act: label => el("button", "", label), seg: () => el("div"),
  });
  await mod.changes();
  const row = key => screen.querySelector(`[data-key="${key}"]`);
  const key = (key, target = screen.querySelector(".subs-list")) => mod.changesKey(event(key, { target }));
  return { screen, calls, opened, confirms, row, key, mod };
}

test("source summary is an arrow-selectable item with full detail and no inline panel or warning", async () => {
  const h = await setup();
  h.key("End");
  assert.equal(h.row("sources").classList.contains("sel"), true);
  const detail = h.screen.querySelector(".subs-detail");
  assert.equal(detail.querySelector("h2").textContent, "Update checks");
  assert.equal(detail.querySelectorAll(".subs-src").length, 5);
  assert.match(detail.textContent, /Network Mod: timed out/);
  assert.equal(detail.querySelectorAll(".acct").length, 2);
  assert.equal(h.screen.querySelector(".subs-list .subs-sources"), null);
  assert.equal(h.screen.querySelector(".subs-failed"), null);
  assert.deepEqual(h.calls, ["read"], "navigation must not check or reread a source");
  await detail.querySelectorAll(".subs-src")[0].querySelector("button").onclick();
  assert.deepEqual(h.calls, ["read", "workshop"]);
});

test("source checks stay reachable when history and pending lists are empty", async () => {
  const h = await setup({ pending: [], gone: [], history: { rows: [], older: [], noScan: true } });
  assert.equal(h.screen.querySelector(".subs-detail h2").textContent, "Update checks");
  h.row("sources").onclick();
  assert.deepEqual(h.calls, ["read"]);
});

test("removed copies get their own section, scope, explanation and exact-copy actions", async () => {
  const h = await setup();
  assert.match(h.screen.textContent, /won't update any more/);
  assert.equal(h.row("gone:other-copy"), null, "other-list copy stays out of current-list scope");
  h.row("gone:gone-copy").onclick();
  const detail = h.screen.querySelector(".subs-detail");
  assert.equal(detail.querySelectorAll("p").length, 2);
  assert.match(detail.textContent, /cannot download it again/);
  h.key("l"); assert.deepEqual(h.opened, ["gone-copy"]);
  await h.key("p");
  assert.deepEqual(h.calls.at(-1), ["pin", "#gone"]);
  assert.equal(h.confirms.length, 1, "locking previews before writing");
  assert.ok(h.mod.changesHints().some(x => x.key === "l"));
});

test("arrival details explain changes and link only installed copies; pending uses the same helper", async () => {
  const h = await setup();
  h.row("g:now").onclick();
  const detail = h.screen.querySelector(".subs-detail");
  assert.equal(detail.querySelector("h2").textContent, "1 new mod: Same name");
  assert.equal(h.row("g:now").querySelector(".ri").textContent, "");
  const links = detail.querySelectorAll("button");
  assert.equal(links.length, 1);
  assert.match(detail.textContent, /found in the Mods folder/);
  links[0].onclick(); assert.deepEqual(h.opened, ["arrival-copy"]);
  h.row("p:#pending").onclick(); h.key("l");
  assert.deepEqual(h.opened, ["arrival-copy", "pending-copy"]);
});
