// The looks view's confirmations and the add row's model (21 §6, 16 §2.5 Looks view). Until
// the player ticks "do not show this again", every apply first shows what it would write (a
// dry run) and waits for a yes; the answer is kept in ui.gui.looks.confirmed. The first add of
// a style or random variant has the same gate with a setting of its own. The pieces come in
// as functions (as editorlaunch.js), and the lines are built without the page, so the node
// suite checks the order and the wording's cases.
//
// applyGate({ confirmed, preview, ask, remember, fail }) returns gate(run):
//   confirmed()        whether the confirmation is off
//   preview()          the dry run (a promise of view.LooksDone or view.LookStyleDone)
//   ask(done, go)      shows the dialog; calls go(ticked) on yes, nothing on cancel
//   remember()         stores "do not show this again" (a promise)
//   fail(error)        a preview that could not be read: nothing is asked or written
export const CONFIRMED_KEY = "ui.gui.looks.confirmed";
export const STYLES_CONFIRMED_KEY = "ui.gui.styles.confirmed";

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

// filesText: the files a dry run would write, as links or copies; none is said when it
// writes no file. w has the page's T, Tf and num.
export function filesText(done, { Tf, num }, none) {
  if (!done.files) return none;
  if (done.copied && done.linked) return Tf("%s linked, %s copied (%s)", num(done.linked), num(done.copied), done.size);
  return done.copied ? Tf("%s, copied (%s)", num(done.files), done.size) : Tf("%s, linked (no copies)", num(done.files));
}

// applyLines: the lines of the first apply's confirmation. facts are [name, value] rows (the
// look, the Mod, what, for which lists); done is the dry run (view.LooksDone).
export function applyLines(facts, done, w) {
  const { T } = w;
  const lines = [...facts.map(([k, v]) => k + ": " + v), T("folder") + ": " + done.dir,
    T("files") + ": " + filesText(done, w, T("none: the load order already gives these textures")), "",
    T("RimArcho keeps a local Mod, RimArcho Looks Preference, last in the list. It holds links to the textures you chose and no Defs."),
    T("Your Mods and the load order are not changed, and saves do not depend on it. Undo removes a choice again.")];
  if (done.added) lines.push(T("This list gains RimArcho Looks Preference as its last Mod: one step of the list's undo."));
  if (done.dirChosen) lines.push(T("The folder is created now."));
  if (done.mixed) lines.push(T("Mixed result: some textures of the look still come from another Mod."));
  if (done.buildError) lines.push(done.buildError);
  return lines;
}

// addLines: the lines of the first add's confirmation, for a style (kind "style") or a random
// variant; done is the dry run (view.LookStyleDone), whose note is Go's sentence for the
// chosen target (a category that already has a style for the thing says so there); a random
// variant's note is left out, since the lines above already say it.
export function addLines(facts, done, kind, w) {
  const { T } = w, style = kind !== "variant";
  const lines = [...facts.map(([k, v]) => k + ": " + v), T("folder") + ": " + done.dir,
    T("files") + ": " + filesText(done, w, T("none")), "",
    T("RimArcho keeps a local Mod, RimArcho Styles Preference, near the end of the list. It holds links to the textures and a small Def file."),
    style ? T("The Def is a new style in an existing style category. It is skipped while that category's Mod is not in the list.")
      : T("The Def lets the game pick one of the textures for each item. Items you already have may change look."),
    style ? T("Items that got the style go back to their default look if RimArcho Styles Preference is removed.")
      : T("Saves do not depend on a random variant.")];
  if (style && done.note) lines.push(done.note);
  if (done.added) lines.push(T("This list gains RimArcho Styles Preference near its end: one step of the list's undo."));
  if (done.dirChosen) lines.push(T("The folder is created now."));
  if (done.buildError) lines.push(done.buildError);
  return lines;
}

// addPick: what the operation card's add row shows for a kind ("style" or "variant") and a
// wanted target, from view.LookAdd: the list of that kind, the chosen target (the wanted one,
// else the first that can be added, else the first), the note under the row and whether add
// is enabled. Nothing is decided here: ok, why, note and the "none" sentences are Go's.
export function addPick(add, kind, want) {
  const style = kind !== "variant";
  const list = (add && (style ? add.styles : add.variants)) || [];
  const none = (add && (style ? add.stylesNone : add.variantsNone)) || "";
  const choice = list.find((c) => c.target === want) || list.find((c) => c.ok) || list[0] || null;
  const note = !choice ? none : choice.ok ? choice.note || "" : choice.why || none;
  return { style, list, choice, note, can: !!choice && !!choice.ok };
}
