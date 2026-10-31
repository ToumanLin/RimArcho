# 06 · 界面 · TUI（§9）

> 属于 [06 · 界面](README.md)。章节号在整个 06 里连续，拆成多个文件后不变，引用照旧写 `06 §x.y`。

## 9. TUI

### 9.1 定位与页面映射

TUI 是**方案编辑器的键盘版**：默认打开当前方案的编辑器（§2.5），其他页面经命令面板到达，不画侧栏。TUI 和 GUI 用同一套 view 模型和同样的按键（§3）。

| GUI 页面 | TUI |
| --- | --- |
| 方案编辑器 | 主界面（已实现）。切换、新建、改名、删除方案：`P` 或命令面板；顶栏显示“方案 <名字>” |
| 能不能跑 | `shift+2`（`@`）或命令面板（已实现，二级界面，`esc` 回编辑器）；屏幕级按键 `A` 重新检查、`l` 选择日志 |
| Mod 库存 › 订阅与更新 | `c` 或命令面板；来源、可更新、变化记录按简单列表展示，`1` / `2` / `3` 跳区，`C` 落在单 Mod 的变化记录；历史操作不变。动作（2026-10-30 已实现）：`K` 检查所有有 Mod 的来源，来源行 `enter` 只查这一个；`u` 更新光标所在的可更新项，`U` 更新显示的全部，都先显示 `--dry-run` 的文字再 `enter` 确认，游戏运行时拒绝；`Del` 撤掉订阅（同样先预览；Steam 客户端管的工坊条目在 Mod 库存退订）；可更新行 `p` 先锁定这个版本（按两次）、`ctrl+w` 打开来源页；`+` 或直接粘贴去添加订阅。都排进后台任务（§2.16），状态行写“已排进…队列”，结束时写结果。下载页的更新要选新下载的文件：TUI 说明去窗口或用 `archo subscribe update --from`。账户跳转（LoversLab 登录）仍在 GUI |
| Mod 库存 › 添加订阅 | 订阅与更新的 `+`、`tab`、粘贴，或命令面板“添加订阅”（2026-10-30 已实现）。粘贴框同 GUI，来源识别是同一张表（`subscribe.Detect`）；打开时就在输入，`q`、`?` 等都是字母。认出后下面是这一种要问的一个决定（`↑↓` 选：Git 跟踪默认分支 / 分支 / 标签 / 提交，选了后三个就进名字框，`tab` 在名字框和粘贴框之间切换；工坊用 Steam 订阅 / 用 SteamCMD 下载）和后台跑出的 `--dry-run` 文字，`enter` 订阅（排进后台任务）；`ctrl+u` 清空，`esc` 先退出名字框、再清空、再回订阅与更新。下载页只在页面有唯一 Git 仓库时能在 TUI 订阅（“用 Git 订阅 <仓库>”）；要选 zip / 文件夹或 LoversLab 站点文件的，说明去窗口或 `archo subscribe page <地址> --from <zip\|文件夹>`。没有来源卡和“最近添加” |
| Mod 库存 › 全部 | 命令面板“Mod 库存”、`shift+6`（`^`）：和 GUI 同一套界面（§2.9）：筛选栏、可展开的 Lineage 行、宽时右边是副本详情，按键和右键菜单同 §3.5、§8.5 |
| 游戏日志 | 在能不能跑里 `l` 选择日志；日志项 `enter` 打开技术页（已实现） |
| 整合包、设置 | 命令面板，显示为带命令的文本面板（已实现） |
| 环境 | `E` 切换环境（已实现）；目录和启动设置用 CLI `env set` |
| 更新 RimArcho（待实现，07 §5.1） | 命令面板“更新 RimArcho”：文本面板，内容同 GUI 设置里的更新卡（版本、大小、更新说明摘要、会替换的文件、被什么挡住）。`enter` 确认后，下载、验证、解开排进后台任务（§2.16），状态行写进度；准备好后再按 `enter`：备份数据库并退出，由脚本换文件，不自动重新打开（要再开就运行 `archo`）。TUI 自己也是被换掉的程序，所以这一步会退出 TUI。TUI 打开期间自动检查发现新版本，状态行写一条安静通知 |
| 导航在侧边 / 顶部（`ctrl+b`） | 没有侧栏，不适用 |
| 启动页、编译成整合包、游玩回顾、值得玩 | 不提供页面；对应的 CLI 命令可在 `:` 里运行。没有启动页，所以编辑器里的 `L`（和顶栏的 `L 启动`）仍然直接写入并启动，不像 GUI 那样先去启动页（§2.5）；状态行在“正在启动游戏”后面写最可能出事的问题（“留意：…”，`view.LaunchHint`），`shift+2` 去能不能跑 |
| Archo 面板 | 暂不支持（09 §6） |

