// Query text is the only filter state. Keep raw spans so edits never rewrite
// quoted values or Windows paths elsewhere in the query.
export function queryParts(text) {
  const out = [];
  const pattern = /(?:[^\s"]|"[^"]*"?)+/g;
  for (const m of text.matchAll(pattern)) {
    const negative = m[0].startsWith("-");
    const raw = negative ? m[0].slice(1) : m[0];
    const at = raw.indexOf(":");
    let key = at < 0 ? "" : raw.slice(0, at).toLowerCase();
    if (key === "tags") key = "tag";
    const value = (at < 0 ? raw : raw.slice(at + 1)).replace(/^"|"$/g, "");
    out.push({ raw: m[0], key, value, negative, start: m.index, end: m.index + m[0].length });
  }
  return out;
}
export const quoteValue = (value) => /\s/.test(value) ? '"' + value + '"' : value;
export function queryContext(text, caret) {
  const part = queryParts(text).find((p) => caret >= p.start && caret <= p.end);
  return part ? { start: part.start, end: part.end, text: text.slice(part.start, caret) } : { start: caret, end: caret, text: "" };
}
export function suggestions(groups, text, caret) {
  const ctx = queryContext(text, caret), negative = ctx.text.startsWith("-");
  const token = negative ? ctx.text.slice(1) : ctx.text, at = token.indexOf(":");
  const prefix = negative ? "-" : "";
  if (at < 0) return groups.filter((g) => g.key !== "location" && (g.key.startsWith(token.toLowerCase()) || (g.title || "").toLowerCase().includes(token.toLowerCase())))
    .map((g) => ({ token: prefix + (g.key === "tag" ? "tags" : g.key) + ":", label: g.title, key: true }));
  const typedKey = token.slice(0, at), key = typedKey === "tags" ? "tag" : typedKey.toLowerCase();
  const part = token.slice(at + 1).replace(/^"|"$/g, "").toLowerCase();
  const g = groups.find((g) => g.key === key);
  return (g?.values || []).filter((v) => {
    const value = v.filter.slice(v.filter.indexOf(":") + 1);
    return value.toLowerCase().includes(part) || (v.label || "").toLowerCase().includes(part);
  }).map((v) => {
    const value = v.filter.slice(v.filter.indexOf(":") + 1);
    return { token: prefix + typedKey + ":" + (key === "lists" ? '"' + value + '"' : quoteValue(value)), label: v.label === value ? "" : v.label, key: false };
  });
}
export function setQueryFilter(text, filter, on) {
  const at = filter.indexOf(":"), key = filter.slice(0, at).replace(/^tags$/, "tag"), value = filter.slice(at + 1);
  const parts = queryParts(text).filter((p) => !(p.key === key && (p.value.toLowerCase() === value.toLowerCase() || !p.value))).map((p) => p.raw);
  if (on) parts.push((on < 0 ? "-" : "") + (key === "tag" ? "tags" : key) + ":" + (key === "lists" ? '"' + value + '"' : quoteValue(value)));
  return parts.join(" ");
}
