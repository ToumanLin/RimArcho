// The start-up screen's still, chosen before main.js and Init (16 §7.1 "Archo figure"):
// the cached ui.archo.motion and the round-robin expression come from localStorage. CSS shows
// the screen only after 0.21 s, so a fast start never sees it; js/archo.js takes over from here.
(() => {
  const box = document.getElementById("boot");
  if (!box) return;
  let mode = "live", turn = 0;
  try { mode = localStorage.getItem("ui.archo.motion") || "live"; turn = +localStorage.getItem("archo.turn") || 0; } catch { /* defaults */ }
  if (mode === "off") { box.remove(); return; }
  const pool = ["neutral", "smile", "laugh", "wink", "think", "ponder", "yum"];
  const expr = pool[turn % pool.length];
  try { localStorage.setItem("archo.turn", (turn + 1) % 1000); } catch { /* storage can be off */ }
  const img = new Image();
  img.alt = ""; img.decoding = "async"; img.draggable = false;
  img.dataset.expr = expr;
  img.src = "/archo/archo-s/stills/" + expr + "@360.webp";
  box.querySelector(".archo-slot").append(img);
})();
