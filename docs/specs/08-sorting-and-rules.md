# 08 · Sorting and rules

> Status: TODO(reorg)

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

- Before sorting, add installed, loadable vanilla Core if absent; add absent Archo Monitor when `monitor.default` is on (07 §1.5); then graph the complete list. Preserve player DLC selections. Preview does not change workspace; additions and sort undo together. Name additions in results ("sorted: added Archo Monitor, 3 moved").
- Editor Disabled defaults to Core/DLC first in official order, then others by name; Enabled shows actual load order.
- `ByVersion` selects current game major/minor (keys such as `v1.6`).
- **Official order**: Core → Royalty → Ideology → Biotech → Anomaly → Odyssey; unknown `ludeon.rimworld.*` follow known DLC; all other Mods follow all official content.
- **Exemption**: Mods declaring `loadBefore` (including `ByVersion`) or `forceLoadBefore` an official package (e.g. Harmony) are exempt from "official first", matching game/Harmony design; otherwise almost every list would cycle.
- **Missing entries** are nodes too: no About.xml declarations, but "official first", other Mods' declarations targeting their packageIds and community rules still apply, largely preserving position.
- `incompatible` user rules affect findings only, not sorting (05 §4.3).
- **Unsatisfiable constraints** do not enter the graph; `archo sort` lists reasons under "not used" (corpus G2):

| Condition | Reason |
| --- | --- |
| Declaration or community rule between official packages contradicts official order | contradicts the official order |
| A rule putting a Mod before Core/DLC (About.xml loadBefore / forceLoadBefore, community rule), while other constraints require it after official content, cycles with built-in order | would put the mod before official content … |

  Noncyclic "before Core" (Harmony convention) is honored normally (exemption above).

### 1.2 Algorithm

