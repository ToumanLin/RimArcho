// The browser's own context menu (back, reload, save as, print, inspect) never opens (16 §8.5).
// DefaultContextMenuDisabled is not enough: builds without Wails' `production` tag, the
// release builds included, run Wails in debug mode, which turns WebView2's menu back on. So the
// page cancels it. Editable text fields keep it for cut, copy, paste and select all.

const textInputs = new Set(["", "text", "search", "url", "email", "tel", "password", "number"]);

export function keepsNativeMenu(t) {
  if (!t || t.nodeType !== 1 || t.disabled || t.readOnly) return false;
  if (t.isContentEditable) return true;
  if (t.tagName === "TEXTAREA") return true;
  return t.tagName === "INPUT" && textInputs.has((t.getAttribute("type") || "").toLowerCase());
}

export function blockNativeMenu(doc) {
  doc.addEventListener("contextmenu", (e) => { if (!keepsNativeMenu(e.target)) e.preventDefault(); });
}
