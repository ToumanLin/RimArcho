import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules } from "./dom.mjs";

// The looks view (16 §2.5 Looks view): the gallery's grid geometry, which decides what is in
// the DOM, and the first apply's confirmation with its remembered "do not show this again".
const { GRID, gridColumns, spanOf, gridLayout, visibleRows, cardTop, stepCard, scrollFor, texURL } =
  await loadModules("./looksgrid.js", {}, {});
const { applyGate, CONFIRMED_KEY, STYLES_CONFIRMED_KEY, filesText, applyLines, addLines, addPick } = await loadModules("./looksconfirm.js", {}, {});

const ones = (n) => ({ spans: Array(n).fill(1) });

test("cells are at least the minimum width and fill the box", () => {
  for (const width of [200, 640, 1000, 1337, 2560]) {
    const { cols, cell, inner } = gridColumns(width);
    assert.equal(inner, width - GRID.padX * 2);
    assert.ok(cols >= 1);
    if (inner >= GRID.min) assert.ok(cell >= GRID.min, `${width}: cell ${cell}`);
    assert.ok(cols * cell + (cols - 1) * GRID.gap <= inner, `${width}: the row fits`);
    assert.ok((cols + 1) * GRID.min + cols * GRID.gap > inner, `${width}: no further cell fits`);
  }
  assert.equal(gridColumns(0).cols, 1);
});

test("a look takes one cell per texture, at most a row", () => {
  assert.equal(spanOf(0, 6), 1);
  assert.equal(spanOf(1, 6), 1);
  assert.equal(spanOf(4, 6), 4);
  assert.equal(spanOf(9, 6), 6);
});

test("a large category fills whole rows; only its first has a heading", () => {
  const width = 1000, { cols, cell } = gridColumns(width);
  const lay = gridLayout([ones(cols * 2 + 1)], width);
  assert.equal(lay.rows.length, 3);
  assert.deepEqual(Array.from(lay.rows, (r) => r.heads.length), [1, 0, 0]);
  assert.deepEqual(Array.from(lay.rows, (r) => r.cards.length), [cols, cols, 1]);
  assert.equal(lay.rows[0].h, GRID.head + GRID.card + GRID.gap);
  assert.equal(lay.rows[1].h, GRID.card + GRID.gap);
  assert.equal(lay.rows[1].y, lay.rows[0].y + lay.rows[0].h);
  assert.deepEqual(Array.from(lay.rows[0].cards, (c) => c.x), Array.from({ length: cols }, (_, i) => i * (cell + GRID.gap)));
  assert.equal(lay.cards.length, cols * 2 + 1);
  assert.deepEqual({ ...lay.cards.at(-1) }, { row: 2, x: 0, w: cell, group: 0, item: cols * 2 });
  assert.equal(lay.total, lay.rows[2].y + lay.rows[2].h + GRID.foot);
});

test("small categories share a row, each under its own heading, and never overlap", () => {
  const width = 1000;
  const lay = gridLayout([ones(1), ones(2), ones(1), { spans: [] }, ones(40)], width);
  const first = lay.rows[0];
  assert.deepEqual(Array.from(first.heads.slice(0, 3), (h) => h.group), [0, 1, 2]);
  assert.deepEqual(Array.from(first.cards.slice(0, 4), (c) => lay.cards[c.n].group), [0, 1, 1, 2]);
  for (let i = 1; i < first.heads.length; i++) {
    const before = first.cards.filter((c) => lay.cards[c.n].group === first.heads[i - 1].group).at(-1);
    assert.equal(first.heads[i].x, before.x + before.w + GRID.group, "a wider gap between categories");
  }
  for (const row of lay.rows) {
    for (let i = 1; i < row.cards.length; i++) {
      assert.ok(row.cards[i].x >= row.cards[i - 1].x + row.cards[i - 1].w + GRID.gap, "cards do not overlap");
    }
    const last = row.cards.at(-1);
    assert.ok(last.x + last.w <= lay.inner + 1, "cards stay inside the box");
  }
  // the large category has one heading; its further rows start at the left edge
  assert.equal(lay.rows.flatMap((r) => r.heads).filter((h) => h.group === 4).length, 1);
  assert.equal(lay.rows.at(-1).heads.length, 0);
  assert.equal(lay.rows.at(-1).cards[0].x, 0);
  assert.equal(lay.cards.length, 44);
});