1. Build a directed graph. Each edge records origin: Mod declaration, community rule or user rule, and suppressible reference (§2.2). Count duplicate edges once. Omit edges suppressed by `ignore`, reporting counts.
2. Find strongly connected components (Tarjan). If a cycle contains both built-in official edges and "before official content" rules, remove the latter (§1.1 unsatisfiable constraints) and retry. For remaining cycles, **suppress no constraints**: emit `order.cycle` (info, 05 §4.3.1; muted and uncounted in "can it run"), list each edge and origin, and sort the rest normally. Within a cycle: represent each Mod pair by its strongest constraint; retain pairs from strongest to weakest using the abandonment order below, dropping any pair that cycles with retained pairs. Sort members by retained constraints, preserving current order where possible. `archo sort` marks dropped constraints `[broken]`. (Changed 2026-10-26: previously kept the whole cycle's current order, also preserving dependency violations; corpus replay showed about 5% more strong-constraint violations than RimCrow as a result.)
3. Suggest one **edge to suppress** per cycle: weakest dropped constraint (if none suppressible, weakest edge in the cycle). **Abandonment order**, weak to strong:
   1. Constraint strength (§1.3, including per-rule `observed.json` judgments): harmless, weak, strong;
   2. Within equal strength: community rule, `loadAfter`, `loadBefore` / `force*`, dependency, built-in official order, user rule;
   3. Ties: first abandon the edge already violated by the current list (From after To).

   Never suggest user rules; remove them with `archo rule remove`. Keep suppressible references for CLI. UI adopts sorter choices automatically, explaining dropped requirements without counting them in "can it run" (merged into automatic sorting, 16 §5), without requiring manual suppression or an error dialog. Choices apply only to this sort, not stored ignore rules.
4. Topologically sort components (Kahn). When several are available, compare in order: attached to the just-emitted node first (step 5); earlier tier first (top, normal, bottom, §1.5); earliest first member in current list first. Without top/bottom tiers this minimizes differences from the original.
5. `attach`: prioritize a Mod attached after the just-emitted node, provided hard constraints permit.
6. Output new order and diff. UI writes directly to the plan (one reversible edit), not the game. `archo sort --dry-run` only displays. Results include missing required dependencies, unresolved entries, recommended replacements and automatic cycle resolution; API contract: 16 §6.1.

Why cycles do not block: the game does not sort and only warns on declaration violations, so cyclic lists usually run. Example: Ferny's Progression Production declares `loadAfter` Furniture, while Better Architect Mods mutually declare `loadAfter` and dependencies, forming a real 3-Mod cycle. RimArcho reports `order.cycle`, evidence and suppressible references, abandoning the weakest constraint while sorting. `order.cycle` blocks neither game writes, launch nor pack builds (10 §2).

### 1.3 Severity of declaration violations

Corpus conclusions (numbers in 18):

- **Declaration violations are common and mostly harmless.** About half of logs violate dependencies or load-order declarations, nearly half community rules. Above 100 Mods, error proportions are similar with/without violations. Most frequently violated constraints seldom involve Mods attributed as error sources. One (`modoptionssort` before Core) is impossible. Thus `order.violated` is at most warning, not error (05 §4.3); sort results and findings distinguish "causes errors" from "declared but not game-enforced".

**Constraint strength** (`internal/order/strength.go`, corpus P1: 6,734 HugsLib 1.6 logs, docs/research/decisions-corpus.md). For each violated constraint, compare logs violating versus preserving it (both Mods enabled, same list-size tier), measuring the proportion where either Mod is attributed as a finding source:

| Constraint | Violations | Source ratio observed/expected | Strength | Severity |
| --- | --- | --- | --- | --- |
| Mod needs Core/DLC first (dependency, loadAfter, forceLoadAfter targeting official package) | 192 | 3–4.5 | Strong | Merge into `official.order` |
| Dependency (between Mods) | 2,903 | 1.41 | Strong | warning |
| loadBefore (between Mods) | 395 | 1.34 | Strong | warning |
| forceLoad*, user rules | Few | — | Strong | warning |
| loadAfter (between Mods) | 5,430 | 1.14 | Weak | info |
| Community rules (between Mods) | 6,541 | 1.13 | Weak | info |
| Declared before Core/DLC | 451 | 1.05 | Harmless | info |

- Above 1.3 is strong. Weak/harmless violations appear only in technical information, excluded from "can it run" counts (16 §5). Plan editor › sorting explanations and order findings state strong/weak, strong first (16 §2.5).
- Per-rule judgments with enough corpus violations ship in `internal/order/observed.json` (aggregate counts only, no log IDs). "Harmless": ≥ 30 violations with source proportion no higher than preserved logs (if fewer than 30 preserved logs, ≥ 100 violations and proportion ≤ 2%). "Harmful": ≥ 20 violations, proportion ≥ 20%, more than twice preserved proportion, z ≥ 3. Use table entries where present, otherwise type defaults. Harmless-violation explanations show corpus counts and suggest `ignore` first.
- `archo data update` fetches third-party datasets only (§2.3); RimArcho has no data repository, so this table is embedded and updated with releases.
- **Odyssey too late** is the commonest official-order violation (23% of corpus logs): DLC enabled after list creation is appended. `official.order` (warning) offers the cheapest moves: move official packages after Mods back after their preceding official package, or move the few Mods before official content after the last official package, whichever uses fewer steps (at most 3; otherwise suggest sorting). Do not separately report constraints from official packages to Mods; list strong ones (Mod requires this DLC) as `official.order` evidence. Automatic sort also restores official order (05 §4.3).
- Mods without declared `loadBefore` official packages go after all DLC, even if originally before Core (e.g. RimCrow Companion).

### 1.4 Other details

- Community rules read only `<data directory>/cache/communityRules.json` (§2.3).
- Diffs do not mark unchanged positions as moved (07 §3.2).
- Target: sort 500 Mods in ≤ 1 second (S5); measurements: 18.

### 1.5 Top and bottom tiers

Added 2026-10-26. Sources: community `loadTop` / `loadBottom` (`{"value": true}`), user `top` / `bottom` (§2.1), RimCrow import (07 §6.2). User rules win if both exist for one Mod.

- **Tiers are preferences, not constraints**: compare only when topological sort has multiple available components (§1.2 step 4), never violate constraints, never enter finding analysis (no "not last" finding). Official content is unaffected and stays first (§1.1). Top Mods without loadBefore official packages go immediately after all DLC.
- **Bring related Mods**: top Mods pull all required predecessors along constraint paths to top; bottom Mods pull all dependents along constraint paths to bottom. Otherwise top Mods wait for dependencies emitted at original positions. Official order is hard, so pulling never puts Mods before DLC (RimCrow propagates position weights similarly but lacks official constraints; replay put Mods before DLC in 4.8% of lists).
- Mods pulled both ways (e.g. requiring a bottom Mod while required by a top Mod) stay normal; explicitly top/bottom Mods keep their own tier. Constraints still apply.
- Preserve current order within tiers.
- Suppress community tiers with `archo rule add ignore community:<pkg>:loadtop` (or `loadbottom`); user rules with `archo rule remove`.
- `archo sort` lists each top/bottom Mod, source, suppression method and pulled count under "ends"; mark those pulled both ways `[also pulled to the other end]`.
- No RimCrow-style position weights, alphabetical tie-breaking or guessed Mod-type positions (translations, textures): no corpus evidence, and these cause excessive moves on every sort.

## 2. Rules and community data

### 2.1 User rules

Global on Lineages, shared by all games/plans (03 §2 item 6; `store.Rules` ignores environment), so original/locked versions share rules and package-ID changes do not lose them. (Corrected 2026-10-30: previously said "stored per environment" though code was already global.)

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
| `archo rule edit <r:n> <kind> <A> [<B>] \| ignore <constraint ref> [--note <text>]` | Replace rule kind, targets and note (2026-10-30); retain `r:<n>` and creation time (`App.UpdateRule`) |
| `archo rule list` | Rules: `r:<n>`, kind, targets, note, creation time |
| `archo rule remove <r:n>` | Delete one |

- Every rule supports a note and records creation time.
- IDs increase monotonically (`rule_seq` in `meta`), never reused after deletion; `r:<n>` always means the same rule in notes/scripts. GUI deletion status "undo" restores it unchanged, retaining `r:<n>` (`Store.RestoreRule`).
- UI: GUI "load-order rules" in editor left column (16 §2.5, default only rules involving this list); TUI editor `r` and details "rules" technical page. Rules are not list edits and do not enter plan undo stacks. MCP `manage_rules` lacks `edit` (TODO, 14 §2).

### 2.2 Suppressible constraint references

| Printed form | Refers to | Stored form |
| --- | --- | --- |
| `decl:<#instance>:<field>:<pkg>` | Mod About.xml declaration; lowercase field `loadafter`, `loadbefore`, `forceloadafter`, `forceloadbefore`, `dependency` | `decl:<Lineage>:<field>:<pkg>` |
| `official:<#instance>` | Built-in official-order constraint on this Mod | `official:<Lineage>` |
| `community:<pkg>:<field>:<pkg>` | Community rule, field `loadafter` or `loadbefore` | Same |
| `community:<pkg>:loadtop`, `community:<pkg>:loadbottom` | Mod's community top/bottom tier (§1.5) | Extra trailing `:` |

Print the Lineage's primary Instance reference; store the Lineage so suppression survives copy/package-ID changes. Printed references work unchanged in `rule add ignore` (`TestPrintedIgnoreRefsRoundTrip`).

### 2.3 Community data

| Dataset | File (`<data directory>/cache/`) | Purpose | Default source |
| --- | --- | --- | --- |
| Community sorting rules `rules` | `communityRules.json` | Supplement loadBefore/After (§1.1) | `https://raw.githubusercontent.com/RimSort/Community-Rules-Database/main/communityRules.json` |
| Workshop metadata `steamdb` | `steamDB.json` | packageId/Workshop-ID mapping: links for missing entries; dependency completion (when author omits `modDependencies`, unimplemented) | `https://raw.githubusercontent.com/RimSort/Steam-Workshop-Database/main/steamDB.json` |
| Replacement rules `replacements` | `replacements.json.gz` | "Use another Mod instead": `mod.replaced` (05 §4.3); import preview also suggests replacements for uninstalled entries | `https://raw.githubusercontent.com/emipa606/UseThisInstead/main/replacements.json.gz` |

- Update online via `archo data update`, settings "update now" or automatic checks (16 §2.16); ordinary page reads are offline.
- `archo data update [<datasets>…] [--dry-run]`:
  - Download sequentially (all three if unnamed); `--dry-run` lists sources only.
  - Stage temporary files, validate JSON (`rules` for rules/replacements, `database` for steamDB), record dataset version/timestamp and entry count.
  - **Replace only when all ready**: stage data, source records and compact steamDB index, checking write/close errors. Any download, validation or staging failure preserves existing files. Stage old files during replacement; rename or database-generation write failures restore the whole set. First-install failure removes partial new files. If restoration also fails, preserve backup directory and include location in error. Implementation: `internal/app/datareplace.go`; no cross-file atomicity promise for process kill/power loss.
  - Unchanged source with local file: send previous ETag / Last-Modified as `If-None-Match` / `If-Modified-Since`; skip unchanged downloads. Per-file limit 512 MB, timeout 10 minutes (steamDB about 50 MB). Auto-decompress gzip and strip UTF-8 BOM.
  - Write `<file>.source.json` beside each dataset, replaced/rolled back together: dataset, source, ETag, Last-Modified, version, download time, check time, entries, bytes. 304 advances only check time; older sidecars without it use download time.
  - Compress downloaded steamDB to `steamDB.index.json` (packageId → [Workshop ID, name]; smallest Workshop ID for duplicate packageIds; skip `appid` entries), avoiding repeated parsing of the 50 MB original.
- `archo data status`: source, version, download/check times, entry count per dataset; "old" after 30 days; automatic-check status: 13 §4.2.
- Manual/automatic updates share a lock; download/validation failure preserves files. Success advances database `community:generation` to invalidate other connections' compact indexes; GUI/TUI discard page analysis caches. Automatic failure notifications: 16 §2.16.
- Sources configurable: `config set data.rules.url`, `data.steamdb.url`, `data.replacements.url`; local paths allowed.
- Community data stores packageIds, resolved to Lineages on use.

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
