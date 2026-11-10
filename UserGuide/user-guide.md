# RimArcho user guide

English | [&#31616;&#20307;&#20013;&#25991;](user-guide.zh-CN.md)

This guide is for players. It walks through every part of RimArcho 1.0.9: what each page is for, what its controls do, and how to get common jobs done. For how things work inside, see the [specifications](../docs/specs/README.md).

## Contents

1. [What RimArcho is](#1-what-rimarcho-is)
2. [Install and first run](#2-install-and-first-run)
3. [Getting around](#3-getting-around)
4. [Games and Mod folders](#4-games-and-mod-folders)
5. [Lists](#5-lists)
6. [Sorting and rules](#6-sorting-and-rules)
7. [Can it run](#7-can-it-run)
8. [Launching the game](#8-launching-the-game)
9. [Game log and Archo Monitor](#9-game-log-and-archo-monitor)
10. [Mod library](#10-mod-library)
11. [Updates and subscriptions](#11-updates-and-subscriptions)
12. [Saves](#12-saves)
13. [Packs](#13-packs)
14. [Texture optimisation](#14-texture-optimisation)
15. [Search files and Mod settings](#15-search-files-and-mod-settings)
16. [Background tasks](#16-background-tasks)
17. [Settings](#17-settings)
18. [The terminal interface](#18-the-terminal-interface)
19. [The command line](#19-the-command-line)
20. [AI agents (MCP)](#20-ai-agents-mcp)
21. [Test runs](#21-test-runs)
22. [Your files, backups and uninstalling](#22-your-files-backups-and-uninstalling)
23. [Recipes](#23-recipes)
24. [Not in this version](#24-not-in-this-version)

---

## 1. What RimArcho is

RimArcho is a RimWorld launcher and Mod manager in one place. Pick your Mods, put them in order, check that the game will run, launch it, and find out why when something breaks, without switching between a manager, log files, Workshop pages and forums. It is built for players with hundreds of Mods.

Everything works without AI. AI assistants can connect too (section 20), and they get the same facts and follow the same rules as you.

One download gives you four ways to work, all built on the same code and showing the same facts:

| | Program | Use it for |
| --- | --- | --- |
| Desktop window | `RimArcho.exe` | Everyday use, with mouse and keyboard |
| Terminal interface | `archo` (or `archo tui`) | The same list editor in a terminal, keyboard only |
| Command line | `archo <command>` | Scripts and quick questions |
| MCP server | `archo mcp` | AI agents such as Claude Code or Codex |

### Words used in this guide

The interface uses everyday words; the documentation and command line sometimes use technical ones.

| Interface | Command line / docs | Meaning |
| --- | --- | --- |
| game | environment (`env`) | One RimWorld installation folder, with its launch and save settings |
| list | plan | A Mod list: which Mods are enabled and in what order |
| copy | instance | One physical Mod folder on disk |
| copies of the same mod | lineage | All copies and versions of one Mod (Workshop, local, locked…) |
| pack | pack | A frozen set of Mod files with its own saves |
| can it run | problems / findings | Everything RimArcho found that may stop the game or deserves a look |

---

## 2. Install and first run

### Install

1. Download `archo-<version>-windows-amd64.zip` and unzip it anywhere, for example `C:\Tools\RimArcho\`.
2. Double-click `RimArcho.exe`. The window needs Microsoft Edge WebView2, which Windows 10 and 11 already have.
3. Optional: add the folder to `PATH` so you can type `archo` in any terminal.
4. Optional, for reading Mods' C# code: install ilspycmd (it needs the .NET SDK). RimArcho never installs it for you and tells you when it is missing.
   ```
   dotnet tool install --global ilspycmd
   ```

The programs are not code-signed. The first time, Windows SmartScreen may say "Windows protected your PC": choose **More info**, then **Run anyway**.

**Started from a Steam Workshop folder?** If you start RimArcho from inside a subscribed Workshop item, it offers **install RimArcho** into a folder of its own (default `%LOCALAPPDATA%\Programs\RimArcho`; **change…** picks another). It shows the space needed and warns when the drive has less than 10 GB free. **install and restart** copies itself there and restarts; **use here this time** keeps running from the Workshop folder with a warning, and asks again next time. RimArcho will not update itself while it runs from a Workshop folder.

### The first-run guide

The first time the window opens, a guide sets you up.

**Managed folders.** First it asks: "Where should RimArcho keep SteamCMD, downloaded Mods and tools?" The default is `%LOCALAPPDATA%\RimArcho`. Pick a drive with room if you plan to download many Mods, then **use this folder** (or **not now**).

**Step 1 — find the game.** "Which RimWorld installs should RimArcho manage?"
- **a game you have**: tick the installs it found (you can add several), or **add this folder** to pick one yourself. Then **add and continue** (or **add N games**).
- **download a version**: if SteamCMD is missing, **download SteamCMD** first (it shows the size and destination). Choose a game branch, a **game folder**, optionally a **SteamCMD account**, then **download and add**. Passwords and Steam Guard codes are typed only into SteamCMD's own window; RimArcho never sees them.

**Step 2 — read the mods.** RimArcho reads every Mod folder once and shows progress per game. Later scans only read what changed. **scan again** retries after a failure; **next** continues.

**Step 3 — where to start.** "Where does your first list come from?"
- **the game's current list** → **use this list**: start from what the game has enabled now.
- **move over from RimSort or RimCrow** → **import…**: RimArcho looks in the usual places for their lists, rules and databases (without scanning whole drives), then shows what will be imported, what already exists, what is skipped and what conflicts. Existing values stay; importing player data cannot be undone, so the import button is the confirmation.
- **open a pack or a Steam collection** → **choose a pack…**, or paste a collection link and **open**.
- **skip**: finish without importing.

Nothing in the guide writes to the game.

### First run from the command line

```
archo env detect                 # find the game, the user data folder and the Workshop folder
archo env add main --detected    # register them as game "main"
archo scan                       # read every Mod folder (incremental afterwards)
archo status                     # overview: Mods, the list, problem counts
```

---

## 3. Getting around

### The sidebar

| Group | Page | Key | For |
| --- | --- | --- | --- |
| playing | **start** | `shift+1` | Choose a game and a list or pack, then launch |
| | **can it run** | `shift+2` | What may stop the game, and the errors from your last game |
| | **game log** | `shift+3` | Player.log, with errors grouped and traced to Mods |
| library | **lists** | `shift+4` | Edit the current list. Below it: **all lists** and your recent lists |
| | **packs** | `shift+5` | Frozen Mod sets with their own saves |
| | **saves** | — | Saves grouped by colony, load checks, playthrough history |
| | **mod library** | `shift+6` | Every installed Mod. Below it: **all mods**, **updates**, **add a subscription**, **search files**, **mod settings** |
| | **texture optimisation** | `shift+7` | Convert textures so the game loads faster |
| | **game** | `shift+8` | Your RimWorld installations and their settings |
| footer | **background tasks** | `ctrl+j` | Downloads, conversions and other long jobs |
| footer | **settings** | — | Program settings, accounts, folders, uninstall |

The **can it run** badge counts the things you can act on. Under **lists**, a green marker shows the list currently written to the game, and amber shows a list with changes made outside RimArcho waiting for you.

`ctrl+b` switches between the side bar and a top bar.

### Keys that work everywhere

| Key | Does |
| --- | --- |
| `alt+←` / `alt+→` (or mouse side buttons) | Back / forward |
| `esc` | Close the top layer, leave details, clear the selection or the filters |
| `tab` / `shift+tab` | Next / previous control |
| `1`–`9` | Jump to a numbered area of the page |
| `?` | Keys for this page |
| Menu key / `shift+F10` / right-click | What you can do with the row under the cursor |
| `ctrl+k` or `:` | The command panel |

Two rules hold across pages: **arrow keys only move** (the cursor, focus, scrolling) and do not change your list; **letter keys act at once** on the selection, and `u` (or `ctrl+z`) undoes them.

The window works with screen readers (and with AI agents that operate it through UI Automation): pages, controls, rows and states are named, list rows read as "n of total" with their selection, and progress and errors are announced. Dialogs keep the keyboard inside them.

Paths, IDs and web addresses that are shortened on screen have a copy icon that copies the whole value. Text in descriptions, logs and explanations can be selected and copied. The gaps between cards can be dragged to resize them; double-click a gap to reset it.

### The command panel (`ctrl+k`)

One box finds commands, Mods, lists and games, and also runs `archo` commands.

- Search forgives a lot: each word can start a different word of the name, in any order (`lib hug` finds HugsLib), and small typos are forgiven (`sotr` finds sort, `vanilka expnded` finds Vanilla Expanded).
- `ctrl+n` / `ctrl+p` move between results; `enter` runs the selected one.
- Type a command such as `problems` or `sort --dry-run`: `tab` completes commands, flags and Mod names, `↑`/`↓` recall history, `ctrl+enter` runs the text as typed, `ctrl+u` clears it.
- **preview the sort**, **preview saving** and **preview the launch** show what would happen without doing it.
- `esc` closes the panel and keeps your draft and the last output.

### Reference numbers (`ctrl+\`)

Pressing `ctrl+\` draws a small red number (like `lib12`) on every control. When you report a bug or ask for help, say "lib12 does X" instead of describing where the button is. The numbers stay the same after restarts; `ctrl+shift+\` renumbers the page.

---

## 4. Games and Mod folders

### The game page

**game** (`shift+8`) lists every RimWorld installation RimArcho manages, with its version and folder. **add a game** registers another; **use this one** switches to it; **remove the game** forgets it in RimArcho (game files, Mods, saves and lists stay).

Each game has its own settings:

- **game folder** and **game version** (read from the game's `Version.txt`).
- **start the game through**: Steam, or the game's exe directly. Games inside a Steam library default to Steam.
- **saves**: share the normal save folder, or give this game **own saves**, so a second install (say 1.6 and 1.7 side by side) never touches the other's saves and settings.
- **extra arguments** passed to the game.

You can register several installs, for example one for 1.6 and one for 1.7. Lists are shared between games; each game remembers which list it uses. A list can say which game versions it is meant for and warns when used with another.

You can download another game version with SteamCMD from the first-run guide or with `archo env download` (`archo env branches` lists the real public branches). Downloading the game needs an account that owns it.

### Where Mods come from

For the current game RimArcho reads:

- the game's `Data/` (Core and DLC) and its local `Mods/` folder;
- the Steam Workshop folder;
- SteamCMD download folders;
- extra folders you add (for example a personal Mod collection on another drive);
- RimArcho's own lock library (section 10) and the bundled Archo Monitor;
- another registered game's `Mods/` folder.

All of them are listed in **settings › folders**, each with its real path. **Locate** reconnects a folder you moved yourself; **move to…** moves it for you with a preview. Tags, lists and Mod identities stay attached. A destination that is already in use is never overwritten.

### Links

The game only sees Mods in its own folders. When you enable a Mod from a SteamCMD folder, an extra folder, the lock library or another game, RimArcho places a link to it in the game's `Mods/` folder when it saves the list. Links are named `_Archo_…` (`_Archo_env_…` for another game's local Mods). A link is not a copy: it takes no space, and needs no administrator rights. RimArcho removes the link when you disable that Mod, and it only ever creates or removes links with its own names. Drives formatted as FAT or exFAT cannot hold these links; RimArcho tells you instead of copying.

Switching games prepares any links the new game's list needs, then scans it. Switching never writes the game's Mod list by itself.

### Copies and changes

RimArcho tracks every Mod folder as a **copy** and groups the copies of one Mod together. A list points to a particular copy, not just to a package ID, so one list can use the Workshop version while another uses a locked older version.

Between scans RimArcho records what changed: a Mod appeared, was updated, moved, copied, or disappeared. A copy that disappears stays in your lists as "missing" and reconnects if it comes back. When it cannot tell whether two folders are the same Mod, it asks you under **can it run › for you to confirm** (**same**, **copy**, **unrelated**; **why this question** shows the evidence).

---

## 5. Lists

A list is which Mods are enabled and in what order. You can have as many as you like and switch freely. Editing a list changes RimArcho's working copy only; nothing reaches the game until you save or launch.

### The list editor

Open **lists** (`shift+4`). The editor has three areas plus details:

- **Enabled** (`2`): the load order, top to bottom. A dot marks entries that differ from the last save.
- **Disabled** (`1`): installed Mods not in the list.
- **Staging** (`3`): a holding area, below Disabled. Mods put here remember their position.
- Details (`4`): the selected Mod, filters, load-order rules and history, on the left.

**Opening a list is not the same as making it current.** You can look at and edit another list without changing what the next launch uses. A list becomes current when you save it, press `L` in its editor, choose it on **start**, or choose **make current**.

| Action | Key | Mouse |
| --- | --- | --- |
| Enable / disable / put back | `space` | Double-click, or drag between areas |
| Enable at… / move to… a position or after a Mod | `m` | Drag within Enabled |
| Move down / up one row (ten rows) | `J` / `K` (`alt+J` / `alt+K`) | |
| Stage, or put back from Staging | `t` | Drag to Staging |
| Stage everything enabled / put everything back | `T` | |
| Switch to another copy of the Mod | `x` | |
| Tags / note | `g` / `n` | |
| Sort | `S` | **sort** |
| Save to the game | `w` | **save changes** |
| Make current and go to **start**, ready to launch (`L` again launches) | `L` | **launch** |
| Undo / redo | `u` or `ctrl+z` / `ctrl+y` or `ctrl+r` | **undo** |
| History | `H` | |
| Add a load-order rule | `r` | |
| Check the files again | `A` | |
| Rescan | `R` | **rescan** |
| Import / export | `i` / `e` | **import/export** |
| Pick a list / go to the game page | `P` / `E` | |
| This Mod's changes | `C` | |
| Full details | `enter` | |
| Folder / Workshop page / Steam page | `ctrl+o` / `ctrl+w` / `ctrl+s` | |

Select a range with `shift+↑/↓` or shift-click, one more row with `ctrl+space` or ctrl-click, everything shown with `ctrl+a`. Dragging a selected row moves the whole selection; dragging an unselected row moves just that one. `esc` cancels a drag.

When you enable a Mod, RimArcho places it where the known load-order constraints want it.

### Search and filters

`/` or `ctrl+f` searches; `\` opens the filter bar. Click a filter to include it, right-click (or `x`) to exclude it. Filter groups include lists, tags, sources, **Location** (which folder a copy lives in) and content. The search box understands:

- plain words (name, author, package ID), quoted phrases and `-word` to exclude;
- `tag:`, `author:`, `type:`, `has:` (for example `has:problem`), `location:"D:\Mods"`.

Filters apply to Enabled and Disabled together; Staging has its own search.

### Staging: set Mods aside without losing their place

Staging is for troubleshooting and for preparing. Select a batch of suspects, press `t`, test the smaller list, then focus Staging and press `T`: every Mod returns to its remembered position. Staging lives only in RimArcho and is never written to the game.

### Undo and history

Every edit enters the list's undo history (up to 100 steps each way), and edits survive closing RimArcho. **History** (`H`) also keeps saved versions: each save, launch, import, and each change made outside RimArcho (in the game's Mod menu or another manager). Pick a version to see what differs and **go back to this version**; that too can be undone.

Tags, notes and identity answers are not list edits and are not undone with `u`.

### Saving to the game

**save changes** (`w`) writes the list to `Config/ModsConfig.xml` in the game's user data folder (the shared one, or the game's own if it has **own saves**):

1. It prepares the links the list needs.
2. It keeps the previous file next to it as `ModsConfig.xml.rimarcho.bak`.
3. It writes a temporary file and swaps it in, so a crash never leaves a half-written file.

Before writing, RimArcho checks whether the game's list was changed elsewhere. If so, it keeps that change as a version in history and asks before overwriting it.

From the command line, `archo save --dry-run` shows what would be written.

### All lists: create, copy, import, export

**all lists** shows every list with its Mod count, dates, where it came from, which games and versions it suits, and an estimate of the memory it needs (**what is counted** breaks it down by category and largest Mods).

| Action | Key |
| --- | --- |
| **new / import** | `n` (`i` opens import directly) |
| **edit** | `enter` |
| **rename** | `F2` |
| **duplicate** | `D` |
| **delete** (asks first; the last list cannot be deleted) | `Delete` |
| **make current** (no game file is written) | |
| **game versions** the list is meant for; **reset to default** | |
| **export…** | `e` |
| **create a pack…** | `B` |

**Creating a list.** **start from**: **an empty list**, **copy a list** (its current editing content, unsaved changes included, or a past version), **from a save** (the Mods that save used), or **import**.

**Importing.** Choose a file or paste text, then **preview**. The preview shows which Mods are installed, which are missing (with Workshop links), version differences, and suggested replacements. **subscribe all N** subscribes to the missing ones with Steam or SteamCMD. Then **create and edit**. Inside the editor, import can also **replace the whole list** or **add the ones you lack**; either is one undoable edit. RimArcho reads:

- RimArcho lists (`.archolist.json`) and a pack's `pack.json`;
- the game's `ModsConfig.xml`, in-game `.rml` lists, and the Mod list stored in a save;
- RimSort XML, JSON and clipboard lists;
- RimCrow XML, JSON and share codes (including old `RMM1-` codes);
- plain package-ID text, Workshop links or IDs, Rentry pages, and Steam collections.

Importing never writes to the game.

**Exporting.** RimArcho list, RimCrow share code, plain text, `ModsConfig.xml`, `.rml`, RimCrow `ModList.xml`, or Workshop links. **save as a file…** (`ctrl+s`) or **copy** (`ctrl+c`). A RimArcho list records a fingerprint of each Mod's version, so the person you send it to can tell whether they have the same version. Export never overwrites an existing file.

### Following a Steam collection

A list can **follow** a Steam collection. When the collection changes, **see the changes** shows what was added and dropped, and **catch up** applies them as one undoable edit: **add new only**, or **also disable dropped** (dropped Mods are disabled, not unsubscribed or deleted). Following a collection does not subscribe you to every Mod in it; new Mods you lack come with Workshop links. Checks happen when **all lists** opens and a check is due, or when you ask.

### Moving from RimSort or RimCrow

Use **settings › storage › import…** (or the first-run guide). RimArcho finds RimSort lists and user rules, and RimCrow databases and saved lists, then previews them. From RimCrow it also brings aliases, notes, groups (as tags) and supported rules. Existing values are kept and conflicts are reported; the source files are left untouched.

---

## 6. Sorting and rules

**sort** (`S`) orders the Enabled list. It uses:

- the official order of Core and DLC;
- your own rules, which are always strong and win ties;
- what each Mod's `About.xml` declares: dependencies, load before / after, incompatibilities, and supported versions;
- community rules, which are normally weak.

Not every rule weighs the same. Whether breaking a declared rule actually caused errors was measured in tens of thousands of player-shared logs: dependencies and rules whose breaking really causes errors are strong; ones that showed no harm (such as many "load before Core" requests) are only preferences.

It keeps your existing order wherever the constraints allow, so a sort moves as little as possible. It adds Core if missing and, by default, Archo Monitor. A sort is one undoable edit and does not write to the game. `archo sort --dry-run` shows the result first.

After sorting, RimArcho tells you what it could not solve: missing dependencies (with the offer to enable or download them), entries it cannot find, recommended replacements, and rules that contradict each other. A contradiction (a cycle) never blocks saving or launching: RimArcho explains which rules could not all hold and picks an order.

**Stand-ins count.** A dependency is satisfied by:
- an alternative the requiring Mod lists in its `alternativePackageIds`, as the game does;
- a continued version that the community's "Use This Instead" data names as the replacement.

So a translation made for an old Mod works with the continuation you actually use, without enabling the outdated original. The Mod's details show the stand-in as "(in place of …)". The game's own Mod menu may still warn about the original; the game loads fine.

### Your load-order rules

Under **load-order rules** in the editor (or `r` on a Mod), add rules such as:

- load A before / after B;
- load A immediately after B (for a patch or translation);
- A and B are incompatible;
- keep A near the start / near the end;
- ignore one declaration or community rule you disagree with.

Rules belong to the Mod, not to a list, so they apply to every list and game. Switch between **current list** and **all rules** to review them. Editing a rule does not re-sort by itself. Rule deletion has its own undo.

**Community data** can be refreshed in **settings › community data** (**update now**) or with `archo data update`.

---

## 7. Can it run

**can it run** (`shift+2`) answers "will this list work?" It has two columns:

- **Before you play**: problems found in the list and files, grouped as **breaks the game**, **worth knowing** and **for you to confirm**.
- **When you last played**: errors from your last game, traced to Mods (section 9). While the game runs, the current game's errors appear separately.

Every item says where its evidence comes from: a Mod's own declaration, RimArcho's inference, community data, the game log, or your rules. Certainty is shown as certain, likely or possible.

### What it checks

- **Dependencies**: a required Mod or DLC missing or disabled.
- **Incompatibilities**: two enabled Mods that say they cannot run together, plus combinations that often fail in shared logs.
- **Load order**: rules broken by the current order, and Core/DLC out of order.
- **Versions**: Mods that do not declare support for this game version. (Missing a declaration alone does not prove a Mod fails.)
- **Duplicates and copies**: the same Mod enabled twice, a copy the game cannot load because another hides it, hidden Mods and leftovers of unsubscribed Workshop items, locked copies whose Workshop version changed.
- **Content**: RimArcho merges every Mod's Defs the way the game does and simulates the XML patches in order. A patch that would fail comes with its file, line, and a reason:

  | Reason | What it means |
  | --- | --- |
  | `target-inactive` | The patch targets a Mod that is installed but disabled |
  | `target-missing` | No installed Mod defines the target: a missing dependency or an outdated patch |
  | `target-later` | A later patch creates the target |
  | `node-absent` | The Def exists, but the part being patched does not |
  | `wrong-version-route` | The target exists only in folders this game version does not load |
  | `stale-name` | The target Mod is enabled but renamed that thing; similar names are suggested |
  | `unguarded-optional` | A patch for an optional Mod has no check that the Mod is there |

  It also finds Defs whose parent is missing (and which disabled Mod provides it), and Defs two Mods both define.
- **Code**: RimArcho reads Harmony patches straight from the Mods' DLLs, without running the game. It points out methods patched by several Mods, prefixes that can skip the original method, several transpilers on one method, and same-named DLLs that hide each other. These become warnings when the method shows up in a real error.
- **Links and folders**: broken or missing links, folders that moved.
- **Memory**: whether your RAM and video memory look enough for this list.

It follows version folders and `LoadFolders.xml` the way the game does, so it only reports on files the game will actually load.

### Fixing

- `F` fixes what it can in one step: enables installed dependencies and sorts. One undo reverts it.
- Each item has its own buttons: enable the dependency, subscribe to it, disable one of two incompatible Mods, sort, repair links, accept an outside change, or open the explanation.
- `A` checks the files again; `l` opens the game log; `v` on the memory item refreshes the estimate.
- Version warnings can be muted for one Mod, a group or all; a mute holds for that game version across lists.

Fixed items stay as greyed rows with an **undo** until you leave the page. One fix can clear several items.

From the command line: `archo problems`, `archo problems show p:<id>`, `archo problems ignore p:<id>`.

---

## 8. Launching the game

**start** (`shift+1`) has three columns: **game**, **list or pack**, and **launch**. Click a game to switch to it, click a list or pack to choose it (choosing does not write any game file). Lists come first, most recently played first.

| Control | Key |
| --- | --- |
| **launch the game** | `L` (or `enter` on the button) |
| **save N changes** (save without launching, then start the game through Steam yourself) | `w` |
| Focus a column / choose a list / choose a game | `1`–`3` / `↑↓` / `←→` |
| **last game**: the log of your last game, or can it run if it had errors | |

Under the launch button, a line names the most likely trouble with this list; click it to open **can it run**, or **hide** it until something changes.

When you launch, RimArcho saves the list, prepares links, and starts the game the way the game page says (Steam or the exe). It then follows the game's log live. While the game runs, the launch area says "the game is running" with an error count, and actions that could disturb the game (updates, renaming saves, moving folders) wait or refuse.

**Memory check.** If the list probably needs more RAM than is free, the button says **launch with limited RAM** or **launch with RAM risk**. A serious shortfall opens **RAM may be insufficient**, showing **loading RAM needed** and **RAM available now**. **cancel** is focused; **launch anyway** is your call. The estimate is a guide, not a crash prediction.

**A folder in the way.** If an ordinary folder in `Mods/` would hide the copy your list wants, RimArcho can offer **move and go on**: it moves that folder into the lock library (your lists that used it keep using it) and links the right copy. The preview names every move.

**Mod settings follow the copy.** When a Mod's folder name changes (say from the Workshop to a link), the game would look for a differently named settings file. RimArcho copies your existing settings to the new name, never overwriting anything.

---

## 9. Game log and Archo Monitor

### The game log page

**game log** (`shift+3`) shows Player.log, Player-prev.log, and Archo Monitor's companion logs. Repeated errors are grouped into clusters with a count (for example `E1 ×500`), so one bug does not fill the screen. Changing details such as numbers and colonist names are ignored when grouping: "Exception ticking Hubbard" and "Exception ticking Martin" are one cluster.

Select a cluster to see the raw lines, the Mods involved and, for known errors, **Archo log analysis**: how serious it is, what it usually means, and what usually fixes it.

| Key | Does |
| --- | --- |
| `↑↓` | Choose an error |
| `1` / `2` / `3` | Errors / log text / analysis |
| `←→` | Switch between Player.log and the companion log |
| `enter` | Expand companion entries |

**add its errors to needs attention** sends an older log's errors to **can it run**. **add Archo Monitor** adds the helper Mod to the list (undoable).

### How errors are traced to Mods

Each Mod involved in an error gets a role, with the evidence:

| Role | Meaning |
| --- | --- |
| **source** | Its files, its code in the stack, or its failing patch produced the error |
| **patcher** | It patches a method in the stack. That alone does not make it guilty |
| **dependency** | A disabled Mod that would provide the missing thing |
| **mentioned** | Its name appears, but nothing stronger points at it |

A stack full of Harmony patches is not a list of culprits; RimArcho keeps patchers apart unless their own code is the source. Evidence includes the XML or DLL file and line, the failing patch (matched to RimArcho's own simulation), who defines a missing Def, texture or class, and the first Mod frame in a stack. A class the game cannot find is traced to the Mod whose XML names it, with up to three similar existing classes ("did you mean…").

When one XML file fails to load, the many "could not resolve reference" errors that follow are folded under that first failure, so you start from the cause.

### Which game a log belongs to

RimArcho knows which launch wrote each log. A log from another list or pack, from a list you have changed since, or from another game version is marked stale: still readable, folded away, and not counted in **can it run**. A game started from Steam directly is still matched to the list RimArcho last saved.

### Live tracking

While the game runs, new errors and growing counts appear about once a second, without popping anything up. After the game exits, the whole log is analysed again.

### Logs from other places

From the command line or an AI agent you can analyse any log or a pasted stack trace, including logs uploaded by HugsLib (the upload's header is skipped, and 1.7 logs are recognised):

```
archo log                         # the newest Player.log
archo log --prev                  # the previous one
archo log --file "C:\Logs\Player.log"
archo log --text "<pasted error>"
archo log --cluster E1            # one cluster in full
archo log --follow                # live, while the game runs
```

### Archo Monitor

Archo Monitor (`toumanlin.archomonitor`) is a small helper Mod that comes with RimArcho. Inside the game it records what Player.log leaves out:

- which XML file and Def was loading when an error happened;
- which phase the game was in: starting up, main menu, loading a save, creating a colony, playing;
- how often each error repeats, even after the game has reached its message limit and stopped logging;
- what Mod settings screens contain, so RimArcho can show labels and defaults for settings outside the game.

It writes `ArchoMonitor.log` and `ArchoMonitor-prev.log` next to the game's own logs and uploads nothing. These files can contain paths and save names, so look before sharing them. RimArcho adds it to new lists right after Harmony (switch this off in **settings › game log › add Archo Monitor to lists**) and keeps it up to date. Logs from RimCrow's companion (`RimCrow_Realtime.log`) are read too.

### "My game broke, what now?"

1. Before restarting again and again, open **can it run** or **game log**: the game keeps only two logs.
2. Check that the log belongs to the list you played.
3. Start with the first source error, or the root of a folded group.
4. Read the roles and evidence. Do not remove every patcher named in a stack.
5. Make one small change (enable the provider, disable the source, update the Mod, stage a batch), launch again, and compare the clusters.
6. If the problem is loading a save, also read that save's **can it load?** (section 12).

---

## 10. Mod library

**mod library › all mods** (`shift+6`) shows every installed Mod across all games, with its copies grouped under it. Filters and details sit on the left; the table is on the right.

Columns: Name, Size, State, Source and Used in; add Updated, Version, Author, Tags and Last played from the header menu (`s`; `<`, `>`, `=` change width). Click a header to sort; drag to reorder or resize. **collapse all** / **expand all** fold the copies.

The library does not change lists: it is for looking after the files. File actions, each with a preview and a warning when lists use the copy:

- **lock this version**, **update to the Workshop version**, **unlock**, **move into the lock library**;
- **unhide**, **resubscribe**, **unsubscribe**, **move to the recycle bin**;
- **check for updates**, **update subscription**, **remove subscription**, **open download page**;
- **remove the converted textures**;
- **do not list as a copy**.

Keys: `enter` details, `C` changes, `g` tags, `n` note, `R` rescan, `ctrl+o` / `ctrl+w` / `ctrl+s` folder / Workshop / Steam page, `Delete` the fitting remove action.

**Cleaning up.** Filter to copies used by no list, expand a Mod to compare its copies, select what you want gone, read the preview, confirm. Removed folders go to the Windows recycle bin; lists that used them show the entry as missing. If the recycle bin is not available, RimArcho refuses rather than deleting for good.

### Locking a version

**lock this version** keeps a Mod exactly as it is now, whatever the Workshop does later. RimArcho copies it into its lock library and switches the current list to that copy (undoable); other lists keep the Workshop version. When the Workshop version changes, RimArcho tells you and lets you compare before you **update to the Workshop version** (a new locked copy; the old one, with any edits you made, is kept). **unlock** switches the list back to the Workshop copy without deleting the locked one.

### Hidden Mods

Some managers hide a Mod by renaming its `About.xml` to `About.xml.disabled`. RimArcho tells apart a Workshop original hidden behind a locked copy, the leftover of an unsubscribed item, and a Mod hidden on purpose, and offers the fitting action: compare, **unlock**, **unhide**, **resubscribe**, keep hidden, or recycle.

### Tags and the Tag manager

Press `g` on one or several Mods to tag them. Tags on all selected Mods show ✓, on some show ~. Typing narrows the tags or offers "new tag …"; `enter` applies or removes. The Mod's own tags come first.

The **mods | tags** switch (`v`) groups the library by tag (pinned tags first, **no tag** last). On a tag's header, the details become the Tag manager:

- **rename** (`F2`), **note** (`n`, shown in the header), **merge into…** (`M`), **delete tag** (`Delete`);
- **filter to this tag**; **keep on top** / **stop keeping on top**;
- **similar** offers to merge tags spelled almost the same;
- under **all tags**, ctrl-click several to merge or delete them together.

Merge and delete ask first and cannot be undone with `u`. Tags, notes and aliases belong to the Mod, so they follow all its copies across lists and games. They never touch game files.

### Mod details

`enter` on a Mod (in the editor or the library) opens its full details with six areas: **Mod**, **Copies**, **Used in**, **Fields**, **History**, **Technical** (`1`–`6`). You see the preview image, author, tags, description, problems, every copy with its notes and location, which lists and packs use it, dependencies (including stand-ins), IDs, and the Mod's change history. **Technical** lists its Defs, patches, Harmony patches and rules.

`←`/`→` go to the previous or next Mod, `[`/`]` to another copy, `esc` returns. Looking at another copy does not change your list; **use this one in the current list** does (same position, undoable). The Workshop ID has **web page** and **Steam page** links.

---

## 11. Updates and subscriptions

### The updates page

**mod library › updates** sorts Mods into **waiting for you to update**, **won't update any more**, and **already updated**. Each change is labelled by what it means for you: **Affects launch**, **Worth a look**, or **Content only**. `a` switches between the current list and every Mod.

| Action | Key |
| --- | --- |
| **check for updates** | `K` |
| **update all** | `U` |
| update this one | `u` |
| **add a subscription** | `+` |
| Lock first (keep the current version) | `p` |
| The changed files | `i` |
| Dependencies / sort | `D` / `S` |
| Source page / folder | `ctrl+w` / `ctrl+o` |
| **show in the mod library** | `l` |

A safe update: read the impact, lock the current version first if you are mid-playthrough, update, read the preview, then check the list again.

Automatic checks only look; they never install Mod updates. Updates wait while the game runs. **update all** queues Git and SteamCMD Mods as one batch and Steam Mods as one request to Steam. A Workshop item that was removed or made private keeps working locally, but RimArcho warns that unsubscribing may lose it for good.

### Adding a subscription

**add a subscription** accepts:

- **Workshop** IDs or links: **subscribe with Steam** (through the running Steam client, so the author gets credit) or **download with SteamCMD** (no Steam client needed; does not subscribe your account);
- **Git** repository addresses (GitHub and others; no Git install needed): follow **the default branch**, **a branch**, **a tag**, or pin **one commit**. Local edits are protected: an update refuses a folder you changed;
- **download pages**, including LoversLab: RimArcho tracks the page and lets you pick the download (ZIP or folder; extract RAR and 7z first). With a LoversLab login it can list the real files.

Read the preview, then **subscribe** (`enter`). Pasting a supported address on the updates page opens this with it filled in.

**From SteamCMD to Steam later.** After you have played a SteamCMD batch three times, RimArcho can offer to subscribe to those Mods through Steam and replace the downloaded copies, crediting the authors. Lists are switched over; dismissing the offer is permanent for that batch.

### Accounts

**settings › accounts**:
- **SteamCMD**: **log in** / **log in again** / **log out**. You type your password and Steam Guard code only in SteamCMD's own window; RimArcho stores just the account name.
- **LoversLab** (optional): logs in through a separate window; the session is stored encrypted for your Windows user and never goes into exports, packs, diagnostics or backups.

Managing local games and Mods never needs a Steam account.

---

## 12. Saves

**saves** shows every save of every game and pack, grouped by colony, in three cards: colonies and saves, the selected save, and its playthrough.

The playthrough card shows the day and date, storyteller, difficulty, play time, wealth, colonists and deaths, recent events, and a timeline of wealth and colonists with a point per save.

| Action | Key |
| --- | --- |
| Search | `/` |
| Choose a save / fold a colony | `↑↓` / `←→` |
| **rename** (keeps a RimSaves prefix) | `F2` |
| **copy**, **back up**, **recycle…**, **open location** | |
| **make a list** from the save's Mods | |
| The quick action shown (launch, make a list…) | `enter` |

### Can it load?

For each save, **can it load?** compares the Mods it was saved with against the list it belongs to (its pack, the list that last launched it, or a list with exactly its Mods). The verdict is one of:

- **loads with the list**
- **loads, but content will be lost**
- **will not load cleanly**
- **being written** (the game is still saving)

It names the consequences, for example "loading without it loses 3 colonists of race Kurin_Race, including 1 dead; 1 faction; 3 things". Missing Mods that own colonists, factions or classes are flagged more strongly than a plain difference. It also catches damaged or cut-off files (and points to the last readable save), game-version changes, added Mods and changed order.

Fixes enable the missing Mods and subscribe to absent ones in the save's order as one undoable edit; optional fixes disable Mods added since, or **Use the save's order**. After Steam finishes downloading, scan and check again. Rename and recycle wait while the game runs.

From the command line: `archo saves list`, `archo saves show <save> --play`.

---

## 13. Packs

A pack is a frozen copy of a list: the exact Mod files, in order, with its own saves and settings. Workshop updates never change it. Use it to keep a long playthrough stable, or to hand a working modpack to a friend.

**Creating.** From a list, **create a pack…** (`B`). RimArcho checks for missing Mods and blocking problems, sorts a copy of the list (your list stays as it is), and copies only what the chosen game version loads (other version folders are left out). Core, DLC and the game are never included. Before writing, **see the list first, write nothing** (`V`) shows the contents, space needed and warnings.

Textures can be pre-converted at full size, or slimmed to 80, 75, 60, 50, 40 or 25 % for a smaller pack (`D` changes the choice; **count the textures** measures it). Then **create pack** (`B`). An interrupted copy offers **create again** or **delete what it copied** (your original Mods are never touched).

**Playing.** **play** (`L`) on the pack's page, or choose it on **start**. The first launch copies your existing Mod settings into the pack without replacing its own. A pack needs the same game major version (1.6, 1.7…) and its DLC; a different build of the same version only warns.

**Sharing.** **export** (`e`) writes an `.archopack` with the Mod files (no game, DLC, saves or settings). **export the list** (`E`) writes a small list file without Mod files. Importing checks every file against the pack's fingerprints.

Pack links (`!ArchoPack_…`) stay after the game closes so the pack starts quickly next time; saving or launching a normal list removes them. If you then start the game from Steam, RimArcho warns and offers **remove the links**.

**move to the recycle bin** (`Delete`) removes a pack.

---

## 14. Texture optimisation

**texture optimisation** (`shift+7`) converts Mods' PNG textures to DDS, which the game loads faster and with less memory. It uses the free tool todds (**download todds…** if missing).

1. Choose the current list or every installed Mod, and a scale.
2. Optionally **count the textures** to see PNG and DDS sizes and the expected savings.
3. **convert**. Already converted textures are skipped.

Original PNGs stay where they are; RimArcho writes the DDS next to them and remembers which files it made. **remove every converted texture…** deletes only those, so the game uses the PNGs again. Outdated conversions (the Mod's PNG changed since) are shown.

---

## 15. Search files and Mod settings

### Search files

**mod library › search files** searches inside Mods: Defs, patches and `About.xml` of the current list by default, and C# code too if you add it (decompiled on demand with ilspycmd). Results form a tree of Mod → file → matching line beside a file viewer. Results stream in as they are found.

`enter` searches; `alt+c` case-sensitive; `alt+r` regular expression; `n`/`N` next/previous match; `alt+z` wrap lines; `ctrl+o` open folder; `ctrl+c` copy path; `esc` stop. Searching never changes Mod files.

### Mod settings

**mod library › mod settings** lists the settings files of the current list's Mods, including HugsLib settings. Select one to see its stored values and, when Archo Monitor has seen the in-game settings screen, the labels and defaults.

Switches change directly; value fields save with `enter` and cancel with `esc`; **reset** removes a stored value. Each change writes the file at once, keeps a `.rimarcho.bak` backup, and refuses if the game is running or the file changed meanwhile. Complex or ambiguous values stay read-only. **open the Config folder** (`ctrl+o`).

---

## 16. Background tasks

**background tasks** (`ctrl+j`, press again to go back) lists running, queued, stopped and finished jobs: downloads, updates, conversions, decompiling. Group them by Running, Queued, Today, Yesterday and Earlier; select one for its progress, steps and per-item results.

| Action | Key |
| --- | --- |
| **stop the rest** / **cancel the wait** | `x` |
| **retry N items** / **try again** / **do the N left** | `r` |
| Run **later** / **sooner** | `J` / `K` |
| **go to …** what it worked on | `o` |
| **open the folder** | `ctrl+o` |
| **take off the record** / **take the done ones off the record** | `Delete` / `C` |
| Bring removed records back | `ctrl+z` |

One failed item does not stop a batch, stopping keeps what is done, and retry skips items that can never succeed. Records survive restarts. RimArcho runs nothing in the background once you close it.

---

## 17. Settings

| Section | Settings |
| --- | --- |
| **about** | Version and **release notes**; **check for updates** / **install update**; **automatically check for updates** (RimArcho, Mods, tools, community data; checks only, never installs); **bugs and suggestions…**; Discord and GitHub |
| **interface** | Language (follow system, English, `简体中文`; changes in place); **day/night theme**; **accent colour**; **Archo animation** (off, still or animated; Archo keeps you company on the start-up screen, in loading cards and in the first-run guide); Workshop descriptions and their images on or off; GPU acceleration (off uses about 20–35 MB less memory; next start); the **start** page background: game art, background Mods or your own images, with dimming, blur, focus point, stars and rotation |
| **accounts** | SteamCMD and LoversLab sign-in (section 11) |
| **updates** | **update source**: automatic (Steam Workshop first, then GitHub) or GitHub; **check interval** |
| **folders** | Every game, Workshop, SteamCMD and extra Mod folder, the lock library, source library, packs, Monitor and downloads: **locate**, **move to…**, add or remove sources (removing keeps the files); clean up or pre-decompile the source library |
| **game log** | **add Archo Monitor to lists**; the installed Monitor version |
| **network** | Proxy (system, off, or an address); GitHub mirror |
| **external tools** | todds and SteamCMD: status, path, choose another, install, update, remove |
| **community data** | The three datasets with source, counts and date; **update now** |
| **storage** | **back up** the database; **compact**; **remove old tombstones** (forget long-gone Mods); cache sizes and **clear**; **import…** from RimSort or RimCrow |
| **uninstall** | **uninstall RimArcho…** (section 22) |

`1`–`9` jump to the first nine sections. Most choices apply at once.

**Feedback.** **bugs and suggestions…** opens a form: title, description, and optionally Mods, paths and logs to attach. Read the preview, then **copy the text**, **open GitHub**, or **send**. Reports are public; nothing is sent until you choose to.

**Updating RimArcho.** **check for updates** shows the new version and its notes; **install update** says how much it downloads, replaces the program files, and restarts. The download is verified first, your data stays, and a failed update rolls back. If another RimArcho window is open, the update waits for it to close. From a terminal: `archo update --check`, then `archo update`.

---

## 18. The terminal interface

Run `archo` (or `archo tui`) in a terminal. It opens the current list's editor with Disabled, Enabled and Staging, and details when the terminal is wide enough. It uses the terminal's font and shows no images. It starts in about a third of a second with 1,354 Mods.

| Key | Does |
| --- | --- |
| Arrows, `1`–`4` | Move; jump to Disabled, Enabled, Staging, details (arrows never change the list) |
| `PgUp` / `PgDn` | Move a page in a list; scroll the details when they have focus |
| `shift+↑/↓`, `ctrl+space` | Select a range; select one more |
| `space` | Enable / disable / put back |
| `t` / `T` | Stage or put back; stage all or put all back |
| `m`, `J` / `K` | Move to a position or after a Mod; move down / up |
| `g` | Tags |
| `enter`, `ctrl+w` | Details; Workshop page |
| `S` / `w` / `L` | Sort / save / go to launch (launch there with `L` or `enter`) |
| `u` / `ctrl+r` | Undo / redo |
| `/` | Search: words, `tag:`, `author:`, `type:`, `has:`, `-not` |
| `P` / `E` | Lists / switch game |
| `R` / `A` | Rescan / check the files again |
| `shift+1` / `shift+2` | Launch / can it run |
| `c` | Updates and changes |
| `ctrl+k` / `?` | Command panel / all keys |
| `:` | Run an `archo` command (`tab` completes, arrows recall history) |
| `esc` | Close a panel, back toward the editor |

A few keys differ from the window: in the terminal, `tab` switches sub-pages and `i` shows technical information. Some pages are simpler than in the window: the full game log page is window-only, and packs and settings appear as text panels.

---

## 19. The command line

`archo help` lists every command; `archo <command> --help` shows its options. Output is plain indented text, the same text AI agents get.

### References you can paste

Results contain short references that work in the next command:

| Reference | Points to |
| --- | --- |
| `#a3f9` or `a3f9` | One Mod copy |
| `L#7c2e` | All copies of one Mod |
| `pkg:ceteam.combatextended` | A Mod by package ID (if several copies match, you get a choice) |
| `ws:2890901044` | A Workshop item |
| `p:3f2a1c` | A problem |
| `log:E3` | An error cluster in the current log |
| `r:1` | One of your rules |
| `def:ThingDef/Gun_Revolver` | A Def |
| `v3` | A version in the list's history |

In PowerShell and bash, quote a leading `#` (`archo mod "#a3f9"`) or leave it off; unquoted it starts a comment.

Many commands that change files take `--dry-run` to show what would happen. Uninstall and some save actions need `--yes`.

### Commands by task

| Task | Commands |
| --- | --- |
| Overview | `status`, `scan`, `list`, `find`, `mod`, `library`, `changes`, `history`, `describe` |
| Games and folders | `env detect/list/add/use/set/remove`, `env branches`, `env download`, `folder list/check/set/move/add/remove` |
| Edit the list | `enable`, `disable`, `move`, `swap`, `sort`, `stage add/drop/restore`, `undo`, `redo` |
| List repairs | `loadout complete-dependencies`, `loadout replace-recommended`, `loadout use-installed` |
| Lists and history | `plan list/new/use/rename/remove/versions/cost`, `loadout log/diff/checkout`, `loadout import/export`, `import rimcrow` |
| Collections | `plan follow/unfollow/sync`, `collection` |
| Rules | `rule add/edit/list/remove` |
| Problems | `problems`, `problems show/ignore/unignore`, `problems mute-version/unmute-version`, `analyze`, `why` |
| Content and code | `def`, `patches`, `harmony`, `files search`, `source`, `source search`, `source game` |
| Logs | `log`, `pattern` |
| Play | `save`, `launch`, `test run`, `monitor` |
| Copies | `pin`, `unpin`, `copy-local`, `unhide`, `trash`, `links check/repair`, `identity`, `lineage split/merge` |
| Subscriptions | `subscribe workshop/git/page/list/check/update/remove`, `workshop subscribe/unsubscribe/resubscribe/state` |
| Player data | `alias`, `note`, `tag` (incl. `tag rename/merge/delete/note`), `mod-settings`, `mod-settings set` |
| Packs | `pack build/list/launch/export/import/check/remove/unlink/stop` |
| Saves | `saves list/show/index/fix/rename/copy/backup/trash` |
| Textures | `textures status/optimize/clean/tool` |
| Upkeep | `config get/set`, `update`, `deps`, `db status/backup/vacuum`, `cache status/clear`, `gc`, `data status/update`, `uninstall` |
| Interfaces | `gui`, `tui`, `mcp`, `guide`, `version` |

Examples:

```
archo find "Combat Extended"
archo problems --code patch.no-match
archo log --cluster E3
archo sort --dry-run
archo launch --dry-run
archo source "#a3f9" --type MyMod.CompThing
archo config set ui.lang zh
```

---

## 20. AI agents (MCP)

`archo mcp` lets an AI agent (Claude Code, Codex, or any MCP client) use RimArcho. The agent sees the same facts and evidence as you and can take the same actions; every tool returns exactly what the matching command prints.

**Claude Code:**
```
claude mcp add rimarcho -- C:\Tools\RimArcho\archo.exe mcp
```

**Codex**, in `~/.codex/config.toml`:
```toml
[mcp_servers.rimarcho]
command = 'C:\Tools\RimArcho\archo.exe'
args = ["mcp"]
```

**Any client**, in `.mcp.json`:
```json
{ "mcpServers": { "rimarcho": { "command": "C:\\Tools\\RimArcho\\archo.exe", "args": ["mcp"] } } }
```

By default the agent gets the tools for diagnosing and fixing a list: reading Mods, Defs, patches, Harmony patches, logs, Mod settings, saves and C# source; editing, saving and launching the list; test runs; locking versions; rules; ignoring problems. Add `--tools all` (`"args": ["mcp", "--tools", "all"]`) to also let it manage packs, subscriptions, the library, textures, memory estimates, upkeep, tags and notes, and change Mod settings and saves. Each tool costs the agent context, so add them only when you want the agent to do those jobs.

Then ask, for example: "My log has `Patch operation … failed` errors, which Mod is wrong?", or paste a stack trace.

**Safety.** List edits go into the same undo history as yours. Read-only tools are marked so your client can approve them automatically; tools that change files are marked as such, and the agent is told to preview and ask you first. Approval itself is up to your agent program's permission settings, so set them to ask before changing files. The agent can preview an uninstall but cannot run one.

RimArcho also ships the `rimworld-probe` agent skill (in `skills/`), which lets an agent build a small test Mod and run it in the game to check how something behaves (section 21).

The built-in assistant, Archo, is planned for 1.1 (section 24).

---

## 21. Test runs

A test run starts the real game with a separate, throwaway user folder, so your saves, settings and logs are untouched. It checks whether a list reaches the main menu (or, with `--map`, a fresh test map), and reports load time, peak memory, missing Mods and grouped errors with their likely sources. It compares the errors with your own last log and with the previous comparable test run, so you see what a change fixed or broke.

```
archo test run
archo test run --map
```

A **probe run** (`archo test run --probe <folder>`) adds a small purpose-built test Mod that sets up a situation in a test map and reports the outcome (and can save a screenshot). It is for checking one specific behaviour; the `rimworld-probe` skill helps an agent write one. The probe is linked only for the run and never joins your list.

Test runs work with RimWorld 1.6 and 1.7, from the command line and for AI agents (`test_run`). The window has no test-run button yet.

---

## 22. Your files, backups and uninstalling

### What RimArcho writes

RimArcho writes game files only when you ask:

| Where | What |
| --- | --- |
| `Config/ModsConfig.xml` in the game's user data folder | Your list, on save or launch; the previous file is kept as `ModsConfig.xml.rimarcho.bak` |
| Game `Mods/` | Links named `_Archo_…` and `!ArchoPack_…` (locked and local copies live in the lock library and are linked in) |
| Mod settings files | Only when you edit a setting (with a `.rimarcho.bak`), or a copy for a renamed folder |
| Mod folders, Workshop ones included | Only on request: un-hiding (`About.xml.disabled` → `About.xml`), DDS textures you converted, moving a folder to the recycle bin |
| Lock library, packs, downloads | Copies and downloads you asked for, in folders you chose |

Apart from those requested actions it does not change Mods' files. Your Mods, saves and packs go to the Windows recycle bin when removed; the DDS files RimArcho made itself are deleted directly when you remove converted textures.

### Where RimArcho keeps its own data

| Folder | Contains |
| --- | --- |
| `%APPDATA%\RimArcho\` | The database (lists, history, tags, rules), settings, caches, logs, the source library, packs and backups by default |
| `%LOCALAPPDATA%\RimArcho\` (or the folder you chose) | Locked copies, Archo Monitor, subscriptions, SteamCMD and its downloads, tools, account sessions |
| The program folder | `RimArcho.exe`, `archo.exe`, docs, the bundled Monitor and agent skills |

### Backups

- **settings › storage › back up** (or `archo db backup`) saves the database to `backups\` in the data folder. Do this before trying an older version.
- **saves › back up** copies saves to `backups\saves`.
- The game's list has one automatic backup (`ModsConfig.xml.rimarcho.bak`); the full list history lives in RimArcho.

### Uninstalling

Close the game and other RimArcho windows, then **settings › uninstall RimArcho…** (or `archo uninstall` to preview, `archo uninstall --yes` to run). The preview lists every step with its folders and sizes. By default it:

- keeps packs and downloaded games;
- moves saves that only RimArcho kept out to a folder you choose;
- recycles locked copies;
- keeps the game's current list but removes RimArcho's entries from it;
- removes its links, converted textures, account sessions, data and program files.

It never unsubscribes Workshop items or removes your ordinary Mods and saves. If a step fails, RimArcho keeps its data so you can try again.

---

## 23. Recipes

**Set up a new list from scratch.** **all lists** → **new / import** → **an empty list** → **create and edit**. Search, `space` to enable, `S` to sort, `F` on **can it run** to fill in dependencies, then `L` in the editor and `L` again on **start**.

**A Mod update broke my game.** **updates** → find the Mod (**Affects launch**) → open its changes. To keep playing, **lock this version** of a copy that still works (or switch to an older copy with `x`), or disable it and stage its dependants with `t`. Check **can it run** and launch.

**Find the Mod that causes an error.** Open **game log**, pick the first source error, read its evidence. If unclear, stage half the suspects (`t`), launch, compare; put them back with `T`. Undo restores everything.

**Keep a long playthrough safe from updates.** Make a pack of the list (**create a pack…**). It has its own saves and never changes. Or lock the few Mods you care about.

**Play 1.6 and 1.7 side by side.** Register both installs on **game**, give the second **own saves**, and mark each list's **game versions**. Download an older version with SteamCMD if you need one.

**Share a list with a friend.** **export…** → RimArcho list (they see which versions match) or Workshop links. For the exact files, **export** a pack.

**Load a friend's list.** **new / import** → paste the share code, file or collection link → **preview** → **subscribe all N** → **create and edit**.

**Rescue an old save.** **saves** → the save → **can it load?** → apply the fixes (or **make a list** from the save) → check the list → launch.

**Move from RimSort or RimCrow.** **settings › storage › import…**, check the preview, import. Your old manager's files stay as they were.

**Ask an AI agent for help.** Connect `archo mcp` (section 20) and describe the problem or paste the error. Review its proposed changes; undo works on its edits too.

---

## 24. Not in this version

Planned, not yet available:

- **Archo, the built-in AI assistant** (1.1): a panel in the window using the same tools, with confirmation before changes. Today, connect an outside agent through MCP instead.
- **Untouched content** (1.1): reading your saves across colonies to find Mods and content you have never used.
- **Subscribe or unsubscribe a whole Workshop collection in one action** (1.2); following a collection already works.
- **Playthrough themes and balance conflicts** (1.2).
- **What's worth playing** (later): shared packs and recommendations based on what you have not played.
- A test-run button in the window; matching a save to a list that is only *almost* the same; restoring items from the recycle bin inside RimArcho; a general Workshop search.

Known limits: custom patch operations some Mods define cannot be simulated (RimArcho says so); Harmony targets chosen at runtime cannot be resolved; Git subscriptions do not support private repositories, submodules or LFS; detecting a running game works on Windows only.
