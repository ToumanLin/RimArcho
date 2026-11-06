# Changelog

All notable changes to RimArcho. Versions follow semantic versioning; the MVP is 0.5.0.

## 1.0.1 (unreleased)

- Updates page: Update all with many Mods opens its confirmation at once. Before, it built a preview for each Mod in turn, about 10 seconds for a long list. Confirming now queues one task for the Git and SteamCMD Mods, downloaded by SteamCMD in shared batches, and one for Steam client Mods, which Steam gets in one request. Before, every Mod was its own task and its own SteamCMD run. The TUI's update all (`U`) does the same.
- The first-run guide no longer shows `✗ no environment configured` (in English) on the status line before a game is added.
- Can it run: after Sort, or after choosing which Mod of an incompatible pair to disable, the cards under errors from the last run no longer each turn into a ✓ row with its own undo.
- Add subscription: the preview reads in the interface language (it was English in Chinese).
- List editor: dragging a Mod that is not selected moves that Mod. Before, when other rows were selected, they moved instead of the one under the pointer.
- List editor: Space or Enter on a focused button (Delete, Rename and the others in the list's header) presses that button. Before, Space disabled the Mod under the cursor and Enter opened its details.
- Cancelling a folder or file picker no longer shows `✗ cancelled by user`.
- Copy buttons beside web addresses in text (the release notes in Settings › About) no longer copy the closing `)` of a Markdown link or a trailing full stop, which made the copied link a dead page.
- A failed move of a managed folder now says to set its new location with `archo folder set`, not `SetDirectory`.
- Game log: a misspelled class of a patch operation (`<Operation Class="PatchOperationConditionl">`, also with a namespace) now names the Mod whose patch has it, and the error is an XML load error, shown by default. Before, `Could not find type named … from node …` was filed as other (hidden) with no Mod identified.
- Game log page: after switching the interface language in place, the note beside the title ("its errors are in needs attention") also changes language; it also follows the log's state when the page redraws.
- Can it load: a Mod unsubscribed or deleted while it was still in the list now says what the save loses without it (`loading without it loses 5 colonists of race Ratkin …`). Before, it said nothing in the save needed it.
- Mod settings: an empty stored value (`<PinnedMods />`) is read only and shows as empty. It can be an empty list as well as empty text, and `archo mod-settings set` wrote text into XML Extensions' list, which then failed to load in the game.
- Background tasks in Chinese: the RimArcho update check's result "new version available" is translated.
- `archo problems`: when two copies of a Mod are installed and neither is in the list, the finding no longer suggests `archo swap`, which failed with "not enabled", and no longer says both copies are ignored because another copy wins.
- RimArcho's own update and uninstall now know `README.zh-CN.md` and `skills/` (agent skills, such as `rimworld-probe` for checking Mod behaviour in the game). Release zips carry them from 1.0.2 on. RimArcho 1.0.0 refuses a zip with files it does not know, so a 1.0.0 install should update to 1.0.1 before 1.0.2 is out; after that, download 1.0.2 by hand.
- `rimworld-probe` skill: probes also build and run on RimWorld 1.7, and the game it starts no longer attaches to a running Steam client.

- Switching the interface language now also changes "Background tasks" at the foot of the sidebar, and the search hints of the filter bars (list editor, Mod library, Background tasks, file search). Before, they kept the old language until RimArcho restarted.

- Game log: in a 1.7 log, an XML Extensions patch failure (`[Mod - Start of stack trace]` … `Source file:`) is one `patch-failed` error per failure, `[Mod] Verse.PatchOperationAdd(xpath=…): Failed …` with its patch file, as in 1.6. Before, it was a hidden Mod message `[Mod - Start of stack trace]` with all of one Mod's failures in one row. In both versions, a block whose operation is a Mod's own class (`AnomalyPatch.PatchOperationRemoveIf`) or ends in `): Error` also counts as a patch failure now.
- Game log: a log uploaded by HugsLib (pasted, or saved to a file) is read from `Log file contents:` on. Before, `archo log` and MCP `analyze_log` read HugsLib's Mod list and patch list as game messages and missed the game's version and, for 1.7, the log format, so a 1.7 log was read as 1.6 and a log from another game version was analysed against the installed game's version.
- Game log: stacks in Unity's own format (`Verse.Log:Error (string)`, `ModNs.Type:Method()`) are read as stacks. Unity prints them for an exception after `[Ref …] Duplicate stacktrace`, and for every message when the player turns on full stack traces. Before, each frame line counted as a message of its own, so `archo log` and MCP `analyze_log` showed clusters such as `UnityEngine.Debug:LogException(…)` and the exception lost its stack; now the exception keeps its frames and names the Mod in them.
- Game log: a class the game cannot find now points at the Mod that names it. When no installed Mod defines the class, `archo log` and MCP `analyze_log` search the enabled Mods' Defs and patches for it (also in `Class="…"` attributes) and name that Mod and line as the likely source, with up to three similar existing classes (`did you mean: VanillaFurnitureEC.StatPart_Bin (#l4uq VanillaFurnitureEC.dll)`). Before, these errors said "no mod identified".
- Two Mods that ship a DLL with the same name: the game loads only the first in load order, so classes that only the other copy has are missing. A new problem, `asm.duplicate-name` (a warning when classes are lost), names both Mods and the lost classes; a "could not find type" error in the log now names the Mod whose copy loads first as the likely source; `archo mod` / MCP `get_mod` list each assembly with its size and class count and mark a name another enabled Mod also ships. Types the compiler adds (`Microsoft.CodeAnalysis.EmbeddedAttribute` …) do not count as lost.
- Game log: when a def file fails to load (`Exception loading def from file …`), the errors that only follow from its missing defs (references to them, and config checks that read a null stat) are folded into that one error: its row says `+ N consequent lines in M clusters`, and `--all` still lists them, each marked as a consequence. `archo def` / MCP `get_def` says when the newest log reports that a def's file did not load, and test runs list only the root error.

- Game log: a missing texture whose path a Mod's own code builds (`"OdysseyPatch/OutfitStands/OutfitStand_" + defName`) now names that Mod as the source, and when no installed Mod ships the file, its detail says `own art missing`: a defect of that Mod that only affects how things look, which adding the file from elsewhere would only hide. `archo test run` / MCP `test_run` give the same note. Before, these clusters said "no mod identified" and agents searched for a Mod to provide the texture.
- `archo test run` / MCP `test_run` list the run's informational lines (Mods' own reports, other messages, known noise) under `informational: N` (`info` in JSON), so a run's warnings no longer need a second look at the log; they still never fail a run. `--json` and the MCP result leave out the launch `args` (still in the run folder's `report.json`), and the messages under `fixed` and `gone` are one line of at most 100 characters.
- `archo problems --all` and MCP `list_problems all=true` list info problems as one row per code with a count when there are more than 30 (`info harmony.shared-target  N problems  (archo problems --code harmony.shared-target --all; MCP: list_problems code=harmony.shared-target all=true)`); `--code`, `--subject` and `--severity info` still list each one, and a long `--code` list's `… N more` command is `archo problems --code <code> --all`. In MCP evaluation these results were 20–25 KB, mostly info rows.
- Game log: the `nearby` lines of a cluster's detail skip the IL listings that some Mods' patches print (`AdjustParms - ldfld …`) and show the messages around them instead.
- `archo patches --target <defName>` and MCP `get_patches target=<defName>` with a bare name also find patch operations nested in `FindMod` or `Sequence`, by the defs they touched, as `--target <Type>/<defName>` does; a `target:` line says how the name was matched. Before, `target=Beauty` missed Vanilla Furniture Expanded's patch that `target=StatDef/Beauty` listed. `--target ThingDef/Lib_Gun` no longer also lists operations on `LibXGun` (`_` matched any character).
- MCP: the `analyze_log` description says to start there for any game error, before reading the log with other tools.

- Test runs test locked versions. After locking a Mod's version and editing the copy (`archo pin`, MCP `pin_mod`), `archo test run` and MCP `test_run` used to load the original, because the copy only reached the game when the list was saved, and listed the copy as missing. Now a test run links the list's Mods the game cannot see (locked versions, SteamCMD downloads, extra folders) into the game's `Mods/` for the run and removes the links when the game exits; the report lists them under `linked`. A Mod that would sit next to another copy of itself in `Mods/` is not linked and stays under `missing` with the reason. The MCP Instructions now say: lock the version, edit the copy, then `test_run`.

