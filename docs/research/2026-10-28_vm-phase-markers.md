# VM：Archo Monitor 阶段标记的真实回合（2026-10-28 下午）

分支 `dev/0.9.0`。guest 里先后是 `4d031c0`、`876e0cb`、`4cf327c`、`3ccb442` 和 B8 修复那个提交的构建，Monitor 0.9.1。VM `Win11-Player-Test`，真实 home（快照 `RimArcho-Real` 之后的状态），界面中文。证据在 `D:\RimArcho-test\vm-phase`（截图、`phases16.txt`、`dl17*.txt`、`sz.txt`、`cats.txt`、`crash17-evidence.txt`、`sv3–7.txt`、`pl.txt`、B8 的 `b8live.txt`、`b8pl.txt`）。

游戏里的操作用 guest 内的 `SetCursorPos` / `mouse_event`（`vm.ps1 exec -Interactive`）注入：宿主上的 computer-use 点击到不了 VMware 里。RimDevTools 的自动测试没用上：它改 ModsConfig.xml、加自己的工具 Mod、用开发者快速测试地图代替“新殖民地”，也没有存读档步骤，测的就不是同一件事了。

## 1. 1.6（1.6.4871，列表 phase-test，18 个 Mod）

走完一整个回合：主菜单 → 新殖民地（坠毁、卡桑德拉、和平、随机地点、无文化）→ 3 倍速玩到第 2 天 22 时 → 存档 “New Arrivals5” → 读这个存档 → 玩到第 3 天 → 返回主菜单 → 退出到桌面。

- **两份日志的阶段顺序都对。** ArchoMonitor.log：`session`（15:31:25）→ `menu`（15:32:10，加载完紧接着）→ `new`（15:36:36）→ `play`（15:38:12）→ `save`（15:42:19，`context.save` = “New Arrivals5”）→ `play`（15:42:23）→ `menu`（15:44:33）→ `exit`（15:45:03，entries 5）。Player.log 里同样的 `[Archo Monitor] phase …` 行在第 51、54、83、100、120、134 行。
- **只有加载阶段有报错**，这和这份 1.6 列表相符：[SSR] 的两种 XML 错误（3 次）和 MVCF 的一个交叉引用。
- `archo problems`：每个日志簇后面带 `[loading xN]`。
- 能不能跑：“上次运行的报错”卡片里只有“加载游戏时”一组，空的阶段组不显示。
- 游戏日志：Archo Monitor 页签上有“加载游戏”分隔线，5 条都在下面；右侧卡片 62/38、40% 的比例对。

## 2. 1.7：下载能用，VM 原本起不来

- **下载**：`archo env download unstable --to C:\Games\RimWorld-unstable`，缓存登录，16.4 GB，约 37 分钟。
  - 中途 SteamCMD 掉线一次：`steamcmd has been disconnected from steam with result 3 (No Connection)`，接着 `Retrying...`。
  - 最后 SteamCMD 写了 `Success! App '294100' fully installed.`，RimArcho 却报“连不上 Steam”，退出码 1，没有加环境。这是 **B1，已修**（`876e0cb`）。
  - 修好后同一目录重跑：只做校验，`env:1.7 downloaded and added`，1.7.4990 rev1069。
  - SteamCMD 登录把 guest 的 Steam 客户端挤下线（已知），`steam.exe -shutdown` 后重开恢复。
- **下载连带了全部订阅的工坊 Mod（B2，未修，要决定）**：
  - 游戏本身约 1 GB，`steamapps\workshop\content\294100` 有 1293 个条目、15.6 GB。这是账户订阅的全部内容，`appworkshop_294100.acf` 里有记录。
  - RimArcho 按 02 §0 第 3 条把这个目录登记成共用的 `steamcmd` 来源。结果每个 Mod 都多了一份副本：2595 个实例，1294 个 Lineage 有多个实例，`pkg.duplicate-installed` 一千多条。1.6 环境也看得到，因为来源是共用的。
  - 连带的小问题：缺内容的提供方也列两份（SteamCMD 副本和工坊副本），`archo log` 的下一步先建议启用 SteamCMD 那份，而列表原来用的是工坊那份。
  - 08 §11 原先把“下载完成后的目录内容”列为未验证，现在知道了。
