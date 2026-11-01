# Unity 1.6 heavy-texture memory calibration

Broken down by [2026-11-01 Unity 1.6 texture memory breakdown](2026-11-01_unity16-texture-memory-breakdown.md): most of the texture coefficient is the VM graphics driver's copy of each texture.

Measured on 2026-11-01 UTC in the Player-Test VM (8 GiB RAM, VMware virtual GPU, RimWorld 1.6.4871 rev591). This extends the [previous private-byte calibration](2026-10-31_unity16-memory-calibration.md), rather than calibrating physical RAM residency or VRAM.

## Method and coverage

The VM was restored from `RimArcho-Real-Vulkan` before and after testing. The guest was not rebooted. The existing pagefile allocation was 1920 MiB. Setting a custom maximum of 48907 MiB, while preserving the initial size and at least 15 GiB free if fully allocated, did not immediately change the commit limit: `GlobalMemoryStatusEx` returned 10,601,910,272 bytes both before and after. Therefore this run used BRIEF plan B. Windows later expanded its current allocation during loading; the largest observed commit limit was 23,058,296,832 bytes. This later expansion does not establish that the initial setting took immediate effect.

Each new list contained 250 entries, including 244 nonofficial Mods. Lists were dependency-completed and sorted, without changing `plan:default`. The names 4.5/6/8 denote selection targets; the actual payload below is the combined textures and DDS estimate, including its uncertainty. The previous feasible-250 baseline described as approximately 3 GiB in the brief actually contains 1.806–1.915 GiB by this metric.

Sampling followed METHOD-3: one-second process private bytes, working set, peak working set, system commit and commit limit. Menu resident is median private bytes 55–85 seconds after the first successful menu event. Startup peak is the largest sampled private bytes before that event. Post-load resident is median private bytes 30–55 seconds after successful loading. These are sampled peaks, not guaranteed instantaneous maxima. No run is described as a strict cold-cache measurement. Texture compression was enabled when checked through the game's Prefs API.

Task-local evidence and reproducible selectors, sampler, analyzer and fit scripts are in `D:\RimArcho-test\heavy-calib`. Only numerical aggregates, CSV and bounded text logs were exported; no Mod files or saves were exported. The measurement driver ran inside the already installed Archo Monitor Mod. Its transient changes and all guest test data were discarded by the final snapshot restore.

## Measurements

Memory values are GiB; time is seconds from launch to menu. Peak WS is the largest sampled process peak working set through menu arrival.

| Run | Texture + DDS payload | Menu private | Startup private peak | Peak WS | Menu time | Max system commit |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Baseline repeat | 1.806–1.915 | 5.312 | 6.958 | 4.155 | 195.656 | 12.018 |
| 4.5 target | 4.072–4.413 | 9.017 | 11.957 | 4.539 | 311.837 | 16.800 |
| 6 target | 5.075–5.537 | 10.146 | 13.410 | 4.748 | 347.686 | 17.982 |
| 8 target, first attempt | 6.975–7.521 | 12.819 | 15.898 | 4.747 | 426.849 | 20.577 |
| 8 target, full retry | 6.975–7.521 | 13.055 | 15.926 | 4.437 | 530.250 | 20.659 |

The baseline repeat is 1.3–2.1% below the previous texture group's 5.384–5.428 GiB resident values; startup is 1.8–5.2% below its 7.085–7.345 GiB peaks. The repeat remains inside the previous model's intervals.

The first 8-target attempt has valid menu measurements, but the test driver could not advance Worldbuilder's custom world-selection page. Only the blocking task-local driver was fixed: call the same `DoConfigurePlanet` method used by that page's Next button. The retry completed new colony, 120 seconds at speed 1, unique save, reload, 60 seconds at speed 1, return to menu and quit. It reached about 10,636 game ticks, below one day. Autosave preferences were changed through the in-game Prefs API from 1 to 14 days and restored to 1 day / 5 saves; the saved settings were checked read-only. Post-load median private bytes were 13.463 GiB. The created save was `RimArcho-calib-heavy-8-1031-203620`; the first attempt created no save.

