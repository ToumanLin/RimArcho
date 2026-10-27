# VM crawler and player results — 2026-10-27

Work is on `codex/vm-crawler`, based on `1eb3b05`. Tooling was frozen at
`3892b3b` when the user requested wrap-up; no further runs or retries were started.
Evidence: `D:/VMs/Transfer/vm-crawler-final` (compact JSON, ledgers, traces, recipes
and small logs). No screenshots, databases, profiles or real content are committed.

## App findings

| Severity | Finding / recipe | Branch status |
| --- | --- | --- |
| Medium | F1: log virtual rows can cause a ResizeObserver loop; [manual recipe](../../scripts/vm/repros/F1-log-resize.md). | Fixed: `3a7f505`. No minimized F1 trace claimed. |
| High | F2: stage an enabled item, then undo; staging metadata remains; [recipe](../../scripts/vm/repros/F2-staging.json). | Open, known stage-undo work; that worktree was untouched. |
| Medium | F3: identical saved-version checkout advertises “u undoes” but changes origin metadata without an undo entry; [recipe](../../scripts/vm/repros/F3-checkout-origin.json). | Open; requires undo-state design work. |
| Low | F4: sorting another game's list and undoing changes its stored Core binding; [recipe](../../scripts/vm/repros/F4-cross-game-undo.json). | Open canonical-restoration discrepancy; visible impact unconfirmed. |
| Medium | F5: opening an empty Tasks page throws on null rows and leaves it busy; [recipe](../../scripts/vm/repros/F5-empty-tasks.json). | Fixed: `66dac75`; real-window functional replay passed. |

F2 shrank from 32 to 8 inputs, F3 from 29 to 7, F4 from 34 to 26, with independent
fixture reproduction. F5 needs one click. F3/F4 have not been hidden by snapshot exclusions.

The guest's last-plan deletion refusal is clear: “default is the game's only list;
it cannot be deleted.” The runner creates a private companion plan before testing
confirmation cancellation. Subscribe was disabled because the prepared subscribed
source directory and its config disagreed, **not because Git was missing**. Aligning
the private DB config enabled its dry-run preview without Git. Preparation is fixed
in `74aa693`. The visible message uses the internal name “SetDirectory”; actual
subscription/download execution remains untested because external actions are off.

## Coverage and runs

Asserted means a successful control/oracle/return check. Discovered includes hidden,
disabled, skipped and repeated state-specific controls; these are not percentages
of all possible application behavior.

| Crawl | Asserted / discovered | States | Player time | Skipped local / external / unknown |
| --- | ---: | ---: | ---: | ---: |
| Fixture, local enabled | 302 / 2215 | 60 | 30.05 min | 0 / 242 / 271 |
| VM real content, local enabled | 236 / 1563 | 47 | 20.87 min | 0 / 59 / 475 |

Per-page counts, percentages and reasons are in `fixtures/local-crawl/ledger.md`
and `real/crawl/ledger.md` in the evidence folder. The safe fixture crawl asserted
258/2238 without functional failures; the longer fixture crawl asserted 344/3010
before the F4 restoration failure. The ledger comparison recorded 512 new and 537
removed discovered controls; different bounded discovery scopes contribute to this diff.

| Seed | Fixture steps | VM steps | VM time | Selected-run outcome |
| --- | ---: | ---: | ---: | --- |
| 1 | 200 | 171 | 297.2 s | F2; VM branch also had an unreachable sidebar-toggle replay. |
| 2 | 200 | 200 | 225.8 s | F2 only. |
| 3 | 200 | 102 | 321.8 s | F2 in isolated branch and at end. |
| 4 | 200 | 62 | 202.0 s | F2 in isolated branch and at end. |
| 5 | 200 | 25 | 104.9 s | F2 in isolated branch and at end. |
| 7 | 200, local enabled | — | — | F2 only; 651.3 s fixture run. |
| 42 | 200 | — | — | F2 only; additional fixture sweep. |

These are the selected latest completed sweeps, not every development attempt.
All selected 1–5 host runs and the VM batch have zero performance failures. F5's
functional replay separately recorded a 2.14 s cold start against its 2 s budget.
`go test ./...` passed; the last Node suite passed 20/20 before the freeze.

## Rerun later

From this worktree, in host PowerShell 7, with Player signed into the test VM:

```powershell
pwsh scripts/vm/player-vm.ps1 -Mode crawl -Minutes 30 -Allow local -GuestBaseline 'C:\Users\Public\Documents\VM-Test\real-baseline'
pwsh scripts/vm/player-vm.ps1 -Mode walk -Seed 1 -SeedCount 5 -Steps 200 -Minutes 20 -Allow local -GuestBaseline 'C:\Users\Public\Documents\VM-Test\real-baseline'
```

These commands reuse baseline content and take private DB backups; external actions
remain off. For the fixture recipes use `node scripts/vm/run.mjs --mode walk
--driver W --tools D:/RimArcho-test/tools-fixed --out D:/RimArcho-test/repro
--replay scripts/vm/repros/F3-checkout-origin.json --branch true --shrink false`.

## Stopping state and unfinished work

No snapshots were created, changed, deleted or reverted. Seed 5 finished after its
batch supervisor was stopped; no next seed or new crawl was launched. All folders
created by this task in guest VM-Test were deleted after evidence retrieval;
`real-baseline` and pre-existing Phase 1a/RimArcho/pwsh7 folders remain. Guest test
Node count is zero. Guest C: free after cleanup: **23,732,092,928 bytes**. The
existing baseline measured **22,580,416,214 bytes** and was left unchanged.

At the final inventory (2026-10-27 23:31 UTC), `D:/RimArcho-test` measured
**1,396,928,233 bytes**, below 2 GB, including a separate `f4` directory measured
at **1,009,955,753 bytes** that was left untouched. The final evidence measured
**69,761,904 bytes**, below 1 GB, before this small report correction.
Old `D:/vm-*` and `D:/stage-undo-*` folders were not deleted.

Coverage is incomplete. The real crawler predates the final driver fixes and its
later failures cascade after F3; they are not separate app bugs. The fixture local
crawl also includes the now-fixed empty-Tasks error and minute-budget accounting
error. Replay-count, unreachable-branch, known-undo continuation and budget fixes
are committed but were not rerun in the VM after the user froze tooling. F2/F3/F4
remain open; a fresh full crawl and five complete real-content 200-step walks remain
unfinished. No real-content host copy or external-action run is pending.
