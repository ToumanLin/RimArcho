# Game Configs VM acceptance round — 2026-11-12

Result: **not accepted**. Both isolated Game Configs completed real 1.6 colony/save/load rounds, but their game-log pages cannot find Player.log. Three app bugs are recorded below. No app code or VM tooling was changed.

## Setup and evidence

- Host checkout: `dev/1.1`, `2235e6da14a60c82ad9b99661cae6e99ad845743`. Report branch: `vm/game-configs-round`; worktree: `.claude/worktrees/game-configs-round`.
- VM: `Win11-Player-Test`, `D:\VMs\Player-Test\Win11-Player-Test.vmx`. Restored **RimArcho-Real-Vulkan**, then started with `gui`. Player was already signed in; VMware Tools reported running. Guest IP for this round: `192.168.86.130`.
- Steam freshly logged on at 2026-11-11 23:31:12 Pacific (`connection_log.txt`). No sign-in prompt, credentials entered, SteamCMD, account/subscription changes, snapshot creation or snapshot deletion.
- Test interval: approximately 2026-11-11 23:30 through 2026-11-12 00:30 Pacific, within the four-hour cap.
- Existing schema-18 `archo.db*` moved aside inside the guest, then `env add Steam --detected` and real-Mod scans. The initial setup script was interrupted when the GUI was launched during its scan; the GUI scan subsequently completed. Acceptance baselines use completed `scan:2`, not that interrupted command.
- Baseline: 1,301 live copies: six official, 1,293 Workshop, two installed Monitor versions; 1,300 Lineages. Small played lists: Harmony, bundled Archo Monitor 0.11.1, Core, all five DLCs, HugsLib 12.0.0. The subscribed Monitor folder `3835688284` was supplied through `-ExtraMonitor` for every driver round.
- Detection reports `1.6.4871 rev590`; the running game reports `1.6.4871 rev591`. This observed metadata difference was retained, not fixed. 1.7 was not present and was not tested.
- Built unchanged code with `vm.ps1 build` in the worktree. CLI SHA-256: `5D435C5A55B3A38C8CAF398D855F8235B9B234712CE0179823E255251FD5F9D5`; GUI: `632AE1604DE4F7CC0930F0F4DE7890C4AFD7CB7E9CDB14EA491F893BE335F214`. Full paths and pre-split hash: `build-hashes.txt`.
- Evidence root, including all screenshots referenced below: `D:\RimArcho-test\gameconfig-vm-20261112\`. Real games, Mods, databases and saves stayed in the guest; host evidence consists of screenshots, diagnostic output and log excerpts. The pre-split executable is also staged in this host test folder. No bulk real-content transfer was made. Final guest C: free space was 58,719,113,216 bytes; no precise before/after virtual-disk allocation measurement was taken.

## Requested steps

| Step | Result | Evidence and limits |
| --- | --- | --- |
| 1. Pre-split upgrade | PARTIAL: data/launch PASS; literal output equality FAILS on an additional Monitor line | Built `4f8ea248` in `.claude/worktrees/game-configs-presplit`, created/scanned guest home `C:\Users\Public\Documents\VM-Test\GC-Upgrade`, backed it up while closed, then opened with the new executable. `env list`, `status`, `plan list`, `launch --dry-run` retained names, paths, sources, inventory, current list and launch command. Environment output is reworded/restructured; new status adds `monitor: not installed`, so output is not literally identical apart from wording. `upgrade.txt`, `upgrade-recheck.txt`, `upgrade-final.txt`. Initial status-count differences came from comparing before and after log-cache work; the final comparison with the game stopped has identical findings. Final recheck adopted the same externally changed nine-entry list on both sides. The larger release row's seeded ignores, packs, numeric-ID and failure-recovery matrix was not exercised. |
| 2. Add configs and seed settings | PASS | GUI added `GC-A` with default Copy current Game Config/current `default` list/own saves. Plans stayed at one. CLI `env add GC-B --like GC-A --saves own --new-list` created the plain `GC-B` Core/DLC list. Both roots initially contained only the source's `KeyPrefs.xml` and `Knowledge.xml`; source had no Prefs or HugsLib yet. After A's first game visit generated those, `GC-Seed --like GC-A --saves own` copied three Config XML files and two HugsLib files byte-identically, without ModsConfig.xml, logs or saves; plan count remained two. `02-add-config.png`, `add-checks.txt`, `seed-tui.txt`. No Mod files copied; installation inventory remained shared. |
| 3. GUI/TUI switch without deployment | PASS for exercised switch paths | GUI Game Config page switched B → A → B; native TUI flat chooser switched A/B, with names, short versions, current marker and Add Game Config, no installation headings. `stable-switch-before.txt`, `stable-switch-after-gui.txt`, `stable-switch-after-tui.txt`, `stable-gui-switch.txt`, `tui-final-attempt.txt`, `tui-after-switch.txt`. All three ModsConfig hashes/mtimes, Mods listing/junction target/mtime, inventory and `scan:2` remained unchanged. Earlier measurements spanning game launches are superseded by these stable captures. The GUI chip opens the config page; its flat rows were used, rather than claiming a separate popup was tested. |
| 4. Enter each game, save/load, isolate settings/saves, inspect logs | FAIL overall; gameplay and isolation PASS | A and B each completed all seven driver steps, including 120 s play, save, load, 60 s play, return to menu. Both exited afterward through Quit to OS. A: 2,971 + 1,913 played ticks, load tick 3,025. B: 6,032 + 3,068 played ticks, load tick 6,227; final tick 9,321. Both well below 60,000 ticks/day. Driver opened 39 windows/tabs before each save and 20/19 after load. `A-round-console.txt`, `B-round-console.txt`, `round-A/`, `round-B/`. Both Monitor phase/save/load checks passed. Saves page identifies A's save as GC-A/default and B's as GC-B/GC-B, with their own full folder paths. The page is globally scoped by design; seeing both colonies there is not cross-folder copying. B's Run in background=True left A=False. Both restored autosave=1 day/count=5. `after-B.txt`, `28-A-save-page.png`, `A-save-path.txt`, `GC-A-saves-page.png`, `GC-B-saves-page.png`. Game log pages fail and Can it run reports no last-run errors without finding Player.log: bug 1. |
| 5. Player.log location | FAIL isolation; question answered | Both savedatafolder launches wrote the same default LocalLow Player.log. B replaced A's active log and rotated it to Player-prev.log. Monitor logs were separate. Exact paths/times below. |
| 6. External default-data process while B selected | FAIL display clarity; launch guard PASS | Directly started RimWorldWin64.exe without arguments, PID 13504. CLI launch refusal correctly named `Game Config Steam`; another launch was blocked. CLI status and GUI home did not identify Steam: bug 2. `external-start.txt`, `external-check.txt`, `external-home.txt`, `22-external-home.png`, `23-external-game.png`. `save` was allowed, including on Steam, and explicitly said the new list is read on the next start; this is recorded rather than treated as proof of an unsafe-write defect. No physical-link change was requested by that save. Unreadable/unknown-root and dangerous link-write cases remain untested. External game reached menu and exited through Quit to OS; no colony or save created. |
| 7. Launch arguments | PASS for requested literal refusal and display | `env set GC-B --launch-args "-savedatafolder=C:\x"` refused with the named `--user-data` fix. GUI displayed `-screen-width 1280 -screen-height 720` as command-line text, not JSON. `add-checks.txt`, `21-B-settings.png`, `settings-refresh.txt`. Mixed-case/quoted override variants and a stored legacy override were not tested. A PowerShell 5.1 empty-string invocation failed in the harness; it is not an app defect finding. |
| 8. Pack built in A, visible/launched from B | PASS | Built `GC-Small` in A: 3 carried Mods, six official, 3.8 MB. B's CLI list retained `env GC-A`; GUI details show GC-A. Launch from selected B used the pack's own userdata and reached the main menu, then Quit to OS. `pack-build.txt`, `pack-launch.txt`, `24-pack-page.png`, `26-pack-game.png`, `27-pack-detail.png`, `pack-detail.txt`. No pack save created. |
| 9. Remove non-current sibling | PASS functionally; localization FAIL | Removed `GC-Seed` in GUI with B current and game stopped. Fresh CLI list has only A, B, Steam; all five Seed file hashes/mtimes remain identical, inventory stays at 1,301 live copies and both global lists remain. `remove-before.txt`, `remove-verified-after.txt`, `remove-verified-env-list.txt`, `29-removal-verified.png`. First attempt while pack ran did not remove the row; its unchanged file check was insufficient evidence of removal. Only the stopped-game retry is counted as passing. Confirmation localization: bug 3, `25-remove-confirm.png`. |

## Player.log answer (03 §6.3)

On this real Windows RimWorld 1.6 build, **`-savedatafolder` does not relocate Unity's Player.log**. The game's own Options window also shows the custom save folder alongside the default LocalLow log folder (`06-A-options.png`, `17-B-options.png`).

| File | Observed location |
| --- | --- |
| Current Unity log for A and B | `C:\Users\Player\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios\Player.log` |
| Rotated Unity log | Same directory, `Player-prev.log` |
| A Monitor log | `C:\Users\Player\AppData\Roaming\RimArcho\userdata\GC-A\ArchoMonitor.log` |
| B Monitor log | `C:\Users\Player\AppData\Roaming\RimArcho\userdata\GC-B\ArchoMonitor.log` |

No Player.log or Player-prev.log existed directly under either custom root or under its `Logs/` subdirectory. No default `Logs/Player.log` existed either.

All times in this paragraph are **UTC**. A's first launch updated default Player.log at 2026-11-12 07:43:52 (`logs-A-menu.txt`); its driver independently reports the same default log path (`A-menu-progress.txt`). After A's full round/normal exit, Player.log was 17,419 bytes, modified 07:58:26.1051771, SHA-256 `07253ABF29B95BBCDC2D78B7CCDA56646E2327F7C4BEB292DC14CFEC15EC0328` (`after-A.txt`). During B's first launch, default Player.log was a new 2,883-byte file modified 07:59:55; Player-prev.log was 17,419 bytes modified 07:58:26 (`logs-B-menu.txt`). Thus B replaced A's active log and retained it only as the single previous log. Subsequent launches rotate that slot again. A's Monitor file remained in A, last modified 07:58:23; B's new Monitor file was already in B at 07:59:55. The full round exit records are A 23:58:23.929 Pacific and B 00:08:01.969 Pacific (`after-B.txt`).

This is the shared-file outcome anticipated by 03 §6.3: newly created configs need their own explicit `-logfile` to obtain stable per-config Unity logs. No workaround was applied in this bug-finding round; 1.7 remains unanswered.

## Bugs

### 1. Custom Game Config log pages cannot read their actual game runs

Severity: high. Repro: create own-saves sibling without extra launch arguments; launch it from RimArcho; enter a colony, save, load, exit; open Game log or run `archo log`. Both A and B show Player.log not found under their custom data directory, although the game wrote default LocalLow Player.log and a config-specific ArchoMonitor.log. Can it run's last-run section says the last run had no errors. It cannot substantiate that claim from the missing Unity log, and the game-log page does not provide the sibling's Monitor history either.

Expected: each config's own Unity and Monitor run is available and correctly attributed (03 §6.3; 06 §4). Evidence: `15-A-log-page.png`, `GC-A-log-page.png`, `GC-B-log-page.png`, `GC-A-needs-page.txt`, `GC-B-needs-page.txt`, `after-A.txt`, `after-B.txt`, the path/timestamp captures above. One root-cause finding covers shared log placement, missing logs and the unsupported clean last-run display.

### 2. Status/home hide the actual sibling owner of an external process

Severity: medium. Repro: select GC-B, start the same installation's RimWorldWin64.exe directly without savedatafolder, then inspect `archo status` and GUI Launch. Actual: status begins `env:GC-B` and says `game: running the list`; GUI shows GC-B as selected and an unnamed running game. Neither names Steam nor says unknown. Trying another launch correctly refuses with `Game Config Steam is running its list`, proving attribution is available but omitted from these displays.

Expected: name the actual config or explicitly say unknown; do not leave selected B appearing to own default-data Steam's process (03 §6.2, 09 §4). Evidence: `external-check.txt`, `external-home.txt`, `22-external-home.png`. Launch refusal passed. This finding does not allege that every write should be blocked: ordinary list saves were explicitly allowed and warned that the game reads them next start.

### 3. Sibling-removal confirmation contains untranslated English

Severity: low. Repro: in the Chinese GUI, remove a non-last Game Config. The confirmation's middle line is `other Game Configs use this game folder: its inventory and analysis stay`, surrounded by translated text. Expected: translated confirmation text in the selected language. Evidence: `25-remove-confirm.png`. Functional stopped-game removal passed independently.

## Saves and safety

Exactly two new saves were created; none overwritten:

- `C:\Users\Player\AppData\Roaming\RimArcho\userdata\GC-A\Saves\GC-Test-A-20261112.rws` — 20,718,840 bytes, saved 2026-11-12 07:53:11 UTC.
- `C:\Users\Player\AppData\Roaming\RimArcho\userdata\GC-B\Saves\GC-Test-B-20261112.rws` — 4,142,846 bytes, saved 2026-11-12 08:05:30 UTC.

Both saves were loaded in-game. They were kept for Saves-page verification until the final snapshot restore; any Steam Cloud consequences of these permitted new names are not undone by reverting. No existing save was renamed, deleted or overwritten. For both configs, Options was set to 14-day autosaves before colony play; the frozen driver additionally suppresses autosaving. After returning to menu, Options was restored to one day/count five before Quit to OS (`08-A-autosave14.png`, `14-A-restored.png`, `18-B-autosave14-setting.png`, `20-B-restored.png`, `after-B.txt`). B's changed Run in background option remained separate from A.

## Scope left and handoff

Not covered: 1.7; Proton/Linux; the whole release table; unknown/unreadable process arguments; stored legacy launch-argument overrides; interrupted test recovery; installation relocation/reattach; pre-existing destination overwrite protection; upgrade's complete ignores/history/pack/ID matrix; ordinary-player account permissions and accessibility narration. The Add dialog's full keyboard/reference-number matrix was not exercised. These remain pending in the corresponding release rows.

Host automation reused frozen VM scripts; separate task scripts and the pre-split build were staged outside product source. No application fixes, changelog or specification changes were made. Main-checkout changes to `ToDo.md` and `spikes/patterns/RESULT.txt` were left untouched. Only this report and relevant release-table status cells belong in the acceptance commit.

Final VM state: evidence exported; all games exited normally; VM shut down, restored to **RimArcho-Real-Vulkan**, and left powered off (`final-vm-state.txt`: `Total running VMs: 0`). No snapshots created or deleted.
