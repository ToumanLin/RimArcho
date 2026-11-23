# Game Configs VM fix recheck — 2026-11-12

Result: **all five requested application checks PASS** on Windows RimWorld 1.6. No new application bugs found. Cleanup had one procedural deviation, recorded below; this is not an unqualified claim of compliance with every VM rule.

## Setup and evidence

- Tested unchanged `dev/1.1` commit `c4fee607120a1c74190bc995d60ad9746f4aadcc`. Report branch: `vm/game-configs-recheck`; worktree: `.claude/worktrees/game-configs-recheck`. No app code or VM tooling changed.
- Followed the [previous round](2026-11-12_game-configs-vm-round.md), `scripts/vm/README.md`, and `D:\VMs\PLAYER-TEST-AI-HANDOFF.md`. VM: `Win11-Player-Test`, `D:\VMs\Player-Test\Win11-Player-Test.vmx`; restored `RimArcho-Real-Vulkan`, then started `gui`. Tools running; guest IP `192.168.86.130`; signed-in desktop account Player.
- Steam reconnected successfully at 00:57:29 Pacific. No sign-in prompt appeared; no credentials entered, SteamCMD, subscription/account changes, or snapshot creation/deletion.
- Old schema-18 `archo.db*` moved into the guest's `schema18-aside-gameconfig-20261112` folder. Ran `env add Steam --detected`, then waited for `scan:1` to finish before opening the GUI: 6m58.58s, 1,301 live copies (six official, 1,293 Workshop, two Monitor copies), 1,300 Lineages. The scan read 15.1 GB of existing content; it did not transfer that content to the host.
- A is named `Steam`, uses default LocalLow data. B is `GC-Recheck-B`, own saves; C is `GC-Recheck-C`, own saves and never launched. All are siblings of the same installation. Test list: Harmony, bundled Archo Monitor 0.11.1, Core and five DLCs, HugsLib; nine entries. Every driver invocation supplied the subscribed Monitor folder `3835688284` through `-ExtraMonitor`.
- Detection says `1.6.4871 rev590`; the running game says `1.6.4871 rev591`, as in the previous round. No 1.7 test.
- Built with frozen `vm.ps1 build`. CLI SHA-256: `BAFA026377529277653392A8F03F5050D5354C33F0A4AA358D8520C218CF1378`; GUI: `D74F5D8FA8BB95A78122B08DE9EA8F68D91310BD3F95D516F1D0025CAE31BE4B`. `build-hashes.txt` records paths.
- Evidence root, including every screenshot below: `D:\RimArcho-test\gameconfig-vm-recheck-20261112\`. Exported screenshots, page text, CLI output, driver results and logs total about 42.3 MB. The build transfer archive was 60.5 MB. No game/Mod downloads or bulk content exports. Peak growth of the VM's sparse disk was not measured, so the evidence does not establish a strict total disk-growth bound of 5 GB.
- VM work ran approximately 00:57–01:36 Pacific, within two hours. Host main-checkout changes to `ToDo.md` and `spikes/patterns/RESULT.txt` were left untouched. No extra build worktrees were created.

## Requested checks

| Step | Result | Evidence |
| --- | --- | --- |
| 1. B launch arguments, full colony/save/load, isolated log and rotation | PASS | `B-dry-run.txt` includes `-savedatafolder=<B>` and `-logfile <B>\Player.log`; `A-dry-run.txt` has neither custom-data nor logfile argument. `B-round-final-console.txt` and `round-B/` pass all seven steps and Monitor/Player phase checks: menu, colony, 120 s play, save, load, 60 s play, menu. Played 6,465 then 3,161 ticks; final menu transition began at tick 9,850, below 60,000. Exited via Quit to OS after restoring autosave. B wrote its own Player.log; default log retained its original hash and timestamp. Repeat menu launch/normal exit passed (`B-repeat-console.txt`, `round-B-repeat/`); Player-prev.log exactly matches the colony run's Player.log. Metadata below. |
| 2. A default-data menu launch and exit | PASS | `A-menu-console.txt`, `round-A/`, `13-A-menu.png`, `14-A-options.png`, `after-A.txt`. A reached the menu with Monitor, exited via Quit to OS; default Player.log updated. B's Player.log, Player-prev.log and ArchoMonitor.log hashes and timestamps remained identical to `after-B-repeat.txt`. No A colony or save. |
| 3. B Game log/Can it run; unlaunched C missing-log verdict | PASS | Checked B immediately after the full colony run, before its repeat launch. `B-log-page.png/.txt` shows the B Player/Monitor pair; `B-player-tab.png/.txt` captures Player view. `B-archo-log.txt` identifies B's full Player.log path, 369 lines and menu/new/play/save/load/menu phases; its three clusters are informational. `B-problems.txt` has zero errors/warnings, consistent with `B-needs-page.png/.txt` saying no last-run errors and naming the loaded new save. `C-needs-page.png/.txt` explicitly says the last-run log was not found and displays C's full Player.log path. C was never launched; `final-guest.txt` confirms that file never existed. Nothing was deleted/renamed to manufacture the missing-log case. |
| 4. External default-data game while B selected | PASS | `external-start.txt` records direct RimWorldWin64.exe command with no arguments, PID 2428. `external-status.txt` names Game Config Steam as running plan:default while selected context remains B. `external-home.png/.txt` names Steam in GUI home. `external-tui.txt`, `external-tui-home.txt`, `15-external-tui.png`, `16-external-tui-home.png` name Steam in native TUI status and Start. None attributes the process to B. `17-external-menu.png`: main menu only; normal Quit to OS; no colony/save. |
| 5. Remove C in Chinese GUI | PASS | `18-remove-C-confirm.png`, `remove-C-dialog.txt`: all warning prose is translated, including the shared-installation inventory/analysis line that was English in bug 3. Only product names, the chosen config identifier and key labels remain Latin. Confirmed removal; `after-remove-env-list.txt` contains only B and Steam. Japanese/last-sibling variants were outside this recheck. |

## Log metadata

All timestamps in this table are UTC. B root: `C:\Users\Player\AppData\Roaming\RimArcho\userdata\GC-Recheck-B`. Default root: `C:\Users\Player\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`.

| Observation | File | Bytes | Modified UTC | SHA-256 |
| --- | --- | ---: | --- | --- |
| Before B, after full B, and after B repeat | Default Player.log | 558,522 | 2026-10-25 02:51:19.9277947 | `B7D6F292CB8F93BC65D2333F056B4C0C0485112856A6C35FD028F3905017EF27` |
| After B colony and normal exit | B Player.log | 17,675 | 2026-11-12 09:25:11.6437838 | `E32B3B30A542FF31BA101B8FAF1D0814BE382DD3DC4F405EEFCED8FCE3383000` |
| After B repeat, unchanged by A and external run | B Player-prev.log | 17,675 | 2026-11-12 09:25:11.6437838 | `E32B3B30A542FF31BA101B8FAF1D0814BE382DD3DC4F405EEFCED8FCE3383000` |
| After B repeat, unchanged by A and external run | B Player.log | 13,328 | 2026-11-12 09:28:42.7245818 | `261A69E8923AEA0CFB4F038548E0DF9180DE3ABD5437A15061F1E6EC674CED27` |
| After B repeat, unchanged by A and external run | B ArchoMonitor.log | 2,995 | 2026-11-12 09:28:39.7791503 | `F65001BE13508895229003DD0D971DA118DB5BABD08F390936D7F22A633B6402` |
| After A normal exit | Default Player.log | 13,419 | 2026-11-12 09:32:14.8622222 | `5E26B4AE66FA6D02D0449A57D3EA7905FB6CD9B1FBA68519C8D32B2517878700` |

Source captures: `logs-before-B.txt`, `after-B-round.txt`, `after-B-repeat.txt`, `after-A.txt`, `final-logs.txt`. Final B files exported as `B-Player.log`, `B-Player-prev.log`, `B-ArchoMonitor.log`. Full colony Monitor phase evidence is in `round-B/`, before the repeat run replaced its companion file.

## Bugs and harness limitations

No new application bugs found; previous bugs 1–3 pass the requested subset. Remaining release-row coverage stays pending: packs and 1.7, unreadable/unknown-data processes, Japanese and last-sibling removal.

Setup needed retries: an incorrect Monitor package selector, followed by ambiguous installed versions, was corrected to bundled instance `4ts4`. The first menu launch preceded that correction and had six entries; it exited through the menu without colony play. A subsequent Monitor menu run passed. A Prefs script initially used the wrong XML root; that attempted full-round launch was stopped normally with `quitgame` before colony play and without a save, then its driver was cleaned up. The corrected closed-game XML edit used `//autosaveIntervalDays`, verified 14 days/count five, and the final full round passed. These attempts are not counted as product failures. The frozen tooling was not edited.

