# M1 事实底座 — 实现记录与判断

> 日期：2026-10-19。范围：01 §4 的 M1（U1、U7、U9）。依据：00、02（全文）、05 §1–§3、07。
> 本文记录 M1 实现中做出的判断、与规格的偏差、真实环境实测结果和遗留问题。

## 1. 交付内容

| 包 | 内容 |
| --- | --- |
| `cmd/archo` | 入口。无参数时提示 TUI 尚未实现（M2）；`ARCHO_MEMSTATS=1` 时退出前打印进程内存 |
| `internal/env` | 环境模型、自动探测（02 §2.2）、`Version.txt`、Player.log 定位（02 §2.3）、只读 ModsConfig.xml、数据目录 |
| `internal/source` | 来源类型、junction/符号链接/8.3 短路径解析（`GetFinalPathNameByHandle`）、子目录枚举、重新绑定评估（02 §3） |
| `internal/scan` | About.xml 容错解析、PublishedFileId、LoadFolders 原文、ACF、git HEAD、`origin.json`、分层指纹（xxh3-128）、相似度、按来源并发采集（02 §4） |
| `internal/identity` | 对账 R1–R7、Lineage 归属、变更事件、待确认项（02 §5、§6），纯逻辑 |
| `internal/store` | SQLite（modernc，WAL + `synchronous=NORMAL`）、表结构、持久化、问题处理、Lineage 拆分合并、gc（02 §8） |
| `internal/app` | 各命令背后的操作：环境、扫描流水线、引用解析、身份维护 |
| `internal/cmds` | 唯一的命令表：参数解析、执行、大纲文本输出（05 §1–§3） |
| `internal/render` | 大纲文本：两空格缩进、`key: value`、截断提示、`next:`、CJK 对齐 |
| `internal/surface/cli` | CLI 表面，只负责 `--env` 全局参数和调用 `cmds.Execute` |
| `internal/ids`、`internal/vdf`、`internal/procmem` | ID 与短引用、Valve KeyValues 解析、进程内存 |
| `internal/testkit`、`testdata/` | 合成环境（假时钟、固定种子 ID、`mklink /J`）、手写 Mod 模板、golden 文件 |

CLI 命令：`env list|detect|add|set|use|rebind`、`scan [--full]`、`status`、`list [--filter] [--all]`、`find`、`mod`、`changes [<ref>] [--since] [--all]`、`identity list|resolve`、`lineage split|merge`、`gc [--dry-run]`、`help`。

## 2. 测试

| 测试 | 位置 | 状态 |
| --- | --- | --- |
| A1–A11（02 §9） | `internal/app/accept_test.go` | 全部通过 |
| 附加：残留、R6 待确认 + `resolve same`、Lineage 拆分/合并、墓碑 gc（含被待确认项保留）、仅 ACF manifest 变化、拒绝重新绑定 | 同上 | 通过 |
| 单元：About.xml（BOM、标签大小写、`authors`、无 packageId 的派生）、语义文件分类、相似度、增量哈希、ACF、VDF、ID、探测、Player.log、ModsConfig | 各包 `_test.go` | 通过 |
| golden：全部 M1 命令，三个场景 `basics`、`lifecycle`、`rebind` | `internal/cmds/golden_test.go`、`testdata/golden/*.txt` | 通过，多次运行结果稳定 |

`go test ./internal/...` 全部通过；`GOOS=linux`、`GOOS=darwin` 可以编译（未在这两个平台上运行）。A6 用手写的 `.rimarcho/origin.json` 模拟 copy-local（命令本身在 M5）。A3 的“Loadout 和用户规则不受影响”在 M1 里体现为 Instance ID 和 Lineage ID 不变，Loadout 在 M2 才有。A10 的“分析层报告包名重复”属于 M3；M1 中 `pkg:` 引用会列出全部候选（退出码 3）。

## 3. 判断与偏差

编号 J1–J22。标注“偏差”的条目与规格文字不一致，需要回写规格或确认。

**J1 代码结构。** 在 07 §2 之外增加了 `internal/app`（命令背后的操作，让 `cmds` 只做参数和渲染）、`internal/ids`、`internal/vdf`、`internal/procmem`、`internal/testkit`。表面层只依赖 `cmds`。

