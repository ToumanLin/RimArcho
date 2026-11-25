import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, deferred, loadModules } from "./dom.mjs";

const el = (tag, cls, text) => new Element(tag, cls, text);
const key = (value, extra) => ({ key: value, preventDefault() {}, stopPropagation() {}, ...extra });

test("row menu opens alias and note actions only for a single eligible Mod", async () => {
  const ops = await loadModules("./ops.js", { "./state.js": { S: {}, T: s => s }, "./popups.js": {} });
  ops.setTable([{ code: "tags", label: "tags", group: 2 }, { code: "note", label: "note", key: "n", group: 2, one: true }]);
  const calls = [];
  const menu = (codes, data) => ops.menuItems(codes, c => calls.push(c), data);
  const add = menu(["tags", "note"], {});
  assert.equal(add.map(i => i.label).join("|"), "tags|add alias…|add note…");
  add[1].run();
  add[2].run();
  assert.deepEqual(calls, ["alias", "note"]);
  const edit = menu(["note"], { alias: true, note: "remember" });
  assert.equal(edit.map(i => i.label).join("|"), "edit alias…|edit note…");
  assert.equal(edit[1].key, "n");
  assert.equal(menu(["tags"], {}).map(i => i.label).join("|"), "tags");
});

async function frame(fields = [], options = {}, api = {}) {
  const host = el("div"), writes = [], refreshes = [], errors = [];
  const detail = await loadModules("./detail.js", {
    "./state.js": {
      S: {}, $: () => host, el, T: s => s, Tf: (s, v) => s.replace("%s", v), errText: String, factText() {},
      api: {
        SetModAlias: async (...args) => { writes.push(["alias", ...args]); return "saved"; },
        SetModNote: async (...args) => { writes.push(["note", ...args]); return "saved"; },
        LibrarySetNote: async (...args) => { writes.push(["copy note", ...args]); return "saved"; },
        ...api,
      },
      say: (kind, msg) => { if (kind === "fail") errors.push(msg); },
    },
    "./icons.js": { icon: { plus: "+", edit: "pencil" }, sourceIcon: () => "", statusIcon: () => "" },
    "./fields.js": { fieldList: () => el("div") },
    "./ops.js": { sorted: () => [] },
  });
  const f = detail.detailFrame(host, null, { playerDataSaved: () => refreshes.push("saved"), ...options });
  f.head("The original Mod", "", "test-id");
  f.fields(fields);
  return { host, f, writes, refreshes, errors };
}

test("alias editing trims and saves on Enter, cancels on Escape, and clears with empty input", async () => {
  const { host, writes, refreshes } = await frame();
  const edit = () => {
    host.querySelector(".md-alias button").onclick();
    return host.querySelector(".mod-alias-in");
  };
  assert.equal(host.querySelector(".md-alias button").textContent, "add alias");
  let input = edit();
  input.value = "temporary";
  await input.onkeydown(key("Escape"));
  assert.equal(host.querySelector(".mod-alias-in"), null);
  assert.deepEqual(writes, []);
  input = edit();
  input.value = "  Custom name  ";
  await input.onkeydown(key("Enter", { isComposing: true }));
  assert.deepEqual(writes, []);
  await input.onkeydown(key("Enter"));
  assert.equal(host.querySelector(".mod-original-name").textContent, "original name: The original Mod");
  assert.equal(host.querySelector(".md-alias button").textContent, "edit alias");
  input = edit();
  assert.equal(input.value, "Custom name");
  input.value = "";
  await input.onkeydown(key("Enter"));
  assert.deepEqual(writes, [["alias", "test-id", "Custom name"], ["alias", "test-id", ""]]);
  assert.deepEqual(refreshes, ["saved", "saved"]);
  assert.equal(host.querySelector(".mod-original-name"), null);
});

