# Crawler and impatient-player runs

Use Node 22+, the Phase 1a tools and a logged-in Windows desktop. Both modes launch
their own **real RimArcho window** and send CDP input. They never invoke product
actions through evaluation. `Runtime.evaluate` observes state/geometry and installs
the existing observer. All results, profiles, DB backups and images stay outside Git.

```powershell
$tools='D:/vm-crawler-tools'
go build -o "$tools/archo.exe" ./cmd/archo
go build -ldflags '-H windowsgui -X github.com/ToumanLin/RimArcho/internal/buildinfo.GUI=1' -o "$tools/RimArcho.exe" ./cmd/archo
go build -o "$tools/vmstate.exe" ./spikes/vmstate
node scripts/vm/run.mjs --mode crawl --driver W --tools $tools --minutes 60 --out D:/vm-player-results
node scripts/vm/run.mjs --mode walk --driver W --tools $tools --seed 42 --steps 200 --minutes 10 --out D:/vm-player-results
node scripts/vm/run.mjs --mode crawl --driver W --tools $tools --baseline D:/vm-phase1a-real-baseline --minutes 60 --out D:/vm-player-real
```

Every invocation owns a fresh test home. Safe real-content runs use the normal
SQLite `.backup` path and the audited, already isolated content baseline. They never
open the player's AppData home. `--allow local` or `--allow external` makes a full
relocated filesystem copy through `prepare-real.ps1` before starting the window.
The legacy `--owned-files` shortcut cannot disable this protection for these modes.
Allow enough time and disk space for copying a large library. Before launching the
window, crawler/walker setup clears only the copied DB undo/redo history and records
the removed step count. The content start is retained. This prevents random Undo
from crossing into pre-run history and a later edit destroying the redo path back
to the experiment start; the baseline DB is never changed.

## Safety and coverage

The policy in `player-policy.mjs` classifies each discovered edge before input.
Safe includes navigation, views, cancelable panels and reviewed undoable workspace
edits. Local-destructive includes persistent local writes and confirmations.
External includes native pickers, browser/application launches, Steam and downloads.
Unknown controls are always skipped, even with `--allow external`. The live target
is classified again immediately before clicking, including during replay. Sidebar
route classification never overrides an external or destructive toolbar operation;
reviewed selection rows are classified independently of the mod title they display.

Local/external crawler edges run in fresh child cases: the parent state remains
available for other controls. A child records nonundoable writes explicitly and
verifies its resulting canonical state stays stable after dismissal. Workspace edits
still undergo undo comparison; F2 staging differences remain failures. External
mode is intended for the disposable test VM and can open a native dialog that needs
desktop interaction; a blocked native workflow is never counted as coverage.

A control records every graph state in which it was discovered. Sidebar controls
are deduplicated across focus/popups on their page; page controls use list/view
context; popup and menu controls retain their full opening context. This avoids
requiring the same sidebar button to pass again under every unrelated overlay.

The crawler uses page + current list + popup/title + menu options + focus + navigation
layout + view modes as a state key. It deduplicates states, saves input recipes and
checks each return. Discovery combines CONTROL-style selectors, delegated semantic
attributes, pointer handlers and nested controls; real wheel input reaches offscreen
controls. Keyboard recipes cover editor columns, the palette, context menu, tags,
position, note, copy choice, plan/history choosers, import/export, help and filtering.
Refnums are enabled in the isolated home; observed OpTable and command entries are
saved in `operation-seeds.json`.

`ledger.json` lists every discovered control, including hidden, disabled, covered,
unclassified and unreachable controls. Repeated virtual rows are represented by
area/action/missing class; this is not every possible combination of real content.
The short `ledger.md` reports per-page coverage as asserted / eligible controls.
Eligible means permitted, visible, enabled and not covered by a popup/menu. An
offscreen eligible control remains in the denominator when scrolling cannot reach
it. Hidden/disabled controls remain in the ledger with their prerequisites/reasons.
`pressed-ok` requires generic oracles, a canonical snapshot and verified return.
These assertions do not prove action-specific properties such as sort optimality.

Pass `--ledger <previous-ledger.json>` to diff stable control keys (new/removed).
For a walker, it also restricts random button choices to the prior crawler's permitted
controls. Without it, the walker discovers and classifies controls at runtime. It
mixes 20–100 ms bursts and normal pacing, page/list switching, repeated/held keys,
selection, sort, enable/disable, move, stage, double/triple clicks and popup cancellation.
`--steps` counts action groups, not raw key or pointer events. The trace records every
driver action, including resets and final undo. Continuous DOM sampling and checkpoint
oracles check responsiveness, loading, sidebar integrity, scripts and problem sequences.

