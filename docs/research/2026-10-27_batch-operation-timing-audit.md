# 批量操作耗时审计（2026-10-27）

结论：**并非所有 batch action 都慢；最严重的是文件/订阅操作在确认窗出现前同步逐项预检，部分预检还占着 GUI 共用锁。** 真实库存取消订阅 100 项等了 11.14 秒才出现确认窗，1,293 项等了 98.57 秒。方案编辑和任务记录操作的数据库修改通常很快，但方案编辑后的全量刷新仍可达到秒级。“全部更新”存在另一条逐项预览链，135 项需 39.58 秒。

## 1. 范围、机器与安全边界

- 主 checkout：`D:\Game\RimModManager_Upgrade\RimArcho`，分支 `dev/0.9.0`；开始时 HEAD `887b668`。已读 `../AGENTS.md`、`docs/00a-architecture.md`、`docs/specs/06-ui/` 的总览、分层、交互、GUI 技术和相关页面规格。
- 实机 Windows，AMD Ryzen 9 7950X（32 逻辑处理器）；Chrome `C:/Program Files/Google/Chrome/Application/chrome.exe`，headless=new，1600×1000，独立临时 profile；Node 原生 WebSocket 驱动 CDP。参考 `scripts/check-accounts-page.cjs`，**没有使用 `/dev/pagecheck` 的离线数据**。
- `%APPDATA%\RimArcho\archo.db*`、`config.toml` 复制到专用系统临时目录；另复制真实 `tasks.json`。DB 主文件 357,650,432 字节，副本 `PRAGMA integrity_check=ok`。方案编辑使用该副本的 SQLite backup，互不污染。
- 真实库存：5,183 个 lineage、6,573 份实例；当前方案原有 715 项，其中 570 份已安装非官方项可用于编辑基准；未启用工坊样本池 2,244。任务历史起初 36 条，真实重试/来源检查后最多 40 条，未伪造任务凑 100 条。
- 首先构建 `./cmd/archo`，以用户指定的 `go run ./spikes/guidev -home <副本> -exe <archo.exe>` 启动真实页面。随后为计时用 Go `-overlay` 构建相同 harness；overlay、计时日志、诊断 JS、二进制均只在临时目录，checkout 业务代码未改。
- 实际 Mod 路径仍指向本机内容，用于只读目录、链接、PNG 和进程检查。取消订阅、删除、重新订阅、锁定/搬移/更新、DDS 转换/清理**均未确认执行**；只测 dry-run/确认窗。没有操作任何真实 Steam 订阅或 Mod 文件，也没有使用 `I Dont Need It` 标签作真实执行。真实执行用过的 Mod：**无**；需重新订阅/回收站恢复：**无**。
- 写入仅涉及临时 DB、任务记录、标签、临时方案和临时导出文件。来源检查确实访问真实 Steam 公共查询接口，仅更新副本中的来源元数据；没有下载更新。临时标签已清除，编辑大多逐轮撤销，最后丢弃整个副本。
- 研究期间其他 worker 将主分支推进到 `d64c974`（连续 space 编辑刷新修复）。主要页面 assets 冻结于起始版本，后续 harness 构建读到了当时的 Go 文件。本文是起始实现的审计，**不是 d64c974 或 background-nonblocking 分支的修后验收**；不能据此断言旧刷新问题在最新代码仍存在。结束时保留其他 worker 的变动和报告。

## 2. 测量口径

- “首次反馈”是触发原按钮/键盘处理函数后，MutationObserver 观察到相关 DOM 改变的下一次 requestAnimationFrame；是下一绘制机会，非显示器呈现时间。方案操作出现“加载中”也计为反馈，**不代表修改已完成**。
- 对有确认窗的危险操作，表中总耗时是“点击→预览完成”，确认→真实执行完成统一为“未执行”。不能把 dry-run 时间当成下载、复制、回收站或 Steam 实际执行时间。
- 普通编辑总耗时以原处理函数及刷新 Promise 返回为终点；API 单独计时另标。部分页面先结束 Promise、下一帧才可见，因此小操作可能首次反馈数值略大于处理函数时间。
- 每类取真实可用条目前 10、前 100、全部 eligible；不同操作的“全选”数量不同，不是同样工作量排名。无适用实例/排队状态标 N/A，不制造假 Mod。单轮观测，少量复测；未做严格 OS 冷/热缓存隔离，结果不可当作 P95 或独立性能对照实验。
- 诊断 JS 仅导出原模块内部入口、设置原选择集合并触发原处理函数；未替换业务 API 或算法。来源页原本没有多选待更新行，10/100 项使用真实 pending 数组切片后走原 `changesKey(U)`；135 项点击原“全部更新”。导入和缺失项 10/100 使用原 API，单独标注；移动方向/步数的原处理函数及过滤移动API另有补测；未把API单独返回时间冒充整页完成。
- 响应探针：等待期间按方向键看光标下一帧、10ms timer 的最大间隔、请求 `API.Setting('ui.lang')`，并实切任务页/库存页。**JS 可以动但 API 一直等锁，也算界面操作受阻。** guidev 的 HTTP/1 连接队列会放大部分切页等待；Wails 原生桥接没有同样的每站 6 连接限制，故切页绝对值不直接外推。
- Go overlay 只在函数入口记 time.Now、defer 输出 elapsed；记录 handler、命令派发、索引/Loadout、链接/进程检查、来源筛选、网络、SetTags 等。父子函数时间重叠，**禁止把嵌套时间相加当总成本**。计时开销和日志 IO 未做扣除。

