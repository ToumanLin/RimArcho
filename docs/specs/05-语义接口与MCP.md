# 05 · 语义接口、CLI 与 MCP

> 本文定义：所有表面共用的命令、文本输出格式、CLI 命令表、MCP 工具表。
> 原则：一套命令；输出是带引用的大纲文本，不是 JSON。

## 1. 命令与表面的关系

```
命令（核心库中的一个函数，带参数校验）
  ├─ CLI：archo <命令> [参数]           → 打印文本
  ├─ MCP：工具调用                       → 返回同一段文本
  └─ TUI：按键 / 命令行 `:<命令>`        → 更新界面，或在面板中显示同一段文本
```

- 同一个命令在 CLI 和 MCP 中的输出**逐字相同**。golden 测试同时覆盖两者。
- TUI 的界面文字可以本地化（中文 / 英文）。CLI 和 MCP 的文本使用固定的英文词汇，保证 agent 解析稳定；Mod 名称、备注等用户内容保持原文。

## 2. 文本输出格式

### 2.1 规则

1. 第一行是主体：`<引用> <名称>`，后面可跟简短状态。
2. 用两个空格缩进表示层级。每行要么是 `key: value`，要么是一个列表项。
3. 实体一律写引用（02 §7、00 §2），方便 agent 直接拿去调用下一个命令。
4. 列表超过默认上限时截断，最后一行写明剩余数量和展开命令：`… 37 more (archo problems --all)`。
5. 证据行的格式：`evidence: <来源类型> <引用> <说明>`。
6. 结尾可以有 `next:` 小节，列出建议的下一步命令。
7. 不输出 ANSI 颜色。CLI 在终端里可以加颜色，MCP 和重定向时一律关闭。