- Scans also read the type names of the game's own `Assembly-CSharp.dll` (and `Assembly-CSharp-firstpass.dll`), kept per game version, so a save's classes can be checked against the game as well as the Mods. They are read again only when the files change; the first read of a 1.6 game adds about 60 ms to the scan. The database upgrades to schema 31 on the first start after updating (`Upgrading the RimArcho database (schema 29 → 31)…` on the CLI's and MCP's stderr); the steps only add empty tables, for these type names and for the saves read below.
- `archo saves index` reads every save in full (new and changed ones; a few hundredths of a second per 10 MB, 2–4 at a time) and keeps what it found in RimArcho's database, so the next run reads only saves that changed. Saves that play in the same world are now one colony even when their file names differ, named after the player's faction.
- Can it load now uses a save's full read (`archo saves show` reads its save; `archo saves index` and the Saves page read the rest). A Mod that is not installed, or installed but not in the list, says what loading without it loses, for example `loading without it loses 3 colonists of race Kurin_Race (1 dead), 1 faction (Kurin Clan), 1 class (ModA.CompBroken), 3 things, 1 hediff`, and is an error when that includes colonists, factions or classes; otherwise it stays a warning. Only the facts of the save's own Mods are needed: a Mod the list has and the save never had does not leave the check incomplete. What a removed Mod owns is still known from its last copy; for a Mod RimArcho never had installed, what no installed Mod provides is given to it and marked `inferred`. Two new items: `malformed`, a save whose XML breaks off in the middle, with the line (a file that only stops early stays `cut-off`), and `class-missing`, classes no Mod in the list nor the game defines, naming the installed Mod that has them when there is one. `saves fix --launch` does not launch a malformed save. `archo saves show`, MCP `get_save` and the Saves page show these; the verdicts of `saves list` / `list_saves` follow. Without the full read, or before the list has been analysed for the game version, the items are as before.
- Saves from the command line and MCP. `archo saves list` lists every game's and pack's saves by colony, newest first, each with its `sv:` reference, the list it matches (`plan:<name> (launch)` or `(same-mods)`, `pack <name>`, `no list`) and whether it loads with that list; `--colony`, `--mod <ref>` (saves using a Mod, installed or not), `--attention` (warnings and errors), `--all`, and the global `--env` (one game's saves). `archo saves show <save>` says where a save belongs and whether it can load: each item with its consequence and the `archo` command that fixes it, then how its Mods differ from the list (`--mods`: all of them). `saves list` gives each save's in-game day once the save has been read in full (`meta only` until then, and a note that says how many and that `archo saves index` reads them). `saves show` reads its save in full first (cached for next time) and tells the playthrough: the faction, day and date, real play time, storyteller and difficulty, colonists alive, enslaved and dead, animals, mechanoids and prisoners, wealth, raids and other statistics, the newest letters, and what changed since the colony's previous save. `--play` lists every colonist (where they are, dead, new or died since the previous save) and the letters; the output stays within 40 lines and `--all` shows everything. A malformed save says where reading stopped; a save the game is still writing is not read. `archo saves fix <save>` applies the save's Mod fixes to its list as one undoable edit (`--mod`, `--disable-added`, `--order`, `--dry-run`), subscribes to missing Workshop Mods, and with `--launch` starts the game, but not while a subscribed Mod is still downloading. `archo saves rename`, `copy`, `backup` and `trash` change save files; `backup` and `trash` only list the files and sizes until `--yes`. A save is named by `sv:<hex>`, its file name or `<colony>/<save>`; a name several saves share lists them (exit 3). MCP: the read-only core tools `list_saves` and `get_save` (`play=true` for `--play`), and `manage_saves` (`--tools all`; destructive; `confirm=true` for backup and trash). `archo mcp` now serves 29 core tools, `--tools all` 45. The `errors` guide topic points a save that will not load to `get_save`.
- New Saves page under Library. Your saves are grouped by colony, newest first, and searchable by colony or save name in a search box of its own; the colonies, the selected save and the playthrough are cards `1`–`3`. Selecting a save says whether it loads with its list as the list is now, under the file, game changes and Mod list changes: the Mods that are not installed in one row (View lists their packageIds and Workshop pages and the Defs lost without them), each Mod installed but not in the list with Enable, the Mods added since with Disable, and those in another order, each with what it means; a fix is one undoable edit to that list, and Workshop Mods are then subscribed as missing Mods are. When the save's Mods have not been analysed, so what they own could not be checked, the bar offers to make a list from the save and analyse it in the background; the save is checked again when the analysis is done. A save cut off by a crash says it can never load and offers to recycle it; a save no list matches offers to make a list from it. The save's whole Mod list follows in load order with the missing and reordered ones marked. Rename (`F2`), copy, back up, make a list and recycle work from the page, and Open location shows the file's folder; back up and recycle first list the files and sizes, and rename and recycle wait while the game runs. The right side shows the playthrough at the selected save: the colony, its in-game day and date, storyteller, difficulty and play time; wealth, colonists, animals, research points and raids with how they changed since the colony's previous save; the colonists, the dead struck through; the newest letters; and the colony's wealth and colonists over time, where each save is a point that opens it. Opening the page reads new and changed saves in full in the background (Background tasks › read the saves, with its progress on the status line); the rows then show each save's in-game day instead of its date, and saves of the same world join one colony. A save you select that is not read yet is read at once, without waiting for the rest.
- List editor: dragging Mods works again in the window. Since 1.0.0 a drag started but never dropped, so Mods could not be reordered, enabled, disabled or staged by dragging (the keys still worked). The window's custom title bar hosts the page in a way that has no Windows drop target, so dragging now follows the mouse itself. Dragging near the top or bottom of a list scrolls it, and `esc` cancels. Dragging Mod names out to another program as text no longer works.
- A Mod's details: the Archo ID, package ID and Workshop ID now end in a copy icon, like addresses; click the icon to copy. Clicking the text no longer copies it, so it can be selected.
- Test runs compare with your game log. `archo test run` and MCP `test_run` used to compare only with the previous test run of the same plan, so the first run of a plan said nothing about whether your errors were gone. Now every run is also compared with the log `archo log` reads (your last game's `Player.log`): each error says whether it was already there (`before: present` / `absent` in JSON, `before: in the player's log` in text), and `fixed` lists your log's errors that the test run no longer has, with the Mods and confidence they were attributed to. `player_log` counts them; errors a test run cannot show, such as ones from loading a save or from play, or every one when the run did not reach the menu, are counted as `unreached` and not as fixed. Errors that disappeared since the previous run (`gone`) now keep their Mods and confidence too. The exit code (and MCP `isError`) is unchanged: any remaining error still fails.
- Game log: errors the game writes as `Tried to …` are no longer dropped. RimWorld 1.6 writes `Log.Error` without a stack, and RimArcho kept such a line only when it said error, failed, could not, missing or similar, so `Tried to add pawn X to lord with lord jobProgressionEducation.LordJob_AttendClass but this pawn is already a member of lord with lord jobRecreationalSexWithEuterpe.LordJob_Birthday` vanished, even when it filled a log every tick. These lines are now clusters. One that names a Mod's type is a new `game-error` category, a warning shown by default in `archo log`, MCP `analyze_log`, `archo problems` and test runs; one that names only game types stays `other`. Type names the game glued to the word before them (`jobNs.Type`) now name their Mod. In the lord message, the Mod whose lord adds the pawn is the source and the other lord's Mod is only mentioned, so the bystander is not blamed; the message for each pawn joins one cluster.
- Game log: two lines that appear in nearly every Player.log are now recognised as known noise. They are a new informational category, `noise`, hidden by default like other informational clusters. The first is `Material 'SolidColorMat-…' with Shader '…' doesn't have a texture property '_MainTex'`, a material made in code; no def or file causes it. Before, it was a `material` problem whose pattern sent agents to search for `uiIconColor`. The second is `Translation data for language … has N errors`, the count of problems in the translation files, which affects only text. Before, this line was dropped without a word. `archo log` and MCP `analyze_log` name both kinds on the `hidden:` line (`known noise, not mod problems: log:E5 material made in code, log:E6 translation error count`). Their detail says what they are, and other clusters' nearby lines mark them `(known noise: …)`. `archo test run` / MCP `test_run` no longer fail on them alone; they are counted under `counts.noise`. A def's own material without a texture keeps `pattern:g:asset.maintex`.
- Game log: an error cluster with no Mod identified, whose kind of error appears in more than 5% of public logs, is labelled `common kind, no source found`. The label appears on its `archo log` / `analyze_log` row and as `note: common kind, no source found: this kind is in 6.7% of public 1.6 logs (pattern:g:asset.maintex)` in its detail. `test_run` gives the same `note` on the cluster. The label changes no attribution or exit code, and a common kind is not harmless: it can be the root cause (the `guide` error kinds say so).
- A `Player.log` RimArcho cannot tie to a launch (the game started from Steam before RimArcho ever wrote the list, a log copied from another PC, or this game before its folder moved) is now attributed properly. `archo log` and MCP `analyze_log` bring this environment's analysis up to date first, as for its own logs, so Mod types and Harmony patches in such a log name their Mod; before, nothing was analysed and those clusters said "(no mod identified)". When the log's `Mono path` names a game folder no environment has, the output says `game: log game path … is not this environment's game; attributed against env:main`. A log another environment's game wrote is still refused. While Mods are not yet analysed, a failing type reads `owner unknown, assemblies not analysed yet (N enabled mods)` instead of "no enabled mod's analysed assemblies define it", and `archo log` adds an `analysis:` line. Such a log of this environment's game is now counted as the last run in `archo problems`, `archo status` and MCP `list_problems` / `status`, titled `source unknown (no launch and no ModsConfig.xml write by RimArcho before it; counted as the last run)`; before, they said `0 error` over a log full of errors. A log whose game folder no environment has stays out of the counts; its folded row names both forms, `(archo problems --code log --all; MCP: list_problems code=log)`, and the stale reason names the unknown game folder.
- Network settings for places where GitHub or Steam are blocked, such as mainland China. RimArcho's own downloads (community data, self-update, todds, Workshop descriptions and images, update checks) now follow the Windows system proxy, as set by Clash or v2rayN in system-proxy mode, after the `HTTPS_PROXY` / `HTTP_PROXY` variables; before, only the window's pages did. Settings › network sets the proxy (`system`, `off`, or an address such as `http://127.0.0.1:7890` or `socks5://127.0.0.1:7891`) and a GitHub mirror such as `https://gh-proxy.org/`, tried before GitHub; when the mirror fails, GitHub is asked directly. `archo config set network.proxy …` / `network.github_mirror …` do the same. A download that cannot reach its site now says which setting to try. The todds download is now checked against the 0.4.1 release's SHA-256, not only its size.
- A Mod's full details › technical information: for a Mod that is not in the current list, patches and Harmony showed 0 and "no assemblies", even when the Mod has patch files and DLLs. RimArcho analyses those only for the current list, so the tabs now say "not analysed (not in the current list)" with no count. `archo patches --mod`, `archo harmony --mod` and MCP `get_patches` / `get_harmony` with a Mod answer the same. Defs are still counted for every Mod.
- Databases from pre-1.0 development builds are no longer upgraded. RimArcho refuses to open one and says to move `archo.db` away to start with a new database; the file is left unchanged. `RimArcho.exe` now shows this, and any other failure to open its database, in an error dialog instead of closing without a word. Databases from 1.0.0 are unaffected.
- Sidebar lists: opening a list that is not the current one no longer moves it up under All lists. The four lists follow the order you enabled them: the current list first, then the one enabled before it, and so on; lists never enabled follow by their last edit or launch.
- Plan editor: the details pane no longer leaves a blank band of about 18 px between its header and the "copies" list when the Mod has several copies.
- Change Mod settings without starting the game: on Mod library › Mod settings, on/off values are switches, numbers and text are fields (Enter saves), HugsLib choices are lists, and a stored value can be reset to the Mod's default. RimArcho changes only values it understands, only while no game runs, and first copies the file to `<file>.rimarcho.bak`; the rest of the file stays exactly as it was. Lists and nested values still change in the game. `archo mod-settings set <ref> name=value… [--reset name] [--file key] [--dry-run]` and MCP `set_mod_settings` (`--tools all`) do the same; `archo mod-settings <ref>` names each file's key when a Mod has several. Uninstall removes these backups. Archo Monitor 0.11.0 also records each HugsLib enum setting's choices.
- HugsLib settings: Mods built on HugsLib keep their settings in HugsLib's own file and window, so Mod settings did not show them. Now each such Mod has a HugsLib row on Mod library › Mod settings, in `archo mod-settings` and MCP `get_mod_settings`. With Archo Monitor 0.11.0, which records each HugsLib setting's title, description and default once the game has started, every setting shows by title with its value, a changed value with its default, and one at its default greyed; settings the Mod hides are left out. Without that record, the values HugsLib stores show for packs named after the Mod's package id. Read only: change values in HugsLib's window.
- Archo Monitor 0.11.0: when you open a Mod's settings window, it records what the window shows in its first frame: headings, and each checkbox, choice, slider (with its range), text or number field with its label and value. Mod library › Mod settings, `archo mod-settings <ref>` and MCP `get_mod_settings` show it beside the file's values, with the date, so the file's keys can be read against the game's labels. Monitor patches the game's drawing only from the first settings window you open, and then only records while that window's first frame draws. RimArcho now reads Monitor's log only past where it last stopped.
- Archo Monitor 0.10.0: when the main menu first opens, it records which settings file each Mod with settings reads and its settings window's title. RimArcho keeps these records per game data folder, so a Mod's settings are found again under a folder it used long ago, even a copy since deleted or renamed, and a pack's settings survive renaming the pack. The record is not an entry: the game log page and `analyze_log` are unchanged.
- Mod settings follow the Mod. The game keeps a Mod's settings in a file named after the folder it sees the Mod under, so playing the same Mod from a SteamCMD or extra-folder copy (an `_Archo_` link), another game's copy or a pack seemed to lose its settings and then kept a second set. Now saving or launching a list copies a Mod's settings saved under its other folder names to the name the game reads, when that file does not exist yet: the newest copy wins, existing files are never replaced and the originals stay. Packs get the same on every launch, from their own settings first and the game's otherwise, plus HugsLib's settings while the pack has none; test runs get it in their own data folder only. `archo save` / `launch` / `pack launch` (and MCP `save_loadout` / `launch_game` / `manage_packs action=launch`) list the copies under `Mod settings`; dry runs list what would be copied.
- Screen readers and AI agents can read and operate the window without screenshots. Only the page you are on is read (the editor no longer sits behind other pages), and it is named after its breadcrumb. The sidebar and breadcrumb mark where you are. Editor lists read each Mod as name and "n of total" with its selection; a missing Mod adds "not installed" and a grouped copy its copy. The Mod library, tasks, packs and texture tables have rows and cells under their headers, with lineages expanded or collapsed. Nothing changes on screen.
- Confirmations and prompts hold the keyboard: focus moves into the dialog, Tab and Shift+Tab stay inside it instead of reaching the page behind, Esc closes it, and focus returns to where it was. Enter still confirms; Tab reaches Cancel first, and Enter there cancels. The URL prompt no longer loses Esc after Tab.
- Screen readers and AI agents follow work and its results: the status line reads results politely and failures at once, and work in progress (rescans, checking the mods' files, packs, texture conversion, downloads, tasks, the guide's steps, pages loading) reads as a progress bar with its count, or as still running. Undo on the status line is a button. A field that refused its input (a URL, a list or pack name, an import, a setting, a collection or subscription link, a command) reads as invalid with the reason. Nothing looks different.
- Screen readers and AI agents can use menus and lists: right-click and ⋯ menus, column header menus and the top bar's sub-pages read as a menu named after what it acts on (the Mod, list, column, task or log file), with checked column toggles; the command panel's results, the list and game choosers, tags and the position input read as lists with the highlighted item; the problems panel and version warnings are named panels. The add subscription box has a name. Nothing looks different.
- Esc on a menu, or choosing an item, puts the focus back where the menu opened, also after a right-click; Esc on a column header's menu goes back to the table's rows. In the sidebar, Esc on a right-click menu used to leave it open. The Menu key or Shift+F10 on a list in the sidebar or breadcrumb opens its menu, as right-click does. On the game log, Enter or Space on the file picker opens it. `p` moves the focus into the problems panel, and Esc puts it back.
- Screen readers and AI agents read states and find rows again. On/off and one-of-N buttons (segmented choices, the background's focus and image groups, a list's game versions, export formats, the launch page's picks) say whether they are on; the load-order rules, history, Staging and can it run's worth knowing say whether they are open; Background tasks say which tasks are selected, apart from the cursor, and read each task's state, line and time; a Mod's full details say which copy they show; an editor row with unsaved edits says so. Rows of tasks, packs, a pack's Mods, textures, load-order rules, Search files, the game log and settings keep an id taken from what they show, so they are found again after a refresh or a sort. Settings rows, the search filters, uninstall's steps and the update sources are named, so a "log in", ×, number or "check" button reads with its row.
- Search files: expanding or collapsing a file result through UI Automation (screen readers, agents) no longer sends the window to the launch page. Mod library: UI Automation's expand and collapse now work on a Mod with several copies, through its arrow.
- Tab moves between controls on every page of the window, as on a web page, and reaches the copy buttons beside addresses. On Packs and in a Mod's full details it used to switch to the next pack or technical tab when the page showed no address; use the sidebar, the breadcrumb or the technical tabs instead. The packs page's hint line no longer lists Tab. The terminal keeps Tab for sub-pages.
- Rescan and check the mods' files show … on the status line at once, before RimArcho answers. A pack build or texture conversion that fails at once keeps its failure on the status line; before, the "goes on in the background" reply could replace it.
- A scan, a check of the mods' files, a pack build or a texture conversion that fails says which one on the status line ("create a pack  <name>: …"), since it can end while you are on another page. Removing converted textures now says when it is done.
- Create a pack: the textures choice no longer disappears while counting textures, and keeps its focus.
- Mod library › Mod settings: a new page with the current list's Mods that have settings, one row per settings file with its settings window's title (when Archo Monitor saw it), how many values it stores and its size. The values of the file under the cursor show beside the table: on/off, numbers and text, with lists, dictionaries and nested values counted, and the file's path. Read only: change values in the game. `ctrl+o` opens the Config folder. A Mod's details say how many values its settings store, and open goes to the page at that Mod. In the terminal (TUI), the command panel's Mod settings shows the same as text.
- `archo mod-settings` lists the current list's Mods that have settings (their settings window's title when Archo Monitor saw it, the file, or "defaults" when there is none yet); `archo mod-settings <ref>` shows what one Mod's settings files store: on/off values, numbers and text, with lists, dictionaries and nested values counted. Only values that differ from the Mod's defaults are in a file. `archo mod <ref>` shows the same for that Mod. MCP: the read-only core tool `get_mod_settings`. Read only: change values in the game.
- MCP: `archo mcp` now serves 27 core tools by default: diagnosis (`status`, `analyze_log`, `list_problems`, `get_mod`, `get_mod_settings`, `get_def`, `get_patches`, `get_harmony`, `search_files`, `read_source` and the rest), plan edits, `save_loadout`, `launch_game`, `test_run`, `pin_mod` / `unpin_mod`, plans and their history, `manage_rules`, `ignore_problem`, `scan` and `list_library`. `archo mcp --tools all` serves all 41 tools, adding the player's management tools: `manage_subscriptions`, `manage_packs`, `manage_library`, `exchange_list`, `manage_textures`, `estimate_memory`, `maintenance`, `resolve_identity`, `manage_hidden`, `check_links`, `repair_links`, `mute_version_warnings`, `set_player_data` and `preview_uninstall`. To keep using those tools, add `--tools all` to the agent's `archo mcp` command. The full list cost an agent about 11k tokens on every turn (29% of its tokens in an evaluation), and the management tools were never called there. The core `tools/list` is 27 KB instead of 43 KB. The server instructions name what `--tools all` adds, and `archo guide` lists those tools on a separate line. Any other `--tools` value is a usage error (exit 2).
- MCP: the aliases `copy_local` (use `pin_mod`) and `add_rule` (use `manage_rules`) are removed, with their guide topics. Both were renamed in 0.9.0 and kept as aliases through the 1.0.0 release, the minor version 13 §5 requires. The CLI command `archo copy-local` stays.
- `archo test run` / MCP `test_run` compare error clusters with the previous run of the same plan and map mode: JSON adds `baseline` (`new`, `present`, `gone` counts), `delta` on each cluster and a `gone` list; the text marks new clusters and lists the gone ones. A fixed error is now visible even when unrelated errors remain. The exit code and MCP `isError` are unchanged: any error cluster still fails.
- Log attribution names the Mod whose assembly defines a type in "static constructor of T" (and other type evidence) in a freshly scanned environment: `test_run` brings the tested plan's analysis up to date after the game exits (JSON `analysis_error` if that fails), and `archo log` / MCP `analyze_log` do the same for the environment's own logs and pasted text. Before, they used only a stored analysis, so a Harmony patch failing in a Mod's static constructor, or a texture path named only in a Mod's code, came back with no Mod. While the MCP wait (10 s) runs out, `analyze_log` says `analysis: still computing`.
- MCP: long tool calls send a progress notification every 10 seconds when the client asks for progress; for `test_run` it names the current step, such as waiting for the game to load. Clients that restart their request timeout on progress, such as Pi, no longer give up on `test_run` after 60 seconds. Giving up cancelled the call, which closed the game before it could report.
- A bare packageId now works wherever a Mod reference does, in any case: `archo mod CETeam.CombatExtended`, `archo enable khamenman.outfitstandsplus`, MCP `get_mod`, `edit_loadout refs` / `before` / `after`, `list_changes ref`, `get_patches mod` and the rest mean the same as `pkg:<packageId>`. Before, they failed with "not a reference". A word counts as a packageId when it has a dot, a letter, and only letters, digits, `_`, `-` and dots, so versions such as `1.6`, Def references and paths are not taken for one. A packageId that matches nothing says so and suggests a search by its longest part (`search with archo find outfitstandsplus`); on MCP the suggestion names the tool, `find_mods query=outfitstandsplus`, instead of the CLI. A bare word that is no ID prefix in use (`archo mod Alpha`) now suggests `archo find Alpha` instead of `archo find <words>`.

## 1.0.0 — 2026-11-02

The first public release. It contains everything below: 0.5.0 to 0.9.0 were development milestones
and were not released on their own.

## 0.9.0 — plans and launching (development milestone, in 1.0.0)

- The window no longer grows while it sits on the launch page: refreshing the launch memory comparison every 30 seconds reuses the page's data instead of rereading the whole Mod list (30 MB and about 95 ms per refresh for 6,574 Mods, now 0.2 MB and 3–4 ms). Pages unused for 5 minutes (Mod library, list management, Mod details, texture optimization, uninstall) give their memory back; opening one again reads it anew.
- The window's pages now share one copy of the Mod index instead of one each: with 6,574 Mods its own (Go) memory after visiting every page went from 120 to 74 MB, and to about 65 MB once unused pages close. Left in front without mouse or keyboard input for 2 minutes, the window also asks WebView2 to trim its memory, as it already did in the background; the next input brings it back.
- The window uses less memory with many Mods: its pages share one copy of the list registry and game links instead of one each, and only pages that open instantly keep their content cached. After a minute in the background (for example while playing), and at once when minimised, it asks WebView2 to trim its memory; it returns to normal when the window is active again. With 6,574 Mods: 290 → about 160 MB in the background, 325 → 290 MB after visiting every page.

- Search files follows the card rules: the query, results and file view are cards 1–3 (the digit keys focus them), the left column starts at 38% of the page, and while the filter panel is open it scrolls inside the query card, whose height drags (38% by default, double-click restores). With text selected in the file view, ctrl+c copies that text.

- Search files shows searching… while discovering its scope and one not found summary for empty results. The source-library note appears only when the run decompiled assemblies.

- Search files now has search/files/mods fields with shared library filters, streaming results and stoppable C# searches including Core. The window adds a bounded file viewer, highlighted matches and persistent wrapping; the TUI shares the fields and streaming tree. CLI/MCP search flags and output are unchanged.

- Uninstall now lists a relocated source library and its size, then deletes its generated code. Libraries already covered by data-folder deletion are counted once; linked library paths are refused.
- Decompiled C# now lives in a movable Source library with readable paths, separate from clearable caches. Settings offers size, confirmed removal of unused folders, and a stoppable task to decompile the current list. Identical DLLs are searched once and results name their sharing copies.
- CLI adds `source game`, `source search --game`, `source clean --dry-run/--yes`, and `source decompile --active`. MCP `read_source` adds `game`; source and search output return library paths. Existing decompile history migrates; the old source cache is discarded once on opening.
- Mod library and the plan editor support `lists:"<name>"` and its negation, matching exact Mod copies enabled by a list. The filter panel shows Lists first, marks the current list, and leaves out the list being edited. CLI/MCP library filters accept `lists:<name>` too.
- GUI: keys are drawn as key caps everywhere (buttons, dialog buttons, the hint line), so a key no longer reads as part of its label: confirm [enter ↵] instead of confirm enter.
- Mod library's Subscriptions and updates page is now Updates in the GUI, TUI, and tool descriptions. The source summary selects Update checks in the detail pane, with per-source results, failed Mod names and reasons, and account links only where needed.
- Workshop items removed by their author or made private appear under Won't update any more, with a short explanation and actions to lock this version or open its exact copy in Mod library. They do not count as failed checks.
- Change-history titles now say what was added or removed and name single Mods, including CLI/MCP output. History and Recently added names, plus pending-update library actions, open the exact installed copy even when versions share a name.
- Interface translations moved into language files. Archo Monitor settings now follow the game language. Mod roles on the game log page use normal brackets in English.
- Documentation is now in English, with a new Chinese README (`README.zh-CN.md`).
- Unity 1.6 RAM estimates now use heavy-texture VM measurements up to 7.5 GiB, reducing high-payload resident and loading budgets; larger loads remain extrapolated.
- Unity 1.6 memory estimates now count each texture's graphics-driver copy (1.2 bytes per texture byte, 0.66 GiB for vanilla) as VRAM instead of RAM. A computer with a large enough graphics card sees a much lower RAM need; VRAM the card cannot hold, or all of it without dedicated VRAM (VMs, integrated GPUs), still counts as RAM, and Can it run, Loading cost and `archo plan cost` say how much (JSON `vramInRAMLow` / `vramInRAMHigh`). Without dedicated VRAM the estimates are unchanged. The real-card split is inferred from the game's code and VM measurements, not yet measured.
- GUI: off/on dropdowns are now rounded rectangular two-segment switches with concentric inner corners. Other dropdowns use rounded menus with centered SVG arrows, concentric option corners, a title, checked current value, keyboard navigation and scrolling; menus stay within the window and above scroll containers.
- Uninstall confirmation now lists what will be removed or changed, counts, sizes,
  removal methods and what stays. The dialog defaults to Cancel; only its red
  Uninstall button starts the work. TUI shows the same summary and reviews changed
  choices before confirmation. Progress is centered with a collapsible step list,
  restored after reload. Windows' shutdown-block reason follows the app language.
- Uninstall now opens a dedicated progress page and disables navigation until the report.
  Go blocks window/application closing during uninstall, including custom close buttons;
  the page explains that closing is available after completion. Progress/report survives
  a WebView reload. Windows receives a shutdown-block reason during uninstall.
  TUI uninstall runs in the background with progress; Ctrl+C, q and Esc show the same
  notice while it runs, and work again afterwards. Embedded GUI/TUI commands
  (`uninstall --yes`) use the same protected workflow; previews remain read-only.
- GUI file, folder and web addresses now have a trailing copy icon, show ✓ after
  copying, and shorten in the middle as the window or column changes width. Hover
  and copy retain the complete address; address text stays selectable. Local path
  settings, first-run directory choices, tool programs and import files open the
  system picker directly from an editable address or its Change/Add button. Choices
  apply immediately; cancelling preserves the value and directory moves still need
  preview confirmation. Community-data URLs use a validated small dialog. Narrow
  address rows wrap their buttons before shrinking the address.

- GUI: Ctrl+K and `:` open one bottom command panel on every page. Search actions,
  mods, games and lists, or run archo commands with shared TUI completion and history.
  Commands run in background tasks; output keeps the CLI text and exit code and can
  be copied. Destructive commands still require `--yes`.
- Windows: use flat caption buttons with thin separators and a red close hover;
  replace the settings icon with a six-tooth gear in both navigation layouts.

- Windows: move minimize, maximize / restore and close into the navigation's top row,
  replacing the native title bar. Drag its blank space to move the window or
  double-click to maximize / restore; window controls follow the page theme.

- Memory estimates now report an unknown game version instead of silently using
  the 1.6 model; all lists use the selected game environment and its effective folders.

- Unity 1.6 RAM estimates now use VM measurements for the baseline, texture memory,
  per-mod overhead and startup reserve. Resident and loading budgets stay separate;
  existing 1.7 texture calibration is retained. `plan cost` and MCP `estimate_memory`
  report the same capacity levels.
- The launch button turns yellow when available RAM is inside the loading budget,
  red below its lower bound. Red launches ask for confirmation with Cancel focused;
  the TUI also confirms. Can it run compares RAM and VRAM on separate lines.
  Unknown readings keep the ordinary button; VRAM shortages remain warnings.

- Settings › Feedback (feedback, also in the command palette): write a bug report or
  suggestion; it becomes a public issue in the GitHub repository, which the page says
  above the preview. Two ways:
  - **send** needs no GitHub account. It goes through RimArcho's feedback relay, a
    Cloudflare Worker in `feedback-relay/` that holds the GitHub App key; RimArcho holds
    none. An optional contact (Discord name or email) goes only to the maintainer, never
    into the issue. The relay allows one report a minute and a daily cap. The relay is
    deployed and its address is built in (`internal/feedback/relay.go`;
    `RIMARCHO_FEEDBACK_RELAY` overrides it for testing).
  - **open GitHub** opens the new-issue page with the title and text filled in, for
    players with an account. A report too long for a link (about 2,000 characters) is
    copied for you, and the link says where to paste it.

  RimArcho's version, the game version and Windows always go with a report. The enabled
  mods, the paths (your user folder shown as `%USERPROFILE%`) and the newest game log's
  errors go only when ticked. The preview shows the report as it reads (title, text, basic
  info, each attachment folded under its heading), not the Markdown that is sent or copied.
  In the TUI the same panel ticks the attachments with `m` `p` `l`, sends with `s` (title,
  text and contact on the input line) and opens GitHub with `enter`.
- Settings now start with About in both GUI and TUI: RimArcho's version and
  Check for updates, the existing automatic-check control (covering RimArcho, mods,
  tools and community data), Feedback, then Discord/GitHub community links. Updates
  retains interval controls; the old community group and duplicate version/check
  entries are removed. The remaining groups are Interface, Accounts, Updates,
  Folders, Game log, External tools, Community data, Storage and Uninstall.
- Settings: the language choice reads follow system / Simplified Chinese / English instead of
  auto / zh / en, and RimArcho's subscription download folder is labelled
  (subscription downloads) instead of "Subscriptions".

- The database (`archo.db`) is about a third smaller: on a synthetic library like a big
  real one, 245 → 156 MB. Each mod version's list of textures, sounds, translations and
  other non-code files is stored as one compressed block, and Def / assembly facts name
  their file by a number in their set's list instead of repeating the path. The first
  start after updating upgrades the database (schema 28) in one go, about 5 seconds on
  that library; nothing is rescanned or reanalysed. While it runs, `RimArcho.exe` shows
  a small window with a progress bar before the main window, and the TUI, CLI and MCP
  write one line to stderr ("Upgrading the RimArcho database (schema 27 → 28)…", then the
  time it took); an upgrade under 0.3 s shows nothing. The freed space is reclaimed by the
  next database compaction. Finding which mod ships a missing texture or DLL from the game
  log is two to three times faster; the change history's file lists, Can it run, `def`,
  `harmony` and `db vacuum --dry-run` show the same as before.

- RimArcho updates itself. While the window or the TUI is open, the automatic checks also
  ask GitHub Releases for a newer RimArcho (daily by default, the same switch and interval
  as the other checks; a release build only looks at releases, a prerelease build such as
  1.0.0-rc1 also at newer prereleases; development builds never check). A new version
  shows in Settings › updates as the first row, "RimArcho", with its release notes folded
  under it and **install update** / **check for updates**, as a line at the top right of
  the launch page, and once as a quiet notice. Installing asks first, then downloads the
  release zip, checks it against the release's `.sha256` file, waits until no other
  RimArcho runs from the program folder (window, TUI, `archo mcp`; it can be stopped while
  it waits), swaps the program files, checks that the new program starts and puts the old
  one back if it does not, and restarts the window or the TUI. The data folder is not
  touched. New command `archo update` (installs; `--check` only checks and shows what it
  would do). `archo status` / MCP `status`: the `automatic checks` line says
  "RimArcho 0.9.1 available". There is no MCP tool that installs. TUI: command palette
  "update RimArcho" or `:update`. `release.ps1` now also writes
  `dist/archo-<version>-windows-amd64.zip.sha256`; upload it with the zip, or the update
  is not offered for installing.

- TUI: the pages use the window's names and keys. The top bar and breadcrumbs say
  can it run (not needs attention) and mod library › subscriptions and updates / all / add a subscription / search files; subscriptions and updates'
  lists are sources, waiting for you to update and already updated, with the window's scope line (list "…", `a` all mods).
  In the editor `H` opens the history chooser, `A` checks the mods' files again, and on a
  narrow screen `4` opens the full detail like `enter`; `?` works in the full detail and
  the technical page. can it run starts with the window's sentence ("…: things to fix before you start: 3 items"),
  has `F` fix N items automatically when something can be fixed in one go, `c` for an item's mod and `u`
  to undo; while the game runs the section reads errors from this run. The palette's tags,
  game log, search files, load-order rules and background tasks now do something (they did nothing before), and
  the key hints no longer offer `!` and `#` for what moved.
- TUI: new search files page (palette, or `tab` from add a subscription): the window's file search in the
  terminal, on the same search. Type and `enter`; the kind and scope rows below the box
  take `↑↓ ←→ space`, the window's `alt+` switches work, `[` `]` turn the page, `enter`
  opens the file's folder. `esc` stops a running search, and leaving the page stops it.
- TUI: `r` in the editor opens a load-order rules screen with the window's rule form for the
  cursor's mod (kind, the other mod from candidates, a note), instead of a typed
  `after <ref>` line. The screen lists the rules for the open list (`a` shows all),
  `enter` edits one, `Del` removes it and `u` right after puts it back, `o` shows its mod
  in the list. Also in the palette as load-order rules.
- TUI: palette background tasks shows the background tasks as text: running, queued, today,
  yesterday, earlier, one line each with its state and how it ended. Stop and retry stay
  in the window.
- TUI: after an edit on can it run (`F`, an item's key), the check that follows by itself no
  longer writes its progress and "analysis done" over the status line, so "…, u to undo" stays;
  the page head says "checking mod files…" meanwhile. `A` still reports in the status line.
- TUI: a simple start page (`shift+1`): the current list, its game and full version, how it
  stands, the last game's errors, the most likely trouble, and ▶ launch the game. The editor's `L`
  now goes there first, as in the window, and `L` or `enter` there launches; `w` saves
  without launching.
- TUI: picking a list in `P`, in the palette or with `tab` in the editor only opens it, as
  in the window: the list that launches stays, and the top bar says "launching X". `P` has
  make current; saving the list or `L` in its editor makes it current.
- TUI: `T` outside staging puts every enabled mod into staging, as in the window; in
  staging it still puts everything back.
- GUI: the palette's load-order rules opens the editor's rules card.
- Palette (both): a name that ends with what you typed ranks like one that starts with it,
  so "sort" still puts automatic sorting first, next to the new load-order rules.

- List editor: the cards stacked in a column (details, load-order rules, history; Enabled and Staging) are
  4 px apart like the columns, not 12 px.

- Game log: a missing texture or DLL is again traced to a Workshop, SteamCMD or other
  shared-folder mod that ships it ("ships … but is not enabled", with `archo enable`).
  Since mod folders became shared by every game, only mods in the game's own `Mods/` were
  found, and the rest read "providers: none among installed mods". The same holds for the
  "installed copies" note when an assembly's types cannot be listed. Logs already analysed
  are attributed again.

- mod library opens faster on a large library. Whether a mod has code, Defs / Patches, translations or convertible textures, and which languages it has, are now worked out once when a scan records the mod, instead of reading its whole file list each time. On 1354 mods the first open's content lookups went from about 245 ms to under 2 ms; the `type:` search, the texture mark and the languages in the details show the same as before. The first start after updating upgrades the database (schema 27) by reading the stored file lists once: about half a second for a library with a million file rows.

- MCP: what an agent reads when it connects is about a fifth of what it was. The server
  instructions and the tool descriptions went from 53,325 to 10,420 characters (8,816 +
  44,509 → 1,589 + 8,831; roughly 13k → 2.6k tokens), and the whole `tools/list` answer
  with parameter schemas from 81 KB to 44 KB. Each description now says in one to three
  sentences what the tool does, when to use it or another tool, and whether to dry-run and
  ask the player first. The detail moved to a new read-only tool `guide`
  (`archo guide [<topic>]` on the command line, same text): `topic=diagnose` is the
  error-diagnosis workflow that used to be in the instructions, `errors` the checklist of
  error kinds, `plans`, `words` (what the interface calls things), and `topic=<tool name>`
  a tool's full reference with an example call; no topic lists them, an unknown one is
  an error. No tool or parameter was renamed or removed (`add_rule` and `copy_local` stay
  as aliases); the `env` parameter's help is shorter. A test caps these sizes so they do
  not creep back up.
- The window's list editor has load-order rules (load-order rules) above history in its left column.
  Rules are shared by all games and lists; the card shows the ones that involve a mod
  enabled in the current list (only the current list, the default) or every rule (all load-order rules), and
  the count on its bar follows that choice. One sentence each with its note; the mod
  names jump to the mod in the list. `r` on a mod, or add a rule at the bottom, opens a
  small form in place: the kind, the other mod (type its name and pick), a note. edit
  changes a rule and keeps its `r:<n>`; delete is immediate and the status line's undo (or
  `u` right after) puts it back with the same number. Ignore rules still come from Can it
  run and `archo sort`; here their note can change.
- New `archo rule edit <r:n> <kind> <A> [<B>] | ignore <constraint> [--note <text>]`:
  replaces a rule and keeps its `r:<n>`. The MCP tool `manage_rules` has no `edit` yet.
- New page mod library › search files (also in the command palette): the window's way to
  `archo files search`. Search the files the game loads for the enabled mods, or for the
  mods you name, with case, whole-word, regex and defName switches and the Defs /
  patches / About / translations / textures / sounds / assemblies kinds. 60 lines a page,
  grouped by mod and file; a click on a path copies it, `enter` opens its folder. "C# code"
  searches the decompiled C# of the mods you name (`archo source search --in`, needs
  ilspycmd), decompiling for at most a minute per search. A new search, stop or leaving
  the page cancels the one running.

- The terminal interface no longer uses WezTerm. `archo` (and a double click on
  `archo.exe`) opens the TUI in the terminal it was started from; a double click opens
  Windows' default terminal (Windows Terminal on Windows 11). RimArcho no longer offers
  to download WezTerm, no longer ships the Maple Mono font (the release zip has no
  `fonts/` folder), and the TUI no longer draws thumbnails or description images: the
  name starts at the left edge and images show as text placeholders, as they already
  did in other terminals. A link in a description now shows its domain after the text,
  faint (`source (github.com)`), as the spec already said. A WezTerm an earlier version downloaded into the data folder
  (`wezterm\`, `wezterm.lua`, `logs\wezterm-start.log`) is no longer used and is left
  where it is; delete it yourself, or uninstalling RimArcho removes it with the data
  folder. The settings `terminal.host`, `wezterm.path`, `ui.previews` and
  `ui.preview_dwell_ms` are gone; `ui.icons` is `compat` (the default) or `nerd` for a
  terminal that already uses a Nerd Font, and an old `auto` reads as `compat`. `archo
  deps` and Settings › External tools list todds and SteamCMD only, and `archo deps
  install wezterm` is gone. `archo --here` still works and does the same as `archo`.

- The TUI's command line (`:`) is a real input box now. It runs any archo command, as
  in a terminal, and runs it in the background, so the screen keeps working while a
  long command (a pack build, a test run) is going. `tab` completes the word under the
  cursor: command names (by prefix, then anywhere in the name, so `remove` lists every
  remove command), flags, mods by name (it inserts the mod's reference; an empty word
  offers the mod under the cursor first), game, list and pack names, setting keys and
  their values. Several candidates show above the line; `tab` / `shift+tab` or `↑↓`
  go through them, `enter` takes one, `esc` puts the word back. `↑↓` go through the
  last 50 commands, which are kept for the next session. `←→`, `home` / `end`,
  `ctrl+←→`, `ctrl+w` and `ctrl+u` edit the line. Quotes keep spaces in one argument,
  so `pack launch "My pack"` as printed by the packs panel works (before, it split the
  name). The line above the input shows the recognised command's usage. A usage error
  (an unknown command, a missing argument) no longer runs anything: the line stays
  with the error above it. One line of output goes to the status line, more opens a
  panel. `--env <name>` works as in the CLI. `log --follow`, which never ends, is
  refused with a pointer to the status line. `analyze` with `--game-version` now runs
  as a child process like a bare `analyze`, instead of inside the TUI.

- Importing a Steam collection, a list of workshop links or a list of workshop IDs no
  longer leaves Core and the DLCs out. A list like that cannot name official content, so
  the import keeps the Core and DLCs the current plan has enabled, first in the list
  (or the installed Core when the plan has none), whether it replaces the plan or becomes
  a new one. A list that names Core or a DLC is still taken as it is. The preview's
  counts are still the list's own mods.

- With the window and the TUI open together (or two windows), neither drops the other's
  background-task records any more. Each window runs and stops only the tasks it started.
  When it writes `tasks.json` it reads the file back first and keeps the other's
  records. Opening a window no longer marks another open window's running tasks as
  stopped; only tasks whose window has gone are stopped. A task the other window runs
  shows up on background tasks once it ends, and a record removed in one window is gone from both.

- The window's sidebar no longer has the line above Settings ("Game running since
  14:05" while the game runs, "Analyse this run's log" after it). It only opened Can it
  run, which is right above it, and its number was already in that entry's badge. The
  game's errors still reach Can it run and its badge every two seconds; the Launch page
  and Can it run's right column say the game is running. After the game exits (or when
  RimArcho opens after a game run without it), the window now reads the new Player.log
  by itself, once, in the background, so the badge counts that run without a click.
  The TUI keeps its status-line indicator.

- Launch page: when the current list has edits not yet written to the game, the corner
  of the Launch block has a small "Save N changes" button (`w`). It saves without
  launching, like `w` in the editor. Before, the block only said "N unsaved", and
  saving meant opening the editor or launching. The button is not shown while the game
  runs, during a switch, or when a pack or another list is applied.

- Can it run is laid out in two columns. The left column has what breaks the game,
  then Worth knowing, then what is for you to confirm. The right column has the errors
  from the last run (or this run). Drag the gap between them to resize; double-click
  it to go back to the default. Errors in the last run and the item about the same mod
  on the left link to each other.
  Worth knowing is a section of its own: the hints from shared logs (a prerequisite
  a mod seems to need, pairs that often fail together, a mod's own known error). It
  sits above "for you to confirm", is not counted, and can be folded.
  Every action is a button at the bottom of its item; there is no "⋯" menu. A choice
  such as "disable A / disable B" shows one button per option, none of them the
  default. Actions that need a confirmation ask inside the item (`enter` / `esc`).
  An item that an action resolved stays where it was as one line: ✓, what was
  done, and an Undo button (`u`) on the latest batch that can be undone. These lines
  go away when you leave the page.
  Weaker load-order preferences and load-order cycles are now lines of the "sort"
  item, because sorting settles them; `v` lists the cycle. Hidden or locked mods
  and mods with out-of-date converted textures are no longer listed here: one line
  at the end opens Mod library (filtered to them) or Texture optimisation. Broken
  game-folder links list the current game's first; other games' links fold into one
  line, and the repair still fixes all of them.
  Keys: `1`–`9` jump to a section, `←` `→` switch columns, or move between an
  item's buttons when one has the focus; `esc` returns to the list. The TUI's `@`
  screen uses the same order, in one column, with the same merged lines and the
  closing "elsewhere" line.

- Subscriptions now work outside the window. MCP has a write tool,
  `manage_subscriptions` (list, add, check, update, remove; `dry_run` for add, update
  and remove), marked destructive and open-world. In the TUI, mod library › add a subscription opens
  from subscriptions and updates (`+`, `tab` or a paste) or the palette's add a subscription: paste an address,
  choose what to track, read the dry run, `enter` subscribes. On subscriptions and updates, `K` checks
  every source (`enter` on a source: that one), `u` / `U` update one / every one shown
  after their dry run, and `Del` removes a subscription. These run as background tasks
  recorded in `tasks.json`; updates are refused while a game runs. The TUI, MCP and the
  window recognise a pasted address the same way. In the TUI a download page subscribes
  only through its single Git repository; choosing a zip, a folder or a LoversLab file
  stays in the window or `archo subscribe … --from`.

- Analysing the same mod content for several game versions now stores one copy of
  its Def, Type, Harmony and assembly facts. Schema 26 converts existing caches in
  place. Removing a game or replacing analysis slots also removes facts no game
  needs; analysis slots for an older game version expire after 30 days without a
  new analysis. Slots for the game's current version do not expire by age.

- The database (`archo.db`) takes less space. Upgrading to schema 25 drops analysis
  indexes no query uses (about 40 MB on a library of 4,000 mods analysed for two game
  versions) and compresses stored log reports (a 9 MB report becomes about 0.4 MB).
  After an analysis or a scan that recorded mod updates, the `archo.db-wal` file is cut
  back instead of staying at its largest size (often 100 MB or more) while RimArcho is
  open. Databases that missed the index for finding a Def by `Name` (two updates both
  used schema 15) get it back, so a log with many missing parent templates is
  attributed quickly again.

- The GUI now cleans old snapshot file lists and analysis facts no game needs, and
  reclaims database space, weekly while idle. Current snapshots, unanswered identity
  evidence and each mod copy's last three updates keep their complete file lists;
  older history keeps its events and declarations and explains when file lists have
  been cleaned.
  Settings' existing Compact button, `db vacuum` and MCP maintenance's `vacuum`
  action use the same cleanup and report freed space. Running games block it;
  tombstone collection remains manual.

- When several installed mods provide a missing parent template, Can it run and
  `archo log` name them in the same order every time. Before, the first provider and
  the log problem's `p:` reference could change after the analysis cache was rebuilt.
  The lookup still starts from the name index, so the fixed order costs nothing: on a
  synthetic library of 4000 mods, the 190 lookups for a log with 190 missing-parent
  clusters take about 10 ms; an earlier build of this change took half a second.
  `archo def` and log attribution use the same lookup.

- Environments sharing a user data folder now exclude logs written by another game
  before attribution or analysis. Can it run, the game log, sidebar counts, new-log
  hints, CLI and MCP use only eligible logs, falling back to an eligible previous log.
  Explicit log paths and game-version overrides cannot bypass ownership checks: a log
  another environment's game wrote is refused.
  A game's own logs from before an upgrade still use their recorded game version.
  Logs with unknown owners remain uncounted and do not trigger another version's analysis.

- Archo Monitor records game loading, save loading, new colony generation, playing and
  returns to the main menu. Can it run groups recorded errors by phase; the game log
  adds quiet phase separators. CLI and MCP include phase counts. Older logs keep their
  existing display. The bundled Monitor version now comes from `monitor/VERSION` and
  is updated independently of the launcher.

- Last-run errors in Can it run now name the kinds of errors (with a sample resource
  path for missing textures), include both error and warning counts, and open that run's
  game log with a related cluster selected. Unattributed errors open the complete log;
  earlier logs open their own file. The TUI keeps its existing log inspection view.
  The evidence line labels the mod's file and puts gaps between it, the log name and the
  time ("file Defs/…/Apparel.xml   ArchoMonitor.log   Oct 28 15:45"); before, the window
  ran them together so they read as one path.
  While the game runs, its "this run" items open the game log at that error group too
  ("open game log"), instead of "see the log and evidence" showing a block of text.

- When a save uses content from mods that are installed but not in the list (for
  example hairstyles of a hair mod you turned off), Can it run names those mods and adds
  "it is not in the list". Before, it named a mod that only patched the game's
  save-reading code (Vanilla Expanded Framework) and said "source not confirmed".
  `archo log` and MCP list the mod that defines the missing content first, with
  confidence "likely". Logs analysed by an earlier version are analysed again once.
- While the game runs, "this run" items no longer start with "this run," (the section
  heading says it) and their evidence shows only the log file, not the count again. When
  the previous run's errors follow them, a "last run" heading with that run's time
  separates the two, in the window and the TUI.
- While the game runs, the last run in Can it run is the run before it (Player-prev.log).
  Before, a check during the run read the log the game was still writing as the last run,
  so the run in progress was listed twice and its errors counted twice. The sidebar's
  count now includes that last run while the game runs (it used to leave it out). The game log
  page says "the game is writing it: its errors join needs attention when it exits"
  instead of offering to add them. `archo log` still reads the log being written.
- While the game runs, an error Archo Monitor records before the game finishes writing it
  to Player.log is listed once. Before, when the game's next log line came more than 5
  seconds later, "this run" listed it twice, once from Player.log and once from
  ArchoMonitor.log. The counts of "this run" items no longer grow past the larger of the
  two logs' counts. `archo log --follow` prints it once too. A line the game logs as
  noise, such as "Translation data for language English has 9 errors", is no longer listed
  under "this run" from ArchoMonitor.log; the analysis after the game exits never listed it.
- An open Can it run updates itself when the background check finishes and when the game
  exits. Before, it kept the run's old time and items until you left the page and came
  back.
- When RimArcho updates Archo Monitor in your list after the game has already started
  (for example when you launch before the first scan finishes), the run's errors still
  count once the game exits. Before, Can it run treated the list as changed since the
  launch: the run that had just ended was missing and `archo problems` said "stale log
  (the plan changed since the launch …) not counted". Only a Monitor version swap at the
  same place is ignored; any other change still makes the log stale. Until the next
  launch, the new Monitor is no longer reported as "enabled but the game will not load
  it" because the old version's link is still in Mods/; the next launch replaces that
  link.

The new design's 1.0 work (01 §4.1). 1.0.0 follows once 01 §4.2 is verified, with a
`1.0.0-rc1` first.

- Downloading a game version no longer fails when SteamCMD drops its connection and
  reconnects on its own partway through: once SteamCMD says the game is fully installed,
  the version is added. Before, a finished 16 GB download was reported as "Steam could not
  be reached" and no game was added.
- Page widths follow the page's shape. Pages split into columns fill the window like the
  editor and the mod library: game log, all lists (list 38%, details 62%, draggable),
  packs, subscriptions and updates. One-column pages share one narrower width (960px) so
  labels and values sit closer: can it run, game, texture optimisation, settings. Before,
  all of them were centred in 1200px.
- The texture page's English hint no longer names its button in Chinese ("count the textures").
- Sorting adds Archo Monitor right after Harmony when the setting "add Archo Monitor to
  lists" is on and the list has none, also after you took it out; it is part of the same
  undo step as the sort. Before, the setting only covered new lists, so a sort never put
  it back. When it cannot go in (no Harmony in the list, `Mods/` on FAT/exFAT) the sort
  says why. The sort message names what it added ("sorted: added Archo Monitor, 3
  moved"). Turn the setting off to keep it out.
- Game log: the error list takes 62% of the width by default and the log text 38%; the
  Archo log analysis card takes 40% of the right side's height. Double-click a gap to go
  back to these.
- Game log rows select without expanding or jumping to the top. Use the Archo Monitor
  row’s disclosure arrow or Enter for inline details; tab switches and live refresh keep
  the scroll position, and live refresh keeps expanded rows open.
- The launch page has more room again. The headline is no longer cut off: it shrinks to
  fit its line, takes two lines when it must, and a Workshop tag at the end of the name
  (`[UPDATED 1.7]`) moves to the line under it. Under ▶ the problem is one line, with its
  reason on hover, and × shows on hover. Unsaved edits are written inside the launch
  block ("2 unsaved") instead of a line of their own; the block no longer repeats the game
  and list. Only the applied list carries ▲ / ○; another list's state is on hover. Lists
  show just their mod count; packs still say "pack". Games show their short version
  ("1.7"), none when the name already says it, with the full build on hover. Over a
  background picture the three columns stand on a flat dark (or light) ground. The
  top-right lines line up with the launch column, and "nothing changed" is no longer
  shown. The launch page has no breadcrumb. Text on the page uses four sizes.
- Checking Workshop and SteamCMD mods for updates asks Steam about 100 items per request,
  four requests at a time, instead of one request per mod: a 5182-mod library took about
  two minutes and a few items failed every run. A failed request is sent once more after
  a second, and so is an item Steam answered as busy, timed out or rate limited.
- Mods that are no longer on the Workshop (deleted or private) are still listed under the
  check ("N no longer on the Workshop"), but they no longer make the check or the
  automatic mods check fail.
- Same-mod questions are rechecked on every scan even when the scan did not read one of
  the two folders (another game's `Mods/`, a partial scan). Before, such questions stayed
  however little content the two mods shared. Opening the database once (schema 24)
  clears the ones older scans left, such as 67 questions about one VRE patch; answered
  questions are kept.
- Commands that RimArcho prints (`fix:`, `next:`, `… N more (…)`, hints in errors) write
  mod references without `#`: `fix: archo enable nyh3`, not `archo enable #nyh3`. In
  PowerShell and bash a word starting with `#` is a comment, so a pasted suggestion ran
  without its argument. The bare form needs no quotes in cmd, PowerShell or bash, and
  every command already accepted it. Lists and other text keep `#nyh3`; `L#…` and MCP tool
  arguments are unchanged. In the TUI's technical pages, `enter` on such a line still
  opens the mod.
- A player who types `archo enable #nyh3` in PowerShell or bash runs `archo enable`
  without its argument. When the missing argument is a mod reference, the usage error now
  says why: a word starting with `#` is a comment there, so write `nyh3`.
- `stage restore` and `subscribe update` need mod references or `--all`: with neither
  they refuse instead of restoring every staged mod or updating every subscription, so a
  reference the shell dropped cannot widen the command. The TUI's `T` and MCP
  `edit_loadout restore` without refs still restore everything.
- The sidebar lists Mod library below Packs (lists, packs, Mod library). Page keys follow
  the sidebar: `shift+4` lists, `shift+5` packs, `shift+6` Mod library, in the window and
  the TUI.
- Missing base content on can it run now says which added things cannot load, explains
  the observed risk, and opens a focused problem panel with fixes. It finds disabled
  providers in the existing analysis cache and, when needed, searches installed inactive
  mods only when a problem is opened (bounded and cancellable). It suggests absent companions through
  the cached Steam DB, and links the mod's Workshop requirements. A matching last log
  with no errors attributed to the mod allows ignoring the group until the mod changes.
- A missing base template that a mod's own custom patch operation adds is no longer an
  error. RimArcho cannot simulate such operations and already counts them as successful;
  a Name written in one of them now counts as added (Chimera Creation (Continued) names
  Anomaly's chimera this way). The analysis cache is rebuilt once.
- The missing-content panel trusts a fresh analysis: it searches only the installed mods
  whose indexed content may change when enabled, so large libraries no longer end with
  "search incomplete, try again". Disabled SteamCMD and extra-folder mods are now indexed
  too, so their templates are found without reading files. When nothing installed supplies
  the template it says so;
  it no longer suggests trying again. Once installed providers cover every missing
  template, the panel and `problems --ref` drop unconfirmed candidates (such as a
  loadAfter mod), and the CLI no longer prints an empty `fixes:` heading.
- The LoversLab session check asked for an old settings address that the site now
  redirects, so a signed-in session was reported as needing login while the settings row
  still said it worked. It now reads the current settings page. Automatic mod checks
  count a file page as proof of login only when it shows the signed-in header, since
  file pages are public. The "needs login" notice uses a warning mark (⚠ in the window,
  `!` in the TUI) instead of ✓.
- Missing-content rows now keep one short explanation. The panel places enable/subscribe
  first as the primary action, shows source locations beside affected content, and only
  explains optional candidates when it lists them.
- `problems --ref p:…` (also `problems show` and MCP `list_problems ref=`) leads with
  meaning and fixes. Problem inspections show relevant evidence and affected content
  instead of opening a full mod card or all of a mod's patches; `mod` keeps its full card.
- Problem text flows naturally around linked mod names, without the spacing between
  separate facts. Links are limited to mods involved in each item; row popovers keep
  their own mod name as plain text. The editor panel shows evidence only for shared-log
  hints, keeping the cutoff date without repeating author-declaration lines.

- Can it run, the editor's problems panel and mod-row popovers now link each named mod
  separately, including identity questions and folded lists. Installed names reveal their
  editor row; known missing mods open the Workshop. Folded entries display as proper lists.
- Shared-log suggestions use a neutral information symbol in the window and terminal,
  show the corpus cutoff from embedded data, and no longer count as problems. With only
  suggestions, the editor button reads "suggestions". Diagnostic wording explains what to
  try, avoids XML field names and internal log references, and treats corpus rates as hints.

- Identity questions now require matching mod contents, exclude shared About/preview/routing
  files and empty or repeated contents, and stop asking when either mod has several
  candidates. A scan removes obsolete unanswered questions; player answers are preserved.
- Answering “same” now keeps references in every plan, older list versions, undo/redo and
  staged positions valid in the same transaction. Fixed packs follow the merged identity.
- All three identity answers remain equally visible with effects and confirmation.
  A quiet “Why this question?” link opens a small comparison with a conclusion, two columns,
  up to five matching files and the three answers. The main reason also appears on the row.
  CLI/MCP comparisons omit empty rows and method details; bare `identity` lists questions.
- A mod row's version warning now names that mod's declared versions. Version reminders
  can be ignored per mod and game minor version across all lists and games, survive updates,
  and can be restored from the ignored list or mod detail. The version list opens as a
  scrollable panel with clickable mod names, declared versions, compact ignore/restore
  controls, and groups by declared versions. Group/all ignore asks first; all restore is
  available. Panel rows load only when opened. Ignored warnings leave problem counts and
  pack-build version warnings; CLI/MCP help and output use ignore/restore consistently.
  CLI: `problems --include-muted`, `problems mute-version|unmute-version <mod>…
  [--game-version X.Y]`; MCP: `list_problems include_muted`, `mute_version_warnings`.
- While the window or TUI is open, one background scheduler checks SteamCMD, Git and
  download-page mods, managed tools, LoversLab sessions and community data. Checks remember
  their last attempt across restarts. Settings has one switch and interval control; mods
  and data default to daily, tools weekly. Mod updates remain the player's choice, and
  any registered game running blocks installation. New updates appear in subscriptions
  and Settings with quiet notices; CLI/MCP status and data output use one compact summary
  of pending updates or the latest check time. Settings keeps only the switch, interval
  and a note under automatic checks; dates appear with their subjects. Results say what
  has updates, intervals use daily/weekly wording, and Chinese switches show on/off.
  Unconfigured LoversLab accounts have no status text. Tool rows display new versions.
- LoversLab sessions refresh through a signed-in request, saving renewed cookies in the
  existing encrypted session. Settings no longer waits for verification. If the site
  requires login or the session will expire soon, Settings and a notice say so ahead of
  downloads. Managed tool updates refuse tools in use; unsupported todds releases are
  shown without offering an incompatible install. Bundled fonts have no separate update.
- Community data refreshes automatically using ETag and Last-Modified. Unchanged responses
  advance the check time; failed downloads retain the previous data without a notice.
  Data, source records and the compact index are staged together; replacement or database
  failures restore the previous set. This does not cover process crashes or power loss.

- Launching from the window with a required mod missing or disabled now asks once ("launch
  anyway" / cancel) and lists what is missing: the game would not load the mods that need
  them and may reset its list to the base game. The CLI and TUI still launch without asking.
- When an outside change that RimArcho loads by itself drops mods from the list (the game
  resets its list after mods fail to load), `scan` says how many and which version still
  has them, and the can it run page shows a card: see the difference, put the version
  before back (one undoable step), or keep the change.
- The outside-change card's "see the difference" shows the two lists in the window, one
  mod a line; it used to run a CLI command the window does not allow. A save or launch
  refused over an outside change points to that card instead of listing `archo` commands.
- Downloading with SteamCMD as the account the running Steam client is signed in with signs
  the client out; the preview now says so. Steam's "no connection" (code 3) reads "the
  Steam client is offline" with a hint to restart Steam, and Workshop tasks show Steam's
  own reason instead of always "download was not confirmed".
- Steam subscribe and unsubscribe no longer get undone by Steam's own subscription refresh:
  the worker waits for the refresh to land and asks again if an item flips back.
- Background tasks: items handed to a new task by "continue" or "retry" are marked so, and
  the old task no longer offers the same action for them.
- A batch preview in the mod library goes ahead without the copies that cannot be done
  now and names them, instead of refusing the whole batch.
- Mod library cells stay on one line (long text is cut short), and analysis progress is
  shown in the window's language.

- Sorting a list with a constraint cycle now gives up the weakest constraint on the cycle
  (by the log corpus's strength: harmless, weak, then strong; then community rules,
  loadAfter, loadBefore, dependencies) and orders the rest by what holds, instead of keeping
  the whole cycle in its current order. A dependency the list had the wrong way round is now
  put right. `archo sort` marks the constraints it gives up `[broken]`; the suggested ignore
  is the weakest of them.
- Sorting honours loadTop / loadBottom (08 §1.5): from the community rules (RocketMan,
  RimThreaded, Performance Optimizer and others load last; XML Extensions first), from new
  user rules `archo rule add top|bottom <mod>`, and from RimCrow's user rules on import. A
  top mod takes what it needs up with it, a bottom mod takes what needs it down. They are
  preferences: they never break a constraint and official content stays first. `archo sort`
  lists them under "ends"; `archo rule add ignore community:<pkg>:loadbottom` drops a
  community one.
- `archo enable` without a position places each mod by the sort constraints instead of
  always at the end, with the same rule as the editor's suggested position: right after
  the last enabled mod it must load after (never ahead of Core and the DLCs); before the
  first enabled mod that needs it when that is all there is (a library, a DLC enabled
  later); at the end when nothing constrains it; an `attach` mod right after its target.
- The editor's suggested position (`m`) no longer puts a mod that only needs Harmony
  between Harmony and Core.
- The mod library now offers only library actions: list edits and single-mod DDS conversion
  stay off its row menu. Subscribed Workshop originals can be resubscribed through the
  Steam task queue, CLI `workshop resubscribe`, or MCP `manage_library`: confirmation warns
  that RimArcho moves the folder to the recycle bin before Steam downloads it again, losing
  local edits. Only confirmed unsubscribes whose folders are verified gone subscribe again;
  cleanup retries briefly and reports failures per item. Lists keep the same copy identity;
  a running game blocks the operation. Confirmed subscriptions, including leftovers, now
  request high-priority Steam downloads and wait for installed files, with live byte
  progress in background tasks and CLI output. A missing folder never counts as installed;
  stalled downloads fail with a check-Steam hint, and stopping ends the download worker.
- Smaller fixes: the sidebar's lists show all lists first, then at most four lists (the
  current one, the open one, the most recently used); the all-lists page puts "current" /
  "make current" at the right of each row and drops the collection icon; the editor's
  disabled column labels the source only for mods with several copies; Workshop mods
  uploaded with a `.git` folder show as Workshop, not Git; the game log pairs a run with
  Archo Monitor's log first and no longer with a stale or empty RimCrow log, and opens on
  Archo Monitor's tab when the run has one.
- `ctrl+\` turns reference numbers on and off: every control, text block, image and chart
  gets a thin red frame numbered at its top-left (`lib12`: the page, then the number), so a
  change can be asked for as "change lib12". The frames take no clicks.
- Lists are shared by every game (07 §1.7). Switching games no longer hides the lists made
  in another one; each game keeps its own current list. A list's Core, DLC and game-folder
  `Mods/` entries are read as the open game's own copy by packageId. Removing a game keeps
  the lists. The database moves to schema v19: lists with the same name in two games get the
  game's name added ("default (1.6)").
- The mod library also lists every other game's own `Mods/` folder, and a list can enable
  those mods in any game: writing to the game links them in as `_Archo_env_<ID>`. When the
  open game has its own copy of the same package in `Mods/`, that copy is used. A game's
  `Mods/` shows as `local-<game>` (`local-main`, `local-1.6`); `source:local` still matches
  them all.
- Removing a game and adding it back restores the SteamCMD folder inside it, with its mods'
  IDs unchanged; it used to stay "removed". `folder add` on a removed folder also restores it
  instead of registering a new one.
- In the plan editor, hovering a row's warning or error icon says what the problem is;
  clicking it pins the popover so its text can be read and selected, with the same quick
  actions as "can it run" (Esc or a click elsewhere closes it).
- Most text in the window can now be selected and copied: mod names, paths, details, logs
  and popovers. Only controls and draggable rows keep selection off.
- The plan editor and mod library filter bars now include Location, with one value per
  registered source folder. Multiple locations, exclusions and quoted paths in search are
  supported; CLI/MCP library queries accept `location:<source folder path>` too.
- Opening a list no longer switches the list you play (07 §1.3). A list opened from the
  sidebar, the palette or list management is only shown and edited; the current list — the
  one the launch page shows and launches — changes when you make another current, write it
  to the game or launch it from its editor. New lists open without becoming current. The
  editor says which list launches and offers "make current". lists opens the current list,
  which comes first in the sidebar and in list management (manage moves last), and reordered
  entries slide to their new place. `--open-plan <name>` runs one command on another list.
- List management is simpler (16 §2.17). The sidebar entry is now "all lists" (and the mod
  library's first entry "all mods"). One "new / import" button opens one form that starts
  empty; importing has a short guide, a file button that reads the file at once (no typed
  path) and drag-and-drop, and its one main button says "preview" until the content is read,
  then "create and edit". "back to management", "continue later" and "create only" are gone;
  leaving the form drops a draft whose name was never changed. The picked list shows rename,
  duplicate, delete and edit side by side; double-click the name (or F2) to rename in place,
  and "duplicate" copies the list at once and lets you name the copy.
- Mods an imported list has but you don't: the preview offers "subscribe all" (Steam or
  SteamCMD, one background task), and "can it run" folds them into one item with the same
  action instead of one error per mod (and none for the mods that need them). Once
  downloaded they take their place in the list.

### Breaking
- Database schema v10 → v18: plans (v11), analysis kept per plan (v12), copy notes (v13),
  shared mod folders (v14, below), an index for looking defs up by `Name` (v15), the DDS
  RimArcho wrote (v16), a plan's game versions and followed collections (v17), the link
  ledger (v18). Each environment's workspace, undo stack, versions and
  staging become its first plan, `default`; each `loadout tag` name becomes a plan of its own
  with one `import` version (07 §1.4).
- `launch --force` is removed: launching always writes the plan first (09 §2).
- Severities follow the log corpus (05 §4.3.1): `official.order` error → warning,
  `order.cycle` warning → info, `version.unsupported` reversed (stopping at the previous minor
  version → warning, older only → info), `harmony.multi-transpiler` warning → info (warning when
  a current log's stack passes through the method); `order.violated` is info for weak and
  harmless constraints (08 §1.3). Analyzer version 7: stored analyses are rebuilt once.
- `loadout tag` is removed: a named list is a plan (13 §4.3).
- `archo save` means "write to the game without launching" (07 §4.1).
- `save` and `launch` refuse while ModsConfig.xml holds an outside change that was not loaded
  (07 §4.1, §4.3).
- Locked copies made by `pin` live in the lock library, not in `Mods/` (07 §5).
- Light packs are gone: a pack always carries its mods' files. Sharing a list with each mod's
  version is the RimArcho list export format (Packs, below); `pack export --light` is now
  `pack export --list`, and `pack build --light` and the build page's full / light choice are
  removed. MCP `manage_packs` `list` replaces `light`.
- One environment is one game folder: `env add` refuses a game folder another environment
  uses, and the Workshop, SteamCMD and other folders are no longer an environment's (below).

### Environments and shared mod folders (03 §2)
- An environment is one game folder: its `Data` and `Mods`, version, launch, arguments,
  saves and plans. Saves are `--saves shared` (the game's default folder) or `--saves own`
  (`<data>/userdata/<env>`, started with `-savedatafolder`).
- The Workshop folder (found from Steam), SteamCMD folders, other folders, the Archo Monitor
  folder and the lock library are shared by every environment: `archo folder list|add|remove`,
  the settings page's mod folders, MCP `status show=folders`. One Workshop folder; naming another
  moves it. `env add/set --workshop/--steamcmd/--extra/--remove-source` still work and mean the
  folder commands.
- One record per mod: a shared folder's mods, every lineage, tags, notes, aliases, copy notes,
  history and user rules are the same in every environment; mod library's used in lists every
  environment's plans. Problem ignores stay per environment. Analysis stays per game version.
- `scan` reads the shared folders once plus the game's own; `scan --game-only` reads only the
  game's (adding a game), and its `Mods/` copies still join the Workshop original's lineage.
- The lock library is `lock.dir` itself; older `<lock.dir>/<env>` folders keep working as
  `src:locked-<env>`.
- Migration v14 merges each environment's copies of a shared folder, their instances by
  folder name and the lineages through them, and rewrites every reference (plans, undo,
  versions, staging, aliases, snapshots and origin markers, events, questions, notes, rules,
  player data, ignores, launch fingerprints). IDs on disk (pack.json, origin.json) resolve
  through the new `id_alias` table.
- A SteamCMD game's own Workshop folder (`<game>\steamapps\workshop\content\294100`) is a
  shared SteamCMD folder, added on `env add`, on a download and, for games added earlier, on
  the next scan. It never replaces or moves the Steam client's Workshop folder; a removed one
  stays removed.
- delete game on the environment page, in the window's environment chooser (`Delete`) and in the
  TUI's `E` (`d`): a dry run, then one confirmation. Only RimArcho's records go (plans, launch
  records, the mods read from its `Data`/`Mods`, history); game files, mods, saves and the
  shared folders stay. Refused while that game runs; another environment takes over, or the
  guide opens when none is left. `env remove <name>` without `--yes` is now that dry run.
- The environment page shows only the game folder, version, launch, saves and arguments. The
  add-environment guide offers a game you have (detected installs, a typed folder or the folder
  dialog; named after the version, e.g. 1.6) or download a version, and after adding reads only the
  new game's `Data` and `Mods`.
- A game outside a Steam library launches `direct` by default even when a Workshop folder is
  given (`env add --workshop`); only a game inside a Steam library defaults to `steam`.

### Downloading a game version (11 §3)
- `archo env branches` lists RimWorld's Steam branches from SteamCMD (anonymous
  `app_info_print`); `archo env download <branch> --to <dir>` downloads one
  with SteamCMD and adds it as an environment. The guide's download a version does the same with a
  progress bar.
- Anonymous Steam login is the default — RimArcho also serves players without a Steam
  account. An account name can be stored in the `steamcmd.account` setting (settings page,
  SteamCMD group) or given for one download (the download form / `--account`); `anonymous`
  there overrides a saved account, and clearing the setting goes back to anonymous. The
  branch list is always read anonymously. The password and Steam Guard code are typed in
  SteamCMD's own window — RimArcho never asks for, receives, stores or passes them, only the
  account name. Progress and the result come from SteamCMD's `logs/console_log.txt`;
  failures are said plainly (no ownership, password, Steam Guard, disk full, branch,
  network; for an anonymous login, Steam refusing it without a license, with the way out:
  an account, or a game folder the player already has).
- `steamcmd.path` can be set (settings page, SteamCMD group) to a steamcmd the player
  already has.
- SteamCMD is downloaded from Valve into `%LOCALAPPDATA%\RimArcho\steamcmd` only after the
  player saw its size (`--install-steamcmd` in the CLI).

### Archo Monitor (19, 07 §1.5)
- RimArcho ships its own in-game companion mod, Archo Monitor (`toumanlin.archomonitor`,
  1.5 / 1.6 / 1.7). It writes `ArchoMonitor.log` in the save data folder: the XML file and def
  being loaded at an error, the mods on the stack and their Harmony patches, and errors after
  the game stops logging; its JSON lines are a superset of RimCrow Companion's.
- It is installed to `%LOCALAPPDATA%\RimArcho\monitor\ArchoMonitor-<version>` (`monitor.dir`),
  registered as `src:monitor` and linked into `Mods/` as `_Archo_monitor_…`. New plans, and
  every existing plan once, get it right after Harmony as one undoable edit;
  `config set monitor.default off` turns that off. Newer versions replace older ones in plans.
- `archo monitor`, `archo monitor install [--dry-run]`; `status` and `scan` report it; the
  settings page has a game log group.
- The log analysis and the live tracker read `ArchoMonitor.log` (and RimCrow's
  `RimCrow_Realtime.log` / `RMM_Realtime.log`): files become source evidence and the log chip
  in can it run, counts and times come from it, errors only it saw become clusters.
- The game log page shows the run's companion log as its own section, "Archo Monitor · this run":
  one row per entry (time, level, kind such as patch error / assembly conflict, first line, ×count, related
  mods by name and files), details on click, a link to the matching Player.log cluster, and
  the session's version, game and start–end as its header. It follows the live tracker while
  the game runs. The file picker lists ArchoMonitor.log under its Player.log. A plan without
  the monitor gets one line with add Archo Monitor (one undoable step) (16 §2.10).
- Log lines on that page take RimWorld's own debug-log colours (error #ff6666, warning
  #ffeb04, message white; darkened for the light theme with the hue kept), and mods' Unity
  rich text (`<color>`, `<b>`, `<i>`) is shown as styled text, parsed in Go; other tags stay
  plain text. The first visit to the page no longer stays blank when the log reads quickly.
- The release zip has `monitor/ArchoMonitor/`; building it needs the .NET SDK 10.

### Plans (07 §1)
- `plan list|new <name> [--from current|v<n>|<file>]|use|rename|remove --yes`;
  `loadout import <file> --new [<name>]`. The last plan cannot be removed; names ignore case.
- Workspace, undo, versions (numbered per plan) and staging belong to a plan; `status`,
  `loadout log` and `stage list` name it. Share codes carry the plan name.
- The ModsConfig.xml write record is kept per user-data folder and names the writer (plan,
  version).
- The window: one sidebar entry per plan and + new list; new (empty, copy, a version, an
  import), rename, delete (confirmed; never the last) from the plan page's ⋯ menu, a
  right-click, `P` or the palette. The status bar says "in the game now:: <list> v<n>". The TUI
  has the same switcher on `P` and names the plan in its top bar.

### Outside changes (07 §4.3)
- A change made to ModsConfig.xml outside RimArcho goes to the plan that last wrote the file,
  in whichever environment shares the folder: loaded as an `external` version when that plan
  is untouched since, else recorded and marked. With no record it goes to the current plan.
- `archo loadout ignore-external` clears the mark; the version stays. The mark shows in
  `status`, `loadout log`, can it run (with "see the difference"), the sidebar and the plan page.

### Launching (09 §1, §2, §4)
- `archo launch` writes the plan to the game (links, ModsConfig.xml, a `launch` version unless
  the list equals the newest one) and then starts it; Steam or the game exe is found before
  anything is written.
- `launch --dry-run` and `save --dry-run` show one preview: links, path, version rewrite,
  entries and missing, plus the command line for launch.
- `archo save` works while the game runs and says the game reads it on its next start.
- One game at a time: a game running from any registered environment blocks `launch` and
  `pack launch`, naming the environment and the plan or pack. The window disables launch.
- Launch records carry the plan, the enabled list's fingerprint and the version played.
  A game in `RimWorld2` no longer counts as running in `RimWorld`.
- Launch, write to the game and sort act on one click or key in the window and the TUI; the
  status line says what happened (sort: undo / u). A refusal names its reason and lists the
  commands that resolve it. Loading a history version or an outside change is one undoable
  step, with no confirmation. The palette's preview the sort / preview save / preview the launch show the dry runs
  (16 §6).

### Locked versions (07 §5)
- `archo pin <instance> [--name] [--git] [--dry-run]` (`copy-local` is an alias) copies the
  mod into the lock library (`lock.dir`, default `%LOCALAPPDATA%\RimArcho\locked`, one folder
  per environment, scanned as source `src:locked`) and swaps it into the current plan. The game
  sees a copy only through an `_Archo_` link, so several copies coexist and each plan uses
  its own; no About.xml is renamed.
- `mod.pinned` covers RimArcho's copies too, and turns into "upstream has an update" when the upstream
  differs from `origin_snapshot`.
- `pin --update <instance>` makes a fresh copy and swaps it in; the old one stays.
  `pin --move-to-library` (and the first `--update`) moves an older RimArcho copy out of
  `Mods/` into the library — a rename on the same volume, else copy, verify, recycle.
- `unpin` on a RimArcho copy swaps the upstream back into the current plan.

### Logs (05 §3, 06 §4)
- A log belongs to the newest launch before it was written (else the plan that last wrote
  ModsConfig.xml; else "source unknown"). It is stale for another game version, another
  plan, environment or pack, or a list changed since that launch; the header says why.
  ModsConfig.xml's modification time is no longer used.
- Analysis is kept per plan's enabled set: switching plans and back does not re-run it.
- "analyse this run's log" shows in the sidebar foot and the TUI top bar when Player.log is newer than
  the last launch and the log last seen.
- `archo log` (MCP `analyze_log`) prints a cursor; `--since <cursor>` (MCP `since`) lists only
  the clusters that are new (`new`) or whose count grew (`+N`) since then, ArchoMonitor.log
  included, for an agent polling a running game (06 §6). When the game restarted, the cursor
  is called stale and the old run is Player-prev.log (`--prev --since`).
- The first query on a new log is fast again (18): on corpus logs 1.1–14 s → 128–330 ms,
  where it was 568 ms on the reference log. Looking defs up by `Name` has an index (schema v15);
  the files mentioning each def name are indexed once per enabled set (`cache/refindex/`, built
  after an analysis), so cross-reference referrers no longer read every mod file; repeated log
  entries are classified once. Referrer evidence is listed by file.

### Search (16 §3.7)
- `tag:` matches the start of any word in a tag: `tag:facial` finds Facial Hair and Facial
  Animation, `tag:animation` finds Facial Animation.
- From the second letter, grey ghost text completes field names (`au` → `author:`) and
  values after `tag:`, `type:` and `has:`; Tab (or → in the GUI) takes it, otherwise Tab
  still jumps to the next list with matches.

### MCP (13 §1.2, 14 §2)
- New tools: `manage_packs`, `ignore_problem`, `resolve_identity`, `scan`, `exchange_list`,
  `manage_rules` (`add_rule` stays as an alias), `manage_hidden`, `maintenance`, `pin_mod`
  (`copy_local` stays as an alias), `unpin_mod`; `status` gains `show` pages (envs,
  settings, data, db, cache); `launch_game`, `manage_plans`, `plan_history`;
  `edit_loadout` gains `checkout` and `ignore_external`; `list_library`, `manage_library`;
  `preview_uninstall` (read-only; there is no tool that uninstalls); `search_files`,
  `test_run`, `check_links`, `repair_links` (below); `manage_textures` (texture optimisation) and
  `manage_subscriptions` (Subscriptions). 40 tools in all.
- `manage_plans` gains `versions`, `follow`, `unfollow` and `sync`; `manage_packs` gains
  `check`.
- `exchange_list`, `maintenance` and `manage_subscriptions` are open-world; the destructive
  ones (`save_loadout`, `launch_game`, `manage_plans`, `manage_packs`, `resolve_identity`,
  `manage_hidden`, `manage_library`, `manage_subscriptions`, `manage_textures`,
  `repair_links`, `maintenance`) are listed in the server instructions with "show the dry
  run and ask first".
- Instructions and descriptions speak of the current plan, writing to the game and locked
  versions.

### Uninstall (02 §2)
- `archo uninstall` shows what RimArcho put on the machine and what happens to each piece;
  `--yes` runs it. Every row of the write list (02 §1) has an uninstall step or a stated
  reason; a test keeps the two in step.
- RimArcho's `_Archo_` / `!ArchoPack_` links leave every game's `Mods/` (never their targets;
  a same-named folder that is not a link stays and is listed). Each ModsConfig.xml RimArcho
  wrote keeps the player's list without Archo Monitor and the mods only reached through a
  removed link, and loses `.rimarcho.bak` (`--restore-modsconfig-backup` puts the backup back
  instead). Monitor and the SteamCMD RimArcho downloaded are deleted.
- Choices, defaults first: lock library copies to the recycle bin (`--keep-locked
  [--locked-to <env>]`: into a game's `Mods/` as ordinary mods); packs kept, moved out of the
  data folder (`--remove-packs`, `--packs-to`); own save folders and save backups moved out
  (`--remove-saves`, `--saves-to`; default `Documents\RimArcho saves`); downloaded games kept
  (`--remove-games`). Refused while a game or another RimArcho from the same folder runs.
- The data folders and the program go last, by a script that waits for RimArcho to close; it
  deletes only the release zip's files, the program folder only when empty, and lists what it
  could not delete in `%TEMP%\RimArcho-uninstall.log`. A failed step keeps them, so it can
  run again.
- GUI settings › uninstall RimArcho: the plan with the questions inline, one button, progress, the
  report and close RimArcho; TUI palette uninstall RimArcho asks for the word `uninstall`; MCP
  `preview_uninstall` is the dry run only.

### mod library (16 §2.9)
- `archo library` lists every mod on this machine, one line per Lineage (copies, size,
  the most important state, the plans using it), with `--search`, `--include` /
  `--exclude key:value` (tag, source, version, state, used), `--sort name|size|changed|played`,
  `--facets` and `--lineage <ref>` for every copy's path, states, plans and last launch.
  "Used in" covers every environment of the same game and the packs; states include
  subscribed items with no folder, unsubscribed leftovers, copies the game ignores and
  locked versions whose upstream changed.
- `archo library move-to-locked <ref> [--dry-run]` moves a spare copy out of `Mods/` into the
  lock library; `archo library remove-from-plans <ref…>` takes mods out of every plan (one
  undo step each). Moving now keeps the instance (`pin --move-to-library` too), so every plan
  keeps it. `trash` refuses while any plan uses the folder and names the plans.
- MCP: `list_library` (read-only) and `manage_library` (destructive).
- The window's mod library › all: filters and a table like a file manager's details view.
  Click a header to sort (again reverses, with an arrow), drag it to move the column, drag
  its edge to resize, right-click the headers to show or hide columns (name size state source
  used in by default; update date version author tags last played on request); the layout is kept in
  `ui.gui.library.columns`. The name cell holds only the name, the copy's kind is in source,
  tags are not in rows. Rows show only states to act on (hidden, leftover, upstream
  updated, unloadable, missing, ignored). does not support 1.7 and not in any list are not states at all:
  not in the filters, not `state:` values or completions, not in the CLI / MCP (`state:unsupported`
  and `state:unused` are gone; `used:none` still finds copies no plan uses).
  The filters mark the game's version and add update date (within a week / within a month / within half a year / earlier,
  the Workshop update time, else the folder's). No environment chip on this page; the
  button says rescan. A selection action bar; the TUI has the list in the palette.
- A copy's details: its note at the top, then only the actions that apply — restore, unsubscribe /
  delete, resubscribe, back up locked version, open the folder, open the web page — then one field list. delete now also takes the player's own copies in `Mods/` and the lock library
  (not links, not while a plan uses them).
- Copy notes (schema v13 `instance_note`): each copy has its own note, apart from the mod's.
  RimArcho writes the first version from how the copy was made ("this locked version was created on October 21,
  for list 'test'; upstream has had 2 updates since creation, reason: …"), worked out each time so the update
  count stays current. Copies RimArcho did not make get one too: where they came from, the
  day RimArcho first saw them and, beside a Workshop copy, how often it updated since. Edit it in the window (click), the TUI (`n` in mod library), the CLI
  (`archo note <ref> <text> --copy`, `--clear`) or MCP (`set_player_data` `copy_note`);
  `pin --reason`, `library move-to-locked --reason` and the MCP `reason` record why.
- unsubscribe and resubscribe go through the running Steam client, as RimCrow does (11 §2): a
  short-lived `archo steam-worker` process, started in `%LOCALAPPDATA%\RimArcho\steam` with
  the game's `steam_api64.dll` copied in and `steam_appid.txt`, calls the Steamworks flat API
  (no cgo) and exits. Unsubscribing makes Steam delete the folder, so it shows the dry run
  first and is refused while the mod is enabled, used by a plan, or the game runs. Only when
  Steam is not running or the DLL cannot be loaded does the window open the Workshop page
  instead, and say why. CLI `archo workshop unsubscribe|subscribe|state <ref…>`; MCP
  `manage_library` actions `unsubscribe`, `subscribe`, `steam_state`. Without Steam,
  `workshop state` / `steam_state` show whether each item's local folder is there, marked
  local, with the subscription and update state unknown (never "not subscribed");
  subscribing and unsubscribing still need Steam and say so.
- `archo library --sort` takes every column (name, size, changed, played, state, source,
  used, updated, version, author, tags; a leading `-` reverses) and `--include updated:week`
  (month, half-year, older); `--lineage` lists each copy's `updated:` and `note:`.
- RimArcho's own copy note is in the normal text colour, like the player's; only the small
  "written by RimArcho · click to edit" line stays muted.
- A detail's history keeps the newest three changes and links to see all in change history: changes narrowed to
  that mod's Lineage, every scan that changed one of its copies, with a removable "only <name>"
  (× or `esc` shows every mod again). The TUI opens it with `C`.
- `ctrl+o` opens the cursor's mod's folder and `ctrl+w` its Workshop page, in the plan editor
  and mod library, in both interfaces; without a Workshop ID the status line says so.

### One interface for the plan editor and mod library (16 §2.5, §2.9, §3, §8.5)
- One filter bar on both pages: a search box (the grey completion and `tag:` word matching
  as before, now with `version:`), the filters in force as removable chips, and filters (`\`).
  Open, it shows one line per group (tags, source, version with the current game marked, content and has
  in the editor; mod library adds state, used in, update date): the values most used first with counts,
  then a box to type in for the rest. Left click (enter, space) keeps a value, right click
  (`x`) leaves it out; values of one group are "or", groups "and". The keyboard reaches all of
  it (↑↓ groups, ←→ values, a letter types into the group's box, backspace in an empty search
  takes the last chip off).
- The editor's bar sits above Disabled and filters Disabled and Enabled together; the heads say
  "Enabled 6 / 34". Staging keeps its own search. While the bar filters, `J` / `K` (and alt+) move
  past the next row shown (`view.StepAt`); `m`, `space` and drops still place by the whole
  load order.
- mod library loses its filter column: the bar is over the table, and a copy's details are a
  permanent right-hand column (the editor's detail card) that follows the cursor; `enter`
  lays them over the page, `enter` or `esc` closes them, `→` on a copy focuses them.
- One key table on both pages (16 §3.5), in both interfaces: `PgUp` / `PgDn` move the cursor
  (they scroll only the focused details), `ctrl+a` selects everything shown, `enter` opens /
  closes the details, `esc` backs out (details → selection → search and filters), `space`
  takes effect / stops (enable / disable in the editor; add to the current list / remove from the current list / switch to this copy in
  mod library), `m`, `t`, `x`, `g`, `n` (the mod's note in the editor, the copy's in mod library),
  `ctrl+o` / `ctrl+w`, `u` / `ctrl+z`, `ctrl+y` / `ctrl+r` (mod library's undo is the plan's),
  `Del` (unsubscribe / delete, dry run first). `tab` / `shift+tab` switch the pages under one sidebar
  entry (all ↔ changes, the plans); the editor's areas are `←→` and `1`–`4` only. `#`, `o`, `d`,
  `f` and `!` are no longer shortcuts (`shift+1`–`7` go to the sidebar's pages). `?` shows the
  keys of the page it is pressed on (`view.KeyHelp`).
- One row menu on both pages, from one op table (`view.OpTable`, `view.CopyOps`): list
  (space, m, t, x, remove from all lists), organize (tag g, note n), open (ctrl+o, ctrl+w), version (lock this
  version, update the locked version, unlock, back up locked version), disk (restore, resubscribe, then unsubscribe / delete in red),
  each with its key, only what applies. The editor's menu now has the version and disk
  ops too.
- New rule (16 §3.1 rule 6): everything the mouse can do, the keyboard can do. The menu key
  and `shift+F10` open the row's menu at the cursor (↑↓, enter, esc); `H` lists the history,
  `C` a mod's changes, `<` `>` `=` set the editor's column widths, `s` in mod library is the
  headers' menu (sort, move, size, show columns). A test checks every op has a key or the
  menu and a runner on both pages.
- mod library's page data is 0.77 MB instead of 1.47 MB on a 1,284-mod library: each copy
  carries op codes, the labels come once, and folder paths and Workshop URLs stay in Go.

### changes (16 §2.9, §2.3)
- One row per mod: its changes in the time range merged into "name · what it means", with a
  tier pill (affects launch / worth a look / content only), sorted by tier then name. The sentence comes only from
  what the scans recorded: supported versions added or dropped (the game's own version
  flagged), the mod's version, dependencies and incompatibilities with their installed /
  enabled state, load rules (compared from the two declarations), files grouped as code / Defs /
  Patches / About / textures / sounds / translations / other with size changes, the Workshop
  update time, and a locked version left behind.
- Default: the current plan's mods since that plan was last played (the last scan when it never
  was); `a` / the two buttons switch to the whole library. Older changes fold under earlier by
  day; new and gone mods are one summary row. One mod's history (the detail's link, TUI `C`)
  lists every change by scan.
- Opening a row shows the grouped file summary first, then the raw file list behind `i`, then
  its actions: open the web page (the Workshop change notes, `ctrl+w`), see dependencies (`D`), sort (`S`),
  lock this one (`p`, dry run first). `tab` goes to all.
- `archo changes` prints the same page in English (`--library`, `--since`, a ref for one mod's
  history); MCP `list_changes` gains `library`. The old per-event listing is gone. The builder
  is the new `internal/modchanges`, shared by the GUI, the TUI, the CLI and MCP.
- Home page: the "since the last scan" line is replaced by one sentence about the current plan
  ("since this list was last played · 5 mods updated, 1 needs attention →"), which opens changes with that scope and
  time. The last game line shows only the result ("last game · October 21 · 3 errors"), reads a newer
  Player.log by itself when the page loads or the window regains focus, and is hidden when there
  was never a run.

### The window (16 §2)
- One full detail page (16 §4.2): `enter` in the plan editor and in mod library › all opens the
  same page. The mod on the left (picture, name, author, state, tags, description); its
  copies (state, size, version, folder, note, and the selected copy's actions), where each is
  used with the current plan's `space`, the fields and the history on the right; technical information under
  both, one tab per section of `view.Sections`. `← →` the previous / next mod of the list it
  came from, `esc` back there, `[ ]` another copy, the row menu and copy keys on that copy,
  `1`–`6` its parts. The editor's detail column and mod library's details stay as its short form.
- `1`–`9` focus a page's areas on every page, as `1`–`4` do in the editor, and each area's
  heading shows its number: mod library (filter, table, details), can it run's sections, game log (the
  errors, the log text), games, settings's groups, the build page's parts. The TUI's can it run and
  mod library do the same.
- texture optimisation is a page of its own under library, after packs (`shift+7`; games is now `shift+8`, `*` in
  the TUI, where `shift+7` shows the textures panel). settings keeps only where todds is.
- A sidebar replaces the top tabs, in two groups: playing (start, can it run with its badge,
  game log) and library (mod library › changes, list, packs, games), then settings (`shift+1`–`7`).
  Only can it run shows a number, and it leaves out hidden mods. Drag the sidebar's right
  edge to change its width (double-click resets it).
  It collapses, and starts collapsed below 760 px.
- The top bar has a clickable breadcrumb, the environment chip and the page's actions.
- A home page: each plan and pack, newest played first, with its state (another plan's from
  its stored analysis) and a launch button, and the last run and last scan's changes.
- A first-run guide: find games (several at once), scan them, start from the game's list, an
  import, a pack or a Steam collection. games › add game opens it.
- The window follows the new design: no page title under the breadcrumb (it names the page)
  but one muted line saying what the page is for; the environment chip sits by the page name
  (plan and environment on can it run); page actions are quiet icon + text buttons. Home rows
  have a fixed environment column, a list / pack tag, a meta line (mods, follows Workshop
  updates / versions locked, last played) and a state chip; the activity is a row of cards.
  The sidebar shows quiet counts (plans, packs, environments, the library).
- can it run is one column of four sections by consequence (breaks the game, errors from the last run,
  for you to confirm, hidden Mod). Each item is a card: a bold line, a plain sentence, an evidence
  chip (declared by the mod / inferred / log / community data and the source) and one primary action, the
  rest under ⋯; ← → jump between sections. The view model carries `Section`, `Title`,
  `Why` and `Evidence` for both interfaces. A mod's missing inherited definitions, and its
  patches that miss the same disabled target, are now one item each (the count drops).
- Motion (16 §7.1): three duration tokens (120 / 180 / 260 ms); pages fade in with a 4px
  rise; the sidebar's highlight slides; list edits slide the visible rows to their new
  place and flash the changed ones; badges and counts bump; the status line slides in;
  lazily loaded parts shimmer. Only opacity and transform move, and nothing does under
  prefers-reduced-motion.
- Polish after a side-by-side with magpie: the sidebar takes magpie's sizes and edges (212 px,
  30 px entries, 14 px icons, small group titles), the open entry is raised, sub-entries hang
  16 px in on a guide line with their mod counts and their own sliding highlight, and they
  open and close smoothly (rows 0fr → 1fr, a turning chevron). Less always-on chrome: the
  sidebar's environment / "game not running" box is gone (one line shows only while a game runs or
  a new log waits), and the status line no longer says "in the game now:…" (with several plans, a
  green dot on that plan's entry) nor repeats the breadcrumb; the sidebar has no empty head.
  The home page lists the plans and packs of every environment: what is applied now is one
  large card (environment · version, name [list / pack], mods · last played, the problem
  if any, ▶ launch the game); every other one is a line on shared columns (game versions — a pack's
  from its mods' supportedVersions —, name, kind, mods, apply, which switches to it without
  writing game files); the last run and scan are two lines. No page actions on home. The plan editor has three columns,
  Disabled | Enabled | details, with Staging and history as folds under details (drag to a folded Staging still
  stages; `3` opens it); two splitters, old four-column widths carried over.
- start is laid out like the game's main menu over a background image (16 §2.3): the plans and
  packs down the left (kind first, then the name; the applied one raised with edit ↵, the
  others with apply ↵ on the cursor), the last run and scan under them, ▶ launch the game in the
  bottom right with the problem as a tab under it. `↑` `↓`, `enter` (apply / edit), `L`.
- settings › launch page background (16 §2.14): the game's art (RimWorld's planet by default, the DLC
  covers), the images of every mod with "background" in its name (their
  `VBE.BackgroundImageDef`s: Vanilla Backgrounds Expanded, Shella Backgrounds, …), your own
  PNG / JPG, or plain; dimming, blur, focus, and rotation among starred images, with a small
  live preview. Each image is decoded once, one at a time, into 1920 / 480 px JPEGs in
  `cache/backgrounds/`; the page never loads an original, and drops the image when you leave.
