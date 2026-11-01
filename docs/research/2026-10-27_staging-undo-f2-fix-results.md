# Staging undo F2: fix and host measurements

2026-10-27. Based on `3d19db0` from `dev/0.9.0`, workspace `.claude/worktrees/stage-undo`, branch `codex/stage-undo`. These are Windows host measurements, not VM measurements. Runs follow the [Phase 1a runbook](../../scripts/vm/phase1a.md).

## Root cause and fix

Previously, `App.Stage` / `Restore` edited entries first, then separately wrote `stage:<plan>`; `Unstage` wrote only meta, while the undo stack saved only entries. `Staged()` filtered enabled entries from its return value but left them in the DB. Thus `stage add → undo → disable → stage list` made old staged entries reappear.

`workspace_undo.staging` saves the complete pre-edit staging JSON for each step, including entries, order, positions, and anchors. SQL NULL means old history lacks a snapshot; an empty string means no staging meta; JSON `null` and `[]` retain their respective forms. Edits and undo/redo commit entries, staging, and both history stacks in one SQLite transaction. Metadata-write failure rolls everything back, and the cache does not publish the failed workspace.

All ordinary list edits record staging and persistently remove staged entries that are enabled again. Coverage includes add/drop/restore, GUI/API Toggle/Stage/Place, batch-selection Commit/Revert, history checkout, replace/merge imports, external ModsConfig loading, and Monitor edits. Ordinary disable has no path that automatically stages an entry. Placing from staging in the GUI now directly uses the full shared Enable transaction, without a separate subsequent Unstage. Credit-reference checks, copy handoff, and link identity merges also include historical staged entries and anchors.

Old history restores entries only, preserving unrelated staged entries and removing hidden staged entries enabled in current or target entries; the reverse step saves a complete snapshot. It does not infer positions or anchors absent from old history.

Specification updates: [04 §1.2, §2.1–2.2](../specs/04-plans-and-sorting.md) require entries and staging to enter the undo stack together, give stage drop and staging-only edits of disabled entries their own steps, and define old-history compatibility; [06 §2.5](../specs/06-ui/2.5-plans-and-packs.md) updates the GUI contract.

## Commits

| Commit | Content |
| --- | --- |
| `273e297` | Root-cause fix, schema migration, shared edit paths, specifications, and app/store/GUI API regression tests |
| `5baaa15` | Scenario 4 adds redo and exact ghost repro; CLI golden adds history steps for staging-only edits |
| `29d4ce4` | Unrelated test race exposed by full tests: fake HTTP request counter made atomic; 20 consecutive targeted rounds passed |
| `97bc79b` | Read-only observer supports old schemas without migrating the baseline for observation |
| `0d29395` | WAL transaction-upgrade issue found in a real window; acquire write lock before reading snapshots, plus two-connection contention test |

The final matrix used a clean workspace at `0d29395`; all four tools were rebuilt in `D:/stage-undo-tools`. Each result.json saves actual tool hashes.

## Scenario 4 results

All functional assertions passed in the final rounds, including complete entries + staging payloads for stage undo/redo, and no staged-entry reappearance after undo followed by ordinary disable. W input came from CDP on a real Wails window; real content was copied from `D:/vm-phase1a-real-baseline` into a new external test home using `sqlite3 -readonly .backup`. The real APPDATA home was not opened; no DB, screenshots, or profiles were committed.

| Content / driver | Functional assertions | Cold ready / budget | Scenario time | Result directory |
| --- | --- | --- | --- | --- |
| fixture C | 10/10 | Not applicable | 1.736s | `D:/stage-undo-results/s4-C-NA2SQb` |
| fixture A | 13/13 | 0.693s / 2s | 0.909s | `D:/stage-undo-results/s4-A-XyHS78` |
| fixture W | 283/283 | **2.295s / 2s, over budget** | 19.431s | `D:/stage-undo-results/s4-W-loRQP5` |
| Real copy W | 269/269 | **5.791s / 5s, over budget** | 84.025s | `D:/stage-undo-results/s4-W-QsaRm2` |

