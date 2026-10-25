import { api } from "./api.js";

// theme.js runs before CSS and the runtime. Attach once the bridge is ready,
// then send the current scheme as well as every explicit/automatic change.
function syncWindowTheme() {
  return api.SetWindowTheme(document.documentElement.dataset.scheme === "dark")
    .catch(error => console.error("Window theme:", error));
}
globalThis.archoTheme.subscribe(syncWindowTheme);
syncWindowTheme();
