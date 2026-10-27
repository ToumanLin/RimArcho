# 06 · 界面 · GUI 技术（§8）

> 属于 [06 · 界面](README.md)。章节号在整个 06 里连续，拆成多个文件后不变，引用照旧写 `06 §x.y`。

## 8. GUI 技术

### 8.1 交付

同一份源码，链接两次：

| 文件 | 子系统 | 双击 | 用途 |
| --- | --- | --- | --- |
| `archo.exe` | 控制台 | WezTerm 中的 TUI（§9.2） | CLI、`archo mcp`、`archo tui`；`archo gui` 启动 GUI |
| `RimArcho.exe` | Windows GUI（`-H windowsgui`，没有控制台黑框） | GUI | 给玩家双击的入口（可带 `--env <名称>`）；带命令参数时按 CLI 执行，供 GUI 的扫描和分析子进程使用 |

- 两个 exe 共用数据目录和数据库，可以同时打开（SQLite WAL 支持多进程）。GUI 窗口获得焦点时比较 `Stamp()`（数据库状态，`ChangeKey`），变了就重新加载；在旧视图上的编辑不覆盖对方的修改（G8）。
- `archo gui`：同目录下有 `RimArcho.exe` 就启动它并立即返回；没有就在当前进程中运行 GUI。
- 发布包含两个 exe（体积见 07 §6）。

### 8.2 前端

- Wails `v3.0.0-beta.28`（v3 仍是 beta，锁定这个版本，升级单独验证）。不需要 CGO（`CGO_ENABLED=0`）。
- 纯 HTML + CSS + 原生 JavaScript（ES modules）。**不用框架、不用 npm、没有构建步骤**。文件放在 `internal/surface/gui/assets/`（`index.html`、`style.css`、`js/` 下 `api.js` `state.js` `lists.js` `detail.js` `screens.js` `popups.js` `exchange.js` `icons.js` `shell.js` `home.js` `guide.js` `plans.js` `motion.js` `main.js`），用 `go:embed` 打进 exe。
- 调用 Go：手写的 `api.js` 用 `import { Call } from "/wails/runtime.js"` 和 `Call.ByName`，用 `Proxy` 拼出方法全名 `github.com/ToumanLin/RimArcho/internal/surface/gui.API.<方法>`。不使用 `wails3` 生成的绑定，构建只需要 `go build`。
- `internal/surface/gui`：`api.go`（Wails 服务，每个方法只调用 view）、`gui.go`（窗口、`/preview/`、`/img/` 本地图片处理器、CSP、图标路由）、`exchange.go`（导入导出对话框）、`text.go`。
- Go → 前端：Wails 事件（`scan` 扫描结束、`analyze` 分析进度与结束、`pack` 编译进度）。游戏运行状态由前端每 2 秒调用 `Live` 轮询。
- 列表虚拟滚动：只创建可见行。
- **页面读取**（§3.1 规则 9：进入优先于读取）：**每一页**都已通过 `beginPageRead` 在等待数据前切换页面并显示骨架，包括启动、能不能跑、变化、整合包及编译预览、设置、环境、引导、卸载预览、Mod 库、方案编辑器、游戏日志、贴图优化、全部列表、后台任务和全屏详情。同页刷新保留已显示的内容；切换 Mod 或整合包时显示相应占位，避免旧对象的操作还可点击。每次读取校验页面代号、请求序号、环境及方案；晚到的成功或错误不覆盖新页面。同一页面连续请求只接收最后一次的结果，失败结束加载状态并允许重试。全屏详情的延迟工坊描述也遵守相同校验。
- **预览的操作状态**：整合包类型/贴图选项改变后重新读取预览，未完成时快捷键也不使用旧预览启动编译。卸载选择重新估算时先撤掉旧确认按钮；只有读取完整结果后才显示下一步操作。
- 样式表首行 `[hidden]{display:none !important}`：弹层容器的 `display:flex` 会盖过 `hidden` 属性，形成一层看不见的遮罩吃掉所有点击。
- WebView2 用户数据放在 `<数据目录>/webview2`，不放 `%APPDATA%\RimArcho.exe`。
- `spikes/guidev`：开发用，在 127.0.0.1 上用同一个 API 驱动页面，便于在浏览器里调试（`-assets <目录>` 直接读磁盘上的页面文件，改了刷新就能看到）；不进发布包。
- `/dev/pagecheck` 用模拟 API 检查各页的立即反馈、晚到结果与错误、重入和失败重试，也覆盖大日志的可变行高与键盘定位；使用真实前端模块，不连接产品数据库。

