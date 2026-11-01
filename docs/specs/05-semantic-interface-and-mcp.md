# 05 · Semantic Interface and MCP

> Status: CLI/MCP implemented (41 MCP tools, including aliases `add_rule`/`copy_local` and guide). 1.1 save tools and a few reference forms remain missing, marked individually below.
> Basis: 00, 00a; command semantics in 02, 03, 04, 08; 09 (Archo uses the same tools).

## 1. One Command and Tool Layer

### 1.1 Tool Layer

All RimArcho operations share a command table in `internal/cmds` and operation functions in `internal/app`. Five surfaces use them without separate logic:

```
Tool layer: command table (internal/cmds: arguments, execution, outline text)
            └─ operation functions (internal/app, via internal/view when needed)
  ├─ CLI: archo <command> [arguments]        → print text
  ├─ MCP: tool call                         → same CLI argv, same text
  ├─ Archo (09): same MCP tool table         → same text
  ├─ TUI: keys / archo in command panel      → update UI or show same text
  └─ GUI: buttons, drag and drop            → typed view/app functions (same as TUI)
```

- CLI/MCP output for the same command is **identical word for word**. Tools translate arguments to CLI argv and return `cmds.Execute` text without extra formatting. Golden tests/`TestCLIParity` cover both.
- Archo has no private tools; its table is §5.2 (09 §2.1).
- TUI/GUI mutations call typed `internal/app` / `internal/view` functions, never construct/reparse command text. `cmds.Execute` is for inherently textual cases: sort/save/launch previews, inspect actions, full-screen Def/Patch/Harmony pagination, player-entered archo commands (06 §1). Both paths share operation functions, so players/agents receive the same facts.
- Language: CLI/MCP use fixed English vocabulary for stable agent parsing; user content (Mod names/aliases/notes) stays original. TUI/GUI localize Chinese/English with English lookup keys (`view.Text` T/Tf); command panel output stays English. Precedence: `ARCHO_LANG` → `ui.lang` (`auto|en|zh`) → system UI language (Windows `GetUserPreferredUILanguages`, otherwise `LANG`).

### 1.2 A Tool for Every Player Action

Account sessions are player-only exceptions: no MCP tools for login, forgetting, credential reads/writes/import, login-window control, site notice acceptance, or general authenticated HTTP proxy. Account output only says player login required, without display name, cookies, tokens, temporary URLs, raw authentication logs. Settings account queries expose coarse status only. Fixed downloads using local sessions let agents reference file pages/results, not authenticated requests. Storage/isolation: 07 §3.1.

AI assistants are a second user class (00 §3.2), so **every interface action needs a corresponding command/tool**, labeled read-only or mutation. Without a tool, Archo/external agents can only suggest player button presses.

Add CLI/MCP alongside new player actions; tool writes share operation functions/undo history (04 §3).

Player actions/tools (marked **not implemented** where missing):

| Player action | Current state | Target |
| --- | --- | --- |
| Launch plan (08 §2) | Complete | `launch_game`, `destructiveHint: true` |
| List/create/switch/rename/delete plans (04 §1) | Complete | `manage_plans`, `destructiveHint: true` (deletes history, irreversible) |
| Write to game without launch | `save_loadout` description now save without launching | Complete |
| History/diff/checkout (04 §3) | Complete | Read-only `plan_history` log/diff/steps; checkout via `edit_loadout` |
| Outside changes: diff/load/ignore (04 §4.3) | Complete | `loadout diff v<n> workspace`, `loadout checkout v<n>`, `loadout ignore-external`; `plan_history action=diff`, `edit_loadout action=checkout` / `ignore_external` |
| Lock/unlock/update version (04 §7) | Complete | `pin_mod` creates/swaps locked copy in current plan; `copy_local` alias; `unpin_mod`; `pin --update <instance>` included in `pin_mod` |
| Build/list/launch/export/import/remove/stop packs, remove leftover links (08 §5) | Complete | `manage_packs`, `destructiveHint: true` |
| Ignore/unignore findings (03 §4.2) | Complete | `ignore_problem` |
| Mute/restore Mod version reminders (03 §4.2) | Complete | `mute_version_warnings`, `undo=true` restores; ask player before batch mute |
| Answer identity questions, split/merge Lineages (02 §5.4, §6) | Complete | `resolve_identity`, `destructiveHint: true` |
| Scan | Complete | scan |
| Inventory list/filter/sort; move to locked/remove from all plans (06 §2.9) | Complete | CLI library/move-to-locked/remove-from-plans; read-only `list_library`, mutation `manage_library` (`destructiveHint: true`, moves `Mods` folders). Other actions: `edit_loadout`, `pin_mod`/`unpin_mod`, `manage_hidden` |
| Subscriptions list/add Git/page/Workshop (SteamCMD if needed)/check/update/remove (02 §3.4, 06 §2.9) | Complete (2026-10-30) | CLI `subscribe list/git/page/workshop/check/update/remove`; `manage_subscriptions` (`destructiveHint: true`, replaces/recycles files; `openWorldHint: true`). Detect origin from url like window paste field (`subscribe.Detect`, shared test table). No LoversLab session tools: agent reads public pages, player downloads files and supplies from. Steam manages client Workshop updates; unsubscribe/resubscribe via `manage_library` |
| List import/export/collection comparison (04 §9) | Complete | `exchange_list`, import one undoable edit |
| List/delete user rules | Complete | `manage_rules`, `add_rule` alias |
| Restore/recycle hidden `Mods` (08 §6, §7) | Complete | `manage_hidden`, `destructiveHint: true` |
| Environments/settings | Complete (status show=envs/settings/data/db/cache) | Read-only in status; changes player-only (broad RimArcho configuration impact) |
| Community update/RimCrow import/database maintenance/vacuum/gc | Complete | maintenance, `destructiveHint: true` |
| Update RimArcho (07 §5.1) | Complete (2026-10-31) | No tool: replaces program, player decision like uninstall; status `automatic checks` line reports new version (RimArcho 0.9.1 available) |
| Uninstall (01 §4.1) | Complete | Read-only `preview_uninstall` only (`uninstall --dry-run`, CLI parameters), no execution; rationale 01 §4.1 |
| Saves list/inspect/unplayed/backup/recycle (10 §`1.6`, §4) | None (1.1) | CLI `saves list/show/backup/trash`, unplayed; read-only `list_saves`/`get_save`/`list_unplayed`; `manage_saves` `destructiveHint: true` |

