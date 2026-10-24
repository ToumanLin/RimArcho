// The detail's one field list (06 §4.1, §2.9): the plan editor's detail and a copy's
// detail on Mod 库存 draw view.Field rows with it. A click on a value copies it and says
// "已复制" beside it; a row's own button opens the location, the page, or the tag bar.
import { el, esc, api, T, errText, say } from "./state.js";
import { icon } from "./icons.js";
import * as rt from "/wails/runtime.js";

const Clipboard = rt.Clipboard || { SetText: () => Promise.reject(new Error("no clipboard")) };

// Only these values copy on a click; the rest are just shown.
const copyable = new Set(["id", "package", "path", "workshop", "subscription-url", "subscription-repo"]);

// copyablePath decorates an existing element; the displayed text may be shortened.
// Only activation keys are consumed: arrows, page keys and esc keep their page behavior.
export function copyablePath(node, path) {
  if (!path) return node;
  if (node.querySelector(".path-copy")) node.textContent = node.textContent;
  node.classList.add("copy", "path-copy");
  node.dataset.path = path;
  node.tabIndex = 0;
  node.setAttribute("role", "button");
  node.title = path + "\n" + T("click to copy");
  node.addEventListener("mousedown", e => e.stopPropagation());
  node.addEventListener("click", e => { e.preventDefault(); e.stopPropagation(); copy(path, node); });
  node.addEventListener("keydown", e => {
    if (e.key !== "Enter" && e.key !== " ") return;
    e.preventDefault(); e.stopPropagation();
    if (!e.repeat) copy(path, node);
  });
  return node;
}

// Render known paths before splitting facts, preserving spaces (even double spaces)
// inside a filename. Paths may also be { text, path } to resolve a displayed relative
// filename. The fallback is limited to rooted paths in legacy command output.
export function pathContent(node, text, paths = []) {
  text = String(text ?? "");
  node.replaceChildren();
  const known = [...new Map(paths.filter(Boolean)
    .map(p => typeof p === "string" ? { text: p, path: p } : p)
    .filter(p => p.text && p.path).map(p => [p.text, p])).values()]
    .sort((a, b) => b.text.length - a.text.length);
  const append = (value, path = value) => {
    const span = document.createElement("span");
    span.textContent = value;
    node.append(copyablePath(span, path));
  };
  const plain = value => {
    // Legacy command/task text has no metadata. Only rooted paths with a clear
    // boundary qualify; URLs and keyboard shortcuts cannot match that boundary.
    const rooted = /(^|[\s("'（：])((?:[a-z]:[\\/]|\\\\[^\\\s]+\\|\/[^/\s:]+\/)[^<>"|\n]*?)(?=:\s|:\d+\b|["'?？。；，）]|$)/gi;
    for (const part of value.split(/( {2,}|\n| → )/)) {
      if (!part) continue;
      let at = 0;
      for (const m of part.matchAll(rooted)) {
        const start = m.index + m[1].length;
        if (start > at) node.append(part.slice(at, start));
        append(m[2]); at = start + m[2].length;
      }
      if (part.slice(at)) node.append(part.slice(at));
    }
  };
  let at = 0;
  while (at < text.length) {
    let next = -1, found;
    for (const p of known) {
      const i = text.indexOf(p.text, at);
      if (i >= 0 && (next < 0 || i < next)) { next = i; found = p; }
    }
    if (next < 0) { plain(text.slice(at)); break; }
    plain(text.slice(at, next)); append(found.text, found.path); at = next + found.text.length;
  }
  return node;
}

// fieldList draws fields; acts.tags runs the tag bar, acts.folder opens the location.
export function fieldList(fields, acts = {}) {
  const box = el("div", "fields");
  for (const f of fields || []) {
    const r = el("div", "frow " + f.kind);
    r.append(el("span", "k", f.label));
    const v = el("span", "v");
    if (f.items && f.items.length) {
      for (const x of f.items) {
        const line = el("span", "dep " + x.state);
        const ic = { enabled: f.kind === "conflicts" ? icon.err : icon.ok, disabled: icon.warn, missing: f.kind === "conflicts" ? icon.ok : icon.err }[x.state] || "";
        const note = x.state === "disabled" ? T(" (not enabled)") : x.state === "missing" ? T(" (not installed)") : "";
        line.innerHTML = ic + `<span class="n">${esc(x.name)}</span>` + (note ? `<span class="dim">${esc(note)}</span>` : "");
        v.append(line);
      }
    } else if (f.kind === "tags") {
      if (!f.list || !f.list.length) v.append(el("span", "dim", T("none")));
      for (const t of f.list || []) {
        v.append(el("span", "chip", t));
      }
    } else {
      const t = el("span", copyable.has(f.kind) && f.value ? "copy" : "", f.value || "—");
      if (f.value && copyable.has(f.kind)) {
        copyablePath(t, f.value);
        if (f.kind !== "path") t.classList.remove("path-copy");
      }
      v.append(t);
      if (f.extra) v.append(el("span", "dim", `  (${f.extra})`));
    }
    r.append(v);
    const actions = [...(f.act ? [{ act: f.act, label: f.actLabel }] : []), ...(f.actions || [])];
    for (const a of actions) {
      const b = el("button", "fact", a.label);
      if (a.act === "tags") b.dataset.envWrite = a.act;
      if (a.act === "steam") b.title = "ctrl+s";
      b.onclick = () => {
        if (a.act === "url") return api.OpenURL(f.url).then(() => say("ok", T("opened the page")), (e) => say("fail", errText(e)));
        if (a.act === "steam") return api.OpenSteamWorkshop("ws:" + f.value).then((t) => say("ok", t), (e) => say("fail", errText(e)));
        if (acts[a.act]) return acts[a.act](f);
      };
      r.append(b);
    }
    box.append(r);
  }
  return box;
}

// copy puts text on the clipboard and shows "已复制" next to the node for a moment.
export async function copy(text, node) {
  let ok = false;
  // the window's own clipboard (Wails), else the page's, else the old way
  try { await Clipboard.SetText(text); ok = true; } catch { /* not in the window */ }
  if (!ok) try { await navigator.clipboard.writeText(text); ok = true; } catch {
    const t = el("textarea");
    t.value = text;
    t.style.position = "fixed";
    t.style.opacity = "0";
    document.body.append(t);
    const focused = document.activeElement;
    t.select();
    try { ok = document.execCommand("copy"); } catch { ok = false; }
    t.remove();
    focused?.focus({ preventScroll: true });
  }
  if (!ok) return say("fail", T("could not copy"));
  const row = node.closest(".frow") || node.parentNode;
  if (!row || !node.isConnected) return;
  const old = row.querySelector(".copied");
  if (old) old.remove();
  const tip = el("span", "copied", T("copied"));
  tip.setAttribute("role", "status");
  node.after(tip);
  setTimeout(() => tip.remove(), 1600);
}
