import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { SourceTextModule, SyntheticModule, createContext } from "node:vm";

// A small DOM stand-in keeps the regression test independent of a browser installation.
class Element {
  children = []; dataset = {}; style = {}; attributes = {}; text = ""; disabled = false;
  constructor(tag, className = "", text = "") { Object.assign(this, { tag, className, text }); }
  append(...nodes) { for (const n of nodes) { n.parent = this; this.children.push(n); } }
  set innerHTML(_) { this.children = []; this.text = ""; }
  set textContent(t) { this.text = t; this.children = []; }
  get textContent() { return this.text + this.children.map((n) => n.textContent).join(" "); }
  setAttribute(k, v) { this.attributes[k] = v; }
  focus() {}
  after(n) { this.parent.children.splice(this.parent.children.indexOf(this) + 1, 0, n); n.parent = this.parent; }
  replaceWith(n) { this.parent.children.splice(this.parent.children.indexOf(this), 1, n); n.parent = this.parent; }
  isEqualNode(n) {
    return ["tag", "className", "text", "disabled", "hidden"].every((k) => this[k] === n[k]) &&
      JSON.stringify(this.dataset) === JSON.stringify(n.dataset) && JSON.stringify(this.attributes) === JSON.stringify(n.attributes) &&
      this.children.length === n.children.length && this.children.every((child, i) => child.isEqualNode(n.children[i]));
  }
  matches(s) {
    if (s.startsWith(".")) return s.slice(1).split(".").every((c) => this.className.split(" ").includes(c));
    const attr = s.match(/^\[data-([\w-]+)(?:="([^"]*)")?\]$/);
    if (attr) {
      const key = attr[1].replace(/-([a-z])/g, (_, c) => c.toUpperCase());
      return key in this.dataset && (attr[2] === undefined || this.dataset[key] === attr[2]);
    }
    return this.tag === s;
  }
  querySelectorAll(s) {
    const parts = s.split(" "), last = parts.pop();
    const all = this.children.flatMap((n) => [n, ...n.querySelectorAll("*")]);
    return all.filter((n) => (last === "*" || n.matches(last)) && (!parts.length || n.parent.matches(parts.at(-1))));
  }
  querySelector(s) { return this.querySelectorAll(s)[0] || null; }
}

const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; };
const flush = () => new Promise(setImmediate);
const result = (request, loading, name = "pending") => ({
  request, loading, scope: "plan", scale: 100, scales: [100, 50], labels: ["full size", "50%"],
  mods: 1, tool: "todds", toolOk: true, png: name, estFacts: [name], convert: 1,
  rows: [{ ref: "mod-a", name, pending: loading, png: name, estimate: name, convert: 1 }],
});

async function setup() {
  const screen = new Element("div"), timers = [], requests = [], polls = [], messages = [];
  const S = { screen: "textures", screenSeq: 1 };
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const $ = (sel) => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen /, ""));
  const api = {
    Textures: (...args) => { const d = deferred(); requests.push({ ...d, args }); return d.promise; },
    TextureMeasure: (id) => { const d = deferred(); polls.push({ ...d, id }); return d.promise; },
  };
  const deps = {
    "./state.js": { S, $, el, api, T: (s) => s, Tf: (s, ...args) => { let i = 0; return s.replace(/%[sd]/g, () => args[i++]); }, errText: (e) => e.message, say: (...args) => messages.push(args) },
    "./screens.js": { openScreen: () => { screen.innerHTML = ""; }, page: (cls) => el("div", cls), button: (label, _, run, cls) => Object.assign(el("button", cls, label), { onclick: run }) },
    "./popups.js": { confirm: () => {} },
  };
  const context = createContext({ document: { activeElement: null }, setTimeout: (f) => timers.push(f) });
  const source = await readFile(new URL("../assets/js/textures.js", import.meta.url), "utf8");
  const module = new SourceTextModule(source, { context });
  await module.link((name) => new SyntheticModule(Object.keys(deps[name]), function () {
    for (const [key, value] of Object.entries(deps[name])) this.setExport(key, value);
  }, { context }));
  await module.evaluate();
  return { screen, timers, requests, polls, messages, S, tex: module.namespace };
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
