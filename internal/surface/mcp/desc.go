package mcp

// Tool descriptions (05 §5.3): when to use each tool and what to use instead, how to write
// references, and an example call with the first lines of output. Together with
// Instructions they cover the error-debugging and assembly-inspector workflows without any
// skill (D7, I4).

// Instructions is sent to the client at initialisation.
const Instructions = `RimArcho knows one RimWorld installation: every mod instance, the plans (named enabled lists; one is the current plan), Defs, Patch simulation results, Harmony patches, Player.log clusters and decompiled C#. Every tool returns outline text; references in it (#id, L#id, pkg:, ws:, def:Type/name, p:<hex>, log:E<n>, r:<n>) can be passed straight to other tools.

Diagnosis workflow (use the exact evidence, not paraphrases):
1. Start from the error. If the user pasted log text, call analyze_log with text=…; otherwise analyze_log (current Player.log), then analyze_log cluster=E<n> for the cluster in question. Each cluster lists candidate mods with a role: source (its code/data produced the error), patcher (only has a Harmony patch on a frame), dependency (provides something that is missing or disabled), mentioned (appears in the message or a lower frame). Never call a patcher or a mentioned mod the cause without further evidence.
2. Confirm identity and state with get_mod: the exact instance (#id), whether it is enabled and at which position, its path, other copies in its lineage.
3. Classify the failure:
   - XML (PatchOperation failure, missing cross-reference, duplicate/missing parent def, malformed XML): use get_patches failed=true (reasons: target-inactive, target-missing, target-later, wrong-version-route, stale-name, unguarded-optional, node-absent), get_def, search_files.
   - Material/icon (_MainTex, ResolveIcon): XML-side first; search_files for uiIconColor without uiIconPath/texPath in the mods named.
   - Assembly (C# exception, Pawn.Tick, ThingWithComps.Tick, world tick): get_harmony target=<method> for every patched frame, then read_source type=<patch class> or search=<pattern>. For tick crashes check AllComps mutations in the cluster's candidates: read_source search="AllComps.Remove(" in=[…] (also "AllComps.Add(", "InitializeComps", "PostExposeData").
   - Mixed: confirm the XML side loaded (get_patches failed=true) before blaming code.
4. list_problems gives the static picture (dependencies, incompatibilities, load order, def overrides, patch failures, Harmony conflicts); explain_relation explains two specific mods.
5. Report: failure stage, XML/Assembly/Mixed, the exact mod instance and path, the file+line or class+method, why it fails, whether a named mod is cause, dependency or observer, and the smallest fix.
6. If it worked before: list_changes (mods that changed) and plan_history action=diff (what changed in the plan since a version).
Plans: every editing tool acts on the current plan. edit_loadout changes it (stored at once, every step undoable) and never touches the game; edit_loadout action=checkout goes back to a version from plan_history. Nothing reaches the game until it is written to the game: save_loadout writes the current plan without starting the game, launch_game writes it and starts the game (refused while any game runs). manage_plans lists, creates, switches (writes no game file), renames and removes plans.
Inventory: list_library lists every mod on this machine with its copies, sizes, states and the plans using it; manage_library moves a spare Mods/ copy into the lock library or takes mods out of every plan. manage_textures converts mods' PNG textures to DDS next to them (lighter loading and video memory; the PNGs stay) and takes them back.
Editing mod files: pin_mod first creates a locked version (a copy the current plan uses instead; never edit Workshop files in place — Steam overwrites them), then edit the copy.
Tools marked destructive (save_loadout, launch_game, manage_plans, manage_packs, resolve_identity, manage_hidden, manage_library, manage_textures, maintenance) write game or mod files, start the game or are hard to undo: show the dry run where there is one and ask the user before the real call (manage_plans: before action=remove).`