- "unsaved N" is now "not written N" in both interfaces. The old log › manager actions view is gone;
  the history stays in the editor's third column.
- Switching the language in settings happens in place: the page keeps its place, the lists their
  selection and scroll, the details stay open; it no longer reloads to the home page.
- The plan editor: sort (`S`) is in the Enabled column's header, import / export (`i`) in the
  toolbar. Rows no longer show tags, in any column or interface (they are in the detail and
  `tag:` search, 16 §3.8). The detail is one field list — Archo ID, package ID, location [open the location],
  workshop [opens the page], tag [edit], languages, dependency, conflicts with — shared with mod library; clicking Archo ID,
  package ID, location or the Workshop ID copies it and says copied (16 §4.1); the list has no
  background and long values wrap. The TUI detail draws the same list.
- The TUI's can it run has the GUI's four sections, each heading with its count (and which run
  the run errors are from), and every item's evidence chip on the line under it: an icon
  (Nerd Font or the compatible set), the kind and the source, e.g. `≣ declared by the mod · About.xml
  modDependencies`. ← → jump between sections. The top bar's can it run N now counts this
  run's errors too, the same number as the GUI's sidebar badge (G13).
- The running game is at the GUI sidebar's foot and, in the TUI, at the right end of the
  status line (no longer in the top bar): `● game running · since 14:05` with this run's errors as
  an amber number, `● analyse this run's log` after the game when a new log waits, else nothing (G17).