## 3. 批量入口清单与覆盖

清单来源：`assets/js/{library,lists,main,ops,planmanage,subscriptions,tasks,textures,exchange,screens,credit}.js`，`internal/view/ops.go` 的 opTable，以及 GUI API 到 app/store 的调用链。

| 入口/操作 | 前端与 Go API | 实测覆盖或边界 |
|---|---|---|
| 多选启用/停用、暂存项放回 | main.verb / Toggle | 10、100、570；未启用另取 10/100/2244，后两档被真实重复 packageId 拒绝 |
| 启用到指定位置、列表内移到指定位置、拖入启用列 | main.place/drop / Place | 位置窗、列表内Place、从未启用列启用到指定位置10/100/570；后一项先停用原来合法项，再启用回去，逐轮撤销 |
| 上/下移、上/下 10、过滤后移动、同列拖动 | MoveBy / MoveShown / Place | 上移/下移/上10/下10、指定位置10/100/570；MoveShown原API另测10/100/570，显示位置从真实列表隔行抽取 |
| 多选放入暂存、放回、拖出暂存 | Stage / Unstage / Toggle | 已启用 10/100/570、未启用 10/100/2244；实际副本 DB 修改 |
| 全部暂存、全部放回 | StageAll / RestoreAll | 全方案 715 项中可暂存 570 项；复测 3 轮 |
| 多选标签添加/移除 | pop.tags / TagCandidates / SetTag | 编辑器 10/100/570，库存 10/100/6573；添加后移除临时标签 |
| 从所有方案移除 | RunOp('remove') / RemoveFromPlans | 10/100/570；只修改副本方案，提示涉及 3–5 个方案 |
| 整方案排序 | main.run / Run('sort') | 715 项原方案，原处理函数含刷新；不是逐项事务 |
| 锁定这个版本 | LibraryPreview/LibraryDo('pin') | dry-run 10/100/6559 |
| 更新至工坊版本 | 同上 'pin-update' | dry-run 10/100/149，与来源更新是不同入口 |
| 解除锁定 | 同上 'unpin' | N/A：真实 eligible=0；未为测试创建真实锁定副本 |
| 移入锁定库 | 同上 'move' | dry-run 10/100/149 |
| 取消隐藏 | 同上 'unhide' | dry-run 10/13；100 N/A |
| 取消订阅、重新订阅 | 同上 'unsubscribe'/'resubscribe' | dry-run 10/100/1293 或 1294 |
| 删除到回收站 | 同上 'delete' | dry-run 10/100/3760 |
| 撤掉订阅来源 | 同上 'subscription-remove' | dry-run 10/100/1503；源码中与单纯 Steam 退订不同 |
| 多选来源检查/来源更新 | 'subscription-check'/'subscription-update' | 初始无 eligible；来源检查后出现 135 待更新，更新预览经来源页测 10/100/135；库存该入口未独立复测 |
| 来源页“检查更新”/单来源“检查” | checkAll / CheckSubscriptionSource | 真实全量 1294+1503；未独立重跑 10/100 网络子集，单来源耗时由两个真实任务和 Go 日志拆出 |
| 来源页“全部更新” | updateRows / WorkshopUpdatePreview 或 LibraryPreview | 10/100 真实待更新样本、全部135；本次全部为 SteamCMD，workshop 更新分支没有待更新样本 |
| 导入页/合集缺失项/诊断页“全部订阅” | exchange.getAll、screens.runAction / Subscribe(batch=true) | 原选择窗 10/100/570；不选 Steam/SteamCMD 执行；诊断页真实有91个可订阅缺失项，未发起订阅 |
| 诊断页整组启用/停用、移除缺失项 | screens.doAction / DoAction | 真实缺失组移除10/100/143（API测量）；依赖批量启用无适用组；与编辑器共用方案编辑层 |
| 诊断页全部修复部署链接/移除整合包链接 | DoAction repair-links / unlink-packs | 无适用批量卡片；仅代码追踪，未写真实 Mods 目录 |
| 贴图统计（当前方案/全库） | textures.count / Textures、TextureMeasure | 方案真实目标8/98/570，全库6573；全量实际读取本机 PNG |
| 批量转换 DDS、清除 DDS | TexturesRun / TexturesClean；库存 textures-clean | 方案/全库确认窗；未确认；库存 textures/clean eligible=0，不制造 DDS。转换需要真实文件写，不能用统计时间替代 |
| 整方案导出8种格式、保存/复制 | PlanExport / PlanExportSave / PlanExportCopy | 真实10/100/715项，原页面逐格式按钮；保存到临时目录 API 实测；复制未触碰用户剪贴板，仅代码追踪 |
| 导入为新方案、合并/替换整方案 | ImportPreviewFor / ImportAsPlanPreview / ImportApplyTo | 真实原方案导出切片10/100，预览和新方案API实测；合并真实已安装10/100/570，替换10/100/715，核验实际条目变化 |
| 合集追踪“跟上”同步新增/删除 | PlanSync / PlanFollow | 无合适的真实待同步合集状态；未另造外部合集；仅代码追踪 |
| 任务多选停止/取消、重试、继续、队列前移/后移 | tasks.run / TaskAct | forget/undo/clear真测；retry选3个真实失败社区数据任务；stop/resume/up/down无合法批量活跃样本，未重试订阅类任务 |
| 任务多选移除、清理已完成、撤销 | TaskAct forget/clear/undo | 10/全部40，clear29；100 N/A |
| SteamCMD 批次补 Steam 订阅/替换 | credit.js / CreditAnswer(batchIDs) | 识别为批次入口；没有可安全确认的到期样本，仅审阅，未执行 |
| 全范围重新扫描、分析、编译整合包；整方案保存/启动 | Scan/Analyze/Build/Run | 属于范围级任务，扫描/构建/启动有独立真实 IO 语义，本轮仅追踪调度/锁，不额外写真实游戏文件，不宣称执行耗时覆盖 |

