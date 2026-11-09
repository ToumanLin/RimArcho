// Run before the stylesheet: the cache avoids a flash while Init reads the saved config.
(() => {
  const root = document.documentElement;
  const system = matchMedia("(prefers-color-scheme: dark)");
  const listeners = new Set();
  let theme = "auto";
  function update() {
    root.dataset.theme = theme;
    root.dataset.scheme = theme === "auto" ? (system.matches ? "dark" : "light") : theme;
    listeners.forEach(fn => fn());
  }
  function apply(value) {
    theme = ["light", "dark"].includes(value) ? value : "auto";
    try { localStorage.setItem("ui.theme", theme); } catch { /* Storage can be disabled. */ }
    update();
  }
  // ui.motion (16 §2.14): "reduced" stills the window as the system's reduced motion does
  // (style.css, motion.js still); cached so the first drawing already follows it
  function motion(value) {
    root.dataset.motion = value === "reduced" ? "reduced" : "auto";
    try { localStorage.setItem("ui.motion", root.dataset.motion); } catch { /* Storage can be disabled. */ }
  }
  globalThis.archoTheme = { apply, motion, subscribe: fn => listeners.add(fn) };
  try { theme = localStorage.getItem("ui.theme"); } catch { /* Init supplies the saved value. */ }
  apply(theme);
  let saved = "auto";
  try { saved = localStorage.getItem("ui.motion"); } catch { /* Init supplies the saved value. */ }
  motion(saved);
  system.addEventListener("change", update);
})();
