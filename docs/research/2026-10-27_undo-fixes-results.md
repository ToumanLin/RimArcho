# Undo fixes — 2026-10-27

Branch: `codex/undo-fixes`, based on `dev/0.9.0` at `a4580fc`.
Bug A is crawler F3; bug B is crawler F4, unrelated to cold-start work.

## Bug A: identical saved-version checkout

`Checkout` called the ordinary editor, which correctly skipped an unchanged list,
then unconditionally persisted checkout origin metadata and cleared a matching
conflict. No undo step accompanied that second write. GUI/TUI feedback nevertheless
promised undo.

The specifications did not define identical-list checkout, so this case now follows
the requested no-op behavior. Equality is checked after resolving the saved list
for the current game. The stored workspace, origin, conflict, staging and both
history stacks stay unchanged. CLI/GUI/TUI report no changes; GUI receives an
explicit undo flag, and version-row tooltips no longer promise undo unconditionally.
The clarified contract is in spec 04 §3.2 and the editor-specific feedback in 06 §2.5.

## Bug B: cross-game sort and undo

`LoadoutState.List` resolves stored Core/DLC/local-mod entries for the current game.
The editor recorded that resolved list as the undo snapshot, replacing the original
Core binding before undo ever ran. Reverse snapshots and picker batches had the
same distinction between stored and displayed entries.

Snapshots now use `Workspace.Entries`, including the reverse state recorded during
undo/redo and the initial state of a batch. Restoring writes the stored payload
exactly; returned lists and history differences resolve it for the current game.
The existing transactional entries/staging history implementation remains in use.
Spec 04 §2.1 states the binding-preservation rule.

Sorting uses the active game's Core in its edited output; undo must restore the
previously stored binding. The reported drift normally has no visible effect on
the Core row because reads already resolve Core for the active game. Direct C/A
verification also confirmed neither fixture ModsConfig.xml changed. This is not
a claim about game launch behavior; no game was launched.

## Regression evidence

Before the fixes, `TestIdenticalCheckoutIsNoOp` failed on changed origin/conflict
metadata, and `TestCrossGameSortUndoPreservesBindings` failed on the stored undo
snapshot, restoration and cross-game redo result. Both passed after the fixes.

Coverage includes identical older versions with five origin kinds and existing
redo history; exact stored Core restoration; localized undo/redo results and
history differences; and batch commit/undo and cancel. The TUI adds
`TestHistoryArea/identical_checkout_is_a_no-op`; the GUI covers the identical checkout at
the start of `TestHistoryCheckoutUndo`, which also asserts that a real edit still
advertises undo. Existing staging regression tests
passed. Both edited JavaScript files passed `node --check`.

`go test ./...` passed for the complete repository (exit 0), with `GOFLAGS=-p=1`
to bound concurrent build storage and `TMP`/`TEMP` under the authorized undo
directory. Full output is `D:/RimArcho-test/undo/go-test.log`.

The frozen runner was attempted with scenarios 3 and 4 for drivers C and A:

```powershell
node scripts/vm/run.mjs --scenario 3 --driver C --tools D:/RimArcho-test/undo/tools --out D:/RimArcho-test/undo/runs
```

All four invocations stopped before fixture creation at the existing global
`hostOutput` guard: `host test output exceeds 1800 MiB; free space before another run`.
No runner code, snapshot exclusions or repro recipes were changed. The two JSON
recipes are W-only; they still express the expected canonical restoration/no-op
outcome and need no expectation change.

Separate fresh generated fixtures passed the equivalent CLI and loopback API
operations: identical checkout, and foreign-list sort/undo/redo/undo. SQLite
observation checked workspace metadata, history and exact entries/Core binding;
both ModsConfig.xml hashes stayed unchanged. The external verification script and
summary are `D:/RimArcho-test/undo/verify-direct.py` and
`D:/RimArcho-test/undo/direct-verification.json`. Per-run fixture cases and built
verification binaries were removed. No W runs, VM actions, Steam/SteamCMD or game
launches were performed; other host test directories were left untouched.

The subscription managed-directory refusal now says to locate the directory in
settings or reconnect its drive, replacing the internal name `SetDirectory`.

## GUI checks remaining

- Replay the unchanged F3/F4 recipes in the user's chosen window/VM environment.
- Check identical-version checkout says “no changes” without an undo link, while
  a different-version checkout still offers undo and restores the prior list.
- Sort another game's list, undo and redo; also switch games between these actions.
  Core should remain the active game's displayed copy with no spurious history edit.
- Check the unavailable subscription-directory preview points to settings.
