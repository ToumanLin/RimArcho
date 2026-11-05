// Shared single-value controls (16 §3.10). A button keeps native disabled/fieldset,
// focus and change behavior; only the open menu owns document listeners and DOM.
import { icon } from "./icons.js";

let opened = null, serial = 0;

export function closeChoiceMenu(restoreFocus = false) {
  if (!opened) return false;
  opened.close(restoreFocus);
  return true;
}

function node(tag, cls, text) {
  const n = document.createElement(tag);
  n.className = cls;
  if (text != null) n.textContent = text;
  return n;
}

export function choiceControl(label, options, value) {
  const items = options.map(o => typeof o === "object" ? { ...o, value: String(o.value) } : { value: String(o), label: String(o) });
  const binary = items.length === 2 && items.some(o => o.value === "off") && items.some(o => o.value === "on");
  if (binary) items.sort((a, b) => (a.value === "off" ? -1 : 1) - (b.value === "off" ? -1 : 1));
  const b = node("button", "choice-control " + (binary ? "choice-switch" : "choice-select"));
  b.type = "button";
  b.id = "choice-control-" + ++serial;
  b.dataset.choice = "";
  b.setAttribute("role", binary ? "switch" : "combobox");
  b.setAttribute("aria-label", label);
  let current = "", menu = null, active = -1, rows = [], controller, observer;
  let prefix = "", typedAt = 0;
  const disabled = () => b.matches(":disabled") || b.closest("[inert]");
  const enabled = () => items.map((o, i) => o.disabled ? -1 : i).filter(i => i >= 0);
  const text = node("span", "choice-value");
  if (binary) {
    for (const o of items) {
      const part = node("span", "choice-part", o.label === "off" ? "Off" : o.label === "on" ? "On" : o.label);
      part.dataset.value = o.value;
      part.setAttribute("aria-hidden", "true");
      b.append(part);
    }
  } else {
    b.setAttribute("aria-haspopup", "listbox");
    b.setAttribute("aria-expanded", "false");
    const arrow = node("span", "choice-arrow");
    arrow.innerHTML = icon.chev;
    arrow.setAttribute("aria-hidden", "true");
    b.append(text, arrow);
  }
  const paint = () => {
    const selected = items.find(o => o.value === current);
    b.title = selected?.label || "";
    if (binary) {
      b.setAttribute("aria-checked", String(current === "on"));
      for (const part of b.children) part.classList.toggle("on", part.dataset.value === current);
    } else text.textContent = selected?.label || "";
  };
  Object.defineProperty(b, "value", {
    get: () => current,
    set(v) {
      current = items.some(o => o.value === String(v)) ? String(v) : "";
      if (menu) close(false);
      paint();
    },
  });
  b.value = value ?? items[0]?.value ?? "";
  const commit = i => {
    if (disabled() || !items[i] || items[i].disabled) return;
    const next = items[i].value, changed = current !== next;
    close(true);
    b.value = next;
    if (changed) {
      b.dispatchEvent(new Event("input", { bubbles: true }));
      b.dispatchEvent(new Event("change", { bubbles: true }));
    }
  };
  function close(focus) {
    if (!menu) return;
    controller.abort(); observer.disconnect();
    menu.remove(); menu = null; rows = [];
    b.setAttribute("aria-expanded", "false");
    b.removeAttribute("aria-activedescendant");
    b.removeAttribute("aria-controls");
    opened = null;
    prefix = "";
    if (focus && b.isConnected && !disabled()) b.focus({ preventScroll: true });
  }
  function highlight(i, reveal = true) {
    active = i;
    rows.forEach((row, at) => row.classList.toggle("active", at === active));
    if (rows[i]) {
      b.setAttribute("aria-activedescendant", rows[i].id);
      if (reveal) rows[i].scrollIntoView({ block: "nearest" });
    }
  }
  function open() {
    if (disabled() || !enabled().length) return;
    closeChoiceMenu();
    const contextMenu = document.getElementById("menu");
    if (contextMenu) { contextMenu.hidden = true; contextMenu.inert = true; }
    controller = new AbortController();
    menu = node("div", "choice-menu");
    const title = node("div", "choice-title", label);
    title.id = "choice-title-" + ++serial;
    const list = node("div", "choice-options");
    list.id = "choice-list-" + serial;
    list.setAttribute("role", "listbox");
    list.setAttribute("aria-labelledby", title.id);
    rows = items.map((o, i) => {
      const row = node("div", "choice-option");
      row.id = "choice-option-" + serial + "-" + i;
      row.setAttribute("role", "option");
      row.setAttribute("aria-selected", String(o.value === current));
      if (o.disabled) row.setAttribute("aria-disabled", "true");
      const check = node("span", "choice-check", o.value === current ? "✓" : "");
      check.setAttribute("aria-hidden", "true");
      row.append(check, node("span", "choice-label", o.label));
      row.onpointermove = () => { if (!o.disabled) highlight(i, false); };
      row.onclick = e => { e.stopPropagation(); commit(i); };
      list.append(row);
      return row;
    });
    menu.append(title, list);
    // Keep focus on the combobox: Tab follows the page's own DOM order.
    menu.onmousedown = e => e.preventDefault();
    document.body.append(menu);
    const r = b.getBoundingClientRect(), gap = 6, edge = 8;
    const below = innerHeight - r.bottom - gap - edge, above = r.top - gap - edge;
    const up = menu.getBoundingClientRect().height > below && above > below;
    menu.style.maxHeight = Math.max(0, Math.min(340, up ? above : below)) + "px";
    menu.style.minWidth = Math.min(Math.max(r.width, 200), innerWidth - edge * 2) + "px";
    const m = menu.getBoundingClientRect();
    menu.style.left = Math.max(edge, Math.min(r.right - m.width, innerWidth - m.width - edge)) + "px";
    menu.style.top = Math.max(edge, up ? r.top - gap - m.height : r.bottom + gap) + "px";
    b.setAttribute("aria-expanded", "true");
    b.setAttribute("aria-controls", list.id);
    opened = { close };
    const listen = (target, name, fn, capture = false) => target.addEventListener(name, fn, { capture, signal: controller.signal });
    listen(document, "pointerdown", e => { if (!b.contains(e.target) && !menu.contains(e.target)) close(false); }, true);
    listen(document, "focusin", e => { if (!b.contains(e.target) && !menu.contains(e.target)) close(false); });
    listen(window, "resize", () => close(false));
    listen(document, "scroll", e => { if (!menu.contains(e.target)) close(false); }, true);
    observer = new MutationObserver(() => {
      if (!b.isConnected || disabled() || !b.checkVisibility()) close(false);
    });
    observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["disabled", "hidden", "inert"] });
    const selected = items.findIndex(o => o.value === current && !o.disabled);
    highlight(selected < 0 ? enabled()[0] : selected);
  }
  b.onclick = e => {
    if (disabled()) return;
    if (binary) {
      const part = e.target.closest(".choice-part");
      commit(part ? items.findIndex(o => o.value === part.dataset.value) : items.findIndex(o => o.value !== current));
    } else if (menu) close(false); else open();
  };
  b.addEventListener("keydown", e => {
    if (e.isComposing || e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === "Escape" && !menu) return;
    e.stopPropagation();
    if (e.key === "Tab") { close(false); return; }
    if (disabled()) return;
    if (binary) {
      if (![" ", "Enter", "ArrowLeft", "ArrowRight"].includes(e.key)) return;
      e.preventDefault();
      if (e.repeat) return;
      commit(e.key === "ArrowLeft" ? 0 : e.key === "ArrowRight" ? 1 : items.findIndex(o => o.value !== current));
      return;
    }
    if ([" ", "Enter", "ArrowDown", "ArrowUp", "Home", "End", "Escape"].includes(e.key)) {
      e.preventDefault();
      if (e.key === "Escape") return close(true);
      if (!menu) return open();
      if (e.key === "Enter" || e.key === " ") return commit(active);
      const choices = enabled(), at = choices.indexOf(active);
      highlight(e.key === "Home" ? choices[0] : e.key === "End" ? choices.at(-1) : choices[Math.max(0, Math.min(choices.length - 1, at + (e.key === "ArrowDown" ? 1 : -1)))]);
    } else if (e.key.length === 1) {
      e.preventDefault();
      if (!menu) open();
      if (!menu) return;
      const now = performance.now(), char = e.key.toLocaleLowerCase();
      prefix = now - typedAt < 700 ? prefix + char : char; typedAt = now;
      const needle = [...prefix].every(c => c === char) ? char : prefix;
      const start = needle.length === 1 ? active + 1 : active;
      for (let n = 0; n < items.length; n++) {
        const i = (Math.max(0, start) + n) % items.length;
        if (!items[i].disabled && items[i].label.toLocaleLowerCase().startsWith(needle)) { highlight(i); break; }
      }
    }
  });
  return b;
}
