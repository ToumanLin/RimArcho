package mcp

// What a client sees when it connects (05 §5.3): short Instructions and one short
// description per tool, enough to pick the right tool. The full references, the diagnosis
// workflow and the error-kind checklist are guide topics (internal/cmds/guide*.go), read
// when needed. TestInitialContextBudget keeps this small.

// Instructions is sent to the client at initialisation.
const Instructions = `RimArcho knows one RimWorld installation: every mod instance, the plans (named mod lists; one is current), Defs, simulated Patches, Harmony patches, Player.log error clusters and decompiled C#. Tools return outline text; its references (#id, L#id, pkg:, ws:, def:Type/name, p:<hex>, log:E<n>, r:<n>, pattern:<id>) pass straight to other tools. Results are capped at 24 KB: narrow with mod=, target=, a cluster or a pattern instead of all=true.
Start with status. For a game error or crash, read guide topic=diagnose before concluding: it says how to classify the error, which tool gives which evidence and how to report. In short: analyze_log (text= for a pasted log), then analyze_log cluster=E<n>; a patcher or a mentioned mod is not the cause without further evidence, and with no source the cause is undecided.
Editing tools act on the current plan and never touch the game until save_loadout or launch_game. Never edit Workshop files in place: pin_mod first, then edit the copy.
Tools marked destructive (save_loadout, launch_game, manage_plans, manage_packs, resolve_identity, manage_hidden, manage_library, manage_subscriptions, manage_textures, repair_links, maintenance) write game or mod files, start the game or are hard to undo: show the dry run where there is one and ask the player before the real call.
The player's management tools (packs, subscriptions, library actions, textures, memory estimate, maintenance, identity, hidden mods, links, player data, uninstall preview) are served only by archo mcp --tools all; otherwise use the archo CLI (guide topic=<tool name> gives the command).
guide topic=<tool name> gives a tool's full reference (every action, parameter details, an example call and output); guide alone lists the other topics: errors (kinds the workflow does not cover), plans, words (what the interface calls things, for talking to the player).`

const descGuide = `Read-only help: a tool's full reference (topic=<tool name>: every action, parameter details, an example call and output) or a workflow (diagnose, errors, plans, words). No topic lists them.`

const descStatus = `Overview of the current environment: game version, mods per source, the current plan and how it differs from the game's list, whether a game runs, problem counts, pending updates. Call first in a session. show=envs|folders|settings|data|db|cache gives one read-only page of RimArcho's setup instead.`

const descModSettings = `Mod settings, read only. No ref: the current list's Mods that have settings (window title, file, or defaults when there is none yet). ref: what one Mod's files and HugsLib packs store (a missing value is at its default; lists summarised), plus window labels and HugsLib titles and defaults as Archo Monitor saw them.`

const descModSettingsSet = `Change a Mod's simple settings values with the game closed: on/off, numbers, one line of text, a HugsLib choice; reset removes a stored value. Config files: only values the file stores. Backs the file up to <file>.rimarcho.bak first. Next: get_mod_settings.`

const descFind = `Find mods by name, packageId, author, workshop id or folder (query), or list them (no query; active=true: enabled mods in load order). A packageId can match several copies: each candidate is listed with its ref. Next: get_mod.`

const descMod = `Everything about one mod instance: enabled state and position, lineage (other copies), declared dependencies / load order / incompatibilities, content counts, its problems, its path. Refs: #<id>, L#<id>, pkg:<packageId>, ws:<workshopId>; an ambiguous ref returns the candidates.`

const descDescribe = `A mod's description from About.xml and its Steam Workshop page (fetched on demand, cached). Use it to learn what a mod does before suggesting it.`

const descDef = `Who defines a Def (load order, file:line), which definition wins, and every patch that touched it. For "No ThingDef named X", duplicate defs and "which mod wins"; mod=<ref> lists a mod's defs.`

const descPatches = `PatchOperations and their simulated outcome (same order and semantics as the game). failed=true lists operations that matched nothing, with the reason. For "Patch operation … failed" log lines.`

const descHarmony = `Harmony patches read from mod assemblies: target=<Type.Method> lists every patch on that method with its owner; mod=<ref> one mod's patches; shared=true methods several mods patch. For stack frames with "- PREFIX/POSTFIX owner:" lines; then read_source on the patch class.`

const descWhy = `How two mods relate: positions, declared dependencies / order / incompatibility both ways, sort constraints and violations, shared defs, patches of one on the other, shared problems. For "why must A load after B".`

const descProblems = `Static problems of the current plan: dependencies, incompatibilities, load order, def overrides, patch failures, Harmony conflicts, the log. Warning and above by default; code takes a prefix (dep, patch, harmony); ref=<p:…> shows one problem with its evidence and fix commands.`

const descChanges = `What changed in the mods since the current plan was last played (or since=…), one row per mod, the changes that affect launch first. For "it worked yesterday"; ref=<mod> gives one mod's history.`

const descLog = `Attribute Player.log errors to mods: clusters log:E<n> with candidate mods and their roles. text= analyses a pasted log or stack trace (use it whenever the user pastes one); cluster=E<n> shows one cluster in full; since=<cursor> polls a running game; pattern=<id> explains an error kind.`

const descFiles = `Text search in the files of the enabled mods as the game loads them (Defs/Patches/About by default, load order, shadowing). def=true finds a whole defName with its roles: defines, inherits, patches, references. For XML evidence; C# is read_source. Paged: follow next:.`

const descSource = `C# in the Source library, decompiled on demand. ref with type reads one type; game=true selects game code or adds it to search. search=<text> searches log suspects or in=[refs]; shared code appears once with its copies. scope="active" (all enabled mods, many minutes) only when the user asks.`

