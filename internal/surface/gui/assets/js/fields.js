// The detail's one field list (06 §4.1, §2.9): the plan editor's detail and a copy's
// detail on Mod library draw view.Field rows with it. Address and identifier rows have a copy icon;
// a row's own action opens the location, the page, or the tag bar.
import { el, esc, api, T, errText, say } from "./state.js";
import { icon } from "./icons.js";
import { addressLabel } from "./address.js";
import * as rt from "/wails/runtime.js";

const Clipboard = rt.Clipboard || { SetText: () => Promise.reject(new Error("no clipboard")) };

// Addresses and identifiers show their value with a trailing copy icon.
const copyable = new Set(["id", "package", "path", "workshop", "subscription-url", "subscription-repo"]);

// Address text stays selectable; only its trailing icon copies the full value.
export function copyablePath(node, path) {
  if (!path) return node;
  const display = node.querySelector(".address-text")?.dataset.address ?? (node.textContent || path);
  node.replaceChildren(addressLabel(display));
  node.classList.add("path-copy");
  node.dataset.path = path;
  node.removeAttribute("tabindex");
  node.removeAttribute("role");
  node.title = path;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "address-copy";
  button.innerHTML = icon.copy;
  button.title = T("copy");
  button.setAttribute("aria-label", T("copy") + ": " + path);
  button.addEventListener("mousedown", e => e.stopPropagation());
  button.addEventListener("click", e => { e.preventDefault(); e.stopPropagation(); copy(path, button); });
  button.addEventListener("keydown", e => {
    if (e.key !== "Enter" && e.key !== " ") return;
    e.preventDefault(); e.stopPropagation();
    if (!e.repeat) button.click();
  });
  node.append(button);
  return node;
}

// Only editable local addresses open a picker; the copy icon remains independent.
export function editablePath(node, choose, file = false) {
  const paths = node.classList.contains("path-copy") ? [node] : node.querySelectorAll(".path-copy");
  for (const path of paths) {
    const text = path.querySelector(".address-text");
    if (!text) continue;
    text.classList.add("address-pick");
    text.tabIndex = 0; text.setAttribute("role", "button");
    text.title = path.dataset.path + "\n" + T(file ? "click to choose a file" : "click to choose a folder");
    text.setAttribute("aria-label", text.title);
    text.onclick = e => {
      e.preventDefault(); e.stopPropagation();
      const selection = globalThis.getSelection?.();
      if (!selection || selection.isCollapsed || !text.contains(selection.anchorNode)) choose();
    };
    text.onkeydown = e => {
      if (e.key !== "Enter" && e.key !== " ") return;
      e.preventDefault(); e.stopPropagation(); if (!e.repeat) choose();
    };
  }
  return node;
}

// Render known paths before splitting facts, preserving spaces (even double spaces)
// inside a filename. Paths may also be { text, path } to resolve a displayed relative
// filename. The fallback recognises rooted paths and web URLs in legacy output.
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
  // A web address ends before trailing punctuation and an unmatched ")": the one that closes
  // a Markdown link ([text](https://…/CHANGELOG.md)) or a sentence's parenthesis (VM round
  // 2026-11-05: the release notes' copy button copied ".../CHANGELOG.md)", a 404).
  const urlEnd = url => {
    if (!/^https?:\/\//i.test(url)) return url;
    for (;;) {
      const last = url.slice(-1);
      if (".,;:!?".includes(last)) { url = url.slice(0, -1); continue; }
      if (last === ")" && (url.match(/\(/g) || []).length < (url.match(/\)/g) || []).length) { url = url.slice(0, -1); continue; }
      return url;
    }
  };
  const plain = value => {
    // Legacy command/task text has no metadata. Rooted paths and web addresses
    // need a clear boundary; shortcuts do not qualify.
    const rooted = /(^|[\s("'\uff08\uff1a])((?:https?:\/\/[^\s<>"'\uff1f\u3002\uff1b\uff0c\uff09]+)|(?:(?:[a-z]:[\\/]|\\\\[^\\\s]+\\|\/[^/\s:]+\/)[^<>"|\n]*?)(?=:\s|:\d+\b|["'?\uff1f\u3002\uff1b\uff0c\uff09]|$))/gi;
    for (const part of value.split(/( {2,}|\n| → )/)) {
      if (!part) continue;
      let at = 0;
      for (const m of part.matchAll(rooted)) {
        const start = m.index + m[1].length;
        if (start > at) node.append(part.slice(at, start));
        const hit = urlEnd(m[2]);
        append(hit); at = start + hit.length;
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
      const t = el("span", "", f.value || "—");
      if (f.value && copyable.has(f.kind)) copyablePath(t, f.value);
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
        // Mod library › Mod settings at this Mod's files (main.js opens it)
        if (a.act === "settings") return document.dispatchEvent(new CustomEvent("open-mod-settings", { detail: f.target }));
        if (acts[a.act]) return acts[a.act](f);
      };
      r.append(b);
    }
    box.append(r);
  }
  return box;
}

const copyTimers = new WeakMap();

// Copy feedback stays inside address buttons; other copy actions keep their text tip.
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
  if (!ok) { say("fail", T("could not copy")); return false; }
  if (node.classList.contains("address-copy")) {
    clearTimeout(copyTimers.get(node));
    node.innerHTML = icon.ok;
    node.title = T("copied");
    node.setAttribute("aria-label", T("copied"));
    node.dataset.copied = "true";
    const status = document.createElement("span");
    status.className = "address-status"; status.setAttribute("role", "status");
    status.textContent = T("copied"); node.append(status);
    copyTimers.set(node, setTimeout(() => {
      node.innerHTML = icon.copy; node.title = T("copy");
      node.setAttribute("aria-label", T("copy") + ": " + text);
      delete node.dataset.copied;
      copyTimers.delete(node);
    }, 1600));
    return true;
  }
  const row = node.closest(".frow") || node.parentNode;
  if (!row || !node.isConnected) return;
  const old = row.querySelector(".copied");
  if (old) old.remove();
  const tip = el("span", "copied", T("copied"));
  tip.setAttribute("role", "status");
  node.after(tip);
  setTimeout(() => tip.remove(), 1600);
}