- **没有 Vulkan 时 1.7 启动就崩溃**：“No Vulkan support was found on this computer”（`VK_ERROR_INCOMPATIBLE_DRIVER`）。VMware 的虚拟显卡没有 Vulkan。游戏自己换了三种 Vulkan 设置（stage 1–3），三次都崩，生成 `Crashes\crash-…165213/165354/165529.txt`。
- **崩溃在 RimArcho 里看不出来（B3，未修，要决定）**：
  - 日志里是 `ERROR [Vulkan] [General] [Loader Message] vkCreateInstance: Found no drivers!` 这类行。去掉类别 `[Vulkan]` 后，消息以 `[General]` 开头，被当成“Mod 自报”（`modReportRe`）。`Vulkan fallback stage N` 归“其他”。这两类都是 `Informational`，所以被隐藏了。
  - 结果：能不能跑没有“上次运行的报错”；启动页写“上次游戏 … 没有报错”；游戏日志页却写“它的错误已列入需要处理”，自相矛盾。`archo log` 是 `list: (none)`，`hidden: 4 informational clusters`。
  - 真正的原因只在 `Crashes\crash-*.txt` 里，RimArcho 不读这个目录。
  - 后面装了软件 Vulkan 之后，每次启动的日志里仍有几条这样的 `[General] [Loader Message]` 行被列成“Mod 自报”（游戏日志页）。

## 3. 1.7 在软件 Vulkan 下的回合

经玩家同意，在 guest 里装了 Mesa lavapipe：pal1000/mesa-dist-win 的 `mesa3d-26.2.4-release-msvc.7z`（67 MB，核对过 SHA256），解到 `C:\Mesa\x64`，在 `HKLM\SOFTWARE\Khronos\Vulkan\Drivers` 登记 ICD。宿主上留了一份在 `D:\RimArcho-test\mesa`。游戏能跑，但帧率很低，注入的点击要按住 700 ms 才算数。

- **第 1 轮（17:13–17:58）整回合**：新殖民地 → 玩 → 存档 “New Arrivals9”（17:45）→ 读档（`save` 17:49:26，`play` 17:49:28）→ 玩约 2 分钟 → 返回主菜单（17:56:21）→ 退出到桌面（`exit` 17:58:09）。阶段顺序和 1.6 一样都对。报错只在加载阶段（[SSR] 和 MVCF），同 1.6。
- **第 2 轮（18:02–18:13）读档阶段的报错**：在 phase-test 里停用 5 个发型 Mod（AFU 男 / 女、Won 男 / 女、Lolidrop），读 “New Arrivals9”。游戏弹出 Mods mismatch，列的正是这 5 个，选 Load anyway。读档时有 23 次 `Could not load reference to RimWorld.HairDef named …`（8 个聚类，`[save xN]`）。
  - 运行中能不能跑分“加载游戏时 / 读取存档时”两组，退出后的上次运行也是；游戏日志页有“读取 New Arrivals9 18:06”分隔线。阶段分组对。
  - `4cf327c` 在真实运行里验证了：“本次运行”的“打开游戏日志”选中的是同一个聚类。
- **这一轮找到并修掉的**：
  - **B4 归因错**：这 23 次全记在 Vanilla Expanded Framework 名下，写“尚未确定来源”。它只是补丁了读档时经过的 `Pawn.ExposeData`。RimArcho 其实查到了提供这些发型的 Mod 装着但没启用（`archo log --cluster` 的 `providers: … (not enabled)`），只是排在补丁方后面。现在 `xref-missing` 有没启用的提供方时，它排在补丁方前，置信度 likely；能不能跑写“AFU女士发型 — 11 个警告 / 引用的内容不存在；它不在列表里”，四个发型 Mod 各一行。归因缓存版本加一，旧日志重新分析。
  - **B5 页面不刷新**：能不能跑开着时，后台检查完成的事件只更新侧栏；游戏退出也不重读。结果第 1 轮退出后写着“17:49”（读档时），第 2 轮写“18:06”，要切走再回来才变成退出时间。现在两种情况都会重画。复测：游戏 18:54:11 退出，没切页，下一次轮询后显示 18:54（窗口最小化时浏览器把 2 秒轮询放慢到约 1 分钟一次）。
  - **B6 重复的字**：每条本次运行都以“本次运行，”开头，区标题已经写了“本次运行”；凭据又写一遍“本次运行，N 次”。现在是“Won hair_men：3× Could not load …”，凭据只写日志文件名。