const descEdit = `Change the current plan: enable, disable, move, swap (another copy), sort, undo, redo, checkout (a version), stage / unstage / restore, ignore_external. Stored at once and undoable; no game file is written until save_loadout or launch_game.`

const descSave = `Save the current plan to the game (ModsConfig.xml and RimArcho's links in Mods/) without starting it. Changes the list the game loads next: call with dry_run=true first, show the player and ask before the real call.`

const descLaunch = `Save the current plan like save_loadout, then start the game. Refused while any game runs. Starts a program and changes the game's list: call with dry_run=true first, show the player and ask.`

const descTestRun = `Start the game with a plan in isolated test data, wait for the main menu (map=true: a quick test map), capture startup errors, stop it, return a JSON report. Compares with the player's log (before, fixed) and the previous run (delta, gone). Takes minutes; never touches the player's saves, config, logs or game folder.`

const descPlans = `The environment's plans (named mod lists): list, new, use (switch; writes no game file), rename, remove, import, versions, follow / unfollow / sync a Workshop collection. remove deletes a plan with its whole history: ask the player first, naming the plan.`

const descHistory = `The current plan's history (read-only): log lists its versions (lists saved to or read from the game), diff compares two versions or a version with the current list, steps shows the undo steps. Go back with edit_loadout action=checkout.`

const descPin = `Lock a version of a mod: copy it into RimArcho's lock library and use the copy in the current plan (one undoable edit), so Workshop updates stop reaching it. Do this before editing a mod's files. update=true remakes it from the updated upstream; dry_run=true previews.`

const descUnpin = `Unlock a locked version: the current plan uses the upstream (usually Workshop) copy again; the locked copy stays. dry_run=true previews.`

const descPlayer = `The player's own data on mods, in RimArcho's database: tags, alias and note (shared by every copy of a mod), copy_note (one copy's own note). Nothing reaches the game.`

const descRules = `User load-order rules: add (after, before, incompatible, attach, top, bottom, or ignore a wrong About.xml / community constraint — how an order.cycle is broken), list, remove. Rules guide sort and the order problems; they do not reorder by themselves.`

const descIgnore = `Ignore a problem (p:<hex>) until its mod's files change; undo=true stops ignoring it. Only after the player decided the problem is fine: ask first.`

const descVersionMute = `Ignore or restore (undo=true) mods' unsupported-game-version warnings for one game minor version, across all plans and games. Only when the player chooses to.`

const descScan = `Rescan the mod folders and reconcile them with RimArcho's records (new, updated, moved and vanished mods become change events). Use after the player changed mods outside RimArcho; full=true re-hashes everything (slow).`

const descExchange = `Exchange the current plan with list files and Workshop collections: export (several formats), import (replaces the plan; one undoable edit), collection (compare with, or import, a Workshop collection). Ask before replacing the player's plan.`

const descPacks = `Packs: a plan compiled into a self-contained folder with its own mod copies, list and saves. list, check (read-only), build, launch, export, import, remove, stop, unlink. build copies gigabytes, launch starts the game, remove deletes: dry_run=true first and ask.`

const descIdentity = `RimArcho's identity of mods: answer open questions (is this new folder the mod that vanished? same, copy or unrelated), split or merge lineages. Answers rewrite history: quote the comparison from action=list or why and ask the player.`

const descHidden = `Mods another manager hid (About.xml renamed to About.xml.disabled): unhide renames it back; trash moves a hidden or unsubscribed Workshop folder to the recycle bin. Changes mod folders: dry_run=true first and ask.`

const descMaintenance = `RimArcho's own data: data_update (downloads community data), import_rimcrow, vacuum (compacts the database), gc (drops expired records and their history). dry_run=true first, show the player, then ask.`

const descCheckLinks = `Read fresh health and ownership evidence of RimArcho's links in Mods/ for the selected game, or every known game (JSON). Changes no files.`

const descRepairLinks = `Repair RimArcho's links in Mods/: remove eligible links or finish an interrupted operation; never deletes link targets. dry_run=true previews; the real call needs confirm=true after the player approves, and unproven removals need include paths.`

const descLibrary = `The mod library: every mod on this machine, one line per lineage with its copies, size, state and the plans using it. Filter with search, include, exclude; lineage=<ref> shows every copy. For "what do I have, where, used where" and cleanup.`

const descManageLibrary = `Mod library actions: move_to_locked (a spare Mods/ copy into the lock library), remove_from_plans, unsubscribe / subscribe / resubscribe Workshop items through Steam, steam_state (read-only). Moves or deletes folders: dry_run=true first and ask.`

const descManageSubscriptions = `Mods RimArcho installs and updates itself (Git repositories, download pages, Workshop items via SteamCMD): list, add url=…, check (finds updates, installs nothing), update, remove. dry_run=true first for add, update and remove, and ask. Items the Steam client keeps: manage_library.`

const descTextures = `Convert mods' PNG textures to DDS next to them for lighter loading and less video memory (the PNGs stay): status (read-only estimate), optimize, clean. optimize: dry_run=true first and ask; refused while a game runs.`

const descUninstall = `Preview what uninstalling RimArcho would delete, move or keep (read-only). No tool uninstalls: that is the player's decision alone.`

const descMemory = `Estimate resident RAM, loading RAM and VRAM for a plan. Available physical RAM below loading low is severe, below high warning, otherwise ok; unknown stays unknown. VRAM shortages warn only. Unity 1.6 RAM is VM-calibrated. Reads headers and caches estimates. Limits: guide estimate_memory.`
