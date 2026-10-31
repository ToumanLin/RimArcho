# ToDo

待研究、还没定的事。定了就写进规格（实现计划放 01 §5），研究过程放 docs/research/。

## 研究：archo.db 里的文本 ID 和路径改成整数

**起因**：2026-10-29 只读测量了一个用了 10 天的库，443 MB。其中分析事实和文件清单里，大量空间花在重复的文本上：

| 表 | 行数 | 重复文本 |
| --- | --- | --- |
| `def_fact` | 76 万 | `file` 共 33.6 MB（平均 44 字节）；`instance_id` 共 19.8 MB（26 字节），还在两个索引里各存一遍 |
| `snapshot_file` | 98 万 | `rel_path` 共 59.6 MB，占表的三分之二；其中 72% 的行只用于增量扫描的 mtime 比较 |
| `type_fact`、`harmony_*` | 20 万 | 同样带 `instance_id`、`file` |

**粗估**：`instance_id` 换成整数代理键，`file` / `rel_path` 换成路径表里的整数，剩下的事实表还能再小三成左右。

**要回答的问题**：
- 代理键放在哪：`instance` 加整数列，还是事实表单独一张映射表？还要考虑 `id_alias`、身份合并（same 回答）、Lineage 拆分，以及迁移时键会不会变。
- 路径表的粒度：全局一张、按 Mod 一张、还是按快照一张？Mod 更新后旧路径何时清理（和旧快照文件清单的保留规则一起看）。
- `snapshot_file` 的非语义文件是否改成每个快照一个压缩块，只在增量扫描时整块读。需要先确认 `files.go`、贴图（`textures/%.png`）、`languages/` 这些按路径查的查询怎么办。
- 未启用的 Mod 是否只存 Def 的名字层（类型、defName、Name），不存 `file` / `line` / `parent`。需要先确认 `target-inactive`、缺失父模板的提供方等功能是否需要完整字段。
- 查询和页面加载会不会变慢（06 §10、07 §6），迁移要多久（以 76 万行事实、98 万行文件清单的规模实测）。

**前置**：先合并事实跨版本共用（`codex/fact-share`）和自动整理（`codex/db-maintenance`），在它们之后的结构上测量。

## 列表编辑器`用户排序规则`, 文件搜索页面
RimArhco有`用户排序规则`, `rg搜索`功能, 只是我们没有做任何GUI入口给它

## 计时器和定期行为的重新整理
在规格里加一张定时行为总表
1. 内存释放计时器
2. 自动检查更新:(扫描mod库, 检查变动, 拉取github,loverslab等上游的更新, 自动更新社区数据, 依赖项, RimArcho自己的自动更新)

## RimArcho自动更新
需要做RimArcho的自动更新 

## TUI
TUI不再渲染图片, 不再使用WezTerm
TUI的界面应该更新和GUI尽量一致
增加输入框允许玩家输入指令

## 设置页
增加反馈选项, 反馈内容可以发送到github issue
增加社区链接, 如Discord, github
设置页面重新排序, 将常用的放到一起

## 本地化
docs应该做成英语, 项目中除了本地化和词表外不留中文;
需做中文ReadMe

## MCP
精简MCP指令, 做到渐进式披露

## Github static check doesn't pass
56s
Run go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...
  go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...
  shell: C:\Program Files\PowerShell\7\pwsh.EXE -command ". '{0}'"
go: downloading honnef.co/go/tools v0.8.2
go: downloading golang.org/x/tools v0.51.0
go: downloading golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678
go: downloading github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c
go: downloading golang.org/x/mod v0.41.0
go: downloading golang.org/x/exp v0.0.0-20231110203233-9a3e6036ecaa
Error: internal\app\autoupdate_test.go:122:29: should use constant http.StatusServiceUnavailable instead of numeric literal 503 (ST1013)
Error: internal\app\autoupdate_test.go:199:29: should use constant http.StatusServiceUnavailable instead of numeric literal 503 (ST1013)
Error: internal\app\autoupdate_test.go:248:29: should use constant http.StatusServiceUnavailable instead of numeric literal 503 (ST1013)
Error: internal\app\workshopsub.go:463:6: func steamWorkerProcess is unused (U1000)
Error: internal\logs\phase_test.go:10:6: func phaseTotals is unused (U1000)
Error: internal\store\content_test.go:83:5: identical expressions on the left and right side of the '||' operator (SA4000)
Error: internal\store\factsets_test.go:240:3: should use t.Fatalf(...) instead of t.Fatal(fmt.Sprintf(...)) (S1038)
Error: internal\store\migrate26_test.go:197:3: should use t.Fatalf(...) instead of t.Fatal(fmt.Sprintf(...)) (S1038)
Error: internal\surface\gui\gamebranches_test.go:76:27: empty critical section (SA2001)
Error: internal\surface\gui\readers.go:268:2: empty critical section (SA2001)
Error: internal\view\problemview.go:98:27: do not pass a nil Context, even if a function permits it; pass context.TODO if you are unsure about which Context to use (SA1012)
exit status 1
Error: Process completed with exit code 1.

## Archo Agent
Archo 要做的是能动手的 Mod 开发 agent，不是翻译、打标签这类小功能 
