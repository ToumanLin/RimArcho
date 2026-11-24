// SteamCMD's password prompt (11 §3): SteamCMD shows nothing while the password is typed,
// and an input method can swallow the keys. RimArcho never asks for the password, so it
// says three things, an icon each: before a sign-in (the account field), and in a card in
// the corner while a signed-in SteamCMD window is open ("steamcmd-signin" events).
import { el, T } from "./state.js";
import { icon } from "./icons.js";

const HINTS = [
  ["hidden", "the password stays hidden while you type: type it, then press Enter"],
  ["keyboard", "switch the input method to English first"],
  ["mouse", "right-click pastes into that window"],
];

// steamHint is the three hints as icon rows; head adds the window's name above them.
export function steamHint(head = false) {
  const box = el("div", "steam-hint");
  if (head) {
    const h = el("div", "steam-hint-head");
    h.innerHTML = icon.key;
    h.append(el("span", "", T("SteamCMD window")));
    box.append(h);
  }
  for (const [name, text] of HINTS) {
    const row = el("div", "steam-hint-row");
    row.innerHTML = icon[name];
    row.append(el("span", "", T(text)));
    box.append(row);
  }
  return box;
}

let card = null;

// steamSignIn shows the card while a signed-in SteamCMD window is open (ev.open).
export function steamSignIn(ev) {
  if (!ev.open) {
    card?.remove();
    card = null;
    return;
  }
  if (card) return;
  card = steamHint(true);
  card.classList.add("steam-hint-card");
  card.setAttribute("role", "status");
  document.body.append(card);
}
