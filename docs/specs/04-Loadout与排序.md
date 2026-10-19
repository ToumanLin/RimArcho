# 04 · Loadout、排序与玩家数据

> 本文定义：Loadout 与历史、ModsConfig.xml 的读写、自动排序、规则、玩家数据（别名、标签、备注）、copy-local、启动游戏、RimCrow 导入。

## 1. Loadout 模型

- 每个环境有一个**工作区 Loadout**，即玩家当前正在编辑的列表。
- 内容：有序的 Instance ID 列表，加上游戏主次版本、已知 DLC 列表。
- Loadout 里存的是 Instance，不是 packageId（D8 §6.7）。

## 2. 编辑与保存（U3）

### 2.1 操作

`enable`、`disable`、`move`（到某位置，或某 Mod 前后）、`swap`（用同一 Lineage 的另一个 Instance 替换）、`sort`（§4）。

每个操作都是命令，TUI、CLI、MCP 共用（05）。

### 2.2 保存

保存 = 把工作区写入 `ModsConfig.xml`，同时在历史中记一个版本（§3）。

写入规则：
1. 备份原文件到 `<user_data_root>/Config/ModsConfig.xml.rimarcho.bak`，只保留最近一份；完整历史在 RimArcho 自己的数据库里。
2. 写入临时文件后原子替换。
3. `activeMods` 中写入小写 packageId，按 Loadout 顺序排列。
4. 同一 packageId 同时存在工坊版和非工坊版，而 Loadout 选择的是工坊版时，写入 `<packageId>_steam`；选择的是非工坊版时，写入不带后缀的 `<packageId>`。M0 已根据反编译确认（07 §6.1 第 5 项）：游戏按“官方 → `Mods/` → 已订阅的工坊”的顺序登记 Mod，工坊副本与已登记的非工坊副本重名时，给工坊副本加 `_steam` 后缀。两份同时启用时，游戏只保留不带后缀的那一份，所以 Loadout 不得同时包含这两个 Instance。
5. `version` 写入环境当前的游戏版本字符串，与游戏自己保存时一致。`knownExpansions` 以及未知元素保持原样。注意：游戏读取时，如果 `version` 的主次版本与当前游戏不同，会**丢弃整个列表**并重置（M0 反编译确认）。因此：
   - 读到的 `version` 与游戏主次版本不同时，在状态栏和 `status` 中提示“这份列表是为 X 版本保存的，游戏启动时会被重置；保存一次即可按当前版本写入”；
   - 保存时如果版本发生了变化，在确认步骤里明确显示“将从 X 版本改写为 Y 版本”；
   - 启动游戏前如果仍不一致，阻止启动并提示先保存。

### 2.3 外部修改

玩家可能在游戏内的 Mod 菜单或其他工具里改了 ModsConfig.xml。每次扫描和 TUI 启动时：
- 比较 ModsConfig.xml 与 RimArcho 上次写入的内容。
- 不一致时，把外部内容导入为一个新的历史版本，来源标为 `external`，并在 Changes 视图中提示。
- 工作区里有未保存的修改时，不覆盖工作区，而是提示玩家选择保留哪一份。

### 2.4 读取 ModsConfig.xml 时的实例解析

按游戏的解析顺序（`ModsConfig` 加载过程，M0 反编译确认）：
1. 先按完整 ID 匹配。`<pkg>_steam` 对应工坊来源的 Instance，前提是同时存在非工坊的同名副本。
2. 没有匹配时，按**文件夹名**匹配，并把条目改写为该 Mod 的 packageId。
3. 还没有匹配，而条目以 `_steam` 结尾时，去掉后缀再匹配。这种情况是本地副本已经删除，只剩工坊版。
4. 普通 packageId 有多个 Instance 时，非工坊版优先。两份来源相同（例如 `Mods/` 下有两个副本）时，游戏只接受按文件夹名排序靠前的那一份，其余的报错后忽略。两种情况都给出 `pkg.duplicate-installed`。

M0 时玩家的实际环境（485 个启用的 Mod）中没有本地与工坊同包名的情况，也没有 `_steam` 条目，所以以上规则只有反编译依据，还没有游戏内的实测。
- 找不到对应 Instance 的条目：保留为“缺失条目”，显示在列表中并产生 `dep.missing` 风格的问题，不静默丢弃。

## 3. 历史（U8）

- 每次保存、外部修改导入、撤销，都生成一个版本：`v<n>`，记录时间、来源（`user` / `external` / `import` / `undo`）和完整列表。
- `archo loadout log`：列出历史。
- `archo loadout diff [vA] [vB]`：按“新增 / 移除 / 移动 / 实例替换”输出差异。默认对比工作区与最近一次保存的版本。
- `archo loadout checkout <版本>`：把某个历史版本载入工作区，需要再保存才会写入游戏。
- `archo loadout tag <版本> <名字>`：给版本起名，相当于 RimCrow 的“预设列表”。
- 撤销 / 重做：针对工作区的编辑操作，TUI 中为 `u` / `Ctrl+r`。

## 4. 自动排序（U4）

### 4.1 约束

| 约束 | 来源 | 强度 |
| --- | --- | --- |
| Core 在最前，DLC 按官方顺序紧随其后 | 内置 | 硬 |
| 依赖在被依赖方之前 | About.xml | 硬 |
| `loadBefore` / `loadAfter` / `forceLoad*`（含 `ByVersion`） | About.xml | 硬 |
| 社区排序规则 | 社区数据（§5.2） | 硬 |
| 用户规则 | 玩家（§5.1） | 硬，且可以屏蔽上面任意一条约束 |
| 尽量保持当前相对顺序 | 当前列表 | 软 |

