# GUI 与 CLI 行为对照（2026-10-27，dev/0.9.0）

目的：给“用 CLI 写自动化 / 半自动脚本覆盖 GUI 操作”打底。逐个 GUI API（`internal/surface/gui` 的 `*API` 方法）追到 `internal/app` / `internal/view` 的函数，再找走到同一函数的 `archo` 命令。

**2026-10-27 复核更新**：基线为 `codex/automation-research` / `dev/0.9.0` 的 `0b30d42`。当前静态枚举为 184 个导出 `*API` 方法、132 个命令注册项（含别名及入口），替换原稿“约 180 / 80”的估算。`codex/background-nonblocking@bd8d5d8` 已修 D1，但尚未合入此基线；不能把分支上的修复标成这里已完成。本文保留 D1–D10 编号，以下标“更新／新增”的内容为本次核验变动。

完整动作清单、逐项修复优先级、三层驱动、10 个首批场景与 VM 方案见 [VM 自动化测试方案](2026-10-27_vm-automation-test-plan.md)。本页记录行为差异；具体实施计划归该方案。

## 结论

- **大体一致。** 05 §1.1 的共享核心设计基本成立：多数领域写操作调用相同 `app.*` 函数；撤销栈按方案存在数据库里、可跨进程（`internal/store/loadout.go:57`）；启动锁、链接健康检查、写 ModsConfig 的备份和原子替换在 app 层。运行中能否执行按动作区分，不能统称“游戏正在运行就拒绝”：例如保存允许写下次启动使用的配置（`internal/cmds/loadout.go:381`），某些文件变更会拒绝。不少 GUI 动作拼命令再 `cmds.Execute`（预览、设置页大部分操作、查看、卸载）；**更新：新增 Workshop 订阅任务直接走 SubscribeWorkshopBatch，不能再笼统说所有添加订阅都是同一命令**（`internal/surface/gui/subscribe.go:128`）。扫描和分析仍起 `archo scan` / `archo analyze` 子进程。
- **但不是完全一致。** 有几处 GUI 在 gui / view 层多做或换了一条路径，见下面“需要修的差异”。另有一批 GUI 动作没有 CLI 命令。

## 需要修的差异（按重要性）

| # | 差异 | GUI | CLI | 影响 |
|---|---|---|---|---|
| D1 | **批量删除 / 取消隐藏绕过链接生命周期（更新：在途已修，基线仍有）** | 基线 `gui/tasks.go:411` `batchFiles` 仍 `TrashPlan` 后 `TrashPath`（`app/batchfiles.go:31`）或 `os.Rename`，最后一次 `Scan(false)` | `trash` / `unhide` → `app/hidden.go:120`、`:203` 的单项动作和 `scanAfterFileOp`（`:270`） | `codex/background-nonblocking@bd8d5d8:internal/surface/gui/tasks.go:386` 已改为 detached session 逐项 `a.Trash` / `a.Unhide`，共享链接生命周期。合入后须重跑跨环境链接、取消及失败继续场景，不再重复实现 |
| D2 | **空格启用的位置算法不同** | `view/verbs.go:103` 先 `SuggestAt(整组)` 再 `Enable(At)`，整块插入 | `enable` 不带位置：先追加，再 `order.Place` 逐个排位（`app/loadout.go:598`），`attach` 规则单独处理 | 多选、带 attach 规则时位置可能不同。CLI 没有“只算建议位置”的命令，脚本无法复现 GUI |
| D3 | **Toggle 的缺失条目原位恢复不同（更新）** | `view/verbs.go:105` 带 `At`，跳过 `app/loadout.go:578` 的无位置替换分支 | 不带位置时可原位替换 | 新 UseInstalled（`app/useinstalled.go:13`）可保原位、批量重验、一步撤销，CLI 有 `loadout use-installed`；它没有改变 Toggle，不能据此宣布 D3 全部消失。这里仍是代码推导，未做实机复现 |
| D4 | **“当前编辑的方案”不同** | 侧栏打开的方案只在 GUI 进程内存里（`app/plan.go:369`）；编辑、撤销、保存、建包、分析都作用于它，子进程带 `--open-plan` | 默认作用于当前方案；`--open-plan` 存在（`surface/cli/cli.go:194`），但 `archo help` 只列了 `--env` | 同一条命令在 GUI 的 `:` 面板和终端里可能改不同方案。脚本必须显式写 `--open-plan` |
| D5 | **已有订阅更新的粒度（更新表述）** | 一个任务内逐 ref 执行（`gui/tasks.go:84`），失败不挡后面；提交时 page 更新项跳过（`:118`） | 整组 `subscribe update` → UpdateSubscriptions（`cmds/subscribe.go:73`），page 需 --from | 结果顺序、跳过和失败处理不同。这里不是新增 Workshop 订阅任务；后者重做见下表 |
| D6 | **退订 / 重新订阅的容错** | 逐项 dry-run，坏 ref 只失败一项；每 50 个交给 Steam；Steam 不在时打开工坊页 | 一个坏 ref 整条命令失败 | 脚本要自己逐项过滤 |
| D7 | **建包并发** | 任意包在建就拒绝（`api.go:343`），分离子进程，关窗也继续 | 只拒绝同名包（`app/pack.go:415`），本进程同步 | CLI 能并发建两个包 |
| D8 | **需要处理页的“转换贴图”（更新定位）** | `gui/api.go:1053` 持锁调 `view/needs.go:1017` 的同步 OptimizeTextures，未走贴图页的子进程／停止入口 | CLI 有 textures optimize，但无 textures stop | GUI 两入口调度不一致；needs 应走统一任务入口，CLI stop 调用 StopTextures |
| D9 | 下载游戏后是否切换环境 | 切换（`shell.go:460`） | 不切换 | 脚本要补 `env use` |
| D10 | 小差异 | 备注 trim；合集预览默认勾“跟随”；合集检查 10 分钟节流；`problems` 过期时起子进程分析 | 备注原样；`collection --import` 默认不跟随；`plan sync --check` 每次联网；`problems` 在本进程分析 | 一般无害，脚本知道即可 |

