# pardeike's GAB stack and DecompilerServer vs. Archo MCP and the probe skill (2026-11-08)

Status: research, no product change. Sources read at these commits (cloned outside the repository): DecompilerServer `42c3f91` (2026-10-01, v1.4.0), GABP `0a97d7d` (2026-03-21), GABS `2c01448` (2026-09-30, v1.1.2), Lib.GAB `bdbf4c9` (2026-10-07, 1.0.6), RimBridgeServer `1dcd044` (2026-10-07, v2.3.0). Paths on their side are prefixed with the repository name; paths without a prefix are in RimArcho (`dev/1.0`). There is no .NET SDK in the research container and no RimWorld, so nothing on the .NET side was built or run; GABS was built and its tests run (`go test -count=1 ./...`, Go 1.24.7, `GOTOOLCHAIN=local`: all 11 packages pass).

## Verdict

Use all five as references; integrate none of them into Archo MCP now. DecompilerServer adds things `ilspycmd` plus text search cannot do: IL-level callers and usages, string-literal cross-references, and a member-level 1.6 vs 1.7 diff. But it is a separate .NET 10 MCP server with 44 tools, and an agent can already run it next to `archo mcp` without RimArcho doing anything. It deserves one optional line in the skill's "Finding the code to call", not a wrapper or a bundle. GABS + RimBridgeServer give a live, interactive game with 128 fixed tools, which helps UI exploration and fast iteration. But RimBridgeServer builds only for 1.6, changes game behaviour just by being loaded, and several of its tools save to disk by default. GABS isolates nothing; Lib.GAB mis-frames non-ASCII requests; GABS deviates from its own spec in three places. Forwarding those tools would also break Archo MCP's tool-list budget and its one-blocking-call `test_run` lifecycle. The probe already does what the live tools do and more (any C#, load-time observation, both game versions, a reproducible artifact); what it lacks is interactivity. Revisit options (b) and (c) only if transcripts show agents losing real time relaunching probes to explore UI, and only once a 1.7 build of RimBridgeServer exists and the framing bug is fixed.

## 1. DecompilerServer

### What it is

A stdio-only MCP server (`DecompilerServer/Program.cs:44-47`, `.WithStdioServerTransport().WithToolsFromAssembly()`) on ICSharpCode.Decompiler 10.0.1.8346 and the official `ModelContextProtocol` 1.3.0 package (`DecompilerServer/DecompilerServer.csproj:12-14`). It has 44 `[McpServerTool]` methods under `DecompilerServer/Tools/` (counted from source). They fall into these groups:

- **Context admin:** `load_assembly`, `select_context`, `list_contexts`, `unload`, `warm_index`, `clear_caches`, …
- **Discovery:** `search_types`, `search_members`, `list_namespaces`, `get_members_of_type`, `resolve_member_id`, …
- **Source and metadata:** `get_decompiled_source`, `get_source_slice`, `get_il`, `get_member_signature`, …
- **Analysis:** `find_usages`, `find_callers`, `find_callees`, `search_string_literals`, `search_attributes`, `find_derived_types`, `get_implementations`, `get_overrides`, …
- **Comparison:** `compare_contexts`, `compare_symbols`.
- **Text generators:** `generate_harmony_patch_skeleton`, `suggest_transpiler_targets`, …

### Beyond `ilspycmd` plus text search (SKILL.md "Finding the code to call", `skills/rimworld-probe/SKILL.md:58-71`)

| Capability | DecompilerServer | Our method today |
| --- | --- | --- |
| Callers / usages of a member | An IL scan of every method in the module builds a reverse index keyed by MVID + metadata token (`DecompilerServer/Services/UsageAnalyzer.cs:98-124`). Overloads and same-named members in other assemblies do not collide. Callers are usages of kind Call/NewObject (`:136-143`); callees decode one method on demand (`:154-179`). | `grep` for the name in decompiled text: overloads, same-named members and comments all match. |
| String literals | An `ldstr` inventory per method (`DecompilerServer/Services/UsageAnalyzer.cs:243-268`). This is a direct way from a `"Key".Translate()` string to its method: way in no. 1 of SKILL.md (`skills/rimworld-probe/SKILL.md:67`). | `grep` for the key in decompiled text, which works but also hits unrelated text. |
| Inheritance | Derived types, implementations and overrides through the ILSpy type system (`DecompilerServer/Services/InheritanceAnalyzer.cs`). | `grep` for `: BaseClass`. |
| IL view | A paged IL decoder (`DecompilerServer/Services/IlAnalysisService.cs:61`, `DecompilerServer/Tools/GetIL.cs:12`), useful for transpiler targets. | `ilspycmd --il` (not mentioned in SKILL.md). |
| 1.6 vs 1.7 | Each assembly is a named context; `compare_contexts` diffs public surface (types by `FullName`, members by `kind:name`, overloads grouped into one fingerprint) (`DecompilerServer/Tools/CompareContexts.cs:31-33`, `DecompilerServer/Services/TypeSurfaceComparer.cs:148-168`). `compare_symbols` mode `body` decompiles one member on both sides and runs a line LCS diff (`DecompilerServer/Tools/CompareSymbols.cs:110-129`). RimWorld is the README's example (`DecompilerServer/README.md:354-374`). Renames show as remove + add. | Two decompiles and "check a method exists in both" by hand (`skills/rimworld-probe/SKILL.md:98`). |

Limits that matter for RimWorld:

- **Only some opcodes are indexed:** `call`, `callvirt`, `newobj`, `ld(s)fld` and `st(s)fld` (`DecompilerServer/Services/UsageAnalyzer.cs:321-330`, checked). `ldftn` / `ldvirtftn` are not indexed, so a method passed as a method group (`action = DoThing`) has no caller. A lambda body is its own compiler-generated method, and its calls are found.
- **Override matching is approximate.** It compares only parameter counts and is marked "Simplified" (`DecompilerServer/Services/InheritanceAnalyzer.cs:369-373`), so overloaded virtuals can be wrong.
- **Signature search is text matching** on the rendered signature (`DecompilerServer/Services/SearchServiceBase.cs:424-428`).

### Memory

- **What stays in memory:** the main DLL and every resolved dependency (`DecompilerServer/Services/AssemblyContextManager.cs:19`, `DecompilerServer/Services/OwnedAssemblyResolver.cs:10`).
- **Indexes** are built at load by default (`rebuildIndex`, `DecompilerServer/Services/AssemblyContextManager.cs:307-316`).
- **Decompiled source** is produced lazily per member, into byte-bounded LRU caches. Their sizes are documented as retention estimates, not process limits: 32 MiB source, 64 MiB each for the symbol and reference indexes (`DecompilerServer/ARCHITECTURE.md:465-475`).
- **At most 4 contexts** are loaded at once by default (`DecompilerServer/Services/DecompilerWorkspace.cs:12`, `DECOMPILER_MAX_LOADED_CONTEXTS`).
- **Nothing is cached on disk.**
- **A possible slow path:** an entry larger than its cache's cap is not retained (`DecompilerServer/Services/BoundedCache.cs:82-83`), and the reference index is estimated at about 300 bytes per call site (`DecompilerServer/Services/UsageAnalyzer.cs:20-21`). If `Assembly-CSharp`'s index exceeds 64 MiB, every `find_callers` would rescan the IL. Whether it does cannot be determined without loading the real DLL.
- **No measured memory figures** exist in the repository.
- **Compared with Archo:** Archo's source library writes durable files once (`docs/specs/05-facts-and-findings.md:314`) and keeps nothing resident.

### Can it load 1.7's `Assembly-CSharp.dll`?

Probably, but this is not shown.

- **Finding the DLL:** `gameDir` mode tries `<gameDir>/Assembly-CSharp.dll` first, which is the 1.7 layout, then `*_Data/Managed`, which is the 1.6 layout (`DecompilerServer/Services/AssemblyContextManager.cs:371-418`, checked).
- **Resolving references:** it uses ILSpy's `UniversalAssemblyResolver` with `throwOnError:false` and `targetFramework:null` (`DecompilerServer/Services/OwnedAssemblyResolver.cs:17-26`). Missing references such as `UnityStubs.dll` degrade output instead of failing.
- **Version handling:** there is no RimWorld-, Waylith- or 1.7-specific code. ICSharpCode.Decompiler 10 reads net10 metadata.
- **Open question:** whether the resolver finds `System.Runtime` 10 from the game folder can only be settled by loading a real 1.7 DLL, which is a VM test.

### Runtime and packaging

- **Target:** every project is `net10.0` (`DecompilerServer/DecompilerServer.csproj:5`).
- **Releases** are framework-dependent single files for win-x64, linux and osx (`--self-contained false -p:PublishSingleFile=true`, `DecompilerServer/.github/workflows/release.yml:118-128`, checked). The user needs the .NET 10 runtime, and nothing in the repository offers a self-contained build. Artifact sizes were not measured (the GitHub API is closed to this session).
- **Archo's rule:** 17 §1 makes decompilation an external, optional, undistributed tool that RimArcho never installs (`docs/specs/17-engineering.md:25`, `docs/specs/05-facts-and-findings.md:312`, `internal/app/source.go:33-34`).

### Should SKILL.md or 14 mention it, and should Archo wrap or bundle it?

- **SKILL.md: yes, one optional line.** In "Finding the code to call", say that if a DecompilerServer MCP is configured, `find_callers` / `search_string_literals` replace the grep step and `compare_contexts` / `compare_symbols` answer "does this exist in both 1.6 and 1.7". Keep `ilspycmd` as the baseline, because it needs no MCP configuration and the probe only needs names.
- **14: no.** 14 describes Archo's own surface, and 14 §3 keeps Archo independent of the skill and of outside tools (`docs/specs/14-mcp.md:87`).
- **Wrap: no.** Archo already has `read_source` (ilspycmd, durable source library, search; `docs/specs/14-mcp.md:43`) and its own Harmony facts from metadata (`get_patches`, `get_harmony`; `internal/analyze/harmony.go`). A wrapper would only rename DecompilerServer's tools, add a .NET 10 runtime dependency to a pure-Go product, and spend tool-list budget (see §3). An agent that wants call graphs can add DecompilerServer as a second MCP server today.
- **Bundle: no.** Bundling either a framework-dependent build (still needs the runtime) or a self-contained one (a much larger release zip, size not measured) contradicts 17 §1.
- **Possible later change:** a narrow `find_callers`-like query inside `read_source` could be built on Archo's own metadata reader (`internal/dotnet`) if transcripts show the need. That is separate work, not DecompilerServer.

## 2. GABS + RimBridgeServer vs. the probe

### RimBridgeServer's full tool set

There are 128 `[Tool("…")]` methods in `RimBridgeServer/Source/RimBridgeTools.cs` (counted). They are registered through Lib.GAB with `RegisterToolsFromInstance` (`RimBridgeServer/Source/RimBridgeStartup.cs:128`), and each forwards to one of 14 capability providers (`RimBridgeServer/Source/RimBridgeCapabilities.cs:24-112`). A separate companion DLL adds 10 `multiplayer/*` tools (`RimBridgeServer/Companions/Multiplayer/MultiplayerTools.cs`). Other mods can add tools through its SDK, or as `BridgeTools` DLLs (`RimBridgeServer/Source/RimBridgeExtensionDiscovery.cs:179`, `:198-207`, `:471`).

| Group | Count | Tools (line ranges in `RimBridgeServer/Source/RimBridgeTools.cs`) |
| --- | --- | --- |
| Bridge, diagnostics, waits | 12 | `ping`, `get_game_info`, `get_bridge_status`, capabilities and operations lists, `list_logs`, `wait_for_operation`, `wait_for_game_loaded`, `wait_for_long_event_idle` (11-117) |
| DPA profiling | 6 | `dpa_status`, `dpa_patch_methods`, `dpa_snapshot`, `dpa_reset`, `dpa_stop`, `dpa_cleanup` (126-178) |
| Scripting | 7 | `get_script_reference`, `get_lua_reference`, `run_script`, `run_lua`, `run_lua_file`, `compile_lua`, `compile_lua_file` (185-237) |
| Time, debug actions, mods, mod settings | 21 | `pause_game`, `set_time_speed`, `play_for`, `play_until_letter`, `step_game_ticks`, debug-action list/search/execute, `set_debug_setting`, `list_mods`, `set_mod_enabled`, `reorder_mod`, `get_mod_settings`, `update_mod_settings`, `reload_mod_settings`, `open_mod_settings` (246-445) |
| Architect, zones, map cells | 19 | designators, `set_god_mode`, zones and areas, `get_cell_info`, `flood_fill_cells`, … (454-639) |
| UI and input | 23 | `get_ui_state`, `get_ui_layout`, `click_ui_target`, `set_text_field`, `press_key`, `close_window`, `open_window_by_type`, `switch_language`, `start_debug_game`, `go_to_main_menu`, … (676-888) |
| Selection, colonists, gizmos, letters, alerts | 15 | `list_colonists`, select, `set_draft`, gizmos list/execute, letters, alerts (895-1001) |
| Camera and screenshots | 13 | camera moves, `get_screen_targets`, `screenshot_cell_rect`, `take_screenshot` (1008-1121) |
| Saves and spawning | 5 | `list_saves`, `spawn_thing`, `save_game`, `load_game`, `load_game_ready` (1136-1172) |
| Context menus and map clicks | 7 | `open_context_menu`, `right_click_cell`, `click_cell`, `drag_cell`, float-menu options (1187-1258) |

The Lua is not a general interpreter, and nothing runs arbitrary C#:

- It compiles a MoonSharp syntax tree into a JSON script of capability calls (`RimBridgeServer/Source/RimBridgeServer.Core/LuaScriptCompiler.cs:47-69`, `:101`).
- The script may use only `rb.call`, `rb.poll`, `rb.assert`, `print` and `ipairs` (`RimBridgeServer/Source/RimBridgeServer.Core/LuaScriptCompiler.cs:993-1070`).
- `rb.call` reaches registered capabilities, not .NET methods (`RimBridgeServer/Source/RimBridgeServer.Core/CapabilityScriptRunner.cs:511-520`).

### Main thread

- **The pump:** a Harmony postfix on `Root.Update` calls `RimBridgeMainThread.Pump()` and `RimBridgeAsyncScheduler.Pump()` every frame (`RimBridgeServer/Source/RimBridgeRuntime.cs:123-145`, checked). The queue they drain is a locked `Queue<IMainThreadWorkItem>` (`:201-235`).
- **Timeouts:** `Invoke` waits 5 s by default, and a timeout of 0 or less means wait forever (`:237-265`). Capability calls pass 0 (`RimBridgeServer/Source/BuiltInCapabilityModuleProvider.cs:91-93`), so a hung frame hangs the call. GABS's own 30 s request timeout is the outer bound (`GABS/internal/gabp/client.go:52`).
- **Our probe for comparison:** it already runs on the main thread, as a `MapComponent.MapComponentUpdate` (`skills/rimworld-probe/template/src/ProbeRunner.cs:270`, `:283`).

### What it writes to disk

| Write | Default | Evidence | Lands in |
| --- | --- | --- | --- |
| `ModsConfig.xml` | `set_mod_enabled` / `reorder_mod` have **`save = true`** | `RimBridgeServer/Source/RimBridgeTools.cs:389`, `:400`; `ModsConfig.Save()` in `RimBridgeServer/Source/RimWorldModConfiguration.cs:170`, `:233` | Vanilla path (`GenFilePaths.ModsConfigFilePath`, `:311`) |
| `Prefs.xml` | `switch_language` **always** saves | `LanguageDatabase.SelectLanguage(target); Prefs.Save();` (`RimBridgeServer/Source/RimWorldLanguages.cs:116-117`, checked) | Vanilla Prefs path |
| Mod settings | `update_mod_settings` has **`write = true`** | `RimBridgeServer/Source/RimBridgeTools.cs:427`; `mod.WriteSettings()` (`RimBridgeServer/Source/RimWorldModSettings.cs:138`) | Vanilla `Config/` |
| Saves | `save_game` | `GameDataSaveLoader.SaveGame` (`RimBridgeServer/Source/LifecycleCapabilityModule.cs:845-847`) | Vanilla `Saves/` |
| Screenshots | `take_screenshot` | `ScreenshotTaker.TakeNonSteamShot` into the screenshot folder (`RimBridgeServer/Source/ViewCapabilityModule.cs:747-756`); the optional **`outputDirectory` accepts any absolute path**, creates it, moves the PNG there (`:650-686`, checked) | Vanilla folder, or anywhere |
| Code it loads | at startup | Every DLL in `<parent of Mods>/BridgeTools` and each mod's `BridgeTools/` (`RimBridgeServer/Source/RimBridgeExtensionDiscovery.cs:198-207`, `:471`) | Game folder (a read, but it runs code) |
| Files it reads | `run_lua_file` | Any path (`RimBridgeServer/Source/ScriptingCapabilityModule.cs:287-298`) | Anywhere |
| Token | standalone mode | Printed to the log (`RimBridgeServer/Source/RimBridgeStartup.cs:174-176`) | `Player.log` |

All the game-data writes go through RimWorld's own APIs, so under `-savedatafolder` they land in the isolated folder. That is RimWorld's behaviour, which our test runs already rely on (`docs/specs/20-game-test-runs.md:25`). It writes no files of its own.

The risks are that writes happen by default, the screenshot path is free, and DLLs are loaded from the game folder. RimBridgeServer also changes the game just by loading:

- it forces `Application.runInBackground = true` and re-forces it after every `PrefsData.Apply` (`RimBridgeServer/Source/RimBridgeStartup.cs:96`, `RimBridgeServer/Source/RimBridgeRuntime.cs:111-120`);
- it turns on the TPS counter and `UltraSpeedBoost` (`RimBridgeServer/Source/RimBridgeStartup.cs:99-104`, checked);
- it applies 59 Harmony patch classes, 49 of them on UI (`Widgets`, `GUI.Button`, `GUI.Label`, private `GUI.DoTextField`; e.g. `RimBridgeServer/Source/RimBridgeUiWorkbench.cs:2752-2966`), plus 6 transpilers (`RimBridgeServer/Source/RimBridgeVirtualPointer.cs:635-675`).

For a UI-mod conflict those patches are a confound: the bridge patches the same methods the mods under test do.

### Versions

- **Build:** the mod targets `net472` (`RimBridgeServer/Source/RimBridgeServer.csproj:6`) against the NuGet reference package `Krafs.Rimworld.Ref` 1.6.4850 (`:66-97`), and outputs to `1.6/Assemblies`.
- **Load folders and supported versions:** `LoadFolders.xml` has only `<v1.6>`, and `About/About.xml` lists `supportedVersions` 1.6 only (both checked). The repository ships only `1.6/Assemblies`.
- **No version handling:** there is no `#if` and no `1.7` or `UnityStubs` anywhere.
- **What would port:** the helper projects (Core, Contracts, Sdk) already multi-target `net472;net10.0` (e.g. `RimBridgeServer/Source/RimBridgeServer.Core/RimBridgeServer.Core.csproj:4`). Lib.GAB targets `netstandard2.0;net10.0` (`Lib.GAB/Lib.GAB/Lib.GAB.csproj:4`).
- **What a 1.7 build needs:** references to the game folder's `Assembly-CSharp.dll` and `UnityStubs.dll` (as our probe does, `docs/specs/20-game-test-runs.md:54`), and the 59 patch classes and 6 transpilers on UnityEngine members to resolve and work on Waylith.
- **Open question:** whether `UnityStubs.dll` provides `GUI.DoTextField`, `Input.GetMouseButton*`, `Texture2D.EncodeToPNG` and the rest, and whether Waylith honours patches on them, cannot be determined statically. So 1.7 is a port, not a rebuild.

### Can it run inside our sandbox under GABS `DirectPath`?

As far as the code shows, yes:

- **Launch:** `DirectPath` runs `exec.Command(target, args...)` with the configured working directory (`GABS/internal/process/controller.go:166-168`, `:206-221`). Args pass unchanged, so `-savedatafolder=…`, `-logFile`, `-quicktest` work.
- **Handoff:** port and token go to the child as environment variables `GABP_SERVER_PORT` / `GABP_TOKEN` (`GABS/internal/process/controller.go:338-339`, checked). Lib.GAB reads them (`Lib.GAB/Lib.GAB/GabpServerBuilder.cs:127-133`, checked).
- **Files:** GABS's `bridge.json` lives under `~/.gabs/<gameId>/` and is diagnostic only (`GABS/internal/config/bridgecfg.go:31-45`, `GABS/internal/config/paths.go:247-252`). Nothing is resolved relative to the game folder.
- **Process handle:** the PID comes from `cmd.Start` with a start-time fingerprint (`GABS/internal/process/controller.go:262-271`).
- **Junctions:** the working-directory check resolves symlinks on both sides (`GABS/internal/process/context_delivery.go:118-134`). Whether Go treats a junction as a link depends on its `winsymlink` setting. Both sides are resolved the same way, so a false mismatch looks unlikely, but this was not tested on Windows.

GABS isolates nothing itself:

- "Profiles" are only sets of args and environment variables (`GABS/internal/config/profiles.go:13-23`). A search for savedata, userdata, sandbox and isolation finds only documentation telling the user to add `-savedatafolder` (`GABS/docs/CONFIGURATION.md:233`).
- Two GABS modes would break our rules:
  - **`SteamManaged` writes `steam_appid.txt` into the game folder** (`GABS/internal/steam/steam.go:184`, checked). That is exactly the attachment our sandbox avoids (`docs/specs/20-game-test-runs.md:55`, `skills/rimworld-probe/scripts/probe.mjs:87`).
  - With `stopProcessName` set, GABS signals every process of that name when it cannot verify the PID (`GABS/internal/process/stop_gate.go:640-660`), which could include a player's own game.
- **Ending:** when the MCP session ends, GABS only closes its clients (`GABS/internal/mcp/stdio_server.go:113-140`); the game keeps running.

Under GABS, RimBridgeServer's own writes would land in our isolated user data. Isolation would still depend on the caller: `DirectPath` only, no `stopProcessName`, our sandbox folder as the target, no `outputDirectory`, and no `BridgeTools` folder in the game root (the sandbox junctions every top-level game folder, `skills/rimworld-probe/scripts/probe.mjs:89`).

### Where live fixed tools beat a scripted probe

- **Exploring before writing a scenario.** `get_ui_state`, `get_ui_layout`, `get_screen_targets`, `list_letters` / alerts, `get_cell_info` and `list_mods` answer "what is on screen and what can I click" without a relaunch. A probe run takes 1–3 minutes, longer on a first 1.7 run (`skills/rimworld-probe/SKILL.md:31`). An agent that does not yet know which window or gizmo to target pays that per guess.
- **Iterating on one map:** `step_game_ticks`, `play_until_letter`, `spawn_thing`, `set_draft`, debug actions, then look again.
- **UI input as the player gives it:** clicks, text fields and float menus go through real input paths via a virtual pointer. The probe calls the methods the UI calls (`skills/rimworld-probe/SKILL.md:75`), which is usually better evidence but cannot show a click that never reaches its handler.
- **Profiling** (DPA) and cropped screenshots of a cell rectangle.

### Where they limit an agent that can write C#

- **Only what is exposed.** There is no arbitrary C#, reflection or Harmony; Lua reaches only capabilities. Anything beyond the 128 tools needs a mod again, through RimBridgeServer's SDK, and that is a probe with more steps. The probe's `Call` / `Get` / `Set` / `Patches` / `CompNamed` reach any member of any mod by name (`skills/rimworld-probe/template/src/ProbeRunner.cs:82`, `:130-158`).
- **No load time.** The server starts only after play data is loaded and the UI root exists (`RimBridgeServer/Source/RimBridgeRuntime.cs:140-143`). Patch order at `[StaticConstructorOnStartup]`, the probe's `OnStartup` (`skills/rimworld-probe/template/src/ProbeRunner.cs:244`), and the probe's position next to a given mod (`--probe-after`, `docs/specs/20-game-test-runs.md:44`) have no equivalent.
- **Observer effect:** forced `runInBackground`, `UltraSpeedBoost`, and 59 UI patches that mods under test may also patch.
- **No artifact.** A tool-call session is not a file that reruns in both load orders or in both game versions. A scenario is, and its `RESULT` line feeds `test_run` attribution and comparisons (`docs/specs/20-game-test-runs.md:46-47`).
- **1.6 only.**
- **Context cost:** 128 tool schemas, or GABS's indirection through `games.tool_detail` / `games.call_tool` (`GABS/internal/mcp/stdio_server.go:764-3205`).

## 3. Integration options for Archo MCP

These constraints on our side shape every option:

- **Tool-list budget.** All 45 Archo tools are about 47 KB against a 48 KB limit for `tools/list` (`docs/specs/14-mcp.md:11`, `:88`). 128 forwarded tools cannot be listed. GABS hit the same problem: it does not list mirrored tools at all (`GABS/internal/mcp/stdio_server.go:6030-6035`, checked) and exposes them through meta tools.
- **Lifecycle.** `test_run` is one blocking call that launches, waits, kills the game and reports (`docs/specs/20-game-test-runs.md:28`, `internal/gametest/run.go:54-57`). Calls in one `archo mcp` process run one at a time (`docs/specs/14-mcp.md:15`), and the server is not resident (`:13`). A live game must outlive a call, keep its temporary `_Archo_` links until it exits (`docs/specs/20-game-test-runs.md:29`), and be killed when the session closes.
- **Isolation.** Test runs never touch the player's saves, configuration or logs (`docs/specs/20-game-test-runs.md:21`, 17 §4 `docs/specs/17-engineering.md:176-182`).

| Option | What it would change | Cost | Risk |
| --- | --- | --- | --- |
| **(a) Reference only** | Nothing in code. Optionally one line in SKILL.md about DecompilerServer (§1), and RimBridgeServer's source named as a cookbook for UI driving (`RimWorldInput.cs`, `ContextMenuCapabilityModule.cs`, `ViewCapabilityModule.cs`) in "Finding the code to call". | Minutes | None |
| **(b) `test_run` loads RimBridgeServer and forwards its tools** | A new live-session lifecycle in `internal/gametest` and `App.TestRun` (start, keep running, stop on session close or timeout, links held during the session). New meta tools in `internal/surface/mcp` (start / call / stop, at least 3 rows within the 48 KB budget) plus guide topics. A GABP client (option c). A tool allow-list that drops or rewrites `outputDirectory`, `run_lua_file`, `switch_language` and `save_game`, and forces `save=false` / `write=false` (§2 table). Port and token through the environment, with a fresh random port per run. Where RimBridgeServer comes from: Archo would have to download or bundle a third-party mod (11 and 02 policy). 1.6 only. New VM rows. | Large: weeks, mostly lifecycle and policy, not the client | High: isolation now depends on a filter over a fast-moving third-party tool set (63 commits since May); observer effects confound diagnoses; no 1.7; Lib.GAB framing bug (§4) |
| **(c) A GABP client in Go** | About 800 lines: framing, `session/hello`, `tools/list`, `tools/call`, events. GABS's client is 835 lines in `internal/gabp` plus framing in `internal/util/jsonrpc.go`. It is under `internal/`, so it cannot be imported, but copying is allowed under MIT with the notice. `gabp-runtime` v1.0.0 has no license file, so do not depend on it. Tests against a fake bridge. Must send both `arguments` (spec) and accept GABS-era peers. | Small to medium by itself | Low by itself, but worth nothing without (b) or (d) |
| **(d) The probe template hosts its own GABP endpoint through Lib.GAB** | `ProbeRunner.cs` gains a server with scenario-declared tools. Lib.GAB + Gabp.Runtime + Newtonsoft.Json DLLs go into `1.6/Assemblies` and `1.7/Assemblies`. The probe no longer quits on its own, so the line contract changes. A client is needed (GABS or c). | Medium | Medium: to add a tool the agent still rebuilds and relaunches, so the live gain is only parameterised calls on one map; Newtonsoft on Waylith not verified; framing bug |
| **(e) Borrow ideas, not code** | Into the skill template, when a scenario needs them: a cropped screenshot of a cell rectangle, `play_until_letter`-style waiting, reading letters and alerts. All are a few lines on the main thread the probe already owns (`ProbeMapComponent`). The rule that Steam-managed launch must not write `steam_appid.txt` is already ours. | Small, per need | Low |

On (e), I found nothing better than keeping the probe and closing its one gap cheaply. If interactivity becomes a demonstrated need, the agent can run GABS + RimBridgeServer itself on 1.6, inside the skill's sandbox, with RimBridgeServer as one more `--mods` entry and GABS `DirectPath` pointed at the sandbox exe with `-savedatafolder`. That is the agent's own infrastructure, consistent with the skill being a skeleton and a method, and it puts nothing in Archo MCP. If the skill wants to allow it, a short paragraph listing the conditions from §2 belongs in its "Limits".

### Side finding (not part of this research's scope)

The skill's sandbox leaves out `steam_appid.txt` because, attached to a running Steam client, 1.6 changed the player's real `Prefs.xml` (`docs/specs/20-game-test-runs.md:55`, `skills/rimworld-probe/SKILL.md:88`). RimArcho's own test run starts the game directly in the environment's real game folder (`docs/specs/20-game-test-runs.md:27`, `internal/gametest/run.go:137-144`), and `internal/gametest` does nothing about `steam_appid.txt` (a grep finds no match). If that file is in a Steam install's folder, as the skill's runner implies, `archo test run` with Steam running may attach the same way. This cannot be settled statically. It needs a VM check: run `archo test run` with Steam running, then compare the hash of the real LocalLow `Prefs.xml` before and after the run and look for `SteamAPI.Init() failed` in the run log. The acceptance test in `docs/specs/20-game-test-runs.md:65` checks only LocalLow modification times, and it does not say whether Steam was running.

## 4. Fit and maturity

| | DecompilerServer | GABP | GABS | Lib.GAB | RimBridgeServer |
| --- | --- | --- | --- | --- | --- |
| License files | MIT (`LICENSE`) | Root `LICENSE` MIT; `LICENSES/CODE-LICENSE.txt` Apache-2.0; `LICENSES/SPEC-LICENSE.txt` CC BY 4.0 for `SPEC/`; README says spec CC BY 4.0, code Apache-2.0 (`GABP/README.md:192-195`); packages say Apache-2.0 | MIT, "GABS Contributors" | MIT | MIT |
| GitHub sidebar (2026-11-08) | MIT | **MIT**: GitHub reads the root `LICENSE`, the README says otherwise | MIT | MIT | MIT |
| Compatible with RimArcho (MIT)? | Yes | Yes under any of the three; copying spec text needs CC BY attribution | Yes; copying keeps the notice | Yes | Yes. Redistributing the mod is a separate question; it depends on Krafs reference packages only at build time |
| Commits since 2026-05-01 / total | 16 / 81 | 0 / 28 (last 2026-03-21) | 131 / 366 | 10 / 55 | 63 / 178 |
| Tests | 222 `[Fact]` + 6 `[Theory]`, synthetic libraries | Schema fixtures (9 valid, 8 invalid), validated with ajv in CI; no live conformance test against any implementation | 731 `Test` functions in 130 files; all pass here on Linux | 69 xUnit | 182 xUnit (Core 154, LiveSmoke 12, Sdk 10, Contracts 6) |
| CI | Only on `v*` tags (`release.yml`), Ubuntu | Validation on push | Full suite on Ubuntu; Windows runs vet, build and three packages, not `internal/mcp` (`GABS/.github/workflows/test.yml:41-70`) | Tests on release / manual dispatch | **No tests in CI** (tag, release-draft and doc workflows only) |
| Windows | win-x64 artifact; tests not run on Windows | n/a | Build-tagged Windows process code; `taskkill`, `tasklist` | n/a (library) | RimWorld on Windows; tests are net10.0 |
| Maintainer | One (pardeike) for all five | | | | |

Protocol stability:

- **GABP 1.0 calls itself stable.** It is "Version 1.0, Status: Stable" (`GABP/SPEC/1.0/gabp.md:3-5`), wire tag `gabp/1`, with only additive changes until `gabp/2` (`GABP/VERSIONING.md:12-18`).
- **The implementations deviate from it:**
  - GABS sends `tools/call` arguments as `parameters` (`GABS/internal/gabp/client.go:565-568`, checked), where the spec and schema require `arguments` and reject extra properties (`GABP/SPEC/1.0/wire-model-decisions.md:20-36`, `GABP/SCHEMA/1.0/methods/tools.call.request.json:35-41`). Lib.GAB accepts both, preferring `parameters` (`Lib.GAB/Lib.GAB/Server/GabpServer.cs:546-547`), so the pair works together, but a spec-conforming client and GABS differ.
  - GABS never writes the `Content-Type` header the spec makes mandatory (`GABP/SPEC/1.0/transport.md:46-64`; `GABS/internal/util/jsonrpc.go:218`).
  - The spec has the bridge write `bridge.json` under `%APPDATA%\gabp` with mode 0600 (`GABP/SPEC/1.0/transport.md:149-175`, `GABP/SPEC/1.0/security.md:120-128`). GABS hands the port and token over through environment variables, and keeps a diagnostic `~/.gabs/<id>/bridge.json` with mode 0644.
  - The GABS client puts no upper bound on `Content-Length` (`GABS/internal/util/jsonrpc.go:176-195`).
- **Lib.GAB framing bug (checked).** The reader decodes each 8 KB chunk to a string, then compares the byte count from `Content-Length` with a count of characters, and slices by characters (`Lib.GAB/Lib.GAB/Transport/TcpTransport.cs:187`, `:227-229`). A request with non-ASCII text has fewer characters than bytes. It therefore waits for data that is not coming, or takes part of the next message. A multi-byte character split across two reads is also corrupted. Requests carrying Chinese mod names, save names or text-field values, which RimArcho's players have, would hang or fail. No test covers non-ASCII.

## Recommendation

| Option | Value | Cost | Risk | Recommend |
| --- | --- | --- | --- | --- |
| (a) Reference only, plus one optional SKILL.md line for DecompilerServer and RimBridgeServer's source as a UI cookbook | Medium: better code finding for agents that have DecompilerServer; nothing to maintain | Minutes | None | **Yes** |
| (b) `test_run` loads RimBridgeServer, Archo MCP forwards its tools | Medium for UI exploration on 1.6 only | Large (lifecycle, budget, policy, mod distribution) | High (isolation by filter, observer effects, no 1.7, framing bug, single maintainer moving fast) | No; revisit only if transcripts show repeated relaunch-to-explore loops, after a 1.7 build and a framing fix exist |
| (c) Archo's own GABP client in Go | None alone | Small to medium | Low | No (only as part of b) |
| (d) Probe template hosts GABP through Lib.GAB | Low: a new tool still means rebuild and relaunch | Medium | Medium | No |
| (e) Borrow helpers into the template when a scenario needs them; document the agent-run GABS sandbox recipe as an option in the skill | Low to medium | Small | Low | Yes, on demand |
| Wrap or bundle DecompilerServer in Archo | Low: an agent can configure it directly | .NET 10 runtime dependency, a bigger zip, tool budget | Contradicts 17 §1 | No |

Follow-ups this research leaves open:

- **DecompilerServer on real game DLLs:** load a real 1.7 `Assembly-CSharp.dll` in the VM and measure its memory on 1.6 and 1.7, before SKILL.md promises anything for 1.7.
- **Steam and `archo test run`:** the VM check described in the side finding (§3).
- **Research index:** this report is not listed in `docs/research/README.md`; the task asked for a commit of the report alone.
