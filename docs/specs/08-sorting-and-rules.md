# 08 · Sorting and rules

> Status: implemented in the CLI, MCP and GUI: automatic sorting with top and bottom tiers (§1), user rules, suppressible constraint references and community data (§2). Not implemented: completing dependencies from steamDB (§2.3) and `edit` in MCP `manage_rules` (§2.1).
> Basis: 00, 00a, 04 §2–§3, 05 §4, 07, 13 §4, 16 §2.5, 16 §5, 18.

Automatic sorting orders a plan's enabled Mods (07) so that hard constraints from About.xml declarations, community rules and the player's own rules hold, while keeping the current order where it can. This document covers the constraints and the algorithm (§1), and user rules, suppressible constraint references and community data (§2). The editor's sorting UI: 16 §2.5; how sort results appear in can it run: 16 §5.

## 1. Automatic sorting

### 1.1 Constraints

| Constraint | Source | Strength |
| --- | --- | --- |
| Core first, DLC immediately following in official order | Built-in | Hard |
| Dependencies before dependents | About.xml `modDependencies` (including `ByVersion`) | Hard |
| `loadBefore` / `loadAfter` (including `ByVersion`), `forceLoadBefore` / `forceLoadAfter` | About.xml | Hard |
| Community sorting rules | Community data (§2.3) | Hard |
| User rules | Player (§2.1) | Hard; may suppress any constraint above |
| Preserve current relative order where possible | Current list | Soft |

- Before sorting, add the installed, loadable vanilla Core if it is absent, and Archo Monitor if it is absent and `monitor.default` is on (07 §1.5); then build the graph of the complete list. The player's DLC choices are kept. A preview does not change the workspace; the additions and the sort undo together. The result names the additions ("sorted: added Archo Monitor, 3 moved").
- The editor's Disabled lists Core and DLC first, in official order, then the others by name; Enabled shows the actual load order.
- `ByVersion` selects the current game's major.minor (keys such as `v1.6`).
- **Official order**: Core → Royalty → Ideology → Biotech → Anomaly → Odyssey; unknown `ludeon.rimworld.*` packages follow the known DLC; all other Mods follow all official content.
- **Exemption**: Mods that declare `loadBefore` (including `ByVersion`) or `forceLoadBefore` an official package (for example Harmony) are exempt from "official first", as the game and Harmony intend; otherwise almost every list would have a cycle.
- **Missing entries** are nodes too: they have no About.xml declarations, but "official first", other Mods' declarations naming their packageIds and community rules still apply, so they mostly keep their positions.
- `incompatible` user rules affect only findings, not sorting (05 §4.3).
- **Unsatisfiable constraints** do not enter the graph; `archo sort` lists them with reasons under "not used" (corpus G2):

| Condition | Reason |
| --- | --- |
| Declaration or community rule between official packages contradicts official order | contradicts the official order |
| A rule putting a Mod before Core/DLC (About.xml loadBefore / forceLoadBefore, community rule), while other constraints require it after official content, cycles with built-in order | would put the mod before official content … |

  A "before Core" that forms no cycle (the Harmony convention) is honored as usual (exemption above).

### 1.2 Algorithm

