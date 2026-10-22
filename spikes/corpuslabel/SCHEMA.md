# Label schema corpuslabel/2 (benchmark v2, G6)

One JSON object per (log, error cluster). `validate` checks every rule marked **[v]**.
The pilot (`corpuslabel/1`, RESULT.txt) agreed on source mods, but only 34% on cause type and
19% on fix kind: the enums were loose ("other", "update" the mod vs the game, "none" vs
"disable"). v2 decides both by ordered rules and ties fix kind to cause type.

```json
{
  "schema": "corpuslabel/2",
  "item": "XR07", "gist": "<log id>", "key": "<cluster key>",
  "verdict": "cause | unknown",
  "causes": [{"pkg": "<packageId>", "role": "source | partner | missing"}],
  "stage": "assembly-load | xml-load | patch-apply | def-resolve | static-init | game-start | save-load | map-gen | tick | ui | other",
  "cause_type": "missing-dependency | optional-absent | load-order | incompatible-mods | version-mismatch | outdated-mod | mod-bug | unknown",
  "fix": {"kind": "enable-dependency | reorder | update | disable | report | ignore | unknown", "mods": ["<packageId>"], "note": "<one sentence>"},
  "confidence": "high | medium | low",
  "difficulty": "L1 | L2 | L3",
  "evidence": ["<verbatim quote>", "log: <raw log line>", "env: <exec output>", "corpus: <co-occurrence>"],
  "code": [{"file": "<decompiled .cs>", "type": "Ns.Type", "method": "Method", "lines": "120-134", "text": "<verbatim>"}],
  "versions": [{"pkg": "<packageId>", "log": "<header version>", "installed": "<library modVersion>", "same": "yes | no | unknown"}],
  "model": "<model>", "prompt": "<prompt version>", "note": "<reasoning, 1-3 sentences>"
}
```

## Verdict and causes

- `cause`: the evidence shows *why* a mod is responsible (not only that it is present).
  `unknown`: it does not. An honest `unknown` beats a guess. **[v]** `causes` is empty unless
  `cause`, and has at least one entry with `cause`.
- Roles: `source` = the mod whose own content or code is wrong or out of date (the one to fix,
  update or drop); `partner` = the other active side of an ordering or incompatibility
  problem; `missing` = a mod that should be active and is not.
- **[v]** `source`/`partner` packageIds are in the log's list; a `missing` packageId appears in
  the packet (or the exec transcript). Ludeon packages are never a `source`.

## cause_type: take the FIRST rule that applies

