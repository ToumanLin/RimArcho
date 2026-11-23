// Interface size from anywhere (16 §2.14, §3): ctrl+wheel and ctrl+'+' / '-' / '0' step along
// Settings › Interface › interface size's choices, the same ui.zoom (API.StepZoom), so the
// window reopens at it. A wheel over something that zooms itself — the Saves page's over-time
// chart — is that thing's: it cancels the event first, and this leaves a cancelled wheel alone.
import { api } from "./api.js";
import { say, Tf, errText } from "./state.js";

let chain = Promise.resolve();
let wheel = 0; // the wheel's travel since the last step, in CSS pixels

// step queues one step (> 0 larger, < 0 smaller, 0 back to 100%); steps run in order.
export function stepZoom(dir) {
  chain = chain.then(async () => {
    try {
      const pct = await api.StepZoom(dir);
      say("ok", Tf("interface size %s", pct + "%"));
      document.querySelector('[data-setting-key="ui.zoom"]')?.showValue?.(String(pct));
    } catch (e) { say("fail", errText(e)); }
  });
  return chain;
}

// zoomKey: ctrl+'+' ('=' unshifted), ctrl+'-', ctrl+'0', on the main keys and the keypad. True
// when it took the key.
export function zoomKey(e) {
  if (!(e.ctrlKey || e.metaKey) || e.altKey) return false;
  const k = e.key, pad = e.code?.startsWith("Numpad");
  const dir = k === "+" || k === "=" ? 1 : k === "-" || k === "_" ? -1 : k === "0" && (!e.shiftKey || pad) ? 0 : null;
  if (dir === null) return false;
  e.preventDefault();
  if (!e.repeat || dir !== 0) stepZoom(dir);
  return true;
}

// wireZoom listens on the window. The keys are taken before any page or field sees them (a
// field that keeps its keys to itself would swallow ctrl+'-'), as a browser's zoom is. On the
// wheel a mouse notch (about 100 px) is one step; a touchpad's or a pinch's small deltas add
// up to one. Turning the other way starts over.
export function wireZoom() {
  window.addEventListener("keydown", (e) => { if (!e.isComposing && zoomKey(e)) e.stopPropagation(); }, true);
  window.addEventListener("wheel", (e) => {
    if (!e.ctrlKey || e.defaultPrevented) return;
    e.preventDefault();
    const dy = e.deltaMode === 1 ? e.deltaY * 40 : e.deltaMode === 2 ? e.deltaY * 800 : e.deltaY;
    if (Math.sign(dy) !== Math.sign(wheel)) wheel = 0;
    wheel += dy;
    if (Math.abs(wheel) < 50) return;
    stepZoom(wheel < 0 ? 1 : -1);
    wheel = 0;
  }, { passive: false });
}
