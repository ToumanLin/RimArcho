# Changelog

All notable changes to RimArcho. Versions follow semantic versioning; the MVP is 0.5.0.

## Unreleased

## 1.1.5 — 2026-11-27

- Packs: building a pack of a list that was never checked is no longer refused with the Def and patch errors of another list; `archo pack build` notes that the list has not been analysed.
- Archo Monitor 0.13.1: the main menu's translation panel lists the translators again in every language other than English. Since 0.12.0 Monitor's own translation folders, loaded before Core, hid the language's details from the game: no translator names, the folder name in place of the language's own name, and the game's default grammar in place of the language's own (articles and plurals in English, particles in Korean, cases in Russian).
- RimWorld probe skill: `run --lang <folder>` starts the sandbox game in another language, and `--before-core <pkgId>,...` loads mods before the official content, as players load Archo Monitor.
- **What RimArcho did shows how long things took.** Each action that runs for a while (a launch, an uninstall, a batch of saves, a subscription removal, an update) now shows its time as a small muted number at the right of its row ("850 ms", "12.4 s", "3 min 5 s"). Can it run checks are listed there too, the newest 20, each with how long it took and, opened, how long each step took (reading the list, Defs, patches, Harmony, saving, references).
- **Slow checks can report themselves.** When a Can it run check takes more than 30 seconds, in the window or the terminal, RimArcho asks once when it finishes: "This check took 48 s. Send the timings to help make it faster?", with send, always send and not now. "what is sent" shows exactly the text that goes: RimArcho's version and the OS, the game version, how many Mods are active and installed, how long each step took, and the CPU's thread count, the RAM and whether the game is on an SSD. No Mod names, paths, user names or Steam IDs. It asks at most once a day, never sends without your click unless you chose always send, and never asks during an agent's or MCP's run. Settings › network › "send the timings of slow checks": ask, always or never.
- CLI / MCP output: `archo ops` and MCP `list_operations` print a row's duration after its subject (`#12 … launch ok default  12.4 s`) and have the new code `check` (a can-it-run check, its detail the game version, the Mod counts and each phase's time); `archo ops --op check` lists them. New setting `feedback.timings ask|always|off` (`config set`, MCP's settings listing). The database schema is 48 (`op_log.ms`). The feedback relay has a new route, `POST /timings`, that adds the timings as a comment to one fixed issue (`TIMINGS_ISSUE` in `wrangler.toml`; redeploy the Worker); without it RimArcho drops the timings quietly.
- Looks: a style or random variant no longer shows as broken in a list that holds another copy of its Mod without the textures (a pack's copy, a trimmed local copy) while the copy it was added from is still installed; it is asleep in that list, and `archo looks style list` says why.
- Looks: the game no longer logs “Mod RimArcho Looks Preference did not load any content” (or the same for RimArcho Styles Preference) after the last choice or entry is removed; the generated Mods carry an empty language file.
- `archo test run`: a Mod the game lists as “(incompatible version)” no longer makes the run report that Mod and every Mod after it as not loaded (`status: incomplete`).
- Looks view: the gaps between the right column's cards can be dragged in both layers (Mods, load-order rules, preference; override chain, operation), and the Mods card gets more of the column; load-order rules no longer keep empty room beside each rule; the preference card shows each Mod's name on its own line with its states and files under it; a search also finds Mods by name and narrows the Mods card to them; the mouse's back button in the looks editor returns to the gallery; in side by side the two textures stand level and meet at the middle. Going to the editor and back no longer reads the list's textures again.
- RimWorld probe skill: a scenario can take named screenshots at any frame (`Screenshot(name, target)` saves `probe-<name>.png`); the skill says how to run a probe where node or a second game copy is missing (`archo test run --probe`). CLI / MCP output: `archo test run --probe` reports them as `screenshots` (JSON: the paths; text: their number and folder).
- Looks, found in the first test with a real game: a choice whose look left the list with its Mods (asleep or broken) is now listed, with undo, on the Preference card and by `archo looks`; the looks view follows a language change at once; the editor | looks switch, side by side | slider, texture | mask and the facings are translated; `archo looks style add --dry-run` names the folder the add will write (`Style/<entry>/`, no longer `Style/0/`); removing a random variant no longer prints the note about saves that only a style needs. `archo pack build` names the Mods in its "things to fix first" list instead of printing `@{id}` tokens.
- CLI / MCP output: `archo looks` may print a section `choices whose look is not in this plan now` (look id, state, Mod, the undo command); `archo looks style remove` prints its `note:` only for a style; `archo pack build`'s refusal names Mods by reference.
- **Packs carry your looks choices.** A pack no longer needs `RimArcho Looks Preference`, and never contains it: when a pack is built, RimArcho removes from the pack's copies every texture the game would not show with that list (a texture a later Mod overrides, and with a looks choice the other Mods' versions of the chosen look), so inside the pack the load order alone shows what you chose. Your Mods are not touched: only the pack's own copies lose files, and the pack gets smaller. `RimArcho Styles Preference` is copied into the pack like any Mod. A new revision (`--update`) does the same; a choice made after the build needs a new revision. If the list's textures cannot be compared, a list with looks choices is not built (the pack would lose them); without choices the build goes on and says so.
- CLI / MCP output: `archo pack build` prints `looks: removed N textures the game would not show (X MB) from the copies; the pack carries the choice of M looks` (`would remove` with `--dry-run`, where sizes are already the pack's after the step), a `warning:` line when the step was skipped, and the progress line `looks: N/M textures …` between copy and textures; `build.json` has the step `looks`; `pack.json` has `looks` (`removed`, `removed_bytes`, `choices`) and each entry's `bytes` is its copy after the step.
- The generated Mods `RimArcho Looks Preference` and `RimArcho Styles Preference` are RimArcho's own in the Mod library: their source is `looks` (`source:looks`), their rows no longer offer lock, delete, move, "do not list as a copy" or texture conversion, and `archo trash`, `archo pin` / `copy-local` / `pin --update`, `archo library move-to-locked`, `archo lineage split` / `merge` and their MCP tools refuse them with a message that points to the looks view and `archo looks`. Enabling or disabling them in a list, tags, alias and notes work as before; `archo uninstall` removes them. They no longer count as newly added Mods, and Updates no longer lists their rebuilds as changes.
- **Looks view: add a look as a style or a random variant, in test.** In the looks editor's operation card, under apply: **add in game as** a *style* in a style category of your list, or a *random variant* of the default look or of a style the item already has, with the selected Mod's textures. For a style you can also allow new items to receive it at random. The line under the row says what it will do, or why it cannot be added (no Def uses the look, the graphic class is not supported, the category already has a style for this item). The first add shows what RimArcho will write (the local Mod `RimArcho Styles Preference`, links to the textures and a small Def file) and what it means for saves; "do not show this again" is kept apart from the first apply's. The override chain lists the look's styles and random variants with a remove button (`del` on the row); removing a style asks first, because items in a save that had it go back to their default look. The Preference card has a second row, `RimArcho Styles Preference`, with its entries by state and its files; selecting it shows the looks that have a style or random variant in the gallery.
- **Looks: styles and random variants, in test**. Besides choosing which Mod's version of a look the game shows, you can keep another Mod's version beside it. `archo looks style add <look-id> --from <mod> --category <StyleCategoryDef> [--random]` adds that Mod's textures as a new style of the thing in an existing style category (with `--random`, new items may also receive it at random); `archo looks style add <look-id> --from <mod> --variant default|<ThingStyleDef>` makes the thing's default look, or one of its styles, pick at random between what the game shows now and that Mod's version (items already in a save may change look). `archo looks style choices <look-id> --from <mod>` says what can be added and why something cannot (no Def uses the look; a worn or folder look; a graphic class that cannot become a random set; a look with facings before game 1.7; official textures; the game already shows that version). `archo looks style list` shows every entry with its state in the current list (active, skipped, broken, asleep) and `archo looks style remove <entry>` deletes one. Entries apply to all lists. RimArcho writes them into a second generated Mod, `RimArcho Styles Preference` (textures as hard links where it can, plus generated Defs and patches that test for their Def first), keeps it at the end of the list just before `RimArcho Looks Preference`, rebuilds both before every save or launch, and leaves out only the one whose rebuild failed (`styles:` in `archo save` / `archo launch`). No Mod folder is written. `--dry-run` shows everything first. None of the generated Defs has been checked in a running game yet.
- CLI / MCP output: `archo looks rebuild` also rebuilds `RimArcho Styles Preference` and prints its section; `archo save` and `archo launch` may print a `styles:` line; `archo ops` and MCP `list_operations` have the new codes `looks.style` and `looks.stylebuild`; `archo uninstall`'s `looks` step deletes both generated Mods; texture optimisation skips both; a sort keeps both last. The window's service gained `LookStyleAdd`, `LookStyleAddPreview` and `LookStyleRemove`, the add choices and entries in a look's detail, and the `RimArcho Styles Preference` card's figures in the looks overview (no page uses them yet). The database schema is 47 (`look_style`, `style_file`).
- **Looks view, in test.** The list editor has a second view: the switch **editor | looks** at its top left. Looks shows every look of the list as a gallery, per Mod: which of a Mod's textures a later Mod overrides, which are in game, which are mixed. Search across the whole list, filter (overridden, overriding, mixed, chosen, no def), and open a look to compare the selected Mod's texture with the one in game (side by side or with a slider, at 2x and 4x, texture or mask), see the Mods that supply it in load order, and **apply in game** the entire look, a body type or one texture, for this list or for all lists. Select several looks in the gallery to use one Mod for all of them in one step; RimArcho first says how many would be mixed. Undo takes a choice back. The first apply shows what will be written (the local Mod `RimArcho Looks Preference`, last in the list, with links and no Defs) and that the feature is still in test; your Mods and the load order are not changed. Arrows only move: nothing is applied without the apply button.
- Looks: groundwork for the looks view of the list editor (no page yet). The window's service can now list a list's looks per Mod with their state, describe one look with every texture's Mods in load order, predict what an apply would lead to (already in game, mixed with how many textures still from which Mod, no texture), apply or undo several looks at once as one undo step and one rebuild of `RimArcho Looks Preference`, and serve texture thumbnails and originals, DDS in BC1 / BC2 / BC3 and uncompressed included (BC7 and PSD have no preview). Thumbnails are kept in the data folder under `cache/looktex/`.
- **Looks, in test.** When several Mods of a list ship a texture at the same path, the game shows the file of the Mod that loads last. `archo looks` lists every such case in the current list (`--filter overridden|overriding|mixed|chosen|nodef`, `--mod`, `--search`), and `archo looks show <look-id>` shows each slot's Mods in load order and which one the game shows. `archo looks apply <look-id> --from <mod> [--slot <slot>]… [--scope plan|all]` makes the game show another Mod's textures without changing the load order and without touching any Mod: RimArcho writes the chosen files, as hard links where it can, into a generated Mod `RimArcho Looks Preference` that it keeps as the list's last entry (one undo step) and rebuilds before every write to the game. `archo looks undo <look-id>` removes the choice, `archo looks rebuild` rebuilds the generated Mod, and every write has `--dry-run`. The folder is the new setting `looks.dir`, chosen at the first apply on the game's drive. A choice whose Mod is not in the list sleeps until it returns; one whose Mod no longer has the texture is shown as broken. The generated Mod is not rebuilt while a game runs: the choice is kept and the next write to the game rebuilds it. Texture optimisation leaves `RimArcho Looks Preference` alone, sorting keeps it last, Uninstall deletes it (`looks` step), and the operation log has `looks.choice` and `looks.build` rows. Please report problems through Feedback.
- **Settings › About** now has three parts: RimArcho, **author**, and **changelog**. RimArcho reads "version 1.1.4" with one button at its right, "check for updates", which becomes "install update" once a check finds a new version (the separate check button beside install is gone, also in the terminal's update panel), and the automatic-check switch under it. Author is who made RimArcho, Zixian "Jason" Zhang (`ToumanLin`): B.S. Aerospace Engineering, UCLA (class of 2027), looking for a full-time role from spring 2027, with a row of buttons for the personal website, LinkedIn and GitHub. Changelog is this file, built into RimArcho (it is never downloaded): the running version open, the next few as folded rows with their dates, the rest behind "older versions"; click a row or press enter to open it. Each version's changes sit right under its heading at the same left edge, no indent or bullet marks; each change is one line, its first phrase, with "more" for the rest in smaller muted text; the card uses only three text styles. The terminal's settings text lists the author's lines and links, and the running version with a pointer to the window or CHANGELOG.md for the text.
- **No Version Warning** is RimArcho's fourth community dataset, as in RimSort: emipa606's lists of Mods known to work on a game version their About.xml does not list. A Mod the list names for your game version no longer counts as a version warning. It is shown apart from the warnings you ignored yourself: on Can it run the version warning item says "N more confirmed working by community data" in a muted line (when every warning is confirmed, there is no warning item, only a quiet line under "worth knowing"), the version list shows them as its last group (nothing to ignore there), the Mod's details say the same, and a list's version chips count them separately. Supported versions and a list's default and allowed versions still follow what the Mods declare. It updates with the other three (`archo data update`, Settings › Community data › update now, the automatic data check), with conditional requests, and its source is `data.noversionwarning.url`; without it nothing changes.
- CLI / MCP: `archo data update` and MCP `maintenance` `data_update` accept the dataset `noversionwarning`, and `data status` lists it (version: the newest list's date and the game versions it covers). `archo problems` (MCP `list_problems`) adds `confirmed-version-warnings: N mods confirmed working by community data (No Version Warning; …)`, and `--include-muted` / `include_muted` lists them marked `[confirmed working by No Version Warning]`. `archo plan versions` adds `; N confirmed working by community data (No Version Warning)` to a version's line.
- **Settings in seven groups.** The Settings list is down from eleven rows to seven: about, interface, **updates** (with community data), **accounts and network**, **folders and storage**, **tools** (external tools, game log and, where it exists, Archo Agent) and uninstall. A merged group shows its former groups one under another, each under its own heading at the top of the card, with the same rows; its line combines their facts ("community data 11-20  standard intervals", "SteamCMD: anonymous  proxy: system", "7 folders  1 unavailable  database 6.3 MB", "Archo Monitor 0.13.0  todds 7.0.2  SteamCMD installed"); a fact that does not fit the list is left out whole instead of cut to a few letters, and its dot is amber when any part needs a look. Keys `1`–`7` pick them. Links into Settings and the group you last viewed land in the merged group. The terminal's settings text has the same seven groups, each former group indented under its group.
- **Settings in two panes.** The Settings page is now RimArcho's list beside what it opens, as Feedback and All lists: the groups at the left, each with one line that says how it stands ("1.1.4  1.1.5 available", "English  dark  110%", "7 folders  1 unavailable" with the drive's free space as a small bar), and the picked group at the right. A dot marks a group with something to do (accent: an update) or to look at (amber: a folder that is gone, a drive under 10 GB, a tool that cannot run), and the page opens on the first amber group, else on the one you last viewed. ↑ / ↓ change the group, tab goes into it, and the gap between the panes drags. Links into Settings (the command panel's update, a subscription source's "log in") open the group and mark the row. Every setting does what it did; the terminal's settings text is unchanged.
- Fixed (#31): Can it run's RAM figure looked two to three times too high on 1.7 ("loading needs 13.0 GB–23.8 GB" for a game Task Manager shows at a few GB). It was the memory the game *reserves* (private bytes, which Windows backs with RAM plus the page file), compared with free physical RAM. The RAM line now shows what Task Manager shows, the game's expected working set: "RAM: about X in Task Manager (estimated)", and the RAM bar draws it; "RAM is tight" now means that figure does not fit in the RAM available now. What the game reserves while loading moves to the details ("While loading, the game reserves X of RAM and page file; this computer can still reserve Y") and still decides the red "RAM may be insufficient", whose launch warning now shows that reserve against what can still be reserved. The 1.7 model was re-measured on 1.7.5021 (vanilla, 250 and 724 Mods, real cards and emulated 16 / 8 / 4 GB cards). All lists' loaded memory shows the same figure ("RAM about X in Task Manager"), and its loading line says it is RAM + page file.
- Fixed (#31): on 1.7, lists with many texture Mods (HD hair, apparel, races, upscaled textures) were still told "RAM is tight". The working-set figure grew with every GB of textures, but 1.7 loads textures only when they are drawn and keeps no copy in RAM: lists with 1 to 19 GB of textures, measured on 1.7.5021, used 1.2–2.3 GB at the main menu and 2.0–3.7 GB in a new colony (4.4 GB a minute after a first launch). The estimate now stops growing at a few GB; a list shaped like #31's (615 Mods, about 19 GB of textures) reads "about 1.9–7.2 GB in Task Manager" instead of 11–21 GB, which fits in 8 GB free. "While loading, the game reserves …" now spans a repeat launch and a first launch, the same on any graphics card: a first launch after adding many texture Mods encodes them all and briefly reserves much more (up to about 26 GB for 19 GB of textures, even on an 8 GB card), while later launches reserve little. CLI / MCP: `archo plan cost` and `estimate_memory` show the same figures.
- Archo Monitor 0.13.0 records the game's own memory: when the main menu and a map or save open, and a minute later, one line with the game's working set (and Task Manager's private working set), private bytes, their peaks, and on 1.7 the VRAM the driver reports. Numbers only. Can it run then adds "Last run: the game used X at the main menu and Y in play (Archo Monitor)" when that run loaded as many Mods as the list has.
- CLI / MCP: `archo plan cost` (MCP `estimate_memory`) has a new first line `RAM, working set: … (estimated; what Task Manager shows …)`, and the RAM and loading-budget lines end in `(private bytes, commit)`. `--json` adds `capacity.workingSetLow` / `workingSetHigh` and `capacity.lastRun` (the Monitor record above); `ramLow` / `ramHigh` keep their meaning (the loading budget, commit). The `[warning]` level now compares the working set with available RAM.
- Fixed: Mods you get from Steam now go into the list once Steam has downloaded them. Before, the task said done as soon as Steam accepted the subscription, while Steam was still downloading, and the Mods were not added: you had to sort again. Now "get with Steam" after a sort, Can it run's subscribe and a followed collection's "subscribe all" follow Steam's download in the background task ("Steam is downloading 3 / 14", each Mod "waiting for Steam" or "downloading 45%"), read only the Workshop folder again when it ends, and put the Mods in the list they were asked for, where sorting would put them, as one undoable edit ("installed; 3 mods added to Main colony"). A collection's Mods go in the collection's order. A download that stops moving for 2 minutes fails with that reason and can be retried. Recommended replacements are left for the replace step. CLI: `archo subscribe workshop <IDs> --wait` waits for Steam; `--add-to <plan>` then enables them (`--add <IDs>` for some of them, `--collection <ID>` for a followed collection's order) and lists them under `added to plan:<name>`.
- Fixed: after "subscribe all 50" on a followed collection's card, **catch up** never enabled those Mods, because they were already in the collection when you last caught up. Catching up now also enables collection Mods that were not installed at the last catch-up and are installed now, in the collection's order. The card counts them ("50 installed, not enabled", "ready to catch up"), so it no longer says "no change since the last catch-up" while they wait; `archo plan sync --check` lists them under `installed since`. Mods you disabled yourself stay disabled: only Mods that were missing at the last catch-up count.
- Faster Mod checks on large lists. A check now reads each Mod DLL's Harmony patches once and keeps them in the data folder (`cache/harmony/`), so later checks of any list skip DLLs they have seen; before, every check decompiled every DLL again, which with the check's memory cap kept the collector busy. On the 720-Mod Progression Modpack (reference desktop): a repeated check 4.1–6.3 s → about 2.7 s, its CPU time 28–37 s → 5.4–6.9 s; an automatic (background) check 7.1–7.7 s → 2.7–3.6 s, CPU 12–13 s → 3.7–4.9 s. The first check after installing new DLLs still reads them. Results are the same, except that a failed patch's "does not exist in …" now lists the Mods it depends on in the same order every time.
- Fixed: on All lists, a followed Steam collection's **catch up** could freeze for more than ten seconds on a large collection (900 items). When the card had been read more than ten minutes before, catching up read the whole collection from Steam again, eleven requests paced at one a second. It now catches up with exactly the changes the card shows, without Steam, in well under a second (920 items, 451 enabled: 14 ms, was 7–11 s); the button shows "catching up…" meanwhile. Checking a followed collection now asks Steam only for the names of new items (two requests instead of eleven for 900 items). `archo plan sync` still reads the collection first when the last check is older than ten minutes.
- Fixed: a followed collection's card said "✓ same as the collection" beside "50 not installed". The check mark only ever meant that the collection had not changed since you last caught up; it now says **no change since the last catch-up**, as `archo plan sync --check` does, and the not-installed count stays.
- Catching up with a collection is now in the operation log ("What RimArcho did", `archo ops`, MCP `list_operations`, and "recent operations" in feedback): one `plan.sync` row with the list, the collection, how many Mods were enabled, disabled and moved, and their names. `archo ops --op plan` lists them.
- Changed: Workshop Mods downloaded with SteamCMD (`subscribe workshop --steamcmd`, the fallback when Steam is not running, and SteamCMD subscription updates) download anonymously first, even with a saved Steam account, so public Mods no longer open the SteamCMD window or ask for the password, and SteamCMD no longer signs the Steam client out for them. Only items anonymous download is denied (access denied, not owned) are retried once, signed in with the saved account, in one run; the result's notes and the Archo log name those items. Without a saved account they fail with "anonymous download was denied; this item needs a Steam account (Settings › accounts)". Removed items (File Not Found), disk and network failures never sign in. Game downloads still need an account.
- Clearer: on Mod library › Updates, the button on an updated Mod now says **keep this updated version** instead of "keep this one", which did not say whether it kept the version before or after the update (#30). Its confirmation carries the same words and adds that later updates do not change the kept copy; the TUI's p prompt says the same.
- Fixed (#29): with RimArcho open, switching to another virtual desktop (`win`+`tab` or `win`+`ctrl`+arrows) left a dead area on that desktop's wallpaper and icons, with the size and place of RimArcho's window (the whole desktop when it was maximized). An invisible helper window of the page's browser engine stayed on every desktop and took the clicks; it now lets them through.
- **Tags from the list editor** (GitHub #28). The list editor's details heading is now a switch, **details | tags**. Tags opens the Tag manager right there: every tag under all tags with its Mod count, and a click on one shows its name, colour, note, rename, merge and delete; filter to this tag in the heading shows only its Mods. All tags ends with **no tag** and how many enabled Mods have none: click it to show only those Mods, again to stop. The switch shows tags whenever the Tag manager is open (a tag's header in Not enabled, a tag clicked in a Mod's details) and remembers your choice. The filter panel's tags group now ends with **no tag** and its count, in the list editor and the Mod library. The search word is `has:notag` (the same as `-has:tag`); `archo library --include has:notag` and MCP `list_library` take it too.
- Fixed: a Mod check on a very large list could stop at "patches: 7000/31477 operations" and never finish, with the elapsed time frozen and almost no CPU in use. A few patch path shapes (`[last()]`, `position()`, `following::`, `count(//…)`, or an absolute path inside a predicate) made the check walk the whole list again for every Def, which took hours for one operation. Such an operation now stops after a fixed amount of work and is reported as not simulated (new info finding `patch.too-slow`, listing the file, line and path; `archo problems --code patch.too-slow`, MCP `list_problems`); the rest of the check goes on and the result is the same on every computer. The patch step also reports progress at least every two seconds, so the elapsed time keeps moving while it works. Feedback reports now include one line about the list's last Mod check: when it finished, its operations and time, the slowest operation, and how many were not simulated.
- **Russian and Korean.** The window and the TUI now speak Russian (`Русский`) and Korean (`한국어`), chosen in Settings › interface › language or by the system language (`ru-RU`, `ko-KR`). Game terms follow RimWorld's own translations (Mod options: `Настройки мода` / `모드 설정`; Steam Workshop: `Мастерская Steam` / `창작마당`); RimArcho's own words keep one name each (Game Config: `Конфигурация игры` / `게임 구성`; pack: `сборка` / `모드팩`). Russian counts read as a label and number (`модов: 485`), or with the noun in the right form (`3 мода`, `5 модов`). Korean uses Malgun Gothic, and the details' field names get more room in Russian instead of ending in "…". The Launch page's expansion backgrounds take their names from the game's Russian and Korean files. In the TUI, a Russian keyboard layout works for the letter keys (`е` is `t`). CLI and MCP output stay English.
- Archo Monitor 0.12.2: the in-game "Errors by Mod…" window and Monitor's settings in Russian and Korean.
- Feedback has a new kind, **translation fix**: press ctrl+\ to show each element's code, then write codes and better wording, one per line. Each row shows the element's current text, and only the codes you name go with the report (their place in the page and text). In Korean and Russian the kind says Claude translated the language and asks for corrections. `archo feedback l10n --fixes "hom22 → …"` and MCP `draft_feedback` draft it; the relay labels it `localization`.
- Feedback ticks what a maintainer needs most for each kind: a bug brings the game log and recent operations, other / help the recent operations, wrong diagnosis and outdated data the enabled Mods and Can it run's problems. Untick any of them under "change".

- Mod settings take the game's own name in every language: "Mod options", `Mod 选项`, `MODオプション`. This covers the Mod library page, the details row, the palette, and the import, export, pack and uninstall texts. The Game Config page's lead now reads "Game Config = game version + Mod options" (zh: `游戏版本 + Mod 选项`); it no longer mentions the Mod list or the saves folder. CLI output and MCP tool descriptions say Mod options too; the command `archo mod-settings` and the MCP tools `get_mod_settings` / `set_mod_settings` keep their names.
- Texts speak as the one developer: I instead of we (README, release post, the Chinese Looks test notice).

## 1.1.4 — 2026-11-25

- Fewer repeats on screen. Mod library and Play review no longer repeat the Mod's name (and the copy's source) in the details heading. On Updates, the foot of the list says only how many sources were checked; the time of the last check is in its hover and in Update checks, so the left column shows one time (the last scan's). A waiting update's head no longer repeats installed → available, and a SteamCMD copy's install time shows once. In Chinese, Mod settings are now called `Mod 配置` (was `Mod 设置`, which read like RimArcho's own Settings).
- The Game Config page explains itself as "Game Config = game version + Mod list + saves folder; RimWorld starts from it" and opens with a new section, game versions on this computer: each RimWorld install with its version and folder, the Game Configs that use it as chips, and installs RimArcho finds in Steam's libraries that no Game Config uses yet (with Add Game Config). Then all Game Configs (each row now names its game version; the folder is in the hover) and this Game Config's settings.
- Mod details no longer show the category row and its picker. The category glyph before each name and the category filter stay; `archo category` still sets one.
- Give a Mod a custom name from its details or row menu (#23). A pencil marks aliased rows, and the original name stays visible in details and on hover. The list editor and Mod library also offer an Add note button and menu entry; it opens an empty box with RimArcho's automatic note as the hint, so your note is not glued to it.
- Launch opens while the last game log is analysed in the background. Logs over 64 MiB are analysed from both ends, with omitted bytes and repeated error counts in Game log, `archo log` and MCP `analyze_log`.
- (#24) Mod checks are quieter on laptops. A check that starts by itself (after edits, or on opening Can it run or Launch) runs at low priority on two threads, so the game and the window come first and the fans stay calmer; "check again" still runs at full speed. Patches that name several Defs with `or` (`Defs/ThingDef[defName="A" or defName="B"]`), start with `*/` instead of `Defs/`, or break the expression over lines no longer make the check walk every Def (on the 1,305 Workshop Mods of my test machine, 94% of patch paths now use the fast lookup, up from 89%); on a synthetic library with many such patches, a check took half as long, with the same findings.
- New setting, Settings › game log › "check files automatically": after edits (as before), when saved (no check after edits, even after a visit to Can it run; only when you save or launch the list, or open Can it run or Launch while its findings are out of date), or off (only "check again"). CLI `archo config set analysis.auto edit|save|off`; `archo analyze --background` is how automatic checks start.
- Trim or delete oversized Player logs from Launch or Game log after confirming their sizes and closing the game: an amber size badge beside the log opens the actions, and a small mark after the counts shows that only the beginning and end were read (hover for the skipped size). `archo log --trim` and `--delete` and MCP `maintenance` support the same preview and confirmation.
- Fixed: on a RimWorld 1.6 Game Config that shares the default folder with a 1.7 install, Launch, Game log, `archo log` and `archo status` could read a 1.7 run's `Logs\Player.log` once Player.log was gone (for example after deleting an oversized one); they now read 1.6's Player-prev.log, and Launch says the last run's log was not found when only another version's log is left, instead of an empty last-game line. Launch's last-game line no longer stays at "reading the log…" when another read parsed the log first.

- Launching from the list editor asks before saving unsaved changes to the game.
- Can it run opens promptly when you switch pages during a Mod check, even if Launch or Game log is still loading.
- You can keep browsing while RimArcho prepares a launch, writes the Mod list to the game, or backs up, copies, renames, or recycles saves.
- List edits wait through brief background Mod updates while the window stays responsive. If RimArcho is preparing a launch in this window, it asks you to retry the edit after the launch.
- Ignoring one missing dependency shared by many Mods applies the ignores together, so Can it run responds sooner.
- Mod checks save their results in short batches. You can still save edits while a check writes, and an interrupted check leaves the previous results available.
- The database moves to schema 45 (staged Mod checks). Versions 1.1.3 and earlier cannot open it after the upgrade. Analysis references still use `a:` followed by a number. The numbers can skip values because a check reserves its number before publishing.

## 1.1.3 — 2026-11-24

- Fixed (#21): RimArcho no longer copies its bundled Archo Monitor into its own Monitor folder when you subscribe to the Monitor's Workshop item with the same or a newer version; it uses the Workshop copy. `archo scan` and `archo monitor install` say "using the Workshop copy <version>; the bundled <version> is not copied", and Settings › game log shows "(Workshop copy)" after the version. A local copy installed earlier stays where it is; lists that used it move to the Workshop copy (one undoable edit). An older Workshop copy still gets the bundled one.
- Fixed (#19): signing in to SteamCMD looked broken: its window shows nothing while you type the password, and with a Chinese input method on the keys may not reach it, so Enter sent an empty password. Now, when SteamCMD's window asks for the password or a Steam Guard code (Settings › accounts, "download a version", Workshop downloads with an account), a card in RimArcho's corner asks for it in a password field (the input method is off there) and RimArcho types it into SteamCMD's window. Or choose "type it myself in the window". If the Steam Mobile app must approve the sign-in, the card says so. The password goes only into that window: it is never stored, logged, sent over the network or put in a feedback report (README, "What it never does", is reworded to say exactly this). Account sign-in now closes SteamCMD's window by itself once it has signed in or failed.
- The CLI on Windows asks for the password with echo off before a signed-in SteamCMD run when the account is not signed in (Enter alone: type it yourself), and SteamCMD then runs in the same terminal, also for Workshop downloads. The TUI cannot type into SteamCMD's window and shows the hints instead.
- For typing it yourself, RimArcho says before and while the SteamCMD window is open: the password stays hidden while you type, switch the input method to English first, right-click pastes into that window. The window's title says it too, and the TUI and CLI print it. A refused password now says "Steam did not accept the password" with the same hints instead of "login required", also for account sign-in.

- README has a new **Code signing policy** section and, under Privacy and safety, **What goes over the network**: every request RimArcho makes, what it sends and the setting that turns it off (or that there is none). The Windows release can now be built by GitHub Actions and signed for free through SignPath.io, certificate by SignPath Foundation; until that is set up, releases stay unsigned. Release notes on GitHub end with links to both sections.
- Archo Monitor's DLL now names RimArcho as its product (file properties), as code signing requires.

- `archo version` no longer opens the data folder's database (it only prints the folder's path), so it can no longer create or upgrade it. A development build refuses to upgrade the database in the default data folder to a schema released RimArcho cannot open; it names `RIMARCHO_HOME` and `RIMARCHO_ALLOW_DEV_UPGRADE=1`.
- README (English and Chinese): a FAQ entry for when RimArcho.exe won't start with "no permission" even as administrator: extract and unblock the ZIP, antivirus, and Windows 11's Smart App Control, which blocks unsigned programs.
- **Two Mods changing the same thing.** When the XML patches of two enabled Mods replace or remove the same part of a Def (say both set a gun's market value), the one loaded later wins. The sort preview now says when a sort would change which one wins, and offers the rule that keeps the current winner (`archo rule add after …`); the problem behind it (`patch.contested`, still info and hidden by default) says who wins now. Sorting does not move Mods for this: RimWorld merges every Mod's Defs before applying any patch, so a Mod that patches another's Defs works in either order. Hide a pair with `archo rule add ignore contested:#a:#b`. Sorting reads only the last check; without one the preview says so. CLI `archo sort` lists them under "same patch target" (`--json`: `contestNote`); MCP `edit_loadout action=sort` the same; `manage_rules` takes the `contested:` reference.

- **A copy of your newest save at every launch.** Before the game starts, RimArcho copies the newest save of the game (or pack) being launched into `backups/saves/<time>/` in its data folder, where "Back up" on the Saves page puts them. It runs beside the game's start, so launching is not slower (a 200 MB save takes about 0.1 s), and a copy that fails never stops the launch. An unchanged save is not copied again, and only the last 5 automatic copies are kept; your own backups are never deleted. Turn it off in Settings › Game log ("back up the newest save at launch") or with `archo config set saves.launch_copy off`; `archo launch` and MCP `launch_game` follow the same setting. Each copy is a `save.backup` row in the operation log (`archo ops`).
- The list editor shows each Mod's category as a small glyph before its name: game & DLC, library, content, code, patches, translation or textures. RimArcho works it out from what the Mod contains, never its name: a Mod with only patch files is "patches", one with a DLL that four or more of your other Mods require is a "library", and so on. Hover the glyph for the name; the details say why ("only patches: 14 XML in Patches, no Defs or DLL"). If it is wrong, pick another category in the details; "automatic" gives it back. The filter bar has a category group, and searches take `category:patches`. Categories are only shown: sorting does not use them. Interface and performance Mods are not told apart (nothing in a Mod's files says which), so there is no such category.
- CLI and MCP: new `archo category <ref> [<category>] [--clear]`; `archo mod` and `get_mod` show a `category` line with its reason; `set_player_data` takes `category`. The database moves to schema 44 (the player's category).
- **Move between RimArcho and RimStack.** Import finds RimStack's data (`%LOCALAPPDATA%\RimStack`; on Linux `~/.local/share/RimStack` or its Flatpak folder) and offers it as a fourth manager beside RimSort, RimCrow and RimPy, part by part: its notes, tags, ignored warnings and "works on this version" marks (as one tag, `RimStack: ignored warnings`), its Loaders and Last categories (as top and bottom rules), its load-order rules (as "before" rules), and its saved lists. RimStack's other categories, its automatic snapshots and its own caches are not imported. New CLI command `archo import rimstack [--dir <folder>] [--dry-run]`; MCP `maintenance action=import_rimstack`.
- Lists from RimStack import anywhere a list does: its saved list files (`lists\<name>.json`), the text its "copy the load order" gives, and the pages it publishes on Rentry.co. Copied lists from RimSort and RimStack now also give the game version they were made for.
- Export has two new formats: **RimStack** (a saved list to put in RimStack's `lists` folder, named after the list so RimStack can open it) and **RimStack text** (lines to paste into RimStack or RimSort, or to put on Rentry). CLI `archo loadout export --format rimstack|rimstack-text`; MCP `exchange_list format=rimstack|rimstack-text`.
- Fixed: importing a Rentry.co link failed with "no mods found" for every page: Rentry now asks for an access code on `/raw`. RimArcho reads the page's text from its edit page instead.

## 1.1.2 — 2026-11-24

- Fixed: in 1.1.0 and 1.1.1, updating RimArcho from the window always failed right after the download with "runtime error: invalid memory address or nil pointer dereference"; nothing was changed. The window's background tasks also wrote nothing to the operation log (`archo ops`). Both are fixed for updates from this version on. **On 1.1.0 or 1.1.1, update once by hand**: run `archo update` in the program folder, or download the ZIP and unzip it over the old one. A background task that crashes now writes its stack to `logs/gui.log`.

## 1.1.1 — 2026-11-23

- The Archo Monitor Workshop page (English and Chinese) is rewritten in short sections, each under an animated crayon Archo header (Archo Monitor, RimArcho, AI · MCP, Download, Credits): bullets with the numbers instead of paragraphs; every fact and link is kept. The pictures are in `docs/assets/workshop-monitor/` and load from GitHub, so they appear once they are on `master`.
- The Launch page opens at once. Coming back to it from another page drew an empty page first while it read again (about 0.15 s with 13 lists on a large library, more on a slow disk); it now shows what it showed last in the same frame and updates behind it. Each read is also four times faster: the problem counts of your other lists, shown when you hover them, were worked out again on every visit and are now kept until something changes. At start the page no longer waits for those counts either; they fill in a moment later.

## 1.1.0 — 2026-11-23

- Fixed (Linux window, experimental): on systems with GLib older than 2.74, such as Ubuntu 22.04, `rimarcho` could crash with a Go runtime error ("unknown caller pc", "found pointer to free object") when a subprocess ended while the window was starting. GLib's SIGCHLD handler ran on a goroutine's small stack and overwrote memory next to it. RimArcho now installs it before WebKitGTK starts and moves every signal handler except JavaScriptCore's to Go's signal stack. The same crash broke CI's `linux-gui` test job intermittently.
- Update checks: the Archo Monitor Workshop item (3844969469) is now the third source beside GitHub and Lanzou. It is unlisted for now, so Steam does not show it to the check and its line reads "not found"; GitHub and Lanzou answer as before. Once the item is public, every 1.1.0 install starts reading it.
- Play review: the line at the top said "1 colony, 1 save", which explained nothing. It now says "how much of each Mod's content has appeared in the colony", with a legend of the tiles' bar colours: yellow none yet, blue some, green all. The counts stay on the colonies card.
- Fixed: a patch whose target loads only with another Mod said "what it changes is only in another game version's files". Progression: Storage's Reel Storage patches change things Reel Storage loads only when VE Props is enabled; the item now says "part of Progression: Storage does not take effect without VE Props" and offers to enable it (a new patch reason, `folder-inactive`, in `archo patches` and MCP `get_patches`). Patches that really find nothing are still reported.
- Fixed: a list without Core (a new, empty list with Mods enabled and never sorted) launched without a word; the game then reset ModsConfig.xml and loaded Core alone. It is now an error, `official.core-missing`: Can it run shows "Core is not in the list" with Sort (which adds Core), the window asks before launching, and `archo launch` / MCP `launch_game` list it first among the checks with `fix: archo sort`. RimArcho still never adds Core on its own when launching. Pack builds are not blocked by it (the build sorts).
- The README (English and Chinese) is rewritten from the release post: badges, screenshots of each area, requirements, what RimArcho touches and never does, and a FAQ. The MCP tool count it gives is today's 47 (30 by default).
- **Archo Monitor wears crayon Archo (Monitor 0.12.1).** Its icon in the game's Mod list and its preview picture are now Archo in her crayon style, as in RimArcho's window, instead of the vector portrait; the Workshop item's animated preview shows her watching the log, frowning at an error, tracing it and finding the Mod. Nothing else in Monitor changed; RimArcho installs 0.12.1 beside 0.12.0 and moves your lists to it as with any Monitor update.
- Fixed: picking another Game Config on the Launch page could change that Game Config's list. It applied the list played most recently in any Game Config (lists are shared), so picking a 1.7 Game Config made the 28-Mod list last played in 1.6 its current list. Picking a game now keeps its own current list (or the item you applied there earlier in the session).
- Fixed: after a game copy, opening "copy the game folder…" again left "copy and add" greyed out with no size and no reason. The form kept the last pick and proposed the first copy's folder, which is taken. It now starts from the current Game Config and proposes the next free name and folder (`RimWorld 1.6-copy-2`).
- Fixed: a game copy (Add Game Config › "copy a game folder", the Game Config page's "copy the game folder…") could not be stopped: its row on Background tasks had no "stop the rest". It now has, and a stopped copy leaves no folder and no Game Config.
- Fixed: when a Game Config's `Player.log` was gone and only `Player-prev.log` was left (deleted by hand or by a cleaner), Can it run showed that previous run's errors but the Launch page had no "last game" line at all. Launch now reads the previous file too.
- CLI: `archo launch` (and `--dry-run`) said "before you play: 1 problems that often cause errors" for one problem; it now says "1 problem that often causes errors".
- Fixed: right after you disabled a Mod in the list editor, Can it run and the editor's problems could say "part of <Mod> does not take effect: <the disabled Mod> is not enabled" for a patch written to be harmless when its target is missing (`<success>Always</success>`), with an "enable" button and a problem count, until the check ended and dropped it. Seen with Progression: Gravship after disabling Royalty. Such patches no longer give that item; lists are checked again once after this update.
- Fixed: Saves said "1 colonies 1 saves" over one colony with one save, and "read 1 saves" in the status line, in English; they now say "1 colony 1 save" and "read 1 save".
- Fixed: with a list open in the editor that was not the current one, the Launch page showed the current list but its launch button (and the command panel's launch) started the open list instead, asking about that list's missing Mods. Launch now starts the list it shows; the editor still launches its own list by making it current first.
- Fixed: the list editor's history said "1 steps" in English for one step (and What RimArcho did's steps were read out the same way); it now says "1 step".
- Fixed: when another process switched the current list (`archo plan use`, an agent through MCP) while the window stayed in front, Can it run checked the new list but its pill still named the old one, also after leaving and coming back; after the window got the focus back an open Can it run kept the old pill until you left it. Both now show the list checked; the pill also turns plain when the list open in the window becomes current from outside (it stayed amber).
- Import: the game's row read `快捷读取 1.6 的当前列表` in Chinese ("quick-read the current list of 1.6") and `…を読む` in Japanese; both now say "the current list of 1.6" as English does.
- Fixed: for a new player whose RimWorld had never been started (no `ModsConfig.xml`), the first list was empty, without Core, and Mods added to it launched without Core (the game then disabled them all). The first list is now Core and the installed DLC, as the game itself loads.
- Fixed: a new player whose RimWorld had never been started saw "last game: the last run's log was not found: …\Player.log" on Launch right after the first-run guide. With no log and no launch from RimArcho it now says "not launched yet", as a Game Config with its own saves already did.
- First-run guide, where your first list comes from: the row for RimSort, RimCrow and RimPy said "looking for RimSort, RimCrow and RimPy…" for good, though the first run does not look for them. It now has a "look for RimSort, RimCrow and RimPy" button that does.
- First-run guide: after reading your Mods, the card's heading still said "Reading your Mods…" beside "✓ 1.6: found 1303 Mods". It now says "Your Mods are read" (or "Some Mods could not be read" when a game's scan failed).
- Fixed: checking one source under Mod library › Updates (Steam Workshop, SteamCMD, …) showed in the status line and Background tasks as "check for updates  RimArcho: 259 have updates"; it now names the source it checked.
- Fixed: after switching the current list from outside the window (`archo plan use`, MCP `manage_plans` use), coming back to the window updated the sidebar but left Launch and Can it run on the list from before (Can it run caught up only with a later background check, so it stayed one switch behind). The visible page now follows when the window gets the focus.
- `archo uninstall`: the `links` step called another RimArcho's link in the game's `Mods/` (left alone, correctly) "a folder named like one but not a link". It now counts links not proven to be this home's apart from folders that only carry the name.
- Fixed: a second RimArcho data folder on the same Windows user (`RIMARCHO_HOME`, used by tests and harnesses) could not choose its own managed folder: `archo config set local.dir <folder>` said "managed folders are already in use" because the default folder held the first home's files. Before it has a Game Config, a home may now choose its own.
- Fixed: the text interface kept a processor core busy while idle on a large list (1,292 enabled Mods): its memory cap was below what such a list needs, so it ran a garbage collection every few milliseconds with nothing changing on screen. The cap is now only a ceiling against runaway growth, as in the window; idle memory is still returned after two seconds without input.
- Fixed: the text interface's navigation row numbered Game Config "7", but shift+7 opens texture optimisation and Game Config is shift+8; clicking "7 Game Config" also opened texture optimisation. The row now shows each entry's own shift digit, and a click opens that entry.
- Fixed: Can it run's "has a maintained replacement" card, with the replacement installed, offered "enable B" and "disable A" both on Space (window and terminal). Space only enabled B, so both copies stayed in the list and the card stayed; when B has A's name the two read "enable MendAndRecycle / disable MendAndRecycle". It is now one action, "use B instead" (Space), that puts B in A's place as one undo step.
- Fixed: on Windows with a user name in Chinese (or any non-English letters), SteamCMD never worked: it refuses to run from a folder such as `C:\Users\新玩家\AppData\Local\RimArcho\steamcmd`, so every Workshop download, game download and sign-in failed with only "SteamCMD exited before confirming the download". RimArcho now starts SteamCMD through a link under `C:\Users\Public\RimArcho` that points at its folder; its files and sign-in stay where they were. Uninstall removes the link.
- Fixed: unsubscribing from a Workshop Mod while the game ran, after a launch without the Steam client had linked it into `Mods`, was refused with only "affected game is running: <game folder>" and left no row in Background tasks › What RimArcho did (`archo ops`). It now says the game is running and records the refusal, as it does without the link.
- Fixed: What RimArcho did showed a refused save recycling's reason twice ("the game is running: …; the game is running: …"), and in Chinese and Japanese named save batches in English ("1 save"). The reason is given once and the count is in your language (`1 个存档`).
- Fixed: with the window open, switching the current list from outside it (`archo plan use`, MCP `manage_plans` use, another window) could leave Can it run, the Mod library and the editor on the old list until the window restarted, while Launch showed the new one. This happened when the list open in the window had become current after it was opened. The window now follows the current list whenever its open list is the current one; a list you opened yourself while another one is current stays open. Can it run's head shows the list it checks as a pill, amber when that list is only open here and not the one the game launches (its tooltip names the current list).
- Fixed: on a computer whose graphics adapter reports only a token amount of dedicated memory (a VMware VM's 4 MB, Intel integrated graphics' about 128 MB), RimWorld 1.7's memory estimate capped the textures at that card's size, so Can it run, All lists, `archo plan cost` and MCP `estimate_memory` said far less RAM than the same list needs on 1.6 (31–71 MB against 266–550 MB in a VM), and the VRAM bar read 4.0 MB. Dedicated memory under 1 GB now counts as unknown: the RAM estimate is the full, uncapped one and the VRAM capacity shows `—` (`unknown` in the CLI). Cards of 1 GB and more are estimated as before.
- Saves › all saves: in a narrow window the button under "can it load?" ("make a list from this save to analyse its assemblies") ran out of the middle card and was cut off. It now wraps inside the card.
- List editor: the head is shorter. The line on the left says "603 enabled" (hover for the rest). Rescan, rename, import/export and delete are icons side by side (hover or a screen reader gives the word), the unsaved count is a badge on save instead of its own "32 unsaved" beside it, and the order is problems, the tools, save, launch.
- List editor, the steps after a sort: each Mod's name is a link (an installed Mod opens its details beside the dialog, a Mod to download opens its Workshop page). A list entry that is not installed can be removed from the list on its row (a bin at the row's end; undo takes it back), instead of only being downloaded. A Mod that Steam's last check found removed from the Workshop (delisted, deleted or made private) is no longer offered for download: it is under "cannot be fixed here" with that reason, like a local Mod nobody can download, and can be removed from the list there.
- Feedback: the "attached" chip of the game log was cut off in Chinese (`游戏日志里的错误（11月22日 23:40 的 Player.log）`). Chips now show the attachment's short name ("game log", "paths", "recent operations"); hovering shows the full heading with the file and time, then the lines. A long chip (a Mod's name) ends in "…" instead of being cut.
- Fixed: a Game Config with its own saves that you had never launched said "the last run's log was not found: …\Player.log" on Launch and Can it run (once a sibling Game Config's game had exited). It now says "not launched yet".
- Fixed: with two RimArcho installs sharing one game (two data folders), uninstalling one removed the other's `_Archo_` links from the game's `Mods` folder, such as the link to its Archo Monitor, once a scan had seen them. Uninstall now removes only its own links, and `archo links check` reports the other's as not its own. Windows and Linux.
- Development: on Windows, RimArcho's own tests no longer move their synthetic folders into your real Recycle Bin. A test build refuses the real bin, and the test fixtures recycle into a folder of their own.
- **Linux (experimental): the mouse back and forward buttons work in the window**, as on Windows: one press goes one page back or forward. WebKitGTK handed both buttons to the page as the same unnamed button (which could also act as a left click where the pointer was); the window now takes them from GTK itself.
- Fixed: the list editor's problems button and its panel said "1 problems" in English for a list with one problem; they now say "1 problem".
- Fixed: with RimWorld 1.6 and 1.7 installed side by side, after a 1.7 game run the 1.6 Game Config's Can it run, `archo log` and `archo problems` read the 1.7 run's log as its own ("analysed: as 1.7 (log version)"). A 1.7 log that a 1.7 Game Config's launch wrote now belongs to that Game Config only.
- Fixed: `archo env list` showed no `player-log` for a RimWorld 1.7 Game Config with its own saves, although `archo log` read that Game Config's log. 1.7 always writes its log to the game's default folder; `env list` now shows that file when the last 1.7 launch was this Game Config's.
- Fixed: in RimWorld 1.7, warnings logged from the game loop, such as `[Steamworks.NET] SteamAPI.Init() failed` when the game starts without Steam, were counted as exceptions (an error on Can it run and in `archo log`). Their stacks have frames that name a source file (`… [0x6] in …\PlatformBase.Loop.cs:143`), which were taken for an exception's own stack.
- Fixed: `archo workshop subscribe` (MCP `manage_library` subscribe) refused a Workshop Mod you had unsubscribed from once Steam had deleted its folder, saying it was "subscribed already". It now subscribes to it again.
- Fixed: after you unsubscribed from a Workshop Mod (in RimArcho or in Steam) and Steam had not yet deleted its folder, RimArcho went on treating it as subscribed and loadable: it could be added to lists the game would not load, `archo workshop subscribe` said it was "subscribed already", and it could not be moved to the recycle bin. It is now an unsubscribed leftover from the next scan. RimArcho now goes by Steam's subscription list (`WorkshopItemDetails` in `appworkshop_294100.acf`), not the list of installed folders, which keeps unsubscribed items until Steam deletes them.
- Fixed: a test run (`archo test run`, MCP `test_run`) while the Steam client was not running loaded none of the list's Workshop Mods and reported "NOT TESTED: these Mods were not loaded". It now links them into the game's `Mods` folder for the run, as launching without Steam does, and removes the links afterwards.
- Fixed: after Can it run checked the list again, Steam updated a Mod or RimArcho was updated, the Launch page's "last game" line said "not checked yet" until you opened Can it run, which then said the run's errors (or none). Launch now reads the last game's log again by itself.
- Fixed: pressing launch (`L`) in the list editor while the game was running made the open list current and asked whether there was enough RAM (the running game was using it) before refusing. It now only says the game is running, and the current list stays as it was.
- Fixed: Play review put a Mod under "never seen in the saves" when everything the saves showed of it was what you pick from (Ideology's memes, styles and abilities in a new colony), though its details said 36 seen. It is now under "picked from, some in use".
- Fixed: the Launch page said "1 mods changed" for one changed Mod; it now says "1 mod changed".
- Fixed: Updates said "checked 3 sources" under a list whose only source read "not checked yet": it counted sources with no Mods, which the page does not list, after an automatic check. It now counts the sources it lists, and one reads "checked 1 source".
- Fixed: the Launch page's "last game" line counted the Mods in Can it run's last-run section instead of their errors, so a run with 69 errors from two sources said "2 errors". It now adds up the errors those items name.
- **Linux:** a Windows copy of RimWorld that Steam runs through Proton is badged "Proton" instead of "Steam" on the first-run guide's cards, on the Game Config page (beside its name and after the game version), and in the Game Config switcher of the window and `archo` ("RimWorld 1.6  Proton"), so it can be told apart from a native Linux game. `archo env list` and `archo env detect` (MCP `manage` envs) add a `build: proton` line for it.
- Text interface: a Mod's full details (its Defs, patches and Harmony sections) and sending feedback now read on a database connection of their own. They used the interface's own while you went on pressing keys, so switching Game Config at that moment could mix the two or fail with "another launch or credit replacement is running".
- **List editor: what the sort finds is fixed one step at a time.** After a sort that finds required Mods that are not enabled, entries with no folder whose copy is installed, recommended replacements or Mods to get from the Workshop, the window no longer lists them all with one "fix N and get M" button. A strip at the top shows each step as an icon and a count (use copies, replace, enable, download); the current step lists every Mod it changes, with the position it lands at in the list and the Mod that needs it, before anything changes. Apply it, skip it (`s`) or stop (`esc`); a skipped step can be taken up again from its chip. Each step is its own undo step, and the latest one, the sort included, has undo (`u`) right on its chip. What no step can fix is folded under "N cannot be fixed here", each with its reason. Downloads pick Steam or SteamCMD on the step itself. Each fact appears once: the current step's chip is its heading, its count is only on the chip, and the Mod rows sit flush under the strip without icons or indent.
- CLI: `archo loadout use-installed`, `replace-recommended` and `complete-dependencies` take `--dry-run`: they show what they would change and write nothing (agents run them through the CLI; MCP has no tool for them).

- **Linux (experimental):** the first-run guide's "download a version" suggested the game folder `~/Games\RimWorld 1.6`, with a backslash; it is now `~/Games/RimWorld 1.6`. (Linux-Test VM, 2026-11-23.)
- **Linux (experimental):** dragging a Mod in the list editor no longer selects the text of the rows it passes over (WebKitGTK ignored the rule that keeps rows unselectable).
- After removing RimArcho's own SteamCMD (`archo deps remove steamcmd`, Settings › external tools), the terminal and `archo folder check` no longer ask where the SteamCMD folder moved; it is shown as unused until SteamCMD is installed again.
- **Linux (experimental): SteamCMD downloads finish.** Downloading a Workshop Mod or a game version with SteamCMD always ended "download was not confirmed", even when SteamCMD had downloaded it: RimArcho reads SteamCMD's console log, which SteamCMD writes on Windows but not on Linux. RimArcho now writes that log itself, so `archo subscribe workshop … --steamcmd`, the SteamCMD fallback and `archo env download` see the result. (Linux-Test VM, anonymous download, 2026-11-23.)
- Test runs (`archo test run`, MCP `test_run`): after a launch without the Steam client running, a Workshop Mod of the plan (Harmony, say) was reported as "not loaded" and the run as not tested, because the game took it through the `_Archo_workshop_…` link that launch left in `Mods/` instead of the Workshop folder. That link is the same folder, so the Mod now counts as loaded and the report names the link. (Linux-Test VM, 2026-11-23.)
- **Linux (experimental):** the uninstall confirmation (window and terminal) said RimArcho's data and the program are deleted "after closing"; on Linux they are deleted while uninstall runs, and it now says "delete directly".
- **Linux (experimental):** `archo uninstall` and the window's uninstall no longer say the folders and program go "once this command ends" and that leftovers are listed in `/tmp/RimArcho-uninstall.log`: on Linux they are deleted while uninstall runs, so the report marks them done, and anything that could not be deleted is shown as a failure on its line. The terminal's "no game added yet" screen now also points to `rimarcho` when it is beside `archo` (it looked only for `RimArcho.exe`).
- **Linux (experimental):** the window no longer closes when the button at the top left moves the navigation between the side and the top (ctrl+b); on Linux it moves without the animation.
- Test runs (`archo test run`, MCP `test_run`): when the plan's analysis was out of date, the run let go of the Mod list lock only at its very end, so the analysis it starts after the game exits waited for that lock, failed (`analysis_error`: "another launch or credit replacement is running") and an error from a Mod's code (a static constructor, a Harmony patch) named no Mod (on Windows too, where the analysis runs as a separate `archo analyze`). The run now lets go of the lock once the game has exited and its temporary links are gone, and the error names the Mod again.
- **Linux: the window, experimental.** The Linux download now has two programs, as on Windows: `archo` (the command line, text interface and MCP; static, needs nothing installed) and `rimarcho`, the window. `rimarcho` needs GTK 3 and WebKitGTK 4.1 (Ubuntu and Debian: `libwebkit2gtk-4.1-0`; Fedora: `webkit2gtk4.1`; Arch: `webkit2gtk-4.1`); Ubuntu 22.04, Debian 12 and Fedora 40 are the oldest systems it is built for. `archo gui` starts `rimarcho` when it is beside it, in its own session, so closing the terminal leaves the window open. `rimarcho` without a desktop (over ssh, say) says so and points to `archo` instead of crashing. Scans and analysis started from the window run `rimarcho` again with command arguments, as `RimArcho.exe` does. Uninstall removes both programs. The window now opens drawn (WebKitGTK's DMA-BUF renderer, which left the page blank on some graphics drivers, is off unless you set `WEBKIT_DISABLE_DMABUF_RENDERER=0`), fits the screen (at most 90 % of it, centred; it used to open at 1440×880, partly off a smaller screen), and closes cleanly on `kill`, Ctrl+C or a closed terminal, as with its close button (this used to crash WebKit's page process with a crash report). A startup problem is shown in a dialog (zenity, else kdialog, else a desktop notification) instead of the window silently not opening, and so is a database upgrade's progress. A WebKitGTK older than 2.44 is refused with a message naming the package to update. Files and folders dropped on the window go where they go on Windows; a dropped Mod folder (`/home/…`) is now recognised as a local Mod. The window icon is drawn at 64 px, which saves about 55 MB of memory at start.
- **Linux (experimental):** `archo uninstall` now refuses while another `archo` from the same folder runs (a second terminal, `archo mcp`), as on Windows, instead of deleting the data folder under it. Background tasks tell a closed terminal from a new process that got the same process ID. Kept saves and packs go to your Documents folder as your desktop names it (`XDG_DOCUMENTS_DIR`, such as `~/文档`), not always `~/Documents`. On a hard disk, scans and texture checks read with 2 workers instead of 8, so the disk does not thrash. A 1.6 `Player.log` finds its companion log by the file's creation time where the file system records it.
- Terminal: removing a Game Config while a Mod's details were still loading could read the Game Config being removed; the details now load on their own connection, like the window's.
- Import: **portable RimCrow and RimSort are found wherever they are**, such as `D:\Game\RimModManager`. RimArcho now also looks where Windows recorded a manager's program running, where RimCrow's links in the game's Mods folder point, and at shortcuts on the desktop and the taskbar. It no longer takes any recently opened folder for a manager, and no longer offers files beside a manager that are not Mod lists. "Not found? Add it yourself" is now **"Not found? Choose its .exe"**: choose the manager's program (or drop its folder) and RimArcho reads its data from there; a data file (`mod_manager.db`, `userRules.json`, a saved list) still works.
- Fixed: with two Game Configs of different game versions open at once (the window and an agent through MCP, say), the shared-log data of one could be used for the other's checks for a moment, so Can it run, `problems` and sorting could show 1.6 data on a 1.7 list or the other way round. Each Game Config now always reads its own game version's data.
- Feedback: a line under the title says that reports sent from here become public issues on GitHub; the title moved up into the form card's head beside Archo, so the head holds it instead of empty space; with sending available the head has only preview and send (copy the text and open GitHub are gone; a build without the relay still opens GitHub). The community links moved here from Settings › about, as a small card under the templates: Discord and GitHub, and in Chinese the QQ group 1042873484 (click copies the number). The template list, and Import's left card, now look like All lists: rows inset in the card, rounded, with no lines between them.
- Fixed: **another manager's link is no longer counted as a copy of the Mod** (player report "can't delete a Mod"). RimCrow puts `_Link_<Workshop ID>` links into the game's Mods folder, pointing into its own Mod folder; the Mod library showed each as a second copy (`local-1.6`) beside the Workshop copy, with copy actions such as "do not list as a copy" and "keep this version", and no way to delete it. Now such a link has no row of its own and does not count as a copy: the Mod's details say **linked into the game: RimCrow `_Link_…`** with its target, and **show where it is**, which opens the folder holding the link and says how to remove it there (deleting just the link leaves the folder it points to). The game still loads it, so your lists, load order, Can it run and the launch preview are unchanged. A link that is a Mod's only copy keeps its row, with **a link, not a folder: show where it is** in place of delete and no copy actions. A link into a folder RimArcho reads (the Workshop folder, an extra folder) is that copy; if you add RimCrow's folder as an extra folder later, the next scan shows its copy there. RimArcho never deletes or changes these links; `archo trash` refuses one, naming the link and RimCrow. `archo library` (and MCP `list_library`) counts the same copies, and `--lineage` prints `linked in: … (not a copy …)`. Mods you put in `Mods/` yourself as real folders still move to the recycle bin; when a pack reuses that copy at launch, the confirm now says so too (the pack then launches with its own copy).
- **Feedback and Import line up.** Feedback now fills the window like All lists: the templates take 38% at first, and the gap between the two cards drags (double-click resets it; it is remembered). Both cards start on one line: the problems, preview, copy, open GitHub and send are in the right card's head, with Archo beside them at her usual 176 px. The breadcrumb names what is shown ("feedback / bug", "feedback / my reports"). On Import, a result's counts and buttons are now the head of its card, so the right card's top meets the left card's
- **Report a wrong diagnosis from where you see it.** Each Can it run item has a small flag at its right (shown when you point at the item, select it or tab to it). It opens Feedback on "wrong diagnosis" with that item filled in (its ID, kind, sentences, Mods and where the claim comes from are attached), so you only say why it is wrong and what happened in the game. An item that comes from shared logs or the replacement library (a known error, a prerequisite seen in shared logs, a combination that often fails, "has a maintained replacement") also offers "the data is outdated", which opens "outdated data" with the Mod and the item filled in. A Mod's details, in the editor and the Mod library, now show those shared-log and replacement lines under the version notice, each with the same flag. Nothing is sent until you preview and send it on Feedback. Terminal: `W` on a Can it run item opens the feedback panel filled in (`4` for outdated data). CLI: `archo feedback --template <kind>` is the same as `archo feedback <kind>`, and an outdated-data draft takes `--finding <id>`; MCP `draft_feedback` takes `finding` for `data` too, and still only drafts.
- Fixed: a Mod in the game's Mods folder that is a link another tool made, such as RimCrow's `_Link_<Workshop ID>`, had no way out of its menu in the Mod library: no delete and no reason why (player report). RimArcho never deletes another tool's link, so in place of "move to the recycle bin" its menu and detail now offer **a link, not a folder: show where it is**, which opens the folder holding the link and says who made it and how to remove it there (deleting just the link leaves the folder it points to). `archo trash` refuses such a copy with the same reason, naming the link instead of its target. Mods you put in `Mods/` yourself as real folders still move to the recycle bin; when a pack reuses that copy at launch, the confirm now says so too (the pack then launches with its own copy).
- **Feedback, redone: pick what kind of report it is.** The page now has Import's two panes. On the left: feature request, bug, wrong diagnosis, outdated data and other / help, then My reports. On the right, the one picked asks only its own questions (for a bug: what you did, what you expected, what happened instead, does it happen again) and attaches what it needs by itself (a bug ticks the game log; a wrong diagnosis attaches the diagnosis you pick from Can it run and your Mods; outdated data attaches the Mod you name and the versions of the shared-log data and the replacement library). What goes with it is a row of small chips; "change" shows the boxes. The buttons and Archo stand at the top. A bug's "files were changed or lost" box attaches the recent operations and marks the report as a priority on GitHub. A feature request lists similar requests already on GitHub as you type, each with a +1 button that needs no GitHub account. "Add another problem" keeps up to three problems in one draft; send files each as its own issue. Developer replies now open from the left (My reports). The terminal's feedback panel has the same templates (`1`–`5`) and asks the template's questions on `s`; `b` attaches Can it run's problems. New `archo feedback [<kind>] [--<question> …]` lists the templates or drafts a report (its text and GitHub link; nothing is sent), and the MCP tool `draft_feedback` (all group) does the same. Needs the updated feedback relay: until it is deployed, a send with two or more problems says to send them one at a time, and similar requests do not show.
- Helper processes RimArcho starts (scans, analysis, texture conversion, save reading, pack builds, Archo's tools, the update probe) can no longer open a RimArcho window. Previously, a program that ignored a helper's arguments opened a window that started the helper again, which repeated until the machine froze.
- **RimArcho now keeps a record of what it did to your files.** Background tasks › "What RimArcho did" (`W`) lists every Mod it moved to the recycle bin or deleted for good (quick delete), every subscription it removed, every Workshop unsubscribe, every time it wrote your list to the game (with the list version, so you know where to find it in the history), the links it made or removed, save renames, copies, backups and recycling, launches, RimArcho updates and rollbacks, and uninstall — from the window, the terminal, the command line, an agent or Archo, refusals and failures included. Each row is a time, an icon and the name; only one that failed or was refused gets a ✕ / ⊘ mark, in one column at the right, and a red tint, and one "problems" button (`f`) shows only those. Opening a row shows who did it, its steps, the path, the reason and where to get the item back. It never undoes anything itself. New `archo ops [--since] [--op] [--failed] [--all]` (`--op delete` lists quick deletes) and the MCP tool `list_operations` (core group) print the same text; Archo's rows say `agent`. Feedback can attach the newest 30 rows ("recent operations", off by default; `o` in the terminal). Kept for 90 days, at most 500 rows. Database schema 43.
- Can it run: your last save's card now uses the whole width of its column, and Archo stands in front of its bottom right instead of beside it, so the wide empty strip next to her is gone and the Mod chips fit on fewer lines (at a 1,050 px window the card was 230 px wide, now 426 px). Her animation may pass over the card; clicks go through her to the card.
- **Shared-log data now updates every week, apart from RimArcho.** What RimArcho learned from public game logs (known errors, prerequisites authors forgot to declare, which load-order rules matter) used to change only with a new RimArcho. It now comes with Archo Monitor: each week's data is in Monitor's folder, so subscribing to Monitor on the Workshop, or a new Monitor folder from RimArcho, brings it without a RimArcho update. Players without the Workshop item get it through SteamCMD signed in anonymously (never your own account, which would sign your Steam out), only while automatic checks are on and no game runs. RimWorld 1.6 and 1.7 each use their own logs; 1.7 no longer borrows 1.6's numbers. Can it run shows which data it uses as a small pill (`W47 1.6`; hover for the source), and `archo status` and MCP `status` have a `corpus:` line. An entry whose Mod was updated after the last log that showed the error is marked "may be fixed" and lowered to information. Monitor's weekly data updates do not appear under Updates.
- A Monitor newer than this RimArcho (a newer log format) now says "Monitor is newer than RimArcho, update RimArcho" in the game log page and in `archo log` (`monitor:` line) instead of showing its log as empty.

- **A game downloaded with SteamCMD no longer doubles your Mods.** Signed in, SteamCMD also copies every Workshop item you subscribe to into the downloaded game's folder (over 15 GB for a big collection), and RimArcho added that folder as a Mod folder, so each Mod showed up twice with "installed more than once" notes, in every Game Config. While the Steam client's Workshop folder is known, RimArcho now leaves that folder out; if it had added it already, the next scan takes it out again, and lists that used a copy from it switch to the Workshop copy. No files are deleted (the copies stay in the game folder; delete them yourself if you want the space back), and `archo folder add steamcmd <folder>` still adds it on purpose. `archo folder list` and MCP `status show=folders` no longer list it.
- **1.7 test runs keep Chinese, Japanese and other non-English text.** On a system whose language cannot show it (Chinese Mod names on English Windows, Korean or emoji on Chinese Windows), `archo test run` and MCP `test_run` logged such text from RimWorld 1.7 as `?`, and accented letters arrived as broken bytes. The game is now told to write its console in UTF-8, so Mod names, file names and messages in the run's `Player.log`, the report and its error clusters read as in the game. 1.7 errors also reach the report again: the console writes `ERROR`, `DEBUG` and `TRACE` with one space after the time, which RimArcho read as a continuation of the line before, so a test run missed those errors.
- **1.7 Game Configs with their own saves now find their game log.** RimWorld 1.7 ignores `-logfile` and always writes `Logs\Player.log` in the default LocalLow folder, so `archo log`, the game log page and Can it run said "no Player.log found" for such a config. RimArcho now reads that file when this config's launch is the newest one before it was written; after another config or a pack ran, it still reports the log as missing rather than borrowing that run. A pack played on 1.7 is followed the same way: the running game page and `archo log --follow` read that file while the pack's launch is the newest. (VM round 2026-11-21.)
- **Archo Monitor shows errors in the game (Monitor 0.12.0).** Its Mod settings page has a new button, "Errors by Mod…", that opens a window listing this session's errors grouped by the Mod behind each one, so the Mod is useful even without RimArcho. Each Mod is one row with a bar split by kind (XML loading, patches, missing references, tick and drawing exceptions…) and its count; open a row for its messages, each with a kind, a count, the XML file and Def being loaded when it happened, the other Mods on the stack and a Copy button. After the game stops writing its log (10,000 messages), an amber note and `+N` show how many errors came back since, which Player.log no longer has. "Warnings too" adds warnings. Also in Debug actions › Archo Monitor with dev mode on. It never opens by itself, does nothing while closed and adds no work per tick. English, Chinese and Japanese. Texture errors that name a Def (`for def 'X'`) are now tied to the Mod whose XML defines it, in the window and in ArchoMonitor.log.
- **Patches that find nothing are now shown, and "not enabled" follows your edits at once.** Can it run used to show only one kind of failing patch: a Mod changing another Mod that is installed but not enabled. That item now updates as soon as you edit the list (disable the Mod it needs: it appears; enable it again: it goes), before the background check ends. The other failing patches, which used to stay in the technical details, are one quiet item per Mod under "worth knowing", with a mark on the Mod's row: "N patches of A find nothing", and why (what it changes is not there, a later Mod adds it, only another game version has it, or it changes a Mod that is not installed). It is not counted; once the game log shows a patch of that Mod failing, it becomes a counted warning that starts "it failed in the last run". After this update every list is checked once more (patches only; Mods are not read again). CLI and MCP: `problems` shows the same target-inactive findings between checks, with keys starting `quick|`; in `patches` / `get_patches` a succeeded operation reached only through FindMod, MayRequire, Conditional or a conditional folder has the reason `guarded`.
- **No more of the same work over and over.** Fast clicks, switching pages and editing a list no longer start the same work again and again (a player's background tasks showed "Can it run" checks of the same lists every few minutes, with high CPU and memory). Checking the mods' files still keeps Can it run, the editor's problems and Launch's warnings current by itself, but a run of edits now ends in one check of the final list, two seconds after the last edit; an edit during a check gets one check after it instead of a queue; and a check that failed or left the list unchanged does not start again by itself. Background tasks keeps one row for a list's check, updated in place, instead of a line of finished copies. In the editor, "N problems" shows a spinning ring and a faded count from the edit until the new findings arrive. Can it run shows the check's state as an icon (its words and "last checked …" on hover); **check again** reads "up to date" and is off while the shown check is current, and is busy while a check runs, however it started. A page read asked for again while it is on its way (the launch page, the Mod library, the game log, All lists, updates, packs, settings, the Game Config page, Mod settings, file search, textures) joins it instead of starting another; Feedback asks for replies at most once a minute; the build page measures the folders once for repeated entries; clicking update or check source again while one is waiting adds no more tasks. CLI and MCP are unchanged.
- Can it run: "X has a maintained replacement" can now be ignored (`I`, "ignore it"), for a Mod you keep on purpose, such as an unofficial port for a newer game while Use This Instead names the official Mod. It stays hidden until that Mod changes, and the window after Sort no longer suggests the replacement. Before, only `archo problems ignore` could, and the card kept asking you to subscribe to the official Mod. Its English explanation no longer stops mid-sentence ("… does the same and is kept up"). (Player report, 1.0.9.)
- Fixed: searching in a list and then opening another list without clearing the search showed the first list's matches in the wrong rows (or none). Opening another list now clears the search, and the matches are always read again for the list shown. (Player report, 1.0.9.)
- **The command palette suggests what fits the page.** Opened with `:` or ctrl+K, its empty list now starts with the page's own commands (in the list editor sort, save, launch, undo; on Can it run check again, rescan, undo; in the Mod library rescan, filter, updates…), then the page's numbered sections to jump to (Settings' groups, Import & export's sections, with their number key), then everything else after a thin line. Typing still searches every command, Mod, Game Config and list, with the page's commands first among equally good matches. The TUI palette (ctrl+K) puts the screen's commands first the same way. The palette also looks cleaner: the whole window dims behind it, the bottom line included (it used to stay bright and read as part of the palette); the input has no extra box drawn inside it; the selected row looks like a list's cursor; and rows no longer repeat their name in grey on the right (a translated command still shows its English name).
- **Import replaces Import & export.** The page brings things in only: its Share section (exporting the current list or a pack) is gone; lists still export from All lists' and the editor's `e`, packs from the pack page, and `archo loadout export`, `archo pack export` and MCP `exchange_list` are unchanged. The sidebar and breadcrumb say Import. The left column lists the ways in, each with its button: read the game's current list, import a Mod list (All lists' form), import Mod subscriptions (add a subscription), import a pack, and another manager's data with the managers found as cards and "Not found? Add it yourself". The right is a drop pane with Archo in its middle until something is read. Reading the game's list, or dropping a list, shows it next to your list in order: removed, added and moved Mods tinted, long unchanged runs folded, one line of counts, "N not installed" with "subscribe all", and "add only what it lacks", "save as a new list…" or "apply to <list>". The import button shows that it is working until the list opens. Over a result, its counts and buttons stand at the left and Archo at the right. Only one row is marked at a time: reading the game's list after picking a manager drops the manager's picks. The columns keep their widths when a result appears (picking a manager made them jump). Each part of another manager's data has "view": a saved list or a profile opens as the order comparison, notes, tags and rules as a list of what each Mod gets (grey where RimArcho has it already); "back" returns to the tree with your picks.
- **Drop a file anywhere to import it.** Dragging a file over any page dims the window under "drop to import"; dropping it opens Import: a list is compared with yours, a pack starts its task, another manager's file or folder shows what it holds; a file RimArcho cannot import is answered "Not imported" with what it can import, a list it cannot read says why, and a folder with no manager's data in it says so, instead of nothing happening. This now works in the Windows window too (RimArcho registers its own drop target). A dropped Mod folder or zip, or a link dragged from a browser, opens Library › add a subscription with it filled in: a link gets its usual card, a Mod folder or zip a "local Mod" card that shows where it would be copied and installs it on "install it". Several files: the first is taken. Add a subscription also has "choose zip…" and "choose folder…" for a local Mod, and `archo subscribe local <zip|folder> [--dry-run]` does the same from the command line.
- **Archo stays alive and changes face smoothly.** She no longer freezes a few seconds after a change: at rest she idles calmly (breathing, a blink, her hair moving) and only stops while the window is minimised or hidden, has been in the background for a minute, or she is scrolled out of view. Changing expression is now drawn movement instead of a fade: she eases out of one face into her calm pose and into the next. On Can it run she stands beside your last save's card at the top of the page, a little larger, where the card's short content left room, instead of above the last run's card with an empty strip on wide windows; without a last save she stands in the bottom right of the right column. The idle costs about 2 % of one CPU core while visible. The program is about 6 MB larger.
- Small GUI fixes: on Launch, the item counts in 02 list or pack keep clear of the scrollbar; the "since you last played" line in the top right stands on a shaded ground over a light background picture, and its → stays at the end of its first line in a narrow window. The game log no longer shows two file pickers after switching to the Archo Monitor tab, nor a blank band above the picker when switching logs. Updates' scope switch shortens a long list name with … (the full name on hover) so the start of the range stays readable. On the Game Config page the in-use row keeps its version and folder on one line like the others.
- **Can it run, redesigned.** Every section is now a card headed the same way: its number, a small icon in the section's colour, the name, the count, and on the right the date or the run it is about. Your last save is laid out like any other item, with the rows that need attention as chips (+N for the rest). The two column captions are gone (screen readers still name the columns). Archo now looks over the top of the last run's card instead of standing alone below it, and that column always has a card: "no errors in the last run" or why the log is missing. The number keys now count your last save's card too: 1 focuses it, and the sections follow.
- **Import & export uses the width of the window.** On a wide page Bring in shows its four sources two to a line with each button right after its text, and From another manager and Share stand side by side below it, so the page fits without scrolling; a preview, the managers' tree and a run span the page under their section. A narrow window keeps one column. Exporting a pack or its list from Share now says "exporting…" and holds its buttons until the file is written.
- **The network falls back by itself.** When GitHub can't be reached, RimArcho now tries public GitHub mirrors (gh-proxy.com, ghfast.top, ghproxy.net) on its own: `network.github_mirror` is `auto` by default, and homes that never set it (or cleared it) get it. A mirror you set still goes first; `off` never uses one. With the system proxy, a proxy tool that has exited no longer breaks downloads: RimArcho goes direct. When a request fails and no proxy is set, it tries Clash's and v2rayN's usual local ports (only on this computer). Steam requests go direct first, so game accelerators such as NetEase UU, Xiaoheihe and Watt Toolkit keep working without a proxy; RimArcho honours the hosts file and the certificates they install. A route that worked is used first for the next 10 minutes, and a blocked site fails within about 35 seconds. Settings › network shows the route last used ("last: through mirror gh-proxy.com"), a finished background task says when it needed a fallback, and a Steam error suggests turning on the accelerator's Steam Community acceleration. RimArcho's own update is still checked against GitHub's checksum, never one from an automatic mirror. CLI: `config set network.github_mirror auto|off|<prefix>`; `config get` and MCP's settings listing show `network.github_mirror: auto`. Errors say which routes were tried.
- New installs keep packs and the source library under Local (`%LOCALAPPDATA%\RimArcho\packs` and `…\source`), next to kept versions and subscriptions, instead of in `%APPDATA%\RimArcho`; existing ones stay where they are. When you upgrade, a packs or source folder already in `%APPDATA%\RimArcho` becomes that folder's setting, so nothing moves; Settings › folders › "move to…" moves it if you want. `archo folder list` shows the kept path on an upgraded install. Database schema 42.
- **Settings › folders folds by where the folders are.** The folders are grouped under their root: the game and Workshop folders, RimArcho's own folder in `AppDataLocal` (kept versions, subscriptions, Archo Monitor, SteamCMD), its data folder in `AppDataRoaming` (source library, packs), and each other place you moved or added a folder. Each heading shows the root with its copy button, how many folders it holds, the size RimArcho has recorded there and the drive meter once (a group spread over two drives keeps one per folder). Rows show the folder's name under the root, with the full path on hover. Groups where everything is fine start folded; a group with a missing folder or under 10 GB free starts open. Click a heading or press Enter on its arrow to fold or unfold it; RimArcho remembers your choice per group. TUI, CLI `folder list` and MCP are unchanged.
- Archo now keeps you company on five more pages: Can it run, Feedback, Import & export, Uninstall and Background tasks. She stands at the edge, not in the way, and her expression follows what happens: she thinks while something runs, gives a thumbs-up when all is well, looks surprised when something breaks the game, salutes when your report is sent, and is sad when you uninstall. After each change she moves for a few seconds, then holds still; when nothing happens for a while she changes to another calm face now and then (never over a warning or an error). The "Archo animation" setting turns her off on these pages too.
- **Archo moves smoothly and no longer talks.** Her loops now play every drawn frame (15 per second instead of 8, at the speed they were drawn), so she no longer hitches; she no longer moves her mouth as if speaking, since there is no chat; and her ribbon is no longer cut at the top. With Archo animation on you never see a still picture of her: a new expression fades in over the last one, and when she rests she stops on the last moment of her own movement. The program is about 4 MB larger again.
- Archo has new expressions: 18 hand-drawn crayon loops (among them a thumbs-up, a dizzy spiral, sleepy and a skeptical look) replace the old moving puppet on the start-up screen, in loading cards and in the first-run guide. She is framed from the head to the chest with her whole hair in view. Animated no longer needs graphics acceleration; in the first-run guide she moves for a few seconds after each change of expression and then holds still. The program is about 4 MB larger.
- Can it run's heading, when there is something to handle, is the list's name with ▲ and the number to handle before you start and ✕ and the number of error groups from the last run (the sentence is its tooltip). Settings › folders shows each folder's drive as a small meter with "440 GB free of 1.8 TB", amber under 10 GB free. Background tasks show a finished or failed task as ✓ or ✕ without the word. Saves' "what this save needs" bar shows its counts on keyboard focus too.
- Mod library, Packs and a pack page: a faint bar behind each size shows at a glance where the space goes (scaled to the largest Mod or pack listed). In the Mod library "in 3 lists" is three dots; the list names are in the tooltip.
- **Import previews draw what a list brings.** Under the list's format a two-part bar shows its Mods installed (green, ✓ N) and not installed (red, ✕ N), and "replace <list>" and "add the ones you lack" show their effect as Saves' marks: +7 −12 ↕31 (⇄ for a Mod switched to another version). The sentences are the tooltips and what screen readers read.
- Mod library › Updates: each updated Mod ends in one mark instead of a "worth a look" label on every row: ▲ for "affects launch", an amber dot for "worth a look", nothing for "content only". The files that changed show as counts by kind ("Defs +1  Patches ~2"), and Update checks shows each source's state as ✓, ✕ or ○. The words are in the tooltips, the details and for screen readers; `archo changes` and MCP `list_changes` are unchanged.
- Texture optimisation: after "count the textures", video memory before and after converting is one bar (what stays solid, what is saved hatched) with "1.8 GB → 0.6 GB" beside it, instead of a second sentence.
- **Error counts are bars.** Can it run's "errors from the last run" draws each group's kinds (errors in patched methods, exceptions, other) as one bar scaled to the largest group, with the total after it; hovering or focusing it says the kinds and counts. On the Game log each error's ×count has a small bar beside it, the analysis card shows its severity as one to three steps, and "try first" ends with a ten-step meter and the percentage of repeat uploads where small list changes made the error go away.
- **RAM and VRAM are bars.** Can it run's RAM / VRAM item, All lists' loaded memory and the "RAM may be insufficient" launch warning draw one bar per resource: what is in use now (grey), what the list needs (the accent, its low–high range hatched), and what is free, with the computer's total at the end and the graphics card's name small on its own line under VRAM. A need past the total runs beyond the bar's end in red. On All lists the need is split into the game and DLC (lighter) and the Mods. The exact sentences ("RAM: loading needs …; available now … of …") show on hover and keyboard focus and are what screen readers and agents read; CLI `plan cost` and MCP `estimate_memory` are unchanged.
- **Fewer words to learn.** "Game Config" keeps its name everywhere (the sidebar, Launch's 01 column, the switcher, Saves, Mod settings), and the Game Config page says what it is in one line. "lock this version" is now **keep this version** (and "unlock" is "stop keeping this version"); the Settings row for the lock library shows only once a version is kept. Rereading the Mod folders is always **rescan** and rechecking the list is **check again**. Chinese uses `跟着` for following a Steam collection everywhere. The key hint line at the bottom shows only while you use the keyboard (a key press outside a text field shows it, a mouse click hides it); `?` and `ctrl+k` still list every key. MCP's guide tells agents the new words. CLI output is unchanged.
- **Shorter paths through common tasks.** Launch's "last game  N errors" opens the game log at its first error, not Can it run (which is about the next launch). When a download finishes, the status line offers "add to <current list>" (one undo step); a Steam download offers it after the next scan. The first-run guide no longer asks where RimArcho keeps its downloads (it uses the default; change it in Settings › folders), and the first-list step shows only the game's current list and "something else…", which shows the rest. A list's Export page groups its formats "for a friend" (RimArcho list, share code, and a link to packs) and "for another manager", and folds the file's text under "show the file".
- **Less to read.** Pages say what they show in fewer words, in English, Chinese and Japanese. Can it run's heading is one short line ("default: 2 to fix before you play, 2 errors last time"); the order and version items say what goes wrong in plain words; where a finding comes from is behind an ⓘ (hover or focus); the column heads lose their explanations. Updates loses its lead sentence. The game log names error kinds in plain words ("error in the game's code" instead of harmony-stack), drops "its errors are in needs attention", and folds the shared-log numbers under "how sure is this?". Saves, Play review, Texture optimisation, Mod settings, background tasks ("clear finished"), the first-run guide, Add Game Config, Import & export, Export, Packs, Settings and the editor's details (only your own note; no "number N"; one "full details" link) are shorter. "1 problem breaks the game" is singular. Reading the game's current list when your list already is that list says so and offers nothing to import; importing it as a new list names it after the Game Config. A download that fails while you are on another page says "The download didn't finish." with a "see why" link. CLI, MCP and TUI output is unchanged.
- The window no longer tells you to type a command. When Steam is not there to start the game, the launch says "Steam isn't running or installed." in your language and offers **start the game directly** (this game then always starts without Steam). The game log's note about the missing in-game helper (Archo Monitor) has an **install it** button. Switching games with none added opens Add Game Config. Saves' and Can it run's buttons no longer show the equivalent command on hover. CLI and TUI output is unchanged.
- After the first-run guide (or any Game Config switch) finishes, the status line no longer keeps saying "switching to 1.6…" for minutes; the switch's line goes when the switch ends.
- Play review: marking a Mod functional moves the pick to the tile that took its place, so the details, the mark on the map and "mark as functional" all name the same Mod. Before, the details fell back to another Mod (often a DLC) with no tile marked.
- Mod settings, list import and pack refresh say "1 file", not "1 files".
- Packs: a revision that leaves out a Mod can be launched after the previous revision was played. The old revision's link to the removed Mod made every launch stop with "…!ArchoPack_…: target does not exist"; the launch now removes it like the pack's other unused links.
- Import & export: "From another manager" no longer comes up empty when one of the manager's files cannot be read; that file's row shows why and the rest can still be brought over. RimSort rules saved with a byte order mark (for example after editing in Notepad) are read.
- **Play without Steam running.** "The game's own exe" really starts the game on its own now: RimArcho puts a `steam_appid.txt` (294100) in the game folder when there is none, as RimSort and RimCrow do; without it the game restarted itself through Steam. A Game Config set to Steam starts the game directly when the Steam client is not running. Either way, without Steam your list's Workshop Mods are linked into `Mods/` (`_Archo_…`) so the game still loads them; the next save or Steam launch removes the links. A Workshop Mod's settings follow it between the two folder names, the newer file winning. `archo launch` says "launched RimWorld directly: Steam is not running" and lists the file it writes; uninstall removes the `steam_appid.txt` RimArcho wrote.
- **Replies to your feedback, inside RimArcho.** Reports you send from Feedback are listed under "my reports" with their state and the developer's replies; a small dot on the sidebar's Feedback entry says a new reply came. RimArcho checks only when it starts and when you open Feedback, stops 30 days after an issue closes, and fails quietly when the relay is unreachable. "follow up on #N" starts a new report about it. Replies are public on GitHub, and there is no promise of one. Settings › network › "check for replies to my reports" turns checking off. CLI: `config set feedback.replies on|off`; MCP's settings listing shows `feedback.replies`. The feedback relay returns a reply token with each report and answers `GET /status/<n>?t=<token>` (redeploy the Worker).
- Creating a pack on Windows no longer fails now and then with "rename …: Access is denied" when a virus scanner still holds a file it just wrote; the last step waits up to 3 s. The create-a-pack page's "include Mod settings" box sits beside its label again instead of on a line of its own.
- Sidebar: the Saves group moves below Library, and Import & export is the last entry of Library. The shift+1–8 keys are unchanged.
- **Packs carry Mod settings, and can be updated.** A new pack now takes the list's Mod settings with it (Config files and HugsLib settings, by Mod, not by folder name); on its first launch they become the pack's own settings under the names its folders have there, and the receiver's own Mod settings are not copied in (their game settings still are). "include Mod settings" on the create-a-pack page turns it off. A pack keeps an ID and a revision: when the name on the create-a-pack page is an existing pack's, "make revision N of this pack" replaces its Mods and keeps its saves and settings. Importing a newer revision of a pack you have asks first, with the Mods added, removed and changed, and updates that pack in place instead of adding a second one; an older or the same revision is refused. Packs from earlier versions still import and get an ID when exported. CLI: `pack build --no-settings`, `pack build --update`, `pack import --update` (without it a newer revision is only shown); build and import output gain "Mod settings" and revision lines. MCP: `manage_packs` `no_settings`, `update`. Pack manifest format 3.
- **Lists can carry Mod settings.** The list Export page's RimArcho list has "include Mod settings" (off by default). Importing such a list says it carries settings and offers "apply settings" with a preview per Mod (new, kept, replaced, the same); files you have are kept unless you tick "replace settings files I have", and a replaced file goes to the recycle bin first. Lists without settings are written as before, so older RimArcho versions still read them. CLI: `loadout export --format archo --settings`, `mod-settings apply <file> [<package>…] [--replace] [--dry-run]`; `loadout import` says when a list carries settings. MCP: `exchange_list settings=true`, `set_mod_settings action=apply`.
- Confirmations read in the body type, not as a code log: a line like "added: Real Ruins" is a row with its name muted on the left (every confirmation, e.g. updating a pack).
- Import & export: a preview, a pack being added, the managers' tree and the move are drawn like the rest of the page. They appear directly under the section they came from with their own label, as a card of rows: no large bold heading, the facts on one line, the destinations as rows you click (with a labelled "new list name" field), the Mods and Mod settings as groups, and "import" at the bottom right next to "close", kept in view while the preview scrolls. Destination counts are spaced ("1 added  2 removed"), a long pasted share code ends in an ellipsis when the field is not focused, and the page scrolls only as far as the result needs. The hint line shows tab and enter only while a preview is open. While a move runs, the manager row shows only a quiet status line; the panel below has the progress. A RimSort instance shows as a muted name after "notes and tags". The lists' new / import form shows the same rows.
- Import & export: the pack cards read "A pack (with Mod files)", so a pack is not taken for a Mod list.
- List editor: Disabled can be sorted by last updated (Workshop), files modified or date added, besides A-Z (GitHub #5); the choice sits before the individual | tags switch and is kept. New Mods stay first in every order. The search also takes `version:current` for the game's own version (`-version:current`: Mods that do not declare it); `author:` and `version:1.6` already worked and are suggested as you type.
- Import & export looks like the rest of RimArcho: numbered sections of rows as on Game Config and Settings (Bring in, From another manager, Share) instead of a grid of boxed cards; one plain button style; pasted text is a one-line field that grows while you type (`enter` reads it); the header's cancel is gone (`esc` still leaves).
- **Import & export** (was Import) is now the first entry of Library and has a "Share" side: the current list in every export format (it opens the list's export page) and a pack as a file or only its list. Lists' and packs' own export buttons stay. A new first card, "The game's current list", reads the current Game Config's ModsConfig.xml as a list to preview.
- First-run guide: after reading the Mods, "next" opens Import & export with "The game's current list" picked; "use this list" or "skip" ends the guide, and any list, pack or manager data brought in there ends it too. The guide's own "where to start" step is gone.
- Add Game Config: the Mod list choice starts with "the game's current list (N Mods)" when the game has a ModsConfig.xml, and picks it by default. With an own saves folder the game's ModsConfig.xml is copied there, so the new config starts from the game's order.
- Launch page: when the game was updated since the last scan, or Steam has a game update waiting, a top-right line says so with "back up <game> first", which opens Add Game Config's "copy a game folder" for the current config.
- **Mod settings: every file, cleanup, reset and copy.** Mod library › Mod settings now shows the whole Config folder: switch between "this list", "not in this list", "not installed" (settings left behind by Mods you removed) and "all files", each with size and last write. Select one or several Mod settings files and move them to the recycle bin; the game's own files (ModsConfig.xml, Prefs.xml…) are never touched. "reset to defaults…" on a Mod moves its settings files to the recycle bin so the game starts it with its defaults. "copy settings from…" takes the list's settings (all Mods or one) from another Game Config, with a preview; a file it replaces goes to the recycle bin first. All of these wait until the game is closed. CLI: `archo mod-settings files`, `recycle [--orphans]`, `reset <ref>`, `copy-from <game-config> [<ref>…]` (each with `--dry-run`). MCP: `get_mod_settings files=…`, `set_mod_settings action=recycle|reset|copy_from`.
- Import page: less text. Each card is a title, one line on what you get and its input; the "You get…" boxes, the source chips and the hint lines are gone (what a file button accepts is its tooltip). "paste from the clipboard" reads "paste".
- Mod lists (editor, Mod library, background tasks): selecting with the mouse now works like other lists (GitHub #9). A click selects only that Mod, so a Mod selected earlier no longer comes along into the next action; ctrl+click adds the Mod under the cursor and the clicked one, and toggles; shift+click selects a range, and ctrl+shift+click adds a range. Pressing an already selected Mod still drags the whole selection. In background tasks, shift with the arrows or Page Up / Down now selects every task in between, not only the two ends.
- Window: Texture optimisation moved into Mod library as its last sub-page; `shift+7`, the command panel and every link and background task still open it. Feedback moved from Settings › About to its own sidebar row, directly above background tasks.
- Window: `ctrl`+mouse wheel and `ctrl+=` / `ctrl+-` / `ctrl+0` change the interface size on any page, one step at a time through 100, 110, 125 and 150% (`0` goes back to 100%). It is the same setting as Settings › Interface › interface size, so the window reopens at it. Over the Saves page's over-time chart, `ctrl`+wheel still zooms the chart (GitHub issue #8).
- List editor: launch (the button at the top right, `L` and the command panel's launch) now starts the game directly instead of opening the Launch page, so testing a list again and again no longer means going back each time (GitHub issue #6). It runs the same checks as the Launch page: the open list becomes current, unsaved changes are saved, a serious RAM shortfall and missing required Mods ask once, and a running game or an outside change not yet loaded is refused with the reason. The TUI editor's `L` still opens its Launch page.
- Window: Mods you subscribe to in Steam (or add to `Mods/` with another tool) show up on their own. RimArcho checks the Mod folders when the window starts and whenever it gets the focus, and rescans in the background when they changed; a list the game's menu enabled them in no longer says they are not installed (GitHub #10). While Steam is still downloading subscribed Mods, the window checks every 20 seconds, and Can it run says "still downloading in Steam" instead of "not installed".
- Lists: when another tool or the game's menu changes ModsConfig.xml while it holds another list's content (for example `default`, edited since), the change is still kept in that list's history, but it no longer stops saving or launching the list you use. Before, the only way past the error was to switch to that list. Can it run's card for it has "ignore it" and "go to list management". CLI: `save` and `launch` refuse only over the current list's own unloaded outside change.
- Steam: RimArcho asks Steam's Workshop service much less often, after a player found Steam refusing to open Mod pages. Details fetch a Mod's Workshop description only once the cursor has rested on it for 0.4 s (in the editor and the Mod library too, not just full-screen details), the update check's answers fill the description cache, and a removed or private Mod is asked about once a day instead of on every view. All Workshop requests share one limit (2 at once, about 1 a second after a short burst); when Steam answers "too many requests" or "rate limited", RimArcho stops asking for at least a minute, doubling up to 30 minutes, and says "Steam is limiting requests; RimArcho asks again after HH:MM" instead. The pause holds across a restart and for `archo` commands and MCP.
- Answering "same" to an identity question now carries the new Mod's notes, tags, user rules and ignored findings over to the Mod it was, and removes the left-over empty record. Before, they stayed on a record nothing pointed to any more.
- TUI: drawing the screen no longer reads the database; Can it run's count is brought up to date after each key or result instead.
- Window: the top bar no longer shows the Game Config chip (`name 1.6`) beside the breadcrumb; the sidebar and the Launch page's 01 column already name and switch it.
- Launch page: each game in 01 shows its full version (`1.6.4871 rev590`) instead of the short one. Keys: `←` `→` now move between the three columns, and `↑` `↓` change what the focused column holds (the game in 01, the list or pack in 02); before, `←` `→` changed the game.
- Window start: the sidebar's background-tasks line appears together with the first page instead of standing alone while the rest of the window loads.
- **Play review: content you never saw.** Each Mod is now in one of three groups by what your saves show: seen in play, only set up (named only in a bill, a quest or research not begun: chosen or offered, not there yet) and never seen. By progress starts with "never seen in the saves", then "only set up"; each group says what it means and what it cannot tell (content that comes only late, in raids or quests, or with another storyteller). In the saves lists a Mod's only-set-up names. No cleanup or removal suggestions.
- **Functional Mod mark.** Mods with nothing a save can show (interface, performance, frameworks, libraries, translations) are still detected when they add no counted content; now you can also mark a Mod "functional" (it leaves the map and the never-seen group) or "not functional" (a detected one comes back). Play review's Details card has "mark as functional", and the foot line under the map unfolds the functional Mods with "remove your mark" / "not functional". The mark is shared by all copies of a Mod. CLI: `archo functional <ref> [--not] [--clear]`; MCP: `set_player_data` action `functional`. Database schema 41.
- CLI/MCP `saves review` (`list_saves review=true`): new lines "only set up means" and "never seen means", a new group "only set up (N):" with each Mod's added content and up to three set-up names, "(N marked by you)" after the functional count, `--mod` shows the mark and an "only set up" line, and the next step `archo functional <mod>`. Kept per-save review results are rebuilt once (review cache version 3).
- All saves: progress bars while it loads. The first listing shows "listing the saves N/M" with a bar in the loading card, and while the saves are read in full a bar in the page's head follows "reading the saves N/M".

- **Copy a game folder** (for example a backup before RimWorld updates): Add Game Config has a third way, "copy a game folder", also opened by "copy the game folder…" on the Game Config page. Choose the Game Config whose game is copied, the folder and the new config's name, whether the game's own Mods folder comes along and whether the copy keeps its saves apart; the page says how much it copies and whether it fits. The copy is a background task you can stop; a stopped or failed copy leaves nothing behind. The new Game Config plays the same Mod list; RimArcho's Mod links are never copied (launching the copy makes them again), and the copy starts with the game's own exe, so Steam never updates it. CLI: `archo env copy <name> --to <dir> [--name] [--mods] [--saves own] [--dry-run]`.
- Settings › Interface has a "first-run guide" row whose "preview…" opens the guide as a new player sees it, managed-folder step included, without saving anything: the folder choice, adding games, downloading a version, reading the Mods and the starting list are played out on the page only, and "end the preview" (or any way out of the last step) returns to Settings. The TUI's settings panel points to `archo gui` for it.
- List editor: Mods that are only missing a version declaration (for example made for 1.5 on a 1.6 game) now show a muted clock instead of the amber warning triangle, so the rows that need fixing stand out. Can it run still lists the version reminder as before.
- List editor: clearing out Mods you unsubscribed is one click. The problem panel shows each item's first two fixes as buttons, so a group of Mods that are not installed shows both "subscribe" and "remove them from the list"; and clicking the red icon on any not-installed row offers "remove all N not installed".
- List editor: "only these in the list" at the top of the problem panel filters the lists to the Mods with something to fix (`has:problem`, which no longer matches version-only reminders or hints); "show every mod" puts the whole list back.
- Settings › Interface: "interface size" (100%, 110%, 125%, 150%) makes text and controls larger together, for 2K and 4K screens at 100% Windows scaling. It applies at once and is kept for the next start. CLI: `archo config set ui.zoom 100|110|125|150`.
- TUI: version-only rows use `◷` (Nerd Font: clock) instead of `!`.
- Window: every hint line ends with `? all keys` and shows at most 7 keys. The editor with a selection shows space, t, m, g, enter, esc clear the selection and `?`. Less common keys (ctrl+s, J K, x in the editor; ctrl+s steam page in the Mod library; copy the path in Search files; ctrl+o / ctrl+w, scope and add on Updates) moved to `?`. The Import page's hint line gained `?`.
- Window: "update RimArcho" in the command panel (ctrl+k) now opens Settings › About with the check or install button focused; before, it did nothing.
- Window: reading a Mod's source (ilspycmd), making a local Git copy, and installing SteamCMD, checking a SteamCMD account or signing it out from Settings no longer flash a console window.
- Chinese and Japanese: the texture convert-again and remove confirmations and results, the stale-texture evidence line, "moved to the recycle bin", the save review's one-Mod footer and the TUI update panel's key lines are translated.
- MCP (Linux): a Workshop download with a named SteamCMD account no longer hands SteamCMD the protocol's stdin and stdout; without a cached login it fails instead of prompting.
- CLI/MCP documentation: the command table, settings keys, JSON commands and MCP tool table now list every command, flag, key and tool (`deps`, `folder`, `links`, `textures`, `plan cost/follow/sync`, `check_links`, `repair_links`, `manage_textures` and others). The finding catalog lists `pkg.duplicate-mods`, `pack.links-left` and `texture.stale`.
- Window: section labels (list areas, history, details sections, settings labels, sidebar groups, problem groups) are no longer in spaced capitals; group notes in the Mod library's Tags view are no longer italic. The dropdowns in Add Game Config (Mod list, saves folder), Mod settings' choice fields and the saves owner filter are now the window's own menus with keyboard support, instead of the system's. Background tasks' "copy the name" and a list's "copy" export use the window's clipboard and say when copying fails, instead of failing silently. Key caps on the Enabled list's undo and sort buttons, the saves and review find boxes and the saves switch buttons now look the same as elsewhere (enter shows "enter ↵").

- The Import page is rebuilt around what you get: three cards (a Mod list, a pack, everything from another manager), each saying what it becomes and what it accepts. Moving in from RimSort, RimCrow or RimPy is no longer all or nothing: pick a manager, then tick in a tree which profiles, lists and kinds of data (aliases, notes, groups, tags, colour marks, ignored warnings, rules) come over; each part says what it becomes, and what RimArcho already has is greyed. Moving in and adding a pack run as background tasks, shown in place part by part, with stop, continue and retry. The page remembers what came in: a manager says when it was imported and what is new since, a list imported before is not imported again unless you ask (another copy, or update the list it became when the file changed). Running RimCrow imports and pack imports no longer hold the window.
- RimSort's local database is imported too: each instance's notes and tags. A Mod's colour from RimSort or RimCrow becomes a tag with that colour ("RimSort colour #e06c75"), "ignore warnings" in RimSort and ignored issues in RimCrow become a tag, and RimCrow group colours become their tags' colours. The Import page finds each RimSort instance's `aux_metadata.db`. New CLI command `archo import rimsort --db <aux_metadata.db> [--dry-run]`; `archo import rimcrow` (and its MCP tool) reports the tag colours it set.
- Import has its own entry in the sidebar (under Library). The page first shows what other managers left on this computer, RimSort, RimCrow and now RimPy, then takes a file or pasted text. Its one "choose a file…" accepts lists, saves, packs (.archopack) and RimCrow or RimSort data, and sends each to the right preview. `i`, a dropped file, Settings' import row and the guide's "move over" all open this page.
- RimPy's saved lists (in `AppData\LocalLow\RimPy Mod Manager\ModLists`) are found and can be imported like RimSort's.
- Add Game Config, the first-run guide's page with a game you have or a version downloaded with SteamCMD, now asks under the game for the config's name, whether to copy the current config's settings (offered for that config's own game folder; another game starts with the game's defaults), the Mod list and the saves folder (shared, own, or a folder you choose). A game another config already uses can be picked; its new config is ready at once, without reading the Mods again. Switching Game Config while the page is open updates what it copies from and its lists.
- Game Config page: the saves folder is a choice again (shared, own folder in RimArcho, a folder you choose) instead of a path field. Settings no longer repeats the Game Config's own settings.
- Launch page background: switching between Game Configs of the same game no longer reads the backgrounds again; a background Mod a scan finds still shows up among the choices.
- Saves across Game Configs: when saves are in two or more Saves folders, All saves and Play review name whose folder each colony is in (Game Configs or pack) and can filter by it. A save that the game started with the current config would not list says so above Can it load?, with Copy to the saves of <config>, which copies only the file (`archo saves copy <save> --to-current`, MCP `manage_saves action=copy_to_current`). The save's header says whose folder it is and when Steam Cloud does not sync it. `saves show` prints `saves of` and `not listed`; `saves list` adds `[saves of …]` when saves come from two or more folders.
- Can it run: choose which save it checks. Check this save (Saves, middle card) or Check this colony (right card, its newest save) for the current Game Config; Can it run then shows the save you chose, or the newest save of the colony, and Check the last loaded save goes back. Each Game Config keeps its own choice; a chosen colony is still found after its full read or a new save, and a save written over in place is checked again. CLI `archo saves choose <save> [--colony] | --clear`, MCP `manage_saves action=choose|choose_colony|unchoose`.
- Updates (Windows): RimArcho's Workshop item is gone, so the update check now asks GitHub, the Archo Monitor Workshop item (once it is published) and the Lanzou share (`https://rimarcho.lanzn.com/b01i608qja`, password `ctot`) at the same time and offers the newest version any of them names. The download comes from GitHub, else Lanzou, always the exact version found; GitHub's SHA-256 is used when it can be fetched, otherwise the zip's own checks and the start-up test of the new program. When neither download works (Lanzou may ask for a browser check), Settings › About says so and offers “open download page” with the password and “install from zip…”; the TUI's update panel shows the same, `o` opens the page. GitHub requests keep the proxy and GitHub mirror settings. A subscribed copy of the old item is no longer read.
- Updates (Windows): the updater now accepts any safe top-level entry in the release zip and removes the previous release's entries the new zip no longer has (moved aside first, so a failed update puts them back). Later releases can add or drop files without a stepping-stone release. Uninstall removes the added entries too.
- Release scripts (maintainer): `scripts/publish-github.ps1 -Version X.Y.Z [-DryRun]` creates the GitHub release at the current commit with the four assets and the CHANGELOG section as notes, refusing a dirty tree, an unpushed commit, an existing tag, mismatched checksums or binaries from another commit; `scripts/publish-lanzou.mjs` uploads the Windows ZIP to the Lanzou folder through ego lite. `scripts/release.ps1` now replaces an earlier archive of the same version and ends by printing the publishing steps (GitHub, Lanzou, Workshop, verify).
- Workshop: RimArcho is no longer published as a Workshop item. The new Archo Monitor item carries only the in-game Mod, and its description links the GitHub and Lanzou downloads. Maintainer CLI: `archo workshop publish` assembles that item only (refusing any program, archive or checksum in it), titles it Archo Monitor, takes its descriptions from `UserGuide/monitor-item.steam.txt` and `UserGuide/monitor-item.zh-CN.steam.txt`, refuses the retired item 3835688284, and has `--new-item` to create the item once.
- CLI: `archo update --check` prints a `sources` section with each source's answer (`github: ok 1.0.11`, `monitor: not configured`, `lanzou: failed: …`) and, after a failed download, the manual path; the `download` line no longer names a checksum file and an `integrity` line says how the zip is checked. New `archo update --from <zip>` installs a downloaded release zip (the file is copied, not changed). `status` (CLI and MCP) shows the sources after `RimArcho <version> available`.
- Game Configs: own/custom user data folders and packs now launch with their own Player.log and preserve Player-prev.log (a launcher that fails leaves both as they were); default-folder launches keep the game defaults. `archo env list` shows that log for 1.7 too. Extra -logfile overrides are refused. Can it run and home report missing logs with the path checked instead of claiming a clean run, including when 1.7 ignores redirection.
- Status, TUI and GUI Launch now name the actual running Game Config, or explicitly say its owner is unknown, using the same attribution as launch refusals. The window's launch page follows another Game Config's game starting or stopping, and its last game line says whose game runs.
- Chinese and Japanese: translate both sibling and last-config removal confirmation lines.
- Adding a Game Config selects a global Mod list instead of copying the editor list. Add Game Config offers all lists and a new Core/DLC list; `env add` accepts `--list <plan>` or `--new-list`, and `--like` defaults to the source config's current list. New lists use the config or RimCrow profile name without a suffix.
- Settings shows extra launch arguments as quoted command-line text, including arguments imported from RimCrow. Arguments saved by 1.0.9 with a lone quote (`it's`) still launch as they did. RimCrow import shows profiles once, with translated statuses and player wording for source settings in Chinese and Japanese.

- Fixed the blank GUI sidebar and lists after opening or switching Game Configs. Picker selection and removal now follow the displayed row order; Game Config labels no longer replace labels for the game itself.

- Game Configs: the window's pickers show config names and versions in a flat list, the text interface's `E` under a heading per game folder; add copy/empty configs with a game-folder picker and explicit no-Mod-files/no-saves copy boundaries. Per-config settings, rename, shared-folder move previews and removal messages keep effects clear. Proton adding is disabled with a reason; legacy aliases show a same-folder notice that stays until got it (window) or x (text interface).
- RimCrow: select profiles from a GUI checklist and import configured profiles as Game Configs and independent current lists, adopting their data folders in place. Preview conflicts and shared-folder deletion risks; keep player edits on repeat imports, and finish an interrupted import on the next one. JSON argv arrays preserve spaces, empty arguments and quoting through launch; `-savedatafolder` overrides are refused.

- Window and TUI, plan editor: a Mod's details now show its size (the files the last scan read), as the Mod library's details already did.
- CLI and TUI with no game added: the error (and the TUI's hint) now also names `archo env add <name> --game <dir>`, so a player whose game folder detection does not find (a Linux Steam library on another mount, a custom install) sees that they can give the folder by hand, instead of only `--detected`.
- Game Configs: add another Game Config to the same installation with `--saves own` or a distinct `--user-data`. `--like <Game Config>` copies settings and the displayed workspace into a new independent plan, including unsaved list edits, without saves or history; data-root launch arguments are omitted with a notice. Empty own-saves Game Configs start with Core and DLC.
- Switching sibling Game Configs changes context without scanning, deploying links or writing ModsConfig.xml. `env list` groups them by installation. Installation relocation updates every sibling and names them; reattaching one to a known installation affects only that Game Config, and the confirmation says when the old installation's Mod records and analysis go because no Game Config uses it any more. Removing a sibling preserves inventory until the last Game Config is removed.
- Packs appear in every Game Config with a compatible game version and show their origin Game Config. Pack settings-copy errors are now reported.
- SteamCMD credit handoffs capture data-root obligations, surviving Game Config root edits and removal while the installation remains. When one game's data folder cannot be switched yet, the others are, and RimArcho retries the rest later (a game whose installation is gone keeps the SteamCMD copy instead). An interrupted test run's shared Mods journal is recovered before a sibling writes or runs a test. The database upgrades these obligations on open.
- Running-game detection uses the installation and effective user data folder instead of the selected Game Config. Unknown sibling identity still blocks unsafe writes. Launch history captures installation, Game Config name and data root; a matching game path alone cannot prove sibling log ownership.
- Launch arguments: `env add/set --launch-args` refuses `-savedatafolder`; use `env set <name> --user-data <dir>` instead. Plan, pack and test launches also refuse stored overrides before writing, keeping the stored arguments for correction.
- Environments: adding or changing a Linux Proton game now refuses custom user data folders as well as own saves, before changing records or creating data folders. Use the game's shared default folder instead.
- Database: separate game installations from Game Configs while preserving existing IDs, settings, lists and launch behavior. The database upgrades on first open.
- New: playthrough review, the page Saves › Play review, `archo saves review` and MCP `list_saves` with `review=true`. Saves is its own sidebar group between Playing and Library, with All saves and Play review; neither has a Shift shortcut. For every installed Mod a colony ran with, the review says what it adds by category and how much the colony's saves show, all saves of a colony together. The page has four numbered cards: searchable colonies with day and save count; a Mod map; the selected Mod's full library details (picture, authors, facts, fields, description and history); and In the saves, with the total in its head and categories folded until opened to show names not seen. Drag the borders to resize the columns and the two right cards; sizes are remembered, and double-click resets them. Use `/` to find a colony, Up/Down to select one, arrows and Home/End to move among squares, Enter on a category header to fold or unfold it, and 1–4 to focus the cards. Show in the mod library is in the Details card head. A short folded note says "not exact: a save shows only what is in it now" and opens to explain the limits. The page fills the window width. Group the map by progress (the default), kind of content, your tags or largest category. Progress separates nothing in the saves, a little seen, mostly seen, all seen and picked-from content with some in use. Within each group, sort by most content (the default), least seen or A-Z. Switch between picture tiles and a list with a small picture, name, seen/size and bar on each row; small preview JPEGs are cached. Grouping, sorting and view choices are remembered per viewer. Mods adding quests, research, events, creatures, factions or biomes use those categories alone for progress and coverage: nothing seen (amber), some (below 50%), most (at least 50% but below all), all (green). Most content first uses all counted categories. The colour legend is removed. The other content Mods show whether anything is in the saves, with an amber outline when nothing is; functional Mods are counted in a foot line. CLI and MCP retain coverage groups, lowest share seen first for partly seen Mods; `--mod <ref>` lists names not seen. Only what is there counts as seen (a thing, pawn, terrain or biome, research with progress, an event that fired); bills, filters and research not begun do not, and recipes are not counted. Not seen is not proof of never met. `--read` first reads saves lacking a current full read; the page and the MCP tool always do. The page opens on the last-played colony, shows colony cards before full reads, and fills the map with counted progress as saves finish, newest first; All colonies reviews every colony. Per-save results persist across restarts, with shared content catalogs stored once rather than repeated for every save, so unchanged saves need neither another full read nor attribution; finished pages also remain cached in the window.
- Saves: the style a thing was made in and an ideoligion's style categories are now read from a save, so a style Mod whose styles are in use shows its cultural elements in `saves show --content` and is under Loses content in What this save needs, no longer under Safe to remove.
- Saves: a save's reading now keeps apart what is there (things, pawns, research with progress, events that fired) and what is only set up (bills, what a quest would fire, research not begun), for the coming playthrough review. Save content and What this save needs count both, as before. Saves read before are read again when selected.
- Saves: events that are over are now read from a save. The storyteller keeps the time every incident last fired, on the world and on each map, so `events` in `saves show --content` and in What this save needs counts what has happened in the colony, not only what is running or queued: on a long colony 40–80 events of a dozen Mods where there were 5–7. An event Mod whose events have fired is under Remove with care, no longer under Safe to remove. Events are no longer marked as possibly undercounted. Saves read before are read again when selected.
- Saves › What this save needs, `saves show --content`: a Def of a Mod's own Def class (Vanilla Factions Expanded - Tribals' research projects, Childhood Backstories' backstories, creep joiner kinds) is now found when a save references it; it was listed under "come from no installed Mod", and its Mod could be counted as safe to remove.
- Saves: the terrain of a save's maps and the biomes of its world are now read. So are the rock, ore and chunks of its maps. A Mod whose floor, rock or ore is on a map or whose biome is on the world map is under Loses content, with the floors and biomes named; a floor or biome Mod with nothing in the save can be Safe to remove. When a save has terrain or biomes that match no Mod, only the Mods they could belong to are under Could not check ("the save has terrain, biomes or rock that could not be named"). `saves show --content` counts them in `terrain` and `biomes`. Saves read before are read again when selected.
- Window, plan editor: Disabled rows no longer start with a source icon. Only Mods with one copy had it (the box of a Mods/ folder, the Workshop icon), so the names did not line up with Mods of several copies.
- Japanese interface: Settings › language now offers `日本語`, and a Japanese Windows opens RimArcho in Japanese. Translation by tousin61 (community contribution, brought up to date for this release); dates and counts read the Japanese way, kanji use Japanese fonts, and the menu art names the DLC from the game's Japanese translation. Archo Monitor's settings in the game are in Japanese too.
- New Mods are marked: a Mod a scan found in the last 7 days has a "new" mark after its name in the plan editor and Mod library, and new Mods come first in the editor's Disabled list, so a Mod you just subscribed to is at the top instead of somewhere in the alphabet. Mods found by a game's first full scan are not marked (a `--game-only` first scan does not count). The filter `added:new` (also `added:week`, `month`, `half-year`, `older`) finds them in the editor and Mod library; Mod library has an "added" column and sort. `archo library` says `new` on such a line and accepts `--include added:new` and `--sort added`; MCP `list_library` takes the same filter.
- Window, launch page: the last-run and update lines in the top right are readable over a background image again; on a grey part of the image (the default planet in the light theme) they were grey on grey.
- `archo mod-settings` and the TUI's Mod settings panel: a label in the game's window that ends in a colon (`Search:`) no longer prints with two.
- Archo Monitor 0.11.1: opening the settings window of a Mod that draws an empty text box (Alpha Animals' Spawn Toggles, with its search box) no longer puts a NullReferenceException from Archo Monitor in the game log. Monitor's window recording now never throws into a Mod's window; if it fails, it skips that record and logs its own error.
- Mod library › Mod settings: a HugsLib setting that HugsLib never saves (HugsLib's own "show all Mod update news", which its dialog draws as a button) is no longer offered as an on/off switch. Needs Archo Monitor 0.11.1, which records it; `archo mod-settings` and MCP leave it out too.
- Mod library › Mod settings: after you change a value in a Mod's settings window in the game, the "in the game's window" part (recorded when the window opened) now says it changed since and that the file's values are the current ones, instead of silently showing the old value. Opening and closing the window without a change does not count. Needs Archo Monitor 0.11.1, which records the file as the window opened. Also in the TUI panel and `archo mod-settings`.
- Window: the launch page appears much sooner after starting. In 1.0.9 a large library took 7–15 seconds (each conflict on every list read all enabled Mods again, and the page was read three times one after another); on the same home it now takes about 1.5 seconds, and the sidebar about 1.2.
- Saves › What this save needs: a Mod that other Mods of the save require is no longer "safe to remove". It takes the heaviest group among the Mods requiring it (Harmony, required by a Mod that breaks the save, is under Breaks the save) and says "needed by N Mods"; `archo saves show --content` says the same in `breaks the save`. A Mod's name is now a link to the mod library, a chevron opens the row, and the names and summaries can be selected and copied.
- Saves: the right card now answers "which Mods does this save need?" instead of listing its Defs by category. Every Mod of the save is in one of five groups: **breaks the save** (its pawns, factions, code or a map's weather are in it), **loses content** (its buildings, items and the like), **remove with care** (only research, recipes, appearance and the like), **safe to remove** (nothing of it is in the save) and **could not check** (not installed or not analysed). A bar shows the split, a find box answers "can I remove this one?", and opening a Mod shows what it holds. The card describes the save itself; what the current list's changes cost stays in the middle card.
- `archo saves show --content` (MCP `get_save content=true`): two new lines, `removing a Mod` (the count of each group above) and `breaks the save` (those Mods with what each holds). `archo saves show <save> --mod <ref>` (MCP `get_save mod=<ref>`) answers for one Mod whether it can be removed mid-playthrough: its group, why, what it holds and which of the save's Mods need it.
- Saves: a map's weather is now read from a save, so a Mod whose weather a map has counts as breaking the save when removed. Saves read before are read again when selected.
- Saves › Can it load?: "loses 7 classs" reads "7 classes" (English).
- Saves › content in this save: a count of one reads "1 faction", not "1 factions" (English).
- Saves: a save file that cannot be opened (its permissions, or another program holding it) no longer shows "loads with <list> as it is" next to its error; the verdict says it cannot be loaded, the row says the file could not be opened (not "no Mod list at its start"), and its content and Over time cards say so instead of loading forever.
- Saves: a save the program may not open (for example a file whose permissions deny reading) no longer makes the window's "read the saves" task fail; it ends "read N saves; 1 could not be read: …" as intended. `archo saves index` also says how many saves could not be read on an earlier run.
- Saves: when a save changed between Recycle's or Back up's confirmation and the click, the refusal is now in the window's language and says to press it again.
- MCP: a tool call whose arguments are refused before it runs (an unknown action, `manage_saves` trash with `confirm=true` but no token, a URL not recognised) now says why, instead of `error: unknown command "…" (archo help)`. `manage_saves` backup and trash previews say to call again with `confirm=true token=…` and suggest that call, not the `archo saves … --yes --token` command.
- Saves › Can it load?: errors from loading a save with another list (for example one missing two of its Mods) no longer mark the save as not loading cleanly with the list it was made with; the last-load row shows only against the list that load used.
- `archo saves show` (and MCP `get_save`): the colonists line no longer counts the starting pawns the player did not choose, which the game keeps as world pawns of the player's faction; they show as "N world pawns", in What this save needs too. Research counts only projects with progress above 0 (a new colony said "160 projects with progress" instead of 14).
- Can it run: long choices (such as "disable Lib (12 enabled mods need it: …)") wrap inside the card instead of running past its edge.
- `archo mod <ref>` (and MCP `get_mod`): under `depends:`, a dependency met by another Mod says so: "pkg:<id> met by <name> (an alternative it accepts / Use This Instead / your replaces rule)", as the window's details do.
- Scans and other edits no longer fail with "database is locked (SQLITE_BUSY)" when another RimArcho process (the window and the text interface together) writes at the same moment; they wait for its write.
- Text interface, Can it run: an item's `R` (an enabled Mod replaces it, repair the links, unhide) did a rescan instead. The selected item's keys now come before the page-wide and global ones (except q, ? and :).
- Game log: after an analysis, a log read before it is attributed again. Before, the stored report could survive (a new analysis reused the old one's number), so Mods installed since still read "assemblies not analysed yet", and their errors were blamed on another Mod that patches the same method.
- `archo status` (MCP `status`) and the refusals that name a running game say "a test run" while `archo test run` has the game open, instead of "running the list" as if the player had started it.
- MCP: a result's `archo plan list` hint is now the tool call `manage_plans action=list`.
- Test runs: a run that stops before its target (a timeout) no longer lists the previous run's errors as gone; they are "not reached by this run" (`baseline.unreached`).
- Test runs: `--save` with a save that uses Mods the plan lacks names the first 8 and how many more, instead of every one on one line (a big list's save named 474).
- Pin (`archo pin`, MCP `pin_mod`, the window's lock): pinning a Mod whose original is a plain folder in `Mods/` left the list unchanged ("the game would not load" the copy). The copy now replaces it in the current list; saving or launching offers to move the original into the lock library (`--move-duplicates`), and test runs move it aside while they run, as specified.
- `archo save` refused for a Mods/ folder in the way of a linked copy says "save blocked", not "launch blocked".
- Copied game folder: a copy of a RimArcho link that the list no longer uses (for example the installed Monitor's, after a Workshop Monitor of the same version took its place) made launch and save say "rename or remove it yourself"; they now point to the repair in Can it run (`archo links repair`), as for the links the list uses.
- Can it run (Chinese): the confirm for a link repair asked about links marked "will be removed", which a recovery or a copy to recycle does not show; it now asks to repair the links as previewed above.
- Mod library › remove subscription: the "quick delete" box sits beside its words (it was on a line of its own), and the preview reads in the window's language ("remove: 1 subscription", folder, affected link).
- A quick delete that could remove only part of a Mod folder (a file held open by another program), and a move into the lock library that was refused ("nothing was moved"), now read in Chinese in the Chinese window; the copy button next to the folder copies only the folder, not the words after it. English: "<folder>: only part of it was deleted (…)".
- Background tasks: a task of one item that failed now gives that item's reason in the window's language in the status line too (its row in Tasks already did); link previews say "detach link", "create link" and "detach and reattach" in Chinese.
- `archo history`: the header counts "edits to undo", not "unsaved edits" (the undo steps stay after a save, so a saved list read "17 unsaved edits"); "1 version", not "1 versions".
- Background tasks: a pack build left running when the window closed now reads "went on in the background" with "the Packs page shows how it ended", instead of "stopped" with the copy step marked failed (the build had finished).
- Settings: an accent colour or theme set with `archo config set` while the window is open is applied when Settings shows it; Settings marked the new colour selected while the window kept the old one.
- CLI: plural counts read "2 aliases", "7 classes" (were "2 aliass"); `archo subscribe` previews say "1 subscription".
- Text interface, Can it run: the memory item's lines are joined in the interface's language (Chinese read `…内存里。; 显存…`).
- First-run guide, "read the mods" (Chinese): the last phase read "indexing def references" in English; it reads in Chinese.
- SteamCMD downloads: the first Workshop download after installing SteamCMD reported "SteamCMD did not exit successfully" although the Mod was downloaded (SteamCMD exits 7 from the run in which it updates itself); it is reported downloaded.
- Text interface opened before any game was added (double-clicking archo.exe on a new machine): it says "No game added yet." in the interface's language instead of the English error "no Game Config configured (…)", which repeated the hints below it. Keys that need a Game Config say the same instead of opening empty pages or starting a scan; help (`?`), the command line (`:`) and `E` still open.
- Text interface, Mod library: a Mod with several copies showed its row a few cells short without its copies count, which pulled the details column on that line to the left; the row keeps its width and "4 copies".
- Text interface with a Chinese input method left in Chinese mode: shift+1–8, `?`, `:` and the other punctuation keys did nothing (the input method sends `！`, `￥`, `……`, `？`, `：`); they work as on a US layout. Text boxes keep what was typed.
- Uninstall preview: the mod-list step says that RimArcho's backups of Mod settings are deleted too (it said only "RimArcho's backup"); the changed settings stay.
- `archo env add` that fails partway (after the Game Config was registered) no longer leaves a half-added Game Config: it is removed again, with any installation records it created; settings files it copied stay.
- Text interface, launch page: while the game runs, `L` and Enter say so instead of trying to launch, and the editor keeps the list it has open.
- Window: a confirmation that finishes loading after you opened another dialog (moving a folder in Settings, changing a game folder, removing a Game Config) no longer replaces that dialog.
- Window: arrow keys, Home and End in choice lists skip choices that are unavailable.

## 1.0.9 — 2026-11-10

- **Linux (experimental):** releases now include `archo-<version>-linux-amd64.tar.gz` with the terminal UI, CLI and MCP (no window). It is tested only with synthetic data so far, not with a real game on Linux; expect bugs and back up saves and Mod lists. The Linux entries below are part of it.
- Copying a game folder (for a second game) no longer breaks Archo Monitor. Explorer copies RimArcho's `_Archo_` links as plain folders; the copy's Monitor folder was then picked as the newest Monitor, every list switched to it, and launching refused with "is not a link; rename or remove it yourself". RimArcho now ignores those copies when it picks Monitor, puts the installed Monitor back in lists already switched (on the next scan, in either game), and Can it run offers R "move the copies to the recycle bin" for folders whose files match what RimArcho links there exactly; launching then makes the links again. Any copy with changed files stays where it is. Launch and save point to this repair. CLI: `archo links check` / `links repair` show the action `recycle copy`, and a repair reports `recycled: <n>`; MCP `check_links` / `repair_links` return `copy` and `recycled`.
- Home page: right after an update, the last game's line said "no errors" while its log was still waiting to be analysed again; it now says "not checked yet" until the analysis has run (window and TUI).
- Linux: SteamCMD runs with a home folder of its own (next to the program, or under the cache folder), so it can no longer change the desktop Steam client's settings or library list.
- Save recycling and backups report only files successfully moved or copied; total failures no longer print a success headline. Save renaming refuses another file's name regardless of case on every platform, while allowing the same save's case-only rename.
- Linux: relative `XDG_DATA_HOME` values are ignored for the data directory; an unset or relative `HOME` without an absolute XDG value now gives a clear startup error.
- Game log: audio and asset paths with spaces keep machine-independent problem references with either separator, including Linux logs read on Windows. Existing stable Windows keys are unchanged; the log attribution cache version is now 22, so stored reports are rebuilt when read.
- CLI/MCP status labels findings and logs from a previous run while the game is running, and points to `archo log --follow` for the current run. `env detect` also lists additional games found in registered Steam libraries, including unmanaged Proton copies without a manifest; `env add --detected` keeps its first-match choice.
- Linux: texture optimisation uses native todds from `PATH` or `textures.todds`; missing-tool guidance explains how to provide it, and no Windows encoder download is offered.
- Linux: Can it run and CLI/MCP memory estimates read system RAM from `/proc`; unavailable RAM or VRAM is described clearly in English and Chinese. Process probes and game test reports now read resident and peak memory on Linux.

- The rebuild guide lists Windows and native Linux Managed folders; environment command help describes game directories on either platform.
- Game log: shared Linux logs now identify Mods from native absolute file paths and keep problem references stable across installation locations. Running-game checks also recognize game directories reached through filesystem links.
- CLI: `archo help` describes `update` as checking for a newer RimArcho and showing update options; Windows installation behavior is unchanged.
- The text interface switches pages faster with shift+1–8. Launch, Can it run, Mod library and texture optimisation keep keys responsive while reading, reuse unchanged data, and discard replies for pages already left. Page layout and styling are unchanged.
- Text interface (`archo`): a new top, laid out like the window's top navigation. The first row lists the pages in the window's order with their `shift+1`–`8` digits: start, can it run (with its count in amber), game log, lists, packs, mod library, texture optimisation and game, the current one highlighted, and the game and version at the right; at 80 columns only the digits and the current page's name. The second row shows the page's tabs, such as all lists and each list (► marks the one that launches) or all mods, updates, add a subscription, search files and mod settings, with the page's actions on the right: unsaved changes with `w`, `L` launch, and on Can it run its conclusion with `F` `A` `l`. Click a page or a tab to open it. The position (`Enabled  3 / 26`, `can it run  1 / 3`) moved to the status line. Area headings are bold with their digit at the left edge, rows put the source and status icons together before the name, the update sources line up in columns, and grey text is lighter so it stays readable on dark terminals. New setting `ui.tui.theme auto|dark|light` (`archo config set ui.tui.theme light`): a light palette for light terminals; auto asks the terminal for its background, and reads dark on Windows, where terminals do not answer. Terminals with 256 or 16 colours (a plain xterm) get colours picked for them, so faint text and the selected row stay apart.
- RimArcho.exe no longer starts invisibly when the Microsoft Edge WebView2 Runtime is missing: a message box says so and where to get it. Under Wine or Proton it says WebView2 does not run there and to use `wine archo.exe` in a Linux terminal. Other errors while opening the window also show a message box instead of nothing.
- archo.exe in a console that cannot draw the text interface (Wine's console window, where it showed lines of `[K`) now prints a short plain explanation instead: open RimArcho.exe, or under Wine run `wine archo.exe` from a Linux terminal. A double-clicked window waits for Enter before closing. With no game added, the text interface also points to RimArcho.exe and its first-run guide.
- Right-clicking where the window has no menu of its own (a page's background, a heading, a picture) no longer opens the browser's menu with back, reload, save as and print. Text fields still offer cut, copy and paste.
- Can it run: when two Mods conflict (declared incompatible, or often failing together in shared logs), the choice of which to disable now says how many enabled Mods need each one, directly or through another Mod, with the first names: "disable Lib (12 enabled mods need it: …)". The one fewer Mods need comes first. Before, both options looked alike, and disabling a library to settle a conflict left every Mod built on it without its dependency.
- Can it run: a missing dependency can be ignored (`I`). It stays hidden for every Mod that needs it until one of them updates, and the window after Sort no longer lists it; completing dependencies leaves it alone. Before, only `archo problems ignore` could, and the window after every Sort still asked for it.
- Can it run: when an enabled Mod looks like a continuation of a missing dependency (same name once tags such as "(Continued)" and version numbers are dropped, or a shared packageId part), the item offers "<it> replaces it" (`R`). That adds a rule: the Mods that need the original count it as met, load after the continuation, and the details show it "(in place of …)". For continuations that the community replacement list does not know, and that the dependent Mod does not list as an alternative.
- CLI / MCP: `archo rule add replaces <A> <B|pkg:id>` and `manage_rules` `kind=replaces`: A stands in for B wherever a Mod requires B. B need not be installed. `rule list` shows it as `pkg:<id>`; the rules fold in the list editor offers the kind too.
- Saves: the right card now shows what content the save references. Each Def the save names is traced to the Mod that provides it (the last one in the save's load order when several do) and sorted into categories players know, by the game's own rules: factions, races, animals, weapons, apparel, buildings, items, terrain, research, xenotypes and genes, recipes, appearance, and so on. Besides `<def>`, the save's pawn kinds, materials, research progress, xenotypes, memes, quests and bills count too (the saves are read again once). Corpses, blueprints and frames count as the thing they stand for, and the genes Biotech makes for each skill and drug (aptitudes, dependencies, addiction immunity) as genes of the Mod with that skill or drug. A name that Defs of several types share is listed once in a category. The card says what it cannot see yet: Defs added by patches, terrain and biomes, and content left only in history. Each Mod's Def facts are rebuilt by its next analysis (opening Can it run starts it; a later change to what the analysis records starts it again); until then the card names it "not analysed yet", leaves out what it adds and offers to make a list from the save and analyse it. After the update the saves keep their in-game days, colonies and names: a save is read again only when its content card is opened. The card lists each Mod with its count and categories ("12 weapons, 3 factions"), opens to the Defs' names, and has a chip per category that filters the Mods to it. Defs that no installed Mod of the save defines are counted as "source not found". A Def only means the content was present, not how much it was played. The "at this save" numbers, the colonists and the events are gone from the card (`archo saves show --play` still has them).
- CLI / MCP: `archo saves show <save> --content` (`get_save content=true`) lists the same: category totals, each Mod in save order with its categories, sources not found, Mods not analysed yet (the first six and how many more) and what is not covered, within 40 lines (`--all` for every Mod). A name that several Def types define counts once per type. The `get_save` parameter descriptions are shorter.
- Database upgrade 34 → 35: Def facts gain the fields the categories need. Existing facts stay usable; each Mod's are re-read by its next analysis.
- Saves: "over time" moved to the middle card, under "can it load?", and zooms: −, + and fit in its heading, ctrl+wheel around the pointer, or + − 0 on the keyboard, up to 64×. The buttons and keys zoom around the selected save while it is in view. Zoomed in, drag the chart or scroll it sideways; on a long colony the saves no longer sit on top of each other, and saves made on the same day sit side by side so each can be clicked.
- Saves: Back up and Recycle now act only on the files you were shown. Before, a confirmation named the save by its place, so an autosave the game rotated onto the same name between the preview and the confirm was recycled instead. A save that changed since the preview is now refused ("preview again"). A backup now goes to the folder its preview named, a second backup in the same second gets its own folder (`-2`), and two saves of the same name from different folders no longer collide (the second goes into a subfolder named after its pack or game). The recycle preview says the game is running, as the confirm did. CLI: `saves backup` / `saves trash` take `--token <t>`, and the preview's confirm command includes it. MCP: `manage_saves` takes `token`; `action=trash confirm=true` without a token is refused.
- Saves: running `archo saves fix <save> --mod <id>` (or MCP `mods=[…]`) again once Steam had downloaded the Mod failed with "no Mod of … to enable"; it now says the fix is already done. `saves fix --launch` no longer waits forever for a list entry that cannot be downloaded (no Workshop ID, an item installed under another packageId, or removed from the Workshop): it launches and says which Mods it is without. `saves show --current` again compares with the colony's previous save. `saves list --colony` also finds a colony by its saves' own name after you renamed it. Renaming a save can no longer replace a file that appeared under the new name meanwhile. Fixing a save while a launch starts no longer works from a check made before it.
- Saves: colony names you set no longer come back or spread. Clearing a name cleared it under one key only, so an older name could reappear; a name set before a colony's full read could pass to a later colony saved under the same file name. Clearing now clears every key the colony answers to, the name moves with the colony when it regroups by world, and names of colonies that are gone are removed (`archo saves index`).
- Saves page: an open name field no longer loses what you typed. Renaming a colony while the full read reported progress, or renaming a save while the game started or exited, a fix was applied or undone, or the list was read again, rebuilt the field and could save half a name. The field now stays as it is until you press enter or esc or leave it; the card is redrawn afterwards. Renaming a save and then clicking another save keeps the other save selected (before, the renamed save was selected again). Copy and Make a list no longer make two copies or lists on a double click. Back up then Recycle in quick succession shows the Recycle confirmation, not whichever preview came back last. Coming back to the page no longer shows the last visit's confirmation, name field or quick fix, and `enter` does nothing until the new check is shown. A save that disappears in a re-read no longer leaves its cards and fix buttons on screen. Rescan says "saves read again" only when it did. After Back up the focus returns to Back up; after renaming, to the tree. Opening Saves while the game exits no longer leaves the page marked as loading for screen readers.
- Saves page: a save that cannot be read is no longer read again on every visit. Each visit started the background task "read the saves" for it, which failed with "archo saves list" as its reason. The failure is now remembered until the file changes, the task ends with "read N saves; M could not be read: <reason>", and the save's right card says why. `archo saves index` skips such a save (`saves show` still tries it again). An empty `.rws` file is no longer counted as unread forever. Leaving the page while the saves were being listed no longer starts the background read with no page shown. The status line no longer keeps "reading the saves n/m" after the read finished. Selecting saves quickly no longer leaves the last one waiting for its play facts, and a save the game wrote again after the listing is no longer read over and over until the next listing. `archo saves index` also removes the cached reads of deleted saves when it is given saves to read first (as the Saves page does).
- For AI agents (`archo mcp`, CLI): Archo now spots a Mod DLL that will fail in the game because it was compiled by hand. A Mod built against a publicized game (Krafs.Publicizer) only works when its build skips verification, which `dotnet build` with the publicizer does and a plain `csc` build does not; the game then throws `FieldAccessException` at start. `get_mod` / `archo mod` show `built: !!` with the private members it uses when the DLL at a project's output is such a build, and `read_source mode=diff` / `archo source diff` say `access: !!` for the whole new build (it no longer treats `IgnoresAccessChecksTo` as enough: the game ignores it). `read_source` / `archo source` on a Mod that ships its source add a `shipped:` line: the project, the source file of the type you asked for, and that a change is rebuilt with `dotnet build`. The agent instructions, `guide topic=rebuild` and the log's `saves:` note now say to rebuild only with `dotnet build` and to `test_run` a rebuilt DLL even when the error needs a save. In my tests, agents that compiled a Mod with `csc` handed in DLLs that broke the game's start.
- Core and the DLCs now carry a tag `RimWorld`, kept on top in tag lists and the tags popup, so the game's own content sits together in the Tags views and tag filters. It is added once: remove it from a DLC, or delete the tag, and it stays gone; a DLC you install later gets it. A pinned-tag list you set yourself gets `RimWorld` in front once.
- Click a tag in a Mod's details to see it in the Tag manager, in the list editor and in the Mod library: its note, colour, rename, merge and delete. Choosing a tag colour now shows at once, without a pause. All tags in the Tag manager can be sorted by Mod count or A–Z, either way round. The Tag manager no longer lists similar tags with merge in; merge into… stays. Its colour swatches have room between them, and in the editor its facts line counts the tag's Mods that are not enabled (it showed more "shown" than the tag had Mods). A tag group's header in Disabled shows its colour as a band, like the rows, instead of a dot.
- The list editor's Disabled column can be grouped by tag: the new individual | tags switch at its top (or `v`) puts each Mod under a header for its first tag, in the Tag manager's order, with untagged Mods last under "no tag". Click a header, or press ← / → on it, to fold or unfold that group; the buttons beside the switch fold or unfold them all. The choice and the folded groups are remembered. Each Mod appears once, so selecting, enabling, dragging and undo work as before, and the filter bar shows only matching Mods in each group. With the cursor on a tag's header, the details show that tag as the Mod library's Tags view does: its note, colour, rename, merge and delete (F2, n, M, Del), and every tag to jump to.
- The suggested tags (the tags popup, the Tags view's kept-on-top tags and `archo config get tags.pinned`) are now in English for English players: framework, visuals, apparel, … Before, every player got the Chinese list. The list follows the display language until you change it; a list you have pinned or unpinned stays as it is, and tags already on Mods are not renamed. Renaming a tag to a suggested name that no Mod has yet (such as `visuals`) now renames it instead of saying the tag already exists.
- The window jumps around less. Moving to another Mod in the list editor or Mod library no longer redraws its details in up to four steps (name, then picture, then a note pushed in above it): the details shown stay until the next Mod's are ready, then change at once; they dim only if that takes more than 0.2 s, and their buttons cannot be clicked meanwhile. The details keep room for their scrollbar, so text no longer re-wraps between Mods. Opening a page no longer flashes a small grey loading card first: the card appears only when a page takes more than 0.21 s, and the page fades in when it is ready.
- The window jumps around less, part 2. The launch page no longer redraws, restarts its motion and scrolls back to the top every time the window gets the focus; only a change redraws it, and a redraw keeps the scroll. The problem line under ▶ no longer lifts the headline when it comes, goes or is hidden. All lists' memory estimate no longer pushes the sections below it down when it arrives. The editor's head stays on one line, so a problems or unsaved count appearing no longer moves the lists down. Scroll areas keep their scrollbar's room, filter chips keep their width when turned on, and the editor's number column no longer widens at 100 mods. At start, the top navigation, the sidebar width, the column sizes you chose and the window buttons are there from the first frame instead of moving in a moment later.
- In light mode, the window buttons (minimize, maximize, close) over the launch page's picture are dark again; they were faint grey with white dividers.
- Search files: the "available values" list under the files and mods fields now opens over the results card instead of being cut off at the bottom of the query card.
- Tags can have a colour, and it shows in the lists. Give a tag a colour in Mod library › Tags › the Tag manager: one of eight colours, or one of your own. Every Mod whose first tag it is gets a thin coloured band at the left of its row, in the list editor (Enabled, Not enabled, Staging) and in the Mod library. Neighbouring Mods of one colour join into one band, so a group such as "Gravship" or "Medieval" reads as one block in the load order; in Not enabled the band runs from the Mod's name down through its copies. A Mod's tags now have an order: drag them in its details (or in the tags popup, `g`), or move one with `ctrl+←` / `ctrl+→`; the first tag decides the Mod's colour and is underlined when it has one. New tags go last; existing tags keep the alphabetical order they were shown in. Renaming or merging a tag keeps its colour and its place on each Mod. CLI: `archo tag color <tag> <colour>|--clear`, `archo tag order <ref> <tag…>`, and `tag list` shows colours. MCP: `set_player_data` actions `tag_color` and `tag_order`. The database moves to schema 34 on first start; an older RimArcho cannot open it afterwards.
- "Use this copy" in a list's details no longer lags. The Enabled list now swaps the copy in at once (it used to wait about a third of a second on a big library) and the details show the new copy in under 0.2 s instead of about 0.55 s.
- The status line at the bottom stays one line. A long or multi-line message used to wrap onto two or three lines inside the one-line bar, which then showed the middle of them; now the message is cut at the end with an ellipsis, its mark and undo link stay visible, and hovering shows all of it. Screen readers still hear the whole message. New messages fade in instead of sliding up. The key hints make room for the message and show only whole keys.
- Settings › interface › motion: follow system (default) or reduced. Reduced stills the window as Windows' reduced animations do: the sidebar, lists, pages and status line change without sliding or fading. CLI / MCP: `config set ui.motion auto|reduced`; the settings list shows `ui.motion`.
- Settings › interface › accent colour has a custom colour: the last swatch, "custom…", opens a colour picker with the named accents on top, a colour wheel, a lightness slider and a hex field. A named accent applies at once; Enter or "use colour" applies the wheel's colour. RimArcho makes light and dark shades from it, and accent text stays readable on both themes. The picker works by keyboard too: arrows turn the wheel, Enter picks, Esc closes without a change. CLI / MCP: `config set ui.accent` also takes a colour such as `#3a7bd5` (stored in lower case). The terminal interface keeps its own colours.
- A new RimArcho version is easier to notice: an "↑ update" button now sits beside Settings in the sidebar (with the navigation at the top, left of background tasks). It opens Settings › About with "install update" ready, and appears as soon as a check finds the version, without restarting. Its × (on hover) hides it until a newer version comes out; the line on the launch page and the Settings row stay.
- Game log: for a NullReferenceException in a Mod's code, a cluster now says who passed the null. Archo reads the instruction at the frame's IL offset (now shown with the frames, `+0x76`), finds the value it dereferences and, when that is a parameter, follows it down the stack, through a Harmony patch to the caller of the patched game method, to the frame that passes `null`: `trace: frame 1 …Postfix +0x76 dereferences parameter recipient (ldarg.3, ldfld Pawn::genes)` … `frame 3 ColonistMutterings.PlayLogEntry_Mutter..ctor passes null for recipient (ldnull at IL_0007, base constructor call)`, with a `game contract:` line saying how the game's own calls pass that argument. The Mod passing the null becomes the source and comes first; the Mod whose patch threw becomes a patcher. Before, both were "source, likely" in an arbitrary order and four of six agents in an evaluation fixed the wrong Mod. When the trace cannot be read, nothing changes. Candidates with the same role are now ordered by their evidence, the same way on every run. Constructor frames (`Ns.Type..ctor`) now count as their Mod's frames. ArgumentNullException is traced from the parameter its message names.
- CLI / MCP: `archo source callers <Type.Method>` (`read_source mode=callers type=…`) lists every call of a method in the game and the enabled Mods, base-constructor calls included, with what each argument is (null, this, a parameter, a local, a field, a constant) and a summary per parameter: `recipient: game sites 7, null at 0 …; Mod sites 1, null at 1 (…)`. Needs no ilspycmd.
- CLI / MCP: `archo source diff <old.dll> <new.dll>` (`read_source mode=diff dlls=[old, new]`) shows what a rebuild of a Mod changed: methods whose code changed, added or removed methods, references to other assemblies' members (a non-public game member used without a publicizer is flagged), assembly versions and attributes.
- Game log: a "Tried to add pawn … to lord … already a member of lord" cluster lists the source Mod's `Lord.AddPawn` calls and whether each method checks `GetLord()` first; `next:` opens the unchecked one.
- Game log: a "Collection was modified" error now names the Mods whose code changes the list, not only the Mod whose loop threw. The code that changed the list never appears in the stack, so a comp removing itself in its tick (SSR Body's) went unnamed while a profiler's loop took the blame. Archo now reads every enabled Mod's assemblies (no decompiling, cached per DLL) for code that adds, removes or reorders a pawn's comps: comps doing it in their own tick come first, with the def or patch that attaches them (`attached to: ThingDef/Human by … Patches/Patches.xml:41`), and code elsewhere is marked "not in a tick method". The cluster also says which list the throwing loop walks (`enumerated: ThingWithComps.AllComps (foreach in …)`). Tick crashes with an index error get the same list. The tick checklist now says what the game's loop survives (a swap, a comp removing itself as the last one) and what throws.
- Game log: a Mod whose only link to an error is a Harmony patch on a method further down the stack, below the Mod code that threw, is now listed instead of being a candidate. It no longer counts as an implicated Mod, so source searches no longer default to it.
- CLI / MCP: `archo log --cluster` / `analyze_log cluster=` add the roles `list-modifier` and `listed`, the `enumerated:` line and the `comp list changed by` section; source searches without `in` (`--implicated`) cover list modifiers and leave out listed Mods. For a patch line whose target is chosen in `TargetMethod()`, the next step is `archo harmony --mod` and the patch class, not `archo harmony --target <frame>`, which found nothing. `archo harmony --target` / `get_harmony target=` lists such patches when the newest log shows them on that method, or counts them, instead of saying there are no declared patches. Simple `TargetMethod()` bodies (`AccessTools.Method`, `Constructor`, `PropertyGetter`, `typeof(T).GetMethod`) now give a real target, marked `target read from TargetMethod()`; Mods are analysed again once.

- Closing the window while a pack is being created no longer stops the build and deletes what it copied. The build goes on in the background as the create-pack message says, and the task record says so; the Packs page shows how it ended.
- Database maintenance runs again after you leave the plans, launch or texture page while it is still measuring. Before, the abandoned measurement kept saying "loading", which held maintenance off until RimArcho restarted.
- The command line (TUI `:` and the GUI command panel) no longer crashes when completing after `--`, as in `config set -- `. Words after `--` and the `--env` value are counted the way the command itself reads them.
- The home page no longer says "ready to play" from a check that is older than the list: such a list shows "the files changed since the last check" until it is checked again (problems found by the old check still show).
- Launch is no longer blocked for good by "link operation needs recovery" (`mods.link-recovery`) after a link step failed. A link step that failed without changing anything (for example, a Mods folder on a drive that cannot hold junctions) no longer leaves a recovery behind. An interrupted link whose Mod folder has since been deleted can now be cleared with "repair the previewed links" in Can it run (or `archo links repair --all`), which closes the record; before, repair refused it as "needs manual resolution" and there was no way to clear it.
- Updating SteamCMD subscriptions no longer looks frozen. Background tasks used to show every Mod as "working on it" and 0 of N done until SteamCMD had finished the whole group (hundreds of Mods, often many minutes); now the Mod SteamCMD is downloading is the one worked on, and each Mod counts as done as soon as SteamCMD confirms it, with its record written then. "Stop the rest" now stops SteamCMD at once instead of after the whole group; Mods already downloaded stay done.
- Automatic checks (Mod, tool, community data and RimArcho updates, the LoversLab session) closed or killed mid-check no longer wait a whole interval (up to a week for tools) before the next try, and no longer show "checking" forever: after 30 minutes the status says "check interrupted" and the check runs again when its interval since the last finished check is up. A check during which a subscription update recorded its pending count no longer loses its result.
- Changing a game's folder (`env set --game`, Settings › folders) no longer leaves the environment on the old folder while its Mods point at the new one when something fails halfway; the game and its Mod folders change together. Moving a folder in Settings ("move to…") that cannot record its new place now moves it back and reconnects the games' links, instead of leaving them broken; across drives the original goes to the recycle bin only after the move is recorded. A folder removed while it waited for a re-binding answer no longer brings that question back when added again.
- Moving a Mod copy from the game's Mods folder into the lock library (Mod library "move into the lock library", `archo pin --move-to-library`, `pin --update`) no longer leaves the games' links to it pointing at nothing when the move fails halfway, which blocked launch with "link operation needs recovery". A failure now puts the folder back in Mods and reconnects the links; across drives, the original goes to the recycle bin only after the move is recorded.
- Internal: the unused pick-session API (`App.BeginBatch`, `Store.FinishBatch`) and its `credit.remap` counter are gone. The TUI stopped using pick sessions before 1.0 (each verb is one edit and one undo step, 16 §3); a session ending could have dropped or overwritten another window's edits to the same list. No visible change.
- Deleting a Mod permanently, or deleting Mods during uninstall, no longer links a game back to a half-deleted Mod folder when a file in it cannot be deleted (for example one a virus scanner holds open). The game links stay removed and the message names the folder left to delete yourself; before, the game kept loading what was left of the Mod. A deletion that removed nothing still puts the links back.
- Updating RimArcho no longer leaves the program folder without `RimArcho.exe` or `archo.exe` when RimArcho is killed or the PC turns off while it replaces its files. RimArcho now writes down what it is replacing first; the next start finishes the update and checks the new program, and puts the old version back only if the new one does not start. Before, the next start deleted both the old and the new files.
- Resubscribe (Mod library, `archo workshop resubscribe`) no longer leaves Workshop Mods unsubscribed when it is stopped or interrupted. Stop now takes effect before RimArcho unsubscribes, or during the download, never in between. If RimArcho closes or loses Steam between unsubscribing and subscribing again, Can it run shows "a resubscribe was cut short" with "subscribe again"; before, the Mods were simply gone with no trace.
- Data kept right after errors and interruptions:
  - Finished tasks no longer disappear from Background tasks when writing `tasks.json` fails.
  - Saving after undoing a checkout is recorded as your own save, not as "restored vN". A checkout writes the list and where it came from together.
  - A new list gets Archo Monitor added even when it reuses a deleted list's ID.
  - Copy folders left by an interrupted copy (`<name>.archo-tmp`) are no longer scanned as Mods.
  - Switching games scans again when the last scan failed or was cut short.
  - A pack's HugsLib settings copy that was cut short is completed on the next prepare, instead of counting as the pack's own.
  - A Workshop manifest that Steam is rewriting (`appworkshop_294100.acf`) no longer marks every Workshop Mod as subscribed; the last known value is kept.
  - A failure to record converted textures stops the run instead of marking the Mod optimised.
  - Feedback relay: a report GitHub failed to file no longer counts against the shared daily limit.
- Things that stayed stuck:
  - A drive taken offline that held only finished link operations no longer blocks moving, removing or updating Mods in other games.
  - A compaction cut short (RimArcho closed or killed during VACUUM) no longer blocks the next one for 15 minutes; automatic maintenance on a disk too full to compact finishes without compacting and waits its week, instead of running at every start.
  - A todds that failed its first check (held by a virus scanner, a timeout) is found later without restarting RimArcho.
  - A LoversLab login made in another RimArcho window works here without a restart.
  - "Directory check failed" in Can it run disappears once the directories can be read again, and no longer repeats.
  - Restarting the game while RimArcho follows its log no longer drops the old run's last log entry.
  - The log reader no longer crashes on a Monitor phase marker inside an XML Extensions error.
  - TUI: a command run from ctrl+k while a Mod's full details are open opens its page ready for keys; before, keys were ignored until esc.
  - TUI: after a failed uninstall, a scan, command or analysis that was running no longer stays busy.
  - TUI Mod library: when the list cannot be shown, keys and clicks no longer move, select or delete Mods that are not on screen.
  - Window: ctrl+k while the first-run folder dialog is working no longer leaves the window unresponsive. A failing step at start-up shows its error and the window still opens its first page.
  - Window: a first-run scan that fails to start no longer blocks scanning again.
  - Window: installing todds, or a conversion ending, while textures are being counted no longer leaves "stop counting" up for good. An empty import file no longer disables Preview.
  - Window: ctrl+j on Background tasks returns to the page you came from, including a pack's page; opening Tasks another way no longer hijacks the next ctrl+j.
- No more double actions or stale details:
  - In Can it run, Enter no longer runs a confirmation whose item has disappeared.
  - Enter pressed twice no longer adds a rule twice or subscribes twice; a double-click no longer repeats a save's backup or recycle.
  - Moving quickly from one Mod to another and back no longer shows the other Mod's name in the details.
- Reading a Mod's code (Mod details, CLI `archo source`, MCP `read_source`) works with ilspycmd 11, which installs as a `.cmd` launcher that Archo did not find. Its "You are not using the latest version of the tool" notice no longer appears as the last lines of the code, also in code read before. A type asked for under the wrong namespace now says "type … not found" and names the types with that name (`RimWorld.PlayLogEntry_Interaction` → `Verse.PlayLogEntry_Interaction`); before, the notice was shown as if it were that type's code.
- A test run no longer calls an error "fixed" that it could not have seen. When your log shows an error after loading a save, and the log has no Archo Monitor phases, the game's own "Loading game from file" and "Initializing new game" lines now place it: a menu run counts it as not reached, and so does a map run unless the error came after a new game.
- Testing a pinned copy works when the original sits in `Mods/`. The original (a folder or a link with the same packageId) is moved aside into `_ArchoTestHidden` next to `Mods/` for the run and moved back after it; before, the run quietly loaded the original and compared its errors as if it had tested the copy. A run in which any listed Mod was not loaded now has status `incomplete`, starts with "NOT TESTED: these Mods were not loaded: …" and reports nothing as fixed. The report lists the folder the game took for each Mod (`loaded`: from Archo Monitor's log where it says, otherwise from what was in the game folders), and a Mod missing from the game's running list moves to `missing`.
- Test runs can load a save: `archo test run --save <save>` (MCP `test_run` `save`: a `list_saves` reference or name, or a `.rws` path) copies the save into the run's own data, lets the game load it at startup, plays 4,000 ticks (`--ticks`) and stops; status `save`, `phase: save`. It is refused when the save has Mods the plan lacks, is from another game version, or is cut off. `--ticks N` also plays N ticks on a test map.
- CLI / MCP test run reports add `map_seconds` (from the new-game or load line to the stop), `ticks`, `ticks_reached`, `ticks_estimated`, `phase`, `timeout_seconds`, `hidden`, `loaded`, `loaded_total`, `save`, `not_tested` and `last_log_line`; the text has a `map:` line saying how long the map ran. A timeout says "NOT TESTED: the game did not reach the map within … s …; last log line: …". Map and save runs without a time limit get twice the previous such run's load time, at least 10 minutes; the `test_run` description gives rough durations.
- In a test run with ticks, "… is referenced … but is not deep-saved" gets a note that it was written while the run's own autosave was saved in dev mode. It still counts as an error, because a Mod can cause it.
- Game log (CLI `archo log`, MCP `analyze_log`): the cluster list shows exceptions and Harmony-stack errors before missing textures and other low-severity rows, and `next:` points at the first one listed. Cluster IDs (`log:E<n>`) keep the order of first appearance. Every missing-texture cluster now says it is rendering only (the game draws a placeholder; nothing stops loading or playing, and it causes none of the other errors), not only the ones whose own mod asks for art it does not ship. Test runs give the same note for `texture-missing` clusters, also in `fixed`.
- Game log: a Mod that is only named by the message or a lower stack frame ("mentioned") no longer gets the cluster as its own problem. Mod details (`get_mod`), `list_problems subject=` and a test run's `mods` leave it out; `archo log --cluster` still lists it with the role `mentioned`.
- Game log: a cluster of an exception or Harmony-stack error with two Mods as source candidates (for example a `[Mod]` message prefix and the top mod frame) gets a `fixes:` line telling the reader to report both and name both fix places: a guard or a different loop in the one that threw hides the error, undoing the action of the one that caused it removes it. The `diagnose` guide says the same in step 5.
- Game log: when the log shows `Loading game from file <name>` and that save is not in the environment's `Saves/` (nor in the `Saves/` beside the log), `archo log` / `analyze_log` print `saves: this run loaded save "<name>", which is not in Saves/; a test run cannot reproduce errors that need that save`. Pasted text has no `Saves/` to compare with and prints nothing. The log cache version is 17, so old logs are analysed again.
- MCP: the suggested commands in tool results are tool calls, not CLI commands: `next: read_source ref=iqmu type=ModA.CompBroken`, `fix: edit_loadout action=enable refs=[z3bw]`, `(analyze_log all=true)`. A command that has no tool or uses an option the tools lack (`archo plan use`, a `<ref>` placeholder) stays as written. The CLI is unchanged; `guide` text is not rewritten.
- MCP `guide topic=diagnose`: a Mod that invokes code generically (a static-constructor runner, a type lister, a log uploader) is not the owner of the error it triggers, but a Mod whose own call passes the arguments can be: compare what the callee dereferences with what the call passes, using the `trace:` lines of `analyze_log cluster=` and the callers view of `read_source` when they are shown. For a Harmony patch of a game method at the top of the stack, read the caller's source and the callee's vanilla contract before blaming the patch.
- CLI / MCP: Mod details (`archo mod`, `get_mod`) show whether a copy's files are the Workshop copy's, from the last scan and without hashing again: `provenance: files identical to Workshop 1001 (#6olp), as of the last scan`, `2 files differ from Workshop 1001 (#6olp), as of the last scan: Defs/Extra.xml added, Defs/ThingDefs/prov.xml changed`, or `files are the Workshop copy ws:1001 (this folder links to it)`.
- MCP: a list parameter accepts a string. `read_source` `in`, `edit_loadout` `refs` and every other parameter that is a list of strings take an array, a JSON array written as a string (`"[\"#vyzv\"]"`, which some clients send for an array) or one bare reference (`"#vyzv"`). The call used to be refused as invalid arguments, and agents then made one call per mod.
- CLI / MCP: every source search says what it did not search. `archo source search` and `read_source search` end with `searched 2 of 39 enabled mods (implicated); not searched: 37 (scope="active": repeat while pending)` (the CLI names `--active --yes`); a search of named or implicated mods that finds nothing adds that the code may be in the mods not searched and names the call that covers them. A call that decompiled something prints `decompiled: N assemblies now, X s`. The `fix:` and `next:` lines of the tick patterns (who modifies a list: `AllComps.Remove(`, `AllComps.Add(`) search every enabled mod instead of the two mods of the stack, which are usually patchers.
- MCP: `read_source` `scope="active"` is described as it works: each call decompiles for the 10 s budget, returns what it found with `pending`, and is repeated until nothing is pending. Its guide topic no longer says "many minutes, only when the user asks". A search over the game's code and the mods is also about twice as fast once the code is decompiled (8 file readers, a byte search that copies nothing, ilspycmd's version read once). The first `game=true` search of a game build still decompiles the whole of `Assembly-CSharp.dll` in one run that the 10 s budget cannot stop; it is done once per game build.
- CLI / MCP: reading a type shows more of it. `archo source <ref> --type T` and `read_source ref type` show a type of at most 250 lines and 12 KB whole (they showed the first 34 lines); a longer one shows its first 34 lines and a `members:` line (`ctor 51, Concerns 61, ... ToGameStringFromPOV_Worker 141`) and the line count it left out. New `--member <name>` (MCP `member`, `ctor` for the constructor) shows one member with its line numbers; `--all` still shows the whole type.
- CLI / MCP: a Mod that ships C# source says how to rebuild it. `archo mod <ref>` and `get_mod` get a `build:` block: the source folder and `.cs` count, the project, framework and packages, whether a publicizer (`Krafs.Publicizer`) is involved and what it publicizes, where the DLL lands and whether it is there, references that do not resolve from this copy, whether `dotnet` is on this machine, and the commands, `dotnet restore` then `dotnet build -c Release`, in the project's folder. Source without a project file is listed too. Nothing is run. New guide topic `rebuild` (`guide topic=rebuild`, `archo guide rebuild`): an empty NuGet cache does not mean the machine is offline (run `dotnet restore`), a Mod that uses a publicizer must not be compiled with plain `csc` (FieldAccessException at run time), a small separate Harmony patch Mod is usually better than rebuilding a large one, and `test_run` checks the result.

## 1.0.8 — 2026-11-08

- A Mod is no longer told it has a maintained replacement when it is that replacement. Some continuations keep the old Mod's packageId: Vanilla Base Generation Expanded (Workshop `3209927822`) was flagged as replaced by itself, and "subscribe" opened its own page. Use This Instead now matches by packageId only when the Workshop ID does not say it is another item; the import preview no longer suggests such a Mod as its own replacement either.

## 1.0.7 — 2026-11-08

- 1.7 memory estimates no longer ask for far more than the game uses. On an 8 GB graphics card, 270 Mods were shown needing 10–28 GB of VRAM and 18–41 GB of RAM, and Can it run and the launch button said memory was insufficient, although the game loads fine. 1.7 keeps only what fits the card in VRAM and loads other textures from its disk cache when they are drawn. The estimate now stops at that limit for the card it finds, for RAM as well as VRAM, and says how much loads on demand. 1.7 no longer warns about VRAM. "Insufficient memory" (red) now means the low estimate exceeds what Windows can still commit (RAM plus page file). An estimate above the RAM free right now is "memory is tight" (yellow); it is listed under worth knowing and no longer counted as something that breaks the game.
- CLI / MCP: `archo plan cost` adds "VRAM, textures on demand" and "RAM, commit available"; its totals and `estimate_memory`'s levels use the capped 1.7 figures. `--json` `capacity` adds `texturesOnDemand`. Saved estimates are measured again once.
- Use This Instead replacements work again. Two rules in the current Use This Instead list have a number where a packageId belongs, and RimArcho then dropped the whole list: no "use X instead" suggestions, and a continuation never stood in for the Mod it replaces, so the sort, Can it run and Start reported a missing dependency (for example MultiFloors needing Prepatcher). A malformed rule is now skipped on its own; the same holds for single bad entries in the Steam Workshop database and the community sorting rules.
- "Install and restart" from RimArcho's Workshop item no longer deletes the item's own `README.md`, `README.zh-CN.md` and `skills\`. Only the files the extracted release added are removed; a release extracted into a folder of its own inside the item is removed entirely.
- A community data update that fails no longer says it succeeded. `archo data update` (and MCP `maintenance` `data_update`) now titles a failure "community data not updated" and shows, for each dataset, why it failed or the version it kept, instead of "community data updated" with the version it had downloaded. The window's update task no longer lists the downloaded entry counts after its error.
- The green dot on Launch now really holds while the game runs. The page redrew itself every 30 s to refresh the RAM estimate, so the dot blinked again each time and the window did extra work; the refresh now changes only the launch button, and only when its RAM warning changes.
- Dragging a Mod in the editor over a row's ⚠ or ✖ badge no longer opens the badge's problem popover. The popover covered the rows below, so dropping on one of them did nothing and the popover stayed open afterwards. Starting a drag also closes a popover that is already open.
- Adding two games of the same version in one go now names them `1.6-2` and `1.6-3`. Both used to be named `1.6-2`: the first was added and the second failed. If two names you typed clash, or one belongs to a game you already have, nothing is added and the message says which name to change.
- Command panel (ctrl+k, GUI and TUI): typos now find commands in a large library. `subscirbe` found nothing, and `sotr` listed a hundred Mods whose long names merely hold those letters in order (Echoes of the Rim, Weapons Dominator…) before sort, which the window cut off entirely. Commands also match by their ID (`subscribe`, `analyze`, `search-files`), so `subscirbe` finds add a subscription and `analyze` + Enter runs check the files again instead of jumping to a Mod with "Analyzer" in its name. A typo at the start of a word now ranks above letters scattered through a name, and a one-word query matches the initials of neighbouring words (`vfe` finds Vanilla Factions Expanded). Exact prefixes still come first, and Chinese names still match by prefix, suffix and substring.
- The window sees a game that is already running when it opens. Started a second time with nothing changed since the last start, it often missed the game: Start offered "launch the game" and "save N changes", Can it run said the last run had no errors, and nothing refreshed after the game exited until the page was opened again. Whenever a list had no Harmony, the Archo Monitor check at start threw away the window's lists without changing anything, and the next read came from a saved copy that did not give them back. The check now refreshes the page only when it changed something, the saved copy gives the lists back, and the running game is followed through such a gap.
- Settings: RimArcho's version in About sits on the same line as "RimArcho is up to date" and the check button. In a narrower window (or at 150 % display scale) the button wrapped under it, leaving the version alone at the top of the row; a short value no longer reserves the width meant for folder addresses.
- Feedback: the relay now holds each sender to one report a minute. The limit let a second report through 13 seconds after the first; it is now kept exactly per sender, and a report GitHub refuses no longer uses up the minute. The relay's issue footer says "Feedback page" again instead of a Chinese word.

## 1.0.6 — 2026-11-07

- Reading the mods shows how far it is. With a thousand Mods the first-run guide's “read the mods” step sat on “reading the mods of 1.6…” for minutes with no sign of life; its row now counts the folders as they are read (“reading the mods of 1.6: 340 of 1,200”), then says when it saves and checks the files, and the bar moves with it. A rescan from the window shows the same count on its background task. The guide also no longer reads a new game's own `Mods/` once in the window before its scan.
- CLI: `archo scan --progress` writes `progress: scan mods <read>/<total>` lines, then `progress: scan saving <total>`, to stderr. Without the flag the output is unchanged.
- RimArcho's Workshop item is tagged like a Mod: `Mod` and the game versions Archo Monitor supports (`1.5`, `1.6`, `1.7`), instead of RimArcho's own version (`v1.0.5`). The release now shows as the description's last line, `RimArcho v1.0.6`, and the update check reads it there. 1.0.5 looked for the version tag: without it, 1.0.5 still finds updates through a subscribed copy of the item or GitHub.
- CLI (maintainer only): `archo workshop publish` always sets the description, so its last line names the release: `--description`, else the repository's `UserGuide/release-post.steam.txt` (and `--description-zh`, else `UserGuide/release-post.zh-CN.steam.txt`). With no English description it is refused. The tags are read from Monitor's About.xml; the dry run lists them and each description's last line.
- A game's first scan reads each Mod folder once. Archo Monitor used to be installed after the scan and then every folder was read a second time; it is now installed first and read in the same pass. With 3,000 synthetic Mods the first `archo scan` took 3.8 s instead of 6.8 s, and the first-run guide's "read the mods" step is shorter by the same amount. A new Monitor version, installed when the window opens or by `archo monitor install`, reads only the Monitor's folder instead of rescanning the whole library. `archo scan --game-only` reads the Monitor's folder too when it was just installed, instead of a second full scan.
- CLI: `archo scan` shows the Monitor's source and its new instance in the scan's own report, and `archo status`'s "changes" line now counts the whole first scan, not only the Monitor rescan that followed it.

## 1.0.5 — 2026-11-07

- Archo keeps you company while RimArcho works. A start that takes longer than 0.21 s shows Archo's chibi figure in the middle of the window with a usage tip below it, until the first page is drawn. A page that takes longer than 0.21 s to load shows the figure on the left of its loading card and a tip on the right, changing every 8 s. The first-run guide has Archo beside each step, winking, thinking while a scan or download runs and saluting when it is done. Waits take turns through seven calm expressions (neutral, smile, laugh, wink, think, ponder, yum), the next one every 4 s in a long wait. Settings › interface › Archo animation chooses off, still or animated (default). Animated uses one small WebGL figure (about 0.15 ms of CPU per frame) that stops when hidden and is released 10 s after the wait. Without GPU acceleration or with reduced motion, it shows still images. Nothing is loaded at start, so a fast start costs nothing.
- CLI: `archo config set ui.archo.motion off|still|live`.
- CLI (maintainer only): `archo workshop publish` takes `--description <file>` and `--description-zh <file>` to set the Workshop description from Steam BBCode files (Simplified Chinese as its own language), and `--preview <file>` for a PNG, JPG or animated GIF preview under 1 MB; without it the preview is the repository's `docs/assets/release-post/workshop-preview.gif`. The dry run shows each description's size. The Workshop item now carries both READMEs, both user guides and the agent skills (`skills/`) beside the release zip; the zip itself is unchanged from GitHub's.
- Opening the start page while it loads shows the loading card across the page. Before, the card was a thin bar in the page's first column, and Archo in it was cut off at the sides.
- First-run guide: choosing where RimArcho keeps SteamCMD, downloaded Mods and tools is now the guide's first step instead of a dialog over it, with the default folder filled in; "not now" is gone. The lead, the steps and the card line up on one edge, and Archo stands on the step bar's row behind the card instead of in a box beside it; she is no longer cut off at the sides anywhere. In "where to start" the three choices' buttons line up and the collection link field has room. A short line that could show under the title bar at 150 % display scale is gone.
- Saves: Can it load? now checks the save against what the Launch page has applied (your current game and list, or the pack you applied there). When the save belongs to another game, list or pack, the top line says how it loads with those ("with main and list default: loads as it is") and offers Switch and launch, which applies them and opens the Launch page; below it, the verdict, the rows and the Mod changes are for your current game and list, and each row's fix edits that list. The page no longer starts the game: Launch opens the Launch page with its button focused, and Fix and launch is gone. CLI: `archo saves show --current` and `archo saves fix --current` check and fix against the current list instead of the one the save matched.
- Saves: rename a colony. Rename in the right card's header (or double-click the colony name) gives the colony your own name in RimArcho; the name its saves give it (the faction, world, RimSaves folder or file name) shows on hover, the search still finds it, and an empty name brings it back. Save files and the game keep their names. CLI: `archo saves rename-colony <colony|save> [<name>]`. The database is upgraded to schema 33 on first open.
- Saves: the Mod list at the bottom of the middle card shows only how the list the save is checked against (usually your current list) differs from the save: Mods the list has and the save lacks green with +, Mods the save has and the list lacks red with −, load order ignored. Past 12 changes the rest fold behind Show all N changes; Show the full list unfolds the save's whole list in load order with the changes marked. A save with no list or pack shows its whole list folded. A Mod that is in the Mod library links to it there.
- Saves: Can it load? has a row for the last load of the save with separate error and warning counts ("the last load logged 69 errors and 224 warnings", when, the first message and how many kinds), also shown in Can it run's your last save card and the TUI. Warnings alone give a warning row; the first message prefers errors, then clusters that began during the load. Open the game log selects that cluster. It needs Archo Monitor, which names the save it loads, and a log already analysed (Can it run or the game log page); a run that loaded several saves says nothing about them. CLI and MCP `saves show` / `get_save` list it as `error last-load` or `warning last-load` with separate counts, how many kinds, its consequence and `fix: Open the game log (archo log --file …)`.
- Can it run: the last run's errors are three cards instead of one: errors when the game last started, errors when <save> was last loaded, and errors when <save> was last played, named from Archo Monitor's phase marks. Errors whose phase was not recorded get their own card; a log without Monitor's marks keeps one card. The TUI shows the same three headings.
- Can it run: Before you start begins with your last save: the save the last run loaded, checked against the current list (loads with the list, content will be lost, will not load cleanly) with the rows that need attention, and Open in Saves to fix them there. The TUI shows it as a line under the heading with `archo saves show <ref>`.
- The plan page's problems button no longer pulses "updating…" for good. It pulsed whenever the files had not been checked yet, even with no check running, until Can it run was opened; that pulse kept the window redrawing at about 12% of a CPU core. It now pulses only while the list's problems are being refreshed or the files are being checked.
- The green dot on Launch while the game runs blinks three times and then holds. Its endless blink redrew the window for the whole game session (about 5% of a CPU core).
- Mod thumbnails and description images use less memory and no longer hold up the window:
  - Making a thumbnail no longer makes other actions wait. Before, every page call waited while a large `Preview.png` was decoded.
  - At most two pictures are decoded at once; the rest wait their turn.
  - Shrinking a picture no longer needs a second full-size copy: a 4K PNG peaks at about 34 MB instead of 67.
  - A description image over 16 MB is refused instead of cut off, and so is any picture over 40 megapixels, before it is decoded.
  - Description images are written to the cache whole, so a crash or a closed window during the write no longer leaves a broken image that shows up blank until the cache is cleared.
- Command panel (ctrl+k) search is fuzzier. Each word you type can start a different word of the name, in any order: `ad cu` finds Add Cube, `lib hug` finds HugsLib (words split at spaces, punctuation and capitals). Typos are forgiven in words of 4 letters or more (one typo, two from 8 letters; swapping two neighbouring letters counts as one): `sotr` finds sort, `vanilka expnded` finds Vanilla Expanded. Closer matches still come first: prefix, substring, word starts, letters in order, then typo matches. Applies to commands, Mods, games and lists, in the GUI and the TUI.
- Tags popup (a Mod's details → tags edit, or `g`): the Mod's own tags come first (with several Mods selected: tags on all of them, then on some), so they are no longer lost among the rest; the TUI tag bar uses the same order. In the GUI, typing narrows the tags shown to those containing the text, like the filter bar, and a "new tag …" line appears when the text is no tag yet; Enter on a name that exists in another case uses the existing tag instead of making a second one. With many tags the popup no longer runs off the bottom of the window: the tags scroll inside it.
- The error box `RimArcho.exe` shows when it cannot open its database is in the Windows display language. Before, it used the interface language saved in that database, so on an English Windows with RimArcho set to Chinese it put a Chinese sentence around an English error.
- Dependencies: a Mod that requires another is satisfied by the Mod standing in for it. A dependency's `alternativePackageIds` in About.xml count, as they do in the game, and so does a continued version that Use This Instead (community data) names as the original's replacement. Sort orders the requiring Mod after the stand-in and no longer offers to download the original; Can it run, the launch question and `archo problems` no longer report the dependency missing; the Mod's details show the stand-in "(in place of …)". Example from a player: a translation requiring a Mod made for 1.3/1.4, with a 1.7 continuation under another packageId enabled. Before, the only way to sort quietly was to enable the outdated original, which then failed its version check.
- Replacing a Mod with its recommended replacement (after Sort, or `mod.replaced` in Can it run) is offered also when another Mod requires the original; before, it said "X requires Y, so it stays".
- The first scan after updating reads each Mod's dependencies again, so `alternativePackageIds` take effect for Mods that did not change.
- Mod library: a Mods | Tags switch (`v`) above the table. The Tags view lists the Mods under a header row per tag (pinned tags first, untagged Mods under No tag at the end); a Mod with several tags appears under each. Headers open and close with ← →.
- Mod library: on a tag's header the details become a Tag manager, laid out like a Mod's details: rename the tag (F2), give it a note shown in its header (`n`), merge it into another tag or a new name (`M`), delete it (`Del`), or keep it on top of the tag list. Tags spelled almost the same are offered as a merge, and every tag is listed below as a chip that jumps to its group; ctrl+click several chips to merge or delete them together. Merge and delete ask first; tag changes cannot be undone with `u`.
- Mod library: collapse all and expand all buttons in the table heading (every Mod's copies, or in the Tags view every tag group). An expanded Mod's copies now line up with the Mod names instead of one step further in.
- CLI: `archo tag rename <tag> <new>`, `tag merge <tags…> --into <tag>`, `tag delete <tags…>` (each with `--dry-run`) and `tag note <tag> [<text>] [--clear]` change a tag on every Mod; `tag list` shows pinned tags and notes, and says `1 mod` instead of `1 mods`.
- MCP: `set_player_data` has the actions `tag_rename`, `tag_merge`, `tag_delete` and `tag_note`, with `dry_run`; `refs` is now optional.
- The database moves to schema 32 (tag notes) when a 1.0.x database is opened.
- Archo Monitor is recognized by packageId from every Mod source, including RimArcho's Workshop item. The highest Monitor modVersion wins; on a tie, the Workshop copy wins and the next save or launch removes the bundled copy's link. Plans keep one active Monitor, and Can it run, CLI and MCP no longer report installed Monitor copies as duplicate Mods. Workshop Monitor also works when `Mods/` is on FAT or exFAT. Monitor keeps its own version, independent of the launcher.
- Maintainers: `archo workshop publish` assembles and validates the RimArcho Workshop item, then creates or updates it through the signed-in Steam client without starting RimWorld. Supports a local-only dry run, stable version tag, item ID persistence, change note, visibility and upload progress. Release scripts never publish.
- RimArcho can check for stable updates on the Steam Workshop and use a subscribed release zip or anonymous SteamCMD, with GitHub fallback for the same version. RimArcho's Workshop item is https://steamcommunity.com/sharedfiles/filedetails/?id=3835688284. Settings has one update source choice: automatic (Workshop first) or GitHub; prereleases use GitHub. `archo update --check` names the source and any check fallback. When the Web API cannot see the item, a subscribed copy still gives the version. The Workshop source stays disabled until its item is configured.
- Steam Workshop distribution: started from the subscribed item folder, the GUI offers "install RimArcho" to a folder of its own, then restarts there and removes only the extracted release files (the item keeps its own files, LICENSE included). Declining keeps a session warning; self-update refuses installation in Workshop folders, and TUI/CLI startup warns on stderr.
- First-run setup asks where to keep managed downloads and tools, with a low-space warning. The `local.dir` setting chooses their root before it holds content; existing folder overrides remain effective.
- The guide and Settings can discover RimSort lists and user rules and RimCrow databases and saved lists. Imports show a preview before confirmation; RimCrow player data and RimSort rules preserve existing values and leave the source files unchanged.
- Probe runs: `archo test run --probe <folder>` and MCP `test_run` with `probe` add a probe Mod (a small test Mod that logs `[Probe]` lines and ends with one `[Probe] RESULT` or `[Probe] EXCEPTION` line) to the tested list, run a quick test map with `-probequit` and stop at that line instead of waiting for a quiet log. The probe is linked into `Mods/` for the run only and loads last, or next to a Mod with `--probe-after` / `--probe-before` (`probe_after`, `probe_before`). The report adds `probe`: the outcome, the RESULT text, the `[Probe]` lines, the screenshot the probe saved (moved into the run folder) and the probe's dependencies the list lacks; error clusters, attribution and both comparisons are as before, and a probe run compares only with earlier runs of the same probe. Success needs the RESULT line and no error clusters. Works for 1.7 too, with a probe built for .NET 10. The `rimworld-probe` skill's template keeps running when its window is in the background and ends at an exception in `OnStartup`; its `SKILL.md` describes the line contract.
- Updates page: Update all with many Mods opens its confirmation at once. Before, it built a preview for each Mod in turn, about 10 seconds for a long list. Confirming now queues one task for the Git and SteamCMD Mods, downloaded by SteamCMD in shared batches, and one for Steam client Mods, which Steam gets in one request. Before, every Mod was its own task and its own SteamCMD run. The TUI's update all (`U`) does the same.
- The first-run guide no longer shows `✗ no environment configured` (in English) on the status line before a game is added.
- Reference numbers (ctrl+\) are saved again: `refnums/<page>.txt` in the data folder and the numbers' memory, so a number stays with its control after a restart. Before, the numbers and the page's resize gaps kept redrawing each other every frame while the numbers were on, so nothing was ever saved.
- Mod details: the web page and Steam page links beside the Workshop ID stay on the Workshop ID's line when there is room; in a narrow pane they wrap together instead of the second one dropping alone to the next line.
- Can it run: after Sort, or after choosing which Mod of an incompatible pair to disable, the cards under errors from the last run no longer each turn into a ✓ row with its own undo.
- Add subscription: the preview reads in the interface language (it was English in Chinese).
- List editor: dragging a Mod that is not selected moves that Mod. Before, when other rows were selected, they moved instead of the one under the pointer.
- List editor: Space or Enter on a focused button (Delete, Rename and the others in the list's header) presses that button. Before, Space disabled the Mod under the cursor and Enter opened its details.
- Cancelling a folder or file picker no longer shows `✗ cancelled by user`.
- Copy buttons beside web addresses in text (the release notes in Settings › About) no longer copy the closing `)` of a Markdown link or a trailing full stop, which made the copied link a dead page.
- A failed move of a managed folder now says to set its new location with `archo folder set`, not `SetDirectory`.
- Game log: a misspelled class of a patch operation (`<Operation Class="PatchOperationConditionl">`, also with a namespace) now names the Mod whose patch has it, and the error is an XML load error, shown by default. Before, `Could not find type named … from node …` was filed as other (hidden) with no Mod identified.
- Game log: an error that throws while ticking every colonist ("Exception ticking Hubbard (at (121, 0, 135))", "Exception ticking world pawn Martin", a hediff's "for pawn …") is now one cluster with the count of all pawns, in `archo log`, MCP `analyze_log` and the Game log page. Before, each pawn's name made its own cluster, 20 for one bug. Things other than humanlike pawns keep their own cluster per kind.
- Game log page: after switching the interface language in place, the note beside the title ("its errors are in needs attention") also changes language; it also follows the log's state when the page redraws.
- Can it load: a Mod unsubscribed or deleted while it was still in the list now says what the save loses without it (`loading without it loses 5 colonists of race Ratkin …`). Before, it said nothing in the save needed it.
- Mod settings: an empty stored value (`<PinnedMods />`) is read only and shows as empty. It can be an empty list as well as empty text, and `archo mod-settings set` wrote text into XML Extensions' list, which then failed to load in the game.
- Background tasks in Chinese: the RimArcho update check's result "new version available" is translated.
- `archo problems`: when two copies of a Mod are installed and neither is in the list, the finding no longer suggests `archo swap`, which failed with "not enabled", and no longer says both copies are ignored because another copy wins.
- RimArcho's own update and uninstall now know `README.zh-CN.md` and `skills/` (agent skills, such as `rimworld-probe` for checking Mod behaviour in the game). Release zips carry them from 1.0.6 on. RimArcho 1.0.0 refuses a zip with files it does not know, so a 1.0.0 install should update to 1.0.5 before 1.0.6 is out; after that, download 1.0.6 by hand.
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

### Game test runs (20 §2)
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
