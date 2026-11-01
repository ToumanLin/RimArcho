# 06 · Interface · TUI (§9)

> Part of [06 · Interface](README.md). Section numbers run through all of 06 and stay unchanged when split across files. References still use `06 §x.y`.

## 9. TUI

### 9.1 Positioning and page mapping

TUI is the **keyboard version of the plan editor**: it opens the current plan's editor by default (§2.5), with other pages through the command panel and no sidebar. TUI and GUI share view models and keys (§3).

| GUI page | TUI |
| --- | --- |
| Plan editor | Main screen (implemented). Open, create, rename, delete lists: `P` or command panel; top bar shows list <name>. Like GUI, **opening is not switching** (2026-10-31, 04 §1.3): `P`, command-panel lists, and editor `tab` only open a list; the current (launch) list stays unchanged, and the top bar adds launching X. Only set as current in `P`, save (`w`) in its editor, or `L` makes it current. `T` matches GUI: put all back in Staging, stage everything Enabled elsewhere (2026-10-31). `H` history picker, `A` recheck files, narrow-screen `4` full-screen details match GUI (since 2026-10-31, §2.5, §3.5) |
| can it run | `shift+2` (`@`) or command panel (implemented, secondary screen, `esc` returns to editor); one column. Page-head conclusion, `F` automatic fix, `A` recheck, `l` choose log, item `c`, and `u` undo match GUI (§2.4) |
| Mod inventory > subscriptions and updates | `c` or command panel; sources, waiting for you to update, already updated are simple lists (GUI names, §2.9); `1` / `2` / `3` jump areas, `C` lands on one Mod's change history; history actions unchanged. Actions (implemented 2026-10-30): `K` checks all sources with Mods; `enter` on a source checks only that source; `u` updates the cursor's updatable item, `U` all displayed items, both show `--dry-run` text then `enter` confirms, refused while game runs; `Del` removes subscription after preview (Steam-client-managed Workshop items unsubscribe in Mod inventory); `p` on updatable row first locks this version (press twice), `ctrl+w` opens source page; `+` or direct paste opens add subscription. All queue as background tasks (§2.16), status says queued, then reports result. Download-page updates require choosing the newly downloaded file: TUI directs to the window or `archo subscribe update --from`. Account navigation (LoversLab login) stays in GUI |
| Mod inventory > add subscription | subscriptions and updates `+`, `tab`, paste, or command panel add subscription (implemented 2026-10-30). Same paste box and source detection (`subscribe.Detect`) as GUI; typing starts immediately, so `q`, `?`, etc. are input. Once recognized, show the one source-specific decision (`↑↓`: Git default branch / branch / tag / commit; last three open name field, `tab` switches name / paste; Workshop via Steam subscription / SteamCMD download) and background-generated `--dry-run` text; `enter` subscribes through background tasks. `ctrl+u` clears, `esc` leaves name field, then clears, then returns to subscriptions and updates; outside name field `tab` goes to search files, `shift+tab` back to subscriptions and updates. Download pages work in TUI only with one unique Git repository (subscribe with Git <repository>); zip / folder or LoversLab site file selection directs to window or `archo subscribe page <url> --from <zip\|folder>`. No source cards or recently added |
| Mod inventory > all | Command panel mod library, `shift+6` (`^`): same interface as GUI (§2.9): filter bar, expandable Lineage rows, copy details at right when wide, keys and context menu per §3.5, §8.5 |
| Mod inventory > search files | Command panel search files, add subscription `tab` (implemented 2026-10-31, TUI bullet in §2.9). Same `view.FileSearch`; no terminal copy, results cannot open full-screen details |
| Editor sorting rules | Secondary screen: editor `r`, command panel sorting rules (implemented 2026-10-31, TUI bullet in §2.5) |
| launch | `shift+1` (`!`), editor `L`, command panel launch game (implemented 2026-10-31, `surface/tui/home.go`). Same data as GUI (`view.BuildHomeFromState`, current list only regardless of editor). Essentials only: current list name, game and full version, Mod count and status (ready / N problems breaks the game / not checked yet), last game and errors, likeliest problem (worth knowing: … shift+2, `Home.Launch`), then ▶ launch game L, with save N changes w below if unsaved. No 01 / 02 areas, background, or current list since last played line; switch lists with `P`. Keys: `L` / `enter` launch, `w` save only, `P` lists, `@` can it run, `esc` editor. Editor `L` first comes here like GUI (making its open list current if needed); another `L` here launches. Status appends worth knowing: … after launching game (`view.LaunchHint`). Entering with `shift+1` while editing another list: `L` launches current list, editor follows it |
| game log | `l` chooses log in can it run; command panel game log does the same; log item `enter` opens technical page (implemented). **Game log page pending** (error list, raw log, companion log area, §2.10), excluded this round (decided 2026-10-31) |
| packs, settings | Command panel, text panels with commands (implemented) |
| background tasks | Command panel background tasks: read-only text panel (2026-10-31), groups running, queued, today, yesterday, earlier; one row per task: status, action, object, result (failure reason, `view.TaskStatus`). Stop, retry, forget in the window; completion still reports in status |
| Environments | `E` switches environment (implemented); directories / launch settings use CLI `env set` |
| update RimArcho (2026-10-31, 07 §5.1) | Command panel update RimArcho or `:update`: text panel matching GUI settings RimArcho row (version, last check, release notes / page if newer, installation confirmation: version, size, replace program files and restart, data stays, wait for other RimArcho processes). Without update `enter` checks then reopens panel; with update `enter` installs: background download, verification, extraction; status reports phases (updating RimArcho: waiting for 1 other RimArcho to close). Open panel during install: `enter` stops, changing nothing before replacement. After replacement and verifying the new version runs, TUI exits and opens new version in same terminal with same arguments (`cli.restartTUI`, old process waits for it). `:update --check` only checks, writes output in panel. An automatic check finding a newer version while TUI is open gives one quiet status notification |
| Side / top navigation (`ctrl+b`) | No sidebar, not applicable |
| Create a pack, play review, worth playing | No pages; corresponding CLI commands run in `:` (§9.4). Packs and texture optimization remain text panels with commands (decided 2026-10-31, no pages this round) |
| Archo panel | Not supported yet (09 §6) |