- 游戏运行状态在**状态行右端**（GUI 侧栏没有对应的一行：2026-10-30 删去，GUI 自己读新日志，§2.1）：游戏运行时是 `● 游戏运行中  14:05 起`，本次运行的报错数是后面的琥珀色数字；游戏退出后有新的 Player.log 时是 `● 分析本次日志  shift+2`（强调色）；都没有时什么都不显示。状态行的一句话在左边，两者同时出现时这一段留在右边。新错误到来时顶栏“能不能跑 N”更新，不弹窗（`view.LiveTracker.Update`，刷新间隔见 §2.16.1 T01；文字是 `view.LiveText`）。
- 能不能跑的日志项按 §2.4 的同一份阶段分组：缩进的淡色标题，不占光标、不计数；上下键始终落在实际报错项上。没有标记的日志不加标题；查看动作仍用现有技术页。
- 右键菜单的键：Bubble Tea v1.3.10 不认 xterm 的 `shift+F10`（`CSI 21;2 ~`），Windows 控制台送来的是不带 shift 的 F10，菜单键在 xterm 类终端里是 F16；所以 TUI 里 F10、`shift+F10`、F16 都打开菜单，右键在指针处打开。TUI 的筛选栏占整个宽度（“未启用”一列放不下筛选块）；Mod 库存的副本详情只有备注、动作、状态和字段，没有简介和图（**待实现**）；Mod 库存在最底层按 `esc` 回到编辑器（TUI 没有侧栏）。
- 跳页用 `shift+1`–`8`（终端里是 `!@#$%^&*`，美式布局；`shift+7` 贴图优化是带命令的文本面板），子页之间用 `tab` / `shift+tab`（§3.5）；`g` 是标签，不再是“到顶”（2026-10-22 定）。原来“`g` 前缀跳页”的建议不采用。
- `archo tui` 进入 TUI，与不带参数相同，但不经过 WezTerm 宿主（§9.2）。界面上不出现 `#id`：结果由 TUI 写成人话，错误经 `view.Humanize`；保留 CLI 文字（带引用）的只有技术页和 `:` 的输出。技术页里 `enter` 跟进所选行的第一个引用；打印的命令里不带 `#` 的实例引用（05 §3.1 规则 11）也能跟进，只要当前索引认得它（`lineRef`）。
- 颜色：termenv 在 ConPTY 下无法查询终端、会退回无色，所以 `SetColors` 在终端声明支持 24 位色时强制 24 位色：`COLORTERM=truecolor|24bit`、`TERM_PROGRAM=WezTerm`，或有 `WT_SESSION`（Windows Terminal）。其他终端沿用 termenv 的检测。

### 9.2 WezTerm 宿主

图标和缩略图都依赖终端能力，而玩家的终端五花八门（例如 Windows Terminal 默认字体 Cascadia Mono 没有 Nerd Font 图标）。因此 RimArcho 自带一个受控的显示环境。

**启动流程**（`internal/host`）。只有不带参数的 `archo` 会走到这里；`archo tui`、`archo --here`（强制在当前终端运行）、CLI 子命令和 `archo mcp` 从不触发（`wantsHost`）。检查顺序：

