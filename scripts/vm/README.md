# Testing in the Player-Test VM

`vm.ps1` builds RimArcho, copies it into the VMware test VM (`Win11-Player-Test`) and runs it there, all from the host with no clicks. `cdp.mjs` then drives the real RimArcho window over WebView2's DevTools protocol. The VM's own operating guide is `D:\VMs\PLAYER-TEST-AI-HANDOFF.md`; read it first. Its rules still apply: the test VM only, no revert or baseline change without the user's OK, evidence copied out before a revert, and one test task at a time.

Credentials come from `D:\VMs\Setup\Invoke-VMRun.ps1`, which reads them from DPAPI files. Never print or commit them.

## Once per VM (again after a snapshot revert)

```powershell
pwsh scripts/vm/vm.ps1 status    # running, Tools "running", guest IP
pwsh scripts/vm/vm.ps1 prep      # no display-off or sleep while idle (else screenshots go black)
pwsh scripts/vm/vm.ps1 forward   # guest 0.0.0.0:9223 -> 127.0.0.1:9222, plus a firewall rule
```

## Each change

```powershell
pwsh scripts/vm/vm.ps1 go        # build + push + launch (about 45 s)
```

The build matches `scripts/release.ps1` (`GUI=1`, fonts and Archo Monitor copied from the newest `dist/archo-*` folder), without `-s -w` and the icon.

## Smoke test (one step)

```powershell
pwsh scripts/vm/smoke.ps1                 # build, push, CLI checks, launch, window checks
pwsh scripts/vm/smoke.ps1 -NoBuild        # reuse what is in the guest (about 1 minute)
pwsh scripts/vm/smoke.ps1 -Shot page.png  # plus one screenshot of the window
```

It prints only `PASS` / `FAIL` lines and exits with the number of failures; nothing to read when it passes. Window checks: page keys, `alt+←/→`, the real mouse side buttons (sent as mouse input, not as page events), the menu background, no script errors. Add a check to `smoke.mjs` (window) or the guest script in `smoke.ps1` (CLI) instead of testing by hand.

## Looking and acting

| What | Command |
| --- | --- |
| The page, as the player sees it | `node scripts/vm/cdp.mjs <ip>:9223 shot page.png` |
| The whole guest screen | `pwsh scripts/vm/vm.ps1 shot screen.png` |
| Run JS in the page, read state, click | `node scripts/vm/cdp.mjs <ip>:9223 eval "document.querySelector('.x').click(), 1"` |
| A key press | `node scripts/vm/cdp.mjs <ip>:9223 key "Shift+Digit2"` |
| A CLI command in the guest | `pwsh scripts/vm/vm.ps1 cli "env detect"` |
| Any PowerShell script in the guest | `pwsh scripts/vm/vm.ps1 exec check.ps1` |

`<ip>` is the guest IP from `status`; it comes from DHCP, so don't hard-code it.

DevTools is on only because `launch` sets `RIMARCHO_DEVTOOLS_PORT`; players never set it, so nothing listens. CDP key presses and clicks go to the page, not to Windows: native dialogs (the folder picker) and anything outside the WebView need the VMware window and computer use.

## Limits

- `launch` and `cdp.mjs` need a user signed in on the guest screen. Windows Update restarts the guest by itself and leaves it at the lock screen: `launch` then fails with `runProgramInGuest (exit -1)`, and a guest screenshot shows the clock. `status`, `exec`, `cli` and `push` still work. Sign in through the VMware window. Auto sign-in and pausing updates in the guest would remove this step; both are security settings, so the VM's owner makes that call.
- GUI tests run as **Player** (an administrator), because Player is the user logged in on the guest screen. `-StandardUser` only applies to `exec` and `cli`. Before a release, also do the first run as **新玩家**: log in as 新玩家 in the guest, then run `launch -StandardUser`.
- Files copied in by VMware Tools have no download mark, so SmartScreen warnings aren't tested.
- The guest has no GPU driver; game performance is checked on a real machine.
- Leftovers in the guest: `C:\Users\Public\Documents\VM-Test`, `%APPDATA%\RimArcho` and `%LOCALAPPDATA%\RimArcho` of the user who ran it. Delete the last two for a first run again.
