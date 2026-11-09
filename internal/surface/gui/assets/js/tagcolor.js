// Tag colours (16 §2.9 Tag manager): a tag's colour is a preset name or #rrggbb. A Mod is
// marked with its first coloured tag's colour, a band in its row's left slot (.m); rows
// next to each other with the same colour join into one band.
// The presets, as cmds.TagColorNames; red and amber mean problems and warnings.
export const TAG_PRESETS = ["blue", "cyan", "teal", "grass", "orange", "pink", "plum", "violet"];

// tagColors is every coloured tag's colour by lower-cased tag: the newest of the board's
// and the tag list's (both come from Go after each change). No imports: fields.js and the
// popups use it, and their test suites stub state.js.
let colors = {};
export function tagColors() { return colors; }
export function setTagColors(map) { colors = map || {}; }
// fromTagList takes the colours from API.TagList's rows.
export function fromTagList(list) {
  const m = {};
  for (const t of list || []) if (t.color) m[t.tag.toLowerCase()] = t.color;
  setTagColors(m);
}

// tagCSS is a stored colour as CSS: a preset's theme colour, or the hex itself.
export function tagCSS(v) {
  if (TAG_PRESETS.includes(v)) return `var(--tag-${v})`;
  return /^#[0-9a-f]{6}$/i.test(v || "") ? v : "";
}

// markOf is the mark of a Mod with these tags (in its order): its first coloured tag.
export function markOf(tags) {
  const m = tagColors();
  for (const t of tags || []) {
    const css = tagCSS(m[t.toLowerCase()]);
    if (css) return { tag: t, color: css };
  }
  return null;
}

// paintMark fills a row's .m slot: the colour, and whether the band goes on from the row
// above (up) and into the row below (down).
export function paintMark(span, mark, up = false, down = false) {
  if (!span) return;
  span.classList.toggle("on", !!mark);
  span.classList.toggle("up", !!mark && up);
  span.classList.toggle("down", !!mark && down);
  span.style.background = mark ? mark.color : "";
  if (mark) span.title = mark.tag; else span.removeAttribute("title");
}