Old exclusion of launch/`import rimcrow`/gc/`identity resolve` from MCP is canceled: expose tools with write/irreversibility annotations, external harness decides permission; Archo requires player confirmation (09 §2.2).

## 2. References

Every object has stable references, usable in any output and directly as other-command arguments (I2). Complete set:

| Reference | Target | Example | Notes |
| --- | --- | --- | --- |
| `#<id>` | Instance | `#a3f9` | Shortest environment-unique prefix, ≥4, path/package-independent (02) |
| `L#<id>` | Lineage | `L#7c2e` | Separate prefix calculation; single-Instance commands list candidates for multi-Instance Lineages |
| `pkg:<packageId>` | Matching Instance | `pkg:ceteam.combatextended` | Case-insensitive, list all ambiguous candidates (I3) |
| `ws:``<Workshop-ID>` | Matching Instance | `ws:2890901044` | May match Steam/SteamCMD/local copies |
| `def:<Type>/<defName>` | Def | `def:ThingDef/Gun_Revolver` | def also accepts `Type/defName`, `Type@Name` (abstract parent), bare defName |
| `p:<6-hex-digits>` | Finding | `p:3f2a1c` | First 3 bytes sha1(code \| key \| subject Lineage), stable across analyses |
| `log:E<n>` | Current log cluster n | `log:E3` | `--cluster` accepts E3 too |
| `r:<n>` | User rule | `r:1` | |
| decl:<`#id`>:`<field>`:`<package-id>` | Declared order constraint | `decl:#iqmu:loadafter:tester.y` | `rule add ignore` target (04 §6) |
| `q:<n>` | Identity question | `q:12` | Bare number accepted |
| `src:<label>` | Source | `src:workshop` | `env rebind`/`env set --remove-source` |
| `scan:<n>` | Scan | `scan:17` | `changes --since scan:<n>` |
| `env:<name>` | Environment | `env:main` | Output only; arguments/`--env` use bare name |
| `v<n>` | Plan version | v3 | `loadout diff/checkout`; workspace means current edits |
| `pattern:<id>` | Error pattern (03 §6.8) | `pattern:g:xml.type-missing` | Environment-independent; `pattern:m:<packageId>:<hash>` is Mod-specific known error |

- Bare ID prefixes accepted (`archo mod a3f9`): PowerShell/bash treat # words as comments, dropping `archo mod #a3f9` argument. Use double quotes for # (`"#a3f9"`); cmd passes single quotes literally. Mid-word # (`L#7c2e`/`decl:#iqmu:…`) is not comment. Printed commands omit # (§3.1 rule 11). TUI :, GUI, MCP bypass shell, either form works.
- `harmony:<Type.Method>` (e.g. `harmony:Verse.Pawn.Tick`) appears only in `harmony --target` headings; arguments use bare method name; prefixed input not implemented.
- patch:`#<id>`/`<file>`#`<ordinal>` single-operation references not implemented; use patches `--mod` plus file:line.
- plan names identify plans, unique case-insensitively within environment.

## 3. Text Format

### 3.1 Rules

1. Referenced outline text, not JSON, except `log --follow --json` (one new cluster/JSON line for scripts, 03 §6.6).
2. First line subject: `<reference>` `<name>`, optional short status.
3. Two-space hierarchy; each line `key: value` or list item.
4. Always reference entities (§2) for follow-up calls.
5. Default max 30 items, ≤40 lines/call (I5). Overflow ends with count/expand command: … 37 more (archo problems `--all`); without expansion command, … 37 more.
6. Evidence: `evidence:` `<source-type>` `<reference>` `<explanation>`.
7. Optional `next:` section with commands.
8. No ANSI, even terminal; identical MCP/redirect/terminal output.
9. Column padding max 40 characters; long names do not push whole columns. East Asian ambiguous-width –/…/· count 1 (07 §8).
10. Local timezone (UTC in tests).
11. Printed archo commands use Instance ID without #: `fix: archo enable nyh3`, … 3 more (archo problems --subject 5sed), `error: #z3bw is not enabled (archo enable z3bw)`. Copyable in cmd/PowerShell/bash without quotes (§2). Command extends from archo to line end or enclosing ), ;, two spaces. Other references keep `#nyh3`; mid-word # and double-quoted text unchanged. `render.ShellSafe` in `Doc.String`/error output handles centrally; rendering code still uses `Index.Ref`. Printed text only; MCP still accepts `#nyh3`.

### 3.2 Detail Levels

- Default summary. `--all` removes truncation (also collapse/default-hidden categories in findings); `--detail` expands original evidence, XML/log snippets.
- `problems --ref` and problems show share focused details; MCP `list_problems ref=` identical. Missing-parent provider lookup on demand (03 §4.3), bounded/cancelable; show impact, enable commands, subscription candidates first (no empty `fixes:`); unresolved templates get `providers:` (incomplete or no installed definer), then affected content with file:line; separate evidence only for other notes. Group unignored same-Mod items, check ignored individually. Default section limits, `--ref … --all` expands fixes/ignore commands; provider/log conditions 03 §4.3. Inactive targets/known errors also lead with impact/actions. Details omit Mod card identity/file/Workshop/history lists; mod stays complete card.
- Default finding display (collapse/hide/stale counts): 03 §4.5; the line `hidden: N def.override, M patch.contested` explains hidden findings.

### 3.3 Exit Codes and Errors

| Code | Meaning | Destination |
| --- | --- | --- |
| 0 | Success | stdout |
| 1 | Error, starts `error:` | stderr |
| 2 | Usage `error:` unknown command/argument, argument count, with `usage:` | stderr |
| 3 | Ambiguous reference, candidate list | stdout |

