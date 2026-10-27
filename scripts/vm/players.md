# Crawler and impatient-player runs

Use Node 22+, the Phase 1a tools and a logged-in Windows desktop. Both modes launch
their own **real RimArcho window** and send CDP input. They never invoke product
actions through evaluation. `Runtime.evaluate` observes state/geometry and installs
the existing observer. All results, profiles, DB backups and images stay outside Git.

```powershell
$tools='D:/RimArcho-test/tools'
go build -o "$tools/archo.exe" ./cmd/archo
go build -ldflags '-H windowsgui -X github.com/ToumanLin/RimArcho/internal/buildinfo.GUI=1' -o "$tools/RimArcho.exe" ./cmd/archo
go build -o "$tools/vmstate.exe" ./spikes/vmstate
```

Each run owns a fresh home and clears only that DB's old undo/redo history before
its start checkpoint. Automatic collection network refresh is deferred in that DB
while external actions are disabled. No real AppData home is opened by a test.

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
not retained. A replay timeout terminates its owned process tree; cleanup does not
follow junctions into shared content. A killed process may require cleanup of its exact owned run directory.

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

`-SeedCount 5 -Seed 1` runs seeds 1–5 sequentially in one bundle and returns
`evidence/seed-<N>`. Interactive execution requires the wrapper's account to match
the user signed in to the guest desktop. If authentication fails, restore that
existing wrapper access or sign the test VM into the working standard account;
the runner never changes guest credentials or login state.

Saved-version checkout claims “u undoes” but can change workspace origin metadata
without an undo entry (F3). Those checks run on disposable DB branches and retain
the canonical failure while the parent stays usable. An unrestorable parent stops
its crawl branch instead of attributing later failures to unrelated controls.
