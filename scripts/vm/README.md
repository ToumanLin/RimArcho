# Testing in the Player-Test VM

`vm.ps1` builds RimArcho, copies it into the VMware test VM (`Win11-Player-Test`) and runs it there, all from the host with no clicks. `cdp.mjs` then drives the real RimArcho window over WebView2's DevTools protocol. The VM's own operating guide is `D:\VMs\PLAYER-TEST-AI-HANDOFF.md`; read it first. Its rules still apply: the test VM only, no revert or baseline change without the user's OK, evidence copied out before a revert, and one test task at a time.

Credentials come from `D:\VMs\Setup\Invoke-VMRun.ps1`, which reads them from DPAPI files. Never print or commit them.

## Once per VM (again after a snapshot revert)

Real-data rounds revert to the snapshot `RimArcho-Real-Vulkan`, a live one: Player signed in, Steam up, the game, the Workshop mods and software Vulkan installed. If Steam shows its sign-in window after a revert (Steam can reject the snapshot's saved session; it did on 2026-10-30), the VM's owner signs in; game rounds need Steam for the Workshop mods. Never type the account's password from a script.

```powershell
pwsh scripts/vm/vm.ps1 status    # running, Tools "running", guest IP
pwsh scripts/vm/vm.ps1 prep      # no display-off or sleep while idle (else screenshots go black)
pwsh scripts/vm/vm.ps1 forward   # guest 0.0.0.0:9223 -> 127.0.0.1:9222, plus a firewall rule
```

## Each change

```powershell
pwsh scripts/vm/vm.ps1 go        # build + push + launch (about 45 s)
```

The build matches `scripts/release.ps1` (`GUI=1`, Archo Monitor built from `monitor/` with `monitor/VERSION`), without `-s -w` and the icon. The Monitor is rebuilt only when its source changed; the `built …` line names the version bundled. `build -Monitor <mod folder>` bundles another Monitor instead (an older one, to test an update), with a warning that it is not `monitor/VERSION`. The font comes from the newest `dist/archo-*` release build, in this checkout or the main one.

## Smoke test (one step)

```powershell
pwsh scripts/vm/smoke.ps1                 # build, push, CLI checks, launch, window checks
pwsh scripts/vm/smoke.ps1 -NoBuild        # reuse what is in the guest (about 1 minute)
pwsh scripts/vm/smoke.ps1 -Shot page.png  # plus one screenshot of the window
```

It prints only `PASS` / `FAIL` lines and exits with the number of failures; nothing to read when it passes. Window checks: page keys, `alt+←/→`, the real mouse side buttons (sent as mouse input, not as page events), the menu background, no script errors. Add a check to `smoke.mjs` (window) or the guest script in `smoke.ps1` (CLI) instead of testing by hand.

## Game rounds (one step, no screenshots)

```powershell
pwsh scripts/vm/round.ps1                    # launch, new colony, play, save, load, play, main menu, delete the save, quit
pwsh scripts/vm/round.ps1 -Launch gui        # press the launch button in the RimArcho window instead
pwsh scripts/vm/round.ps1 -Play 300 -Coverage 0.3 -Difficulty Rough
pwsh scripts/vm/round.ps1 -Script round.txt  # any steps (below)
pwsh scripts/vm/round.ps1 -KeepMonitor       # run the Monitor installed in the guest, not the bundled one
pwsh scripts/vm/round.ps1 -ExtraMonitor "<folder>"  # also put the driver in another Monitor copy, e.g. a subscribed Workshop item the plan uses
pwsh scripts/vm/round.ps1 -Cleanup           # remove a driver an interrupted round left behind
```

This is the "actually enter the game" round of [pre-release VM tests](../../UserGuide/release-vm-tests.md). **Archo Drive** (`drive/`, test only, never shipped) plays it inside the game. It calls what the game's buttons call:

- each page's Next (`CanDoNext`, then `DoNext`) after "New colony";
- the save and load dialogs' file action;
- "quit to main menu" and "quit to OS".

So the Archo Monitor phase hooks fire as they do for a player. While playing it runs at speed 3, opens every main tab, then each inspect tab of three colonists. It answers message boxes with button A ("Load anyway" for a mods mismatch) and closes windows that pause the game after 2 s. Each such dialog or window is written to its log.

How the round runs:

1. `round.ps1` runs `archo monitor install`, as the window does when it opens, so the game gets the Monitor this build bundles (`-KeepMonitor` skips it). Then it copies `ArchoDrive.dll` into the installed Archo Monitor folders (the directory `archo monitor` names), so the mod list is the same as a player's. It puts the script next to it as `ArchoDrive.txt`. The order matters: a development Monitor folder whose files differ from the bundle is replaced, driver and all.
2. It launches with `archo launch`, or the home page button with `-Launch gui`.
3. It waits. The driver writes a line at least every 5 s, so silence for `-HangSeconds` means the game is stuck.
4. It removes the driver and collects the evidence into `D:\RimArcho-test\rounds\<run>`, a few KB:
   - `drive.log`, the driver's steps;
   - the session, phase and exit lines of ArchoMonitor.log, and Player.log's `[Archo Monitor]` lines;
   - `archo problems` and `archo log` after the run.
5. `round-check.mjs` checks every step, the phase order (`menu > new > play > save(<name>) > play > menu`, then exit), the save name, and that the game ran the Monitor RimArcho bundles (`monitor.txt` is `archo monitor` at the start).

A failed round kills the game and takes one guest screenshot (`failure.png`, `-NoShot` for none) for a person to look at. The driver acts only when `ArchoDrive.txt` is there and moves it away on start, so the next launch is not driven.

The guest's saves sync to the player's real Steam Cloud when the game exits. So while a script runs, autosave is off; the game's Prefs are not changed. The standard round deletes its save before it quits. When a round fails, `round.ps1` deletes the round's saves before it stops the game.

Round script, one step per line (`drive/src/Script.cs`):

```text
menu                                   # be at the main menu
new difficulty=Peaceful coverage=0.3   # also scenario=, storyteller=, permadeath=on
play 120 speed=3 tabs=on               # real seconds
save ArchoRound 1
load ArchoRound 1
wait 5
delete ArchoRound 1                    # the load dialog's delete button
quit                                   # last step only
```

Any step takes `timeout=<s>`. Parser tests: `dotnet run --project scripts/vm/drive/test`.

A script that does not end with `quit` leaves the game running when its last step is done, for a test that goes on by hand or with other tools. A script of just `menu` waits for the main menu. The driver sees the menu itself, so this works with a Monitor that writes no phase markers; round-check then fails only the phase lines. The driver stays loaded until the game quits, so autosave stays off. `pwsh scripts/vm/vm.ps1 quitgame` closes the game afterwards. If such a script saved, `round.ps1` leaves the save for you to delete before the game quits. The checks after the round are a person's job: what can it run and the game log page show. Use `cdp.mjs text` and `problems.txt` for those, not screenshots.

## Looking and acting

| What | Command |
| --- | --- |
| A page's text, as the player reads it | `node scripts/vm/cdp.mjs <ip>:9223 text "#main"` |
| The page, as the player sees it | `node scripts/vm/cdp.mjs <ip>:9223 shot page.png` |
| The whole guest screen | `pwsh scripts/vm/vm.ps1 shot screen.png` |
| Click a visible control (page event) | `node scripts/vm/cdp.mjs <ip>:9223 click "#nav [data-id=home]"` |
| Click it with the real mouse, through Windows | `node scripts/vm/input.mjs <ip> realclick "#nav [data-id=home]"` |
| Observe page state | `node scripts/vm/cdp.mjs <ip>:9223 eval "document.title"` |
| A key press | `node scripts/vm/cdp.mjs <ip>:9223 key "Shift+Digit2"` |
| A CLI command in the guest | `pwsh scripts/vm/vm.ps1 cli "env detect"` |
| Quit the game as its window's X does, then check the Monitor's exit line | `pwsh scripts/vm/vm.ps1 quitgame` |
| Any PowerShell script in the guest | `pwsh scripts/vm/vm.ps1 exec check.ps1` |

`<ip>` is the guest IP from `status`; it comes from DHCP, so don't hard-code it.

DevTools is on only because `launch` sets `RIMARCHO_DEVTOOLS_PORT`; players never set it, so nothing listens. CDP key presses and clicks go to the page, not to Windows. For anything that needs Windows input, use the input agent below. The game is not one of these: use `round.ps1`.

## Real mouse and keyboard

`input-agent.ps1` runs in the signed-in desktop session and sends real input with `SendInput`, so it reaches what CDP cannot: the native window frame (drag, edge resize, double-click, Aero Snap, `win+↑`, `alt+f4`), mouse-event drag and drop as a player does it, the clipboard, Windows Terminal (the TUI), native dialogs. `input.mjs` drives it from the host.

```
pwsh scripts/vm/vm.ps1 forward        # once per VM (again after a revert): adds port 9231 for the agent
pwsh scripts/vm/vm.ps1 input start    # (re)start the agent; prints "input agent ready"
pwsh scripts/vm/vm.ps1 input status   # running? its log
pwsh scripts/vm/vm.ps1 input stop
```

| What | Command |
| --- | --- |
| Screen size, DPI, cursor, locked or not | `node scripts/vm/input.mjs <ip> screen` |
| A process's windows: rect, client area, DPI, maximized, foreground | `node scripts/vm/input.mjs <ip> windows RimArcho` |
| Bring a window to the front | `node scripts/vm/input.mjs <ip> focus WindowsTerminal` |
| Click / drag at screen pixels | `node scripts/vm/input.mjs <ip> click 640 20` · `drag 300 10 900 10` |
| Click / drag a page element with the real mouse | `node scripts/vm/input.mjs <ip> realclick "<selector>" [left\|right] [ctrl]` · `realdrag "<from>" "<to>" [holdMs]` |
| Keys, to whatever window has focus | `node scripts/vm/input.mjs <ip> key ctrl+shift+k [repeat]` (also `win+up`, `alt+f4`, `KeyK`, `Digit2`) |
| Type text (Unicode, Chinese too; bypasses the IME) | `node scripts/vm/input.mjs <ip> type "plan new 测试 列表"` |
| Read / set the clipboard | `node scripts/vm/input.mjs <ip> clip` · `setclip <text>` |
| The text a window shows through UI Automation (Windows Terminal: the TUI screen) | `node scripts/vm/input.mjs <ip> text WindowsTerminal` |

In a test script, `connectReal(ip)` from `input.mjs` gives both connections and a calibration: `real.click(sel, { mods: ['ctrl'] })`, `real.drag(from, to, { holdMs, during })`, `real.input.key('esc')`, `real.cdp.evaluate(...)`. Calibration moves the real cursor to two points in RimArcho's window and reads where the page saw them, so page zoom, display scaling and the frameless frame need no special handling; call `real.recalibrate()` after moving or resizing the window. Read results with `cdp.mjs text` and `input.mjs text`, not screenshots.

- The agent accepts one host connection at a time, with the token from `input start` (`D:\RimArcho-test\transfer\rimarcho\<VM>\input-token.txt`, or `RIMARCHO_VM_INPUT_TOKEN`). When the connection drops, it releases every key and button still held.
- Input fails with "the guest screen is locked" while Windows shows the lock screen or a UAC prompt; sign in through the VMware window. A guest restart stops the agent: `input start` again.
- `type` sends characters, not key strokes, so it does not test the IME. Typing Chinese through the IME still needs a person.
- Display scaling for a 150% round: change it in the guest's Settings › Display, sign out and in, `input start`, recalibrate.

## Limits

- `launch` and `cdp.mjs` need a user signed in on the guest screen. Windows Update restarts the guest by itself and leaves it at the lock screen: `launch` then fails with `runProgramInGuest (exit -1)`, and a guest screenshot shows the clock. `status`, `exec`, `cli` and `push` still work. Sign in through the VMware window. Auto sign-in and pausing updates in the guest would remove this step; both are security settings, so the VM's owner makes that call.
- GUI tests run as **Player** (an administrator), because Player is the user logged in on the guest screen. `-StandardUser` only applies to `exec` and `cli`. Before a release, also do the first run as **the standard test account (`\u65b0\u73a9\u5bb6`, Unicode escapes)**: log in as \u65b0\u73a9\u5bb6 in the guest, then run `launch -StandardUser`.
- Files copied in by VMware Tools have no download mark, so SmartScreen warnings aren't tested.
- The guest has no GPU driver. Vulkan comes from Mesa lavapipe (software, `C:\Mesa\x64`, in the snapshot; a host copy is in `D:\RimArcho-test\mesa`), so RimWorld 1.7 starts, at a very low frame rate. Game performance is checked on a real machine.
- Leftovers in the guest: `C:\Users\Public\Documents\VM-Test`, `%APPDATA%\RimArcho` and `%LOCALAPPDATA%\RimArcho` of the user who ran it. Delete the last two for a first run again.


## Phase 1a local and CI scenarios

The [Phase 1a runbook](phase1a.md) covers the build, deterministic fixture, private real-data backup, scenarios 1–4, canonical comparisons, budget interpretation and VM hook. Results and test homes default to the OS temporary directory. A nonzero exit includes functional **or** performance failures; inspect both arrays in `result.json`.
