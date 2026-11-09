// A Mod's tags in its order, as chips the player drags, or moves with ctrl+← / ctrl+→, to
// reorder them. The first coloured tag marks the Mod's rows (16 §2.9 Tag manager); it is
// underlined here so the order's effect shows.
import { T } from "./state.js";
import { markOf, tagCSS, tagColors } from "./tagcolor.js";

const el = (tag, cls) => { const e = document.createElement(tag); e.className = cls; return e; };
const Tf = (f, ...a) => { let i = 0; return T(f).replace(/%[sd]/g, () => String(a[i++])); };

// tagOrderChips returns the chips; onOrder(list) gets the new order after each move.
export function tagOrderChips(tags, onOrder, cls = "chip") {
  const box = el("span", "tag-order");
  let list = [...tags], from = -1;
  const clear = () => { for (const c of box.children) c.classList.remove("drop-before", "drop-after"); };
  const after = (c, e) => { const r = c.getBoundingClientRect(); return e.clientX > r.left + r.width / 2; };
  const move = (i, to, focus) => {
    if (i < 0 || i >= list.length) return;
    const next = [...list], [t] = next.splice(i, 1);
    if (to > i) to--;
    to = Math.max(0, Math.min(to, next.length));
    next.splice(to, 0, t);
    if (next.join("\n") === list.join("\n")) return;
    list = next;
    draw(focus ? to : -1);
    onOrder(list);
  };
  const draw = (focus = -1) => {
    box.replaceChildren();
    const lead = markOf(list)?.tag;
    list.forEach((t, i) => {
      const c = el("span", cls + " tag-drag" + (t === lead ? " lead" : ""));
      const css = tagCSS(tagColors()[t.toLowerCase()]);
      if (css) {
        const m = el("span", "tm-mark");
        m.style.background = css;
        m.setAttribute("aria-hidden", "true");
        c.append(m);
      }
      c.append(document.createTextNode(t));
      if (list.length > 1) {
        c.draggable = true;
        c.tabIndex = 0;
        c.title = (t === lead ? T("this tag's colour marks the Mod") + "\n" : "") + T("drag to reorder; ctrl+← / ctrl+→ moves it");
        c.setAttribute("aria-label", Tf("%s, tag %d of %d", t, i + 1, list.length) + (t === lead ? T("; its colour marks the Mod") : ""));
        c.ondragstart = (e) => { from = i; e.dataTransfer.effectAllowed = "move"; e.dataTransfer.setData("text/plain", t); c.classList.add("dragging"); };
        c.ondragend = () => { from = -1; c.classList.remove("dragging"); clear(); };
        c.ondragover = (e) => {
          if (from < 0) return;
          e.preventDefault();
          clear();
          c.classList.add(after(c, e) ? "drop-after" : "drop-before");
        };
        c.ondrop = (e) => {
          if (from < 0) return;
          e.preventDefault();
          const f = from;
          from = -1;
          clear();
          move(f, i + (after(c, e) ? 1 : 0), false);
        };
        c.onkeydown = (e) => {
          if (!e.ctrlKey || e.altKey || (e.key !== "ArrowLeft" && e.key !== "ArrowRight")) return;
          e.preventDefault();
          e.stopPropagation();
          move(i, e.key === "ArrowLeft" ? i - 1 : i + 2, true);
        };
      }
      box.append(c);
    });
    if (focus >= 0) box.children[focus]?.focus();
  };
  draw();
  return box;
}