1. Build a directed graph. Each edge records its origin (Mod declaration, community rule or user rule) and its suppressible reference (§2.2). Duplicate edges count once. Edges suppressed by `ignore` are left out and counted in the result.
2. Find strongly connected components (Tarjan). If a cycle contains both built-in official edges and "before official content" rules, remove the latter (§1.1, unsatisfiable constraints) and try again. For the remaining cycles, **suppress no constraints**: emit `order.cycle` (info, 05 §4.3.1; muted and not counted in "can it run"), list each edge with its origin, and sort the rest normally. Within a cycle, each pair of Mods is represented by its strongest constraint; pairs are kept from strongest to weakest in the abandonment order below, and a pair that would form a cycle with those kept is dropped. The members are sorted by the constraints kept, keeping the current order where possible. `archo sort` marks dropped constraints `[broken]`.
3. Suggest one **edge to suppress** per cycle: the weakest dropped constraint (if none can be suppressed, the weakest edge in the cycle). **Abandonment order**, weak to strong:
   1. Constraint strength (§1.3, including the per-rule judgments in `observed.json`): harmless, weak, strong;
   2. Within equal strength: community rule, `loadAfter`, `loadBefore` / `force*`, dependency, built-in official order, user rule;
   3. Ties: first abandon the edge the current list already violates (From after To).

   User rules are never suggested; remove them with `archo rule remove`. The suppressible references are kept for the CLI. The UI adopts the sorter's choices automatically and explains the dropped requirements without counting them in "can it run" (merged into automatic sorting, 16 §5); it asks for no manual suppression and shows no error dialog. The choices apply only to this sort; they are not stored as ignore rules.
4. Sort the components topologically (Kahn). When several are available, compare in this order: one attached to the node just emitted (step 5); the earlier tier (top, normal, bottom, §1.5); the one whose first member comes earliest in the current list. Without top and bottom tiers, this keeps the differences from the original order as small as possible.
5. `attach`: a Mod attached after the node just emitted comes first, if the hard constraints permit.
6. Output the new order and the diff. The UI writes directly to the plan (one reversible edit), not to the game. `archo sort --dry-run` only shows the result. Results include missing required dependencies, unresolved entries, recommended replacements and how cycles were resolved; API contract: 16 §6.1.

Why cycles do not block: the game does not sort and only warns about declaration violations, so lists with cycles usually run. Example: Ferny's Progression Production declares `loadAfter` Furniture, while Better Architect Mods declare `loadAfter` and dependencies on each other, which forms a real cycle of 3 Mods. RimArcho reports `order.cycle` with evidence and suppressible references, and drops the weakest constraint while sorting. `order.cycle` blocks neither game writes, launch nor pack builds (10 §2).

### 1.3 Severity of declaration violations

Corpus conclusions (numbers in 18):

- **Declaration violations are common and mostly harmless.** About half of the logs violate dependencies or load-order declarations, and nearly half violate community rules. Above 100 Mods, the proportion of errors is similar with and without violations. The constraints violated most often seldom involve Mods attributed as error sources. One (`modoptionssort` before Core) is impossible to satisfy. So `order.violated` is at most a warning, not an error (05 §4.3); sort results and findings distinguish "causes errors" from "declared but not enforced by the game".

**Constraint strength** (`internal/order/strength.go`; corpus P1: 6,734 HugsLib 1.6 logs, docs/research/decisions-corpus.md). For each violated constraint, compare the logs that violate it with those that keep it (both Mods enabled, same list-size tier), measuring the proportion where either Mod is attributed as a finding's source:

| Constraint | Violations | Source ratio observed/expected | Strength | Severity |
| --- | --- | --- | --- | --- |
| Mod needs Core/DLC first (dependency, loadAfter, forceLoadAfter targeting official package) | 192 | 3–4.5 | Strong | Merge into `official.order` |
| Dependency (between Mods) | 2,903 | 1.41 | Strong | warning |
| loadBefore (between Mods) | 395 | 1.34 | Strong | warning |
| forceLoad*, user rules | Few | — | Strong | warning |
| loadAfter (between Mods) | 5,430 | 1.14 | Weak | info |
| Community rules (between Mods) | 6,541 | 1.13 | Weak | info |
| Declared before Core/DLC | 451 | 1.05 | Harmless | info |