### 新增能力复核（本次更新）

| 能力 | GUI / app 证据 | CLI 当前等价 |
|---|---|---|
| needs / ListProblems 缓存与发布 | `internal/surface/gui/needscache.go:39`、`:105`、`:139`；ListProblems 取同一 Needs，后台按上下文键及版本发布 | `problems` 有共享问题核心；没有 sequence、updating、list-problems 事件及 GUI 缓存合同。比较最终事实，缓存／前端消费单独测 |
| sort notices 四类 | `internal/view/preview.go:47`、`:78` 返回 missingDependencies / missingMods / replacements / cycles | `sort [--dry-run] [--json]` 与 GUI 共享 `app.SortNotices`；文字和 JSON 都含已安装／远程／未知依赖、缺失项副本、推荐替换和 cycles |
| CompleteDependencies | `internal/surface/gui/ops.go:128` → `internal/app/dependencies.go:164`，启用已安装依赖闭包并排序，一步撤销 | `loadout complete-dependencies`，同一 app 操作与单步撤销；远程缺依赖只报告 |
| ReplaceRecommended | `internal/surface/gui/ops.go:137` → `internal/app/sortnotices.go:206`，整批重验和替换 | `loadout replace-recommended <ref…>`，同一 app 批量重验与单步撤销 |
| UseInstalled | `internal/surface/gui/ops.go:145` → `internal/app/useinstalled.go:13`；needs 也可触发 | `loadout use-installed <packageId…>`，同一 app 原位批量修复与单步撤销 |
| SteamCMD 批次回补 Steam | `internal/surface/gui/subscribe.go:89`、`:146` → SubscribeWorkshopBatch；`internal/app/credit.go:19`、`:97`、`:127`；`internal/surface/gui/credit.go:35` | CLI subscribe workshop 只调 SubscribeWorkshop（`internal/cmds/subscribe.go:67`）；无公开 credit-batch / prompt / answer / run。GUI 内部参数并非注册的 CLI flag。回补是领域能力缺口，不应和账号登录一起列成“故意不做” |
| subscribe 任务重做 | `internal/surface/gui/subscribe.go:128`、`tasks.go:165`：逐项进度、永久失败、取消、只继续剩余项、同一任务行恢复 | 核心订阅逻辑部分共用；CLI 无 GUI task ID / 同行恢复／任务控制入口。应共享逐项结果，窗口取消与反馈仍单测 |
| 游戏加载烟雾 | `internal/cmds/test.go:15` 的 test run，`internal/app/gametest.go:80` 隔离配置／日志 | 已有 `archo test run --json`，CLI-only 的专用入口；可用作游戏加载验证，但还须断言 Missing 为空及实际加载列表，不能只看进程或退出码 |

