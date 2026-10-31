# AGENTS.md

RimArcho：Go 写的 RimWorld 启动器和 Mod 管理器。`RimArcho.exe` 是窗口，`archo.exe` 是 TUI、CLI 和 MCP。开发分支 `dev/0.9.0`。

## 动手前

- 先读 [docs/specs/README.md](docs/specs/README.md)，再读要改的那块的规格（`NN §x.y`）。架构见 [docs/00a-架构.md](docs/00a-架构.md)，决策理由见 [docs/00-顶层设计.md](docs/00-顶层设计.md) §6。
- 如果实际代码和 docs 不同，以实际代码为准，并在汇报的时候提及实际代码和 docs 的冲突。

## 改完代码，同一个提交里

1. **规格**：改了行为就改负责它的那一节，写法见 specs/README 的“维护”。
2. **[CHANGELOG.md](CHANGELOG.md)**：写进最上面那个未发布版本。玩家看得到的变化、CLI / MCP 输出和工具的变化都要写。
3. **[docs/发布前VM测试.md](docs/发布前VM测试.md)**：新功能、新按键、新对话框，或改了行为的旧功能，加一行。发新版本前，在 VM 里把这张表全测一遍找 bug。

## 性能

页面加载务必快，内存占用务必少。要求和门槛见 06 §10，实测基线见 07 §6。

## 测试

- `go test ./...`，`node --test "scripts/vm/*.test.mjs" "feedback-relay/*.test.mjs"`。
- 自动测试只用合成数据（`internal/testkit`）。真实的游戏、Mod 和 `%APPDATA%\RimArcho` 只在 VM 里碰（[scripts/vm/README.md](scripts/vm/README.md)，安全规则见 07 §4）。
