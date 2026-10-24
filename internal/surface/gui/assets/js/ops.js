// The ops both pages share (06 §3.5, §8.5): the op table (view.OpTable: code, label, key,
// menu group) and the row menu built from a row's codes. The plan editor and Mod 库存 › 全部
// list the same groups in the same order, each item with its key on the right, only the
// ops that apply, 退订 / 删除 last in red.
import { S } from "./state.js";
import * as pop from "./popups.js";

let table = new Map();
let order = [];

// setTable takes the op table (the start data's, or Mod 库存's page).
export function setTable(list) {
  if (!list || !list.length) return;
  order = list;
  table = new Map(list.map((o) => [o.code, o]));
}

export function op(code) {
  if (!table.size && S.start && S.start.ops) setTable(S.start.ops);
  return table.get(code) || { code, label: code, group: 9 };
}

// sorted puts codes in the table's (the menu's) order.
export function sorted(codes) {
  op("");
  return order.map((o) => o.code).filter((c) => codes.includes(c));
}

// menuItems are the menu's items for codes; run(code) runs one.
export function menuItems(codes, run) {
  const out = [];
  let group = 0;
  for (const c of sorted(codes)) {
    const o = op(c);
    if (group && o.group !== group) out.push({ sep: true });
    group = o.group;
    out.push({ label: o.label, key: o.key || "", cls: o.danger ? "danger" : "", envWrite: o.group !== 3 ? c : "", run: () => run(c) });
  }
  return out;
}

// openMenu shows the row menu at x, y (keys: opened from the keyboard, the first item focused).
export function openMenu(x, y, codes, run, keys = false) {
  const items = menuItems(codes, run);
  if (items.length) pop.menu(x, y, items, keys);
}

// keyed: the op of codes that key runs ("space", "m", "Del" …), in menu order; for space
// the first plan op (启用, 加入, 停用, 拿掉, 放回, 换成这一份).
export function keyed(codes, k) {
  for (const c of sorted(codes)) {
    const o = op(c);
    if (o.key === k) return c;
    if (k === "space" && c === "use") return c; // space takes effect: this copy in the plan
  }
  return "";
}

// menuKey: the menu key or shift+F10 (06 §3.1 rule 6).
export const menuKey = (e) => e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey);