**J2 相似度（偏差风险，需规格确认）。** 按 02 §4.3 字面实现：`max(J(路径, 哈希), J(路径))`。但只要路径相同就计入 J(路径)，所以 J(路径) ≥ J(路径, 哈希) 恒成立，实际效果就是“按路径的 Jaccard”，哈希不起作用。后果：
- 内容改了、路径没变，相似度仍是 1.0。这有利于识别“移动 + 更新”。
- 小 Mod 加一个文件，相似度会明显下降。例如 4 个语义文件的 Mod 新增 1 个后是 0.8，低于 R3 的 0.85，于是改名加新增文件会走 R6 待确认，而不是 `moved`（见 `TestA7LocalRename` 的最后一段，测试按此行为编写）。
- 建议规格明确意图：如果想处理“文件挪到 `1.6/` 子目录”这类情况，可以加第三项“只按哈希的 Jaccard”。

**J3 规则执行顺序。** R1、R2、R5、R3 按规则逐条对全部观测执行，只与已知 Instance 匹配；剩下的观测再逐个走 R4、R6、R7。这样，同一次扫描中刚由 R7 建立的 Instance 也能成为 R4 的“原版”。没有这一点，首次扫描时工坊版和 `Mods/` 副本（A5），以及 Steam 和 SteamCMD 的同一个工坊 ID（A9），都会被判成两个无关的 Lineage。观测的处理顺序是 workshop → steamcmd → official → local → extra，让原版先于副本建立。

**J4 候选范围。** R2 的候选包括墓碑（匹配后发出 `reappeared`，位置变了再加 `moved`）。R3 只考虑本次扫描前还活着、这次没出现的 Instance；墓碑留给 R6。R6 的候选是墓碑加上本次正在消失的 Instance。R4 的候选是本次已出现的 Instance，包括本次新建的。

**J5 R5 细化。** 如果某个消失的 Instance 的最新快照带有同一份出身标记（`instance_id` 相同），且相似度 ≥ 0.85，就判为“副本被改名”（`moved`），不再新建副本。否则只要给 copy-local 出来的副本改个名，就会多出一个新 Instance。

**J6 R6 的产出。** R6 会新建 Instance 和 Lineage，发出 `discovered`，同时生成待确认项。处理方式：
- `same`：新 Instance 并入候选。候选保留自己的 ID，接管新的位置、快照、事件和别名；删除新 Instance 的 `discovered` 事件，在该问题所属的扫描批次下补一条 `reappeared`；涉及这两个 Instance 的其他待确认项标为 `superseded`。如果候选在此期间已经被 R1/R2 匹配回来，拒绝 `same`。
- `copy`：新 Instance 加入候选的 Lineage，标记为“确认”。
- `unrelated`：只关闭问题。

**J7 事件。** 只要快照哈希变了就发出 `updated`，包括 R2/R3 的移动加更新；02 §5.3 的字段事件只在哈希变化时比较。`discovered` 的 detail 记录位置，`copied` 的 detail 记录原版。detail 中保存完整 ID（`#<26 位>`），输出时换成短引用。

**J8 快照和存储（偏差：表结构有扩展）。** 只有快照哈希、ACF manifest、git HEAD 或出身标记变化时才写新快照。内容没变但 mtime 变了，就直接更新当前快照里的 mtime；mtime 只用来跳过哈希，不参与身份判断。相对 02 §8 增加的列：
- `snapshot_file.mtime`：02 §4.1 的增量扫描需要它。
- `snapshot`：`acf_time_updated`、`acf_size`、`origin`、`file_count`、`total_size`、`semantic_count`。
- `instance`：`environment_id`、`lineage_basis`（''/inferred/confirmed/manual）、`real_path`、`residue`、`current_snapshot_id`。
- `source`：`label`（引用 `src:<label>`）、`pending_root`。
- `instance_alias`：`source_id`、`rel_path`。
- `identity_question`：`environment_id`、`scan_id`、`instance_id`、`resolution`、`resolved_at`。

**J9 来源根目录失效（偏差）。** 根目录不存在时，来源标为 `missing`：本次不扫描，它的 Instance 既不匹配也不转墓碑。00 §6.5 原本的设想是用墓碑处理“移动硬盘暂时拔出”。改的理由是：A4 中玩家先扫描、后改路径时，不会把整个来源变成墓碑再“复活”一遍，重新绑定时也能和活着的 Instance 比对。墓碑仍然用于单个目录消失的情况。

