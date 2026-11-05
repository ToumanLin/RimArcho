import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { Element, loadModules, deferred, event, flush } from "./dom.mjs";

// add a subscription (16 §2.9): the paste box recognises the source, asks one decision, shows the dry
// run and only then subscribes; a download-page update needs the player's files.
async function setup(data = null) {
  const calls = [], confirmations = [], messages = [];
  let backs = 0, dones = 0;
  const opened = [];
  const el = (tag, cls, text) => {
    const e = new Element(tag, cls, text);
    e.value = ""; e.checked = false;
    e.prepend = (n) => e.insertBefore(n, e.firstChild);
    return e;
  };
  const api = {
    SubscriptionPage: async () => ({ Name: "Example Colony Tweaks", Version: "1.2", Main: "https://gitgud.io/Example/colony", Repos: ["https://github.com/Example/dependency", "https://gitgud.io/Example/colony"] }),
    SubscribePreview: async (r) => { calls.push(["preview", JSON.parse(JSON.stringify(r))]); return { apply: true, lines: ["dry run, nothing changed", "folder: example"] }; },
    Subscribe: async (r) => { calls.push(["subscribe", JSON.parse(JSON.stringify(r))]); return "queued"; },
    PickFolder: async () => "downloaded-folder", PickSubscriptionZip: async () => "downloaded.zip",
    OpenURL: async (url) => calls.push(["open", url]),
    OpenModsFolder: async () => calls.push(["mods"]),
    SubscriptionFilePage: async () => "",
    SubscriptionFromPreview: async (...args) => { calls.push(["from-preview", ...args]); return { title: "update", apply: true, lines: ["old → new"] }; },
    SubscriptionFrom: async (...args) => { calls.push(["from", ...args]); return "queued"; },
  };
  const deps = {
    "./state.js": { el, api, T: (s) => s, Tf: (s, ...a) => { let i = 0; return s.replace(/%[sd]/g, () => a[i++]); }, errText: (e) => e.message, say: (...a) => messages.push(a) },
    "./popups.js": { confirm: (...args) => confirmations.push(args) },
  };
  const mod = await loadModules("./subscribe.js", deps, { URL, setTimeout, clearTimeout, document: { dispatchEvent: e => opened.push(e.detail) }, CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init.detail; } } });
  const form = mod.addForm({ data, back: () => backs++, done: () => dones++ });
  const root = form.el;
  // a button by its own label (key hints are separate children)
  const button = (label, host = root) => host.querySelectorAll("button").find((b) => b.text === label);
  const option = (label, host = root) => host.querySelectorAll(".opt").find((b) => b.querySelector(".nm")?.textContent === label);
  const click = async (label, host = root) => { const b = button(label, host) || option(label, host); assert.ok(b, label); if (!b.disabled) await b.onclick(); await flush(); };
  const paste = async (text) => { form.setText(text); await flush(); await flush(); };
  return { mod, form, root, api, opened, calls, confirmations, messages, button, option, click, paste, el, backs: () => backs, dones: () => dones };
}

// The table is shared with the Go copy (subscribe.Detect), which the TUI and MCP use.
test("pasted text is recognised by its source", async () => {
  const { mod } = await setup();
  const cases = JSON.parse(readFileSync(new URL("../../../subscribe/testdata/detect.json", import.meta.url), "utf8"));
  assert.ok(Object.keys(cases).length > 10);
  for (const [text, kind] of Object.entries(cases)) assert.equal(mod.detect(text), kind, text);
});

test("Git: the default branch previews at once; another ref needs its name; subscribe sends what was previewed", async () => {
  const h = await setup();
  await h.paste("https://github.com/Example/colony");
  assert.deepEqual(h.calls[0], ["preview", { kind: "git", url: "https://github.com/Example/colony", refType: "branch", ref: "" }]);
  assert.ok(h.root.textContent.includes("dry run, nothing changed"));
  assert.equal(h.button("subscribe").disabled, false);
  await h.click("a tag");
  assert.equal(h.button("subscribe").disabled, true, "a tag needs its name");
  assert.equal(h.calls.length, 1, "no preview without the name");
  const name = h.root.querySelector(".subscribe-choices input");
  name.value = "v2.2.0"; name.oninput(); await flush(); await flush();
  assert.deepEqual(h.calls.at(-1)[1], { kind: "git", url: "https://github.com/Example/colony", refType: "tag", ref: "v2.2.0" });
  await h.click("subscribe");
  assert.deepEqual(h.calls.at(-1), ["subscribe", { kind: "git", url: "https://github.com/Example/colony", refType: "tag", ref: "v2.2.0" }]);
  assert.equal(h.dones(), 1);
  assert.equal(h.form.el.querySelector(".subscribe-card"), null, "the box is empty again");
});

test("a preview for older text cannot enable subscribe", async () => {
  const h = await setup(), reply = deferred();
  h.api.SubscribePreview = () => reply.promise;
  h.form.setText("https://github.com/Example/old");
  h.api.SubscribePreview = async (r) => { h.calls.push(["preview", r]); return { apply: false, lines: ["refused"] }; };
  await h.paste("https://github.com/Example/new");
  reply.resolve({ apply: true, lines: ["outdated preview"] }); await flush();
  assert.equal(h.button("subscribe").disabled, true);
  assert.ok(!h.root.textContent.includes("outdated preview"));
});

