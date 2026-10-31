import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, deferred, flush } from "./dom.mjs";

async function setup(desktop = true, os = "windows", environment = null) {
  const app = new Element("div"), top = new Element("header"), side = new Element("aside");
  app.append(side, top);
  const nodes = { "#app": app, "#top": top, "#side": side };
  const calls = [], events = new Map();
  let maximized = false, blocked = true, closed = false;
  const win = {
    Minimise: async () => calls.push("minimize"),
    ToggleMaximise: async () => { calls.push("maximize"); maximized = !maximized; },
    IsMaximised: async () => maximized,
    Close: async () => { calls.push("WindowClosing"); if (!blocked) closed = true; },
  };
  const mod = await loadModules("./window-chrome.js", {
    "/wails/runtime.js": desktop ? { Window: win, System: { Environment: () => environment.promise }, Events: { On: (name, fn) => events.set(name, fn) },
      Application: { Quit: () => assert.fail("must emit WindowClosing instead of quitting") } } : {},
    "./state.js": { $: selector => nodes[selector], el: (tag, cls) => new Element(tag, cls), T: s => s, factText() {} },
  }, { _wails: desktop ? { environment: { OS: os } } : undefined, console,
    getComputedStyle: n => ({ getPropertyValue: () => n.drag ? "drag" : "no-drag" }) });
  mod.initWindowChrome();
  mod.placeWindowControls("side");
  await flush();
  return { ...mod, app, top, side, win, calls, events,
    controls: top.children[0], closed: () => closed, unblock: () => { blocked = false; } };
}

test("late native environment still creates controls in the current navigation layout", async () => {
  const env = deferred(), s = await setup(true, undefined, env);
  // undefined uses setup's default; null represents Wails not yet publishing OS.
  assert.equal(s.controls.children.length, 3);
  const late = await setup(true, null, env);
  assert.equal(late.top.children.length, 0);
  late.placeWindowControls("top");
  env.resolve({ OS: "windows" }); await flush(); await flush();
  assert.equal(late.side.lastChild.children.length, 3);
  assert.equal(late.app.classList.contains("window-chrome"), true);
});

test("browser shim and native-frame platforms have no window controls or drag surface", async () => {
  for (const [desktop, os] of [[false, "windows"], [true, "linux"], [true, "darwin"]]) {
    const s = await setup(desktop, os);
    assert.equal(s.top.children.length, 0);
    assert.equal(s.side.children.length, 0);
    assert.equal(s.app.classList.contains("window-chrome"), false);
    assert.equal(s.events.size, 0);
    assert.deepEqual(s.calls, []);
  }
});

test("buttons stay on the right in both layouts and call the cancellable window close", async () => {
  const s = await setup();
  assert.equal(s.controls.children.length, 3);
  s.initWindowChrome();
  assert.equal(s.top.children.length, 1);
  s.placeWindowControls("top");
  assert.equal(s.side.lastChild, s.controls);
  assert.equal(s.top.children.length, 0);
  s.placeWindowControls("side");
  assert.equal(s.top.lastChild, s.controls);
  const [min, max, close] = s.controls.children;
  await min.onclick();
  await max.onclick();
  await s.events.get("common:WindowMaximise")();
  assert.equal(max.attributes["aria-label"], "restore window");
  await max.onclick();
  await s.events.get("common:WindowUnMaximise")();
  assert.equal(max.attributes["aria-label"], "maximize window");
  await close.onclick();
  assert.equal(s.closed(), false, "a Go close guard can cancel the close");
  s.unblock();
  await close.onclick();
  assert.equal(s.closed(), true);
  assert.deepEqual(s.calls, ["minimize", "maximize", "maximize", "WindowClosing", "WindowClosing"]);
});

test("caption double-click ignores controls and their children; keyboard activation stays native", async () => {
  const s = await setup();
  const drag = new Element("span"); drag.drag = true;
  let prevented = 0;
  for (const target of [s.controls, ...s.controls.children, new Element("svg")]) {
    s.app.dispatchEvent(event("", { type: "dblclick", target, preventDefault: () => prevented++ }));
  }
  await flush();
  assert.deepEqual(s.calls, []);
  s.app.dispatchEvent(event("", { type: "dblclick", target: drag, preventDefault: () => prevented++ }));
  await flush();
  assert.deepEqual(s.calls, ["maximize"]);
  assert.equal(prevented, 1);
  let stopped = 0;
  for (const key of ["Enter", " ", "Tab"]) s.controls.dispatchEvent(event(key, {
    type: "keydown", stopPropagation: () => stopped++, preventDefault: () => assert.fail("native activation/tab must work"),
  }));
  assert.equal(stopped, 3);
});

test("an older maximize query cannot overwrite the current restore label", async () => {
  const s = await setup();
  const old = deferred();
  s.win.IsMaximised = () => old.promise;
  const first = s.events.get("common:WindowMaximise")();
  await flush();
  s.win.IsMaximised = async () => true;
  await s.events.get("common:WindowMaximise")();
  old.resolve(false);
  await first;
  assert.equal(s.controls.children[1].title, "restore window");
});
