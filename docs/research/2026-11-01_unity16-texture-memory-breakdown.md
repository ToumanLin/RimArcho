# Unity 1.6 texture memory breakdown (2026-11-01)

Follows [2026-10-31 calibration](2026-10-31_unity16-memory-calibration.md) and [2026-11-01 heavy textures](2026-11-01_unity16-heavy-texture-memory.md). Question: where does the memory behind the 1.6 VM texture coefficient (1.6–1.8× payload, 1.3–1.6× above 3 GiB) go, and does it exist on a real PC?

## Answer

**The hypothesis holds for the VM.** Unity keeps no CPU copy of a loaded texture: after `Apply(…, makeNoLongerReadable: true)` its pixel buffer is released, and the D3D11 texture is created as `USAGE_DEFAULT` with no CPU access. What stays is the driver's copy of every texture. The VMware SVGA 3D adapter has no dedicated video memory (dedicated usage 0), so that copy is committed guest system memory charged to the game process, and appears in its private bytes.

Per byte of texture payload (the model's textures + DDS estimate), the VM's private bytes grew by 1.51 between two 244-Mod lists that differ mainly in textures ([Evidence 2](#evidence-2-vm-measurement-of-the-drivers-share)):

| Part | Per payload byte | Share | Real PC with a discrete GPU |
| --- | ---: | ---: | --- |
| Driver: committed GPU memory (the D3D11 textures) | 1.22 | 81% | In dedicated VRAM while it fits (inferred, not measured) |
| Mono heap, reserved | 0.20 | 13% | Same |
| Unity native allocators, reserved | 0.035 | 2% | Same |
| Other private bytes | 0.055 | 4% | Probably the same |
| **Private bytes** | **1.51** | | |

The driver's share is 1.01 bytes per byte Unity reports as texture memory. It exceeds 1.0 per payload byte because Unity's texture memory is 1.16–1.19× the payload: some textures stay uncompressed (RGBA32 / ARGB32 / RGB24) where the file model counts them compressed or not at all. The earlier calibration runs, which did not record GPU memory, split the same way: of their 1.40 slope, 1.15 is neither Mono nor Unity ([Evidence 1](#evidence-1-splitting-the-existing-calibration-runs)).

On a real PC with a discrete GPU the driver's share should move to VRAM, and RimWorld's RAM need should drop by about 1.2 bytes per payload byte. That part is inferred from how Unity creates the textures, not measured; [what to measure](#what-to-measure-on-a-real-pc) is below. The model now reports that share as VRAM and adds it back to RAM only when the computer has no dedicated video memory or not enough of it ([Model](#model)).

## How one texture is loaded (1.6.4871, Unity 2022.3.35f1, D3D11)

C# from `Assembly-CSharp.dll` 1.6.9676.17735 (ilspycmd); native from `UnityPlayer.dll` 2022.3.35.70150 with Unity's public PDB (`UnityPlayer_Win64_player_mono_x64.pdb`, RSDS `FF67DE78-9DE3-4C50-A915-72EED5F70D3B`, age 1) in Ghidra 11.2.1. Offsets are RVAs from image base `0x180000000`. Only names and offsets are recorded here; no decompiled code is kept in the repository.

**PNG** (`Verse.ModContentLoader<T>.LoadTextureViaImageConversion`), with texture compression on and both sides divisible by 4, on a GPU with compute shaders (the VM qualifies: its census shows GPU-compressed DXT5):

| Step | CPU allocation | Size | Freed |
| --- | --- | --- | --- |
| `VirtualFile.ReadAllBytes` | Managed `byte[]` on the Mono heap | PNG file size | Garbage after the call; the Boehm heap does not shrink, so its reserved size follows the peak of in-flight buffers and fragmentation |
| `new Texture2D(2, 2, RGBA32, mipChain)` + `ImageConversion.LoadImage` (`ImageConversion_CUSTOM_LoadImage` 0x217B80, `LoadImageIntoTextureImpl<PngDecoder>` 0xCCD5E0) | `SharedTextureData` from Unity's `MemoryManager` (`Texture2D::InitTextureInternal` 0x4947D0) | w×h×4 × 4/3 | When the texture becomes non-readable (below). `LoadImage` already uploads once |
| `Apply(true, true)` (`Texture2D::Apply` 0x49AAE0 → `UpdateImageData` 0x497590 → `UploadTexture` 0x495F00) | Threaded device: the pixels are copied into the command ring buffer (`GfxDeviceClient::UploadTexture2D` 0x9E5330, command 0x2764, `WriteBufferData`) | Bounded ring buffer | After the render thread consumes the command |
| D3D11 upload of an existing texture (`TexturesD3D11Base::UploadAll2DData` 0x922480 → `Upload2DData` 0x920790) | One `STAGING` texture per mip (`CreateAndMapStagingTexture` 0x920940: `Usage` 3, `CPU_ACCESS_WRITE`), mapped, filled, `CopySubresourceRegion` into the texture (`UnmapAndCopyToStaging` 0x920E60) | One mip | Released immediately; the context is flushed every 64 MiB of uploads |
| `UploadTexture` after the upload | — | — | `!isReadable` → `SharedObject<SharedTextureData>::Release`, pointer cleared: **the CPU copy is gone** |
| `StaticTextureAtlas.FastCompressDXT(deleteOriginal: true)` | None on the CPU: `Texture2D(RGBA_DXT5_UNorm, DontInitializePixels | DontUploadUponCreate)`, per mip a `RenderTexture` and the `EncodeBC3_AMD` compute kernel, `Graphics.CopyTexture` | — | Render textures and the RGBA32 original are `DestroyImmediate`d; `TexturesD3D11Base::DeleteTexture` 0x925810 releases D3D11 objects immediately, with no deferred queue in Unity |

Without compute shaders RimWorld calls `Texture2D.Compress(true)` (CPU DXT, `Texture2D::Compress` 0x49A2E0) before the same `Apply(true, true)`. Non-block-aligned PNGs skip compression and stay RGBA32.

**New D3D11 textures** (`TexturesD3D11Base::UploadTexture2D` 0x9225B0 → `CreateDefaultTexture` 0x921590): `D3D11_TEXTURE2D_DESC` with `Usage = DEFAULT (0)`, `BindFlags = SHADER_RESOURCE (8)`, `CPUAccessFlags = 0`, and the pixels as creation-time initial data (`PrepareTextureDataForCreateTimeUploadDX11`); the subresource array is freed right after `CreateTexture2D`. Unity holds no shadow copy; where the bytes live is the driver's choice.

**DDS** (`Verse.ModDdsLoader.TryLoadDds`): the file is memory-mapped, copied by `LoadRawTextureData` into `SharedTextureData`, then `Apply(!hasMips, true)` uploads and releases it as above. The mapping is closed when the method returns.

**Static atlases** (`StaticTextureAtlas.Bake`): GPU only. Small textures are drawn into a render texture, copied into the atlas texture, mipmapped by a compute shader and DXT-compressed with `FastCompressDXT`. In 1.6 the originals stay resident, so the atlas is a second GPU copy of small textures (the model's atlas upper bound).

So after loading, a texture costs: its D3D11 allocation (GPU memory: DXT5/DXT1/BC7 1 or 0.5 byte per pixel, RGBA32 4, × 4/3 for mips), a small native `Texture2D` object, and a managed wrapper. Transients during loading: the PNG `byte[]`, the decoded RGBA32 buffer with mips, a staging mip, the RGBA32 GPU original before compression, and per-mip render targets.

## Evidence 1: splitting the existing calibration runs

The calibration driver logged Mono's and Unity's reserved sizes at the main menu in every earlier run ([2026-10-31](2026-10-31_unity16-memory-calibration.md), [2026-11-01](2026-11-01_unity16-heavy-texture-memory.md)). The rest of private bytes is "unattributed": the graphics driver, other DLLs, thread stacks and anything else. GiB, menu resident:

| Run | Payload | Private bytes | Mono reserved | Unity reserved | Unattributed |
| --- | ---: | ---: | ---: | ---: | ---: |
| Vanilla | 0 | 1.443 | 0.228 | 0.331 | 0.883 |
| 244-Mod light | 0.99 | 3.981 | 1.190 | 0.507 | 2.283 |
| 494-Mod large | 2.92 | 8.237 | 1.695 | 0.792 | 5.750 |
| 244-Mod texture (3 runs) | 1.86 | 5.31–5.43 | 1.31–1.39 | 0.52–0.57 | 3.46–3.48 |
| Heavy 4.5 | 4.24 | 9.017 | 1.869 | 0.650 | 6.498 |
| Heavy 6 | 5.31 | 10.146 | 1.871 | 0.669 | 7.606 |
| Heavy 8 (2 runs) | 7.25 | 12.82–13.06 | 2.36–2.52 | 0.85–0.86 | 9.62–9.68 |

A straight-line fit over the seven 244-Mod texture runs (lists that also differ a little in XML and sound) gives, per payload byte: private bytes 1.40, unattributed 1.15 (82%), Mono 0.19 (14%), Unity 0.056 (4%). Largest residuals: 0.27, 0.23, 0.15 and 0.05 GiB. Unity's own allocators therefore do not hold the textures. The large unattributed part was the candidate for the driver, which Evidence 2 measured.

Mono grows with textures because each PNG is read into a managed `byte[]` (`VirtualFile.ReadAllBytes`) and the Boehm heap keeps its peak reservation. That matches the slope, but no allocation trace was taken.

## Evidence 2: VM measurement of the driver's share

Three runs on 2026-10-31 and 11-01 in the same VM and snapshot as the earlier calibration (`RimArcho-Real-Vulkan`, 8 GiB RAM, RimWorld 1.6.4871, texture compression on). A small Harmony driver Mod logged Unity's counters (`Profiler.GetMonoHeapSizeLong`, `GetTotalReservedMemoryLong`, `Texture.currentTextureMemory`) and a census of every loaded texture (format, mips, readability; bytes from `GraphicsFormatUtility.ComputeMipmapSize`). A guest probe read the process's private bytes, working set, a `VirtualQueryEx` walk of its address space, and the Windows "GPU Process Memory" counters for the game: total committed, shared usage (resident), dedicated usage. Adapter: VMware SVGA 3D, user-mode driver `vm3dum64_10.dll` 9.17.11.4, `d3d11.dll` 10.0.26100.9711; Unity reports 128 MB of video memory.

GiB at the main menu, 90 seconds after loading finished. "Other" is private bytes minus Mono, Unity and committed GPU memory:

| Run | Payload | Private bytes | Mono reserved | Unity reserved | GPU committed | Other | Unity texture memory | GPU resident | Working set |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Vanilla | 0 | 1.553 | 0.323 | 0.331 | 0.662 | 0.236 | 0.574 | 0.640 | 1.493 |
| 244-Mod texture (`h-baseline`) | 1.86 | 5.476 | 1.417 | 0.581 | 2.863 | 0.616 | 2.733 | 0.645 | 2.607 |
| Heavy 4.5 (`heavy-4p5`) | 4.24 | 9.064 | 1.881 | 0.665 | 5.770 | 0.747 | 5.608 | 0.542 | 3.043 |

- The runs reproduce the earlier private bytes (9.064 vs 9.017 GiB for heavy 4.5; 5.476 vs 5.312 for the texture list).
- Dedicated usage was 0 throughout. Committed GPU memory follows Unity's texture memory: 1.03–1.05× in the Mod lists, 1.15× in vanilla, where render targets, meshes and buffers weigh more.
- Committed GPU memory counts in private bytes whether or not it is resident: GPU resident memory stayed at 0.54–0.65 GiB and the working set at 3.0 GiB while private bytes reached 9.1 GiB. The `VirtualQueryEx` walk found 9.04 GiB of private commit, 5.9 GiB of it in 247 read-write regions of 16–64 MiB (driver heaps). In the VM the texture copies are committed and mostly paged out; the calibration measured the commit, not the working set.
- CPU copies: 184 textures (5.5 MB) were still readable in heavy 4.5. Every other texture had released its pixels, as the disassembly says.
- Contrast heavy 4.5 − texture list (same 244 Mods; +2.38 GiB payload, also +10 MB XML and +0.04 GiB sound): private bytes +1.51 per payload byte; committed GPU memory +1.22, Mono +0.20, Unity +0.035, other +0.055. Unity texture memory +1.21 per payload byte, so committed GPU memory per byte of Unity texture memory is 1.01.
- Unity texture memory per payload byte, vanilla's textures removed: 1.16 (texture list), 1.19 (heavy 4.5). In heavy 4.5, 1,129 uncompressed textures (RGBA32 / ARGB32 / RGB24) hold 0.86 GiB of the 5.61.

The vanilla run went through a full play flow: new colony, about 7,100 ticks, save `RimArcho-texmem-vanilla-1031-232744`, load, about 3,600 ticks, quit. Autosave was set to every 14 days for the run and restored to 1 day / 5 saves before quitting. In the loaded game, private bytes were 1.822 GiB and committed GPU memory 0.826 GiB.

## VM versus a real PC

| | VM (VMware SVGA 3D) | Discrete GPU (inferred) | Integrated GPU (inferred) |
| --- | --- | --- | --- |
| Where the driver puts a texture | Committed system memory; no dedicated memory | Dedicated VRAM while the budget allows | System memory shared with the GPU |
| In process commit (Task Manager "Commit size") | Yes, all of it | No, while it fits | Yes |
| Beyond the VRAM budget | — | Windows demotes allocations to system memory ("Shared GPU memory") | — |

Unity does not decide this: it hands the driver `USAGE_DEFAULT` textures with initial data and keeps nothing. On a discrete GPU the textures should live in VRAM, and the process should commit about 1.2 bytes less per payload byte than in the VM.

### What to measure on a real PC

On a Windows PC with a discrete GPU: RimWorld 1.6, texture compression on, main menu, a minute after loading finishes. In Task Manager → Details, add the columns "Commit size", "Dedicated GPU memory" and "Shared GPU memory", and read them for `RimWorldWin64.exe`:

| List | Expected Commit size | Expected Dedicated GPU memory |
| --- | ---: | ---: |
| Vanilla (Core + DLC) | ≈ 0.9 GiB (VM: 1.55) | ≈ 0.66 GiB |
| A list with payload P (`archo plan cost`) | VM RAM estimate − 1.2 × P | ≈ 0.66 GiB + 1.2 × P |

If Commit size stays near the VM figure while dedicated GPU memory also holds the textures, the driver keeps a system-memory copy, and `TextureGPU16` must go back to 0 for real cards. Shared GPU memory above a few hundred MB means VRAM ran out and textures spilled to RAM. Performance Monitor's "GPU Process Memory" counters (Total Committed, Dedicated Usage, Shared Usage) give the same numbers per process. Note the resolution: render targets grow with it.

## Model

`internal/loadcost` keeps the VM calibration and splits it:

- `TextureGPU16 = 1.2`: per payload byte, 1.2 of the calibrated 1.6 texture RAM rate is the driver's copy and is reported as VRAM. RAM keeps the rest (Mono, Unity, other). Atlas, sound, XML and DLL rates are unchanged. The 1.2 is the measured 1.22, which also covers the uncompressed textures the payload misses.
- Vanilla baseline: the VM's 1.43 GiB becomes 0.77 GiB RAM + 0.66 GiB VRAM (committed GPU memory at the vanilla menu).
- `Spill`: VRAM that does not fit in 90% of the dedicated memory (`UsableVRAM`, an assumption) is added back to RAM. With no dedicated memory reported (VM, integrated GPU, unknown adapter), all of it is. The loading reserve is still computed from the payload, so it is unchanged.
- In the VM (no dedicated memory) every estimate equals the old one: the calibration tests pass with the same ranges. 1.7 is unchanged.

Examples (GiB; RAM includes the VRAM held in system memory; dev/1.0 gave exactly the "no dedicated VRAM" RAM figures):

| List | VRAM | No dedicated VRAM (VM): resident / loading | 8 GiB card: resident / loading | 24 GiB card: resident / loading |
| --- | ---: | ---: | ---: | ---: |
| Heavy 4.5 (244 Mods, payload 4.2) | 5.5–6.0 | 8.4–10.4 / 10.1–14.0 | 2.8–4.5 / 4.6–8.1 | 2.8–4.5 / 4.6–8.1 |
| 714-Mod list from the 720 run (payload 24–27) | 29.2–33.5 | 36.4–51.8 / 41.1–66.2 | 29.2–44.6 / 33.9–59.0 | 14.8–30.2 / 19.5–44.6 |

The 714-Mod list needs about 30 GiB of VRAM, so even a 24 GiB card leaves 7.6–11.9 GiB of it in RAM.

## Uncertain

- **Real discrete GPU: not measured.** The split rests on the D3D11 descriptor and on WDDM placing default textures in VRAM. Whether a vendor driver also commits a system-memory backing for them is open; the [real-PC measurement](#what-to-measure-on-a-real-pc) decides it.
- `UsableVRAM = 0.9` is an assumption, as is treating every computer without a reported dedicated adapter as having no VRAM.
- The 0.66 GiB VRAM baseline was measured at the VM's resolution; render targets scale with resolution and the game's settings.
- Mono's 0.20 per payload byte is attributed to PNG read buffers by reading the code, not by an allocation trace. "Other" (0.055) is unexplained.
- Startup peaks were not split; the loading reserve keeps the earlier VM fit.
- 1.7 (a different Unity version) was not examined.
