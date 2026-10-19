# M2 Loadout — 实现记录与判断

> 日期：2026-10-19。范围：01 §4 的 M2（U2、U3、U4、U8），以及主会话对 M1 遗留问题的裁定（decisions-m1.md §6）。
> 依据：04 §1–§4、§8；06 §2–§3、§7.2；05 §3。本文记录实现中的判断、与规格的偏差、真实环境实测和待定问题。

## 1. 交付内容

| 部分 | 内容 |
| --- | --- |
| M1 裁定落实 | 相似度改为 `max(J(路径), J(哈希))`（多重集合）；`About.xml.disabled` 建 Instance 并标记 `unloadable: about-disabled`；`env remove <名> --yes` 和 `env set --remove-source src:<标签>`（只限 steamcmd/extra），已补进 05 §3；TUI 的扫描以子进程 `archo scan` 执行 |
| 修复的 M1 缺陷 | 对 junction 根目录做指纹时什么都没走到：`WalkDir` 不会进入重解析点，所以真实环境里 RimCrow 部署的 25 个 `_Link_*` Mod 在 M1 中都是空清单。`Compute` 现在走链接目标。重扫后这些 Mod 发出 `updated`；56 个停用的工坊原版通过 R4 和部署副本归入同一个 Lineage |
| `internal/loadout` | 条目（Instance 或缺失条目）；按游戏顺序登记的 Registry（`_steam` 令牌、按文件夹名回退、同来源重复被拒，04 §2.4）；ModsConfig.xml 的拼接式改写（保留未知元素、BOM 和换行风格）；单份备份 + 原子替换；基于 LCS 的 diff |
| `internal/order` | 带来源记录的约束图、Tarjan 强连通分量（`order.cycle`）、按当前位置优先的 Kahn 排序、attach 优先；S1–S5 |
| `internal/rules` | 读取本地 RimSort `communityRules.json`，实现 `order.Community` 接口；不联网 |
| `internal/app` | 工作区与 ModsConfig.xml 同步（导入、外部修改、冲突）、编辑与 undo/redo、排序、保存、历史、规则、启动 |
| CLI | `enable`、`disable`、`move`、`swap`、`sort [--dry-run]`、`undo`、`redo`、`save [--dry-run]`、`loadout log/diff/checkout/tag`、`rule add/list/remove`、`launch [--dry-run] [--force]`、`list --active`；`status` 增加 Loadout 状态 |
| `internal/surface/tui` | 列表视图（06 §2–§3）、保存流程（§7.2）、`E` 切换环境、`:` 命令行、Inspect；`archo` 不带参数时启动 TUI |
| 存储 | schema v2：`instance.unloadable`，以及 `workspace`、`workspace_undo`、`loadout_version`、`loadout_tag`、`modsconfig_state`、`user_rule`、`launch` 七张表；v1 数据库自动迁移 |

## 2. 测试

| 测试 | 位置 | 状态 |
| --- | --- | --- |
| S1–S5（04 §4.3） | `internal/order/order_test.go`（算法层；500 个 Mod 排序 0.5 毫秒）；`TestSortCycleIgnoreAttach`（经过 CLI） | 通过 |
| Loadout 流程：导入与缺失条目自愈、编辑与撤销重做、拒绝启用（残留、about-disabled、仅在 SteamCMD、同包名）、`_steam` 写入、版本改写、备份、外部修改与冲突、历史与 checkout/tag/undo 来源、启动方案与阻止 | `internal/app/loadout_test.go` | 通过 |
| ModsConfig 改写（BOM、CRLF、未知元素、转义、缺失元素、新建文件）、原子写入、Registry、diff | `internal/loadout/loadout_test.go` | 通过 |
| 打印出的 ignore 引用可以原样用于 `rule add ignore` | `TestPrintedIgnoreRefsRoundTrip` | 通过 |
| M1 裁定：重组目录、只改内容、about-disabled + RimCrow junction 部署、环境/来源移除 | `internal/app/accept_test.go` | 通过 |
| CLI golden：新增 `loadout` 场景，覆盖全部新命令 | `testdata/golden/loadout.txt` | 通过 |
| TUI（teatest）：空格启用、J 移动、u 撤销、S 预览后确认、w 预览后保存、L 预览、R 与 `:scan` 走子进程钩子、80×24 布局与中文对齐、E 切换环境、Inspect | `internal/surface/tui/tui_test.go` | 通过 |

