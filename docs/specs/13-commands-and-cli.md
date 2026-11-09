# 13 · Commands and CLI

> Status: the tool layer, references, text format and CLI are implemented; the MCP server is 14. Not implemented: `unplayed` (12 §3) and the reference forms marked in §2.
> Basis: 00, 00a, and the command semantics in 02–12.

This spec covers the command and tool layer that every surface shares, the references and text format of its output, and the CLI commands. The MCP server that serves the same commands to agents is 14; Archo uses the same tools (15).

## 1. One command and tool layer

### 1.1 Tool layer

All RimArcho operations share a command table in `internal/cmds` and the same operation functions in `internal/app`. Five surfaces use this layer without implementing separate logic:

```
Tool layer: command table (internal/cmds: arguments, execution, outline text)
            └─ operation functions (internal/app, via internal/view when needed)
  ├─ CLI: archo <command> [arguments]        → print text
  ├─ MCP: tool call                         → same CLI argv, same text
  ├─ Archo (15): same MCP tool table         → same text
  ├─ TUI: keys / archo in command panel      → update UI or show same text
  └─ GUI: buttons, drag and drop            → typed view/app functions (same as TUI)
```

- The CLI and MCP output for the same command is **identical word for word**. Each MCP tool translates its arguments into a CLI argv and returns the text from `cmds.Execute` without extra formatting. Exceptions: MCP `config get` hides the SteamCMD account name (§1.2), a Mod reference that matches nothing suggests `find_mods` on MCP where the CLI suggests `archo find` (§3.3), the coverage line of `source search` and the `help:` line of the `build:` block name the MCP call (`scope="active"`, `guide topic=rebuild`) where the CLI names its flag or command (§4.2), and every printed command is a tool call on MCP (below). Golden tests and `TestCLIParity` cover both surfaces.
- **Suggested commands on MCP.** A result's `next:` and `fix:` lines and the commands in parentheses (`archo source iqmu --type ModA.CompBroken`, `(archo log --all)`) are what the CLI player types. A tool result suggests the tool call instead: `read_source ref=iqmu type=ModA.CompBroken`, `(analyze_log all=true)`, `edit_loadout action=enable refs=[z3bw]`. `cmds.ExecuteContext` rewrites the result when the context is an MCP call (`cmds.IsMCP`; `cmds.MCPHints`, `internal/cmds/mcphints.go`), command by command, with a table of the tools' parameters: a command is translated only when every flag and argument has a parameter (`archo problems` becomes `list_problems min_severity=info`, since the tool shows warnings and above unless asked); one that has no tool (`archo plan use`), uses a flag the table lacks or holds a placeholder (`<ref>`) stays as written. `guide` text and raw documents (source text) are not rewritten. `TestHintsAreValidToolCalls` checks that each translation builds the argv of the command it replaces.
- Archo has no private tools; its table is the MCP tool table in 14 §2 (15 §2.1).
- When the TUI or GUI changes data, it calls typed `internal/app` or `internal/view` functions. It never builds command text to parse it again. `cmds.Execute` is used only where the result is text by nature: the sort, save and launch previews; inspect actions; Def, Patch and Harmony pagination in full-screen details; and archo commands the player types (16 §1). Both paths share the operation functions, so players and agents receive the same facts.
- Language: CLI and MCP text uses a fixed English vocabulary so that agents can parse it reliably. User content, such as Mod names, aliases and notes, keeps its original text. The TUI and GUI show interface text in Chinese or English, using the English text as the lookup key (`view.Text` T/Tf). Command output in the command panel stays in English. The interface language is chosen in this order: `ARCHO_LANG` → `ui.lang` (`auto`, `en`, or a bundled locale directory name) → the system UI language (Windows `GetUserPreferredUILanguages`, otherwise `LANG`). In English, and when the CLI starts, the Chinese translations are not parsed; a selected locale is loaded once and shared by concurrent readers (16 §2.14).

### 1.2 A tool for every player action

Account sessions are a player-only exception. MCP has no tools for logging in, forgetting a session, reading, writing or importing credentials, controlling a login window, accepting site notices, or acting as a general authenticated HTTP proxy. Account-related output only says that the player needs to log in; it contains no display name, cookies, tokens, temporary URLs or raw authentication logs. Account queries in settings also show only a coarse status. If a download uses a local session, the agent can name file pages and results but cannot make authenticated requests. 11 §4 owns the storage and isolation rules.

AI assistants are a second class of user (00 §3.2), so **every action the player can take in the interface needs a matching command and tool**, labeled as read-only or a change. Without a tool, Archo and external agents can only suggest that the player press a button.

When adding a player action, add its CLI command and MCP tool at the same time. Tool writes use the same operation functions and undo history (07 §3).

Every tool exists, but `archo mcp` serves only the core group by default; the player's management tools are served only with `--tools all` (14 §1, §2). A core session can run the same commands through the CLI.

The following table maps player actions to tools. Missing tools are marked **not implemented**:

| Player action | Current state | Target |
| --- | --- | --- |
| Launch plan (09 §2) | Complete | `launch_game`, `destructiveHint: true` |
| List, create, switch, rename, and delete plans (07 §1) | Complete | `manage_plans`, `destructiveHint: true`: deleting a plan also deletes its history and cannot be undone. |
| Write to the game without launching | Complete | `save_loadout`, `destructiveHint: true`; its description says it saves without launching. |
| View history, compare versions, and check out a version (07 §3) | Complete | Read-only `plan_history` provides log, diff, and steps; checkout uses `edit_loadout`. |
| Handle outside changes: view the diff, load, or ignore (07 §4.3) | Complete | Use `loadout diff v<n> workspace`, `loadout checkout v<n>`, or `loadout ignore-external`. The tools are `plan_history action=diff`, `edit_loadout action=checkout` / `ignore_external`. |
| Lock, unlock, and update a locked version (07 §5) | Complete | `pin_mod` maps to `pin`, creates a locked copy, and swaps it into the current plan. `unpin_mod` unlocks the version. `pin --update <instance>` updates it through `pin_mod`. |
| Build, list, launch, export, import, remove, or stop building packs, and remove leftover links (10) | Complete | `manage_packs`, `destructiveHint: true` |
| Ignore/unignore findings (05 §4.2) | Complete | `ignore_problem` |
| Mute or restore Mod version reminders for a game version (05 §4.2) | Complete | `mute_version_warnings`; `undo=true` restores them. Ask the player before muting in bulk. |
| Answer identity questions and split or merge Lineages (04 §2.4, §3) | Complete | `resolve_identity`, `destructiveHint: true` |
| Scan | Complete | `scan` |
| List, filter, and sort the Mod library; move copies to the lock library or remove them from all plans (16 §2.9) | Complete | The CLI provides `library`, `library move-to-locked` and `library remove-from-plans`. `list_library` is read-only; `manage_library` is a change with `destructiveHint: true` because it moves `Mods` folders. Adding to the current plan uses `edit_loadout`; locking, updating, and unlocking use `pin_mod` and `unpin_mod`; restoring visibility and recycling use `manage_hidden`. |
| List subscriptions; add Git, page, or Workshop subscriptions (using SteamCMD if needed); check, update, and remove them (11 §1, 16 §2.9) | Complete | The CLI provides `subscribe list/git/page/workshop/check/update/remove`. `manage_subscriptions` has `destructiveHint: true` because updates replace files and removal recycles folders; it also has `openWorldHint: true`. The origin is detected from `url` as the window's paste field does (`subscribe.Detect`, with a shared test table). LoversLab sessions are player-only and have no tools: the agent reads public pages, while the player downloads the files and supplies `from`. Steam manages updates for client Workshop items; unsubscribe and resubscribe use `manage_library`. |
| Import and export lists and compare Workshop collections (07 §7) | Complete | `exchange_list`; importing is one undoable plan edit. |
| List and delete user rules | Complete | `manage_rules` |
| Restore visibility or recycle hidden Mods (09 §5, 02 §3) | Complete | `manage_hidden`, `destructiveHint: true` |
| Manage environments and settings | Complete | Read-only queries are part of `status` (`show=envs/folders/settings/data/db/cache`). Changes are player-only because they affect RimArcho's own configuration broadly. |
| Update community data, import RimCrow data, maintain and vacuum the database, and run gc | Complete | `maintenance`, `destructiveHint: true` |
| Update RimArcho (17 §5.1) | Complete | There is no tool: replacing the program is a player decision, like uninstalling. The `automatic checks` line of `status` reports a new version (`RimArcho <version> available`). |
| Uninstall (02 §2) | Complete | Only the read-only `preview_uninstall` is available (`uninstall --dry-run`, with the CLI parameters). There is no execution tool; 02 §2 gives the reason. |
| List and inspect saves, fix a save's list, rename, copy, back up and recycle saves, view unplayed content (12 §1.6, §4) | Partial: `saves list/show/index/fix/rename/copy/backup/trash`, `list_saves`, `get_save` and `manage_saves` are implemented; `unplayed` and `list_unplayed` are not | CLI `saves list/show/index/fix/rename/copy/backup/trash` and `unplayed` (§4.2). Read-only tools `list_saves` and `get_save` (core) and `list_unplayed`; the change `manage_saves` (all tools, `destructiveHint: true`) (14 §2). |

