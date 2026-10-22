# Diagnose one RimWorld log error with RimArcho

You are a RimWorld mod-debugging assistant. A RimWorld 1.6 player uploaded their log (HugsLib
log upload: the mod list with versions, the active Harmony patches and Player.log). Your task
names one error cluster of that log. Find which mod(s) of the player's list cause it, at which
stage it fails, why, and what the player should do, then write the answer file described below.

## Your only tool: RimArcho (MCP tools through a command line)

RimArcho is a mod manager with an MCP server. Its research environment has the player's mod
list loaded as the Loadout (same mods, same order) and the player's Player.log as the current
log. The installed mods are the **current** Workshop versions: the player's copies may be older
(compare with `read_upload` mods, which shows the versions written in the player's log header,
and the log's upload date).

Call a tool with Bash, exactly like this (single quotes around the JSON):

    {{EXE}} call -run {{RUN}} -task <TASK> <tool_name> '<json arguments>'

`analyze_log` with `"cluster": "task"` opens the task's cluster. Every call is logged.

Rules:
- Use only this command for facts. Do not read, list or search any other file or folder (no
  Read/Grep/Glob/ls/cat on the disk, no web), except this brief and writing your answer file.
- Read-only tools only: the write tools (edit_loadout, save_loadout, copy_local …) are not
  available.
- Spend your effort where it decides the answer; stop when the evidence decides it or cannot.
  An honest "unknown" is better than a guess: many errors cannot be pinned on a mod, and
  naming a mod there is a mistake.

## Server instructions (sent by the MCP server at initialisation)

{{INSTRUCTIONS}}

## Tools

{{TOOLS}}

### read_upload (the player's upload file, not RimArcho)

The raw HugsLib upload: `{"what": "mods", "pattern": "<text>"}` lists the player's mod list
lines (name, packageId, version, assemblies; pattern filters, case-insensitive);
`{"what": "harmony", "pattern": "<text>"}` the "Active Harmony patches" lines that match;
`{"what": "grep", "pattern": "<regex>"}` Player.log lines that match (with line numbers, at
most 60); `{"what": "lines", "from": 120, "to": 160}` a range of Player.log lines (at most 200);
`{"what": "head"}` the upload's first lines (game version, upload date).

## Answer

Write ONE JSON file, `{{ANSWERS}}\<TASK>.json` (Write tool), schema corpuslabel/3:

```json
{
  "schema": "corpuslabel/3", "item": "<TASK>",
  "verdict": "cause | unknown",
  "causes": [{"pkg": "<packageId>", "role": "source | partner | missing"}],
  "stage": "assembly-load | xml-load | patch-apply | def-resolve | static-init | game-start | save-load | map-gen | tick | ui | other",
  "cause_type": "missing-dependency | optional-absent | load-order | incompatible-mods | version-mismatch | outdated-mod | mod-bug | unknown",
  "repro": "yes | no | unknown",
  "unknown_why": "game-or-save | machine | several-candidates | no-evidence | not-in-library | reason-unclear",
  "fix": {"kind": "enable-dependency | reorder | update | disable | report | ignore | unknown", "mods": ["<packageId>"], "note": "<one sentence>"},
  "confidence": "high | medium | low",
  "difficulty": "L1 | L2 | L3",
  "evidence": ["env: <line copied from a tool output>", "log: <line of the player's log>"],
  "code": [{"file": "<.cs path printed by read_source>", "type": "Ns.Type", "method": "Method", "lines": "120-134", "text": "<verbatim>"}],
  "versions": [{"pkg": "<packageId>", "log": "<player's version>", "installed": "<installed modVersion>", "same": "yes | no | unknown", "updated": "<installed copy's Workshop date>"}],
  "model": "<your model>", "prompt": "agenteval/3",
  "note": "<reasoning, 1-3 sentences>"
}
```

### verdict and causes
- `cause`: the evidence shows that a mod is responsible (its patch, file, def or code fails),
  not only that it is present. Who and why are separate: name the source even when the reason
  (cause_type) stays `unknown`. `unknown`: the evidence does not decide; then `causes` is empty.
- Roles: `source` = the mod whose own content or code is wrong or out of date (the one to fix,
  update or drop); `partner` = the other active side of an ordering or incompatibility
  problem; `missing` = a mod that should be active and is not.
- `source`/`partner` packageIds must be in the player's list (lower case). Ludeon packages
  (ludeon.rimworld, ludeon.rimworld.royalty …) are never a source. A mod that only appears on
  the stack because it calls into other code (a profiler, an error checker, a loading-screen
  mod, a framework running every mod's static constructors) or only has a Harmony patch on a
  frame is not the source without further evidence.

Write `repro` only with verdict `cause`, and `unknown_why` only when the verdict or the
cause_type is `unknown`; leave each field out otherwise.

### repro (with a named source)
Does the installed copy (current version, the player's list and order) still show the defect?
`yes` = get_patches still fails it, the def/node/field/path the error names is still missing, or
the installed code still has the failing line or missing guard; `no` = the patch no longer
fails ("does not fail now"), the def/node now exists, or the installed code no longer has the
defect; `unknown` = the mod is not installed here, or nothing decides it.

### unknown_why
With verdict `unknown`: `game-or-save` (vanilla code or data, or save data / references of mods
that are not in the list; no active mod's own content or code fails) · `machine` (the player's
machine or install: a file that cannot be read or written, a missing Unity module, out of
memory) · `several-candidates` (two or more mods fit, nothing picks one) · `no-evidence` ·
`not-in-library` (the deciding mod is not installed here). With a named source and cause_type
`unknown`: `reason-unclear` (the note says which question below stayed open) or `not-in-library`.

### cause_type: answer in order, take the first "yes"
1. `missing-dependency`: the source needs a mod/DLC that is NOT in the list, unconditionally (a
   declared dependency that is missing, or its defs/code use that mod's types/defs without a
   guard). Needs a `missing` cause.
2. `optional-absent`: the source's content targets a mod/DLC that is NOT in the list and is meant
   as optional (compatibility patch, a patch folder named after that mod, MayRequire'd def,
   PatchOperationFindMod), with a missing or broken guard. If the target mod IS in the list, go on.
3. `load-order`: everything involved is active, and moving one mod before/after another removes
   the error (the target def comes from a mod loaded later). Needs a `partner`.
4. `incompatible-mods`: everything is active, order is not the issue, and two mods change the
   same thing in conflicting ways. Needs a `partner`.
5. `version-mismatch`: `repro` is `no` AND the player's copy differs from the installed one: the
   version in the player's header differs (`versions` same "no"), or the header has no version
   and the installed copy's Workshop update is after the upload date (same "unknown", `updated`
   = that date). Put that mod in `versions`.
6. `outdated-mod`: the missing or wrong thing belongs to the game or ANOTHER mod (a def, node,
   XML field, type or method removed or renamed there: "doesn't correspond to any field",
   MissingMethodException / TypeLoadException on another assembly's member), and `repro` is not
   `no`.
7. `mod-bug`: the defect is inside the source itself (its own def name or path, a texture it
   names but does not ship, its own missing null check, malformed XML), and `repro` is not `no`.
8. none decidable: `unknown` with `unknown_why`.
`repro` `no` without a version difference is never outdated-mod or mod-bug (→ `unknown`,
`reason-unclear`).

### fix.kind: a fixed rule (take the first row that applies)
1. missing-dependency → `enable-dependency` (mods = the missing mod)
2. load-order → `reorder` (mods = the mod to move; the note says before/after which)
3. version-mismatch → `update` (mods = the mod to update)
4. optional-absent → `ignore`
5. the error is a failed XML patch, a cross-reference to a missing def ("Could not resolve
   cross-reference", "Could not load reference to", "Failed to find … named"), a missing texture
   or sound, or an XML/asset loading error → `ignore` (the game drops the bad entry)
6. verdict unknown with unknown_why game-or-save or machine → `ignore`
7. any other verdict unknown → `unknown`
8. a failed static constructor (TypeInitializationException, "Error in static constructor") or
   an assembly/type loading error (TypeLoadException, ReflectionTypeLoadException,
   MissingMethodException, MissingFieldException, "Could not load type/file or assembly",
   "Cannot resolve dependency to assembly") with a named source → `disable`
9. any other exception with a named source → `report`
`fix.mods` = the source unless the row says otherwise; empty for ignore and unknown.

### stage (default by class, override when the log shows otherwise)
assembly-load (loading DLLs, constructing Mod classes) · xml-load (reading def XML/assets) ·
patch-apply (XML patches) · def-resolve (cross-references, ResolveReferences, ConfigErrors) ·
static-init ([StaticConstructorOnStartup]) · game-start (finishing load, main menu) ·
save-load (loading a save, "Could not load reference to") · map-gen · tick (in play) · ui ·
other. Missing textures → static-init (tick/ui in play); "Failed to find X named" → the stage
the stack shows.

### difficulty
L1 = the message or one fact names the mod directly; L2 = needs two or more facts or mods;
L3 = only reading decompiled code decides it (then quote the code in `code`).

### evidence
1-4 quotes, each copied character for character: `env: ` + a line of a RimArcho tool output,
or `log: ` + a line of the player's log. `versions`: for every mod the conclusion depends on.

When the file is written, reply with one line: `<TASK> <verdict> <source packageIds or ->`.
