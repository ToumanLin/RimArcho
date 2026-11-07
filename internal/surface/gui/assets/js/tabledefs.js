// Numeric values travel alongside formatted cells so sorting is language independent (16 §3.9).
const text = (key) => (a, b) => (a[key] || "").localeCompare(b[key] || "", undefined, { numeric: true });
const number = (key) => (a, b) => (a[key] || 0) - (b[key] || 0);
const name = { key: "name", title: "name", w: 220, fixed: true, dir: 1, cmp: text("name") };
export const packColumns = [
  { ...name, title: "pack name" },
  { key: "mods", title: "mods", w: 80, dir: -1, num: true, cmp: number("mods") },
  { key: "size", title: "size", w: 100, dir: -1, num: true, cmp: number("bytes") },
  { key: "versions", title: "game version", w: 120, dir: -1, cmp: text("versions") },
  { key: "played", title: "last played", w: 160, dir: -1, cmp: (a, b) => (Date.parse(a.playedAt) || 0) - (Date.parse(b.playedAt) || 0) },
];
export const packModColumns = [
  name,
  { key: "order", title: "load order", w: 100, dir: 1, num: true, cmp: number("order") },
  { key: "size", title: "size", w: 100, dir: -1, num: true, cmp: number("bytes") },
  { key: "note", title: "version folders left out", w: 240, dir: 1, cmp: text("note") },
];
export const textureRank = (r) => r.stale ? 0 : r.convert ? 1 : 2;
export const textureColumns = [
  name,
  { key: "pngCount", title: "PNG count", w: 90, dir: -1, num: true, cmp: number("pngCount") },
  { key: "currentSize", title: "current size", w: 120, dir: -1, num: true, cmp: number("pngBytes") },
  { key: "estimate", title: "estimated size", w: 120, dir: -1, num: true, cmp: number("savedBytes") },
  { key: "state", title: "progress / state", w: 230, dir: 1, cmp: (a, b) => textureRank(a) - textureRank(b) || (b.stale || 0) - (a.stale || 0) || (b.convert || 0) - (a.convert || 0) },
];
// Mod library › Mod settings (16 §2.9): one row per settings file.
export const modSettingsColumns = [
  { ...name, title: "mod", w: 160 },
  { key: "title", title: "settings window", w: 160, dir: 1, cmp: text("title") },
  { key: "values", title: "stored values", w: 84, dir: -1, num: true, cmp: number("count") },
  { key: "size", title: "size", w: 64, dir: -1, num: true, cmp: number("bytes") },
  { key: "file", title: "file", w: 260, shown: false, dir: 1, cmp: text("file") },
];
