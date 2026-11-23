// The start-up screen's figure (its loop, or its still with motion still or reduced), chosen before main.js and Init (16 §7.1 "Archo figure"):
// the cached ui.archo.motion and the round-robin expression come from localStorage. CSS shows
// the screen only after 0.21 s, so a fast start never sees it; js/archo.js takes over from here.
(() => {
  const box = document.getElementById("boot");
  if (!box) return;
  let mode = "live", turn = 0;
  try { mode = localStorage.getItem("ui.archo.motion") || "live"; turn = +localStorage.getItem("archo.turn") || 0; } catch { /* defaults */ }
  if (mode === "off") { box.remove(); return; }
  const pool = ["neutral", "smug", "laugh", "wink", "think", "ponder", "yum"];
  const expr = pool[turn % pool.length];
  try { localStorage.setItem("archo.turn", (turn + 1) % 1000); } catch { /* storage can be off */ }
  const img = new Image();
  img.alt = ""; img.decoding = "async"; img.draggable = false;
  img.dataset.expr = expr;
  // with motion the loop itself (no still ever shows then); theme.js has set data-motion
  const live = mode === "live" && !matchMedia("(prefers-reduced-motion: reduce)").matches && document.documentElement.dataset.motion !== "reduced";
  img.dataset.live = live ? "1" : "";
  img.src = live ? "/archo/archo-s/anim/" + expr + ".webp" : "/archo/archo-s/stills/" + expr + "@360.webp";
  box.querySelector(".archo-slot").append(img);
})();
