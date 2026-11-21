# Faster Vulkan for RimWorld 1.7 in the test VM

> Status: measured on 2026-11-21 in Player-Test (snapshot `RimArcho-Real-Vulkan-17`, UID 14). No product change. Result: Dozen (Vulkan on D3D12) is ruled out because the VMware SVGA driver has no D3D12. No other option keeps the VM's snapshot-and-revert workflow *and* gives a hardware Vulkan. Recommendation in §4.

## 1. Question

RimWorld 1.7 (Waylith) renders only with Vulkan. VMware's virtual GPU has no Vulkan, so the VM uses Mesa lavapipe (`C:\Mesa\x64`, a CPU renderer). Frame rate is so low that a click must be held about 700 ms, and no performance figure from the VM means anything. Is there a faster Vulkan for a test VM, short of running the game on the host?

## 2. Dozen (Mesa Vulkan on D3D12): ruled out

Dozen (`vulkan_dzn.dll`, in the same pal1000 mesa-dist-win 26.2.4 package as lavapipe, host copy `D:\RimArcho-test\mesa\x\x64`) translates Vulkan to D3D12. It needs a D3D12 device on a real adapter.

What the guest has (`dxdiag /t`, VMware Workstation 26.0.1 build 25688693, `virtualHW.version = 22`, `mks.enable3d = TRUE`, 8 GB graphics memory):

| | Value |
| --- | --- |
| Adapter | VMware SVGA 3D (`PCI\VEN_15AD&DEV_0405`), driver 9.17.11.4 |
| Driver model | WDDM 2.0 |
| DDI version | **11.3** |
| Feature levels | 11_0, 10_1, 10_0, 9_3, 9_2, 9_1 |
| DirectX runtime | DirectX 12 (the OS runtime, not the driver) |

A probe calling `D3D12CreateDevice(default adapter, …)` in the guest:

| Minimum feature level | HRESULT |
| --- | --- |
| 11_0 (`0xb000`) | `0x887A0004` DXGI_ERROR_UNSUPPORTED |
| 12_0 (`0xc000`) | `0x887A0004` DXGI_ERROR_UNSUPPORTED |

The driver's user-mode part implements D3D11 only (DDI 11.3), so there is no D3D12 device for Dozen to sit on. With no adapter, Dozen reports no physical device and the Vulkan loader falls back to lavapipe. The ICD was therefore not registered and no frame rate was measured; the registry is unchanged (`HKLM\SOFTWARE\Khronos\Vulkan\Drivers` lists only `C:\Mesa\x64\lvp_icd.x86_64.json`).