只对已启用的 Mod 建图。约束指向未启用的 Mod 时直接忽略。

### 4.2 算法

1. 建有向图，每条边记录它的来源（哪个 Mod 的哪条声明、哪条社区规则、哪条用户规则）。
2. 求强连通分量。存在环时：不自动打破；产出 `order.cycle` 问题，列出环上每条边及其来源。环内成员保持当前相对顺序，其余部分照常排序。问题中给出一条“建议屏蔽”的边，挑选顺序：社区规则 → 声明的 loadAfter/loadBefore → 依赖；同类中选与当前顺序冲突的那条。TUI 中一键即可把它加为 `ignore` 规则（M2 裁定）。
3. 拓扑排序。多个节点同时可选时，选当前位置最靠前的。这样结果和原列表的差异最小。
4. `attach` 规则（§5.1）：被附着的 Mod 尽量紧跟在目标之后，前提是不违反硬约束。
5. 输出新顺序与 diff。TUI 先显示 diff，玩家确认后写入工作区（还没有保存到游戏）。

### 4.3 验收

| # | 场景 | 期望 |
| --- | --- | --- |
| S1 | 已排好序的列表再排一次 | 顺序不变 |
| S2 | 两个 Mod 互相声明 loadAfter | `order.cycle`，指出两条声明各自来自哪个 About.xml |
| S3 | 用户规则屏蔽其中一条 | 排序成功，结果满足剩下的约束 |
| S4 | 语言包设置了 `attach` 到原 Mod | 语言包紧跟在原 Mod 之后 |
| S5 | 500 个 Mod 排序 | ≤ 1 秒 |

## 5. 规则

### 5.1 用户规则

挂在 Lineage 上，所以原版和本地副本共享，作者改了包名也不会丢。

| 规则 | 含义 |
| --- | --- |
| `after A B` | A 在 B 之后 |
| `before A B` | A 在 B 之前 |
| `incompatible A B` | A 和 B 不能同时启用 |
| `attach A B` | A 紧跟在 B 之后（语言包、补丁包） |
| `ignore <约束引用>` | 屏蔽一条声明或社区规则 |

每条规则可以加备注，记录创建时间。

### 5.2 社区数据

| 数据集 | 用途 | 格式 |
| --- | --- | --- |
| 社区排序规则 | loadBefore/After 补充 | RimSort 社区规则格式（`communityRules.json`） |
| 工坊元数据库 | packageId 与工坊 ID 对照、依赖、支持版本 | RimSort 格式（`steamDB.json`） |
| 替代规则 | “请改用某 Mod” | Use This Instead 格式（`replacements.json`） |

- 来源可以是本地文件，也可以是配置的 URL。
- 只有玩家执行 `archo data update`，或在 TUI 中点击更新时，才访问网络。
- 社区数据按 packageId 存储，匹配时解析到 Lineage。导入时记录数据集版本和导入时间。

## 6. 本地副本（U10）

`archo copy-local <实例> [--name <目录名>] [--git]`

1. 把 Instance 复制到 `local` 来源，默认目录名为 `<原目录名>_local`；已存在时报错，不覆盖。
2. 写入 `.rimarcho/origin.json`：

   ```json
   {"origin_instance": "<ID>", "origin_lineage": "<ID>", "workshop_id": "<ID>", "origin_snapshot": "<hash>", "copied_at": "<ISO 时间>"}
   ```

3. 带 `--git` 时：初始化 git 仓库并提交一次基线，`.rimarcho/` 加入提交。
4. 不自动修改 Loadout。完成后提示可以执行 `archo swap <原实例> <副本>`。

复制时不跟随指向 Mod 目录以外的符号链接。

## 7. 玩家数据与 RimCrow 导入（U11）

### 7.1 玩家数据

| 数据 | 挂在 | 说明 |
| --- | --- | --- |
| 别名 | Lineage | 显示名称优先使用别名 |
| 标签 | Lineage | 多个，可用于筛选；RimCrow 的“分组”导入为标签 |
| 备注 | Lineage | 纯文本 |

### 7.2 一次性导入

`archo import rimcrow [--db <路径>]`

1. 读取 RimCrow 数据库中的别名、备注、分组、标签、用户规则。字段映射在 M5 根据 RimCrow 的实际表结构确定（参考 `inventory/` 中的 `new:db-schema` 一节）。
2. 映射到 RimArcho 的 Instance：先用 RimCrow 记录的绝对路径换算成“来源 + 相对路径”；失败时用 packageId 匹配，若有多个候选则不导入，列入报告。
3. 输出报告：成功、跳过、冲突（RimArcho 中已有不同的值）各多少条，并逐条列出失败项。
4. 可以重复执行，结果相同，不会产生重复数据。

## 8. 启动游戏

| 方式 | 命令 |
| --- | --- |
| `direct` | 直接运行游戏可执行文件；用户数据目录不是默认位置时，附加 `-savedatafolder=<路径>` |
| `steam` | 通过 Steam 启动：`steam -applaunch 294100 [参数]` |

- 工作区有未保存的修改时，先询问是否保存。
- 启动后，RimArcho 记录启动时间。下次打开 TUI，或在 TUI 中检测到游戏进程退出时，如果 Player.log 比启动时间新，提示“分析本次日志”。
