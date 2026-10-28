// Variable-height log rows: keep only the viewport and a small margin in the DOM.
// The spacer can follow a header in the same scroll container.
export function virtualRows(scroll, count, renderRow, saved) {
  const spacer = document.createElement("div");
  spacer.style.position = "relative";
  spacer.style.overflowAnchor = "none";
  scroll.append(spacer);
  const heights = new Float64Array(count).fill(80);
  if (saved) heights.set(saved.heights.subarray(0, count));
  const offsets = new Float64Array(count + 1);
  const nodes = new Map();
  let selected = -1, frame = 0, dead = false, layoutPending = false, correctionPending = 0;

  function layout() {
    for (let i = 0; i < count; i++) offsets[i + 1] = offsets[i] + heights[i];
    spacer.style.height = offsets[count] + "px";
    for (const [i, row] of nodes) row.style.top = offsets[i] + "px";
  }
  function origin() {
    return spacer.getBoundingClientRect().top - scroll.getBoundingClientRect().top + scroll.scrollTop - scroll.clientTop;
  }
  function indexAt(y) {
    let lo = 0, hi = count;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (offsets[mid + 1] <= y) lo = mid + 1;
      else hi = mid;
    }
    return lo;
  }
  function schedule() {
    if (!dead && !frame) frame = requestAnimationFrame(() => {
      frame = 0;
      if (dead) return;
      if (layoutPending) {
        const top = scroll.scrollTop;
        layoutPending = false;
        layout();
        // Layout can clamp scrollTop when the spacer shrinks. Preserve the value
        // before layout, including any player scroll since the resize notification.
        scroll.scrollTop = top + correctionPending;
        correctionPending = 0;
      }
      draw();
    });
  }
  const observer = new ResizeObserver((entries) => {
    let changed = false;
    const anchor = indexAt(Math.max(0, scroll.scrollTop - origin()));
    let correction = 0;
    for (const entry of entries) {
      if (entry.target === scroll) { schedule(); continue; }
      const i = +entry.target.dataset.virtualIndex;
      if (nodes.get(i) !== entry.target) continue;
      const h = entry.target.getBoundingClientRect().height;
      if (h > 0 && Math.abs(h - heights[i]) > 0.5) {
        if (i < anchor) correction += h - heights[i];
        heights[i] = h;
        changed = true;
      }
    }
    if (changed) {
      // Resize delivery only measures. Writing the spacer/row layout here can
      // resize the observed scroll box again and trigger a ResizeObserver loop.
      layoutPending = true;
      correctionPending += correction;
      schedule();
    }
  });
  function draw() {
    if (dead) return;
    const y = Math.max(0, scroll.scrollTop - origin());
    const start = Math.max(0, indexAt(y) - 6);
    const end = Math.min(count, indexAt(y + scroll.clientHeight) + 7);
    for (const [i, row] of nodes) {
      if (i < start || i >= end) {
        observer.unobserve(row);
        row.remove();
        nodes.delete(i);
      }
    }
    for (let i = start; i < end; i++) {
      let row = nodes.get(i);
      if (!row) {
        row = renderRow(i);
        row.dataset.virtualIndex = String(i);
        Object.assign(row.style, { position: "absolute", top: offsets[i] + "px", left: "0", width: "100%", boxSizing: "border-box" });
        nodes.set(i, row);
        spacer.append(row);
        observer.observe(row);
      }
      row.classList.toggle("cur", i === selected);
      const inner = row.querySelector?.(":scope > .lrow");
      if (inner) inner.classList.toggle("cur", i === selected);
    }
  }
  layout();
  observer.observe(scroll);
  scroll.addEventListener("scroll", schedule, { passive: true });
  // Construction can precede attachment to the page; measure on the next frame.
  schedule();
  return {
    // Capture before destroy removes the spacer and the browser clamps scrollTop.
    // Pending measurements already changed heights, but not the scroll coordinates.
    snapshot() {
      return { heights: heights.slice(), scrollTop: scroll.scrollTop + correctionPending };
    },
    update(index, paint) {
      const row = nodes.get(index);
      if (!dead && row) paint(row); // ResizeObserver measures the new inline details.
    },
    select(index, reveal = true) {
      if (dead || !count) return;
      selected = Math.max(0, Math.min(count - 1, index));
      if (reveal) {
        const top = origin() + offsets[selected], bottom = top + heights[selected];
        if (top < scroll.scrollTop) scroll.scrollTop = top;
        else if (bottom > scroll.scrollTop + scroll.clientHeight) scroll.scrollTop = bottom - scroll.clientHeight;
      }
      draw();
    },
    destroy() {
      dead = true;
      cancelAnimationFrame(frame);
      observer.disconnect();
      scroll.removeEventListener("scroll", schedule);
      nodes.clear();
      spacer.remove();
    },
  };
}
