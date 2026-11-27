# Small-file reads and list load cost (2026-10-24)

Branch `claude/smallfile-io-9071ae`. Code: `internal/fsscan` (traversal, sizes, fingerprints, caching), `internal/loadcost` (RAM / video memory estimates), `spikes/fsscan` (read-only benchmark).

## 1. Conclusions

1. **The Go standard library on Windows already obtains sizes and times while listing directories.** `os.File.ReadDir` uses `GetFileInformationByHandleEx(FileIdBothDirectoryInfo)` with a 64 KiB buffer; `DirEntry.Info()` makes no additional system call. `FindFirstFileEx(FindExInfoBasic, FIND_FIRST_EX_LARGE_FETCH)` is slower; 256 KiB `FileFullDirectoryInfo` is only 0–5% faster. Do not implement another native enumerator; retain only the open mode requiring directory-list permission alone (`fsscan/open_windows.go`).
2. **Concurrency is the biggest variable.** Listing all 1,315 Mod folders: 3.7 seconds single-threaded, 0.61 seconds with 8 workers, 0.55–0.61 seconds with 16; more workers add no speed. `fsscan.Workers` uses 16 for SSD, 2 for HDD, 8 when unknown (seek-penalty detection through `IOCTL_STORAGE_QUERY_PROPERTY`, no administrator needed). No local HDD was available; the HDD setting of 2 is **unmeasured**.
3. **Opening a file for the first time is expensive; listing directories is not.** With Defender real-time protection enabled, the first open of a PNG to read 33 bytes takes about 1.3 ms/file single-threaded (780/s), 6,000/s with 8 workers, 10,000/s with 16; 32 drops to 4,800/s. Second reads of the same files reach 30,000–200,000/s. Caching should therefore avoid reopening files whose size and mtime are unchanged (scan hashes, PNG headers on the texture page, and per-Mod load-cost results all follow this rule). Use 8–16 workers for first reads.
4. **Skipping by directory mtime is not worthwhile.** Statting all directories (0.27 seconds) is only twice as fast as listing everything (0.55 seconds), and in-place file overwrites do not change directory mtime, so updates would be missed. Not implemented.
5. **The USN journal requires administrator access** (opening `\\.\D:` was denied), so it is not used. `ReadDirectoryChangesW` works only while the program runs; it could later invalidate GUI caches while open, but was not implemented here.
6. **Batch reads of small files**: existing reads already read only headers (PNG 33 bytes, DDS 128 bytes, WAV headers). The bottleneck is the fixed cost of each open, not read volume; combining reads offers no benefit.

## 2. Measurement environment

Windows 11, Ryzen 9 7950X (32 threads), D: Nextorage NVMe SSD, Go 1.27.2, Defender real-time protection enabled, non-administrator. Workshop `294100` + game `Mods`: 1,315 folders, 84,093 directories, 285,133 files, 23.1 GB, all read-only. Without administrator privileges the system file cache could not be cleared; "cold" means the first access to these files since boot.

## 3. Enumeration and read measurements (milliseconds, median of the last two of 3 runs)

| Method | 1 | 4 | 8 | 16 | 32 |
| --- | ---: | ---: | ---: | ---: | ---: |
| `filepath.WalkDir`, concurrent by Mod (existing approach) | 2,919 | 874 | 611 | 673 | 689 |
| Shared directory queue + `ReadDir` (fsscan approach) | 3,727 | 1,127 | 737 | 607 | 568 |
| `FindFirstFileEx` LARGE_FETCH | 2,946 | 1,345 | 1,000 | 805 | 676 |
| 256 KiB `FileFullDirectoryInfo` | 2,811 | 978 | 611 | 554 | 586 |
| Stat directories only (directory-mtime cache cost) | 932 | 327 | 268 | 279 | 294 |
| `scan.ComputeSkip`, rehash everything | 7,479 | 2,437 | 1,528 | 1,117 | 1,078 |
| `scan.ComputeSkip`, reuse hashes (unchanged rescan) | 3,591 | 1,135 | 718 | 504 | 499 |
| `textures.Fingerprint` (texture-page check) | 2,497 | 851 | 536 | 381 | 361 |
| `scan.ReadDecl`(About.xml) | 240 | 74 | 47 | 31 | 27 |

