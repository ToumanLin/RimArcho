import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, deferred, flush, event } from "./dom.mjs";

const result = (request, loading, name = "pending") => ({
  request, loading, scope: "plan", scale: 100, scales: [100, 50], labels: ["full size", "50%"],
  mods: 1, tool: "todds", toolOk: true, png: name, estFacts: [name], convert: 1,
  rows: [{ ref: "mod-a", name, pending: loading, png: name, estimate: name, convert: 1 }],
});

async function setup() {
  const screen = new Element("div"), timers = [], requests = [], polls = [], messages = [];
  const S = { screen: "textures", screenSeq: 1 };
  let created = 0;
  const el = (tag, cls, text) => { created++; return new Element(tag, cls, text); };
  const $ = (sel) => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen /, ""));
  const api = {
    Setting: async () => "", SetSetting: async () => {},
    Textures: (...args) => { const d = deferred(); requests.push({ ...d, args }); return d.promise; },
    TextureMeasure: (id) => { const d = deferred(); polls.push({ ...d, id }); return d.promise; },
  };
  let readSeq = 0;
  const beginPageRead = (name) => {
    S.screen = name;
    const seq = ++readSeq, epoch = S.screenSeq;
    const current = () => seq === readSeq && S.screen === name && S.screenSeq === epoch;
    return { current, finish() {}, fail(e) { if (current()) messages.push(["fail", e.message]); } };
  };
  const deps = {
    "./state.js": { S, $, el, api, T: (s) => s, Tf: (s, ...args) => { let i = 0; return s.replace(/%[sd]/g, () => args[i++]); }, errText: (e) => e.message, say: (...args) => messages.push(args) },
    "./screens.js": { beginPageRead, pageLoading: () => false, page: (cls) => el("div", cls), button: (label, _, run, cls) => Object.assign(el("button", cls, label), { onclick: run }) },
    "./popups.js": { confirm: () => {} },
  };
  const document = { activeElement: null, body: new Element("body") };
  const tex = await loadModules("./textures.js", deps, { document, setTimeout: (f) => timers.push(f) });
  return { screen, timers, requests, polls, messages, S, tex, created: () => created };

}

for (const reject of [false, true]) test(`late ${reject ? "error" : "result"} cannot replace newer options`, async () => {
  const h = await setup();
  h.tex.open();
  h.requests[0].resolve(result(1, true));
  await flush();
  const scale = h.screen.querySelector('[data-f="scale"]');
  assert.equal(scale.disabled, false, "scale remains usable during measurement");
  assert.ok(h.screen.querySelector(".tex-pending"), "unmeasured values shimmer");
  h.timers.shift()();
  await flush();
  assert.equal(h.polls[0].id, 1);
  scale.value = "50"; scale.onchange();
  h.requests[1].resolve(result(2, false, "new estimate"));
  await flush();
  if (reject) h.polls[0].reject(new Error("old failure"));
  else h.polls[0].resolve(result(1, false, "old estimate"));
  await flush();
  assert.ok(h.screen.textContent.includes("new estimate"));
  assert.ok(!h.screen.textContent.includes("old estimate"));
  assert.equal(h.messages.length, 0, "old errors are discarded too");
  assert.equal(h.requests[1].args[1], 50);
});

test("row updates preserve selects and conversion progress; leaving discards replies", async () => {
  const h = await setup();
  h.tex.open(); h.requests[0].resolve(result(1, true));
  await flush();
  const scope = h.screen.querySelector('[data-f="scope"]');
  h.tex.event({ text: "conversion progress", frac: 0.25 });
  h.timers.shift()(); await flush();
  const p = result(1, true, "measured row"); p.rows[0].pending = false;
  h.polls[0].resolve(p); await flush();
  assert.equal(h.screen.querySelector('[data-f="scope"]'), scope, "polling keeps live selects in place");
  assert.ok(h.screen.textContent.includes("measured row"));
  assert.ok(h.screen.textContent.includes("conversion progress"));
  h.timers.shift()(); await flush();
  h.S.screen = "home"; h.S.screenSeq++;
  const before = h.screen.textContent;
  h.polls[1].resolve(result(1, false, "abandoned estimate")); await flush();
  assert.equal(h.screen.textContent, before);
});