备注、切换版本/指定某份副本、取消副本归类、打开目录/网页等在 opTable 中 `One=true`，**不是多选批量动作**。库存页也不提供方案启用/停用入口；这些在编辑器完成。

## 4. 实测：确认前预览

单位秒，每格为 **首次反馈 / 预览返回**。每格均真实操作本机内容；所有确认窗关闭，确认→执行完成均未测。全选只取该操作允许的 eligible，非盲目把6573份实例全提交。

| 操作 | 10项 | 100项 | 全选（数量） | 等待时界面 |
|---|---:|---:|---:|---|
| 取消订阅 | 1.236 / 1.240 | 11.140 / 11.148 | 98.570 / 98.585（1293） | 按键可动；共用API锁排队 |
| 重新订阅 | 0.363 / 0.367 | 3.176 / 3.182 | 39.987 / 40.003（1294） | 按键可动；共用API锁排队 |
| 锁定版本 | 0.123 / 0.131 | 0.776 / 0.780 | 111.844 / 111.909（6559） | 按键可动；共用API锁排队 |
| 更新至工坊版本 | 1.308 / 1.311 | 11.565 / 11.570 | 22.722 / 22.727（149） | 按键可动；共用API锁排队 |
| 移入锁定库 | 0.900 / 0.907 | 8.751 / 8.759 | 36.092 / 36.106（149） | 按键可动；共用API锁排队 |
| 取消隐藏 | 0.131 / 0.150 | N/A | 0.127 / 0.133（13） | 按键可动；共用API锁排队 |
| 撤掉订阅来源 | 4.325 / 4.330 | 14.114 / 14.118 | 121.034 / 121.045（1503） | 按键可动；Setting不长期阻塞 |
| 删除到回收站 | 0.643 / 0.647 | 6.208 / 6.214 | 234.330 / 234.341（3760） | 按键可动；共用API锁排队 |

取消订阅10项复测为0.679/0.683秒，重新订阅10项复测0.341/0.346秒。冷暖缓存、样本大小/目录形态会影响斜率，不能简单按10项线性外推全库。

来源页“全部更新”另列：

| 待更新数量 | 首次确认窗反馈 | 预览返回 | Setting响应 | 真实更新 |
|---:|---:|---:|---:|---|
| 10 | 3.127s | 3.133s | 1.7ms | 未确认 |
| 100 | 32.107s | 32.114s | 1.4ms | 未确认 |
| 135 | 39.583s | 39.590s | 2.5ms | 未确认 |

### 4.1 界面响应证据

全选退订、重订、锁定、更新至工坊、移入锁定库、删除期间，Setting 请求分别等待98.49、39.90、111.21、22.64、36.03、234.18秒，与预检几乎重合。键盘光标反馈多在17–39ms，timer最大间隔通常低于140ms，证明主要不是 JS 一直占满，而是后端 API 锁阻塞。

删除全选等待中实切任务页，数据就绪等了120.56秒；再切库存页骨架约24ms、就绪约331ms。Task 列表读取本身并不全程持 `s.mu`，不能用该值误判 Tasks()；页面 Setting/Leave 及 harness HTTP 连接队列也参与等待。**锁的直接依据是 LibraryPreview 代码与独立 Setting 探针。**

撤掉订阅来源和全部更新已用 detached reader：全选 Setting分别16.4ms、2.5ms；但点击后没有“正在预检”状态，用户依然要盲等121秒/39.58秒。释放锁只能改善期间可操作性，不能消除逐项重复工作。

## 5. 实测：方案编辑、标签与诊断动作

### 5.1 多选编辑

下面采用补验证轮（核验 API 成功结果），单位 **首次反馈ms / 处理及刷新完成s**。初轮这些操作总耗时约0.88–1.27s；补验证轮为1.84–4.36s，说明全量刷新/缓存状态对耗时影响明显，不是稳定的每Mod线性成本。

| 操作 | 10项 | 100项 | 全选570项 | 结果与响应 |
|---|---:|---:|---:|---|
| 停用 | 62.2 / 1.886 | 46.9 / 2.205 | 314.0 / 4.358 | 成功；可响应，随后刷新 |
| 重新启用刚停用的项 | 61.9 / 2.071 | 62.0 / 2.702 | 62.1 / 3.357 | 成功；可响应，随后刷新 |
| 放入暂存 | 103.1 / 2.069 | 115.7 / 3.315 | 85.1 / 2.473 | 成功；可响应，随后刷新 |
| 从暂存放回 | 86.5 / 1.840 | 91.3 / 4.319 | 63.2 / 2.223 | 成功；可响应，随后刷新 |

