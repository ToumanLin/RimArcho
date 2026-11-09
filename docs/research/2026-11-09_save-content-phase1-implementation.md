# Save-content phase 1 implementation

Date: 2026-11-09. Backend implementation in `feat/save-content`; no commit created. This is the delivery and validation record for the phase 1 task, based on [the attribution research](2026-11-09_save-content-attribution.md). The maintained behavior contract is 12 §1.4–§2.3.

Implemented typed save references, bounded unresolved Def fields, an additive schema migration, selected-save attribution and inheritance, the requested GUI API/JSON model, and CLI/MCP content mode. Existing Def and DLL facts remain readable while background analysis replaces old fact sets. Frontend, locales, CHANGELOG, release VM checklist, ToDo and the original research report were left to their owner.

## Files changed by this implementation

Paths below exclude pre-existing user changes and `internal/surface/gui/zz_savehome_test.go`.

| Area | Files |
| --- | --- |
| L1 | `internal/savefile/scan.go`, `scan_rules.go`, new `scan_content.go`, new `scan_content_test.go` |
| Def extraction | `internal/analyze/analyze.go`, new `internal/analyze/defcontent.go` |
| Storage | `internal/store/analysis.go`, `savecontent.go`, `schema.sql`, `store.go`, `gametypes_test.go`, new `defcontent.go`, new `defcontent_test.go` |
| Application | New `internal/app/savecontent.go`, `savecontent_resolve.go`, `savecontent_test.go`, `savecontent_bench_test.go` |
| GUI backend and view | `internal/surface/gui/saves.go`, new `saves_content_test.go`; new `internal/view/saves_content.go`, `saves_content_test.go` |
| CLI | `internal/cmds/saves.go`, `guide_tools.go`, new `saves_content.go`, `saves_content_golden_test.go`; new `testdata/golden/saves_content.txt` |
| MCP | `internal/surface/mcp/saves.go`, `saves_test.go`, `desc.go` |
| Specifications | `docs/specs/05-facts-and-findings.md`, `12-saves-unplayed-and-worth-playing.md`, `13-commands-and-cli.md`, `14-mcp.md`, `17-engineering.md`, `18-performance.md` |
| Delivery record | This file |

## Design decisions and differences from the research proposal

- L1 retains `Facts.Defs` unchanged and adds `DefRefs[type][name]` occurrence counts. The research's richer evidence-kind/rule/sample-path record was omitted: the agreed JSON card needs counts and typed identities, and paths would add storage/allocation without a consumer. Configuration and help-link contexts are excluded from this separate reference map. ParserVersion increases once, from 1 to 2.
- Analyzer version increases from 11 to 12. Schema 35 adds `def_fact.content_fields` and `fact_set.def_content_version`; migration neither deletes views nor clears fact sets. A per-Mod marker supports analyzed Mods with zero Defs. Old fields/views stay available to existing readers, but old providers contribute no content totals until rebuilt. References matching known old providers are not mislabeled source-not-found.
- Raw predicate fields preserve presence, null, list append/reset and retained nested package conditions. Extraction is bounded at 256 nodes and 512 bytes per value/condition; truncation marks classification incomplete. Missing/cyclic/over-depth parents and incomplete inputs enter `other`, with a classification-unknown name suffix.
- Candidates follow save package order and installed-instance resolution, with Core/DLC included. Conditions use the saved packages and resolved package identities. Untyped names count every type candidate; typed forms constrain matching. Duplicate resolved Instances retain their first save position. The selected environment's store version is restored after the query.
- Inheritance is Name-keyed across types, selects the greatest parent Mod position not after the child (first node on ties), and is memoized for one selected-save query. Parent/race/flesh dependencies are fetched in batches; traversal is guarded at 64 levels.
- PawnKindDefs use their resolved race category but retain their own typed identity and defining Mod. This avoids treating a Mod's pawn-kind variant as if the race's provider defined it.
- Wrappers collapse to base identities; contained objects provide minified evidence. Generated items, recipes and terrain retain their generated identity and category while using the base provider. Generation lookup is constrained by base type and does not discard a different type sharing the generated name.
- Built-in plant families are recognized by class name; custom class/plant-block evidence is marked inferred. Exact DLL ancestry is outside phase 1. Race predicates include organic flesh and saved Anomaly-package conditions.
- The exact requested JSON shape replaces the research's illustrative richer match/confidence/pagination model. Name lists are capped at 100 per category, sorted by uses then name; totals omit names. Categories with known severe undercoverage carry `under`.
- `App.SaveContent` uses cached L1 only, never opens a save, and does no work during listing. L2 is recomputed for the selected save, without a persistent resolved catalog. CLI/MCP retain the existing selected-save inline L1 read before calling it. Content mode takes precedence over play/mods and respects the 40-line default limit.
- The existing analysis-selected effective-folder fact sets are reused. Save-specific LoadFolders reevaluation and a post-Patch scope catalog remain deferred; consequently a different analyzed package scope can affect available raw folder facts. The card explicitly discloses Patch/grid/history/scenario limits and uses installed versions.
- The MCP schema was already close to its context-size budget. Existing `get_save` descriptions were shortened while adding the new parameter; the budget test passes.

