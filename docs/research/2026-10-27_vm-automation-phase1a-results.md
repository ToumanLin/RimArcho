# VM automation Phase 1a measurements (2026-10-27)

This report records implementation and measurements on the Windows host; the [Phase 1a runbook](../../scripts/vm/phase1a.md) owns run steps, and acceptance follows [plan §§4–7, §11](2026-10-27_vm-automation-test-plan.md). Completing the runner does not mean all application acceptance checks pass: the staging-undo oracle retains its failure, and exceeding performance budgets independently sets exit code 1.

## Deliverables and isolation

Branch `codex/vm-phase1a`, worktree `RimArcho/.claude/worktrees/vm-phase1a`, base `c4adf74`. Code and commits stayed in that worktree; builds, DBs, screenshots, traces, and browser profiles all stayed outside the repository.

- CLI adds `loadout complete-dependencies`, `loadout replace-recommended <ref…>`, and `loadout use-installed <packageId…>`, directly calling shared GUI app methods; batches revalidate first, with one undo step per invocation. `sort [--dry-run] --json` and text output use shared `app.SortNotices`. Command tests cover rejected batches without partial changes, single-step undo, consistent JSON/GUI sources, and write-free dry runs. Also ran each built CLI operation on a disk fixture, undid it, and independently read workspace/undo depth; all three passed (private evidence `D:/vm-phase1a-cli-repairs/cli-repairs.json`).
- CDP supports Space, left click, Shift case, held-key repeat, real double/triple clicks, and timeout/disconnection rejection. Input traces retain actual delivery times, selections, focus, edit queues, and events. 20/50/100ms are scheduling targets; CDP round trips and rendering may lengthen actual intervals, so trusted input traces are authoritative.
- Final fixture has 36 instances (two Core, 33 extra, one Workshop), two game versions, and three plans, including dependencies, unknown/remote missing dependencies, installed copies, and recommended replacements. Selecting 100 is explicitly marked unsupported in the small library; the real library runs 1/10/100.
- Canonical snapshots use independent read-only SQLite, normalizing sources/paths/package IDs/content fingerprints and problem-association keys. They retain every plan's order, enabled set, staging positions and anchors, full undo/redo, saved versions, tags/notes/rules, configuration hash/XML, and links. W enabled order is additionally checked against independent DB observation.
- Settled requires complete page content and navigation, no loading/error/env busy, an empty edit queue, and consecutive stable states. Continuous sampling checks blank pages, false no-game states, lost sidebar, errors, decreasing problem sequence, and results overwritten by loading. After fresh publication, verify Needs/ListProblems/Board context, counts, and per-Mod associations.
- Subscription double/triple clicks submit for real to a loopback TLS failure endpoint owned by this process, asserting one explicitly failed task per group and unchanged plans/instances. Successful-download deduplication is outside this evidence's conclusions. Unsubscribe is only cancelled on a real Workshop row. Native overwrite export uses a PID-scoped Windows-control helper to choose No, then Cancel, verifying sentinel bytes in the existing file.
- Discovery records 112 final-page candidates (43 hittable; evidence `D:/vm-phase1a-results/s3-W-lBIWzo/controls.json`), saving only semantic keys, bounds, and hittability for final-page controls; it is not a complete state-graph crawler. VM hooks are documented; this round neither started/restored a VM nor launched RimWorld.

Real-data provisioning uses `sqlite3 -readonly … ".backup …"`, copies games, shared libraries, userdata, and 23 external junction targets, then relocates typed path columns in the copied DB. Resolved paths are audited before App.Open and again after scanning. Nested SteamCMD sources must be retained; otherwise discovery registers the original nested directory as a new source. Already discovered nested roots skip folder set, which rejects overlap. This implementation deviation from the plan is verified by pre-migration audit, unchanged scan, and post-migration audit; no database-wide string replacement was used.

Private baseline `D:/vm-phase1a-real-baseline`; environment scan: 4112 live, 55 tombstones, 756539 files, about 42.6GB; complete DB: 7878 instance records, 5 plans; default test list: 481 entries. Environment switching owns another complete copy, `D:/vm-phase1a-results/s2-W-YpnfF9/case`. Final scans of baseline and exclusive copy both reported changes: none. The final preparer also ran backup→copy→typed relocation→pre-open audit→env validate→scan→audit end to end from a small fixture, output `D:/vm-phase1a-preparer-check`, passed.

