# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §5); research goes in docs/research/.

## review docs, delete unnecessary 

## Mod library管理标签
- Mod library左栏, 支持以Tag分组展示, 提供便捷批量增加删除Tag的方式

## RimArcho安装
- 在设置中提供从RimSort RimCrow导入数据库, 在初始化RimArcho的时候提供导入数据库功能, 自动解析到玩家电脑的RimSort RimCrow数据库地址
- 初始化RimArcho的时候, 询问玩家是否将RimArcho管理的目录使用默认值在C盘local, 如果玩家C盘太满, 提示

## MCP test&improve
Baed on benchmark exam on real mod list and error log from corpus research

## Decompilation

Decompilation serves two purposes: investigating Mod conflicts and creating Mods. Reconsider storing decompiled assemblies in the cache and propose an approach.

2026-11-01: decided in [research/2026-11-01_decompile-storage.md](docs/research/2026-11-01_decompile-storage.md): a durable source library including the game's own code; being implemented. Mod projects are deferred (New Mod Project below).

### Decompilation management
manage decompiled mod instance

## New Mod Project
Allow player to initialize a new mod project with `about.xml`
I suggest a new side bar group: developing
developing
/decompile
/new mod project
Developing would possibly need new tools such as Bash, ilspyCMD, .Net SDK, 

## Translation
Auto translate mod details? Need to think about it.

## Save analysis

### 1.1 剩余（10）
- §1.4–1.5：从存档提取引用的 Def，映射回提供它的 Mod
- §2 内容目录：Def 类型 → 内容类别的映射（现为草案）
- §3 未使用的内容（1.1 核心答案）：三组分类、解读、功能型 Mod 的处理，Unplayed 分组下的 Play review 页面
- §4.3 "上次加载的报错"

### Playthrough review (data for the third question)

这个估计还得琢磨琢磨, 目前不满意

Join multiple saves of a colony into a timeline: in-game days, real play time, population, wealth, and deaths.

Include research progress (the undecided item in 10 §1.4) and DLC in content not yet played.

Optionally count occurrences instead of only recording that something appeared. This costs little and can show how deeply the content was played.

## Archo Agent

Archo should be a Mod development agent that can take action, not just translate or tag. Tentatively use a modified Pi agent; the approach needs research.

# Deferred

## Auto Update
RimArcho would need to publish to steam workshop, need to find a way to upload update without start the game; Auto update should pull from steamworkshop, not from github.

## Research displaying, editing, and managing Mod configuration

Not now: profiles, exports, cleanup.

## TUI

The TUI lacks a navigation bar and feels slow during use.

## Loading animation
Maybe add a UI loading animation--low priority for now

## 1.6 memory on a real graphics card is inferred
Calibrated in the VM up to 7.5 GiB texture payload (2026-11-01). Most of the VM's texture RAM is the driver's copy, which the VMware adapter (no dedicated VRAM) commits as process memory; the model now counts it as VRAM and adds back only what does not fit in 90% of the card (research: unity16-texture-memory-breakdown). The 720 list: 36–52 GiB resident without dedicated VRAM, 15–30 GiB with a 24 GiB card. Deferred (2026-11-01): the estimate is good enough for now, so no real-hardware run is planned. If false warnings are ever reported, run 1.6 once on a PC with a discrete GPU and compare Task Manager's "Commit size" and "Dedicated GPU memory" with the estimate (vanilla: about 0.9 GiB commit, 0.66 GiB dedicated).