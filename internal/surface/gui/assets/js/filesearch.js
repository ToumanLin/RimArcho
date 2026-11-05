// Mod library › Search files (16 §2.9). Query completion and filters are shared with the library.
import { S, $, el, api, T, errText, say } from "./state.js";
import { beginPageRead, page, area, hintsChanged } from "./screens.js";
import { copy } from "./fields.js";
import { createQuery } from "./filesearchquery.js";
import { wireSeparator } from "./separator.js";
import { createResults, many } from "./filesearchtree.js";
import { createViewer } from "./filesearchview.js";
const X = {
  text: "",
  files: "Defs patches About",
  mods: "",
  case: false,
  regex: false,
  initialized: false,
  env: null,
  groups: [],
  modsFound: [],
  byID: new Map(),
  rows: [],
  expanded: new Set(),
  cursor: 0,
  selected: null,
  line: 0,
  at: null,
  update: null,
  running: false,
  request: null,
  since: 0,
  seq: 0,
  visit: 0,
  readSeq: 0,
  filterSeq: 0,
  wrap: false,
  wrapTouched: false,
  timer: 0,
  filterTimer: 0
};
let root, queryCard, resultsCard, viewerCard;
let observer, viewer, query, results;
let begins = Promise.resolve();
const visible = () => S.screen === "search" && root?.isConnected;
const consume = (e) => {
  e.preventDefault();
  e.stopPropagation();
};
// Build the page before loading its stored inventory and settings.
export async function open() {
  leave();
  const visit = ++X.visit;
  const read = beginPageRead("search", {
    label: "loading…",
    retry: open
  });
  root = page("filesearch", "");
  root.replaceChildren();
  const left = el("div", "fs-left"),
    gap = el("div", "fs-split"),
    vgap = el("div", "hsplit fs-vsplit");
  queryCard = el("section", "card fs-query");
  resultsCard = el("section", "card fs-results");
  viewerCard = el("section", "card fs-viewer");
  viewer = createViewer(X, viewerCard, visible);
  query = createQuery(X, queryCard, {
    run,
    shortcuts,
    stop,
    focusResults,
    visible,
    consume,
    gap: vgap,
    layout: () => placeQuery(),
    nudge: (d) => setQuery(queryCard.getBoundingClientRect().height + d, true)
  });
  results = createResults(X, resultsCard, viewer, query, {
    key,
    stop,
    run
  });
  left.append(queryCard, vgap, resultsCard);
  root.append(left, gap, viewerCard);
  $("#screen").replaceChildren(root);
  query.buildQuery();
  results.buildResults();
  // Cards 1 query, 2 results, 3 file (16 §3.1): the digit keys focus them.
  area(queryCard, queryCard.querySelector(".fs-field"), focusInput);
  area(resultsCard, resultsCard.querySelector(".fs-result-head"), focusResults);
  area(viewerCard, null, viewer.focus);
  viewer.draw();
  // The gap under the query card drags while the filter panel is open; closed, the card
  // follows its three fields (16 §3.1 rule 10). Default 38% of the column.
  let queryH = null;
  const placeQuery = () => {
    const open = !!query.modsBar?.open;
    left.classList.toggle("filters-open", open);
    left.style.setProperty("--fs-query-h", queryH === null ? "38%" : queryH + "px");
    vsep.refresh();
  };
  const queryRange = () => [160, Math.max(160, left.clientHeight - 140)];
  const setQuery = (v, save) => {
    const [min, max] = queryRange();
    queryH = Math.round(Math.max(min, Math.min(max, v)));
    placeQuery();
    if (save) {
      api.SetSetting("ui.gui.search.filters", String(queryH)).catch(() => {});
    }
  };
  const vsep = wireSeparator(vgap, {
    axis: "y",
    key: "ui.gui.search.filters",
    enabled: () => innerWidth >= 900 && !!query.modsBar?.open,
    range: queryRange,
    get: () => queryCard.getBoundingClientRect().height,
    set: (v) => setQuery(v, false),
    reset: () => {
      queryH = null;
      placeQuery();
    },
    serialize: () => (queryH === null ? "" : String(queryH)),
    restore: (v) => {
      queryH = v && Number.isFinite(+v) ? +v : null;
      placeQuery();
    }
  });
  placeQuery();
  let side = null;
  const width = () => root.getBoundingClientRect().width;
  const place = () => root.style.setProperty("--fs-side", side === null ? "38%" : side + "px");
  wireSeparator(gap, {
    key: "ui.gui.search.side",
    enabled: () => innerWidth >= 900,
    range: () => [260, Math.max(260, width() - 300)],
    get: () => side ?? width() * 0.38,
    set: (v) => {
      side = v;
      place();
    },
    reset: () => {
      side = null;
      place();
    },
    serialize: () => (side === null ? "" : String(side)),
    restore: (v) => {
      side = v && Number.isFinite(+v) ? +v : null;
      place();
    }
  });
  place();
  observer = new MutationObserver(() => {
    if (!visible()) {
      leave();
    }
  });
  observer.observe($("#screen"), {
    childList: true
  });
  read.finish();
  api
    .Setting("ui.gui.search.wrap")
    .then((v) => {
      if (visible() && visit === X.visit && !X.wrapTouched) {
        X.wrap = v === "true";
        viewer.applyWrap();
      }
    })
    .catch(() => {});
  try {
    const info = await api.FileSearchInfo();
    if (!visible() || visit !== X.visit) {
      return;
    }
    if (!X.initialized || X.env !== S.envSeq) {
      X.files = info.files;
      X.mods = info.mods;
      X.env = S.envSeq;
      X.initialized = true;
    }
    X.groups = info.groups || [];
    query.filesBar.setText(X.files);
    query.modsBar.setText(X.mods);
    query.refreshPanel();
    if (X.mods !== info.mods) {
      query.refreshFilters();
    }
  } catch (e) {
    if (visible() && visit === X.visit) {
      say("fail", errText(e));
    }
  }
  hintsChanged();
}
// Capture the current fields and sequence a replacement without accepting stale responses.
export async function run() {
  X.text = query.textInput.value;
  X.files = query.filesBar.input.value;
  X.mods = query.modsBar.input.value;
  if (!query.validateKinds()) {
    return;
  }
  if (!X.text.trim()) {
    say("fail", T("type what to look for"));
    return;
  }
  const q = query.request(),
    seq = ++X.seq,
    visit = X.visit;
  clearTimeout(X.timer);
  X.running = true;
  X.stopWanted = false;
  X.request = null;
  X.since = 0;
  X.at = q;
  X.modsFound = [];
  X.byID.clear();
  X.rows = [];
  X.expanded.clear();
  X.cursor = 0;
  X.selected = null;
  X.line = 0;
  X.update = {
    summary: "",
    done: false
  };
  ++X.readSeq;
  results.drawTree();
  viewer.draw();
  results.drawStatus();
  const current = () => visible() && visit === X.visit && seq === X.seq;
  try {
    const n = await (begins = begins.catch(() => {}).then(() => api.BeginFileSearch()));
    if (!current()) {
      api.StopFileSearch(n).catch(() => {});
      return;
    }
    X.request = n;
    await api.StartFileSearch(q, n);
    if (X.stopWanted) {
      await api.StopFileSearch(n);
    }
    if (current()) {
      poll(seq, visit, n);
    }
  } catch (e) {
    if (current()) {
      X.running = false;
      say("fail", errText(e));
      results.drawStatus();
    }
  }
}
// Consume only new deltas while this page visit and request still own the results.
async function poll(seq, visit, n) {
  if (!visible() || seq !== X.seq || visit !== X.visit) {
    return;
  }
  try {
    const update = await api.FileSearchUpdate(n, X.since);
    if (!visible() || seq !== X.seq || visit !== X.visit) {
      return;
    }
    X.since = update.next;
    X.update = update;
    X.running = !update.done;
    if (update.deltas?.length) {
      results.applyDeltas(update.deltas);
      results.drawTree();
    } else if (update.done && !X.rows.length) {
      results.drawTree();
    }
    results.drawStatus();
    if (!update.done) {
      X.timer = setTimeout(() => poll(seq, visit, n), 300);
    }
  } catch (e) {
    if (visible() && seq === X.seq) {
      X.running = false;
      results.drawStatus();
      say("fail", errText(e));
    }
  }
}
function stop() {
  X.stopWanted = true;
  if (X.running && X.request !== null) {
    api.StopFileSearch(X.request).catch((e) => say("fail", errText(e)));
  }
}
// Invalidate pending reads before releasing page state and listeners.
export function leave() {
  ++X.seq;
  ++X.visit;
  ++X.readSeq;
  ++X.filterSeq;
  clearTimeout(X.timer);
  clearTimeout(X.filterTimer);
  if (X.request !== null) {
    api.StopFileSearch(X.request).catch(() => {});
  }
  X.request = null;
  X.running = false;
  X.selected = null;
  viewer?.clear();
  X.update = null;
  X.modsFound = [];
  X.byID.clear();
  X.rows = [];
  X.expanded.clear();
  observer?.disconnect();
  observer = null;
  results?.destroy();
  query?.destroy();
}
function focusResults() {
  results?.focusResults();
}
export function applyDeltas(deltas) {
  results.applyDeltas(deltas);
}
export function focusInput() {
  query.textInput?.focus();
  query.textInput?.select();
}
// Handle shortcuts shared by fields, tree and viewer.
function shortcuts(e) {
  const ctrl = e.ctrlKey || e.metaKey,
    k = e.key.toLowerCase();
  if (ctrl && !e.altKey && k === "f") {
    consume(e);
    focusInput();
    return true;
  }
  if (e.altKey && !ctrl) {
    if (k === "c" || k === "r") {
      consume(e);
      query.toggle(k === "c" ? "case" : "regex");
      return true;
    }
    if (k === "z") {
      consume(e);
      viewer.toggleWrap();
      return true;
    }
  }
  return false;
}
// Keep native button activation local, and route result navigation through the shared cursor.
export function key(e) {
  if (shortcuts(e)) {
    return true;
  }
  const k = e.key,
    ctrl = e.ctrlKey || e.metaKey;
  if (k === "Escape" && X.running) {
    consume(e);
    stop();
    return true;
  }
  if (e.target?.tagName === "INPUT" || e.altKey) {
    return false;
  }
  if ((k === "Enter" || k === " ") && e.target?.closest?.("button") && !results.element?.contains(e.target)) {
    return false;
  }
  // Card 3 focused on the file text: the arrows and page keys scroll it.
  if (e.target?.closest?.(".fs-code") && /^(Arrow(Up|Down|Left|Right)|Page(Up|Down)|Home|End)$/.test(k)) {
    return false;
  }
  // ctrl+c over selected file text copies that text, not the path.
  if (ctrl && k.toLowerCase() === "c" && e.target?.closest?.(".fs-code") && String(getSelection())) {
    return false;
  }
  if (k === "\\") {
    consume(e);
    query.modsBar.toggle();
    return true;
  }
  if (k === "/") {
    consume(e);
    focusInput();
    return true;
  }
  if (ctrl && k.toLowerCase() === "o" && X.selected) {
    consume(e);
    viewer.openFile(true);
    return true;
  }
  if (ctrl && k.toLowerCase() === "c" && X.selected) {
    consume(e);
    copy(X.selected.path);
    return true;
  }
  if (k === "Enter" && X.selected) {
    consume(e);
    viewer.openFile(false);
    return true;
  }
  if ((k === "n" || k === "N") && viewer?.hasMatches()) {
    consume(e);
    viewer.nextMatch(k === "n" ? 1 : -1);
    return true;
  }
  const row = X.rows[X.cursor];
  if (!row) {
    return false;
  }
  if ((k === "ArrowRight" || k === "ArrowLeft") && row.file) {
    consume(e);
    if (k === "ArrowRight" && many(row.file)) {
      X.expanded.add(row.file.id);
    }
    if (k === "ArrowLeft") {
      X.expanded.delete(row.file.id);
      const i = X.rows.findIndex((r) => r.type === "file" && r.file.id === row.file.id);
      if (i >= 0) {
        X.cursor = i;
      }
    }
    // UIA Expand/Collapse dispatches a key to the row; retain its native target until dispatch ends.
    requestAnimationFrame(() => { if (visible()) results.drawTree(); });
    return true;
  }
  let i = X.cursor,
    d = 0;
  if (k === "ArrowDown" || k === "j") {
    d = 1;
  } else if (k === "ArrowUp" || k === "k") {
    d = -1;
  } else if (k === "Home") {
    i = 0;
    d = 1;
  } else if (k === "End") {
    i = X.rows.length - 1;
    d = -1;
  } else if (k === "PageDown") {
    d = Math.max(1, Math.floor(results.element.clientHeight / 24));
  } else if (k === "PageUp") {
    d = -Math.max(1, Math.floor(results.element.clientHeight / 24));
  } else {
    return false;
  }
  consume(e);
  if (k !== "Home" && k !== "End") {
    i = Math.max(0, Math.min(X.rows.length - 1, i + d));
  }
  while (X.rows[i]?.type === "mod") {
    const next = i + (d < 0 ? -1 : 1);
    if (next < 0 || next >= X.rows.length) {
      return true;
    }
    i = next;
  }
  results.selectRow(i);
  return true;
}
export function hintList() {
  const list = [
    {
      key: "↑↓",
      label: T("move the cursor")
    }
  ];
  if (X.selected) {
    list.push(
      {
        key: "enter",
        label: T("open file")
      },
      {
        key: "ctrl+o",
        label: T("show in folder")
      },
      {
        key: "ctrl+c",
        label: T("copy the path")
      }
    );
  }
  if (viewer?.hasMatches()) {
    list.push({
      key: "n N",
      label: T("next / previous match")
    });
  }
  if (X.running) {
    list.push({
      key: "esc",
      label: T("stop")
    });
  }
  list.push(
    {
      key: "\\",
      label: T("filter")
    },
    {
      key: "?",
      label: T("all keys")
    }
  );
  return list;
}