- `esc` brings every page back to its base (no popup or menu, nothing selected, the filter
  bar closed) in at most 3 presses, checked on every page of both interfaces (G5, T20). In the
  window, a row menu or ⋯ menu opened with the mouse now closes with `esc`, and `esc` on the
  rows of the editor or mod library clears and closes an open filter bar in one press.
- The navigation sits at the side or at the top: the toggle (heading the sidebar, or the
  strip's left end; `ctrl+b`) moves it, and `ui.gui.nav` remembers it. At the top the same
  entries form one strip, groups as thin separators, sub-entries as a dropdown from the
  chevron (`↓` from the keyboard), settings and the running game at the right end; it scrolls
  when the window is narrow. The change is a view transition. (This replaces the expanded /
  icon rail / hidden sidebar made the same day.)
- The top bar has no buttons on any page, only the breadcrumb and the environment chip. Page
  actions sit at the right end of the page's description line; the plan editor has that line
  too ("enabled: 34 items, in the order the game loads them") with its undo, import / export, build a pack, write to the
  game, ⋯ and launch the game.
- In the window, the editor's launch the game (button, `L`, the palette's start) opens the start page with
  the current plan applied and the focus on ▶ launch the game; `L` or enter there starts the game. The
  TUI's `L` and the CLI / MCP `launch` still launch at once.