Typing `archo enable #nyh3` in PowerShell/bash drops comment, leaving `archo enable` (§2). Missing Mod references (insufficient arguments with `<ref>`, `<ref…>`, `<A>`, `<mod>`; missing `<ref>` flag value; textures without Mod) append `(in PowerShell and bash a word starting with # is a comment: write a mod reference as nyh3, not #nyh3)` (`cmds.commentHint`). All-target commands require refs or `--all`, rejecting neither with same hint (`refsOrAll`): `stage restore`, `subscribe update`, textures requiring `--plan`/`--all`. MCP `edit_loadout restore` without refs still restores all, translated to `--all`. Optional-reference read-only commands behave normally: changes current plan, `subscribe check` all.

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
| archo (no arguments), `archo tui` | Open TUI in current terminal; double-click `archo.exe` uses Windows default terminal (06 §9.2). Legacy `archo --here` accepted with same effect |
| `archo gui` | Open desktop window: delegate to adjacent `RimArcho.exe` and return immediately, otherwise run in-process |
| `archo mcp` | stdio MCP server (§5) |
| `archo <command> [arguments]` | Execute once and exit |
| `--env <name>` / `--env=<name>` | Other environment for this command only |
| `--open-plan <name>` / `--open-plan=<name>` | Other plan for command only, no current-plan change (04 §1.3; GUI analysis/pack subprocesses); save/launch makes it current |
| `--version` / `-v` / `archo version` | Print version |
| `archo help`, `archo <command> --help` | Command table; command usage/each argument |

Double-click `RimArcho.exe` opens GUI (optional `--env`); command arguments execute CLI for GUI scan/analysis subprocesses (07 §5).

### 4.2 Command Table

Commands/output call a plan Loadout / plan; editing commands below act on **current plan**.