### 8.3 安全

- 页面的 Content-Security-Policy：`default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'`。`script-src 'self'` 下 Wails 运行时正常工作，不需要 `unsafe-eval`。
- 工坊简介和图片都是外部内容：简介由 Go 端解析成 BBCode 块结构后传给前端，前端用 `textContent` 和元素构建，**从不把外部文字当 HTML 插入**；图片只经过 `/img/` 处理器从本地缓存读出，不直接加载远程地址。
- 链接用系统浏览器打开；`OpenURL` 拒绝非 http(s) 链接。
- Wails 在 Windows 上用 WebView2 的资源拦截，不开任何 TCP 端口。
- 设置页只运行 `view.SettingCommand` 允许的命令（§2.14）。

### 8.4 长任务

- 扫描和分析在子进程中运行（`archo scan`、`archo analyze`，`app.ChildScan` / `ChildAnalyze`，子进程带 `CREATE_NO_WINDOW`）。进度通过事件推到状态行，界面不阻塞。TUI 同样如此（`:` 里输入 `scan` 也交给子进程）。
- 启动时：有索引就立即显示列表，然后在后台增量扫描；从没扫描过时自动开始扫描。
- 加入新游戏（§2.15、§2.8）：扫描和第一次分析在子进程里，引导页和其他页面都能操作；新游戏的数据读完前它的页面显示骨架。Go 端的 GUI 方法不在调用里做可能超过约 100ms 的事，长的一律交给协程或子进程，结果用事件推回（§3.1 规则 9）。
- 列表、侧栏和历史一起由 `Workspace` 读取，结果用事件推回。主 App 的 Mod 索引仍有效时（改列表之后就是这样）直接在主 App 上构建，几十毫秒；索引要重载时（扫描之后、换游戏）改用独立连接，不在持页面锁时加载全库。
- 整合包编译在后台进行，进度推到编译页，可以停止（§2.6）。
- 所有这些和 Mod 库的文件、Steam 动作都记成后台任务（§2.16，`app.Tasks`）：一处看到进行中的、排着的和做过的，结果写进 `<数据目录>/tasks.json`。

### 8.5 鼠标

方案编辑器里的鼠标操作是动词的另一种触发方式：

