import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";
import { runInNewContext } from "node:vm";
import { Element, loadModules, event, deferred, flush } from "./dom.mjs";

async function setup(dark = false) {
  const values = new Map([["--side-w", "240px"]]);
  const root = { dataset: {}, style: {
    setProperty: (name, value) => values.set(name, value),
    removeProperty: name => values.delete(name),
  } };
  const media = { matches: dark, addEventListener: (name, fn) => {
    assert.equal(name, "change"); media.change = fn;
  } };
  const saved = new Map();
  const globals = {
    document: { documentElement: root },
    localStorage: { getItem: k => saved.get(k), setItem: (k, v) => saved.set(k, v) },
    matchMedia: query => { assert.equal(query, "(prefers-color-scheme: dark)"); return media; },
  };
  runInNewContext(await readFile(new URL("../assets/js/theme.js", import.meta.url), "utf8"), globals);
  const { applyAccent } = await loadModules("./accent.js", {}, globals);
  return { applyAccent, values, root, media, theme: globals.archoTheme, saved };

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

test("explicit themes override the OS and auto resumes following it", async () => {
  const { applyAccent, theme, root, media, values, saved } = await setup();
  applyAccent("blue");
  theme.apply("dark");
  assert.equal(root.dataset.theme, "dark");
  assert.equal(root.dataset.scheme, "dark");
  assert.equal(values.get("--accent"), "#70b8ff");
  assert.equal(saved.get("ui.theme"), "dark");
  media.change();
  assert.equal(root.dataset.scheme, "dark");
  media.matches = true;
  theme.apply("light");
  media.change();
  assert.equal(root.dataset.scheme, "light");
  assert.equal(values.get("--accent"), "#0090ff");
  theme.apply("auto");
  assert.equal(root.dataset.scheme, "dark");
  media.matches = false; media.change();
  assert.equal(root.dataset.scheme, "light");
});

test("the cached theme is applied before Init, even if storage is unavailable", async () => {
  const source = await readFile(new URL("../assets/js/theme.js", import.meta.url), "utf8");
  for (const unavailable of [false, true]) {
    const root = { dataset: {} };
    runInNewContext(source, {
      document: { documentElement: root },
      matchMedia: () => ({ matches: true, addEventListener() {} }),
      localStorage: {
        getItem() { if (unavailable) throw Error("disabled"); return "light"; },
        setItem() { if (unavailable) throw Error("disabled"); },
      },
    });
    assert.equal(root.dataset.scheme, unavailable ? "dark" : "light");
  }
});

test("appearance options navigate without saving, serialize clicks, and keep failed values unselected", async () => {
  const calls = [], failures = [], gate = deferred();
  let focused, refreshed = 0;
  class Button extends Element { focus() { focused = this; } }
  const { appearanceControl } = await loadModules("./appearance.js", {
    "./state.js": {
      el: (tag, cls, text) => new Button(tag, cls, text),
      api: { ApplySetting: async (argv, value) => {
        calls.push([Array.from(argv), value]);
        if (value === "light") await gate.promise;
        if (value === "dark") throw Error("save failed");
      } },
      errText: e => e.message, say: (_, text) => failures.push(text),
    },
  });
  const argv = ["config", "set", "ui.theme", "{}"];
  const control = appearanceControl({edit: "segments", label: "theme", value: "auto", choices: ["auto", "light", "dark"], argv}, async () => { refreshed++; });
  const buttons = control.querySelectorAll("button");
  buttons[0].onkeydown(event("ArrowLeft"));
  assert.equal(focused, buttons[2]);
  assert.deepEqual(calls, []);
  const light = buttons[1].onclick();
  await flush();
  const dark = buttons[2].onclick();
  assert.equal(calls.length, 1);
  gate.resolve(); await light; await dark;
  assert.deepEqual(calls, [[argv, "light"], [argv, "dark"]]);
  assert.equal(refreshed, 1);
  assert.equal(buttons[1].attributes["aria-pressed"], "true");
  assert.equal(buttons[2].attributes["aria-pressed"], "false");
  assert.deepEqual(failures, ["save failed"]);
});