The original APPDATA home was read-only; test RIMARCHO_HOME, APPDATA, and LOCALAPPDATA were all isolated. Existing port listeners were never taken over. No private Mod content or data tables appear in the report.

## Application findings

### F1: list switching loses the last click (fixed)

Repro: while A is shown, click B, then click A before OpenPlan finishes. Old logic considered A already displayed, skipped the later request, and ended on B. The fixture W2 trusted pointer trace confirms two real clicks; executing original navigation source with a delayed mock also reproduces it. `06fdadb` processes explicit OpenPlan requests in order and checks read context after completion; two Node regressions cover returning to the original list and failure of the earlier request. W2's twenty alternating plan clicks passed.

### F2: staging undo leaves ghost staging (not fixed, P0-2)

W/A/C and a real DB copy fail the same independent oracle: stage an enabled entry, then undo; workspace entries recover, but `stage:<plan>` item/position/anchor do not. Stage in `internal/app/stage.go` writes meta separately, Staged merely filters enabled entries, and UndoRedo in `internal/app/loadout.go` restores entries only. This is the existing P0-2 history-payload boundary (the current specification excludes the staging list itself from undo), not a regression introduced by this branch. The ghost also shows that "leave staging after re-enabling" does not converge persistently. The failure is retained; the oracle was not weakened to compare visible lists only.

Visible consequence confirmed on a fresh fixture: after stage→undo, staging shows 0; ordinary disable of the same entry makes it reappear in staging with its old position/anchor. Independent repro commands (use a new directory outside the repository):

```powershell
D:/vm-phase1a-tools/vmstate.exe --mode fixture --root D:/phase1a-stage-new
$env:RIMARCHO_HOME='D:/phase1a-stage-new/home'
$env:APPDATA='D:/phase1a-stage-new/appdata'
$env:LOCALAPPDATA='D:/phase1a-stage-new/localappdata'
D:/vm-phase1a-tools/archo.exe stage add pkg:vm.dependent
D:/vm-phase1a-tools/archo.exe undo
D:/vm-phase1a-tools/archo.exe stage list
D:/vm-phase1a-tools/archo.exe disable pkg:vm.dependent
D:/vm-phase1a-tools/archo.exe stage list
```

### F3: rapid Space input triggers a layout-observer loop (fixed)

The first final-fixture round `D:/vm-phase1a-final/s1-W-yiUNPp` recorded two `ResizeObserver loop completed with undelivered notifications` errors during a 1-entry/100ms Space burst (8649ms and 8669ms after observer start). List-state and undo assertions still passed, but the no-error oracle correctly failed; browser errors were not suppressed. Synchronous renderRows in the list resize callback changes observed scroll layout; `eb4fb78` coalesces it into the next animation frame. `752f18a` executes actual lists source, verifying no layout writes during resize delivery and coalescing of same-column notifications. Repeated real-window results after the fix are below.

### F4: real-scale performance exceeds budget (not fixed)

Cold startup includes Esc on the startup notice, opening the default 481-entry list, and waiting for stability; repeated rounds took about 5.17–6.28s, exceeding the 5s threshold for a thousand-Mod library. Real W1 `s1-W-gCWSim` also recorded sampling pauses of 624ms and 980ms, exceeding 500ms; the responsible application rendering/host scheduling segment is not yet identified, so no proven root cause is claimed. One environment/library-page switch in `s2-W-hgMW6z` exceeded 15s with env busy still present at timeout; rerun `s2-W-h3impU` switched in 3.09s and 3.72s and passed functionality. The initial timeout evidence is retained; thresholds were not raised.

## Distinguishing driver issues from application issues

Early real-library `s4-W-nwRrWf` could not click index 1 after move→undo: the target row was outside the virtual-list viewport, a driver positioning error. `712ccd2` uses real number-key focus, Home, and ArrowDown; rerunning completed and exposed F2. Early settled checks missing the edit queue, duplicate nested-source registration during provisioning, and historical-key collisions from path casing were also corrected. Final end-to-end backup/migration checks and unchanged scans passed. These failures were not counted as application defects, nor were unreached branches counted as passing.

## Acceptance records and budgets