const descStatus = `Overview of the current environment: game version, installed mods per source, last scan, the current plan (and how many plans there are: manage_plans), its enabled count and how it differs from the list in the game (its newest version: plan_history), whether a game is running, and problem counts. Use first in a session; use get_mod for one mod and list_problems for the problems themselves. show= gives one read-only page of RimArcho's own setup instead: envs (environments — one per game folder — with their saves, launch mode and Data/Mods sources), folders (the mod folders every environment shares: Workshop, SteamCMD, other folders, Archo Monitor, the lock library), settings (config values), data (community data), db (database path, size, backups), cache (cache sizes). Changing environments and settings is left to the player in RimArcho.
Example: status {}
  env:main  RimWorld 1.6.4633 rev1261
    mods: 485 live (official 6, local 3, workshop 476), 0 tombstones
    lineages: 480 (5 with several instances)`

const descFind = `Find mods by name, packageId, author, workshop id or folder (query), or list mods (no query): active=true lists the enabled mods in load order, filter narrows the list (source:workshop, residue, unloadable, copies, pkg:<id>, ws:<id>). A packageId can map to several instances (workshop and local copies): the output lists every candidate with its ref. Use get_mod next with a ref from the output.
Example: find_mods {"query": "combat extended"}
  mods matching "combat extended": 2
    #a3f9  Combat Extended  [active 34/212, workshop ws:2890901044]
    #d10b  Combat Extended  [local CE_local]`

const descMod = `Everything about one mod instance: enabled state and position, package and lineage (other copies), declared dependencies/load order/incompatibilities, content counts (defs, patches, assemblies, Harmony patches), its current problems and its location on disk. Refs: #<id> (instance), L#<id> (lineage: ambiguous if several copies), pkg:<packageId>, ws:<workshopId>. An ambiguous ref returns the candidates instead of picking one.
Example: get_mod {"ref": "pkg:ceteam.combatextended"}
  #a3f9 Combat Extended  [active 34/212, workshop ws:2890901044]
    status: live
    package: ceteam.combatextended`

const descDef = `Who defines a Def, in load order, with file:line, which one is effective, and every PatchOperation that touched it (with hit counts or patch.no-match reasons). Use for missing cross-references ("No ThingDef named X"), duplicate defs and "which mod wins". With mod=<ref>: that mod's defs by type. def accepts Type/defName, Type@Name (abstract parent Name), def:… or a bare defName.
Example: get_def {"def": "ThingDef/Gun_Revolver"}
  def:ThingDef/Gun_Revolver
    defined-by (load order):
      1. #5k2q Core  Defs/ThingDefs_Misc/Weapons/RangedIndustrial.xml:120`

const descPatches = `PatchOperations and their simulated outcome against the merged Defs (same order and semantics as the game). failed=true lists operations that matched nothing with the reason: target-inactive (the def's mod is installed but disabled), target-missing (no installed mod defines it), target-later (created by a later patch), wrong-version-route (exists on disk outside the loaded version folder), stale-name (target mod active, def renamed; similar names given), unguarded-optional (not wrapped in FindMod/MayRequire/conditional folder), node-absent (def exists, inner path does not), exception. Use for "Patch operation … failed" log lines. mod=<ref> limits to one mod; target=Type/defName or an XPath part.
Example: get_patches {"failed": true}
  patches: 9 operations, 9 failed, 0 not simulated
    list:
      #ed2b Node Research  Patches/x.xml:40  Verse.PatchOperationAdd(Defs/ThingDef[defName="X"])  hit 0  (patch.no-match: target-inactive)`

const descHarmony = `Harmony patches read from mod assemblies (no game run needed): target=<Type.Method> lists every prefix/postfix/transpiler/finalizer on that method with the owning mod and patch class (bool prefixes can skip the original); mod=<ref> lists a mod's assemblies and patches (and says when it patches from code with Harmony.Patch, which cannot be resolved statically); shared=true lists methods several mods patch. Use for stack traces with "- PREFIX/POSTFIX owner:" lines and for "which mods touch Pawn.Tick". Then read_source with the patch class to see the code.
Example: get_harmony {"target": "Verse.Pawn.Tick"}
  harmony:Verse.Pawn.Tick  2 patches from 2 mods
    prefixes:
      #56g5 PA's God Hands  GodHandMod.Patch_Pawn_Tick_Freeze.Prefix  (returns bool: may skip the original)`

