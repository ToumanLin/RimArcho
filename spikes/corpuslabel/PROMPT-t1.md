# Labelling prompt — version corpuslabel-v3-t1 (tier 1: evidence packet only)

(v3, G12: schema corpuslabel/3 — new fields `repro` and `unknown_why`; cause type by ordered
questions with a patch-failed checklist; fix kind is a fixed rule. v2.2: who and why are separate.)

You label the real cause of one error cluster from a RimWorld 1.6 Player.log (shared through
HugsLib). The evidence packet is everything you know: the cluster's message, extra lines and
stack, the log lines around it, the mods of the player's list that the evidence points at, and
facts from a research copy of those mods (current versions, which may differ from the player's;
textures and sounds were stripped from the research copy, so a file search never finds them).
You do not see any automatic verdict. Work only from the packet and general RimWorld modding
knowledge; do not use outside knowledge of specific mods.

## Step 1 — who (verdict and causes)

Name a mod only when the packet shows that it is responsible (its patch, file or def is the one
failing), not merely that it is present. **Who and why are separate**: when the packet shows whose
patch/file/def fails but not why, give verdict `cause` with that `source` and cause type `unknown`.

| Class | What usually causes it | Who to name |
|---|---|---|
| patch-failed | A patch's XPath matches nothing: the target def/node is absent | `source` = the mod that owns the patch (`[Mod name]` prefix, the file path, `patches --failed`); `partner` = an *active* mod whose def the patch targets, only for load-order/incompatible-mods |
| xref-missing | A def references a def that nobody defines | `source` = the mod whose files contain the reference; `missing` = the mod that would define it, only if the packet names it |
| xml-load | A def's XML has a field or class the game does not know, or a malformed value; or an asset file fails to load | `source` = the mod whose files contain that XML or file |
| texture-missing | A def or code asks for a texture/sound path that no active mod provides | `source` = the mod whose def or files name the path; if only Core/DLC match, verdict `unknown` |

Ludeon packages (`ludeon.rimworld*`) are the game: never a `source`.
**Corpus co-occurrence** is correlation, not proof: only when the log itself names no mod, a mod in
nearly all those logs and rare in others (e.g. 95% vs 5%) whose kind fits the error may be the
`source`, with confidence `low` or `medium`.

