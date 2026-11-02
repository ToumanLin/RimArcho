// Tells Go the player is using the page, so an idle window can lower WebView2's memory
// and get it back at once on the next input (winmem_policy.go, 07 §8.3). At most one call
// every inputEvery; the first input after a quiet spell is always sent.
import { api } from "./api.js";

const inputEvery = 30000;
let last = 0;
function used() {
  const now = performance.now();
  if (now - last < inputEvery) return;
  last = now;
  api.PageInput().catch(() => {});
}
for (const type of ["pointerdown", "pointermove", "wheel", "keydown"]) {
  window.addEventListener(type, used, { capture: true, passive: true });
}
