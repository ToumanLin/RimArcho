// Text cut short with an ellipsis shows its whole text on hover: a name in the sidebar, a
// row, a breadcrumb. The title is set only while the text is cut, and never over a title
// the page set itself.
const MARK = "autoTitle";

function cut(e) {
  if (!(e instanceof HTMLElement) || e.clientWidth === 0) return false;
  if (e.scrollWidth <= e.clientWidth + 1) return false;
  const cs = getComputedStyle(e);
  return cs.textOverflow === "ellipsis" || cs.overflow === "hidden" && cs.whiteSpace === "nowrap";
}

addEventListener("mouseover", (ev) => {
  let e = ev.target;
  for (let depth = 0; e && depth < 4; depth++, e = e.parentElement) {
    if (!(e instanceof HTMLElement)) continue;
    const ours = e.dataset[MARK] !== undefined;
    if (e.title && !ours) return; // the page's own title wins
    const text = (e.textContent || "").trim();
    if (text && cut(e)) {
      e.title = text;
      e.dataset[MARK] = "";
      return;
    }
    if (ours) { e.removeAttribute("title"); delete e.dataset[MARK]; }
  }
}, { passive: true });
