# Label schema corpuslabel/3 (benchmark v3, G12)

One JSON object per (log, error cluster). `validate` checks every rule marked **[v]**.

History. The pilot (`corpuslabel/1`) agreed on source mods but only 34% on cause type and 19%
on fix kind. `corpuslabel/2` (G6, benchmark v2) decided both by ordered rules; labeller vs
reviewer then agreed 69-97% on cause type and 88-93% on fix kind, but agents vs gold only 57-71%
and 61-76%, and 21 of 73 named sources had cause type `unknown`. The disagreements were of
three kinds: version-mismatch vs unknown/outdated-mod (nobody wrote down whether the current copy
still fails), report vs disable and ignore vs unknown (fix kind was a judgement of impact), and
unknown used both for "no mod is responsible" and "I could not tell". `corpuslabel/3` adds two
fields and turns the fix kind into a rule:

- `repro`: does the research environment (same list and order, current library copies) still
  show the defect? It decides version-mismatch vs outdated-mod/mod-bug.
- `unknown_why`: why the verdict or the cause type stays unknown (an enum).
- `fix.kind` follows from cause type, class and verdict (table below); the validator computes it.

Every corpuslabel/2 rule still holds unless this file says otherwise.

```json
{
  "schema": "corpuslabel/3",
  "item": "XR07", "gist": "<log id>", "key": "<cluster key>",
  "verdict": "cause | unknown",
  "causes": [{"pkg": "<packageId>", "role": "source | partner | missing"}],
  "stage": "assembly-load | xml-load | patch-apply | def-resolve | static-init | game-start | save-load | map-gen | tick | ui | other",
  "cause_type": "missing-dependency | optional-absent | load-order | incompatible-mods | version-mismatch | outdated-mod | mod-bug | unknown",
  "repro": "yes | no | unknown",
  "unknown_why": "game-or-save | machine | several-candidates | no-evidence | not-in-library | reason-unclear",
  "fix": {"kind": "enable-dependency | reorder | update | disable | report | ignore | unknown", "mods": ["<packageId>"], "note": "<one sentence>"},
  "confidence": "high | medium | low",
  "difficulty": "L1 | L2 | L3",
  "evidence": ["<verbatim quote>", "log: <raw log line>", "env: <exec output>", "corpus: <co-occurrence>"],
  "code": [{"file": "<decompiled .cs>", "type": "Ns.Type", "method": "Method", "lines": "120-134", "text": "<verbatim>"}],
  "versions": [{"pkg": "<packageId>", "log": "<header version>", "installed": "<library modVersion>", "same": "yes | no | unknown", "updated": "<library Workshop date>"}],
  "model": "<model>", "prompt": "<prompt version>", "note": "<reasoning, 1-3 sentences>"
}
```

## Verdict and causes (unchanged from v2)

- `cause`: the evidence shows that a mod is responsible (its patch, file, def or code fails), not
  only that it is present. Who and why are separate: name the source even when the reason (cause
  type) stays `unknown`. `unknown`: it does not. **[v]** `causes` is empty unless `cause`, and
  has at least one entry with `cause`.
- Roles: `source` = the mod whose own content or code is wrong or out of date; `partner` = the
  other active side of an ordering or incompatibility problem; `missing` = a mod that should be
  active and is not.
- **[v]** `source`/`partner` packageIds are in the log's list; a `missing` packageId appears in
  the packet (or the exec transcript). Ludeon packages are never a `source`.

## repro (new)

**[v]** Required with verdict `cause`, absent otherwise. Does the research environment — the same
list and order with the library's current copies — still show the defect?

| value | tier 1 (packet facts) | tier 2 (code) |
|---|---|---|
| `yes` | the simulation still fails, or the def/node/field/type/path the error names is still missing (`def`, `files search`) | the library code still has the failing line / missing member / missing guard |
| `no` | "simulation: this operation does not fail now", or the def/node/path the error names now exists | the library code no longer has the defect (null check added, member exists, line changed) |
| `unknown` | the source is not in the library, or the packet has no fact either way | the code is unavailable or does not decide |

## unknown_why (new)

**[v]** Required whenever verdict or cause type is `unknown`; absent otherwise.

| value | when | allowed with |
|---|---|---|
| `game-or-save` | the failure is in vanilla code or data, or in save data / references of mods that are not in the list; no active mod's own content or code fails | verdict unknown |
| `machine` | the player's machine or install: a file that cannot be written or read, a missing Unity module, out of memory, a corrupt or partial download | verdict unknown |
| `several-candidates` | two or more mods fit and the evidence does not pick one | verdict unknown |
| `no-evidence` | nothing in the packet, log or code points at any mod | verdict unknown |
| `not-in-library` | the mod whose content or code would decide is not in the research library | either |
| `reason-unclear` | the source is clear, but none of the cause-type tests below can be decided; the note says which test failed | cause_type unknown with verdict cause |

## cause_type: answer the questions in order, take the first "yes"