Launch, `import rimcrow`, gc and `identity resolve` are MCP tools too, with annotations that say they write files or cannot be undone. The external agent's harness decides whether to permit them; Archo always asks the player to confirm (15 §2.2).

## 2. References

Every object has stable references that can appear in any output and can be passed directly as arguments to another command (I2). The complete set is:

| Reference | Target | Example | Notes |
| --- | --- | --- | --- |
| `#<id>` | Instance | `#a3f9` | The shortest prefix unique within the environment, at least 4 characters. It is independent of the path and package ID (04). |
| `L#<id>` | Lineage | `L#7c2e` | Its prefix is calculated separately from Instance prefixes. If the Lineage has several Instances, commands that need one Instance list the candidates. |
| `pkg:<packageId>` | Matching Instance | `pkg:ceteam.combatextended` | Matching ignores case. If ambiguous, all candidates are listed and none is chosen arbitrarily (I3). A bare packageId means the same (below). |
| `ws:<Workshop-ID>` | Matching Instance | `ws:2890901044` | It may match several Steam, SteamCMD, or local copies. |
| `def:<Type>/<defName>` | Def | `def:ThingDef/Gun_Revolver` | The `def` command also accepts `Type/defName`, `Type@Name` for an abstract parent, and a bare defName. |
| `p:<6-hex-digits>` | Finding | `p:3f2a1c` | The first 3 bytes of sha1(code \| key \| subject Lineage), stable across analyses. |
| `log:E<n>` | Current log cluster n | `log:E3` | `--cluster` also accepts `E3` without the prefix. |
| `r:<n>` | User rule | `r:1` | |
| `decl:<#id>:<field>:<package-id>` | Declared order constraint | `decl:#iqmu:loadafter:tester.y` | `rule add ignore` target (08 §2) |
| `q:<n>` | Identity question | `q:12` | A bare number is also accepted. |
| `src:<label>` | Source | `src:workshop` | `env rebind`/`env set --remove-source` |
| `scan:<n>` | Scan | `scan:17` | `changes --since scan:<n>` |
| `env:<name>` | Environment | `env:main` | This form appears only in output; arguments and `--env` use the bare name. |
| `v<n>` | Plan version | `v3` | Used by `loadout diff/checkout`; `workspace` means the current edits. |
| `sv:<hex>` | Save (12 §1.6) | `sv:4be1c0` | The first 3 bytes of sha1(real path), stable while the file stays where it is (`app.SaveRef`). A full path, a file name, a name without a RimSaves folder, or `<colony>/<save>` also resolves (`SaveList.Find`); several matches list the candidates (exit 3). |
| `pattern:<id>` | Error pattern (06 §8) | `pattern:g:xml.type-missing` | The reference is environment-independent. `pattern:m:<packageId>:<hash>` identifies a Mod-specific known error. |

- A bare packageId is also accepted wherever a Mod reference is: `CETeam.CombatExtended` means `pkg:ceteam.combatextended`, in any case (`app.PackageRef`, used by `Index.Resolve`, `disable` of missing entries, `files search --mod` and version reminders). A word is a bare packageId when it has two or more dot-separated parts of ASCII letters, digits, `_` and `-`, with at least one letter (`app.BarePackageID`). No other reference has that shape: ID prefixes have no dot, prefixed forms contain `:` or `#`, Def references `/` or `@`, paths `/` or `\`, names a space, and a game version such as `1.6` has no letter. One that matches nothing is an error that names it (§3.3), as for `pkg:`; `disable` notes it was not enabled.
- Bare ID prefixes are also accepted (`archo mod a3f9`). PowerShell and bash treat words starting with `#` as comments, so `archo mod #a3f9` loses its argument. To include `#`, use double quotes (`"#a3f9"`); cmd passes single quotes literally. A `#` within a word (`L#7c2e` or `decl:#iqmu:…`) is not a comment. Printed commands omit `#` (§3.1 rule 11). Arguments from the TUI command line (`:`), the GUI and MCP bypass the shell, so either form works.
- `harmony:<Type.Method>` (for example, `harmony:Verse.Pawn.Tick`) appears only in `harmony --target` headings. As an argument, use the bare method name. Input with the prefix is not implemented.
- References to a single patch operation, `patch:#<id>/<file>#<ordinal>`, are not implemented. Use `patches --mod` and the file:line to locate the operation.
- The `plan` commands identify a plan by name. Plan names are unique within an environment, ignoring case.

## 3. Text format

### 3.1 Rules

1. Output is outline text with references, rather than JSON. The only exception is `log --follow --json`, which emits one JSON line per new cluster for scripts (06 §6).
2. The first line names the subject: `<reference>` `<name>`, optionally followed by a short status.
3. Use two-space indentation for hierarchy. Each line is either `key: value` or a list item.
4. Always include references for entities (§2) so they can be passed directly to the next command.
5. By default, a list contains at most 30 items and a call returns at most 40 lines (I5). On overflow, the last line gives the remaining count and an expansion command: `… 37 more (archo problems --all)`. If no expansion command is available, it shows only `… 37 more`.
6. An evidence line has the form `evidence: <source-type> <reference> <explanation>`.
7. The output may end with a `next:` section listing suggested commands.
8. No ANSI colors, even in a terminal, so the MCP, redirected and terminal output is identical.
9. Columns are padded to at most 40 characters so that a long name does not push the whole column aside. East Asian ambiguous-width characters –, … and · count as 1 cell (17 §7).
10. Times are in the local timezone, or UTC in tests.
11. Printed archo commands use an Instance ID without `#`: `fix: archo enable nyh3`, `… 3 more (archo problems --subject 5sed)`, and `error: #z3bw is not enabled (archo enable z3bw)`. They can be copied into cmd, PowerShell, or bash without quotes (§2). A command runs from `archo` to the end of the line, or to the enclosing `)`, a `;`, or two spaces. References outside commands still use `#nyh3`; a `#` within a word and double-quoted text are unchanged. `render.ShellSafe` does this in one place, in `Doc.String` and error output; rendering code still uses `Index.Ref`. This affects only printed text: MCP arguments still accept `#nyh3`.

