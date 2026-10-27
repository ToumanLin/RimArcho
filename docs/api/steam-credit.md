# Steam credit：GUI API 合同

后端入口在 `internal/surface/gui/credit.go`、`subscribe.go`；业务规则归 [02 §3.4](../specs/02-环境身份与扫描.md#34-订阅git下载页steamcmd2026-10-24)。这是 Wails GUI 方法，错误沿用现有 Promise reject；没有新增 HTTP 路由。此改动未实现前端弹窗或修改 JS/CSS。

## 全部订阅

```go
Subscribe(r SubscriptionRequest) (string, error)
SubscribePreview(r SubscriptionRequest) (*view.Preview, error)
```

`SubscriptionRequest` 完整 JSON 结构（既有字段保持兼容，只有 `batch` 是新增）：

```json
{
  "kind": "workshop",
  "url": "8100 8200",
  "refType": "",
  "ref": "",
  "git": false,
  "from": "",
  "steamcmd": true,
  "batch": true
}
```

`kind` 为既有的 `workshop/git/page`。`url` 在 workshop 中是空格分隔的 ID 或工坊 URL；`refType/ref` 用于 Git；`git/from` 用于下载页。省略布尔字段等于 false。`batch` 只在 workshop + steamcmd + 实际执行时生效，预览不建批次。返回值仍是“任务已排队”的本地化字符串，失败通过任务结果呈现。一个请求记录一个业务批次，只有本次下载成功且扫描为 SteamCMD 实例的 ID 入账。

导入 getAll、关注合集、需要处理页批量 subscribe，先显示两种下载方式，再调用 `Subscribe({kind:"workshop",url:ids.join(" "),steamcmd,batch:true})`。其它调用可以不传 batch。

## 查询与回答

```go
CreditPrompt() (*app.CreditPromptData, error)
CreditAnswer(batchIDs []int64, replace bool) (*CreditAnswerResult, error)
```

`CreditPrompt()` 无参数，返回 JSON 对象或 `null`：

```json
{"batchIDs":[1,2],"modCount":14,"examples":["Harmony","Camera+","Dubs Break Mod"]}
```

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| batchIDs | number[] | 当前到期批次，按创建顺序；回答时原样回传 |
| modCount | number | 仍为 live SteamCMD 实例的 Mod 数，跨批次按 Workshop ID 去重 |
| examples | string[] | 最多三个名字，用于解释询问范围 |

没有到期批次、没有仍可交接的原副本、Steam 当前不可用、任一已登记游戏运行时返回 `null`。暂时不可用不会消费询问。查询本身不改变批次状态；前端需要合并并发查询、同一时间只显示一个弹窗。游戏退出、窗口回到前台，以及应用首次就绪时调用；有弹窗期间不要再次开弹窗。API 失败保留用户尚未提交的答案。

Wails 调用顺序是 `CreditAnswer(prompt.batchIDs, true)` 或 `CreditAnswer(prompt.batchIDs, false)`，不传请求对象。正常返回：

```json
{"batchIDs":[1,2],"state":"replacing","taskID":"t1793000000000-1"}
```

拒绝返回：

```json
{"batchIDs":[1,2],"state":"dismissed","taskID":""}
```

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| batchIDs | number[] | 本次提交的批次 ID |
| state | string | `dismissed` 或 `replacing`；重复接受的批次已处理完时可为 `done` |
| taskID | string | 对应后台任务 ID；拒绝或已处理完为空字符串 |

空 ID、未知批次、尚未到期或试图反转 dismissal 会 reject，整个回答不部分提交。重复 dismissal 幂等；重复接受正在 replacing/done 的批次不会另建订阅义务。回答接受先落盘再启动任务；即使之后启动失败，下次启动仍从 SQLite 恢复。多个接受操作可共享同一个正在运行的任务 ID。只在 API 成功后关闭本次弹窗；拒绝后这一批永久不再返回。

## 后台任务、进度与取消

任务种类 `steam-credit-replace`，lane 为 `self`。沿用现有 `Tasks()` 的 `view.TaskPage`，读取对应 `rows`；本任务使用现有字段：

```json
{
  "id":"t1793000000000-1",
  "kind":"steam-credit-replace",
  "lane":"self",
  "state":"run",
  "title":"用 Steam 订阅替换 SteamCMD Mod",
  "obj":"SteamCMD Mod",
  "line":"等待 Steam 安装完成或游戏退出后替换",
  "count":"3 / 14",
  "pct":21,
  "bar":true,
  "items":[{
    "ref":"c4fe1092",
    "name":"Harmony",
    "state":"ok",
    "stateText":"完成",
    "rank":4,
    "why":"已切换到工坊里的这一份，SteamCMD 副本已移入回收站",
    "size":"",
    "bytes":0,
    "got":0,
    "downloadSize":0
  }],
  "acts":[{"id":"stop","label":"停止剩下的","key":"x"}]
}
```

以上只列本任务消费的既有 TaskRow 字段；其它既有字段不变。总数计入所有记录项，进度是终态项 / 总项，包含明确失败和保留副本；不伪造 Steam 下载字节百分比。等待期间单项 `wait`，当前处理项 `run`，完成/保留 `ok`（原因看 `why`），不可恢复错误 `fail`。任务结束 `done/part/fail`，停止 `stop`。失败保留原始 EResult 和工坊页，未完成项目留在 SQLite。

事件名只有现有 `tasks`：变化时 payload 为 `{}`，最多每 500 ms 通知一次；结束时立即发送：

```json
{"ended":{"ok":false,"id":"t1793000000000-1","kind":"steam-credit-replace","ref":"","text":"本地化任务结果"}}
```

`ended.ok` 仅在任务 state 为 `done` 时为 true。收到变化后重新 `Tasks()`，不从事件推断逐项状态。没有新增 `credit-progress` 事件。

停止调用 `TaskAct("stop", [taskID], "")`，在当前有界 Steam 调用或单项交接边界生效；继续调用 `TaskAct("resume", [taskID], "")`，也可用 `CreditAnswer(batchIDs,true)` 重新唤起尚未完成的义务。停止不撤销订阅、已替换方案或已回收的副本，不再次弹 credit 询问。下次 GUI 启动自动续跑；没有常驻进程。任务历史可以清理，不影响续跑。明确的永久失败不因“继续”而重试。

## 前端文案

前端沿用英文句子作 key（`T()` / `Tf()`），中文登记在 `internal/view/subscriptionstext.go`；实现见 `assets/js/credit.js`。

| 英文 key | 中文 |
| --- | --- |
| from Steam (slow, but credits the modder with a subscription; Steam manages it) | 从 Steam 订阅（慢，但订阅能给作者带来订阅数，由 Steam 管理） |
| from SteamCMD (fast, but does not credit the modder; RimArcho manages it) | 从 SteamCMD 下载（快，但不计入作者的订阅数，由 RimArcho 管理） |
| Do you want to credit the modders by subscribing from Steam? | 要在 Steam 上订阅，给作者带来订阅数吗？ |
| %d mods, such as %s | 共 %d 个 Mod，例如 %s |
| replace these SteamCMD mods with Steam subscriptions, little by little | 在后台逐步把这批 SteamCMD Mod 换成 Steam 订阅 |
| dismiss | 不用了 |

## 限制与验证

历史、撤销、整合包或链接引用会让旧副本保留，故不承诺最终一定释放所有 CMD 空间。保留项的处理已结束，不后台反复回收。真实客户端可能重新下载，下载/回收站都可能暂时占用双份空间。Steam 使用当前客户端登录账户；没有新增账号绑定或账号切换确认 API。

单元测试使用假的 Steam/SteamCMD runner，不接触真实账号；覆盖创建门槛、成功筛选、会话去重与实例匹配、合并/延后/永久拒绝、重试/暂停/取消、启动竞态、跨游戏部署续跑、失败与引用保留、JSON 和 GUI 任务恢复。真实账户订阅与真实游戏中的交接仍需集成验收。
