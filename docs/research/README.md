# Research

Investigations, measurements and open questions; once a report's conclusions are in a specification and nothing still depends on it, it is removed (git history keeps it).

## Log corpus and agents

| Document | What it is | Status |
| --- | --- | --- |
| [00-log-corpus-design.md](00-log-corpus-design.md) | Goals (G1–G11), collection and cleaning rules for the public HugsLib log corpus | Active; open questions in §10 |
| [01-pattern-library-integration-draft.md](01-pattern-library-integration-draft.md) | Pre-integration draft of the error pattern library in MCP | Integrated (06 §5); kept as the corpus track's record |
| [02-common-mod-authoring-error-patterns.md](02-common-mod-authoring-error-patterns.md) | The most common loading-phase errors in player logs, for Mod authors | Active (v1; runtime and save errors still to do, §5) |
| [decisions-corpus.md](decisions-corpus.md) | Corpus P1 decisions and measurements (attribution, ordering, severity calibration) | Evidence for severities and constraint strength (05 §4.3, 08) |
| [mcp-eval-design.md](mcp-eval-design.md) | Design, cases and aggregate results of the Archo MCP A/B evaluation | Active |
| [2026-11-04_mcp-transcript-review.md](2026-11-04_mcp-transcript-review.md) | Round 1 transcript review: where agents lost time with `archo mcp` | Evidence for known noise (06 §1); proposals in §7, status in round 2 §6 |
| [2026-11-05_mcp-transcript-review-round2.md](2026-11-05_mcp-transcript-review-round2.md) | Round 2 transcript review (h1, h2, h5; Pi and dsh) | Open: proposals in §7 |
| [2026-11-08_ai-mod-repair.md](2026-11-08_ai-mod-repair.md) | What RimArcho must provide for agents to fix Mods for players: zero setup, copies, proof, fix records; where C# rebuilding fits | Proposal (ToDo: Archo Agent); order in §5 |

## Performance and memory

| Document | What it is | Status |
| --- | --- | --- |
| [2026-10-24_small-file-reads-and-load-memory.md](2026-10-24_small-file-reads-and-load-memory.md) | Directory traversal and first-open costs; list load cost model and the 1.7 measurements | Traversal in 18 §2; kept as evidence for the 1.7 `loadcost.Coef` texture rates (§4) |
| [2026-10-31_unity16-memory-calibration.md](2026-10-31_unity16-memory-calibration.md) | Unity 1.6 private-byte calibration in the VM: baseline, per-Mod and texture terms | Evidence for `loadcost` (16 §2.17) |
| [2026-11-01_unity16-heavy-texture-memory.md](2026-11-01_unity16-heavy-texture-memory.md) | The 1.6 calibration extended to 7.5 GiB of texture payload | Evidence for `loadcost.Coef` and `LoadingBudget` (16 §2.17) |
| [2026-11-01_unity16-texture-memory-breakdown.md](2026-11-01_unity16-texture-memory-breakdown.md) | Where the 1.6 texture memory goes: the driver's copy, Mono, Unity | Evidence for `Coef.TextureGPU16` (16 §2.17); open: measurement on a real graphics card |
| [dds-texture-optimization.md](dds-texture-optimization.md) | How the game loads PNG and DDS textures; todds; PNG to DDS measurements | Implemented (10 §2); open: in-game visual checks (§6) and decisions (§7) |
| [2026-10-26_page-load-time-and-optimization.md](2026-10-26_page-load-time-and-optimization.md) | Backend load times of every page on real data, with ranked optimizations | Open: P1–P3 done, P4–P8 not all done (§7) |
| [2026-10-27_batch-operation-timing-audit.md](2026-10-27_batch-operation-timing-audit.md) | Timing of every batch action and the causes of slow previews | Open: recommendations in §8 not all done |

## Steam, SteamCMD and downloads

| Document | What it is | Status |
| --- | --- | --- |
| [2026-11-05_workshop-distribution.md](2026-11-05_workshop-distribution.md) | Publishing RimArcho on the Workshop and updating from it with anonymous SteamCMD | Open (ToDo); tests in §4 |
| [2026-10-26_steamcmd-parallel-downloads.md](2026-10-26_steamcmd-parallel-downloads.md) | Parallel and batched SteamCMD downloads, shared-library races | Batching in 11 §1; kept as evidence for the Workshop distribution research |
| [2026-10-24_login-sessions-loverslab-steamcmd.md](2026-10-24_login-sessions-loverslab-steamcmd.md) | LoversLab and SteamCMD login sessions: mechanisms and boundaries | Implemented (11 §3, §4); kept as evidence for the Workshop distribution research |

## VM testing

| Document | What it is | Status |
| --- | --- | --- |
| [2026-10-27_vm-automation-test-plan.md](2026-10-27_vm-automation-test-plan.md) | VM automation plan: player-action map, scenario contract, budgets, scenarios | In use: §3, §5–§7 cited by [release-vm-tests.md](../../UserGuide/release-vm-tests.md) and `scripts/vm/phase1a.md`; §2 is a 2026-10-27 snapshot |
| [2026-11-05_release-1.0.1-vm-round.md](2026-11-05_release-1.0.1-vm-round.md) | Pre-release VM round for 1.0.1: sessions, bugs found and fixed | Earlier release round |
| [2026-11-07_release-1.0.6-vm-round.md](2026-11-07_release-1.0.6-vm-round.md) | VM round for 1.0.6: sessions, 8 bugs found and fixed, open items | Latest release round; cited by release-vm-tests.md |
| [2026-10-27_vm-scenarios-5-6-9.md](2026-10-27_vm-scenarios-5-6-9.md) | VM scenarios 5, 6, 9 on real data (subscribe, unsubscribe and recycle, save and launch) | Method cited by release-vm-tests.md; open: B6, B7, B10, B12–B15 (§7) |
| [2026-10-27_vm-real-home-round-and-scenarios-7-8.md](2026-10-27_vm-real-home-round-and-scenarios-7-8.md) | Real-home random-player rounds, export/reimport, post-sort repair | Open: §5 item 3 (save says "no changes" while links are pending) |
| [2026-10-28_vm-phase-markers.md](2026-10-28_vm-phase-markers.md) | Full real rounds with Archo Monitor phase markers on 1.6 and 1.7 | Cited by release-vm-tests.md; open: B2 (game download also fetches subscribed Workshop items), B3 (a 1.7 Vulkan crash is not shown) |

## Other

| Document | What it is | Status |
| --- | --- | --- |
| [2026-10-24_game-test-runs-rimdevtools.md](2026-10-24_game-test-runs-rimdevtools.md) | How RimDevTools tests a game, and test-run measurements on 1.6 and 1.7 | Implemented (20 §2); open: §2.1 log locations are not yet in 10 §3 (pack logs on 1.7) or 17 §6 |
| [2026-11-01_decompile-storage.md](2026-11-01_decompile-storage.md) | Where decompiled code lives: the Source library and Mod projects | A and B in 05 §5.4; open: C, Mod projects (ToDo) |
| [2026-11-02_gui-accessibility.md](2026-11-02_gui-accessibility.md) | Screenshot-free GUI acceptance for agents: criteria, baseline, fix rounds | Criteria in 16 §11 (A1–A18); open: failing criteria (01 §4), proposed rule and open decisions |
| [2026-11-04_save-format-fields.md](2026-11-04_save-format-fields.md) | Element paths of a save's full read (L1), from the game's `ExposeData` | Implemented (12 §1.7); open: not yet checked against real saves |