全选停用补验证首次反馈314ms，超过200ms目标。其 Toggle API只有22.4ms，后续 BarFilter为2.056s。不能把整页4.36s归因到停用数据库事务。

| 其他操作 | 10项 首次ms/总s | 100项 首次ms/总s | 全选 首次ms/总s | 备注 |
|---|---:|---:|---:|---|
| 下移 | 107.0/1.078 | 102.2/1.085 | 110.4/1.130（570） | 初轮；MoveBy |
| 打开位置窗 | 10.1/0.010 | 11.1/0.011 | 7.1/0.007（570） | 本地弹窗 |
| 位置窗应用 | 67.4/1.004 | 72.8/1.048 | 70.5/0.993（570） | Place |
| 未启用项放入暂存 | 46.9/1.288 | 44.3/2.904 | 57.7/1.231（2244） | Stage |
| 从暂存拖回未启用 | 50.1/0.999 | 44.1/0.984 | **539.9/1.883（2244）** | 全量本地更新/绘制，timer gap533ms |
| 全部暂存 | — | — | 100–106/0.835–0.925 | 方案715项中实际570项 |
| 全部放回 | — | — | 13–15/0.943–1.219 | 570项 |
| 在所有方案停用 | 52.5/0.045 | 46.5/0.046 | 50.5/0.043（570） | RunOp处理时间，不含后续完整页就绪 |
| 整方案排序 | — | — | 总1.471（715） | 原Run处理与刷新完成；未单独记首次反馈 |


补测移动/启用位置变体：每轮使用原570个合法已启用项的子集，改完撤销。单位首次ms/处理及刷新s；MoveShown标明API时间。

| 操作 | 10项 | 100项 | 全选570项 |
|---|---:|---:|---:|
| 上移1位 | 21.9/1.133 | 13.5/1.150 | 17.2/1.270 |
| 上移10位 | 12.4/1.126 | 13.5/2.088 | 12.1/1.092 |
| 下移10位 | 12.3/1.304 | 10.8/1.277 | 12.2/1.050 |
| 已停用合法项启用到第20位 | 30.0/1.122 | 30.9/1.098 | 56.8/1.102 |
| 过滤后MoveShown（仅API） | 43.2ms | 47.2ms | 46.3ms |

这些补测未出现错误，移动API返回了对应数量和新位置。API单独测试没有切页/按键长等待窗口，不能由几十毫秒时间推导更大规模下绝无阻塞。

未启用池的启用：10项真实成功，Toggle156.9ms，整页2.032s；100/2244项分别在98.2/100.4ms被 `pkg.duplicate-installed` 拒绝（`[NL] Facial Animation - Experimentals` 的另一份副本获胜）。拒绝后页面结束为0.942/0.968s，**这两项不是启用成功的性能成绩**。

### 5.2 标签

| 范围/数量 | 标签窗首次ms | 提交首次ms | SetTags/SetTag耗时 | 提交至写完成 | 页面刷新观察 |
|---|---:|---:|---:|---:|---|
| 编辑器10 | 18.8 | 15.3 | Go SetTags 1.0ms | 原处理含刷新1.208s | 可响应 |
| 编辑器100 | 11.6 | 26.0 | Go SetTags 3.7ms | 原处理含刷新2.227s | 可响应 |
| 编辑器570 | 338.6 | **2206.0** | Go SetTags **2176.8ms** | 原处理含刷新3.840s | Setting约2146ms，持锁等待 |
| 库存10 | 33.6 | 33.7 | SetTag API11.7ms | 46.4ms | 验证全页就绪1.622s |
| 库存100 | 240.4 | 20.7 | SetTag API8.3ms | 33.2ms | 验证全页就绪1.497s |
| 库存6573 | 214.6 | 296.5 | SetTag API281.9ms | 300.0ms | 验证全页就绪1.926s |

库存全选按lineage去重，最多写5183个家族的标签。两轮结果有明显状态/SQLite写入差异，**不能声称标签始终低于300ms**。移除全库临时标签的Go SetTags另测1.721s，也暴露逐条autocommit的波动。表中库存“验证全页就绪”包含计时脚本额外 `Library.open()`，与原 `load()`可能重叠；不是严格的原产品刷新终点。单次 Library重读约1.25–1.46s。

### 5.3 诊断页批量从方案移除缺失项

真实缺失组143项。10/100真实refs切片调用原DoAction，再刷新，首次63.5/55.8ms，总2.120/1.981s；143项原DoAction直接返回并显示结果16.3ms（提示“已从列表中移除143个Mod”），未计整页刷新。每轮撤销。未将API时间与完整页面时间混用；全量原卡片按钮未展开，未声称已点该按钮。

## 6. 实测：后台统计/网络、任务与导出

### 6.1 来源检查和贴图