## GUI 有、CLI 没有的动作

**应补 CLI 的（自动化会用到）：**

| GUI 动作 | GUI 入口 | 现在 CLI 只能 |
|---|---|---|
| 结构化问题发布（新增） | `ListProblems` | `problems` 有共享核心；无 GUI sequence/updating/事件合同 |
| Steam credit 批次／答复／推进（新增） | `Subscribe(Batch)` / `CreditPrompt` / `CreditAnswer` | 无公开等价命令，需显式策略与状态入口 |
| 启用到建议位置 / 只看建议位置 | `Toggle`（`SuggestAt`） | 无法复现，见 D2 |
| 多选整块移动、按筛选结果移动 | `MoveBy`、`Place`、`MoveShown` | `move` 只收 1 个 ref，脚本逐个算 `--to` |
| 全部暂存 | `StageAll` | 自己列出所有非 Core/DLC 再 `stage add` |
| 从暂存多选放到指定位置 | `Place` | 拆成 `enable --at` + `stage drop` |
| 合并导入 | `ImportApply(merge=true)` | 无 |
| 从粘贴文本 / Rentry 链接导入 | `view.ReadImport`（`view/exchange.go:55`） | 只认文件和分享码 |
| 合集导入为新方案并跟随 | `ImportAsPlanPreview(follow)` | `collection --import` 只能替换工作区 |
| 更新到工坊版本（原地覆盖锁定副本） | `LibraryDo(LibPinUpdate)` → `UpdateToWorkshop` | `pin --update` 是另一个函数（新文件夹重做） |
| 让 Steam 客户端更新工坊 Mod | `WorkshopUpdate` → `UpdateWorkshopClient` | `workshop resubscribe` 近似，但会删了重下 |
| 按来源整体检查更新 | `CheckSubscriptionSource` | `subscribe check` 逐个查 |
| 停止贴图转换 | `TexturesStop` | 杀进程或写停止文件 |
| 添加 Archo Monitor（放在 Harmony 之后） | `AddMonitor` | `monitor install` 走的是 `EnsureMonitor`，不是同一函数 |
| 新建方案后打开它 | `NewPlan` | `plan new` 后要自己 `plan use` 或用 `--open-plan` |
| 批量 `trash` / `pin` / `unpin` / `unhide` / `library move-to-locked` | 多选 | 这些命令只收 1 个 ref，脚本循环 |
| 后台任务：排队、停止、重试、续做 | `Tasks`、`TaskAct` | CLI 同步执行，Ctrl-C 中断 |

**账号／认证交互目前只有 GUI：** SteamCMD / LoversLab 登录、验证、登出、遗忘（`gui/accounts.go:18`），LoversLab 文件浏览与安装。自动化中的认证弹窗和人工接手以后单独处理。**更新：Steam credit 回补移到领域缺口，不再与登录一起认定“故意不给 CLI”。** VM 可按用户接受的范围使用其真实 Steam 账号；客户端订退订仍会改变账号全局状态。

**纯界面、不需要 CLI：** 打开文件夹 / 网页、选文件 / 文件夹对话框、剪贴板、背景图、`ui.gui.*` 布局偏好、首页选中的整合包、引导里的目录校验和包名实时检查、引用编号模式。

## CLI 比 GUI 多的

`swap <ref> <ref>` 能指定副本（GUI 副本超过 2 份时报歧义）；`pin --name/--git/--reason`、`library move-to-locked --reason`；`textures optimize --force/--no-mipmaps`；`pack import x.archolist.json --as-plan`；`plan sync --dry-run`；`rule add/list/remove`；各命令的 `--dry-run`。**新增：`test run [plan] --json [--map]` 是现成游戏 smoke 入口**（`internal/cmds/test.go:15`）；专用 GUI 没有对应按钮，可通过 RunCLI 调用。

## 对照总表（GUI 动作 → CLI）

✅ 主要领域函数相同，仍需状态断言，不代表 GUI 调度／呈现已验收；≈ 参数、组合或呈现不同；⚠ 见上面的差异；❌ 没有 CLI。**新增核验边界**：暂存 meta 单独写入（`internal/app/stage.go:57`、`:126`），UndoRedo 只恢复 workspace entries（`internal/app/loadout.go:839`）；因此两边一致也不能证明“列表＋暂存”已完整撤销，VM 方案将其列为独立 oracle。

### 方案编辑器

