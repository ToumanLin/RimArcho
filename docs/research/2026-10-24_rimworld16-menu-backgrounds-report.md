# RimWorld 1.6 menu background extraction

Date: 2026-10-24

RimWorld 1.6.4871 rev590 stores all six official main-menu backgrounds in one streamed Unity resource file. The implementation now extracts them with pure Go, preserving the existing JPEG cache and official package keys. Each background whose extraction fails retains its official About/Preview.png fallback. No DLC background was skipped on the supplied installation, and no dependency was added.

## Scope

Authorization: the requested local compatibility fix and read-only resource-format research. Inputs: `D:\rimworld16`, the reference 1.7 installation at `D:\SteamLibrary\steamapps\common\RimWorld`, and the existing reference images. Only the current `codex/r3-menu-bg-16` worktree contains generated code, docs, caches and review images. Network access and game execution were unnecessary. The game installations were opened for reading only. This is asset-format research; executable imports, security severity and attack indicators do not apply.

## Evidence and findings

E1: `Version.txt` and the SerializedFile headers identify 1.6.4871 rev590, Unity **2022.3.35f1**, SerializedFile **22**, with big-endian headers, little-endian metadata/objects and no embedded type trees. `resources.assets` is 7,370,912 bytes; its metadata is 150,433 bytes and object data starts at byte 150,496. `resources.assets.resS` is 254,078,496 bytes. The other large `resources.resource` file is unnecessary for these textures.

E2: the ResourceManager (class 147, object ID 13) in `globalgamemanagers` contains 6,249 path mappings. Its external file ID **3** is `resources.assets`. For every background, the key is `textures/` plus the lower-case ExpansionDef.backgroundPath; the PPtr resolves exactly to a Texture2D (class 28):

| Expansion | Resource suffix under textures/ui/heroart/ | Object ID | Source pixels | Stream offset | Stream size |
| --- | --- | ---: | --- | ---: | ---: |
| Core | bgplanet | 1342 | 4096 × 2560 | 93,583,456 | 10,485,760 |
| Royalty | menubg_royalty | 891 | 3840 × 2160 | 54,272,000 | 8,294,400 |
| Ideology | menubg_ideology | 2646 | 4096 × 2304 | 177,945,264 | 9,437,184 |
| Biotech | menubg_biotech | 3014 | 4096 × 2304 | 215,615,856 | 9,437,184 |
| Anomaly | menubg_anomaly | 2962 | 4096 × 2304 | 204,015,584 | 9,437,184 |
| Odyssey | menubg_odyssey | 289 | 3840 × 2160 | 9,976,576 | 8,294,400 |

All six use **TextureFormat 25 / BC7**, a single mip and `m_StreamData.path = resources.assets.resS`. No inline menu pixels, DXT1 or DXT5 were found in this installation. BC1/DXT1, BC3/DXT5 and RGBA32 are additionally covered with synthetic tests. Unity pixels are bottom first; output is flipped during streaming box reduction.

E3: all five `Data/<DLC>/AssetBundles/resources_<dlc>` files are UnityFS version 8, flags `0x243`, with LZ4HC block flag 3. Container counts are Royalty 670, Ideology 1,491, Biotech 2,136, Anomaly 2,200 and Odyssey 2,562. None contains a menu background or BGPlanet path/name. They are not needed for this fix. AssetBundle decompression is deliberately outside this implementation; a texture that actually references a bundle falls back to its official preview.

F1 (confidence: high; status: verified; evidence: E1–E3): all six backgrounds can be extracted from the base Unity resources with exact ResourceManager resolution. The DLC directories determine which DLC to offer; images from another environment are never substituted.

F2 (confidence: high; status: verified; evidence: integration and regression tests below): missing/corrupt resources no longer force official entries to plain. Extracted art wins per package; remaining official previews keep the same keys, including the default planet key.

Evidence source hashes: not embedded, because no game data is distributed with this change. The local version, object IDs, offsets and reproduction commands identify the inspected data. UnityPy 1.25.3 was used only as an independent research decoder; the application and committed tests have no Python or external-tool dependency.

## Extraction path and limits

P1 (`path_type=solve`, E1 → E2 → F1): ExpansionDefs → select Assets.zip when present, otherwise SerializedFile → ResourceManager path and external PPtr → Texture2D header → bounded inline/sidecar reader → four-row BC/RGBA decode → flip and box reduction → 1920/480-wide JPEG pair. The 1.7 ZIP route retains full-entry size/CRC validation. Missing ResourceManager data permits a unique case-insensitive basename match; a readable mapping with a missing path does not permit guessing. Duplicate names are rejected.

