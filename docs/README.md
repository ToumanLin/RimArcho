# RimArcho documentation

| Where | What it is | Kept how |
| --- | --- | --- |
| [user-guide.md](../UserGuide/user-guide.md) ([Chinese](../UserGuide/user-guide.zh-CN.md)) | For players: every page and feature, how to use it, recipes | Updated when a feature players see changes; the Chinese copy in the same commit |
| [00-top-level-design.md](00-top-level-design.md) | What RimArcho is, who it serves, the three questions it answers, principles, roadmap and the decision record (§6) | Changes only with a product decision |
| [00a-architecture.md](00a-architecture.md) | Domain model, module layers, technical principles, why Mod identity works the way it does | Changes with the architecture |
| [specs/](specs/README.md) | The specifications `01`–`19`: how each area behaves now. Code comments cite them as `NN §x.y` | Updated in the same commit as the behavior |
| [glossary.md](glossary.md) | Chinese and English terms, and where the interface's words differ from the documentation's | Add a row when a term is introduced |
| [release-vm-tests.md](../UserGuide/release-vm-tests.md) | Checks to run in the VM before a release, one row per new or changed feature | A row per change; tested rows leave the table |
| [research/](research/README.md) | Investigations and open questions; conclusions move into the specifications and the report is removed once it has landed | One file per question |
| [design/](design/) | Interactive interface drafts (HTML) with a README each | One folder per page |
| [api/](api/) | GUI API contracts that the frontend calls | Next to the code they describe |
| [assets/](assets/) | Images used by the documentation and the README | — |

Read offline: `node scripts/docs-reader/build.cjs` writes `.cache/docs-reader/rimarcho-docs-reader.html`, one self-contained page with every Markdown document here.