| Group | Command | Description |
| --- | --- | --- |
| Environments | `env list` / `env detect` / `env use <name>` | List environments (one/game directory) and shared folders; suggest detected game/userdata/Workshop paths (02 §2.2); switch environment |
| | `env add <name> [--detected] [--game <directory>] [--saves shared\|own \| --user-data <directory>] [--launch direct\|steam]` | New environment (02 §2.1); one per game directory; add Steam-library Workshop if absent. First becomes current. `--workshop`/`--steamcmd`/`--extra` remain `folder add` aliases |
| | `env set <name> [--game <directory>] [--saves shared\|own] [--user-data <directory>] [--launch …] [--launch-args <arguments>]` | Change game (rebind `Data`/`Mods`, 02 §3.2), saves, launch, extra arguments (e.g. `-popupwindow`, `""` clears). `--remove-source` equals `folder remove` |
| | `env branches [--install-steamcmd]` / `env download <branch> --to <directory> --account <account-name> [--name <environment>] [--saves …] [--install-steamcmd]` | SteamCMD branch list/version download/new environment; login in SteamCMD (08 §11) |
| Mod folders | `folder list` / `folder add workshop\|steamcmd\|extra <directory>` / `folder remove src:<label>` | Shared folders (02 §3.1); read-only MCP `status show=folders` |
| | `env rebind src:<label> yes\|no` | Answer original-source question |
| | `env remove <name> [--yes]` | Forget environment, RimArcho data only; without yes preview what is forgotten/retained/next environment. Refuse running game; removing current selects another. Same GUI/TUI E (06 §2.8); no MCP mutations |
| Scan | `scan [--full] [--game-only]` | Scan/reconcile (02 §4); full rehashes semantic files; game-only current `Data`/`Mods` |
| Queries | status | Environment overview: version, source Mod counts/residue/Lineage/last scan/changes, plan enabled/missing/game differences, ModsConfig version mismatch, finding counts, running state |
| | `list [--active] [--filter <expression>] [--all]` | `Mods`; active current plan load order; filters `source:``<label\|kind>`, `status:live\|tombstone\|all`, residue, unloadable, copies, `pkg:`, `ws:`, `tag:`, `lineage:`, plain words; live only default |
| | `find <words…> [--all]` | Name/alias/package/author/Workshop ID/folder search |
| | `mod <reference> [--diff <reference>]` | Details; semantic-file differences vs other Instance |
| | `describe <reference> [--source auto\|about\|workshop\|both] [--refresh] [--offline]` | `About` description (versioned descriptionsByVersion) and Workshop (BBCode plain text, images `[image: URL]`); auto/network/cache 06 §4.3; offline cache-only |
| | `def [<Type/defName>] [--mod <reference>] [--all]` | Definers/patchers in load order, effective marked; or all Mod `Defs` |
| | `patches [--mod <reference>] [--target <Type/defName \| XPath-fragment>] [--failed] [--all]` | Operations/simulation; failed only with reasons (03 §4.4) |
| | `harmony [--mod <reference>] [--target <Type.Method \| fragment>] [--shared] [--all]` | Mod/method/shared Harmony patches |
| | `why <A> <B>` | Positions, both declarations, constraints/violations, shared `Defs`, cross-Mod patches, shared Harmony targets/kinds, shared findings |
| | `problems [--ref <p:reference>] [--subject <reference>] [--code <code-or-prefix>] [--severity <level>] [--min-severity <level>] [--all] [--detail] [--ignored] [--include-muted] [--no-analyze]` | Findings (03 §4); rebuild stale analysis in subprocess unless no-analyze; include-muted shows `[version warning ignored]`, otherwise count `ignored-version-warnings` |
| | `problems show\|ignore\|unignore <p:reference>` | Evidence/fixes; ignore until Mod changes; restore |
| | `problems mute-version\|unmute-version <Mod-references…> [--game-version X.Y]` | Version reminders (03 §4.2), default current major/minor; pkg works after removal; mod card preference/restore command |
| | `changes [<reference>] [--since scan:<n>\|<days>d\|YYYY-MM-DD] [--all]` | Change events, latest scan default |
| | `library [--search <word>] [--include <key:value>]… [--exclude <key:value>]… [--sort name\|size\|changed\|played] [--lineage <reference>] [--facets] [--all]` | Inventory (06 §2.9), one Lineage/row: copies/total size/main state/usage; lineage details paths/states/plans/last play/actions; facets values/counts; `tags:` exact, `content:`, `has:`, `name:`, `author:`, `id:`; source roles/aliases 02 §0.2; legacy `location:` retained |
| | `library remove-from-plans <references…>` | Remove from all plans (all environments sharing game directory), one undo each |
| | `library move-to-locked <reference> [--dry-run]` | Move spare `Mods` copy to lock library; same Instance/plans (06 §2.9) |
| | `log [--cluster <E#>] [--file <path>] [--prev] [--text <text>] [--select] [--all] [--since <cursor>]` | Attribution (03 §6); prev reads Player-prev, text pasted logs/stacks, select for problems. `cursor: log1.…`; since new/count+N only, expired restart shows all new-run clusters (03 §6.6); incompatible cluster/text |
| | `pattern [<pattern:id>] [--mod <packageId>] [--all]` | Pattern rules/causes/routes/fixes/corpus counts (03 §6.8); no args generic patterns, mod known errors |
| | `log --follow [--once] [--json]` | Track during game (03 §6.6), emit new clusters; once current content only; JSON per cluster |
| | `files search <pattern> [--mod <reference>]… [--regex] [--case] [--word] [--def] [--kind <kinds,…>] [--languages] [--lang <language>]… [--all-versions] [--game with\|without\|only] [--game-version <x.y>] [--mode matches\|files\|count] [--page <n>] [--limit <n>] [--per-file <n>] [--all]` | Loaded-scope search (03 §5.5): enabled exact copies/current folders/`Defs`/`Patches`/`About`, no translations; grouped load order with position/copy/folder/Def/XPath, pagination; def finds definitions/parent-child/patches/references |
| | `source <reference> [--type <Namespace.Type>] [--all]` | On-demand ilspycmd, DLL-hash cache (03 §5.4) |
| | `source search <pattern> (--in <references…> \| --implicated \| --active --yes) [--regex] [--all] [--budget <seconds>]` | C# search, required scope: named `Mods`, latest log candidates (Harmony warnings without log), or all enabled. Active estimate first (assemblies/DLL bytes/cache ~3×/1.5 seconds each), yes required; budget bounds decompilation, unfinished noted still computing |
| | analyze | Rebuild Def/Patch/Harmony, normally on-demand subprocess |
| Plan edits | `enable <references…> [--at <n>] [--before <reference>] [--after <reference>]` / `disable <references…>` | Enable/disable |
| | `move <reference> [--to <n>] [--before <reference>] [--after <reference>] [--up] [--down]` / `swap <reference> [<reference>]` | Move/swap same-Lineage Instance |
| | `sort [--dry-run]` | Constraint sort (04 §5), preview only with dry-run |
| | `undo [<n>]` / `redo [<n>]` | One/n steps (04 §3) |
| | `stage list` / `stage add <references…>` / `stage drop <references…>` / `stage restore <references…> \| --all [--at\|--before\|--after]` | Staging (04 §2.2), remembers enabled original positions; `restore --all` restores all; add/restore one undo each; reenabled `Mods` leave staging |
| History | `history [--all]` | Current edit steps/undo stack and versions, latest 10 default |
| | `loadout log [--all]` / `loadout diff [<v>] [<v>]` / `loadout checkout <v>` | Versions/diff (latest→workspace default)/load version as undoable edit |
| Write/launch | `save [--dry-run]` | **Write without launch** (UI save), for external/Steam launches: current ModsConfig backup/atomic replace/current game version, sync links, remove pack links (04 §4.1, 08 §3); dry-run preview |
| | `launch [--dry-run]` | Current plan (08 §2); preview command only; write first, refuse running game, record launch and launch version; dry-run uses the `save --dry-run` preview plus launch command |
| | `test run [<plan>] [--env <environment>] [--map] [--timeout <duration>] [--settle <duration>] [--json]` | Isolated run (08 §12), default current, direct launch/collect loading log/stop; `10m` timeout, 5s after menu. Target reached/no clusters: 0, else 1; stdout report, stderr progress |
| Plans | See §4.3 | |
| Exchange | `loadout export [--format text\|archo\|modsconfig\|rimsort\|rml\|rimcrow\|rimcrow-xml] [--out <file>]` | Current plan; never overwrite out |
| | `loadout import <file\|share-code> [--new [<name>]]` | Replace current from text/ModsConfig/save/`.rml`/RimSort/RimCrow code/Workshop IDs, undoable, no game writes (04 §9); new plan (04 §9.1), default own list name/filename |
| | `collection <URL\|ID> [--import]` | Online Workshop comparison; import installed subset in collection order, undoable |
| Locked versions | `pin <instance> [--name <folder>] [--git] [--dry-run]` | Copy to lock library (default `<folder>_local`), `.rimarcho/origin.json`, optional Git baseline, swap current plan only (04 §7); `--update` updates the locked version; `--move-to-library` moves the old `Mods/` copy; `copy-local` alias |
| | `unpin <reference> [--dry-run]` | Restore/use Workshop, including RimCrow style |
| Hidden `Mods` | `unhide <reference> [--dry-run]` / `trash <reference> [--dry-run]` | Restore `About.xml.disabled` to `About.xml`; recycle own `Mods`/locked/SteamCMD copies or hidden/unsubscribed Workshop (08 §6, §7) |
| Workshop | `workshop unsubscribe <references…> [--dry-run]` / `workshop subscribe <references…> [--dry-run]` / `workshop resubscribe <references…> [--dry-run]` / `workshop state <references…>` | Steam client unsubscribe (Steam deletes), resubscribe/status; unavailable error/page (08 §10) |
| Packs | `pack build <name> [--dry-run]` / `pack stop <name>` | Build current plan; cancel/remove partial build (08 §5.2) |
| | `pack list` / `pack launch <name> [--dry-run]` / `pack unlink [--dry-run]` | List/launch/remove only `!ArchoPack_` links (08 §5.3, `pack.links-left` fix) |
| | `pack export <name> <file> [--list]` / `pack import <file> [--as-plan]` / `pack remove <name> [--dry-run] [--unfinished]` | Export `.archopack` (zip), list-only `.archolist.json` (04 §9.3); extract/compare list, as-plan loads current; recycle, unfinished deletes interrupted build (08 §5.4) |
| Rules | `rule add after\|before\|incompatible\|attach <A> <B> [--note <text>]` / `rule add top\|bottom <A> [--note]` / `rule add ignore <constraint> [--note]` | User rules (04 §6), ignore constraint refs printed by sort/problems |
| | `rule edit <r:n> <kind> <A> [<B>] \| ignore <constraint> [--note]` | Replace keeping r:n (04 §6.1) |
| | `rule list` / `rule remove <r:n>` | |
| Player data | `alias <reference> [<name>] [--clear]` / `note <reference> [<text>] [--clear]` | Lineage display name/notes |
| | `tag add\|remove <reference> <tags…> [--mod <reference>]…` / `tag list` | Repeated mod applies to several |
| | `import rimcrow --db <path> [--rules <path>] [--dry-run] [--include-disabled-rules] [--all]` | Repeatable one-time aliases/notes/groups-as-tags/rules import (04 §8) |
| Identity | `identity [list] [--all]` / `identity why <q:n>` / `identity resolve <q:n> same\|copy\|unrelated` | No subcommand lists; all list/inspect/answer include trimmed comparison (02 §5.4) |
| | `lineage split <reference>` / `lineage merge <reference> <reference>` / `gc [--dry-run]` | Split/merge, expired tombstones/unused snapshots (02 §5.5) |
| Community | `data update [<datasets…>] [--dry-run]` / `data status` | Manual rules/steamdb/replacements downloads, source/download/check times (04 §6); UI automatic updates (06 §2.16) |
| Maintenance | `db status` / `db backup` / `db vacuum [--dry-run]` | Path/size/backups; consistent copy in data/backups; compact (02 §7.2) |
| | `cache status` / `cache clear <category>` | Size/clear previews/workshop/images/source/logs; rebuild on demand |
| Settings | `config get [<key>]` / `config set <key> <value>` | Below |
| Help | `guide [<topic>]` | Agent help (§5.3), no topic lists; diagnose/errors/plans/words or full MCP tool reference; identical MCP, unknown usage error |
| Archo Monitor | monitor / `monitor install [--dry-run]` | Bundled/installed version/source/current position; install/update/default add (08 §9, 04 §1.5) |
| Update | `update [--check]` | GitHub version check (automatic record, 06 §2.16.1 A15); check-only version/date/bytes/replaced files/other RimArcho waits/release notes, no download. Otherwise download/SHA-256/wait other same-folder processes (progress who, `Ctrl+C` stops without changes)/replace/verify runnable, rollback otherwise; no restart, ask reopen; no data changes (07 §5.1); no checks in development |
| Uninstall | `uninstall [--dry-run] [--yes] [--keep-locked [--locked-to <environment>]] [--remove-packs] [--packs-to <directory>] [--remove-saves] [--saves-to <directory>] [--remove-games] [--restore-modsconfig-backup] [--all]` | Remove RimArcho artifacts/reverse changes (01 §4.1); no yes = preview like dry-run; both usage error. Flags reverse defaults: keep-locked moves to game `Mods` (default recycle); `--remove-*` recycle (default retain); `--*-to` retention destination; restore backup vs default current-list cleanup; refuse running game/other RimArcho |
| Saves (1.1, not implemented) | `saves list [--colony <name>]` / `saves show <save>` / `unplayed [--group never\|partial\|functional] [--mod <reference>]` | Read-only (10 §`1.6`) |
| | `saves backup <saves…>` / `saves trash <saves…>` | Player confirmation (10 §4.4) |