The documentation/code discrepancy identified in the research is corrected in 05 §5.1: analysis checks parent-name existence after Patch simulation; it does not resolve inherited classification there. Selected-save raw-field resolution is a separate operation.

## Measurements

Synthetic data only, Windows amd64, Ryzen 9 7950X. Five benchmark iterations; these are measured samples, not real-save latency guarantees. Canonical measurements are recorded in 18 §2.

`go test -count=1 ./internal/savefile -run '^$' -bench '^BenchmarkScan27MB$' -benchtime=5x -benchmem`

| Scan | Time | Throughput | Allocated | Allocations | Peak heap |
| --- | --- | --- | --- | --- | --- |
| Before typed references | 87.87 ms | 307.46 MB/s | 7,423,072 B/op | 81,613/op | 3.577 MiB |
| After typed references | 108.07 ms | 250.00 MB/s | 8,174,315 B/op | 82,448/op | 3.779 MiB |

Same 25.77 MiB dense fixture: about 23% more scan time and 10% more cumulative allocation. This excludes opening files/antivirus and encoding the cache.

`go test -count=1 ./internal/app -run '^$' -bench '^BenchmarkSaveContent$' -benchtime=5x -benchmem`

| Selected-save L2 | Time | Allocated | Allocations |
| --- | --- | --- | --- |
| 1,500 distinct names | 7.53 ms | 5,746,497 B/op | 45,447/op |
| 15,000 distinct names | 81.25 ms | 65,385,636 B/op | 441,290/op |

Twenty synthetic Mods with inherited Item definitions, warm database and cached L1. Includes cache gunzip/JSON, batched candidate/parent lookup, inheritance and grouping. Excludes scanning and frontend rendering. Allocation figures are cumulative, not retained/peak memory.

## Validation and remaining checks

- `go build ./...`: passed.
- `go test -count=1 ./...`: all packages pass except GUI `TestWording`, which rejects `" · "` in user-owned `internal/surface/gui/assets/js/saves.js` under 16 §7.3. The file was not edited, and the test was not weakened. The requested all-green acceptance remains blocked by that frontend wording issue.
- Synthetic coverage includes typed/untyped candidates, last provider per type/save order, abstract exclusion, A/B/C child-relative inheritance, empty/reset lists, tools-only weapons, apparel with verbs, race predicates, custom plants, wrappers and generated identities, hidden factions, unchecked providers, package conditions, Core/DLC, malformed results, name bounds, cached-only reads, API JSON, CLI goldens/line limit and MCP CLI parity.
- Schema-34 migration test preserves existing Def facts, DLL/type facts, content views and `CachedDefs`, while marking content fields unready.
- `node --test "scripts/vm/*.test.mjs" "feedback-relay/*.test.mjs"`: 107 passed.
- `node scripts/docs-refs/refs.cjs check`: all references resolve.
- `gofmt`: applied to implementation Go files; verification produced no filenames.
- `git diff --check`: passed.