**J10 重新绑定（偏差：新增命令）。**
- 抽样改为确定性的等间隔抽样，不用随机抽样，结果可以复现；只抽有 About.xml 的目录。
- 旧来源下没有任何 Instance 时直接绑定。
- 05 §3 的环境命令只有 list/add/use/detect，而 A4 需要“修改环境路径”，因此新增 `env set <name> --game/--user-data/--workshop/--steamcmd/--extra/--launch`。
- 匹配率不足时，来源标为 `rebind-pending`，暂不扫描，由新增的 `env rebind src:<label> yes|no` 回答 02 §3.2 的“这是原来的来源吗？”：
  - `yes`：采用新根目录。
  - `no`：旧来源退役，标签改为 `<label>-old`，它的 Instance 在下次扫描时转为墓碑；同时在新根目录下建一个新来源。

**J11 订阅与残留。**
- 判为“已订阅”的条件：出现在 ACF 的 `WorkshopItemsInstalled` 或 `WorkshopItemDetails` 中。真实环境下两者都是 1269 项，有 5 个文件夹不在其中。
- 找不到 ACF 时，全部视为已订阅，扫描输出中显示 `no ACF`。
- 残留规则只用于 `workshop` 来源；SteamCMD 目录游戏本来就不会直接加载。

**J12 别名位置。**
- 多个位置解析到同一个真实目录时，按以下优先级选主位置：已是某个 Instance 的位置 → 非链接 → 来源对账顺序 → 来源顺序。其余记为别名。
- junction 指向所有来源之外的目录时，只有一个位置，并记录真实路径（`mod` 输出中的 `links-to`），不算别名。
- 不跟随 Mod 目录内部的链接（07 §5.3 安全底线）。
- `.git`、`.svn`、`.rimarcho` 不计入指纹，所以 copy-local 副本和原版的指纹相同。

**J13 派生 packageId。** 用 ilspycmd 反编译 1.7.4982 的 `ModMetaDataInternal.TryParsePackageId`，按原逻辑复刻：`ConvertToASCII(author + 描述哈希前 3 位) + "." + ConvertToASCII(name)`。`author` 缺省为 `Anonymous`，描述哈希用 `GenText.StableStringHash`（UTF-16，int32 溢出回绕），结果标注为“推断”。官方内容缺 packageId 时，Core 用 `ludeon.rimworld`，其余用 `ludeon.rimworld.<文件夹>`。

**J14 Lineage 主 Instance。** 优先级：活着的 → 工坊类来源 → 最早 `first_seen` → ID。02 §6 没有提到墓碑，这里补上了“活着的优先”。

**J15 引用。**
- 短引用是在本环境内的最短唯一前缀，至少 4 位；Instance 和 Lineage 分开计算。
- 新增引用：`q:<n>`（待确认项）、`src:<label>`、`scan:<n>`、`env:<name>`。
- `mod` 等命令也接受不带 `#` 的 ID 前缀，因为 `#` 在 bash 和 PowerShell 中是注释符，带 `#` 的引用需要加引号（`'#a3f9'`）。
- 引用对应多个 Instance 时，输出候选列表，退出码为 3。退出码：0 成功，1 错误，2 用法错误，3 有歧义。

**J16 输出细节。**
- `list` 默认只列活着的 Instance。`--filter` 支持 `source:`、`status:live|tombstone|all`、`residue`、`copies`、`pkg:`、`ws:`、`lineage:` 和普通词。
- `changes` 默认显示最近一次扫描。
- 默认截断 30 行，保证 ≤ 40 行（I5）。
- 列宽最多填充到 40，避免一个超长名字把整列推开。
- 东亚模糊宽度字符按 1 格计算（07 §6.1 第 3 项）。
- 时间按本地时区输出（测试中为 UTC）。
- `status` 会只读解析 ModsConfig.xml，显示条目数和版本；版本主次号与游戏不同时提示“游戏加载时会重置列表”。

**J17 并发。** 每个来源一个 goroutine，每个来源内部最多 NumCPU 个 worker（02 §4.1）。ID 的分配在对账阶段按确定的顺序进行，与并发无关。

**J18 环境命令。** `env add <name> --detected` 用探测结果写入；探测本身（`env detect`）只输出建议（02 §2.2）。第一个环境自动成为当前环境；`--env <name>` 可以对单条命令切换环境。

**J19 gc。** 除了过期墓碑，还清除既不是当前快照、也没有被任何事件或待确认项引用的旧快照。墓碑被清除时，它的事件一并删除；空的 Lineage 也删除。

