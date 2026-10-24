// Shared card gaps and the sidebar edge (06 §3.1, 06 §7.3).
// Pages own their geometry and saved format so existing GUI settings keep working.
import { api, T } from "./state.js";

export function wireSeparator(handle, opts) {
  const axis = opts.axis || "x", direction = opts.direction || 1;
  const key = () => typeof opts.key === "function" ? opts.key() : opts.key;
  const enabled = () => handle.getClientRects().length > 0 &&
    getComputedStyle(handle).visibility !== "hidden" && (!opts.enabled || opts.enabled());
  const range = () => { const [min, max] = opts.range(); return [min, Math.max(min, max)]; };
  const save = (reset = false) => {
    const value = reset ? "" : opts.serialize ? opts.serialize() : String(Math.round(opts.get()));
    return api.SetSetting(key(), value).catch(() => {});
  };
  function refresh() {
    const [min, max] = range();
    handle.tabIndex = enabled() ? 0 : -1;
    handle.setAttribute("aria-disabled", String(!enabled()));
    handle.setAttribute("aria-valuemin", String(Math.round(min)));
    handle.setAttribute("aria-valuemax", String(Math.round(max)));
    handle.setAttribute("aria-valuenow", String(Math.round(Math.max(min, Math.min(max, opts.get())))));
  }
  function set(value) {
    const [min, max] = range();
    opts.set(Math.round(Math.max(min, Math.min(max, value))));
    refresh();
  }
  function reset() {
    if (!enabled()) return;
    opts.reset(); refresh(); opts.changed?.(); save(true);
  }
  handle.classList.add("separator");
  handle.setAttribute("role", "separator");
  handle.setAttribute("aria-orientation", axis === "x" ? "vertical" : "horizontal");
  handle.title ||= T("drag to resize, double click: back to the default");
  handle.setAttribute("aria-label", handle.title);
  handle.addEventListener("focus", refresh);
  handle.addEventListener("keydown", (e) => {
    const keys = axis === "x" ? ["ArrowLeft", "ArrowRight"] : ["ArrowUp", "ArrowDown"];
    if (e.key !== "Enter" && !e.key.startsWith("Arrow")) return;
    e.preventDefault(); e.stopPropagation();
    if (!enabled()) return;
    if (e.key === "Enter") return reset();
    if (!keys.includes(e.key)) return;
    set(opts.get() + (e.key === keys[1] ? 40 : -40) * direction);
    opts.changed?.(); save();
  });
  handle.addEventListener("mousedown", (e) => {
    if (e.button !== 0 || !enabled()) return;
    e.preventDefault(); handle.focus();
    const at = axis === "x" ? "clientX" : "clientY", start = e[at], size = opts.get();
    const cursor = axis === "x" ? "resizing" : "vresizing";
    handle.classList.add("drag"); document.body.classList.add(cursor);
    let moved = false;
    const move = (m) => {
      if (!enabled()) return;
      moved = true; set(size + (m[at] - start) * direction);
    };
    const up = () => {
      handle.classList.remove("drag"); document.body.classList.remove(cursor);
      removeEventListener("mousemove", move); removeEventListener("mouseup", up);
      if (moved) { opts.changed?.(); save(); }
    };
    addEventListener("mousemove", move); addEventListener("mouseup", up);
  });
  handle.addEventListener("dblclick", reset);
  // A late setting read must not overwrite a keyboard or mouse adjustment.
  let touched = false;
  handle.addEventListener("keydown", () => { touched = true; });
  handle.addEventListener("mousedown", () => { touched = true; });
  handle.addEventListener("dblclick", () => { touched = true; });
  const ready = opts.restore ? api.Setting(key()).then((value) => {
    if (!touched) opts.restore(value);
    refresh();
  }).catch(() => {}) : Promise.resolve();
  handle.refreshSeparator = refresh;
  requestAnimationFrame(refresh);
  return { refresh, ready };
}

// Query the live DOM so revisiting a page does not retain its previous handles.
addEventListener("resize", () => {
  document.querySelectorAll(".separator").forEach((handle) => handle.refreshSeparator());
});
// Folded cards, cached pages and narrow layouts change availability without a resize.
let scheduled = false;
new MutationObserver(() => {
  if (scheduled) return;
  scheduled = true;
  requestAnimationFrame(() => {
    scheduled = false;
    document.querySelectorAll(".separator").forEach((handle) => handle.refreshSeparator());
  });
}).observe(document.documentElement, { subtree: true, childList: true, attributes: true, attributeFilter: ["class", "style", "hidden"] });
