import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, deferred, flush } from "./dom.mjs";

async function setup() {
  const saved = new Map(), menus = [], screen = new Element("div"), S = { screen: "packs", screenSeq: 1 };
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const api = { Setting: async (k) => saved.get(k) || "", SetSetting: async (k, v) => { saved.set(k, v); } };
  const document = { activeElement: null, body: new Element("body"), querySelector: (s) => $(s) };
  const $ = (s) => s === "#screen" ? screen : screen.querySelector(s);
  const deps = {
    "./state.js": { S, $, el, api, T: (s) => s, Tf: (s, ...args) => { let i = 0; return s.replace(/%[sd]/g, () => args[i++]); }, errText: (e) => e.message, say() {} },
    "./popups.js": { menu: (...args) => menus.push(args), text() { throw Error("preview must stay inline"); } },
    "./motion.js": { shimmer: () => el("span", "skel") },
    "./screens.js": {
      page: (cls) => el("div", cls), area() {},
      button: (label, _, run, cls) => Object.assign(el("button", cls, label), { onclick: run }),
      act: (label, _, run) => Object.assign(el("button", "", label), { onclick: run }),
      openScreen: () => screen,
      beginPageRead: (name) => { S.screen = name; return { current: () => S.screen === name, finish() {}, fail(e) { throw e; } }; },
    },
  };
  const globals = { document, setTimeout, clearTimeout };
  const table = await loadModules("./datatable.js", deps, globals);
  const defs = await loadModules("./tabledefs.js", deps, globals);
  return { ...table, ...defs, api, saved, menus, screen, S, el, deps, globals };
}

test("all three tables sort raw values in their natural direction, then reverse", async () => {
  const h = await setup();
  for (const [columns, sort, rows, expected] of [
    [h.packColumns, "played", [{ name: "Z", playedAt: "2026-01-02", bytes: 9 }, { name: "A", playedAt: "2026-01-01", bytes: 100 }, { name: "N", playedAt: "0001-01-01T00:00:00Z", bytes: 2 }], ["Z", "A", "N"]],
    [h.packModColumns, "order", [{ name: "Z", order: 2 }, { name: "A", order: 10 }, { name: "N", order: 1 }], ["N", "Z", "A"]],
    [h.textureColumns, "estimate", [{ name: "Z", savedBytes: 9 }, { name: "A", savedBytes: 100 }, { name: "N", pending: true, savedBytes: 200 }], ["A", "Z", "N"]],
  ]) {
    const state = { cols: null, sort, dir: 1 };
    const t = h.createTable({ key: "test", columns, state, redraw() {} });
    const names = () => Array.from(t.sorted(rows), (r) => r.name);
    assert.deepEqual(names(), expected);
    t.sortBy(sort);
    assert.deepEqual(names(), sort === "estimate" ? ["Z", "A", "N"] : [...expected].reverse());
    t.sortBy("name"); assert.deepEqual(names(), sort === "estimate" ? ["A", "Z", "N"] : ["A", "N", "Z"]);
    t.sortBy("name"); assert.deepEqual(names(), sort === "estimate" ? ["Z", "A", "N"] : ["Z", "N", "A"]);
    for (const col of columns) {
      t.sortBy(col.key);
      const box = h.el("section"), head = h.el("div"); box.append(head); t.draw(box, head);
      const active = head.querySelector(".th.on");
      assert.ok(active);
      assert.equal(active.attributes["aria-sort"], state.dir * col.dir > 0 ? "ascending" : "descending");
    }
  }
  const t = h.createTable({ key: "test", columns: h.packColumns, state: { cols: null, sort: "size", dir: 1 }, redraw() {} });
  assert.deepEqual(Array.from(t.sorted([{ name: "GB", bytes: 1000000000, size: "1 GB" }, { name: "MB", bytes: 900000000, size: "900 MB" }]), (r) => r.name), ["GB", "MB"]);
});

