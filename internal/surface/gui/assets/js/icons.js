// Icons (06 §7): sources and row states, drawn as small SVGs in currentColor.
const svg = (body, cls = "") => `<svg class="i ${cls}" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`;

export const icon = {
  // Texture optimisation: a picture
  textures: svg('<rect x="2.2" y="2.8" width="11.6" height="10.4" rx="1.8"/><circle cx="5.8" cy="6.2" r="1.2"/><path d="M2.6 11.6l3.6-3.2 2.6 2.2 2-1.6 2.8 2.4"/>'),
  official: svg('<path d="M8 1.8l1.9 3.9 4.3.6-3.1 3 .7 4.3L8 11.6l-3.8 2 .7-4.3-3.1-3 4.3-.6z"/>'),
  workshop: svg('<circle cx="8" cy="8" r="5.6"/><circle cx="8" cy="8" r="2"/>'),
  local: svg('<rect x="2.5" y="3" width="11" height="10" rx="1.6"/><path d="M2.5 6h11"/>'),
  copy: svg('<rect x="4.5" y="4.5" width="9" height="9" rx="1.5"/><path d="M2.5 10.5v-7a1 1 0 0 1 1-1h7"/>'),
  git: svg('<circle cx="4.5" cy="4" r="1.6"/><circle cx="4.5" cy="12" r="1.6"/><circle cx="11.5" cy="6" r="1.6"/><path d="M4.5 5.6v4.8M11.5 7.6c0 2-2 2.6-5.4 3.2"/>'),
  err: svg('<circle cx="8" cy="8" r="6"/><path d="M5.8 5.8l4.4 4.4M10.2 5.8l-4.4 4.4"/>', "err"),
  warn: svg('<path d="M8 2.2l6.2 11H1.8z"/><path d="M8 6.6v3M8 11.4v.2"/>', "warn"),
  hint: svg('<circle cx="8" cy="8" r="6"/><path d="M8 7v4M8 4.6v.2"/>', "hint"),
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
  // Saves (06 §2.11): a save file
  saves: svg('<path d="M3 2.6h7.8l2.6 2.6v8.2H2.6V3z"/><path d="M5.2 2.6v3.2h5V2.6M5 13.4V9.2h6v4.2"/>'),
  search: svg('<circle cx="7" cy="7" r="4.2"/><path d="M10.2 10.2l3.2 3.2"/>'),
  env: svg('<rect x="2" y="2.8" width="12" height="8.4" rx="1.6"/><path d="M5.6 13.6h4.8M8 11.2v2.4"/>'),
  library: svg('<rect x="2.4" y="2.6" width="3" height="10.8" rx=".8"/><rect x="6.5" y="2.6" width="3" height="10.8" rx=".8"/><path d="M11 3.4l2.6-.6 1.4 10-2.6.6z"/>'),
  log: svg('<path d="M4 1.8h5.2L12.4 5v9.2H4z"/><path d="M9 1.8V5h3.4M6 8h4.4M6 10.6h4.4"/>'),
  settings: svg('<path d="M5.60 3.84L6.76 3.36L6.69 1.84L9.31 1.84L9.24 3.36L10.40 3.84L10.40 3.84L11.39 4.61L12.68 3.78L13.99 6.05L12.64 6.76L12.80 8.00L12.80 8.00L12.64 9.24L13.99 9.95L12.68 12.22L11.39 11.39L10.40 12.16L10.40 12.16L9.24 12.64L9.31 14.16L6.69 14.16L6.76 12.64L5.60 12.16L5.60 12.16L4.61 11.39L3.32 12.22L2.01 9.95L3.36 9.24L3.20 8.00L3.20 8.00L3.36 6.76L2.01 6.05L3.32 3.78L4.61 4.61L5.60 3.84Z"/><circle cx="8" cy="8" r="2.1"/>'),
  play: svg('<path d="M5 3.2v9.6L12.6 8z"/>', "fill"),
  plus: svg('<path d="M8 3v10M3 8h10"/>'),
  // Background tasks: stacked bars, the work in a queue
  tasks: svg('<path d="M3 4.5h10M3 8h10M3 11.5h6"/>'),
  chev: svg('<path d="M6 4l4 4-4 4"/>'),
  // the list management page (06 §2.17)
  import: svg('<path d="M8 2.4v7.2M5.2 6.8L8 9.6l2.8-2.8"/><path d="M2.8 10.4v2.2h10.4v-2.2"/>'),
  export: svg('<path d="M8 9.6V2.4M5.2 5.2L8 2.4l2.8 2.8"/><path d="M2.8 10.4v2.2h10.4v-2.2"/>'),
  more: svg('<circle cx="3.6" cy="8" r=".9"/><circle cx="8" cy="8" r=".9"/><circle cx="12.4" cy="8" r=".9"/>', "fill"),
  ext: svg('<path d="M9.4 2.6h4v4M13.2 2.8L7.6 8.4"/><path d="M11.8 9.6v3.6H2.8V4.2h3.6"/>'),
};

export function sourceIcon(kind) { return icon[kind] || icon.local; }

// statusIcon: view.Status* (none, unloadable, warn, log, err).
export function statusIcon(s) {
  if (s === -1) return icon.hint;
  return ["", icon.unloadable, icon.warn, icon.log, icon.err][s] || "";
}

// bowMark is RimArcho's mark, the bow from the portrait (06 §7), drawn simply enough for
// 16–24 px: three wings and the knot, with the accent rim to the lower left as on the
// portrait. size picks the drawing: 24 keeps one lighter facet per wing, 16 is flat. The
// colours are --bow-* (the dark theme has a lighter rim and a grey outline).
const BOW = {
  wings: [
    "M233 264C219 211 216 163 191 117L230-5Q239-13 248 16L282 89L248 209Z",
    "M219 224C194 211 129 225 94 211C75 185 88 116 91 77Q92 48 85 18C143 55 183 148 219 224Z",
    "M227 244Q162 237 121 259C87 277 62 301 42 309Q81 325 131 316L209 266Z",
  ],
  knot: "M227 243L216 231L222 224L232 235L245 222L248 243L236 258Z",
  facets: [
    "M194 116L261 63L271 91L248 199L231 155Z",
    "M95 209L166 126L186 161L151 183L177 213L144 216Z",
    "M156 258L199 253L139 310L120 317Z",
  ],
};

export function bowMark(size = 16) {
  const px = 340 / size; // drawing units per pixel at the intended size
  const paths = (ds) => ds.map((d) => `<path d="${d}"/>`).join("");
  const all = paths([...BOW.wings, BOW.knot]);
  const rim = (1.3 * px).toFixed(1);
  return `<svg class="bow" viewBox="-6 -20 340 340" aria-hidden="true">` +
    `<g class="bow-halo" fill="none" stroke-width="${(2 * px).toFixed(1)}" stroke-linejoin="round">${all}</g>` +
    `<g class="bow-rim" transform="translate(-${rim} ${rim})">${paths(BOW.wings)}</g>` +
    `<g class="bow-body">${all}</g>` +
    (size >= 24 ? `<g class="bow-facet">${paths(BOW.facets)}</g>` : "") +
    `</svg>`;
}