Only metadata and short Texture2D headers are read eagerly. Pixel reads use `io.SectionReader` over level 0, never the whole sidecar or compressed texture. One decode lane is shared with the ZIP route; scratch/output memory is returned after each pair. Unity cache keys include the game path and size/mtime/existence of resources.assets, resources.assets.resS and globalgamemanagers. A source change during extraction prevents publication. Incomplete pairs are repaired, and language changes reuse image bytes.

SerializedFile metadata framing supports versions 17–22, with or without blob type trees and either metadata endianness. Texture layout handling covers the known Unity 2018–2022 families, including 32/64-bit stream offsets and the 2022 mipmap limit group. Unknown layouts, stripped top mips, non-2D images, unsupported formats, bundle streams and malformed ranges are skipped for preview fallback. Real-install verification here covers only the supplied 1.6 build; older layouts have synthetic coverage.

## Validation and performance

On Windows amd64, Go 1.27.2, CGO_ENABLED=0, Ryzen 9 7950X:

| Measurement | Result |
| --- | --- |
| Six-background first Prepare, integration run | 1.278 s |
| Same cache, integration run | 49.0 ms |
| Go heap peak sampled every 5 ms during first Prepare | 11.75 MiB, baseline 0.71 MiB |
| First Prepare in a fresh benchmark process, one iteration | 1.403 s |
| That process's peak Windows working set | 57.59 MiB |
| Cumulative allocations in the benchmark (not live/peak memory) | 61,735,888 bytes; 67,464 allocations |

“First” means an empty RimArcho image cache; the filesystem cache was not flushed. The process peak includes the test executable and Go runtime and was sampled through Windows PeakWorkingSet64. These are extraction measurements, not a full Wails window's memory. PNG review encoding occurs after the integration measurement and is excluded from the isolated benchmark.

`tmp/menu-backgrounds-16/` contains six PNGs converted from the produced full JPEGs for inspection: Core is 1920 × 1200, each DLC 1920 × 1080. Every full/thumbnail pair was checked for dimensions and non-uniform image data. Independent UnityPy/Pillow decode-and-box-resize comparisons give mean RGB absolute errors of about 2.1–4.0 / 255 after the Go JPEG pipeline; the different box rounding and JPEG loss account for small pixel differences. Royalty was also visually inspected for correct orientation. The review images are gitignored and contain no new committed game data.

Synthetic tests cover SerializedFile header versions, type-tree presence, byte order, MonoBehaviour type framing, scripts and externals, invalid/truncated metadata, exact PPtr lookup, missing/ambiguous names, inline and streamed pixels, 32/64-bit offsets, BC1/BC3/BC7/RGBA32, unused mips, vertical flipping with non-divisible box scaling, bad stream paths/ranges, cache reuse/invalidation and incomplete-pair repair. GUI/app regressions cover official preview discovery, full and partial extraction failure, broken ZIPs and JPEG serving.

The following passed with CGO_ENABLED=0:

```powershell
$env:CGO_ENABLED = '0'
go build ./...
go vet ./...
go test ./...

$env:RIMARCHO_TEST_GAME16 = 'D:\rimworld16'
go test ./internal/menubg -run '^TestGame16$' -v -count=1
go test ./internal/menubg -run '^$' -bench '^BenchmarkGame16Prepare$' -benchtime=1x -benchmem
```

The opt-in test skips when RIMARCHO_TEST_GAME16 is unset. RIMARCHO_TEST_GAME16_OUTPUT optionally changes the PNG review directory. To measure the process rather than the Go build driver, first compile with `go test -c ./internal/menubg -o tmp/menubg.test.exe`, run that binary with the equivalent `-test.run`, `-test.bench`, `-test.benchtime` flags, and read its PeakWorkingSet64 from PowerShell.

## Changed files

- `internal/menubg/unity_file.go`: bounded SerializedFile metadata, object table, external files and ResourceManager.
- `internal/menubg/unity_texture.go`: versioned Texture2D header and inline/sidecar level-0 ranges.
- `internal/menubg/unity_cache.go`: exact resource resolution and Unity extraction/cache preparation.
- `internal/menubg/cache.go`, `catalog.go`, `dds.go`: source selection, common definitions/stamps and streaming RGBA/vertical flip.
- `internal/menubg/unity_test.go`, `unity_integration_test.go`: synthetic fixtures, opt-in real extraction and benchmark.
- `internal/app/backgrounds.go`, `backgrounds_test.go`: official preview discovery and regression.
- `internal/surface/gui/background_worker.go`, `background_worker_test.go`: per-key merge and failure/serving regressions.
- `docs/specs/06-ui/2.11-other-pages.md` §2.14 and `docs/specs/07-technology-and-engineering.md`: Chinese contracts dated 2026-10-24.
- This dated investigation record.

Timeline (2026-10-24): inspected headers and resource mapping; checked all five DLC containers; implemented the Go path and preview fallback; verified real images and memory; completed synthetic regressions and all required repository checks.