test("columns round trip separately for every table; menus and keyboard reach every gesture", async () => {
  const h = await setup();
  for (const [name, columns] of [["packs", h.packColumns], ["pack.mods", h.packModColumns], ["textures", h.textureColumns]]) {
    const key = `ui.gui.${name}.columns`, box = h.el("section"), head = h.el("div"); box.append(head);
    let draws = 0;
    const t = h.createTable({ key, columns, redraw() { draws++; t.draw(box, head); } });
    await t.load(); t.draw(box, head);
    const extra = columns[1].key;
    t.move("name", extra, true);
    t.key(event(">")); t.key(event("s"));
    const menu = h.menus.at(-1);
    assert.equal(menu[3], true);
    const find = (s) => menu[2].find((a) => a.label === s);
    assert.ok(find("sort by " + columns[1].title));
    assert.ok(find("move " + columns[0].title + " left"));
    find(columns[1].title).run();
    assert.ok(!t.visible().find((c) => c.key === extra));
    assert.ok(t.visible().find((c) => c.key === "name"));
    const again = h.createTable({ key, columns, redraw() {} }); await again.load();
    assert.equal(again.state.cols[0].key, extra);
    assert.equal(again.state.cols.find((c) => c.key === extra).shown, false);
    assert.equal(again.state.cols.find((c) => c.key === "name").w, columns[0].w + 24);
    t.key(event("=")); assert.equal(t.state.cols.find((c) => c.key === "name").w, columns[0].w);
    t.key(event("<")); assert.equal(t.state.cols.find((c) => c.key === "name").w, columns[0].w - 24);
    head.children[0].oncontextmenu(event("", { clientX: 10, clientY: 20 }));
    assert.equal(h.menus.at(-1)[3], false);
    h.menus.at(-1)[2].find((a) => a.label === "reset the columns").run();
    assert.deepEqual(JSON.parse(h.saved.get(key)).map((c) => c.key), Array.from(columns, (c) => c.key));
    const cell = head.children[0], edge = cell.querySelector(".rz");
    cell.onpointerdown(event("", { button: 0, target: edge, clientX: 10, pointerId: 1 }));
    assert.equal(edge.captured, 1, "capture stays on the edge so it receives double clicks");
    cell.onpointermove({ clientX: 90 }); cell.onpointerup();
    assert.equal(JSON.parse(h.saved.get(key))[0].w, columns[0].w + 80);
    edge.ondblclick(event("")); assert.equal(JSON.parse(h.saved.get(key))[0].w, columns[0].w);
    const drag = head.children[0];
    drag.onpointerdown(event("", { button: 0, target: drag, clientX: 20, pointerId: 1 }));
    drag.onpointermove({ clientX: 190 }); drag.onpointerup();
    assert.equal(JSON.parse(h.saved.get(key))[1].key, "name");
    assert.ok(draws >= 7);
  }
  assert.equal(h.saved.size, 3);
});

test("legacy column settings survive extraction; malformed settings cannot hide the name", async () => {
  const h = await setup(), defs = h.packColumns;
  for (const saved of ["null", "{}", "1", "bad", '[null,1,{"key":"unknown"}]']) {
    assert.equal(h.mergeColumns(defs, saved).length, defs.length);
  }
  const cols = h.mergeColumns(defs, '[{"k":"size","w":170,"h":1},{"k":"name","w":300,"h":1},{"k":"size"}]');
  assert.equal(cols[0].key, "size"); assert.equal(cols[0].shown, false); assert.equal(cols[0].w, 170);
  assert.equal(cols[1].shown, true); assert.equal(cols[1].w, 300); assert.equal(cols.length, defs.length);
  const tasks = h.mergeColumns(defs, '[{"key":"name","shown":false,"w":0},{"key":"size","shown":true,"w":180}]');
  assert.equal(tasks[0].shown, true); assert.equal(tasks[0].w, defs[0].w); assert.equal(tasks[1].w, 180);
});

