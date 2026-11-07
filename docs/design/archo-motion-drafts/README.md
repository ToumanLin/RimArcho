# Archo motion: plan and drafts

Archo's chibi figure keeps the player company while the GUI loads. Loading cards rotate seven calm expressions, as a still or as a live figure: neutral, smile, laugh, wink, think, ponder and yum. This folder holds the plan and the mockup it was reviewed with.

**Status (2026-11-07):** the first three places are implemented (16 §7.1 "Archo figure"): the start-up screen, loading cards and the first-run guide, with usage tips (`view.Tips`) and the setting `ui.archo.motion`. The decisions below changed the mockup's plan:

- Placement B: the figure left in the loading card, a usage tip on the right (above three shimmer lines).
- The setting offers off | still | animated, default animated.
- No separate still phase, and a shorter wait: at 0.21 s (start-up screen and loading cards alike) the figure appears and turns live as soon as the puppet is ready (the still only covers its creation).
- First release: the start-up screen (centred, a tip below), loading cards, the first-run guide. The other places below stay for later.
- Not decided: whether the chibi replaces `portrait.svg` as Archo's image elsewhere (15, 16 §2.13).

The figure, its rig and its asset pipeline live outside this repository in `archo-portrait/anim` (README there). Only the exported runtime would enter the repository.

## Mockup

```powershell
node docs/design/archo-motion-drafts/server.cjs
```

Open `http://127.0.0.1:8767/`. The server reads the product's `style.css` and `icons.js` from this checkout. It reads the Archo runtime from `RIMARCHO_ARCHO_RUNTIME`, by default `D:/Game/RimModManager_Upgrade/archo-portrait/anim/runtime`.

- **Scenes**:
  - Page loading: the Mod library with simulated reads of 0.2, 1, 4 and 15 seconds, or a failure.
  - Long tasks: two tasks, one finishes and one fails.
  - Other places.
- **Switches**:
  - Archo: live, still, off.
  - Size: 96, 120 or 160 px.
  - Placement: A centred above the skeleton, B to its left.
  - Simulated reduced motion, and the theme.
- **Readout**: live figures, CPU per frame, texture memory, and figures created and released.
- `archo-wait.js` is a draft of the product module `js/archo.js`.

## 1. What changes for the player

1. **Page loading.** `beginPageRead` (`js/screens.js`) puts a fixed-size slot in its `page-loading` card.
   - A read that ends within 0.4 s shows nothing new. Routine reads have a 200 ms target (16 §10), so most pages never show Archo.
   - Longer reads show a still of the next expression in the rotation, fading in with the §7.1 entry motion.
   - After 1.2 s the still cross-fades into the live figure.
   - A wait longer than 4 s moves to the next expression every 4 s.
   - When the read ends, the figure fades out with the exit timing, and the page draws.
   - Reads long enough to show Archo (18, `docs/research/2026-10-26_page-load-time-and-optimization.md`):
     - first whole-library read in a new process: 1.45 s;
     - subscriptions: 0.56–0.74 s;
     - log page: 0.57 s;
     - uninstall preview: 4.5 s;
     - Can it run with a stale analysis: 35 s;
     - cross-version log analysis: 30 s.
2. **Rotation.** Each wait takes the next of the seven expressions, round-robin, so repeated loads do not repeat a face.
3. **A new setting.** Settings › interface › Archo animation (`ui.archo.motion`): live (default), still, or off.
   - Off leaves the shimmer alone, exactly as today.
   - With `ui.gpu off`, without WebGL2, or under `prefers-reduced-motion`, live behaves as still: one image, no rotation.

### Other places for the figure (later steps, listed for the decision)

| Where | Expression | Kind |
| --- | --- | --- |
| Long tasks on the tasks page (texture count and conversion, analysis, downloads, pack build, scan) | 🧐 🤔 alternating | Live, only one at a time; the others still |
| Task finished | 😁 with a hop (`success`), 2.4 s, then fades | Live once |
| Task failed, page read failed | 😭 / 😳 | Still beside the error |
| One-sentence empty states (staging empty, no logs, no saves) | 😪 | Small still |
| First-run guide: welcome, scanning, scanned | 😜 → 🧐 → 🫡 nod | Live |
| "Ask Archo to analyze" card in the game log: running, found problems, nothing found | 🤔 / 😳 / 😊 | Live while running, then still |
| Archo panel (15, 1.1) | All 16 agent states, talking while text streams | Live, the large package |
| Settings › about | A click shows 🤪 😆 🥰 | Easter egg |

## 2. Cost

All measured in Chromium on this machine (Ryzen 9 7950X, RTX 4090). WebView2 numbers come from the protocol in §4.