| 操作 | 实际数量 | 首次反馈ms | 完整耗时s | 期间响应/结果 |
|---|---:|---:|---:|---|
| 来源页检查全部 | 工坊1294 + SteamCMD1503 | 17.7（排队ack11.6） | **219.480** | 任务页52.4ms打开；两任务结束但各有9/14条查询失败 |
| 贴图统计：小方案 | 8份非官方Mod | 14.3 | 5.041 | Setting2.2ms，timer gap低于51ms |
| 贴图统计：中方案 | 98份非官方Mod | 12.7 | 4.087 | Setting2.4ms，可响应 |
| 贴图统计：原方案 | 570份非官方Mod | 12.2 | 39.015 | Setting14.3ms，可响应 |
| 贴图统计：全库存 | 6573份实例 | 25.4 | **179.634** | Setting53.1ms；timer最大gap902.9ms |
| 贴图转换确认窗 | 上述8/98/570目标 | 19/27/48 | 只测确认窗 | 未生成DDS |
| 清除DDS确认窗 | 同范围 | 16/18/23 | 只测确认窗 | 未删DDS |

贴图10/100的构造保留10/100个已安装项，其中2个官方项不进入贴图目标，所以有效数是8/98，不能虚报成10/100。统计全库读出1870个有PNG的结果行、281462个PNG、约8.9GB，241048个待转换；估计DDS需约49.7GB，实际转换不合理，也未获准对普通Mod执行。副本未携带原统计cache，先小后大，已有OS缓存逐步变热；98项比8项快不代表工作量越大越快。

全来源检查的两个任务约100秒、118秒，Go `fetchWorkshop`真实执行2797次，累计216.416秒；两次 `CheckSubscriptionSource` 累计218.818秒。部分失败是真实结果，并未用假响应代替。检查后副本出现135个真实待更新SteamCMD记录，供第4节预览测试。

### 6.2 任务页与全部订阅选择窗

| 操作 | 数量 | 首次/返回或完成ms | 结果 |
|---|---:|---:|---|
| 任务多选移除 | 10 | 22.5 | 实际少10条 |
| 移除撤销 | 10 | 20.0 | 恢复10条 |
| 任务全选移除 | 40 | 33.1 | 实际少40条，Task API返回空记录 |
| 移除撤销 | 40 | 16.7 | 恢复40条 |
| 清理已完成 | 29 | 31.4 | 完成记录移除 |
| 清理撤销 | 29 | 41.7 | 恢复 |
| 多选失败任务重试 | 3个data-update | 35.4排队 | **只新增1个任务** |
| 全部订阅：方法选择窗 | 10/100/570 | 14.5/21.0/23.7 | 仅弹窗，未选择执行 |

移除和撤销很快。`TaskAct` 的 retry/resume/up/down只读 `ids[0]`，因此“多选重试快”不能证明批量吞吐好，是漏处理其他选项。stop遍历ids，forget批量Forget，clear清理结束记录。没有制造100条历史，也没有重试真实订阅操作。

补充边界：空任务记录在harness返回 `rows:null` 时，tasks.js drawList 的 `X.page.rows.length` 抛错；通过原TaskAct(undo)恢复了40条。属于空态兼容问题，和批量性能分开处理。

### 6.3 导入/导出

用原715项真实方案的RimArcho导出JSON截取前10/100项，原ImportPreviewFor + ImportAsPlanPreview写入临时方案；导出后重新解析entries核验10/100，没有伪造Mod。715项仍用原方案。部分原方案条目确实缺失，导出本来就支持它们。

| 数量 | 导入预览API | 导入为新方案API | 导出页首次反馈 | 导出页API/处理返回 | 逐格式切换完成范围 | 保存临时文件API |
|---:|---:|---:|---:|---:|---:|---:|
| 10 | 4.1ms | 1.9ms | 21.2ms | 51.4ms | 40.0–51.6ms | 4.4ms |
| 100 | 11.5ms | 2.7ms | 16.6ms | 41.4ms | 43.9–55.0ms | 3.8ms |
| 715 | — | — | 25.3ms | 6.1ms | 44.8–98.2ms | 7.4ms |

8种格式：RimArcho、RimSort、RML、RimCrow XML、分享码、纯文本、ModsConfig、工坊链接。切换点击真实 `.fmt` 按钮，以预览DOM替换为终点，没有算入人为100ms等待。首反馈可能只是格式按钮选中态，表中同时提供完整替换时间。导出页715项处理返回6.1ms、下一帧25.3ms，是终点差异，并非负开销。保存只写专用临时目录，未覆盖游戏ModsConfig；API返回时间从提交路径起算，不含用户选文件的时间。Clipboard、原生系统文件选择窗及合集同步未独立端到端实测，见清单。合并/替换另测如下，属于页面原API计时，不包括表单填写/解析预览/后续刷新。


| 导入应用方式 | 输入数量 | 应用前→应用后条目 | 原API返回ms |
|---|---:|---:|---:|
| 合并真实已安装项 | 10 | 0→10 | 1.7 |
| 合并真实已安装项 | 100 | 0→100 | 1.5 |
| 合并真实已安装项 | 570 | 0→570 | 2.5 |
| 替换（保留真实缺失条目） | 10 | 2→10 | 1.6 |
| 替换（保留真实缺失条目） | 100 | 2→100 | 1.5 |
| 替换（保留真实缺失条目） | 715 | 2→715 | 3.0 |

补测发现：合并会跳过未安装项，原715项切片前10/100实际仅合入2个官方项；因此这两条作废，改用已安装工坊项10/100/570，实际新增数量逐条核验。替换操作支持缺失项，保持原真实10/100/715条目，测试前目标只有2项，避免no-op。以上均只改临时方案，无游戏文件写入。

## 7. 慢操作排序

