// The order comparison of two Mod lists, side by side (16 §2.18 "Order comparison"): the
// compared list on the left, the incoming one on the right, each row numbered and tinted by
// what happens to it (removed, added, moved). Long runs of unchanged rows fold. The words are
// on hover and in aria labels (16 §7 principle 9).
import { el, T, Tf } from "./state.js";
import { icon } from "./icons.js";

// a fold keeps this many rows at each end of an unchanged run, and folds only what is longer
const KEEP = 1, FOLD_MIN = 3;

// orderDiff classifies both lists. A row's st is "gone" (only on the left: the incoming list
// drops it), "added" (only on the right), "moved" (its order among the shared Mods changes),
// "shift" (the same order, another number) or "same". Moved rows that stay together on both
// sides share a block.
export function orderDiff(current, incoming) {
  const A = unique(current), B = unique(incoming);
  const ia = new Map(A.map((r, i) => [r.key, i])), ib = new Map(B.map((r, i) => [r.key, i]));
  const keep = lcs(A.filter(r => ib.has(r.key)).map(r => r.key), B.filter(r => ia.has(r.key)).map(r => r.key));
  const st = (k, i, other) => !other.has(k) ? null : keep.has(k) ? (i === other.get(k) ? "same" : "shift") : "moved";
  const left = A.map((r, i) => ({ ...r, n: i + 1, st: st(r.key, i, ib) || "gone" }));
  const right = B.map((r, i) => ({ ...r, n: i + 1, st: st(r.key, i, ia) || "added" }));
  let block = -1, prev = -2;
  for (const r of right) {
    if (r.st !== "moved") { prev = -2; continue; }
    const li = ia.get(r.key);
    if (li !== prev + 1) block++;
    r.block = left[li].block = block;
    prev = li;
  }
  const n = (rows, s) => rows.filter(r => r.st === s).length;
  return {
    left, right,
    counts: {
      gone: n(left, "gone"), added: n(right, "added"), moved: n(right, "moved"),
      same: n(right, "same") + n(right, "shift"), shift: n(right, "shift"),
      missing: right.filter(r => r.missing).length, other: right.filter(r => r.other).length,
    },
  };
}

function unique(rows) {
  const seen = new Set();
  return (rows || []).filter(r => r && r.key && !seen.has(r.key) && seen.add(r.key));
}

// lcs: the keys of a longest common subsequence of a and b (the Mods whose order is kept)
export function lcs(a, b) {
  const n = a.length, m = b.length, w = m + 1;
  const T_ = n * w < 50e6 ? new Uint16Array((n + 1) * w) : null;
  if (!T_ || n > 65000 || m > 65000) return new Set();
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--)
    T_[i * w + j] = a[i] === b[j] ? T_[(i + 1) * w + j + 1] + 1 : Math.max(T_[(i + 1) * w + j], T_[i * w + j + 1]);
  const out = new Set();
  for (let i = 0, j = 0; i < n && j < m;) {
    if (a[i] === b[j]) { out.add(a[i]); i++; j++; }
    else if (T_[(i + 1) * w + j] >= T_[i * w + j + 1]) i++;
    else j++;
  }
  return out;
}

// folded: a side's rows with long unchanged runs folded into one row, unless the run is open
export function folded(rows, side, fold, open) {
  if (!fold) return rows.map(r => ({ row: r }));
  const out = [];
  for (let i = 0; i < rows.length;) {
    if (rows[i].st !== "same" && rows[i].st !== "shift") { out.push({ row: rows[i++] }); continue; }
    let j = i;
    while (j < rows.length && (rows[j].st === "same" || rows[j].st === "shift")) j++;
    const id = side + i, len = j - i, keepHead = KEEP, keepTail = j === rows.length ? 0 : KEEP;
    if (len - keepHead - keepTail < FOLD_MIN || open.has(id)) for (let k = i; k < j; k++) out.push({ row: rows[k] });
    else {
      for (let k = i; k < i + keepHead; k++) out.push({ row: rows[k] });
      out.push({ fold: len - keepHead - keepTail, id });
      for (let k = j - keepTail; k < j; k++) out.push({ row: rows[k] });
    }
    i = j;
  }
  return out;
}