const descWhy = `Explain how two mods relate: positions, declared dependencies / load order / incompatibility in both directions, the sort constraints between them (and whether the current order violates them), defs both define, patches of one on the other's defs, and problems they share. Use when a problem names two mods or the user asks "why does A need to load after B".
Example: explain_relation {"a": "#i4uc", "b": "#iqmu"}
  #i4uc Over One ↔ #iqmu Over Two
    positions: enabled at 2 / enabled at 4
    declared:`

const descProblems = `Current problems: declarations (dep.missing, dep.inactive, incompat.active, order.violated, order.cycle, official.order, pkg.duplicate-*, version.unsupported, mod.unloadable), Defs (def.parent-missing, def.abstract-collision, def.override), Patches (patch.no-match, patch.unknown-op, patch.contested), Harmony (harmony.multi-transpiler, harmony.skip-prefix, harmony.shared-target, harmony.manual) and the selected log (log.*). Default: warning and above, version.unsupported folded into one row, def.override/patch.contested hidden; min_severity=info or all=true shows them. Each problem has a p:<hex> ref: ref=<p:…> shows its evidence and fix commands. code takes a prefix (dep, patch, harmony). The Def/Patch/Harmony layers are rebuilt automatically when the Loadout changed (a few seconds).
Example: list_problems {"code": "dep"}
  problems: 1 error, 0 warning, 0 info
    analysis: a:1 2026-10-19 07:21
    list:`

const descChanges = `What changed in the mods, one row per mod: its change events merged, with a sentence built from what the scans recorded (supported versions added or dropped, the mod's version, dependencies and incompatibilities added or removed with their installed/enabled state, load rules, files changed grouped as code / Defs / Patches / About / textures / sounds / translations, the Workshop update time, a locked version left behind). Rows are tiered: "affects launch" (a new dependency not installed or enabled, dropped support for the game version, a new incompatibility with an enabled mod, a package id change), "worth a look", "content only". Default: the current plan's mods since that plan was last played (the last scan when it never was); library=true: every mod; since=scan:<n>|<days>d|YYYY-MM-DD; ref=<mod>: that mod's Lineage, every change by day. New and gone mods are one summary group; older changes follow under "earlier", by day. Use when the user says "it worked yesterday" to find what changed.
Example: list_changes {"library": true}
  changes: 2 mods (library, since you last played · Oct 17)
    affects launch:
      #a3f9  Combat Extended  needs Harmony now (not enabled)
    content only:
      #51aa  Yayo's Combat 3  only textures changed`

const descLog = `Attribute Player.log errors to mods. Without cluster: the current Player.log (1.7+: Logs/Player.log; older: the user data folder; prev=true for Player-prev.log) as clusters log:E<n> with class (exception, harmony-stack, xml-load, patch-failed, xref-missing, material, other), count and the main candidate. cluster=E<n>: message, top frames with their Harmony patch lines, every candidate with role (source / patcher / dependency / mentioned) and evidence, confidence, a checklist for hot spots (Pawn.Tick, ThingWithComps.Tick) and next tool calls. text=<pasted stack trace or log excerpt> analyses that instead — use it whenever the user pastes an error. Patch failures are matched to the simulation (file, line, reason); cross-reference errors list referencing files and whether any provider exists.
Example: analyze_log {"cluster": "E3"}
  log:E3 exception x512  first at Player.log:4021
    message: ArgumentOutOfRangeException in List` + "`" + `1.get_Item
    top-frames:`

const descFiles = `Search the effective content (the version folders and LoadFolders branches the game actually loads, Languages excluded) of the enabled mods for text: defNames, XPath targets, uiIconColor, class names in XML. mod=<ref> limits to one mod; regex=true for a regular expression. Use for XML evidence; use read_source for C#.
Example: search_files {"pattern": "Reload_Standard"}
  files search "Reload_Standard": 2 matches in 4590 files of 482 mods
    matches:
      #mj2v  1.6/Core/Defs/ThingDefs/Apparel/Apparel.xml:463  <soundReload>Reload_Standard</soundReload>`

