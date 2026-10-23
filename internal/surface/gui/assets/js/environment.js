// Environment changes may prepare links and scan folders before the new board is ready.
// Keep feedback visible for the whole operation, including the destination's refresh.
// quiet: the caller shows the progress itself (the launch page's ▶ block, 06 §2.3); the
// window is still busy and inert until the switch is done.
import { S, $, el, T, Tf, say } from "./state.js";

export async function withEnvironmentSwitch(name, action, { quiet = false } = {}) {
  if (S.busy.env) return false;
  const app = $("#app"), focus = document.activeElement;
  const wasInert = app.inert, wasBusy = app.getAttribute("aria-busy");
  const title = Tf("switching to %s…", name);
  const overlay = el("div", "env-progress");
  overlay.id = "env-progress";
  overlay.tabIndex = -1;
  const card = el("div", "env-progress-card");
  card.setAttribute("role", "status");
  card.setAttribute("aria-live", "polite");
  card.append(el("div", "title", title), el("div", "note", T("loading the game, please wait…")));
  const bar = el("div", "env-progress-bar");
  bar.setAttribute("role", "progressbar");
  bar.setAttribute("aria-label", title);
  bar.append(el("span"));
  card.append(bar);
  overlay.append(card);
  S.busy.env = true;
  app.inert = true;
  app.setAttribute("aria-busy", "true");
  if (!quiet) {
    document.body.append(overlay);
    overlay.focus({ preventScroll: true });
  }
  say("busy", title);
  try {
    // Let the browser paint before asking Go to do the potentially slow switch (a hidden or
    // minimised window gets no frames: go on after a moment anyway).
    await new Promise((resolve) => { requestAnimationFrame(() => requestAnimationFrame(resolve)); setTimeout(resolve, 100); });
    await action();
    return true;
  } finally {
    S.busy.env = false;
    app.inert = wasInert;
    if (wasBusy === null) app.removeAttribute("aria-busy");
    else app.setAttribute("aria-busy", wasBusy);
    overlay.remove();
    if (focus && focus.isConnected && !app.inert) focus.focus({ preventScroll: true });
  }
}
