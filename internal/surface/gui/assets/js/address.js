// Fit a display label without changing the full value used by hover and copy.
export function middleEllipsis(text, width, measure) {
  if (measure(text) <= width) return text;
  if (measure("…") > width) return "";
  const chars = Array.from(text);
  const candidate = count => {
    const head = Math.ceil(count * .4), tail = count - head;
    return chars.slice(0, head).join("") + "…" + (tail ? chars.slice(-tail).join("") : "");
  };
  let lo = 0, hi = chars.length - 1;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (measure(candidate(mid)) <= width) lo = mid;
    else hi = mid - 1;
  }
  // Prefer whole directories when they use most of the available space.
  const cuts = [...text.matchAll(/[\\/]/g)].map(m => m.index + 1);
  const start = /^https?:\/\//i.test(text) ? cuts.findIndex(n => n > text.indexOf("://") + 3) : 0;
  for (const head of [cuts[start + 2], cuts[start + 1], cuts[start]]) {
    for (const tail of [cuts.at(-2), cuts.at(-1)]) {
      if (!head || !tail || head >= tail) continue;
      const value = text.slice(0, head) + "…" + text.slice(tail);
      if (Array.from(value).length >= lo * .7 && measure(value) <= width) return value;
    }
  }
  return candidate(lo);
}

let resize, mutations, canvas;
function fit(node) {
  const width = node.getBoundingClientRect().width;
  if (!width) return;
  canvas ||= document.createElement("canvas").getContext("2d");
  const style = getComputedStyle(node);
  // Chromium leaves the font shorthand empty when inherited font-variant is set.
  canvas.font = `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
  const spacing = parseFloat(style.letterSpacing) || 0;
  const measure = text => canvas.measureText(text).width + Array.from(text).length * spacing;
  const label = node.firstElementChild;
  const text = middleEllipsis(node.dataset.address, width, measure);
  if (label.textContent !== text) label.textContent = text;
}

// One observer for visible labels; detached pages release their observations. The
// sizing pseudo-element keeps the original intrinsic width when a label is shortened.
export function addressLabel(text) {
  const node = document.createElement("span");
  node.className = "address-text";
  node.dataset.address = text;
  const label = document.createElement("span");
  label.className = "address-label";
  label.textContent = text;
  node.append(label);
  if (!resize) {
    resize = new ResizeObserver(entries => entries.forEach(e => fit(e.target)));
    const visit = (root, fn) => {
      if (root.nodeType !== Node.ELEMENT_NODE) return;
      if (root.matches(".address-text")) fn(root);
      root.querySelectorAll(".address-text").forEach(fn);
    };
    mutations = new MutationObserver(records => {
      for (const record of records) {
        for (const root of record.removedNodes) visit(root, n => { if (!n.isConnected) resize.unobserve(n); });
        for (const root of record.addedNodes) visit(root, n => { if (n.isConnected) resize.observe(n); });
      }
    });
    mutations.observe(document.documentElement, { childList: true, subtree: true });
    document.fonts?.ready.then(() => document.querySelectorAll(".address-text").forEach(fit));
  }
  return node;
}
