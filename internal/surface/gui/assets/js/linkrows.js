import { el } from "./state.js";
import { copyablePath } from "./fields.js";

// Preserve the leaf of long paths; the full value is available to copy and hover.
function path(value, label = value) {
  const node = el("span", "link-path");
  node.textContent = label;
  return copyablePath(node, value);
}

export function linkRows(groups) {
  const body = el("div", "link-groups");
  for (const group of groups || []) {
    const section = el("section", "link-group");
    const head = el("div", "link-game");
    head.append(el("strong", "", group.game), path(group.path));
    section.append(head);
    for (const item of group.rows || []) {
      const row = el("div", "link-row");
      row.append(path(item.path, item.name), el("span", "link-arrow", "→"), path(item.target), el("span", "link-reason", item.reason), el("span", "link-tag", item.action));
      section.append(row);
    }
    body.append(section);
  }
  return body;
}