| 操作 | 键盘 | 鼠标 |
| --- | --- | --- |
| 移动光标 / 切换区 | `↑↓` / `←→`、`1`–`4` | 单击 |
| 选择 | `shift+↑↓`、`ctrl+space`、`esc` 清除 | `shift+单击` 范围、`ctrl+单击` 切换 |
| 启用 / 停用 | `space` | 双击；拖到另一列 |
| 暂存 / 放回 | `t` | 拖到暂存列 / 拖出 |
| 调整顺序 | `J` `K`、`m` 位置输入 | 在“已启用”内拖动；落点线旁显示“第 N 位” |
| 标签 | `g` | 右键菜单 › 标签 |
| 撤销 / 重做 | `u` / `ctrl+r`、`ctrl+z` / `ctrl+y` | 状态行里的“撤销”链接 |
| 排序 / 写入 / 启动 | `S` / `w` / `L` | 右上按钮 |
| 搜索、筛选 | `/`、`ctrl+f`、`\` | 筛选栏（未启用上方）；暂存的搜索框 |
| 右键菜单 | 菜单键、`shift+F10` | 右键 |
| 列宽 | `<` `>` `=` | 拖动分隔线、双击恢复 |
| 历史 | `H`、`u` / `ctrl+r` | 点历史里的一步 |
| 页面后退 / 前进（§3.1 规则 11） | `alt+←` / `alt+→` | 侧键 XButton1 / XButton2 |

- **拖放规则**：从“未启用”或暂存拖到“已启用”的某一行上 = 放到那一行的序号；拖到最后一行下方 = 放到末尾；在“已启用”内拖 = 移动；拖到暂存 = 暂存；“已启用”拖到“未启用” = 停用；暂存拖到“未启用” = 移出暂存、不启用（`view.Unstage`）。按下和松开在同一行是单击。拖动中状态行说出落点；拖动中按 `esc` 取消。松手即生效，可以 `u` 撤销。
- **右键菜单**（方案编辑器和 Mod 库存）：共用动作表和分组顺序，每项右边写它的键，只列适用的项（`view.RowOps` / `view.CopyOps`，名字和键来自 `view.OpTable`）：
  1. 方案（仅方案编辑器，Mod 库存整组不提供）：`space`（启用 / 停用 / 放回）、`m`、`t`、`x`、从所有方案里拿掉；
  2. 整理：标签 `g`、备注 `n`；
  3. 打开：打开文件夹 `ctrl+o`、打开工坊页面 `ctrl+w`；
  4. 版本：锁定这个版本、更新锁定版本、解除锁定、备份锁定版本；
  5. 磁盘：恢复、清除已有转换贴图、重新订阅及订阅来源动作，最后是红色的退订 / 删除（`Del`）。Mod 库存不提供单 Mod DDS 转换；重新订阅的确认说明 RimArcho 先把文件夹移到回收站，再由 Steam 重新下载；条件见 §2.9，清理验证与 Steam 调用见 08 §10。

  组之间是一条细线。菜单键和 `shift+F10` 在光标所在行打开它，焦点在第一项；`↑↓`（或 `tab`）移动、`enter` 执行、`esc` 关闭，按菜单里写的键也直接执行。有多选时只列能作用于多个的项（文件夹、页面、备注、位置、换成另一份只对一个）。
- 方向键、滚轮、单击永不改动数据（§3.1 规则 1）。只有双击、拖放、按钮和动词键会改动数据。
- **GUI / TUI 的差异**（2026-10-24）：页面历史与 `alt+←` / `alt+→` 只在 GUI 提供；TUI 保留现有的页面切换和技术页栈返回，不处理鼠标侧键。
- TUI 的鼠标：单击移动光标、`ctrl+单击` 加选、`shift+单击` 范围选择、拖动列之间的空白调整列宽、单击详情区聚焦、滚轮滚动详情。TUI 不做拖动移 Mod 和框选。

### 8.6 图标与 Windows 资源

- **图形**（2026-10-25 换成带轮廓光的新版）：`internal/icon/portrait.svg`（1024×1024，矢量），放在圆角深色底（`#1B1D23`，圆角半径为边长的 0.22）上。48px 及以上显示整幅；40px 及以下只显示头部（头发和脸，视框 x 120–910、y 170–960），任务栏上仍能认出；只裁脸太紧，小尺寸只剩一块肤色。
- **渲染**：`internal/icon/svgr.go` 的小型 SVG 渲染器，不加依赖。按元素树绘制：`<g>` 继承 fill / stroke 等属性；transform（matrix、translate、scale、rotate、skewX/Y）；路径、矩形（含圆角）、圆、椭圆、线、折线、多边形；nonzero 与 evenodd 填充；描边（圆角连接）；纯色、线性与径向渐变（用户坐标或图形包围盒，含 gradientTransform）；opacity、fill-opacity、stroke-opacity；clip-path；filter 只认 feGaussianBlur。defs、clipPath、渐变和滤镜里的内容只被引用，不直接画。4 倍超采样；256px 全部尺寸约 1 秒。
- **不提交二进制**：仓库公开，不放二进制。`icon.PNG(size)` 抗锯齿光栅化（每像素超采样），供窗口图标和资源使用；`icon.SVG(size)` 是同一构图的矢量版，页面的 favicon 用 `/icon.svg`（256）和 `/icon-small.svg`（16），由它生成。
- **窗口图标**：Wails 先取 exe 的图标组 ID 3，没有时才用传入的 PNG。所以发布版不画；开发版（没有 `.syso`）按系统大图标尺寸（`SM_CXICON`，100% 缩放下 32px）画一张，画完立即 `FreeOSMemory`。以前画 256px，要 43 MB 堆（4 倍超采样的 float64 缓冲 32 MB），只为得到一张 32px 的图标，Go 之后也不把这些内存还给系统。
- `go run ./scripts/winres`（发布脚本调用）写 `cmd/archo/rsrc_windows_amd64.syso`：一个 COFF 资源对象，含图标组和版本信息。`.syso` 不提交（`.gitignore`）。
- **图标组 ID 3**：Wails 窗口按 ID 3 取应用图标；资源管理器取第一个图标组，两者是同一个。尺寸 16、20、24、32、40、48、64、256，每个尺寸一张 PNG。
- **版本信息**：`FileVersion`、`ProductVersion` 取发布版本号，`ProductName` RimArcho，`FileDescription` 区分 `archo.exe`（RimArcho command line）和 `RimArcho.exe`（RimArcho）。两个 exe 由同一个包构建，发布脚本在两次构建之间各写一次 `.syso`。
- **没有资源时**（开发构建、`spikes/guidev`），窗口用 `application.Options.Icon` 的 PNG（256 px）作后备。


