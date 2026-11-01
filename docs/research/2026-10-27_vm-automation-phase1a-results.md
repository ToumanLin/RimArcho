# VM 自动化 Phase 1a 实测（2026-10-27）

本报告记录宿主 Windows 上的实现与实测；运行步骤由 [Phase 1a runbook](../../scripts/vm/phase1a.md) 维护，验收依据是 [方案 §§4–7、§11](2026-10-27_vm-automation-test-plan.md)。完成 runner 不等于应用验收全绿：暂存撤销 oracle 保留失败，性能超预算也单独使退出码为 1。

## 交付与隔离

工作分支 `codex/vm-phase1a`，worktree 为 `RimArcho/.claude/worktrees/vm-phase1a`，基点 `c4adf74`。代码与提交只在该 worktree；构建、DB、截图、轨迹和浏览器 profile 全部在仓库外。

- CLI 新增 `loadout complete-dependencies`、`loadout replace-recommended <ref…>`、`loadout use-installed <packageId…>`，直接调用 GUI 的共享 app 方法；批次先重验，每次一撤销步骤。`sort [--dry-run] --json` 和文字输出使用共享 `app.SortNotices`。命令测试包含拒绝批次无半改、单步 undo、JSON/GUI 源一致及 dry-run 无写。另用构建后的 CLI 在磁盘 fixture 上逐项运行、undo、独立读取 workspace/undo depth，三项通过（私有证据 `D:/vm-phase1a-cli-repairs/cli-repairs.json`）。
- CDP 支持 Space、左击、Shift 大小写、长按 repeat、真实双／三击和超时／断线拒绝；输入轨迹保留实际送达时间、选区、焦点、编辑队列和事件。20/50/100ms 是调度目标，CDP 往返及渲染可能拉长实际间隔，以 trusted input 轨迹为准。
- 最终 fixture 有 36 份实例（两个 Core、33 个 extra、一个 Workshop）、两个游戏版本及三个方案，含依赖、未知／远程缺依赖、已安装副本和推荐替换。100 选项在小库明确标不足；真实库执行 1/10/100。
- canonical snapshot 独立只读 SQLite，归一化来源／路径／包名／内容指纹与问题归属键；保留所有方案的顺序、enabled set、暂存位置与锚点、完整 undo/redo、保存版本、标签／备注／规则、配置 hash/XML 和链接。W 的启用顺序另与独立 DB 观察核验。
- settled 要求页面内容和导航完整、无 loading/error/env busy、编辑队列清空且连续状态稳定。持续采样检查白页、假无游戏、侧栏丢失、错误、问题 sequence 倒退与结果被 loading 覆盖；fresh 后核对 Needs/ListProblems/Board 的上下文、计数和每 Mod 归属。
- 订阅双／三击真实提交到本进程拥有的 loopback TLS 失败端点，断言每组一个明确失败的任务、方案／实例不变；成功下载去重不在该证据的结论内。退订在真实 Workshop 行上仅取消。原生覆盖导出用 PID 限定的 Windows 控件辅助程序选择 No、再 Cancel，核对已存在文件的 sentinel 字节。
- discovery 记录 112 个最终页面候选（43 个可命中，证据 `D:/vm-phase1a-results/s3-W-lBIWzo/controls.json`），仅保存最终页面控件的语义键、边界和可命中状态；不是完整状态图 crawler。VM hook 已文档化，本轮未启动／还原 VM，也未启动 RimWorld。

真实数据供给使用 `sqlite3 -readonly … ".backup …"`，复制游戏、共享库、userdata 与 23 个外部 junction 目标，再对复制 DB 的有类型路径列迁移；在 App.Open 前审计解析后的路径，扫描后再次审计。必须保留嵌套 SteamCMD 来源，否则发现过程会把原嵌套目录再注册为新来源。对已发现的嵌套 root 不再调用会拒绝重叠的 folder set；该偏离方案的实现选择由迁移前审计、无变化扫描和迁移后审计验证，未做全库字符串替换。

私有基线 `D:/vm-phase1a-real-baseline`；环境扫描 4112 live、55 tombstone、756539 文件、约 42.6GB，完整 DB 共 7878 实例记录、5 方案，默认测试列表 481 项。环境切换独占另一个完整副本 `D:/vm-phase1a-results/s2-W-YpnfF9/case`。基线及独占副本最终扫描均为 changes: none。最终 preparer 还从小型 fixture 完整执行 backup→copy→typed relocation→pre-open audit→env validate→scan→audit，输出 `D:/vm-phase1a-preparer-check`，通过。