Dozen can also run on WARP (Microsoft's D3D12 CPU rasterizer, `DZN_DEBUG=warp`). That is still CPU rendering, through two translation layers instead of one; it was not tried and is not expected to beat lavapipe.

The VMware SVGA device's configuration shows the ceiling: `svga.sm5 = 1`, `svga.gl43 = 1`, no D3D12 or Vulkan capability. A hardware Vulkan in a VMware Workstation Windows guest would need VMware to ship a D3D12 or Vulkan guest driver; it does not today.

## 3. Other options, on paper

Host, read-only: NVIDIA GeForce RTX 4090 (driver 32.0.16.1088, i.e. 610.88, 2026-07-21), AMD Radeon integrated graphics, Ryzen 9 7950X (16 cores / 32 threads), 63 GB RAM, Windows 11 Pro 26300. The hypervisor is already running (`HypervisorPresent = True`), so VMware already runs on the Windows Hypervisor Platform and Hyper-V can coexist with it. Whether the Hyper-V role itself is installed was not read (it needs elevation).

| Option | Vulkan in the guest | Effort | Disk | Snapshot and revert |
| --- | --- | --- | --- | --- |
| **A. Keep lavapipe, more vCPUs** (4 → 12–16) | CPU, as now | Small: power off, change `numvcpus`, cold boot, new snapshot | None | Kept, but needs a new live snapshot (the user's call), since a live snapshot fixes the vCPU count |
| **B. Hyper-V GPU partitioning (GPU-P / GPU-PV)** with the RTX 4090 | Not supported: GPU-PV exposes D3D11 and OpenGL; Vulkan and D3D12 are reported unavailable ([Microsoft Q&A](https://learn.microsoft.com/en-us/answers/questions/5605686/is-it-possible-to-add-support-for-directx-12-in-hy), [Easy-GPU-PV](https://github.com/jamesstringerparsec/Easy-GPU-PV)). Not verified on this host | Large: enable Hyper-V (admin, host reboot), new Windows VM, copy host NVIDIA driver files into it and redo after every host driver update, then Steam (the user's sign-in), RimWorld 1.6 + 1.7, Mods, RimArcho tooling (`vm.ps1` and `input.mjs` are VMware-only) | 60–130 GB for a new VM (the current chain is 125 GB) | Weakened: saved-state checkpoints are reported unsupported while a GPU partition is attached ([NAKIVO](https://nakivo.com/blog/hyper-v-gpu-passthrough)); the adapter must be removed and the VM off for each checkpoint, so no "revert to a signed-in Steam" |
| **B′. Hyper-V DDA** (whole GPU to the guest, real driver, real Vulkan) | Yes | Not available: DDA is a Windows Server feature, and it would take the host's only discrete GPU away from the host | — | — |
| **C. Windows Sandbox vGPU** | Same GPU-PV as B, so no Vulkan | Small to enable (admin) | Small | No: the sandbox is discarded on close, no snapshots, Steam sign-in and the whole game and Mod download every run |
| **D. A second physical machine** (any Vulkan GPU, e.g. a used GTX 1060-class or better) | Yes, native | Medium: hardware, Windows, Steam (the user's sign-in), games and Mods once; RimArcho's VM scripts would need a non-VMware transport (they use VMware Tools for files and guest programs) | Its own disk; nothing on the host | No VM snapshots; a disk image (e.g. a Windows Backup or Macrium image) restored between rounds is the nearest, at minutes per restore instead of 1–2 minutes |
| **E. Host** (rejected by the user's rule) | Yes | — | — | — |

Notes:

- **A** is the only option inside the current workflow. lavapipe draws with one thread per CPU (llvmpipe's `LP_NUM_THREADS` defaults to the CPU count), and the host has 32 threads with 4 given to the VM. More vCPUs should raise the frame rate, but by how much is **not measured**: Unity's own main thread and the game's simulation also compete for those CPUs, and the gain flattens once rasterization stops being the bottleneck. It will never give performance figures that apply to players.
- **B** fails at the first requirement (Vulkan). Some community reports say NVIDIA's Vulkan ICD can be made to load in GPU-PV guests by copying more host driver files; that is undocumented, breaks with driver updates, and still leaves the checkpoint limitation.
- Any new VM, a Hyper-V install or hardware purchase needs the user's OK; none was started.

## 4. Recommendation

Keep lavapipe in the VMware VM for **functional** 1.7 checks (logs, Monitor, RimArcho behaviour, saves): it is slow but it works with the snapshot-and-revert workflow, and nothing else in a VM gives a hardware Vulkan. As a cheap improvement, try option A once: with the user's OK, power off, set 12 vCPUs, cold boot, time menu-to-map and the click-hold needed, and take a new live snapshot if it helps.

For anything about **performance** (frame rate, load times, GPU memory), use real hardware: a second physical machine (option D) if 1.7 performance checks become regular, otherwise a one-off run on a player's or the user's machine. Do not build a Hyper-V GPU-P VM: it gives no Vulkan and loses saved-state snapshots.

## 5. Open

- The vCPU experiment in §4 (needs the user's OK for the cold boot and the new snapshot).
- A baseline lavapipe frame rate and menu-to-map time on 4 vCPUs was not recorded in this round (the task stopped once Dozen was ruled out); measure it together with the vCPU experiment.
