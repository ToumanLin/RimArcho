# RimArcho specifications

For what RimArcho is and the decisions behind it, see [00 · Top-level design](../00-top-level-design.md); for the architecture, see [00a · Architecture](../00a-architecture.md). Before changing an area, read its specification, then check the code it links to.

| Spec | Contents |
| --- | --- |
| **Scope** | |
| [01 · Scope and status](01-scope-and-status.md) | Version scope, capabilities, exclusions, work still open |
| [02 · File writes and uninstall](02-file-writes-and-uninstall.md) | Every file RimArcho writes (the safety boundary), uninstall, the recycle bin |
| **What is installed** | |
| [03 · Environments and sources](03-environments-and-sources.md) | Terms, environments (one game directory each), Mod sources, links and aliases |
| [04 · Scanning and identity](04-scanning-and-identity.md) | Scanning, reconciliation, Lineage, change events, storage |
| [05 · Facts and findings](05-facts-and-findings.md) | Effective content, fact types, the finding catalog, Def / Patch / Harmony analysis |
| [06 · Game log](06-game-log.md) | Log parsing, attribution, which launch owns a log, live tracking, Archo Monitor logs, error patterns |
| **What is played** | |
| [07 · Plans](07-plans.md) | The plan model, editing, history, writing to the game, locked versions, player data, list exchange |
| [08 · Sorting and rules](08-sorting-and-rules.md) | Automatic sorting, user rules, community data |
| [09 · Launch](09-launch.md) | Launching a plan, link deployment, running-game detection, hidden Mods, Archo Monitor, test runs |
| [10 · Packs](10-packs.md) | Building, launching, importing and exporting packs |
| [11 · Downloads, subscriptions and accounts](11-downloads-and-accounts.md) | Git and download-page subscriptions, SteamCMD, Steam client calls, game downloads, account sessions |
| [12 · Saves, unplayed and worth playing](12-saves-unplayed-and-worth-playing.md) | Save reading and management, untouched content (1.1), worth playing (long term) |
| **Interfaces** | |
| [13 · Commands and CLI](13-commands-and-cli.md) | The tool layer, references, text format, CLI commands |
| [14 · MCP](14-mcp.md) | `archo mcp`: transport, tool table, instructions and guide |
| [15 · Archo](15-archo.md) | The built-in AI assistant (1.1) |
| [16 · Interface](16-interface/README.md) | GUI and TUI: pages, interaction rules, details, visual design, GUI and TUI technology (a directory) |
| **Engineering** | |
| [17 · Technology and engineering](17-engineering.md) | Technology, code structure, data locations, tests, releases, game behavior notes |
| [18 · Performance](18-performance.md) | Performance targets and measured baselines |

Research notes are in [research/](../research/); interface drafts are in [design/](../design/).

## References

Use `NN §x.y` references between specifications and in code comments (for example, `16 §2.9`); `NN` alone names a whole specification. Section numbers belong to the whole specification, not the file: when a specification is split into a directory, its section numbers and references stay unchanged; find the file in that directory's README table. When changing section numbers, update references in code comments and other specifications too.

`node scripts/docs-refs/refs.cjs check` lists references and links that point nowhere. Code or documents from a branch cut before the November 2026 renumbering still use the old numbers: `node scripts/docs-refs/refs.cjs remap scripts/docs-refs/map-2026-11.json <files>` rewrites them.

## Maintenance

- Write documentation in English; Chinese belongs only in language files, `docs/glossary.md`, `README.zh-CN.md`, and `ToDo.md`; a name quoted verbatim (an account, a Mod, a path) may appear inside an inline `code` span.
- **Write each fact in one place.** Elsewhere, reference `NN §x.y` instead of copying it. Copies will eventually diverge.
- **Update the specification with behavior changes, in the same commit.** Changes to responsibilities, interfaces, storage formats, state transitions, file writes, failure handling, or compatibility rules must update the section that owns them. Refactoring that changes none of these needs no specification update.
- **Describe the current state.** Describe implemented behavior as it is; label unfinished work "not implemented", "draft", or "undecided", or put it in 01 §4. Do not record when something was decided or changed, what it used to be called, or how it was before: that history lives in git and `CHANGELOG.md`. Ideas without a decision belong in research/ or design/, not specifications. Decision rationale belongs in 00 §6; specifications record only the conclusions.
- **Use the glossary's terms.** Documentation, the CLI and MCP say plan, environment and Instance; quote the interface's own words ("list", "game") only when describing a control or page (`docs/glossary.md`).
- **Point to code; do not repeat it.** Name files, types, and functions (for example, `view.Nav`), and describe meanings and constraints their signatures cannot express. Omit implementation details that change often.
- **Keep measurements in 18.** Other specifications state targets and reference 18 for measured values; they do not narrate earlier timings.
- **Split files that grow too long.** If a file exceeds about 40 KB or covers unrelated areas, split it into a directory with the same name: put status, basis, and a file table in `README.md`; split other files by section, with filenames starting with their first section number (for example, `2.9-mod-library-and-game-logs.md`). Keep section numbers unchanged. Start each file with a line naming its parent specification.
- **Acceptance follows the specification.** The "Acceptance" section at the end of each specification describes how to check that area. State which checks ran and which did not when submitting changes.
