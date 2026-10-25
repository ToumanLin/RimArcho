import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, deferred, event } from "./dom.mjs";

async function setup() {
  const calls = [], confirmations = [], messages = [];
  let closed = 0;
  const el = (tag, cls, text) => {
    const e = new Element(tag, cls, text);
    e.value = ""; e.checked = false; e.listeners = {};
    e.addEventListener = (name, fn) => (e.listeners[name] ||= []).push(fn);
    e.prepend = (n) => e.insertBefore(n, e.firstChild);
    const append = e.append.bind(e);
    e.append = (...ns) => {
      append(...ns);
      if (tag === "select" && e.children.length && !e.value) e.value = e.children[0].value;
    };
    return e;
  };
  const api = {
    SubscriptionPage: async () => ({ Name: "Example Colony Tweaks", Version: "1.2", Main: "https://gitgud.io/Example/colony", Repos: ["https://github.com/Example/dependency", "https://gitgud.io/Example/colony"] }),
    SubscribePreview: async (r) => { calls.push(["preview", JSON.parse(JSON.stringify(r))]); return { apply: true, lines: ["dry run, nothing changed", "folder: example"] }; },
    Subscribe: async (r) => { calls.push(["subscribe", JSON.parse(JSON.stringify(r))]); return "queued"; },
    PickFolder: async () => "downloaded-folder", PickSubscriptionZip: async () => "downloaded.zip",
    OpenURL: async (url) => calls.push(["open", url]),
    SubscriptionFromPreview: async (...args) => { calls.push(["from-preview", ...args]); return { title: "update", apply: true, lines: ["old → new"] }; },
    SubscriptionFrom: async (...args) => { calls.push(["from", ...args]); return "queued"; },
  };
  const deps = {
    "./state.js": { el, api, T: (s) => s, Tf: (s, ...a) => { let i = 0; return s.replace(/%[sd]/g, () => a[i++]); }, errText: (e) => e.message, say: (...a) => messages.push(a) },
    "./popups.js": { confirm: (...args) => confirmations.push(args) },
  };
  const mod = await loadModules("./subscribe.js", deps);
  const form = mod.subscribeForm(() => closed++);
  const root = form.el;
  const button = (label, host = root) => host.querySelectorAll("button").find((b) => b.textContent === label);
  const click = async (label, host = root) => { const b = button(label, host); assert.ok(b, label); if (!b.disabled) await b.onclick(); };
  const emit = (e, name = "input") => {
    e["on" + name]?.(event(""));
    for (let n = e; n; n = n.parent) for (const fn of n.listeners[name] || []) fn(event(""));
  };
  const set = (e, value) => { e.value = value; emit(e); };
  const type = root.querySelector("select"), url = root.querySelector("input");
  return { mod, root, api, calls, confirmations, messages, button, click, emit, set, type, url, el, closed: () => closed };
}

test("inline Git form previews before queuing and invalidates changed input", async () => {
  const h = await setup();
  assert.equal(h.root.tag, "section");
  assert.equal(h.button("subscribe").disabled, true);
  h.set(h.url, "https://gitgud.io/Example/colony.git");
  await h.click("preview");
  assert.equal(h.button("subscribe").disabled, false);
  assert.ok(h.root.textContent.includes("dry run, nothing changed"));
  assert.equal(h.confirmations.length, 0, "forms do not open a modal");
  h.set(h.url, "https://gitgud.io/Example/other");
  assert.equal(h.button("subscribe").disabled, true);
  await h.click("subscribe");
  assert.equal(h.calls.length, 1);
  await h.click("preview"); await h.click("subscribe");
  assert.equal(h.calls[2][0], "subscribe");
  assert.equal(h.calls[2][1].url, "https://gitgud.io/Example/other");
  assert.equal(h.closed(), 1);
});

test("a preview for old input cannot enable subscribe", async () => {
  const h = await setup(), reply = deferred();
  h.set(h.url, "https://gitgud.io/Example/colony");
  h.api.SubscribePreview = () => reply.promise;
  const preview = h.click("preview");
  h.set(h.url, "https://gitgud.io/Example/other");
  reply.resolve({ apply: true, lines: ["outdated preview"] }); await preview;
  assert.equal(h.button("subscribe").disabled, true);
  assert.ok(!h.root.textContent.includes("outdated preview"));
});