按各自真实全量范围的步骤结束时间排列，**不是等数量吞吐排名**。前两种“预览”没有执行真实删除/退订。

| 排名 | 操作/阶段 | 数量 | 完成时间 | 首次反馈 | 性质 |
|---:|---|---:|---:|---:|---|
| 1 | 删除到回收站：预览 | 3760 | 234.34s | 234.33s | 无状态、持共用锁 |
| 2 | 检查全部来源 | 2797 | 219.48s | 17.7ms | 有反馈后台网络，两来源部分失败 |
| 3 | 全库贴图统计 | 6573 | 179.63s | 25.4ms | 有反馈后台IO，大表有绘制停顿 |
| 4 | 撤掉订阅来源：预览 | 1503 | 121.04s | 121.03s | 无状态，已释放共用锁 |
| 5 | 锁定版本：预览 | 6559 | 111.91s | 111.84s | 无状态、持共用锁，扫描真实树 |
| 6 | 取消订阅：预览 | 1293 | 98.59s | 98.57s | 无状态、持共用锁 |
| 7 | 重新订阅：预览 | 1294 | 40.00s | 39.99s | 无状态、重复进程检查 |
| 8 | 全部更新：预览 | 135 | 39.59s | 39.58s | 前端逐项请求，无状态 |
| 9 | 移入锁定库：预览 | 149 | 36.11s | 36.09s | 无状态、持共用锁 |
| 10 | 更新至工坊版本：预览 | 149 | 22.73s | 22.72s | 无状态、持共用锁 |

同为100项的预览首次反馈：全部更新32.11s > 撤掉订阅来源14.11s > 更新至工坊版本11.57s > 取消订阅11.14s > 移入锁定库8.75s > 删除6.21s > 重新订阅3.18s > 锁定0.78s。小规模已超200ms，不只全库才慢。

非预览还需处理：编辑器标签570项提交首次2.206s、SetTags2.177s；库存全选标签首次297ms，移除标签Go1.721s；拖出暂存2244项首次540ms；全选停用补验证314ms；贴图全库统计timer gap903ms。大多数普通编辑虽很快出现状态，完整刷新仍0.9–4.4s。

## 8. Go handler拆分与根因

### 8.1 逐项预检把全局工作重复N次（主要根因）

调用链：`library.js:run` → `gui/library.go:LibraryPreview` → `view/library.go:LibPreview`。LibPreview遍历refs，每项调用一次 `cmds.Execute(... --dry-run)`，再调用 `usedWarning`。不是一个真正的批量预检。

| 实测 | handler总ms | 被重复执行的部分（次数 / 累计ms） |
|---|---:|---|
| 退订10（复测） | 663.5 | readIncomingLinks 10/334.0；GameRunning 10/291.4；usedBy 20/14.3；Execute 10/650.0 |
| 重订10（复测） | 324.8 | GameRunning 10/307.8 |
| 删除100 | 6191.2 | readIncomingLinks 100/2952.8；planIncomingLinks 100/2921.3；usedBy 200/148.7 |
| 删除3760 | 234229.8 | readIncomingLinks 3760/104689.9；planIncomingLinks 3760/106642.8；usedBy 7520/6810.6；Execute 3760/224150.9 |
| 移入锁定库100 | 8724.2 | readIncomingLinks 100/3865.3；planIncomingLinks 100/3418.8；usedBy 100/137.5 |
| 更新至工坊100 | 11544.6 | readIncomingLinks 100/3126.5；planIncomingLinks 100/2886.9；其余含copyTree只读扫描 |
| 撤掉来源1503 | 120953.4 | readIncomingLinks 1503/51025.8；usedBy 3006/2475.7；Execute 1503/118396.4；loadIndex一次209.2 |
| 锁定6559 | 111281.5 | PinFor 6559/91072.3（单项最大7851.1）；Execute 6559/95482.0 |

具体定位（行号以审计时主checkout为参考）：

- `internal/app/linklifecycle.go:51 readIncomingLinks`：每项重新 InspectGameLinks、枚举各游戏Mods链接、规范化路径；不是只查这一个ref。`planIncomingLinks:177` 每次新建probe，重复检查游戏进程。
- `internal/app/gamerun.go:49 GameRunning`：probe缓存只在本次闭包内复用，跨Mod不共享；Windows进程枚举约30ms/次。重新订阅10项里约95%的handler时间就是这一步。
- `internal/app/workshopsub.go:Workshop`：预检每项GameRunning；退订还查使用引用和链接。`libraryact.go:252 usedBy` → `library.go:614 libUses` 重建全部方案/整合包引用；`usedWarning`再问一次，形成每Mod两次引用查询。
- `internal/app/pin.go:119 PinFor` → `copylocal.go:copyLocal/copyTree`：dry-run也枚举真实目录树，6559项目录大小极不均匀。不是数据库慢就能解释。确认内容19677行，handler返回到首次反馈另约563ms，包含桥接/传输/文本绘制，不能全归为JS冻结。

建议一次BatchPreviewContext读取 Index/Loadout、全局引用索引、链接快照、进程快照；每Mod只做关联判断和必要的自身目录扫描，最后统一构造确认结果。**执行前须重新校验进程/链接/目标身份**，不能永久缓存安全结论。删除全选约211秒耗在重复链接/进程相关步骤，改N次为1次可去掉其中大部分重复成本；目录树扫描和真正文件IO不会消失。此为结构性收益估计，未实现/未验收加速比例。