test("a download page defaults to its repository, or takes downloaded files", async () => {
  const h = await setup();
  await h.paste("https://example.invalid/mods/colony");
  assert.ok(h.root.textContent.includes("Example Colony Tweaks"));
  assert.equal(h.calls.at(-1)[1].git, true);
  await h.click("use downloaded files");
  assert.equal(h.button("subscribe").disabled, true, "no files chosen yet");
  await h.click("choose zip…"); await flush();
  assert.equal(h.calls.at(-1)[1].from, "downloaded.zip");
  await h.click("subscribe");
  assert.deepEqual(h.calls.at(-1), ["subscribe", { kind: "page", url: "https://example.invalid/mods/colony", from: "downloaded.zip" }]);
});

test("Workshop items carry the SteamCMD choice", async () => {
  const h = await setup();
  await h.paste("2009463077");
  assert.equal(h.calls.at(-1)[1].steamcmd, false);
  await h.click("download with SteamCMD");
  assert.deepEqual(h.calls.at(-1)[1], { kind: "workshop", url: "2009463077", steamcmd: true });
});

test("LoversLab: a refused session keeps the manual route; consent and install each need the player", async () => {
  const h = await setup();
  h.api.SubscriptionPage = async () => { throw new Error("public challenge"); };
  h.api.LoverFiles = async () => ({ reason: "login required; download a zip in your browser" });
  await h.paste("https://www.loverslab.com/files/file/123-fixture/");
  assert.ok(h.option("download from LoversLab").className.includes("on"), "no repository: LoversLab first");
  assert.ok(h.root.textContent.includes("login required"));
  assert.ok(h.button("open the page"));
  assert.ok(h.option("use downloaded files"));

  const g = await setup();
  g.api.SubscriptionPage = async () => ({ Name: "Fixture" });
  g.api.LoverFiles = async () => ({ files: [{ id: "opaque-choice", name: "fixture.zip" }], agreement: "I agree to the author rules" });
  let agreed = 0, installed = 0;
  g.api.LoverFollow = async (page, id, agree) => { assert.equal(id, "opaque-choice"); assert.equal(agree, true); agreed++; return { ready: "opaque-ready" }; };
  g.api.LoverInstallPreview = async () => ({ title: "install subscription", lines: ["fixture subscription folder"], apply: true });
  g.api.LoverInstall = async () => { installed++; return "done"; };
  await g.paste("https://www.loverslab.com/files/file/123-fixture/");
  await g.click("fixture.zip");
  assert.equal(agreed, 0); assert.equal(installed, 0);
  assert.equal(g.confirmations[0][1][0], "I agree to the author rules");
  await g.confirmations[0][3](); assert.equal(agreed, 1); assert.equal(installed, 0);
  await g.confirmations[1][3](); assert.equal(installed, 1);
  assert.equal(g.dones(), 1);
});

test("esc clears the box first, then goes back", async () => {
  const h = await setup();
  await h.paste("https://github.com/Example/colony");
  h.root.onkeydown(event("Escape"));
  assert.equal(h.root.querySelector(".subscribe-card"), null);
  assert.equal(h.backs(), 0);
  h.root.onkeydown(event("Escape"));
  assert.equal(h.backs(), 1);
});

test("the empty box shows the source cards with account lines and the local route", async () => {
  const h = await setup({ sources: [{ key: "page", account: "logged in, until Nov 30" }, { key: "steamcmd", account: "rim_player" }], recent: [{ name: "Performance Fish", source: "Git", added: "2026-10-12" }] });
  assert.ok(h.root.textContent.includes("logged in, until Nov 30"));
  assert.ok(h.root.textContent.includes("Performance Fish"));
  await h.click("open the Mods folder");
  assert.deepEqual(h.calls.at(-1), ["mods"]);
});

test("a download-page update previews the chosen files and replaces only after confirming", async () => {
  const h = await setup(), host = h.el("section");
  const panel = h.mod.updateFromForm(host, "#example", () => {});
  assert.equal(host.firstChild, panel);
  await h.click("choose folder…", panel); await h.click("update from these files", panel);
  assert.equal(h.calls.at(-1)[0], "from-preview");
  assert.equal(h.confirmations.length, 1);
  assert.ok(!h.calls.some(([method]) => method === "from"));
  await h.confirmations[0][3]();
  assert.deepEqual(h.calls.at(-1), ["from", "#example", "downloaded-folder"]);
});

test("recent names open the exact installed instance when names match", async () => {
  const h = await setup({ recent: [{id:"copy-a",name:"Same name",source:"Git",added:"today"},{id:"copy-b",name:"Same name",source:"Git",added:"yesterday"}] });
  const links = h.root.querySelectorAll(".subscribe-recent button");
  assert.equal(links.length,2);
  assert.ok(links.every(x => x.type === "button"), "native buttons support Tab and Enter");
  links[1].onclick(); links[0].onclick();
  assert.deepEqual(h.opened,["copy-b","copy-a"]);
});
