import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

async function setup(workshop = false) {
  const app = new Element("div"), top = new Element("header"); top.id = "top"; app.append(top);
  let popup, state, picked, moves = 0, writes = 0, fail = false;
  const info = { path: "X:/RimArcho", free: 4 * 1073741824, low: true, workshop, bytes: 32000000, replace: workshop };
  const pop = { open(_, title, build) { popup = new Element("div"); state = {}; build(popup, state); }, close() { if (state.canClose?.() !== false) state.onclose?.(); } };
  const module = await loadModules("./firstrun.js", {
    "./state.js": { el: (t,c,s) => new Element(t,c,s), $: s => s === "#top" ? top : app, api: {
      FinishWorkshopRelocation: async () => {}, WorkshopFolder: async path => ({ ...info, path: path || info.path }),
      ManagedFolder: async () => info, FolderSpace: async path => ({ ...info, path, low: false, free: 40 * 1073741824 }),
      ChooseManagedFolder: async () => { writes++; }, RelocateWorkshop: async () => { moves++; if (fail) throw Error("copy failed"); },
    }, T: s => s, Tf: (s,...args) => s.replace(/%s/g,()=>String(args.shift())), errText: e => e.message, say() {} },
    "./screens.js": { button(label,_,run) { const b = new Element("button","",label); b.onclick = run; return b; }, chooseLocalPath(_,done) { picked = done; } },
    "./fields.js": { editablePath: n => n, copyablePath: n => n, pathContent: n => n }, "./popups.js": pop,
  });
  return { module, app, popup: () => popup, button: label => popup.querySelectorAll("button").find(n=>n.textContent===label),
    writes: () => writes, moves: () => moves, choose: path => picked(path), fail: () => { fail = true; } };
}

test("managed-folder choice only saves after confirmation and refreshes free space", async () => {
  const h = await setup(); const done = h.module.managedFolders(); await flush();
  assert.equal(h.writes(),0); assert.match(h.popup().textContent,/Less than 10 GB/);
  h.button("change…").onclick(); await h.choose("Y:/RimArcho");
  assert.match(h.popup().textContent,/Y:\/RimArcho/); assert.doesNotMatch(h.popup().textContent,/Less than 10 GB/);
  assert.equal(h.writes(),0); await h.button("use this folder").onclick(); await done; assert.equal(h.writes(),1);
});

test("declining relocation keeps one warning line and copies nothing", async () => {
  const h = await setup(true); const done = h.module.workshopStart(); await flush();
  assert.match(h.popup().textContent,/already in this folder will be replaced/);
  h.button("use here this time").onclick(); assert.equal(await done,true); assert.equal(h.moves(),0);
  assert.match(h.app.querySelector("#workshop-warning").textContent,/self-update is unavailable/);
});

test("failed relocation stays in the dialog for retry", async () => {
  const h = await setup(true); h.fail(); const done = h.module.workshopStart(); await flush();
  assert.equal(h.moves(),0); await h.button("move and restart").onclick();
  assert.match(h.popup().textContent,/copy failed/); assert.equal(h.button("move and restart").disabled,false);
  h.button("use here this time").onclick(); assert.equal(await done,true);
});