- Launch uses §2.3 red / yellow wording; severe lists required and available amounts separately before launch, cancel selected by default, explicit launch anyway required. Memory levels: §2.4.
- Game status at **status line right** (no GUI sidebar equivalent since 2026-10-30; GUI reads new logs itself, §2.1): `● game running since 14:05`, with current errors as amber count; new Player.log after exit: `● read the new log  shift+2` in accent; otherwise blank. Status sentence stays left when both appear. New errors update top can it run N without dialog (`view.LiveTracker.Update`, interval §2.16.1 G01; text `view.LiveText`).
- Can it run log items use §2.4 phase groups: indented muted headings, no cursor or count; arrows always land on actual errors. Unmarked logs have no headings; viewing still opens existing technical page.
- Context menu keys: Bubble Tea v1.3.10 does not recognize xterm `shift+F10` (`CSI 21;2 ~`); Windows console sends plain F10, xterm-like terminals send F16 for the menu key. Therefore F10, `shift+F10`, F16 all open the menu; right-click opens at pointer. Filter bar spans full TUI width (Disabled column cannot fit filters). Mod inventory copy details have notes, actions, status, fields but no description (**pending**); no images (§9.3); bottom-level inventory `esc` returns to editor (no sidebar).
- Jump pages with `shift+1`–`8` (`!@#$%^&*` in US terminal layout; `shift+7` texture optimization is a command text panel); subpages `tab` / `shift+tab` (§3.5); `g` is tags, not top (decided 2026-10-22). The proposed `g`-prefix navigation was not adopted.
- `archo` and `archo tui` open in current terminal (§9.2). No `#id` in interface: TUI uses plain wording, errors through `view.Humanize`. Only technical pages and `:` output retain CLI text / references. Technical-page `enter` follows the selected line's first reference; unprefixed instance references in printed commands (05 §3.1 rule 11) also follow if known to current index (`lineRef`).

### 9.2 Which terminal runs it

Since 2026-10-30 TUI bundles no terminal (00 §6 D24): `archo` without arguments and `archo tui` open in their launching terminal. Double-clicking `archo.exe` lets Windows open its default terminal (Windows Terminal on Windows 11, console window on older systems); TUI runs there and the window closes on exit. Old shortcut `archo --here` remains accepted, same as `archo` (`cli.globalFlags`).

- Player terminal controls fonts; RimArcho distributes none.
- Colors: termenv cannot query under ConPTY and falls back to no color, so `SetColors` forces 24-bit color when the terminal declares 24-bit color support: `COLORTERM=truecolor|24bit` or `WT_SESSION` (Windows Terminal). Others use termenv detection.
- Old data-directory `wezterm\`, generated `wezterm.lua`, and `logs\wezterm-start.log` are unused and not automatically deleted (decided 2026-10-30); uninstall removes them with data, or players may delete them. Settings `terminal.host`, `wezterm.path`, `ui.previews`, `ui.preview_dwell_ms` are removed; old database values unread.

### 9.3 Icons, links, images

RimArcho cannot control terminal fonts, so:
- Default icons use §7 Unicode fallbacks. `ui.icons compat|nerd`: compat default; select nerd if terminal already uses Nerd Font. Old database auto means compat (unknown old values use defaults, `cmds.Setting`).
- No images: no thumbnails or reserved space; names start flush left; description images become text placeholders (§4.4).
- Links are not clickable; append muted domain text.

### 9.4 Command line (`:`)

Advanced input: runs one archo command just like terminal `archo …`, same `cmds.Commands` and text output (05 §4). Decided 2026-10-30: **all commands run directly, without extra confirmation**, like CLI; add `--dry-run` yourself to preview file-changing commands that support it. Code: `internal/surface/tui/cmdline.go`.

**Open / close.** `:` in editor, can it run, subscriptions and updates, Mod inventory; or command panel run archo command. `:` in text panels, including prior output, closes the panel then opens input, allowing consecutive commands. Input occupies bottom line after `❯ archo `, help above, up to 8 candidate rows above help. `esc` closes candidates first, then input. Unrun text remains for next `:` (§3.1 rule 8); execution closes / clears input.

**Editing.** `←→` cursor, `home` / `end` line ends, `ctrl+←` / `ctrl+→` words, `backspace` / `delete` characters, `ctrl+w` previous word, `ctrl+u` clear; direct paste. Leading `archo` optional. `"with spaces"`, `'with spaces'` each form one argument (copy printed `archo pack launch "name"` directly); backslashes preserved for Windows paths. `#` is not comment: `#a3f9` / `a3f9` both work (05 §3.3).

