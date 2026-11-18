// The hint line's groups (16 §2.2, §3.1 rule 4): at most 7, ending with ? all keys. Each
// page's provider lists its actions most useful first and ends with ? itself
// (TestUIHintLines); when renderHints adds groups of its own (tail: the editor's esc that
// clears the selection), the actions before ? give way, never ?.
export const HINT_MAX = 7;

export function hintLine(list, tail = []) {
  const help = list.filter((a) => a.key === "?").slice(0, 1);
  const acts = list.filter((a) => a.key !== "?");
  const room = Math.max(0, HINT_MAX - tail.length - help.length);
  return [...acts.slice(0, room), ...tail, ...help];
}