只读取原始 APPDATA home；测试应用的 RIMARCHO_HOME、APPDATA、LOCALAPPDATA 全部隔离，端口已有监听时拒绝接管。没有将私有 Mod 内容或数据表写进报告。

## 应用发现

### F1：列表切换丢失最后一次点击（已修复）

重现：在 A 显示时点 B，OpenPlan 尚未完成就点 A。旧逻辑认为 A 已显示，跳过后一次请求，最终留在 B。fixture W2 的 trusted pointer trace 确认两次真实点击；用延迟 mock 执行原 navigation 源也能复现。`06fdadb` 保序处理显式 OpenPlan，并在完成后检查读取上下文；两个 Node 回归覆盖返回原列表与前请求失败。W2 的二十次交替方案点击通过。

### F2：暂存 undo 留下幽灵暂存（未修复，P0-2）

W/A/C 和真实 DB 副本均失败同一独立 oracle：从启用列表 stage 一项，再 undo，workspace entries 恢复，`stage:<plan>` 的 item/position/anchor 却未恢复。`internal/app/stage.go` 的 Stage 独立写 meta，Staged 只过滤已启用项，`internal/app/loadout.go` 的 UndoRedo 只恢复 entries。这是原有 P0-2 历史 payload 边界（当前规格的暂存列表本身不入 undo），不是本分支引入的回归；ghost 行为也说明“重新启用后离开暂存”没有持久收敛。保留失败，没有把 oracle 改成只比可见列表。

可见后果已在新建 fixture 上确认：stage→undo 后暂存显示 0；再普通 disable 同一项，它重新出现在暂存栏，带旧位置／锚点。独立重现命令（使用新的仓库外目录）：

```powershell
D:/vm-phase1a-tools/vmstate.exe --mode fixture --root D:/phase1a-stage-new
$env:RIMARCHO_HOME='D:/phase1a-stage-new/home'
$env:APPDATA='D:/phase1a-stage-new/appdata'
$env:LOCALAPPDATA='D:/phase1a-stage-new/localappdata'
D:/vm-phase1a-tools/archo.exe stage add pkg:vm.dependent
D:/vm-phase1a-tools/archo.exe undo
D:/vm-phase1a-tools/archo.exe stage list
D:/vm-phase1a-tools/archo.exe disable pkg:vm.dependent
D:/vm-phase1a-tools/archo.exe stage list
```

### F3：快速空格触发布局观察循环（已修复）

最终 fixture 首轮 `D:/vm-phase1a-final/s1-W-yiUNPp` 在 1 项／100ms 的 Space burst 期间记录两次 `ResizeObserver loop completed with undelivered notifications`（观察器起点后 8649ms、8669ms）。列表状态和 undo 断言仍通过，但 no-error oracle 正确失败；未屏蔽浏览器错误。列表 resize callback 同步 renderRows 会改变被观察的滚动布局；`eb4fb78` 将它合并到下一 animation frame。`752f18a` 执行实际 lists 源，验证 resize delivery 内没有布局写且同列通知合并；修复后真实窗口重复结果列在下表。

### F4：真实规模性能超预算（未修复）

冷启动含启动提示 Esc、打开默认 481 项列表及稳定等待；多轮约 5.17–6.28s，超过千级库 5s 门槛。真实 W1 的 `s1-W-gCWSim` 还记录到 624ms、980ms 采样停顿，超过 500ms；尚未定位是应用哪段渲染／宿主调度导致，不宣称根因已证明。`s2-W-hgMW6z` 一次切换环境／切库页超过 15s，超时时 env busy 仍在；后续重跑 `s2-W-h3impU` 两次切换 3.09s、3.72s，功能通过，保留首次超时证据，未提高通过阈值。

## 驱动问题与应用问题的区分

真实库早期 `s4-W-nwRrWf` 在 move→undo 后无法点击 index 1：目标行已在虚拟列表视口外，是驱动定位错误。`712ccd2` 改为真实数字键聚焦、Home、ArrowDown，重跑后完成并暴露 F2。早期 settled 漏等编辑队列、供给时嵌套来源重复注册／路径大小写历史键碰撞也已修正；最终备份迁移端到端检查和无变化扫描通过。这些失败没有计为应用缺陷，也没有把相应未跑到的分支算通过。

## 验收记录与预算

