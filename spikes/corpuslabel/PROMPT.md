# Labelling prompt — version corpuslabel-p3

(p2: packets may carry a "Corpus co-occurrence" fact; see the last rule under "How to decide". p3: a mod frame that calls straight into the throwing game method is the lead; packets may show lines just after the message.)

You label the real cause of one error cluster from a RimWorld 1.6 Player.log (shared through
HugsLib). The evidence packet is everything you know: the cluster's message, extra lines and
stack, the log lines before it, the mods of the player's list that the evidence points at, and
facts from a research copy of those mods (current versions, which may differ from the player's).
You do not see any automatic verdict. Work only from the packet; do not use outside knowledge of
specific mods except general RimWorld modding knowledge below.

## How to decide

Name a mod only when the packet shows *why* it is responsible, not merely that it is present.
Being in the stack, or patching a method in the stack, makes a mod a suspect, not a cause.

| Class | What usually causes it | Who to name |
|---|---|---|
| patch-failed | A patch file's XPath matches nothing: the target def/node is absent (the other mod is missing, outdated, or the patch is not guarded) | `source` = the mod that owns the patch file (path, `[Mod name]` prefix, `patches --failed`). `partner` = the mod whose def the patch targets, if the packet shows it is active but different |
| xref-missing | A def references another def that nobody defines | `source` = the mod whose files contain the reference (`files search`), if the packet shows it. `missing` = the mod that would define it, only if the packet names it (e.g. a dependency not in the list) |
| xml-load | A def's XML has a field the game does not know (often an old-version field), or a malformed value | `source` = the mod whose files contain that XML (`files search`) |
| texture-missing | A def or code asks for a texture path that no active mod provides | `source` = the mod whose files contain the path; if only Core/DLC files match, the cause is not shown: verdict `unknown` |
| exception / harmony-stack | Code throws. The innermost mod frame (a type a mod's assembly defines) is the best lead, also when it sits just below one or two game frames: the mod called the game with input the game could not handle. A Harmony patch on the throwing game method is a lead when no mod frame is in the stack | `source` = the mod whose code throws or calls the throwing game code, if a frame or a patch points at it |
| save-load | References in a save to things that no longer exist (usually a mod removed mid-game) | almost always `unknown` unless a mod is named |
| material | Shader/material problems of a mod's assets | `source` only if the packet ties the material to a mod |

Ludeon packages (`ludeon.rimworld*`) are the game: never name them as `source`.

**Corpus co-occurrence** lists mods of this list that appear far more often in other players'
logs with the same error. It is correlation, not proof. When the log itself names no mod (no mod
frame, no file path, no file search hit), a mod that is in nearly all of those logs and rare in
the others (for example 95% vs 5%) is a reasonable `source` with `confidence` `low` or `medium`,
if its kind fits the error (a library for a shader warning, a patcher for an assembly loading
error). Weak ratios (like 60% vs 25%) are not enough on their own. Popular mods
(`erdelf.humanoidalienraces`, translation packs) co-occur with everything; do not blame them for
co-occurrence alone.

Use `"verdict": "unknown"` when the packet does not identify a responsible mod. An honest
`unknown` is worth more than a guess. Use `"benign"` for messages that need no action (cosmetic
warnings the game recovers from).

## Output

Write exactly one JSON object, no comments, with these fields (all required; use the exact
words from the allowed lists):

```json
{
  "schema": "corpuslabel/1",
  "item": "P07",
  "gist": "<log id from the packet's 'log:' line>",
  "key": "<copy from the task>",
  "verdict": "cause | unknown | benign",
  "causes": [{"pkg": "<packageId as written in the packet, lower case>", "role": "source | partner | missing"}],
  "stage": "assembly-load | xml-load | patch-apply | def-resolve | static-init | game-start | save-load | map-gen | tick | ui | other",
  "cause_type": "missing-dependency | load-order | incompatible-mods | outdated-mod | game-version | mod-bug | missing-content | other",
  "fix": {"kind": "disable | enable-dependency | reorder | update | other | none", "mods": ["<packageId>"], "note": "<one short sentence>"},
  "confidence": "high | medium | low",
  "evidence": ["<verbatim quote from the packet>", "..."],
  "model": "<your model name>",
  "prompt": "corpuslabel-p3",
  "note": "<one or two sentences: your reasoning>"
}
```

Rules checked by the validator:
- `causes` is empty unless `verdict` is `cause`; with `cause` it has at least one entry.
- Every `pkg` with role `source` or `partner` must be a packageId from the player's list as shown
  in the packet; role `missing` must appear somewhere in the packet text.
- `evidence`: 1–4 short quotes copied character for character from the packet (a line or part of
  a line). No paraphrase.
- Stage: when the error happens. `assembly-load` (DLL loading), `xml-load` (reading def XML),
  `patch-apply` (XML patches), `def-resolve` (cross-references, post-load), `static-init`
  (static constructors), `game-start` (finishing load, UI/menus), `save-load` (loading a save),
  `map-gen`, `tick` (in play), `ui` (drawing/OnGUI).
