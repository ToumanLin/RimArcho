# needs cache and list-problems API measurements

Measured on 2026-10-27. The interface contract is in [06 §8.7](../specs/06-ui/8-gui-technology.md#87-%E5%88%97%E8%A1%A8%E9%97%AE%E9%A2%98%E4%B8%8E-needs-%E7%BC%93%E5%AD%98-api). This records one validation with real content; these figures are not latency guarantees for every machine.

## Data and method

Windows amd64, AMD Ryzen 9 7950X, Go 1.27.2. Data came from an isolated copy of `%APPDATA%\RimArcho`: copy `archo.db`, WAL, SHM, and `config.toml`, checkpoint only the copy; also copy caches and create an empty packs directory. Real game/Mod/log paths were read-only, and HTTP was disabled. No ModsConfig saves, scans, subscriptions, or real Mod file changes were performed.

The same large list resolves differently in two environments, so distinguish list-entry counts from actually enabled counts:

| Scenario | List entries | Actually installed and enabled | Uninstalled entries | Indexed instances |
| --- | ---: | ---: | ---: | ---: |
| Large list opened in 1.6 environment | 715 | 715 | 0 | 6640 |
| Same list opened in main (1.7) environment | 715 | 572 | 143 | 6640 |

The request recorded 713 enabled entries; by measurement time the same large list had 715. The main comparison uses **715 actually enabled Mods in the 1.6 environment**. Baseline commit `926eb32824e617f687674a35ef976c18137bf474` was exported with `git archive` to a system temporary directory and run from the original source. Before/after runs used the same data copy and list. Before timing, the baseline completed one original analysis of this set, avoiding treating old results with missing analysis as final results.

Backend processing time excludes WebView rendering and IPC; Workspace time includes waiting from the request to the `environment` event. The OS file cache was not cleared. Background analysis ran real `Analyze` in a separate App, keeping the environment and open list aligned; production subprocess startup and IPC overhead were excluded.

## Before/after: large 1.6 list

Units: ms. Warm-read columns retain actual samples rather than mixing different freshness states into one latency.

| Operation | Original source | Cache implementation |
| --- | --- | --- |
| First Board composition | 505.338 | 492.787 |
| First Needs response | 394.043,311 items, analysis already fresh | 19.472, initially returns 121 old analysis items, `stale=true/updating=true` |
| Stable Needs hit / return to page | 192.487 / 203.878 / 216.914 | 12.178 / 11.185 / 10.799 |
| Ordinary disable operation | 3.108, edit result only | 196.463, includes updated problem summary |
| Needs read after disabling | 550.907 | 13.667 |
| First Workspace | Not measured separately in this baseline round | 421.039 |
| Stable Workspace hit | See previous day's P6 research | 12.100 / 10.778 / 11.656 |
| ListProblems | No such API | 29.479 / 22.786 / 46.256 / 24.625 |

After content analysis completed, the new full result contained **143 items**, not the 121 old items initially returned. One analysis commit invalidated Workspace; recomposition took 681.781ms. Warm reads in the table are subsequent hits on the same key. First composition and effective invalidation still have costs; the frontend can retain its existing list and consume problem-update events.

ListProblems returned 267 Mods with associated problems and 143 list items, totaling 314,774 B of JSON. Full problems are transmitted once in `items`; each Mod references them through `problemIds`.

## Merge and fix validation

Original needs **311 items → 143 items**. Missing dependencies and disabled patch targets merge by normalized packageId:

| Target | Declared dependency problems | Cards after merging | Mods needing it (including patch targets) |
| --- | ---: | ---: | ---: |
| Biotech | 47 | 1 | 53 |
| Ideology | 32 | 1 | 34 |
| Anomaly | 15 | 1 | 17 |
| Odyssey | 15 | 1 | 17 |

Two actual returned `enable` actions were sampled and executed on the copy, verifying that the target entered the enabled list and its dependency card disappeared immediately. Each was then undone, restoring the list fingerprint. Including summaries, the actions took 172.788ms and 169.405ms; subsequent reads took 13.238ms and 13.838ms. Background publication was deferred for these action checks to isolate undo validation; the ordinary disable sample above ran with real background work enabled.

After disabling, full content analysis published `stale=false/never=false/updating=false` results in **11,658.055ms**. The analyzer itself recorded 5,705ms, reusing Def caches for **4294 Mods**, reading/simulating 3970 Def files and 1683 Patch files. Def/assembly facts reuse each Mod's effective-content key; Patch simulation depends on the full document and order, so it still recomputes in the background.

Additional main (1.7) sample: original warm needs reads 159.742–173.617ms, after edits 516.059–531.947ms; cache hits about 7–9ms, full results 263 items → 133 items. This scenario has only 572 installed and enabled Mods and cannot replace the 715-enabled validation above.

## Reproduction

Source: [TestNeedsRealCopy](../../internal/surface/gui/needscache_real_test.go). Skipped by default; runs only with a copy directory set and a `needs-benchmark-copy` marker present. The SQLite read-only backup below incorporates committed WAL contents, equivalent to copying DB/WAL/SHM; outputs go to a system temporary directory.

```powershell
$benchCopy = @"
from pathlib import Path
import os, sqlite3, shutil, tempfile, sys
src = Path(os.environ["APPDATA"]) / "RimArcho"
dst = Path(tempfile.mkdtemp(prefix="rimarcho-needs-"))
with sqlite3.connect((src / "archo.db").as_uri()+"?mode=ro", uri=True) as a:
    with sqlite3.connect(dst / "archo.db") as b:
        a.backup(b)
shutil.copy2(src / "config.toml", dst / "config.toml")
if (src / "cache").exists():
    shutil.copytree(src / "cache", dst / "cache")
(dst / "packs").mkdir()
(dst / "needs-benchmark-copy").touch()
with sqlite3.connect(dst / "archo.db") as db:
    print("plans=" + repr(db.execute("select id,name from plan").fetchall()), file=sys.stderr)
print(dst)
"@ | python -
$env:RIMARCHO_NEEDS_HOME = $benchCopy
$env:RIMARCHO_NEEDS_ENV = '1.6'
$env:RIMARCHO_NEEDS_PLAN = '5'
go test ./internal/surface/gui -run '^TestNeedsRealCopy$' -count=1 -timeout 6m -v
```

`5` is the large-list ID in this copy; choose from `plans=` output when rerunning. Leave `RIMARCHO_NEEDS_PLAN` unset to test the environment's current list. The test edits/undoes only on the copy and restores the list fingerprint afterward.

Validation commands:

```powershell
go build ./...
go vet ./...
go test ./internal/... -count=1
```

Regressions cover cache inputs, invalidation after writes from other connections, case/directory-alias/`_steam` merging, content reuse across consecutive edits and undo, old background results not overwriting newer edits, discarding Workspace when superseded by edits, live directory/link changes, and executing a link-repair action generated by an independent session in the main session.

## Boundaries

First opening, or completion of scanning/analysis, may invalidate the cache. The hit target does not mean first full content analysis also completes within 50ms. Old results must show `updating`; failures return through `error` and analysis events. API shutdown waits for started reads and analyses to exit, avoiding leftover writes.

Subscription and DLC-store actions were checked only for generation, dispatch, and existing task interfaces; real validation did not download or purchase anything. The frontend must consume `Board.problems`, `Result.problems`, and `list-problems`, filtering late events by environment/list and publication sequence. Testing excludes page rendering.