test("packs and pack mods render the shared table; dry run toggles inline and rejects late replies", async () => {
  const h = await setup(), previews = [], visits = [];
  Object.assign(h.api, {
    Packs: async () => ({ rows: [{ name: "old", playedAt: "2025-01-01" }, { name: "new", playedAt: "2026-01-01" }] }),
    Pack: async () => ({ name: "pack", facts: [], dir: "test", mods: [{ name: "second", order: 2 }, { name: "first", order: 1 }] }),
    BuildPage: async () => 1,
    BuildPageResult: async () => ({ name: "example", plan: "plan", env: "env", mods: 2, steps: [], ok: true, tex: 0 }),
    PackNameCheck: async () => "",
    PackDryRun: (...args) => { const d = deferred(); previews.push({ ...d, args }); return d.promise; },
  });
  const packs = await loadModules("./packs.js", h.deps, h.globals);
  packs.initPacks({ renderHints() {}, go: (name) => { visits.push(name); h.S.screen = name; } });
  await packs.list();
  assert.equal(h.screen.querySelectorAll(".data-row")[0].textContent.startsWith("new"), true);
  packs.packsKey(event("s")); assert.equal(h.menus.at(-1)[3], true);
  await packs.pack("pack");
  assert.equal(h.screen.querySelectorAll(".data-row")[0].textContent.startsWith("first"), true);
  await packs.build();
  const button = h.screen.querySelector(".bd-dry");
  button.onclick();
  assert.equal(button.attributes["aria-expanded"], "true");
  assert.equal(h.screen.querySelector("#pk-dry-output").parent.className, "card bd-side");
  previews[0].resolve(["line one", "line two"]); await flush();
  assert.equal(h.screen.querySelector("#pk-dry-output").textContent, "line one\nline two");
  packs.packsKey(event("Escape"));
  assert.equal(h.screen.querySelector("#pk-dry-output").hidden, true); assert.equal(visits.length, 0);
  packs.packsKey(event("V")); packs.packsKey(event("V"));
  previews[1].resolve(["late list"]); await flush();
  assert.equal(h.screen.querySelector("#pk-dry-output").hidden, true);
  packs.packsKey(event("V"));
  const name = h.screen.querySelector("#pk-name"); name.value = "another"; await name.oninput();
  previews[2].resolve(["old name"]); await flush();
  assert.equal(h.screen.querySelector("#pk-dry-output").hidden, true);
  packs.packsKey(event("V")); h.S.screen = "home"; h.S.screenSeq++;
  previews[3].resolve(["abandoned"]); await flush();
  assert.ok(!h.screen.textContent.includes("abandoned"));
});


test("table positions and cells follow visible columns after moving and hiding", async () => {
  const h = await setup(), box = h.el("section"), head = h.el("div");
  const t = h.createTable({ key: "test", columns: h.packColumns, redraw() {} });
  box.append(head);
  t.draw(box, head, { role: "treegrid", labelledby: "visible-heading", count: 1354, multiselectable: true });
  const row = h.el("div");
  const paint = () => {
    row.replaceChildren(...t.visible().map((c) => h.el("span", "", c.key)));
    t.row(row, 699);
  };
  paint();
  assert.equal(box.getAttribute("aria-rowcount"), "1355");
  assert.equal(box.getAttribute("aria-colcount"), "5");
  assert.equal(box.getAttribute("aria-labelledby"), "visible-heading");
  assert.equal(head.getAttribute("role"), "row");
  assert.equal(head.getAttribute("aria-rowindex"), "1");
  assert.equal(row.getAttribute("aria-rowindex"), "701");
  t.move("name", "size", true);
  t.state.cols.find((c) => c.key === "mods").shown = false;
  t.draw(box, head, { count: 32 });
  paint();
  assert.equal(box.getAttribute("aria-rowcount"), "33");
  assert.equal(box.getAttribute("aria-colcount"), "4");
  head.children.forEach((header, i) => {
    assert.equal(header.getAttribute("aria-colindex"), String(i + 1));
    assert.equal(row.children[i].textContent, header.dataset.k);
    assert.equal(row.children[i].getAttribute("role"), "gridcell");
    assert.equal(row.children[i].getAttribute("aria-colindex"), String(i + 1));
    if (header.querySelector(".arrow")) assert.equal(header.querySelector(".arrow").getAttribute("aria-hidden"), "true");
  });
  t.draw(box, head, { role: "table" });
  paint();
  assert.equal(row.children[0].getAttribute("role"), "cell");
});
