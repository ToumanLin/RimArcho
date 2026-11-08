# 01 · Scope and status

> Status: 1.0.0 was released on 2026-11-02, 1.0.5 and 1.0.6 on 2026-11-07, 1.0.7 and 1.0.8 on 2026-11-08 (`CHANGELOG.md`). What each version includes and excludes, and what is still open.
> Basis: 00 (positioning, three questions, principles), 00a (architecture). Each feature's own specification governs its details.

## 1. Version scope

| Version | Question answered | Contents |
| --- | --- | --- |
| 1.0 | What am I playing now, and can it run properly? | Released. Environments with one game directory each and shared Mod folders (03), scanning and identity (04), facts, findings and log attribution (05, 06), plans and sorting (07, 08), launch and link deployment (09), packs (10), downloads, subscriptions and accounts (11), CLI and MCP (13, 14), GUI and TUI (16). In detail: multiple plans shared by all games (07 §1), writing on launch and running only one game at a time (09 §1, §2), attributing outside changes to their writer (07 §4.3), locked versions (07 §5), list exchange (07 §7), the community data channel (08 §2.3), hidden Mods (09 §5), the Archo Monitor companion Mod (19 §6), live game log tracking (06 §6), downloading game versions with SteamCMD (11 §3), unsubscribing and resubscribing through Steam (11 §2), the sidebar and first-run guide (16 §2), the launch page (16 §2.3), the Mod library and change history explaining each Mod's impact (16 §2.9), the shared filter bar, keys and context menus of the editor and the Mod library (16 §3), the settings page and history area (16 §2.14, 07 §3.3), uninstall (02 §2), memory targets (16 §10), a tool for every player action (13 §1). From the log corpus: severity and constraint strength (05 §4.3.1, 08 §1.3), pre-launch notices (09 §2), known dependencies and combinations (05 §4.3), manual Harmony patch targets (05 §5.3), nearby messages and the error pattern library in cluster details (06 §3, §8) |
| 1.1 | What have I not played? | Content catalog (12 §2), untouched content (12 §3), built-in AI assistant Archo (15). Save reading and save management (12 §1, §4) are implemented ahead of 1.1, in 1.0.5 |
| 1.2 | What have I not played? (continued) | Subscribe to and unsubscribe from Workshop collections in one action (07 §7), playthrough themes and content conflicts (12 §5) |
| Long term | What is worth playing? | Player-shared packs (the sharing website is a separate project), recommendations based on understanding the player's content and play goals (12 §6) |
| 1.x | — | Targets of manual `Harmony.Patch` calls that are known only at runtime (scanning methods across the game, collections built by helper methods, `Assembly.GetType`; statically resolvable targets are implemented, 05 §5.3); storing conditional compatibility patches as facts; auditing AllComps additions and removals, icons and materials; static cross-references from Def field types in Assembly-CSharp metadata; macOS and Linux (only compilation is verified) |

## 2. Capabilities

Listed by who can do what; status refers to the current code.

### 2.1 Players

