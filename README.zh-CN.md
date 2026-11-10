[English](README.md) | 简体中文

# RimArcho

<img src="docs/assets/rimarcho.svg" width="96" alt="RimArcho 图标：Archo 酱">

一个懂 RimWorld 玩家的一站式启动器。选 Mod、排顺序、检查能不能跑、启动游戏、出了错查明原因，都在一个地方完成。它面向装了几百个 Mod 的玩家，也面向和玩家一起工作的 AI 助手。

RimArcho 要回答三个问题：

1. **我现在在玩什么，它能正常跑吗？** 1.0 版回答这个问题。
2. **有什么没玩到？** 计划读取存档，找出从没碰过的 Mod 和内容。
3. **有什么值得玩？** 更远的计划：玩家互相分享整合包，AI 助手根据你没玩到的内容给出建议。

所有功能都能在没有 AI 的情况下使用。AI 助手是第二类用户：内置 AI 工作台和外部 agent（通过 MCP）拿到和玩家相同的事实、执行相同的动作，也遵守相同的规则。它们的改动可以撤销，未经确认不会写入游戏。（内置工作台计划在 1.1 推出；外部 agent 现在就能接入，见下文。）

同一套程序提供桌面窗口（`RimArcho.exe`）、终端界面、可用于脚本的 CLI 和 MCP 服务（`archo.exe`）。窗口和终端界面是同一套 Go 代码的两种表现：显示相同的列表、详情和“需要处理”项，执行相同的动作。CLI 和 MCP 输出相同的纯文本大纲，其中每个引用（`#id`、`pkg:`、`p:…`、`log:E3`）都能粘贴到另一条命令里使用。

它了解你的安装中的这些信息：

- **身份。** 每个 Mod 文件夹（工坊、本地 `Mods/`、SteamCMD、额外目录）都是一个实例。同一个 Mod 的副本组成一个谱系。每次扫描之间的变化都会记录：更新、移动、复制、重新出现、消失。
- **启用配置。** 编辑启用列表的工作区副本。每次编辑都能撤销，排序会说明约束，只有 `save` 才会写入游戏（先备份，再原子替换文件）。
- **问题。** 检查依赖、不兼容和加载顺序。按游戏的方式合并和继承 Def，模拟 PatchOperation，让失败带有原因（`target-inactive`、`stale-name`、`node-absent`…）。直接从 Mod 的程序集读取 Harmony 补丁。
- **日志。** 将 Player.log 的报错归因到 Mod。每个 Mod 都有一个角色（错误源、补丁方、依赖或被提及），同时给出证据、文件与行号，以及下一步。
- **代码。** 按需用 ilspycmd 反编译程序集，按 DLL 哈希缓存结果。

