// The first apply's confirmation (21 §6, 16 §2.5 Looks view): until the player ticks "do not
// show this again", every apply first shows what it would write (a dry run) and waits for a
// yes. The answer is kept in ui.gui.looks.confirmed; an apply is undoable either way. The
// pieces come in as functions (as editorlaunch.js), so the node suite checks the order.
//
// applyGate({ confirmed, preview, ask, remember, fail }) returns gate(run):
//   confirmed()        whether the confirmation is off
//   preview()          the dry run (a promise of view.LooksDone)
//   ask(done, go)      shows the dialog; calls go(ticked) on yes, nothing on cancel
//   remember()         stores "do not show this again" (a promise)
//   fail(error)        a preview that could not be read: nothing is asked or written
export const CONFIRMED_KEY = "ui.gui.looks.confirmed";

export function applyGate({ confirmed, preview, ask, remember, fail }) {
  let asking = false;
  return async function gate(run) {
    if (confirmed()) return run();
    if (asking) return; // a second press while the dry run is on its way
    asking = true;
    let done;
    try { done = await preview(); } catch (e) { asking = false; return fail(e); }
    asking = false;
    ask(done, async (ticked) => {
      // the choice is written even when the setting cannot be kept: the dialog shows again
      if (ticked) { try { await remember(); } catch { /* asked again next time */ } }
      return run();
    });
  };
}