test("download page defaults to its main Git repository and accepts native zip choice", async () => {
  const h = await setup();
  h.set(h.type, "page"); h.set(h.url, "https://example.invalid/download");
  await h.click("read page");
  assert.ok(h.button("subscribe with Git: https://gitgud.io/Example/colony"));
  await h.click("preview");
  assert.equal(h.calls[0][1].git, true);
  await h.click("choose zip…");
  assert.equal(h.button("subscribe").disabled, true);
  await h.click("preview"); await h.click("subscribe");
  assert.equal(h.calls.at(-1)[1].git, false);
  assert.equal(h.calls.at(-1)[1].from, "downloaded.zip");
});

test("ambiguous repos require downloaded files, and late page replies are discarded", async () => {
  const h = await setup(), reply = deferred();
  h.set(h.type, "page"); h.set(h.url, "https://example.invalid/old");
  h.api.SubscriptionPage = () => reply.promise;
  const read = h.click("read page");
  h.set(h.url, "https://example.invalid/new");
  reply.resolve({ Name: "Old result", Main: "https://gitgud.io/Example/old", Repos: [] }); await read;
  assert.ok(!h.root.textContent.includes("Old result"));
  h.api.SubscriptionPage = async () => ({ Name: "Example Colony Tweaks", Repos: ["https://gitgud.io/Example/a", "https://github.com/Example/b"] });
  await h.click("read page"); await h.click("preview");
  assert.equal(h.button("subscribe").disabled, true);
  await h.click("choose folder…"); await h.click("preview"); await h.click("subscribe");
  assert.equal(h.calls.at(-1)[1].from, "downloaded-folder");
});

test("page replacement uses the existing warning confirmation after choosing files", async () => {
  const h = await setup(), host = h.el("section");
  const panel = h.mod.updateFromForm(host, "#example", () => {});
  assert.equal(host.firstChild, panel);
  await h.click("choose folder…", panel); await h.click("preview", panel);
  assert.equal(h.calls.at(-1)[0], "from-preview");
  assert.equal(h.confirmations.length, 1);
  assert.ok(!h.calls.some(([method]) => method === "from"));
  await h.confirmations[0][3]();
  assert.deepEqual(h.calls.at(-1), ["from", "#example", "downloaded-folder"]);
});

test("workshop form carries SteamCMD choice and escape collapses the panel", async () => {
  const h = await setup();
  h.set(h.type, "workshop"); h.set(h.url, "1234");
  const checkbox = h.root.querySelectorAll("input").find((e) => e.type === "checkbox");
  checkbox.checked = true; h.emit(checkbox);
  await h.click("preview");
  assert.equal(h.calls[0][1].steamcmd, true);
  h.root.onkeydown(event("Escape"));
  assert.equal(h.closed(), 1);
});

test("LoversLab fallback retains manual zip; consent and install each need the player", async () => {
  const h = await setup();
  h.set(h.type, "page"); h.set(h.url, "https://www.loverslab.com/files/file/123-fixture/");
  h.api.SubscriptionPage = async () => { throw new Error("public challenge"); };
  h.api.LoverFiles = async () => ({ reason: "login required; download a zip in your browser" });
  await h.click("read page"); await h.click("read LoversLab files");
  assert.ok(h.root.textContent.includes("login required")); assert.ok(h.button("choose zip…"));
  h.api.LoverFiles = async () => ({ files: [{ id: "opaque-choice", name: "fixture.zip" }], agreement: "I agree to the author rules" });
  let agreed = 0, installed = 0;
  h.api.LoverFollow = async (page, id, agree) => { assert.equal(id, "opaque-choice"); assert.equal(agree, true); agreed++; return { ready: "opaque-ready" }; };
  h.api.LoverInstallPreview = async () => ({ title: "install subscription", lines: ["fixture subscription folder"], apply: true });
  h.api.LoverInstall = async () => { installed++; return "done"; };
  await h.click("read LoversLab files"); await h.click("fixture.zip");
  assert.equal(agreed, 0); assert.equal(installed, 0);
  assert.equal(h.confirmations[0][1][0], "I agree to the author rules");
  await h.confirmations[0][3](); assert.equal(agreed, 1); assert.equal(installed, 0);
  await h.confirmations[1][3](); assert.equal(installed, 1);
});