**J20 内存优化。** 增量扫描只加载带哈希的语义文件行，不加载完整清单。文件夹没有变化、也没有重新哈希时，清单读完就释放；只有外部状态变化（例如 ACF manifest）时，新快照的文件行用 SQL 从旧快照复制。

**J21 配置。** `config.toml` 只读取 `tombstone_days`（默认 90）。首次启动时写入默认文件。

**J22 安全底线。** RimArcho 只写自己的数据目录（`%APPDATA%\RimArcho\`，测试中为临时目录）。`env add`、`env set` 和 `scan` 会检查数据目录与游戏、用户数据及各来源根目录互不包含。ModsConfig.xml 只读。

## 4. 真实环境实测（01 §5.2）

**测量环境：** Windows 11，SSD，Go 1.27.2。游戏 1.7.4982 rev1280（Steam，`D:\SteamLibrary`）；ModsConfig.xml 为 1.6.4871（485 个条目）。全程只读。数据目录先用临时目录反复测量，最后一次写入 `%APPDATA%\RimArcho\`。

| 项目 | 数值 |
| --- | --- |
| Mod | 1286 个 Instance：official 6、local 70、workshop 1210 |
| 工坊目录 | 1274 个文件夹，其中 64 个没有 About.xml，全部是 RimCrow 把 `About.xml` 改名为 `About.xml.disabled` 后停用的；这 64 个里有 5 个同时已经退订（残留，只计数） |
| `Mods/` | 74 个文件夹，其中 23 个是 RimCrow 建立的 `_Link_*` junction，指向 `D:\Game\RimModManager\mods\*` |
| 文件 | 261,448 个，20.9 GB；语义文件 48,033 个 |
| 身份 | 1282 个 `discovered`、4 个推断副本（`Mods/` 中的 alt 版本）、3 个多成员 Lineage；没有残留 Instance |

| 指标 | 目标 | 实测 |
| --- | --- | --- |
| 首次全量扫描 | ≤ 60 秒 | 冷缓存（当天首次运行）**12.5 秒**；热缓存 1.7–2.1 秒（重复 4 次）。峰值工作集 125–140 MB |
| 增量扫描（无变化） | ≤ 3 秒 | 0.58–0.74 秒（进程内计时），0.61–0.83 秒（墙钟）。峰值工作集约 100 MB（优化前 255 MB，见 J20）；`GOGC=50` 时 79 MB |
| `scan --full`（已有库） | — | 1.72 秒，峰值 140 MB |
| 查询（`status`、`list`、`find`、`mod`、`changes`） | MCP ≤ 500 毫秒（M4 目标，此处作参考） | 34–45 毫秒（墙钟，含进程启动），工作集约 26 MB |
| 数据库大小 | — | 5 次扫描后 26.8 MB |

只读校验：扫描前后 ModsConfig.xml 的 md5 相同；游戏目录、工坊目录和 `Config/` 下没有任何文件的 mtime 晚于扫描前放置的标记文件。

## 5. 遗留问题与后续

1. **相似度定义**（J2），需要回写 02 §4.3。
2. **RimCrow 停用的工坊 Mod**（`About.xml.disabled`）不是 Instance，所以 `Mods/_Link_<id>` 部署副本和工坊原版之间建立不了 Lineage。可选做法：
   - 把 `About.xml.disabled` 当作声明来源，建一个“已停用”的 Instance；
   - 用 PublishedFileId 加内容相似度，与“曾经出现过的工坊 Instance”关联。

   需要在 M2（Loadout）或 M5（RimCrow 导入）之前决定。
3. **TUI 内存**：一次增量扫描的峰值约 100 MB，超过 TUI 空闲 80 MB 的目标。M2 中应该在短命子进程里扫描（00 §2 原则 5），或者扫描后调用 `debug.FreeOSMemory`。
4. **移除来源或环境**：没有对应命令（`env remove`、移除 steamcmd/extra 来源）。05 §3 也没有列出。
5. **shell 中的 `#`**：带 `#` 的引用要加引号。TUI 和 MCP 不受影响。
6. **`--active`**（05 §3 的 `list --active`）依赖 Loadout 与 `_steam` 解析，留到 M2。
7. **UTF-16 编码的 About.xml**：未处理，真实环境中没有遇到。
8. **macOS 和 Linux**：只验证了能编译，没有运行测试；junction 测试在这两个平台上改用符号链接。
9. `spikes/` 保留在仓库中，`go vet ./...` 会包含它们。
