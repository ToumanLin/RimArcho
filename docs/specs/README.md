# RimArcho Specifications

For top-level design and decision records, see [00-Top-level design](../00-top-level-design.md); for architecture, see [00a-Architecture](../00a-architecture.md). Before changing an area, read its specification, then check the code it links to.

| File | Contents |
| --- | --- |
| [01-Scope and status](01-scope-and-status.md) | Version scope, exclusions, file write list (safety boundary), pending work and validation |
| [02-Environments, identity and scanning](02-environments-identity-and-scanning.md) | Environments, sources, scanning, reconciliation, Lineage, change events, storage |
| [03-Facts and analysis](03-facts-and-analysis.md) | Effective content, Def / Patch / Harmony analysis, finding catalog, log attribution |
| [04-Plans and sorting](04-plans-and-sorting.md) | Plans, editing, history, writing to the game, sorting, rules, locked versions, import and export |
| [05-Semantic interface and MCP](05-semantic-interface-and-mcp.md) | Tool layer, references, text formats, CLI, MCP |
| [06-ui/](06-ui/README.md) | Sidebar and pages, interaction rules, details, visual design, GUI and TUI technology (split by section) |
| [07-Technology and engineering](07-technology-and-engineering.md) | Technology, code structure, tests, releases, performance targets and measured baselines, game behavior notes |
| [08-Launch and packs](08-launch-and-packs.md) | Launch model, link deployment, game process detection, packs, hidden Mods, recycle bin |
| [09-Archo](09-Archo.md) | Built-in AI assistant (1.1) |
| [10-Unplayed and worth playing](10-unplayed-and-worth-playing.md) | Save reading, untouched content, save management (1.1); worth playing (long term) |

Log corpus research is a separate track; see [research/](../research/). Interface mockups are in [design/](../design/).

## References

Use `NN §x.y` references between specifications and in code comments (for example, `06 §2.9`). Section numbers belong to the whole specification, not the file: when a specification is split into a directory, section numbers and references stay unchanged. Find the file in that directory's README table. When changing section numbers, update references in code comments and other specifications too.

## Maintenance

- Write documentation in English; Chinese belongs only in language files, `docs/glossary.md`, and `README.zh-CN.md`.
- **Write each fact in one place.** Elsewhere, reference `NN §x.y` instead of copying it. Copies will eventually diverge.
- **Update the specification with behavior changes, in the same commit.** Changes to responsibilities, interfaces, storage formats, state transitions, file writes, failure handling, or compatibility rules must update the section that owns them. Refactoring that changes none of these needs no specification update.
- **Describe the current state.** Describe implemented behavior as it is; label unfinished work "not implemented", "draft", or "undecided", or put it in 01 §5. Ideas without a decision belong in research/ or design/, not specifications. Decision rationale belongs in 00 §6; specifications record only the conclusions.
- **Point to code; do not repeat it.** Name files, types, and functions (for example, `view.Nav`), and describe meanings and constraints their signatures cannot express. Omit implementation details that change often.
- **Split files that grow too long.** If a file exceeds about 500 lines or covers unrelated areas, split it into a directory with the same name: put status, basis, and a file table in `README.md`; split other files by section, with filenames starting with their first section number (for example, `2.9-inventory-and-logs.md`). Keep section numbers unchanged. Start each file with a line naming its parent specification.
- **Acceptance follows the specification.** The "Acceptance" section at the end of each specification describes how to check that area. State which checks ran and which did not when submitting changes.