test("alias editor retains the draft when a write is refused", async () => {
  const write = deferred();
  const { host, errors } = await frame([{ kind: "alias", value: "Existing", extra: "The original Mod" }], {}, { SetModAlias: () => write.promise });
  host.querySelector(".md-alias button").onclick();
  const input = host.querySelector(".mod-alias-in");
  input.value = "new value";
  const saving = input.onkeydown(key("Enter"));
  assert.equal(input.readOnly, true);
  write.reject(new Error("write refused"));
  await saving;
  assert.equal(input.readOnly, false);
  assert.equal(host.querySelector(".mod-alias-in").value, "new value");
  assert.match(errors[0], /write refused/);
});

test("the editor displays the Lineage note and the library displays the copy note", async () => {
  const editor = await frame([], { modNote: true });
  const { host, writes, f } = editor;
  assert.equal(host.querySelector(".md-note button").textContent, "add note");
  host.querySelector(".md-note button").onclick();
  let area = host.querySelector("textarea");
  area.value = "First line\nSecond line";
  f.fields([]);
  assert.equal(host.querySelector("textarea"), area);
  area.onkeydown(key("Enter", { shiftKey: true }));
  assert.deepEqual(writes, []);
  area.onkeydown(key("Enter"));
  await new Promise(setImmediate);
  assert.equal(host.querySelector(".md-note .txt").textContent, "First line\nSecond line");
  assert.deepEqual(writes, [["note", "test-id", "First line\nSecond line"]]);
  f.inst({ id: "test-id", ref: "#test", note: "Existing copy note" });
  assert.equal(host.querySelector(".md-copy-note .txt").textContent, "Existing copy note");
  assert.equal(host.querySelector(".md-copy-note button").textContent, "edit copy note");
  const library = await frame();
  library.f.inst({ id: "test-id", ref: "#test", auto: "Generated note" });
  assert.equal(library.host.querySelector(".md-note .txt").textContent, "Generated note");
  library.host.querySelector(".md-note button").onclick();
  area = library.host.querySelector("textarea");
  area.value = "My copy note";
  area.onkeydown(key("Enter"));
  await new Promise(setImmediate);
  assert.deepEqual(library.writes, [["copy note", "#test", "My copy note"]]);
  assert.equal(library.host.querySelector(".md-note .txt").textContent, "My copy note");
});

// Release 1.1.4 VM round: "add note" on a copy whose box shows RimArcho's automatic text
// started the editor with that text, caret at its end, so the typed note ran on from it.
test("add note starts empty with the automatic text as the hint; edit note keeps the player's text", async () => {
  const library = await frame();
  library.f.inst({ id: "test-id", ref: "#test", auto: "Workshop subscription, first seen Nov 21" });
  library.host.querySelector(".md-note button").onclick();
  let area = library.host.querySelector("textarea");
  assert.equal(area.value, "");
  assert.equal(area.placeholder, "Workshop subscription, first seen Nov 21");
  area.onkeydown(key("Enter")); // nothing typed: nothing saved, the automatic text stays
  await new Promise(setImmediate);
  assert.deepEqual(library.writes, []);
  assert.equal(library.host.querySelector(".md-note .txt").textContent, "Workshop subscription, first seen Nov 21");
  library.host.querySelector(".md-note button").onclick();
  area = library.host.querySelector("textarea");
  area.value += "menu note R114";
  area.onkeydown(key("Enter"));
  await new Promise(setImmediate);
  assert.deepEqual(library.writes, [["copy note", "#test", "menu note R114"]]);

  const own = await frame();
  own.f.inst({ id: "test-id", ref: "#test", note: "Player's note", auto: "Generated note" });
  own.host.querySelector(".md-note button").onclick();
  area = own.host.querySelector("textarea");
  assert.equal(area.value, "Player's note");
  area.value += " and more";
  area.onkeydown(key("Enter"));
  await new Promise(setImmediate);
  assert.deepEqual(own.writes, [["copy note", "#test", "Player's note and more"]]);
});
