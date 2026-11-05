# 05 · Semantic Interface and MCP

> Status: the CLI and MCP are implemented (45 MCP tools including the guide tool; `archo mcp` serves the 29 core tools by default and all 45 with `--tools all`, §5.1). The save commands and tools are implemented except `unplayed` / `list_unplayed` (2026-11-04); those and a few reference forms are marked below.
> Basis: 00 and 00a; the command semantics in 02, 03, 04, and 08; and 09 (Archo uses the same tools).

## 1. One Command and Tool Layer

### 1.1 Tool Layer

All RimArcho operations share a command table in `internal/cmds` and the same operation functions in `internal/app`. Five surfaces use this layer without implementing separate logic:

```
Tool layer: command table (internal/cmds: arguments, execution, outline text)
            └─ operation functions (internal/app, via internal/view when needed)
  ├─ CLI: archo <command> [arguments]        → print text
  ├─ MCP: tool call                         → same CLI argv, same text
  ├─ Archo (09): same MCP tool table         → same text
  ├─ TUI: keys / archo in command panel      → update UI or show same text
  └─ GUI: buttons, drag and drop            → typed view/app functions (same as TUI)
```

- The CLI and MCP output for the same command is **identical word for word**. Each MCP tool translates its arguments into a CLI argv and returns the text from `cmds.Execute` without extra formatting. Exceptions: MCP `config get` hides the SteamCMD account name (§1.2), and a Mod reference that matches nothing suggests `find_mods` on MCP where the CLI suggests `archo find` (§3.3). Golden tests and `TestCLIParity` cover both surfaces.
- Archo has no private tools; its table is the MCP tool table in §5.2 (09 §2.1).
- When the TUI or GUI mutates data, it calls typed `internal/app` or `internal/view` functions. It never constructs command text and parses it again. `cmds.Execute` is used only for inherently textual cases: sort, save, and launch previews; inspect actions; Def, Patch, and Harmony pagination in full-screen details; and archo commands entered by the player (06 §1). Both paths share the operation functions, so players and agents receive the same facts.
- Language: CLI and MCP text uses fixed English vocabulary so that agents can parse it reliably. User content, such as Mod names, aliases, and notes, retains its original text. The TUI and GUI localize interface text into Chinese or English, using the English text as the lookup key (`view.Text` T/Tf). Command output in the command panel stays in English. The interface language follows this precedence: `ARCHO_LANG` → `ui.lang` (`auto`, `en`, or a bundled locale directory name) → the system UI language (Windows `GetUserPreferredUILanguages`, otherwise `LANG`). English and CLI startup do not parse Chinese interface translations; selected locales load once for concurrent readers (06 §2.14).

### 1.2 A Tool for Every Player Action

Account sessions are a player-only exception. MCP provides no tools for login, forgetting a session, reading, writing, or importing credentials, controlling a login window, accepting site notices, or acting as a general authenticated HTTP proxy. Account-related output only says that player login is required; it contains no display name, cookies, tokens, temporary URLs, or raw authentication logs. Account queries in settings also expose only coarse status. If a fixed download uses a local session, the agent can reference file pages and results but cannot make authenticated requests. See 07 §3.1 for the authoritative storage and isolation rules.

AI assistants are a second user class (00 §3.2), so **every action available to the player in the interface needs a corresponding command and tool**, labeled as read-only or a mutation. Without a tool, Archo and external agents can only suggest that the player press a button.

When adding a player action, add its CLI command and MCP tool at the same time. Tool writes use the same operation functions and undo history (04 §3).

Every tool exists, but `archo mcp` serves only the core group by default (§5.1, §5.2), because each listed tool costs agent context on every turn. The tools for these actions are served only by `archo mcp --tools all`; a core session can run the same CLI commands: packs (`manage_packs`), subscriptions (`manage_subscriptions`), library actions (`manage_library`), list exchange (`exchange_list`), textures (`manage_textures`), the memory estimate (`estimate_memory`), maintenance (`maintenance`), identity answers (`resolve_identity`), hidden Mods (`manage_hidden`), links (`check_links`, `repair_links`), version reminders (`mute_version_warnings`), player data (`set_player_data`), Mod settings changes (`set_mod_settings`), the uninstall preview (`preview_uninstall`) and save management (`manage_saves`). A new tool joins the core group only when agents need it to diagnose, edit, save, launch or test a plan.

The following table maps player actions to tools. Missing tools are marked **not implemented**:

| Player action | Current state | Target |
| --- | --- | --- |
| Launch plan (08 §2) | Complete | `launch_game`, `destructiveHint: true` |
| List, create, switch, rename, and delete plans (04 §1) | Complete | `manage_plans`, `destructiveHint: true`: deleting a plan also deletes its history and cannot be undone. |
| Write to the game without launching | The `save_loadout` description now says save without launching. | Complete |
| View history, compare versions, and check out a version (04 §3) | Complete | Read-only `plan_history` provides log, diff, and steps; checkout uses `edit_loadout`. |
| Handle outside changes: view the diff, load, or ignore (04 §4.3) | Complete | Use `loadout diff v<n> workspace`, `loadout checkout v<n>`, or `loadout ignore-external`. The tools are `plan_history action=diff`, `edit_loadout action=checkout` / `ignore_external`. |
| Lock, unlock, and update a locked version (04 §7) | Complete | `pin_mod` maps to `pin`, creates a locked copy, and swaps it into the current plan. `unpin_mod` unlocks the version. `pin --update <instance>` updates it through `pin_mod`. |
| Build, list, launch, export, import, remove, or stop building packs, and remove leftover links (08 §5) | Complete | `manage_packs`, `destructiveHint: true` |
| Ignore/unignore findings (03 §4.2) | Complete | `ignore_problem` |
| Mute or restore Mod version reminders for a game version (03 §4.2) | Complete | `mute_version_warnings`; `undo=true` restores them. Ask the player before muting in bulk. |
| Answer identity questions and split or merge Lineages (02 §5.4, §6) | Complete | `resolve_identity`, `destructiveHint: true` |
| Scan | Complete | scan |
| List, filter, and sort the inventory; move copies to the lock library or remove them from all plans (06 §2.9) | Complete | The CLI provides library, move-to-locked, and remove-from-plans. `list_library` is read-only; `manage_library` is a mutation with `destructiveHint: true` because it moves `Mods` folders. Adding to the current plan uses `edit_loadout`; locking, updating, and unlocking use `pin_mod` and `unpin_mod`; restoring visibility and recycling use `manage_hidden`. |
| List subscriptions; add Git, page, or Workshop subscriptions (using SteamCMD if needed); check, update, and remove them (02 §3.4, 06 §2.9) | Complete (2026-10-30) | The CLI provides `subscribe list/git/page/workshop/check/update/remove`. `manage_subscriptions` has `destructiveHint: true` because updates replace files and removal recycles folders; it also has `openWorldHint: true`. Detect the origin from url as the window paste field does (`subscribe.Detect`, with a shared test table). LoversLab sessions are player-only and have no tools: the agent reads public pages, while the player downloads files and supplies from. Steam manages updates for client Workshop items; unsubscribe and resubscribe use `manage_library`. |
| Import and export lists and compare Workshop collections (04 §9) | Complete | `exchange_list`; importing is one undoable plan edit. |
| List and delete user rules | Complete | `manage_rules` |
| Restore visibility or recycle hidden `Mods` (08 §6, §7) | Complete | `manage_hidden`, `destructiveHint: true` |
| Manage environments and settings | Complete (status show=envs/settings/data/db/cache) | Read-only queries are part of status. Changes are player-only because they affect RimArcho's own configuration broadly. |
| Update community data, import RimCrow data, maintain and vacuum the database, and run gc | Complete | maintenance, `destructiveHint: true` |
| Update RimArcho (07 §5.1) | Complete (2026-10-31) | There is no tool: replacing the program is a player decision, like uninstalling. The status `automatic checks` line reports a new version (RimArcho 0.9.1 available). |
| Uninstall (01 §4.1) | Complete | Only the read-only `preview_uninstall` is available (`uninstall --dry-run`, with the CLI parameters). There is no execution tool; see 01 §4.1 for the rationale. |
| List and inspect saves, fix a save's list, rename, copy, back up and recycle saves, view unplayed content (10 §1.6, §4) | Partial (2026-11-04): `saves list/show/index/fix/rename/copy/backup/trash`, `list_saves`, `get_save`, `manage_saves`. `unplayed` and `list_unplayed` are not implemented | CLI `saves list/show/index/fix/rename/copy/backup/trash` and `unplayed` (§4.2). Read-only tools `list_saves` and `get_save` (core) and `list_unplayed`; the mutation `manage_saves` (all tools, `destructiveHint: true`) (§5.2). |

The old rule excluding launch, `import rimcrow`, gc, and `identity resolve` from MCP is cancelled. Expose these tools with annotations indicating file writes or irreversible effects. The external agent's harness decides whether to permit them; Archo always requires player confirmation (09 §2.2).

## 2. References

Every object has stable references that can appear in any output and can be passed directly as arguments to another command (I2). The complete set is:

| Reference | Target | Example | Notes |
| --- | --- | --- | --- |
| `#<id>` | Instance | `#a3f9` | The shortest prefix unique within the environment, at least 4 characters. It is independent of the path and package ID (02). |
| `L#<id>` | Lineage | `L#7c2e` | Calculate its prefix separately from Instance prefixes. If the Lineage has multiple Instances, commands requiring one Instance list the candidates. |
| `pkg:<packageId>` | Matching Instance | `pkg:ceteam.combatextended` | Matching is case insensitive. If ambiguous, list all candidates without choosing one arbitrarily (I3). A bare packageId means the same (below). |
| `ws:``<Workshop-ID>` | Matching Instance | `ws:2890901044` | It may match multiple Steam, SteamCMD, or local copies. |
| `def:<Type>/<defName>` | Def | `def:ThingDef/Gun_Revolver` | The def command also accepts `Type/defName`, `Type@Name` for an abstract parent, and a bare defName. |
| `p:<6-hex-digits>` | Finding | `p:3f2a1c` | The first 3 bytes of sha1(code \| key \| subject Lineage), stable across analyses. |
| `log:E<n>` | Current log cluster n | `log:E3` | `--cluster` also accepts E3 without the prefix. |
| `r:<n>` | User rule | `r:1` | |
| decl:<`#id`>:`<field>`:`<package-id>` | Declared order constraint | `decl:#iqmu:loadafter:tester.y` | `rule add ignore` target (04 §6) |
| `q:<n>` | Identity question | `q:12` | A bare number is also accepted. |
| `src:<label>` | Source | `src:workshop` | `env rebind`/`env set --remove-source` |
| `scan:<n>` | Scan | `scan:17` | `changes --since scan:<n>` |
| `env:<name>` | Environment | `env:main` | This form appears only in output; arguments and `--env` use the bare name. |
| `v<n>` | Plan version | v3 | Used by `loadout diff/checkout`; workspace means the current edits. |
| `sv:<hex>` | Save (10 §1.6) | `sv:4be1c0` | The first 3 bytes of sha1(real path), stable while the file stays where it is (`app.SaveRef`). A full path, a file name, a name without a RimSaves folder, or `<colony>/<save>` also resolves (`SaveList.Find`); several matches list the candidates (exit 3). |
| `pattern:<id>` | Error pattern (03 §6.8) | `pattern:g:xml.type-missing` | The reference is environment-independent. `pattern:m:<packageId>:<hash>` identifies a Mod-specific known error. |

- A bare packageId is also accepted wherever a Mod reference is: `CETeam.CombatExtended` means `pkg:ceteam.combatextended`, in any case (`app.PackageRef`, used by `Index.Resolve`, `disable` of missing entries, `files search --mod` and version reminders). A word is a bare packageId when it has two or more dot-separated parts of ASCII letters, digits, `_` and `-`, with at least one letter (`app.BarePackageID`). No other reference has that shape: ID prefixes have no dot, prefixed forms contain `:` or `#`, Def references `/` or `@`, paths `/` or `\`, names a space, and a game version such as `1.6` has no letter. One that matches nothing is an error that names it (§3.3), as for `pkg:`; `disable` notes it was not enabled.
- Bare ID prefixes are also accepted (`archo mod a3f9`). PowerShell and bash treat words starting with # as comments, so `archo mod #a3f9` loses its argument. To include #, use double quotes (`"#a3f9"`); cmd passes single quotes literally. A # within a word (`L#7c2e` or `decl:#iqmu:…`) is not a comment. Printed commands omit # (§3.1 rule 11). TUI :, GUI, and MCP arguments bypass the shell, so either form works.
- `harmony:<Type.Method>` (for example, `harmony:Verse.Pawn.Tick`) appears only in `harmony --target` headings. As an argument, use the bare method name. Input with the prefix is not implemented.
- Single-operation references of the form patch:`#<id>`/`<file>`#`<ordinal>` are not implemented. For now, use patches `--mod` plus file:line to locate the operation.
- The plan command identifies a plan by name. Plan names are unique within an environment, ignoring case.

## 3. Text Format

### 3.1 Rules

1. Output is outline text with references, rather than JSON. The only exception is `log --follow --json`, which emits one JSON line per new cluster for scripts (03 §6.6).
2. The first line names the subject: `<reference>` `<name>`, optionally followed by a short status.
3. Use two-space indentation for hierarchy. Each line is either `key: value` or a list item.
4. Always include references for entities (§2) so they can be passed directly to the next command.
5. By default, a list contains at most 30 items and a call returns at most 40 lines (I5). On overflow, the last line gives the remaining count and an expansion command: … 37 more (archo problems `--all`). If no expansion command is available, show only … 37 more.
6. An evidence line has the form `evidence:` `<source-type>` `<reference>` `<explanation>`.
7. The output may end with a `next:` section listing suggested commands.
8. Do not output ANSI colors, even in a terminal. The MCP, redirected, and terminal output is therefore identical.
9. Pad columns to at most 40 characters so that a long name does not push the whole column aside. Count East Asian ambiguous-width –/…/· characters as 1 cell (07 §8).
10. Output times in the local timezone, or UTC in tests.
11. Printed archo commands use an Instance ID without #: `fix: archo enable nyh3`, … 3 more (archo problems --subject 5sed), and `error: #z3bw is not enabled (archo enable z3bw)`. They can be copied into cmd, PowerShell, or bash without quotes (§2). A command extends from archo to the end of the line, or to the enclosing ), a ;, or two spaces. References outside commands still use `#nyh3`; # within a word and double-quoted text are unchanged. `render.ShellSafe` handles this centrally in `Doc.String` and error output; rendering code still uses `Index.Ref`. This affects only printed text. MCP arguments still accept `#nyh3`.

### 3.2 Detail Levels

- The default output is a summary. `--all` removes truncation; for findings, it also removes collapsing and shows categories hidden by default. `--detail` expands the original evidence, such as XML or log snippets.
- `problems --ref` and problems show open the same focused details; MCP `list_problems ref=` returns identical text. Providers of missing parent templates are looked up on demand (03 §4.3), with a bounded, cancellable wait. Show the impact, enable commands, and subscription candidates first, omitting an empty `fixes:` section. For unresolved templates, show `providers:` to indicate an incomplete lookup or that no installed provider defines them. Then list the affected content with its file:line; use a separate evidence section only for other notes. Group unignored items from the same Mod, but check ignored items individually. Limit the sections by default; `--ref … --all` expands repair and ignore commands. See 03 §4.3 for provider and log conditions. Inactive targets and known errors also lead with their impact and available actions. Details omit the identity, file, Workshop, and history lists from the Mod card; mod remains the complete card.
- The default finding display, including collapsing, hiding, and stale-log counts, follows 03 §4.5. When findings are hidden by default, the line `hidden: N def.override, M patch.contested` explains them.

### 3.3 Exit Codes and Errors

| Code | Meaning | Destination |
| --- | --- | --- |
| 0 | Success | stdout |
| 1 | Error, starts `error:` | stderr |
| 2 | A usage `error:` for an unknown command or argument, or the wrong argument count, with `usage:` | stderr |
| 3 | Ambiguous reference, candidate list | stdout |

A Mod reference that matches nothing is `error: <ref> matches nothing (<why>; search with archo find <words>)` (`app.RefError`): `no mod has packageId <id>` with the packageId's longest dot-separated part as the search word (`khamenman.outfitstandsplus` → `archo find outfitstandsplus`), `no mod has Workshop ID <id>`, or `not a reference (…)` for a word of no reference form, searched as given. On MCP the search names the tool instead: `search with find_mods query=<words>` (`RefError.MCPError`, chosen by `cmds.ForMCP` on the tool call's context). The rest of the error, and every other error, reads the same on both surfaces (§1.1, §5.1).

If a player types `archo enable #nyh3` in PowerShell or bash, the shell treats the reference as a comment and the program receives only `archo enable` (§2). When the missing argument is a Mod reference, append `(in PowerShell and bash a word starting with # is a comment: write a mod reference as nyh3, not #nyh3)` to the usage error (`cmds.commentHint`). This applies when there are too few arguments and the usage contains `<ref>`, `<ref…>`, `<A>`, or `<mod>`, when a `<ref>` flag value is missing, or when textures has no Mod. Commands that can affect all targets require references or `--all`; if neither is given, refuse with the same hint (`refsOrAll`). These commands include `stage restore`, `subscribe update`, and textures operations requiring `--plan` or `--all`. MCP `edit_loadout restore` without refs still restores all, translating the existing parameters to `--all`. Read-only commands with optional references retain their normal behavior: changes reads the current plan, and `subscribe check` checks all subscriptions.

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
    Assemblies/CETools.dll  31744 B, 12 types  same name as #b2c1   (asm.duplicate-name, 03 §4.3)
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
  archo source search "AllComps.Remove(" --in a3f9 --in 77b0