// LEGEND: what each tint means, in the order the legend draws them
const LEGEND = () => [
  // the words are the import effect's own ("%d added"), so each language keeps its word order
  { k: "gone", word: T("%d removed"), tip: (n, l, r) => Tf("%d in %s, not in %s", n, l, r) },
  { k: "added", word: T("%d added"), tip: (n, l, r) => Tf("%d in %s, not in %s", n, r, l) },
  { k: "moved", word: T("%d moved"), tip: (n) => Tf("%d change places", n) },
  { k: "same", word: T("%d unchanged"), tip: (n, l, r, c) => c.shift ? Tf("%d in the same order; %d of them at another number", n, c.shift) : Tf("%d in the same order", n) },
];

// legend: the counts as dots and numbers, each named on hover
export function legend(d, leftName, rightName, vertical = false) {
  const box = el("div", "ld-legend" + (vertical ? " v" : ""));
  for (const x of LEGEND()) {
    const n = d.counts[x.k];
    const c = el("span", "ld-k k-" + x.k + (n ? "" : " zero"));
    const tip = x.tip(n, leftName, rightName, d.counts);
    c.title = tip; c.setAttribute("aria-label", tip);
    c.append(el("i", "dot"));
    // the number in bold where the language puts it
    const [before, after = ""] = x.word.split("%d");
    if (before.trim()) c.append(el("span", "w", before.trim()));
    c.append(el("b", "num", String(n)));
    if (after.trim()) c.append(el("span", "w", after.trim()));
    box.append(c);
  }
  return box;
}

// orderView draws the comparison: the two lists side by side, long unchanged runs folded (a
// click opens one). opts: left and right (the lists' names), rightTip (where the incoming list
// comes from, on hover), rightNote (an amber mark: another game version) and its tip.
// Hovering a Mod lights it on both sides. It returns { el, redraw }.
export function orderView(d, opts) {
  const open = new Set();
  const root = el("div", "ld");
  const heads = el("div", "ld-heads");
  const head = (name, n, side, tip = "", note = "", noteTip = "") => {
    const h = el("div", "ld-h " + side);
    h.title = tip || name;
    h.append(el("span", "hn", name), el("span", "hc num", String(n)));
    if (note) { const w = el("span", "hw", note); w.title = noteTip; h.append(w); }
    return h;
  };
  heads.append(head(opts.left, d.left.length, "l"), head(opts.right, d.right.length, "r", opts.rightTip, opts.rightNote, opts.rightNoteTip));
  const body = el("div", "ld-body");
  root.append(heads, body);
  const redraw = () => {
    body.textContent = "";
    const L = folded(d.left, "l", true, open), R = folded(d.right, "r", true, open);
    const grid = el("div", "ld-grid");
    const col = (items, side) => {
      const c = el("div", "ld-col " + side);
      for (const it of items) c.append(it.fold ? foldRow(it, () => { open.add(it.id); redraw(); }) : modRow(it.row));
      return c;
    };
    grid.append(col(L, "l"), col(R, "r"));
    body.append(grid);
    grid.onmouseover = e => {
      const k = e.target.closest?.(".ld-row")?.dataset.key || "";
      if (k === grid.dataset.hl) return;
      for (const x of grid.querySelectorAll(".hl")) x.classList.remove("hl");
      grid.dataset.hl = k;
      if (k) for (const x of grid.querySelectorAll(".ld-row")) if (x.dataset.key === k) x.classList.add("hl");
    };
  };
  redraw();
  return { el: root, redraw };
}

const STATE_WORD = () => ({ gone: T("not in the incoming list"), added: T("not in your list"), moved: T("changes place"), shift: T("same order, another number"), same: T("same place") });

function modRow(r) {
  const row = el("div", "ld-row s-" + r.st + (r.missing ? " miss" : ""));
  row.dataset.key = r.key;
  const nm = el("span", "nm", r.name);
  row.append(el("span", "n num", String(r.n)), nm);
  const words = [STATE_WORD()[r.st]];
  if (r.missing) { const m = el("span", "ld-ic miss"); m.innerHTML = icon.close; row.append(m); words.push(T("not installed")); }
  else if (r.other) { const m = el("span", "ld-ic other"); m.innerHTML = icon.vers; row.append(m); words.push(T("installed in another version")); }
  row.title = r.name + "\n" + words.join(", ");
  return row;
}

function foldRow(it, unfold) {
  const b = el("button", "ld-row ld-fold");
  b.type = "button";
  const ic = el("span", "n"); ic.innerHTML = icon.more || "⋮";
  b.append(ic, el("span", "nm", Tf("%d unchanged, folded", it.fold)));
  b.title = T("unfold");
  b.onclick = unfold;
  return b;
}
