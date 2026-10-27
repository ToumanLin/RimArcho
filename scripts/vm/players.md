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
to the experiment start; the baseline DB is never changed. Unless `--allow external` is enabled, copied
collection refresh timestamps are deferred through the run budget to prevent the
plans page from automatically contacting Workshop on opening. Follow source/content
remain retained and compared; the setup is listed in `deferredExternalEffects`.

## Disk policy (required)

Host tests use only the generated fixture. All tools, bundles, logs and results live
under `D:/RimArcho-test`; no host real-data baselines are accepted. Before each run
the runner checks the root against an 1800 MiB admission limit. Keep the total below
2 GiB and measure it before reporting. Never remove pre-existing `D:/vm-*` or
`D:/stage-undo-*` directories as part of this task.

Every run terminates its own process tree and deletes its owned case/home/content,
APPDATA and WebView profiles. Retained evidence consists of result, coverage ledger,
trace/replay/shrink, operation seeds and bounded logs. Canonical snapshots are read
in full and compared in memory; checkpoint hashes/counts and first differences are
retained in result.json. Screenshots, databases and complete observer samples are
not retained. A killed process may require cleanup of its exact owned run directory.

```powershell
$tools = 'D:/RimArcho-test/tools'
node scripts/vm/run.mjs --mode crawl --driver W --tools $tools --minutes 60 --out D:/RimArcho-test/crawl
foreach ($seed in 1..5) {
  node scripts/vm/run.mjs --mode walk --driver W --tools $tools --seed $seed --steps 200 --shrink false --out D:/RimArcho-test/walk
}
node --test scripts/vm/*.test.mjs
go test ./...
```

## VM real content

Read `D:/VMs/PLAYER-TEST-AI-HANDOFF.md`. Use only the released test VM; no snapshot
reverts, credential printing or baseline replacement. The existing guest baseline
is `C:/Users/Public/Documents/VM-Test/real-baseline` (about 22 GB). Reuse its content:
a run takes only a SQLite backup and config file, never another game/workshop/cache
copy. `--baseline` requires `RIMARCHO_VM_GUEST=1 --reuse-content true`.

With shared content, filesystem/deployment writes are skipped with a reason even
with `--allow local`. Reviewed DB-only plan/tag operations remain available in
private disposable DB branches. External operations stay disabled unless explicitly
authorized. Scenario 2 deployment switching requires private content and is refused.

```powershell
pwsh scripts/vm/player-vm.ps1 -Mode crawl -Minutes 60 -Allow local -GuestBaseline 'C:\Users\Public\Documents\VM-Test\real-baseline'
pwsh scripts/vm/player-vm.ps1 -Mode walk -Seed 1 -Steps 200 -Allow local -GuestBaseline 'C:\Users\Public\Documents\VM-Test\real-baseline'
# Player-visible last-plan refusal, second-plan cancellation and subscription preview only:
pwsh scripts/vm/player-vm.ps1 -Mode walk -Probe -GuestBaseline 'C:\Users\Public\Documents\VM-Test\real-baseline'
```

The host script builds and bundles tools, Node, SQLite and scripts, verifies hashes,
executes with `-Interactive`, retrieves allowlisted evidence and deletes its owned
bundle and guest result copies. Transfer/evidence live under
`D:/RimArcho-test/transfer`. Portable PowerShell 7 is optional; DB-only reuse runs
under guest inbox PowerShell 5.1. Guest `disk.json` records free space and retained
run bytes. No private content is uploaded or copied back.

## Coverage and reproduction

Each ledger control records its stable key, state/page, tier, eligibility, outcome,
reason and timing. `pressed-ok` requires generic oracles, canonical comparison and
verified return. The coverage denominator includes eligible controls; disabled,
unknown, external and inaccessible edges remain visible with reasons. `--ledger`
diffs new/removed controls. Representative virtual rows cover action classes,
not every mod combination. A bounded crawl reports incomplete discovery explicitly.

State keys include page, plan, popup/menu, focus and relevant view/layout modes.
Discovery recipes include sidebar routes, editor columns, right-click menus,
palette, documented shortcuts and disclosure sections. Safety is checked again
before each real click, repeated click and held-key repeat. Unknown actions are
never pressed. Persistent permitted actions use disposable DB branches.

The walker uses a seeded PRNG, weighted key/button bursts, list/page races,
double/triple clicks, sort/move/toggle, misclick/cancel and random classified controls.
Continuous checks cover responsiveness, page/sidebar integrity, errors, loading,
result preservation and monotonic problem sequences. At completion undo restores
the start canonical domain. Exclusions are recorded in each result: undo history,
open-list navigation, computed analysis, refresh timestamps and UI/log/clipboard
state. Staging remains compared; F2 remains a known app failure owned by stage-undo.

`trace.json`, `replay.json`, `minimal-trace.json` and `shrink.json` contain exact
inputs, seed and replay command. Shrinking retries shorter prefixes and chunk
removals in fresh small cases. `--shrink false` disables retries for seed sweeps.
The bounded shrinker does not claim a globally minimal sequence.
