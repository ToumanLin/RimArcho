# Archo's animation CPU on a real GPU

> Status: measured (2026-11-23). No product change: neither cheap lever (smaller frames, 10 fps) brings the animated figure clearly under 2 % of a core above "off", and the one that does (a slower idle) changes how she looks, so the choice is the player's (§5). Baseline recorded in 18 §2.

VM round 2 lane B ([report](2026-11-23_release-1.1-vm-round2-B.md)) measured Archo animated at 4.2–6.7 % of one core on Can it run, still 1.0 %, off 0.8 %, in a VM without a GPU (software rendering, 4 vCPUs). The target is about 2 % per figure (16 §10 "Nothing moves at rest", 16 §7.1 "Archo figure"). This report measures the same page on the host's real GPU and tries the two cheap levers.

## 1. Setup

- **Host**: AMD Ryzen 9 7950X, NVIDIA RTX 4090, 3840×2160 at 59 Hz, Windows display scale 150 %. WebView2 157.0.4341.60.
- **App**: the offline memprobe host (`D:\RimArcho-test\memprobe\harness\memhost-main.go.txt`, built unchanged except its pprof port, from dev/1.1 at 09c92f24) on a copy of the 1,354-Mod synthetic home (`D:\RimArcho-test\archo-cpu\root`), current plan `default` (485 Mods). One window at a time; isolated `APPDATA` / `LOCALAPPDATA`; DevTools on its own port; the process tree killed after each run. The window opens at 1440×880 CSS px.
- **Page**: Can it run, Archo's placement (176 CSS px wide, `ARCHO_PAGE`). At rest she plays an expression three times, then the calm idle (8.4 fps), and every 45 s cycles to the next calm expression (in, three loops, out, bridge: about 10 s at 15 fps). The sandbox's "locate the Archo Monitor folder" dialog was open in every run (static; the same in all modes).
- **Measure**: process-tree CPU time (the Go host and every WebView2 child: browser, GPU, renderer, utility, crashpad), from `Win32_Process` kernel + user time, as % of one core. Start, 10 s, navigate to Can it run, 15 s, then 3 windows of 30 s. A `focus` event before each window keeps the idle from stopping (it stops after a minute in the background); the window stayed visible throughout.
- **Device scale**: 1.5 is the host's own (`ui.zoom` 100). 1.0 comes from WebView2's `--force-device-scale-factor=1`, added by a temporary, uncommitted environment hook in `BrowserArgs` (`WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` has no effect under Wails). `ui.zoom` cannot go below 100.
- **Software**: `ui.gpu off` (no GPU process; the page is drawn in software, like the VM).
- **Packs**: `scripts/archopack/build.py` from crayon-rig's local `clips-notalk`; a fresh build is byte-identical to the committed pack. Variants (`D:\RimArcho-test\archo-cpu\variant.py`, a wrapper that overrides build.py's constants):
  - **a**: every animated clip 210 px wide instead of 300.
  - **b**: two of every three phases at 99 ms (10.1 fps) for loops, in, out and bridge; clip lengths unchanged, so `js/archo.js` splices them as before. The idle is unchanged (already 7.5 fps with 15 fps blinks).
  - **a+b**: both.
  - **c** (extra probe, not a requested lever): the idle every 4th frame instead of every 2nd (5.1 fps with its blinks); everything else as committed.

## 2. Results

% of one core, process tree, mean of three 30 s windows (each window in brackets). "Above off" subtracts the same scale's off mean.

| Scale | Archo | Mean | Windows | Above off | Pack |
| --- | --- | --- | --- | --- | --- |
| 1.5, GPU | off | 1.35 | 0.92 / 1.79 / 1.13; repeat 1.27 / 2.05 / 0.92 | — | — |
| 1.5, GPU | still | 1.30 | 1.12 / 1.64 / 1.13 | −0.05 | — |
| 1.5, GPU | animated (committed) | **3.68** | 3.84 / 4.20 / 3.33; repeat 3.84 / 3.17 / 3.72 | **+2.33** | 14.47 MB |
| 1.5, GPU | a: 210 px | 3.41 | 2.81 / 3.59 / 3.84 | +2.06 | 9.04 MB |
| 1.5, GPU | b: 10 fps | 3.74 | 3.69 / 3.64 / 3.89 | +2.39 | 9.95 MB |
| 1.5, GPU | a+b | 3.16 | 3.13 / 3.33 / 3.02 | +1.81 | 6.27 MB |
| 1.5, GPU | c: idle 5.1 fps | 2.54 | 2.41 / 2.81 / 2.41 | +1.19 | 14.27 MB |
| 1.0, GPU | off | 1.06 | 1.08 / 1.27 / 0.82 | — | — |
| 1.0, GPU | still | 1.50 | 1.74 / 1.69 / 1.07 | +0.44 | — |
| 1.0, GPU | animated (committed) | **3.00** | 2.61 / 2.66 / 3.74 | **+1.94** | 14.47 MB |
| 1.0, GPU | a: 210 px | 3.35 | 3.17 / 3.18 / 3.69 | +2.29 | 9.04 MB |
| 1.0, GPU | b: 10 fps | 2.88 | 2.55 / 2.77 / 3.33 | +1.82 | 9.95 MB |
| 1.0, GPU | a+b | 3.00 | 2.51 / 3.38 / 3.12 | +1.94 | 6.27 MB |
| 1.5, software | off | 1.53 | 1.89 / 1.63 / 1.07 | — | — |
| 1.5, software | animated (committed) | 3.25 | 3.21 / 3.53 / 3.02 | +1.72 | 14.47 MB |
| 1.5, software | a+b | 2.98 | 3.78 / 2.76 / 2.41 | +1.45 | 6.27 MB |

