// Settings › Folders in folds (16 §2.14): the directory rows grouped under their root
// (view.FolderFold). A heading shows the root, how many folders it holds, their recorded
// size and, when they share one drive, its meter; clicking it or Enter on its chevron folds
// and unfolds it. The player's choice is kept per fold (view.FoldsKey).
import { copyablePath } from "./fields.js";
import { factText, el, api, T } from "./state.js";
import { diskMeter } from "./membar.js";
import { icon } from "./icons.js";

const FOLDS_KEY = "ui.gui.settings.folders";

// headMeters: the rows whose drive meter their fold's heading draws instead.
export function headMeters(g) {
  const out = new Set();
  for (const f of g.folds || []) if (f.disk) for (const i of f.rows) out.add(i);
  return out;
}

// settingValue writes a row's value: a row in a fold shows its folder under the fold's root;
// hover and copy keep the full path.
export function settingValue(node, it) {
  if (it.rel && it.rel !== it.value) {
    factText(node, { text: it.rel, paths: [{ text: it.rel, path: it.value }] });
    node.title = it.value;
    return node;
  }
  return factText(node, { text: it.value || "", paths: it.valuePath || it.edit === "url" ? [it.value] : [] });
}

// folderFolds appends g's folds to card, each with its rows (rows[i] are item i's elements);
// rows in no fold (adding a folder) follow.
export function folderFolds(card, g, rows) {
  const placed = new Set();
  for (const f of g.folds) {
    const box = el("div", "folder-fold" + (f.attention ? " attention" : ""));
    box.dataset.fold = f.key;
    const head = el("div", "folder-fold-head");
    const toggle = el("button", "folder-fold-toggle");
    toggle.type = "button";
    const chev = el("span", "fold-chev", "›");
    chev.setAttribute("aria-hidden", "true"); // the state is aria-expanded
    toggle.append(chev);
    const root = f.root ? copyablePath(el("span", "folder-fold-root", f.root), f.root) : el("span", "folder-fold-root", f.title || "");
    root.id = "folder-fold-" + f.key;
    const count = el("span", "folder-fold-count muted num", f.count);
    count.id = root.id + "-count";
    toggle.setAttribute("aria-labelledby", root.id + " " + count.id);
    head.append(toggle, root, count);
    if (f.attention) {
      // a row needs attention: the mark says so folded or not; the row says what
      const warn = el("span", "folder-fold-warn");
      warn.innerHTML = icon.warn;
      warn.setAttribute("aria-hidden", "true");
      head.append(warn);
    }
    if (f.size) {
      const size = el("span", "folder-fold-size muted num", f.size);
      if (f.hover) size.title = f.hover;
      head.append(size);
    }
    if (f.disk) head.append(diskMeter(f.disk));
    const body = el("div", "folder-fold-rows");
    for (const i of f.rows) {
      for (const n of rows[i] || []) body.append(n);
      placed.add(i);
    }
    const show = (open) => {
      box.classList.toggle("open", open);
      body.hidden = !open;
      toggle.setAttribute("aria-expanded", String(open));
      toggle.title = T(open ? "hide" : "show");
    };
    show(f.open);
    toggle.onclick = (e) => {
      e.stopPropagation();
      f.open = !f.open;
      show(f.open);
      keep(f.key, f.open);
    };
    // the whole heading folds, except its copy button and the path's text
    head.onclick = (e) => { if (!e.target.closest(".path-copy")) toggle.click(); };
    box.append(head, body);
    card.append(box);
  }
  rows.forEach((ns, i) => { if (!placed.has(i)) for (const n of ns) card.append(n); });
}

// keep writes one fold's choice, leaving the other folds' choices (or their defaults) alone.
// Writes go one after another, so quick clicks on two headings keep both.
let keeping = Promise.resolve();
function keep(key, open) {
  keeping = keeping.then(async () => {
    let m = {};
    try { m = JSON.parse((await api.Setting(FOLDS_KEY)) || "{}") || {}; } catch { m = {}; }
    m[key] = open;
    await api.SetSetting(FOLDS_KEY, JSON.stringify(m));
  }).catch(() => {});
}
