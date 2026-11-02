// Bounded text preview and actions for Search files (06 §2.9).
import { el, api, T, Tf, errText, say } from "./state.js";
import { button, hintsChanged } from "./screens.js";
import { copyablePath } from "./fields.js";
import { virtualRows } from "./virtualrows.js";
import { syntaxParts } from "./searchsyntax.js";
// Render highlights and optional syntax tokens as text nodes.
export function appendParts(node, parts, language = null) {
  for (const part of parts) {
    if (part.hit) {
      const mark = el("mark");
      mark.textContent = part.text;
      node.append(mark);
    } else if (language) {
      for (const token of syntaxParts(part.text, language)) {
        const span = el("span", token.cls);
        span.textContent = token.text;
        node.append(span);
      }
    } else {
      node.append(document.createTextNode(part.text));
    }
  }
}

// The page owns selection and request sequences; the viewer owns only its bounded text and DOM.
export function createViewer(X, viewerCard, visible) {
  let codeRows;
  let fileText = null;
  // Refresh hints before and after the guarded file read so match keys follow its contents.
  async function readFile(file) {
    const seq = ++X.readSeq,
      visit = X.visit;
    fileText = null;
    draw();
    hintsChanged();
    try {
      const data = await api.ReadSearchFile(file.mod.id, file.path, X.at);
      if (!visible() || seq !== X.readSeq || visit !== X.visit) {
        return;
      }
      fileText = data;
      draw();
      markSelected();
      hintsChanged();
    } catch (e) {
      if (visible() && seq === X.readSeq) {
        say("fail", errText(e));
      }
    }
  }
  // Virtualize the bounded file while keeping actions available for binary results.
  function draw() {
    codeRows?.destroy();
    codeRows = null;
    viewerCard.replaceChildren();
    // The card's number (area 3) is drawn again with each file.
    const num = () => (viewerCard.dataset.area ? [el("span", "anum", viewerCard.dataset.area)] : []);
    if (!X.selected) {
      fileText = null;
      const empty = el("p", "fs-view-empty muted");
      empty.append(...num(), T("select a file"));
      viewerCard.append(empty);
      return;
    }
    const file = X.selected,
      head = el("div", "fs-view-head"),
      top = el("div", "fs-view-title"),
      actions = el("div", "fs-view-actions");
    const name = file.path.split(/[\\/]/).pop();
    actions.append(
      button(T("open file"), "enter", () => openFile(false), "small primary"),
      button(T("show in folder"), "ctrl+o", () => openFile(true), "small")
    );
    const title = el("h2");
    title.append(...num(), name);
    top.append(title, actions);
    const where = el("div", "fs-view-path");
    where.textContent = file.path;
    copyablePath(where, file.path);
    head.append(top, where);
    viewerCard.append(head);
    if (!fileText || fileText.binary) {
      return;
    }
    const body = el("div", "fs-code" + (X.wrap ? " wrap" : ""));
    body.tabIndex = 0;
    body.setAttribute("role", "region");
    body.setAttribute("aria-label", T("file contents"));
    viewerCard.append(body);
    codeRows = virtualRows(
      body,
      fileText.lines.length,
      (i) => {
        const row = fileText.lines[i],
          node = el("div", "fs-code-line"),
          text = el("span", "fs-code-text");
        node.dataset.line = String(row.number);
        node.classList.toggle("hit", fileText.hits.includes(row.number));
        node.append(el("span", "fs-code-number", String(row.number)));
        appendParts(text, row.parts, fileText?.type?.includes("C#") ? "cs" : "xml");
        node.append(text);
        return node;
      },
      {
        heights: new Float64Array(fileText.lines.length).fill(20.4)
      }
    );
    if (fileText.note) {
      viewerCard.append(el("p", "fs-view-note muted", fileText.note));
    }
    const foot = el("div", "fs-view-foot"),
      match = el("span", "fs-match-position");
    if (fileText.hits.length) {
      foot.append(
        match,
        button(T("next"), "n", () => nextMatch(1), "quiet small"),
        button(T("previous"), "N", () => nextMatch(-1), "quiet small")
      );
    }
    foot.append(
      el(
        "span",
        "fs-file-type muted",
        fileText.source ? T("read-only: decompiled code from the source library") : fileText.type
      )
    );
    const wrap = button(T("wrap lines"), "alt+z", toggleWrap, "quiet small fs-wrap");
    wrap.setAttribute("aria-pressed", String(X.wrap));
    foot.append(wrap);
    viewerCard.append(foot);
    markSelected();
  }
  // Move the current match without rebuilding the file text.
  function markSelected() {
    if (!fileText) {
      return;
    }
    const at = fileText.hits.indexOf(X.line),
      label = viewerCard.querySelector(".fs-match-position");
    if (label) {
      label.textContent = Tf("match %d of %d", at < 0 ? 1 : at + 1, fileText.hits.length);
    }
    const index = fileText.lines.findIndex((l) => l.number === X.line);
    if (index >= 0) {
      codeRows?.select(index);
    }
    // Virtual rows use cur for the selected file line, with no extra count sentence.
  }
  function nextMatch(d) {
    if (!fileText?.hits.length) {
      return;
    }
    const hits = fileText.hits,
      at = hits.indexOf(X.line);
    X.line = hits[(Math.max(0, at) + d + hits.length) % hits.length];
    markSelected();
  }
  function toggleWrap() {
    X.wrap = !X.wrap;
    X.wrapTouched = true;
    applyWrap();
    api.SetSetting("ui.gui.search.wrap", String(X.wrap)).catch((e) => say("fail", errText(e)));
  }
  function applyWrap() {
    viewerCard?.querySelector(".fs-code")?.classList.toggle("wrap", X.wrap);
    viewerCard?.querySelector(".fs-wrap")?.setAttribute("aria-pressed", String(X.wrap));
  }
  function openFile(folder) {
    const file = X.selected;
    if (!file) {
      return;
    }
    (folder ? api.OpenSearchFolder(file.mod.id, file.path) : api.OpenSearchFile(file.mod.id, file.path)).catch((e) =>
      say("fail", errText(e))
    );
  }
  function clear() {
    fileText = null;
    codeRows?.destroy();
    codeRows = null;
  }
  // Key 3: the file text when there is one (the arrows then scroll it), else its actions.
  function focus() {
    const target = viewerCard.querySelector(".fs-code") || viewerCard.querySelector(".fs-view-actions .btn");
    if (target) {
      target.focus({ preventScroll: true });
    } else {
      viewerCard.tabIndex = -1;
      viewerCard.focus({ preventScroll: true });
    }
  }
  return {
    draw,
    focus,
    readFile,
    markSelected,
    nextMatch,
    toggleWrap,
    applyWrap,
    openFile,
    clear,
    hasMatches: () => !!fileText?.hits.length
  };
}