- **B7 运行中同一批错误列两遍（已修，见 §4、§4a）**：运行中，能不能跑的“上次运行”部分读的是游戏正在写的 Player.log，所以本次运行的错误又列了一遍，计数也翻倍，而且两段的阶段小标题一样。先加了“上次运行  时间”小标题，再按玩家的决定改了根本：运行中正在写的日志不当问题日志。
- 第 3、4 轮（18:35–18:54）是装了修复的复测，只到读档或主菜单。

## 4. 这一轮顺带修的（都在 `dev/0.9.0`）

- `7235e14`：能不能跑的凭据行原来是 `ArchoMonitor.log  Defs/…/Apparel.xml  <时间>`，窗口里空格被合并，读起来像一条路径。现在写成“文件 Defs/…  ArchoMonitor.log  时间”，窗口按两个空格拆成几段、间距 12px（06 §7.3）。
- `49a7e1e` + `4cf327c`：游戏运行中，“本次运行”的条目也用“打开游戏日志”，按第一行定位到日志页的对应聚类（实时聚类按批次编号，和日志页不同）。
- `876e0cb`：B1。
- `69964c2`：B4、B5、B6，B7 的小标题。
- `3ccb442`：B7。游戏运行时，当前环境的游戏正在写的日志（`App.LiveLog`）不当问题日志，上次运行改读 Player-prev.log；只读缓存（侧栏计数）也一样；游戏日志页对正在写的日志写“游戏还在写它：退出后列入需要处理”。
- `a0c3682`：B8。实时追踪里 Monitor 的条目先到时，等 Player.log 写完那一条，晚到的并进同一条（§4c）。
- 下一个提交：实时追踪判断“Player.log 有没有它”时也看噪声行（§4c）。

## 4a. B7 的真实验证（1.6，还原到 `RimArcho-Real` 之后）

列表 default 加 Harmony、Archo Monitor、[SSR] 和它的三个前置（12 个 Mod），[SSR] 加载时报 3 个错误、1 个警告。

- 第 1 次运行（19:22–19:24）只到主菜单再退出，作为“上次运行”。
- 第 2 次运行（19:26–19:43）走完整回合：新殖民地（坠毁、卡桑德拉和平、可随时读档、意识形态关闭）→ 玩 → 存档 “New Arrivals10” → 读档 → 玩 → 主菜单 → 退出。
  - 运行中：`archo problems` 的日志行是 “Player-prev.log … 19:24, current (from the launch … 19:22)”。能不能跑先列本次运行，再列“上次运行  列表  10 月 28 日 19:24”，本次运行没有列第二遍。
  - 侧栏计数：运行中原来只算本次运行，少了上次运行（只读缓存那一路运行中读不到报告，改动前就这样）。改成同样读 Player-prev.log 后，侧栏 5 = 区里 5（上次运行 1 + 本次运行 4）。
  - 退出后不用切页，能不能跑自己换成“列表 10 月 28 日 19:43”，侧栏 1 = 区里 1，`archo problems` 回到 Player.log。
  - 再启动到主菜单：游戏日志页对 Player.log 写“游戏还在写它：退出后列入需要处理”，没有“列入”按钮。
- **B8 实时追踪把一条错误拆成两条（已修，见 §4c）**：第 2 次运行中，“本次运行”里 [SSR] 的 `Could not resolve cross-reference … Reload_Standard` 出现两次，一条凭据是 Player.log，一条是 ArchoMonitor.log。中途重启 RimArcho、整份日志一次读完后只剩一条。看起来是伴生日志的条目比 Player.log 的那一行先到时，单独成了一个“只在 ArchoMonitor.log”的聚类，那一行到了以后没有合并。改动前就有。
- 小事：实时的“本次运行”列了一条没有归因的 `Translation data for language English has 9 errors`（凭据 ArchoMonitor.log），退出后的完整分析不列它。

## 4b. 测试存档进了真实的 Steam 云

