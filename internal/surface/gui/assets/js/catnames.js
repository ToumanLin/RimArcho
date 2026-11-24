// A Mod's category names (16 §2.5) as view.CategoryText says them, in English (T translates).
// No imports: lists.js and fields.js use it, and their test suites stub the rest.
export const CATEGORY_KEYS = ["game", "library", "content", "code", "patches", "translation", "textures"];

export function categoryName(k) {
  return { game: "game & DLC", library: "library", content: "content", code: "code", patches: "patches", translation: "translation", textures: "textures" }[k] || "none";
}