```

## 4. CLI Commands

### 4.1 Entry Points and Global Parameters

| Syntax | Effect |
| --- | --- |
| archo (no arguments), `archo tui` | Open the TUI in the current terminal. Double-clicking `archo.exe` uses the Windows default terminal (06 §9.2). Legacy `archo --here` is accepted with the same effect. |
| `archo gui` | Open the desktop window. Delegate to the adjacent `RimArcho.exe` and return immediately if it exists; otherwise run in-process. |
| `archo mcp [--tools core\|all]` | Run the stdio MCP server (§5) with the core tools (default) or all tools (§5.1). |
| `archo <command> [arguments]` | Execute one command and exit. |
| `--env <name>` / `--env=<name>` | Use another environment for this command only. |
| `--open-plan <name>` / `--open-plan=<name>` | Use another plan for this command only, without changing the current plan (04 §1.3; used by GUI analysis and pack subprocesses). Saving or launching makes that plan current. |
| `--version` / `-v` / `archo version` | Print the version. |
| `archo help`, `archo <command> --help` | Show the command table, or a command's usage and each argument. |

Double-clicking `RimArcho.exe` opens the GUI, optionally with `--env`. With command arguments, it executes the CLI for GUI scan and analysis subprocesses (07 §5).

### 4.2 Command Table

Commands and output call a plan Loadout / plan. The editing commands below act on the **current plan**.

| Group | Command | Description |
| --- | --- | --- |
| Environments | `env list` / `env detect` / `env use <name>` | List environments (one per game directory) and shared Mod folders; suggest detected game, user-data, and Workshop paths (02 §2.2); or switch the current environment. |
| | `env add <name> [--detected] [--game <directory>] [--saves shared\|own \| --user-data <directory>] [--launch direct\|steam]` | Create an environment (02 §2.1), with one per game directory. Add its Steam-library Workshop directory if absent. The first environment becomes current. `--workshop`, `--steamcmd`, and `--extra` remain aliases for `folder add`. |
| | `env set <name> [--game <directory>] [--saves shared\|own] [--user-data <directory>] [--launch …] [--launch-args <arguments>]` | Change the game directory (rebinding `Data` and `Mods`, 02 §3.2), saves, launch method, or extra launch arguments (for example, `-popupwindow`; `""` clears them). `--remove-source` is equivalent to `folder remove`. |
| | `env branches [--install-steamcmd]` / `env download <branch> --to <directory> --account <account-name> [--name <environment>] [--saves …] [--install-steamcmd]` | Use SteamCMD to list RimWorld branches or download a version and add it as an environment. Login happens in SteamCMD (08 §11). |
| Mod folders | `folder list` / `folder add workshop\|steamcmd\|extra <directory>` / `folder remove src:<label>` | Manage Mod folders shared by all environments (02 §3.1). MCP provides the read-only `status show=folders`. |
| | `env rebind src:<label> yes\|no` | Answer whether this is the original source. |
| | `env remove <name> [--yes]` | Forget an environment, removing only RimArcho data and leaving game files untouched. Without yes, preview what will be forgotten, retained, and used as the next environment. Refuse if that game is running. Removing the current environment selects another. The GUI environment page and TUI E behave the same way (06 §2.8). MCP provides no mutations; environment changes are player-only. |
| Scan | `scan [--full] [--game-only]` | Scan and reconcile (02 §4). Full rehashes all semantic files; game-only scans only the current game's `Data` and `Mods`. |
| Queries | status | Show the current environment overview: game version, Mod counts per source, residue, Lineages, last scan, changes, the plan's enabled and missing counts and differences from the game list, a ModsConfig version-mismatch notice, finding counts, and game-running state. |
| | `list [--active] [--filter <expression>] [--all]` | List `Mods`. Active lists the current plan in load order. Filters accept `source:``<label\|kind>`, `status:live\|tombstone\|all`, residue, unloadable, copies, `pkg:`, `ws:`, `tag:`, `lineage:`, and plain words. By default, list only live Instances. |
| | `find <words…> [--all]` | Search by name, alias, package ID, author, Workshop ID, or folder. |
| | `mod <reference> [--diff <reference>]` | Show Mod details; the diff option lists semantic files that differ from another Instance. A Mod with settings gets a settings section (06 §2.9 "Mod settings"). |
| | `mod-settings [<reference>] [--all]` | Mod settings (06 §2.9 "Mod settings"): the current list's Mods that have settings, or what one Mod's settings files and HugsLib packs store. |
| | `mod-settings set <reference> [<name>=<value>…] [--reset <name>]… [--file <key>] [--dry-run]` | Change a Mod's simple settings values while no game runs, after backing the file up (06 §2.9 "Changing values"). |
| | `describe <reference> [--source auto\|about\|workshop\|both] [--refresh] [--offline]` | Show the `About` description (descriptionsByVersion for the game version) and the Workshop description (BBCode converted to plain text, with images as `[image: URL]`). See 06 §4.3 for auto selection, networking, and caching. Offline reads only the cache. |
| | `def [<Type/defName>] [--mod <reference>] [--all]` | List the definers and patchers of a Def in load order, marking the effective one, or list all `Defs` in a Mod. |
| | `patches [--mod <reference>] [--target <Type/defName \| XPath-fragment>] [--failed] [--all]` | Show patch operations and simulation results. Failed lists only patches with no match and their reasons (03 §4.4). |
| | `harmony [--mod <reference>] [--target <Type.Method \| fragment>] [--shared] [--all]` | Show Harmony patches for a Mod, on a method, or shared by multiple Mods. |
| | `why <A> <B>` | Explain the relationship between two Mods: their positions, declarations in both directions, constraints and violations, shared `Defs`, cross-Mod patches, shared Harmony targets and patch kinds, and shared findings. |
| | `problems [--ref <p:reference>] [--subject <reference>] [--code <code-or-prefix>] [--severity <level>] [--min-severity <level>] [--all] [--detail] [--ignored] [--include-muted] [--no-analyze]` | List findings (03 §4). Rebuild stale analysis in a subprocess unless no-analyze is given. Include-muted shows ignored version reminders with `[version warning ignored]`; otherwise report only the `ignored-version-warnings` count. |
| | `problems show\|ignore\|unignore <p:reference>` | Show a finding's evidence and repair commands, ignore it until its Mod changes, or restore it. |
| | `problems mute-version\|unmute-version <Mod-references…> [--game-version X.Y]` | Mute or restore version reminders (03 §4.2), defaulting to the current game's major/minor version. The pkg reference works even after the Mod is removed. The mod card shows the preference and restore command. |
| | `changes [<reference>] [--since scan:<n>\|<days>d\|YYYY-MM-DD] [--all]` | Show change events, defaulting to the latest scan. |
| | `library [--search <word>] [--include <key:value>]… [--exclude <key:value>]… [--sort name\|size\|changed\|played] [--lineage <reference>] [--facets] [--all]` | Show the inventory (06 §2.9), with one Lineage per row: copies, total size, most important state, and usage. Lineage details list each copy's path, states, plans, last play, and actions. Facets lists filter values and counts. Filters include `lists:<name>` (06 §3.7), `tags:` (exact), `content:`, `has:`, `name:`, `author:`, and `id:`. For example, CLI `--include "lists:Main colony"` or MCP `list_library` with `include: ["lists:Main colony"]`. See 02 §0.2 for source roles and aliases; legacy `location:` is retained. |
| | `library remove-from-plans <references…>` | Remove the references from all plans in all environments sharing the game directory, with one undo per plan. |
| | `library move-to-locked <reference> [--dry-run]` | Move a spare `Mods` copy to the lock library. The Instance is unchanged and plans continue to use it (06 §2.9). |
| | `log [--cluster <E#>] [--file <path>] [--prev] [--text <text>] [--select] [--all] [--since <cursor>]` | Attribute logs (03 §6). Prev reads Player-prev; text analyzes a pasted log or stack; select includes the log in problems. The cluster list ends with `cursor: log1.…`. Since returns only new clusters or counts increased by +N. After a restart, report that the cursor expired and list all clusters from the new run (03 §6.6). Since cannot be combined with cluster or text. |
| | `pattern [<pattern:id>] [--mod <packageId>] [--all]` | Show a pattern's recognition rules, common causes, diagnostic routes, fixes, and corpus counts (03 §6.8). With no arguments, list generic patterns; mod lists a Mod's known errors. |
| | `log --follow [--once] [--json]` | Track Player.log while the game runs and emit new clusters (03 §6.6). Once stops after reading the current content; JSON emits one line per cluster. |
| | `files search <pattern> [--mod <reference>]… [--regex] [--case] [--word] [--def] [--kind <kinds,…>] [--languages] [--lang <language>]… [--all-versions] [--game with\|without\|only] [--game-version <x.y>] [--mode matches\|files\|count] [--page <n>] [--limit <n>] [--per-file <n>] [--all]` | Search the loaded scope (03 §5.5): by default, the enabled copies, current-version folders, and `Defs`, `Patches`, and `About`, without translations. Group results in load order, with the position, copy, folder, and enclosing Def or XPath, and paginate them. Def finds a defName's definitions, parent-child relationships, patches, and references. |
| | `source <reference> [--type <Namespace.Type>] [--all]` / `source game [--type <Namespace.Type>] [--all]` | Read or decompile the selected Mod or current game's assembly into the Source library (03 §5.4); return library paths. |
| | `source search <pattern> [--in <reference>… \| --implicated \| --active --yes] [--game] [--regex] [--all] [--budget <seconds>]` | Search C# in named Mods, implicated Mods, or every enabled Mod; `--game` adds the current game's code and can be the sole scope. Active cold searches first estimate assembly count, DLL bytes, generated size (about 3× DLL bytes) and time (about 1.5 seconds per assembly); yes confirms. The budget stops new decompiles; dispatched work finishes, and undispatched assemblies are marked pending. Identical code is reported once with sharing-copy attribution. |
| | `source clean [--dry-run] [--yes]` | Without yes, preview unused folder count, bytes and paths. Yes removes unused folders; dry-run always previews. Liveness rules: 03 §5.4. |
| | `source decompile --active` | Explicitly decompile the current list's Mods with progress. GUI Settings queues this as a stoppable background task (06 §2.16). |
| | analyze | Rebuild the Def, Patch, and Harmony facts. Other commands normally request this on demand in a subprocess. |
| Plan edits | `enable <references…> [--at <n>] [--before <reference>] [--after <reference>]` / `disable <references…>` | Enable or disable the referenced Mods. |
| | `move <reference> [--to <n>] [--before <reference>] [--after <reference>] [--up] [--down]` / `swap <reference> [<reference>]` | Move an entry, or swap it for another Instance in the same Lineage. |
| | `sort [--dry-run]` | Sort by constraints (04 §5). Dry-run only previews the result. |
| | `undo [<n>]` / `redo [<n>]` | Undo or redo the latest step, or n steps (04 §3). |
| | `stage list` / `stage add <references…>` / `stage drop <references…>` / `stage restore <references…> \| --all [--at\|--before\|--after]` | Manage staging (04 §2.2). Mods staged from the enabled list remember their original positions. `restore --all` restores all staged Mods. Add and restore are each one undoable edit; re-enabled `Mods` automatically leave staging. |
| History | `history [--all]` | Show the current plan's edit steps (undo stack) and versions, defaulting to the latest 10 versions. |
| | `loadout log [--all]` / `loadout diff [<v>] [<v>]` / `loadout checkout <v>` | List versions, compare them (by default, latest→workspace), or load a version into the current plan in one undoable edit. |
| Write/launch | `save [--dry-run]` | **Write without launch** (called save in the UI), for players launching through Steam or another route. Render the current plan into ModsConfig.xml, backing it up, replacing it atomically, and writing the current game version. Sync links and remove pack links (04 §4.1, 08 §3). Dry-run previews the write. |
| | `launch [--dry-run]` | Launch the current plan (08 §2). Preview only with dry-run. Write before launching, refuse if a game is running, and record the launch and launch version. Dry-run shows the `save --dry-run` preview plus the launch command. |
| | `test run [<plan>] [--env <environment>] [--map] [--timeout <duration>] [--settle <duration>] [--json]` | Run with isolated data (08 §12), defaulting to the current plan. Launch directly, collect the loading log, then stop. The default timeout is `10m`, with a 5s wait after reaching the menu. Return 0 if the target is reached with no clusters, otherwise 1. Write the report to stdout and progress to stderr. |
| Plans | See §4.3 | |
| Exchange | `loadout export [--format text\|archo\|modsconfig\|rimsort\|rml\|rimcrow\|rimcrow-xml] [--out <file>]` | Export the current plan. Out never overwrites an existing file. |
| | `loadout import <file\|share-code> [--new [<name>]]` | Replace the current plan from a text list, ModsConfig, save, `.rml`, RimSort list, RimCrow share code, or Workshop IDs. This is undoable and writes no game files (04 §9). New creates a new plan (04 §9.1), defaulting to the list's own name or filename. |
| | `collection <URL\|ID> [--import]` | Read a Workshop collection online and compare it with installed Mods. Import adds the installed subset to the current plan in collection order, as an undoable edit. |
| Locked versions | `pin <instance> [--name <folder>] [--git] [--dry-run]` | Create a locked version by copying it to the lock library (default `<folder>_local`), writing `.rimarcho/origin.json`, optionally creating a Git baseline, and swapping the copy into the current plan only (04 §7). `--update` updates the locked version; `--move-to-library` moves the old `Mods/` copy into the library. `copy-local` is an alias. |
| | `unpin <reference> [--dry-run]` | Unlock by restoring and using the Workshop copy, including RimCrow-style locked versions. |
| Hidden `Mods` | `unhide <reference> [--dry-run]` / `trash <reference> [--dry-run]` | Restore `About.xml.disabled` to `About.xml`, or recycle own copies in `Mods`, the lock library, or SteamCMD, and hidden or unsubscribed Workshop folders (08 §6, §7). |
| Workshop | `workshop unsubscribe <references…> [--dry-run]` / `workshop subscribe <references…> [--dry-run]` / `workshop resubscribe <references…> [--dry-run]` / `workshop state <references…>` | Unsubscribe through the Steam client (Steam deletes the folder), resubscribe, or read the status. If Steam is unavailable, report an error and give the Workshop page (08 §10). |
| Packs | `pack build <name> [--dry-run]` / `pack stop <name>` | Build the current plan into a pack, or stop a running build and remove its partially copied files (08 §5.2). |
| | `pack list` / `pack launch <name> [--dry-run]` / `pack unlink [--dry-run]` | List or launch packs, or remove only the `!ArchoPack_` links in Mods (08 §5.3; the fix for `pack.links-left`). |
| | `pack export <name> <file> [--list]` / `pack import <file> [--as-plan]` / `pack remove <name> [--dry-run] [--unfinished]` | Export a `.archopack` (zip), or only its list as `.archolist.json` (04 §9.3). Import extracts the pack or compares the list; as-plan also loads the list into the current plan. Remove recycles the pack; unfinished deletes an interrupted build (08 §5.4). |
| Rules | `rule add after\|before\|incompatible\|attach <A> <B> [--note <text>]` / `rule add top\|bottom <A> [--note]` / `rule add ignore <constraint> [--note]` | Manage user rules (04 §6). Ignore targets the constraint references printed by sort or problems. |
| | `rule edit <r:n> <kind> <A> [<B>] \| ignore <constraint> [--note]` | Replace a rule while keeping its r:n reference (04 §6.1). |
| | `rule list` / `rule remove <r:n>` | |
| Player data | `alias <reference> [<name>] [--clear]` / `note <reference> [<text>] [--clear]` | Set the display name or notes on the Lineage. |
| | `tag add\|remove <reference> <tags…> [--mod <reference>]…` / `tag list` | Manage tags. Repeated mod arguments apply the change to several Mods at once. |
| | `import rimcrow --db <path> [--rules <path>] [--dry-run] [--include-disabled-rules] [--all]` | Perform a one-time import of RimCrow aliases, notes, groups as tags, and user rules. It can be repeated (04 §8). |
| Identity | `identity [list] [--all]` / `identity why <q:n>` / `identity resolve <q:n> same\|copy\|unrelated` | With no subcommand, list identity questions. Listing, inspecting, and answering all include a trimmed comparison (02 §5.4). |
| | `lineage split <reference>` / `lineage merge <reference> <reference>` / `gc [--dry-run]` | Split or merge Lineages, or collect expired tombstones and unused snapshots (02 §5.5). |
| Community | `data update [<datasets…>] [--dry-run]` / `data status` | Manually download rules, steamdb, and replacements, or read their sources and download and check times (04 §6). Interactive interfaces also update automatically (06 §2.16). |
| Maintenance | `db status` / `db backup` / `db vacuum [--dry-run]` | Show the database path, size, and backups; write a consistent copy under data/backups; or compact the database (02 §7.2). |
| | `cache status` / `cache clear <category>` | Show cache sizes or clear previews, workshop, images, source, or logs. The cache is rebuilt on demand. |
| Settings | `config get [<key>]` / `config set <key> <value>` | Below |
| Help | `guide [<topic>]` | Read the agent help (§5.3). With no topic, list the topics. Choose diagnose, errors, plans, or words for a workflow, or a tool name for its full reference. Output is identical to MCP; an unknown topic is a usage error. |
| Archo Monitor | monitor / `monitor install [--dry-run]` | Show the bundled and installed versions, source, and current-plan position; install or update Archo Monitor and add it by default (08 §9, 04 §1.5). |
| Update | `update [--check]` | First check GitHub for a new version, recording it like an automatic check (06 §2.16.1 A15). Check-only shows the version, date, download size, files to replace, other RimArcho instances to wait for, and release notes, without downloading. Otherwise, download, verify SHA-256, wait for other processes in the same program folder to close, replace the files, and verify that the new program runs. The progress says which process is being waited for; `Ctrl+C` stops without changes. Roll back if verification fails. Do not restart; ask the player to reopen. The data directory is unchanged (07 §5.1). Development builds do not check. |
| Uninstall | `uninstall [--dry-run] [--yes] [--keep-locked [--locked-to <environment>]] [--remove-packs] [--packs-to <directory>] [--remove-saves] [--saves-to <directory>] [--remove-games] [--restore-modsconfig-backup] [--all]` | Remove RimArcho artifacts and reverse its changes (01 §4.1). Without yes, only preview, as with dry-run; giving both is a usage error. Flags select the nondefault choice: keep-locked moves copies into the game's `Mods` (the default is recycling); `--remove-*` recycles content (the default is retention); `--*-to` selects a retention destination; and restore-modsconfig-backup restores the backup instead of cleaning the current list. Refuse while a game or another RimArcho is running. |
| Saves (1.1) | `saves list [--colony <name>] [--mod <reference>] [--attention] [--all]` | Read-only. Colonies and their saves (10 §4.1), newest first, each save with its reference, time, in-game day (`day N` from the cached full read, 10 §1.7; `meta only` for a save not read in full yet, with a note counting them and naming `archo saves index`; `malformed` when its read stopped early; `saves list` itself reads no save in full), matched list with its rule (`plan:<name> (launch\|same-mods)`, `pack <name>`, `no list`) and verdict (10 §4.3). The global `--env` keeps one game's saves; without it every game's and pack's are listed. `--mod` keeps saves whose Mod list has that Mod, installed or not (a Mod reference, `pkg:`, a bare packageId or `ws:`); what removing it costs (10 §4.6) is not implemented. `--attention` keeps saves with warnings or errors. Five saves per colony and about 34 lines by default; `--colony` or `--all` lifts the per-colony limit. Code: `internal/cmds/saves.go`. |
| | `saves show <save> [--mods] [--play] [--all]` | Read-only (writes only the full-read cache). Reads the save in full first when it has no valid cache row (10 §1.7: inline, tens of ms per 10 MB), so the load check and the colony see the read (a save read now joins its world's colony, 10 §4.1). Where it belongs (colony, game version and environment, list with the rule that matched, file, time and size), `read:` (`in full`; `stopped at line N, byte B: <why>` for malformed XML, the facts shown being those before that point; `not yet: the file is being written`, with no verdict), the playthrough `play:` (player faction, in-game day and date, real play time; storyteller, difficulty and permadeath; colonists alive, slaves and dead with animals, mechanoids and prisoners; wealth and the statsRecord numbers; `since <save> <sv:ref> (day N):` the changes since the colony's previous save when its read is cached, else that it is not read yet with `archo saves index`; the 3 newest letters; 10 §4.3, §4.5), can it load (each item with its consequence and its `fix:` command, then the `quick fix:`), and the Mod list differences (`--mods`: the whole list in save order). `--play`: the playthrough in full (the world and scenario, research, every colonist with where they are, `dead`, `slave`, and `new` / `died` against the previous save; the letters newest first with their LetterDef), the load check shrunk to its verdict and item count, and the Mod list only with `--mods`. Without `--all` the call keeps to 40 lines (§3.1 rule 5): 10 colonists and 6 letters with `--play`, then the load check's items and the Mod rows share what is left (at least 5 Mod rows, 15 with `--mods`), each ending `… N more (archo saves show <ref> … --all)`. Code: `savesShow`, `internal/cmds/saves_play.go`. |
| | `saves index [<saves…>] [--only] [--progress]` | Full read of new and changed saves into the cache (10 §1.7), the named saves first, then the rest newest first (`--only`: just the named ones); writes only the cache. Without saves, also removes the cached reads of saves that are gone. `--progress` prints `progress: saves <n>/<total> <sv:ref> <colony>/<save>` per save on stderr. The GUI runs it as a child process (`app.ChildSavesIndex`). |
| | `saves fix <save> [--mod <packageId>…] [--disable-added] [--order] [--launch] [--dry-run]` | The save's Mod fixes as one undoable plan edit (`app.FixSave`: `--mod` only those Mods; `--disable-added` and `--order` the information rows), then the Workshop subscriptions through `subscribe workshop`'s operation in the save's environment, then with `--launch` the plan is launched through `launch` and becomes current (10 §4.4). `--launch` waits as 10 §4.4 says, suggesting `archo scan` and the same fix again. A subscription failure keeps the edit and exits 1. |
| | `saves rename <save> <new name>` / `saves copy <save>` | Refused while the game runs (rename only) or when the name exists (10 §4.4). The output gives the new file and its reference. |
| | `saves backup <saves…> [--yes]` / `saves trash <saves…> [--yes]` | Without `--yes`, only list the files and sizes and the command with `--yes`; player confirmation required (10 §4.4). A save that fails is an `error:` line and exit 1. |
| | `unplayed [--group never\|partial\|functional] [--mod <reference>]` | Not implemented. Read-only (10 §3). |

**Log phases**: when explicit markers are present (03 §6.7.3–4), log cluster lines, `problems --code log` findings, and MCP `analyze_log` and `list_problems` include short phase names and counts, such as `[loading x3, save x2, play x4]`. Counts whose phase cannot be recovered are marked unknown. The log phases section lists boundaries in file order, with any recorded save name and time; cluster details also show the phase counts for that cluster. Follow text uses the same annotations, while JSON adds an optional phases array containing the phases and their counts. Without markers, add no phase sections, labels, or JSON fields.

**Analysis parameter**: `--game-version X.Y` on analyze, problems, def, patches, harmony, why, and log analyzes for another major/minor game version. The format must be like `1.6`; restore the version after the command. The corresponding MCP tools accept `game_version`. An explicit log path or version does not bypass the environment ownership check (03 §6.4).

**Automatic check status**: status, `data status`, and the corresponding MCP output use one `automatic checks` line. It shows the switch state, pending Mod and tool update counts, and a login reminder for a configured LoversLab account. If nothing is pending, show `nothing waiting (last MM-DD HH:MM)`; if no check has run, show `not checked yet`. Omit inapplicable accounts and zero counts. Do not use the network or expose cookies. deps separately lists new managed-tool versions and check times; `data status` lists each dataset's check time. See 06 §2.16 for authorization and notifications, and §2.16.1 for defaults and intervals.

**Settings keys** (`config set`): `ui.lang <auto|en|bundled-locale>` (06 §2.14); `ui.icons compat|nerd` (06 §9.3); `ui.mouse on|off` (disable terminal mouse reporting to allow text selection); `workshop.fetch on|off` (fetch Workshop descriptions over the network); `workshop.images on|off` (download images in descriptions); `updates.auto on|off` and `updates.interval normal|frequent|relaxed` (06 §2.16); `ui.gpu on|off` (window GPU acceleration, applied when the next window opens, 06 §2.14); tags.pinned `<comma-separated-tags>`; `monitor.default on|off` (automatically add Archo Monitor to plans, 04 §1.5); and monitor.dir `<absolute-path>` (the Monitor directory, 08 §9). Store settings in the database meta table under cfg:`<key>`. `config.toml` contains only `tombstone_days` (07 §3).

### 4.3 Plan Management

An environment may have several plans (04 §1). The commands are:

| Command | Effect |
| --- | --- |
| `plan list` | List all shared plans (04 §1.7), with the name, enabled count, last played date, and whether each is current in the environment. |
| `plan new <name> [--from current\|v<n>\|<file>]` | Create a plan from the current plan, one of its versions, or a list file in a `loadout import` format. |
| `plan use <name>` | Switch the current plan without writing game files. |
| `plan rename <old-name> <new-name>` | Rename a plan. |
| `plan remove <name> --yes` | Delete a plan and its history. The last plan cannot be deleted. |

- Keep `loadout log/diff/checkout/export/import` for the current plan. Import with `--new` creates a new plan; without it, import replaces the current plan in one undoable edit.
- Other commands act on the current plan. There is no `--plan` parameter yet.
- The three choices for an outside conflict (04 §4.3) are to view the diff between `v<n>` and workspace, load it with `loadout checkout v<n>`, or ignore it with `loadout ignore-external`. Ignoring clears the unimported-external flag but retains the version in history.
- Remove `loadout tag`, which named versions to use as named lists. Named lists are plans.
- Remove `launch --force`. Launch always writes to the game first; there is no longer a launch without writing.

## 5. MCP (`archo mcp`)

### 5.1 Transport and Lifecycle

- Only stdio is supported (`StdioTransport`); no network ports are opened. stdout carries only protocol messages.
- **Tool groups.** `archo mcp` serves the core group (§5.2): the 29 tools for diagnosis (Mod settings and saves included), plan edits, saving, launching, test runs, locking, rules and ignoring problems. `archo mcp --tools all` (or `--tools=all`) serves all 45, adding the player's management tools (§1.2). `--tools core` is the default. Another value, an unknown flag or a positional argument is a usage error (exit 2) before the data folder is opened; `archo mcp --help` shows the flag. A tool outside the served group is an unknown tool to the client. The group of each tool is part of its row in `Tools()`; `Tools()`, `Catalog()` and `ArgvFor()` still cover every tool for harnesses (`spikes/agenteval`), and `ToolDoc.Group` names its group. Measured on synthetic data (2026-11-04): core `tools/list` is 30,158 bytes, all is 48,497 bytes, close to the 48 KB limit (§5.3).
- The server is named rimarcho and uses the release version. It sends the `Instructions` from §5.3 during initialization.
- The server is not resident: the process exits when the harness closes the session.
- The server starts in the current environment. A tool's env parameter switches environments for that call and restores the previous environment afterward.
- Calls within a process run serially and share one App.
- Each tool builds a CLI argv and returns the text from `cmds.ExecuteContext` unchanged, run with `cmds.ForMCP` so that a reference matching nothing names `find_mods` (§3.3). Exit codes 1 and 2 set `isError`. Exit code 3, an ambiguous reference, is a normal result because the text lists the candidates.
- Calls that require analysis wait according to 06 §2.16.1 W01; the CLI has no time limit. If the analysis subprocess exceeds the wait bound, return declaration-level findings with still computing in the background. The subprocess continues, and the next call gets its results. Meanwhile, `get_def`, `get_patches`, and `get_harmony` use the previous, stale analysis if available.
- A `read_source` call with search has a default budget of 10 seconds. It reports how many assemblies remain to be decompiled and says still computing; run again.
- **Progress.** When a tool call carries a progress token, the server sends `notifications/progress` every 10 seconds until the result. Each one has a count that rises by 1 and a message: the command's latest progress line (`test_run`: preparing test data, waiting for the game to load, analysing the log, comparing with the player's log), or running, then the elapsed seconds. Nothing is sent after the result, and a call without a token gets none. Clients that restart their request timeout on progress therefore wait for `test_run` instead of cancelling it. Pi 1.4 asks for progress on every call and cancels after 60 s without any. A cancelled `test_run` stops the game (08 §12). A client whose timeout ignores progress still needs a limit above the run's length.

