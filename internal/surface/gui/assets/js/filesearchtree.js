// Append-only result tree for Search files (16 §2.9).
import { el, T } from "./state.js";
import { button, hintsChanged } from "./screens.js";
import { virtualRows } from "./virtualrows.js";
import { appendParts } from "./filesearchview.js";
export function many(file) {
  return file.count > 1 && file.lines?.some((line) => line.line > 0);
}

// Virtual rows share the page's cursor; only this controller owns result DOM.
export function createResults(X, resultsCard, viewer, query, { key, stop, run }) {
  let tree, summary, notes, stopButton, treeRows;
  function buildResults() {
    const head = el("div", "fs-result-head");
    summary = el("span", "fs-summary muted");
    summary.setAttribute("aria-live", "polite");
    stopButton = button(T("stop"), "esc", stop, "quiet small fs-stop");
    stopButton.hidden = true;
    head.append(el("h2", "", T("results")), summary, stopButton);
    notes = el("div", "fs-notes muted");
    tree = el("div", "fs-tree");
    tree.tabIndex = 0;
    tree.setAttribute("role", "tree");
    tree.setAttribute("aria-label", T("results"));
    tree.onkeydown = (e) => {
      if (key(e)) {
        e.stopPropagation();
      }
    };
    resultsCard.append(head, notes, tree);
  }
  // Append each copy once, then grow its files and count without reordering it.
  function applyDeltas(deltas) {
    for (const delta of deltas || []) {
      let mod = X.byID.get(delta.id);
      if (!mod) {
        mod = {
          id: delta.id,
          name: delta.name,
          copy: delta.copy,
          count: 0,
          files: []
        };
        X.byID.set(mod.id, mod);
        X.modsFound.push(mod);
      }
      const file = {
        ...delta.file,
        mod,
        id: delta.id + "\n" + delta.file.path
      };
      mod.files.push(file);
      mod.count += file.count;
    }
  }
  // Include match rows only for expanded files; Mod headings take no cursor position.
  function flatten() {
    const rows = [];
    for (const mod of X.modsFound) {
      rows.push({
        type: "mod",
        mod
      });
      for (const file of mod.files) {
        rows.push({
          type: "file",
          mod,
          file,
          line: 0
        });
        if (X.expanded.has(file.id)) {
          for (const hit of file.lines || []) {
            rows.push({
              type: "line",
              mod,
              file,
              line: hit.line,
              hit
            });
          }
        }
      }
    }
    return rows;
  }
  // Preserve the selected file and scroll position when streaming rebuilds the visible rows.
  function drawTree() {
    const saved = treeRows?.snapshot(),
      selected = X.rows[X.cursor];
    treeRows?.destroy();
    treeRows = null;
    X.rows = flatten();
    if (selected?.file) {
      const i = X.rows.findIndex((r) => r.file?.id === selected.file.id && r.line === selected.line);
      if (i >= 0) {
        X.cursor = i;
      }
    }
    X.cursor = Math.max(0, Math.min(X.rows.length - 1, X.cursor));
    tree.replaceChildren();
    if (!X.rows.length) {
      if (X.update?.done && !X.update.error) {
        if (X.update.hint) {
          tree.append(el("p", "fs-note muted", X.update.hint));
          if (X.at?.modsQuery.trim()) {
            tree.append(
              button(
                T("search every installed copy"),
                "",
                () => {
                  X.mods = "";
                  query.modsBar.setText("");
                  query.refreshFilters();
                  run();
                },
                "quiet small fs-widen"
              )
            );
          }
        }
      }
      return;
    }
    const sizes = new Float64Array(X.rows.length).fill(24);
    treeRows = virtualRows(tree, X.rows.length, renderTreeRow, {
      heights: sizes
    });
    if (saved) {
      tree.scrollTop = saved.scrollTop;
    }
    if (!X.selected) {
      const first = X.rows.findIndex((r) => r.type === "file");
      if (first >= 0) {
        selectRow(first, false);
      }
    }
    treeRows.select(X.cursor, false);
  }
  function renderTreeRow(i) {
    const row = X.rows[i],
      node = el(row.type === "mod" ? "div" : "button", "fs-tree-row fs-" + row.type);
    node.id = "search-row-" + encodeURIComponent(JSON.stringify([row.type, row.mod.id, row.file?.path, row.line]));
    node.setAttribute("role", "treeitem");
    node.setAttribute("aria-level", String(row.type === "mod" ? 1 : row.type === "file" ? 2 : 3));
    if (row.type === "mod") {
      node.append(
        el("span", "fs-mod-name", row.mod.name),
        el("span", "fs-copy muted", row.mod.copy),
        el("span", "fs-count muted", String(row.mod.count))
      );
      node.setAttribute("aria-expanded", "true");
      return node;
    }
    node.tabIndex = -1;
    if (row.type === "file") {
      const expandable = many(row.file),
        expanded = X.expanded.has(row.file.id);
      const chevron = el("span", "fs-chevron" + (expanded ? " open" : "") + (!expandable ? " none" : ""), "›");
      chevron.setAttribute("aria-hidden", "true");
      const path = el("span", "fs-path"),
        at = row.file.rel.lastIndexOf("/");
      path.append(
        el("span", "muted", row.file.rel.slice(0, at + 1)),
        document.createTextNode(row.file.rel.slice(at + 1))
      );
      node.append(chevron, path, el("span", "fs-count muted", String(row.file.count)));
      if (expandable) {
        node.setAttribute("aria-expanded", String(expanded));
      }
      node.onclick = () => {
        if (expandable) {
          expanded ? X.expanded.delete(row.file.id) : X.expanded.add(row.file.id);
        }
        selectRow(i, false);
        drawTree();
        tree.focus({
          preventScroll: true
        });
      };
    } else {
      node.append(el("span", "fs-line-number muted", String(row.line)));
      const text = el("span", "fs-line-text");
      appendParts(
        text,
        row.hit.parts || [
          {
            text: row.hit.text
          }
        ]
      );
      node.append(text);
      node.onclick = () => {
        selectRow(i, false);
        tree.focus({
          preventScroll: true
        });
      };
    }
    return node;
  }
  function drawStatus() {
    summary.textContent = X.update?.summary || "";
    stopButton.hidden = !X.running;
    notes.replaceChildren(...(X.update?.notes || []).map((text) => el("p", "fs-note", text)));
    if (X.update?.error) {
      notes.append(el("p", "fs-note fail", X.update.error));
    }
    hintsChanged();
  }
  // Read on a file change; moving between its lines reuses the bounded viewer text.
  function selectRow(i, reveal = true) {
    const row = X.rows[i];
    if (!row?.file) {
      return;
    }
    X.cursor = i;
    treeRows?.select(i, reveal);
    const changed = X.selected?.id !== row.file.id;
    X.selected = row.file;
    X.line = row.type === "line" ? row.line : row.file.lines?.[0]?.line || 0;
    // Selection reads just this file; repeated line selections reuse its bounded text.
    if (changed) {
      viewer.readFile(row.file);
    } else {
      viewer.markSelected();
    }
  }
  return {
    buildResults,
    applyDeltas,
    drawTree,
    drawStatus,
    selectRow,
    get element() {
      return tree;
    },
    focusResults() {
      tree?.focus();
      if (X.rows.length && X.rows[X.cursor]?.type === "mod") {
        selectRow(X.rows.findIndex((row) => row.type === "file"));
      }
    },
    destroy() {
      treeRows?.destroy();
      treeRows = null;
    }
  };
}