const descSource = `Decompiled C# of mod assemblies (ilspycmd, run on demand and cached per DLL hash). ref=<mod> type=<Namespace.Type> shows one type (a Harmony patch class from get_harmony, a frame type from analyze_log); ref alone decompiles the mod's assemblies; search=<text> searches decompiled code: in=[refs] names the mods; without it the search covers the mods the newest log's error clusters and the Harmony warnings implicate; scope="active" searches every enabled mod (hundreds of assemblies, many minutes — only when the user asks). A call spends at most budget seconds decompiling and says how many assemblies are still pending — call again to continue. Prefer short concrete patterns: "AllComps.Remove(", "HarmonyPatch(typeof(Pawn), \"Tick\")", "WorldPawnsTick". If ilspycmd is missing the text says how to install it; nothing is installed automatically.
Example: read_source {"ref": "#ipv5", "type": "yayoCombat.HarmonyPatches.Pawn_Tick"}
  #ipv5 Yayo's Combat 3  yayoCombat.HarmonyPatches.Pawn_Tick  (27 lines)
    file: …\cache\source\8612…\yayoCombat.HarmonyPatches\Pawn_Tick.cs
    source:`

const descEdit = `Change the current plan (stored at once, every step undoable; no game file is written): action=enable|disable (refs), move (refs[0] with at / before / after / up / down), swap (refs[0] → another instance of its lineage, optional refs[1]), sort (dry_run=true previews), undo, redo, checkout (version=v<n> from plan_history: load that version into the plan as one edit); staging (the plan's own side list, 06 §2.5): stage (refs leave the plan and remember their place), unstage (out of staging, stays disabled), restore (refs, or all when empty, go back to their old place; at / before / after for one mod); ignore_external drops the "external version not loaded" mark that status shows after ModsConfig.xml changed outside RimArcho (the version stays in the history). Nothing reaches the game until save_loadout or launch_game writes the plan to the game. Another plan: manage_plans action=use first. Refs as in get_mod.
Example: edit_loadout {"action": "checkout", "version": "v1"}
  workspace: 3 active, saved (plan:default, env:main)
    checked out v1 into the workspace (archo save writes it to the game)
    added  #hnyg  Harmony  at 2`

const descSave = `Write the current plan to the game without starting it ("写入游戏", 08 §2), for players who start RimWorld some other way (Steam): sync RimArcho's _Archo_ links in Mods/ and remove pack links, then write ModsConfig.xml (backup first, atomic replace) for the current game version, and record the list as a version of the plan. It changes the list the game reads next: ask the user before calling it, and call with dry_run=true first to show the preview (links, path, version rewrite, entries, changes). Refused while ModsConfig.xml holds an external change that was not loaded: the text gives the commands to see the difference, load it, or ignore it (edit_loadout action=ignore_external). While the game runs it still writes and says the game reads the new list on its next start. launch_game writes the same way and starts the game.
Example: save_loadout {"dry_run": true}
  save (dry run): 2 changes
    added  #b21c  Harmony  at 1
    moved  #a3f9  Combat Extended  34 → 35`

const descCopy = `Old name of pin_mod (same arguments except update, move_to_library and dry_run; same behaviour): lock a mod by copying it into RimArcho's lock library and using the copy in the current plan. Prefer pin_mod.
Example: copy_local {"ref": "#ipv5", "git": true}
  #ipv5 Yayo's Combat 3 locked: copied to C:\Users\me\AppData\Local\RimArcho\locked\main\2854006492_local
    copy: #k2v8  (same lineage, origin marker .rimarcho/origin.json)
    files: 130 (1.5 MB)`

const descPin = `Create a locked version of a mod (usually a Workshop item) before editing it, or so Workshop updates stop reaching it: the mod is copied into RimArcho's lock library (outside the game folder; src:locked) with .rimarcho/origin.json (origin instance, lineage, workshop id, snapshot), stays in the same lineage, and replaces the original in the current plan only (one undoable edit; nothing is swapped when the original is not enabled). The game sees the copy only through an _Archo_ link in Mods/ that save_loadout or a launch creates, so several copies can coexist and each plan uses its own. git=true adds a repository with a baseline commit so every edit can be diffed. Links that point outside the mod folder and the upstream .git are not copied; an existing folder is never overwritten (a taken name gets a suffix). update=true remakes it when list_problems reports mod.pinned "has been updated": shows the changed files, copies the upstream's current content to a new folder and swaps it in; the old copy stays. move_to_library=true moves a copy RimArcho made in Mods/ before the library existed into the library (update does this on its own). Undo with unpin_mod. Never edit Workshop files in place — Steam overwrites them.
Example: pin_mod {"ref": "#ipv5", "git": true}
  #ipv5 Yayo's Combat 3 locked: copied to C:\Users\me\AppData\Local\RimArcho\locked\main\2854006492_local
    copy: #k2v8  (same lineage, origin marker .rimarcho/origin.json)
    files: 130 (1.5 MB)`

