// Version warning lists expand where they were requested, and render only on demand.
import { el } from "./state.js";

let opened = "";
let runner = null;
const listeners = new Map();
export function setVersionRunner(run) { runner = run; }
export function onVersionList(name, redraw) { listeners.set(name, redraw); }
export function toggleVersionList(it) {
  opened = opened === it.id ? "" : it.id;
  for (const redraw of listeners.values()) redraw();
}

export function versionList(it, run) {
  if (opened !== it.id) return null;
  const act = (it.actions || []).find((a) => a.kind === "version-list");
  if (!act) return null;
  const list = el("div", "parts");
  for (const entry of act.entries || []) {
    const row = el("div", "word-facts");
    const b = el("button", "btn small", entry.action.label);
    b.dataset.envWrite = entry.action.kind;
    b.onclick = (e) => { e.stopPropagation(); run(it, entry.action); };
    row.append(el("span", "", entry.text), b);
    list.append(row);
  }
  return list;
}

export function versionNotice(reason) {
  const box = el("div", reason.status ? "word-facts" : "muted word-facts");
  box.append(el("p", "", reason.text));
  for (const action of reason.actions || []) {
    const b = el("button", "btn small", action.label);
    b.dataset.envWrite = action.kind;
    b.onclick = () => runner && runner(reason, action);
    box.append(b);
  }
  return box;
}
