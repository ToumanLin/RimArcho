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

### 4.4 Not implemented / known deviations

- 1.6 could not be calibrated in this round (2026-10-24): `D:\rimworld16` cannot currently reach the main menu (a `_Archo_env_*` junction in Mods points to a deleted directory; see the game-test session record).
- Smaller GPUs were not tested; their video-memory upper bound is capped by the upload threshold, as explained in the UI.
- Language files are counted only, not estimated; saves, maps, and growth during play are excluded.
- Duplicate Def paths (higher-priority load folders overriding same-name XML in lower-priority folders) are not deduplicated, slightly overestimating cost.
- During startup the game creates and deletes something in its root directory (root mtime changed, no file remained). This is the game's own behavior; we did not write to the game directory.
