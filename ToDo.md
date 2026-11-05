# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §4); research goes in docs/research/.

## review docs, delete unnecessary 

## Mod library管理标签
- Mod library左栏, 支持以Tag分组展示, 提供便捷批量增加删除Tag的方式

## RimArcho安装与更新（创意工坊发布）
原"RimArcho安装"和 Deferred 里的"Auto Update"合并为一个项目。调研：[research/2026-11-05_workshop-distribution.md](docs/research/2026-11-05_workshop-distribution.md)。

**分发**：创意工坊条目名为 RimArcho，内容 = Archo Monitor 的 Mod 文件夹（About、LoadFolders、1.5/1.6/1.7）+ 发布 zip + `.sha256`。没有人只为 Monitor 订阅，条目就是 RimArcho 本身。

**玩家步骤**：Steam 订阅 → 打开条目文件夹 → 解压 zip → 双击 `RimArcho.exe`。

**首次启动**
- 检测自己是否运行在创意工坊文件夹里（`steamapps/workshop/content/294100/<id>/`）。如果是，给出默认安装地址（可另选），把自己搬过去，再从新位置重启。Steam 更新或取消订阅时会改写、删除这个文件夹，所以程序不能留在那里。
- 搬走后清理在条目文件夹里解压出来的文件，只留条目原本的内容。
- 询问 RimArcho 管理的目录是否用默认值（C 盘 Local）；C 盘空间不足时提示。
- 提供从 RimSort、RimCrow 导入数据库（自动找到它们在玩家电脑上的数据库位置）；设置里也有同样的入口。

**Monitor**
- 订阅得到的副本就是 Monitor。RimArcho 要把任何来源中 packageId 为 `toumanlin.archomonitor` 的副本都认作 Monitor，否则它和 `_Archo_monitor_` 链接会被报成重复 Mod。
- 版本相同时优先用订阅的副本：不需要建链接，`Mods/` 在 exFAT 上也能用。
- Monitor 版本（`monitor/VERSION`）与启动器版本相互独立。

**更新**
- 已订阅的玩家：Steam 会自动把新 zip 下到条目文件夹，RimArcho 直接从那里读取并校验，不需要 SteamCMD。
- 未订阅的（GOG、GitHub 安装）：用匿名 SteamCMD 拉取条目，绝不用玩家的账号。
- 版本号放在条目 tag 里（不行就改读标题），zip 名必须与之一致。
- GitHub 保留，用于首次下载、预发布版和兜底。
- 建议同时加 ed25519 签名：维护者账号一旦被盗，`.sha256` 防不住。

**上传**（维护者）：`archo workshop publish`，经 steam-worker 调 ISteamUGC 上传，不启动游戏；不放进 `release.ps1`。

**进展**
- 2026-11-05：完成调研。发布 zip 从 1.0.2 起带 `README.zh-CN.md` 和 `skills/`（代码已写，未提交）。
- 创意工坊部分尚未实现。
- 下一步：先做上传工具，传一个不公开的测试条目，在 VM 里实测调研文档第 4 节的问题（tag 能否放版本、不公开条目能否匿名下载、订阅后 Monitor 是否正常加载且不报重复）。
- 调研文档已按"Monitor 即条目"改好：条目内容和 Monitor 识别见 §1.5，首次启动搬家见 §1.6，订阅者直接从条目文件夹取更新见 §1.3。

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