- The gaps between cards are 4 px everywhere (`--card-gap`): the editor's columns and its
  stacked details / Staging / history, the filter card and what is under it, mod library's two sides, the
  log pages' columns. The draggable gaps keep a 10 px grab area that reaches over the cards'
  edges.
- New icon artwork: the portrait with the accent rim light (1024×1024). 48 px and up show it
  whole; 40 px and below show the head (hair and face) instead of the face alone (16 §8.6).
  The icon renderer now draws groups with inherited fill and stroke, transforms, clip paths,
  radial and bounding-box gradients, evenodd fills and Gaussian blur, and skips what is only
  defined (defs, clip paths, gradients, filters).
- The bow from the portrait is RimArcho's mark (16 §7): in front of Archo's name on the log
  page's Archo log analysis card and the ask Archo analyze button. Brand colours: a `#4f46e5`
  rim in the light theme; a `#a5b4fc` rim and a grey outline in the dark theme.

### Memory (17 §7.3)
- settings › interface has GPU acceleration (`config set ui.gpu on|off`, default on). Off draws the window in
  software and saves about 20–35 MB; it takes effect the next time RimArcho opens.
- The window's Go process returns freed memory to the system 2 seconds after heavy work, and
  forgets a page's caches when the player leaves it (mod library, needs attention / log, home); a 32 MiB
  soft cap (now 128 MiB, below) and a 256 KiB SQLite page cache as in the TUI. Idle 85 → 22 MB.
