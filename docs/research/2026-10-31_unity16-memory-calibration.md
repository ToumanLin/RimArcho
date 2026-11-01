# Unity 1.6 memory calibration (2026-10-31)

Measured target: Win11-Player-Test, RimWorld 1.6.4871 rev591, 8 GiB RAM, Unity 2022.3.35f1 / D3D11 / VMware SVGA 3D. Texture compression enabled. Restored `RimArcho-Real-Vulkan` at start and end; only the vanilla group after restoration is called "first". Subsequent lists shared the same guest session without reboot, so are not strictly cold-cache runs. 1.7 and video memory are outside this calibration.

## Metrics and method

Recorded process private bytes, working set, peak working set, system available physical memory, and commit every second. Resident cost is median private bytes 55–85 seconds after the first successful main menu; startup peak is maximum private bytes from process start until first successful main menu; post-load is the median 30–55 seconds after the success event. Working set is recorded separately, not used to fit commit.

One complete flow per list: main menu stable for 90 seconds → temporarily set the in-game Prefs property to a 14-day autosave interval → new colony (150×150, 5% world, peaceful) → normal speed for 120 seconds → save under a new name → reload → 60 seconds → main menu → restore 1 day, 5 saves → normal exit. Prefs.xml was not edited manually; original values were checked read-only after exit, and ticks did not reach a day. Phase 3 did not traverse Mod tabs. 250-light reused one full flow from phase 2, with tab/pause-window interference.

The vanilla group includes 6 official entries plus Harmony and Archo Monitor; the driver DLL was temporarily added to Monitor's 1.6 Assemblies. Measured Harmony's two DLLs at 2,503,168 bytes total, Monitor's 1.6 DLL at 40,448 bytes, ArchoDrive at 32,256 bytes. Local managed-GC difference at driver initialization was about 0.4–0.5 MiB, not total driver overhead. The light fit gives about 2.13 MiB fixed per Mod, allocating about 4.26 MiB to the two measurement Mods, plus about 4.9–12.1 MiB from the model's assembly term. This is an estimate: without paired runs lacking the measurement stack, Harmony, Monitor, and driver overhead cannot be identified independently.

## Results

All values below are GiB (2^30 bytes). Mod counts include 6 official entries; the fitted fixed term excludes official entries.

| List | Mod count | Main-menu resident | Startup private peak | peak working set | After save load |
| --- | ---: | ---: | ---: | ---: | ---: |
| Vanilla and measurement stack | 8 | 1.443 | 2.414 | 2.353 | 1.824 |
| 250-light (phase 2) | 250 | 3.981 | 5.563 | 4.250 | 4.266 |
| 500 (fallback after 720 failed) | 500 | 8.237 | 10.866 | 4.546 | 8.745 |
| 250 loadable texture group | 250 | 5.384 | 7.346 | 4.683 | 5.682 |
| Same 250 group warm start (main menu only) | 250 | 5.428 | 7.085 | 5.240 | — |
| 720 | 720 | Not reached | ≥18.146 | 4.773 | — |
| 250 extremely heavy textures | 250 | Not reached | ≥18.461 | 4.718 | — |

720 crashed during about 20 minutes of sampling; the process was gone when checked after stopping the script. Stack involved AudioClip.CreateUserSound/RuntimeAudioClipLoader; minimum available physical memory was about 5 MiB. No stable value or complete flow can be claimed for this group. Extremely heavy 250 crashed after about 10 minutes, explicitly recording `OutOfMemoryException`, `System out of memory`, and `MemoryLabel: Texture` (request about 52 MiB). Both had system commit peaks about 22.85 GiB; observed process peaks are only lower bounds for incomplete loading. A 500 warm start hit a DX11 fullscreen 1024×768 resolution-switch failure Error window before Mod loading; the test process was closed, run marked invalid, and excluded from fitting.

Unity statistics support retaining heap cost: vanilla menu Mono used/reserved about 189/234 MiB; 500 menu about 1367/1511 MiB, reserved about 1735 MiB after stable waiting; 250 texture-group menu about 1259/1377 MiB. For 500, Unity GFX allocator peaked around 1.43 GB, Default Main around 0.59 GB. Unity used/reserved and system private bytes measure different things and cannot be added. Raw values, phase markers, old CLI estimates, and trimmed logs are in task output `vm-calib/`, with per-round `*-summary.json` / `*-memory.csv` / `*-drive.log` / `*-player-small.log`.

## Fitting and implementation

Subtract the estimated midpoint of measurement-assembly cost from stable vanilla to obtain about 1.434 GiB baseline. For light / loadable texture groups, each with 244 unofficial Mods, first subtract midpoint differences in audio, XML, assemblies, and other categories, eliminating the fixed Mod term and yielding a texture coefficient about 1.625. Then subtract baseline, texture, and other terms from light to get about 2.13 MiB fixed per Mod. These two 250 groups are not a strict experiment changing only textures, so this is an effective coefficient including correlated Mono heap, Unity/driver, and atlas effects; it does not separately measure each mechanism.

Current coefficients are maintained only in `loadcost.Coef`, `Estimate`, `Baseline`, and `LoadingBudget`: the fit uses a 1.43 GiB baseline, texture RAM 1.6–1.8 times payload, and about 1.8–2.5 MiB fixed per unofficial Mod; existing audio/XML/DLL terms remain. Loading reserve combines fixed startup headroom, texture volume, largest PNG decode, and XML/language volume. 1.7 texture/resident terms and original loading-reserve formula remain unchanged.

| Valid group | New resident range GiB | New loading budget GiB |
| --- | ---: | ---: |
| Vanilla and measurement stack | 1.438–1.447 | 2.338–2.747 |
| 250-light | 3.649–4.472 | 4.717–6.303 |
| 500 | 7.001–9.355 | 8.326–12.240 |
| 250 loadable texture group | 5.008–6.052 | 6.259–8.341 |

All four valid groups and the 250 warm-start stable/startup-peak values fall within their respective ranges; synthetic regression `TestUnity16CalibrationCoverage`, using anonymous numerical aggregates, fixes this fact while limiting resident-range width. Failed groups are not fitted as stable targets. Old 250 texture-group estimates were resident 2.3–2.8, loading 2.4–3.0 GiB, far below measured 5.384/7.346, confirming the old 1.6 coefficients were insufficient.

## Limits and findings

Covers only one 8 GiB VM with texture compression enabled on Unity 1.6; compression disabled, real-GPU video memory, 1.7, and driver-copy/paging differences at other physical-memory sizes were not verified. AssetBundle and Mod-specific allocations cannot be fully inferred from normal resource headers; 720 and extremely heavy 250 include such loading, so no estimate upper bound is promised. Maps, saves, and runtime growth remain excluded.

Although dependencies were completed and full flows finished for 500 and loadable 250, Patch/xref/Harmony runtime errors remained; the model does not establish these combinations are playable. RimArcho game logs and can it run were checked for vanilla, 500, and loadable 250; GUI checking for 720 was incomplete. The first 720 launch also hit CLI `SQLITE_BUSY store:index generation` while the test GUI was still in the background; closing this task's GUI allowed a successful retry. An application concurrency defect is not yet proven.

Saves created in this phase: `RimArcho-calib-vanilla-1031-162627`, `RimArcho-calib-500-1031-165626`, `RimArcho-calib-250-feasible-1031-172059`. Save reused from phase 2: `RimArcho-calib-250-light-1031-160148`. 720, extremely heavy 250, and warm starts created no saves. Snapshot restoration cannot guarantee rollback of Steam cloud saves.
