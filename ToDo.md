# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §4); research goes in docs/research/.

## MCP test&improve
Baed on benchmark exam on real mod list and error log from corpus research

## Decompilation

Decompilation serves two purposes: investigating Mod conflicts and creating Mods. Reconsider storing decompiled assemblies in the cache and propose an approach.

2026-11-01: decided in [research/2026-11-01_decompile-storage.md](docs/research/2026-11-01_decompile-storage.md): a durable source library including the game's own code; being implemented. Mod projects are deferred (New Mod Project below).

### Decompilation management
manage decompiled mod instance

## New Mod Project
Allow player to initialize a new mod project with `about.xml`
I suggest a new side bar group: developing, or debugging, not sure
因为开发mod和处理mod冲突处理bug从工具角度来看没有不同, 都要反编译/编译
developing
/decompile
/new mod project
Developing would possibly need new tools such as Bash, ilspyCMD, .Net SDK, 
所以这也要考虑, 假设一般玩家用不到这些工具, 那么这整个group都是没法用的
如果要给一般玩家用, 需要让玩家下载这些工具

## GUI
Player feedback says the GUI is very jumpy. Investigated 11-08 (headless-Edge filmstrips, `D:\RimArcho-test\jumpy`). Fixed: details pane swaps once (editor + Mod library), loading card hidden for fast reads. Still to do:
- Late data pushes pages around after they draw: All lists' memory estimate (2 shimmer lines → 4 text lines) moves the sections below; plans page sections shift ~40px; launch page step 03 / problem note / save button change height and push the headline (`home.js` 395–406, 583–597).
- Launch page redraws on every window focus with no change check and jumps back to the top (`home.js:25`, `main.js:1341`); scan / analyze / new-log events redraw it too.
- Sidebar: sub-entries open and close with a 250 ms height animation on every page change, and on Mod library they appear only after the page has drawn, so the entries below slide twice. Consider no animation, or reserving the space at once.
- No in-app "reduce motion" setting: motion only stops with Windows' reduced-animations setting (`motion.js:6`). Status line slides in every message and changes height (24↔30 px); sidebar highlight slides; list FLIP.
- Scrollbars without `scrollbar-gutter` in the editor lists, library rows and `#screen` pages: content shifts ~5–10 px when a scrollbar appears.
- Editor page head wraps when the problems chip / "N unsaved" / save button change width (`problems.js:33`, `main.js:210, 221`).
- Startup: nav-at-top and sidebar width applied after first paint; window buttons added after an async check squeeze the breadcrumb (`shell.js:37, 136`, `window-chrome.js:17`).
- Smaller: bold on selected rows / chips changes text width; editor number column width follows the digit count; library selection bar in the page flow; Can it run redraws fully on the 2 s live poll while the game runs.
- Check the details swap and the hidden loading card on a real 5,000-Mod library in the VM (WebView2 timing).
- Pre-existing test failures found on the way: `/dev/pagecheck` "subscriptions … update preview" (update skipped preview/confirmation) and feedbackcheck.js:143 (`click` of undefined).
- `motion.js:1-2` says motion uses only opacity and transform; the sidebar grid rows and some progress bars animate layout.

## Translation
Auto translate mod details? Need to think about it. 因为需要OCR. 

## Save analysis

- §1.4–1.5：从存档提取引用的 Def，映射回提供它的 Mod
- §2 内容目录：Def 类型 → 内容类别的映射（现为草案）

### 1.1 剩余（10）
- §3 未使用的内容（1.1 核心答案）：三组分类、解读、功能型 Mod 的处理，Unplayed 分组下的 Play review 页面

### Playthrough review (data for the third question)

这个估计还得琢磨琢磨, 目前不满意

Join multiple saves of a colony into a timeline: in-game days, real play time, population, wealth, and deaths.

Include research progress (the undecided item in 12 §1.4) and DLC in content not yet played.

Optionally count occurrences instead of only recording that something appeared. This costs little and can show how deeply the content was played.

## Archo Agent

Archo should be a Mod development agent that can take action, not just translate or tag. Tentatively use a modified Pi agent; the approach needs research.

# Deferred

