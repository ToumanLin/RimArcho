import { S, api, el, T, Tf, errText, say } from "./state.js";
import * as pop from "./popups.js";
import { visTip } from "./membar.js";

// A game of this installation runs: trimming and deleting are refused (06 §4.1), so the
// menu shows its actions disabled under the reason. The app checks again on apply.
const gameRuns = () => !!(S.live && (S.live.running || S.live.blocked));

// logSizeBadge: oversized Player logs as one amber size badge, beside the file picker on
// Game log and on Launch's last game line (16 §2.3, §2.10). It opens their menu: trim or
// delete each, through a confirmation. The sentence is its name and tooltip.
export function logSizeBadge(files, size, refresh) {
  const b = el("button", "log-size");
  b.dataset.logMenu = "";
  b.append(el("span", "num", size), el("span", "chev", "⌄"));
  const facts = files.map(f => Tf("%s is %s", f.label, f.oversized)).join(", ");
  const tip = facts + ": " + T(gameRuns() ? "the game is running: close it first" : "too large to analyse in full: trim or delete");
  b.title = tip;
  b.setAttribute("aria-label", tip);
  pop.menuOpener(b);
  b.onclick = (e) => {
    e.stopPropagation();
    const r = b.getBoundingClientRect();
    pop.menu(r.left, r.bottom + 4, menuItems(files, refresh), e.detail === 0, { name: facts, opener: b });
  };
  return b;
}

function menuItems(files, refresh) {
  const off = gameRuns();
  const out = off ? [{ label: T("the game is running: close it first"), cls: "why", disabled: true, run() {} }, { sep: true }] : [];
  files.forEach((f, i) => {
    if (i) out.push({ sep: true });
    out.push(
      { label: Tf("trim %s…", f.label), key: files.length > 1 ? f.oversized : "", disabled: off, envWrite: "apply", data: { logCleanup: "trim" }, run: () => preview(f, true, refresh) },
      { label: Tf("delete %s…", f.label), cls: "danger", disabled: off, envWrite: "apply", data: { logCleanup: "delete" }, run: () => preview(f, false, refresh) });
  });
  return out;
}

// coverageMark: [head][skipped middle][tail], how much of a log was analysed (06 §1). The
// middle is hatched while it is still in the file and empty once trimmed away; the sentence
// with the size is the mark's name and tooltip. Inside a button it takes no focus of its own.
export function coverageMark(text, trimmed, focusable = true) {
  const m = el("span", "log-cov" + (trimmed ? " trimmed" : ""));
  m.append(el("i"), el("i", "gap"), el("i"));
  return visTip(m, text, "img", focusable);
}

async function preview(file, trim, refresh) {
  try {
    const { screenSeq, envSeq, popup } = S;
    const p = await api.PreviewLogCleanup(file.path, trim);
    if (S.screenSeq !== screenSeq || S.envSeq !== envSeq || S.popup !== popup || popup) return;
    const lines = [p.Path, Tf("size: %s", p.Size)];
    if (trim) {
      lines.push(Tf("after trimming: %s", p.Remaining), Tf("removed from the middle: %s", p.Omitted), "", T("the removed lines cannot be restored"));
    } else {
      lines.push("", T("the log is deleted permanently: it cannot be restored"));
    }
    pop.confirm(T(trim ? "trim the game log" : "delete the game log"), lines,
      T(trim ? "trim" : "delete"), async () => {
        try {
          await api.ApplyLogCleanup(p.Path, p.Token, trim);
          say("ok", T(trim ? "game log trimmed" : "game log deleted"));
          await refresh();
        } catch (e) {
          say("fail", errText(e));
        }
      });
  } catch (e) {
    say("fail", errText(e));
  }
}