### 2.2 示例

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
    … 3 more (archo problems --subject #a3f9)
  changes: updated 2026-10-17 (archo changes #a3f9)
next:
  archo why #a3f9 #51aa
  archo harmony --mod #a3f9 --shared
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
  archo source search "AllComps.Remove(" --active
```

### 2.3 详略

- 默认输出摘要，单次一般不超过 40 行。
- `--all`：取消列表截断。
- `--detail`：展开证据原文，例如 XML 片段或日志原文。

## 3. CLI 命令表（MVP）

| 分组 | 命令 | 说明 |
| --- | --- | --- |
| 环境 | `env list` / `env add` / `env use <名>` / `env detect` | 管理与探测环境 |
| | `env set <名> [--game …] [--remove-source src:<标签>]` / `env rebind src:<标签> yes\|no` / `env remove <名> --yes` | 修改路径与重新绑定（02 §3.2）；移除 steamcmd/extra 来源；删除环境（只删 RimArcho 的数据）（M1 遗留问题 4） |
| 扫描 | `scan [--full]` | 扫描并对账（02） |
| 查询 | `status` | 当前环境概况：启用数量、问题数量、未保存修改、最近变化 |
| | `list [--active] [--filter <表达式>]` | 列出 Mod |
| | `find <词>` | 按名称、包名、作者、工坊 ID、别名搜索 |
| | `mod <引用>` | Mod 详情 |
| | `def <类型/名>` / `def --mod <引用>` | Def 详情或某 Mod 的 Def |
| | `patches [--mod <引用>] [--target <XPath 或 def 引用>]` | Patch 及命中情况 |
| | `harmony [--mod <引用>] [--target <方法>] [--shared]` | Harmony 补丁 |
| | `why <A> <B>` | 解释 A 和 B 的关系：顺序约束、依赖、冲突、覆盖、共同补丁 |
| | `problems [--subject <引用>] [--code <代码>] [--all]` | 问题列表 |
| | `changes [<引用>] [--since <时间或扫描>]` | 变化事件 |
| | `log [--cluster <ID>] [--file <路径>] [--prev] [--text <文本>] [--select] [--all]` | 日志归因；`--prev` 读 Player-prev.log，`--text` 分析粘贴的堆栈，`--select` 让 `problems` 纳入该日志的 `log.*` 问题 |
| | `source <引用> [--type <类型>] [--all]` / `source search <模式> [--in <引用>…] [--regex] [--budget <秒>]` | 按需反编译与搜索（03 §5.3）；`--budget` 限定本次反编译时长，未完成的程序集在文本中注明“仍在计算” |
| | `files search <模式> [--mod <引用>] [--regex] [--languages]` | 在生效内容中搜索文本；默认不搜 `Languages/`（M4） |
| Loadout | `enable` / `disable` / `move` / `swap` / `sort` | 编辑工作区（04 §2） |
| | `save` / `loadout log` / `loadout diff` / `loadout checkout` / `loadout tag` | 保存与历史（04 §3） |
| | `undo` / `redo`；`sort --dry-run`、`save --dry-run`、`launch --dry-run` / `--force`；`list --active` | 撤销与重做工作区编辑（04 §3，对应 TUI 的 `u` / `Ctrl+r`）；预览而不改动（M2 补充） |
| | `loadout export [--format text]` / `loadout import <文件>` | 文本列表、ModsConfig.xml、存档文件 |
| 规则 | `rule add` / `rule list` / `rule remove` | 用户规则（04 §5） |
| 玩家数据 | `alias` / `tag` / `note` | 别名、标签、备注 |
| 身份 | `identity list` / `identity resolve` / `lineage split` / `lineage merge` | 待确认项与家族（02 §5、§6） |
| 其他 | `copy-local` / `launch` / `data update` / `import rimcrow` / `gc` | 见 04 和 02 |
| 设置 | `config get [<键>]` / `config set <键> <值>` | 用户设置，例如 `ui.lang auto\|en\|zh`（M3 裁定 6） |
| 分析选项 | `--game-version X.Y` | 用于 `analyze`、`problems`、`def`、`patches`、`harmony`、`why`、`log`，覆盖环境的游戏版本（M3 裁定 7）；MCP 对应工具有同名参数 |

## 4. MCP（`archo mcp`）

### 4.1 传输与生命周期

- 只支持 stdio。启动时加载当前环境；可以用工具参数指定其他环境。
- 不常驻：harness 关闭会话时进程退出。
- 内容层的事实没有建好时，第一次调用会在子进程中构建；超过 10 秒仍未完成，就返回已完成的部分，并在文本中说明“仍在计算，稍后重试”。

### 4.2 工具表

工具返回的文本与对应 CLI 命令相同。注解遵循 MCP 规范，是否放行由 harness 决定（D6）。

| 工具 | 对应命令 | readOnlyHint | destructiveHint |
| --- | --- | --- | --- |
| `status` | `status` | true | — |
| `find_mods` | `find` / `list` | true | — |
| `get_mod` | `mod` | true | — |
| `get_def` | `def` | true | — |
| `get_patches` | `patches` | true | — |
| `get_harmony` | `harmony` | true | — |
| `explain_relation` | `why` | true | — |
| `list_problems` | `problems` | true | — |
| `list_changes` | `changes` | true | — |
| `analyze_log` | `log`；也可以直接传入一段堆栈文本 | true | — |
| `search_files` | `files search` | true | — |
| `read_source` | `source` / `source search` | true | — |
| `edit_loadout` | `enable` / `disable` / `move` / `swap` / `sort`（只改工作区） | false | false |
| `save_loadout` | `save`（写入 ModsConfig.xml） | false | true |
| `copy_local` | `copy-local` | false | false |
| `add_rule` | `rule add` | false | false |

MVP 不通过 MCP 开放 `launch`、`import rimcrow`、`gc`、`identity resolve`。这些操作需要玩家在场判断。

### 4.3 工具描述

每个工具的描述都要写明：
- 什么时候用它，什么时候改用别的工具；
- 参数里引用的写法（`#id`、`pkg:`、`ws:`、`def:`）；
- 一条示例调用和示例输出的前三行。

工具描述的目标读者是 agent。要让 agent 不读任何 skill，也能完成现有 error-debugging skill 的工作流（D7）。

## 5. 稳定性

- 输出中的英文关键词（`depends:`、`evidence:`、问题代码等）属于接口，改动要记入变更日志。
- 引用格式稳定。Instance ID 不随路径、包名变化而变化（02）。
- 新增字段只追加，不改变已有行的含义。

## 6. 验收

| # | 场景 | 期望 |
| --- | --- | --- |
| I1 | 同一命令分别通过 CLI 和 MCP 执行 | 输出逐字相同 |
| I2 | 输出中的任意引用 | 都可以直接作为另一个命令的参数 |
| I3 | `pkg:` 对应多个 Instance | 列出候选，不任选一个 |
| I4 | 用 Claude Code 或 Codex 接入 MCP，不加载任何 skill | 能完成一个补丁失败的诊断，以及一个 `Pawn.Tick` 异常的排查 |
| I5 | 默认输出 | 单次不超过 40 行，截断时给出展开命令 |