| # | question | cause_type | needs |
|---|---|---|---|
| 0 | No source named? | `unknown` (+ `unknown_why`) | **[v]** |
| 1 | Does the source need a mod/DLC that is **not in the list**, unconditionally (declared dependency; its defs/code use that mod's defs/types without `MayRequire`/`FindMod`/a LoadFolders condition)? | `missing-dependency` | **[v]** a `missing` cause |
| 2 | Does the source's content target a mod/DLC that is **not in the list** and is meant as optional (a compatibility patch or patch folder named after it, a `MayRequire`d def, a reference to that mod's def) with a missing or broken guard? (If the target mod **is** in the list, go on: it is 5 or 6.) | `optional-absent` | |
| 3 | Everything involved is active, and moving one mod before/after another removes the error (target def defined by a mod later in the list; a declared loadAfter/loadBefore broken here)? | `load-order` | **[v]** a `partner` |
| 4 | Everything active, order not the issue, and two mods change the same thing in conflicting ways (one removes/renames/replaces what the other patches or calls; declared incompatibleWith)? | `incompatible-mods` | **[v]** a `partner` |
| 5 | `repro` is `no` **and** the player's copy of the source (or of the mod it targets, or the game build) differs from the library's: header version ≠ library version, or the header has no version and the library copy's Workshop update is after the log's upload date? | `version-mismatch` | **[v]** `repro` no; a `versions` entry for a cause or fix mod with `same` no, or `same` unknown and `updated` after the upload |
| 6 | The thing that is missing or wrong belongs to **the game or another mod** (a def, XPath node, XML field, class, type or method that was removed or renamed there)? | `outdated-mod` | **[v]** `repro` not no |
| 7 | The defect is **inside the source itself** (its own def name or path, a texture it names but does not ship, its own null check, malformed XML)? | `mod-bug` | **[v]** `repro` not no |
| 8 | none decidable | `unknown` + `unknown_why` `reason-unclear` or `not-in-library` | **[v]** |

`repro` `no` without a version difference (question 5 fails) is not outdated-mod or mod-bug: the
current copy no longer fails, so the answer is `unknown` with `reason-unclear`.

### Patch-failed checklist (most tier-1 disagreements)

1. Target: the XPath's `defName` (or the `PatchOperationFindMod` mod name) and the patch file's
   folder name (`Patches/<OtherMod>/…`, `ModPatches/<OtherMod>/…`).
2. Is the target mod in the list? No → question 2 (`optional-absent`; a `FindMod`-guarded
   patch that still fails is a broken guard). Yes → step 3.
3. `def <defName>`: defined by an active mod/DLC? Loaded after the patch's mod → question 3. The
   def exists but the node does not → question 5 or 6 (the target changed). The def exists nowhere
   → question 6 if the def name belongs to the game or another mod, else 7.
4. `repro` from the packet's simulation line ("does not fail now" = `no`).

## fix.kind: a rule, not a judgement (new)

**[v]** `fix.kind` must equal the first matching row:

| # | condition | fix.kind | fix.mods |
|---|---|---|---|
| 1 | cause_type missing-dependency | `enable-dependency` | the missing mod(s) |
| 2 | cause_type load-order | `reorder` | the mod to move (the note says before/after which) |
| 3 | cause_type version-mismatch | `update` | the mod to update |
| 4 | cause_type optional-absent | `ignore` | |
| 5 | tier-1 class (patch-failed, xref-missing, texture-missing, xml-load): the game drops the bad entry and goes on | `ignore` | |
| 6 | verdict unknown with `unknown_why` game-or-save or machine | `ignore` | |
| 7 | verdict unknown otherwise | `unknown` | |
| 8 | static-ctor or assembly class with a named source (the mod's setup or types fail to load) | `disable` | the source |
| 9 | exception or harmony-stack class with a named source | `report` | the source |

**[v]** `fix.mods` is required except for `ignore` and `unknown`.

## stage (unchanged)

`assembly-load` · `xml-load` · `patch-apply` · `def-resolve` · `static-init` · `game-start` ·
`save-load` · `map-gen` · `tick` · `ui` · `other`. Defaults: patch-failed → patch-apply;
xml-load → xml-load; "Could not resolve cross-reference" → def-resolve; "Failed to find X named"
→ the stage the stack shows (static-init when called from a static constructor); "Could not load
reference to" → save-load; texture-missing → static-init (graphics are built after defs), tick/ui
in play.

## difficulty (unchanged)

- `L1` fact lookup: the message or one fact names the responsible mod directly.
- `L2` cross-mod reasoning: needs two or more facts or mods.
- `L3` needs code. **[v]** a tier-2 `cause` label of difficulty L3 has at least one `code` quote.

## evidence, code, versions (unchanged, plus `updated`)

- `evidence`: 1-4 quotes copied character for character from the packet; `log:` (raw log, **[v]**
  against `raw/<gist>.txt`), `env:` (exec transcript, **[v]**), `corpus:` (not checked).
- `code` (tier 2): **[v]** the file exists, the text is in it at those lines (±3), the type's and
  method's simple names occur in it.
- `versions`: one entry per mod the conclusion depends on. **[v]** `log` equals the header
  version in the packet. `updated` (v3) is the library copy's Workshop date from the packet's
  "research library: …, Workshop updated <date>" line.

## Benchmark v3 files (outside the repository: `corpus-kb\labels\v3\`)

`sample-new.jsonl` (the 360 new items, PF21…, numbering continues after v2) · `sample.jsonl`
(v2's 140 + the new 360) · `rimarcho-v4.jsonl` (RimArcho's final-run verdicts; never shown to
labellers or agents) · `packets\` · `t1\`, `t2\` (labeller labels) · `review\` (reviewer
labels, the gold where present) · `overlap\` (the agreement study) · `v2-migrated\` (v2 gold in
corpuslabel/3) · `execlogs\` · `costs.jsonl` · `tasks.jsonl` + `code\` · `heldout.sha256` (the
frozen held-out id list).

v2 labels are scored as they were (`corpuslabel/2` stays valid); the v3 task set carries v2's
gold migrated to corpuslabel/3 (same sources and verdicts unless the reviewer corrected them;
`repro`, `unknown_why` added, fix kind recomputed).
