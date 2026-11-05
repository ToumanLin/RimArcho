# RimArcho architecture

> Status: rewritten 2026-10-21. The top-level design (00) covers what to build and for whom; this document covers system organization. Specific algorithms and rules are in `docs/specs/`.
> Principle: use RimCrow only as a source of requirements and edge cases; do not reuse its interface, navigation or module structure.

## 1. Domain model

```
Environment (one game directory: version, launch method, saves—shared default or separate; a game directory belongs to only one environment)
  ├─ Source (owned by this game: official Data / local Mods)
  ├─ Plan (an ordered Instance list, with undo stack, version history and staging; multiple per environment)
  ├─ Pack (a fixed artifact compiled from a plan, with its own save directory)
  └─ Launch (one launch: which plan or pack, when, what was enabled)
Mod folders (Sources shared across all environments: Workshop / SteamCMD / other directories / Archo Monitor / lock library)
Source
  └─ Instance (one Mod on disk, with an ID assigned by RimArcho; the same Instance across all environments for shared folders)
       ├─ Snapshot (metadata and content fingerprint from a scan)
       └─ Facts: ModDecl · Def · PatchOp · TypeDef · HarmonyPatch (read per game version, shared by content set)
Lineage (the family of "the same Mod": original, locked versions, local copies, before/after renaming; global)
Rule (user / community / replacement rule; user rules attach to Lineage)
Finding (conclusion + evidence[])
LogRun (one Player.log, with error clusters attributed to Instance / Finding, belonging to a Launch)
Save (one save: colony, enabled Mods at the time, referenced Defs; 1.1)
```

- Plan entries point to Instance, not packageId: players choose "this copy".
- A locked version is not a new concept; it is another Instance in the same Lineage (04 §3, 07 §5).
- One record per Mod: Workshop, SteamCMD and similar folders are shared by all games, so their Instances, Lineages and player-authored data are not split by environment. An environment owns only its `Data`, `Mods`, plans, launch records and deployment links (03 §2).
- ModsConfig.xml is a projection of a plan, not a source of truth. Write it only when launching a plan or writing to the game (07 §4, 09 §2).

## 2. Module layers and delivery

```
┌────────────────── Thin surfaces ────────────────────────┐
│ GUI (Wails)   TUI (Bubble Tea)   CLI   MCP server   Archo │
└───────────┬───────────────────────────────┬─────────────┘
┌───────────▼──────────────┐    ┌───────────▼──────────────┐
│ View models             │    │ Tool layer: commands    │
│ internal/view           │    │ and tool descriptions   │
│ (shared by GUI and TUI)  │    │ (CLI, MCP, Archo)        │
└───────────┬──────────────┘    └───────────┬──────────────┘
┌───────────▼──────────────────────────────▼──────────────┐
│ Application internal/app: plan editing, launching,      │
│ packs, import/export…                                  │
├────────────────────────────────────────────────────────┤
│ Analysis: effective content · sorting · Def · Patch ·   │
│ Harmony · log attribution                              │
├────────────────────────────────────────────────────────┤
│ Identity: Instance reconciliation · Lineage · changes   │
├────────────────────────────────────────────────────────┤
│ Facts: SQLite, updated incrementally by snapshot        │
├────────────────────────────────────────────────────────┤
│ Collection: filesystem · ACF · ModsConfig · Player.log ·│
│ .NET metadata · community data · saves (1.1)            │
└────────────────────────────────────────────────────────┘
External: Steam client (subscriptions, downloads, launching), ilspycmd (source on demand), model providers (Archo, 1.1)
```

- **View models**: `internal/view` provides what pages show and what each object can do right now, directly serializable to JSON. GUI and TUI only render and map input (16 §1).
- **Tool layer**: CLI, MCP and Archo call the same commands; CLI and MCP output is identical, word for word (13 §1).
- **Analysis runs in subprocesses**: `archo analyze` computes, writes to the database and exits; UI and MCP only read results (05 §3).

The same source links into two programs (17 §5):

| Program | Purpose |
| --- | --- |
| `RimArcho.exe` | Double-click opens the desktop window (GUI); command arguments run CLI actions, for scan and analysis subprocesses |
| `archo.exe` | No arguments: TUI in the current terminal; `archo <command>`: one-shot CLI; `archo mcp`: stdio MCP server; `archo gui`: open the window |