所有写 ModsConfig.xml 的测试都在 `t.TempDir()` 中进行。测试工具里的 Launcher 只记录命令，不会启动任何进程。

## 3. 判断与偏差

**K1 工作区与“已保存”。**
- 工作区第一次加载时，导入 ModsConfig.xml，作为 v1（来源 `import`）。
- “最新版本”就是 RimArcho 所知的游戏文件内容；“未保存的修改”是最新版本与工作区之间的 diff。
- 外部修改用文件内容哈希判断，比较对象是 RimArcho 上次看到的和上次写入的内容：
  - 工作区干净时，外部内容直接载入工作区，记为新版本（来源 `external`）；
  - 工作区有未保存的修改时，外部内容只记为版本，并记作 `conflict`（显示在 `status`、`loadout log`、TUI 状态栏中）。执行 `loadout checkout` 或保存后，冲突标记清除。
- 同步发生在每次扫描、TUI 启动，以及每条读取 Loadout 的命令执行时。

**K2 缺失条目。**
- 解析不到 Instance 的条目保留在列表中，保存时原样写回。
- 之后能解析到时（扫描到了新 Mod），自动变为 Instance 条目，不需要手动修复。
- 已在列表中的 Instance 如果后来变得不可登记（被删除、被停用），写入时退回为它的 packageId，避免从游戏列表里悄悄消失。

**K3 可加载性。** 只有活着、不是残留、不是 `unloadable`，并且位于 `Data/`、`Mods/` 或已订阅工坊中的 Instance，才能进 Loadout；通过别名可见也算，例如 SteamCMD 目录经 junction 出现在 `Mods/` 下。`enable` 拒绝时会说明原因。工坊版和本地版同包名时，不能同时启用，只能 `swap`（04 §2.2 第 4 条）。

**K4 保存。**
- CLI 的 `save` 不做交互确认：CLI 一次执行完，MCP 那边是否放行由 harness 决定（D6）。`save --dry-run` 和 TUI 的 `w` 会先显示 diff、版本改写（“从 1.6 改写为 1.7”）以及缺失条目数，再确认。
- `version` 写入 `Version.txt` 中的完整字符串。新建文件时，`knownExpansions` 写入已安装的官方 DLC；已有文件则保持原样。

**K5 版本来源（偏差：`undo` 的含义）。** 来源取值为 `import`、`external`、`user`、`undo`。`undo` 的含义是：checkout 某个历史版本后没有再编辑，直接保存。工作区的撤销本身不生成版本，因为它不写游戏文件。

**K6 新增 CLI 命令（偏差，已补进 05 §3）。**
- `undo`/`redo`：持久化的撤销栈，最多 100 步，CLI 和 TUI 共用；`checkout` 本身也是可以撤销的编辑。
- `--dry-run`：用于 `sort`、`save` 和 `launch`。
- `launch --force`：用已保存的列表启动，不理会未保存的编辑。

**K7 内置官方顺序的豁免（需要确认）。** Core → DLC（Royalty、Ideology、Biotech、Anomaly、Odyssey；未知的 `ludeon.rimworld.*` 排在其后）→ 其余 Mod。这条约束本身不变，但凡是声明了 `loadBefore`/`forceLoadBefore` 某个官方包的 Mod（例如 Harmony），不受“官方内容最先”的约束。不加这条豁免，几乎每个列表都会产生环。

**K8 排序的其他细节。**
- 缺失条目也作为节点参加排序，只受“官方内容最先”约束，基本保持原位。
- 每条约束都有可以屏蔽的引用。输出写法：`decl:<#实例>:<字段>:<pkg>`、`official:<#实例>`、`community:<pkg>:<字段>:<pkg>`；存储时按 Lineage 存。
- `incompatible` 规则会保存，但 M2 的排序不使用它，留给 M3 的问题分析。
- 社区规则只读 `<数据目录>/cache/communityRules.json`；不支持 `loadTop`/`loadBottom`。
- diff 里位置没有变的条目不显示为 moved。

