# 游戏测试运行：RimDevTools 调研与实测（2026-10-24）

目的：给 RimArcho 做“用给定方案启动游戏、到主菜单、收集错误、关游戏、出报告”（08 §12）。先看 `D:\Game\RimModManager_Upgrade\RimDevTools`（只读）怎么做，再在本机两个游戏副本上实测。

## 1. RimDevTools 怎么做

许可证：MIT（`LICENSE`，2026 白金trigger）。可以借代码，但它是 C#，我们是 Go，实际只借思路，没有拷代码。

| 环节 | RimDevTools 的做法 | 代码位置 |
| --- | --- | --- |
| 启动 | `ProcessStartInfo` 直接起 `RimWorldWin64.exe`，参数 `-popupwindow`，auto/xml 模式再加 `quicktest`（直接进测试地图）；可选在隔离桌面（`CreateDesktop`）里跑，不抢前台 | `Infrastructure/GameProcessLauncher.cs`、`Commands/TestCommand.cs` |
| 隔离配置/存档 | **不隔离**。它改玩家真实的 `ModsConfig.xml` 和自己工具 Mod 的设置，先用 `ConfigTransaction` 备份，结束后恢复；中断后下次启动先 `TransactionRecovery` 恢复。另有 `EnvironmentLock` 防并发 | `Infrastructure/ConfigTransaction.cs` |
| 设置方案 | 读外部 ModsConfig 或项目配置，算依赖，渲染后写进真实 `ModsConfig.xml` | `ModListParser`（`TestCommand.cs` 内） |
| 判断成功 | 靠自带的**工具 Mod**（LogSyncMod）：`LongEventHandler.ExecuteWhenFinished` 后 10 秒打 `[LogSyncMod] RUNTIME_READY_TO_CLOSE`；Harmony 补丁 `Root_Entry/Root_Play.Update` 每 5 秒打心跳 `HEARTBEAT runId=… seq=…`。quick 模式要“就绪信号之后又来一次心跳”才算成功；60 秒无心跳判 `GAME_HUNG`；总超时可配 | `source/LogSyncMod/LogSyncMod.cs`、`RuntimeHeartbeat.cs`、`IncrementalLogSignalScanner.cs` |
| 读日志 | LogSyncMod 把日志按 PackageId/关键词过滤后写到自己的同步文件，CLI 增量读这个文件（处理截断重读） | `IncrementalLogReader` |
| 结束 | `Process.Kill(true)` 杀进程树；杀不掉就保留配置事务不恢复，报 `GAME_TERMINATION_FAILED` | `DeployCommand.cs` `ProcessUtil.TerminateAsync` |
| 结果 | 单个 JSON 结果（成功/失败码、runId、Mod 列表、依赖警告），auto 模式另有游戏内写的 `*_Result.json`/`*_Evidence.json` | `CliResult` |
| 前置检查 | Steam 必须在运行；已有 RimWorld 在跑就先等它退出 | `SteamSessionGuard` |

值得借的：心跳/卡死判定的思路、“杀不掉就别恢复”的保守收尾、单一 JSON 结果、先等已有游戏退出。

不借的：改真实 `ModsConfig.xml` 再恢复（我们用 `-savedatafolder` 完全隔离，不碰玩家的东西，也就不需要事务恢复）；必须装工具 Mod 才能判断就绪（游戏目录只读时装不进去，见 §2.3 的替代办法）。

## 2. 本机实测

两个副本：`D:\rimworld16`（1.6.4871 rev590，Unity）和 `D:\SteamLibrary\steamapps\common\RimWorld`（1.7.4985 rev1269，Waylith 引擎）。都只读启动，数据目录放在临时目录。

### 2.1 日志去哪了

- **1.6**：`-savedatafolder=<dir>` 只搬 Config/Saves，Unity 的 Player.log 不跟着走。加 `-logfile <路径>`（两个独立参数，路径别再套引号，套了会得到空文件）就写到指定位置。
- **1.7**：日志位置写死。反编译 `RimWorld.dll`：`FileLog.SetDirectory(Bootstrap.ResolveEarlyLogRoot())`，后者只返回 `%USERPROFILE%\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`，**不看 `-savedatafolder`**；启动时把 `Logs\Player.log` 轮换成 `Player-prev.log`。所以用 `-savedatafolder` 跑 1.7 会覆盖玩家真实的日志（本次第一次探测就发生了：玩家原来的 `Player-prev.log` 被替换）。
  - 解决：加开关 `-NoFileLog`（`FileLog.Disabled`，`SetDirectory` 直接返回，不碰任何日志文件），日志同时走控制台（`Microsoft.Extensions.Logging` 的 console logger），由我们接管 stdout/stderr。`WindowsConsole.OpenIfRequested` 在 stdout 已是有效句柄时不改它，所以管道能收到。
  - 控制台格式和文件格式不同：`HH:MM:SS  LEVEL 消息`，续行 `HH:MM:SS <原文>`，没有日期和分类。转成文件格式（`=== Waylith log ===` 头、`Version:` 行、`日期 时间  LEVEL 消息`）后 `internal/logs` 照常解析。
  - 这也回答了 01 §5.2 / 08 §5.3 的待实测项：**整合包用 `-savedatafolder` 跑 1.7 时，日志不在 `<包>/userdata/Logs/Player.log`，而在环境共用的位置**，现有整合包代码的假设不成立（另记为后续问题）。

