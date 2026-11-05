# 14 · MCP

> Status: TODO(reorg)

## 1. Transport and Lifecycle

- Only stdio is supported (`StdioTransport`); no network ports are opened. stdout carries only protocol messages.
- **Tool groups.** `archo mcp` serves the core group (§2): the 29 tools for diagnosis (Mod settings and saves included), plan edits, saving, launching, test runs, locking, rules and ignoring problems. `archo mcp --tools all` (or `--tools=all`) serves all 45, adding the player's management tools (13 §1.2). `--tools core` is the default. Another value, an unknown flag or a positional argument is a usage error (exit 2) before the data folder is opened; `archo mcp --help` shows the flag. A tool outside the served group is an unknown tool to the client. The group of each tool is part of its row in `Tools()`; `Tools()`, `Catalog()` and `ArgvFor()` still cover every tool for harnesses (`spikes/agenteval`), and `ToolDoc.Group` names its group. Measured on synthetic data (2026-11-04): core `tools/list` is 30,158 bytes, all is 48,497 bytes, close to the 48 KB limit (§3).
- The server is named rimarcho and uses the release version. It sends the `Instructions` from §3 during initialization.
- The server is not resident: the process exits when the harness closes the session.
- The server starts in the current environment. A tool's env parameter switches environments for that call and restores the previous environment afterward.
- Calls within a process run serially and share one App.
- Each tool builds a CLI argv and returns the text from `cmds.ExecuteContext` unchanged, run with `cmds.ForMCP` so that a reference matching nothing names `find_mods` (13 §3.3). Exit codes 1 and 2 set `isError`. Exit code 3, an ambiguous reference, is a normal result because the text lists the candidates.
- Calls that require analysis wait according to 16 §2.16.1 W01; the CLI has no time limit. If the analysis subprocess exceeds the wait bound, return declaration-level findings with still computing in the background. The subprocess continues, and the next call gets its results. Meanwhile, `get_def`, `get_patches`, and `get_harmony` use the previous, stale analysis if available.
- A `read_source` call with search has a default budget of 10 seconds. It reports how many assemblies remain to be decompiled and says still computing; run again.
- **Progress.** When a tool call carries a progress token, the server sends `notifications/progress` every 10 seconds until the result. Each one has a count that rises by 1 and a message: the command's latest progress line (`test_run`: preparing test data, waiting for the game to load, analysing the log, comparing with the player's log), or running, then the elapsed seconds. Nothing is sent after the result, and a call without a token gets none. Clients that restart their request timeout on progress therefore wait for `test_run` instead of cancelling it. Pi 1.4 asks for progress on every call and cancels after 60 s without any. A cancelled `test_run` stops the game (09 §7). A client whose timeout ignores progress still needs a limit above the run's length.

## 2. Tool Table