按要求执行的动作会写入游戏文件：`save` 写入 ModsConfig.xml，`copy-local` 在 `Mods/` 中新建文件夹。启用 SteamCMD 或额外目录里的 Mod 后，`save` 还会把它以 `_Archo_<source>_<folder>` 链接到 `Mods/`（目录联接，不需要管理员权限），停用后移除该链接。它只创建和删除带这个前缀的链接，从不写入链接目标。自身数据位于 `%APPDATA%\RimArcho\`。

切换环境时，也会先补齐目标 Mod 列表缺少的链接，再开始扫描：包括 Archo Monitor 等共用 Mod，以及上一个环境的本地 Mod。本地链接使用 `_Archo_env_<instance>`，更换方案后仍然保留。已有的本地副本会保留；切换不写入 ModsConfig.xml。

## 安装

1. 下载 `archo-<version>-windows-amd64.zip`，解压到任意位置，例如 `C:\Tools\RimArcho\`。
2. 可选：把该目录加入 `PATH`。
3. 可选：安装 ilspycmd，供 `archo source` 查看 C# 代码。需要 .NET SDK：
   ```
   dotnet tool install --global ilspycmd
   ```
   RimArcho 不会代你安装；缺少时会提示。

可执行文件没有代码签名。首次运行 `RimArcho.exe` 或 `archo.exe` 时，Windows SmartScreen 可能显示“Windows 已保护你的电脑”：选择**更多信息**，再选**仍要运行**。你可以自行构建（见下文），核对下载内容与源码是否一致。

### Linux（试验性）

从 1.0.9 起，每个版本另有 `archo-<version>-linux-amd64.tar.gz`：只有 `archo`，包含终端界面、CLI 和 MCP，没有窗口。它能找到原生、Proton 和 Flatpak Steam 的游戏，但目前只用合成数据测过，还没在 Linux 上用真实游戏测过，可能有问题，请先备份存档和 Mod 列表。解压到任意位置后运行 `./archo`。Linux 上 RimArcho 不会自动更新；`archo update` 会告诉你新版本和安装方法。纹理优化需要 `PATH` 上有原生的 `todds`。遇到问题请到 GitHub 反馈。

从源码构建（Go 1.27+）：运行 `go build -o archo.exe ./cmd/archo`；若要生成带版本号的发布压缩包，运行 `pwsh scripts/release.ps1 -Version x.y.z`。

## 首次运行

```
archo env detect                 # 查找游戏、用户数据目录和工坊目录
archo env add main --detected    # 注册为环境 "main"
archo scan                       # 读取每个 Mod 文件夹（之后增量扫描）
```

然后双击 `RimArcho.exe` 打开窗口，或运行 `archo` 打开终端界面（在启动它的终端中运行；双击会打开 Windows 默认终端）。`archo gui` 从终端打开窗口。窗口需要 Microsoft Edge WebView2，Windows 10 和 11 已自带。

接下来可以试试：

```
archo status                       # 概览：Mod、启用配置、问题数量
archo problems                     # 有哪些问题，按严重性降序排列
archo log                          # 归因最新的 Player.log
archo import rimcrow --db "D:\Game\RimModManager\data\mod_manager.db" --dry-run   # 从 RimCrow 迁移
```

`archo help` 列出全部命令。`archo <command> --help` 显示该命令的参数。

## 按键（窗口和终端）

两种界面遵守同一套规则：**方向键只移动**（光标、焦点、滚动），不会改动内容；**修改键立即作用于**选中项（或光标所在行），用 `u` 撤销。顶行始终显示当前位置。

| 按键 | |
|---|---|
| `↑/↓`、`←/→`、`1`–`4` | 移动光标；前往未启用、已启用、暂存、详情 |
| `PgUp/PgDn` | 在任意位置滚动详情 |
| `shift+↑/↓`、`ctrl+space` | 框选范围；选中或取消选中一项 |
| `space` | 启用（放到加载顺序要求的位置）/ 停用 / 从暂存放回 |
| `t`、`T` | 暂存（或放回）；全部放回 |
| `m`、`J` / `K` | 移到指定位置或某个 Mod 后；下移 / 上移 |
| `#` | 标签 |
| `enter`、`o` | 完整详情；工坊页面 |
| `S` / `w` / `L` | 排序 / 写入游戏 / 启动（都会先预览） |
| `u` / `ctrl+r` | 撤销 / 重做（窗口还支持 `ctrl+z` / `ctrl+y`） |
| `/` | 搜索列表：词语、`tag:`、`author:`、`type:`、`has:`、`-not` |
| `!`、`c` | 需要处理；最近变化 |
| `R`、`A`、`E` | 重新扫描；重新检查文件；切换环境 |
| `ctrl+k`、`?` | 命令面板；全部按键 |

方案编辑器和 Mod 库的筛选栏包含**位置**：已登记的来源目录（例如游戏的 `Mods/`、工坊，或某个 SteamCMD、额外目录）。可选多个位置，匹配其中任一位置；右键排除某个位置。位置与其他筛选组组合使用。搜索也接受 `location:"D:\Steam Library\steamapps\common\RimWorld\Mods"`；CLI 用 `archo library --include 'location:D:\Steam Library\steamapps\common\RimWorld\Mods'`。

