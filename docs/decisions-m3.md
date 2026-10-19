# M3 decisions and measurements

## M1 — TUI memory headroom (task a)

Changes:
- `store.Mods` no longer selects `decl_blob` (lite columns); `Mod.Decl()` loads it lazily, `Store.LoadDecls` batches it
  for sort. `ModsFull` is kept for callers that need every declaration (Known, sort input).
- `app.Index()` and `app.Loadout()` are cached, keyed by `Store.ChangeKey()` (`PRAGMA data_version` + `total_changes()`)
  plus env id (+ ModsConfig size/mtime for the loadout). Repeated TUI refreshes no longer rebuild them.
- `debug.SetMemoryLimit(40 MiB)` in `tui.Run` (soft cap; GC works harder when the heap approaches it).
- `spikes/tuimem`: headless TUI driver (no terminal) printing working set / private bytes / Go heap per phase.

Measurements on the real environment (485 entries):

| | before M3 | after |
|---|---|---|
| teatest harness, 10 min idle (WS) | 78.7 MB | ~65–69 MB |
| real `archo.exe` in its own console, idle untouched | — | WS 25.6–28.7 MB, private 63–66 MB |
| headless tuimem after scan + browsing, idle | — | WS 67.9 MB, private 68.8 MB |

Phase breakdown (tuimem, no browsing): opened 10.6 MB WS / 48.7 MB private → workspace loaded 22.9 / 62.4 →
list shown 63.1 / 63.9 → idle 62–63. Go heap in use stays 5–9 MB throughout. About 45 MB of private memory is committed
at process start outside the Go heap (modernc libc/memory arena and TLS stacks); SQLite activity makes it resident.
FreeOSMemory, `PRAGMA shrink_memory` and closing idle connections were tried and gave no measurable effect.

Verdict: against working set of the real binary the ≥15 MB headroom is met comfortably (≤ 29 MB). Against private
bytes / headless WS it is ~12–17 MB below 80 MB — borderline, not clearly met. Per the ruling, we do **not** switch.

### cgo SQLite option (written up, not taken)
- `mattn/go-sqlite3` (cgo) would drop the transpiled libc arena: expected private bytes ~25–35 MB for the same workload
  (SQLite's own page cache is bounded by `cache_size`, default 2 MB).
- Cost: requires a C toolchain (mingw-w64) for every build incl. CI and release, loses trivial cross-compilation, adds
  a DLL-free static-linking chore on Windows; contradicts 07 §1 "pure Go, single exe".
- Reason not switching now: the user-visible number (task manager "Memory" column = private working set) is ~28 MB;
  the 80 MB budget is not exceeded in any measurement; the remaining overhead is fixed, not growing.
- Revisit if the end-of-M3 measurement (with analyzers loaded in the TUI process) crosses 70 MB private.