- A ratio above 1.3 is strong. Weak and harmless violations appear only in the technical information and are not counted in "can it run" (16 §5). The plan editor's sorting explanations and the order findings say strong or weak, strong first (16 §2.5).
- Per-rule judgments for rules with enough corpus violations ship in `internal/order/observed.json` (aggregate counts only, no log IDs). "Harmless": at least 30 violations, with a source proportion no higher than in the logs that keep the rule (if fewer than 30 logs keep it: at least 100 violations and a proportion of at most 2%). "Harmful": at least 20 violations, a proportion of at least 20%, more than twice that of the logs that keep it, and z ≥ 3. Table entries are used where present, otherwise the defaults for the constraint type. Explanations of harmless violations show the corpus counts and suggest `ignore` first.
- `archo data update` fetches only third-party datasets (§2.3); RimArcho has no data repository, so this table is built in and updated with releases.
- **Odyssey too late** is the most common official-order violation (23% of corpus logs): a DLC enabled after the list was made is appended at the end. `official.order` (warning) offers the cheapest moves: move the official packages that come after Mods back after the official package before them, or move the few Mods before official content after the last official package, whichever takes fewer steps (at most 3; otherwise suggest sorting). Constraints from official packages to Mods are not reported separately; the strong ones (a Mod requires this DLC) are listed as `official.order` evidence. Automatic sorting also restores official order (05 §4.3).
- Mods that do not declare `loadBefore` an official package go after all DLC, even if they were before Core (for example RimCrow Companion).

### 1.4 Other details

- Community rules are read only from `<data directory>/cache/communityRules.json` (§2.3).
- Diffs do not mark unchanged positions as moved (07 §3.2).
- Target: sort 500 Mods in ≤ 1 second (S5); measurements: 18.

### 1.5 Top and bottom tiers

Sources: community `loadTop` / `loadBottom` (`{"value": true}`), user `top` / `bottom` (§2.1), RimCrow import (07 §6.2). If both exist for one Mod, the user rule wins.

- **Tiers are preferences, not constraints**: they are compared only when the topological sort has several components available (§1.2 step 4); they never violate constraints and never enter finding analysis (there is no "not last" finding). Official content is not affected and stays first (§1.1). Top Mods that do not declare `loadBefore` an official package go right after all DLC.
- **Related Mods come along**: a top Mod pulls everything it requires, along constraint paths, to the top; a bottom Mod pulls everything that depends on it to the bottom. Otherwise a top Mod would wait for dependencies emitted at their original positions. Official order is hard, so pulling never puts Mods before DLC (RimCrow propagates position weights in a similar way but has no official constraints; in replay it put Mods before DLC in 4.8% of lists).
- Mods pulled both ways (for example one that requires a bottom Mod and is required by a top Mod) stay normal; Mods explicitly top or bottom keep their own tier. Constraints still apply.
- Within a tier the current order is kept.
- Suppress a community tier with `archo rule add ignore community:<pkg>:loadtop` (or `loadbottom`), a user rule with `archo rule remove`.
- `archo sort` lists each top and bottom Mod under "ends", with its source, how to suppress it and how many Mods it pulled; those pulled both ways are marked `[also pulled to the other end]`.
- There are no RimCrow-style position weights, alphabetical tie-breaking or positions guessed from the kind of Mod (translations, textures): the corpus gives no evidence for them, and they cause too many moves on every sort.

## 2. Rules and community data

### 2.1 User rules

User rules are global and belong to Lineages, shared by all games and plans (03 §2 item 6; `store.Rules` ignores the environment), so original and locked versions share rules and package-ID changes do not lose them.

| Rule | Meaning |
| --- | --- |
| `after A B` | A after B |
| `before A B` | A before B |
| `incompatible A B` | Cannot enable A and B together (finding analysis only, not sorting) |
| `attach A B` | A immediately after B (translations, patches) |
| `top A` / `bottom A` | A as early/late as constraints permit (§1.5; preference, not constraint) |
| `ignore <constraint ref>` | Suppress a declaration, community rule or built-in official order |