**Log phases**: explicit markers (03 §6.7.3–4) add short names/counts to log clusters, `problems --code log`, MCP `analyze_log`/`list_problems`, e.g. `[loading x3, save x2, play x4]`; unrecoverable counts unknown. Log phases section lists boundaries in file order with recorded save/time; cluster details phase counts; follow text same, JSON optional phases array. Without markers, no extra sections/labels/JSON.

**Analysis parameter**: `--game-version X.Y` on analyze/problems/def/patches/harmony/why/log, valid like `1.6`, restored after command; MCP `game_version`. Explicit log path/version does not bypass ownership (03 §6.4).

**Automatic check status**: status/`data status`/MCP use one `automatic checks` line: switch, pending Mod/tool updates, configured LoversLab login reminder; no pending: `nothing waiting (last MM-DD HH:MM)`, never checked: `not checked yet`. Omit inapplicable accounts/zero counts; no network/cookies. deps lists managed tool versions/check times; `data status` each dataset check time. Authorization/notifications 06 §2.16, defaults/intervals §2.16.1.

**Settings keys** (`config set`): `ui.lang auto|en|zh`; `ui.icons compat|nerd` (06 §9.3); `ui.mouse on|off` (terminal reporting off for text selection); `workshop.fetch on|off` (descriptions); `workshop.images on|off`; `updates.auto on|off`, `updates.interval normal|frequent|relaxed` (06 §2.16); `ui.gpu on|off` (next window, 06 §2.14); tags.pinned `<comma-separated-tags>`; `monitor.default on|off` (04 §1.5); monitor.dir `<absolute-path>` (08 §9). Stored meta cfg:`<key>`; `config.toml` only `tombstone_days` (07 §3).

### 4.3 Plan Management

An environment may have several plans (04 §1):

| Command | Effect |
| --- | --- |
| `plan list` | All shared plans (04 §1.7): name/enabled/last played/current in environment |
| `plan new <name> [--from current\|v<n>\|<file>]` | From current, its version, or list file (`loadout import` formats) |
| `plan use <name>` | Switch without game writes |
| `plan rename <old-name> <new-name>` | Rename |
| `plan remove <name> --yes` | Delete/history, never final plan |

- Keep `loadout log/diff/checkout/export/import` for current plan; import `--new` creates new, otherwise undoable replacement.
- Other commands current plan, no `--plan` yet.
- Outside conflict choices (04 §4.3): diff `v<n>` workspace, `loadout checkout v<n>`, `loadout ignore-external` clears unimported-external flag, retains historical version.
- Remove `loadout tag` (named versions as lists); named lists are plans.
- Remove `launch --force`: always write before launch, never unwritten launch.

