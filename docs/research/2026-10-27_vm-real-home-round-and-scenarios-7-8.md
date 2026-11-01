# VM 真实 home 轮次、情景 7–8（2026-10-27 晚）

分支 `dev/0.9.0`，起点 `655444b`。接 `tmp/handoff-2026-10-27.md` 的待办 1、2、3（7、8）、6。

## 1. VM 脚本改成直接用真实 home（`f5fa61c`、`5cb0fc4`）

原来的 `--baseline` 要求 guest 里的 22 GB 复制基线，已经删了。现在：

- `player-vm.ps1 -RealHome`（runner `--real-home true`，只在 guest 里允许）直接用 Player 的 `%APPDATA%\RimArcho`、APPDATA/LOCALAPPDATA 和 Steam 内容。
- 开跑前用 SQLite 备份 `archo.db`，并复制 `config.toml`、`tasks.json`；应用退出后还原。这样种子 1–5 每轮从同一状态开始。快照还原仍是整轮结束时的最终复位。
- 爬虫分支和缩减重放（子进程）用这份备份建私有 DB（`--start-db`），因为父进程的窗口还开着。
- 内容写入（删除、移动 Mod、部署）照旧拦下；只允许 DB 内的列表/标签操作。
- 运行期间界面语言设为英文：走查器和控件分类按英文文字识别（旧基线的准备脚本也这样设）。还原 DB 后语言回到玩家原来的设置。
- 宿主先用生成的 fixture 冒充真实 home 验证：walk/crawl 都通过，跑完 DB 的 `.dump` 与开跑前逐字节一致，中文设置也还原了。

## 2. 随机玩家，VM 真实数据，种子 1–5 × 200 步

VM 快照 `RimArcho-Real`，真实 home：1271 个实例，1 个列表（default）。`-Allow local`，每个种子上限 20 分钟。证据在 `D:\RimArcho-test\transfer\vm-player-walk-*\evidence`。

**第一轮**（`f5fa61c`）：五个种子都跑到收尾（179/162/200/20/200 步），每轮真实 home 都还原成功。之前卡住所有种子的 F2 没有再出现，也没有性能失败。所有失败都来自工具：收尾时在命令面板找英文“undo”，而真实 home 是中文界面。修复见 `5cb0fc4`。

**第二轮**（`5cb0fc4`）：

| 种子 | 步数 | 用时 | 结果 |
|---|---:|---:|---|
| 1 | 200 | 232 s | 通过 |
| 2 | 200 | 223 s | 撤销后暂存不一致（标为 F2）→ 见下 |
| 3 | 200 | 214 s | 功能通过；一次首反馈 297 ms（预算 200 ms） |
| 4 | 200 | 433 s | 通过 |
| 5 | 200 | 241 s | 通过 |

种子 2 的“F2”是走查器的误报：收尾时它找“entries 与开头相同的最新一步”撤销到那里。但只暂存一个未启用的 Mod 不改 entries，所以“暂存后又做别的编辑”时，最新匹配的一步暂存还是满的。在 fixture 上用 CLI 复现：`stage add` 未启用项，再 `enable` 另一项，两个撤销步的 entries 相同，只有 staging 不同。`26a4307` 改为优先找 entries 和 staging 都相同的一步。用这个版本重跑种子 2：200 步通过（465 s）。

种子 3 的 297 ms 是从“游戏”页切走的一次页面切换，990 个反馈样本里唯一超预算的一个，未深究。

结论：F2 修复之后，五个种子在真实数据上都没有发现应用 bug；暂存撤销在真实数据上表现正确。

## 3. 待办 6：编辑器的行和侧栏（`597bc1b`）

- 列表里没装上的条目：以前只显示 packageId。现在显示名字（导入时带的名字 → 曾装过或别的游戏里的副本 → 本地 steamDB 索引 → packageId），同一行写“未安装”；如果有已安装的副本可以顶上，写“有已安装的副本”。文件夹已被删除的 Mod（墓碑）也写“未安装”。
- 启用栏里多副本 Mod 的来源（“└ workshop”那一行）挪到名字同一行，行高恢复一行。
- Shell 的 `error` 字段：Shell 读取失败（后台任务占着数据）时会返回带 error、没有列表的 shell。以前编辑器的 Workspace 读取会直接用它，侧栏的列表就没了。现在两条读取路径都保留原侧栏，再读（最多 10 次）。

