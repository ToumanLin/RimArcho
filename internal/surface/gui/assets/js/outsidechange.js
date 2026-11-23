// Another process (the TUI, the CLI, an agent through MCP) may have changed the data: the
// window looks again when it gets the focus (G8). The board and the sidebar are read
// again; when that moved the list the window has open (an outside `plan use` while the open
// list was the current one, 07 §1.3), the visible page is drawn again as well, as after a
// Game Config switch. Without that, Launch and Can it run kept the list from before until
// a later background event (VM round 2026-11-23: Can it run one switch behind).

// openListKey: the Game Config and the list the window has open, and whether it is current.
export function openListKey(S) {
  const p = S.shell && S.shell.plan;
  return [S.board && S.board.env, p && p.id, !!(p && p.current)].join("|");
}

// lookAgain reads again when the data's stamp changed; redraw draws the visible page again
// when the open list changed. It answers whether anything was read.
export async function lookAgain(S, stamp, reload, redraw) {
  if (!S.board || (await stamp()) === S.board.stamp) return false;
  const was = openListKey(S);
  if (!(await reload())) return false;
  // the guide is not redrawn (that would restart it); the editor is the board itself
  if (S.screen && S.screen !== "guide" && openListKey(S) !== was) await redraw();
  return true;
}