**K9 规则 ID** 单调递增，删除后也不复用，所以 `r:<n>` 在备注和脚本里始终指同一条规则。

**K10 启动。**
- 以下情况阻止启动：ModsConfig.xml 的版本主次号与游戏不同（总是阻止）；工作区有未保存的修改（除非 `--force`）。
- `steam` 方式运行 `steam.exe -applaunch 294100`；用户数据目录不是默认位置时，两种方式都附加 `-savedatafolder=`。
- 每次启动都记录下来。游戏结束后提示“新日志”（06 §7.3）放到 M4，与日志分析一起做。

**K11 TUI（偏差：部分未实现）。**
- 详情栏在宽度 < 100 列时隐藏（06 §1 规定 80×24 时隐藏）。
- 弹出面板占满整个屏幕，不叠加在列表上。
- Problems 页显示排序发现的环和缺失条目，完整的问题分析在 M3；Changes 页显示待确认项、`changes` 的输出和外部修改提示。
- 新增按键 `R`：以子进程方式重新扫描。首次启动且从未扫描过时，自动扫描。
- 未实现：鼠标、中英文切换（界面目前为英文）、06 §7.1 中询问社区数据和 RimCrow 导入的步骤。

**K12 双重标记。** 工坊残留文件夹里只有 `About.xml.disabled` 时，同时标为 residue 和 about-disabled。真实环境中有 5 个这样的文件夹。

**K13 R4 方向。** RimCrow 停用的工坊原版比部署副本晚出现（在 M2 中才建为 Instance），所以在事件中它被记为对方的“推断副本”。这不影响 Lineage：主 Instance 仍然按“工坊优先”选出（02 §6）。

**K14 内容相同的重写不算外部修改。** 游戏或其他工具重写了 ModsConfig.xml，但解析出的列表和 `version` 都没变（只是格式不同）时，只更新“已看到”的哈希，不新增 `external` 版本。

## 4. 真实环境实测（只读）

**测量环境：** 游戏 1.7.4982，ModsConfig.xml 为 1.6.4871（485 个条目）。只运行了 `scan`、`status`、`list --active`、`sort --dry-run`，以及 TUI 测量用的测试程序（只发送导航和搜索按键）。数据只写入 `%APPDATA%\RimArcho\`。

| 项目 | 数值 |
| --- | --- |
| 重扫（含 M1 修复） | 1354 个活 Instance，其中 4+64 个 about-disabled，5 个残留；用时 2.47 秒，峰值工作集 121 MB |
| 导入的列表 | 485 个条目，3 个缺失。`status` 正确提示“这份列表是为 1.6 保存的，游戏 1.7 启动时会重置” |
| `list --active` | 墙钟 54 毫秒，工作集 32 MB |
| `sort --dry-run` | 墙钟 43–52 毫秒（含进程启动和加载）。2090 条边，移动 13 个，发现 1 个环 |
| TUI 空闲内存 | 见下方 TUI 一节 |

**实际发现：**
- Ferny 的 Progression 系列中有一个真实的约束环，涉及 3 个 Mod：Production `loadAfter` Furniture；Better Architect 系列之间互相声明 `loadAfter` 和依赖。RimArcho 按规格报告 `order.cycle`，列出 4 条证据和屏蔽引用，环内保持原有顺序。
- RimCrow Companion 原本排在第 2 位，在 Core 和 DLC 之前；排序后移到 DLC 之后，因为它没有声明 `loadBefore` 官方包。

## 5. 待定问题

1. **K7 豁免规则是否合适。** 另一种做法是只在列表里已经有这类 Mod 排在 Core 前面时才豁免。
2. **真实环境中的 Progression 环。** 规格把 `loadAfter` 当作硬约束。游戏本身不排序，只显示警告，所以这样的环在游戏中可以正常运行。是否要把“成环的 loadAfter”降为软约束？目前的做法是提示玩家用 `ignore` 规则处理。
3. **TUI 的本地化和鼠标**（06 §1）还没有做。
4. **启动后检测新日志**（06 §7.3）放在 M4。
5. **`loadout export/import`**（05 §3、04 §3）不在本次 M2 的范围内，还没有做。