| Capability | Specification | Status |
| --- | --- | --- |
| Find the game, user data and Workshop directories automatically; set them by hand when detection fails | 03 §3 | Implemented (GUI: first-run guide, 16 §2.15) |
| One environment per game directory; the Workshop, SteamCMD and other Mod folders, Archo Monitor and the lock library are shared by all environments; saves in the game's default folder or in the environment's own | 03 §2 | Implemented |
| Download a game version with SteamCMD as a new environment | 11 §3 | Implemented; a real login and download are not yet validated (§4.2) |
| Switch, create, rename and delete plans; all games share plans | 07 §1, §1.7 | Implemented |
| See enabled state, order and findings in one list; enable, disable, move and stage | 07 §2, 16 §2.5 | Implemented |
| Sort in one action; explain conflicting constraints when sorting fails | 08 §1 | Implemented |
| See missing dependencies, incompatibilities, unsupported versions, duplicate package IDs and patches that match nothing, with remedies | 05 §4 | Implemented |
| Attribute errors to Mods with evidence | 06 | Implemented |
| Keep lists and rules through Workshop updates, author renames and package ID changes, and see the changes | 04 §2, 16 §2.9 | Implemented |
| Undo, or go back to an earlier version of a plan | 07 §3 | Implemented |
| Keep identities, rules and history when the game path changes | 03 §4.2 | Implemented |
| Lock a Mod version against Workshop updates | 07 §5 | Implemented (lock library) |
| Import lists from RimCrow, RimSort, Steam collections and others, and export them for others | 07 §6, §7 | Implemented |
| Launch a plan, writing it to the game on launch | 09 §2 | Implemented |
| Build a plan into a pack and play it with separate saves | 10 | Implemented |
| See new errors live while the game runs | 06 §6 | Implemented |
| Handle hidden Mods (`About.xml.disabled`) and unsubscribed residue | 09 §5 | Implemented |
| See every owned Mod on one page: copies per Lineage, locations, sizes and usage; filter and clean up | 16 §2.9 | Implemented (GUI Mod library › all mods; CLI `library`; MCP `list_library`, `manage_library`; TUI list and details) |
| Unsubscribe from and resubscribe to Workshop items through Steam | 11 §2 | Implemented; real calls are not yet validated (§4.2) |
| See which Mods of the current list have settings, what their settings files and HugsLib packs store, and change simple values | 16 §2.9 | Implemented (GUI Mod library › Mod settings, with controls for simple values; CLI `mod-settings`, `mod-settings set`; MCP `get_mod_settings`, `set_mod_settings`; TUI panel, read only); the real-game check is in the release VM tests |
| See what a Mod's changes mean for the player (affects launch / watch / content only), by default for the current plan since the last play session | 16 §2.9 | Implemented |
| Record live logs through Archo Monitor while the game runs | 19, 06 §7 | Implemented |
| Do by keyboard everything that can be done by mouse (shared keys and context menus in the editor and the Mod library) | 16 §3.1 | Implemented |
| Uninstall RimArcho and reverse its changes | 02 §2 | Implemented; uninstalling an installed release on a clean machine is not yet validated (§4.2) |
| Subscribe to Mods in Git repositories and on download pages (LoversLab: Git when a gitgud.io repository exists; otherwise the player downloads the zip and installs it), check for and pull updates; download Workshop items with SteamCMD when the Steam client is unavailable | 11 §1, 16 §2.9 | Implemented (GUI, CLI, TUI, MCP `manage_subscriptions`; the TUI subscribes to a download page only through its single Git repository; choose a zip or folders and LoversLab site files in the window, or use `--from`); real SteamCMD Workshop downloads are not yet validated (§4.2) |
| Manage saves: grouped by colony, whether each loads with its list, fixes, rename, copy, back up, recycle | 12 §4, 16 §2.11 | Implemented in 1.0.5 (GUI Saves page; CLI `saves`; MCP `list_saves`, `get_save`, `manage_saves`) |
| See Mods and content never played | 12 §3 | Not implemented (1.1) |

### 2.2 AI assistants

| Capability | Specification | Status |
| --- | --- | --- |
| Query facts and conflicts for Mods, Defs, patches and Harmony | 14 | Implemented |
| Given a stack trace, find the Mods that own the Harmony patches in it, and the next steps | 06, 14 §3 | Implemented |
| Decompile a single Mod on demand and search its source | 05 §5.4 | Implemented |
| A tool for every player action | 13 §1 | Implemented (uninstall has only a read-only preview, 02 §2) |
| Built-in Archo: conversations and one-action tasks | 15 | Not implemented (1.1) |

## 3. Exclusions