宿主 AMD Ryzen 9 7950X（32 逻辑核）、约 63.1GiB RAM，D: 为 Nextorage SSD NE1N2TB NVMe；GPU 列表含 RTX 4090（驱动 32.0.16.1088）、AMD 集显和 USB 虚拟显示器，未固定 WebView 的实际 GPU。Node v22.17.0。每轮 result.json 记录源 commit、工作区状态、三个／四个工具 SHA-256 与内容规模；本轮是本机登录桌面、真实 Wails，非 guidev 伪装窗口。

`go test ./...` 完整通过（最终重建后的 GUI 包 11.55s），6 项 Node 协议／导航／布局回归通过。原始协议 mock 只验证驱动边界；下列 W 证据来自真实窗口。


修复后 fixture W1 五轮的功能 oracle 全通过、未再观察到 ResizeObserver 错误；四轮整体 pass，一轮仅 cold-ready 2.099s 超过 2s。私有证据根为 `D:/vm-phase1a-resize-verified`，五轮目录：`s1-W-auGKla`、`s1-W-UdiQs2`、`s1-W-ackVQg`、`s1-W-y9ZCXO`、`s1-W-BSgM2o`。小库 100 选项显式 unsupported，不能算覆盖。

最终 C/A fixture 矩阵（`D:/vm-phase1a-final`）：

| 场景 | C | A | W（布局修复前矩阵；W1 修复后结果见上） |
| --- | --- | --- | --- |
| 1 | `s1-C-F61GgV` pass | `s1-A-gLzvzo` pass | `s1-W-yiUNPp` 发现 F3，数据／undo 断言通过 |
| 2 | `s2-C-SfQ87x` pass | `s2-A-UrtbDV` pass | `s2-W-ans1CC` pass |
| 3 | `s3-C-sElKYy` pass | `s3-A-Pbc3o0` pass | `s3-W-NvoI3T` pass，含订阅失败提交去重 |
| 4 | `s4-C-RaAPxY` F2 | `s4-A-Aqhnyq` F2 | `s4-W-qb5M2e` F2，其余取消／undo／配置检查通过 |

C1 为 sort 历史，A1 另含 toggle；C2 只有只读上下文投影，A2 另测并发 Workspace request/event，均不声称覆盖 W 的快速键或导航。C/A/W 的场景3六个共同 checkpoint 完全一致，包含 canonical 问题计数／每 Mod severity；场景4 C/A 九个 checkpoint 也一致，但两者均失败 F2，说明差分一致不足以证明正确。比较证据为 OS temp 下 `RimArcho-vm-compare-Uo9K8o`、`RimArcho-vm-compare-medlSa`、`RimArcho-vm-compare-ELjcpc`。

修复后真实库结果（`D:/vm-phase1a-resize-verified`）：

| 场景 | 结果／发现 | 私有目录 |
| --- | --- | --- |
| 1 | 功能通过，1/10/100 实选与所有 cadence 完成；cold 5.174s；4 次采样间隙 516–682ms，性能失败 | `s1-W-LLCY7B` |
| 2 | 功能通过，环境切换并返回、最后方案／页胜出；cold 5.495s 超预算 | `s2-W-ZNL6Zi` |
| 3 | 功能通过，重复 sort、启用／导入／订阅提交双三击；sort 1.141s；cold 5.300s 超预算 | `s3-W-2LAygR` |
| 4 | 仅 F2 功能失败；删除／退订／目录移动／原生覆盖取消通过；cold 5.244s 超预算 | `s4-W-Moa3vX` |

| 指标 | 实测 | 预算 | 判断 |
| --- | --- | --- | --- |
| cold-ready（small，五轮） | 1.450–2.099s | 2s | 1 轮 2.099s 失败，其余通过 |
| cold-ready（real，四场景） | 5.174–5.495s | 5s | 4 轮失败 |
| W1 burst 后 stable（small，五轮） | 0.379–0.877s | 10s | 通过 |
| W1 burst 后 stable（real） | 2.214–5.882s | 10s | 通过 |
| W2 导航 burst 后最终页 stable | 0.525–2.427s | 5s / 硬15s | 通过；不是单页热读测量 |
| W2 两次环境 switch ready | 2.509–2.955s | 5s / 硬15s | 通过；早期一次15s失败仍保留 |
| W3/W4 sort→stable（real） | 1.141–2.933s | 5s | 通过；真实列表已排序时验证幂等，实际排序 undo/redo 的共同差分由 unsorted fixture 覆盖 |
| W4 删除方案确认 ready | 143.875–164.908ms | 200ms | 通过 |
| W4 dialog close（cancel/Esc/backdrop） | 45.094–64.289ms | 200ms | 通过；原生 helper 耗时独立于该组 |
| fresh problem publication 等待（real） | 0.733–8.651s | 120s | 通过 |
| ListProblems hot read（real，每场景5次） | 22.716–97.892ms | 100ms | 20 个读样本均通过，不把混合上下文样本称为性能 P95 |




