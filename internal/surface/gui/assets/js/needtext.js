// Structured mod names come from Go. All three problem surfaces share navigation and
// evidence rendering; no renderer searches names in prose.
import { S, el, T, say, errText, api } from "./state.js";

let ui;
export function initNeedText(u) { ui = u; }

export function needText(tag, cls, text, parts, activated, ownID) {
  // A sentence flows as text; word-facts lays out separate facts with flex gaps.
  const node = el(tag, cls.split(/\s+/).filter((c) => c !== "word-facts").join(" "));
  if (!parts?.length) { node.textContent = text || ""; return node; }
  for (const p of parts) {
    if ((!p.id && !p.url) || (ownID && p.id === ownID)) { node.append(document.createTextNode(p.text)); continue; }
    const link = el("a", "need-name", p.text);
    link.href = p.url || "#";
    link.title = T(p.id ? "show it in the list" : "open it in the Workshop");
    link.onkeydown = (e) => {
      if (e.key === "Enter" || e.key === " ") { e.preventDefault(); e.stopPropagation(); link.click(); }
    };
    link.onclick = async (e) => {
      e.preventDefault(); e.stopPropagation();
      activated?.();
      try {
        if (p.id) {
          if (S.screen) await ui.go("plan");
          if (!S.screen) await ui.palette({ kind: 1, id: p.id });
        } else await api.OpenURL(p.url);
      } catch (err) { say("fail", errText(err)); }
    };
    node.append(link);
  }
  return node;
}

export function needParts(it, activated) {
  const list = el("ul", "parts");
  (it.parts || []).forEach((p, i) => list.append(needText("li", "", p, it.content?.parts?.[i], activated)));
  return list;
}

export function needEvidence(ev) {
  const line = el("div", "ev-line");
  line.append(el("span", "ev-chip " + ev.kind, ev.label));
  // two spaces part facts in the source (16 §7.3): each is its own span, a gap apart
  if (ev.source) for (const part of ev.source.split(/ {2,}/)) if (part) line.append(el("span", "src", part));
  return line;
}
