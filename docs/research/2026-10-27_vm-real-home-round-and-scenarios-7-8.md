# VM real-home rounds and scenarios 7–8 (evening of 2026-10-27)

Branch `dev/0.9.0`, starting at `655444b`. Continues items 1, 2, 3 (7, 8), and 6 in `tmp/handoff-2026-10-27.md`.

## 1. VM scripts now use the real home directly (`f5fa61c`, `5cb0fc4`)

The old `--baseline` required a 22 GB copied baseline in the guest; that requirement is removed. Now:

- `player-vm.ps1 -RealHome` (runner `--real-home true`, allowed only in the guest) directly uses Player's `%APPDATA%\RimArcho`, APPDATA/LOCALAPPDATA, and Steam content.
- Before each run, back up `archo.db` with SQLite and copy `config.toml` and `tasks.json`; restore after the application exits. Seeds 1–5 thus start from the same state each round. Snapshot restoration remains the final reset after the entire round.
- Crawler branches and minimized replays (subprocesses) build private DBs from this backup (`--start-db`), since the parent window remains open.
- Content writes (deleting/moving Mods, deployment) remain blocked; only DB list/tag operations are allowed.
- UI language is set to English during runs: the walker and control classifier recognize English text (the old baseline preparation script did this too). Restoring the DB restores the player's original language setting.
- First validated on the host with a generated fixture posing as the real home: walk/crawl both passed; the post-run DB `.dump` was byte-identical to its pre-run state, including restored Chinese settings.

## 2. Random player, real VM data, seeds 1–5 × 200 steps

VM snapshot `RimArcho-Real`, real home: 1271 instances, 1 list (default). `-Allow local`, limit 20 minutes per seed. Evidence: `D:\RimArcho-test\transfer\vm-player-walk-*\evidence`.

**First round** (`f5fa61c`): all five seeds reached cleanup (179/162/200/20/200 steps); the real home was successfully restored each time. F2, previously blocking every seed, did not recur, and there were no performance failures. All failures were in tooling: cleanup searched the command panel for English "undo", but the real home used the Chinese UI. Fixed in `5cb0fc4`.

**Second round** (`5cb0fc4`):

| Seed | Steps | Time | Result |
|---|---:|---:|---|
| 1 | 200 | 232 s | Passed |
| 2 | 200 | 223 s | Staging mismatch after undo (labeled F2) → see below |
| 3 | 200 | 214 s | Functionality passed; one first-feedback sample 297 ms (budget 200 ms) |
| 4 | 200 | 433 s | Passed |
| 5 | 200 | 241 s | Passed |

Seed 2's "F2" was a walker false positive: cleanup undid to "the latest step whose entries match the beginning". Staging a disabled Mod does not change entries, so after staging followed by another edit, the latest matching step still has full staging. Reproduced on a fixture with CLI: `stage add` a disabled entry, then `enable` another; the two undo steps have identical entries but different staging. `26a4307` prefers a step matching both entries and staging. Seed 2 rerun with this version passed 200 steps (465 s).

Seed 3's 297 ms was a page switch away from the "game" page, the only over-budget sample among 990 feedback samples; not investigated further.

Conclusion: after the F2 fix, none of the five seeds found application bugs with real data; staging undo behaved correctly with real data.

## 3. Item 6: editor rows and sidebar (`597bc1b`)

- Uninstalled list entries previously showed only packageId. They now show a name (name from import → previously installed copy or copy in another game → local steamDB index → packageId), with "not installed" on the same row; if an installed copy can substitute, show "installed copy available". Mods whose folders were deleted (tombstones) also show "not installed".
- For Mods with multiple copies in the enabled column, the source (the "└ workshop" line) moves onto the name row, restoring one-line row height.
- Shell `error` field: when Shell reads fail (background tasks hold the data), they return a shell with error but no lists. Previously the editor's Workspace read used it directly, losing sidebar lists. Both read paths now retain the original sidebar and retry (up to 10 times).

## 4. Scenario 7: export 8 formats × reimport

CLI driver: run `loadout export` on the current list for each format, then `loadout import --new`, comparing entries (packageId order and instances).

Host-generated fixture (5 entries, including 2 unresolved): archo, text, modsconfig, rimsort, rml, rimcrow, and rimcrow-xml all preserved packageId order and instances on round trips. Workshop-link format: no Workshop Mods in the fixture, so export contained comments only and reimport reported "no mods in the file", as expected.

VM real home (build from `3e8dae8`, CLI, snapshot restored afterward):

- default list (6 entries): seven formats round-tripped identically; no Workshop Mods, so workshop format behaved as above.
- Temporary `rt-big` built with CLI (Workshop Mods plus completed dependencies, 42 entries; intended to take 470, but truncated `archo list` output yielded only 30): eight exports plus eight imports took 2.1 s.
  - Seven formats: no entries lost, order preserved; every reimport added one Archo Monitor entry (position 2). This is "new lists add Monitor by default" (07 §1.5), not a bug.
  - workshop format: lost Core, Royalty, and Biotech (format records Workshop entries only), and likewise added Monitor. The result is a list without Core. The format is inherently lossy, but whether imports should automatically restore Core/owned DLC needs a decision.

Also checked the steamDB index used for missing editor rows in the VM: neither guest nor host had downloaded steamDB, so lookup falls straight back to packageId. For players who have downloaded it, the index is parsed once per process; a simulated 45,000 entries (4.4 MB) parsed in about 13 ms.

GUI portion (preview cancellation, overwrite confirmation, undo after merge) not performed.

## 5. Scenario 8: post-sort repair dialog

Used guidev + real browser clicks on a host fixture containing installed but disabled dependencies, dependencies to get from the Workshop, unresolved entries with available copies, installed recommended replacements, and entries without a known source.

- All five categories appear in the dialog; Esc cancels without undoing the sort; `u` correctly undoes the sort.
- "fix N and get M" applies local fixes first, then opens Steam/SteamCMD selection; Esc in that selector does not subscribe (subscription was not clicked).
- Each fix is one undo step; repeated undo returns to the pre-sort state.

Found and fixed (`3e8dae8`):

1. **Recommended replacements conflict with dependencies.** `dependent` hard-depends on `vm.old`, yet the dialog suggested "old → new". One click replaced old with new, then "complete dependencies" added old back, enabling both. Now a replacement is not proposed when another enabled Mod depends on the original by packageId (`canReplace` false, with the dependent named in the reason).
2. **Two untranslated history entries**: "complete dependencies and sort", "replace recommended mods".

Found, not fixed (decision needed):

3. **Save says "no changes", although saving must still create links.** After "use installed copies", the list is unchanged from the previous version (same packageId), so the button shows "no changes"; however, `save --dry-run` lists new `_Archo_` links, and the repair explanation says "any needed links are created when you save or launch". The button works, but its text implies saving is unnecessary. Computing pending links while reading the editor would add Workspace read cost (the path F4 targets), so this remains unchanged.

## 6. Disk

Host `D:\RimArcho-test` used 464 MB at cleanup (15 MB total for three VM evidence rounds, 34 KB for scenario 7 evidence). Each generated fixture was deleted after use. Each guest round retained only 5–8 MB evidence; C: had about 60 GB free. Finally restored snapshot `RimArcho-Real`, verified the guest had no run directories from this session and the real-home DB was still the 10-24 version. VMware did not show those two prompts during this restore.