const descUnpin = `Unlock a locked version: the current plan uses the upstream (usually the Workshop copy) again instead of the locked copy; the copy stays in the lock library. ref may name the copy or the upstream. For a lock made by another manager (RimCrow: the Workshop folder's About.xml renamed to About.xml.disabled), About.xml is renamed back first. Only the current plan changes; call with dry_run=true to preview.
Example: unpin_mod {"ref": "#k2v8"}
  unpinned: the current plan uses #ipv5 instead of #k2v8 Yayo's Combat 3
    copy: stays at C:\Users\me\AppData\Local\RimArcho\locked\main\2854006492_local (no files change)
    upstream: #ipv5 at D:\Steam\steamapps\workshop\content\294100\2854006492`

const descDescribe = `A mod's description: About.xml and the Steam Workshop page (BBCode rendered as plain text; images as [image: url]). The Workshop text is fetched for this one mod on demand and cached until the item is updated; offline=true uses the cache only, and the user can turn fetching off (config workshop.fetch). source=auto shows the Workshop text when About.xml is short or the same, both otherwise. Use it to learn what a mod does before suggesting it.
Example: get_description {"ref": "#uuhe"}
  description of #uuhe Vehicle Framework
    workshop: 3014915404 (fetched, updated 2026-10-02)
    Workshop page:
      Vehicle Framework
      [image: https://i.imgur.com/abc.png]
      Adds vehicles to RimWorld …`

const descPlayer = `Change the player's own data on mods (shared by all copies of a mod): action=tag_add / tag_remove (tags on one or more refs; tags are what list --filter tag:<t> and the TUI's tag: search use), alias (the name RimArcho shows; text "" clears), note (free text; "" clears), copy_note (a note on one copy only, e.g. why this locked version exists; list_library lineage=… shows it, and RimArcho's own first version until one is written). Nothing reaches the game.
Example: set_player_data {"action": "tag_add", "refs": ["#uuhe", "#vpcd"], "tags": ["vehicles"]}
  tagged 2 mods: vehicles`

const descUninstall = `Preview what uninstalling RimArcho would do (archo uninstall --dry-run): every step with its folders and sizes — RimArcho's links in the games' Mods/ deleted (never their targets), the ModsConfig.xml lists it wrote cleaned of Archo Monitor and mods only reached through a removed link (restore_modsconfig_backup: the backup put back), the lock library's copies (default recycle bin; keep_locked: into a game's Mods/ as ordinary mods, locked_to picks the environment), Archo Monitor, packs (default kept; remove_packs), the environments' own save folders (default moved out to Documents; remove_saves), game folders downloaded with SteamCMD (default kept; remove_games), RimArcho's SteamCMD, then its data folders and the program once it has closed. Read-only: nothing changes. There is no tool that uninstalls — removing RimArcho is the player's decision alone; to go ahead they run archo uninstall --yes themselves or use 设置 › 卸载 RimArcho in the window. Use this to answer "what would uninstalling remove or keep?".
Example: preview_uninstall {"keep_locked":true}
  uninstall: dry run, nothing changed
    links: delete 2 links in the games' Mods/ (never their targets)
      unlink      C:\Games\RimWorld\Mods\_Archo_locked_1001_local → C:\Users\me\AppData\Local\RimArcho\locked\1001_local
    locked: 1 lock library copy into env:main's Mods/ as ordinary mods, 12.0 KB
      move        C:\Users\me\AppData\Local\RimArcho\locked\1001_local → C:\Games\RimWorld\Mods\1001_local  (A)  12.0 KB
    data: once RimArcho has closed, delete its data: database, settings, caches, the Steam runtime folder (2 folders), 48.3 MB
  next:
    archo uninstall --keep-locked --yes`
