// Steam credit (02 §3.4): mods a "subscribe all" downloaded with SteamCMD give their authors no
// subscription. After three game sessions that used them, ask once whether to replace that
// batch with Steam subscriptions, little by little in the background; no thanks never asks again.
import { S, api, T, Tf, say, errText } from "./state.js";
import * as pop from "./popups.js";

// getChoices: the two ways a "subscribe all" gets Workshop items; index 1 is SteamCMD.
export const getChoices = () => [
  T("from Steam (slow, but credits the modder with a subscription; Steam manages it)"),
  T("from SteamCMD (fast, but does not credit the modder; RimArcho manages it)"),
];

let asking = null; // the check in flight, so the triggers share one

// checkCredit asks the backend whether a batch is due (it says null while the game runs or
// Steam is not there) and shows the question; called when the window is ready, gets the
// focus, and when the game exits. Another popup open: wait for the next check. Closed with
// esc: not answered, so a later check asks again.
export function checkCredit() {
  if (!asking) asking = ask().finally(() => { asking = null; });
  return asking;
}

async function ask() {
  if (S.popup) return;
  let p;
  try { p = await api.CreditPrompt(); } catch { return; }
  if (!p || !p.batchIDs?.length || S.popup) return;
  const what = p.examples?.length
    ? Tf("%d mods, such as %s", p.modCount, p.examples.join(T(", ")))
    : Tf("%d mods", p.modCount);
  pop.choose(T("Do you want to credit the modders by subscribing from Steam?") + "  " + what, [
    T("replace these SteamCMD mods with Steam subscriptions, little by little"),
    T("dismiss"),
  ], async (i) => {
    try {
      const r = await api.CreditAnswer(p.batchIDs, i === 0);
      if (i === 0) say("ok", r.taskID ? T("replacing with Steam subscriptions in the background; see the tasks page") : T("already replaced"));
    } catch (e) { say("fail", errText(e)); }
  });
}
