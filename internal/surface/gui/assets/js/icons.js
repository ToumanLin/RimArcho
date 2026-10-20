// Icons (06 §3): sources and row states, drawn as small SVGs in currentColor.
const svg = (body, cls = "") => `<svg class="i ${cls}" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`;

export const icon = {
  official: svg('<path d="M8 1.8l1.9 3.9 4.3.6-3.1 3 .7 4.3L8 11.6l-3.8 2 .7-4.3-3.1-3 4.3-.6z"/>'),
  workshop: svg('<circle cx="8" cy="8" r="5.6"/><circle cx="8" cy="8" r="2"/>'),
  local: svg('<rect x="2.5" y="3" width="11" height="10" rx="1.6"/><path d="M2.5 6h11"/>'),
  copy: svg('<rect x="4.5" y="4.5" width="9" height="9" rx="1.5"/><path d="M2.5 10.5v-7a1 1 0 0 1 1-1h7"/>'),
  git: svg('<circle cx="4.5" cy="4" r="1.6"/><circle cx="4.5" cy="12" r="1.6"/><circle cx="11.5" cy="6" r="1.6"/><path d="M4.5 5.6v4.8M11.5 7.6c0 2-2 2.6-5.4 3.2"/>'),
  err: svg('<circle cx="8" cy="8" r="6"/><path d="M5.8 5.8l4.4 4.4M10.2 5.8l-4.4 4.4"/>', "err"),
  warn: svg('<path d="M8 2.2l6.2 11H1.8z"/><path d="M8 6.6v3M8 11.4v.2"/>', "warn"),
  log: svg('<rect x="2.5" y="2.5" width="11" height="11" rx="2"/><path d="M5 6h6M5 8.5h6M5 11h3.5"/>', "log"),
  unloadable: svg('<circle cx="8" cy="8" r="5.6" stroke-dasharray="2.2 2"/>', "dim"),
  ok: svg('<path d="M3.2 8.4l3 3 6.6-6.8"/>', "ok"),
};

export function sourceIcon(kind) { return icon[kind] || icon.local; }

// statusIcon: view.Status* (none, unloadable, warn, log, err).
export function statusIcon(s) {
  return ["", icon.unloadable, icon.warn, icon.log, icon.err][s] || "";
}