No real games, Mods, saves or player application data were accessed. No VM acceptance was run. Exact runtime class ancestry, real-save path coverage, actual matching shares, cold database timings and GUI rendering remain unverified. Post-Patch facts, terrain/biome grids and DefMap decoding are intentionally absent. A reference is surviving evidence, not completion or proof that missing content was never played.

## English strings for the owner

The following is the complete added/changed backend copy inventory. Locale files were not edited. Notes, coverage sentences, suffixes and category labels pass through `view.T`/`view.Tf` for the GUI. CLI/MCP text follows the existing English tool-output convention.

Notes:

```text
This save has not been read yet.
These references are partial because the save is malformed.
The save's Mod list could not be read.
```

Coverage sentences:

```text
Patch-added Defs and Patch changes to existing Defs are not reflected.
Terrain grids and world biomes are not decoded; only named references are included.
Past events, quests and storytellers may leave no surviving references; indexed history maps are not decoded.
The original ScenarioDef cannot usually be recovered from a save's scenario name and parts.
```

Name suffix templates:

```text
%s (hidden)
%s (inferred)
%s (classification unknown)
```

GUI labels and CLI category forms (some nouns already exist elsewhere):

| Key | GUI label | CLI singular / plural |
| --- | --- | --- |
| factions | factions | faction / factions |
| races | humanlike races | humanlike race / humanlike races |
| animals | animals | animal / animals |
| mechanoids | mechanoids | mechanoid / mechanoids |
| creatures | other creatures | other creature / other creatures |
| buildings | buildings | building / buildings |
| weapons | weapons | weapon / weapons |
| apparel | apparel | apparel item / apparel items |
| plants | plants | plant / plants |
| items | items | item / items |
| terrain | terrain | terrain type / terrain types |
| research | research projects | research project / research projects |
| events | events | event / events |
| quests | quests | quest / quests |
| xenotypes | xenotypes and genes | xenotype or gene / xenotypes and genes |
| culture | culture | cultural element / cultural elements |
| abilities | abilities | ability / abilities |
| hediffs | health conditions | health condition / health conditions |
| traits | traits | trait / traits |
| recipes | recipes | recipe / recipes |
| appearance | appearance | appearance option / appearance options |
| biomes | biomes | biome / biomes |
| storytellers | storytellers | storyteller / storytellers |
| scenarios | scenarios | scenario / scenarios |
| other | other | other entry / other entries |
| unknown | source not found | name / names |

CLI flag help, labels and output templates:

```text
content references by Mod and category, with coverage limits
content
note
totals
source not found
not analysed
limits:
none
%d distinct name; sources use installed Mod versions
%d distinct names; sources use installed Mod versions
%d name
%d names
… %d more Mods (archo saves show %s --content --all)
```

MCP `get_save` parameter descriptions (including shortened existing descriptions):

```text
environment; default current
sv:ref, name or colony/save
all Mods
all colonists and events
content by Mod
```

Changed MCP tool description:

```text
One save (sv:ref, name or colony/save): load issues with fixes, Mod differences and playthrough (play=true for all colonists/events). content=true shows referenced content by Mod/category, using installed versions, with coverage limits. Read-only.
```

Added text in the detailed `get_save` guide (the surrounding guide is unchanged):

```text
content=true selects referenced content instead: category totals, Mods in save order, sources not found, Mods not analysed and coverage limits. It uses installed Mod versions, counts every Def-type candidate for untyped names, and does not measure completion or depth of play; Patches and compressed terrain/world grids are not resolved. content takes precedence over play and mods.
```

## Draft CHANGELOG paragraph for CLI/MCP

Added `archo saves show --content` and `get_save(content=true)` to report saved Def references by Mod and category using installed versions, with separate source-not-found references, Mods awaiting analysis and explicit coverage limits. Typed references now include research, pawn kinds, genes, ideology, bills and appearance. Existing Def and DLL facts remain usable during background rebuilding for the new content fields.

For the owner's release checklist: cover content mode/API after a background read, untyped duplicate candidates, hidden factions, wrapper collapse, old-schema readiness/rebuild, partial-save notes, limits and the CLI 40-line/`--all` behavior in the VM. The protected release checklist was not edited.