| GUI 动作 | 键 / API | CLI | |
|---|---|---|---|
| 启用 | space / `Toggle` | `enable <ref…> [--at n]` | ⚠ D2 D3 |
| 停用 | space / `Toggle` | `disable <ref…>` | ✅ |
| 暂存 / 放回 / 全部放回 | `t` `T` / `Stage` `Toggle` `RestoreAll` | `stage add` / `stage restore [<ref>]` | ✅ |
| 移出暂存 | 拖 / `Unstage` | `stage drop` | ✅ |
| 上下移 1 位、10 位 | `J` `K` `alt+J` `alt+K` / `MoveBy` | `move <ref> --up/--down/--to` | ≈ 只能单个 |
| 移动到… | `m` / `Place` | `move --to/--before/--after`、`enable --at`、`stage restore --at` | ≈ 只能单个 |
| 换副本 | `x` / `Swap` | `swap <ref> [<ref>]` | ✅ CLI 更强 |
| 排序 / 保存 / 启动 | `S` `w` `L` / `Run` `RunMoving` | `sort` / `save` / `launch [--move-duplicates]` | ✅ |
| 排序提示与补齐（新增） | `Run("sort")`、`CompleteDependencies`、`ReplaceRecommended`、`UseInstalled` | `sort [--dry-run] [--json]` 共享 `app.SortNotices`；三修复命令见上 | ✅ 领域路径及单步撤销相同，窗口调度另测 |
| 列表问题与 needs 缓存（新增） | `Needs`、`ListProblems`、`list-problems` | `problems` | ≈ 核心问题共享，缓存／事件不等价 |
| 预览 | `Preview` | `sort/save/launch --dry-run` | ✅ 就是同一命令 |
| 撤销 / 重做 / 历史 | `u` `ctrl+r` `H` | `undo [n]` / `redo [n]` / `history` / `loadout checkout v<n>` | ✅ 跨进程共享 |
| 标签 / 备注 | `g` `n` | `tag add/remove` / `note [--clear]` | ✅（备注 trim 见 D10） |
| 方案：新建、切换、改名、删除、游戏版本 | `NewPlan` `UsePlan` `RenamePlan` `RemovePlan` `PlanVersionsSet` | `plan new/use/rename/remove --yes/versions` | ✅（新建后打开见 D4） |
| 打开但不设为当前 | `OpenPlan` | 全局 `--open-plan <名>` | ⚠ D4 |
| 跟随合集 / 取消 / 同步 / 检查 | `PlanFollow*` `PlanSync` `PlanCheck` | `plan follow/unfollow/sync [--check]` | ✅ |
| 内存 / 显存估算 | `PlanLoadCost*` | `plan cost` | ✅ |
| 导出 | `ExportSave` `PlanExport*` | `loadout export --format --out` | ≈ 无剪贴板，不覆盖已有文件 |
| 导入（替换 / 新方案） | `ImportApply` `ImportAsPlan` | `loadout import <file\|code> [--new]` | ≈ 见“没有的动作” |
| 命令行 | `:` / `RunCLI` | 任意命令 | ⚠ 作用于 GUI 打开的方案 |

### Mod 库存与订阅

| GUI 动作 | API | CLI | |
|---|---|---|---|
| 浏览、筛选 | `Library` | `library [--search --include --exclude --sort]` | ≈ |
| 加入当前方案 | `LibraryDo(enable)` | `enable` | ⚠ D2 |
| 从所有方案拿掉 | `LibraryDo(remove)` | `library remove-from-plans` | ✅ |
| 锁定 / 解除 | `LibraryDo(pin/unpin)` | `pin` / `unpin` | ✅（GUI 走后台任务） |
| 更新到工坊版本 | `LibraryDo(pinupdate)` | — | ❌ |
| 收进锁定库 | `LibraryDo(move)` | `library move-to-locked` | ✅ |
| 删除到回收站 / 取消隐藏 | `LibraryDo(delete/unhide)` | `trash` / `unhide` | ⚠ D1 |
| 退订 / 重新订阅 | `LibraryDo(unsub/resub)` | `workshop unsubscribe/resubscribe` | ⚠ D6 |
| 副本备注 | `LibrarySetNote` | `note --copy` | ✅ |
| 详情 / 查看 | `Detail` `Inspect` | `mod` `describe` `problems` `patches` `def` `harmony` | ✅ |
| 变化 | `Changes` | `changes [--library]` | ≈ |
| 目录迁移 | `DirectoryMovePreview` 等 | `folder move <id> <dir> [--dry-run]` | ✅ |
| 添加订阅（git / 网页 / 工坊）（更新） | `SubscribePreview` `Subscribe` | `subscribe git/page/workshop [--dry-run]` | ≈ git/page 多走同一命令；Workshop 任务走 SubscribeWorkshopBatch，取消／继续／credit 见新增复核 |
| 用下载的文件更新 | `SubscriptionFrom` | `subscribe update <ref> --from <zip>` | ✅ |
| 订阅检查 / 更新 / 移除 | `LibraryDo(sub*)` | `subscribe check/update/remove` | ⚠ D5 |
| 工坊客户端更新、按来源检查 | `WorkshopUpdate` `CheckSubscriptionSource` | — | ❌ |
| 账号、LoversLab（更新） | `AccountAction` `Lover*` | — | GUI-only，特殊交互以后处理 |
| Steam credit 批次回补（新增分类） | `CreditPrompt` `CreditAnswer` | — | ❌ 领域能力缺口，不是账号交互豁免 |