The actual application log page and Can it run's last-run errors both displayed this run. The selected list produced Mod content/cross-reference/runtime errors; completing this lifecycle does not certify the list's compatibility. No `SQLITE_BUSY store:index generation` was reproduced. The existing older sibling GUI executable did not remain running when invoked by `archo gui`; the page check used the current CLI's embedded GUI from a separate guest directory. This packaging observation was not reproduced against a fresh current GUI build.

## Fit and interpretation

For each group, subtract the 1.43 GiB vanilla baseline, 2.15 MiB per nonofficial Mod, and the midpoint of the other resource categories, then divide by the midpoint of texture + DDS payload. The resulting effective coefficients are 1.586, 1.543, 1.446, 1.403 and 1.436. Propagating the modeled category and payload bounds gives approximately 1.414–1.768, 1.417–1.678, 1.333–1.569 and 1.306–1.508 for the first four rows.

The original 1.6–1.8 linear texture coefficient puts the 6-target resident lower bound at 10.306 GiB and the 8-target at 13.458 GiB, above both observed residents. A lower effective coefficient is warranted. This is an empirical bend across different list compositions, not an isolated experiment proving a Unity, Mono, GPU-driver or memory-pressure mechanism. A linear 1.5–1.7 interval also covers these observations. The data cannot uniquely identify a knee or rule out that simpler fit; the chosen piecewise curve preserves the earlier low-payload calibration while lowering the high-payload marginal estimate. It does not introduce a saturation cap.

`loadcost.Coef` and `textureRAM16Rate` are the coefficient source. For combined payload P, the resident texture contribution is:

- First 3 GiB: 1.6–1.8 times payload.
- Payload above 3 GiB: an additional 1.3–1.6 times payload.
- Evaluate low and high payload bounds separately; distribute each total between textures and DDS by payload share.

The vanilla baseline and fixed per-Mod overhead are unchanged. Startup reserve retains its existing formula; it is added to the revised resident curve. Measured peak-minus-resident reserves were 1.647, 2.940, 3.264, 3.078 and 2.871 GiB. Both startup and resident intervals cover the old four calibration groups and all new observations. Resident intervals remain within the existing 35% high/low tolerance.

| Calibration group | Revised resident interval | Revised startup interval |
| --- | ---: | ---: |
| Previous vanilla | 1.438–1.447 | 2.338–2.747 |
| Previous light | 3.649–4.472 | 4.717–6.303 |
| Previous 500 | 7.001–9.320 | 8.326–12.205 |
| Previous texture | 5.008–6.052 | 6.259–8.341 |
| Baseline repeat | 5.008–6.051 | 6.258–8.341 |
| 4.5 target | 8.377–10.422 | 10.109–14.020 |
| 6 target | 9.683–12.226 | 11.566–16.333 |
| 8 target, both runs | 12.266–15.633 | 14.433–20.657 |

Anonymous numeric fixtures in `TestUnity16CalibrationCoverage` cover these measurements. The memory CLI golden adds a synthetic 4 GiB texture fixture built from small headers, without allocating texture pixels. The persistent memory estimate key changes so cached launch budgets are recomputed.

## Limits

The largest group approached 20.66 GiB system commit, so payload was not increased under plan B. The 12 GiB target, original 720 list, extreme-250 list and roughly 24 GiB payload remain unmeasured here. The new curve beyond 7.5 GiB is extrapolation. One heavy-list repeat and one VM cannot establish a universal high-end slope. List composition, arbitrary Mod allocations, AssetBundles, saves and play growth remain outside reliable coverage. Paging made working sets much smaller than private bytes; it does not make these commitments free. VMware VRAM is unsuitable for calibration, and 1.7 behavior was not measured or changed.