### 8.7 列表问题与 needs 缓存 API

编辑器和能不能跑读取同一个后端快照。入口是 `API.Needs`、`API.ListProblems` 和 `Board.problems`；结构定义在 [`view.Needs`](../../../internal/view/needs.go) 和 [`view.ListProblems`](../../../internal/view/listproblems.go)，缓存实现是 [`gui/needscache.go`](../../../internal/surface/gui/needscache.go)。

**调用时机**：

- Workspace 的 `environment` 事件里，`board.problems` 已包含列表问题，首屏直接使用。
- 打开能不能跑调用 `Needs()`；只需要编辑器问题时调用 `ListProblems()`。两者命中同一个缓存。
- `Toggle`、`Stage`、`Unstage`、`MoveBy`、`Place`、`StageAll` 等返回 `view.Result` 的编辑，在 `result.problems` 中带回更新摘要。返回字符串的 `Undo`、`Redo`、`DoAction` 等通过 `list-problems` 事件带回同一结构。
- 后台日志／内容分析补齐也发 `list-problems`；`analyze` 的进度与完成事件继续保留。无需为了刷新标记再请求一次问题列表。

`ListProblems` JSON 的主要字段：

| 字段 | 用途 |
| --- | --- |
| `environment`, `plan` | 环境 ID 和打开列表 ID，用于过滤导航前的迟到事件；启动页仍按当前列表读取 |
| `listVersion` | 这份结果对应的列表内容指纹 |
| `revision` | 缓存输入指纹；相同输入复用，不应按字典序比较 |
| `sequence` | 当前 API 会话内递增的发布序号；同环境、同列表只接收不小于已显示序号的结果 |
| `updating`, `error?` | 后台仍在补齐时保留上一份内容结果并标 `updating=true`；失败时保留问题，返回错误说明 |
| `count`, `severity` | 与 needs 共用的计数、最高级别（`error` / `warning` / `info`）；隐藏维护项及淡色信息项沿用 needs 的计数规则 |
| `mods` | 以 Instance ID 索引；值含 `count`、`severity`、`summary`、`problemIds`。未出现的 Mod 没有可展示的问题 |
| `items` | 列表级完整 `Need` 数组；每个问题只传一份，按 `problemIds` 取对应项 |

`Need` 在原字段之外含 `id`、可选的 `target` 和 `requiredBy`。依赖的 `target` 是去空白、大小写归一、去 `_steam` 且按注册表解析目录别名后的 packageId；`requiredBy` 是 `{id, name}` 数组。声明依赖、语料中的缺依赖、目标未启用的补丁可以按目标合并；需要它的 Mod 都获得问题归属。已有未安装列表条目覆盖依赖时，归属附在该条目的汇总项上。强顺序问题继续合成一个自动排序项；单条强约束另有 `move` 动作。

**执行动作**：把 `item.actions` 中的完整 Action 对象传给 `DoAction(action, option)`。`id` 是动作标识，执行参数仍由 `kind`、`refs`、`after`、`options` 等原字段承载，不是只传 ID。`choose-disable` 的 `option` 为从零开始的选项索引，其他数据动作传 `0`。`enable` 自动按约束放置依赖；`move` 使用一个 `refs` 和 `after`；`sort` 直接排序；`subscribe` 进入既有订阅任务队列。DLC 的商店链接通过 `open-url` 打开。`view`、`list`、`fold`、`save` 等界面动作仍由界面分派，确认规则仍按 `confirm` / `warn` 执行。

**缓存边界**：每个 API 会话最多保留八个环境／列表／游戏版本／语言槽位。输入还包含列表内容、数据库变化键、扫描快照与分析器版本（`AnalysisKey`）、日志路径及大小／mtime，以及每次现查的目录和游戏链接状态。Workspace 复用完整 Board／Shell／History；需要重新组合时用独立连接，避免持有主锁。

列表编辑走 `App.IncrementalProblems`，同步重算声明层，保留可复用的上一份内容层问题；已启用目标对应的旧 `patch.no-match` 提示立即移除。后续日志读取与完整分析在独立会话／原分析子进程中进行，发布前重新核对输入，过期工作不能覆盖新编辑。内容分析继续使用按启用集合保存的分析槽和按 Mod 内容键保存的 Def／程序集事实；补丁模拟依赖顺序及整个文档，仍在后台重算。