- Serial `WalkDir`: 3,713 ms. First full hash (cold contents) single-threaded: **178 seconds**; 7.5 seconds on the same batch afterward.
- Comparing `fsscan.Walk` / `fsscan.Size` with the shared directory queue in the same process showed a gap within 3% (a mid-run fix addressed a single dispatcher goroutine and GC discarding the standard library's 64 KiB directory buffers, which had nearly doubled the time).

**Before and after migration** (same machine, alternating old/new runs; game calibration was running, adding noise to the two scan rows):

| Caller | Before | After |
| --- | ---: | ---: |
| `dirSize` (pack size, uninstall): entire Workshop | 5,055 ms | **754 ms** |
| `dirSize`: largest Mod (5.1 GB) | 4.6 ms | 2.6 ms |
| Scan `ComputeSkip`, 16 workers, full hash | 1,118 | 1,019 |
| Scan `ComputeSkip`, 16 workers, unchanged | 768 | 686; allocations 586 → 246 MB |
| Texture-page summary concurrency | 4 | `min(8, Workers)` (18 §2 measured 4 → 8:584 → 352 ms) |
| Scan concurrency per source | `NumCPU` (32) | `fsscan.Workers` (SSD 16, HDD 2) |

## 4. List load cost

### 4.1 How the game loads resources (confirmed by decompilation)

**1.6 (Unity)**: PNGs decode with `LoadImage`; with both dimensions divisible by 4 and texture compression enabled (the default), they compress to DXT (1 byte/pixel); otherwise they remain RGBA32 (4 bytes). Both have mipmaps (about ×4/3), with no CPU copy after upload (`makeNoLongerReadable`). Item textures smaller than 512 pixels are also copied into static atlases. Audio clips up to 300 KB decode fully into memory; larger ones stream. `.dds` uploads unchanged and shadows same-name PNGs.

**1.7(Waylith,.NET + Vulkan)**:
- PNG / JPG / PSD decode and then encode to **BC7** (1 byte/pixel; opaque grayscale uses BC4; textures with both dimensions ≤16 pixels stay RGBA32), with full mipmaps and no divisibility-by-4 requirement. Encoded results go to `<data directory>/TextureCache/v23/` for direct reads next time. No CPU copy is retained. Compression cannot be disabled (`Prefs.TextureCompression` is always true).
- **Upload on demand**: by default only textures already drawn reside in video memory. After a fraction of the GPU budget is used (`max(budget/2, min(0.8·budget, budget−512 MB))`), textures unused for 60 frames are evicted. Video memory is therefore not "the sum of all textures"; smaller GPUs display less.
- After textures smaller than 512 pixels are copied to static atlases, originals are evicted from video memory, largely avoiding duplication.
- Audio: clips whose estimated decoded PCM16 size is at most 2 MiB stay in memory (WAV uses decoded data; OGG / MP3 use compressed file bytes); larger clips stream.
- The merged XML document is freed after loading, leaving Def objects; assemblies are never unloaded once loaded.

### 4.2 Measurements (1.7.4985, RTX 4090 24 GB, isolated `-savedatafolder`, sampled every 3 seconds within 150 seconds after the main menu appeared)

From the current `default` list (484 Mods, including 6 official), take the first 100, first 250, and all entries in load order. "First" means an empty texture cache; "repeat" means the second launch with the same data directory.

| List | Main menu reached | Private bytes | Working set | Video memory |
| --- | ---: | ---: | ---: | ---: |
| Vanilla + all DLCs | 12 s | 2.30 GB | 1.34 GB | 1.25 GB |
| First 100 (first / repeat) | 85 s / — | 5.3 / 4.24 GB | — / 1.62 GB | 2.1 / 3.0 GB |
| First 250 (first / repeat) | 128 s / — | 6.7 / 7.66 GB | — / 3.69 GB | 2.4 / 4.82 GB |
| All 484 (first / repeat ×3) | 353 s / 91–115 s | 15.1 / 21.9–22.1 GB | 8.2 / 7.2–7.7 GB | 7.3 / 15.3 GB |

- The three repeat launches were nearly identical, all using 15,283 MB video memory: this is the engine's upload threshold (about 65% of 24 GB), not the actual texture size. Working set peaked at about 15 GB during loading.
- This list has 30,080 DDS files (5.2 GB payload) shadowing most PNGs; only about 900 PNGs actually load.

### 4.3 Model (`internal/loadcost`, all coefficients in `Coef`)

> Correction, 2026-10-31: the 1.6 texture-memory multiplier ×0–0.1 and baseline below were not measured. `makeNoLongerReadable` does not mean textures, Mono heap, decoding/compression buffers, atlases, or driver copies consume no system memory; it does not establish that 1.6 uses less memory than 1.7. This row records the old assumption only; the current 1.6 model was replaced by the [2026-10-31 VM calibration](2026-10-31_unity16-memory-calibration.md).

| Component | 1.7 | 1.6 (rules only, unmeasured) |
| --- | --- | --- |
| Texture payload | PNG as BC7 + mipmaps (≤16 pixels as RGBA32); DDS file minus header; unknown JPG / PSD dimensions estimated as file bytes ×4–×12 pixels | DXT or RGBA32 + mipmaps, adding static atlases to the upper bound |
| Video memory | Payload × **0.9–2.6** (measured) | Payload |
| RAM | Payload × **1.5–3.5** (measured, including Vulkan memory retained by the engine in system memory) | Payload × 0–0.1 |
| Audio | Bytes of files retained in memory × 1–1.5 | Decoded PCM for files ≤300 KB × 1–2 |
| Def XML | Bytes × 3–8 | Same as left |
| Patch XML | Bytes × 0.5–2 | Same as left |
| Assemblies | Bytes × 2–5 | Same as left |

Add the measured vanilla + DLC baseline (1.7: RAM 2.3 GB, video memory 1.2 GB; 1.6's 2.0 / 1.0 GB are unmeasured placeholders).

**Comparison**: estimates (baseline + Mods) vs. measurements: first 100: RAM 4.1–7.0 / measured 4.2–5.3, video memory 2.3–4.6 / measured 2.1–3.0; first 250: RAM 5.6–10.4 / measured 6.7–7.7, video memory 3.1–6.9 / measured 2.4–4.8; all: RAM 11.0–23.2 / measured 15.1–22.1, video memory 6.2–16.0 / measured 7.3–15.3. Only the first launch of the first 250 had video memory (2.4 GB) below the lower bound. **Ranges span about a factor of two**: first and repeat launches of the same list differ that much, and testing covered only one machine with a 24 GB GPU.

**Cost**: 476 Mods took 47 seconds on the first pass (mostly opening 30,000 DDS headers for the first time), then 0.45 seconds (`cache/loadcost-v1.json`, keyed by the fsscan listing fingerprint of each Mod's load folders; PNG headers already read on the texture page are reused).

> Correction, 2026-11-07: players with 8 GB cards were shown 10–28 GB of VRAM and 18–41 GB of RAM for 270 Mods that load fine. Waylith 1.7.5000 (decompiled `TextureResidency`, `Texture2D`, `RimWorldWaylith.Bootstrap`) uploads on demand unless `-EagerTextureUpload` is passed. Above `HighWatermark(budget) = max(budget/2, min(0.8·budget, budget − 512 MiB))` of the VMA device heap budget it evicts textures to the disk cache, or, without a valid cache, moves them to a host heap capped at a quarter of that heap's budget. The 15,283 MB above is that watermark for a budget of 0.78 × 24 GB. The ×0.9–2.6 and ×1.5–3.5 rates therefore hold only below the watermark. Mod RAM was 1.1–1.5× the resident mod VRAM on repeat launches (100 / 250 / 484) and 2.1× on the 484 first launch. `loadcost.Cap17` caps both at the card's watermark (16 §2.17). No run on a smaller card has been made.

### 4.5 Working set and 1.7.5021 (2026-11-26, GitHub #31)

A player with 621 Mods, an RTX 5080 (16 GB) and 32 GB RAM (8 GB free) was told "RAM: loading needs 13.0 GB–23.8 GB … RAM is tight" and called it wildly too high. The figure was private bytes (commit) compared with free physical RAM, while players read Task Manager's **Memory** column, which is the **private working set**. One calibration on this host answers what the game actually keeps in RAM.

**Method.** 1.7.5021 rev1365, RTX 4090 24 GB, 64 GB RAM, Windows 11. `RimWorld.exe` launched directly (Steam client running, not used to launch), `-savedatafolder=D:\RimArcho-test\mem17\data-<list>` and `-NoFileLog` (console captured), a 1280 × 720 window, Mods read in place. The 1.7 bootstrap deletes the Unity-era `Player.log` / `Player-prev.log` in the real LocalLow root (`Bootstrap.DeleteOldUnityLogs`): they were copied aside and restored after each run. Every 3 s: working set, private bytes, peak working set (process), private working set (`Win32_PerfFormattedData_PerfProc_Process.WorkingSetPrivate`) and `GPU Process Memory … Dedicated Usage`. Main menu = `Config/Knowledge.xml` written; figures are medians 60–150 s after it. Lists: Core + 5 DLCs; the first 250 and all 724 entries of the host's Mod list in load order (235 and 665 non-official Mods found); and the #31 list as posted (621 entries, about 200 of its Mods installed here). Smaller cards were emulated with Waylith's `WAYLITH_VRAM_BUDGET_MB` (caps the device-local heap). The 724 list restarts itself once at start-up (a Mod restarts the game after patching); the sampler followed the new process.

| List (non-official Mods) | Launch, card | Menu s | Private WS (Task Manager) | WS | Private bytes | Peak private | GPU dedicated |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Vanilla + DLC | first / repeat, 24 GB | 11 / 11 | 1.15 / 1.15 | 1.35 / 1.35 | 1.74 / 1.74 | 1.84 | 0.39 |
| First 250 (235) | first, 24 GB | 27 | 2.38 | 3.01 | 3.47 | 3.72 | 0.62 |
| First 250 (235) | repeat, 24 GB | 19 | 1.66 | 2.27 | 2.39 | 2.46 | 0.55 |
| All 724 (665) | first, 24 GB | 164 | — | 12.35 | 10.20 | 12.42 | 3.43 |
| All 724 (665) | repeat, 24 GB | 103 | 4.47 | 10.09 | 8.00 | 9.11 | 3.35 |
| All 724 (665) | repeat, 16 GB (emulated 15,974 MB) | 87 | 4.85 | 10.48 | 8.45 | 9.73 | 3.36 |
| All 724 (665) | repeat, 8 GB (emulated) | 87 | 5.12 | 10.73 | 8.80 | 10.75 | 3.42 |
| All 724 (665) | repeat, 4 GB (emulated) | 87 | 5.06 | 10.67 | 8.60 | 9.23 | 3.22 |
| All 724 (665) | repeat, 2 GB (emulated) | 87 | 5.02 | 10.62 | 8.14 | 8.30 | 2.94 |
| #31 list (≈200 present) | first, 24 GB | 108 | 4.07–5.29 | 4.79 | 6.35 | 8.12 | 1.71 |
| #31 list (≈200 present) | repeat, 24 GB | 47 | 2.67–3.16 | 3.33 | 4.66–5.09 | 5.09 | 1.78 |

GiB. "—": the private-working-set sampler started after that run. Peak private is the sampled maximum from launch.

**Findings.**

1. **Working set ≠ what Task Manager shows.** On 1.7 the process working set includes several GiB of shared pages (the memory-mapped texture cache): 10.1 GiB working set but 4.5 GiB private working set for the 724 list. Private working set is the figure to show.
2. **1.7.5021 commits much less than 1.7.4985.** Vanilla private bytes fell from 2.30 to 1.74 GiB; the 484-Mod repeat launches of §4.2 reached 22 GB, the 724-Mod ones here 8.0–8.8 GiB. Private working set / private bytes: vanilla 0.66, 250 0.69, 724 0.56–0.62, #31 part 0.57–0.62 (§4.2's 0.33–0.55 on 1.7.4985).
3. **Neither figure follows the card.** From 24 GiB down to an emulated 2 GiB card, private working set stayed at 4.5–5.1 GiB and private bytes at 8.0–8.8 GiB while dedicated VRAM fell from 3.35 to 2.94 GiB. Textures above the watermark go to the disk cache (shared pages) or to a host heap; the private figures do not shrink. So the working set has no card term. The private-bytes cap (`Cap17`, fitted on 1.7.4985) is not supported by these runs; it is kept for the commit range until a list heavy enough to pass a real card's watermark is measured (none of these lists did on 24 GB).
4. **Texture payload explains most, loosely.** The 724 list and the #31 part have about the same estimated payload (2.68 / 2.56 GiB) but settled at 4.5–5.1 vs 2.7–3.2 GiB; their measured mod VRAM differed the same way (2.96 vs 1.39 GiB): the payload estimate overstates what the #31 part actually uploads by about 1.7×. Per Mod the private working set is small (the 250 lightest-first Mods: 0.5 GiB). First launches (textures encoded into the cache) settle 0.7–2 GiB higher than repeats.

**Model** (`loadcost.WS`, `WorkingSet`): 1.15 GiB + 0.5–1.6 × payload + 0.4–2 MiB per non-official Mod + the other categories' private-bytes range, no card term, and never above the resident private range. On the measured lists: 250 → 1.49–2.36 (measured 1.66–2.38), 724 → 2.99–7.35 (4.47–5.12), #31 part → 2.64–5.99 (2.67–5.29) GiB (`TestWorkingSet17Calibration`). The 1.7 RAM baseline (private bytes) is now 1.73 GiB. 1.6: the RAM estimate is CPU-side memory on a real card; the VM breakdown (2026-11-01) put its working set at 0.75–0.95 of it (`Other16Low`–`High`); not measured on a real card.

**#31's shape.** Its report (9.7 GB VRAM kept on a 15.6 GB card, 8.3 GB on demand) implies an estimated payload of about 18.7 GiB. The headline becomes "about 11.2–21.0 GB in Task Manager" (`TestMemoryIssue31Shape`); the commit while loading is 11.9–22.0 GB; with 8 GB free the warning stays, because the working-set estimate itself exceeds it. Whether that list really uploads 18.7 GiB cannot be told here: for the ≈200 of its Mods installed on this host the payload estimate was 1.7× what the game uploaded. Archo Monitor 0.13.0 records the game's own private working set, private bytes and VRAM at the menu and in play (19 §2); a report from such a run would settle it.

**Not covered.** One PC and one GPU vendor; the emulated cards cap the heap but keep the 4090's bandwidth and driver; the payload of the heaviest measured list was 2.7 GiB; saves and play (Monitor's play records will cover them); the 1.7 VRAM baseline (1.2 GB from 1.7.4985; 0.39 GiB dedicated here) was left unchanged.

> Superseded, 2026-11-27: the texture term above (0.5–1.6 × payload) and #31's 11.2–21.0 GB were extrapolated past 2.7 GiB of payload. §4.6 measured lists up to 18.8 GiB: the term saturates, and #31's shape now gives 1.9–7.2 GiB.

### 4.6 Texture-heavy lists, 0.9–18.8 GiB of payload (2026-11-27, GitHub #31)

§4.5's lists stopped at 2.7 GiB of payload; #31's list is about 18.7 GiB. This round asks how 1.7's private working set and private bytes grow with payload, on lists clean enough to load and play.

**Method.** Same host, game and sampler as §4.5 (1.7.5021 rev1365, RTX 4090, 64 GiB RAM; private working set from `Win32_PerfFormattedData_PerfProc_Process.WorkingSetPrivate`, working set, private bytes, `GPU Process Memory … Dedicated Usage`, every 4–5 s; the real LocalLow `Player.log` / `Player-prev.log` backed up and restored after each run).
- **Mods.** 130 texture-heavy Workshop items that list 1.6 or 1.7, picked by size through the anonymous Steam Web API from searches for HD, upscaled, hair, apparel, face, race and faction Mods, downloaded with SteamCMD `+login anonymous` into a test folder; 52 were dropped (a dependency not on the host, or no 1.7 payload). The 78 kept give 20.8 GiB of estimated payload: pawn-render Mods (races, hair, apparel, faces, mechanoid skins) 13.5 GiB, world and item Mods (upscaled terrain, floors, furniture, food, plants, vanilla retextures) 5.2 GiB, other 2.1 GiB. Payload per downloaded byte was about 3.5 (upscaled PNGs compress well).
- **Lists.** Each level is a prefix of one order that alternates pawn and world Mods, with Harmony, Core, the five DLCs and the dependencies the Mods name (Humanoid Alien Races, Vanilla Expanded Framework and others, read in place from the host's Workshop folder). Payload measured with RimArcho's own estimator (`loadcost.Measure` / `Estimate`, LoadFolders for 1.7). Every level loaded all its Mods (the game's "Initializing new game with mods" list matched) with 6–73 ERROR lines. A first attempt built on the host's own list (§4.5's 724 entries) was dropped: two of its Mods break play on 1.7 (an `OnGUI` exception every frame, map layers that fail to build), so its play samples were not valid.
- **Launch.** The game cannot read Mods from a folder outside its own `Mods` and the Workshop, and the real install's `Mods` was not to be written to, so the runs used a copy of the 1.7 install (without `Mods`) in the test folder, whose `Mods` held junctions to the real local Mods and to the downloads. Started directly from the exe, `-savedatafolder` per level, `-NoFileLog`, a 1280 × 720 window.
- **Runs.** Per level: a first launch with the texture cache deleted ("first"), a repeat launch ("repeat"), and a repeat launch with `-quicktest` (a new colony of three pawns on screen, the game running; "play"). Menu figures are medians 45–75 s after `Config/Knowledge.xml` appeared; play figures 45–75 s and 165–195 s after "Initializing new game" (equal within 0.05 GiB; the first is shown). Two Mod notice dialogs paused some play runs: the map and pawns were still drawn. The 18.8 GiB level also ran with `WAYLITH_VRAM_BUDGET_MB` 8192 and 16384 (first launch and play).

| Payload (non-official Mods) | Launch, card | State | Private WS | WS | Private bytes | Peak private WS | Peak private bytes | GPU dedicated (peak) |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 (vanilla, §4.5 + this round) | repeat, 24 GB | menu / play | 1.15 / 1.86 | 1.35 / 2.09 | 1.74 / 2.60 | — / 1.86 | 1.84 / 2.61 | 0.39 / 0.51 |
| 0.91 (7) | first, 24 GB | menu | 1.48 | 1.72 | 2.30 | 3.04 | 4.23 | 0.62 (0.80) |
| 0.91 (7) | repeat, 24 GB | menu / play | 1.22 / 1.98 | 1.46 / 2.25 | 1.82 / 2.76 | 1.98 | 1.83 / 2.76 | 0.43 / 0.55 |
| 2.11 (11) | first, 24 GB | menu | 2.01 | 2.27 | 3.14 | 5.00 | 7.61 | 0.91 (2.03) |
| 2.11 (11) | repeat, 24 GB | menu / play | 1.24 / 1.98 | 1.47 / 2.25 | 1.87 / 2.84 | 2.04 | 1.95 / 2.84 | 0.47 / 0.66 |
| 6.06 (22) | first, 24 GB | menu | 2.16 | 2.44 | 3.91 | 8.86 | 14.76 | 1.41 (5.15) |
| 6.06 (22) | repeat, 24 GB | menu / play | 1.32 / 2.05 | 1.57 / 2.35 | 1.98 / 2.88 | 2.12 | 2.05 / 2.96 | 0.47 / 0.60 |
| 12.27 (44) | first, 24 GB | menu | 3.75 | 4.03 | 6.75 | 12.26 | 20.25 | 2.48 (9.70) |
| 12.27 (44) | repeat, 24 GB | menu / play | 1.56 / 2.55 | 1.83 / 2.85 | 2.22 / 3.53 | 2.55 | 2.29 / 3.53 | 0.47 / 0.66 |
| 18.75 (72) | first, 24 GB | menu | 4.36 | 4.70 | 8.67 | 15.13 | 25.78 | 3.77 (14.38) |
| 18.75 (72) | first, 16 GB (emulated) | menu | 2.97 | 3.28 | 6.42 | 15.17 | 23.19 | 3.21 (12.75) |
| 18.75 (72) | first, 8 GB (emulated) | menu | 2.84 | 3.15 | 4.84 | 17.71 | 25.22 | 1.77 (6.81) |
| 18.75 (72) | repeat, 24 GB | menu / play | 2.31 / 3.72 | 2.62 / 4.09 | 3.32 / 5.07 | 3.78 | 3.34 / 5.14 | 0.82 / 1.06 |
| 18.75 (72) | repeat, 16 GB (emulated) | play | 3.34 | 3.68 | 4.54 | 3.34 | 4.63 | 1.00 |
| 18.75 (72) | repeat, 8 GB (emulated) | play | 3.61 | 3.95 | 4.86 | 3.67 | 4.92 | 1.06 |

GiB. Peaks are the sampled maxima from launch to the end of the run (menu runs: menu + 150 s; play runs: play + 200 s); a single peak private WS on a menu / play row is the higher of the two runs. The texture cache (`TextureCache/`) held 0.21 / 0.58 / 1.23 / 1.72 / 2.52 GiB for the five levels. Two levels of the host's own list, measured before it was dropped for play (369 and 380 non-official Mods, 0.57 and 2.39 GiB of payload): menu 1.83 / 1.93 GiB repeat, 2.81 / 4.84 GiB first.

**Findings.**

1. **Settled, the working set hardly follows the payload.** From 0.9 to 18.8 GiB of payload, 20 times more, the repeat launch's menu figure went from 1.22 to 2.31 GiB and play from 1.98 to 3.72 GiB, and part of that rise is the Mods' code and Defs (7 → 72 Mods, many races). Dedicated VRAM stayed at 0.4–1.1 GiB: at the menu and in a young colony 1.7 uploads only what is drawn, and keeps no CPU copy. A first launch settles 0.3–2 GiB higher (2.84–4.36 GiB at 18.8 GiB of payload). Neither depends on the card.
2. **The first launch's peak is large, follows the payload, and not the card.** While encoding every texture into the cache, a first launch uploaded up to 14.4 GiB to the card and peaked at 25.8 GiB of private bytes (15.1 GiB private working set) at 18.8 GiB of payload; on emulated 16 and 8 GB cards the peak was the same (23.2 / 25.2 GiB; 15.2 / 17.7 GiB private working set). Beyond the baseline, the peak was 2.8× the payload at 0.9 GiB, falling to 1.3× at 18.8 GiB. A repeat launch peaked within 1.6 GiB of vanilla.
3. **What drives the working set is not the payload.** §4.5's 724 list (2.7 GiB of payload, 665 Mods) settled at 4.5–5.1 GiB, twice the 18.8 GiB heavy list. Its first 380 entries (2.4 GiB of payload) settled at 1.9 GiB: the rest of that list's Mods add 2.5 GiB with little payload. Code, Defs and Mods that load or build textures themselves weigh more than texture files.
4. **`Cap17` does not hold for the loading commit.** On an 8 GB card `Cap17` cut the 18.8 GiB level's loading budget to 6.7–11.2 GiB; the first launch peaked at 25.2 GiB on that card. The resident range it cuts is still above what was measured (8 GB card, first launch settled at 4.8 GiB of private bytes; model 6.2–10.3), so it stays for that range and for VRAM.

**Model** (`loadcost.WS`, `WorkingSet`; `LoadingBudget`; `TestWorkingSet17Calibration`):
- Working set: 1.15 GiB + 0.4–2 MiB per non-official Mod + the other categories' private-bytes range, plus a texture term of 0 (lower bound) to `min(1.6 × payload, 3.5 GiB)` (upper bound, `WS.Tex17Cap`). The cap is the largest texture share any measured list needed: the #31 part's first launch (5.29 GiB) less its other terms.
- Loading budget (private bytes, the severe level's comparison): the lower bound is a repeat launch (no texture term), the upper bound a first launch (the texture and DDS RAM range capped at the payload + 8 GiB, `Coef.Tex17LoadHighOver`), plus the XML / decode reserve, on any card (`LoadCost.FitTo` no longer applies `Cap17` to it). It holds every measured repeat and first-launch peak, the 724 list's first launch within 3% (12.42 against 12.09–12.27 GiB, short before this change too) and the 250 list's first launch excepted (3.72 against 3.3 GiB, as before).

| Payload | Measured private WS (menu, repeat – first; play) | Old estimate (§4.5) | New estimate | Measured peak private bytes (repeat / first) | Old loading budget (24 / 16 / 8 GB card) | New loading budget (any card) |
| ---: | --- | ---: | ---: | --- | --- | ---: |
| 0.91 | 1.22–1.48; 1.98 | 1.62–2.65 | 1.17–2.65 | 1.83 / 4.23 | 3.1–5.0 (all) | 1.75–4.96 |
| 2.11 | 1.24–2.01; 1.98 | 2.22–4.58 | 1.17–4.58 | 1.95 / 7.61 | 5.2–9.5 (all) | 1.75–9.15 |
| 6.06 | 1.32–2.16; 2.05 | 4.24–11.01 | 1.21–4.81 | 2.05 / 14.76 | 11.2–23.5 / 11.2–20.7 / 6.3–10.2 | 1.80–15.96 |
| 12.27 | 1.56–3.75; 2.55 | 7.41–21.14 | 1.27–4.99 | 2.29 / 20.25 | 17.4–31.5 / 11.9–21.0 / 6.4–10.6 | 1.90–22.45 |
| 18.75 | 2.31–4.36; 3.34–3.72 | 10.85–31.98 | 1.47–5.43 | 3.34 / 23.2–25.8 | 17.7–32.2 / 12.2–21.7 / 6.7–11.2 | 2.16–29.58 |

GiB. The estimates are the model's working set before `FitTo` keeps it at or below the resident range; §4.5's lists still hold (250 → 1.31–2.36, 724 → 1.65–6.54, #31 part → 1.36–5.37). The loading budgets here leave out the languages and PNG-decode reserves (under 0.1 GiB for these lists).

**#31's shape** (615 Mods, about 18.7 GiB of payload, `TestMemoryIssue31Shape`): the working set goes from 11.2–21.0 to **1.9–7.2 GiB**, so with 8 GB free there is no "RAM is tight" (below about 7.2 GiB free it returns); the loading budget goes from 11.9–22.0 GiB (16 GB card) to 2.2–29.7 GiB: a repeat launch needs little, a first launch with every texture new may reserve up to about 30 GiB of RAM plus page file.

**Not covered.** One PC, one GPU vendor; emulated cards keep the 4090's driver and bandwidth. Play was a three-pawn colony for three minutes: a large colony, raids with many modded pawns, or the world map can draw far more textures than that, and Archo Monitor's play records are the way to see it. The heavy lists hold 7–72 Mods; lists of several hundred code-heavy Mods (like §4.5's 724) settle higher for reasons other than textures, which the per-Mod and other-category terms only roughly cover. Whether #31's list behaves like these lists can only be told by a Monitor record from the player.

### 4.4 Not implemented / known deviations

- 1.6 could not be calibrated in this round (2026-10-24): `D:\rimworld16` cannot currently reach the main menu (a `_Archo_env_*` junction in Mods points to a deleted directory; see the game-test session record).
- Smaller GPUs were not tested; their video-memory upper bound is capped by the upload threshold, as explained in the UI.
- Language files are counted only, not estimated; saves, maps, and growth during play are excluded.
- Duplicate Def paths (higher-priority load folders overriding same-name XML in lower-priority folders) are not deduplicated, slightly overestimating cost.
- During startup the game creates and deletes something in its root directory (root mtime changed, no file remained). This is the game's own behavior; I did not write to the game directory.
