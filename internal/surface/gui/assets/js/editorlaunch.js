// The editor's launch (its button, L and the command panel's launch, 16 §2.5): it starts the
// game from the editor, so a player testing a list again and again stays on it (issue #6).
// A running game is refused first, as Launch's ▶ is disabled then: the VM round of
// 2026-11-22 made another list current and asked about RAM the running game itself was
// using. Ask about unsaved changes before making the open list current. Then the same checks as Launch's ▶: the RAM
// estimate's severe warning asks once (09 §2), run asks about missing required Mods and gives
// a refusal (outside changes not loaded) in the status line.
//
// d: { S, T, say, unsavedConfirm(count, go), usePlan(id), homeMemory(), memoryConfirm(m, go),
// run(id), busy() } — busy is the window's own reason to refuse, or "".
export function editorLauncher(d) {
  let launching = false;
  async function start(approved) {
    if (approved) {
      let m = null;
      try { m = await d.homeMemory(); } catch { /* unknown */ }
      if (!m || m.token !== approved) return d.say("busy", d.T("the list changed; review the updated estimate before launching"));
    }
    d.say("busy", d.T("starting…"));
    await d.run("launch");
  }
  return async function launch(confirmed = false) {
    const busy = d.busy();
    if (busy) return d.say("busy", busy);
    const live = d.S.live;
    if (live && (live.running || live.blocked)) return d.say("fail", live.blocked || d.T("the game is running: close it first"));
    if (launching) return;
    const unsaved = d.S.board?.unsaved;
    if (!confirmed && unsaved > 0) {
      d.unsavedConfirm(unsaved, () => launch(true));
      return;
    }
    launching = true;
    try {
      const open = d.S.shell && d.S.shell.plan;
      if (open && !open.current && !(await d.usePlan(open.id))) return;
      let m = null;
      try { m = await d.homeMemory(); } catch { /* unavailable means unknown, as on Launch */ }
      if (m?.assessment?.ramSeverity === "severe") {
        d.memoryConfirm(m, () => start(m.token));
        return;
      }
      await start(null);
    } finally { launching = false; }
  };
}
