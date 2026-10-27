# Background readers and needs-cache merge verification

Measured 2026-10-27 on Windows, Go 1.27.2. Merges `dev/0.9.0` at `0b30d42` into `codex/background-nonblocking`, retaining `b3edbbd` and `bd8d5d8`.

The API struct retains `filesMu`, detached page readers, the shared cold-index reader, and editor row caching, alongside `needsCache`, `needsSequence`, `listEdits`, and the workspace snapshot. The retained workspace reader is named `workspaceReader` so dev's `workspaceRead` method can keep its name. `UseInstalled`, the problems frontend, and subscription/task changes are retained.

`boardLocked` gets its needs from `needsLocked` and fills `Board.Problems`. A rebuilding Workspace computes needs once on its retained detached session; `shellLocked(reader.needs)` shares that result. The page pins a WAL read snapshot and releases it before publishing. Publication uses `AdoptReadSnapshot` and `saveNeedsLocked`, preserving one authoritative needs snapshot and assigning the receiving API's sequence and link-action tokens. Environment-switch adoption also uses `saveNeedsLocked`.

The workspace fast path requires unchanged needs and invocation keys. It validates needs only with a warm main index, so a scan-invalidated index reload stays outside the page mutex. Cold reads retain detached index/row caches and durable index-key invalidation. Request/environment sequences and `listEdits` reject overtaken results; unrelated commits can retry up to four times. Fast-path validation and publication retain the page lock through sequence checks.

`installIndexTriggers` no longer returns when `archo_index_gen` exists. Every `Open` executes all `CREATE TRIGGER IF NOT EXISTS` statements; the existing conditional INSERT creates the token row only once. `OpenSession` still never installs triggers or writes. `TestOpenRestoresIndexTriggers` drops a trigger, reopens, checks the generation is unchanged, and verifies an indexed INSERT advances it. GUI regressions cover shared needs adoption, unchanged-workspace reuse, count updates after edits, and an invalidated reader leaving other API calls responsive.

The measurement uses a fresh `sqlite3 -readonly ... .backup` of the real database and a copied `config.toml`, outside the repository, with `RIMARCHO_HOME` pointing there and `GOMEMLIMIT=256MiB`. The library contains 5,183 rows. Headless Chrome drives guidev through [check-background-readers.cjs](../../scripts/check-background-readers.cjs). Idle values are medians of three repetitions. Background values are one concurrent burst of Home, Shell, Workspace, PlanManage, and Library with ten immediate space presses; Workspace includes stale-result retries. Editor navigation includes two animation frames, as in the historical script.

Two SteamCMD mods tagged “I Dont Need It” are copied into scratch sources; only the copied DB's two instance paths are redirected. A temporary Go overlay on the dev harness checks canonical scratch paths, waits 15 seconds immediately before the actual Windows recycler, then invokes `source.Trash`. This matches the historical overlap gate; 15 seconds is not deletion latency. Child scan/analyze/monitor commands are disabled during this navigation/deletion check. All real installation and APPDATA data remain read-only. The historical column is quoted from `bd8d5d8`'s commit message, rather than a rerun against today's data.

Times in ms, idle / background:

| Metric | bd8d5d8 | Merged |
| --- | ---: | ---: |
| Home | 59 / 518 | 65 / 990 |
| Shell | 20 / 249 | 19 / 397 |
| Workspace | 58 / 1038 | 37 / 2133 |
| PlanManage | 88 / 243 | 59 / 393 |
| Library | 283 / 836 | 229 / 1313 |
| Editor navigation | 70 / 469 | 48 / 350 |
| Space success | 10/10 | 10/10 |
| Space median during background burst | 25.5 | 78 |

The merged result keeps the UI usable and improves idle Workspace, PlanManage, Library and editor navigation. Background API contention and space latency are higher than the historical result; the merged edits now include problem summaries. This run does not isolate the individual contributors to the difference. Both recycle items finish successfully, the editor retains 22 visible rows and 5 sidebar lists, and Chrome records no errors.

Warm editor / “can it run” switching is measured separately without the extra animation-frame wait, six samples each. Editor: 46–55 ms, median 48.5; needs: 74–108 ms, median 81.5. These remain below 110 ms but are slightly above dev's reported 12–42 / 55–96 ms ranges.

A real dependency-enable action changes `Board.problems.count` from 123 to 122; undo restores 123 and the original list fingerprint. Board sequences advance 20 → 24 → 27. `list-problems` events include the current environment ID, plan ID, and increasing sequences. The benchmark checks all these fields and restores the edit.

Validation passed: `go build ./...`, `go vet ./...`, `go test ./...`, `node --experimental-vm-modules --test internal/surface/gui/testdata/*.mjs` (73/73), and `node --check scripts/check-background-readers.cjs`. The Chrome script exits successfully. The copied mods are restored and all 41 original files are checked by SHA-256; scratch databases, binaries, overlays, browser profiles, and benchmark recycle-bin copies are removed. No scratch artifacts are committed.
