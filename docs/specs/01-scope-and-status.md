# 01 · Scope and status

> Status: rewritten on 2026-10-21; updated for the new 1.0 scope on 2026-10-22. What each version includes and excludes, which files RimArcho writes, and what remains unfinished.
> Basis: 00 (positioning, three questions, principles), 00a (architecture). Each feature's own specification governs its details.

## 1. Version Scope

| Version | Question answered | Contents |
| --- | --- | --- |
| 1.0 | What am I playing now, and can it run properly? | Implemented foundation: environments and identity (02), analysis and log attribution (03), plan editing and sorting (04), CLI and MCP (05), packs v1 and link deployment (08), live game log tracking (06 §6), list exchange (07 §7), community data channel (08 §2), hidden Mods (09 §5), settings page and history area (06). **1.0 work from the new design (implemented in `dev/0.9.0`)**: multiple plans per environment (07 §1), writing on launch and running only one game at a time (09 §1, §2), attributing outside changes to the writer (07 §4.3), locked versions (07 §5), sidebar interface and first-run guide (16 §2), completing the tool layer (13 §1). **Added to 1.0 on 2026-10-21/22 (implemented)**: Mod inventory (16 §2.9), one game directory per environment and shared Mod folders (03 §2), downloading game versions with SteamCMD (11 §3), unsubscribing and resubscribing through Steam (11 §2), Archo Monitor companion Mod (09 §6), change history explaining the impact per Mod (16 §2.9), launch page (16 §2.3), shared filter bar, keys, and context menus for the editor and Mod inventory (16 §3), uninstall (02 §2), reducing memory use (16 §10). **Log corpus integrated into 0.9 (implemented)**: corpus-based severity and constraint strength (05 §4.3.1, 08 §1.3), pre-launch notices (09 §2), known dependencies and combinations in the log corpus layer (05 §4.3), manual Harmony patch targets (05 §5.3), nearby messages and error pattern library in cluster details (06 §3, §8) |
| 1.1 | What have I not played? | Save reading and content catalog (12 §1, §2), untouched content (12 §3), save management (12 §4), built-in AI assistant Archo (09) |
| 1.2 | What have I not played? (continued) | Subscribe and unsubscribe to Workshop collections in one action (07 §7), playthrough themes and content conflicts (12 §5) |
| Long term | What is worth playing? | Player-shared packs (the sharing website is a separate project), recommendations based on understanding player content and play goals (12 §6) |
| 1.x | — | Targets in manual `Harmony.Patch` calls resolved only at runtime (scanning methods across the game, collections built by helper methods, `Assembly.GetType`; statically resolvable targets were implemented in 0.9, 05 §5.3), storing conditional compatibility patches as facts; auditing AllComps additions/removals, icons and materials; static cross-references using Def field types in Assembly-CSharp metadata; macOS and Linux (only compilation has been verified). (The in-game helper Mod was brought forward to 0.9: Archo Monitor, 09 §6) |

## 2. Capabilities

Listed by who can do what; status refers to the current code.

### 2.1 Players

