// The detail's one field list (06 §4.1, §2.9): the plan editor's detail and a copy's
// detail on Mod 库存 draw view.Field rows with it. A click on a value copies it and says
// "已复制" beside it; a row's own button opens the location, the page, or the tag bar.
import { el, esc, api, T, errText, say } from "./state.js";
import { icon } from "./icons.js";
import * as rt from "/wails/runtime.js";

const Clipboard = rt.Clipboard || { SetText: () => Promise.reject(new Error("no clipboard")) };

// fieldList draws fields; acts.tags runs the tag bar, acts.folder opens the location.
export function fieldList(fields, acts = {}) {
  const box = el("div", "fields");
  for (const f of fields || []) {
    const r = el("div", "field " + f.kind);
    r.append(el("span", "k", f.label));
    const v = el("span", "v");
    if (f.items && f.items.length) {
      for (const x of f.items) {
        const line = el("span", "dep " + x.state);
        const ic = { enabled: f.kind === "conflicts" ? icon.err : icon.ok, disabled: icon.warn, missing: f.kind === "conflicts" ? icon.ok : icon.err }[x.state] || "";
        const note = x.state === "disabled" ? T(" (not enabled)") : x.state === "missing" ? T(" (not installed)") : "";
        line.innerHTML = ic + `<span class="n">${esc(x.name)}</span>` + (note ? `<span class="dim">${esc(note)}</span>` : "");
        line.querySelector(".n").onclick = () => copy(x.name, line);
        v.append(line);
      }
    } else if (f.kind === "tags") {
      if (!f.list || !f.list.length) v.append(el("span", "dim", T("none")));
      for (const t of f.list || []) {
        const c = el("span", "chip", t);
        c.onclick = () => copy(t, c);
        v.append(c);
      }
    } else {
      const t = el("span", "copy", f.value || "—");
      if (f.value) {
        t.title = T("click to copy");
        t.onclick = () => copy(f.value, t);
      }
      v.append(t);
      if (f.extra) v.append(el("span", "dim", `  (${f.extra})`));
    }
    r.append(v);
    if (f.act) {
      const b = el("button", "fact", f.actLabel);
      b.onclick = () => {
        if (f.act === "url") return api.OpenURL(f.url).then(() => say("ok", T("opened the Workshop page")), (e) => say("fail", errText(e)));
        if (acts[f.act]) return acts[f.act](f);
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
    t.select();
    try { ok = document.execCommand("copy"); } catch { ok = false; }
    t.remove();
  }
  if (!ok) return say("fail", T("could not copy"));
  const row = node.closest(".field") || node.parentNode;
  const old = row.querySelector(".copied");
  if (old) old.remove();
  const tip = el("span", "copied", T("copied"));
  row.append(tip);
  setTimeout(() => tip.remove(), 1600);
}