```
双击 archo.exe（或在任意终端里运行 archo）
  ├─ 已经运行在 WezTerm 中（TERM_PROGRAM=WezTerm）→ 在这里运行，完整模式
  ├─ terminal.host none → 在这里运行
  ├─ 找到了 WezTerm：PATH 中的 wezterm-gui，然后 %ProgramW6432% / %ProgramFiles% / %LOCALAPPDATA%\Programs 下的 WezTerm\，然后 <数据目录>/wezterm/
  │    → 写 <数据目录>/wezterm.lua，启动 wezterm-gui --config-file <数据目录>/wezterm.lua start -- archo tui [--env x]，本进程退出
  │      （--config-file 是全局参数，必须写在 start 之前）
  └─ 没找到，且有控制台可问 → 询问：
       “下载 WezTerm 便携版（约 66 MB）以显示图标和 Mod 预览图？[Y/n]”
       Y：下载、校验、解压到 <数据目录>/wezterm/，设 terminal.host wezterm，然后按上一行启动
       n：设 terminal.host none，在当前终端以兼容模式运行，并说明怎么改
```

- 下载只在玩家同意后进行，不静默下载。`archo config set terminal.host auto|wezterm|none`：auto（默认）表示还没问过，没找到 WezTerm 时询问一次；none 以后在当前终端运行；wezterm 表示总是用宿主。
- 下载来源：WezTerm 官方 GitHub Releases 的最新稳定版（`/repos/wezterm/wezterm/releases/latest`），取 `WezTerm-windows-*.zip` 和同名 `.sha256`。边下载到 `.part` 文件边计算哈希，校验不符则不解压、不启动，回到当前终端。解压前检查每个条目都在 `<数据目录>/wezterm/` 之内，有一个越界就整体拒绝。
- 没有控制台可问时（输出被重定向、管道），不询问，直接在当前终端运行。
- 任何失败（下载、校验、启动）都说明原因，然后在当前终端运行 TUI。

**wezterm.lua**（`internal/wezterm` 生成，每次启动重写，手改会丢失）：

| 项 | 设置 |
| --- | --- |
| 字体 | **随 RimArcho 自带** Maple Mono NF CN（Regular、Bold；SIL OFL 1.1，许可证文件一并分发），放在 `<程序目录>/fonts/`，通过 `font_dirs` 加载，不安装到系统。它同时包含 Nerd Font 图标和中文字形，所以图标、中英文的宽度和风格完全受控。回退顺序为 Symbols Nerd Font Mono（WezTerm 内置）、Cascadia Mono、Segoe UI Symbol；不能设 `font_locator = 'ConfigDirsOnly'`，否则 Maple 没有的字形（如 `⬥`、`⇣`）会显示成方框。字号 11 |
| 配色 | 与 §7 色板一致的深色方案：前景 `#E6E6E6`、背景 `#0E0F12`、光标 `#A5B4FC`、选区 `#2C2E4A` |
| 窗口 | 无标签栏、无滚动条，标题为 “RimArcho”，初始 160×46，内边距左右 8、上下 6 |
| 图片 | iTerm2 内联图片协议（WezTerm 原生支持）；关闭 kitty 图形协议 |
| 宽度 | `treat_east_asian_ambiguous_width_as_wide = false`，`unicode_version = 14`，与 RimArcho 的宽度计算一致 |
| 退出 | `exit_behavior = 'CloseOnCleanExit'`：archo 正常退出时关窗；出错退出时保留窗口，错误信息可读 |
| 内存 | `front_end = 'OpenGL'`（WebGpu 内存高得多，以后的 WezTerm 可能把它设为默认，所以固定）；`line_state_cache_size`、`line_quad_cache_size`、`line_to_ele_shape_cache_size` 都降到 128（默认 1024，浏览 Mod 会显示大量不同的行，大多是 CJK，每行都被缓存；128 仍能容纳最大化窗口的行，CPU 不变）；`glyph_cache_image_cache_size = 16`（图片由 archo 自己的 LRU 保存）；`scrollback_lines = 0`（TUI 在备用屏幕上运行） |
| 其他 | 关闭响铃、关闭字体连字（`harfbuzz_features = { 'calt=0', 'clig=0', 'liga=0' }`）、关闭更新检查和更新窗口、关闭配置热加载、关闭窗口时不确认 |