Host AMD Ryzen 9 7950X (32 logical cores), about 63.1GiB RAM, D: Nextorage SSD NE1N2TB NVMe; GPU list includes RTX 4090 (driver 32.0.16.1088), AMD integrated graphics, and USB virtual display; actual WebView GPU was not pinned. Node v22.17.0. Each result.json records source commit, workspace state, SHA-256 for three/four tools, and content scale; runs used the local logged-in desktop and real Wails, not a guidev imitation window.

Full `go test ./...` passed (GUI package 11.55s after final rebuild), as did 6 Node protocol/navigation/layout regressions. Original protocol mocks verify driver boundaries only; W evidence below comes from real windows.


All five post-fix fixture W1 rounds passed functional oracles, with no further ResizeObserver errors observed. Four passed overall; one failed only cold-ready at 2.099s against 2s. Private evidence root: `D:/vm-phase1a-resize-verified`; round directories: `s1-W-auGKla`, `s1-W-UdiQs2`, `s1-W-ackVQg`, `s1-W-y9ZCXO`, `s1-W-BSgM2o`. The small library explicitly marks selecting 100 unsupported; that is not coverage.

Final C/A fixture matrix (`D:/vm-phase1a-final`):

| Scenario | C | A | W (matrix before layout fix; post-fix W1 results above) |
| --- | --- | --- | --- |
| 1 | `s1-C-F61GgV` pass | `s1-A-gLzvzo` pass | `s1-W-yiUNPp` found F3; data/undo assertions passed |
| 2 | `s2-C-SfQ87x` pass | `s2-A-UrtbDV` pass | `s2-W-ans1CC` pass |
| 3 | `s3-C-sElKYy` pass | `s3-A-Pbc3o0` pass | `s3-W-NvoI3T` pass, including deduplication of failed subscription submissions |
| 4 | `s4-C-RaAPxY` F2 | `s4-A-Aqhnyq` F2 | `s4-W-qb5M2e` F2; other cancellation/undo/configuration checks passed |

C1 covers sort history; A1 also toggles. C2 has read-only context projection only; A2 also tests concurrent Workspace request/event behavior. Neither claims W rapid-key/navigation coverage. The six common scenario-3 checkpoints match across C/A/W, including canonical problem counts/per-Mod severity. Scenario 4's nine C/A checkpoints also match, yet both fail F2, showing differential agreement alone cannot establish correctness. Comparison evidence in OS temp: `RimArcho-vm-compare-Uo9K8o`, `RimArcho-vm-compare-medlSa`, `RimArcho-vm-compare-ELjcpc`.

Post-fix real-library results (`D:/vm-phase1a-resize-verified`):

| Scenario | Result/finding | Private directory |
| --- | --- | --- |
| 1 | Functionality passed; actual 1/10/100 selections and all cadences completed; cold 5.174s; 4 sampling gaps 516–682ms, performance failed | `s1-W-LLCY7B` |
| 2 | Functionality passed; switch environment and return, last plan/page wins; cold 5.495s over budget | `s2-W-ZNL6Zi` |
| 3 | Functionality passed; repeated sort, double/triple enable/import/subscribe submissions; sort 1.141s; cold 5.300s over budget | `s3-W-2LAygR` |
| 4 | Only F2 failed functionality; delete/unsubscribe/directory-move/native-overwrite cancellation passed; cold 5.244s over budget | `s4-W-Moa3vX` |

| Metric | Measured | Budget | Verdict |
| --- | --- | --- | --- |
| cold-ready (small, five rounds) | 1.450–2.099s | 2s | One 2.099s round failed; others passed |
| cold-ready (real, four scenarios) | 5.174–5.495s | 5s | Four rounds failed |
| W1 stable after burst (small, five rounds) | 0.379–0.877s | 10s | Passed |
| W1 stable after burst (real) | 2.214–5.882s | 10s | Passed |
| W2 final page stable after navigation burst | 0.525–2.427s | 5s / hard 15s | Passed; not a single-page warm-read measurement |
| W2 two environment switches ready | 2.509–2.955s | 5s / hard 15s | Passed; earlier 15s failure retained |
| W3/W4 sort→stable (real) | 1.141–2.933s | 5s | Passed; already-sorted real lists verify idempotence; common actual-sort undo/redo differentials use an unsorted fixture |
| W4 delete-plan confirmation ready | 143.875–164.908ms | 200ms | Passed |
| W4 dialog close (cancel/Esc/backdrop) | 45.094–64.289ms | 200ms | Passed; native-helper time is separate from this group |
| Fresh problem publication wait (real) | 0.733–8.651s | 120s | Passed |
| ListProblems hot read (real, 5 per scenario) | 22.716–97.892ms | 100ms | All 20 reads passed; mixed-context samples are not called a performance P95 |