The game itself still displays the default log-folder label in Options even with explicit logfile redirection; actual file metadata, CLI paths and matched Monitor phases establish where the output went.

## Saves and VM cleanup

Exactly one new save was created and loaded; none overwritten, renamed or deleted:

- `C:\Users\Player\AppData\Roaming\RimArcho\userdata\GC-Recheck-B\Saves\GC-Recheck-B-20261112.rws` — 8,232,437 bytes, modified 2026-11-12 09:21:39 UTC.

B's autosave interval was verified at 14 days before the full colony round (`autosave14.txt`); the driver additionally suppresses autosaves. At the menu, restored one day/count five before Quit to OS (`11-B-restored.png`, `after-B-round.txt`). A also had one day/count five (`14-A-options.png`). Final enumeration found only the named new save; no game processes remained (`final-guest.txt`). The save stayed in the VM until restore; snapshot restore does not undo any Steam Cloud synchronization of the permitted new name.

Evidence was exported before cleanup. Requested normal guest shutdown, then restored `RimArcho-Real-Vulkan`. **Procedural deviation:** the immediate pre-restore `vmrun list` still listed Win11-Player-Test; shutdown completion was not verified before issuing restore. Restore succeeded and the final list excludes Win11-Player-Test (`final-vm-state.txt`): it is restored and powered off. Eval-4 was also running and was left untouched. No snapshot was created/deleted. Guest C: free space before cleanup: 62,760,484,864 bytes.