### 3.2 Detail levels

- The default output is a summary. `--all` removes truncation; for findings, it also removes collapsing and shows categories hidden by default, except that more than 30 info findings are listed by code (05 §4.5). `--detail` expands the original evidence, such as XML or log snippets.
- `problems --ref` and `problems show` open the same focused details; MCP `list_problems ref=` returns identical text. Providers of missing parent templates are looked up on demand (05 §4.3), with a bounded wait that can be cancelled. The details show the impact, enable commands and subscription candidates first, and leave out an empty `fixes:` section. For unresolved templates, `providers:` says that the lookup is incomplete or that no installed Mod defines them. Then comes the affected content with its file:line; a separate evidence section is used only for other notes. Items from the same Mod that are not ignored are grouped; ignored items are checked one by one. The sections are limited by default; `--ref … --all` expands the repair and ignore commands. 05 §4.3 gives the provider and log conditions. Inactive targets and known errors also lead with their impact and available actions. Details leave out the identity, file, Workshop and history lists of the Mod card; `mod` remains the complete card.
- The default finding display, including collapsing, hiding and stale-log counts, follows 05 §4.5. When findings are hidden by default, the line `hidden: N def.override, M patch.contested` explains them.

### 3.3 Exit codes and errors

| Code | Meaning | Destination |
| --- | --- | --- |
| 0 | Success | stdout |
| 1 | Error, starts `error:` | stderr |
| 2 | A usage `error:` for an unknown command or argument, or the wrong argument count, with `usage:` | stderr |
| 3 | Ambiguous reference, candidate list | stdout |

A Mod reference that matches nothing is `error: <ref> matches nothing (<why>; search with archo find <words>)` (`app.RefError`): `no mod has packageId <id>` with the packageId's longest dot-separated part as the search word (`khamenman.outfitstandsplus` → `archo find outfitstandsplus`), `no mod has Workshop ID <id>`, or `not a reference (…)` for a word of no reference form, searched as given. On MCP the search names the tool instead: `search with find_mods query=<words>` (`RefError.MCPError`, chosen by `cmds.ForMCP` on the tool call's context). The rest of the error, and every other error, reads the same on both surfaces (§1.1, 14 §1).

If a player types `archo enable #nyh3` in PowerShell or bash, the shell treats the reference as a comment and the program receives only `archo enable` (§2). When the missing argument is a Mod reference, the usage error ends with `(in PowerShell and bash a word starting with # is a comment: write a mod reference as nyh3, not #nyh3)` (`cmds.commentHint`). This applies when there are too few arguments and the usage contains `<ref>`, `<ref…>`, `<A>`, or `<mod>`, when a `<ref>` flag value is missing, or when `textures` is given no Mod. Commands that can affect all targets need references or `--all`; given neither, they refuse with the same hint (`refsOrAll`). These are `stage restore`, `subscribe update`, and the `textures` operations that need `--plan` or `--all`. MCP `edit_loadout restore` without refs still restores all, by passing `--all`. Read-only commands with optional references keep their normal behavior: `changes` reads the current plan, and `subscribe check` checks all subscriptions.

### 3.4 Examples

`archo mod pkg:ceteam.combatextended`

```
#a3f9 Combat Extended  [active 34/212, workshop ws:2890901044]
  package: ceteam.combatextended
  lineage: L#7c2e (also: #d10b local copy, inactive)
  authors: CETeam
  supports: 1.5 1.6
  depends: pkg:brrainz.harmony (#0f12, active 2)
  load-after: pkg:brrainz.harmony
  incompatible: pkg:mlie.yayoscombat3 (#51aa, active 51)
  content: defs 1240, patches 386, assemblies 3, harmony patches 214
  assemblies:
    Assemblies/CombatExtended.dll  2081280 B, 1406 types
    Assemblies/0Harmony.dll  2243072 B, 1950 types, library
    Assemblies/CETools.dll  31744 B, 12 types  same name as #b2c1   (asm.duplicate-name, 05 §4.3)
  problems: 2 error, 3 warning
    error incompat.active  #51aa Yayo's Combat 3 is active
    error log.exception    log:E3 attributed (role: patcher)
    … 3 more (archo problems --subject a3f9)
  changes: updated 2026-10-17 (archo changes a3f9)
next:
  archo why a3f9 51aa
  archo harmony --mod a3f9 --shared
```

`archo def ThingDef/Gun_Revolver`

```
def:ThingDef/Gun_Revolver
  defined-by (load order):
    1. #5k2q Core            Defs/ThingDefs_Misc/Weapons/RangedIndustrial.xml:120
    2. #e4c1 VWE Firearms    1.6/Defs/Weapons.xml:88  ← effective
  patched-by:
    #a3f9 Combat Extended    Patches/Core/Weapons.xml:14  Replace /Defs/ThingDef[defName="Gun_Revolver"]/verbs  hit 1
    #e4c1 VWE Firearms       Patches/CE.xml:3             Add ... hit 0  (patch.no-match: target-later)
```

`archo log --cluster E3`

```
log:E3 exception x512  first at Player.log:4021
  message: ArgumentOutOfRangeException in List`1.get_Item
  top-frames:
    Verse.ThingWithComps.Tick
    Verse.Pawn.Tick
  attribution:
    #a3f9 Combat Extended   patcher  evidence: log harmony:Verse.Pawn.Tick#prefix "- PREFIX CombatExtended.HarmonyCE"
    #77b0 Some Comp Mod     patcher  evidence: log harmony:Verse.Pawn.Tick#postfix
  confidence: possible (no source identified)
next:
  archo harmony --target Verse.Pawn.Tick
  archo source search "AllComps.Remove(" --active --yes --budget 10