### 8.2 确认窗太晚与GUI锁（与background分支直接重叠）

`assets/js/library.js:600 run` 在 `await LibraryPreview` 后才 pop.confirm，没有预检中的可见反馈。`gui/library.go:117 LibraryPreview` 只有subscription-update/remove走独立ReadSession，其余全程 `s.mu`。因此用户先等预览，其他需要该锁的API也一起停住。

建议点击立即展示可关闭的“正在检查N项”，禁用确认、带请求身份防止旧结果覆盖；只读预检使用独立会话，短锁抓快照后释放。目标首次反馈<200ms，但**仅弹窗/释放锁不会缩短实际预检时间**，需和8.1合做。

### 8.3 “全部更新”前端把批量拆成单项（独立于库存预检入口）

`assets/js/subscriptions.js:459 updateRows` 第467行for循环逐项await预览，第472行确认后也逐项发WorkshopUpdate/LibraryDo。SteamCMD预览每项又建ReadSession → Loadout/loadIndex → `subscribe.go:271 selectSubscriptions` → 全量Subscriptions后筛单个ref。

100项实测100次LibraryPreview累计31.924s，其中loadIndex100次17.293s；Loadout累计22.978s（包含索引，不能相加）；Subscriptions100次4.635s、selectSubscriptions4.788s（包含前者）；GameRunning100次3.232s。135项handler累计39.356s，loadIndex135次21.412s，selectSubscriptions135次6.005s，GameRunning4.172s。耗时与页面39.59s吻合。

建议一次发送全部真实refs，在一个读取上下文里批量预览；确认后一次提交批任务，保留app已有按root/account分组的SteamCMD下载能力。当前前端单项submit会破坏聚合机会。预期可将索引/来源筛选从100/135次降为1次；网络和复制成本仍需独立计时。workshop待更新分支本轮无样本，只做源码判断。

### 8.4 来源检查每Mod一请求、跨来源重复

`internal/app/subscriptionupdates.go:43 CheckSubscriptionSource` 每条调用 `describe.go:223 fetchWorkshop`，公开GetPublishedFileDetails请求 `itemcount=1`。本机两来源2797次查询耗216.4s，且相同WorkshopID跨工坊/SteamCMD再次查询。checkAll只排两个来源任务，两个self任务实际串行。

建议使用该公开接口多item参数，按合理大小（如100条）分组，跨来源先去重WorkshopID，有限并发、逐项错误/重试和进度。2797条未去重请求理论上先降到约28批；这是请求数估计，不保证219秒按同倍数下降。不要为减少请求引入Steam凭据抓取或非公开接口。

### 8.5 标签逐条autocommit，轻修改后重刷新

`app/playerdata.go:15 SetTags` 按lineage去重后循环，`store/playerdata.go:73 AddTag/:83 RemoveTag` 每条独立DB.Exec，没有覆盖整批的事务。实测编辑器570标签2.177s，全库移除1.721s；库存全量添加282ms，写入状态会改变成本。

建议一个短事务、复用statement，保留去重/逐项错误语义，批末失效一次缓存。`gui/api.go:SetTag` 仍持s.mu，减少事务/写入时长也能改善期间锁等待。验收用新增和移除两种真实标签，不能只测已有标签的no-op。

编辑器 `main.js:edited/reload` → `lists.js:openEditor/refilter` 的全量重读，与 `gui/ops.go:15 BarFilter` 新会话/board重建，是另一大固定成本。补验证多轮Toggle/Stage仅9–218ms，Workspace2–20ms，BarFilter却806–2095ms。建议对已知编辑结果做局部状态更新，只重算受影响列/筛选；缓存必须依赖真实revision，不把过期数据当新状态。d64c974已调整旧刷新体验，后续优化需在该提交上重新测，本文不重复判为未修复。

`main.js moveLocal/渲染列表`处理2244项拖出暂存，timer gap533ms，属于前端计算/绘制问题；需要profile区分selection查找、行更新和布局，再用Set/Map索引、批量DOM更新。不能把这一次gap直接归因到某一行循环。

### 8.6 真正执行阶段的静态风险（没有冒充实测）

`gui/tasks.go:371 taskBatch`已有优化：delete/unhide走batchFiles，实际文件处理后统一Scan；Steam退订/重订走batchSteam，**50项一组调用Steam**。不能报成“每个Mod启动一次Steam/SteamCMD”。仍有预检查循环和扫描持s.mu风险；其他pin/move经taskItem逐项LibDo，`pin.go:makeLock`每项 `a.Scan(false)`；来源移除 `subscribe.go:494 RemoveSubscriptions`逐项扫描，结束再扫描。

建议批量准备→逐项文件修改→一次扫描/身份对账，并保留中断/失败项进度；实际下载/复制/回收站吞吐需要另在允许的 `I Dont Need It` 集合中做可恢复执行验收。本轮没有该阶段的秒数或修后收益数据。

### 8.7 贴图统计和大结果表

textures.js count已有即时状态、后台扫描与轮询，本轮不属于确认前无响应。但6573实例真实PNG统计需要约180s；结果表1870行非虚拟渲染，最后/增量阶段timer最大gap903ms。应对目录统计做带文件指纹的缓存/批次复用，并对结果用分片或虚拟表格；需要Performance trace验证具体长任务后再定，不能仅靠timer gap认定draw是唯一根因。

