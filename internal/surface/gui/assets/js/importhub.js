// The Import page (16 §2.18): every kind of import in one place, under Library. It holds no
// import logic of its own: each box opens the flow that does the work (importways.js), and
// a file dropped on the window still goes to the list importer (16 §2.17).
import { $, el, T, say } from "./state.js";
import { page, beginPageRead } from "./screens.js";
import { gameWay, managersWay, listsWay, packWay } from "./importways.js";

let ui = null;
export function initImportHub(u) { ui = u; }

export function open() {
  const read = beginPageRead("import");
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = page("import-hub", T("bring what you have into RimArcho; every import is previewed first and none writes the game's files"));
  const ways = el("div", "ways import-ways");
  ways.append(gameWay(), managersWay(), listsWay(), packWay(async (text) => {
    if (text) say("ok", text);
    await ui.reload();
    ui.go("packs");
  }));
  w.append(ways, el("div", "xc-hint", T("or drop a file anywhere in the window")));
  sc.append(w);
  read.finish();
  ui.renderWhere();
}
