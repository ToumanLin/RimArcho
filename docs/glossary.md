# Glossary

This glossary keeps recurring terms consistent across RimArcho documentation; quote the existing English UI text when describing a control or page. Chinese is allowed here as an exception, alongside language files, `README.zh-CN.md` and the Chinese player documents (`docs/user-guide.zh-CN.md`, `docs/highlights.zh-CN.md`). Entries are sorted by Chinese pinyin within each section; the UI column is filled only when its wording or capitalization differs from the documentation term.

The domain/UI distinction follows [03 §1](specs/03-environments-and-sources.md#1-terms) and [16 §7.2](specs/16-interface/7-visual-design.md#72-interface-wording). Interface translations live in feature JSON files under [internal/l10ndata/locale/](../internal/l10ndata/locale/), selected by locale directory name. [internal/view/text.go](../internal/view/text.go) owns lookup and the English `enText` overrides, which take precedence over English lookup keys.

## Environments and scanning

| 中文 | 英文（文档里用的） | 英文界面上的叫法（和文档不一样时才填） | 说明（一句话，可空） |
| --- | --- | --- | --- |
| 包名 | package ID |  | Keep the code identifier packageId unchanged. |
| 本地副本 | local copy |  | A copy made in a local Mod directory. |
| 变更事件 | change event |  |  |
| 别名位置 | alias location |  | Another link location for the same real directory, without a new instance. |
| 出身标记 | origin marker |  | The .rimarcho/origin.json record written into a RimArcho-made copy. |
| 待确认项 | identity question | for you to confirm | An ambiguous identity match that the player must resolve. |
| 对账 | reconciliation |  | Match newly scanned directories to known instances. |
| 工坊 | Workshop | workshop | Steam Workshop; preserve capitalization in prose. |
| 工坊编号 | Workshop ID |  | The published file ID, distinct from package ID. |
| 共用来源 | shared source |  | A source shared across game environments. |
| 官方来源 | official source | official | The game's Data directory containing Core and DLC. |
| 后补订阅 | subscribe after downloading |  | Download with SteamCMD now, subscribe through Steam later to credit authors (11 §1); distinguish subscribing from downloading. |
| 环境 | environment | game | An Environment owns one game directory and its save configuration. |
| 环境来源 | environment source |  | An official or local source belonging to one environment. |
| 快照 | snapshot |  | Metadata and content fingerprints from one scan of an instance. |
| 来源 | source | mod folders; source | A registered Mod root directory; filters use its source role. |
| 来源重新绑定 | source rebinding |  | Change the root path while preserving source identity. |
| 墓碑 | tombstone |  | A retained record of an instance that has disappeared. |
| 目录角色 | directory role |  | The source role, independent of how the Mod was downloaded. |
| 内容集合 | content set |  | Identical effective content may share one stored fact set. |
| 内容指纹 | content fingerprint |  |  |
| 谱系 | lineage | copies of the same mod | A Lineage groups versions and copies of the same Mod. |
| 扫描 | scan |  |  |
| 身份合并 | identity merge |  |  |
| 实例 | instance | this one; another copy | An Instance identifies one physical Mod directory. |
| 锁定库 | lock library |  | The shared directory holding locked versions. |
| 退避 | backoff |  | Delay before retrying a transient failure. |
| 退役来源 | retired source |  | A removed source whose record is kept. |
| 相对路径 | relative path |  |  |
| 预置 | pre-seed |  | Place previously downloaded bytes before Steam client validation |
| 增量扫描 | incremental scan |  |  |
| 真实路径 | real path |  | The resolved directory path after following links. |
| 主实例 | primary Instance |  | The Instance used for a Lineage's default display name. |

## Plans and sorting

| 中文 | 英文（文档里用的） | 英文界面上的叫法（和文档不一样时才填） | 说明（一句话，可空） |
| --- | --- | --- | --- |
| 版本历史 | version history |  | Saved plan versions, distinct from Mod versions. |
| 保存修改 | save changes |  |  |
| 不兼容 | incompatibility |  |  |
| 撤销 | undo |  |  |
| 撤销栈 | undo stack |  |  |
| 当前方案 | current plan | current list | enText also maps the current plan to the current list. |
| 方案 | plan | list | Use plan in domain documentation, CLI and MCP; list in UI quotations. |
| 方案编辑器 | plan editor |  |  |
| 方案条目 | plan entry |  | An entry points to an instance, not a package ID. |
| 放回列表 | put back |  | Restore a staged entry to its previous position. |
| 放进暂存 | stage |  |  |
| 分享码 | share code |  |  |
| 更新锁定版本 | update the locked version |  | The previous locked copy is retained. |
| 跟上合集 | sync with a collection | catch up with the collection | Keep sync as the command name. |
| 跟着合集 | follow a collection | follow a Steam collection |  |
| 工作区 | workspace |  | A storage or code term; avoid it in UI quotations. |
| 规则来源 | rule source |  |  |
| 环路 | cycle |  | A cycle in the load-order constraint graph. |
| 回到旧版 | checkout | go back to this version | Keep checkout as the command and storage operation name. |
| 交接 | handoff |  | 07 §2; SteamCMD credit replacement |
| 交接锁 | handoff lock |  | Coordinates replacement and launch. |
| 加载顺序 | load order |  |  |
| 解除锁定 | unpin | unlock | Keep unpin as the command name. |
| 可选依赖 | optional dependency |  |  |
| 列表 | list |  | The player's name for a plan; preserve the UI wording in quotations. |
| 强连通分量 | strongly connected component |  |  |
| 强约束 | strong constraint |  |  |
| 前置依赖 | dependency |  | Use the same English term as 依赖. |
| 切换版本 | swap | switch version… | Keep swap as the CLI operation name. |
| 启用 | enable |  |  |
| 启用集合 | enabled set |  | Set of enabled Instances; distinct from their load order. |
| 取消关联 | unlink |  |  |
| 弱约束 | weak constraint |  |  |
| 上游 | upstream |  | Use the source's actual name when quoting the UI. |
| 社区规则 | community rule |  |  |
| 锁定版本 | locked version |  | A separate instance in the same lineage. |
| 锁定这个版本 | pin | lock this version | Keep pin as the command name. |
| 替代规则 | replacement rule |  |  |
| 停用 | disable |  | Disable an entry without deleting its files. |
| 拓扑排序 | topological sort |  |  |
| 未保存 | unsaved |  |  |
| 未启用 | disabled | Disabled | The editor area uses Disabled; status text may say not enabled. |
| 写入游戏 | write to the game | save | Write the plan projection to ModsConfig.xml. |
| 写入者 | writer |  | 07 §4.3; plan/version that last wrote ModsConfig.xml |
| 依赖 | dependency |  |  |
| 硬约束 | hard constraint |  |  |
| 已启用 | enabled | Enabled | Preserve capitalization when quoting the editor area. |
| 用户规则 | user rule |  | Rules attached to lineages and shared across environments. |
| 载入外部修改 | load outside changes | use the outside change | Adopt edits made outside RimArcho to ModsConfig.xml. |
| 暂存 | staging | Staging | The editor's temporary holding area. |
| 置底 | bottom tier |  | 08 §1.5; sorting tier |
| 置顶 | top tier |  | 08 §1.5; sorting tier |
| 重做 | redo |  |  |
| 自动排序 | automatic sorting |  |  |

## Facts and analysis

| 中文 | 英文（文档里用的） | 英文界面上的叫法（和文档不一样时才填） | 说明（一句话，可空） |
| --- | --- | --- | --- |
| 版本视图 | version view |  | A game-version view over shared fact sets. |
| 补丁操作 | patch operation |  | An XML PatchOperation, distinct from a Harmony patch. |
| 补丁链 | patch chain |  |  |
| 补丁落空 | patch with no match |  | An XPath operation that finds no target nodes. |
| 程序集 | assembly |  | A .NET assembly. |
| 错误簇 | error cluster |  | Repeated occurrences grouped as one log error. |
| 错误源 | error source |  |  |
| 调用链 | call chain |  |  |
| 定义 | Def |  | A RimWorld XML definition; preserve identifiers such as ThingDef. |
| 定义继承 | Def inheritance |  |  |
| 堆栈帧 | stack frame |  |  |
| 反编译 | decompilation |  |  |
| 方法签名 | method signature |  |  |
| 非语义文件 | nonsemantic file |  | Files outside semantic hashing |
| 分析槽位 | analysis slot |  |  |
| 分析批次 | analysis run |  | An analysis run scoped to an environment, version and enabled set. |
| 父定义 | parent Def |  |  |
| 观察与机会 | observed vs. chance |  | The corpus comparison used to calibrate finding strength. |
| 归因 | attribution |  | Identify the source or cause supported by evidence. |
| 忽略问题 | ignore a finding |  | Ignoring a finding can expire after relevant changes. |
| 交叉引用 | cross-reference |  | A reference by defName to another Def. |
| 类型定义 | type definition |  |  |
| 日志归因 | log attribution |  |  |
| 声明 | declaration |  | Author-supplied metadata or declarations. |
| 生效目录 | effective directory |  | Directory selected by the game's loading rules; distinct from the complete effective content. |
| 生效内容 | effective content |  | Files selected by the game's version and loading rules. |
| 生效文件夹 | effective folder |  | A folder selected by LoadFolders.xml or the game's fallback rules. |
| 事实 | fact |  | A directly collected declaration or observation, distinct from a finding. |
| 事实集 | fact set |  | A stored set of facts shared when effective content is identical. |
| 事实视图 | fact view |  | A version-specific view over a fact set. |
| 守卫 | guard |  | A condition protecting an optional patch operation. |
| 推断 | inference |  | A derived result, distinct from a declaration. |
| 问题 | finding | problems | A domain Finding carries a conclusion and evidence; quote problem in UI text. |
| 问题代码 | finding code |  | A stable code such as dep.missing. |
| 问题目录 | finding catalog |  |  |
| 严重性 | severity |  |  |
| 证据 | evidence |  |  |
| 证据来源 | evidence source |  |  |
| 置信度 | confidence |  |  |

## Interface

| 中文 | 英文（文档里用的） | 英文界面上的叫法（和文档不一样时才填） | 说明（一句话，可空） |
| --- | --- | --- | --- |
| 变化记录 | change history |  |  |
| 侧栏 | sidebar |  |  |
| 订阅与更新 | subscriptions and updates |  |  |
| 反馈中转 | feedback relay |  | Cloudflare Worker that forwards feedback to GitHub issues. |
| 发丝线 | hairline |  | The UI's thin border and separator rule. |
| 分段控件 | segmented control |  | GUI single-choice control. |
| 分隔缝 | splitter |  | A draggable gap between resizable cards or columns. |
| 光学对齐 | optical alignment |  | Visual positioning of icons and text. |
| 后台任务 | background tasks |  |  |
| 兼容模式 | compatibility mode |  | The TUI icon mode using broadly supported Unicode glyphs. |
| 角标 | badge |  |  |
| 焦点 | focus |  |  |
| 聚焦键 | focus key |  | A number key that focuses a page area. |
| 空状态 | empty state |  |  |
| 库存 | inventory | mod library | A technical term for the set of known Mod instances. |
| 眉题 | eyebrow |  | Short contextual label above a heading. |
| 面包屑 | breadcrumbs |  |  |
| 命令面板 | command panel |  |  |
| Mod 库 | Mod library | mod library | The page listing every installed Mod copy; the English page name is lowercase. |
| 能不能跑 | Can it run | can it run | The page that checks whether the current plan can run; the English page name is lowercase. |
| 强调色 | accent color |  |  |
| 启动页 | Launch | start | The first page: choose a game and a list or pack, then launch it. |
| 全屏详情 | full-screen details |  |  |
| 上次运行 | last run |  |  |
| 上次运行的报错 | errors from the last run |  |  |
| 首次引导 | first-run guide |  |  |
| 树形引导线 | tree guide |  | Sidebar child hierarchy, not a selection marker. |
| 微光 | shimmer |  | Loading effect. |
| 详情栏 | detail column |  |  |
| 行高 | row height |  | Virtualized-list measurement. |
| 悬停提示 | tooltip |  |  |
| 虚拟列表 | virtual list |  | Render only visible rows. |
| 续行 | continuation line |  | An additional display line belonging to one row. |
| 需要处理 | needs attention |  | A Mod's status in its details when it has problems; the page is Can it run. |
| 要你确认 | for you to confirm |  |  |
| 游戏日志 | game log |  |  |
| 预览 | preview |  | Show the proposed changes before applying them. |
| 占位骨架 | skeleton |  | Temporary page loading structure. |
| 最近变化 | recent changes |  |  |

## Launch, packs and textures

| 中文 | 英文（文档里用的） | 英文界面上的叫法（和文档不一样时才填） | 说明（一句话，可空） |
| --- | --- | --- | --- |
| 编译整合包 | build a pack | create a pack | Build remains the CLI and domain operation name. |
| 部署链接 | deployment link |  |  |
| 持久台账 | persistent ledger |  | Tracks link operations for rollback and restart recovery. |
| 存档 | save |  | Distinguish the saved game from the save action. |
| 存档读取 | save reading |  |  |
| 独立存档 | own saves |  | An environment or pack uses its own user data directory. |
| 峰值内存 | peak memory |  |  |
| 符号链接 | symbolic link |  |  |
| 功能型 Mod | functional Mod |  | 12 §3.3; Mods without playable content |
| 共用默认存档 | shared default saves |  | Use the game's default user data directory. |
| 回收站 | recycle bin |  |  |
| 精简贴图 | texture slimming |  | DDS conversion with reduced dimensions. |
| 链接部署 | link deployment |  | Expose a Mod through a managed link in the game's Mods directory. |
| 没碰过的内容 | unused content |  | 12 §3; distinct from sidebar not played yet |
| 目录联接 | directory junction |  | A Windows directory junction, distinct from a symbolic link. |
| 内存校准 | memory calibration |  |  |
| 内容目录 | content catalog |  | 12 §2; categories of Mod content |
| 启动 | launch |  |  |
| 启动参数 | launch arguments |  |  |
| 启动记录 | launch record |  | A persisted Launch, distinct from a scanned log run. |
| 轻包 | light pack |  | Not a RimArcho term: list-only sharing exports a RimArcho list (16 §7.2). |
| 入链 | inbound link |  | A link pointing into a directory affected by an operation. |
| 私有字节 | private bytes |  | The Windows process memory counter. |
| 贴图 | texture |  |  |
| 贴图优化 | texture optimization |  |  |
| 退订残留 | unsubscribed residue |  | Files left after a Workshop subscription is removed. |
| 完整包 | full pack | pack | Not a separate kind: every pack includes its files. |
| 显存 | video memory |  |  |
| 隐藏的 Mod | hidden mod |  | A Mod hidden from the game, not necessarily unsubscribed. |
| 用户数据目录 | user data directory |  | Contains Config/ModsConfig.xml and saves. |
| 游玩回顾 | playthrough review |  | 12 §3–4; future page |
| 预转换 | pre-conversion | pre-convert (full size) | DDS conversion that preserves texture dimensions. |
| 载入预算 | loading budget |  | Startup RAM estimate; not a peak guarantee. |
| 占用统计 | resource-usage statistics |  | Plan resource estimates; distinguish from disk-directory size alone |
| 整合包 | pack |  | A fixed artifact compiled from a plan, with its own saves. |
| 直接启动 | direct launch |  | Start the game executable directly. |

## Saves

| 中文 | 英文（文档里用的） | 英文界面上的叫法（和文档不一样时才填） | 说明（一句话，可空） |
| --- | --- | --- | --- |
| 翠象 | Aprimay |  | The first quadrum (days 1–15 of the year), as the game's Chinese translation names it. |
| 赫象 | Jugust |  | The second quadrum (days 16–30). |
| 素象 | Decembary |  | The fourth quadrum (days 46–60). |
| 荼象 | Septober |  | The third quadrum (days 31–45). |

## Testing and VM

| 中文 | 英文（文档里用的） | 英文界面上的叫法（和文档不一样时才填） | 说明（一句话，可空） |
| --- | --- | --- | --- |
| 测试轮次 | test round |  |  |
| 测试驱动 | test driver |  | Archo Drive controls the game during VM test rounds. |
| 代理键 | surrogate key |  | Database research |
| 动作可达性台账 | action-reachability ledger |  | VM crawler coverage artifact |
| 合成数据 | synthetic data |  | Use internal/testkit for automated tests. |
| 回归测试 | regression test |  |  |
| 阶段标记 | phase marker |  |  |
| 就绪边界 | readiness boundary |  | Distinguish API acknowledgement, data readiness, and actual operation completion |
| 跨版本共用 | sharing across versions |  | Identical effective facts may be reused across game versions. |
| 冷启动 | cold start |  |  |
| 前缀压缩 | prefix compression |  | Snapshot path-block encoding |
| 情景 | scenario |  | A numbered VM acceptance scenario. |
| 驱动 | driver |  | Use test driver for Archo Drive and GPU driver for graphics drivers. |
| 实测基线 | measured baseline |  |  |
| 数据库迁移 | database migration |  |  |
| 虚拟机 | virtual machine (VM) |  |  |
| 验收 | acceptance |  |  |
| 预热 | warm-up |  |  |
| 证据文件 | evidence file |  |  |
| 整数键 | integer key |  |  |
| 真实数据 | real data |  | Use only inside the test VM under the project test rules. |

## Corpus and Archo

| 中文 | 英文（文档里用的） | 英文界面上的叫法（和文档不一样时才填） | 说明（一句话，可空） |
| --- | --- | --- | --- |
| 标注集 | labeled dataset |  |  |
| 错误范式 | error pattern |  |  |
| 对象引用 | object reference |  | Stable references accepted by commands and tools. |
| 堆栈形状 | stack shape |  | Keep the identifier StackShape unchanged. |
| 范式 | pattern |  | Use pattern, rather than paradigm, for recurring error shapes. |
| 范式库 | error pattern library |  |  |
| 覆盖率 | coverage |  |  |
| 工具层 | tool layer |  | The shared commands used by CLI, MCP and Archo. |
| 归一化 | normalization |  |  |
| 回退范式 | fallback pattern |  |  |
| Mod 特定范式 | Mod-specific pattern |  | A time-sensitive pattern tied to a Mod or version. |
| 模型提供方 | model provider |  |  |
| 内置助手 | built-in assistant |  | Archo, when referring to the planned built-in assistant. |
| 评测集 | evaluation dataset |  |  |
| 权限边界 | permission boundary |  |  |
| 去重 | deduplication |  |  |
| 任务授权 | task authorization |  |  |
| 日志语料 | log corpus |  |  |
| 通用范式 | generic pattern |  | A pattern independent of a particular Mod. |
| 图片识字 | optical character recognition (OCR) |  | Keep OCR in identifiers and commands. |
| 消息模板 | message template |  |  |
| 修法 | fix |  |  |
| 修法证据 | fix evidence |  |  |
| 修改类工具 | mutation tool |  |  |
| 语料 | corpus |  | Player-shared log data used for research and calibration. |
| 语义接口 | semantic interface |  |  |
| 诊断路线 | diagnostic route |  |  |
| 只读工具 | read-only tool |  |  |