窗口里的鼠标操作效果相同：单击、`shift`/`ctrl`+单击、双击启用或停用，在列表间拖动（启用、停用、暂存）或在已启用列表内拖动（调整顺序），右键查看该 Mod 可做的操作。语言跟随系统；用 `archo config set ui.lang zh|en|auto` 更改。

设置列出所有已登记的游戏、工坊、SteamCMD 和其他 Mod 目录，以及 RimArcho 的锁定库和整合包目录。每项都显示实际路径，包括默认位置。**重新定位**用于重新关联你自行移动的目录；**移动到…**把目录移到新路径。Mod 身份、标签和列表保持关联。不会覆盖已占用的目标。跨盘移动时，先用文件管理器移动目录，再重新定位。

RimArcho 每次启动都会检查这些目录。缺失或变动时会询问新位置；可以跳过提示，之后从设置或**能不能跑**返回处理。目录问题在那里显示为警告。CLI 可运行 `archo folder check`，然后用 `archo folder set <directory-id> <new-directory>` 或 `archo folder move <directory-id> <new-directory>`。

## 接入 agent（`archo mcp`）

`archo mcp` 通过 stdio 提供 RimArcho 工具。默认只提供 agent 诊断和修复 Mod 列表所需的核心工具：读取 Mod、Def、Patch、Harmony 补丁、日志和 C# 源码；编辑、保存和启动方案；测试运行；锁定版本；规则和忽略问题。`archo mcp --tools all` 还会提供玩家的管理工具：整合包、订阅、Mod 库操作、贴图优化、内存估算、维护、身份确认、隐藏的 Mod、链接修复、标签和备注，以及卸载预览。只有希望 agent 也处理这些事时才加 `--tools all`：每个工具在每轮对话都会占用上下文。

每个工具的返回值与对应 CLI 命令的输出完全相同。工具描述和服务说明解释了诊断流程，`guide` 工具给出每个工具的完整说明，因此 agent 不需要额外技能。只读工具带有 `readOnlyHint` 标记，客户端可据此自动批准；会写入游戏或 Mod 文件的工具带有 `destructiveHint` 标记。

**Claude Code**，在终端里运行：

```
claude mcp add rimarcho -- C:\Tools\RimArcho\archo.exe mcp
```

需要管理工具时：`claude mcp add rimarcho -- C:\Tools\RimArcho\archo.exe mcp --tools all`。

或写入项目的 `.mcp.json`：

```json
{
  "mcpServers": {
    "rimarcho": { "command": "C:\\Tools\\RimArcho\\archo.exe", "args": ["mcp"] }
  }
}
```

**Codex**，写入 `~/.codex/config.toml`：

```toml
[mcp_servers.rimarcho]
command = 'C:\Tools\RimArcho\archo.exe'
args = ["mcp"]
```

在 `.mcp.json` 和 `config.toml` 里写 `"args": ["mcp", "--tools", "all"]` 即同时提供管理工具。

然后可以问：“我的日志有 `Patch operation … failed` 报错，是哪个 Mod 出问题了？”或粘贴堆栈。工具调用可传入 `env` 使用另一个环境，传入 `game_version` 按另一个游戏版本分析。

## 文件位置

| | |
|---|---|
| RimArcho 数据 | `%APPDATA%\RimArcho\`（数据库、启用配置历史、反编译缓存） |
| 反编译代码 | `%APPDATA%\RimArcho\cache\source\<dll hash>\` |
| 写入的游戏文件 | `<user data>\Config\ModsConfig.xml`，仅在 `save` 时写入；旧文件在旁边保留为备份 |

## 许可

见仓库。

## 许可证

MIT。见 [LICENSE](LICENSE)。