## mod更新的语义理解
当前只展示什么文件改了, 未来
1. 从steam拉changelog, 如果mod本身有changelog, 自动读取, 有的mod会做in-game changelog, 需要研究怎么放
2. 通过AI接口summary更新内容

## Research displaying, editing, and managing Mod configuration

Not now: profiles, exports, cleanup.

## TUI

The TUI lacks a navigation bar and feels slow during use.

## Loading animation
Maybe add a UI loading animation--low priority for now

## 1.6 memory on a real graphics card is inferred
Calibrated in the VM up to 7.5 GiB texture payload (2026-11-01). Most of the VM's texture RAM is the driver's copy, which the VMware adapter (no dedicated VRAM) commits as process memory; the model now counts it as VRAM and adds back only what does not fit in 90% of the card (research: unity16-texture-memory-breakdown). The 720 list: 36–52 GiB resident without dedicated VRAM, 15–30 GiB with a 24 GiB card. Deferred (2026-11-01): the estimate is good enough for now, so no real-hardware run is planned. If false warnings are ever reported, run 1.6 once on a PC with a discrete GPU and compare Task Manager's "Commit size" and "Dedicated GPU memory" with the estimate (vanilla: about 0.9 GiB commit, 0.66 GiB dedicated).

## Linux version
Players asked for Linux and macOS (2026-11-07). Linux deferred; macOS later still (game files inside `RimWorldMac.app`, Player.log in `~/Library/Logs`, signing and notarization, needs a real Mac). Survey of what is Windows-only today:
- Does not compile: `startWorkshopCMD` has no non-Windows version (`app/subscribeworkshop.go`). Wails needs cgo + GTK/WebKitGTK and a native Linux build, and `surface/cli` imports `gui`, so put the GUI behind a build tag for a CLI/TUI/MCP-only build. CI is Windows-only. 17 §5 wrongly says `GOOS=linux` compiles.
- Safety stubs to fix first: `gameProcesses` returns nil, so "refuse while the game runs" never fires (`/proc`); `toolIdle` always errors, so SteamCMD and todds can't be installed or updated.
- Bugs that only show on Linux: same-drive checks with `filepath.VolumeName` (always "") make `os.Rename` hit EXDEV across mounts (`locklib.go`, `directories.go`; compare `st_dev`); renaming a save to a name differing only by case overwrites the other save (`saveactions.go`); `LocalDir` falls back to `~/.cache`, which holds the lock library (use XDG data dir); trash fails across volumes.
- Steam: SteamCMD installer only knows the Windows zip (Linux is a tar.gz with `linux32/`, needs 32-bit libs); interactive login has no terminal from the GUI; Steam-client subscribe needs `libsteam_api.so` via dlopen (purego); `SteamSignedIn` needs `~/.steam/registry.vdf`.
- Game layout: `ManagedDir`, `menubg`, `gametest/paths.go`, steam_api copy only know `RimWorldWin64_Data`/`.exe`. Proton installs (Windows exe, user data in `compatdata/294100/pfx/...`, `-savedatafolder` as a `Z:` path) are common and not detected.
- GUI on WebKitGTK: `:has()` and `color-mix()` need WebKitGTK ≥ ~2.42; WebGL Archo often falls back to stills; no CDP, so VM harness, memprobe and 16 §10 / 18 baselines need redoing.
- Release: tarball with exec bits (not `Compress-Archive`), `.desktop` + icon; Flatpak sandbox blocks Steam library writes, symlinks and `/proc`. Workshop self-update is windows/amd64 only; release/relocate code assumes `archo.exe` + `RimArcho.exe`. LoversLab login (WebView2 COM + DPAPI): skip, keep the manual-zip fallback.
- Suggested order: 1) compile + safety stubs + Linux-only bugs + CI, ship CLI/TUI/MCP; 2) native GUI tarball, Proton, SteamCMD; 3) Steam-client subscribe. Test in a Linux VM.

你可用agent-bridge parallel派发任务给codex 用 gpt-6.1-sol effort high, 可当worker或者sub agent, 让它做代码研究和修改代码, 然后你review, 如果你让codex修改前端, 请务必检查她的前端实现的用户体验(不必要和功能重复的选项, 重复性文字等), 简单直接的任务在这里做