| Command | Description |
| --- | --- |
| `archo rule add after\|before\|incompatible\|attach <A> <B> [--note <text>]` | References resolve to Lineages; reject same-Lineage pair |
| `archo rule add top\|bottom <A> [--note <text>]` | One reference |
| `archo rule add ignore <constraint ref> [--note <text>]` | Use constraint syntax printed by `archo sort` |
| `archo rule edit <r:n> <kind> <A> [<B>] \| ignore <constraint ref> [--note <text>]` | Replace the rule's kind, targets and note; `r:<n>` and the creation time stay (`App.UpdateRule`) |
| `archo rule list` | Rules: `r:<n>`, kind, targets, note, creation time |
| `archo rule remove <r:n>` | Delete one |

- Every rule can have a note and records its creation time.
- IDs only increase (`rule_seq` in `meta`) and are never reused after deletion, so `r:<n>` always means the same rule in notes and scripts. The "undo" in the GUI's status after a deletion restores the rule unchanged, with its `r:<n>` (`Store.RestoreRule`).
- UI: the GUI's "load-order rules" in the editor's left column (16 §2.5; by default only the rules involving this list); in the TUI, the editor's `r` and the "rules" technical page in details. Rules are not list edits and do not enter plan undo stacks. MCP `manage_rules` has no `edit` (not implemented, 14 §2).

### 2.2 Suppressible constraint references

| Printed form | Refers to | Stored form |
| --- | --- | --- |
| `decl:<#instance>:<field>:<pkg>` | Mod About.xml declaration; lowercase field `loadafter`, `loadbefore`, `forceloadafter`, `forceloadbefore`, `dependency` | `decl:<Lineage>:<field>:<pkg>` |
| `official:<#instance>` | Built-in official-order constraint on this Mod | `official:<Lineage>` |
| `community:<pkg>:<field>:<pkg>` | Community rule, field `loadafter` or `loadbefore` | Same |
| `community:<pkg>:loadtop`, `community:<pkg>:loadbottom` | Mod's community top/bottom tier (§1.5) | Extra trailing `:` |

The printed form names the Lineage's primary Instance; the stored form names the Lineage, so a suppression survives changes of copy or package ID. Printed references work unchanged in `rule add ignore` (`TestPrintedIgnoreRefsRoundTrip`).

### 2.3 Community data

| Dataset | File (`<data directory>/cache/`) | Purpose | Default source |
| --- | --- | --- | --- |
| Community sorting rules `rules` | `communityRules.json` | Supplement loadBefore/After (§1.1) | `https://raw.githubusercontent.com/RimSort/Community-Rules-Database/main/communityRules.json` |
| Workshop metadata `steamdb` | `steamDB.json` | packageId / Workshop ID mapping: links for missing entries; completing dependencies when an author leaves out `modDependencies` (not implemented) | `https://raw.githubusercontent.com/RimSort/Steam-Workshop-Database/main/steamDB.json` |
| Replacement rules `replacements` | `replacements.json.gz` | "Use another Mod instead": `mod.replaced` (05 §4.3); the import preview also suggests replacements for entries not installed | `https://raw.githubusercontent.com/emipa606/UseThisInstead/main/replacements.json.gz` |

- Updates go online through `archo data update`, the settings' "update now" or automatic checks (16 §2.16); ordinary page reads are offline.
- `archo data update [<datasets>…] [--dry-run]`:
  - Downloads one after another (all three if none is named); `--dry-run` only lists the sources.
  - Stages temporary files, validates the JSON (`rules` for rules and replacements, `database` for steamDB), and records each dataset's version or timestamp and entry count.
  - **Replaces only when all are ready**: stages the data, the source records and the compact steamDB index, checking write and close errors. Any download, validation or staging failure keeps the existing files. The old files are staged during replacement; a failure to rename or to write the database generation restores the whole set. A failure on first install removes the partial new files. If restoring also fails, the backup directory is kept and its location is in the error. Implementation: `internal/app/datareplace.go`; there is no promise of atomicity across files if the process is killed or power is lost.
  - When the source is unchanged and a local file exists: sends the previous ETag / Last-Modified as `If-None-Match` / `If-Modified-Since` and skips unchanged downloads. Limit 512 MB per file, timeout 10 minutes (steamDB is about 50 MB). Decompresses gzip and strips a UTF-8 BOM automatically.
  - Writes `<file>.source.json` beside each dataset, replaced and rolled back with it: dataset, source, ETag, Last-Modified, version, download time, check time, entries, bytes. A 304 advances only the check time; older sidecars without one use the download time.
  - Compacts the downloaded steamDB into `steamDB.index.json` (packageId → [Workshop ID, name]; the smallest Workshop ID for duplicate packageIds; `appid` entries skipped), so the 50 MB original is not parsed again and again.