- A general Workshop browser or Workshop search. "Worth playing" starts only from Mods the player owns and packs other players have validated (00 §2).
- A Mod acquisition hub (the other three pages of RimCrow's "inventory hub"). In scope instead: managing local Mods in the Mod library (16 §2.9); subscribing to Git repositories and download pages, and downloading Workshop items with SteamCMD when the Steam client is unavailable (11 §1; otherwise Workshop items come through the Steam client); downloading game versions with SteamCMD (11 §3), where the player logs in through SteamCMD's own window and RimArcho gives it only the account name.
- Updating RimArcho without confirmation: checks for new versions are automatic, but downloading and replacing need the player's confirmation (17 §5.1). Resident background processes. Opening any network port (MCP uses only stdio).
- A sleep screen and recommendation list export (RimCrow has these).
- Buying a code-signing certificate. README explains the SmartScreen prompt on first run.
- Approving permissions for external agents: their harness decides whether to allow writes; tools carry the MCP annotations `readOnlyHint` / `destructiveHint` (14 §2). Built-in Archo uses RimArcho's own confirmation for writes (15 §2).

## 4. Open work and validation

### 4.1 Open work

- **1.1**: extracting the Defs a save references and mapping them back to the Mods that provide them (12 §1.4, §1.5); the content catalog (12 §2, draft); unused content and its page (12 §3, 16 §2.11); built-in Archo (15). Later versions: §1.
- **Defects not yet fixed**: those found in the 1.0.1 VM round are listed under "Open items" in research 2026-11-05_release-1.0.1-vm-round. One needs a decision: a test run compares with the newest Player.log even when that log came from an unrelated, older list (20 §2); either compare only with a log of the same plan, or say how old the log is.
- Work not yet decided is tracked in `ToDo.md` and docs/research/, not here.

### 4.2 Implemented, not yet validated

| Item | How to validate | Who |
| --- | --- | --- |
| T4 new-user usability | A new player enables, sorts and launches within 5 minutes | Player arranges |
| I4 real agent | Run a diagnosis in Claude Code or Codex following README (14 §4) | Player or agent |
| `_steam` rule | With a local and a Workshop copy sharing a package ID, check in the game that both register, the Workshop copy becomes `<package-id>_steam`, and no error appears (the rule from decompilation: 17 §6 item 3) | Player enters game |
| SteamCMD game download | Real login (with Steam Guard), download of a real branch, the progress line format, the error for a branch that does not exist | Player |
| SteamCMD Workshop download | `archo subscribe workshop <ID>` without the Steam client: whether an anonymous login can download RimWorld items, logging in in the SteamCMD window if needed, the formats of the success and failure lines in the log | Player |
| Steam unsubscribe / resubscribe | Unsubscribe from an unwanted Workshop item once for real, then resubscribe | Player |
| Uninstall | Uninstall an installed release on a clean machine, with Documents redirected by OneDrive; in the English interface; with a relocated source library; with RimArcho closed by shutdown or sign-out | Player or VM |
| Native-window-only behavior | `ctrl+w`, the menu key / `shift+F10` | Player |
| TUI | Use the editor, Mod library and change history keys in a real terminal (Windows Terminal) | Player or agent |
| Cold-cache scan | Measure the first scan after a reboot; target ≤ 60 seconds | Player after reboot |
| Missing WebView2 | Double-click `RimArcho.exe` in a VM without WebView2 | Player or VM |
| Pack Player.log location | Where the game writes its log when launched with `-savedatafolder` (10 §3) | Player enters game |
| Real community data download | `archo data update` against the real sources (about 50 MB) | Player |
| DDS textures | Optimized textures and slimmed packs look right in the game, in 1.6 and 1.7 (10 §2; research dds-texture-optimization §6) | Player enters game |
| GUI accessibility | Narrator; the A13 branches that need real files, the network or a launch; A17 on the remaining pages (16 §11) | Player or agent |
| Release VM test rows | The rows still in [release VM tests](../../UserGuide/release-vm-tests.md), among them every row's 1.7 part, real-mouse checks, the LoversLab session, in-game settings windows (Archo Monitor and HugsLib records) and the feedback relay's real send (research 2026-11-05_release-1.0.1-vm-round, "Not covered") | VM, or player where credentials are needed |

### 4.3 Limits of implemented features

- Mod settings: lists, dictionaries and nested values are read only; they change in the game (16 §2.9).
- Memory estimates for 1.6 on a PC with a discrete graphics card are inferred: the model is calibrated only in a VM without dedicated video memory (16 §2.17). A run on real hardware is planned only if false warnings are reported.
- Lowering WebView2 memory when the window is minimized calls the WebView2 API directly (17 §7.3); recheck it when upgrading Wails.
