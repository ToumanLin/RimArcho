// Only actions that conflict with the destination wait; navigation stays live (06 §2.3).
import { S, Tf, say } from "./state.js";
import { setEnvironmentGuard } from "./api.js";

export const environmentReason = () => S.busy.env ? Tf("switching to %s", S.busy.env) : "";
setEnvironmentGuard(environmentReason);

const held = new Map();
let observer;
function updateActions(root = document.body) {
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
  const buttons = [...root.querySelectorAll("[data-env-write]")];
  if (root.matches("[data-env-write]")) buttons.unshift(root);
  for (const b of buttons) {
    if (!held.has(b)) held.set(b, { disabled: b.disabled, title: b.getAttribute("title") });
    if (!b.disabled) b.disabled = true;
    if (b.title !== reason) b.title = reason;
  }
}

export async function withEnvironmentSwitch(name, action) {
  const seq = ++S.envSeq;
  const current = () => S.envSeq === seq;
  S.busy.env = name;
  // Invalidate reads already in flight, even if the destination has the same name.
  S.screenSeq++;
  if (!observer) {
    // Repainted actions wait too; inspect added subtrees and marked controls during the switch (06 §3.1).
    observer = new MutationObserver((records) => {
      for (const record of records) {
        if (record.type === "attributes") {
          if (record.target.matches("[data-env-write]")) updateActions(record.target);
          continue;
        }
        for (const node of record.addedNodes) {
          if (node.nodeType === Node.ELEMENT_NODE && node.isConnected) updateActions(node);
        }
      }
    });
  }
  observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["disabled", "title"] });
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