原始最终证据含每个 checkpoint 的 domain/surface、result.json、window-trace.json、fresh-problems.json、进程日志及截图；截图／DB／profile 仅在上述私有外部目录。真实 W1 完成 234 项检查、W2 30 项、W3 154 项、W4 的唯一失败为 F2。性能轮未开 refnums；发现控件另跑。四个最终 W 运行未观察到白页、假无游戏、侧栏丢失、console/script error 或后台 loading 覆盖成功结果。

## 限制

这不是 P95 或 VM 性能结论。保存每轮原始样本；冷启动和不同动作样本不能混成分位数。DOM 首次 mutation 只是反馈代理，任意 no-op／后台更新可能错误关联到上个输入，因此只对可控导航输入强制 200ms，其余保留诊断；删除方案打开、弹窗关闭另有明确终点计时。native helper 操作原生控件，CDP trusted-input 结论只涵盖 WebView 输入。

A/C 仅做有领域对应的操作，不能代替真实输入、菜单和确认取消。snapshot/比较覆盖本阶段列表和配置；完整文件 manifest、下载成功／credit／破坏后文件恢复、全 crawler、游戏加载仍属后续场景。当前取消检查的源／实例／链接／配置相等并不等于逐文件内容 hash 相等。原始真实数据不会提交或上传。

## 复跑

在指定 worktree 中，构建和完整参数见 [runbook](../../scripts/vm/phase1a.md)。本机已有工具和基线时：

```powershell
Set-Location D:/Game/RimModManager_Upgrade/RimArcho/.claude/worktrees/vm-phase1a
$tools='D:/vm-phase1a-tools'
$out='D:/vm-phase1a-rerun'
foreach ($driver in 'C','A','W') {
  foreach ($scenario in 1..4) {
    node scripts/vm/run.mjs --scenario $scenario --driver $driver --tools $tools --out $out
  }
}
foreach ($scenario in 1,3,4) {
  node scripts/vm/run.mjs --scenario $scenario --driver W --tools $tools --out $out --baseline D:/vm-phase1a-real-baseline
}
node scripts/vm/run.mjs --scenario 2 --driver W --tools $tools --out $out --baseline D:/vm-phase1a-results/s2-W-YpnfF9/case --owned-files true
node --test scripts/vm/cdp.test.mjs scripts/vm/navigation.test.mjs scripts/vm/lists.test.mjs
go test ./...
```

`--owned-files true` 只用于上述已经审计的独占、可丢弃全副本，不能用于真实 home，也不能与其他消费者并发。省略时 W2 自动复制完整内容。`--port 9222` 或环境变量 `RIMARCHO_DEVTOOLS_PORT=9222` 可固定真实窗口端口；默认选择空闲 loopback 端口。退出 1 包含功能或性能失败，先读 result.json 两个 failure 数组，不将预期暴露的 F2 记为通过。

## 提交

| 提交 | 内容 |
| --- | --- |
| `990b8c5` | P0-1 CLI、共享 sort notices、原子撤销与拒绝测试 |
| `06fdadb` | 修复最后一次 plan 点击丢失 |
| `8b39afe` | 保持共享 notice 空字段省略合同 |
| `f6dbde2` | fixture、canonical oracle、C/A/W runner、复制审计及 VM 文档 |
| `f6f7d3d` | 保持嵌套来源、合法多选与真实双／三击 |
| `a9f1f93` | 超时失败不被诊断等待抹掉 |
| `712ccd2` | 虚拟列表用真实键盘定位离屏行 |
| `f626743` | 归一化问题归属 map 的 instance key |
| `8e5dcca` | 清除 CLI 对照表中的陈旧缺口描述 |
| `3878682` | 订阅重复提交、显式退订取消、原生覆盖取消、动作计时及问题差分 |
| `eb4fb78` | 修复 ResizeObserver 同步重绘循环 |
| `752f18a` | 实际 lists 源的 resize 交付回归及运行文档 |
| `f300951` | 非空 editor 列必须有真实 DOM 行，空容器不冒充内容 |