test("a look with several textures is as wide as its cells and is never cut", () => {
  const width = 640, { cols, cell } = gridColumns(width);
  const lay = gridLayout([{ spans: [1, cols, 3, 99] }], width);
  const w = (n) => n * (cell + GRID.gap) - GRID.gap;
  assert.deepEqual(Array.from(lay.cards, (c) => c.w), [w(1), w(cols), w(Math.min(3, cols)), w(cols)]);
  for (const c of lay.cards) assert.ok(c.x + c.w <= lay.inner + 1);
  assert.equal(gridLayout([], width).total, 0);
});

test("only the rows near the box are drawn", () => {
  const width = 1000, { cols } = gridColumns(width);
  const lay = gridLayout([ones(cols * 400)], width);
  assert.equal(lay.rows.length, 400);
  const [a, b] = visibleRows(lay, 0, 600);
  assert.equal(a, 0);
  assert.ok(b < 10, `${b} rows for the first screen`);
  const top = lay.rows[200].y;
  const [c, d] = visibleRows(lay, top, 600);
  assert.ok(c <= 200 && d > 200 && d - c < 14, `${c}..${d}`);
  for (let i = c; i < d; i++) {
    assert.ok(lay.rows[i].y + lay.rows[i].h >= top - GRID.overscan && lay.rows[i].y <= top + 600 + GRID.overscan);
  }
  assert.ok(lay.rows[c - 1].y + lay.rows[c - 1].h < top - GRID.overscan);
  assert.ok(lay.rows[d].y >= top + 600 + GRID.overscan);
  assert.deepEqual([...visibleRows(lay, lay.total + 5000, 600)], [400, 400]);
  assert.deepEqual([...visibleRows(gridLayout([], width), 0, 600)], [0, 0]);
});

test("the arrows move by rows to the nearest card and stop at the ends", () => {
  const width = 1000, { cols } = gridColumns(width);
  const lay = gridLayout([ones(cols + 2), { spans: [cols] }], width);
  assert.equal(stepCard(lay, 1, 1), cols + 1);
  assert.equal(stepCard(lay, cols - 1, 1), cols + 1, "the nearest card of a shorter row");
  assert.equal(stepCard(lay, cols + 1, -1), 1);
  assert.equal(stepCard(lay, 0, -1), 0);
  const last = lay.cards.length - 1;
  assert.equal(stepCard(lay, last, 1), last);
  assert.equal(stepCard(lay, cols, 1), last, "onto the look that takes the whole row");
  assert.equal(stepCard(gridLayout([], width), 0, 1), -1);
});

test("the cursor's card is scrolled into view, and only then", () => {
  const width = 1000, { cols } = gridColumns(width);
  const lay = gridLayout([ones(cols * 50)], width);
  assert.equal(cardTop(lay, 0), GRID.padY + GRID.head);
  assert.equal(scrollFor(lay, 0, 0, 600), 0);
  const n = cols * 30, y = cardTop(lay, n);
  const down = scrollFor(lay, n, 0, 600);
  assert.equal(down, y + GRID.card - 600 + GRID.foot);
  assert.equal(scrollFor(lay, n, down, 600), down, "a card that shows leaves the scroll alone");
  const up = scrollFor(lay, cols * 2, down, 600);
  assert.ok(up <= cardTop(lay, cols * 2) && up >= 0);
});

test("a texture's address carries the look, the provider, the slot, the box and the stamp", () => {
  assert.equal(texURL("Things/Pawn/Humanlike/Bodies/Naked Male", 12, "south_mask", 192, "a b"),
    "/looktex/Things/Pawn/Humanlike/Bodies/Naked%20Male?p=12&s=south_mask&w=192&v=a%20b");
  assert.equal(texURL("UI/Icon#1", 0, "", 0), "/looktex/UI/Icon%231?p=0&s=&w=0&v=");
});

