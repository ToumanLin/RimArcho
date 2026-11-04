# AGENTS.md

RimArcho: a RimWorld launcher and Mod manager written in Go. `RimArcho.exe` is the window; `archo.exe` provides the TUI, CLI and MCP. Development branch: `dev/1.0`.

## Before making changes

- Read [docs/specs/README.md](docs/specs/README.md) first, then the specification for the area you are changing (`NN §x.y`). See [docs/00a-architecture.md](docs/00a-architecture.md) for the architecture and [docs/00-top-level-design.md](docs/00-top-level-design.md) §6 for decision rationale.
- If the actual code differs from the docs, follow the code and mention the discrepancy in your report.
- Write documentation and code in English. Chinese belongs only in language files, `docs/glossary.md` and `README.zh-CN.md`; Markdown may quote a Chinese name verbatim (an account, a Mod, a path) inside an inline `code` span.

## After changing code, in the same commit

1. **Specifications**: if behavior changes, update the section responsible for it. Follow "Maintenance" in specs/README.
2. **[CHANGELOG.md](CHANGELOG.md)**: add an entry to the topmost unreleased version. Include changes visible to players and changes to CLI / MCP output and tools.
3. **[docs/release-vm-tests.md](docs/release-vm-tests.md)**: add a row for each new feature, key or dialog, or existing feature whose behavior changed. Before a release, test the entire table in the VM to find bugs.

## Performance

Pages must load quickly and use little memory. Requirements and thresholds: 06 §10; measured baselines: 07 §6.

## Tests

- `go test ./...`, `node --test "scripts/vm/*.test.mjs" "feedback-relay/*.test.mjs"`.
- Automated tests use only synthetic data (`internal/testkit`). Touch real games, Mods and `%APPDATA%\RimArcho` only inside the VM ([scripts/vm/README.md](scripts/vm/README.md); safety rules: 07 §4).