- WezTerm 是独立进程，有自己的 GPU 和内存开销（实测见 07 §6）。§10 的 archo 目标只约束 archo 进程本身。
- WezTerm 在 Windows 上总会在 `%USERPROFILE%\.local\share\wezterm\` 写 GUI 套接字和日志，这个位置不能通过配置改变；README 和卸载说明要提到它。
- 脚本驱动 WezTerm 时注意：`WEZTERM_UNIX_SOCKET` 为空时，`wezterm cli` 会退回默认套接字，可能是玩家自己的窗口。测量脚本找不到自己的 `gui-sock-<pid>` 时必须停止。

**自带字体的打包**：`scripts/release.ps1 -MapleTag v7.9`（固定版本）从 github.com/subframe7536/maple-font 的发布页下载 `MapleMono-NF-CN.zip` 和 `.sha256`，校验哈希，取 `MapleMono-NF-CN-Regular.ttf` 和 `-Bold.ttf`；`OFL.txt` 取自同一标签的仓库，脚本检查它是 SIL OFL。全部放在发布 zip 的 `fonts/` 下。下载缓存在 `dist/cache/`（不进 git）。仓库里没有字体文件。

### 9.3 兼容模式

在其他终端中运行时（RimArcho 无法控制别的终端用什么字体）：
- 图标换成 §7 表格中的 Unicode 备用字符。`ui.icons auto|nerd|compat`：auto（默认）在 `TERM_PROGRAM=WezTerm` 时用 Nerd Font 图标，否则用兼容字符；玩家自己的终端已经在用 Nerd Font 时可以设 `nerd`。
- 不显示缩略图，也不预留缩略图区，名称顶格。
- 链接不可点击，在文字后面淡色显示域名；描述图片显示为文字占位。
- 其余布局和交互完全相同。

### 9.4 缩略图

- **绘制**：bubbletea 渲染完一帧后，用 iTerm2 内联图片协议（OSC 1337），在详情区预留的矩形里单独绘制图片。做法：包装输出 writer，在同一次写入中帧后追加清除或绘制；绘制时必须临时显示光标（ConPTY 在光标隐藏时不转发定位）。
- **尺寸由 RimArcho 决定，不交给终端**：图片按比例缩放到 18×9 格以内，取能放下它的最少行数 N，用透明像素补成恰好 18×N 格的比例，以 `width=18;height=N;preserveAspectRatio=0` 发送，所以图片覆盖的正好是这 18×N 格（16:9 占 5 行，正方形 8 行，竖图 9 行）。不能用 `preserveAspectRatio=1`：WezTerm 会保持宽度、按比例推算高度，竖图会超出预留区，清除后留下残条。
- **预留区**：宽度固定 18 格；高度等于图片的行数，尺寸还不知道时按 16:9，没有 Preview.png 时是一行，居中显示淡色的“无预览”。名称、作者、事实写在预留区右边。右边剩不到 16 格文字时不显示预留区。全屏详情中预留 36×18 格，规则相同。
- **清除**：选中项变化时，图片在下一帧消失，并在同一次写入中对图片自己占的那几行逐行执行 `ECH 18`。文字覆盖和 bubbletea 的整行重写都不能擦掉图片格子，不能依赖它们；也不擦整个 9 行（会把短图片下面没变的文字擦掉）。
- **不等待**：选中项变化时立即绘制。为了不让快速滚动卡顿：
  - 在后台预取光标上下各 10 个 Mod 的缩略图。可见图片和预取各用一个加载槽、每槽一张，所以当前行的未命中不会排在预取后面；加载完成时即使光标已经离开也保留，留作以后的预取。
  - 编码好的图片序列放在内存 LRU 里，上限 64 张（缩略图已是 360 像素宽，64 张只占几 MB）；屏幕上的和正在加载的从不被逐出。
  - 命中缓存时立刻画；未命中时立刻开始解码，解码完如果光标还在这一行就画，否则不画。
  - 每帧重画已经显示的图片（这一帧可能写过它们），最多再加一张新图片；其余按 §2.16.1 T05 的间隔逐张画，不需要新帧。简略详情里只有缩略图，这条主要影响全屏详情。
- `ui.preview_dwell_ms`：光标停留多久后才画缩略图，默认值和范围见 §2.16.1 T04，玩家觉得闪烁时可以调大。`ui.previews off` 整体关闭图片。
- **描述图片**（只在全屏详情中）：在原位置占一块区域，宽度为列宽和 60 格中较小者，行数按图片比例算、最多 20 行；用与缩略图相同的方法绘制（补成整格比例、`preserveAspectRatio=0`、离开时 ECH 清除）。只有完全进入可见区域时才画，部分可见时显示占位行；加载中占一行。
