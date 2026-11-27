// The editor details' tags view (16 §2.5, §3.8; GitHub #28): the details heading switches
// between details and tags. Tags lists every tag Enabled's Mods carry, each a row with its
// colour, a bar of how many enabled Mods carry it (anywhere in their tags, so the number is
// what its filter shows) and that number, most first; then no tag, the enabled Mods without a
// tag. A click on a row filters to it (tag:<tag> or has:notag), again takes the filter away;
// the pencil shows the tag in the Tag manager. ↑ ↓ move between rows. The element is rebuilt
// only when its numbers, colours, the filter or the language change.
import { el, T, Tf } from "./state.js";
import { tagCSS } from "./tagcolor.js";
import { queryParts } from "./searchsuggest.js";
import { icon } from "./icons.js";

// tally: every tag of items with how many items carry it, most first; and the untagged.
export function tally(items, colors) {
  const by = new Map();
  let none = 0;
  for (const it of items) {
    if (!it.tags || !it.tags.length) { none++; continue; }
    const seen = new Set();
    for (const t of it.tags) {
      const k = t.toLowerCase();
      if (seen.has(k)) continue;
      seen.add(k);
      const s = by.get(k);
      if (s) s.n++; else by.set(k, { tag: t, key: k, n: 1, color: tagCSS(colors[k]) });
    }
  }
  const tags = [...by.values()].sort((a, b) => b.n - a.n || a.tag.localeCompare(b.tag));
  return { tags, none, total: items.length };
}

// filtersOn: the tags (lower-cased) and has:notag the query keeps, not excluded.
export function filtersOn(search) {
  const tags = new Set();
  let none = false;
  for (const p of queryParts(search || "")) {
    if (p.negative) continue;
    if (p.key === "tag" && p.value) tags.add(p.value.toLowerCase());
    if (p.key === "has" && p.value.toLowerCase() === "notag") none = true;
  }
  return { tags, none };
}

let node = null, sig = "";

// tagView returns the view for Enabled's items. act: filter(filter, on) sets a query
// condition, edit(tag) shows a tag in the Tag manager.
export function tagView(items, colors, search, act) {
  const sums = tally(items, colors);
  const on = filtersOn(search);
  const s = sums.tags.map((t) => t.tag + ":" + t.n + ":" + t.color + ":" + on.tags.has(t.key)).join("|") +
    "|" + sums.none + "|" + sums.total + "|" + on.none + "|" + (globalThis.document?.documentElement?.lang || "");
  if (node && s === sig) return node;
  sig = s;
  const box = el("div", "tagview");
  box.setAttribute("role", "list");
  box.setAttribute("aria-label", T("tags"));
  if (!sums.tags.length && !sums.none) return (node = box);
  if (!sums.tags.length) box.append(el("p", "tm-hint", T("no tags yet: select mods and press g to tag them")));
  const max = Math.max(1, sums.total);
  const row = (label, n, color, cls, filter, isOn, edit) => {
    const line = el("div", "tv-line" + cls);
    line.setAttribute("role", "listitem");
    const b = el("button", "tv-row" + (isOn ? " on" : ""));
    b.type = "button";
    b.setAttribute("aria-pressed", String(isOn));
    b.title = Tf("%d of %d enabled mods", n, sums.total);
    const mark = el("span", "tv-mark");
    if (color) mark.style.background = color;
    const bar = el("span", "tv-bar");
    const fill = el("i");
    fill.style.width = (100 * n / max).toFixed(1) + "%";
    if (color) fill.style.background = color;
    bar.append(fill);
    b.append(mark, el("span", "tv-name", label), bar, el("span", "tv-n", String(n)));
    b.onclick = () => act.filter(filter, isOn ? 0 : 1);
    line.append(b);
    if (edit) {
      const e = el("button", "tv-edit");
      e.type = "button";
      e.innerHTML = icon.edit;
      const name = T("edit") + " " + label;
      e.title = name;
      e.setAttribute("aria-label", name);
      e.onclick = () => act.edit(label);
      line.append(e);
    } else line.append(el("span", "tv-edit-gap"));
    box.append(line);
  };
  for (const t of sums.tags) row(t.tag, t.n, t.color, t.color ? "" : " plain", "tag:" + t.tag, on.tags.has(t.key), true);
  if (sums.none) row(T("no tag"), sums.none, "", " none", "has:notag", on.none, false);
  // ↑ ↓ (and Home / End) move between the rows' buttons; the editor's own arrows scroll
  box.onkeydown = (e) => {
    if (e.ctrlKey || e.altKey || e.metaKey || !["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
    const rows = [...box.querySelectorAll(".tv-row")];
    const at = rows.indexOf(e.target.closest?.(".tv-row") || e.target.closest?.(".tv-line")?.querySelector(".tv-row"));
    if (!rows.length || at < 0) return;
    e.preventDefault();
    const to = e.key === "Home" ? 0 : e.key === "End" ? rows.length - 1 : Math.max(0, Math.min(rows.length - 1, at + (e.key === "ArrowDown" ? 1 : -1)));
    rows[to].focus();
    rows[to].scrollIntoView?.({ block: "nearest" });
  };
  return (node = box);
}
