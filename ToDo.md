# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §4); research goes in docs/research/.

## Mod library
Mod详情, 标签`编辑`的弹窗
1. 如果标签太多, 底部会被外窗口clip掉, 应该限制y高度, 增加滚动条
2. 输入和筛选栏表现一致, 基于输入增加/减少下方展示的tag
3. 下方显示的tag中, 应该顶置当前mod所拥有的tag

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

### `游戏存档`中栏技术信息
中栏技术信息底部的完整Mod列表更改为只展示变化(绿色增加红色删除, 不考虑加载顺序改变), 并且在太长的时候折叠, 允许展开, 对数据库中有的mod增加超链接到Mod库对应mod
- §1.4–1.5：从存档提取引用的 Def，映射回提供它的 Mod
- §2 内容目录：Def 类型 → 内容类别的映射（现为草案）
在中栏中
### 1.1 剩余（10）
- §3 未使用的内容（1.1 核心答案）：三组分类、解读、功能型 Mod 的处理，Unplayed 分组下的 Play review 页面
- §4.3 "上次加载的报错"

Can-it-run 中, 将`上次运行时的报错`分为三块, `上次启动时的报错` `上次载入$存档$时的报错` `上次游玩$存档时的报错`

### Playthrough review (data for the third question)

这个估计还得琢磨琢磨, 目前不满意

Join multiple saves of a colony into a timeline: in-game days, real play time, population, wealth, and deaths.

Include research progress (the undecided item in 12 §1.4) and DLC in content not yet played.

Optionally count occurrences instead of only recording that something appeared. This costs little and can show how deeply the content was played.

## Archo Agent

Archo should be a Mod development agent that can take action, not just translate or tag. Tentatively use a modified Pi agent; the approach needs research.

# Deferred

## Research displaying, editing, and managing Mod configuration

Not now: profiles, exports, cleanup.

## TUI

The TUI lacks a navigation bar and feels slow during use.

## Loading animation
Maybe add a UI loading animation--low priority for now

## 1.6 memory on a real graphics card is inferred
Calibrated in the VM up to 7.5 GiB texture payload (2026-11-01). Most of the VM's texture RAM is the driver's copy, which the VMware adapter (no dedicated VRAM) commits as process memory; the model now counts it as VRAM and adds back only what does not fit in 90% of the card (research: unity16-texture-memory-breakdown). The 720 list: 36–52 GiB resident without dedicated VRAM, 15–30 GiB with a 24 GiB card. Deferred (2026-11-01): the estimate is good enough for now, so no real-hardware run is planned. If false warnings are ever reported, run 1.6 once on a PC with a discrete GPU and compare Task Manager's "Commit size" and "Dedicated GPU memory" with the estimate (vanilla: about 0.9 GiB commit, 0.66 GiB dedicated).