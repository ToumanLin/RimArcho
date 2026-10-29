# 05 · 语义接口与 MCP

> 状态：CLI 与 MCP 已实现（33 个 MCP 工具，含别名 `add_rule`、`copy_local`）。还缺 1.1 的存档工具和少数引用形式，逐条标在文中。
> 依据：00、00a；02、03、04、08 的命令语义；09（Archo 用同一套工具）。

## 1. 一套命令与工具层

### 1.1 工具层

RimArcho 的所有操作都是 `internal/cmds` 里的一张命令表加上 `internal/app` 里的同一组操作函数。五个表面都用它，不各写一套逻辑：

```
工具层：命令表（internal/cmds：参数解析、执行、大纲文本）
        └─ 操作函数（internal/app，必要时经 internal/view）
  ├─ CLI：archo <命令> [参数]             → 打印文本
  ├─ MCP：工具调用                         → 把参数拼成同一条 CLI argv，返回同一段文本
  ├─ Archo（09）：同一张 MCP 工具表         → 同一段文本
  ├─ TUI：按键 / 命令面板里输入的 archo 命令 → 改界面，或在面板里显示同一段文本
  └─ GUI：按钮、拖放                       → 调 view / app 的类型化函数（与 TUI 同一函数）
```

- CLI 和 MCP 对同一命令的输出**逐字相同**。MCP 的每个工具只是把参数翻译成一条 CLI argv，再返回 `cmds.Execute` 的文本，不另做格式化。golden 测试和 `TestCLIParity` 同时覆盖两者。
- Archo 没有私有工具，它的工具表就是 §5.2 的 MCP 工具表（09 §2.1）。
- TUI 和 GUI 修改数据时调 `internal/app` / `internal/view` 的类型化函数，不拼命令文字再读回来；`cmds.Execute` 只用于本来就是文字的场景：排序、写入、启动的预览，“查看”类操作和全屏详情的 Def / Patch / Harmony 分页，以及玩家在命令面板里自己输入的 archo 命令（06 §1）。两条路径背后是同一个操作函数，所以玩家看到的和 agent 拿到的是同一份事实。
- 语言：CLI 和 MCP 的文本用固定的英文词汇，保证 agent 解析稳定；Mod 名、别名、备注等用户内容保持原文。TUI 和 GUI 的界面文字本地化（中文 / 英文），以英文原文作查找键（`view.Text` 的 `T` / `Tf`）；面板里显示的命令输出仍是英文。界面语言按 `ARCHO_LANG` → 设置 `ui.lang`（`auto|en|zh`）→ 系统界面语言（Windows 用 `GetUserPreferredUILanguages`，其他系统用 `LANG`）的顺序决定。

### 1.2 每个玩家动作都有工具

账户会话是玩家专用例外：MCP 不提供登录、忘记、读 / 写 / 导入凭据、操作登录窗口、确认站点声明或通用认证 HTTP 代理的工具。账户相关输出仅“需玩家登录”，不包含显示名、Cookie、token、临时链接或认证原始日志。设置查询中的账户选择也只返回粗粒度状态。固定下载若使用本地会话，agent 只能引用文件页与结果，不获得认证请求能力。权威存储与隔离规则见 07 §3.1。

AI 助手是第二类用户（00 §3.2），所以**玩家在界面上能做的每个动作，都要有对应的命令和工具**，并标明只读还是会改动东西。没有工具的动作，Archo 和外部 agent 只能建议，由玩家自己点按钮。

新增给玩家的动作时，同时加 CLI 命令和 MCP 工具；工具的写操作走同一个操作函数，进同一份撤销历史（04 §3）。

玩家动作与工具（标 **待实现** 的还没有）：

| 玩家动作 | 现状 | 目标 |
| --- | --- | --- |
| 启动方案（08 §2） | 已完成 | 工具 `launch_game`，`destructiveHint: true` |
| 方案管理：列出、新建、切换、改名、删除（04 §1） | 已完成 | 工具 `manage_plans`，`destructiveHint: true`（删除方案连同历史，不能撤回） |
| 写入游戏但不启动 | `save_loadout` 的描述已改为“保存但不启动” | 已完成 |
| 看历史、对比、回到某个版本（04 §3） | 已完成 | 工具 `plan_history`（只读：log / diff / steps）；回到版本走 `edit_loadout` 的 `checkout` 动作 |
| 处理外部修改的冲突：看差异 / 载入 / 忽略（04 §4.3） | 已完成 | 看差异 `loadout diff v<n> workspace`、载入 `loadout checkout v<n>`、忽略 `loadout ignore-external`；工具：`plan_history action=diff`、`edit_loadout action=checkout` / `ignore_external` |
| 锁定版本、解除、更新锁定版本（04 §7） | 已完成 | 工具 `pin_mod`（对应 `pin`，创建锁定版本并在当前方案里换上；`copy_local` 保留为别名）；新增工具 `unpin_mod`；更新锁定版本是 `pin --update <实例>`，并入 `pin_mod` |
| 整合包：编译、列表、启动、导出、导入、删除、停止编译、移除留下的链接（08 §5） | 已完成 | 工具 `manage_packs`，`destructiveHint: true` |
| 忽略 / 取消忽略问题（03 §4.2） | 已完成 | 工具 `ignore_problem` |
| 忽略 / 恢复某个游戏版本的 Mod 提醒（03 §4.2） | 已完成 | 工具 `mute_version_warnings`（`undo=true` 恢复），批量忽略先问玩家 |
| 回答身份待确认项、拆分 / 合并 Lineage（02 §5.4、§6） | 已完成 | 工具 `resolve_identity`，`destructiveHint: true` |
| 扫描 | 已完成 | 工具 `scan` |
| Mod 库存：列出、筛选、排序；收进锁定库、从所有方案里拿掉（06 §2.9） | 已完成 | CLI `library`、`library move-to-locked`、`library remove-from-plans`；只读工具 `list_library`，写工具 `manage_library`（`destructiveHint: true`：会移动 `Mods/` 里的文件夹）。其他动作已有工具：加入当前方案 `edit_loadout`，锁定 / 更新 / 解除 `pin_mod`、`unpin_mod`，恢复给游戏和回收站 `manage_hidden` |
| 列表导入、导出、工坊合集对照（04 §9） | 已完成 | 工具 `exchange_list`（导入是一步可撤销的方案编辑） |
| 用户规则列出 / 删除 | 已完成 | `manage_rules`（`add_rule` 保留为别名） |
| 隐藏的 Mod：恢复显示、移到回收站（08 §6、§7） | 已完成 | 工具 `manage_hidden`，`destructiveHint: true` |
| 环境管理、设置 | 已完成（`status` 的 `show`：envs / settings / data / db / cache） | 只读部分并入 `status`；修改留给玩家（这些改的是 RimArcho 自己的配置，影响面大） |
| 社区数据更新、RimCrow 导入、数据库整理与压缩、`gc` | 已完成 | 工具 `maintenance`，`destructiveHint: true` |
| 卸载 RimArcho（01 §4.1） | 已完成 | 只有预览：只读工具 `preview_uninstall`（`uninstall --dry-run`，参数同 CLI）。没有执行卸载的工具，理由见 01 §4.1 |
| 存档：列出、看一份、没碰过的内容；备份、移到回收站（10 §1.6、§4） | 都没有（1.1） | CLI `saves list/show/backup/trash`、`unplayed`；只读工具 `list_saves`、`get_save`、`list_unplayed`；写工具 `manage_saves`，`destructiveHint: true` |

旧规则“`launch`、`import rimcrow`、`gc`、`identity resolve` 不通过 MCP 开放”取消：这些动作也要有工具，用注解标出会写文件或难以撤回，是否放行交给外部 agent 的 harness；Archo 一律放进“要玩家确认”档（09 §2.2）。