### 2.2 隔离是否干净

数据目录里出现的：`Config/{ModsConfig,Prefs,KeyPrefs,Knowledge,LastPlayedVersion}`、`HardwareProfile/`、`ShaderCache/`、`TextureCache/`（1.7 首次约 27 MB 纹理包）。玩家的 LocalLow 目录除 1.7 的日志（已用 `-NoFileLog` 解决）外没被写。游戏目录没被写。Unity 的分辨率等 PlayerPrefs 在注册表，未核对。

### 2.3 怎么知道到了主菜单

原版没有“加载完成”的日志行。反编译 1.6 与 1.7 的 `Assembly-CSharp.dll`：`UIRoot_Entry.Init()` → `MainMenuDrawer.Init()` → 第一句 `PlayerKnowledgeDatabase.Save()`，写 `<数据目录>/Config/Knowledge.xml`。启动前删掉它，它再出现就是到了主菜单，不需要任何工具 Mod。实测 1.7 只开 Core：启动后约 16 秒出现。

测试地图：参数 `quicktest` 走 `Root_Play.SetupForQuickTestPlay`，`Game.InitNewGame` 打 `Initializing new game with mods:`；之后日志安静一段时间视为地图就绪（启发式）。

### 2.4 其它发现

- `D:\rimworld16\Mods` 里有一个 RimArcho 建的悬空链接 `_Archo_env_yvae4hr7j2zwsjepwkqalnykwq`（指向已不存在的 `…\_3700765558_alt_decapitate`）。1.6 的 `ModLister` 扫到它直接抛 `DirectoryNotFoundException`，`ModsConfig`/`ModLister` 类型初始化失败，之后每帧 `Root level exception`，日志 47k 行。这个副本目前**连 Core 都进不了主菜单**。测试运行能把这种情况报成失败（错误簇里有它），但根因是链接维护的 bug，另记。
- 1.7 的 `ModsConfig` 静态构造只改名不删除找不到的 Mod，所以不能靠游戏回写的 ModsConfig 判断哪些 Mod 没加载；要在启动前自己核对“游戏看得见哪些 Mod”。
- 内存：1.7 只开 Core 时峰值工作集约 1.9 GB、稳定在 1.4 GB；1.6 约 0.36 GB（加载失败的情况）。读进程的 `PeakWorkingSetSize` 很便宜，可以放进报告。

## 3. 结论（设计见 08 §12）

自己写，不用工具 Mod：隔离数据目录 + 按版本选日志参数（1.6 `-logfile`，1.7 `-NoFileLog` + 接管 stdout）+ `Knowledge.xml` 作主菜单标志 + 超时/提前退出判定 + 杀进程 + 用现有 `internal/logs` 和 `App.AnalyzeLog` 出错误簇与归因。游戏目录和玩家数据一律不写，所以游戏看不见的 Mod（需要链接的）只报出来，不建链接。

## 4. 真机运行记录（`archo test run`，2026-10-24）

RimArcho 用临时的 `RIMARCHO_HOME` 和临时用户数据目录（不碰玩家的 RimArcho 数据与游戏配置），每次运行前后对比玩家 LocalLow 下 `Logs/*`、`Config/ModsConfig.xml`、`Config/Prefs.xml` 的修改时间：均未变化。游戏目录里的 `_Archo_*` 链接数不变（没建链接）。

| 游戏 | 方案 | 结果 | 加载 / 总时长 | 峰值内存 | 错误簇 | 退出码 |
| --- | --- | --- | --- | --- | --- | --- |
| 1.7.4985 rev1269（Steam 副本） | Harmony、Core、5 个 DLC、JobsLimitsTweak（8 个） | `menu` | 31.5 s / 37.0 s（首次，建缓存）；13.5 s / 19.3 s（复用缓存） | 2188 / 2011 MB | 0（只有 mod-report 3、other 6） | 0 |
| 1.6.4871 rev590（`D:\rimworld16`） | Core（1 个） | `menu`（标志写出，但主菜单初始化已失败） | 11.5 s / 17.1 s | 362 MB | 6 个 exception 簇：`ModsConfig`/`ModLister` 类型初始化失败（悬空链接，§2.4），之后 `Root level exception` 每帧刷 | 1 |

注：1.6 那次“到了主菜单标志”但带着致命错误，所以退出码 1 由错误簇决定，不由状态决定（08 §12）。1.7 控制台日志里中文 Mod 名显示为 `?`（系统 ANSI 代码页，08 §12 已知限制）。

## 5. 后续

- `D:\rimworld16\Mods` 的悬空 `_Archo_env_*` 链接：链接维护应清理或跳过悬空链接（08 §3）。
- 整合包 1.7 的日志位置假设错误（§2.1）：整合包启动 1.7 也应加 `-NoFileLog` 并接管输出，或在报告里改读共用位置（08 §5.3）。
- 1.6 自 03:26 起玩家 `Config/Prefs.xml` 修改时间变了一次，发生在第一次 1.6 探测期间（该次 `-logFile` 参数带了多余引号）；第二次 1.6 探测和两次正式运行都没改它。原因未查明（也可能是并行的其他会话）。
- MCP `test_run` 运行期间持有 MCP 服务器的锁，其它工具调用要等它结束。
- GUI 入口未做。