| Capability | Specification | Status |
| --- | --- | --- |
| Automatically find game, user data, and Workshop directories; specify them manually when detection fails | 03 §3 | Implemented (GUI first-run guide implemented, 16 §2.15) |
| One environment is one game directory; Workshop, SteamCMD, other Mod folders, Monitor, and the lock library are shared across all environments; use shared default saves or own saves | 03 §2 | Implemented |
| Download a game version with SteamCMD as a new environment | 11 §3 | Implemented (real login and download await validation, §4.2) |
| Switch, create, rename, and delete multiple plans; all games share plans (2026-10-25) | 07 §1, §1.7 | Implemented |
| See enabled state, order, and findings in one list; enable, disable, move, and stage | 07 §2, 16 §2.5 | Implemented |
| Sort in one action; explain conflicting constraints when sorting fails | 08 §1 | Implemented |
| See missing dependencies, incompatibilities, unsupported versions, duplicate package IDs, and patches with no match, with remedies | 05 §4 | Implemented |
| Attribute errors to Mods with evidence | 06 | Implemented |
| Preserve lists and rules through Workshop updates, author renames, and package ID changes, and see the changes | 04 §2, 16 §2.9 | Implemented |
| Undo, or return to a plan's historical version | 07 §3 | Implemented |
| Preserve identities, rules, and history after changing the game path | 03 §4.2 | Implemented |
| Lock a Mod version against Workshop updates | 07 §5 | Implemented (lock library, 07 §5) |
| Import lists from RimCrow, RimSort, Steam collections, etc., and export them for others | 07 §6, §7 | Implemented |
| Launch a plan, writing it to the game on launch | 09 §2 | Implemented |
| Build a plan into a pack and play with separate saves | 10 | Implemented |
| See new errors live while the game runs | 06 §6 | Implemented |
| Handle hidden Mods (About.xml.disabled) and unsubscribed residue | 09 §5 | Implemented |
| See all owned Mods on one page: copies per Lineage, locations, sizes, and usage; filter and clean up | 16 §2.9 | Implemented (GUI mod library › all; CLI `library`; MCP `list_library`, `manage_library`; TUI list and details) |
| Unsubscribe and resubscribe to Workshop items through Steam | 11 §2 | Implemented (real calls await validation, §4.2) |
| See which Mods of the current list have settings and what their settings files and HugsLib packs store, and change simple values | 16 §2.9 | Implemented (GUI mod library › Mod settings, with controls for simple values; CLI `mod-settings`, `mod-settings set`; MCP `get_mod_settings`, `set_mod_settings`; TUI panel, read only). Real-game check in the release VM tests |
| See what a Mod's changes mean for the player (affects launch / watch / content only), scoped by default to the current plan since the last play session | 16 §2.9 | Implemented |
| Record live logs through Archo Monitor while the game runs | 09 §6, 06 §7 | Implemented |
| Do everything by keyboard that can be done by mouse (shared keys and context menus for the editor and Mod inventory) | 16 §3.1 | Implemented |
| Uninstall RimArcho and reverse its changes | 02 §2 | Implemented (validation on an installed release pending, §4.2) |
| Subscribe to Mods in Git repositories and download pages (LoversLab: use Git when a gitgud.io repository exists; otherwise players download the zip and install it), check and pull updates; download Workshop items with SteamCMD when the Steam client is unavailable | 11 §1, 16 §2.9 | Implemented (GUI, CLI, TUI, MCP `manage_subscriptions`; TUI subscribes to a download page only through its single Git repository; choose zip / folders and LoversLab site files in the window or use `--from`; real SteamCMD Workshop downloads await validation, §4.2) |
| See Mods and content never played; manage saves | 10 | Not implemented (1.1) |

### 2.2 AI Assistants

| Capability | Specification | Status |
| --- | --- | --- |
| Query facts and conflicts for Mods, Defs, Patches, and Harmony | 14 | Implemented |
| Given a stack trace, identify the Mods owning Harmony patches and the next steps | 06, 14 §3 | Implemented |
| Decompile an individual Mod on demand and search its source | 05 §5.4 | Implemented |
| A corresponding tool for every player action | 13 §1 | Implemented (completed in step 7; uninstall has only a read-only preview, 02 §2; live log `since` cursor completed in step 13, 06 §6; subscription mutation tool `manage_subscriptions` completed in step 17) |
| Built-in Archo: conversations and one-action tasks | 09 | Not implemented (1.1) |

## 3. Exclusions