## 2. 引用

每个对象都有稳定的引用，可以出现在任何输出里，也可以直接作为另一个命令的参数（I2）。全集：

| 引用 | 指向 | 例子 | 说明 |
| --- | --- | --- | --- |
| `#<id>` | Instance | `#a3f9` | 本环境内最短唯一前缀，至少 4 位。不随路径、包名变化（02） |
| `L#<id>` | Lineage | `L#7c2e` | 与 Instance 分开计算前缀。一个 Lineage 有多个 Instance 时，需要单个 Instance 的命令会列出候选 |
| `pkg:<packageId>` | 包名对应的 Instance | `pkg:ceteam.combatextended` | 不区分大小写。对应多个 Instance 时列出全部候选，不任选一个（I3） |
| `ws:<工坊 ID>` | 工坊 ID 对应的 Instance | `ws:2890901044` | 也可能对应多个（Steam 与 SteamCMD 同一个条目、本地副本） |
| `def:<Type>/<defName>` | 一个 Def | `def:ThingDef/Gun_Revolver` | `def` 命令也接受 `Type/defName`、`Type@Name`（抽象父项的 Name）和单独的 defName |
| `p:<6 位十六进制>` | 一个问题 | `p:3f2a1c` | = sha1(代码 \| key \| 主体 Lineage) 的前 3 字节，跨分析稳定 |
| `log:E<n>` | 当前日志的第 n 个错误簇 | `log:E3` | `--cluster` 也接受不带前缀的 `E3` |
| `r:<n>` | 一条用户规则 | `r:1` | |
| `decl:<#id>:<字段>:<包名>` | 一条声明的排序约束 | `decl:#iqmu:loadafter:tester.y` | `rule add ignore` 的目标（04 §6） |
| `q:<n>` | 身份待确认项 | `q:12` | 也接受单独的数字 |
| `src:<标签>` | 来源 | `src:workshop` | `env rebind`、`env set --remove-source` 使用 |
| `scan:<n>` | 一次扫描 | `scan:17` | `changes --since scan:<n>` |
| `env:<名称>` | 环境 | `env:main` | 只出现在输出里；命令参数和 `--env` 直接写名称 |
| `v<n>` | 方案的一个版本 | `v3` | `loadout diff/checkout` 使用；也接受 `workspace`（当前编辑状态） |
| `pattern:<id>` | 一个错误范式（03 §6.8） | `pattern:g:xml.type-missing` | 不属于某个环境；`pattern:m:<packageId>:<hash>` 是某个 Mod 的已知错误 |

- 不带 `#` 的 ID 前缀也被接受（`archo mod a3f9`）：PowerShell 和 bash 里以 `#` 开头的词是注释，`archo mod #a3f9` 会丢掉参数。要带 `#` 就加双引号（`"#a3f9"`）；单引号在 cmd 里会连引号一起传进来。词中间的 `#`（`L#7c2e`、`decl:#iqmu:…`）不是注释。RimArcho 打印的命令一律用不带 `#` 的形式（§3.1 规则 11）。TUI 的 `:`、GUI 和 MCP 的参数不经过 shell，带不带 `#` 都行。
- `harmony:<Type.Method>`（如 `harmony:Verse.Pawn.Tick`）只出现在 `harmony --target` 输出的标题里；作为参数时直接写方法名（`harmony --target Verse.Pawn.Tick`），带 `harmony:` 前缀的输入（待实现）。
- `patch:#<id>/<文件>#<序号>` 这种单个 Patch 操作的引用没有实现（待实现）；现在用 `patches --mod <引用>` 加文件:行号定位。
- 方案：`plan` 命令按名称指定方案，名称在本环境内唯一，不区分大小写。

## 3. 文本格式

### 3.1 规则

1. 输出是带引用的大纲文本，不是 JSON。唯一的例外是 `log --follow --json`（每个新簇一行 JSON，给脚本用，03 §6.6）。
2. 第一行是主体：`<引用> <名称>`，后面可跟简短状态。
3. 用两个空格缩进表示层级。每行要么是 `key: value`，要么是一个列表项。
4. 实体一律写引用（§2），方便直接拿去调用下一个命令。
5. 列表默认最多 30 项，保证单次输出不超过 40 行（I5）。超出时最后一行写剩余数量和展开命令：`… 37 more (archo problems --all)`。没有可用的展开命令时只写 `… 37 more`。
6. 证据行：`evidence: <来源类型> <引用> <说明>`。
7. 结尾可以有 `next:` 小节，列出建议的下一步命令。
8. 不输出 ANSI 颜色（终端里也不加），MCP 与重定向的输出因此和终端一致。
9. 列宽最多填充到 40 个字符，避免一个超长名字把整列推开；东亚“模糊宽度”字符（`–`、`…`、`·`）按 1 格计算（07 §8）。
10. 时间按本地时区输出（测试中为 UTC）。
11. 输出里的 `archo` 命令把实例引用写成不带 `#` 的 ID 前缀：`fix: archo enable nyh3`、`… 3 more (archo problems --subject 5sed)`、`error: #z3bw is not enabled (archo enable z3bw)`。这样原样粘进 cmd、PowerShell、bash 都能运行，不用加引号（§2）。命令从 `archo` 起，到行尾，或到它所在括号的 `)`、`;`、两个空格为止；命令以外的引用（主体行、列表项、说明）仍写 `#nyh3`，词中间的 `#` 和双引号里的文字不变。由 `render.ShellSafe` 在 `Doc.String` 和错误输出处统一处理，写命令的代码照常用 `Index.Ref`。只改打印的文字：MCP 工具的参数照旧接受 `#nyh3`。

### 3.2 详略

- 默认输出摘要。`--all` 取消截断（问题列表里还取消折叠、显示默认隐藏的类别）；`--detail` 展开证据原文，例如 XML 片段或日志原文。
- `problems --ref <p:引用>` 与 `problems show <p:引用>` 是同一个聚焦详情入口，MCP `list_problems ref=` 返回同一文本。缺失父模板按需查找已安装提供方（03 §4.3），等待有上限且可取消；先列影响、启用命令及订阅候选（没有时不写空的 `fixes:`），仍有未找到提供方的模板时写 `providers:` 一行（没查完，或已安装的都没有定义），再列受影响内容，文件:行号跟在对应内容后，只有其他说明时才另列证据；同一 Mod 的未忽略项一起列，已忽略项单独检查。默认限制各段行数，`--ref … --all` 展开修复与忽略命令；提供方及日志条件见 03 §4.3。补丁目标未启用、Mod 的已知错误也先讲影响和处理入口。详情不附 Mod 卡的身份、文件清单、工坊清单和历史；`mod` 保持完整卡片。
- 问题列表的默认显示（哪些折叠、哪些隐藏、过期日志不计数）以 03 §4.5 为准；默认被隐藏时会有一行 `hidden: N def.override, M patch.contested` 说明。

### 3.3 退出码与错误

| 退出码 | 含义 | 输出去向 |
| --- | --- | --- |
| 0 | 成功 | stdout |
| 1 | 错误（文本以 `error:` 开头） | stderr |
| 2 | 用法错误（未知命令、未知参数、参数个数不对，附 `usage:` 行） | stderr |
| 3 | 引用有歧义：输出候选列表 | stdout |