By process (means, 1.5 GPU): off is almost all Go (0.9 %: the host at rest with a 1,354-Mod home) plus the browser process (0.3 %, partly the measuring DevTools calls). Animated adds the GPU process (1.5 %) and the renderer (0.6–1.0 %). Under software rendering the GPU process is gone and the renderer and browser process take its place (1.4 + 1.0 %).

**Noise.** Windows of one run differ by up to 1 point, because a 30 s window holds either one or no 45 s cycle at 15 fps; two runs of the same setup differed by 0.2 (3.79 / 3.58, 1.28 / 1.41). Differences under about 0.5 are not real.

## 3. What the numbers say

- **On a real GPU the animated figure costs about 2 % of a core**: +1.9 % at device scale 1.0, +2.3 % at 1.5. That is the spec's "about 2 % per figure", slightly above it at 150 %. The VM's 4–7 % is the VM: the same software path on this CPU costs +1.7 %, so 4 slow vCPUs, not the pack, explain lane B's number. Above off, the VM measured +3.4 to +5.9; players with a GPU see half of that or less.
- **Neither lever changes it beyond the noise.** 210 px frames: −0.3 at 1.5, +0.35 at 1.0. 10 fps loops: +0.06 at 1.5, −0.12 at 1.0. Together: −0.5 at 1.5, 0.0 at 1.0. The cost is not in decoding or drawing pixels; it is in the compositor waking up for each new frame (the GPU process and the renderer), so it follows the number of frames shown per second, not their size.
- **10 fps loops barely matter because she is mostly in the idle**, which is already 8.4 fps; the 15 fps loops play about 10 s of every 45 s.
- **The idle's frame rate is the lever that works**: at 5.1 fps the figure costs +1.2 %, half of today's, at 1.5 (windows 2.41–2.81 against 3.17–4.20: no overlap). The 2026-11-20 headless measurement (16 §7.1: a 5.5 fps idle 1.8 % against 2.2 %) called this within noise; the native window shows it clearly.

## 4. What each lever looks like

Frame strips in `D:\RimArcho-test\archo-cpu\strips` (not committed):

- `sharpness-smug-frame0.png`: a loop's first frame at 176 CSS px, scale 1.5 (264 device px) and 1.0 (176), light and dark, 300 px against 210 px frames. At 1.0 they look the same; at 1.5 the 210 px frame is upscaled and slightly softer on the crayon outline and the hair strands.
- `motion-laugh-base-vs-b.png`, `motion-shake-base-vs-b.png`: the first 1.2 s of two loops at 66 ms ticks; at 10 fps every third tick repeats the frame before it. The fast hair swing of `shake` shows the steps most.
- Lever c (not stripped): the idle's breath and sway move in 4-frame steps instead of 2; its blinks keep every frame.

Pack size: a 9.04 MB, b 9.95 MB, a+b 6.27 MB, c 14.27 MB, against 14.47 MB committed. Lever a+b would cut the download and the embedded assets by 8.2 MB without a measurable CPU change.

## 5. Recommendation

1. **Keep the frame size and the 15 fps loops.** They are not where the CPU goes. If the pack size matters for the release, a+b saves 8.2 MB with a small visible cost (§4), but that is a size decision, not a CPU one.
2. **If ≤ 2 % at 150 % is a must**, slow the idle (lever c: `IDLE_STEP` 4 in `build.py`, about −1.1 %), and check in the native window that the breath does not look jerky. Alternatively, let the idle stop on its own after a few minutes at rest (she then costs what "still" costs, 0 above off) and restart on the next page change or expression. Either is the player's call; neither is committed.
3. **Correct the spec's wording after the choice**: 16 §7.1 says the native WebView2 figure is a VM check; this host measurement answers it for a real GPU (18 §2). The VM row S6 remains for software rendering.

## 6. Not measured

- A 100 % Windows display scale on the host itself (device scale 1.0 here comes from WebView2's switch; the window's size in device pixels stayed the same, so the page was laid out at 2160×1320 CSS px).
- 144 Hz: the host display runs at 59 Hz. Frame-change cost should not depend on the refresh rate (the figure changes at most 15 times a second), but it is untested.
- Other GPUs (integrated graphics, older drivers), and a real home: the synthetic home and the memprobe host only.
- The other four placements and the start-up screen; only Can it run.
- Several figures at once.