- A general Workshop browser or Workshop search. "Worth playing" starts only from Mods the player owns and packs validated by other players (00 §2).
- (Correction on 2026-10-24: Git repository subscriptions and SteamCMD Workshop downloads when the Steam client is unavailable are now in scope, 11 §1; Workshop items still prefer the Steam client.) Downloading **game versions** through SteamCMD is in scope (1.0, 11 §3): players log in through SteamCMD's own window; RimArcho gives it only the account name.
- Updating RimArcho without confirmation (since 2026-10-31, checks for new versions are automatic; download and replacement require player confirmation, 17 §5.1); resident background processes; opening any network port (MCP uses only stdio).
- Sleep screen and recommendation list export (RimCrow has these; RimArcho does not).
- A Mod acquisition hub: Workshop search (Git repository subscriptions moved into scope on 2026-10-24, 11 §1) (the other three pages in RimCrow's "inventory hub"). Managing local Mods belongs to the Mod inventory page (16 §2.9) and is in scope. (Correction on 2026-10-21: the original "no inventory hub" applied to the MVP scope; the specification rewrite incorrectly made it a permanent exclusion.)
- Buying a code-signing certificate. README explains the SmartScreen prompt on first run.
- Approving permissions for external agents: their harness decides whether to allow writes; tools carry MCP `readOnlyHint` / `destructiveHint` annotations (14 §2). Built-in Archo uses RimArcho's own confirmation for writes (15 §2).

## 4. Pending Work and Validation

### 4.1 1.0 Work to Implement (New Design)

**Release version numbers** (decided on 2026-10-21). The 1.0 / 1.1 / 1.2 in §1 are product versions; code release numbers follow semantic versioning (`scripts/release.ps1 -Version`, CHANGELOG):

| Release | Contents | Condition |
| --- | --- | --- |
| 0.8.0 | Existing code (CHANGELOG 0.8.0 section), finalized as it stands | — |
| 0.9.0 | Steps 1–8 and 10–23 below: all 1.0 work | Steps complete, `release.ps1` tests pass, pre-release VM test checklist complete (17 §5) |
| 1.0.0-rc1 | 0.9.0 plus §4.2 validation and decisions on §4.3 remaining items | Conclusions for every §4.2 item |
| 1.0.0 | Answer the first question (00 §2) | No new blockers during rc |

**Released** (2026-11-02): the owner released **1.0.0** directly from `dev/1.0`, without the separate 0.9.0 and 1.0.0-rc1 releases and before the pre-release VM checklist (17 §5) was run; its 88 rows stay in [release VM tests](../release-vm-tests.md) for the next round, and fixes go into 1.0.x. CHANGELOG lists 0.5.0–0.9.0 as development milestones included in 1.0.0.

0.9.0 has breaking changes: database schema v10 → v27 (v11 plans, v12 analysis cache per plan, v13 copy notes, v14 shared Mod folders, 03 §2; v15 Def lookup index by `Name`, 06 §5; v16 DDS written by texture optimization, 10 §2; later migrations in 04 §4 and 05 §3, v26 facts shared across versions, latest v27 snapshot content types and languages; 1.0.0 shipped schema 29, and databases from these pre-1.0 builds are no longer upgraded, 04 §4), one game directory belongs to only one environment, remove `launch --force` and `loadout tag`, and make `archo save` only write to the game.

**Implementation steps** (dependency order; steps 2, 5, and 6 may run in parallel after step 1):

| # | Step | Dependencies | Specification |
| --- | --- | --- | --- |
| 1 | **Data model (schema v11)**: new `plan` table; store workspace, undo stack, versions, and staging per plan; environment records current plan; one `modsconfig_state` row per user data directory, recording writer; add plan, list fingerprint, and version to `launch`; migrate per 07 §1.4 (each name in `loadout_tag` becomes a plan, then drop table) | — | 07 §1.2, §1.4, 09 §1 |
| 2 | **Plan commands**: `plan list/new/use/rename/remove`, `loadout import --new`; remove `loadout tag`; other commands act on current plan | 1 | 07 §1.3, 13 §4.3 |
| 3 | **Write on launch and single game**: launch follows the 8 steps in 09 §2; remove "block launch if unsaved", "block launch on version mismatch", and `launch --force`; `archo save` writes without launching; combine `launch` and `save` `--dry-run` into one preview; launch records a version with source `launch`; detect game processes across all environments, allowing only one game at a time | 1 | 07 §4.1, 09 §1, §2, §4 |
| 4 | **Attribute outside changes to writer**: identify by user data directory, assign to the plan that last wrote it; three conflict choices: view diff `loadout diff v<n> workspace`, load `loadout checkout v<n>`, ignore `loadout ignore-external` | 1, 3 | 07 §4.3 |
| 5 | **Locked versions**: `pin` swaps in a copy in current plan; identify RimArcho-made copies as locked versions (`mod.pinned`), detect upstream updates using `origin_snapshot`; update locked version with `pin --update <instance>`; RimArcho-made copies can also `unpin` | 1 | 07 §5 |
| 6 | **Attribute logs to launch records**; cache multiple analysis results by each plan's enabled set | 1, 3 | 05 §3, 06 §4 |
| 7 | **Complete tool layer**: `launch_game`, `manage_plans`, `plan_history`, `pin_mod` / `unpin_mod`, `manage_packs`, `ignore_problem`, etc. | 2–5 | 13 §1.2, 14 §2 |
| 8 | **Interface**: sidebar, breadcrumb header, home, plan pages, first-run guide; consistent "save / unsaved" wording; TUI page mapping | 1, 3, 6 | 16 §2, §9 |
| 9 | **Release 0.9.0** (complete pre-release VM checklist first, then `release.ps1 -Version 0.9.0`); validate §4.2, decide §4.3, release 1.0.0-rc1 | 1–8, 10–23 | 17 §5 |
| 10 | **Features added to 1.0 on 2026-10-21/22**: Mod inventory and `library` command, copy notes (v13); one game directory per environment, shared Mod folders (v14); SteamCMD game downloads; Steam unsubscribe/resubscribe; Archo Monitor; change history rework; launch page; shared filter bar, keys, and context menus for editor and Mod inventory, keyboard parity with mouse; uninstall; reduced memory and GPU acceleration settings | 1–8 | 16 §2.3, §2.9, §3, §10, 03 §2, 09 §6–11 §3, 09 §4.1 |
| 11 | **Pack gaps** (complete, branch `claude/packs-gaps-72ff5e`): rearrange build page per mockup, disable "create a pack" with explanation when errors exist (G14); each pack is a sidebar child, `tab` switches, one page per pack; can it run reports pack links left by direct Steam launches and removes them in one action (K12, `pack.links-left`, `archo pack unlink`); **two copies with the same package ID in `Mods/`**: on pack launch, borrow an identical existing copy in `Mods/`, refuse differing content, and temporarily remove same-package `_Archo_` plan links; writing to game does not create duplicate `_Archo_` links; can it run lists existing duplicates (`pkg.duplicate-mods`, K15–K17); recycle bin entry point checks allowlist centrally (H8, `App.ToTrash`) | 10 | 16 §2.6, §2.7, 09 §3, 10 §3, 02 §3 |
| 12 | **Interface gaps** (**complete**, 2026-10-22): evidence chips in TUI can it run (G13); repeated `esc` on new pages returns to base state (G5, T20); game running state at sidebar bottom (G17, right end of status line in TUI) | 10 | 16 §2.4, §11 |
| 13 | **Log gaps** (complete, 2026-10-22): live MCP log `since` cursor (L7); first query for a new log 568 → ≤ 500 ms (index by defName; corpus logs measured at 128–330 ms) | 10 | 06 §5, §6, 18 |
| 14 | **Background tasks** (decided on 2026-10-23): task records and two queues, file and Steam (`app.Tasks`, `<data-directory>/tasks.json`); enqueue Mod library file/Steam actions after confirmation, continue after individual failures, record per-item results; also record scans, checks, pack creation, textures, and game downloads as tasks; background tasks page (one row at sidebar bottom, `ctrl+j`); implement 16 §3.1 rules 7, 8 (dialogs only for warnings, collapse filter bar on focus loss), 16 §3.9 tables, no left vertical bars in GUI | 10 | 16 §2.16, §3.1, §3.9, §7 |
| 15 | **Apply interface rules** (decided on 2026-10-23; **complete**, branch `codex/ui-rules`; acceptance G21 still needs a real large game and native window): background work does not block foreground (adding/switching large games, sizes, settings downloads/database actions); enter every page before reading; no dialogs for new lists, imports, manifests, loading history; remove left color bars from can it run and TUI; remove purpose explanations from page headers; remove `·` from UI text; two corner radii, four font sizes no smaller than 12px, hairlines, animation durations; pack, pack Mod, and texture lists become data tables; `TestStyleRules` and `TestWording` checks | 14 | 16 §3.1 rules 7–10, 16 §3.9, §7.3, acceptance G21–G25; places marked **to change** in 06 |
| 16 | **GUI back/forward navigation** (2026-10-24, implemented): session history, mouse side buttons, keyboard entry points; TUI differences in 16 §8.5 | 15 | 16 §3.1 rule 11, 16 §3.5, §8.5, acceptance G26 |
| 17 | **Mod subscriptions** (2026-10-24, branch `claude/mod-sources-ceb9a8`): Git repositories (go-git, tracking branch / tag / commit), download pages (LoversLab, no login or automatic download), SteamCMD Workshop item downloads (only without Steam client; SteamCMD location comes from settings dependency management); origin in `.rimarcho/source.json`; check/update/remove; `archo subscribe`; inventory source column, states, fields, and actions. **Implemented** (GUI, CLI; `list_library` recognizes new sources/states); TUI subscription actions completed on 2026-10-30 (mod library › add subscription subpage; `K` / `u` / `U` / `Del` on updates, all queued as background tasks) and MCP mutation tool `manage_subscriptions`; `SteamCMDPath()` supplies location (setting `steamcmd.path`, RimArcho's own copy, PATH); `App.FindTool` injected only in tests | 10, 14 | 11 §1, 16 §2.9, 17 §1 |
| 18 | **Updates**: replaces Mod library change subpage; on 2026-10-24 changed to one list (waiting for you to update, already updated) with details, left scope bar, source summary row, and subscription form replaced by paste field on mod library › add subscription subpage (automatic source detection); retain background checks, update previews, and record deep links; remove multiselect; simple TUI list, action differences in 16 §9 | 17 | 16 §2.9, §2.1, §10 |
| 19 | **Full game versions and pack sharing** (2026-10-24, branch `claude/modpack-79d769`): `env.Version`, full version on launch page; `.archopack` format 2 (tar + zstd, copy digests, allowlist excludes game, import verification); `pack check` and "comparison with this game" (build, DLC, Mods) | 11 | 03 §3.1, 10 §1, §4, §5, 16 §2.3, §2.7, 17 §1 |
| 20 | **Search files within loaded scope** (2026-10-24, implemented, branch `claude/scoped-search-47339e`): `internal/modsearch`; `files search` / `search_files` search only enabled copies, folders loaded by this game version, and content subdirectories, grouped in load order, paginated, with containing Def / XPath; Def mode; no index (17 §7.5) | — | 05 §5.5, 13 §4, 14 §2, 18 §2 |
| 21 | **Small-file reads and load memory** (2026-10-24, branch `claude/smallfile-io-9071ae`, implemented): shared `internal/fsscan` (concurrent traversal, list fingerprint, concurrency per disk, caching); move pack sizes, scan fingerprints, and texture page concurrency there; memory / video memory estimates on list management page (`internal/loadcost`, calibration scope and metrics in 16 §2.17) | 15 | 16 §2.17, 18 §2, research 2026-10-24_small-file-reads-and-load-memory |
| 22 | **Game test runs (2026-10-24, implemented)**: launch a selected plan with isolated data, stop after main menu or test map, produce log attribution report; CLI `test run`, MCP `test_run` | — | 09 §7 |
| 23 | **Settings: external tools, accent color** (2026-10-24, implemented, branch `claude/settings-deps-607064`): `internal/deps` centrally locates bundled fonts, todds, SteamCMD, WezTerm (all other features ask it where SteamCMD is and whether it works), `archo deps`, `deps install`, `deps remove`, "external tools" settings group; `ui.accent` (Radix scale, applied and saved immediately); uninstall data step states that downloaded tools are included (uninstall page itself completed in step 10) | 15 | 16 §2.14, §7.4, 17 §2.1, §4 |
| 24 | **archo.db size** (decided on 2026-10-31, research 2026-10-30_archo-db-integer-keys §8–§10): (0) **Complete (two other branches)**: first fix two research findings: `Store.InstanceFiles` also queries Mods in shared folders (log lookup of texture / DLL providers), `Store.DefsNamed` uses name index first (research §1 item 3, §7.2). (1) **Complete**: add `kinds`, `langs` to snapshots, compute from file lists on write, backfill on upgrade (schema v27); Mod inventory content types, convertible textures, and detail languages read these columns. (2) **Complete** (schema v28, 2026-10-31, branch `claude/db-v28-6b0a03`; real database migration time and size await pre-release VM tests): move non-semantic files (except DLL) from `snapshot_file` into one zstd block per snapshot (path sorted, prefix compressed, storing size, not mtime, **no deduplication**); cleanup follows 04 §4.1; add each fact set's file list to `fact_set`, replace fact-row `file` with list index, delete list with set. One transaction for migration, startup progress, no VACUUM; only add a column to `fact_set`, do not drop/recreate it. No global or per-Mod path table, no Instance integer key, no name-only layer for disabled Mods. Synthetic database 245.5 → 156.2 MB, upgrade 4.7 seconds | — | 04 §4, §4.1, §4.2, 05 §3, 16 §8.1, 18 §2 |

**Progress** (2026-10-30, branch `dev/0.9.0`): steps 1–8 and 10–23 are all complete and merged (step 17 TUI subscription actions and MCP mutation tool `manage_subscriptions` completed on 2026-10-30); `go test ./...` passes. Next is step 9: finish the pre-release VM checklist (17 §5), release 0.9.0, then validate §4.2. Superseded on 2026-11-02 by the 1.0.0 release above; the VM checklist and §4.2 validation remain open.

Old specification section numbers in code comments have been replaced using the mapping table; future specification renumbering must update code comments too.

### 4.2 Implemented but Awaiting Validation

| Item | How to validate | Who |
| --- | --- | --- |
| T4 new-user usability | A person completes enable, sort, and launch within 5 minutes | Player arranges |
| I4 real agent | Run a diagnostic in Claude Code following README (only scripted client self-checks exist now) | Player or agent |
| `_steam` rule | With local and Workshop copies sharing a package ID, verify in game that both register, Workshop copy becomes `<package-id>_steam`, and no error occurs. Decompilation of `Verse.ModLister.TryAddMod` in 1.6.4871 and 1.7.4985 on 2026-10-22: identical in both versions; differing sources (`OnSteamWorkshop` differs) cause the Workshop copy to receive a suffix before registration; same sources (for example both in `Mods/`) report "Tried loading mod with the same packageId multiple times" and ignore the later copy. These errors in player logs all involved two copies with the same package ID in `Mods/`: a local copy and an `!ArchoPack_` link (created by RimArcho on pack launch; a RimArcho defect fixed in §4.1 step 11: 10 §3 step 2) | Player enters game |
| SteamCMD game download | Real login (including Steam Guard), download a real branch, progress line format, error for nonexistent branch | Player |
| Steam unsubscribe / resubscribe | Really unsubscribe from an unwanted Workshop item once, then resubscribe | Player |
| Uninstall | Uninstall an installed release on a clean machine; Documents redirected by OneDrive | Player or VM |
| Native-window-only behavior | Folder selection dialog, `ctrl+w`, menu key / `shift+F10`, WebView2 memory drops to "low" when minimized | Player |
| TUI | Exercise editor, Mod inventory, and change history keys in a real terminal | Player or agent |
| Cold-cache scan | Remeasure first scan after reboot, target ≤ 60 seconds | Player after reboot |
| Missing WebView2 | Double-click `RimArcho.exe` in a VM without WebView2 | Player or VM |
| Pack Player.log location | Where logs are written with `-savedatafolder` | Player enters game |
| Real community data download | `archo data update` against real sources (about 50 MB) | Player |
| SteamCMD Workshop download | `archo subscribe workshop <ID>` without Steam client: whether anonymous login can download RimWorld items, login in SteamCMD window if needed, log success/failure line formats | Player |

### 4.3 Remaining Items in Implemented Features

- First-query speed for a new Player.log and live MCP log `since` cursor: §4.1 step 13, complete.
- Pack links left by direct Steam launches: §4.1 step 11, complete (10 §3).
- PNG to DDS (normal optimization and pack slimming): implemented (10 §2, todds 0.4.1, downloaded with player consent); still needs in-game visual checks in 1.6 and 1.7 (docs/research/dds-texture-optimization.md §6).
- Reducing WebView2 memory when GUI is minimized: implemented (direct WebView2 API call, 17 §7.3); recheck when upgrading Wails.
- **Mod settings follow the folder name** (found 2026-11-02, research 2026-11-02_mod-settings §2): the game stores a Mod's settings in `Config/Mod_<folder it sees>_<Mod class>.xml`. A copy deployed as `_Archo_<source>_…` or `_Archo_env_…` (09 §3), or linked as `!ArchoPack_…` (10 §3 step 4), reads a different file than the same Mod from Workshop or `Mods/`, so the player's settings seem lost and then fork. Direction (that research, end), part 1 implemented 2026-11-03: settings follow the Mod, so each write to the game, pack launch and test run copies a Mod's settings to the name its current folder gives it, without asking and without overwriting (09 §3, 10 §3 step 4, 09 §7); a real-game check is pending in the release VM tests. Since 2026-11-03 Archo Monitor 0.10.0 also records each Mod's settings file and window title at the main menu, and RimArcho keeps those records (06 §7.1, §7.4). Since then (2026-11-03): Monitor 0.11.0 records what a settings window draws when the player opens it, and Mod library › Mod settings, `archo mod-settings` and MCP `get_mod_settings` show the files and windows, read only (16 §2.9). Not implemented: HugsLib settings, editing simple values.
- **GUI accessibility for agents** (defined 2026-11-02, 16 §11 A1–A18): A1, A6, A8, A9 and A15 pass since 2026-11-02 (one named page, modal dialogs, list and table rows with positions), A2, A4, A5, A7, A10, A11, A12, A13 and A16 since 2026-11-03 (every control named, with its row as context; states, selection and record ids; menus, the command panel and choosers; progress, results and field errors; every sampled action by key or UIA pattern), and A18 with export exempt; desktop agents read the page through COM UIA (A17 partial). The rows were relaxed on 2026-11-03 to what an agent that reads the tree after each action needs. Still open: Narrator, and the A13 branches that need real files, the network or a launch (release-vm-tests). The rounds and their evidence are in research 2026-11-02_gui-accessibility.
