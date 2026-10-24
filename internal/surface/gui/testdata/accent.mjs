import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules } from "./dom.mjs";

async function setup(dark = false) {
  const values = new Map([["--side-w", "240px"]]);
  const root = { dataset: {}, style: {
    setProperty: (name, value) => values.set(name, value),
    removeProperty: name => values.delete(name),
  } };
  const media = { matches: dark, addEventListener: (name, fn) => {
    assert.equal(name, "change"); media.change = fn;
  } };
  const { applyAccent } = await loadModules("./accent.js", {}, {
    document: { documentElement: root },
    matchMedia: query => { assert.equal(query, "(prefers-color-scheme: dark)"); return media; },
  });
  return { applyAccent, values, root, media };
}

test("changing the accent and OS theme updates fills, selection, text and the exposed scale", async () => {
  const { applyAccent, values, root, media } = await setup();
  applyAccent("blue");
  assert.equal(root.dataset.accent, "blue");
  assert.equal(values.get("--accent"), "#0090ff");
  assert.equal(values.get("--accent-soft"), "#e6f4fe");
  assert.equal(values.get("--sel"), "#d5efff");
  assert.equal(values.get("--accent-fg"), "#ffffff");
  assert.equal(values.get("--accent-11"), "#0d74ce");
  assert.equal(values.get("--accent-dark-11"), "#70b8ff");
  assert.equal(values.get("--accent-dark-1"), "#0d1520");
  for (let i = 1; i <= 12; i++) assert.match(values.get(`--accent-${i}`), /^#[0-9a-f]{6}$/);
  media.matches = true; media.change();
  assert.equal(values.get("--accent"), "#70b8ff");
  assert.equal(values.get("--accent-soft"), "#0d2847");
  assert.equal(values.get("--sel"), "#004074");
  assert.equal(values.get("--accent-fg"), "#0d1520");
  applyAccent("green");
  assert.equal(values.get("--accent"), "#3dd68c");
  media.matches = false; media.change();
  assert.equal(values.get("--accent"), "#30a46c");
});

test("amber uses dark text on its bright fill in both themes", async () => {
  const { applyAccent, values, media } = await setup();
  applyAccent("amber");
  assert.equal(values.get("--accent"), "#ffc53d");
  assert.equal(values.get("--accent-fg"), "#4f3422");
  media.matches = true; media.change();
  assert.equal(values.get("--accent"), "#ffca16");
  assert.equal(values.get("--accent-fg"), "#16120c");
});

test("default and unknown settings restore CSS defaults without removing unrelated overrides", async () => {
  const { applyAccent, values, root, media } = await setup(true);
  applyAccent("default");
  for (const value of ["default", "unknown", "constructor", "__proto__", undefined]) {
    applyAccent("pink");
    applyAccent(value);
    assert.equal(root.dataset.accent, undefined);
    assert.deepEqual([...values], [["--side-w", "240px"]]);
    media.matches = !media.matches; media.change();
    assert.deepEqual([...values], [["--side-w", "240px"]]);
  }
});
