// SteamCMD's password prompt (11 §3): SteamCMD shows nothing while the password is typed,
// and an input method can swallow the keys. The card in the corner, while a signed-in
// SteamCMD window is open ("steamcmd-signin"), says three things, an icon each; when that
// window asks for the password or a Steam Guard code ("steamcmd-prompt"), the card asks for
// it and RimArcho types it into SteamCMD's window (api.SteamCMDAnswer). A password field
// turns the input method off; the field is emptied as soon as it is sent. "type it myself"
// leaves the prompt to the player, with the three hints.
import { el, T, api, errText } from "./state.js";
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
  openCard();
}

function openCard() {
  if (card) return;
  card = steamHint(true);
  card.classList.add("steam-hint-card");
  card.setAttribute("role", "status");
  document.body.append(card);
}

const ASK = {
  password: ["Steam password", "sign in"],
  guard: ["Steam Guard code", "send code"],
};

// steamPrompt shows what SteamCMD's window asks: a field for the password or a Steam Guard
// code, or "approve on your phone"; kind "" takes the question back (the run ended).
export function steamPrompt(ev) {
  openCard();
  card.querySelector(".steam-ask")?.remove();
  const kind = ev.kind || "";
  if (!kind) return;
  if (kind === "phone") {
    const row = el("div", "steam-ask steam-hint-row");
    row.innerHTML = icon.phone;
    row.append(el("span", "", T("approve the sign-in in the Steam Mobile app on your phone")));
    card.querySelector(".steam-hint-head").after(row);
    return;
  }
  const [label, send] = ASK[kind] || ASK.password;
  const form = el("form", "steam-ask");
  const input = el("input", "field small");
  input.type = kind === "password" ? "password" : "text";
  input.autocomplete = kind === "password" ? "off" : "one-time-code";
  input.spellcheck = false;
  input.placeholder = T(label);
  input.setAttribute("aria-label", T(label));
  const ok = el("button", "btn primary small", T(send));
  ok.type = "submit";
  const myself = el("button", "btn small", T("type it myself in the window"));
  myself.type = "button";
  const err = el("div", "steam-ask-err");
  const answer = async (value) => {
    input.value = "";
    ok.disabled = myself.disabled = true;
    try {
      await api.SteamCMDAnswer(kind, value);
      form.replaceWith(Object.assign(el("div", "steam-ask steam-hint-row"), { textContent: value ? T("typed into the SteamCMD window") : T("type it in the SteamCMD window") }));
    } catch (e) { err.textContent = errText(e); ok.disabled = myself.disabled = false; }
  };
  form.onsubmit = (e) => { e.preventDefault(); e.stopPropagation(); if (input.value) answer(input.value); };
  form.onkeydown = (e) => { e.stopPropagation(); if (e.key === "Escape") { e.preventDefault(); answer(""); } };
  myself.onclick = (e) => { e.stopPropagation(); answer(""); };
  const buttons = el("div", "steam-ask-buttons");
  buttons.append(ok, myself);
  form.append(input, buttons, err);
  card.querySelector(".steam-hint-head").after(form);
  input.focus();
}