- `archo data status`: per dataset, the source, version, download and check times and entry count; "old" after 30 days; automatic-check status: 13 §4.2.
- Manual and automatic updates share a lock; a download or validation failure keeps the files. Success advances the database's `community:generation`, which invalidates other connections' compact indexes; the GUI and TUI drop their page analysis caches. Notifications of automatic failures: 16 §2.16.
- Sources can be configured: `config set data.rules.url`, `data.steamdb.url`, `data.replacements.url`; local paths are allowed.
- Community data stores packageIds, resolved to Lineages when used.

## 3. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| S1 | Sort an already sorted list | Order unchanged | Pass (`TestS1SortedIsStable`) |
| S2 | Two Mods mutually declare loadAfter | `order.cycle`, identifying each declaration's About.xml | Pass (`TestS2CycleAndS3Ignore`, `TestSortCycleIgnoreAttach`) |
| S3 | User rule suppresses one declaration | Sort succeeds, satisfying remaining constraints | Pass (same tests) |
| S4 | Translation `attach` to original Mod | Translation immediately follows original | Pass (`TestS4Attach`) |
| S5 | Sort 500 Mods | ≤ 1 second (measurements: 18) | Pass (`TestS5FiveHundred`) |
| S6 | Cycle includes community rule and About.xml declaration | Suggest suppressing community rule | Pass (`TestCycleSuggestPrefersCommunity`) |
| S6b | Cycle has dependency and loadAfter; current order violates dependency | Sort satisfies dependency, drops/suggests suppressing loadAfter | Pass (`TestCycleBreaksWeakestEdge`) |
| S7 | Odyssey last, other official content first | Sort puts Odyssey after Anomaly; first `official.order` suggestion `archo move <Odyssey> --after <previous official package>` clears finding | Pass (`TestOrderSeverityAndDLCMove`, `TestOfficialMoves`) |
| S9 | Community requires Royalty after Biotech | No cycle; rule listed "not used"; official order unchanged | Pass (`TestDropContradictsOfficialOrder`) |
| S10 | Mod declares loadBefore Core and depends on a Mod after Core | No `order.cycle`; loadBefore "not used" | Pass (`TestDropBeforeOfficialInCycle`, `TestSortDropsUnsatisfiable`) |
| S11 | One violation each of dependency order, loadAfter, `superniquito.modoptionssort` loadBefore Core | Dependency warning; loadAfter info; modoptionssort info, explaining no extra corpus errors when violated | Pass (`TestAssess`, `TestOrderSeverityAndDLCMove`) |
| S13 | Community bottom rule, another Mod user-top, top Mod needs library | Top and library first after DLC, bottom and dependents last, others preserve order; ignoring community bottom returns normal tier; repeat sort stable | Pass (`TestTiers`, `TestSortTopBottom`) |
| D1 | Local HTTP fixture serves three datasets | Download, parse, `data status` entry counts | Pass (`TestCommunityData`) |
| D2 | Second dataset invalid JSON | Entire operation fails, old files unchanged | Pass (`TestCommunityData`) |
| D3 | Enabled Mod in replacements | "can it run" shows "use X instead" | Pass (`TestCommunityData`) |
| S8 | Printed ignore reference | Usable unchanged in `rule add ignore` | Pass (`TestPrintedIgnoreRefsRoundTrip`) |