Final raw evidence includes domain/surface at each checkpoint, result.json, window-trace.json, fresh-problems.json, process logs, and screenshots; screenshots/DBs/profiles stay only in the private external directories above. Real W1 completed 234 checks, W2 30, W3 154; W4's only failure was F2. Performance rounds had refnums off; control discovery ran separately. No blank pages, false no-game states, lost sidebar, console/script errors, or successful results overwritten by background loading were observed in the four final W runs.

## Limits

These are neither P95 nor VM performance conclusions. Raw samples from every round are retained; cold-start and different-action samples cannot be mixed into percentiles. First DOM mutation is only a feedback proxy; arbitrary no-ops/background updates may be wrongly associated with the preceding input, so 200ms is enforced only for controlled navigation inputs, with diagnostics retained elsewhere. Delete-plan opening and dialog closing have explicit endpoints timed separately. The native helper operates native controls; CDP trusted-input conclusions cover only WebView input.

A/C perform only operations with domain equivalents and cannot replace real input, menus, or confirmation cancellation. Snapshots/comparisons cover lists and configuration in this phase; full file manifests, successful downloads/credit/file recovery after destruction, full crawler, and game loading remain later scenarios. Equal source/instance/link/configuration state in current cancellation checks does not imply equal per-file content hashes. Original real data will not be committed or uploaded.

## Rerunning

Use the designated worktree; builds and full arguments are in the [runbook](../../scripts/vm/phase1a.md). With local tools and baseline already available:

```powershell
Set-Location D:/Game/RimModManager_Upgrade/RimArcho/.claude/worktrees/vm-phase1a
$tools='D:/vm-phase1a-tools'
$out='D:/vm-phase1a-rerun'
foreach ($driver in 'C','A','W') {
  foreach ($scenario in 1..4) {
    node scripts/vm/run.mjs --scenario $scenario --driver $driver --tools $tools --out $out
  }
}
foreach ($scenario in 1,3,4) {
  node scripts/vm/run.mjs --scenario $scenario --driver W --tools $tools --out $out --baseline D:/vm-phase1a-real-baseline
}
node scripts/vm/run.mjs --scenario 2 --driver W --tools $tools --out $out --baseline D:/vm-phase1a-results/s2-W-YpnfF9/case --owned-files true
node --test scripts/vm/cdp.test.mjs scripts/vm/navigation.test.mjs scripts/vm/lists.test.mjs
go test ./...
```

`--owned-files true` is only for the audited exclusive, disposable full copy above, never the real home or concurrent consumers. Without it W2 copies all content automatically. `--port 9222` or `RIMARCHO_DEVTOOLS_PORT=9222` fixes the real-window port; the default chooses a free loopback port. Exit 1 includes functional or performance failure: read both failure arrays in result.json first, and do not record intentionally exposed F2 as passing.

## Commits

| Commit | Content |
| --- | --- |
| `990b8c5` | P0-1 CLI, shared sort notices, atomic undo and rejection tests |
| `06fdadb` | Fix lost final plan click |
| `8b39afe` | Preserve shared-notice empty-field omission contract |
| `f6dbde2` | Fixture, canonical oracle, C/A/W runner, copy audits, and VM docs |
| `f6f7d3d` | Preserve nested sources, valid multiselection, real double/triple clicks |
| `a9f1f93` | Diagnostic waiting cannot erase timeout failure |
| `712ccd2` | Real keyboard positioning for offscreen virtual-list rows |
| `f626743` | Normalize instance keys in problem-association maps |
| `8e5dcca` | Remove stale gap descriptions from CLI comparison table |
| `3878682` | Duplicate subscription submission, explicit unsubscribe/native-overwrite cancellation, action timing, and problem differentials |
| `eb4fb78` | Fix synchronous ResizeObserver redraw loop |
| `752f18a` | Resize-delivery regression using actual lists source and run docs |
| `f300951` | Nonempty editor columns must contain real DOM rows; empty containers cannot pose as content |

