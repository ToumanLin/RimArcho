# Texture optimization: PNG to DDS

Research on 2026-10-22. Basis: decompiled `Assembly-CSharp.dll` / `Waylith.dll` / `RimWorld.dll` from local 1.7.4985 (Waylith engine) and 1.6.4871; RimCrow (`../RimModManager`) `backend/managers/mgr_texture_opt.py` and `doc/07-%E8%B4%B4%E5%9B%BE%E4%BC%98%E5%8C%96%E8%AF%B4%E6%98%8E.md`; todds 0.4.1 bundled with RimSort; local Workshop directory statistics and one actual conversion.

## 1. Conclusions

1. **The game supports it natively; no prerequisite Mod is needed.** Both 1.6 and 1.7 read `.dds` files in Mods and skip `.png` when a `.dds` of the same name exists. DDS also loads without a PNG. Thus both "normal optimization (keep PNG)" and "pack slimming (delete PNG)" work.
2. **Use todds 0.4.1.** RimSort and RimCrow share this MPL-2.0 encoder, but its repository was archived on 2026-03-31; 0.4.1 is the final release. A fixed version + SHA-256 should be manageable.
3. **DDS is much larger than PNG.** Measured at 9.5 times the size at full dimensions, and still 2.5 times at 50% (§4). After deleting PNGs during pack slimming, **a pack grows rather than shrinks without scaling**. Only scaling saves disk space. Video memory differs: opaque textures convert to BC1 (4 bits per pixel), whereas 1.7 encodes PNG as BC7 and 1.6 as DXT5 (both 8 bits), so full-size conversion also halves video memory for opaque textures (corrected 2026-10-22; previously said "only scaling saves video memory").
4. **1.7 gains less than 1.6.** 1.7 encodes PNG as BC1/BC7 itself and writes a persistent cache, avoiding decoding from the second launch. On 1.7, DDS mainly saves that encoding on the first launch, after Mod updates, and **the first launch of each pack** (packs use `-savedatafolder`, so the cache goes into the pack's own empty `userdata/`).
5. **Generating DDS inside the original Mod has two risks to handle**: old DDS shadows new PNG after an update (players see old textures), and DDS in lower-priority folders shadows PNG in higher-priority folders (§2.3). RimArcho must track generated files itself rather than rely on todds mtime comparisons.

## 2. How the game reads textures

### 2.1 Extensions and shadowing (same in 1.6 and 1.7)

`Verse.ModContentLoader<Texture2D>`:

- Accepted extensions: `.png .jpg .jpeg .psd .dds`.
- `ShadowedByDds`: within one Mod, if `x.dds` exists at the same relative path, `x.png` / `x.jpg` / `x.psd` do not load. Comparison is case-insensitive.
  - Minor defect: the code removes the last 4 characters and appends `.dds`, so `.jpeg` is never shadowed (`x.jpeg` → `x..dds`).
- `.dds` is a first-class format and loads normally on its own.

### 2.2 DDS decoding (`Verse.ModDdsLoader` / `DdsPixelFormat`)

- Header validation: magic `DDS `, header size 124, pixel format size 32. Invalid values throw an exception and produce `BadTex` (a magenta placeholder in 1.7).
- Supported formats: DXT1 (BC1), DXT5 (BC3), BC7 (DX10 extension header, data starts at byte 148), uncompressed RGB24 / RGBA32 / BGR888 / RGB565 / ARGB4444 / Alpha8. Other compressed formats (BC4, BC5, BC6H, etc.) throw `NotSupportedException`.
- Uses the file's mipmaps if present; otherwise calls `Apply(updateMipmaps: true)`. Therefore **generate mipmaps** (todds does so by default).
- Uploads directly with `LoadRawTextureData`, with the origin at bottom left, so **flip vertically** (todds `-vf`, also used by RimCrow).
- DDS has `anisoLevel` 0, PNG 2. DDS is slightly blurrier at a distance; negligible.

### 2.3 Shadowing across load folders (a common pitfall)

`ModContentPack.GetAllFilesForMod` collects files by load-folder priority (high to low), keyed by "path relative to the load folder", first wins. Shadowing is checked against this merged dictionary, so:

- If `1.6/Textures/a.png` (high priority) and `Textures/a.dds` (common folder, low priority) both exist, `a.dds` shadows `a.png`; the game shows the old image from the common folder.
- This works correctly if every PNG in every load folder has its own DDS (the high-priority `a.dds` beats the low-priority `a.dds`).
- Rule: **if any PNG at a given relative path fails conversion, none of our generated DDS files at that path may remain in lower-priority folders**. On failure, roll back the whole group rather than skip just that image.

### 2.4 Texture cache in 1.7 (Waylith)

- `Waylith.Rendering.GpuTextureService`: after PNG decoding, the engine encodes BC1/BC7 itself (CPU or GPU) and writes to `<data-directory>/TextureCache/v23/<settings-tag>/`, LZ4-compressed. The key is full path + file size + modification time.
- Local cache: 209 MB.
- `RimWorldWaylith.Bootstrap.ResolveCacheRootOverride`: when launched with `-savedatafolder`, that folder is the cache root. **Each pack therefore has its own empty cache** and must re-encode all PNGs on its first launch.
- Loaded DDS files are also written to the same cache (`PersistToTextureCache`), taking another copy's worth of disk space.

### 2.5 Local state

| Item | Count |
| --- | --- |
| Workshop Mods with PNG textures | 827 |
| PNG textures | 132,847 images, 4.24 GiB |
| Estimated full conversion to DDS (BC1/BC7 + mipmaps, no scaling) | About 26.7 GiB (6.3 times) |
| Mods already containing `.dds` | 292 Mods, 32,448 files, 5.68 GiB |

Some existing DDS files were published by authors (e.g. `[NL] Facial Animation`: PNG 1.2 MB, DDS 51.7 MB); others were generated earlier by RimSort / RimCrow / RimPy. RimArcho cannot tell them apart, so **leave others' DDS untouched by default**.

## 3. Tool: todds

| Item | Details |
| --- | --- |
| Version | 0.4.1 (final release), repository `todds-encoder/todds`, archived 2026-03-31 |
| License | MPL-2.0; redistribution allowed with the license and source URL |
| Acquisition | RimCrow: `todds_Windows_0.4.1.zip` from GitHub releases; RimSort: bundled `todds/todds.exe` |
| Local copy | `D:\SteamLibrary\steamapps\common\RimWorld\RimSort\todds\todds.exe`, `--help` reports 0.4.1 |

**RimCrow arguments** (reuse in RimArcho):

```
todds -f BC1 -af BC7 -vf -fs -o|-on -t -p [-nm] [-sc <percentage> | -ms <max-side>] <file-list.txt>
```

- `-f BC1 -af BC7`: BC1 for opaque images (0.5 bytes per pixel), BC7 for images with an alpha channel (1 byte).
- `-fs`: pad width and height to multiples of 4 (BC formats require 4×4 blocks).
- Use a txt file list as input to convert only selected files. With txt input, todds does not support a separate output directory; DDS files always go beside PNGs.
- `-on` uses mtime to "reconvert only when DDS is older than PNG". Workshop updates may preserve old mtime, so this is unreliable; RimArcho uses its own manifest (§5.1).

**Other RimCrow rules** (worth reusing):

- Skip any directory named `Source` (the author's source assets).
- Skip files with a `.png` extension whose contents are not PNG.
- Convert `_m.png` (masks) without scaling.
- Protect the minimum side when scaling (default 128): if the result would fall below it, use a larger scale or retain full size.
- Invoke in batches of at most 10,000 files, with a 600-second timeout per batch.

**Tool source (undecided):**

- A. On first use, with player consent, download 0.4.1 from GitHub releases, verify a hardcoded SHA-256, and store in `%APPDATA%\RimArcho\tools\todds\0.4.1\`. The repository is archived, so the URL will not change.
- B. Include it in RimArcho's release zip (about 10 MB extra, with MPL license). Avoids the download step.
- Both allow players to specify an existing `todds.exe` in settings (e.g. RimSort's copy), checking the version from the first line of `--help`.

Recommend A; the spec already requires "download with player consent".

## 4. Measurements

Converted `Common/Textures` from `EGI Holograms and Projectors` (Workshop 2979598490) after copying it to a temporary directory, with 32 threads:

| Method | Size | Relative to PNG | Time |
| --- | --- | --- | --- |
| Original PNG (2,521 images) | 41 MB | 1× | — |
| Full-size DDS | 389 MB | 9.5× | 10.6 seconds |
| DDS scaled to 50% | 102 MB | 2.5× | 4.5 seconds |

At this speed, converting all 130,000 local PNGs at full size takes about 10 minutes and over 20 GiB of disk.

Video memory estimate (RimCrow's formula): BC1 = width×height×0.5, BC7 = width×height×1; multiply by 1.333 with mipmaps. Scaling to 50% reduces video memory to about 1/4.

## 5. Design

### 5.1 Normal optimization (generate in the original Mod, keep PNG)

**Commands:**

- `archo textures status [<Mod>…]`: PNG count and size, generated DDS, stale DDS, others' DDS, estimated size per Mod.
- `archo textures optimize <Mod…> | --plan | --all [--scale <percentage>] [--max-size <pixels>] [--no-mipmaps] [--dry-run]`
- `archo textures clean <Mod…> | --all [--dry-run]`: delete only files in the manifest.

**Write scope.** This is the third exception to the README's "never writes to Mod directories" (the first two are `save` and `copy-local`), and happens only on an explicit player action:

- Create `.dds` only beside PNGs; do not modify or delete PNGs.
- Never overwrite existing `.dds` files absent from the manifest; list them as "others' DDS".
- Refuse while the game is running (files may be in use, as with `save`).

**Manifest.** A new database table, one row per generated file: Instance, relative path, source PNG size + mtime + xxh3, DDS size, parameters (format, scale, mipmaps), todds version, generation time. Both `clean` and stale checks rely on it.

**Stale checks.** Compare with the manifest after every scan:

| Condition | Finding | Severity | Action |
| --- | --- | --- | --- |
| Source PNG changed (different size or hash) | `texture.stale` | warning (game shows old texture) | Optimize again / clean |
| Source PNG gone | `texture.orphan` | info (DDS still loads) | Clean |
| DDS deleted or changed | Remove from manifest | — | — |

**Shadowing rule (§2.3).** Collect PNGs across all effective load folders of the Mod, not just the `Textures/` root. If any PNG at a relative path is skipped or fails conversion, delete all lower-priority DDS files we generated at that path.

**Scan fingerprint.** The structural manifest now includes all files (02 §4.4); new DDS changes the snapshot hash and makes the Mod look "updated". Exclude manifest-tracked DDS from the structural layer. Authors' own DDS still counts.

**Will Workshop updates delete our DDS?** Unknown; test it (§6). Stale checks cover either outcome.

### 5.2 Pack slimming (copies in the pack, delete PNG)

Add a step between "clean" and "measure" in 08 §5.2, operating on copies in `.archo-tmp`:

1. Run todds on PNGs under effective load folders in the copy (same arguments, optional scaling).
2. Delete successfully converted PNGs; keep failed PNGs and roll back lower-priority DDS at the same path per §2.3.
3. Record `textures` in `pack.json`: tool and version, parameters, converted count, failed count, PNG bytes, DDS bytes.

Notes:

- **Measure the converted size**. Without scaling, the pack grows several times larger; free-space checks must use estimated DDS size. Before building, the UI shows "PNG x MB → DDS about y MB".
- **No scaling by default** (decided 2026-10-22). Full-size conversion saves loading time (the first-launch encoding time of each pack on 1.7) and half the video memory for opaque textures (§1 item 3), rather than disk space. The UI therefore calls it **"pre-convert (full size)"** without scaling, and "texture slimming" only when a scale is selected. Players must choose scaling to save disk space.
- Process only `Textures/`. Leave `About/Preview.png` and other files not read through the texture loader untouched.
- Semantic hashes exclude textures, so pack entry `hash` and RimArcho list matching are unaffected.
- Add a marker to the copy's `origin.json` indicating texture slimming, so "see the changes" does not treat it as a modified Mod.

### 5.3 Interface

- Mod inventory page: bulk "convert textures to DDS / clean" actions; texture statistics and stale notices in details.
- "create a pack" subpage on the plan page: a texture toggle, scale (default "full size"), and size estimate before building. The toggle is "pre-convert textures" without scaling, "slim textures" with a scale selected.
- todds download confirmation on first use (file name, source, size).

## 6. Tests needed before implementation

| Item | Method |
| --- | --- |
| Both 1.6 and 1.7 display todds DDS correctly (orientation, transparency, BC1/BC7, dimensions not divisible by 4) | Convert a small Mod, inspect in game, keep screenshots |
| DDS loads without PNG (required for slimming) | As above, delete PNG |
| Whether Workshop updates delete extra files in the Mod directory | Add a marker file to a Mod that will update, wait for the update |
| Load times: normal 1.6 launch, 1.7 cold / warm cache, first pack launch | Test the same list 3 times each; inspect Player.log timestamps |
| Among the 292 local Mods with DDS, how many DDS files are older than PNGs (stale files from earlier tools) | Read-only statistics |

## 7. Open decisions

1. todds source: download with consent (recommended) or bundle in the release zip.
2. ~~Default scale for pack slimming~~ Decided (2026-10-22): no scaling by default; the UI calls it "pre-convert (full size)" without scaling, "texture slimming" only when a scale is selected (§5.2).
3. Whether normal optimization should take over DDS left by earlier tools (e.g. suggest cleaning DDS older than PNG). Read-only notices by default.
4. Whether to support RimCrow's `.dds.zstd` (requires the Image Opt Mod). Suggest no: the game cannot read it natively.

## 8. Implementation record (2026-10-22, branch `claude/texture-opt-3d7a51`)

- Item 1 decided: download with consent (`archo textures tool --install`, library › texture optimization), or point `textures.todds` at an existing copy. GitHub provides no digest for this zip, so check its release-page size (3,846,213 bytes) and the version reported by `--help`.
- todds pads source images to multiples of 4 before scaling, with no padding afterward. Offer only scales that keep width and height divisible by 4 and the shorter side at least 128 pixels (`ScaleSteps`); do not scale masks. DDS size is computed from the actual output format, matching todds output byte for byte.
- todds writes fully opaque RGBA PNGs (e.g. many `_m` masks) as BC1. Validate using the header format rather than guessing from PNG color type.
- RimArcho-generated DDS in original Mods is excluded from scan fingerprints and from copies to other folders or packs.
- Sandbox measurements (EGI Holograms and Projectors + Harmony, RimSort's todds): normal optimization 2521 files, 384.4 MB (estimated 385.1 MB), no changes on rescan; 50% slimmed pack 143.9 MB (estimated 144.4 MB), no PNG left in pack; stopping and cleaning both work. Not yet inspected in game.