test("a conversion event arriving before the page reply keeps its progress", async () => {
  const h = await setup();
  h.tex.open();
  h.tex.event({ text: "new conversion progress", frac: 0.5 });
  h.requests[0].resolve(result(1, true)); await flush();
  assert.ok(h.screen.textContent.includes("new conversion progress"));
  assert.equal(h.screen.querySelector('[data-f="scale"]').disabled, true);
});

test("unchanged pending rows keep their shimmer while another row completes", async () => {
  const h = await setup();
  const p = result(1, true);
  p.rows.push({ ref: "mod-b", name: "second mod", pending: true });
  h.tex.open(); h.requests[0].resolve(p); await flush();
  const second = h.screen.querySelector('[data-ref="mod-b"]');
  const summary = h.screen.querySelector('[data-tex="estimate"]');
  h.timers.shift()(); await flush();
  const next = result(1, true);
  next.rows[0].pending = false; next.rows.push(p.rows[1]);
  h.polls[0].resolve(next); await flush();
  assert.equal(h.screen.querySelector('[data-ref="mod-b"]'), second);
  assert.equal(h.screen.querySelector('[data-tex="estimate"]'), summary);
});

test("measurement can reorder rows without interrupting a header drag or pending shimmer", async () => {
  const h = await setup();
  h.tex.open();
  assert.equal(h.tex.key(event("s")), false, "headers are unavailable before data arrives");
  const p = result(1, true, "Z pending");
  p.rows.push({ ref: "mod-b", name: "A pending", pending: true });
  h.requests[0].resolve(p); await flush();
  const shimmer = h.screen.querySelector('[data-ref="mod-a"]');
  const header = h.screen.querySelector(".lib-th").children[0];
  header.onpointerdown(event("", { target: header, clientX: 20, button: 0, pointerId: 1 }));
  header.onpointermove({ clientX: 190 });
  h.timers.shift()(); await flush();
  const next = result(1, true, "Z pending");
  next.rows.push({ ref: "mod-b", name: "B completed", pending: false, savedBytes: 100 });
  h.polls[0].resolve(next); await flush();
  assert.equal(h.screen.querySelector('[data-ref="mod-a"]'), shimmer);
  assert.equal(h.screen.querySelector(".lib-th").children[0], header);
  assert.equal(typeof header.onpointerup, "function", "drag retains pointer capture target");
  header.onpointerup();
});

test("450 rows arrive in batches and unchanged polls create no DOM", async () => {
  const h = await setup();
  const p = result(1, true);
  p.rows = Array.from({ length: 450 }, (_, i) => ({ ref: `mod-${i}`, name: `Mod ${i}`, pending: true }));
  h.tex.open(); h.requests[0].resolve(p); await flush();
  const table = h.screen.querySelector('.tex-rows');
  const unchanged = h.screen.querySelector('[data-ref="mod-449"]');
  h.timers.shift()(); await flush();
  let before = h.created();
  h.polls[0].resolve(p); await flush();
  assert.equal(h.created(), before, "an unchanged 100ms snapshot draws nothing");
  h.timers.shift()(); await flush();
  const next = { ...p, rows: p.rows.map((r, i) => i === 0 ? { ...r, pending: false, estimate: '32 MB' } : r) };
  before = h.created(); h.polls[1].resolve(next); await flush();
  assert.ok(h.created() - before < 160, "one completed mod must not rebuild 450 rows");
  assert.equal(h.screen.querySelector('.tex-rows'), table);
  assert.equal(h.screen.querySelector('[data-ref="mod-449"]'), unchanged);
  h.timers.shift()(); await flush();
  const done = { ...next, loading: false, rows: next.rows.map(r => ({ ...r, pending: false, estimate: '32 MB' })) };
  h.polls[2].resolve(done); await flush();
  assert.equal(h.screen.querySelectorAll('.data-row').length, 450, "a single poll applies the whole batch");
  assert.equal(h.screen.querySelectorAll('.tex-rows .tex-pending').length, 0);
});
