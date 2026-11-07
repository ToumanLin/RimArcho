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

## Translation
Auto translate mod details? Need to think about it. 因为需要OCR. 

## Save analysis

### 允许重命名殖民地

### 我创建殖民地之后, 在启动器中修改

sav25 launch不应该存在, 只允许从启动界面启动
应改为:
如果当前设置的env和plan是存档对应的值
    能直接载入 [启动] -> 去启动页
如果当前设置的env和plan/pack并不是存档对应的值
    使用$env$和$plan/pack$能直接载入 [切换并启动] -> 去启动页
    使用当前的$env$和$plan/pack$, 下面展示变化和影响

### `游戏存档`中栏技术信息
中栏技术信息底部的完整Mod列表更改为只展示变化(绿色增加红色删除, 不考虑加载顺序改变), 并且在太长的时候折叠, 允许展开, 对数据库中有的mod增加超链接到Mod库对应mod
- §1.4–1.5：从存档提取引用的 Def，映射回提供它的 Mod
- §2 内容目录：Def 类型 → 内容类别的映射（现为草案）

## §4.3 "上次加载的报错"
`Can-it-run` 中, 将`上次运行时的报错`分为三块, `上次启动时的报错` `上次载入$存档$时的报错` `上次游玩$存档时的报错
`Can-it-run` 中, 在`开始之前`以上次加载的存档展示`能不能载入`

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

你可用agent-bridge parallel派发任务给codex 用 gpt-6.1-sol effort high, 可当worker或者sub agent, 让它做代码研究和修改代码, 然后你review, 如果你让codex修改前端, 请务必检查她的前端实现的用户体验(不必要和功能重复的选项, 重复性文字等), 简单直接的任务在这里做