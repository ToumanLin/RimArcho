import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, flush, deferred } from "./dom.mjs";

const install = { label: "install", argv: ["deps", "install", "steamcmd"], task: "install-steamcmd", pending: "downloading…", confirm: "download?" };
const remove = { label: "remove", argv: ["deps", "remove", "steamcmd"], task: "remove-dependency", pending: "removing…", confirm: "remove?" };
const row = (value, editValue, actions) => ({ label: "SteamCMD", value, editValue, edit: "path", file: true, argv: ["config", "set", "steamcmd.path", "{}"], actions });

async function setup(item) {
  const events = {}, calls = [], confirmations = [];
  class Node extends Element {
    get childNodes() { return this.children; }
    prepend(n) { this.insertBefore(n, this.firstChild); }
    removeAttribute(k) { delete this.attributes[k]; }
  }
  const el = (tag, cls, text) => new Node(tag, cls, text);
  const screen = el("div"), popup = el("div"), document = { activeElement: null };
  let groups = [{ title: "external tools", items: [item] }], programPicks = 0, folderPicks = 0, picked = "C:/tools/custom.exe", folder = "C:/new-folder";
  const S = { screen: "settings", screenSeq: 1, busy: {} };
  const api = {
    Settings: async () => groups, Tasks: async () => ({ rows: [] }),
    ApplySetting: async (...args) => { calls.push(args); return ["done"]; },
    ToolInstallOffer: async () => ({url:"https://fixture.invalid/steamcmd.zip",dir:"C:/fixture/steamcmd",bytes:-1}),
    PickProgram: async () => { programPicks++; return await picked; },
    PickFolder: async () => { folderPicks++; return await folder; },
    DirectoryMovePreview: async (...args) => { calls.push(["preview", ...args]); return ["synthetic link remains valid"]; },
  };
  const screens = await loadModules("./screens.js", {
    "./state.js": { S, $: sel => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen /, "")), el, api, T: s => s, Tf: s => s, errText: e => e.message, say() {}, cur() {}, slideThumb() {} },
    "./icons.js": { icon: {}, statusIcon() {}, bowMark: () => "" },
    "./motion.js": { bump() {}, shimmer: () => el("div") },
    "./popups.js": {
      open: (kind, title, build) => { popup.replaceChildren(); S.popup = { kind, title }; build(popup, S.popup); },
      close: () => { S.popup = null; popup.replaceChildren(); },
      confirm: (title, lines, label, submit) => { confirmations.push({ lines, submit }); } },
    "./bg.js": { bgSection: async () => el("div") },
    "./virtualrows.js": { virtualRows() {} },
    "./api.js": { on: (name, fn) => { events[name] = fn; } },
    "./separator.js": { wireSeparator() {} },
    "./fulldetail.js": { full() {}, fullKey() {}, fullSection() {} },
  }, { document, URL, requestAnimationFrame: fn => fn() });
  screens.initScreens({ reload: async () => {} });
  await screens.settings();
  return { screen, popup, calls, confirmations, events, document, screens, S, setPick: value => { picked = value; }, setFolder: value => { folder = value; }, folderPicks: () => folderPicks, programPicks: () => programPicks, setItem: it => { groups = [{ title: "external tools", items: [it] }]; } };
}

const click = (h, label) => h.screen.querySelectorAll("button").find(b => b.textContent === label).onclick(event(""));

test("tool change opens the native file dialog directly and applies the selected file", async () => {
  const h = await setup(row("0.4.1", "", [install]));
  assert.equal(h.screen.querySelector("input"), null);
  await click(h, "change…"); await flush();
  assert.equal(h.programPicks(), 1); assert.equal(h.folderPicks(), 0);
  assert.equal(h.popup.querySelector("input"), null);
  assert.equal(h.calls[0][1], "C:/tools/custom.exe");
  assert.deepEqual(Array.from(h.calls[0][0]), ["config", "set", "steamcmd.path", "{}"]);
});