玩家在 PowerShell 或 bash 里照着列表手打 `archo enable #nyh3`，shell 把 `#nyh3` 当注释吃掉，程序只收到 `archo enable`（§2）。所以缺的参数是 Mod 引用时（参数不够且用法里有 `<ref>`、`<ref…>`、`<A>`、`<mod>`；`<ref>` 参数的值缺失；`textures` 没给 Mod），用法错误后面加一句 `(in PowerShell and bash a word starting with # is a comment: write a mod reference as nyh3, not #nyh3)`（`cmds.commentHint`）。会对全部生效的命令要写引用或 `--all`，两样都没写时拒绝并附同一句（`refsOrAll`）：`stage restore`、`subscribe update`，和要求 `--plan` / `--all` 的 `textures`。MCP 的 `edit_loadout restore` 不给 `refs` 仍是全部放回（参数照旧，转成 `--all`）。引用可选的只读命令照没给引用执行：`changes` 看当前方案，`subscribe check` 查全部订阅。

### 3.4 示例

`archo mod pkg:ceteam.combatextended`

```
#a3f9 Combat Extended  [active 34/212, workshop ws:2890901044]
  package: ceteam.combatextended
  lineage: L#7c2e (also: #d10b local copy, inactive)
  authors: CETeam
  supports: 1.5 1.6
  depends: pkg:brrainz.harmony (#0f12, active 2)
  load-after: pkg:brrainz.harmony
  incompatible: pkg:mlie.yayoscombat3 (#51aa, active 51)
  content: defs 1240, patches 386, assemblies 3, harmony patches 214
  problems: 2 error, 3 warning
    error incompat.active  #51aa Yayo's Combat 3 is active
    error log.exception    log:E3 attributed (role: patcher)
    … 3 more (archo problems --subject a3f9)
  changes: updated 2026-10-17 (archo changes a3f9)
next:
  archo why a3f9 51aa
  archo harmony --mod a3f9 --shared
```

`archo def ThingDef/Gun_Revolver`

```
def:ThingDef/Gun_Revolver
  defined-by (load order):
    1. #5k2q Core            Defs/ThingDefs_Misc/Weapons/RangedIndustrial.xml:120
    2. #e4c1 VWE Firearms    1.6/Defs/Weapons.xml:88  ← effective
  patched-by:
    #a3f9 Combat Extended    Patches/Core/Weapons.xml:14  Replace /Defs/ThingDef[defName="Gun_Revolver"]/verbs  hit 1
    #e4c1 VWE Firearms       Patches/CE.xml:3             Add ... hit 0  (patch.no-match: target-later)
```

`archo log --cluster E3`

```
log:E3 exception x512  first at Player.log:4021
  message: ArgumentOutOfRangeException in List`1.get_Item
  top-frames:
    Verse.ThingWithComps.Tick
    Verse.Pawn.Tick
  attribution:
    #a3f9 Combat Extended   patcher  evidence: log harmony:Verse.Pawn.Tick#prefix "- PREFIX CombatExtended.HarmonyCE"
    #77b0 Some Comp Mod     patcher  evidence: log harmony:Verse.Pawn.Tick#postfix
  confidence: possible (no source identified)
next:
  archo harmony --target Verse.Pawn.Tick
  archo source search "AllComps.Remove(" --in a3f9 --in 77b0