```

## 4. CLI commands

### 4.1 Entry points and global parameters

| Syntax | Effect |
| --- | --- |
| `archo` (no arguments), `archo tui` | Open the TUI in the current terminal. Double-clicking `archo.exe` uses the Windows default terminal (16 §9.2). `archo --here` does the same. |
| `archo gui` | Open the desktop window (16 §8.1). A build without the GUI returns an error explaining that this build has no window and the text interface (`archo`) has the same features. |
| `archo mcp [--tools core\|all]` | Run the stdio MCP server (14) with the core tools (default) or all tools (14 §1). |
| `archo <command> [arguments]` | Execute one command and exit. |
| `--env <name>` / `--env=<name>` | Use another environment for this command only. |
| `--open-plan <name>` / `--open-plan=<name>` | Use another plan for this command only, without changing the current plan (07 §1.3; used by GUI analysis and pack subprocesses). Saving or launching makes that plan current. |
| `--version` / `-v` / `archo version` | Print the version. |
| `archo help`, `archo <command> --help` | Show the command table, or a command's usage and each argument. |

Double-clicking `RimArcho.exe` opens the GUI, optionally with `--env`. With command arguments, it executes the CLI for GUI scan and analysis subprocesses (17 §5).

### 4.2 Command table

Commands and output call a plan `loadout` or `plan`. The editing commands below act on the **current plan**.

| Group | Command | Description |
| --- | --- | --- |
| Environments | `env list` / `env detect` / `env use <name>` | List environments (one per game directory) and shared Mod folders; suggest detected game, user-data, and Workshop paths (03 §3.2); or switch the current environment. |
| | `env add <name> [--detected] [--game <directory>] [--saves shared\|own \| --user-data <directory>] [--launch direct\|steam]` | Create an environment (03 §3.1), one per game directory. Add its Steam library's Workshop directory if it is missing. The first environment becomes current. `--workshop`, `--steamcmd`, and `--extra` are aliases for `folder add`. |
| | `env set <name> [--game <directory>] [--saves shared\|own] [--user-data <directory>] [--launch …] [--launch-args <arguments>]` | Change the game directory (rebinding `Data` and `Mods`, 03 §4.2), saves, launch method, or extra launch arguments (for example, `-popupwindow`; `""` clears them). `--remove-source` is the same as `folder remove`. |
| | `env branches [--install-steamcmd]` / `env download <branch> --to <directory> --account <account-name> [--name <environment>] [--saves …] [--install-steamcmd]` | Use SteamCMD to list RimWorld branches, or download a version and add it as an environment. Login happens in SteamCMD (11 §3). |
| Mod folders | `folder list` / `folder add workshop\|steamcmd\|extra <directory>` / `folder remove src:<label>` | Manage Mod folders shared by all environments (03 §4.1). MCP has the read-only `status show=folders`. |
| | `env rebind src:<label> yes\|no` | Answer a pending source rebinding: whether the new path is the original source (03 §4.2). |
| | `env remove <name> [--yes]` | Forget an environment: remove only RimArcho's data and leave the game files alone. Without `--yes`, preview what will be forgotten, what is kept, and which environment comes next. Refuse while that game is running. Removing the current environment selects another. The GUI environment page and TUI `E` behave the same way (16 §2.8). MCP has no tools that change environments; environment changes are player-only. |
| Scan | `scan [--full] [--game-only] [--progress]` | Scan and reconcile (04 §1). `--full` rehashes all semantic files; `--game-only` scans only the current game's `Data` and `Mods`; `--progress` writes `progress: scan …` lines to stderr (04 §1.1). |
| Queries | `status` | Show the current environment overview: game version, Mod counts per source, residue, Lineages, last scan, changes, the plan's enabled and missing counts and differences from the game list, a ModsConfig version-mismatch notice, finding counts, and whether a game is running. |
| | `list [--active] [--filter <expression>] [--all]` | List Mods. `--active` lists the current plan in load order. Filters accept `source:<label\|kind>`, `status:live\|tombstone\|all`, `residue`, `unloadable`, `copies`, `pkg:`, `ws:`, `tag:`, `lineage:`, and plain words. By default, only live Instances are listed. |
| | `find <words…> [--all]` | Search by name, alias, package ID, author, Workshop ID, or folder. |
| | `mod <reference> [--diff <reference>]` | Show Mod details; `--diff` lists the semantic files that differ from another Instance. A Mod with settings gets a settings section (16 §2.9 "Mod settings"). A Mod whose folder holds C# source gets a `build:` block: source folder, `.cs` count, project, framework, packages, publicizer, output path and the `dotnet restore` / `dotnet build -c Release` commands, read from the files (05 §5.6). |
| | `mod-settings [<reference>] [--all]` | Mod settings (16 §2.9 "Mod settings"): the current list's Mods that have settings, or what one Mod's settings files and HugsLib packs store. |
| | `mod-settings set <reference> [<name>=<value>…] [--reset <name>]… [--file <key>] [--dry-run]` | Change a Mod's simple settings values while no game runs, after backing the file up (16 §2.9 "Changing values"). |
| | `describe <reference> [--source auto\|about\|workshop\|both] [--refresh] [--offline]` | Show the `About` description (descriptionsByVersion for the game version) and the Workshop description (BBCode converted to plain text, with images as `[image: URL]`). 16 §4.3 gives the auto choice, networking and caching. `--offline` reads only the cache. |
| | `def [<Type/defName>] [--mod <reference>] [--all]` | List the definers and patchers of a Def in load order, marking the effective one, or list all `Defs` in a Mod. |
| | `patches [--mod <reference>] [--target <Type/defName \| defName \| XPath-fragment>] [--failed] [--all]` | Show patch operations and simulation results. `--failed` lists only patches with no match and their reasons (05 §4.4). `Type/defName` lists the top-level operations that touched that def, nested ones included. A bare defName lists those whose top-level XPath contains it or that touched any def of that name, which finds operations inside `FindMod` or `Sequence` (they have no XPath of their own), and says so on a `target:` line. Any other fragment matches top-level XPaths only. |
| | `harmony [--mod <reference>] [--target <Type.Method \| fragment>] [--shared] [--all]` | Show Harmony patches for a Mod, on a method, or shared by multiple Mods. |
| | `why <A> <B>` | Explain the relationship between two Mods: their positions, declarations in both directions, constraints and violations, shared `Defs`, cross-Mod patches, shared Harmony targets and patch kinds, and shared findings. |
| | `problems [--ref <p:reference>] [--subject <reference>] [--code <code-or-prefix>] [--severity <level>] [--min-severity <level>] [--all] [--detail] [--ignored] [--include-muted] [--no-analyze]` | List findings (05 §4). Stale analysis is rebuilt in a subprocess unless `--no-analyze` is given. `--include-muted` shows ignored version reminders with `[version warning ignored]`; otherwise only the `ignored-version-warnings` count is reported. |
| | `problems show\|ignore\|unignore <p:reference>` | Show a finding's evidence and repair commands, ignore it until its Mod changes, or restore it. |
| | `problems mute-version\|unmute-version <Mod-references…> [--game-version X.Y]` | Mute or restore version reminders (05 §4.2), by default for the current game's major/minor version. A `pkg:` reference works even after the Mod is removed. The Mod card shows the preference and the restore command. |
| | `changes [<reference>] [--since scan:<n>\|<days>d\|YYYY-MM-DD] [--all]` | Show change events, by default those of the latest scan. |
| | `library [--search <word>] [--include <key:value>]… [--exclude <key:value>]… [--sort name\|size\|changed\|played] [--lineage <reference>] [--facets] [--all]` | Show the Mod library (16 §2.9), one Lineage per row: copies, total size, most important state, and usage. Lineage details list each copy's path, states, plans, last play, and actions. `--facets` lists filter values and counts. Filters include `lists:<name>` (16 §3.7), `tags:` (exact), `content:`, `has:`, `name:`, `author:`, and `id:`. For example, CLI `--include "lists:Main colony"` or MCP `list_library` with `include: ["lists:Main colony"]`. 03 §2.2 gives the source roles and aliases; `location:` is also accepted. |
| | `library remove-from-plans <references…>` | Remove the references from all plans in all environments that share the game directory, with one undo per plan. |
| | `library move-to-locked <reference> [--dry-run]` | Move a spare copy in `Mods` to the lock library. The Instance is unchanged and plans keep using it (16 §2.9). |
| | `log [--cluster <E#>] [--file <path>] [--prev] [--text <text>] [--select] [--all] [--since <cursor>]` | Attribute logs (06). `--prev` reads Player-prev; `--text` analyzes a pasted log or stack; `--select` includes the log in problems. The cluster list ends with `cursor: log1.…`. `--since` returns only new clusters, or counts that rose (+N). After a restart, it reports that the cursor expired and lists all clusters of the new run (06 §6). `--since` cannot be combined with `--cluster` or `--text`. |
| | `pattern [<pattern:id>] [--mod <packageId>] [--all]` | Show a pattern's recognition rules, common causes, diagnostic routes, fixes, and corpus counts (06 §8). With no arguments, list generic patterns; `--mod` lists a Mod's known errors. |
| | `log --follow [--once] [--json]` | Follow Player.log while the game runs and print new clusters (06 §6). `--once` stops after reading the current content; `--json` prints one line per cluster. |
| | `files search <pattern> [--mod <reference>]… [--regex] [--case] [--word] [--def] [--kind <kinds,…>] [--languages] [--lang <language>]… [--all-versions] [--game with\|without\|only] [--game-version <x.y>] [--mode matches\|files\|count] [--page <n>] [--limit <n>] [--per-file <n>] [--all]` | Search the loaded scope (05 §5.5): by default, the enabled copies, current-version folders, and `Defs`, `Patches`, and `About`, without translations. Results are grouped in load order, with the position, copy, folder, and enclosing Def or XPath, and are paginated. `--def` finds a defName's definitions, parent-child relationships, patches, and references. |
| | `source <reference> [--type <Namespace.Type>] [--member <name>] [--all]` / `source game [--type <Namespace.Type>] [--member <name>] [--all]` | Read or decompile the selected Mod's or the current game's assembly into the Source library (05 §5.4); return library paths. `--type` shows a type of at most 250 lines and 12 KB whole; a longer one gets its first 34 lines and a `members:` index, `--all` shows it whole, and `--member <name>` (needs `--type`) shows one member with its line numbers (05 §5.4 "Reading a type"). |
| | `source search <pattern> [--in <reference>… \| --implicated \| --active --yes] [--game] [--regex] [--all] [--budget <seconds>]` | Search C# in named Mods, implicated Mods, or every enabled Mod; `--game` adds the current game's code and can be the only scope. A cold `--active` search first estimates the assembly count, DLL bytes, generated size (about 3× DLL bytes) and time (about 1.5 seconds per assembly); `--yes` confirms. The budget stops new decompiles: started work finishes, and assemblies not yet started are marked pending; repeating the call continues. Identical code is reported once, attributed to every copy that shares it. The result ends with a coverage line, `searched X of N enabled mods (named\|implicated\|active)` and `not searched: R (--active --yes)`, and a search that found nothing in a narrowed scope says the code may be in the mods not searched (05 §5.4 "Search report"). |
| | `source callers <Namespace.Type.Method \| Namespace.Type..ctor>[(N)] [--in <reference>…] [--all]` | List every call of a method in the current game's `Assembly-CSharp.dll` and the enabled Mods (or the named ones) with each argument's shape and a summary per parameter (05 §5.4). Reads IL; needs no ilspycmd. |
| | `source diff <old.dll> <new.dll> [--all]` | Compare two builds of an assembly: changed, added and removed methods, member and assembly references (non-public game members flagged), attributes (05 §5.4). Read-only. |
| | `source clean [--dry-run] [--yes]` | Without `--yes`, preview the count, bytes and paths of unused folders. `--yes` removes unused folders; `--dry-run` always previews. Which folders are in use: 05 §5.4. |
| | `source decompile --active` | Decompile the current list's Mods on request, with progress. GUI Settings queues this as a background task that can be stopped (16 §2.16). |
| | `analyze` | Rebuild the Def, Patch, and Harmony facts. Other commands normally request this on demand in a subprocess. |
| Plan edits | `enable <references…> [--at <n>] [--before <reference>] [--after <reference>]` / `disable <references…>` | Enable or disable the referenced Mods. |
| | `move <reference> [--to <n>] [--before <reference>] [--after <reference>] [--up] [--down]` / `swap <reference> [<reference>]` | Move an entry, or swap it for another Instance in the same Lineage. |
| | `sort [--dry-run]` | Sort by constraints (08 §1). `--dry-run` only previews the result. |
| | `undo [<n>]` / `redo [<n>]` | Undo or redo the latest step, or n steps (07 §3). |
| | `stage list` / `stage add <references…>` / `stage drop <references…>` / `stage restore <references…> \| --all [--at\|--before\|--after]` | Manage staging (07 §2.2). Mods staged from the enabled list remember their original positions. `restore --all` restores all staged Mods. Add and restore are each one undoable edit; Mods that are enabled again leave staging by themselves. |
| History | `history [--all]` | Show the current plan's edit steps (undo stack) and versions, by default the latest 10 versions. |
| | `loadout log [--all]` / `loadout diff [<v>] [<v>]` / `loadout checkout <v>` | List versions, compare them (by default, latest→workspace), or load a version into the current plan in one undoable edit. |
| Write/launch | `save [--dry-run]` | **Write without launching** (called "save" in the interface), for players who launch through Steam or another route. Render the current plan into ModsConfig.xml: back it up, replace it atomically, and write the current game version. Sync links and remove pack links (07 §4.1, 09 §3). `--dry-run` previews the write. |
| | `launch [--dry-run]` | Launch the current plan (09 §2); `--dry-run` only previews. Write before launching, refuse while a game is running, and record the launch and the game version launched. `--dry-run` shows the `save --dry-run` preview plus the launch command. |
| | `test run [<plan>] [--env <environment>] [--map \| --save <save>] [--ticks <n>] [--probe <folder> [--probe-after <packageId> \| --probe-before <packageId>]] [--timeout <duration>] [--settle <duration>] [--json]` | Run with isolated data (20 §2), by default the current plan. Launch directly, collect the loading log, then stop. The default timeout is `10m` (a map or save run: twice the previous one's load time, at least `10m`), with a 5 s wait after reaching the menu. `--save` loads a save (a `saves list` reference or name, or a `.rws` path) in the run's own data and plays `--ticks` (default 4000); `--ticks` with `--map` plays that many ticks on the test map. A report that proves nothing starts with `NOT TESTED:`. `--probe` adds a probe Mod to the tested list (last by default) and runs a test map until its `[Probe] RESULT` line, then waits up to `--settle` (default 15 s) for it to quit (20 §3). Exit 0 if the target is reached with no clusters (a probe run: its RESULT line), otherwise 1. The report goes to stdout and progress to stderr. |
| Plans | See §4.3 | |
| Exchange | `loadout export [--format text\|archo\|modsconfig\|rimsort\|rml\|rimcrow\|rimcrow-xml] [--out <file>]` | Export the current plan. `--out` never overwrites an existing file. |
| | `loadout import <file\|share-code> [--new [<name>]]` | Replace the current plan from a text list, ModsConfig, save, `.rml`, RimSort list, RimCrow share code, or Workshop IDs. This is undoable and writes no game files (07 §7). `--new` creates a new plan instead (07 §7.1), named by default after the list's own name or file name. |
| | `collection <URL\|ID> [--import]` | Read a Workshop collection online and compare it with the installed Mods. `--import` adds the installed subset to the current plan in collection order, as an undoable edit. |
| Locked versions | `pin <instance> [--name <folder>] [--git] [--dry-run]` | Create a locked version: copy the Mod to the lock library (default `<folder>_local`), write `.rimarcho/origin.json`, optionally create a Git baseline, and swap the copy into the current plan only (07 §5). `--update` updates the locked version; `--move-to-library` moves the old `Mods/` copy into the library. `copy-local` is an alias. |
| | `unpin <reference> [--dry-run]` | Unlock: restore and use the Workshop copy, also for RimCrow-style locked versions. |
| Hidden Mods | `unhide <reference> [--dry-run]` / `trash <reference> [--dry-run]` | Restore `About.xml.disabled` to `About.xml`; or recycle RimArcho's own copies in `Mods`, the lock library or SteamCMD, and hidden or unsubscribed Workshop folders (09 §5, 02 §3). |
| Workshop | `workshop unsubscribe <references…> [--dry-run]` / `workshop subscribe <references…> [--dry-run]` / `workshop resubscribe <references…> [--dry-run]` / `workshop state <references…>` | Unsubscribe through the Steam client (Steam deletes the folder), resubscribe, or read the status. If Steam is unavailable, report an error and give the Workshop page (11 §2). |
| | `workshop publish [--version X.Y.Z] [--item <id>] [--note <text>] [--visibility public\|unlisted\|private\|friends] [--description <file>] [--description-zh <file>] [--preview <file>] [--dry-run]` | Maintainer-only RimArcho upload from the repository root (§4.4; 17 §5.1). No MCP tool. |
| Packs | `pack build <name> [--dry-run]` / `pack stop <name>` | Build the current plan into a pack, or stop a running build and remove its partly copied files (10 §2). |
| | `pack list` / `pack launch <name> [--dry-run]` / `pack unlink [--dry-run]` | List or launch packs, or remove only the `!ArchoPack_` links in `Mods` (10 §3; the fix for `pack.links-left`). |
| | `pack export <name> <file> [--list]` / `pack import <file> [--as-plan]` / `pack remove <name> [--dry-run] [--unfinished]` | Export a `.archopack` (zip), or only its list as `.archolist.json` (07 §7.3). Import extracts the pack or compares the list; `--as-plan` also loads the list into the current plan. Remove recycles the pack; `--unfinished` deletes an interrupted build (10 §4). |
| Rules | `rule add after\|before\|incompatible\|attach <A> <B> [--note <text>]` / `rule add replaces <A> <B\|pkg:id> [--note]` / `rule add top\|bottom <A> [--note]` / `rule add ignore <constraint> [--note]` | Manage user rules (08 §2). `replaces`: A stands in for package B where a Mod requires it. `ignore` takes the constraint references that `sort` or `problems` print. |
| | `rule edit <r:n> <kind> <A> [<B>] \| ignore <constraint> [--note]` | Replace a rule and keep its `r:n` reference (08 §2.1). |
| | `rule list` / `rule remove <r:n>` | |
| Player data | `alias <reference> [<name>] [--clear]` / `note <reference> [<text>] [--clear]` | Set the display name or notes on the Lineage. |
| | `tag add\|remove <reference> <tags…> [--mod <reference>]…` / `tag list` / `tag rename <tag> <new> [--dry-run]` / `tag merge <tags…> --into <tag> [--dry-run]` / `tag delete <tags…> [--dry-run]` / `tag note <tag> [<text>] [--clear]` / `tag color <tag> <colour>\|--clear` / `tag order <reference> <tags…>` | Manage tags. Repeating `--mod` applies the change to several Mods at once. `list` shows each tag's Mod count, pinned, colour and note. `rename`, `merge`, `delete`, `note` and `color` act on the tag itself, on every Mod (07 §6.1); `color` takes blue, cyan, teal, grass, orange, pink, plum, violet, `#rgb`/`#rrggbb` or none and refuses anything else (red and amber too). `order` puts a Mod's named tags first, in that order; its first tag's colour marks its rows (16 §3.8); a rename to a name another tag already has is refused, naming the `merge` command to use instead. |
| | `import rimcrow --db <path> [--rules <path>] [--dry-run] [--include-disabled-rules] [--all]` | Import RimCrow aliases, notes, groups (as tags) and user rules once. It can be run again (07 §6). |
| Identity | `identity [list] [--all]` / `identity why <q:n>` / `identity resolve <q:n> same\|copy\|unrelated` | With no subcommand, list identity questions. Listing, inspecting, and answering all include a trimmed comparison (04 §2.4). |
| | `lineage split <reference>` / `lineage merge <reference> <reference>` / `gc [--dry-run]` | Split or merge Lineages, or collect expired tombstones and unused snapshots (04 §2.5). |
| Community | `data update [<datasets…>] [--dry-run]` / `data status` | Download rules, steamdb and replacements on request, or show their sources and download and check times (08 §2). The interactive interfaces also update them automatically (16 §2.16). |
| Maintenance | `db status` / `db backup` / `db vacuum [--dry-run]` | Show the database path, size, and backups; write a consistent copy under `data/backups`; or compact the database (04 §4.2). |
| | `cache status` / `cache clear <category>` | Show cache sizes, or clear previews, workshop, images, source, or logs. The cache is rebuilt on demand. |
| Settings | `config get [<key>]` / `config set <key> <value>` | See the settings keys below. |
| Help | `guide [<topic>]` | Read the agent help (14 §3). With no topic, list the topics. Choose `diagnose`, `errors`, `plans`, or `words` for a workflow, or a tool name for its full reference. Output is identical to MCP; an unknown topic is a usage error. |
| Archo Monitor | `monitor` / `monitor install [--dry-run]` | Show the bundled and installed versions, source, and position in the current plan; install or update Archo Monitor and add it by default (19 §6, 07 §1.5). |
| Update | `update [--check]` | First check the selected self-update source (17 §5.1), recording it like an automatic check (16 §2.16.1 A15). `--check` shows the source, any check fallback reason, version, date, download size (unknown for Workshop), files to replace, other RimArcho processes to wait for, and release notes, without downloading. Otherwise, download, verify SHA-256, wait for other processes in the same program folder to close, replace the files, and verify that the new program runs. SteamCMD download progress is indeterminate. The progress says which process it is waiting for; `Ctrl+C` stops without changes. If verification fails, roll back. Do not restart; ask the player to reopen RimArcho. The data directory is unchanged (17 §5.1). Development builds do not check. |
| Uninstall | `uninstall [--dry-run] [--yes] [--keep-locked [--locked-to <environment>]] [--remove-packs] [--packs-to <directory>] [--remove-saves] [--saves-to <directory>] [--remove-games] [--restore-modsconfig-backup] [--all]` | Remove what RimArcho created and reverse its changes (02 §2). Without `--yes`, only preview, as with `--dry-run`; giving both is a usage error. Flags select the non-default choice: `--keep-locked` moves copies into the game's `Mods` (the default is recycling); `--remove-*` recycles content (the default is keeping it); `--*-to` selects where kept content goes; and `--restore-modsconfig-backup` restores the backup instead of cleaning the current list. Refuse while a game or another RimArcho is running. |
| Saves | `saves list [--colony <name>] [--mod <reference>] [--attention] [--all]` | Read-only. Colonies and their saves (12 §4.1), newest first, each save with its reference, time, in-game day (`day N` from the cached full read, 12 §1.7; `meta only` for a save not read in full yet, with a note counting them and naming `archo saves index`; `malformed` when its read stopped early; `unreadable` when a stored read of the file as it is failed; `saves list` itself reads no save in full), matched list with its rule (`plan:<name> (launch\|same-mods)`, `pack <name>`, `no list`) and verdict (12 §4.3). The global `--env` keeps one game's saves; without it every game's and pack's are listed. `--mod` keeps saves whose Mod list has that Mod, installed or not (a Mod reference, `pkg:`, a bare packageId or `ws:`); what removing it costs (12 §4.6) is not implemented. `--attention` keeps saves with warnings or errors. Five saves per colony and about 34 lines by default; `--colony` (the player's name or the saves' own) or `--all` lifts the per-colony limit. Code: `internal/cmds/saves.go`. |
| | `saves show <save> [--mods] [--play] [--content] [--current] [--all]` | Read-only (writes only the full-read cache). Reads the save in full first when it has no valid cache row (12 §1.7: inline, tens of ms per 10 MB), so the load check and the colony see the read (a save read now joins its world's colony, 12 §4.1). Where it belongs (colony, game version and environment, list with the rule that matched, file, time and size), `read:` (`in full`; `stopped at line N, byte B: <why>` for malformed XML, the facts shown being those before that point; `not yet: the file is being written`, with no verdict), the playthrough `play:` (player faction, in-game day and date, real play time; storyteller, difficulty and permadeath; colonists alive, slaves and dead with animals, mechanoids and prisoners; wealth and the statsRecord numbers; `since <save> <sv:ref> (day N):` the changes since the colony's previous save when its read is cached, else that it is not read yet with `archo saves index`; the 3 newest letters; 12 §4.3, §4.5), can it load (each item with its consequence and its `fix:` command, then the `quick fix:`), and the Mod list differences (`--mods`: the whole list in save order). `--play`: the playthrough in full (the world and scenario, research, every colonist with where they are, `dead`, `slave`, and `new` / `died` against the previous save; the letters newest first with their LetterDef), the load check shrunk to its verdict and item count, and the Mod list only with `--mods`. Without `--all` the call keeps to 40 lines (§3.1 rule 5): 10 colonists and 6 letters with `--play`, then the load check's items and the Mod rows share what is left (at least 5 Mod rows, 15 with `--mods`), each ending `… N more (archo saves show <ref> … --all)`. `--content` selects content instead of play/load-check details: distinct reference names, totals by category, one row per attributed Mod in save order, source not found, not analysed and coverage limits (12 §2). It uses installed versions and stays within 40 lines without `--all`; omitted Mods name `--content --all`. Content takes precedence over `--play`/`--mods`; `--current` does not change its source scope. Code: `savesShow`, `internal/cmds/saves_play.go`, `saves_content.go`. |
| | `saves index [<saves…>] [--only] [--progress]` | Full read of new and changed saves into the cache (12 §1.7), the named saves first, then the rest newest first (`--only`: just the named ones); writes only the cache and colony names. Without `--only`, also removes the cached reads of saves that are gone and settles colony names (`App.SettleColonyNames`, 12 §4.1). A save whose stored read failed is skipped until it changes (12 §1.7). `--progress` prints `progress: saves <n>/<total> <sv:ref> <colony>/<save>` per save on stderr, and `progress: saves-failed <sv:ref> <reason>` after it for a save that could not be read. The GUI runs it as a child process (`app.ChildSavesIndex`). |
| | `saves fix <save> [--mod <packageId>…] [--disable-added] [--order] [--launch] [--current] [--dry-run]` | The save's Mod fixes as one undoable plan edit (`app.FixSave`: `--mod` only those Mods; `--disable-added` and `--order` the information rows), then the Workshop subscriptions through `subscribe workshop`'s operation in the save's environment, then with `--launch` the plan is launched through `launch` and becomes current (12 §4.4). `--launch` waits as 12 §4.4 says, suggesting `archo scan` and the same fix again. A Mod named by `--mod` that the list already loads is reported as already done (`SaveFixResult.Already`: no edit, no error); a packageId the save lacks is an error. A list entry that cannot be downloaded (missing with no Workshop ID, or a Workshop item installed under another packageId; `SaveFixResult.Stuck`) is listed as such and `--launch` does not wait for it, nor for a subscription that failed for good: it launches with a `without:` line naming them. A subscription failure otherwise keeps the edit and exits 1. |
| | `saves rename <save> <new name>` / `saves copy <save>` | Refused while the game runs (rename only) or when the name exists, also when it appears between the check and the rename (a rename that never replaces, 12 §4.4). The output gives the new file and its reference. |
| | `saves rename-colony <colony\|save> [<name>]` | Names a colony in RimArcho (12 §4.1): the colony by its name (the player's or its saves'), its key, or one of its saves. Without a name, or with the saves' own, the colony gets its saves' name back. Writes only RimArcho's database. |
| | `saves backup <saves…> [--yes] [--token <t>]` / `saves trash <saves…> [--yes] [--token <t>]` | Without `--yes`, only list the files and sizes and the command with `--yes --token <t>`; player confirmation required (12 §4.4). With `--token`, a save whose size or modification time changed since the preview refuses the whole action (`app.ErrPreviewChanged`: preview again); a backup with the token goes to the preview's folder. The trash preview is refused while the game runs. A save that fails is an `error:` line and exit 1. |
| | `unplayed [--group never\|partial\|functional] [--mod <reference>]` | Not implemented. Read-only (12 §3). |

**Log phases**: when explicit markers are present (06 §7, 19 §4), log cluster lines, `problems --code log` findings, and MCP `analyze_log` and `list_problems` include short phase names and counts, such as `[loading x3, save x2, play x4]`. Counts whose phase cannot be recovered are marked unknown. The log phases section lists the boundaries in file order, with any recorded save name and time; cluster details also show the phase counts for that cluster. `--follow` text uses the same annotations, while JSON adds an optional phases array with the phases and their counts. Without markers there are no phase sections, labels or JSON fields.

**Analysis parameter**: `--game-version X.Y` on `analyze`, `problems`, `def`, `patches`, `harmony`, `why` and `log` analyzes for another major/minor game version. The value must look like `1.6`; the version is restored after the command. The matching MCP tools accept `game_version`. An explicit log path or version does not bypass the check of which environment owns the log (06 §4).

**Automatic check status**: `status`, `data status` and the matching MCP output use one `automatic checks` line. It shows whether checks are on, the counts of pending Mod and tool updates, and a login reminder for a configured LoversLab account. If nothing is pending, it shows `nothing waiting (last MM-DD HH:MM)`; if no check has run, `not checked yet`. Accounts that do not apply and zero counts are left out. The line uses no network and shows no cookies. `deps` lists new versions of managed tools and their check times separately; `data status` lists each dataset's check time. 16 §2.16 covers permission and notifications, and 16 §2.16.1 the defaults and intervals.

**Settings keys** (`config set`): `ui.lang <auto|en|bundled-locale>` (16 §2.14); `ui.icons compat|nerd` (16 §9.3); `ui.mouse on|off` (turn off terminal mouse reporting to allow text selection); `workshop.fetch on|off` (fetch Workshop descriptions over the network); `workshop.images on|off` (download images in descriptions); `updates.auto on|off` and `updates.interval normal|frequent|relaxed` (16 §2.16); `updates.source auto|github` (17 §5.1); `ui.gpu on|off` (window GPU acceleration, applied when the next window opens, 16 §2.14); `ui.archo.motion off|still|live` (Archo's figure while the GUI waits, default live, 16 §2.14, §7.1); `ui.motion auto|reduced` (the window's motion: follow Windows, or reduced as under the system setting, 16 §2.14, §7.1); `tags.pinned <comma-separated-tags>`; `monitor.default on|off` (add Archo Monitor to plans automatically, 07 §1.5); and `monitor.dir <absolute-path>` (the Monitor directory, 19 §6). Settings are stored in the database meta table under `cfg:<key>`. `config.toml` contains only `tombstone_days` (17 §3).

### 4.3 Plan management

An environment may have several plans (07 §1). The commands are:

| Command | Effect |
| --- | --- |
| `plan list` | List all shared plans (07 §1.7), with the name, enabled count, last played date, and whether each is current in the environment. |
| `plan new <name> [--from current\|v<n>\|<file>]` | Create a plan from the current plan, one of its versions, or a list file in a `loadout import` format. |
| `plan use <name>` | Switch the current plan without writing game files. |
| `plan rename <old-name> <new-name>` | Rename a plan. |
| `plan remove <name> --yes` | Delete a plan and its history. The last plan cannot be deleted. |

- `loadout log/diff/checkout/export/import` act on the current plan. Import with `--new` creates a new plan; without it, import replaces the current plan in one undoable edit.
- Other commands act on the current plan, or on the plan named by the global `--open-plan` (§4.1).
- The three choices for an outside conflict (07 §4.3) are to view the diff between `v<n>` and `workspace`, load it with `loadout checkout v<n>`, or ignore it with `loadout ignore-external`. Ignoring clears the unimported-external flag but keeps the version in history.
- Versions have no names: a named list is a plan.
- Launch always writes to the game first; there is no launch without writing.

### 4.4 Workshop publishing (maintainer)

Run `archo workshop publish` from the repository root after building the release assets (17 §5). `--version` defaults to the running program's version; development builds need an explicit stable `X.Y.Z`. Prereleases are refused even with `--dry-run`. `--item` overrides the Monitor's remembered ID (a positive decimal integer); `--note` supplies the change note. Omitting `--visibility` preserves Steam's current/default value. `--description <file>` sets the item description from a Steam BBCode file and `--description-zh <file>` sets the Simplified Chinese one; each file is UTF-8, at most 8,000 bytes (Steam's limit), with a leading BOM and surrounding blank lines dropped. Without them publish uses the repository's `UserGuide/release-post.steam.txt` and `UserGuide/release-post.zh-CN.steam.txt`. Publish ends each description with a line `RimArcho v<version>`, replacing one already at the file's end; the update check reads that line (17 §5.1), and with it a description must still fit 8,000 bytes. Without `--description` and without the English file, publish is refused; without the Chinese one, that description is left as it is. The item's tags become `Mod` and the game versions in Monitor's About.xml (`1.5`, `1.6`, `1.7`), replacing all others; `--dry-run` lists them and the description's last line. `--preview <file>` uploads that PNG, JPG or GIF (under 1 MB; a GIF stays animated) as the item page's preview. Without it the preview is the repository's `docs/assets/release-post/workshop-preview.gif`, or Monitor's `About/Preview.png` when that file is absent. Assembly, ID persistence and the upload contract are in 17 §5.1.

`--dry-run` writes a staging folder and prints its absolute path, release filenames, title, target item or creation, replacing tag, effective visibility, optional note and preview, and the size of each description. It requires neither a configured environment nor Steam. A real publish uses the selected environment's Steam runtime, prints progress to stderr, and on success prints `ws:<id> RimArcho published`, the version and item page. When required, it tells the maintainer to accept the Workshop legal agreement on that page. Errors use §3.3. This command has no MCP tool.

## 5. Stability

- English output keywords, such as `depends:`, `evidence:`, `next:`, and finding codes, are part of the interface. Record changes in CHANGELOG (17 §5).
- Reference formats are stable. Instance IDs are independent of paths and package IDs (04); `p:` references are stable across analyses. A log finding key keeps only the last three segments of the absolute path, so its references and ignore records stay valid after the library directory moves. Pattern references `pattern:g:<id>` and `pattern:m:<packageId>:<hash>` are stable within a dataset version. Generic IDs persist across versions; a Mod-specific hash comes from its template and stays the same while the template does.
- New fields are appended without changing the meaning of existing lines.
- When renaming an MCP tool, keep the old name as an alias for at least one minor version.
- Moving a tool between the core and all groups (14 §1) changes what a default session lists: record it in CHANGELOG like a rename.

## 6. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| I1 | Execute the same command through the CLI and MCP. | The output is byte-for-byte identical except for the suggested commands, which MCP prints as tool calls (`TestCLIParity`: calls through an in-memory transport, the CLI text passed through `cmds.MCPHints`; `TestSaveToolsCLIParity`: the save tools, previews included; `TestMCPLogHintsAreToolCalls`: no `archo` command in an `analyze_log` result). | Pass |
| I2 | Take any reference from the output. | It can be passed directly as an argument to another command (`TestRefsRoundTrip`: `#id`, `pkg:`, p:, `log:E#`, and `def:`). | Pass |
| I3 | A pkg reference matches multiple Instances. | List all candidates without choosing one arbitrarily (`TestAmbiguousPackage`). | Pass |
| I5 | Read the default output. | A call returns at most 40 lines and gives an expansion command when truncated (`TestCLIParity` and `TestTruncation`). | Pass |
| I7 | Find a tool for each player action in §1.2. | Each action has a CLI command and MCP tool, with annotations matching its file writes. | Partial: `list_unplayed` is missing. |
| I8 | Call `save_loadout` with an unresolved outside conflict. | Refuse and offer diff, load, and ignore commands. Leave the file unchanged. | Pass |
| I9 | Call launch or save while a game is running. | Launch refuses and names the environment whose game is running. Save still writes and explains that the game will read the list on its next launch. | Pass |
| I10 | Run `plan new`, `plan use`, `plan rename`, `plan remove`, and `loadout import --new` through the CLI and MCP. | The output is identical. `plan use` writes no game files, and the last plan cannot be removed. | Pass |
| I11 | Paste the printed commands (`fix:`, `next:`, `… N more (…)`, and error hints) into cmd, PowerShell, or bash. | All arguments survive and references resolve. `TestPrintedCommandsSurviveShells` parses each with PowerShell 5.1 / 7, checks that nothing became a comment and that the expected words are there, then runs it; see also `TestShellSafe`. | Synthetic tests pass; real VM verification is pending. |
| I12 | Type a `#` reference in PowerShell or bash, where it becomes a comment. | If the missing argument is a Mod reference, the usage error explains why (`TestMissingRefNamesTheComment`, run on host PowerShell 5.1 / 7). | Synthetic tests pass; real VM verification is pending. |
