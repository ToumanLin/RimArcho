# Running Phase 1a scenarios

Run commands from the repository root. Node 22+, Go, PowerShell 7 and `sqlite3` must be on PATH. W requires a signed-in Windows desktop and WebView2. C and A use the same generated library without a desktop.

## Build outside the checkout

```powershell
$tools = Join-Path $env:TEMP 'RimArcho-vm-tools'
New-Item -ItemType Directory -Force $tools | Out-Null
go build -o "$tools/archo.exe" ./cmd/archo
go build -ldflags '-H windowsgui -X github.com/ToumanLin/RimArcho/internal/buildinfo.GUI=1' -o "$tools/RimArcho.exe" ./cmd/archo
go build -o "$tools/guidev.exe" ./spikes/guidev
go build -o "$tools/vmstate.exe" ./spikes/vmstate
```

The runner records the checkout commit; rebuild the tools after app changes. Fixtures include two game versions, several dozen synthetic mods, installed/missing dependencies, an unresolved installed copy and a recommended replacement. They cannot launch RimWorld. `vmstate --mode fixture --root <new-directory>` also creates a persistent seed for inspection.

## Run the fixture

```powershell
node scripts/vm/run.mjs --scenario 1 --driver W --tools "$tools"
foreach ($driver in 'C','A','W') {
  foreach ($scenario in 1..4) {
    node scripts/vm/run.mjs --scenario $scenario --driver $driver --tools "$tools"
  }
}
```

Each invocation creates a new case and prints its external result directory, even on failure. Use `--out <external-directory>` to retain evidence, `--plan <name>` to select a list, and `--port <port>` to choose the W DevTools port (also accepts `RIMARCHO_DEVTOOLS_PORT`). An existing listener is rejected. The runner launches and terminates only its own application process tree. W scenarios send CDP keyboard, pointer, wheel and text input; evaluation reads geometry/state and installs observers.

Scenarios contain fast input deadlines, rather than waiting for each app operation. `window-trace.json` preserves trusted input arrival, selection/cursor, modifiers, repeat, focus, edit queue, page/plan context, DOM samples, feedback and backend events. Settled means no loading/error/environment switch, an empty editing queue, intact navigation, and repeated matching samples. API jobs await their matching `environment` event; an acknowledgement alone does not complete a step.

## Copy real content

Choose a new external root with enough free disk space for the game directories, libraries and user data. This reads the source DB with SQLite `.backup`, copies files without following junctions, copies external junction targets separately, relocates typed paths in the copied DB, and audits resolved paths **before** opening the application.

```powershell
$baseline = 'D:/RimArcho-private-baseline'
pwsh scripts/vm/prepare-real.ps1 -Root $baseline -Tools "$tools"
foreach ($scenario in 1..4) {
  node scripts/vm/run.mjs --scenario $scenario --driver W --tools "$tools" --baseline $baseline
}
```

The default source is the real APPDATA home, read only. `-SourceHome <home>` selects another source. `RIMARCHO_HOME`, APPDATA and LOCALAPPDATA are isolated for application processes. Do not pass a real home as a baseline: it must have the preparation manifest and pass the path audit.

Scenarios 1/3/4 take a fresh DB backup and cache copy; their list edits and confirmation previews share the private baseline's content. Scenario 2 can change deployment links during environment switching and therefore makes a complete additional private filesystem copy. Corpus preparation is measured separately from interactive readiness. An already prepared, disposable filesystem clone can be assigned to scenario 2 with `--owned-files true`; that clone is then writable test content and must have no other consumers. Never run another test against shared private content while a filesystem scenario owns it. If an initial copy is interrupted, `-ResumeCopy` resumes only before `relocation.sql` exists, with a matching `copy-source.json`; after relocation starts, use a new root.

## Read and compare evidence

`result.json` separates checks, functional failures, performance failures, timings and unsupported/external coverage. `functionalStatus` can pass while the overall `status` fails a budget. Checkpoint JSON contains canonical domain data and surface observations. The `observation` field has raw runtime handles for validating the visible enabled order; it is excluded from cross-run comparisons.

Canonical identity includes source/environment, relative path, package ID and content fingerprint. Snapshots retain all plans, entry order, enabled membership, workspace origin/conflicts, staging positions/anchors, complete ordered undo/redo payloads, saved versions, tags/notes/rules, instances, source state, problem counts, ModsConfig bytes/hash/XML and game links. SQLite observation is read only and does not call startup recovery. Cache/log/profile bytes are not equivalence criteria.

```powershell
node scripts/vm/compare.mjs <C-result-directory> <A-result-directory>
node scripts/vm/compare.mjs <A-result-directory> <W-result-directory>
```

Compare the same fixture, plan and scenario. The comparator reports the earliest difference at shared checkpoints and its JSON path. Scenario 3 has exact common sort/undo/redo checkpoints. A matching domain result does not imply matching window input semantics. An invocation with no common checkpoints returns unsupported, not success.

The budgets come from the [automation plan §6](../../docs/research/2026-10-27_VM自动化测试方案.md#6-通过判据与时间预算). Navigation mutation feedback uses the 200ms soft threshold; timer sampling gaps over 500ms fail separately. Burst settling has a 10s limit, individual page waits 15s and problem publication 120s. Cold local readiness is recorded against 2s/5s by content scale. First DOM mutation is a feedback proxy, not proof that the action finished. Observer/host overhead is included; these host measurements are not VM results. Repeat at least five independent runs for a timing baseline; retain all samples and do not report P95 from fewer than twenty.

## Coverage boundaries

- W: rapid Space/held/Shift input, navigation during reads, twenty alternating plan clicks, both game versions, repeated sorting, double/triple row activation and import apply, delete-plan/Mod confirmation cancellation, directory-move preview cancellation, import/menu/position/tag Esc, enable/move/stage/sort undo and fresh problem counts.
- A/C: matching sort history and dry-run semantics; A also tests queued workspace completion events and toggle. Both retain the independent stage/undo-payload oracle, which currently exposes a core defect.
- D2 suggested multi-selection placement has no exact CLI equivalent; the ledger marks it unsupported. C/A do not claim input or popup coverage.
- Subscription submission needs an online subscription fixture and remains blocked for Phase 1b. Export overwrite cancellation is a native Windows save dialog, outside CDP; it requires UIA/manual coverage. Neither is counted as passed.
- `--discover true` saves visible controls and their semantic keys, bounds, enabled/hit state. This is discovery on the final page, not an action crawler or a coverage denominator.
- Filesystem manifests for full destructive/download workflows and task/credit replay belong to the later scenarios. Current cancellation checks compare domain/links/config and never confirm deletion or a move.

## VM hook

Use the existing [VM guide](README.md) for transfer, login and credentials. Stage the checkout scripts, four tools, Node and SQLite in the guest's test directory; prepare guest-local paths there. Do not use host absolute paths inside a guest DB. A guest script can run the same `node scripts/vm/run.mjs ...` commands and write results to the guest's external evidence directory.

```powershell
pwsh scripts/vm/vm.ps1 exec guest-phase1a.ps1 -Interactive
```

`-Interactive` runs the script in the signed-in desktop session for W. It is unnecessary for C/A. Export evidence with the existing VM transfer helper before reverting a VM. This phase does not perform any VM reset, online subscription or game launch.

## Regression checks

```powershell
node --test scripts/vm/cdp.test.mjs scripts/vm/navigation.test.mjs
go test ./...
```
