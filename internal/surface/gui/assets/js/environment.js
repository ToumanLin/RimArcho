// Only actions that conflict with the destination wait; navigation stays live (06 §2.3).
import { S, T, Tf, say } from "./state.js";
import { setEnvironmentGuard } from "./api.js";

export const environmentReason = () => S.busy.env ? Tf("switching to %s", S.busy.env) : "";
setEnvironmentGuard(environmentReason);

const held = new Map();
const writeLabels = ["write to the game", "launch the game", "undo", "redo", "auto sort", "apply",
  "delete", "remove", "remove the game", "rescan the mod folders", "rescan", "check the files again"];
let observer;
function updateActions() {
  const reason = environmentReason();
  if (!reason) {
    for (const [b, old] of held) {
      b.disabled = old.disabled;
      if (old.title === null) b.removeAttribute("title"); else b.title = old.title;
    }
    held.clear();
    observer?.disconnect();
    return;
  }
  const opLabels = (S.start?.ops || []).filter((op) => op.group !== 3).map((op) => op.label);
  for (const b of document.querySelectorAll("button.btn, .hm-go, #menu button, button.fact, .lib-bar button, [data-env-write]")) {
    if (!b.matches(".hm-go, [data-env-write]") && ![...writeLabels.map(T), ...opLabels].some((label) => label && b.textContent.trim().startsWith(label))) continue;
    if (!held.has(b)) held.set(b, { disabled: b.disabled, title: b.getAttribute("title") });
    b.disabled = true;
    b.title = reason;
  }
}

export async function withEnvironmentSwitch(name, action) {
  const seq = ++S.envSeq;
  const current = () => S.envSeq === seq;
  S.busy.env = name;
  // Invalidate reads already in flight, even if the destination has the same name.
  S.screenSeq++;
  if (!observer) {
    observer = new MutationObserver(updateActions);
  }
  observer.observe(document.body, { childList: true, subtree: true });
  updateActions();
  say("busy", Tf("switching to %s…", name));
  try {
    await action(current);
    return current();
  } catch (error) {
    if (current()) throw error;
    return false;
  } finally {
    if (current()) {
      S.busy.env = false;
      updateActions();
    }
  }
}
