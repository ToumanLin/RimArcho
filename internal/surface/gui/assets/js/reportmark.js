// The report mark (16 §2.4, §2.9, §2.14): a quiet flag on a Can it run item, and on a Mod
// details line that rests on shared-log data or the replacement library. It opens Feedback
// with the template filled in from what the player was looking at: "this is wrong" (the wrong
// diagnosis template with the item's ID) or "the data is outdated" (outdated data with the Mod
// and the item that showed it). Nothing is sent from here: Feedback's form and preview come
// first, as for any report.
import { el, T } from "./state.js";
import { icon } from "./icons.js";

// startReport opens Feedback on a new problem of kind with these answers (feedback.js reads
// S.feedbackPrefill when it opens).
export function startReport(kind, answers) {
  document.dispatchEvent(new CustomEvent("feedback-report", { detail: { kind, answers } }));
}

// reportMark is the flag. finding: the item's ID (a wrong diagnosis); mod: the Mod's
// packageId when the item rests on the data (outdated data). With both, a click offers the
// two; with one, it starts that report at once.
export function reportMark({ finding = "", mod = "", wrong = true } = {}) {
  const b = el("button", "icon report-mark");
  b.type = "button";
  b.innerHTML = icon.report || "";
  const acts = [];
  if (wrong && finding) acts.push({ label: T("this is wrong"), run: () => startReport("diagnosis", { finding }) });
  if (mod) acts.push({ label: T("the data is outdated"), run: () => startReport("data", finding ? { mod, finding } : { mod }) });
  const name = acts.length > 1 ? T("report it") : acts[0]?.label || "";
  b.title = name;
  b.setAttribute("aria-label", name);
  b.onclick = (e) => {
    e.stopPropagation();
    if (acts.length === 1) return acts[0].run();
    const r = b.getBoundingClientRect();
    // popups.js only when the menu opens: the details and Can it run load without it
    import("./popups.js").then((p) => p.menu(r.left, r.bottom + 4, acts, e.detail === 0, { name, opener: b }));
  };
  if (acts.length > 1) b.setAttribute("aria-haspopup", "menu");
  return b;
}
