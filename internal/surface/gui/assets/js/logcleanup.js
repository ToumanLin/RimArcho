import { S, api, el, T, Tf, errText, say } from "./state.js";
import * as pop from "./popups.js";

export function logCleanupButtons(file, refresh) {
  const row = el("div", "log-cleanup");
  row.append(el("span", "log-cleanup-name word-facts", file.label + "  " + file.oversized));
  for (const [label, trim] of [["Trim", true], ["Delete", false]]) {
    const button = el("button", "btn", T(label));
    button.dataset.logCleanup = label.toLowerCase();
    button.dataset.envWrite = "apply";
    button.onclick = () => preview(file, trim, refresh);
    row.append(button);
  }
  return row;
}

async function preview(file, trim, refresh) {
  try {
    const { screenSeq, envSeq, popup } = S;
    const p = await api.PreviewLogCleanup(file.path, trim);
    if (S.screenSeq !== screenSeq || S.envSeq !== envSeq || S.popup !== popup || popup) return;
    const lines = [p.Path, Tf("Log size: %s", p.Size)];
    if (trim) {
      lines.push(Tf("Size after trimming: %s", p.Remaining), Tf("Middle to remove: %s", p.Omitted), T("The removed lines cannot be restored."));
    } else {
      lines.push(T("This permanently deletes the log."));
    }
    pop.confirm(T(trim ? "Trim the game log?" : "Delete the game log?"), lines,
      T(trim ? "Trim" : "Delete"), async () => {
        try {
          await api.ApplyLogCleanup(p.Path, p.Token, trim);
          say("ok", T(trim ? "Game log trimmed." : "Game log deleted."));
          await refresh();
        } catch (e) {
          say("fail", errText(e));
        }
      });
  } catch (e) {
    say("fail", errText(e));
  }
}
