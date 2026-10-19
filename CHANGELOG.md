# Changelog

All notable changes to RimArcho. Versions follow semantic versioning; the MVP is 0.5.0.

## 0.5.0 — MVP (unreleased)

### M5: local copies, RimCrow import, exchange, polish
- `copy-local <ref> [--name] [--git]` copies a mod into `Mods/`, writes `.rimarcho/origin.json`
  and can add a git baseline commit. It never overwrites a folder, never copies the upstream `.git`,
  never follows links that leave the mod folder, and does not change the Loadout. The MCP tool is
  `copy_local`.
- Player data on lineages: `alias`, `note`, `tag add|remove|list`. The alias is shown as the mod's
  name; `list --filter tag:<t>` filters by tag.
- `import rimcrow --db <mod_manager.db> [--dry-run]` imports aliases, notes, groups (as tags),
  interlocks (as attach rules) and user rules. It reads copies only and can be run again safely.
  The report also lists the about-disabled mods, with guidance.
- `loadout export [--format text|modsconfig] [--out]` and `loadout import <file>` (a text list,
  a ModsConfig.xml or a save's mod list).
- TUI Changes view: identity questions pinned on top (same / copy / unrelated), load or ignore an
  external ModsConfig.xml change, events grouped by scan and type, and file-level diffs on `enter`.
- Mouse support in the TUI: the wheel scrolls and a click selects. `config set ui.mouse off`
  turns it off.
- `archo version` / `--version`; `archo <command> --help`.
- `scripts/release.ps1` builds one `archo.exe` with the version stamped in through ldflags.
- M4 rulings:
  - `harmony.skip-prefix` is info and becomes a warning only when the method is in a stack of the
    newest log.
  - The newest Player.log is included in `problems` automatically.
  - A log from another game version is attributed against that version.
  - `source search` needs a scope (`--in`, `--implicated`, or `--active --yes`, which shows an
    estimate first).

### M4: Harmony, logs, decompiling, MCP
- Harmony patches are read from mod assemblies: prefixes, postfixes, transpilers, finalizers,
  bool prefixes, manual `Harmony.Patch` and Harmony IDs. New findings: `harmony.multi-transpiler`,
  `harmony.skip-prefix`, `harmony.shared-target` and `harmony.manual`. New command: `harmony`.
- `log`: Player.log clusters, each attributed to mods as source / patcher / dependency / mentioned,
  with evidence. Patch failures are matched to the simulation, and missing cross-references list
  their referrers and providers.
- `source` / `source search`: on-demand ilspycmd, cached per DLL hash.
- `archo mcp`: 15 tools over stdio whose text is identical to the CLI, with readOnly/destructive
  annotations and descriptions that cover the diagnosis workflow.
- `--game-version X.Y` on the analysis commands; `version.unsupported` is folded into one row;
  noisy info is hidden by default.

### M3: problems
- The declaration layer (dependencies, incompatibilities, order, duplicates, versions) and the Def
  layer (inheritance after patching, overrides, abstract collisions).
- A PatchOperation simulator with no-match reasons (`target-inactive`, `target-missing`,
  `target-later`, `wrong-version-route`, `stale-name`, `unguarded-optional`, `node-absent`).
- Commands `problems`, `def`, `patches`, `why` and `files search`; TUI Problems view and Inspect.

### M2: Loadout
- A workspace Loadout with history, undo/redo, versions, tags, diff and checkout; `save` takes a
  backup and replaces ModsConfig.xml atomically.
- Constraint sort with explanations, user rules, and `launch`. TUI list view.

### M1: fact base
- Environment detection, sources (official, local, Workshop, SteamCMD, extra), incremental scans,
  identity reconciliation (lineages, copies, moves, tombstones, questions) and change events.
- Commands `env`, `scan`, `status`, `list`, `find`, `mod` and `changes`.
