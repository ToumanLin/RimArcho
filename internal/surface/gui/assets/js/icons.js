// Icons (06 §7): sources and row states, drawn as small SVGs in currentColor.
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
  rescan: svg('<path d="M13.2 8a5.2 5.2 0 1 1-1.6-3.8"/><path d="M13.4 2.6v2.8h-2.8"/>'),
  ok: svg('<path d="M3.2 8.4l3 3 6.6-6.8"/>', "ok"),
  // the sidebar (06 §2.1)
  sidebar: svg('<rect x="2" y="2.8" width="12" height="10.4" rx="2"/><path d="M6.2 2.8v10.4"/>'),
  home: svg('<path d="M2.6 7.4L8 2.8l5.4 4.6"/><path d="M4.2 6.2v6.8h7.6V6.2"/>'),
  needs: svg('<circle cx="8" cy="8" r="5.8"/><path d="M5.6 8.2l1.7 1.7 3.2-3.4"/>'),
  plan: svg('<path d="M5.5 4h8M5.5 8h8M5.5 12h8"/><circle cx="2.8" cy="4" r=".6"/><circle cx="2.8" cy="8" r=".6"/><circle cx="2.8" cy="12" r=".6"/>'),
  packs: svg('<path d="M2.4 5L8 2.4 13.6 5v6L8 13.6 2.4 11z"/><path d="M2.4 5L8 7.6 13.6 5M8 7.6v6"/>'),
  env: svg('<rect x="2" y="2.8" width="12" height="8.4" rx="1.6"/><path d="M5.6 13.6h4.8M8 11.2v2.4"/>'),
  library: svg('<rect x="2.4" y="2.6" width="3" height="10.8" rx=".8"/><rect x="6.5" y="2.6" width="3" height="10.8" rx=".8"/><path d="M11 3.4l2.6-.6 1.4 10-2.6.6z"/>'),
  log: svg('<path d="M4 1.8h5.2L12.4 5v9.2H4z"/><path d="M9 1.8V5h3.4M6 8h4.4M6 10.6h4.4"/>'),
  settings: svg('<circle cx="8" cy="8" r="2.2"/><path d="M8 1.6v2M8 12.4v2M1.6 8h2M12.4 8h2M3.5 3.5l1.4 1.4M11.1 11.1l1.4 1.4M3.5 12.5l1.4-1.4M11.1 4.9l1.4-1.4"/>'),
  play: svg('<path d="M5 3.2v9.6L12.6 8z"/>'),
};

export function sourceIcon(kind) { return icon[kind] || icon.local; }

// statusIcon: view.Status* (none, unloadable, warn, log, err).
export function statusIcon(s) {
  return ["", icon.unloadable, icon.warn, icon.log, icon.err][s] || "";
}
