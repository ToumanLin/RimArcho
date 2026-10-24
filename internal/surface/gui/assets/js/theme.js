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
  globalThis.archoTheme = { apply, subscribe: fn => listeners.add(fn) };
  try { theme = localStorage.getItem("ui.theme"); } catch { /* Init supplies the saved value. */ }
  apply(theme);
  system.addEventListener("change", update);
})();