// One page's gate: the setting is what remember() writes and confirmed() reads.
function page({ confirmed = false, previewFails = false, rememberFails = false } = {}) {
  const calls = [];
  const settings = { [CONFIRMED_KEY]: confirmed };
  let answer = null;
  const gate = applyGate({
    confirmed: () => settings[CONFIRMED_KEY],
    preview: async () => { calls.push("preview"); if (previewFails) throw new Error("no"); return { dryRun: true, files: 3 }; },
    ask: (done, go) => { calls.push(["ask", done.files]); answer = go; },
    remember: async () => { calls.push("remember"); if (rememberFails) throw new Error("read-only"); settings[CONFIRMED_KEY] = true; },
    fail: (e) => calls.push(["fail", e.message]),
  });
  const apply = () => gate(async () => { calls.push("write"); });
  return { calls, settings, apply, yes: (ticked) => answer(ticked), take: () => calls.splice(0) };
}

test("the first apply shows what it would write and writes only after a yes", async () => {
  const p = page();
  await p.apply();
  assert.deepEqual(p.take(), ["preview", ["ask", 3]], "nothing is written before the answer");
  await p.yes(false);
  assert.deepEqual(p.take(), ["write"]);
  assert.equal(p.settings[CONFIRMED_KEY], false);
  await p.apply();
  assert.deepEqual(p.take(), ["preview", ["ask", 3]], "not ticked: asked again");
  // cancel: go is never called, so nothing is written and nothing remembered
  await p.apply();
  assert.deepEqual(p.take(), ["preview", ["ask", 3]]);
});

test("do not show this again is remembered: later applies write at once", async () => {
  const p = page();
  await p.apply();
  await p.yes(true);
  assert.deepEqual(p.take(), ["preview", ["ask", 3], "remember", "write"]);
  assert.equal(p.settings[CONFIRMED_KEY], true);
  await p.apply();
  assert.deepEqual(p.take(), ["write"], "no dry run, no dialog");
  const again = page({ confirmed: true });
  await again.apply();
  assert.deepEqual(again.calls, ["write"]);
});

test("a preview that fails asks nothing and writes nothing; an unkept tick still writes", async () => {
  const p = page({ previewFails: true });
  await p.apply();
  assert.deepEqual(p.take(), ["preview", ["fail", "no"]]);
  const q = page({ rememberFails: true });
  await q.apply();
  await q.yes(true);
  assert.deepEqual(q.take(), ["preview", ["ask", 3], "remember", "write"]);
  assert.equal(q.settings[CONFIRMED_KEY], false);
  await q.apply();
  assert.deepEqual(q.take(), ["preview", ["ask", 3]], "asked again next time");
});

test("a second press while the dry run is on its way does nothing", async () => {
  const p = page();
  const first = p.apply(), second = p.apply();
  await Promise.all([first, second]);
  assert.deepEqual(p.take(), ["preview", ["ask", 3]]);
});

// ---- styles and random variants (21 §5): the add row's model and the first add's confirmation ----

const W = { T: (s) => s, Tf: (f, ...a) => { let i = 0; return f.replace(/%s/g, () => String(a[i++])); }, num: (n) => String(n) };
const ADD = {
  styles: [
    { kind: "style", target: "Animalist", label: "style in Animalist", ok: true, note: "an ideoligion gives it" },
    { kind: "style", target: "Rustic", label: "style in Rustic", ok: true, shared: true, note: "this category already has a style" },
    { kind: "style", target: "Morbid", label: "style in Morbid", ok: false, why: "already added", code: "exists" },
  ],
  variants: [
    { kind: "variant", target: "default", label: "random variant of the default look", ok: false, why: "the graphic class Graphic_StackCount cannot be turned into a random set", code: "class" },
  ],
  variantsNone: "the graphic class Graphic_StackCount cannot be turned into a random set",
  entries: [],
};

test("the two confirmations are remembered apart", () => {
  assert.notEqual(STYLES_CONFIRMED_KEY, CONFIRMED_KEY);
  assert.equal(STYLES_CONFIRMED_KEY, "ui.gui.styles.confirmed");
});