### 其他页面

| GUI 动作 | API | CLI | |
|---|---|---|---|
| 整合包：列表、清单、创建、停止、删除、导出、导入、启动 | `Packs` `PackDryRun` `PackBuild` `PackStop` `PackRemove` `PackExport` `PackImport` `PackLaunch` | `pack list/build [--dry-run]/stop/remove [--unfinished]/export/import/launch` | ✅（建包见 D7、D4） |
| 贴图：统计、安装 todds、转换、清理 | `Textures*` | `textures status/tool --install/optimize/clean` | ≈（统计用的函数不同，数字是否一致未确认） |
| 停止贴图转换 | `TexturesStop` | — | ❌ |
| 环境：列表、切换、移除 | `Environment` `UseEnv` `RemoveEnv` | `env list/use/remove --yes` | ✅ |
| 设置页各项 | `ApplySetting` | `env set/rebind`、`folder *`、`config set`、`db *`、`cache clear`、`data update`、`deps *`、`gc` | ✅ 就是同一命令 |
| 引导：查找、添加、首次扫描 | `GuideFind` `GuideAdd` `GuideScan` | `env detect` → `env add` → `env use` → `scan` → `analyze` | ≈ 一个动作拆成几条 |
| 引导：装 SteamCMD、下载游戏 | `GuideInstallSteamCMD` `GuideDownload` | `deps install steamcmd`、`env download <branch> --to` | ⚠ D9 |
| 启动、首页 ▶ | `Run("launch")` | `launch` | ✅ |
| 需要处理：一键操作（更新） | `DoAction` | `enable` `disable` `rule add ignore` `loadout checkout` `identity resolve` `unhide` `unpin` `trash` `problems ignore` `pack unlink` `links repair` `swap` `textures *` `plan versions` … | ✅ 为主；UseInstalled 有 `loadout use-installed`，转换贴图调度见 D8（`internal/view/needs.go:965`） |
| 扫描 / 分析 | `Scan` `Analyze` | `scan` / `analyze` | ✅ GUI 就是起这两个子进程 |
| 日志：选日志、看簇、模式 | `SelectLog` `LogCluster` `LogPattern` | `log --file --select`、`log --cluster`、`pattern` | ✅ |
| 添加 Archo Monitor | `AddMonitor` | — | ❌ |
| 卸载 RimArcho | `UninstallPlan` `UninstallRun` | `uninstall [--dry-run] --yes …` | ✅ |

## 给脚本的约定

1. 每条命令都写 `--env <名>` 和 `--open-plan <方案名>`，不依赖“当前”。
2. 有 `--dry-run` 的写命令先预览（enable 等不是所有命令都提供该参数），按退出码判断：0 成功，1 错误，2 用法错，3 引用有歧义（输出候选）。0 不能代替状态断言，异步 GUI ack 更不代表任务完成。
3. 删除用 `trash`；基线 D1 仍不同，在途修复合入后重验。多选命令缺口可临时循环，但不宣称循环的失败策略／历史步数与 GUI 完全相同。
4. **新增**：快按、切页／切列表、确认／取消、Esc、状态文案和事件消费必须跑真实 Wails 窗口，CLI 无法覆盖。已有 `RIMARCHO_DEVTOOLS_PORT`（`internal/surface/gui/gui.go:79`）与 `scripts/vm/cdp.mjs:19` 可复用；认证特殊交互以后单列。