If you name no source, set `unknown_why`:
`game-or-save` (vanilla data, or save data / references of mods not in the list — e.g. "Could not
load reference to …" of a removed mod) · `machine` (a file that cannot be read or written, the
player's install) · `several-candidates` (two or more mods fit, nothing picks one) · `no-evidence`
(nothing points at any mod) · `not-in-library` (the deciding mod is "not in the research library").

## Step 2 — repro (only with a named source)

Does the research copy (same list, current versions) still show the defect?
`no` = the packet says "simulation: this operation does not fail now", or the def/node/path the
error names now exists in the research copy. `yes` = the simulation still fails, or the
def/node/field is still missing. `unknown` = the source is "not in the research library", or the
packet has no fact either way.

## Step 3 — cause_type: answer in order, take the first "yes"

1. **missing-dependency** — the source needs a mod/DLC that is NOT in the list, unconditionally
   ("depends on … NOT in the log's list", or it uses that mod's defs without a guard). Add that
   mod with role `missing`.
2. **optional-absent** — the source's content targets a mod/DLC that is NOT in the player's list
   and is meant as optional (a compatibility patch, a patch folder named after that mod, a
   `PatchOperationFindMod`, a reference to that mod's def). If the target mod IS in the list, this
   is not optional-absent: go on.
3. **load-order** — everything is active and the target def comes from a mod loaded later, or a
   declared loadAfter/loadBefore is broken here. Add the other mod as `partner`.
4. **incompatible-mods** — everything is active, order is not the issue, and two mods change the
   same thing in conflicting ways. Add the other mod as `partner`.
5. **version-mismatch** — `repro` is `no` AND the player's copy differs: the header version
   differs from the library's (`versions` `same: "no"`), or the header has no version and the
   packet's "Workshop updated <date>" is after the log's "uploaded:" date (`same: "unknown"`,
   `updated: "<date>"`). Put that mod in `versions`.
6. **outdated-mod** — the missing or wrong thing belongs to the game or ANOTHER mod that is in the
   list (its def, node or field was removed or renamed): e.g. "doesn't correspond to any field",
   an XPath into another active mod's def that lacks that node.
7. **mod-bug** — the defect is inside the source itself: its own def name or path, a texture it
   names but does not ship, malformed XML in its own files.
8. none decidable → `unknown` with `unknown_why` `reason-unclear` (say in the note which question
   you could not answer) or `not-in-library`.

`repro: "no"` without a version difference is never outdated-mod or mod-bug (rule 5 fails →
`unknown`, `reason-unclear`). outdated-mod and mod-bug need `repro` `yes` or `unknown`.

**Patch-failed checklist.** (a) Read the target: the XPath's `defName` and the patch file's folder
(`Patches/<OtherMod>/…`). (b) Target mod not in the list → 2. (c) Target in the list: the packet's
`def` lookup — defined by a mod loaded after the patch's mod → 3; the def exists but not the node →
5 or 6; the def exists nowhere → 6 if the name belongs to the game or another mod, else 7.
(d) `repro` from the simulation line.

## Step 4 — fix.kind: a fixed rule (do not judge impact)

Take the first row that applies:
1. missing-dependency → `enable-dependency` (mods = the missing mod)
2. load-order → `reorder` (mods = the mod to move; note says before/after which)
3. version-mismatch → `update` (mods = the mod to update)
4. optional-absent → `ignore`
5. otherwise, in tier 1 (patch-failed, xref-missing, texture-missing, xml-load) → `ignore`

So every tier-1 label without rows 1-3 has `"fix": {"kind": "ignore", "mods": [], "note": "…"}`.

## stage (default by class)

patch-failed → `patch-apply`; xml-load → `xml-load`; "Could not resolve cross-reference" →
`def-resolve`; "Failed to find X named" → the stage the stack shows (`static-init` when a static
constructor calls it, else `def-resolve`); "Could not load reference to" → `save-load`;
texture-missing → `static-init` (or `tick`/`ui` when it happens in play).

## difficulty

`L1` the message or one fact names the mod directly ("[Mod name]" prefix, a file path in its
folder, one file-search hit) · `L2` needs two or more facts or mods (who defines vs who
references, a missing dependency, load order, co-occurrence) · `L3` only code would decide.

## Output

Write exactly one JSON object, no comments, no markdown fence:

```json
{
  "schema": "corpuslabel/3",
  "item": "<item id from the packet title>",
  "gist": "<log id from the packet's 'log:' line>",
  "key": "<copy the packet's 'key:' line, without 'key: '>",
  "verdict": "cause | unknown",
  "causes": [{"pkg": "<packageId as written in the packet>", "role": "source | partner | missing"}],
  "stage": "...",
  "cause_type": "...",
  "repro": "yes | no | unknown",
  "unknown_why": "game-or-save | machine | several-candidates | no-evidence | not-in-library | reason-unclear",
  "fix": {"kind": "...", "mods": ["<packageId>"], "note": "<one short sentence>"},
  "confidence": "high | medium | low",
  "difficulty": "L1 | L2 | L3",
  "evidence": ["<verbatim quote from the packet>"],
  "versions": [{"pkg": "<packageId>", "log": "<header version or \"\">", "installed": "<library version or \"\">", "same": "yes | no | unknown", "updated": "<Workshop date or \"\">"}],
  "model": "<your model name>",
  "prompt": "corpuslabel-v3-t1",
  "note": "<one or two sentences: your reasoning>"
}
```

Write `repro` only with verdict `cause` (leave the field out otherwise) and `unknown_why` only
when the verdict or the cause type is `unknown` (leave it out otherwise).
`versions` is required for version-mismatch, optional otherwise. Checked by the validator:
`causes` empty unless `verdict` is `cause`; `source`/`partner` packageIds come from the packet's
list of mods; `missing` ones appear in the packet; `evidence` holds 1–4 short quotes copied
character for character from the packet; `repro` and `unknown_why` as above; `fix.kind` equals the
rule; `missing-dependency` needs a `missing` cause; `load-order` and `incompatible-mods` need a
`partner`; a version's `log` equals the header version in the packet.