## 5. MCP (`archo mcp`)

### 5.1 Transport and Lifecycle

- stdio only (`StdioTransport`), no network ports; stdout protocol only.
- Server rimarcho, release version; sends §5.3 `Instructions` on initialization.
- Not resident: exits when harness closes session.
- Starts in current environment; each tool env switches for call, restores afterward.
- Calls serialize in-process, sharing one App.
- Tools build CLI argv, return `cmds.Execute` text unchanged. Exit 1/2 `isError`; 3 ambiguity is normal candidate-list result.
- Analysis waits per 06 §2.16.1 W01 (CLI unlimited); on timeout declaration findings with still computing in the background, subprocess continues, next call gets results; `get_def`/`get_patches`/`get_harmony` use previous stale analysis if available.
- `read_source` search default budget 10 seconds, remaining assembly count/still computing; run again.

### 5.2 Tool Table

Link tools: read-only `check_links` maps `archo links check [--all] --json`; destructive `repair_links` previews with `dry_run=true`, execution requires `confirm=true`, unproven ownership additionally requires include full paths. CLI `links repair [--all] --dry-run` / `--yes [--include <link-path>…]`, JSON supported. Without all, current game/local source; full paths/targets/reasons/evidence/actions. Dangling/check-failed check exits 1. Boundaries: [08 §3](08-launch-and-packs.md#3-%E9%93%BE%E6%8E%A5%E9%83%A8%E7%BD%B2).

MCP annotations guide external harness approval; Archo tiers in 09 §2.2. `openWorldHint: true`: `get_description` (public Steam API), `exchange_list` (collections), maintenance (community downloads), `manage_subscriptions` (Git/pages/Steam); others false.

| Tool | Command | readOnlyHint | destructiveHint |
| --- | --- | --- | --- |
| `guide` | guide (topic), no env (same across environments) | true | — |
| `status` | status | true | — |
| `find_mods` | find with query, otherwise list (active/filter) | true | — |
| `get_mod` | mod | true | — |
| `get_description` | describe (source/refresh/offline) | true | — |
| `get_def` | def (def/mod/all/`game_version`) | true | — |
| `get_patches` | patches (mod/target/failed/all/`game_version`) | true | — |
| `get_harmony` | harmony (mod/target/shared/all/`game_version`) | true | — |
| `explain_relation` | why (a/b/`game_version`) | true | — |
| `list_problems` | problems, ref → `problems --ref` (retains all/`game_version`); default `--min-severity warning` unless severity/`min_severity`/all/`include_muted`; `include_muted` → `--include-muted` | true | — |
| `list_changes` | changes (ref/since/all) | true | — |
| `analyze_log` | log (cluster/text/file/prev/all/since/`game_version`); pattern=`<reference>` → pattern (list means all) | true | — |
| `search_files` | `files search` (pattern/mod/mods/regex/`case_sensitive`/word/def/kind/languages/lang/`all_versions`/game/`game_version`/mode/page/limit/`per_file`/all) | true | — |
| `read_source` | search → `source search`, in → `--in`, `scope: "active"` → `--active --yes`, otherwise `--implicated`; budget default 10. Otherwise source (ref/type) | true | — |
| `edit_loadout` | action enable/disable/move/swap/sort (`dry_run`)/undo/redo/stage (`stage add`)/unstage (`stage drop`)/restore (`stage restore`); at/before/after/up/down | false | false |
| `save_loadout` | save (`dry_run`) | false | true |
| `launch_game` | launch (`dry_run`) | false | true |
| `estimate_memory` | plan cost [`<list>`]; resident scope/load budget/RAM-VRAM capacity comparisons, unknown/severity, 06 §2.4; disposable estimate cache only | true | false |
| `test_run` | `test run --json` (env/plan/map/`timeout_seconds`), may take minutes, isolated player saves/config/logs (08 §12), `openWorldHint: false` | false | false |
| `manage_plans` | `plan list/new/use/rename/remove --yes`, `loadout import --new` | false | true |
| `plan_history` | `loadout log`/diff, history (`action=steps`) | true | — |
| `pin_mod` | pin (ref/name/git/update/`move_to_library`/reason/`dry_run`); reason in copy origin, shown in copy notes (06 §2.9) | false | false |
| `copy_local` | Old pin name (ref/name/git) | false | false |
| `unpin_mod` | unpin (ref/`dry_run`) | false | false |
| `set_player_data` | `tag_add`/`tag_remove` (refs/tags), alias/note (text, empty clears), `copy_note` (note `<reference>` `<text>` --copy, empty clears, 06 §2.9) | false | false |
| `manage_rules` | `rule add/list/remove` (action/kind/a/b/target/note/rule) | false | false |
| `add_rule` | Old `manage_rules`, default action add | false | false |
| `ignore_problem` | `problems ignore`/unignore (undo) | false | false |
| `mute_version_warnings` | `problems mute-version`/`unmute-version` (refs/`game_version`/undo), 03 §4.2 | false | false |
| `scan` | scan (full) | false | false |
| `exchange_list` | `loadout export/import`, `collection [--import]` | false | false |
| `manage_packs` | `pack list/build/launch/export/import/remove/stop/unlink` | false | true |
| `resolve_identity` | `identity list/why/resolve` (`resolve_identity action=why question=q:<n>` read-only), `lineage split/merge` | false | true |
| `manage_hidden` | unhide/trash | false | true |
| `maintenance` | `data update`/`import rimcrow`/`db vacuum` (`action=vacuum`)/gc | false | true |
| `list_library` | library (search/include/exclude/sort/lineage/facets/all) | true | — |
| `manage_library` | `move_to_locked` (`library move-to-locked`, `dry_run`/reason), `remove_from_plans`, unsubscribe/subscribe (`workshop unsubscribe\|subscribe`, `dry_run`), `steam_state`; refs | false | true |
| `manage_subscriptions` | list default; add URL/kind → `subscribe git <url> [--branch\|--tag\|--commit]`, `subscribe page <url> [--git] [--from]`, `subscribe workshop <ID…> [--steamcmd]`; check refs or all; update refs/all/from, refuses running game; remove. add/update/remove `dry_run` | false | true |
| `preview_uninstall` | `uninstall --dry-run` (`keep_locked`/`locked_to`/`remove_packs`/`packs_to`/`remove_saves`/`saves_to`/`remove_games`/`restore_modsconfig_backup`/all); never yes | true | — |

Authoritative tools/annotations: `Tools()` in `internal/surface/mcp/server.go` (aliases included). `destructiveHint: true`: `save_loadout`, `launch_game`, `manage_plans`, `manage_packs`, `resolve_identity`, `manage_hidden`, `manage_library`, `manage_subscriptions`, `manage_textures`, `repair_links`, maintenance. Add missing §1.2 tools likewise.

**Description revisions** (complete; pre-2026-10-21 descriptions disagreed with code, retained as history):

| Tool | Old description | Old actual behavior | Current description |
| --- | --- | --- | --- |
| `save_loadout` | Refuses when ModsConfig.xml changed outside RimArcho. | **Did not refuse**: backup/overwrite directly; detect on read only (external version/conflict) | Save without launch: current plan into ModsConfig.xml (backup/atomic/current version), sync `_Archo_` links/remove pack links. Changes next game-loaded list: ask player, preview `dry_run=true`. Refuse unresolved outside conflict (04 §4.3), offer diff/load/ignore commands. Still writes while game runs, explains next-launch effect |
| `edit_loadout` | Change RimArcho's workspace Loadout … nothing reaches the game until `save_loadout` | Environment's sole workspace | Current plan, immediate persistence/undo each step, no game files until save/launch. Add checkout, no plan parameter yet (§4.3) |
| `status` | Example unsaved edits | Workspace vs latest version | Say differences from game list, list plans |
| `copy_local` | The Loadout is not changed: call `edit_loadout` action=swap … | Did not change plan | Create locked version: independent copy, swap current plan only; Workshop updates do not affect it |

### 5.3 Progressive Disclosure: Instructions, Tool Descriptions and guide

On connection, agent reads only `Instructions` and short descriptions/parameter schemas, enough to choose tools. Details in read-only guide, fetched as needed; maps CLI archo guide [`<topic>`], identical text (§1.1). Rationale: 00 §6 D23.

| Layer | Read when | Contents | Code |
| --- | --- | --- | --- |
| `Instructions` | Initialization | What RimArcho knows; references (`#id`, `L#id`, `pkg:`, `ws:`, `def:Type/name`, `p:<hex>`, `log:E<n>`, `r:<n>`, `pattern:<id>`) usable across tools; 24 KB result limit, narrow with `mod=`, `target=`, cluster, or pattern instead of `all=true` (corpus G7). Start status; errors/crashes read guide diagnose first, state two key rules: patcher/mentioned are not causes without more evidence; no source means unknown cause. Edits current plan only, no game changes until save/launch; pin before Mod file edits. Destructive tools (`save_loadout`/`launch_game`/`manage_plans`/`manage_packs`/`resolve_identity`/`manage_hidden`/`manage_library`/`manage_subscriptions`/`manage_textures`/`repair_links`/maintenance): preview if supported, ask player first. End with guide usage | `Instructions` in `internal/surface/mcp/desc.go` |
| Tool descriptions | `tools/list` | 1–3 sentences: action, when to use/other tools, writes/`dry_run`/player approval where needed. No examples or per-parameter explanations (schema owns those) | `desc*` in `desc.go` |
| guide topics | guide call | diagnose full workflow below; errors `patternChecklist` (03 §6.8); plans (plans/versions/inventory/subscriptions/textures/pin before file edits); words UI names (06 §7.2). Per tool: full actions/parameters/edge cases/one example/first output lines | `internal/cmds/guide.go`, `guide_tools.go`, `guide_patterns.go` |

- Reader is agent: without skills, only `Instructions`/descriptions/on-demand guide must support former error-debugging/assembly-inspector workflows (I4). Same for Archo (09 §2.1).
- Budget (`TestInitialContextBudget`, I13): `Instructions` ≤2000 characters, description ≤320 each, combined ≤12000; `tools/list` JSON including schema ≤48 KB. Short new descriptions, details in guide; justify budget increases in commit. Parameter descriptions short too; env always environment name (default: current).
- No-topic guide lists all; unknown topic usage exit 2/`isError`, lists workflows. Return topic text verbatim, bypass `ShellSafe`, preserve # in example JSON.
- No tools merged: `add_rule`/`copy_local` aliases retained per §6, one-sentence descriptions point to new names; old explanation/examples in guide topics.
- Every tool except guide needs same-name topic with Example: `<tool-name>` { (`TestToolsAndAnnotations`).

Tool schema `game_version`/env must be **direct fields**: SDK does not expand embedded structs, rejecting embedded fields as unknown. Tools without env (guide/`preview_uninstall`) use current environment.

**`guide topic=diagnose`** (English original), use verbatim evidence, not paraphrase:

1. **Start with the error.** Pasted log → `analyze_log text=…`; otherwise current log, then `analyze_log cluster=E<n>`. List candidate roles: source (own code/data produced error), patcher (Harmony on frame), dependency (missing/disabled provider), mentioned (message/lower frame). Never call patcher/mentioned cause without more evidence; own Mod evidence keeps source; frameworks providing types or callers alone are not causes; no source or own faulty files/code means cause undetermined.
2. **Confirm identity/state with `get_mod`**: exact Instance, enabled/position/path/other Lineage copies.
3. **Classify**:
   - XML (failed PatchOperation, missing xref, duplicate/missing parent, malformed XML): `get_patches failed=true` (target-inactive/missing/later, wrong-version-route, stale-name, unguarded-optional, node-absent and player meanings), `get_def`, `search_files`.
   - Material/icon (`_MainTex`/`ResolveIcon`): XML first, named `Mods` with `uiIconColor` but no `uiIconPath`/`texPath`.
   - Assembly (C# exceptions, `Pawn.Tick`/`ThingWithComps.Tick`/world ticks): `get_harmony` per patched frame, `read_source` patch type/search. Tick crash: candidate-only `read_source search="AllComps.Remove(" in=[…]`, plus `"AllComps.Add("`, `"InitializeComps"`, and `"PostExposeData"`.
   - Assembly loading (ReflectionTypeLoadException/TypeLoadException/static constructor): named assembly in message/nearby lines, `analyze_log` identifies enabled loader.
   - Mixed: confirm XML loaded successfully first, then blame code.
   - Others (save/config/assets/noise): `guide topic=errors`.
4. `list_problems` for static dependencies/incompatibility/order/Def overrides/Patch failures/Harmony conflicts; `explain_relation` for two `Mods`.
5. **Report** phase, XML/assembly/mixed, exact Instances/paths, file+line or class+method, why, cause/dependency/bystander roles, smallest fix.
6. Previously worked: `list_changes`, `plan_history action=diff`.

Tool guide highlights (examples/output `internal/cmds/guide_tools.go`):

| Tool | When / alternatives |
| --- | --- |
| `status` | First step; single Mod `get_mod`, finding `list_problems` |
| `find_mods` | Name/package/author/Workshop/folder; ambiguous packages list every ref; next `get_mod` |
| `get_mod` | Complete Instance facts; ambiguity returns candidates |
| `get_def` | No ThingDef named X, duplicates, winning Mod |
| `get_patches` | Logged Patch operation failed; `failed=true` reasons |
| `get_harmony` | PREFIX/POSTFIX stack lines, `Mods` touching `Pawn.Tick`; next `read_source` patch class; bool prefix may skip, manual `Harmony.Patch` unresolved noted |
| `explain_relation` | Two named `Mods` or why A after B |
| `list_problems` | Static findings, `ref=<p:…>` for evidence/fixes, code prefixes dep, patch, harmony; content layers rebuild after edits (seconds) |
| `list_changes` | Worked yesterday |
| `analyze_log` | Always text for pasted errors; categories exception/harmony-stack/xml-load/patch-failed/xref-missing/material/other. Player.log 1.7+ `Logs/Player.log`, earlier userdata root. Poll since=cursor for new/increased Player/Monitor clusters; expired means restart, old run `prev=true`. Known pattern community context, not list evidence; `pattern=<id>` expands |
| `search_files` | XML defName/XPath/`uiIconColor`/class names; C# `read_source` |
| `read_source` | Patch/frame type; short specific patterns; `scope="active"` only on player request (hundreds of assemblies/minutes); missing ilspycmd installation advice, never auto |
| `edit_loadout` | Plan edits, undo each |
| `save_loadout` | §5.2 |
| `copy_local` | Copy before Workshop edits, Steam overwrites; no outside links/upstream `.git`, never overwrite folders |
| `get_description` | Understand before recommending; fetch/cache until item update; `workshop.fetch` disables network |
| `set_player_data` | Tags (`list --filter tag:<t>` and TUI `tag:`), alias/notes shared Lineage; `copy_note` why retain this copy, no game writes |
| `manage_subscriptions` | Install Git/page/SteamCMD or check updates; check before update, dry-run add/update/remove; player supplies page files via from, no agent LoversLab login; client Workshop via `manage_library`; pin old version first |
| `add_rule` | Ignore wrong-direction declared/community constraints to break `order.cycle`; affects sort/findings, no automatic reorder |

## 6. Stability

- English output keywords (`depends:`/`evidence:`/`next:`/codes) are interface; changes in CHANGELOG (07 §5).
- Stable reference formats: Instance independent of paths/package (02), p: across analyses, log keys final three absolute-path segments, preserving references/ignores after moves. Pattern refs `pattern:g:<id>` and `pattern:m:<packageId>:<hash>` are stable within dataset; generic IDs across versions; Mod-specific template hash unchanged if template unchanged.
- Append new fields without changing existing line meanings.
- Keep renamed MCP aliases at least one minor version.

## 7. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| I1 | Same CLI/MCP command | Identical bytes, 19 in-memory calls (`TestCLIParity`) | Pass |
| I2 | Any output reference | Direct argument (`TestRefsRoundTrip`: `#id`/`pkg:`/p:/`log:E#`/`def:`) | Pass |
| I3 | Ambiguous pkg | All candidates, no arbitrary selection (`TestAmbiguousPackage`) | Pass |
| I4 | Claude Code/Codex MCP without skills | Diagnose patch failure/`Pawn.Tick`; since 2026-10-30 agent must read guide diagnose | Scripted `spikes/mcpcheck` pass; real agent pending, especially guide discovery after reduction |
| I5 | Default output | ≤40 lines, expand command (`TestCLIParity`/`TestTruncation`) | Pass |
| I6 | Annotations | `Tools()` `readOnlyHint: true`, 11 destructive in §5.2, guide examples (`TestToolsAndAnnotations`); synthetic writes (`TestWriteTools`), env (`TestEnvParameter`), timeout still computing (`TestStillComputing`) | Pass |
| I7 | Each §1.2 action | CLI/MCP, annotations match writes | Partial: 1.0 tools complete, subscriptions added 2026-10-30; 1.1 saves missing |
| I8 | `save_loadout` unresolved external conflict | Refuse with diff/load/ignore, file unchanged | Pass |
| I9 | Launch/save while game runs | Launch refuses naming environment; save writes, next-launch notice | Pass |
| I10 | CLI/MCP `plan new`/use/rename/remove/import-new | Identical, use no game writes, retain final plan | Pass |
| I11 | Paste fix/next/more/error commands into cmd/PowerShell/bash | All arguments/references (`TestPrintedCommandsSurviveShells`: PS5.1/7 parses no comments, expected words, executes each; `TestShellSafe`) | Synthetic pass; real VM pending |
| I12 | Typed # references become comments | Missing Mod usage explains (`TestMissingRefNamesTheComment`, host PS5.1/7) | Synthetic pass; real VM pending |
| I13 | Initial agent context (§5.3) | `Instructions` ≤2000, description ≤320, combined ≤12000; `tools/list` JSON ≤48 KB; every topic fetchable, unknown `isError` (`TestInitialContextBudget`/`TestGuideTopics`) | Pass |
