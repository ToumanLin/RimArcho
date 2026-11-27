import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules, Element, flush } from "./dom.mjs";

// Settings › About (16 §2.14): the changelog's Markdown is drawn from text nodes only, its
// links open in the browser, and a folded version reads its text when it opens.
async function setup() {
  const asked = [], opened = [];
  const el = (tag, cls = "", text) => { const e = new Element(tag, cls); if (text != null) e.append(String(text)); return e; };
  const document = { createElement: (t) => new Element(t), createTextNode: (t) => new Element("#text", "", String(t)) };
  const deps = { "./state.js": { el, factText: () => {}, T: (s) => s, api: { ChangelogText: async (k) => { asked.push(k); return "- **" + k + "** text"; } } } };
  const mod = await loadModules("./about.js", deps, { document, queueMicrotask });
  return { mod, asked, opened, open: (u) => opened.push(u) };
}
const walk = (n) => [n, ...n.children.flatMap(walk)];

test("bold, code, emphasis, links and nested bullets", async () => {
  const { mod } = await setup();
  const b = mod.parseMarkdown("### Breaking\n- **Bold** and `code` and *em*, see [spec](docs/specs/a.md)\n  continued\n  - nested\n- second\n\nA paragraph\nline two");
  assert.equal(b[0].t, "h");
  assert.equal(b[1].t, "ul");
  assert.equal(b[1].items.length, 2);
  assert.deepEqual([...b[1].items[0].inl.map(r => r.t)], ["b", "text", "code", "text", "em", "text", "link", "text"]);
  assert.equal(b[1].items[0].inl.at(-1).text, " continued");
  assert.equal(b[1].items[0].sub.items[0].inl[0].text, "nested");
  assert.equal(b[2].t, "p");
  assert.equal(b[2].inl[0].text, "A paragraph line two");
});

test("HTML in the text stays text", async () => {
  const { mod, open, opened } = await setup();
  const box = mod.renderMarkdown('- <img src=x onerror="alert(1)"> [x](javascript:alert(1)) [gh](https://github.com/a) [doc](docs/b.md)', { source: "https://github.com/o/r/blob/master/", open });
  const nodes = walk(box);
  assert.ok(!nodes.some(n => n.tag === "img"));
  assert.ok(box.textContent.includes('<img src=x onerror="alert(1)">'));
  const links = nodes.filter(n => n.tag === "a");
  assert.deepEqual([...links.map(a => a.href)], ["https://github.com/a", "https://github.com/o/r/blob/master/docs/b.md"]);
  links[0].onclick({ preventDefault() {}, stopPropagation() {} });
  assert.deepEqual(opened, ["https://github.com/a"]);
});

test("the current version is open; another reads its text once, when opened; older ones wait behind a fold", async () => {
  const { mod, asked, open } = await setup();
  const c = { shown: 2, source: "", versions: [{ key: "Unreleased", title: "unreleased", current: true, text: "- now" }, { key: "1.1.4", title: "1.1.4", note: "2026-11-25" }, { key: "1.1.3", title: "1.1.3" }] };
  const box = mod.changelogView(c, { open });
  await flush();
  const folds = box.children;
  assert.equal(folds.length, 3);
  assert.ok(folds[0].classList.contains("open"));
  assert.ok(folds[0].textContent.includes("now"));
  assert.ok(!folds[1].classList.contains("open"));
  assert.deepEqual(asked, []);
  const toggle = folds[1].children[0].children[0];
  toggle.onclick({ stopPropagation() {} });
  await flush();
  assert.deepEqual(asked, ["1.1.4"]);
  toggle.onclick({ stopPropagation() {} });
  toggle.onclick({ stopPropagation() {} });
  await flush();
  assert.deepEqual(asked, ["1.1.4"]);
  // "older versions": 1.1.3 is drawn only once the fold opens
  assert.ok(!folds[2].textContent.includes("1.1.3"));
  folds[2].children[0].children[0].onclick({ stopPropagation() {} });
  await flush();
  assert.ok(folds[2].textContent.includes("1.1.3"));
});

test("a bullet shows its lead: the bold phrase, else the first sentence; more shows it whole", async () => {
  const { mod } = await setup();
  const lead = (s) => { const l = mod.bulletLead(mod.inline(s)); return l && l.map(r => r.text ?? r.kids.map(k => k.text).join("")).join(""); };
  assert.equal(lead("**Settings in seven groups.** The list is down"), "Settings in seven groups.");
  assert.equal(lead("Fixed: `a; b` broke. Now it works"), "Fixed: a; b broke.");
  assert.equal(lead("CLI / MCP: `archo x` adds this; and that"), "CLI / MCP: archo x adds this");
  assert.equal(lead("Only one sentence."), null);
  assert.equal(lead('It reads "a = b; c" now. Then more'), 'It reads "a = b; c" now.');
  assert.equal(lead("**Bold** only"), "Bold");
  const box = mod.renderMarkdown("- **Lead.** Rest with `code`.\n- Short one.", { leads: true });
  const [first, second] = box.children[0].children;
  const [short, full, more] = first.children;
  assert.equal(short.textContent, "Lead.");
  assert.ok(full.hidden && !short.hidden);
  more.onclick({ stopPropagation() {} });
  assert.ok(!full.hidden && short.hidden);
  assert.equal(full.textContent, "Lead. Rest with code.");
  assert.equal(second.textContent, "Short one.");
});
