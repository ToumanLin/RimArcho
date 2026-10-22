# Labelling prompt — version corpuslabel-v2-t1 (tier 1: evidence packet only)

(Builds on corpuslabel-p3. v2: schema corpuslabel/2 with ordered cause-type rules, fix kind
tied to cause type, difficulty; packets show the library's version of each mod.)

You label the real cause of one error cluster from a RimWorld 1.6 Player.log (shared through
HugsLib). The evidence packet is everything you know: the cluster's message, extra lines and
stack, the log lines around it, the mods of the player's list that the evidence points at, and
facts from a research copy of those mods (current versions, which may differ from the player's;
textures and sounds were stripped from the research copy, so a file search never finds them).
You do not see any automatic verdict. Work only from the packet and general RimWorld modding
knowledge; do not use outside knowledge of specific mods.

## Who to name

Name a mod only when the packet shows *why* it is responsible, not merely that it is present.

| Class | What usually causes it | Who to name |
|---|---|---|
| patch-failed | A patch file's XPath matches nothing: the target def/node is absent (the other mod is missing or changed, or the patch is not guarded) | `source` = the mod that owns the patch (the `[Mod name]` prefix, path, `patches --failed`). `partner` = an *active* mod whose def the patch targets, if the packet shows it |
| xref-missing | A def references a def that nobody defines | `source` = the mod whose files contain the reference (`files search`, "defines"). `missing` = the mod that would define it, only if the packet names it |
| xml-load | A def's XML has a field or class the game does not know (often from an older version), or a malformed value; or an asset file fails to load | `source` = the mod whose files contain that XML or file (`files search`, a path in its folder) |
| texture-missing | A def or code asks for a texture/sound path that no active mod provides | `source` = the mod whose def or files name the path; if only Core/DLC match, verdict `unknown` |

Ludeon packages (`ludeon.rimworld*`) are the game: never a `source`.

**Corpus co-occurrence** lists mods of this list far more common in other players' logs with
the same error. Correlation, not proof. Only when the log itself names no mod, a mod in nearly
all those logs and rare in others (e.g. 95% vs 5%) whose kind fits the error may be the
`source` with confidence `low` or `medium`. Weak ratios (60% vs 25%) are not enough; popular
mods (`erdelf.humanoidalienraces`, translations) co-occur with everything.

## cause_type: take the FIRST rule that applies

1. `unknown` — your verdict is `unknown`.
2. `missing-dependency` — the source needs a mod that is not in the list, unconditionally (the
   packet says "depends on … NOT in the log's list", or its content uses that mod's
   defs/classes without a guard). Add that mod with role `missing`.
3. `optional-absent` — the source's content targets a mod/DLC that is **not active** and is
   meant as optional (a compatibility patch, a patch for another mod's def, a reference to an
   inactive mod's def), but the guard is missing or broken.
4. `load-order` — everything involved is active and moving one mod before/after another fixes
   it (packet: "declared rules broken here", or the target def comes from a mod later in the
   list). Add that other mod as `partner`.
5. `incompatible-mods` — everything is active, order is not the issue, and two mods change the
   same thing in conflicting ways. Add the other mod as `partner`.
6. `version-mismatch` — the packet shows the player's version of the source differs from the
   research library's **and** the library copy does not have the problem. (Rare in tier 1;
   needs a `versions` entry with `"same": "no"`.)
7. `outdated-mod` — the source uses an XML field, def, class or method that no longer exists in
   the current game or in the mod it builds on ("doesn't correspond to any field", "Could not
   find class/type" of another mod's namespace that is active).
8. `mod-bug` — the defect is in the source itself while everything it needs is there: a wrong
   path or def name in its own files, a texture it names but does not ship, malformed XML.

## fix.kind: must fit the cause type

| cause_type | allowed fix.kind | fix.mods |
|---|---|---|
| missing-dependency | `enable-dependency` | the missing mod |
| optional-absent | `ignore` (default) or `enable-dependency` | (the optional mod) |
| load-order | `reorder` | the mod to move |
| incompatible-mods | `disable`, `report`, `ignore` | the source |
| version-mismatch | `update` | the source |
| outdated-mod | `disable`, `report`, `ignore` | the source |
| mod-bug | `report`, `disable`, `ignore` | the source |
| unknown | `unknown` or `ignore` | (none) |

Choose by impact: `ignore` = only a missing texture/sound/compat tweak, the game recovers
(default for patch-failed, texture-missing, and "doesn't correspond to any field");
`report` = one feature of the mod breaks (default for "Could not find class/type");
`disable` = loading or a core loop breaks. `fix.mods` is required except for `ignore`/`unknown`.

## stage (default by class)

patch-failed → `patch-apply`; xml-load → `xml-load`; "Could not resolve cross-reference" →
`def-resolve`; "Failed to find X named" → the stage the stack shows (`static-init` when a
static constructor calls it, else `def-resolve`); "Could not load reference to" → `save-load`;
texture-missing → `static-init` (or `tick`/`ui` when it happens in play).

## difficulty

`L1` the message or one fact names the mod directly ("[Mod name]" prefix, a file path in its
folder, one file-search hit) · `L2` needs two or more facts or mods (who defines vs who
references, a missing dependency, load order, co-occurrence) · `L3` only code would decide.

## Output

Write exactly one JSON object, no comments, no markdown fence, all fields required:

```json
{
  "schema": "corpuslabel/2",
  "item": "<item id from the packet title>",
  "gist": "<log id from the packet's 'log:' line>",
  "key": "<copy the packet's 'key:' line, without 'key: '>",
  "verdict": "cause | unknown",
  "causes": [{"pkg": "<packageId as written in the packet>", "role": "source | partner | missing"}],
  "stage": "...",
  "cause_type": "...",
  "fix": {"kind": "...", "mods": ["<packageId>"], "note": "<one short sentence>"},
  "confidence": "high | medium | low",
  "difficulty": "L1 | L2 | L3",
  "evidence": ["<verbatim quote from the packet>"],
  "model": "<your model name>",
  "prompt": "corpuslabel-v2-t1",
  "note": "<one or two sentences: your reasoning>"
}
```

Checked by the validator: `causes` is empty unless `verdict` is `cause`; `source`/`partner`
packageIds come from the packet's list of mods; `missing` ones appear in the packet; `evidence`
holds 1–4 short quotes copied character for character from the packet (part of one line each);
`fix.kind` fits `cause_type` per the table; `missing-dependency` needs a `missing` cause;
`load-order` and `incompatible-mods` need a `partner`.