| # | cause_type | Applies when | Needs |
|---|---|---|---|
| 1 | `unknown` | verdict is `unknown` | **[v]** unknown ⇔ unknown |
| 2 | `missing-dependency` | the source needs a mod/DLC that is not in the list, unconditionally: a declared dependency ("depends on … NOT in the log's list"), or its defs/code use that mod's types/defs without a guard (`MayRequire`, `FindMod`, a LoadFolders condition) | **[v]** a `missing` cause |
| 3 | `optional-absent` | the source's content targets a mod/DLC that is **not active** and is meant as optional (compatibility patch, `MayRequire`d def, patch for another mod's def, reference to an inactive mod's def), but the guard is missing or broken. The player loses nothing they had | |
| 4 | `load-order` | everything involved is active, and moving one mod before/after another removes the error (a declared loadAfter/loadBefore broken here; a patch whose target def comes from a mod later in the list) | **[v]** a `partner` |
| 5 | `incompatible-mods` | everything involved is active, order is not the issue, and two mods change the same thing in conflicting ways (one removes, renames or replaces what the other patches or calls; a declared incompatibleWith) | **[v]** a `partner` |
| 6 | `version-mismatch` | the player's copy of the source differs from the research library's (log version ≠ installed, or the Workshop copy was updated after the upload) **and** the current copy no longer has the defect (tier 2: the code or def at fault differs) | **[v]** a `versions` entry with `same: no` for a cause mod |
| 7 | `outdated-mod` | the source was written for an older game or an older version of a mod it builds on, and the current copy still is: an XML field, def, type or method that no longer exists ("doesn't correspond to any field", `TypeLoadException`/`MissingMethodException` on another assembly's member, 1.6 not in supported versions) | |
| 8 | `mod-bug` | the defect is in the source itself while everything it needs is present and current: a wrong path or def name in its own files, a missing null check, malformed XML, a texture it references but does not ship | |

## fix.kind: what the player should do

**[v]** The kind must fit the cause type:

| cause_type | allowed fix kinds | fix.mods |
|---|---|---|
| missing-dependency | `enable-dependency` | the missing mod(s) |
| optional-absent | `ignore` (default), `enable-dependency` (only if the packet shows the player wants that content) | the optional mod for enable-dependency |
| load-order | `reorder` | the mod to move first; the note says before/after which |
| incompatible-mods | `disable`, `report`, `ignore` | the mod to drop / report |
| version-mismatch | `update` | the source |
| outdated-mod | `disable`, `report`, `ignore` | the source |
| mod-bug | `report`, `disable`, `ignore` | the source |
| unknown | `unknown`, `ignore` | — |

Among the allowed kinds, choose by impact:
- `ignore`: nothing visible beyond a missing texture, sound or compatibility tweak; the game
  recovers by itself (failed optional patches, unknown XML fields that are dropped, missing
  sounds and textures, log-upload noise).
- `report`: one feature of the mod breaks, the rest works; keep the mod and tell its author.
- `disable`: loading, saving or a core loop breaks (an exception every tick, a failed static
  constructor or Mod constructor that aborts the mod's setup, a save that will not load).

Defaults by class (use unless the packet shows otherwise): patch-failed, texture-missing and
"doesn't correspond to any field" → `ignore`; "Could not find class/type …" and other
exceptions → `report`; failed static constructors / Mod constructors → `disable`.

**[v]** `fix.mods` is required except for `ignore` and `unknown`.

## stage: what the game was doing (default by class, override when the packet shows otherwise)

`assembly-load` (loading DLLs, listing their types, constructing `Mod` classes) ·
`xml-load` (reading def XML and asset files) · `patch-apply` (XML patches) · `def-resolve`
(cross-references, ResolveReferences, ConfigErrors) · `static-init`
(`[StaticConstructorOnStartup]`, after defs) · `game-start` (finishing load, main menu,
graphics init) · `save-load` (loading a save, `Could not load reference to …`) · `map-gen` ·
`tick` (in play) · `ui` (drawing, OnGUI, windows) · `other`.

Defaults: patch-failed → patch-apply; xml-load → xml-load; "Could not resolve cross-reference"
→ def-resolve; "Failed to find X named" → the stage the stack shows (static-init when
called from a static constructor); "Could not load reference to" → save-load;
texture-missing → static-init (graphics are built after defs), tick/ui in play.

## difficulty

- `L1` fact lookup: the message or one fact names the responsible mod directly (a "[Mod
  name]" patch prefix, a file path inside the mod's folder, the mod's own type as the
  innermost frame, a single file-search hit).
- `L2` cross-mod reasoning: needs two or more facts or mods (which mod defines vs references,
  a missing dependency, load order, Harmony patchers on a game method, co-occurrence).
- `L3` needs code: only reading decompiled code decides it. **[v]** a tier-2 `cause` label of
  difficulty L3 has at least one `code` quote.

## evidence, code, versions

- `evidence`: 1-4 quotes copied character for character from the packet. Prefixes cite other
  sources: `log:` (raw log; **[v]** against `raw/<gist>.txt` when `-corpus` is given),
  `env:` (an `exec` transcript; **[v]** against `<execlogs>/<item>.log`), `corpus:` (not checked).
- `code` (tier 2): the decisive decompiled code. `file` is the .cs file `archo source` wrote,
  `lines` its 1-based range, `text` verbatim (whitespace may differ). **[v]** the file exists,
  the text is in it at those lines (±3), and the type's and method's simple names occur in it.
- `versions`: for every mod the conclusion depends on, the header version and the library's
  modVersion. **[v]** `log` equals the header version recorded in the packet.

## Files (outside the repository: `corpus-kb\labels\v2\`)

`sample.jsonl` (items: gist, key, tier, bench class, stratum) · `rimarcho-v4.jsonl`
(RimArcho's verdict in the final run; never shown to labellers or agents) · `packets\<item>.txt`
(what a labeller reads) and `.json` (adds RimArcho's re-run verdict: not for agents) ·
`haiku\`, `t2\` (labeller labels) · `review\` (reviewer's labels, the gold where present) ·
`execlogs\<item>.log` (tool transcripts) · `costs.jsonl` · `tasks.jsonl` + `code\` (G7 tasks).