```

## 4. CLI 命令

### 4.1 入口与全局参数

| 写法 | 作用 |
| --- | --- |
| `archo`（无参数） | 打开 TUI；双击时在 RimArcho 自带的 WezTerm 宿主里打开（06 §9.2） |
| `archo tui` | 在当前终端打开 TUI（WezTerm 宿主运行的就是它） |
| `archo --here` | 不启动 WezTerm 宿主，直接在当前终端打开 TUI |
| `archo gui` | 打开桌面窗口：同目录有 `RimArcho.exe` 就交给它并立即返回，否则在本进程运行 |
| `archo mcp` | stdio MCP server（§5） |
| `archo <命令> [参数]` | 执行一条命令后退出 |
| `--env <名称>` / `--env=<名称>` | 只对这一条命令换用另一个环境 |
| `--open-plan <名称>` / `--open-plan=<名称>` | 只对这一条命令换用另一个方案，不改当前方案（04 §1.3；GUI 的分析、编译整合包子进程用它）；写入游戏或启动会让它成为当前方案 |
| `--version` / `-v` / `archo version` | 打印版本 |
| `archo help`、`archo <命令> --help` | 命令表；单个命令的用法和每个参数 |

`RimArcho.exe`（双击，可带 `--env <名称>`）直接打开 GUI；带命令参数时按 CLI 执行，供 GUI 的扫描、分析子进程使用（07 §5）。

### 4.2 命令表

“方案”在命令和输出里的英文是 Loadout / plan；下表的编辑类命令都作用于**当前方案**。

| 分组 | 命令 | 说明 |
| --- | --- | --- |
| 环境 | `env list` / `env detect` / `env use <名>` | 列出环境（一个游戏目录一个）和共用的 Mod 文件夹；探测游戏、用户数据、工坊路径（只给建议，02 §2.2）；切换当前环境 |
| | `env add <名> [--detected] [--game <目录>] [--saves shared\|own \| --user-data <目录>] [--launch direct\|steam]` | 新建环境（02 §2.1）。一个游戏目录只能有一个环境；游戏在 Steam 库里且还没有工坊目录时顺便加上。第一个环境自动成为当前环境。`--workshop`、`--steamcmd`、`--extra` 仍可用，等于 `folder add` |
| | `env set <名> [--game <目录>] [--saves shared\|own] [--user-data <目录>] [--launch …] [--launch-args <参数>]` | 改游戏目录（触发 `Data`、`Mods` 的重新绑定，02 §3.2）、存档方式、启动方式、额外的游戏启动参数（如 `-popupwindow`，`""` 清除）。`--remove-source` 等于 `folder remove` |
| | `env branches [--install-steamcmd]` / `env download <分支> --to <目录> --account <账户名> [--name <环境>] [--saves …] [--install-steamcmd]` | 用 SteamCMD 列出 RimWorld 的分支、下载一个版本并加为环境；登录在 SteamCMD 里（08 §11） |
| Mod 文件夹 | `folder list` / `folder add workshop\|steamcmd\|extra <目录>` / `folder remove src:<标签>` | 所有环境共用的 Mod 文件夹（02 §3.1）。MCP：`status show=folders`（只读） |
| | `env rebind src:<标签> yes\|no` | 回答“这是原来的来源吗？” |
| | `env remove <名> [--yes]` | 忘掉一个环境，只删 RimArcho 的数据，不动游戏文件；不带 `--yes` 是预演（忘掉什么、留下什么、之后用哪个环境）。那个游戏在运行时拒绝；删当前环境时换到另一个。GUI 环境页和 TUI `E` 同样（06 §2.8）；MCP 不提供（环境修改留给玩家） |
| 扫描 | `scan [--full] [--game-only]` | 扫描并对账（02 §4）；`--full` 重新哈希全部语义文件；`--game-only` 只扫当前游戏的 `Data`、`Mods` |
| 查询 | `status` | 当前环境概况：游戏版本、各来源 Mod 数、残留、Lineage、上次扫描、变化、方案（启用数、缺失项、与游戏里的列表的差异）、ModsConfig 版本不符的提示、问题数、游戏运行状态 |
| | `list [--active] [--filter <表达式>] [--all]` | 列出 Mod。`--active` 按加载顺序列出当前方案；`--filter` 支持 `source:<标签\|类型>`、`status:live\|tombstone\|all`、`residue`、`unloadable`、`copies`、`pkg:`、`ws:`、`tag:<标签>`、`lineage:` 和普通词。默认只列活着的 Instance |
| | `find <词…> [--all]` | 按名称（含别名）、包名、作者、工坊 ID、文件夹搜索 |
| | `mod <引用> [--diff <引用>]` | Mod 详情；`--diff` 列出与另一个 Instance 不同的语义文件 |
| | `describe <引用> [--source auto\|about\|workshop\|both] [--refresh] [--offline]` | 简介：About.xml 的 description（按游戏版本取 descriptionsByVersion）和工坊页面简介（BBCode 转纯文本，图片写成 `[image: URL]`）。`auto` 的选择规则、联网获取与缓存见 06 §4.3；`--offline` 只读缓存 |
| | `def [<Type/defName>] [--mod <引用>] [--all]` | 谁定义、谁 Patch 了一个 Def（按加载顺序，标出生效的那个）；或某个 Mod 的全部 Def |
| | `patches [--mod <引用>] [--target <Type/defName \| XPath 片段>] [--failed] [--all]` | Patch 操作及模拟结果；`--failed` 只列落空的及原因（03 §4.4） |
| | `harmony [--mod <引用>] [--target <Type.Method \| 片段>] [--shared] [--all]` | Harmony 补丁：某 Mod 的、某方法上的、多个 Mod 共同补丁的 |
| | `why <A> <B>` | 两个 Mod 的关系：位置、双向的声明、排序约束及是否被违反、共同定义的 Def、一方对另一方的 Patch、共同的 Harmony 目标（含各自的补丁类型）、共同的问题 |
| | `problems [--ref <p:引用>] [--subject <引用>] [--code <代码或前缀>] [--severity <级别>] [--min-severity <级别>] [--all] [--detail] [--ignored] [--include-muted] [--no-analyze]` | 问题列表（03 §4）。分析过期时先在子进程重建，`--no-analyze` 不重建；`--include-muted` 也列出已忽略的版本提醒（`[version warning ignored]`），默认只报告 `ignored-version-warnings` 数 |
| | `problems show\|ignore\|unignore <p:引用>` | 一个问题的证据和修复命令；忽略到它的 Mod 变化为止；取消忽略 |
| | `problems mute-version\|unmute-version <Mod 引用…> [--game-version X.Y]` | 忽略 / 恢复版本提醒（03 §4.2），默认当前游戏的主次版本；支持 `pkg:`，移除 Mod 后仍能恢复。`mod` 的详情显示偏好和恢复命令 |
| | `changes [<引用>] [--since scan:<n>\|<天数>d\|YYYY-MM-DD] [--all]` | 变化事件，默认最近一次扫描 |
| | `library [--search <词>] [--include <键:值>]… [--exclude <键:值>]… [--sort name\|size\|changed\|played] [--lineage <引用>] [--facets] [--all]` | Mod 库存（06 §2.9）：一行一个 Lineage（几份、合计大小、最要紧的状态、用在）；`--lineage` 列出每一份的路径、状态、方案、上次游玩和动作命令；`--facets` 列出每个筛选值和个数；新增 `tags:`（精确标签）、`content:`、`has:`、`name:`、`author:`、`id:`，`source:` 的目录角色与稳定别名见 02 §0.2，旧 `location:` 保留 |
| | `library remove-from-plans <引用…>` | 从所有方案（同一游戏目录的所有环境）里拿掉，每个方案一步撤销 |
| | `library move-to-locked <引用> [--dry-run]` | 把 `Mods/` 里的备用副本收进锁定库；Instance 不变，各方案照用（06 §2.9） |
| | `log [--cluster <E#>] [--file <路径>] [--prev] [--text <文本>] [--select] [--all] [--since <游标>]` | 日志归因（03 §6）。`--prev` 读 Player-prev.log；`--text` 分析粘贴的日志或堆栈；`--select` 让 `problems` 纳入这份日志。簇列表带一行 `cursor: log1.…`；`--since <游标>` 只列这之后新出现（`new`）或次数增加（`+N`）的簇，游戏重启过时说明游标已过期并列出新一次运行的全部簇（03 §6.6）；不与 `--cluster`、`--text` 同用 |
| | `pattern [<pattern:id>] [--mod <packageId>] [--all]` | 错误范式库（03 §6.8）：一个范式的识别规则、常见原因、诊断路线、修法和语料中的出现次数；不带参数列出所有通用范式，`--mod` 列出某 Mod 的已知错误 |
| | `log --follow [--once] [--json]` | 游戏运行时跟踪 Player.log，新簇出现就输出（03 §6.6）；`--once` 读完现有内容就停；`--json` 每簇一行 JSON |
| | `files search <模式> [--mod <引用>]… [--regex] [--case] [--word] [--def] [--kind <种类,…>] [--languages] [--lang <语言>]… [--all-versions] [--game with\|without\|only] [--game-version <x.y>] [--mode matches\|files\|count] [--page <n>] [--limit <n>] [--per-file <n>] [--all]` | 按载入范围搜索（03 §5.5）：默认启用的那份副本、当前版本的载入文件夹、`Defs`/`Patches`/`About`，不含翻译；结果按载入顺序分组，带位置、副本、载入文件夹和所在 Def / XPath，分页；`--def` 找一个 defName 的定义、父子、补丁和引用 |
| | `source <引用> [--type <Namespace.Type>] [--all]` | 按需反编译（ilspycmd，按 DLL 哈希缓存，03 §5.4） |
| | `source search <模式> (--in <引用>… \| --implicated \| --active --yes) [--regex] [--all] [--budget <秒>]` | 搜索反编译的 C#。必须指定范围：`--in` 点名的 Mod；`--implicated` 最新日志错误簇的候选，没有日志时取 Harmony 警告的主体；`--active` 全部已启用 Mod，先打印估算（程序集数、DLL 大小、缓存约 3 倍 DLL 大小、每个程序集约 1.5 秒），要加 `--yes` 才执行。`--budget` 限定本次反编译时长，未完成的程序集在文本中注明“仍在计算” |
| | `analyze` | 重建 Def / Patch / Harmony 事实（平时由其他命令按需以子进程调用） |
| 编辑方案 | `enable <引用…> [--at <n>] [--before <引用>] [--after <引用>]` / `disable <引用…>` | 启用、停用 |
| | `move <引用> [--to <n>] [--before <引用>] [--after <引用>] [--up] [--down]` / `swap <引用> [<引用>]` | 移动；换成同一 Lineage 的另一个 Instance |
| | `sort [--dry-run]` | 按约束排序（04 §5）；`--dry-run` 只显示结果 |
| | `undo [<n>]` / `redo [<n>]` | 撤销 / 重做最近的一步或 n 步（04 §3） |
| | `stage list` / `stage add <引用…>` / `stage drop <引用…>` / `stage restore <引用…> \| --all [--at\|--before\|--after]` | 暂存（04 §2.2）。从已启用中暂存的 Mod 记住原位置；`restore --all` 全部放回；`add` 和 `restore` 各是一步可撤销的编辑；重新启用的 Mod 自动离开暂存 |
| 历史 | `history [--all]` | 当前方案的编辑步骤（撤销栈）和版本，默认最近 10 个版本 |
| | `loadout log [--all]` / `loadout diff [<v>] [<v>]` / `loadout checkout <v>` | 版本列表；版本之间的差异（默认最新版本 → 当前状态）；把一个版本载入当前方案（一步可撤销的编辑） |
| 写入与启动 | `save [--dry-run]` | **写入游戏但不启动**（界面上叫“保存”），给从 Steam 等其他途径开游戏的玩家：渲染当前方案的 ModsConfig.xml（备份、原子替换、写当前游戏版本）、同步 `_Archo_` 链接、移除整合包链接（04 §4.1、08 §3）。`--dry-run` 显示会写什么 |
| | `launch [--dry-run]` | 启动当前方案（08 §2）。`--dry-run` 只显示启动命令。先写入再启动、已有游戏运行就拒绝、记启动记录和 `launch` 版本；`--dry-run` 显示与 `save --dry-run` 同一份预览加启动命令行 |
| | `test run [<方案>] [--env <环境>] [--map] [--timeout <时长>] [--settle <时长>] [--json]` | 隔离数据测试运行（08 §12）：默认当前方案，直接启动、收集加载日志后停止；超时默认 `10m`，主菜单后等 `5s`。到达目标且没有错误簇返回 0，否则 1；报告始终写 stdout，进度写 stderr |
| 方案管理 | 见 §4.3 | |
| 列表交换 | `loadout export [--format text\|archo\|modsconfig\|rimsort\|rml\|rimcrow\|rimcrow-xml] [--out <文件>]` | 导出当前方案；`--out` 从不覆盖已有文件 |
| | `loadout import <文件\|分享码> [--new [<名字>]]` | 用一份列表替换当前方案（文本、ModsConfig.xml、存档、`.rml`、RimSort、RimCrow 分享码、工坊 ID），一步可撤销，不写游戏文件（04 §9）；`--new` 导入为新方案（待实现） |
| | `collection <链接\|ID> [--import]` | 读工坊合集并与已安装的 Mod 对照（联网）；`--import` 按合集顺序把已安装的部分放进当前方案（可撤销） |
| 锁定版本 | `pin <实例> [--name <文件夹>] [--git] [--dry-run]` | 创建锁定版本（04 §7）：复制到锁定库（默认 `<文件夹>_local`），写 `.rimarcho/origin.json`，`--git` 建基线提交，并只在当前方案里换成这份副本；`--update` 更新锁定版本，`--move-to-library` 把 `Mods/` 里的旧副本移进锁定库。`copy-local` 是别名 |
| | `unpin <引用> [--dry-run]` | 解除锁定：恢复工坊那份并改用它（含 RimCrow 风格的锁定） |
| 隐藏的 Mod | `unhide <引用> [--dry-run]` / `trash <引用> [--dry-run]` | 把 `About.xml.disabled` 改回 `About.xml`；把自己的副本（`Mods/`、锁定库、SteamCMD）或隐藏、已退订的工坊文件夹移到回收站（08 §6、§7） |
| 工坊订阅 | `workshop unsubscribe <引用…> [--dry-run]` / `workshop subscribe <引用…> [--dry-run]` / `workshop resubscribe <引用…> [--dry-run]` / `workshop state <引用…>` | 经 Steam 客户端退订（Steam 删掉文件夹）、重新订阅、读状态；连不上 Steam 时报错并给出工坊页面（08 §10） |
| 整合包 | `pack build <名> [--dry-run]` / `pack stop <名>` | 把当前方案编译成整合包；停止进行中的编译并删除已复制的部分（08 §5.2） |
| | `pack list` / `pack launch <名> [--dry-run]` / `pack unlink [--dry-run]` | 列出整合包；启动整合包；只删掉 `Mods/` 里整合包的 `!ArchoPack_` 链接（08 §5.3；`problems` 报 `pack.links-left` 时的修法） |
| | `pack export <名> <文件> [--list]` / `pack import <文件> [--as-plan]` / `pack remove <名> [--dry-run] [--unfinished]` | 导出为 `.archopack`（zip），`--list` 只导出它的列表（RimArcho 列表 `.archolist.json`，04 §9.3）；解开包，或对照 RimArcho 列表，`--as-plan` 同时把列表载入当前方案；移到回收站，`--unfinished` 删除被中断的编译（08 §5.4） |
| 规则 | `rule add after\|before\|incompatible\|attach <A> <B> [--note <文本>]` / `rule add top\|bottom <A> [--note]` / `rule add ignore <约束> [--note]` | 用户规则（04 §6）；`ignore` 的目标是 `sort` / `problems` 打印的约束引用 |
| | `rule list` / `rule remove <r:n>` | |
| 玩家数据 | `alias <引用> [<名称>] [--clear]` / `note <引用> [<文本>] [--clear]` | 显示名、备注，挂在 Lineage 上 |
| | `tag add\|remove <引用> <标签…> [--mod <引用>]…` / `tag list` | 标签；重复的 `--mod` 一次给多个 Mod 加减 |
| | `import rimcrow --db <路径> [--rules <路径>] [--dry-run] [--include-disabled-rules] [--all]` | 一次性导入 RimCrow 的别名、备注、分组（成为标签）和用户规则，可重复执行（04 §8） |
| 身份 | `identity [list] [--all]` / `identity why <q:n>` / `identity resolve <q:n> same\|copy\|unrelated` | 不带子命令列出待确认项；列出、查看、回答都给出裁剪后的比较（02 §5.4） |
| | `lineage split <引用>` / `lineage merge <引用> <引用>` / `gc [--dry-run]` | 拆分、合并 Lineage；清理过期墓碑和无用快照（02 §5.5） |
| 社区数据 | `data update [<数据集>…] [--dry-run]` / `data status` | 手动下载社区数据（`rules`、`steamdb`、`replacements`），或读取来源、下载与检查时间（04 §6）；交互界面另有自动更新（06 §2.16） |
| 维护 | `db status` / `db backup` / `db vacuum [--dry-run]` | 数据库路径、大小、备份；写一份一致的副本到 `<数据目录>/backups`；整理与压缩（02 §7.2） |
| | `cache status` / `cache clear <类别>` | 缓存大小；清空一类缓存（`previews`、`workshop`、`images`、`source`、`logs`），用到时重建 |
| 设置 | `config get [<键>]` / `config set <键> <值>` | 见下 |
| Archo Monitor | `monitor` / `monitor install [--dry-run]` | 随附与已安装的版本、来源、当前方案里的位置；安装、更新、默认加入方案（08 §9、04 §1.5） |
| 卸载 | `uninstall [--dry-run] [--yes] [--keep-locked [--locked-to <环境>]] [--remove-packs] [--packs-to <目录>] [--remove-saves] [--saves-to <目录>] [--remove-games] [--restore-modsconfig-backup] [--all]` | 删除 RimArcho 放在这台电脑上的东西、恢复它改过的（01 §4.1）。不带 `--yes` 只显示计划（与 `--dry-run` 相同），`--yes` 与 `--dry-run` 同时给是用法错误。参数都是非默认的那一边：`--keep-locked` 锁定库副本放进游戏的 `Mods/`（默认进回收站），`--remove-*` 进回收站（默认保留），`--*-to` 是保留的东西移到哪里，`--restore-modsconfig-backup` 换回备份（默认清理当前列表）。游戏或别的 RimArcho 在运行时拒绝 |
| 存档（1.1，待实现） | `saves list [--colony <名>]` / `saves show <存档>` / `unplayed [--group never\|partial\|functional] [--mod <引用>]` | 只读（10 §1.6） |
| | `saves backup <存档…>` / `saves trash <存档…>` | 要玩家确认（10 §4.4） |

**日志阶段输出**：有明确标记时（03 §6.7.3–4），`log` 的簇行、`problems --code log` 的 finding、MCP `analyze_log` / `list_problems` 都附短阶段名及次数，如 `[loading x3, save x2, play x4]`；不能恢复阶段的次数写 `unknown`。`log` 的 `phases` 段按文件顺序列出边界，带已记录的存档名和时间；`log --cluster` 也列本簇各阶段次数。`log --follow` 的文本沿用同样标注，`--json` 增加可选 `phases` 数组（阶段及其次数）。无标记时不增加阶段段落、标签或 JSON 字段。

**分析参数**：`--game-version X.Y` 用于 `analyze`、`problems`、`def`、`patches`、`harmony`、`why`、`log`，按另一个游戏主次版本分析（格式必须像 `1.6`，命令结束后恢复）。MCP 对应工具有同名参数 `game_version`。

**自动检查状态**：`status`、`data status`（包括 MCP `status` 对应输出）只用一行 `automatic checks`：开关状态，以及等待玩家处理的 Mod / 工具更新数、已设置的 LoversLab 账户登录提醒。没有待处理项时写 `nothing waiting (last MM-DD HH:MM)`；尚未检查时注明 `not checked yet`。省略不适用的账户及零计数，不联网，也不暴露 Cookie；`deps` 另列受管工具的新版本和检查时间，`data status` 列各份数据的检查时间。开关和间隔见 06 §2.16。

**设置键**（`config set`）：`ui.lang auto|en|zh`；`ui.icons auto|nerd|compat`；`ui.mouse on|off`（关掉终端的鼠标上报，方便选中文字）；`workshop.fetch on|off`（是否联网获取工坊简介）；`workshop.images on|off`（是否下载简介里的图片）；`updates.auto on|off`、`updates.interval normal|frequent|relaxed`（06 §2.16）；`terminal.host auto|wezterm|none`；`ui.previews on|off`；`ui.gpu on|off`（窗口的 GPU 加速，下次打开窗口生效，06 §2.14）；`ui.preview_dwell_ms 0–5000`；`tags.pinned <逗号分隔的标签>`；`monitor.default on|off`（是否把 Archo Monitor 自动加进方案，04 §1.5）；`monitor.dir <绝对路径>`（Monitor 目录，08 §9）。设置存在数据库的 meta 表（键 `cfg:<键>`）；`config.toml` 只有 `tombstone_days`（07 §3）。

### 4.3 方案管理

一个环境可以有多个方案（04 §1）。命令：

| 命令 | 作用 |
| --- | --- |
| `plan list` | 列出所有方案（所有游戏共用，04 §1.7）：名称、启用数、最近游玩、是否本环境的当前方案 |
| `plan new <名> [--from current\|v<n>\|<文件>]` | 新建方案：从当前方案、当前方案的某个版本，或一份列表文件（格式同 `loadout import`）创建 |
| `plan use <名>` | 切换当前方案（不写游戏文件） |
| `plan rename <旧名> <新名>` | 改名 |
| `plan remove <名> --yes` | 删除方案及其历史；不能删除最后一个方案 |

- 版本命令保持 `loadout log/diff/checkout/export/import`，作用于当前方案。`loadout import <文件> --new [<名字>]` 把列表导入为新方案；不带 `--new` 是替换当前方案（一步可撤销）。
- 其他命令都作用于当前方案，暂不加 `--plan` 参数。
- 外部修改冲突的三个选项（04 §4.3）：看差异 `loadout diff v<n> workspace`，载入 `loadout checkout v<n>`，忽略 `loadout ignore-external`（清除“有未载入的外部版本”标记，版本留在历史里）。
- 删除：`loadout tag`（给版本起名当命名列表）。命名列表就是方案。
- 删除：`launch --force`（启动时总会先写入，不再有“未写入就启动”的情况）。

## 5. MCP（`archo mcp`）

### 5.1 传输与生命周期

- 只支持 stdio（`StdioTransport`），不开任何网络端口。stdout 只走协议消息。
- server 名 `rimarcho`，版本取发布版本号；初始化时发出 §5.3 的 Instructions。
- 不常驻：harness 关闭会话时进程退出。
- 启动时用当前环境。每个工具都有 `env` 参数，按次切换环境，调用结束恢复。
- 同一进程里的工具调用串行执行（共用一个 App）。
- 每个工具把参数拼成一条 CLI argv，返回 `cmds.Execute` 的文本，原样不改。退出码 1、2 设 `isError`；退出码 3（引用有歧义）是正常结果，因为文本就是候选列表。
- 需要分析的调用最多等 10 秒（CLI 不限时）。分析子进程超过 10 秒仍未完成，就先返回声明层的问题，文本注明 “still computing in the background”，子进程继续跑，下一次调用取回结果；这段时间里 `get_def` / `get_patches` / `get_harmony` 用上一次（已过期）的分析，有的话。
- `read_source` 带 `search` 时默认预算 10 秒，并说明还有多少程序集待反编译（“still computing; run again”）。

### 5.2 工具表

链接健康工具：`check_links` 为只读，映射 `archo links check [--all] --json`；`repair_links` 标记 destructive，`dry_run=true` 只预览，执行必须明确 `confirm=true`，无归属凭据的链接另须 `include` 完整路径列表。CLI 对应 `links repair [--all] --dry-run` / `--yes [--include <链接路径>…]`，也支持 `--json`。不带 `--all` 时检查当前游戏及其 local 来源；输出完整路径、目标、原因、凭据和动作。断链/检查失败令 check 返回退出码 1。动作边界见 [08 §3](08-启动与整合包.md#3-链接部署)。

注解遵循 MCP 规范，是否放行由外部 harness 决定；Archo 按 09 §2.2 分档。`openWorldHint` 为 true 的：`get_description`（可能访问 Steam 公开 API）、`exchange_list`（读取 Steam 合集）、`maintenance`（下载社区数据）；其余都是 false。

| 工具 | 对应命令 | readOnlyHint | destructiveHint |
| --- | --- | --- | --- |
| `status` | `status` | true | — |
| `find_mods` | 有 `query` 时 `find`；否则 `list`（`active`、`filter`） | true | — |
| `get_mod` | `mod` | true | — |
| `get_description` | `describe`（`source`、`refresh`、`offline`） | true | — |
| `get_def` | `def`（`def`、`mod`、`all`、`game_version`） | true | — |
| `get_patches` | `patches`（`mod`、`target`、`failed`、`all`、`game_version`） | true | — |
| `get_harmony` | `harmony`（`mod`、`target`、`shared`、`all`、`game_version`） | true | — |
| `explain_relation` | `why`（`a`、`b`、`game_version`） | true | — |
| `list_problems` | `problems`；`ref` 时为 `problems --ref`（保留 `all`、`game_version`）。默认加 `--min-severity warning`，给了 `severity`、`min_severity`、`all` 或 `include_muted` 时不加；`include_muted` 映射 `--include-muted` | true | — |
| `list_changes` | `changes`（`ref`、`since`、`all`） | true | — |
| `analyze_log` | `log`（`cluster`、`text`、`file`、`prev`、`all`、`since`、`game_version`）；`pattern=<引用>` 对应 `pattern`（`list` 列出全部） | true | — |
| `search_files` | `files search`（`pattern`、`mod`、`mods`、`regex`、`case_sensitive`、`word`、`def`、`kind`、`languages`、`lang`、`all_versions`、`game`、`game_version`、`mode`、`page`、`limit`、`per_file`、`all`） | true | — |
| `read_source` | 有 `search` 时 `source search`：给了 `in` 用 `--in`，`scope: "active"` 用 `--active --yes`，否则 `--implicated`；`budget` 默认 10。否则 `source`（`ref`、`type`） | true | — |
| `edit_loadout` | `action`：`enable` / `disable` / `move` / `swap` / `sort`（`dry_run`）/ `undo` / `redo` / `stage`（`stage add`）/ `unstage`（`stage drop`）/ `restore`（`stage restore`）；位置参数 `at`、`before`、`after`、`up`、`down` | false | false |
| `save_loadout` | `save`（`dry_run`） | false | true |
| `launch_game` | `launch`（`dry_run`） | false | true |
| `test_run` | `test run --json`（`env`、`plan`、`map`、`timeout_seconds`）；启动游戏可能耗时数分钟，隔离玩家存档、配置和日志（08 §12），`openWorldHint: false` | false | false |
| `manage_plans` | `plan list/new/use/rename/remove --yes`、`loadout import --new` | false | true |
| `plan_history` | `loadout log`、`loadout diff`、`history`（`action=steps`） | true | — |
| `pin_mod` | `pin`（`ref`、`name`、`git`、`update`、`move_to_library`、`reason`、`dry_run`）；`reason` 写进这份副本的来历，副本备注会说出来（06 §2.9） | false | false |
| `copy_local` | `pin` 的旧名（`ref`、`name`、`git`） | false | false |
| `unpin_mod` | `unpin`（`ref`、`dry_run`） | false | false |
| `set_player_data` | `action`：`tag_add` / `tag_remove`（`refs`、`tags`）/ `alias` / `note`（`text`，空串清除）/ `copy_note`（一份副本自己的备注，`note <引用> <文字> --copy`，空串清除，06 §2.9） | false | false |
| `manage_rules` | `rule add/list/remove`（`action`、`kind`、`a`、`b`、`target`、`note`、`rule`） | false | false |
| `add_rule` | `manage_rules` 的旧名（`action` 默认 add） | false | false |
| `ignore_problem` | `problems ignore` / `unignore`（`undo`） | false | false |
| `mute_version_warnings` | `problems mute-version` / `unmute-version`（`refs`、`game_version`、`undo`），范围见 03 §4.2 | false | false |
| `scan` | `scan`（`full`） | false | false |
| `exchange_list` | `loadout export/import`、`collection [--import]` | false | false |
| `manage_packs` | `pack list/build/launch/export/import/remove/stop/unlink` | false | true |
| `resolve_identity` | `identity list/why/resolve`（`resolve_identity action=why question=q:<n>` 只读）、`lineage split/merge` | false | true |
| `manage_hidden` | `unhide`、`trash` | false | true |
| `maintenance` | `data update`、`import rimcrow`、`db vacuum`（`action=vacuum`）、`gc` | false | true |
| `list_library` | `library`（`search`、`include`、`exclude`、`sort`、`lineage`、`facets`、`all`） | true | — |
| `manage_library` | `action`：`move_to_locked`（`library move-to-locked`，`dry_run`、`reason`）/ `remove_from_plans`（`library remove-from-plans`）/ `unsubscribe`、`subscribe`（`workshop unsubscribe|subscribe`，`dry_run`）/ `steam_state`（`workshop state`）；`refs` | false | true |
| `preview_uninstall` | `uninstall --dry-run`（`keep_locked`、`locked_to`、`remove_packs`、`packs_to`、`remove_saves`、`saves_to`、`remove_games`、`restore_modsconfig_backup`、`all`）；从不带 `--yes` | true | — |

工具和注解以 `internal/surface/mcp/server.go` 的 `Tools()` 为准（含别名）；`destructiveHint: true` 的是 `save_loadout`、`launch_game`、`manage_plans`、`manage_packs`、`resolve_identity`、`manage_hidden`、`manage_library`、`maintenance`。§1.2 里还没有的工具按同样方式加入。

**描述的改写**（已完成；2026-10-21 之前的描述与代码不符，留作记录）：

| 工具 | 原来的描述 | 原来的实际行为 | 现在的描述 |
| --- | --- | --- | --- |
| `save_loadout` | “Refuses when ModsConfig.xml changed outside RimArcho.” | **不拒绝**：直接备份并覆盖；外部修改的检测只发生在读取时（载入为 external 版本并标冲突） | “保存但不启动：把当前方案写进 ModsConfig.xml（备份、原子替换、写当前游戏版本），同步 `_Archo_` 链接、移除整合包链接。会改变游戏下次读到的列表：先问玩家，并先用 `dry_run=true` 给玩家看差异。ModsConfig.xml 有尚未处理的外部修改冲突（04 §4.3）时拒绝，文本给出看差异 / 载入 / 忽略三个选项的命令。游戏正在运行时照常写入，并提示游戏下次启动才会读到。” |
| `edit_loadout` | “Change RimArcho's workspace Loadout … nothing reaches the game until save_loadout” | 改的是环境唯一的工作区 | “改当前方案（即时保存，每步可撤销）；不写游戏文件，直到 save_loadout 或启动方案。” 加了 `checkout` 动作；`plan` 参数暂不加（§4.3） |
| `status` | 示例里有 “unsaved edits” | 显示工作区与最新版本的差异 | 改说“与游戏里的列表的差异”，并列出方案 |
| `copy_local` | “The Loadout is not changed: call edit_loadout action=swap …” | 不改方案 | “创建锁定版本：复制成独立副本并在当前方案里换上，其他方案不动；工坊更新不影响它。” |

### 5.3 工具描述与诊断流程

每个工具的描述都写明：什么时候用它、什么时候改用别的工具；参数里引用的写法；一条示例调用和示例输出的前三行。描述的读者是 agent：不加载任何 skill，只读 Instructions 和工具描述，就要能完成原 error-debugging skill 和 assembly-inspector 的工作流（I4）。Archo 用同一份描述（09 §2.1）。

工具参数结构里的 `game_version`、`env` 必须是**直接字段**：SDK 推断 schema 时不展开嵌入结构，嵌入的字段会被当成未知属性拒绝。

**Instructions**（初始化时发给客户端，原文英文）的内容：

- 开头：RimArcho 认识一套 RimWorld 安装——每个 Mod 实例、启用列表、Def、Patch 模拟结果、Harmony 补丁、Player.log 错误簇和反编译的 C#。每个工具返回大纲文本，其中的引用（`#id`、`L#id`、`pkg:`、`ws:`、`def:Type/name`、`p:<hex>`、`log:E<n>`、`r:<n>`、`pattern:<id>`）可以直接传给别的工具。
- 诊断流程（用原样证据，不转述）：
  1. **从错误开始。** 用户贴了日志就 `analyze_log text=…`；否则 `analyze_log` 看当前 Player.log，再 `analyze_log cluster=E<n>` 看具体的簇。每个簇列出候选 Mod 和角色：source（它的代码或数据产生了错误）、patcher（只是在某一帧上有 Harmony 补丁）、dependency（提供的东西缺失或被停用）、mentioned（出现在消息或较低的帧里）。没有进一步证据时，不能把 patcher 或 mentioned 说成原因。
  2. **用 `get_mod` 确认身份和状态**：确切的实例、是否启用及位置、路径、同一 Lineage 的其他副本。
  3. **分类：**
     - XML（PatchOperation 失败、交叉引用缺失、父 Def 重复或缺失、XML 格式错误）：`get_patches failed=true`（原因：target-inactive、target-missing、target-later、wrong-version-route、stale-name、unguarded-optional、node-absent）、`get_def`、`search_files`。
     - 材质 / 图标（`_MainTex`、`ResolveIcon`）：先查 XML；在点名的 Mod 里 `search_files` 找有 `uiIconColor` 却没有 `uiIconPath` / `texPath` 的。
     - 程序集（C# 异常、`Pawn.Tick`、`ThingWithComps.Tick`、世界 tick）：对每个被补丁的帧 `get_harmony target=<方法>`，再 `read_source type=<补丁类>` 或 `search=<模式>`。tick 崩溃要在簇的候选里查 AllComps 的改动：`read_source search="AllComps.Remove(" in=[…]`（还有 `"AllComps.Add("`、`"InitializeComps"`、`"PostExposeData"`）。
     - 混合：先确认 XML 一侧加载成功（`get_patches failed=true`），再怪代码。
  4. `list_problems` 给静态全貌（依赖、不兼容、加载顺序、Def 覆盖、Patch 落空、Harmony 冲突）；`explain_relation` 解释两个具体的 Mod。
  5. **报告**：失败阶段、XML / 程序集 / 混合、确切的 Mod 实例和路径、文件+行或类+方法、为什么失败、点名的 Mod 是原因、依赖还是旁观者，以及最小的修复。
- 编辑：先 `pin_mod`（从不原地改工坊文件），再改副本；`edit_loadout` 只改 RimArcho 里的方案；`save_loadout` 写游戏的 ModsConfig.xml（先问用户）。破坏性工具（`save_loadout`、`launch_game`、`manage_plans`、`manage_packs`、`resolve_identity`、`manage_hidden`、`maintenance`）先用 dry run 给用户看，再经用户同意调用。诊断流程加一步：以前能跑时，用 `list_changes` 和 `plan_history action=diff` 找出变化。
- 工具结果上限 24 KB：用 `mod=`、`target=`、模式或簇缩小范围，不用 `all=true`（日志语料 G7）。
- 最后附错误范式检查清单（`patternChecklist`，03 §6.8）：按语料中的实际分布排序，覆盖诊断流程未讲到的类别——读档、缺资源、配置检查、程序集加载，以及噪音；每类给出 `pattern=<id>` 追问。

各工具描述的要点（示例调用与输出见 `internal/surface/mcp/desc.go`）：

| 工具 | 什么时候用 / 改用什么 |
| --- | --- |
| `status` | 会话第一步；单个 Mod 用 `get_mod`，问题本身用 `list_problems` |
| `find_mods` | 按名称、包名、作者、工坊 ID、文件夹找；一个包名可能对应多个实例，输出列出每个候选的引用；下一步用 `get_mod` |
| `get_mod` | 单个实例的全部事实；有歧义的引用返回候选，不任选 |
| `get_def` | “No ThingDef named X”、重复 Def、“哪个 Mod 赢” |
| `get_patches` | 日志里的 “Patch operation … failed”；`failed=true` 给出原因 |
| `get_harmony` | 堆栈里带 “- PREFIX/POSTFIX owner:” 的行；“哪些 Mod 碰了 Pawn.Tick”；下一步 `read_source` 看补丁类。返回 bool 的 prefix 可能跳过原方法；用 `Harmony.Patch` 在代码里打补丁的会注明无法静态解析 |
| `explain_relation` | 问题点名了两个 Mod，或用户问“为什么 A 要在 B 后面” |
| `list_problems` | 静态问题；`ref=<p:…>` 看证据和修复命令；`code` 接受前缀（`dep`、`patch`、`harmony`）；方案改变后 Def / Patch / Harmony 层自动重建（几秒） |
| `list_changes` | 用户说“昨天还好好的”时找变化 |
| `analyze_log` | 用户贴错误时一律用 `text`；簇的类别：exception、harmony-stack、xml-load、patch-failed、xref-missing、material、other；Player.log 位置：1.7 及以后 `Logs/Player.log`，更早在用户数据目录根部。游戏运行时轮询：把上次返回的 `cursor` 作为 `since` 传回，只得到新的或次数增加的簇（Player.log 与 ArchoMonitor.log 一起算）；回答说游标过期就是游戏重启了，旧的那次运行此时是 `prev=true`。已知类别的簇带 `pattern:<id>`（社区背景，不是这份列表的证据），`pattern=<id>` 展开 |
| `search_files` | XML 证据（defName、XPath 目标、`uiIconColor`、XML 里的类名）；C# 用 `read_source` |
| `read_source` | 看补丁类或帧所在类型；搜索时优先用短而具体的模式；`scope="active"` 只在用户要求时用（几百个程序集、很多分钟）；ilspycmd 缺失时文本说明怎么装，从不自动安装 |
| `edit_loadout` | 改方案；每步可撤销 |
| `save_loadout` | 见 §5.2 |
| `copy_local` | 改工坊 Mod 前先复制，工坊文件会被 Steam 覆盖；指向文件夹外的链接和上游 `.git` 不复制；已有文件夹从不覆盖 |
| `get_description` | 推荐一个 Mod 前了解它做什么；工坊文本按需获取并缓存到条目更新为止；玩家可以关掉联网（`workshop.fetch`） |
| `set_player_data` | 标签（`list --filter tag:<t>` 和 TUI 的 `tag:` 搜索用它）、别名、备注；挂在 Lineage 上，所有副本共享；`copy_note` 是一份副本自己的备注（为什么留着这一份）；不碰游戏 |
| `add_rule` | 方向错误的 About.xml / 社区约束用 `ignore` 去掉，这就是打破 `order.cycle` 的办法；规则影响排序和顺序类问题，本身不重排 |

## 6. 稳定性

- 输出中的英文关键词（`depends:`、`evidence:`、`next:`、问题代码等）属于接口，改动要记入 CHANGELOG（07 §5）。
- 引用格式稳定。Instance ID 不随路径、包名变化（02）；`p:` 引用跨分析稳定；日志问题的 key 只保留绝对路径的最后三段，库目录移动后 `p:` 和忽略记录仍然有效。范式引用 `pattern:g:<id>`、`pattern:m:<packageId>:<hash>` 在同一数据集版本内稳定；通用范式的 id 跨版本保留，Mod 特定范式的 hash 取自模板，模板不变则不变。
- 新增字段只追加，不改变已有行的含义。
- MCP 工具改名时保留旧名作别名至少一个次版本。

## 7. 验收

| # | 场景 | 期望 | 状态 |
| --- | --- | --- | --- |
| I1 | 同一命令分别通过 CLI 和 MCP 执行 | 输出逐字相同（`TestCLIParity`：19 次调用经内存传输逐字节相同） | 通过 |
| I2 | 输出中的任意引用 | 都能直接作为另一个命令的参数（`TestRefsRoundTrip`：`#id`、`pkg:`、`p:`、`log:E#`、`def:`） | 通过 |
| I3 | `pkg:` 对应多个 Instance | 列出候选，不任选一个（`TestAmbiguousPackage`） | 通过 |
| I4 | 用 Claude Code 或 Codex 接入 MCP，不加载任何 skill | 能完成一次补丁失败的诊断和一次 `Pawn.Tick` 异常的排查 | 通过（脚本化客户端自检，`spikes/mcpcheck`）；真实 agent 会话待验证 |
| I5 | 默认输出 | 单次不超过 40 行，截断时给出展开命令（`TestCLIParity`、`TestTruncation`） | 通过 |
| I6 | 注解 | 13 个只读工具 `readOnlyHint: true`；`destructiveHint: true` 只有 `save_loadout`；每个描述都有示例调用（`TestToolsAndAnnotations`）。写工具只在合成环境测试（`TestWriteTools`）；`env` 参数（`TestEnvParameter`）；分析超过等待时间时返回“仍在计算”（`TestStillComputing`） | 通过 |
| I7 | 对照 §1.2 的表，逐个玩家动作找对应工具 | 每个动作都有 CLI 命令和 MCP 工具，注解与是否写文件一致 | 部分通过（1.0 的玩家动作都有工具；1.1 的存档工具未做） |
| I8 | ModsConfig.xml 有未处理的外部修改冲突时调用 `save_loadout` | 拒绝，文本给出看差异 / 载入 / 忽略三条命令；ModsConfig.xml 不变 | 通过 |
| I9 | 游戏正在运行时调用启动工具；调用 `save_loadout` | 启动工具拒绝并说明哪个环境的游戏在运行；`save_loadout` 照常写入，提示游戏下次启动才会读到 | 通过 |
| I10 | `plan new` / `use` / `rename` / `remove` 与 `loadout import --new` 经 CLI 和 MCP 各执行一次 | 输出逐字相同；`plan use` 不写游戏文件；不能删除最后一个方案 | 通过 |
| I11 | 把输出里的命令（`fix:`、`next:`、`… N more (…)`、错误里的提示）原样粘进 cmd、PowerShell、bash | 参数一个不少，引用能解析（`TestPrintedCommandsSurviveShells`：PowerShell 5.1 / 7 的解析器逐条读，没有注释、词与预期相同，再逐条执行；规则见 `TestShellSafe`） | 通过（合成夹具）；真实 VM 待验证 |
| I12 | 在 PowerShell 或 bash 手打带 `#` 的引用，引用被当成注释 | 缺的是 Mod 引用时用法错误说明原因（`TestMissingRefNamesTheComment`；宿主上 PowerShell 5.1 / 7 跑过） | 通过（合成夹具）；真实 VM 待验证 |