test("cancelled and late native selections never apply; concurrent opens are ignored", async () => {
  const h = await setup(row("installed", "", []));
  h.setPick(""); click(h, "change…"); await flush(); assert.equal(h.calls.length, 0);
  const pending = deferred(); h.setPick(pending.promise);
  click(h, "change…"); click(h, "change…"); await flush();
  assert.equal(h.programPicks(), 2);
  h.S.screenSeq++; pending.resolve("C:/late.exe"); await flush();
  assert.equal(h.calls.length, 0);
});

test("adding folders is labelled Add and uses only the folder picker", async () => {
  const h = await setup({label:"add another mod folder",edit:"path",argv:["folder","add","extra","{}"]});
  assert.equal(h.screen.querySelector("input"), null);
  await click(h, "add…"); await flush();
  assert.equal(h.folderPicks(), 1); assert.equal(h.programPicks(), 0);
  assert.equal(h.calls[0][1], "C:/new-folder");
});

test("editable address text opens the picker, including after a task refresh", async () => {
  const item = {...row("installed", "C:/tools/old.exe", [install]),note:"C:/tools/old.exe",notePaths:["C:/tools/old.exe"]};
  const h = await setup(item);
  let address = h.screen.querySelector(".address-pick");
  assert.equal(address.attributes.role, "button");
  assert.match(address.title, /click to choose a file/);
  await address.onclick(event("")); await flush();
  h.setItem({...item,value:"new version",actions:[remove]});
  h.events.tasks({ended:true}); await flush();
  address = h.screen.querySelector(".address-pick");
  assert.ok(address); address.onkeydown(event("Enter")); await flush();
  assert.equal(h.programPicks(), 2);
  await click(h, "remove");
  assert.equal(h.confirmations.length, 1);
  await h.confirmations[0].submit(); await flush();
  assert.deepEqual(Array.from(h.calls.at(-1)[0]), ["deps", "remove", "steamcmd"]);
});

test("directory relocation applies on choice; moving still requires preview and confirmation", async () => {
  const h = await setup({label:"lock library",value:"C:/old/locked",valuePath:true,edit:"directory",directory:"locked"});
  await click(h, "locate"); await flush();
  assert.deepEqual(Array.from(h.calls[0][0]), ["folder","set","locked","{}"]);
  h.calls.length=0;
  await click(h, "move to…"); await flush();
  assert.equal(h.calls[0][0], "preview"); assert.equal(h.calls[0][2], "C:/new-folder/locked");
  assert.equal(h.calls.length, 1); assert.match(h.confirmations[0].lines.join(" "), /synthetic link/);
  assert.equal(h.popup.querySelector("input"), null);
  await h.confirmations[0].submit();
  assert.deepEqual(Array.from(h.calls[1][0]), ["folder","move","locked","{}"]);
});

test("URL editor validates before saving and cancel preserves the complete URL", async () => {
  const original="https://example.invalid/rules.json?branch=main&lang=zh#part";
  const h = await setup({label:"rules",value:original,edit:"url",argv:["config","set","data.rules.url","{}"]});
  assert.equal(h.screen.querySelector("input"), null);
  h.events.tasks({ended:true}); await flush();
  assert.equal(h.screen.querySelectorAll(".address-copy").length, 1);
  click(h, "change…");
  const input=h.popup.querySelector("input"); assert.equal(input.value, original);
  const confirm=()=>h.popup.querySelectorAll("button").find(b=>b.textContent==="OK").onclick(event(""));
  for(const bad of ["", "example.invalid/a", "https:example.invalid/a", "file:///synthetic/a", "https://example.invalid/a b"]){
    input.value=bad; confirm(); assert.equal(h.calls.length, 0); assert.ok(h.popup.querySelector(".err").textContent); assert.equal(input.attributes["aria-invalid"], "true");
  }
  h.popup.querySelectorAll("button").find(b=>b.textContent==="cancel").onclick(event(""));
  assert.equal(h.calls.length, 0);
  click(h, "change…"); const next="https://example.invalid/new.json?x=1&y=2#fragment";
  h.popup.querySelector("input").value=next; confirm(); await flush();
  assert.equal(h.calls[0][1], next);
});