**Help line.** Recognized command: usage and sentence (`cmds.Command.Usage`, `Summary`, English like `archo help`); otherwise keys `tab complete  ↑↓ history  enter run  esc close`. Last Enter's usage error appears red here until input changes.

**Completion (`tab`).** Completes cursor word:
- Command names: prefix first, then substring (`remove` finds all removal commands). Candidates show usage / description.
- Leading `-`: command flags (`--dry-run`, etc.) and descriptions.
- Positional arguments follow usage placeholders: Mods (`<ref>`, `<ref…>`, `<mod>`, `<A>`, `<B>`) match current game's names, aliases, package IDs, references, insert unprefixed reference (05 §3.1 rule 11), show name / reference. Empty word's first candidate is cursor Mod. Environment names (`env use|set|remove`), list names (`plan use|rename|remove|cost|versions|follow|unfollow|sync`, `test run`), pack names (`pack launch|export|remove|check|stop`), setting keys (`config get|set`), allowed values after `config set <key>`, literal usage choices (`same|copy|unrelated`, `yes|no`), tool names for `deps install|remove`. No path completion.
- Single candidate inserts directly plus space. Multiple list above input and insert first; `tab` / `↓` next, `shift+tab` / `↑` previous, updating input. `enter` accepts without running; continued typing also accepts; `esc` closes and restores original word. No candidates: nothing to complete in help.

**History.** Without candidate list, `↑` recalls prior commands newest first; `↓` moves back, past newest returns draft. Latest 50 persisted as JSON array in database meta `tui.cmdline.history`, surviving TUI restart; duplicates keep only latest; unexecuted usage errors excluded.

**Run (`enter`).**
- Background execution (§3.1 rule 9), own database connection (`App.ReadSession`), interface usable; status running archo …. One command at a time; Enter while running reports previous command is still running in help.
- Reread list afterward. Single output line in status with success / failure (§7); multiline opens panel titled `archo …`; failure also reports command failed in status. Panel uses original CLI text / references (§9.1); `↑↓` scroll, `esc` return.
- **Usage errors** (unknown command, wrong argument count, unknown flag, exit 2): reopen input unchanged, error in help; correct then Enter. `mcp`, `gui` report these errors themselves, requiring standalone command lines.
- Delegated: `scan` in scan subprocess like `R`, `analyze` in analysis subprocess (03 §3), never surface process. `log --follow` refused as endless; help explains TUI status already follows game, run in terminal for scrolling log.
- `--env <name>` applies only to this command, like CLI.
- After execution: `config set ui.lang` / `ui.icons` immediate; `env use`, `plan use` switch environment / list like `E`, `P`; after `uninstall --yes`, only exit remains, like uninstall page (§2.14).

### 9.5 Differences from GUI

Compared page by page 2026-10-31 (§2.x vs §9.1, keys vs §3.5). Names, subpage order, key hints, command-panel commands, opening / switching lists, `L` first visiting launch, and `T` now match. Remaining differences:

**Intentional** (terminal limits or easier TUI use):
- No sidebar; other pages secondary, bottom-level `esc` returns editor; `alt+←` / `alt+→` inapplicable (§3.1 rule 11).
- Can it run single column, no before you play / last time columns (§2.4).
- Editor `h` `l` arrows (§3.3), `i` technical info, not import / export (§3.5).
- Launch single column with current list / launch only, no other lists, packs, games (§9.1 table).
- No click-to-copy / `ctrl+c` copy: terminal selects text (§3.1 rules 12, 13).
- Subscriptions / updates three simple lists, no table with details (§2.9).

**Not yet aligned** (pending implementation or decision):
- Mod inventory > all `enter` opens copy details (notes, actions, status, fields only), not GUI full-screen details: TUI full-screen details follow editor rows (§9.1). search files results also cannot open them.
- Outside-page rows at can it run bottom are text only, no key to inventory / texture optimization.
- Game log page pending (§9.1 table); packs, texture optimization, background tasks text panels only.
- Mod inventory > all only default columns (size, status, source, used in), `s` sort; no GUI column-menu show / hide.