test("the add row takes the wanted target, else Go's first that can be added; the note and add follow Go", () => {
  let p = addPick(ADD, "style", "");
  assert.equal(p.choice.target, "Animalist");
  assert.equal(p.note, "an ideoligion gives it");
  assert.equal(p.can, true);
  p = addPick(ADD, "style", "Rustic");
  assert.equal(p.choice.shared, true);
  assert.equal(p.note, "this category already has a style", "a shared category keeps Go's note");
  assert.equal(p.can, true);
  p = addPick(ADD, "style", "Morbid");
  assert.equal(p.note, "already added", "what cannot be added shows Go's reason");
  assert.equal(p.can, false);
  p = addPick(ADD, "style", "Gone");
  assert.equal(p.choice.target, "Animalist", "a target that left the list falls back");
  p = addPick(ADD, "variant", "");
  assert.equal(p.style, false);
  assert.equal(p.choice.target, "default");
  assert.equal(p.can, false);
  assert.match(p.note, /Graphic_StackCount/);
});

test("an empty list shows why nothing of that kind can be added and disables add", () => {
  const none = { styles: [], stylesNone: "select a Mod to add its version", variants: [], variantsNone: "select a Mod to add its version", entries: [] };
  for (const kind of ["style", "variant"]) {
    const p = addPick(none, kind, "");
    assert.equal(p.choice, null);
    assert.equal(p.can, false);
    assert.equal(p.note, "select a Mod to add its version");
    assert.equal(p.list.length, 0);
  }
  const absent = addPick(undefined, "style", "");
  assert.deepEqual([absent.choice, absent.can, absent.note], [null, false, ""], "a detail without the add row");
});

test("the files line says links, copies or both", () => {
  assert.equal(filesText({ files: 0 }, W, "none"), "none");
  assert.equal(filesText({ files: 3, linked: 3, copied: 0, size: "1 MB" }, W, "none"), "3, linked (no copies)");
  assert.equal(filesText({ files: 3, linked: 0, copied: 3, size: "1 MB" }, W, "none"), "3, copied (1 MB)");
  assert.equal(filesText({ files: 3, linked: 2, copied: 1, size: "1 MB" }, W, "none"), "2 linked, 1 copied (1 MB)");
});

test("the first add says what is written and what it does to saves, per kind", () => {
  const facts = [["look", "flak jacket"], ["from", "Retexture"], ["as", "style in Animalist"], ["for", "all lists (styles are shared)"]];
  const done = { dryRun: true, dir: "D:/Looks", files: 2, linked: 2, copied: 0, size: "8 KB", note: "an ideoligion gives it", added: true, dirChosen: true };
  const style = addLines(facts, done, "style", W);
  assert.deepEqual(Array.from(style.slice(0, 6)), ["look: flak jacket", "from: Retexture", "as: style in Animalist", "for: all lists (styles are shared)", "folder: D:/Looks", "files: 2, linked (no copies)"]);
  assert.ok(style.some((l) => /RimArcho Styles Preference, near the end of the list/.test(l) && /small Def file/.test(l)));
  assert.ok(style.some((l) => /new style in an existing style category/.test(l) && /skipped/.test(l)));
  assert.ok(style.some((l) => /go back to their default look/.test(l)));
  assert.ok(style.includes("an ideoligion gives it"), "Go's note for the target");
  assert.ok(style.some((l) => /gains RimArcho Styles Preference/.test(l)));
  assert.ok(style.includes("The folder is created now."));
  const variant = addLines(facts, { ...done, added: false, dirChosen: false, note: "", buildError: "a game is running" }, "variant", W);
  assert.ok(variant.some((l) => /pick one of the textures for each item/.test(l) && /may change look/.test(l)));
  assert.ok(variant.includes("Saves do not depend on a random variant."));
  assert.ok(!variant.some((l) => /new style in an existing/.test(l) || /gains RimArcho/.test(l) || /folder is created/.test(l)));
  assert.equal(variant.at(-1), "a game is running", "a rebuild that would not run is said");
});

test("the first apply's lines are the same builder's: no Defs, no save dependency", () => {
  const lines = applyLines([["look", "flak jacket"]], { dir: "D:/Looks", files: 0, added: true, mixed: 1 }, W);
  assert.equal(lines[0], "look: flak jacket");
  assert.equal(lines[2], "files: none: the load order already gives these textures");
  assert.ok(lines.some((l) => /RimArcho Looks Preference, last in the list/.test(l) && /no Defs/.test(l)));
  assert.ok(lines.some((l) => /saves do not depend on it/.test(l)));
  assert.ok(lines.some((l) => /gains RimArcho Looks Preference as its last Mod/.test(l)));
  assert.ok(lines.some((l) => /^Mixed result/.test(l)));
});
