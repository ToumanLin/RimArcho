import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, deferred, flush } from "./dom.mjs";

async function setup(saved = new Map()) {
  const listeners = new Map(), handles = [], writes = [];
  class Handle extends Element {
    listeners = new Map(); visible = true;
    addEventListener(name, fn) { this.listeners.set(name, [...(this.listeners.get(name) || []), fn]); }
    dispatch(name, data = {}) { for (const fn of this.listeners.get(name) || []) fn(event("", data)); }
    getClientRects() { return this.visible ? [{}] : []; }
  }
  const body = new Element("body");
  const api = {
    Setting: async (key) => saved.get(key) || "",
    SetSetting: async (key, value) => { saved.set(key, value); writes.push([key, value]); },
  };
  const { wireSeparator } = await loadModules("./separator.js", { "./state.js": { api, T: (s) => s } }, {
    document: { body, documentElement: body, querySelectorAll: () => handles },
    getComputedStyle: () => ({ visibility: "visible" }), requestAnimationFrame: queueMicrotask,
    MutationObserver: class { observe() {} },
    addEventListener: (name, fn) => listeners.set(name, fn),
    removeEventListener: (name) => listeners.delete(name),
  });
  function make(key, axis = "x", extras = {}) {
    const handle = new Handle("div"); handles.push(handle);
    let size = 300, enabled = true, changed = 0;
    const wired = wireSeparator(handle, {
      key, axis, range: () => [120, 500], get: () => size, set: (v) => { size = v; },
      reset: () => { size = 300; }, restore: (v) => { size = +v || 300; },
      enabled: () => enabled, changed: () => { changed++; }, ...extras,
    });
    return { handle, ...wired, size: () => size, changed: () => changed, disable: () => { enabled = false; } };
  }
  return { make, api, saved, writes, listeners };
}

// 06 §3.1: focusable gaps move by 40px, clamp, reset, and save per page.
test("both orientations expose their range; keyboard, reset and restore round trip independently", async () => {
  const h = await setup();
  for (const [key, axis, before, after, orientation] of [
    ["ui.gui.plans.side", "x", "ArrowLeft", "ArrowRight", "vertical"],
    ["ui.gui.tasks.split", "y", "ArrowUp", "ArrowDown", "horizontal"],
  ]) {
    const s = h.make(key, axis); await s.ready; await flush();
    assert.equal(s.handle.tabIndex, 0);
    assert.equal(s.handle.attributes.role, "separator");
    assert.equal(s.handle.attributes["aria-orientation"], orientation);
    assert.equal(s.handle.attributes["aria-valuemin"], "120");
    assert.equal(s.handle.attributes["aria-valuemax"], "500");
    let prevented = false, stopped = false;
    s.handle.dispatch("keydown", { key: after, preventDefault() { prevented = true; }, stopPropagation() { stopped = true; } });
    assert.ok(prevented && stopped);
    assert.equal(s.size(), 340); assert.equal(s.handle.attributes["aria-valuenow"], "340");
    assert.equal(h.saved.get(key), "340");
    const reopened = h.make(key, axis); await reopened.ready;
    assert.equal(reopened.size(), 340);
    s.handle.dispatch("keydown", { key: before }); assert.equal(s.size(), 300);
    for (let i = 0; i < 20; i++) s.handle.dispatch("keydown", { key: after });
    assert.equal(s.size(), 500);
    for (let i = 0; i < 20; i++) s.handle.dispatch("keydown", { key: before });
    assert.equal(s.size(), 120);
    s.handle.dispatch("keydown", { key: "Enter" });
    assert.equal(s.size(), 300); assert.equal(h.saved.get(key), "");
    s.handle.dispatch("keydown", { key: after }); s.handle.dispatch("dblclick");
    assert.equal(s.size(), 300); assert.equal(h.saved.get(key), "");
    assert.ok(s.changed() > 0);
  }
});

test("drag saves on release; right click, hidden gaps and narrow layouts do not resize", async () => {
  const h = await setup(), s = h.make("ui.gui.log.left"); await s.ready;
  s.handle.dispatch("mousedown", { button: 2, clientX: 10 }); assert.ok(!h.listeners.has("mousemove"));
  s.handle.dispatch("mousedown", { button: 0, clientX: 10 });
  h.listeners.get("mousemove")({ clientX: 90 });
  assert.equal(s.size(), 380); assert.equal(h.writes.length, 0);
  h.listeners.get("mouseup")(); assert.equal(h.saved.get("ui.gui.log.left"), "380");
  assert.ok(!h.listeners.has("mousemove"));
  s.disable(); s.refresh(); assert.equal(s.handle.tabIndex, -1);
  const count = h.writes.length;
  s.handle.dispatch("keydown", { key: "ArrowRight" }); s.handle.dispatch("dblclick");
  s.handle.dispatch("mousedown", { button: 0, clientX: 0 });
  assert.equal(s.size(), 380); assert.equal(h.writes.length, count); assert.ok(!h.listeners.has("mousemove"));
  const hidden = h.make("ui.gui.library.side"); await hidden.ready;
  hidden.handle.visible = false; hidden.refresh(); assert.equal(hidden.handle.tabIndex, -1);
  hidden.handle.dispatch("keydown", { key: "ArrowRight" }); assert.equal(hidden.size(), 300);
});

test("legacy serialized widths, dynamic environment keys and a late read preserve user changes", async () => {
  const h = await setup(new Map([["ui.gui.columns:old", "100fr,200fr,300fr"]]));
  let env = "old", widths;
  const s = h.make(() => "ui.gui.columns:" + env, "x", {
    restore: (v) => { widths = v; }, serialize: () => widths,
  });
  await s.ready; assert.equal(widths, "100fr,200fr,300fr");
  env = "new"; widths = "140fr,160fr,300fr";
  s.handle.dispatch("keydown", { key: "ArrowRight" });
  assert.equal(h.saved.get("ui.gui.columns:new"), widths);
  assert.equal(h.saved.get("ui.gui.columns:old"), "100fr,200fr,300fr");
  const late = deferred(); h.api.Setting = () => late.promise;
  const next = h.make("ui.gui.tasks.side");
  next.handle.dispatch("keydown", { key: "ArrowRight" });
  late.resolve("180"); await next.ready;
  assert.equal(next.size(), 340);
});

test("bottom cards reverse arrow direction so the gap follows the pressed arrow", async () => {
  const h = await setup(), s = h.make("ui.gui.plan.stage", "y", { direction: -1 });
  await s.ready;
  s.handle.dispatch("keydown", { key: "ArrowUp" }); assert.equal(s.size(), 340);
  s.handle.dispatch("keydown", { key: "ArrowDown" }); assert.equal(s.size(), 300);
});