这个 VM 用的是玩家真实的 Steam 账号，RimWorld 的 `Saves/*.rws` 走 Steam 自动云同步（guest 里 Steam 的 `userdata/1026722804/294100/remotecache.vdf`，205 个 .rws 条目）。所以：

- 今天的测试存档 “New Arrivals5”（1.6）、“New Arrivals9”（1.7）、“New Arrivals10”（1.6）都在云里，还原快照后又同步回了 VM，玩家自己的电脑上也会出现。
- 第 2 次运行里游戏自己的自动存档写了 Autosave-4（19:35）和 Autosave-5（19:37），这两个格子在云里原来的内容被覆盖了，没法恢复。
- 删不删这几个测试存档由玩家决定，没有动。以后的回合先在游戏选项里关掉自动存档，存档只用新名字。

## 4c. B8 的修复和真实验证（1.6，还原到 `RimArcho-Real` 之后）

原因：1.6 的这个 VM 里 Player.log 不带堆栈，一条错误只有一行，解析器要等下一行来了才知道它结束了。[SSR] 的 `Could not resolve cross-reference … Reload_Standard` 后面那一行（`Translation data …`）晚了约 19 秒才写，Monitor 的条目早就读进来了，等满 5 秒没在 Player.log 里看到结束的条目，就单独成了“只在 ArchoMonitor.log”的一条；Player.log 那一条结束后按另一个键归类，又成了一条。

修复（`internal/app/livelog.go`）：
- Player.log 正在写的最后一个条目就是它，或者 Player.log 还有一次读不完的内容时，Monitor 的条目接着等。
- 等过了才在 Player.log 出现的，并入那条只在 ArchoMonitor.log 的：Player.log 的条目当第一个样本并重新归因，凭据变回 Player.log。
- 次数每次从 Player.log 自己的计数重算，再取和 Monitor 计数里大的那个。原来没有阶段标记时是累加的，Monitor 的 3 次加上 Player.log 后来的 4 次会显示成 7 次。合成测试 `TestLiveLogMonitorFirst` 在改之前的代码上失败。

真实验证：同样 12 个 Mod 的列表，启动到主菜单后退出，加载期间每 3 秒读一次“本次运行”（`API.Live`）。游戏自己的调试日志里，这条错误在 20:32:36，下一行在 20:32:55，正好是 B8 的时序。“本次运行”从头到尾只有一条，凭据 Player.log，1 次；20:32:41–20:32:52 之间没有出现只在 ArchoMonitor.log 的那条。退出后 `archo log` 也只有一个簇（E6 `xref-missing x1`）。