## 3. Technical principles

1. **One core, multiple surfaces.** GUI, TUI, CLI, MCP and Archo use the same queries and commands. Business rules belong only in application and view-model layers. "If this is a Workshop Mod…" in the interface layer is a defect.
2. **Referenceable entities.** Every object has a stable reference (`#a3f9`, `def:ThingDef/Gun_Revolver`, `log:E42`, `p:3f2a1c`, etc.; full list in 13 §2), usable in any output and directly in the next command.
3. **Conclusions require evidence.** Every finding states the conclusion, evidence references and source types (declaration, inference, log, community data, player) (05 §4).
4. **Undoable changes.** Every plan edit enters the undo stack; historical versions can be restored (07 §3). Actions that cannot be undone (deletion other than game-file writes, identity merges) require confirmation (16 §6).
5. **Always know where you are, where you can go and how to return.** The interface always shows the current location and available actions; `esc` or breadcrumbs return one level (16 §3.1).
6. **Use few resources by default.** No resident process; incremental indexes; heavy work runs in subprocesses that exit when done (18).
7. **Write only listed files.** RimArcho writes only the files listed in 02 §1.

## 4. Identity design rationale

Specific rules are in 02. This section explains the reasons.

### 4.1 No reliable natural primary key

| Signal | Failure case |
| --- | --- |
| Absolute path | Moving the game or library directory makes everything a "new Mod" |
| Workshop ID | After a local copy, its `About/PublishedFileId.txt` still claims the same ID |
| packageId | Authors can change it; different Mods can collide; one package can have multiple copies |
| Name, author | Can change at any time |
| Content hash | Changes on every update |

Identity therefore cannot be "read"; RimArcho must maintain it. Assign an Instance ID when first seeing a Mod; on later scans, use multiple signals to reconcile new observations with known Instances. Ask the player to confirm unmatched cases rather than guess. This is the same approach as git detecting renames or a photo library handling moved folders.

Consequences:
- Store only paths relative to the source root. When a game directory moves, fix it as a whole through source rebinding rather than guessing per Mod (03 §4.2).
- Use modification times only to skip unchanged files, not to identify Mods, because copying resets them.
- Turn missing Mods into tombstones for a retention period, covering unsubscribe-then-resubscribe cases (04 §2.5).

### 4.2 Copies carry their origin

When creating a locked version or local copy, RimArcho writes `.rimarcho/origin.json` in the copy, recording the source Instance, Lineage, Workshop ID and snapshot at copy time. The next scan attaches it directly to the same Lineage, marked "confirmed". Infer manually made copies by content similarity, marked "inferred" (04 §2.1).

RimArcho never writes marker files into Workshop directories: Steam verification or updates would delete them.

### 4.3 Where data belongs

| Data | Attached to | Reason |
| --- | --- | --- |
| Plan entry | Instance | Players choose "this copy" |
| User rules, aliases, tags, notes | Lineage (global) | Original and locked versions should share them; renaming must not lose them; the Mod is the same in every game |
| Ignored finding | Lineage + environment | A finding judges "this game with these Mods together"; another game version may change it (03 §2) |
| Analysis results | Runs scoped to environment / version / enabled set; Instance content sets and version views | Select a set by actual effective content for the version; storage rules in 05 §3 |
| Community and replacement rules | packageId (resolved to Lineage when matching) | External data knows only packageId |
| ModsConfig.xml | Projection of a plan | Generated on launch or write to the game |

Direct benefit: when an author changes packageId, plans and user rules stay connected. RimArcho reports "package ID changed from A to B" and uses the new ID on the next ModsConfig.xml write.

### 4.4 Known difficult cases

- An author forks a new Mod with a different Workshop ID but the same packageId: separate Lineages and a "duplicate package ID" finding.
- The same Workshop ID in both Steam Workshop and SteamCMD directories: two Instances, one Lineage.
- A very old Mod with no packageId: derive a temporary package ID by game rules, marked inferred (04 §1.5).
- The player moves the game and deletes Mods at the same time: rebind the source first; remaining gaps become identity questions.
- Two environments share a Workshop directory: Workshop is a Mod folder shared by all environments, with one Instance and Lineage per Mod (03 §2).