- The soft cap went from 32 MiB to 128 MiB: at 32 MiB a list edit ran garbage collection
  dozens to hundreds of times and was 2–3× slower. Idle memory still comes from the explicit
  release after 2 seconds.
- The window icon is no longer drawn at 256 px: not at all when the exe carries its icon
  (release builds), at the system icon size otherwise. Start-up peak 122 → 67 MB.
- WebView2 runs its network service inside the browser process (one process fewer), and its
  memory target is low while the window is minimised.
- Pack names are checked without a regular expression (about 1.4 MB less).
- Measured on the same data copy, GPU on: idle 202 → 138 MB, plan editor 204 → 138,
  mod library 175 → 151, minimised 171 → 105. GPU off: 182 → 123, 185 → 123, 155 → 132, 150 → 88.

### Packs (16 §2.6, §2.7, 10 §3, 02 §3)
- The recycle bin has one entry that checks the allow-list (02 §3) for every caller: a mod
  folder inside a source folder (not the folder itself), a pack directly in the pack folder,
  saves (a game's `Saves/`, own save folders, save backups) and a game downloaded with
  SteamCMD. A link, or any other path, is refused whoever asks (`trash`, `pack remove`,
  mod library, moving copies to the lock library, uninstall).
- create a pack is the current plan's own page, laid out as drawn: the name, the four things building does (check, leave out, lock, put in place) with their
  numbers, and a summary (environment, mods, size before → after, own saves, location) with
  build and see the list first, without writing to disk (the text of `pack build --dry-run`). With error-level problems
  build is disabled and says fix first: N problems →, which opens can it run (G14); not enough space, an
  unreadable mod, a bad or taken name and a build in progress disable it too. The build's
  progress shows on the four steps with stop; a finished build opens its pack. Builds cut
  short are listed below. Keys: `B` build, `V` the dry run, `n` the name,
  `esc` back to the plan. The old build popup is gone.
- Each pack is a sub-entry under packs with a page of its own: what it is, start (`L`), export
  (`e`, a save dialog) and move to the recycle bin (`Del`), and its mods in load order with their sizes and
  the version folders left out. `tab` / `shift+tab` go from pack to pack (from the list: the
  first one); the list opens a pack with `enter`.
- `Mods/` never gets two copies of one packageId from RimArcho (the game logs "Tried loading
  mod with the same packageId multiple times" and ignores one; the player's 2026-10-21 log
  had six, from launching the pack test next to RimCrow-style copies). Launching a pack uses
  the copy already in `Mods/` when it is the same version (listed as "from Mods/"), refuses
  and names the folders when it is another version, and drops a plan's `_Archo_` link of the
  same package for the run. Writing a plan refuses a new `_Archo_` link next to another copy
  in `Mods/`. Such pairs already there are `pkg.duplicate-mods` errors in can it run › breaks the game,
  whoever made them; the fix only ever changes RimArcho's own links or the plan, never the
  player's folders. Pack links that duplicate a copy raise `pack.links-left` to an error.
- Another version in the way (09 §3, 10 §3): when a pack or a plan's locked copy needs one
  version and `Mods/` holds a plain folder with another, RimArcho offers to move that folder
  into the lock library and go on, with one confirmation listing what moves where and what is
  linked instead. Plans that used the folder keep it (it comes back through a link). The
  refusal names the fix: `--move-duplicates` on `save`, `launch` and `pack launch` (MCP
  `move_duplicates`), or `archo library move-to-locked <ref>`. Links, folders another tool's
  link points at, and folders with links pointing outside never move. `pkg.duplicate-mods`
  gets a move to the lock library action for the plain folder.
- A pack's `!ArchoPack_` links left in `Mods/` while the pack is not being played (a game
  started from Steam would load the pack's copies) are a problem of the plan,
  `pack.links-left`: in can it run › for you to confirm with remove the links, in the tab under launch the game, in
  `archo problems` and MCP `list_problems`. `archo pack unlink [--dry-run]` / MCP
  `manage_packs action=unlink` remove only the links; refused while a game runs.
- `.archopack` format 2 (10 §1, §4): tar + zstd with `pack.json` first, a SHA-256 digest
  per copy and a whitelist that never lets game files in. Import verifies every copy.
  `archo pack check <name|file>` (MCP `manage_packs action=check`) verifies a pack's copies,
  or an archive without extracting it.
- A pack is compared with the game it is imported into or launched on (10 §5): game build,
  DLC and mods. The import result says whether every copy matched and lists the differences;
  launching refuses only on errors.
- The RimArcho list (`loadout export --format archo`, `.archolist.json`) shares a list with
  each mod's version; every list import reads it, takes the installed copy with the listed
  content and marks mods installed in another version. A pack's page exports its list (`E`).
- Packs remember the list they were made from.

### texture optimisation (10 §2)
- Database schema v15 → v16: the DDS RimArcho wrote, per file and per mod.
- `archo textures status|optimize|clean|tool`: converts a mod's PNG textures to DDS next to
  them with todds 0.4.1 (BC1 when opaque, BC7 otherwise, with mipmaps); the PNGs stay. Every
  folder with `Textures/` is covered, and a path is converted everywhere or nowhere. Only the
  files RimArcho recorded are ever deleted; an author's own DDS is left alone. Refused while
  the game runs; can be stopped.
- todds is downloaded only after the player agrees (`tool --install`, settings), or
  `textures.todds` points at an existing copy (e.g. RimSort's).
- A mod updated after its textures were converted shows `texture.stale` in needs attention.
- Uninstall gains a `textures` step that deletes the recorded DDS.
- `pack build --textures [--texture-scale N]`: pre-convert (full size) or slim (scaled) in the
  pack's copies, which then drop their PNGs. Space is checked against the peak; `pack.json`
  records what was done.
- Opening texture optimisation or create a pack no longer reads every texture: count the textures does, and can be
  stopped. Changing the scope or scale keeps the last numbers and says to count again;
  converting works without counting (only new or changed PNGs).
- GUI: a texture optimisation page (estimate, scale, progress, stop, clean), mod library's convert textures to DDS /
  remove the converted textures, the pack build's textures choice on create a pack (its own step textures: off / pre-convert /
  slim 80–25%, with the PNGs converted, the size after and the peak while converting; `D` steps
  through it; without todds the step says so and build points to settings › texture optimisation); TUI palette panel; MCP `manage_textures`.

### Log corpus (05 §4.3, §4.3.1, §5.3, 06 §3, §8; 08 §1; 09 §2)
The research on 6,734 public 1.6 logs (docs/research/), merged from `master`.
- **Severities from the corpus** (G8): each code graded by how often its mods are error
  sources in the same log against chance; see Breaking. `official.order`'s first suggestion is
  the cheapest `archo move` (a DLC enabled late goes back after the previous official package).
- **Constraint strength** (G2, `internal/order/strength.go`, `observed.json`): dependencies,
  loadBefore, forced orders and user rules are strong (warning); loadAfter and community rules
  weak (info); rules the corpus saw broken without effect harmless (info, with the counts and an
  `ignore` suggestion). Constraints that cannot be satisfied (two official packages against the
  official order, a "load before Core" that closes a cycle) are left out and listed under
  "not used" in `archo sort`.
- **Before you play** (`app.LaunchChecks`): `archo launch` and its dry run list the problems
  that most often end in red errors (`patch.no-match`, `def.parent-missing`, `incompat.active`,
  `dep.inactive`, `dep.missing`, `dep.observed-missing`, `combo.known-bad`), one row per code
  with why (the corpus numbers) and fix. It never blocks the launch.
- **The log corpus layer** (G10, `internal/known`): `dep.observed-missing` (an undeclared
  prerequisite seen in shared logs), `combo.known-bad` (a pair or a version that errors),
  `mod.known-error` (a mod's own error), evidence `community`.
- **Manual Harmony targets** (G9, `internal/harmony/manual.go`, `internal/dotnet/il.go`):
  `Harmony.Patch` calls and `(MethodBase, HarmonyMethod, …)` wrappers are read from IL; resolved
  targets are ordinary patches, unresolved ones say why. `HugsLib.dll` is read in its own mod.
- **Log attribution** (G1/G7): callers of failing code, HugsLib's log upload, Harmony 1.x
  references, field errors with their Context, XML Extensions blocks, absolute file paths,
  texture providers, missing assemblies loaded reflection-only, save-load holders. The cluster
  view shows the full message, its extra lines and the messages around it (`nearby`). The
  referrer search keeps using the reference index (06 §5).
- **Error patterns** (G5/G11, `internal/patterns`): `[pattern:g:<id>]` on cluster lines, a
  `pattern:` section in the cluster view (community background: usual causes, route, fixes,
  repeat-upload evidence), `archo pattern` and MCP `analyze_log pattern=`; MCP Instructions end
  with a checklist for save loading, missing assets, config errors, assembly loading and noise,
  and say tool results are capped at 24 KB.
- Privacy: pattern templates with HugsLib's `[Rimworld_dir]` paths are dropped (2 of 1,036).
- **Data from 40,326 logs** (corpus expansion): `internal/patterns` now has 3,947 mod-specific
  errors seen in at least 25 logs (was 1,034 from at least 10) and `internal/known` 95 prerequisites and 486 combinations (was 38 and 180),
  same thresholds. Severities (G8) and constraint strength (G2) keep their 6,734-log measurements.
- **In the pages** (16 §2.3, §2.4, §2.5, §2.10, §4.2; TUI the same):
  - start: the tab under launch the game names the most likely trouble (the first `LaunchChecks` code
    with an item in can it run) and its one-line reason with the corpus numbers; it opens
    can it run and never blocks. The TUI's `L` says it on the status line (“worth a look: …”).
  - can it run: a community logs evidence chip with the measured share (“without it 40/50 (80%) errored”);
    `dep.observed-missing` with the dependencies, `combo.known-bad` with the conflicts,
    `mod.known-error` a dimmed “this is the mod's own problem” note. Weak and harmless order
    preferences are one dimmed, uncounted item; counts and the sidebar badge leave them out.
  - game log (and the Archo Monitor view): a cluster's detail shows the whole message, extra
    lines and nearby messages (none for an entry only ArchoMonitor.log has), then a pattern card (“this kind of error usually means…”, common causes, next
    step, kinds of fix) marked as community background, only when a pattern matches.
  - list editor › sort: the status line counts the strong requirements the sort put right; the
    sort preview (`archo sort`) has a `why` section, strong / weak / harmless, strong first.
  - Mod detail › technical information › Harmony (`archo harmony --mod`): patches from `Harmony.Patch`
    calls are marked, and every unresolved call is listed with its reason.

### Background tasks (16 §2.16)
- mod library's file and Steam actions are queued once confirmed: two queues (files, Steam), one
  failure does not stop the rest, and each item's result is kept (`<data>/tasks.json`).
  Scans, checks, pack builds, texture work, game downloads and the long actions of the
  settings page and the guide are recorded as tasks too.
- background tasks page: a line at the bottom of the sidebar and `ctrl+j`. The CLI and MCP still run
  in the foreground.
- The TUI queues its subscription actions (subscribe, check, update, remove) into the same
  queues and `tasks.json`; the status line says when one is queued and how it ended. Its
  other mod library file actions still run in the foreground.
- The window stays usable during long work (16 §3.1 rule 9): adding a large game, switching
  games, the first scan, folder sizes, and the settings page's downloads and database actions
  run in the background, and a page opens before its data is read; stale results are dropped.

### Interface rules (16 §3.1, §3.9, §7.3)
- Popups are only for warnings: new list, import, the build checklist and loading an older
  version happen in the page. A filter bar folds away when it loses focus.
- Packs, a pack's mods and the texture list are data tables. The gaps between cards can be
  dragged or moved with the keyboard, and their sizes are kept.
- Two corner radii, four font sizes no smaller than 12 px, hairlines, shared motion timings; no
  left colour bars (can it run, the TUI, selected rows), no `·` in interface text, no purpose
  lines under page titles. `TestStyleRules` and `TestWording` check this.
- Every path shown in the window copies on click, by mouse or keyboard (rule 12); text never
  overlaps the controls next to it.
- Back and forward between pages (rule 11): the mouse side buttons, `alt+←` / `alt+→` and
  BrowserBack / BrowserForward share one session history of the last 50 pages; back first
  closes a menu or popup. Deleted lists, other games' pages and finished guides are skipped.
  The TUI keeps its own page switching (16 §8.5).
- start is a grid: the current list or pack as the title, then 01 game → 02 list / pack → 03
  launch. Choosing applies it, choosing again opens it; a switch shows its progress on the
  launch block instead of a popup. The launch tab's problems can be folded away until they
  change. The page shows the game's full version (e.g. 1.6.4871 rev590).
- RimWorld 1.6's own main-menu background is read from the game's Unity assets in the
  background and cached, falling back to the official preview image.
- settings › interface: an accent colour (`ui.accent`, the 15 Radix Colors scales or default), applied at
  once in light and dark. The native title bar and the launch backdrop follow the theme.

### Lists (07 §1.6, §7.6; 16 §2.17)
- list management (the sidebar's lists › manage, `shift+5`): the lists on the left; on the right a list's
  game versions, the Steam collections it follows and sharing, with an export sub-page
  (`e` in the editor opens it too). New lists are created in the page, and unfinished drafts
  are kept.
- A list's game versions (`archo plan versions [<name> [<versions…>|default]]`): by default the
  versions every mod supports plus the game it was made on, as one range. Copies of the list
  and RimArcho list exports carry them. When the current game is not among them, can it run's
  for you to confirm says so (add 1.7, or go to list management).
- Following a Steam collection (`archo plan follow <name> <link|ID> [--mode add|follow]`,
  `plan unfollow`, `plan sync`; `collection --import --follow`): RimArcho keeps a snapshot and
  tells you what changed since; it never changes the list by itself. Catching up is one undo
  step: `add` enables the new installed mods, `follow` also disables the ones the collection
  dropped. The network is used only when asked (list management checks at most every 10 minutes).
- Export format workshop links: Markdown Workshop links, read back by Workshop ID on import.
- Load cost (`archo plan cost [<plan>] [--mods] [--json]`, list management): a RAM and VRAM range for a
  list from its textures, sounds, XML and assemblies. 1.7 is calibrated on the real game;
  1.6 follows the loading rules only and says so. Results are cached per mod.
- Sorting puts the installed base game back when it is missing; the Disabled area lists Core and
  the DLCs first.

### Subscriptions (11 §1, 16 §2.9)
- Mods from a Git repository (`archo subscribe git <url> [--branch|--tag|--commit]`), from a
  download page such as LoversLab (`subscribe page <url> [--git] [--from zip|folder]`: the
  page's Git repository if it has one, else the files the player downloaded; never downloaded
  automatically), and Workshop items through Steam with SteamCMD as the fallback when the
  Steam client is not available (`subscribe workshop <id|url…>`). Where a mod came from is
  kept in `.rimarcho/source.json`. `subscribe list|check|update|remove`; updates are refused
  while the game runs; removing warns when lists use the mod.
- mod library › subscriptions and updates replaces changes: a scope bar (lists / all mods), one list of waiting for you to update then
  already updated (the change records), new and gone, earlier; a source summary line at the bottom;
  the selected item's details (update, choose files, open the source page, lock). mod library ›
  add a subscription takes a pasted link, recognises Git / LoversLab / another page / Workshop, asks one
  question and shows the dry run.
- mod library gains a source column, the source-update state, fields and actions; MCP
  `list_library` knows the new sources and states.
- MCP `manage_subscriptions` maps onto `archo subscribe` (list, add, check, update,
  remove). `add` recognises the source from `url` as the paste box does, or takes `kind`.
  An agent never logs in to LoversLab: it asks the player to download and passes `from`.
- TUI: mod library › add a subscription and the subscriptions and updates keys `K`, `enter` on a source, `u`, `U`, `Del`,
  `+` and paste, queued as background tasks. A page update or a page without a single Git
  repository points to the window or `--from`.

### Accounts (11 §4)
- settings › accounts: one status line each for SteamCMD and LoversLab (log in, log in again, log out).
  SteamCMD's login runs in SteamCMD's own console and is verified, and RimWorld ownership is
  checked. LoversLab signs in in a separate WebView2 window (its own process, a throwaway
  profile); only the site's cookies are kept, encrypted for the current Windows user, so
  downloads that need a login work. Passwords are never asked for or stored. Uninstall
  forgets the accounts first.

### Searching files (05 §5.5, 13 §4)
- `archo files search <pattern>` / MCP `search_files` search only what the game loads: the
  enabled copies in load order, the version or LoadFolders branch for this game version, and
  the content folders (Defs, Patches, About; Languages, Textures, Sounds, Assemblies when
  asked). Results are grouped by mod and file, carry the enclosing def or patch XPath, and are
  paged. `--def` classifies hits (def, abstract, child, patch, patch value, translation,
  reference). Literal text or `--regex`; no index.

### Game test runs (09 §7)
- `archo test run [<plan>] [--map] [--timeout]` / MCP `test_run`: starts the game on isolated
  data with default settings, waits for the main menu (or a quick test map), stops it and
  reports the startup errors by mod (`report.json`, the last 10 runs kept). Missing mods and
  broken links stop it before the game starts.

### Game links (09 §3)
- `archo links check [--all]` / MCP `check_links` find RimArcho's `_Archo_` / `!ArchoPack_`
  links in every known game's `Mods/` that are dangling, point at the wrong mod, cannot be
  checked, or whose name a real folder holds (`mods.link-dangling`, `mods.link-foreign`,
  `mods.link-check-failed`, `mods.link-occupied`). A dangling link or failed check in the
  selected game blocks writing, launching and test runs; other games are only summarised.
- `archo links repair --dry-run|--yes [--include <path>]` / MCP `repair_links` (one
  confirmation in the window) remove only RimArcho's own links, after showing the game, the
  full path, the target, the reason and the ownership evidence. Links without evidence need
  `--include`; real folders and unknown entries are never touched; refused while that game
  runs.
- Moving, recycling, locking or removing a mod or pack first takes down every known game's
  links to it, then puts the needed ones back; each step goes into the link ledger (schema
  v18). An interrupted operation shows as `mods.link-recovery` and finishes through links
  repair. Links to a known mod count as RimArcho's own, old IDs too.

### Managed directories (16 §2.14)
- settings › RimArcho managed directories lists the game folders, Workshop, SteamCMD, other mod folders, the
  lock library, packs, Archo Monitor and the SteamCMD program. Each can be relocated (you moved
  it) or moved by RimArcho (a verified copy across drives, the old folder to the recycle bin).
  Mods keep their identity, tags and lists, and links follow. Every start checks them and asks
  where a missing one went; can it run and the launch check only warn.

### External tools (17 §2.1)
- `archo deps`, `deps install <todds|steamcmd|wezterm>`, `deps remove <name>`: one place that
  finds the bundled font, todds, SteamCMD and WezTerm. settings › external tools installs and updates them
  as background tasks (`wezterm.path`, `steamcmd.path`). Only RimArcho's own copy is removed,
  and SteamCMD not while it holds Workshop content, a game or a mod folder.

### Other
- Unsubscribing or recycling a mod that lists use only warns: the confirmation names the lists,
  and their entries then show as missing. Unsubscribing is still refused while the game runs.
- Mod detail: do not list as a copy separates a copy from its lineage and keeps its player data.
- Faster on 1,349 mods: a list edit 1.3 s → 16–30 ms, a detail 650 → 6 ms, needs attention 730 → 11 ms,
  home 800 → 125 ms (link checks no longer resolve every mod; a list edit no longer reloads the
  index). Folder sizes, scan fingerprints and texture summaries walk folders in parallel, with
  fewer workers on hard disks (the whole Workshop's size 5.05 → 0.75 s).

### Development
- CI on GitHub Actions (windows-latest; `master`, `dev/**` and pull requests): gofmt, `go vet`,
  staticcheck (`staticcheck.conf`) and `go test`.
- Tests that touch no process-wide state run in parallel, and the test App reaches only
  loopback addresses (no real Steam API). `release.ps1` runs `go test -short -count=1`.
- `scripts/vm`: build, push and drive the real window in a test VM over DevTools; `smoke.ps1`
  prints PASS / FAIL. `RIMARCHO_DEVTOOLS_PORT` exists only for this.
- `scripts/vm/round.ps1` plays a whole game round in the VM with no screenshots: new colony,
  play, save, load, main menu and quit. A test-only driver (`scripts/vm/drive`) runs inside
  the game for that one launch. The script then checks Archo Monitor's phases and the save
  name, and that the game ran the Monitor RimArcho bundles. It prints PASS / FAIL.
  `cdp.mjs text` reads a page's text.
- `vm.ps1 build` builds Archo Monitor from `monitor/` (as `release.ps1` does) instead of
  copying an older one from `dist/`, and names the bundled version. `-Monitor <dir>` bundles
  another one. `vm.ps1 quitgame` closes the game as its window's X does and checks the
  Monitor's exit line. `status` says when the VM is off, and `cli` shows archo's progress
  lines as plain text.

### Known gaps
- Command output still says "unsaved" in a few places (`1 change unsaved`).
- TUI: no background tasks page or list management page yet (the CLI and the window have them).
- The TUI, CLI and MCP check download pages without the LoversLab session (public page
  reads); only the window uses a logged-in session.
- Followed collections are checked when list management opens, not yet on a schedule.

## 0.8.0 — toward 1.0 (development milestone, in 1.0.0)

Specs: `docs/specs/` (rewritten 2026-10-21; the old `10-1.0\u5b9e\u73b0.md` (Unicode escapes) was folded into them). Section
numbers below are the current ones.

### Hidden and pinned mods (09 §5, 07 §5)
- Folders another manager hid by renaming About.xml are classified: **pinned** (a loadable copy
  of the lineage is what the game uses; a warning once the workshop copy changed since, with
  `archo mod <copy> --diff <workshop>`), **left over** (unsubscribed) or **hidden** on their own.
- needs attention gets a "hidden mods" group: keep hidden, show again (`archo unhide`), use the workshop
  version (`archo unpin`), move to the recycle bin (`archo trash`). Trash never deletes
  permanently and only takes hidden or unsubscribed workshop/SteamCMD folders.

### Live game log (06 §6)
- `archo log --follow [--once] [--json]` follows Player.log while the game runs: only new bytes
  are read, clusters instead of text are kept, attribution is the full-log engine's.
- RimCrow's companion log (`RimCrow_Realtime.log`) adds timestamps, levels and related mods.
- The window says "game running since … · n errors" and puts this run's errors first in
  needs attention; `archo status` shows a running game.

### Lists from other tools (07 §7)
- `loadout import` reads RimSort (XML, JSON, clipboard, Rentry), RimWorld's `.rml`, RimCrow's
  ModList.xml, JSON and share codes (also pasted as the argument), and workshop-ID lists.
- `loadout export --format rimsort|rml|rimcrow`.
- `archo collection <link|ID> [--import]` compares a Steam Workshop collection with what is
  installed and can load it into the workspace.
- The window imports and exports lists (import / export in the Enabled list's head, `i` / `e`, the
  palette): a file (open dialog, typed path or drop), pasted text, a share code, a Rentry or
  collection link is previewed first — installed, not installed with Workshop links and known
  replacements, what replacing or only adding the missing mods would change — and applied as
  one undo step; ModsConfig.xml still changes only on save. Export writes RimSort, `.rml`,
  RimCrow ModList.xml, text or ModsConfig.xml (save dialog or a path; never the game's own
  file, never over a file without asking) or copies a share code. The TUI has both in the
  palette and `e` for export.

### Community data (08 §2.3)
- `archo data update` / `archo data status`: RimSort's rules and Workshop database, Use This
  Instead's replacements. All-or-nothing; the only command that fetches them.
- Missing mods get direct Workshop links; enabled mods with a maintained replacement are listed.

### Packs (10)
- `archo pack build|list|launch|export|import|remove`: compile the current list into a pack
  (checked, sorted for the pack only, each mod copied without other game versions' folders),
  play it with its own list and saves through links in `Mods/`, share it as a zip or a list.
- The window has a Packs screen: the packs with when they were last played, Play, and build
  from the current list with a preview.
- Building a pack no longer freezes the window: it runs as an `archo pack build` child that
  goes on when the window closes, with its steps (check → sort → copy → write → ready) drawn
  as a diagram, a progress bar and a Stop button. The preview marks what needs attention:
  blockers, too little free space (checked before copying; the build is refused), mods that
  cannot be read, mods that do not list the game's version. A build cut short is shown as
  unfinished (build again, or delete what it copied) instead of disappearing; `archo pack
  list` shows it too, `archo pack stop` and `pack remove --unfinished` handle it, and `archo
  pack build` prints progress lines. Sizes show in GB from 1 GB up.

### Settings (16 §2.14)
- A settings screen (gear in the top bar): environment folders, launch method and arguments,
  language, Workshop descriptions, community data sources, database backup and compaction,
  and each cache with its size and a clear button.
- `archo db status|backup|vacuum`, `archo cache status|clear <kind>`, `env set --launch-args`.

### History (07 §3)
- `archo history`, `archo undo <n>`, `archo redo <n>`. The window shows the history in the empty
  staging list: redo steps, now, undo steps in plain words, then the saved versions.

### Other
- The window's look follows magpie (MIT): cards with hairline borders and 12px corners, a
  centred view switch with a raised thumb, icon and text buttons, frosted popovers.
- The top bar: the environment on the left, the pages as tabs in the middle (Mod lists · needs attention
  · log · packs · settings, `shift+1`–`5` by physical key), rescan, sort, save and launch on the
  right; no name or icon (the window's title bar has them). Switching pages no longer shows the
  mod list in between. The position line moved to the bottom bar. Tags are `g` in the window
  (`#` still works where it is not shift+3).
- log (`l`, `c`): the game log (its error clusters with the mods they are tied to, a cluster's
  evidence beside it, Player.log or Player-prev.log), mod changes, and the manager's actions
  (unsaved steps and saved versions). A log of another game version is analysed in the
  background instead of holding the window for seconds.
- needs attention is three columns (the list · the game · your call); ←→ between them. Moving the
  cursor no longer rebuilds the page.
- The top bar holds the environment, the tabs, and the game: launch (the saved list) while it
  is not running, its state while it is. Sort and save sit in the Enabled list's head with
  the unsaved count, since they act on that list; packs have their own Play.
- The mod list no longer leaves the right side empty at narrower widths (a hidden staging
  column pushed the details into a zero-width track); staging hides below 900 px, the
  details below 680 px, and dragging a column edge skips hidden columns.
- With mods in staging the history stays, folded into a bar under them.
- The environment pill opens an environment page: the environments (switch there), this
  one's folders and launch method, and rescanning its mod folders. Settings keeps only what
  is RimArcho's (language, community data, database, caches, packs folder).
- Packs belong to the environment they were built in (a pack from another machine: to an
  environment of its game version); the packs page and the TUI say how many belong to
  others, `pack list` shows each pack's environment.
- A running game is told apart: its `-savedatafolder` (read from the process's command
  line) says whether a pack or the environment's list is playing; when the command line
  cannot be read, the launch records decide. The live log follows the log that run writes,
  the top bar says "pack X running", `archo status` names it, and needs attention's last-run group
  says whether the log came from the list or a pack. Its column is now called game log.
- Smoother pages: a new page fades in once (no slide that shook the scrollbars), a redraw or
  a sub-view switch does not animate, the tabs and sub-tabs have a sliding thumb, the game
  log shows what it read before at once and a loading line only after 150 ms, popups have no
  backdrop blur.
- `T` in the window moves the whole enabled list to staging in one step, each mod keeping
  its place; Core and the DLCs stay enabled, entries that are not installed stay in the list.
  In staging `T` puts everything back, as in the TUI. Both are in the palette of both
  interfaces.
- The Enabled list's position column is as wide as its longest number.
- About.xml, LoadFolders.xml and Defs saved as UTF-16 are read (04 §1.3).

## 0.7.0 — the window (development milestone, in 1.0.0)

### Icon (16 §8.6)
- The author's portrait artwork (`internal/icon/portrait.svg`) on a dark rounded base; below
  48 px only the face is shown so it still reads in the taskbar. (It replaces the keystone arch
  drawn earlier the same day.)
- Rendered by a small built-in SVG renderer, so the repository holds no image files apart from
  the SVG source and the generated `docs/assets/rimarcho.svg`.
- Both exes carry the icon and Windows version information (FileVersion, description). The
  release script writes the resource object with `go run ./scripts/winres`; no resource compiler is
  needed. The window, its title bar and the page header use the same icon.

### Link deployment (09 §3)
- Mods in SteamCMD and extra folders can be enabled. `save` links each one into `Mods/` as
  `_Archo_<source>_<folder>` before it writes ModsConfig.xml, and removes the link once the mod is
  disabled. `save --dry-run` and the save preview list the links. Windows uses junctions.
- The deployed copy registers like any `Mods/` folder: a `Mods/` mod with the same packageId wins,
  and next to a workshop copy it can only be swapped in.
- Only links with the `_Archo_` prefix are touched; a real folder with that name stops the save.

### GUI (08)
- `RimArcho.exe`: a desktop window (Wails v3 and plain HTML/CSS/JavaScript, no frontend build
  step) next to the terminal UI. A double click opens it; `archo gui` opens it from a terminal.
- The window and the TUI share one layer, `internal/view`: the columns, the search syntax, the
  details, needs attention with its actions, the changes, tags, the verbs and the position input, the
  palette and the interface text. Neither interface builds these itself.
- The window follows the TUI's v3 rules (16 §3) and adds the mouse: click and modifier clicks,
  double-click, drag and drop between and within the lists, a context menu, resizable columns.
- Workshop descriptions are built from parsed BBCode blocks and never inserted as HTML; images
  come only from RimArcho's cache. The page loads nothing remote and the window opens no port.
- Children of the window (scan, analysis) open no console window.
- Typed app calls for tags, alias and note (`SetTags`, `SetAlias`, `SetNote`).

## 0.5.0 — MVP (development milestone, in 1.0.0)

### M5: local copies, RimCrow import, exchange, polish
- `copy-local <ref> [--name] [--git]` copies a mod into `Mods/`, writes `.rimarcho/origin.json`
  and can add a git baseline commit. It never overwrites a folder, never copies the upstream `.git`,
  never follows links that leave the mod folder, and does not change the Loadout. The MCP tool is
  `copy_local`.
- Player data on lineages: `alias`, `note`, `tag add|remove|list`. The alias is shown as the mod's
  name; `list --filter tag:<t>` filters by tag.
- `import rimcrow --db <mod_manager.db> [--dry-run]` imports aliases, notes, groups (as tags),
  interlocks (as attach rules) and user rules. It reads copies only and can be run again safely.
  The report also lists the about-disabled mods, with guidance.
- `loadout export [--format text|modsconfig] [--out]` and `loadout import <file>` (a text list,
  a ModsConfig.xml or a save's mod list).
- TUI Changes view: identity questions pinned on top (same / copy / unrelated), load or ignore an
  external ModsConfig.xml change, events grouped by scan and type, and file-level diffs on `enter`.
- Mouse support in the TUI: the wheel scrolls and a click selects. `config set ui.mouse off`
  turns it off.
- `archo version` / `--version`; `archo <command> --help`.
- `scripts/release.ps1` builds one `archo.exe` with the version stamped in through ldflags.
- M4 rulings:
  - `harmony.skip-prefix` is info and becomes a warning only when the method is in a stack of the
    newest log.
  - The newest Player.log is included in `problems` automatically.
  - A log from another game version is attributed against that version.
  - `source search` needs a scope (`--in`, `--implicated`, or `--active --yes`, which shows an
    estimate first).

### M4: Harmony, logs, decompiling, MCP
- Harmony patches are read from mod assemblies: prefixes, postfixes, transpilers, finalizers,
  bool prefixes, manual `Harmony.Patch` and Harmony IDs. New findings: `harmony.multi-transpiler`,
  `harmony.skip-prefix`, `harmony.shared-target` and `harmony.manual`. New command: `harmony`.
- `log`: Player.log clusters, each attributed to mods as source / patcher / dependency / mentioned,
  with evidence. Patch failures are matched to the simulation, and missing cross-references list
  their referrers and providers.
- `source` / `source search`: on-demand ilspycmd, cached per DLL hash.
- `archo mcp`: 15 tools over stdio whose text is identical to the CLI, with readOnly/destructive
  annotations and descriptions that cover the diagnosis workflow.
- `--game-version X.Y` on the analysis commands; `version.unsupported` is folded into one row;
  noisy info is hidden by default.

### M3: problems
- The declaration layer (dependencies, incompatibilities, order, duplicates, versions) and the Def
  layer (inheritance after patching, overrides, abstract collisions).
- A PatchOperation simulator with no-match reasons (`target-inactive`, `target-missing`,
  `target-later`, `wrong-version-route`, `stale-name`, `unguarded-optional`, `node-absent`).
- Commands `problems`, `def`, `patches`, `why` and `files search`; TUI Problems view and Inspect.

### M2: Loadout
- A workspace Loadout with history, undo/redo, versions, tags, diff and checkout; `save` takes a
  backup and replaces ModsConfig.xml atomically.
- Constraint sort with explanations, user rules, and `launch`. TUI list view.

### M1: fact base
- Environment detection, sources (official, local, Workshop, SteamCMD, extra), incremental scans,
  identity reconciliation (lineages, copies, moves, tombstones, questions) and change events.
- Commands `env`, `scan`, `status`, `list`, `find`, `mod` and `changes`.
