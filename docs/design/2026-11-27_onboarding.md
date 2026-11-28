# First-run guide: what is there, what RimStack does, a proposal (2026-11-27)

Status: proposal. Nothing is implemented. ToDo card `onboarding-rimstack`. RimStack is GPL-3.0, so I took ideas from it, not code.

Mock-ups (dark, 1280×800, en and zh) are in `D:\RimArcho-test\onboarding-proposal\png\`. They were built from the guide's and Import page's real DOM, taken from the memprobe host, with the app's own `style.css` and icons. The only additions are in `mockups\mock.css`, about 40 lines that use existing tokens. Screenshots of today's guide are in `D:\RimArcho-test\onboarding-proposal\current\`.

## 1. Today's first run

Today's first run was captured on the memprobe host: one window, sandboxed `%APPDATA%`/`%LOCALAPPDATA%`, offline, killed after each run. It used two fresh synthetic homes:
- `two`: Steam with two libraries, RimWorld 1.6 and 1.7, 25 Workshop Mods, 1 Mod in the game's `Mods`, 1 Mod in an unrelated folder, and a ModsConfig.xml with 13 enabled.
- `none`: no game.

### GUI

| # | Screen | Asks | Detects by itself |
| --- | --- | --- | --- |
| 0 | Install offer (only when run from a Workshop folder) | install folder | free space |
| 0 | Managed folder (only when the default is unusable) | download folder | default `%LOCALAPPDATA%\RimArcho` |
| 1 | **find the game** ([en](file:///D:/RimArcho-test/onboarding-proposal/current/two-en-1-find.png), [zh](file:///D:/RimArcho-test/onboarding-proposal/current/two-zh-1-find.png)) | which installs (checkboxes, all ticked), a **name** for each (prefilled `1.6`, `1.7`); or "add this folder"; or the "download a version" tab | Steam libraries, six fixed folders (`C:\Games\RimWorld`, `C:\GOG Games\RimWorld`, …), version, Steam/Proton/direct |
| 1b | download a version ([en](file:///D:/RimArcho-test/onboarding-proposal/current/two-en-2-download.png)) | "download SteamCMD" first, then branch, folder, optional account | the games folder on the first config's drive |
| 2 | **read the mods** ([en](file:///D:/RimArcho-test/onboarding-proposal/current/two-en-4-read.png)) | nothing; "next" | Workshop folder of the game's library; Archo Monitor installed and added silently |
| 3 | Import page, first-run view ([en](file:///D:/RimArcho-test/onboarding-proposal/current/two-en-5-firstlist.png)) | "use this list" or another way in; "skip" | the game's ModsConfig.xml (13 Mods) |

No game: [en](file:///D:/RimArcho-test/onboarding-proposal/current/none-en-1-find.png), [zh](file:///D:/RimArcho-test/onboarding-proposal/current/none-zh-1-find.png).

### TUI

The TUI has no guide. It says "No game added yet." and names `env add <name> --detected`, `--game <dir>` and the window program (16 §9). Only `q ? : E` work. I did not capture it: the CLI's detection is not sandboxed on the host, and the spec text is enough.

### Situations

| Situation | Today | Where a new player gets stuck |
| --- | --- | --- |
| No game found | One centred line and "add this folder"; the primary button sits disabled | The "download a version" tab is easy to miss. The line does not say where RimArcho looked. |
| Several installs | All ticked, one name field each | The name fields ask for a decision nobody needs yet (`1.6` is fine). |
| 1.6 and 1.7 side by side | Both added, sharing one user data folder (ModsConfig, saves, Mod settings) | Add Game Config offers "own saves" (`GameFound.Saves`), but the first run does not, so 1.6 saves show up in 1.7. |
| Non-Steam (GOG, a copied folder) | Found only at the six fixed paths; a folder named `RimWorld 1.7` is not | "add this folder", then the native picker |
| Proton (Linux) | Chip "Proton" | — |
| Existing ModsConfig | Becomes the first list ("use this list") | Fine, but it comes after a page change, the step bar disappears and the sidebar fills in at once, with a "can it run 1" badge. |
| Mods in several folders | The Workshop folder is added automatically; any other folder (RimSort's local folder, `D:\Mods`) is not looked for | Those Mods are missing from the library until Settings › Folders › add. The test home's `D-Mods\ExtraPack` was never found. |
| SteamCMD and an account | Asked only inside "download a version" | — (this is right) |
| Language | Never asked; follows the system | A player on an English Windows who wants Chinese must find Settings › Interface first. |
| Archo Monitor | Added to every list silently (`monitor.default` on); it shows as list entry 2 after the first run | An unknown Mod appears in the player's list, with nothing saying why. |
| Network | Automatic checks start 20 s after the window opens (16 §2.16.1 A01, A07): community data is about 50 MB | Never said or asked. |
| Other managers | The Import page's "look for RimSort, …" button | The player has to press it to learn that anything was found. |

Discrepancies between the docs and the code: none found. Step 2's row text repeats the name ("1.6 · 1.6: 30 mods found"). That is a cosmetic bug, not a doc mismatch.

## 2. RimStack's first launch

Source: `crates/app/src/ui/first_run.rs` and `first_run/steps.rs`. The user's five screenshots are in `todo-board/images/20261125-1141*.png`. It has a left rail (logo, five steps) and a right panel of grouped rows. Every step has "Back", "Skip to Start" and "Next".

| # | Step | Shows | Asks |
| --- | --- | --- | --- |
| 1 | Found | a language button row (9); Game: RimWorld version + path, Workshop folder, config/saves folder; Current mod list "N active, M inactive, kept as they are"; Programs: Git, SteamCMD (present or "not installed: …") | language |
| 2 | Community data | four datasets (rules DB, No Version Warning, Use This Instead, Steam Workshop DB), each "used once downloaded: …"; "Import from RimSort" when RimSort's rules exist | which to download (all ticked) |
| 3 | Sorting | "always: each Mod's own rules"; community rules; patch/def hints; "either way, Sort shows every move first" | two checkboxes |
| 4 | Appearance | a live 3-row preview; theme (4), text size (4), Mod rows (3), Workshop pages open in browser/Steam | four segmented choices |
| 5 | Start | three bullets about what happens next | "Start" |

What I take:
- **Language on the first screen.** It costs one row, and a player who reads only Chinese is helped before any other text.
- **"Found" as a confirmation, not a question.** Ticks and paths that the player only checks.
- **Showing the current Mod list as kept.** This reassures the player that nothing is lost.
- **Asking before downloading community data.** RimArcho downloads it silently.
- **"Import from RimSort" offered where it was found,** without a search button.

What RimArcho already does better:
- **It finds every install**, not just the first, and **downloads a version** when there is none. RimStack shows a bare "RimWorld" row.
- **Multi-game configs and copies.**
- **The scan with progress.** RimStack reads Mods in the background with nothing to watch.
- **Archo's figure.** The guide has a face. RimStack shows a logo.
- **The first-list page** has four real ways in, not a summary page.

What I leave out (see §4):
- Sorting choices.
- Appearance choices.
- The text-only "Start" page.
- A Programs block: Git is not used by RimArcho on first run, and SteamCMD is asked when needed.

## 3. Proposed flow

There are three steps, as today in practice: two in the guide plus the Import page. The step bar now runs through all three, and the sidebar stays empty until the guide ends. The changes:
- Step 1 confirms rather than asks.
- Language, extra Mod folders and own saves join step 1.
- The two silent defaults (network, Archo Monitor) are shown during the scan, which the player waits through anyway.

### Step 1 — "found"

Purpose: confirm what RimArcho will manage. Mock-ups:
- one game: [en](file:///D:/RimArcho-test/onboarding-proposal/png/1-found.en.png), [zh](file:///D:/RimArcho-test/onboarding-proposal/png/1-found.zh.png)
- 1.6 + 1.7: [en](file:///D:/RimArcho-test/onboarding-proposal/png/2-found-two.en.png), [zh](file:///D:/RimArcho-test/onboarding-proposal/png/2-found-two.zh.png)
- no game: [en](file:///D:/RimArcho-test/onboarding-proposal/png/3-none.en.png), [zh](file:///D:/RimArcho-test/onboarding-proposal/png/3-none.zh.png)

| Part | Detected (player only confirms) | Player decides | Default | Change later |
| --- | --- | --- | --- | --- |
| **Games**, one card each: big version, revision muted, chip Steam / Proton / started directly, path, "✓ N enabled" from its ModsConfig | Steam libraries, the six fixed folders, plus **new**: `RimWorld*` folders under `<drive>:\Games` and `<drive>:\GOG Games`, so `RimWorld 1.7` is found | untick a game | all ticked | Game Config page (add, rename, remove) |
| Game name | `1.6`, `1.7`, `1.6-2` (`view.GameAt`) | nothing: the field is gone | the version | Game Config page › rename |
| **Saves and Mod settings** (only on a second game whose user data folder another ticked game of a different version also uses) | the shared LocalLow folder | shared with 1.6 / its own | its own (open question 1) | Add Game Config's saves choice; folders in Settings › Folders |
| **Mod folders**, one row each with its icon and Mod count: Steam Workshop, the game's `Mods` folder, and **new**: folders another manager on this computer uses (RimSort's local folder, from the 07 §6.2 discovery) | paths and counts (a directory count, no scan) | untick a found folder (only the other-manager rows have a box; Workshop and the game's folder are always read) | ticked | Settings › Folders and storage › folders |
| **Language** segments: follow system (named), then each bundled locale | the system language | a language; applies in place | follow system | Settings › Interface › language |
| Footer: "add another folder" (today's "add this folder"), "download a version" (today's tab, now a quiet button), primary "add and read Mods" / "add 2 games and read Mods" | — | — | — | — |

**No game found** replaces the games and folders with two tiles: "choose its folder" (the folder with RimWorldWin64.exe) and "download a version" (from Valve with SteamCMD, with the size). Below them, one muted line says where RimArcho looked, and the language row stays. The download flow itself is unchanged: SteamCMD, then branch, folder and account.

This step cannot be skipped: nothing works without a game.

### Step 2 — "read the mods"

Purpose: read the Mods once, and use the wait for the two choices that today are made silently. Mock-up: [en](file:///D:/RimArcho-test/onboarding-proposal/png/4-read.en.png), [zh](file:///D:/RimArcho-test/onboarding-proposal/png/4-read.zh.png).

| Part | Detected | Player decides | Default | Change later |
| --- | --- | --- | --- | --- |
| Scan rows and bar (today's) | everything | nothing | — | — |
| "check for updates": RimArcho, Mods, tools and community data, about 50 MB once | — | on / off | on | Settings › About › automatically check for updates (`updates.auto`) |
| "Archo Monitor": a helper Mod added to your lists that names the Mod behind an error | — | on / off | on | Settings › Tools › game log › add Archo Monitor to lists (`monitor.default`) |
| "next" | enabled when the scan ends | — | — | — |

The two rows take effect before the first automatic cycle: A01 already waits 20 s, and (new) it also waits until the guide has passed step 2. Turning Monitor off before the scan finishes means it is not added to the first list.

### Step 3 — "first list"

Purpose: start from the player's real list. Mock-up: [en](file:///D:/RimArcho-test/onboarding-proposal/png/5-first-list.en.png), [zh](file:///D:/RimArcho-test/onboarding-proposal/png/5-first-list.zh.png).

The page is today's Import first-run view with three changes:
1. The step bar replaces the page head's left, so it stays the guide's step 3, and the sidebar stays empty until the guide ends.
2. The other-manager discovery runs when the page opens. The "look for…" button and the "Not found? Choose its .exe" row give way to one row per manager found, with what it holds ("RimSort found ✓ — 214 tags, 31 rules, 2 lists — import"). The "choose…" row comes back only when nothing was found.
3. Nothing else changes: "use this list" stays the primary button, and "skip" (esc) keeps the game's list.

When it ends, the player lands where they do today.

### TUI

The empty screen lists the detected games (version, source, path). `enter` adds them all and scans, which is the same as `env add --detected` for each. Language, Monitor and updates stay as `config set` lines in the help. The TUI is a second surface, so I kept it to that.

## 4. Deliberately left out

- **Appearance step** (theme, size, accent): the theme follows the system, the size is one `ctrl`+wheel away from any page, and the accent is cosmetic. Asking adds a step for choices nobody needs before seeing their Mods.
- **Sorting step**: RimArcho's sort always uses declarations plus community rules when present, and shows every move before applying it (08 §1). Opting out is a power-user setting.
- **Per-dataset community data**: the four datasets come together under `updates.auto`. Separate boxes would be four decisions about things the player has not met yet.
- **A summary "Start" page**: it is three sentences of text, and the first list page already is the start.
- **Naming games**: the version is a good name, and the Game Config page renames it.
- **Managed folder**: still taken silently and asked only when the default is unusable (unchanged).
- **SteamCMD account**: still asked only inside "download a version" (unchanged).
- **Copy a game folder**: needs an existing Game Config, so it is not a first-run way in.
- **A separate welcome page**: Archo and the step bar already greet the player.

## 5. Open questions for the user

1. **1.7 beside 1.6: own saves by default, or shared?** I propose "its own": RimArcho cannot undo a 1.6 save that a 1.7 run overwrote. Shared is what the game itself does.
2. **Other managers' Mod folders in step 1: ticked or not by default?** I propose ticked: they hold Mods the player already uses, and RimArcho only reads them.
3. **Should "check for updates" off also stop the community data download?** Today one switch covers both (A07). I propose keeping one switch.
4. **Wider non-Steam detection** (`<drive>:\Games\RimWorld*`, `<drive>:\GOG Games\RimWorld*`): is a directory listing of every fixed drive's two folders at start-up acceptable? It costs a few ms per drive.
5. **The TUI part**: worth doing now, or leave the TUI as it is?

## 6. Implementation size (rough)

| Part | Where | Size |
| --- | --- | --- |
| Step 1 layout: game cards without name fields, saves segments, Mod folder rows, language row, no-game tiles, footer buttons | `guide.js`, `style.css` | ~250 lines JS, ~40 CSS |
| Mod folder counts before the scan (directory count) and other managers' Mod folders (RimSort settings) | `view` / `app` (`GuideFind` extension), 07 §6.2 discovery | ~150 lines Go + tests |
| Wider non-Steam detection | `env/detect.go` | ~40 lines + tests |
| Step 2 toggles, applied before the first automatic cycle | `guide.js`, `app.AutomaticLoop` gate | ~60 lines |
| Step 3: step bar on Import's first-run view, discovery on open, found-manager rows | `exchange.js` | ~80 lines |
| Sidebar empty until the guide ends | `shell.js` / `navigation.js` | ~20 lines |
| Strings (~25) in en + 4 locales | `l10ndata/locale` | — |
| TUI empty screen (optional) | `surface/tui` | ~100 lines |
| Specs 16 §2.15, §2.18, 03 §3.2, 16 §9; CHANGELOG; release-vm-tests rows; preview mode for the new parts | docs | — |

Total: about 800–1,000 changed lines with tests, about two days for one session. The detection and discovery parts are separable and can land first.
