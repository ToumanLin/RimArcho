// Slow checks (16 §2.14 "Slow checks"): after a can-it-run check that took longer than 30 s,
// one quiet card in the corner, like SteamCMD's (steamhint.js): the time as a number, one
// question, send / always send / close, and a preview that shows exactly the text that would
// go (api.SlowCheckSend sends what the window kept, never text from this page). Nothing goes
// without a click unless the player chose always send. It never takes focus.
import { el, T, api } from "./state.js";
import { icon } from "./icons.js";

let card = null;

export function slowCheck(ev) {
  card?.remove();
  card = el("section", "slow-check");
  card.setAttribute("role", "status");
  card.setAttribute("aria-label", ev.text);
  const head = el("div", "slow-check-head");
  const took = el("span", "slow-check-took num", ev.took);
  const ask = el("span", "slow-check-ask", ev.ask);
  const close = el("button", "btn quiet icon slow-check-close");
  close.innerHTML = icon.close;
  close.title = T("not now");
  close.setAttribute("aria-label", T("not now"));
  head.append(took, ask, close);

  const pre = el("pre", "slow-check-preview", ev.preview);
  pre.hidden = true;
  const btns = el("div", "slow-check-btns");
  const send = el("button", "btn primary", T("send"));
  const always = el("button", "btn quiet", T("always send"));
  const show = el("button", "btn quiet slow-check-show");
  show.innerHTML = icon.chev;
  show.append(el("span", "", T("what is sent")));
  show.setAttribute("aria-expanded", "false");
  show.onclick = () => {
    pre.hidden = !pre.hidden;
    show.setAttribute("aria-expanded", String(!pre.hidden));
    card.classList.toggle("open", !pre.hidden);
  };
  btns.append(send, always, show);
  card.append(head, btns, pre);

  const done = () => { card?.remove(); card = null; };
  close.onclick = () => { api.SlowCheckDismiss(); done(); };
  const go = async (all) => {
    send.disabled = always.disabled = true;
    const why = await api.SlowCheckSend(all).catch((e) => String(e));
    if (!why) {
      card.classList.remove("open");
      card.classList.add("sent");
      card.replaceChildren(el("div", "slow-check-sent", T("sent, thank you")));
      setTimeout(done, 2500);
      return;
    }
    send.disabled = always.disabled = false;
    card.querySelector(".slow-check-err")?.remove();
    btns.after(el("div", "slow-check-err", why));
  };
  send.onclick = () => go(false);
  always.onclick = () => go(true);
  document.body.append(card);
}
