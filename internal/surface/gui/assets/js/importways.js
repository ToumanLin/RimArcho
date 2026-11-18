// The ways to bring something into RimArcho, shared by the Import page and the first-run
// guide's last step (16 §2.18, §2.15): each way is one box with a title, a line on what it
// brings and its buttons on a shared bottom line. A way only starts a flow that already
// exists (the guide, the list importer, the manager import, pack import); none writes by
// itself.
import { S, el, api, T, Tf, errText, say } from "./state.js";
import { button } from "./screens.js";
import { importFrom } from "./exchange.js";

let ui = null;
export function initImportWays(u) { ui = u; }

// way is one choice: its title, what it does, then its buttons at the bottom, level with the
// other choices' buttons (extra goes between the text and the buttons).
export function way(title, text, actions, extra) {
  const d = el("div", "way");
  d.append(el("div", "t", title), el("div", "muted", text));
  if (extra) d.append(extra);
  const row = el("div", "way-acts");
  for (const a of [].concat(actions)) if (a) row.append(a);
  d.append(row);
  return d;
}

// found says what discovery found, per manager: "found RimSort: 4" (lists, rules, data).
export function foundText(rows) {
  const by = new Map();
  for (const r of rows || []) by.set(r.manager, (by.get(r.manager) || 0) + 1);
  if (!by.size) return T("No RimSort or RimCrow data found.");
  return [...by].map(([m, n]) => Tf("found %s: %d", m, n)).join(", ");
}

// managersWay: lists and player data from RimSort or RimCrow. Discovery runs while the box
// shows (read-only, 07 §6.2) so the box says what is there before anything is opened.
// before runs first (the guide sets where an applied import goes).
export function managersWay(before = () => {}) {
  const status = el("div", "way-found muted", T("looking for RimSort and RimCrow…"));
  const shown = S.screen;
  api.DiscoverManagerImports().then((rows) => {
    if (S.screen !== shown) return;
    status.textContent = foundText(rows);
    if (rows?.length) status.classList.add("on");
  }).catch((e) => { status.textContent = errText(e); });
  const go = button(T("import…"), "", () => { before(); ui.importManagers(); });
  return way(T("move over from RimSort or RimCrow"), T("lists and player data, previewed before anything is imported"), go, status);
}

// listsWay: a list in any format the importer reads (07 §7.2), pasted or from a file.
export function listsWay(before = () => {}) {
  const src = el("input", "field");
  src.placeholder = T("a share code, a collection or Rentry link, packageIds…");
  src.spellcheck = false;
  const read = () => {
    const v = src.value.trim();
    if (!v) return src.focus();
    before();
    importFrom("text", v);
  };
  src.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); read(); } };
  const row = el("div", "typed");
  row.append(src, button(T("open"), "", read));
  const file = button(T("choose a file…"), "", async () => {
    let path = "";
    try { path = await api.PickImportFile(); } catch (e) { return say("fail", errText(e)); }
    if (path) { before(); importFrom("file", path); }
  });
  return way(T("a mod list"), T("RimSort, RimCrow and RimWorld lists, saves, share codes, Steam collections and Rentry; previewed first"), file, row);
}

// packWay: an .archopack, imported and checked (10 §4); done gets the result line.
export function packWay(done) {
  const pick = button(T("choose a pack…"), "", async () => {
    const screen = S.screen;
    try {
      const path = await api.PickPackFile();
      if (path && S.screen === screen) await done(await api.PackImport(path));
    } catch (e) { if (S.screen === screen) say("fail", errText(e)); }
  });
  return way(T("a pack"), T("an .archopack with its mods, order and settings; its files are checked first"), pick);
}

// gameWay: the games RimArcho manages, and adding one (the guide's first steps, 16 §2.15).
export function gameWay() {
  const envs = (S.board && S.board.envs) || [];
  const text = envs.length ? Tf("added: %s", envs.map((e) => e.name || e).join(", ")) : T("no game added yet");
  return way(T("a game"), T("a RimWorld on this computer, a folder you choose, or a version downloaded with SteamCMD"),
    button(T("add a game"), "", () => ui.go("guide")), el("div", "way-found muted", text));
}