The 13 shared C/A checkpoints were identical, including full historical staging payloads. All independent W oracles passed; W also cancels confirmations, enables, and moves entries, so its entire history stack is not forced to match C/A.

The final W rounds had no console/script errors, blank pages, missing sidebar, or database-lock errors. On the real copy, delete confirmation became ready in 152–160ms (budget 200ms), cancel feedback took 55–70ms (budget 200ms), sort-to-settled 1.368s (budget 5s), final fresh problem-count wait 454ms (budget 120s), and warm ListProblems 30–34ms (budget 100ms). Because sorting was already correct, no coverage was claimed for sorting changes absent from that round.

Both final W results have `functionalStatus` passed but overall `status` failed; the sole remaining failure is cold ready exceeding budget. Real-library cold startup already exceeded 5s in Phase 1a. This round's 5.791s and the previous report's 5.17–5.50s are not a controlled performance comparison; one run cannot establish no regression. Fixture samples were 2.448s, 1.828s, and 2.295s, fluctuating across the 2s threshold. The budget was not raised, and failed rounds were retained. These are single-round observations; no P95 is reported.

## Findings and retained failure evidence

- Original F2 repro: `archo stage add <enabled-ref>` → `archo undo` → `archo disable <same-ref>` → `archo stage list`. Fixed; additional tests cover multiple staging batches, positions/anchors, drop/restore, staging-only edits, checkout/import, old history, batch selection, API placement, and failure rollback.
- This transaction change initially introduced WAL upgrade failure: with real-library background analysis running, staging via `2 Home Down t` showed `database is locked (5) (SQLITE_BUSY)` in the status line and did not commit. Evidence: `D:/stage-undo-results/s4-W-4VRju9`, three failed functional assertions. The deferred SQLite transaction read staging before writing; a background connection committed in between, preventing the read transaction from upgrading. `0d29395` reserves a write lock before reading and waits through the existing busy timeout; no retries, delayed input, or skipped assertions were used. The final real W round had no such error.
- The observer initially failed before application startup on old schemas without a staging column: `D:/stage-undo-results/s4-W-PhgyWO`. Fixed in `97bc79b`, with verification that observing an old DB leaves its bytes unchanged.
- Early fixture W: `D:/stage-undo-results/s4-W-LYC5GN` passed functionality but exceeded the cold-start budget at 2.448s; `D:/stage-undo-results/s4-W-XJnCXz` passed functionality and budget at 1.828s. Both predate the write-lock fix; final acceptance uses the rebuilt-tool rounds above.

## Rerunning

Run from this worktree root. Keep all tools, homes, and evidence outside the repository.

```powershell
New-Item -ItemType Directory -Force D:/stage-undo-tools | Out-Null
go build -o D:/stage-undo-tools/archo.exe ./cmd/archo
go build -ldflags '-H windowsgui -X github.com/ToumanLin/RimArcho/internal/buildinfo.GUI=1' -o D:/stage-undo-tools/RimArcho.exe ./cmd/archo
go build -o D:/stage-undo-tools/guidev.exe ./spikes/guidev
go build -o D:/stage-undo-tools/vmstate.exe ./spikes/vmstate
foreach ($driver in 'C','A','W') {
  node scripts/vm/run.mjs --scenario 4 --driver $driver --port 9337 --tools D:/stage-undo-tools --out D:/stage-undo-results
}
node scripts/vm/run.mjs --scenario 4 --driver W --port 9337 --tools D:/stage-undo-tools --out D:/stage-undo-results --baseline D:/vm-phase1a-real-baseline
node scripts/vm/compare.mjs D:/stage-undo-results/s4-C-NA2SQb D:/stage-undo-results/s4-A-XyHS78
go test ./...
node --test scripts/vm/cdp.test.mjs scripts/vm/navigation.test.mjs scripts/vm/lists.test.mjs
```

Final full `go test ./...` passed (app 123.598s, store 5.123s, GUI 24.071s); all 7 Node tests passed. Go output: `D:/stage-undo-results/go-test-final.txt`; Node output: `D:/stage-undo-results/node-test-final.txt`. The original navigation.js logic to skip clicking the current plan when no open is pending was unchanged.
