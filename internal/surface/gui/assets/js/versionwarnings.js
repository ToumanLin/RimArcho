// Detail notices share the row popover's action runner.
import { el } from "./state.js";

let runner = null;
export function setVersionRunner(run) { runner = run; }

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