## 4. 情景 7：导出 8 种 × 再导入

CLI 驱动：对当前列表逐个格式 `loadout export`，再 `loadout import --new`，比较条目（packageId 顺序与实例）。

宿主生成 fixture（5 项，含 2 个未解析条目）：archo、text、modsconfig、rimsort、rml、rimcrow、rimcrow-xml 七种往返后 packageId 顺序和实例都一致。workshop 链接格式：fixture 里没有工坊 Mod，导出文件只有注释，再导入报“文件里没有 Mod”，属预期。

VM 真实 home（`3e8dae8` 的构建，CLI，跑完还原快照）：

- default 列表（6 项）：七种格式往返完全一致；这个列表没有工坊 Mod，workshop 格式同上。
- 用 CLI 临时建的 `rt-big`（工坊 Mod 加补齐依赖，42 项；原想取 470 个，但 `archo list` 的输出被截断，只取到 30 个）：八次导出加八次导入共 2.1 s。
  - 七种格式：没有丢任何条目，顺序一致；每种再导入都多出一项 Archo Monitor（排第 2）。这是“新列表默认加 Monitor”（04 §1.5），不算 bug。
  - workshop 格式：丢了 Core、Royalty、Biotech（格式只记工坊条目），同样多了 Monitor。结果是一个没有 Core 的列表。格式本来就有损，但导入后是否该自动补回 Core/已拥有的 DLC，需要决定。

另外在 VM 里查了编辑器缺失行用到的 steamDB 索引：guest 和宿主都没有下载 steamDB，查不到就直接回退到 packageId。下载过的玩家每个进程解析一次索引；按 4.5 万条（4.4 MB）模拟，解析约 13 ms。

GUI 部分（预览取消、覆盖确认、合并后撤销）没有做。

## 5. 情景 8：排序后的修复弹窗

在宿主 fixture 上用 guidev + 浏览器真实点击。fixture 同时有：已安装未启用的依赖、要从工坊获取的依赖、有可用副本的未解析条目、已安装的推荐替代、找不到来源的条目。

- 弹窗五类都列出；Esc 取消不撤销排序；`u` 撤销排序正确。
- “修复 N 个并获取 M 个”先做本地修复，再弹 Steam/SteamCMD 选择；在选择框按 Esc 不会订阅（没有点订阅）。
- 每项修复一步撤销，连按撤销回到排序前。

找到并修复（`3e8dae8`）：

1. **推荐替换与依赖互相打架。** `dependent` 硬依赖 `vm.old`，弹窗却建议“old → new”。同一次点击先把 old 换成 new，接着“补齐依赖”又把 old 加回来，结果两个都启用。现在当别的已启用 Mod 按 packageId 依赖原版时，不再提出这个替换（`canReplace` false，原因写明谁依赖它）。
2. **历史记录两条没翻译**：“complete dependencies and sort”“replace recommended mods”。

找到、未修（需要决定）：

3. **保存按钮写“没有变化”，其实保存还要建链接。** “使用已安装的副本”后，列表与上一版相比没有变化（同一个 packageId），按钮显示“没有变化”；但 `save --dry-run` 列出要新建的 `_Archo_` 链接，修复说明也写着“保存或启动时会创建所需链接”。按钮可以点，只是文案让人以为不用保存。要在编辑器读取时算待建链接，会增加 Workspace 读取的开销（F4 关注的路径），所以先不改。

## 6. 磁盘

宿主 `D:\RimArcho-test` 收尾时 464 MB（三轮 VM 证据合计 15 MB，情景 7 证据 34 KB）。每次生成的 fixture 用完即删。guest 每轮只留 5–8 MB 证据，C: 剩余约 60 GB。最后已还原快照 `RimArcho-Real`，确认 guest 里没有本次的运行目录，真实 home 的 DB 仍是 10-24 的版本。这次恢复时 VMware 没有弹那两个提示框。
