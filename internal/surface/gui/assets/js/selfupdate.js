// RimArcho's own update in the navigation (16 §2.1, 17 §5.1): while a newer version waits,
// a small accent button "↑ update" sits beside Settings (navigation at the side) or before
// background tasks (at the top; shell.js places it). It opens the RimArcho row in
// settings › About with the install button focused, as the launch page's line does. Its ×
// hides it until a newer version is found; the line and the settings row stay.
import { $, el, api, T, Tf, say } from "./state.js";
import { on } from "./api.js";
import { icon } from "./icons.js";

const HIDDEN_KEY = "ui.gui.update.hidden";
let ui = null;
let line = null;   // view.SelfUpdateLine, or null
let hidden = "";   // the version the player hid

export function initSelfUpdate(u) {
  ui = u;
  // a check found a new version (automatic, or settings' "check for updates")
  on("automatic-update", (ev) => { if (ev?.kind === "self") refresh(); });
  document.addEventListener("task-ended", (e) => { if (e.detail?.kind === "self-check") refresh(); });
  // another window, the TUI or the CLI may have checked meanwhile
  addEventListener("focus", () => refresh());
  api.Setting(HIDDEN_KEY).then((v) => { hidden = v || ""; }).catch(() => {}).finally(refresh);
}

export async function refresh() {
  try { line = await api.SelfUpdateLine(); } catch { return; }
  draw();
}

// draw: also after a language change, for the labels.
export function draw() {
  const box = $("#self-update");
  if (!box) return;
  const v = line && line.version;
  box.hidden = !v || v === hidden;
  if (box.hidden) return box.replaceChildren();
  const go = el("button", "su-go");
  const ic = el("span", "ic");
  ic.innerHTML = icon.up;
  go.append(ic, el("span", "lbl", T("update")));
  go.title = Tf("RimArcho %s can be updated", v);
  go.onclick = goSelfUpdate;
  const x = el("button", "su-x");
  x.innerHTML = icon.close;
  x.title = T("hide until a newer version");
  x.setAttribute("aria-label", T("hide"));
  x.onclick = () => {
    hidden = v;
    api.SetSetting(HIDDEN_KEY, v).catch(() => {});
    draw();
    say("", T("hidden; it comes back with a newer version"));
  };
  box.replaceChildren(go, x);
}

// goSelfUpdate opens the RimArcho row in settings › About, the install button focused.
export async function goSelfUpdate() {
  await ui.go("settings");
  const row = document.querySelector('[data-setting-action="update"]')?.closest("[data-setting-row]")
    || document.querySelector('[data-setting-action="update --check"]')?.closest("[data-setting-row]");
  row?.scrollIntoView({ block: "center" });
  row?.querySelector("button")?.focus({ preventScroll: true });
}