### 8.8 任务批量重试实际只处理第一个

`gui/tasks.go:165 TaskAct` retry/resume/up/down使用 `ids[0]`，而前端把多选ids全部传入。补齐所有eligible任务的一次批量提交/落盘，或对单项动作明确只允许一个选择；前移/后移需定义多项相对顺序。此为功能遗漏，应单独提issue，不以毫秒成绩掩盖。空态rows=null另需归一化空数组。

## 9. 与 codex/background-nonblocking 的重叠

在研究结束前只读查看了 `.claude/worktrees/background-nonblocking` 的未提交diff；当时仍基于887b668。没有构建、运行或修改该worker的文件，因此以下是协作边界，**不是承诺其最终实现或修复已验证**。

| 建议 | 当前可见的重叠程度 | 后续验收重点 |
|---|---|---|
| 即刻展示库存预检状态、关闭/过期结果隔离 | 直接重叠：popups.libraryConfirm、library.js/fulldetail.js调用调整 | 10/100/全选首次反馈<200ms；预检中确认不可绕过 |
| LibraryPreview detached reader、后台文件/Steam任务不持GUI锁 | 直接重叠：gui/library.go、tasks.go、readers.go、WorkspaceSnapshot、filesMu等 | 预检/执行中切页与Setting不等整任务；执行前安全重验 |
| 来源移除真正批处理、批末扫描/保存 | 直接重叠：新增removebatch、batchRemoveSubscriptions、subscribe.go/store相关改动 | 仅允许标签集合执行，恢复，验证Scan次数与逐项错误 |
| 全局链接/进程/引用预检快照，Pin/Move批末扫描 | 部分相邻：未见该diff全面覆盖LibPreview每项CLI和所有Pin/Move重扫描 | 不因释放锁就宣称总耗时解决；重测readIncomingLinks/probe次数 |
| 来源页updateRows一次整批预览/提交 | 未见subscriptions.js对应改动，属额外性能工作 | 100项Index构建由100→1，SteamCMD分组保留 |
| 工坊元数据批量请求/跨来源去重 | 未见subscriptionupdates.go/describe.go批量查询改动 | 同一真实2797条、记录请求数与失败项 |
| SetTags事务/局部刷新、任务ids[0]遗漏、大贴图表绘制 | 本轮diff未见对应完整修复；刷新另有d64c974 | 分别验证写/刷新/多选语义，避免扩大本worker范围 |

建议优先级：先合并并验收即时反馈/解锁（用户可感知收益最大），再消除批量预检重复全局检查及全部更新的单项请求链；随后优化网络元数据、标签事务和大表渲染。所有“预期”都需要用同一真实数据、相同eligible数量重测。

## 10. 可复测方法与研究收尾

在系统临时目录建立专用home/profile，不在checkout改业务代码。简化命令如下（路径用本机实际值替换）：

```powershell
$auditRoot = Join-Path $env:TEMP ('RimArcho-batch-audit-' + [guid]::NewGuid())
# 建目录；将 APPDATA/RimArcho 的 archo.db*、config.toml、tasks.json 复制到 $auditRoot/home
# 用 SQLite integrity_check 核验副本；并确认只读预览不会确认任何外部动作
$exePath = Join-Path $auditRoot 'archo.exe'
go build -o $exePath ./cmd/archo
go run ./spikes/guidev -home (Join-Path $auditRoot 'home') -exe $exePath
# 独立 headless Chrome profile，经 /json 取得 page.webSocketDebuggerUrl，使用 CDP Runtime.evaluate
```

CDP测量逻辑（原页面API/模块，不是mock）：

```javascript
const t0 = performance.now();
let first;
const observer = new MutationObserver(() => {
  // 只在本次状态/确认窗/进度出现时记下一绘制机会
  requestAnimationFrame(() => { first ??= performance.now() - t0; });
});
// 监视 #popup / #status / 当前结果区；设置原选择集合，点击原按钮
// 同时10ms timer、方向键光标、50ms后Setting('ui.lang')探针
// 等原异步处理/最终状态，记录total；关闭确认窗，不调用其执行回调
```

Go拆分复测：把上述第8节函数入口加time.Now和defer elapsed日志，**仅放系统临时overlay文件，用 `go build -overlay <json>` 编译**，不要向checkout留计时修改。单次批预览以LibraryPreview结束作为分组界；网络任务日志独立统计。剔除期间Live/其他读取日志，嵌套函数分开列账。本文全部更新100/135项统计不混入来源页初始化；最初10项之前有其他读取日志，故只报告该组handler及后续100/135纯净分组的细分。

本轮曾发现并纠正三类无效读数：未启用启用被拒绝不能当成功；任务load未await造成旧选择集合（首次全选只实际移除27条）已用40条实删/实撤销补测替换；旧导出脚本100ms人工等待和已安装项/缺失项数量混用已废弃，改用entries核验的10/100/715项。未用这些无效成绩下性能结论。

研究只新增本报告，不提交代码。临时harness、Chrome专用profile、DB副本、计时overlay与日志在证据汇总后清理；不停止其他worker的进程，不回滚并发文件。仓库原有3份10月26日报告和期间其他worker新增的GUI/CLI对照报告均保留。
