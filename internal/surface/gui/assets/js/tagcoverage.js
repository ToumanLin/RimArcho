// Tag coverage (16 §3.8, GitHub #28): Enabled's heading draws its Mods by their first tag's
// colour, one segment per tag (grey for a tag with no colour), the Mods with no tag hatched
// at the end; the segment under the pointer names its tag and count in the tooltip. The heading has no room for a printed count beside undo and sort, so the bar
// takes keyboard focus and its tooltip carries the numbers (16 §7 principle 9). A coloured or grey segment shows its tag in the Tag
// manager (the details, as a tag clicked in a Mod's details does); the hatched part, enter
// or space on the bar, toggles the filter has:notag. Drawn only when some Mod on the board has
// a tag, so a player who does not tag sees nothing. One pass over Enabled's rows, redone only
// when its numbers change.
import { el, T, Tf } from "./state.js";
import { tagCSS, tagColors } from "./tagcolor.js";
import { visTip, visTipText } from "./membar.js";

let node = null, sig = "";

// tally: the rows by first tag, most first; uncoloured tags after the coloured ones.
function tally(items, colors) {
  const by = new Map();
  let none = 0;
  for (const it of items) {
    const t = it.tags && it.tags[0];
    if (!t) { none++; continue; }
    const k = t.toLowerCase();
    const s = by.get(k);
    if (s) s.n++; else by.set(k, { tag: t, n: 1, color: tagCSS(colors[k]) });
  }
  const tags = [...by.values()].sort((a, b) => (!a.color - !b.color) || b.n - a.n || a.tag.localeCompare(b.tag));
  return { tags, none, total: items.length };
}

// anyTagged: some row of either list has a tag.
const anyTagged = (cols) => cols.some((c) => c.items.some((it) => it.tags && it.tags.length));

// tagCoverage returns the heading's coverage element for Enabled (cols[1]), or null. bar is
// the filter bar (set, search), openTag(tag) shows a tag in the Tag manager.
export function tagCoverage(cols, bar, openTag) {
  const items = cols[1].items;
  // a few microseconds for 1,300 rows; the element is rebuilt only when its numbers change
  const sums = anyTagged(cols.slice(0, 2)) && items.length ? tally(items, tagColors()) : null;
  if (!sums) return null;
  const on = /(^|\s)has:notag(\s|$)/i.test(bar?.search || "");
  const s = sums.tags.map((t) => t.tag + ":" + t.n + ":" + t.color).join("|") + "|" + sums.none + "|" + sums.total + "|" + on + "|" + document.documentElement.lang;
  if (node && s === sig) return node;
  sig = s;
  const { tags, none, total } = sums;
  const toggle = () => bar?.set("has:notag", on ? 0 : 1);
  const box = el("span", "tagcov" + (on ? " on" : ""));
  const strip = el("span", "tagcov-bar");
  for (const t of tags) {
    const seg = el("span", "seg-tag" + (t.color ? "" : " plain"));
    seg.style.flexGrow = String(t.n);
    if (t.color) seg.style.background = t.color;
    seg.dataset.tip = t.tag + "  " + t.n;
    seg.onclick = (e) => { e.stopPropagation(); openTag(t.tag); };
    strip.append(seg);
  }
  if (none) {
    const seg = el("span", "seg-none");
    seg.style.flexGrow = String(none);
    seg.dataset.tip = T("no tag") + "  " + none;
    seg.onclick = (e) => { e.stopPropagation(); toggle(); };
    strip.append(seg);
  }
  box.append(strip);
  const text = Tf("%d of %d enabled mods have a tag, %d have none", total - none, total, none) + "\n" +
    T("click a colour: that tag; the stripes or enter: the mods with no tag");
  visTip(box, text, "img", true);
  // the segment under the pointer names its tag and count, the sentence below it
  strip.onpointerover = (e) => { const seg = e.target.closest?.("[data-tip]"); if (seg && seg !== box) visTipText(box, seg.dataset.tip + "\n" + text); };
  strip.onpointerleave = () => visTipText(box, text);
  box.onmousedown = (e) => e.preventDefault();
  box.onkeydown = (e) => {
    if (e.key === "Enter" || e.key === " ") { e.preventDefault(); e.stopPropagation(); toggle(); }
  };
  node = box;
  return node;
}
