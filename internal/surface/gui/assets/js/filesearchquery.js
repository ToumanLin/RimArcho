// Shared filter inputs and completion for Search files (06 §2.9).
import { el, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { button } from "./screens.js";
import { createBar } from "./filterbar.js";
import { queryParts } from "./searchsuggest.js";
const kinds = ["C#", "Defs", "patches", "About", "translations", "textures", "sounds"];

// Keep both filter inputs together so panel writes and completion share one lifecycle.
// layout runs when the filter panel opens or closes; nudge is ctrl+↑↓ in the panel and gap is
// the element under this card that resizes it (using it keeps the panel open).
export function createQuery(X, queryCard, { run, shortcuts, stop, focusResults, visible, consume, layout, nudge, gap }) {
  let textInput, filesBar, modsBar, validation;
  // Route file kinds to their own input while retaining the shared panel protocol.
  function fileGroup(target = false) {
    const parts = queryParts(X.files.trim() || "Defs patches About");
    return {
      key: "files",
      title: T("files"),
      noCounts: true,
      ...(target
        ? {
            target: "files"
          }
        : {}),
      values: kinds.map((value) => ({
        filter: value,
        label: T(value),
        count: 0,
        on: parts.find((p) => p.value.toLowerCase() === value.toLowerCase())?.negative
          ? -1
          : parts.some((p) => p.value.toLowerCase() === value.toLowerCase())
            ? 1
            : 0
      }))
    };
  }
  function panelData() {
    return {
      groups: [fileGroup(true), ...X.groups]
    };
  }
  function fieldRow(label, content) {
    const row = el("div", "fs-field"),
      name = el("label", "fs-label", T(label));
    name.id = "fs-label-" + encodeURIComponent(label);
    row.setAttribute("role", "group");
    row.setAttribute("aria-labelledby", name.id);
    const input = content.tagName === "INPUT" ? content : content.querySelector("input");
    if (input) {
      input.id = "fs-" + label;
      input.setAttribute("aria-label", T(label));
      name.htmlFor = input.id;
    }
    row.append(name, content);
    return row;
  }
  // Use one shared filter panel for the two independently editable query fields.
  function buildQuery() {
    const fields = el("div", "fs-fields"),
      top = el("div", "fs-text-box");
    textInput = el("input", "fs-input");
    textInput.spellcheck = false;
    textInput.value = X.text;
    textInput.placeholder = T("text to find");
    textInput.oninput = () => {
      X.text = textInput.value;
    };
    textInput.onkeydown = (e) => {
      if (e.isComposing) {
        return;
      }
      if (shortcuts(e)) {
        e.stopPropagation();
        return;
      }
      if (e.key === "Enter") {
        consume(e);
        run();
      } else if (e.key === "ArrowDown") {
        consume(e);
        filesBar.focus();
      } else if (e.key === "Escape") {
        consume(e);
        if (X.running) {
          stop();
        } else {
          textInput.blur();
          focusResults();
        }
      }
    };
    top.append(textInput);
    for (const [name, label, title, key] of [
      ["case", "Aa", "case sensitive", "alt+c"],
      ["regex", ".*", "regular expression", "alt+r"]
    ]) {
      const b = button(label, "", () => toggle(name), "quiet small fs-toggle");
      b.dataset.option = name;
      b.title = T(title);
      b.setAttribute("aria-label", T(title));
      b.append(keyCaps(key));
      b.setAttribute("aria-pressed", String(X[name]));
      top.append(b);
    }
    fields.append(fieldRow("search", top));
    const key = (e) => {
      if (e.key === "Escape" && X.running) {
        consume(e);
        stop();
        return true;
      }
      return shortcuts(e);
    };
    filesBar = createBar({
      bare: true,
      noPanel: true,
      placeholder: T("files"),
      data: () => ({
        groups: [fileGroup()]
      }),
      change: (_open, reason) => {
        X.files = filesBar.input.value;
        validateKinds();
        refreshPanel();
        if (reason === "filter" && X.text.trim()) {
          run();
        }
      },
      submit: run,
      key,
      toggle: () => modsBar.toggle()
    });
    fields.append(fieldRow("files", filesBar.el));
    validation = el("p", "fs-validation");
    validation.hidden = true;
    validation.id = "fs-file-error";
    filesBar.input.setAttribute("aria-describedby", validation.id);
    fields.append(validation);
    modsBar = createBar({
      placeholder: T("mods"),
      data: panelData,
      targets: {
        files: filesBar
      },
      focusRoot: () => [queryCard, gap],
      deferOutside: true,
      noGap: true,
      nudge,
      count: () => queryParts(X.files).length + queryParts(X.mods).length,
      change: (onlyOpen, reason) => {
        if (onlyOpen) {
          layout?.();
        } else {
          X.mods = modsBar.input.value;
          refreshFilters();
          if (reason === "filter" && X.text.trim()) {
            run();
          }
        }
      },
      submit: run,
      key,
      leave: focusResults
    });
    const modsLabel = el("label", "fs-label", T("mods"));
    modsLabel.id = "fs-label-mods";
    modsBar.el.setAttribute("role", "group");
    modsBar.el.setAttribute("aria-labelledby", modsLabel.id);
    modsLabel.htmlFor = "fs-mods";
    modsBar.input.id = "fs-mods";
    modsBar.input.setAttribute("aria-label", T("mods"));
    modsBar.el.querySelector(".fbar-row").prepend(modsLabel);
    fields.append(modsBar.el);
    queryCard.append(fields);
    filesBar.setText(X.files);
    modsBar.setText(X.mods);
    refreshPanel();
  }
  function validateKinds() {
    const unknown = X.files
      .trim()
      .split(/\s+/)
      .filter(Boolean)
      .find((w) => !kinds.some((k) => k.toLowerCase() === w.replace(/^-/, "").toLowerCase()));
    validation.textContent = unknown ? Tf("unknown file kind: %s", unknown) : "";
    validation.hidden = !unknown;
    filesBar.input.setAttribute("aria-invalid", String(!!unknown));
    return !unknown;
  }
  function refreshPanel() {
    modsBar?.draw();
    filesBar?.draw();
  }
  // Debounce facet reads and discard responses for older input or page visits.
  function refreshFilters() {
    clearTimeout(X.filterTimer);
    const seq = ++X.filterSeq,
      visit = X.visit,
      query = X.mods;
    X.filterTimer = setTimeout(async () => {
      try {
        const data = await api.FileSearchFilters(query);
        if (!visible() || visit !== X.visit || seq !== X.filterSeq) {
          return;
        }
        X.groups = data.groups || [];
        refreshPanel();
      } catch (e) {
        if (visible() && seq === X.filterSeq) {
          say("fail", errText(e));
        }
      }
    }, 90);
  }
  function toggle(name) {
    X[name] = !X[name];
    queryCard.querySelector(`[data-option="${name}"]`)?.setAttribute("aria-pressed", String(X[name]));
    if (X.text.trim()) {
      run();
    }
  }
  function request() {
    return {
      pattern: X.text,
      files: X.files,
      modsQuery: X.mods,
      case: X.case,
      regex: X.regex
    };
  }
  return {
    buildQuery,
    refreshPanel,
    refreshFilters,
    validateKinds,
    toggle,
    request,
    get textInput() {
      return textInput;
    },
    get filesBar() {
      return filesBar;
    },
    get modsBar() {
      return modsBar;
    },
    destroy() {
      filesBar?.destroy();
      modsBar?.destroy();
    }
  };
}