For link health, the read-only `check_links` maps to `archo links check [--all] --json`. The destructive `repair_links` only previews when `dry_run=true`; execution requires explicit `confirm=true`. Links without ownership evidence additionally require their full paths in include. The CLI equivalents are `links repair [--all] --dry-run` / `--yes [--include <link-path>…]`, with JSON also supported. Without all, check the current game and its local source. Output includes the full paths, targets, reasons, evidence, and actions. A dangling link or failed check makes check exit with code 1. See [09 §3](09-launch.md#3-link-deployment) for the action boundaries.

Annotations follow the MCP specification; the external harness decides whether to permit an action. Archo uses the tiers in 15 §2.2. `openWorldHint: true` applies to `get_description` (the public Steam API), `exchange_list` (Steam collections), maintenance (community downloads), `manage_subscriptions` (Git hosts, download pages, and Steam), and `manage_saves` (a fix's Workshop subscriptions: the Steam Web API and Steam). It is false for the other tools.

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
| `read_source` | With search, use `source search`: in maps to `--in`, `scope: "active"` maps to `--active --yes`, and otherwise use `--implicated`. `game: true` adds `--game`; without search it selects `source game` (optionally with type). Otherwise use source (ref and type). The default dispatch budget is 10 seconds. Library paths and shared-copy attribution follow 05 §5.4. | true | — |
| `edit_loadout` | The action is enable, disable, move, swap, sort (`dry_run`), undo, redo, stage (`stage add`), unstage (`stage drop`), or restore (`stage restore`). Position parameters are at, before, after, up, and down. | false | false |
| `save_loadout` | save (`dry_run`) | false | true |
| `launch_game` | launch (`dry_run`) | false | true |
| `estimate_memory` | plan cost [`<list>`] shows the resident scope, loading budget, RAM and VRAM capacity comparisons, unknown values, and severity; see 16 §2.4 for the definitions. It writes only a disposable estimate cache. | true | false |
| `test_run` | `test run --json` (env, plan, map, and `timeout_seconds`) may take several minutes to launch the game; it sends progress every 10 seconds (§1). It isolates player saves, configuration, and logs, and links the plan's game-invisible Mods (pinned copies) into `Mods/` for the run only (`linked`; a Mod it cannot link is in `missing` with `reason`), 09 §7; `openWorldHint: false`. The report compares the run's clusters with the player's log (`before`, `player_log`, `fixed`) and the previous run (`delta`, `baseline`, `gone`), 09 §7. | false | false |
| `manage_plans` | `plan list/new/use/rename/remove --yes`, `loadout import --new` | false | true |
| `plan_history` | `loadout log`, diff, and history (`action=steps`) | true | — |
| `pin_mod` | pin (ref, name, git, update, `move_to_library`, reason, and `dry_run`); reason is stored in the copy's origin and shown in its copy notes (16 §2.9). | false | false |
| `unpin_mod` | unpin (ref and `dry_run`) | false | false |
| `set_mod_settings` | mod-settings set (ref; set as `name=value` pairs in name order; each reset; file; `dry_run`) | false | false |
| `set_player_data` | The action is `tag_add` or `tag_remove` (refs and tags), alias or note (text; an empty string clears it), or `copy_note` (the note for one copy, using note `<reference>` `<text>` --copy; an empty string clears it, 16 §2.9). | false | false |
| `manage_rules` | `rule add/list/remove` (action, kind, a, b, target, note, and rule) | false | false |
| `ignore_problem` | `problems ignore` or unignore (undo) | false | false |
| `mute_version_warnings` | `problems mute-version` or `unmute-version` (refs, `game_version`, and undo); see 05 §4.2 for the scope. | false | false |
| `scan` | scan (full) | false | false |
| `exchange_list` | `loadout export/import`, `collection [--import]` | false | false |
| `manage_packs` | `pack list/build/launch/export/import/remove/stop/unlink` | false | true |
| `resolve_identity` | `identity list/why/resolve` (`resolve_identity action=why question=q:<n>` is read-only), and `lineage split/merge` | false | true |
| `manage_hidden` | unhide and trash | false | true |
| `maintenance` | `data update`, `import rimcrow`, `db vacuum` (`action=vacuum`), and gc | false | true |
| `list_library` | library (search, include, exclude, sort, lineage, facets, and all) | true | — |
| `list_saves` | `saves list` (colony, mod, attention, all); env both selects the environment and keeps only its saves, like the CLI's global `--env` | true | — |
| `get_save` | `saves show` (save, mods, play, all); the first call for a save reads it in full into RimArcho's cache (12 §1.7) | true | — |
| `manage_saves` | The action is fix (`saves fix`: mods as `--mod`, `disable_added`, order, launch and `dry_run`), rename (name), copy, backup or trash; saves names the targets (fix, rename and copy take the first). Backup and trash only preview unless `confirm=true` (`--yes`) without `dry_run`; never confirm without the player's agreement. Rename and copy have no dry run: `dry_run=true` with them is an error. | false | true |
| `manage_library` | The action is `move_to_locked` (`library move-to-locked`, with `dry_run` and reason), `remove_from_plans` (library remove-from-plans), unsubscribe or subscribe (`workshop unsubscribe\|subscribe`, with `dry_run`), or `steam_state` (workshop state); refs identifies the targets. | false | true |
| `manage_subscriptions` | The action is list (the default, using subscribe list), add, check, update, or remove. Add uses URL or kind to choose `subscribe git <url> [--branch\|--tag\|--commit]`, `subscribe page <url> [--git] [--from]`, or `subscribe workshop <ID…> [--steamcmd]`. Check uses subscribe check with refs, or all subscriptions if refs is absent. Update uses subscribe update with refs or all, and from, and refuses while a game is running. Remove uses `subscribe remove`. Add, update, and remove accept `dry_run`. | false | true |
| `preview_uninstall` | `uninstall --dry-run` (`keep_locked`, `locked_to`, `remove_packs`, `packs_to`, `remove_saves`, `saves_to`, `remove_games`, `restore_modsconfig_backup`, and all); never pass --yes. | true | — |

**Groups.** Core (served by default): `guide`, `status`, `find_mods`, `get_mod`, `get_mod_settings`, `get_description`, `get_def`, `get_patches`, `get_harmony`, `explain_relation`, `list_problems`, `list_changes`, `analyze_log`, `search_files`, `read_source`, `edit_loadout`, `save_loadout`, `launch_game`, `test_run`, `pin_mod`, `unpin_mod`, `manage_plans`, `plan_history`, `manage_rules`, `ignore_problem`, `scan`, `list_library`, `list_saves` and `get_save`. Only with `--tools all`: `manage_subscriptions`, `manage_packs`, `manage_library`, `exchange_list`, `manage_textures`, `estimate_memory`, `maintenance`, `resolve_identity`, `manage_hidden`, `check_links`, `repair_links`, `mute_version_warnings`, `set_player_data`, `set_mod_settings`, `preview_uninstall`, and `manage_saves` (`TestToolGroups`).

The authoritative tools, groups and annotations are `Tools()` in `internal/surface/mcp/server.go`. `destructiveHint: true` applies to `save_loadout`, `launch_game`, `manage_plans`, `manage_packs`, `resolve_identity`, `manage_hidden`, `manage_library`, `manage_subscriptions`, `manage_textures`, `manage_saves`, `repair_links`, and maintenance. Add tools still missing from 13 §1.2 in the same way.

**Description revisions** are complete. The descriptions before 2026-10-21 disagreed with the code; the old descriptions are retained here as a record:

| Tool | Old description | Old actual behavior | Current description |
| --- | --- | --- | --- |
| `save_loadout` | Refuses when ModsConfig.xml changed outside RimArcho. | **Did not refuse**: it backed up and overwrote the file directly. Outside changes were detected only on read, loaded as an external version, and marked as a conflict. | Save without launch: write the current plan into ModsConfig.xml, with a backup, atomic replacement, and the current game version. Sync `_Archo_` links and remove pack links. This changes the list the game will read next, so ask the player and preview the diff with `dry_run=true` first. Refuse unresolved outside conflicts (07 §4.3), offering commands to view the diff, load it, or ignore it. Still write while the game runs, explaining that it reads the list on the next launch. |
| `edit_loadout` | Change RimArcho's workspace Loadout … nothing reaches the game until `save_loadout` | It changed the environment's sole workspace. | Edit the current plan with immediate persistence and an undo for each step. Write no game files until save or launch. The checkout action was added; there is no plan parameter yet (13 §4.3). |
| `status` | The example said unsaved edits. | It showed the difference between the workspace and the latest version. | Say differences from the game list, and list the plans. |
| `copy_local` (removed in 1.0.1; now `pin_mod`) | The Loadout is not changed: call `edit_loadout` action=swap … | It did not change the plan. | Create a locked version as an independent copy and swap it into the current plan, leaving other plans unchanged. Workshop updates do not affect it. |

## 3. Progressive Disclosure: Instructions, Tool Descriptions and guide

On connection, the agent reads only `Instructions`, each tool's short description, and the parameter schemas. This is enough to choose the right tool. Details live in the read-only guide tool and are fetched as needed. It maps to the CLI archo guide [`<topic>`] and returns identical text (13 §1.1). See 00 §6 D23 for the rationale.

| Layer | Read when | Contents | Code |
| --- | --- | --- | --- |
| `Instructions` | Initialization | Explain what RimArcho knows and that output references (`#id`, `L#id`, `pkg:`, `ws:`, `def:Type/name`, `p:<hex>`, `log:E<n>`, `r:<n>`, and `pattern:<id>`) can be passed directly to other tools. Results are limited to 24 KB; narrow the scope with `mod=`, `target=`, a cluster, or a pattern instead of `all=true` (corpus G7). Start with status. For an error or crash, read guide topic=diagnose first and state the two key rules: a patcher or mentioned Mod is not the cause without further evidence; without a source, the cause is undetermined. Editing tools change only the current plan and do not touch the game until save_loadout or launch_game; use pin_mod before editing Mod files, then test_run, which tests the copy without saving. For destructive tools (`save_loadout`, `launch_game`, `manage_plans`, `manage_packs`, `resolve_identity`, `manage_hidden`, `manage_library`, `manage_subscriptions`, `manage_textures`, `manage_saves`, `repair_links`, and maintenance), show a preview if supported, then obtain the player's agreement before calling. Say in one sentence that the player's management tools (packs, subscriptions, library actions, textures, memory estimate, maintenance, identity, hidden Mods, links, player data, save actions, uninstall preview) are served only by `archo mcp --tools all`, and that otherwise the agent uses the archo CLI. End by explaining how to use guide. | `Instructions` in `internal/surface/mcp/desc.go` |
| Tool descriptions | `tools/list` | Use 1–3 sentences to explain what the tool does, when to use it or another tool, and what it writes. Explicitly state any requirement for `dry_run` and player approval. Do not include examples or explain each parameter; the schema owns those descriptions. | `desc*` in `desc.go` |
| guide topics | guide call | Provide workflows: diagnose (the full workflow below), errors (the `patternChecklist`, 06 §8; its save-loading kind names `get_save`), plans (plans and versions, inventory, subscriptions, textures, and pinning before file edits), and words (UI names when talking to the player, 16 §7.2). Provide a topic for each tool name with its full actions, parameter details, edge cases, one example call, and the first lines of example output. | `internal/cmds/guide.go`, `guide_tools.go`, `guide_patterns.go` |

- The reader is an agent. With no skills loaded, `Instructions`, tool descriptions, and on-demand guide topics must be sufficient to complete the former error-debugging and assembly-inspector workflows (I4). Archo uses the same content (15 §2.1).
- The repository's `skills/rimworld-probe` is separate from this surface. It gives an agent a probe-mod skeleton and a sandbox runner for checking behavior in the real game (1.6 and earlier). Archo's tools and guide topics must not depend on it.
- The context has a budget (`TestInitialContextBudget`, I13). For the default core group: `Instructions` ≤2000 characters, each description ≤320 characters, and the combined total ≤12000 characters. The `tools/list` JSON of all tools (`--tools all`), including the schema, is limited to ≤48 KB. Keep new descriptions as short as the existing ones and put details in the guide topic. If a budget increase is necessary, explain it in the commit. Keep parameter descriptions short too; env always uses environment name (default: current).
- With no topic, guide lists all topics. Its tool list puts the tools only `--tools all` serves on a separate line, only with archo mcp --tools all, so the text is the same on every surface and in either group (I1; `cmds.AllOnlyTools`, kept in step with `Tools()` by `TestToolGroups`). Their guide topics remain available to a core session. An unknown topic is a usage error (exit 2 and `isError`), with text listing the workflow topics. Return topic text verbatim, bypassing `ShellSafe` so that # references in example JSON are preserved.
- No tools have been merged. The aliases `copy_local` (old name of `pin_mod`) and `add_rule` (old name of `manage_rules`) were removed in 1.0.1 (13 §5); calling them is an unknown tool.
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
| `get_def` | Use it for No ThingDef named X, duplicate Defs, or which Mod wins. A `log:` line says when the newest log reports the effective definition's file failed to load (06 §3). |
| `get_patches` | Use it for a logged Patch operation failed; `failed=true` gives the reason. `target=<defName>` also finds operations nested in `FindMod` / `Sequence` (P26). |
| `get_harmony` | Use it for PREFIX/POSTFIX stack lines or `Mods` touching `Pawn.Tick`; next, use `read_source` on the patch class. A bool-returning prefix may skip the original method. Patches applied in code through `Harmony.Patch` are marked as not statically resolvable. |
| `explain_relation` | Use it when a finding names two `Mods` or the user asks why A must come after B. |
| `list_problems` | Get static findings. `ref=<p:…>` shows evidence and repair commands; code accepts the prefixes dep, patch, and harmony. After plan edits, the Def, Patch, and Harmony layers rebuild automatically in a few seconds. |
| `list_changes` | Find changes when the user says it worked yesterday. |
| `analyze_log` | Call it first for any game error or crash, before reading Player.log with other tools (MCP review round 2 P10). Always use text for pasted errors. Cluster categories are exception, harmony-stack, xml-load, patch-failed, xref-missing, material, and other. Player.log is at `Logs/Player.log` in 1.7 and later, and at the user-data root in earlier versions. While the game runs, pass the last cursor back as since to get only new clusters or increased counts, combining Player.log and ArchoMonitor.log. An expired cursor means the game restarted; the old run is then available with `prev=true`. Known cluster kinds include a pattern:<id> reference as community context, not evidence about this list; `pattern=<id>` expands it. |
| `search_files` | Find XML evidence: defName, XPath targets, `uiIconColor`, or class names in XML. Use `read_source` for C#. |
| `read_source` | Inspect a patch class or the type containing a frame. Prefer short, specific search patterns. Use `scope="active"` only when the player requests it, because it can cover hundreds of assemblies and take many minutes. If ilspycmd is missing, the text explains installation; it is never installed automatically. |
| `edit_loadout` | Edit the plan, with an undo for each step. |
| `save_loadout` | §2 |
| `pin_mod` | Copy a Workshop Mod before editing it because Steam overwrites Workshop files. Do not copy links outside the folder or the upstream `.git`, and never overwrite an existing folder. |
| `get_description` | Learn what a Mod does before recommending it. Fetch Workshop text on demand and cache it until the item updates. The player can disable network access with `workshop.fetch`. |
| `set_player_data` | Set tags (used by `list --filter tag:<t>` and TUI `tag:` search), aliases, and notes on the Lineage, shared by all copies. `copy_note` belongs to one copy and explains why it is retained. It does not touch the game. |
| `manage_subscriptions` | Use it to install a Mod from Git, a download page, or SteamCMD, or to check subscription updates. Check before update; preview add, update, and remove with dry-run first. The player downloads page files and supplies from; the agent does not log in to LoversLab. Use `manage_library` for client Workshop items. Pin the old version before updating if it should be retained. |
| `manage_rules` | Ignore wrong-direction About.xml or community constraints to break `order.cycle`. Rules affect sorting and order findings but do not themselves reorder the plan. |

## 4. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| I4 | Connect Claude Code or Codex to MCP without loading skills. | Diagnose a patch failure and investigate a `Pawn.Tick` exception. Since 2026-10-30, the agent must read guide topic=diagnose itself. | The scripted client self-check in `spikes/mcpcheck` passes. A real agent session is pending, especially to check guide discovery after the context reduction. |
| I6 | Inspect the annotations. | `Tools()` marks read-only tools with `readOnlyHint: true`; the 12 destructive tools are listed in §2. Every tool guide has an example (`TestToolsAndAnnotations`). Test writes only in synthetic environments (`TestWriteTools`), test env (`TestEnvParameter`), and return still computing on analysis timeout (`TestStillComputing`). | Pass |
| I13 | Read the initial agent context (§3). | `Instructions` ≤2000 characters, each description ≤320, combined ≤12000; `tools/list` JSON ≤48 KB. Every listed topic can be fetched; an unknown topic sets `isError` (`TestInitialContextBudget` and `TestGuideTopics`). | Pass |