Page scheduling rotates across sidebar pages before taking another edge on the
same page. Native disclosure sections are restored with real clicks before recipe
replay; distinct disclosure/tab paths participate in state identity. Undo restoration uses the explicit workspace palette command, including
redo restoration after testing Undo itself; it does not assume undo depth increases.
Background analysis remains observable while lightweight polling waits for page
loads and workspace edits. A nested memory-estimate shimmer is recorded as a pending
region while the page remains usable; it is not a whole-page loading failure. Event evidence retains scalar metadata; independent
canonical checkpoints retain the complete domain.

The default crawl budget is 60 minutes / 200 queued states (`--max-states N` changes
the latter). A bounded crawl reports `crawlLimit` and `playerCoverage.complete=false`;
passing observed controls does not imply complete discovery. Unvisited edges remain
unreachable with reasons. Walkers stop at the first functional failure or budget.

## Canonical restoration and reproduction

The full independent `vmstate` snapshots retain undo/redo payloads. Restoration
compares the canonical domain, including workspace, saved versions, staging, tags,
notes, sources, instances, links and ModsConfig bytes. Explicit exclusions are undo/
redo stacks/depths (undo creates redo history), open-list navigation, computed problem
publication/cache state, UI settings/focus/selection/scroll, clipboard and log/task
history and `plan_collection.checked_at` refresh timestamps. Collection source/hash/
content fields remain compared. Reachable lists are opened during setup to materialize
saved workspaces before the start checkpoint. Exclusions are recorded in every result;
staging is never excluded.
The existing F2 stage/undo defect is recorded as a known app failure until the separate
stage-undo branch is merged.

`result.json`, `trace.json` and `replay.json` contain the seed, failures and replay
command. Replay preserves recorded input spacing. Disposable branch actions are
embedded in the trace so copied evidence does not depend on guest-only branch files. Failed walks automatically retry
shorter prefixes and remove chunks on fresh copies, accepting only a repeat of the
same oracle, known-failure ID and canonical difference path. `minimal-trace.json` and `shrink.json` report the best bounded reduction;
this is not a proof of global minimality. Use `--shrink-runs N` (default 6) or disable
retries with `--shrink false` for seed sweeps.

```powershell
node scripts/vm/run.mjs --mode walk --driver W --tools $tools --seed 42 --steps 200 --replay D:/vm-player-results/walk-W-ABC/minimal-trace.json --shrink false
foreach ($seed in 1..5) {
  node scripts/vm/run.mjs --mode walk --driver W --tools $tools --seed $seed --steps 200 --shrink false --out D:/vm-player-results
  node scripts/vm/run.mjs --mode walk --driver W --tools $tools --seed $seed --steps 200 --shrink false --out D:/vm-player-real --baseline D:/vm-phase1a-real-baseline
}
node --test scripts/vm/*.test.mjs
go test ./...
```

## VM provisioning

Read `D:/VMs/PLAYER-TEST-AI-HANDOFF.md` first. Only use Win11-Player-Test. Do not
reset snapshots or access credentials. Wait until the user releases the VM from its
other test task. The host may prepare a bundle without contacting VMware:

```powershell
pwsh scripts/vm/player-vm.ps1 -Mode walk -Seed 42 -Steps 200 -BundleOnly
# Only after the user says the VM is free:
pwsh scripts/vm/player-vm.ps1 -Mode crawl -Minutes 60
pwsh scripts/vm/player-vm.ps1 -Mode walk -Seed 42 -Steps 200 -StandardUser
```

The host script builds archo/RimArcho/vmstate, bundles Node + SQLite + scripts,
hashes the zip and each file, pushes through `vm.ps1`, verifies hashes in the guest,
and executes via `vm.ps1 exec -Interactive` in the logged-in desktop session. Supply
a standalone SQLite executable if the host copy depends on unbundled runtime DLLs.
Use `-GuestBaseline <already-isolated-guest-baseline>` and `-PowerShellZip <portable-
pwsh7.zip>` for real-content preparation. Guest baseline paths must already be guest
local; the script does not upload private real content or rewrite host DB paths by
string substitution. No guest Git, Node or PowerShell 7 installation is needed.

Results return to `D:/VMs/Transfer/vm-player-<mode>-<time>-<token>/evidence` using an
allowlist: result, ledger, trace, replay and shrink JSON plus the ledger markdown.
Screenshots, DBs, profiles and arbitrary files are never copied back by this script.
Guest runtime validation, interactive desktop behavior and native external workflows
still require an actual guest run; a successful bundle-only check is not VM acceptance.