### 5.2 Tool Table

For link health, the read-only `check_links` maps to `archo links check [--all] --json`. The destructive `repair_links` only previews when `dry_run=true`; execution requires explicit `confirm=true`. Links without ownership evidence additionally require their full paths in include. The CLI equivalents are `links repair [--all] --dry-run` / `--yes [--include <link-path>…]`, with JSON also supported. Without all, check the current game and its local source. Output includes the full paths, targets, reasons, evidence, and actions. A dangling link or failed check makes check exit with code 1. See [08 §3](08-launch-and-packs.md#3-link-deployment) for the action boundaries.

Annotations follow the MCP specification; the external harness decides whether to permit an action. Archo uses the tiers in 09 §2.2. `openWorldHint: true` applies to `get_description` (the public Steam API), `exchange_list` (Steam collections), maintenance (community downloads), `manage_subscriptions` (Git hosts, download pages, and Steam), and `manage_saves` (a fix's Workshop subscriptions: the Steam Web API and Steam). It is false for the other tools.

| Tool | Command | readOnlyHint | destructiveHint |
| --- | --- | --- | --- |
| `guide` | guide (topic); there is no env parameter because the content is the same across environments. | true | — |
| `status` | status | true | — |
| `find_mods` | Use find when query is present; otherwise use list (active and filter). | true | — |
| `get_mod` | mod | true | — |
| `get_mod_settings` | mod-settings (ref, all) | true | — |
| `get_description` | describe (source, refresh, and offline) | true | — |
| `get_def` | def (def, mod, all, and `game_version`) | true | — |
| `get_patches` | patches (mod, target, failed, all, and `game_version`) | true | — |
| `get_harmony` | harmony (mod, target, shared, all, and `game_version`) | true | — |
| `explain_relation` | why (a, b, and `game_version`) | true | — |
| `list_problems` | problems; ref maps to `problems --ref`, retaining all and `game_version`. Add `--min-severity warning` by default, unless severity, `min_severity`, all, or `include_muted` is given. `include_muted` maps to `--include-muted`. | true | — |
| `list_changes` | changes (ref, since, and all) | true | — |
| `analyze_log` | log (cluster, text, file, prev, all, since, and `game_version`); pattern=`<reference>` maps to pattern, where list means all. | true | — |
| `search_files` | `files search` (pattern, mod, mods, regex, `case_sensitive`, word, def, kind, languages, lang, `all_versions`, game, `game_version`, mode, page, limit, `per_file`, and all) | true | — |
| `read_source` | With search, use `source search`: in maps to `--in`, `scope: "active"` maps to `--active --yes`, and otherwise use `--implicated`. `game: true` adds `--game`; without search it selects `source game` (optionally with type). Otherwise use source (ref and type). The default dispatch budget is 10 seconds. Library paths and shared-copy attribution follow 03 §5.4. | true | — |
| `edit_loadout` | The action is enable, disable, move, swap, sort (`dry_run`), undo, redo, stage (`stage add`), unstage (`stage drop`), or restore (`stage restore`). Position parameters are at, before, after, up, and down. | false | false |
| `save_loadout` | save (`dry_run`) | false | true |
| `launch_game` | launch (`dry_run`) | false | true |
| `estimate_memory` | plan cost [`<list>`] shows the resident scope, loading budget, RAM and VRAM capacity comparisons, unknown values, and severity; see 06 §2.4 for the definitions. It writes only a disposable estimate cache. | true | false |
| `test_run` | `test run --json` (env, plan, map, and `timeout_seconds`) may take several minutes to launch the game; it sends progress every 10 seconds (§5.1). It isolates player saves, configuration, and logs, and links the plan's game-invisible Mods (pinned copies) into `Mods/` for the run only (`linked`; a Mod it cannot link is in `missing` with `reason`), 08 §12; `openWorldHint: false`. The report compares the run's clusters with the player's log (`before`, `player_log`, `fixed`) and the previous run (`delta`, `baseline`, `gone`), 08 §12. | false | false |
| `manage_plans` | `plan list/new/use/rename/remove --yes`, `loadout import --new` | false | true |
| `plan_history` | `loadout log`, diff, and history (`action=steps`) | true | — |
| `pin_mod` | pin (ref, name, git, update, `move_to_library`, reason, and `dry_run`); reason is stored in the copy's origin and shown in its copy notes (06 §2.9). | false | false |
| `unpin_mod` | unpin (ref and `dry_run`) | false | false |
| `set_mod_settings` | mod-settings set (ref; set as `name=value` pairs in name order; each reset; file; `dry_run`) | false | false |
| `set_player_data` | The action is `tag_add` or `tag_remove` (refs and tags), alias or note (text; an empty string clears it), or `copy_note` (the note for one copy, using note `<reference>` `<text>` --copy; an empty string clears it, 06 §2.9). | false | false |
| `manage_rules` | `rule add/list/remove` (action, kind, a, b, target, note, and rule) | false | false |
| `ignore_problem` | `problems ignore` or unignore (undo) | false | false |
| `mute_version_warnings` | `problems mute-version` or `unmute-version` (refs, `game_version`, and undo); see 03 §4.2 for the scope. | false | false |
| `scan` | scan (full) | false | false |
| `exchange_list` | `loadout export/import`, `collection [--import]` | false | false |
| `manage_packs` | `pack list/build/launch/export/import/remove/stop/unlink` | false | true |
| `resolve_identity` | `identity list/why/resolve` (`resolve_identity action=why question=q:<n>` is read-only), and `lineage split/merge` | false | true |
| `manage_hidden` | unhide and trash | false | true |
| `maintenance` | `data update`, `import rimcrow`, `db vacuum` (`action=vacuum`), and gc | false | true |
| `list_library` | library (search, include, exclude, sort, lineage, facets, and all) | true | — |
| `list_saves` | `saves list` (colony, mod, attention, all); env both selects the environment and keeps only its saves, like the CLI's global `--env` | true | — |
| `get_save` | `saves show` (save, mods, play, all); the first call for a save reads it in full into RimArcho's cache (10 §1.7) | true | — |
| `manage_saves` | The action is fix (`saves fix`: mods as `--mod`, `disable_added`, order, launch and `dry_run`), rename (name), copy, backup or trash; saves names the targets (fix, rename and copy take the first). Backup and trash only preview unless `confirm=true` (`--yes`) without `dry_run`; never confirm without the player's agreement. Rename and copy have no dry run: `dry_run=true` with them is an error. | false | true |
| `manage_library` | The action is `move_to_locked` (`library move-to-locked`, with `dry_run` and reason), `remove_from_plans` (library remove-from-plans), unsubscribe or subscribe (`workshop unsubscribe\|subscribe`, with `dry_run`), or `steam_state` (workshop state); refs identifies the targets. | false | true |
| `manage_subscriptions` | The action is list (the default, using subscribe list), add, check, update, or remove. Add uses URL or kind to choose `subscribe git <url> [--branch\|--tag\|--commit]`, `subscribe page <url> [--git] [--from]`, or `subscribe workshop <ID…> [--steamcmd]`. Check uses subscribe check with refs, or all subscriptions if refs is absent. Update uses subscribe update with refs or all, and from, and refuses while a game is running. Remove uses `subscribe remove`. Add, update, and remove accept `dry_run`. | false | true |
| `preview_uninstall` | `uninstall --dry-run` (`keep_locked`, `locked_to`, `remove_packs`, `packs_to`, `remove_saves`, `saves_to`, `remove_games`, `restore_modsconfig_backup`, and all); never pass --yes. | true | — |

**Groups.** Core (served by default): `guide`, `status`, `find_mods`, `get_mod`, `get_mod_settings`, `get_description`, `get_def`, `get_patches`, `get_harmony`, `explain_relation`, `list_problems`, `list_changes`, `analyze_log`, `search_files`, `read_source`, `edit_loadout`, `save_loadout`, `launch_game`, `test_run`, `pin_mod`, `unpin_mod`, `manage_plans`, `plan_history`, `manage_rules`, `ignore_problem`, `scan`, `list_library`, `list_saves` and `get_save`. Only with `--tools all`: `manage_subscriptions`, `manage_packs`, `manage_library`, `exchange_list`, `manage_textures`, `estimate_memory`, `maintenance`, `resolve_identity`, `manage_hidden`, `check_links`, `repair_links`, `mute_version_warnings`, `set_player_data`, `set_mod_settings`, `preview_uninstall`, and `manage_saves` (`TestToolGroups`).

The authoritative tools, groups and annotations are `Tools()` in `internal/surface/mcp/server.go`. `destructiveHint: true` applies to `save_loadout`, `launch_game`, `manage_plans`, `manage_packs`, `resolve_identity`, `manage_hidden`, `manage_library`, `manage_subscriptions`, `manage_textures`, `manage_saves`, `repair_links`, and maintenance. Add tools still missing from §1.2 in the same way.

**Description revisions** are complete. The descriptions before 2026-10-21 disagreed with the code; the old descriptions are retained here as a record:

| Tool | Old description | Old actual behavior | Current description |
| --- | --- | --- | --- |
| `save_loadout` | Refuses when ModsConfig.xml changed outside RimArcho. | **Did not refuse**: it backed up and overwrote the file directly. Outside changes were detected only on read, loaded as an external version, and marked as a conflict. | Save without launch: write the current plan into ModsConfig.xml, with a backup, atomic replacement, and the current game version. Sync `_Archo_` links and remove pack links. This changes the list the game will read next, so ask the player and preview the diff with `dry_run=true` first. Refuse unresolved outside conflicts (04 §4.3), offering commands to view the diff, load it, or ignore it. Still write while the game runs, explaining that it reads the list on the next launch. |
| `edit_loadout` | Change RimArcho's workspace Loadout … nothing reaches the game until `save_loadout` | It changed the environment's sole workspace. | Edit the current plan with immediate persistence and an undo for each step. Write no game files until save or launch. The checkout action was added; there is no plan parameter yet (§4.3). |
| `status` | The example said unsaved edits. | It showed the difference between the workspace and the latest version. | Say differences from the game list, and list the plans. |
| `copy_local` (removed in 1.0.1; now `pin_mod`) | The Loadout is not changed: call `edit_loadout` action=swap … | It did not change the plan. | Create a locked version as an independent copy and swap it into the current plan, leaving other plans unchanged. Workshop updates do not affect it. |

### 5.3 Progressive Disclosure: Instructions, Tool Descriptions and guide

On connection, the agent reads only `Instructions`, each tool's short description, and the parameter schemas. This is enough to choose the right tool. Details live in the read-only guide tool and are fetched as needed. It maps to the CLI archo guide [`<topic>`] and returns identical text (§1.1). See 00 §6 D23 for the rationale.

| Layer | Read when | Contents | Code |
| --- | --- | --- | --- |
| `Instructions` | Initialization | Explain what RimArcho knows and that output references (`#id`, `L#id`, `pkg:`, `ws:`, `def:Type/name`, `p:<hex>`, `log:E<n>`, `r:<n>`, and `pattern:<id>`) can be passed directly to other tools. Results are limited to 24 KB; narrow the scope with `mod=`, `target=`, a cluster, or a pattern instead of `all=true` (corpus G7). Start with status. For an error or crash, read guide topic=diagnose first and state the two key rules: a patcher or mentioned Mod is not the cause without further evidence; without a source, the cause is undetermined. Editing tools change only the current plan and do not touch the game until save_loadout or launch_game; use pin_mod before editing Mod files, then test_run, which tests the copy without saving. For destructive tools (`save_loadout`, `launch_game`, `manage_plans`, `manage_packs`, `resolve_identity`, `manage_hidden`, `manage_library`, `manage_subscriptions`, `manage_textures`, `manage_saves`, `repair_links`, and maintenance), show a preview if supported, then obtain the player's agreement before calling. Say in one sentence that the player's management tools (packs, subscriptions, library actions, textures, memory estimate, maintenance, identity, hidden Mods, links, player data, save actions, uninstall preview) are served only by `archo mcp --tools all`, and that otherwise the agent uses the archo CLI. End by explaining how to use guide. | `Instructions` in `internal/surface/mcp/desc.go` |
| Tool descriptions | `tools/list` | Use 1–3 sentences to explain what the tool does, when to use it or another tool, and what it writes. Explicitly state any requirement for `dry_run` and player approval. Do not include examples or explain each parameter; the schema owns those descriptions. | `desc*` in `desc.go` |
| guide topics | guide call | Provide workflows: diagnose (the full workflow below), errors (the `patternChecklist`, 03 §6.8; its save-loading kind names `get_save`), plans (plans and versions, inventory, subscriptions, textures, and pinning before file edits), and words (UI names when talking to the player, 06 §7.2). Provide a topic for each tool name with its full actions, parameter details, edge cases, one example call, and the first lines of example output. | `internal/cmds/guide.go`, `guide_tools.go`, `guide_patterns.go` |

- The reader is an agent. With no skills loaded, `Instructions`, tool descriptions, and on-demand guide topics must be sufficient to complete the former error-debugging and assembly-inspector workflows (I4). Archo uses the same content (09 §2.1).
- The context has a budget (`TestInitialContextBudget`, I13). For the default core group: `Instructions` ≤2000 characters, each description ≤320 characters, and the combined total ≤12000 characters. The `tools/list` JSON of all tools (`--tools all`), including the schema, is limited to ≤48 KB. Keep new descriptions as short as the existing ones and put details in the guide topic. If a budget increase is necessary, explain it in the commit. Keep parameter descriptions short too; env always uses environment name (default: current).
- With no topic, guide lists all topics. Its tool list puts the tools only `--tools all` serves on a separate line, only with archo mcp --tools all, so the text is the same on every surface and in either group (I1; `cmds.AllOnlyTools`, kept in step with `Tools()` by `TestToolGroups`). Their guide topics remain available to a core session. An unknown topic is a usage error (exit 2 and `isError`), with text listing the workflow topics. Return topic text verbatim, bypassing `ShellSafe` so that # references in example JSON are preserved.
- No tools have been merged. The aliases `copy_local` (old name of `pin_mod`) and `add_rule` (old name of `manage_rules`) were removed in 1.0.1 (§6); calling them is an unknown tool.
- Every MCP tool except guide itself needs a same-name guide topic containing Example: `<tool-name>` { (`TestToolsAndAnnotations`).

The tool schema's `game_version` and env must be **direct fields**. The SDK does not expand embedded structs when inferring the schema, so embedded fields are rejected as unknown properties. Tools without env (guide and `preview_uninstall`) run in the current environment.

**`guide topic=diagnose`** contains the following diagnosis workflow, originally written in English. Use the evidence verbatim rather than paraphrasing it:

1. **Start with the error.** If the user pasted a log, call `analyze_log text=…`; otherwise, call `analyze_log` for the current Player.log, then `analyze_log cluster=E<n>` for the relevant cluster. For each cluster, list the candidate Mods and their roles: source (its own code or data produced the error), patcher (it only has a Harmony patch on a frame), dependency (what it provides is missing or disabled), or mentioned (it appears in the message or a lower frame). Never call a patcher or mentioned Mod the cause without further evidence. Evidence belonging to the Mod itself keeps it as the source. A framework that only provides a type, or a Mod that only calls failing code, is not the cause. If the cluster names no source and no tool result identifies a Mod's own faulty files or code, say that the cause is undetermined.
2. **Confirm the identity and state with `get_mod`**: the exact Instance, whether it is enabled and its position, its path, and other copies in the same Lineage.
3. **Classify the error**:
   - For XML errors (a failed PatchOperation, missing cross-reference, duplicate or missing parent Def, or malformed XML), use `get_patches failed=true`, `get_def`, and `search_files`. The reasons include target-inactive, target-missing, target-later, wrong-version-route, stale-name, unguarded-optional, and node-absent; explain what each means for the player.
   - For a material or icon error (`_MainTex` or `ResolveIcon`), check XML first. In the named `Mods`, use search_files to find `uiIconColor` without `uiIconPath` or `texPath`.
   - For an assembly error (a C# exception, `Pawn.Tick`, `ThingWithComps.Tick`, or world tick), use `get_harmony` with target=<method> for each patched frame, then `read_source` with type=<patch class> or search=<pattern>. For a tick crash, check AllComps mutations in the cluster's candidates: `read_source search="AllComps.Remove(" in=[…]`, also checking `"AllComps.Add("`, `"InitializeComps"`, and `"PostExposeData"`.
   - For assembly loading errors (ReflectionTypeLoadException, TypeLoadException, or a static constructor failure), inspect the assembly named in the message or adjacent lines. `analyze_log` identifies which enabled Mod loaded that DLL.
   - For a mixed error, first confirm that the XML side loaded successfully with get_patches failed=true, then investigate the code.
   - For other errors (save loading, configuration checks, missing assets, or noise), use `guide topic=errors`.
4. Use `list_problems` for the static overview: dependencies, incompatibilities, load order, Def overrides, patches with no match, and Harmony conflicts. Use `explain_relation` to explain two specific `Mods`.
5. **Report** the failure phase, whether it is XML, assembly, or mixed, the exact Mod Instances and paths, the file and line or class and method, why it failed, whether each named Mod is a cause, dependency, or bystander, and the smallest fix.
6. If it worked before, use `list_changes` to find changed Mods and `plan_history action=diff` to find plan changes.

The following table gives the tool guide highlights. See `internal/cmds/guide_tools.go` for example calls and output:

| Tool | When / alternatives |
| --- | --- |
| `status` | Use it first in a session. For one Mod, use `get_mod`; for the findings themselves, use `list_problems`. |
| `find_mods` | Search by name, package ID, author, Workshop ID, or folder. A package ID may identify several Instances; the output lists a reference for every candidate. Use `get_mod` next. |
| `get_mod` | Get all facts for one Instance. An ambiguous reference returns candidates rather than choosing one arbitrarily. |
| `get_mod_settings` | Read what a Mod's settings files and HugsLib packs store, its settings window and HugsLib titles and defaults as Archo Monitor last saw them, or which Mods of the current list have settings. |
| `set_mod_settings` | Change simple settings values (on/off, numbers, one line of text, a HugsLib choice) or reset them, with the game closed; the file is backed up first. |
| `get_def` | Use it for No ThingDef named X, duplicate Defs, or which Mod wins. A `log:` line says when the newest log reports the effective definition's file failed to load (03 §6.3). |
| `get_patches` | Use it for a logged Patch operation failed; `failed=true` gives the reason. |
| `get_harmony` | Use it for PREFIX/POSTFIX stack lines or `Mods` touching `Pawn.Tick`; next, use `read_source` on the patch class. A bool-returning prefix may skip the original method. Patches applied in code through `Harmony.Patch` are marked as not statically resolvable. |
| `explain_relation` | Use it when a finding names two `Mods` or the user asks why A must come after B. |
| `list_problems` | Get static findings. `ref=<p:…>` shows evidence and repair commands; code accepts the prefixes dep, patch, and harmony. After plan edits, the Def, Patch, and Harmony layers rebuild automatically in a few seconds. |
| `list_changes` | Find changes when the user says it worked yesterday. |
| `analyze_log` | Always use text for pasted errors. Cluster categories are exception, harmony-stack, xml-load, patch-failed, xref-missing, material, and other. Player.log is at `Logs/Player.log` in 1.7 and later, and at the user-data root in earlier versions. While the game runs, pass the last cursor back as since to get only new clusters or increased counts, combining Player.log and ArchoMonitor.log. An expired cursor means the game restarted; the old run is then available with `prev=true`. Known cluster kinds include a pattern:<id> reference as community context, not evidence about this list; `pattern=<id>` expands it. |
| `search_files` | Find XML evidence: defName, XPath targets, `uiIconColor`, or class names in XML. Use `read_source` for C#. |
| `read_source` | Inspect a patch class or the type containing a frame. Prefer short, specific search patterns. Use `scope="active"` only when the player requests it, because it can cover hundreds of assemblies and take many minutes. If ilspycmd is missing, the text explains installation; it is never installed automatically. |
| `edit_loadout` | Edit the plan, with an undo for each step. |
| `save_loadout` | §5.2 |
| `pin_mod` | Copy a Workshop Mod before editing it because Steam overwrites Workshop files. Do not copy links outside the folder or the upstream `.git`, and never overwrite an existing folder. |
| `get_description` | Learn what a Mod does before recommending it. Fetch Workshop text on demand and cache it until the item updates. The player can disable network access with `workshop.fetch`. |
| `set_player_data` | Set tags (used by `list --filter tag:<t>` and TUI `tag:` search), aliases, and notes on the Lineage, shared by all copies. `copy_note` belongs to one copy and explains why it is retained. It does not touch the game. |
| `manage_subscriptions` | Use it to install a Mod from Git, a download page, or SteamCMD, or to check subscription updates. Check before update; preview add, update, and remove with dry-run first. The player downloads page files and supplies from; the agent does not log in to LoversLab. Use `manage_library` for client Workshop items. Pin the old version before updating if it should be retained. |
| `manage_rules` | Ignore wrong-direction About.xml or community constraints to break `order.cycle`. Rules affect sorting and order findings but do not themselves reorder the plan. |

## 6. Stability

- English output keywords, such as `depends:`, `evidence:`, `next:`, and finding codes, are part of the interface. Record changes in CHANGELOG (07 §5).
- Reference formats are stable. Instance IDs are independent of paths and package IDs (02); p: references are stable across analyses. A log finding key retains only the last three segments of the absolute path, so its references and ignore records remain valid after the library directory moves. Pattern references `pattern:g:<id>` and `pattern:m:<packageId>:<hash>` are stable within a dataset version. Generic IDs persist across versions; a Mod-specific hash comes from its template and remains unchanged while the template is unchanged.
- Append new fields without changing the meaning of existing lines.
- When renaming an MCP tool, retain the old name as an alias for at least one minor version. The 0.9.0 renames `copy_local` → `pin_mod` and `add_rule` → `manage_rules` kept their aliases through the 1.0.0 release (2026-11-02, which contains 0.9.0); 1.0.1 removes both aliases.
- Moving a tool between the core and all groups (§5.1) changes what a default session lists: record it in CHANGELOG like a rename.

## 7. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| I1 | Execute the same command through the CLI and MCP. | The output is byte-for-byte identical (`TestCLIParity`: 19 calls through an in-memory transport; `TestSaveToolsCLIParity`: the save tools, previews included). | Pass |
| I2 | Take any reference from the output. | It can be passed directly as an argument to another command (`TestRefsRoundTrip`: `#id`, `pkg:`, p:, `log:E#`, and `def:`). | Pass |
| I3 | A pkg reference matches multiple Instances. | List all candidates without choosing one arbitrarily (`TestAmbiguousPackage`). | Pass |
| I4 | Connect Claude Code or Codex to MCP without loading skills. | Diagnose a patch failure and investigate a `Pawn.Tick` exception. Since 2026-10-30, the agent must read guide topic=diagnose itself. | The scripted client self-check in `spikes/mcpcheck` passes. A real agent session is pending, especially to check guide discovery after the context reduction. |
| I5 | Read the default output. | A call returns at most 40 lines and gives an expansion command when truncated (`TestCLIParity` and `TestTruncation`). | Pass |
| I6 | Inspect the annotations. | `Tools()` marks read-only tools with `readOnlyHint: true`; the 12 destructive tools are listed in §5.2. Every tool guide has an example (`TestToolsAndAnnotations`). Test writes only in synthetic environments (`TestWriteTools`), test env (`TestEnvParameter`), and return still computing on analysis timeout (`TestStillComputing`). | Pass |
| I7 | Find a tool for each player action in §1.2. | Each action has a CLI command and MCP tool, with annotations matching its file writes. | Partial: 1.0 tools are complete; subscriptions were added on 2026-10-30, the save tools on 2026-11-04. `list_unplayed` is missing. |
| I8 | Call `save_loadout` with an unresolved outside conflict. | Refuse and offer diff, load, and ignore commands. Leave the file unchanged. | Pass |
| I9 | Call launch or save while a game is running. | Launch refuses and names the environment whose game is running. Save still writes and explains that the game will read the list on its next launch. | Pass |
| I10 | Execute `plan new`, use, rename, remove, and import-new through the CLI and MCP. | The output is identical. Use writes no game files, and the final plan cannot be removed. | Pass |
| I11 | Paste the output commands (fix, next, `… N more (…)`, and error hints) into cmd, PowerShell, or bash. | All arguments survive and references resolve. `TestPrintedCommandsSurviveShells` parses each with PowerShell 5.1 / 7, verifies no comments and the expected words, then executes it; see `TestShellSafe`. | Synthetic tests pass; real VM verification is pending. |
| I12 | Type a # reference in PowerShell or bash, where it becomes a comment. | If the missing argument is a Mod reference, the usage error explains why (`TestMissingRefNamesTheComment`, run on host PowerShell 5.1 / 7). | Synthetic tests pass; real VM verification is pending. |
| I13 | Read the initial agent context (§5.3). | `Instructions` ≤2000 characters, each description ≤320, combined ≤12000; `tools/list` JSON ≤48 KB. Every listed topic can be fetched; an unknown topic sets `isError` (`TestInitialContextBudget` and `TestGuideTopics`). | Pass |