同一次看到的两件事：
- 已修（B8 之后的提交）：`Translation data for language English has 9 errors` 在实时“本次运行”里仍是一条凭据 ArchoMonitor.log 的项。Player.log 也有这一行，但实时追踪判断“Player.log 有没有它”时只看算作报错的条目，这一行在 Player.log 里是噪声，就没算上；Monitor 那条带了堆栈，过了噪声过滤。完整分析看的是 Player.log 的全部条目，所以退出后不列它。现在实时追踪也记下 Player.log 里不算报错的行（只记哈希，最多 5 万条），Monitor 的同一条就不再单列（`TestLiveLogMonitorNoise`，没有在 VM 里重测）。
- 已修（`a91adc0`，真实 VM 验证过）：这次还原后马上启动，RimArcho 第一次扫描（1302 个 Mod，27 秒）在启动之后才完成，接着把 Archo Monitor 从 0.9.0-dev 换成随附的 0.9.1。列表在启动后变了，退出后这次运行的日志算“过时”（`stale log … not counted`），能不能跑只剩“更早的日志”一行。另外两份 Monitor 同时装着，`archo problems` 报 `mod.unloadable`：游戏只认其中一份。B7 那一轮是扫描完了才启动，没碰到。查明的原因：
  - 不是测试布置造成的。构建里的 `monitor/ArchoMonitor` 从 `D:\RimArcho-test\vm-phase\ArchoMonitor` 复制，两者逐字节相同：只有一个 Mod 文件夹，packageId `toumanlin.archomonitor`，版本 0.9.1（`monitor/VERSION` 也是 0.9.1）；快照里装的是 0.9.0-dev。玩家从带旧 Monitor 的 RimArcho 升级时走的是同一条路。
  - 过时：打开窗口时的 Monitor 检查（`monitor install` 子进程）复制 0.9.1 后做一次完整扫描（27 秒），扫描完才换方案里的 Monitor。游戏在这之前启动，启动记录的列表指纹是带 0.9.0-dev 的；换完以后指纹不同，退出后按 03 §6.4 第 3 条算“启动之后方案改过”。修法：只换 Monitor 版本不算改过（`App.monitorSwapOnly`）。没有改成等游戏退出再换（退出后那一换照样让这次过时），也没有改成启动时换（启动时 0.9.1 还没扫描到），见 00 §6 D21。
  - 两份同时装着是设计如此（08 §9：旧版本目录留着）。游戏这次加载的是 0.9.0-dev：启动时只链接了 `_Archo_monitor_ArchoMonitor-0.9.0-dev`。`mod.unloadable` 是误报：判断“游戏看得到什么”时，把方案里的 0.9.1（下次写入才链接）和 `Mods/` 里还在的 0.9.0-dev 链接都算进去，文件夹名 `…0.9.0-dev` 排在 `…0.9.1` 前面，新版本就成了被忽略的那份。实际上下次写入先删旧链接再建新链接，两份不会同时在 `Mods/`。现在方案里有同包名的另一份时，不在方案里的旧链接不算（`deploySet`，08 §3）。
  - 合成测试 `TestMonitorUpdateAfterLaunch` 按这次的顺序走：装 0.9.0-dev → 启动 → 游戏运行时更新到 0.9.1 → 写日志、退出 → 再启动。改之前在两处都失败（`Changed: true`；`mod.unloadable`）。
  - 真实验证（`a91adc0`，还原到 `RimArcho-Real`，同样 12 个 Mod，证据在 `D:\RimArcho-test\vm-monitor-swap`）：21:16:30 打开窗口，21:16:36 点首页的启动（v4，带 0.9.0-dev），21:17:07 历史里出现 `update Archo Monitor 0.9.0-dev → 0.9.1`，和原来的时序一样。主菜单退出后 `archo problems` 写 `log Player.log … current (from the launch of plan:default at 2026-10-28 21:16): 2 error, 1 warning`，没有 `mod.unloadable`；能不能跑的“上次运行的报错”是这一次（21:21，[SSR] 3 个错误、1 个警告）。`save --dry-run` 列出 `- _Archo_monitor_ArchoMonitor-0.9.0-dev`、`+ _Archo_monitor_ArchoMonitor-0.9.1`。再启动一次：`Mods/` 里只剩 0.9.1 的链接，Player.log 里是 `[Archo Monitor] 0.9.1`、`phase menu`；退出后这次的日志同样算当前。两次都只到主菜单，没有存档。之后还原到 `RimArcho-Real`，VM 关着。
  - 测试布置要注意：`vm.ps1 build` 从最新的 `dist/archo-*`（现在是 0.9.0-dev）复制 Monitor。要测随附新版本，build 之后把 `dist/vm/RimArcho/monitor/ArchoMonitor` 换成新的那份再打包，这两轮都是这样做的。
  - 还在的：`archo problems` 的 info `pkg.duplicate-installed  2 copies of pkg:toumanlin.archomonitor are installed`。每次 Monitor 更新后都会有（旧版本目录按 08 §9 留着），和这次的时序无关；只在技术信息和 CLI 里，能不能跑不列。

## 5. 待定

- B2：游戏下载后的工坊目录怎么处理。
- B3：1.7 的引擎 ERROR 行和崩溃报告怎么显示。
- 扫描没完成就启动时，Monitor 更新让这次运行算过时：已修并在真实 VM 1.6 验证（§4c）。旧 Monitor 版本让 `pkg.duplicate-installed` 多一条 info，要不要不算它，待定。
- Steam 云里的测试存档要不要删（§4b）。
- 没测到：游玩阶段真的有报错时的分组（这份列表玩的时候不报错）、TUI 和英文界面。
- VM 还原到 `RimArcho-Real` 会去掉 1.7、lavapipe 和这些存档；要再测 1.7 需重装 lavapipe（宿主 `D:\RimArcho-test\mesa`）并重新下载 1.7（约 16 GB，其中 15.6 GB 是 B2 的工坊内容）。