| | Static still | Live figure |
| --- | --- | --- |
| Download | 18 KB per expression at 2× for 120 px (127 KB for all seven) | 238 KB package (small) + 3 modules ≈ 40 KB |
| Decoded memory | ≈ 230 KB per image | 2.4 MB textures (small package; 9.6 MB with the 0.6× package) |
| CPU per frame | 0 | 0.14 ms (1,253 vertices) at 24 fps, under 0.4% of one core |
| GPU | Composited image | One WebGL2 context |
| When nothing waits | Nothing | Nothing: the figure is released 10 s after the last wait |

How the cost stays bounded:

- **Nothing at startup.** The runtime is a dynamic `import()` the first time a wait passes 0.4 s. Cold start and idle memory do not change; startup takes 390–605 ms (18), so it normally ends before Archo would appear.
- **One figure.** A single shared puppet moves its canvas between slots (`mount` / `unmount`). Concurrent waits beyond the first show stills.
- **Small package for small figures.** The 0.3× export renders the same as the 0.6× one at 120–160 px (2× DPR). The 0.6× package is only for the future Archo panel.
- **Coarse mesh below 200 px.** Same picture, about a third of the vertices.
- **Drawing stops** when the slot leaves the page, the page is hidden or minimised, or the slot scrolls out of view (IntersectionObserver + `visibilitychange`). WebView2's low-memory mode does not pause rendering (Codex research), so the figure pauses itself.
- **Release.** 10 s after the last wait, `destroy()` frees the textures and buffers and loses the context. Back-to-back reads reuse the figure; an idle window has none.
- **Transfers.** Wails copies each response into a Go buffer before handing it to WebView2. That copy is transient, not retained, and embedded files get no cache header. A `Cache-Control` header for `/archo/` lets WebView2 cache the package, so later figures do not go through Go again.
- **Binary.** Both executables embed `assets`: +≈ 0.4 MB each with the small package and stills. No size budget exists; the updater limits are far away.

## 3. Specification changes (drafts for review)

- **16 §7 principle 7** gains an exception:
  > Archo's figure may move continuously while it represents a wait or Archo's own activity: loading cards, the tasks page and the Archo panel. It is drawn on one canvas and stops drawing when hidden, out of view or released. It shows a still under `prefers-reduced-motion`, with GPU acceleration off, or when the player chooses still or off.
- **16 §7.1** gains "Archo figure":
  - the 0.4 s / 1.2 s / 4 s timings;
  - the rotation;
  - one live figure;
  - the release after 10 s;
  - fades use `--t-enter` / `--t-exit`.
- **16 §2.14**: the `ui.archo.motion` setting.
- **15 / 16 §2.13**: in the GUI, Archo appears as the chibi figure. The app icon and the bow mark stay `portrait.svg` (§8.6). *Needs your decision.*
- **16 §10 / 18**: acceptance rows from §4.
- **CHANGELOG**, **release-vm-tests**: one row each.

## 4. Acceptance and measurement

Codex proposed this protocol from the existing tooling: the native WebView2 runner `scripts/vm/run.mjs` and `cdp.mjs`, private working set as in 16 §10, `RIMARCHO_PERF=1`.

| Check | Target |
| --- | --- |
| Cold start to interactive list | Unchanged (≤ 1.5 s) |
| Go idle private working set | Unchanged (≤ 80 MB) |
| Total after 10 idle minutes, after 20 load/release cycles | Unchanged from baseline (≤ 200 MB); no growth across cycles |
| Total during a live 10 s wait at 160 px | Recorded; the plan expects a few MB of textures plus the context, to be confirmed |
| Long tasks caused by creating the figure | None over 50 ms (texture upload is spread over frames if needed) |
| CPU per frame | ≤ 0.5 ms |

A debug-only delay for page reads (`RIMARCHO_PERF`) makes the 10 s wait reproducible.

## 5. Work split

| Part | Who |
| --- | --- |
| Runtime into `assets/archo/`: modules, small package, stills | Claude |
| `js/archo.js` (from `archo-wait.js`), slot in `beginPageRead`, `style.css`, settings choice, zh strings, front-end tests | Claude |
| `ui.archo.motion` setting: registry, validation, Init payload, CLI docs, tests | Codex |
| `Cache-Control` for `/archo/` in `PageHandler`, test | Codex |
| Debug delay for page reads; runner script sampling private working set per §4 | Codex |
| Specification edits | Claude drafts, Codex checks references (`refs.cjs check`) |
| Review of everything Codex changes | Claude |

Codex does not touch `internal/surface/gui/assets/`.

## 6. Decisions needed

1. Placement A (centred above the skeleton) or B (to its left).
2. Default for `ui.archo.motion`: live or still.
3. Whether the chibi becomes Archo's GUI image (15, 16 §2.13), with the app icon unchanged.
4. Whether the 0.4 s / 1.2 s thresholds feel right in the mockup.
5. Which of "other places" join the first release.
