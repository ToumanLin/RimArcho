# Labelling prompt — version corpuslabel-v3-t2 (tier 2: code-level research)

(v3, G12: schema corpuslabel/3 — new fields `repro` and `unknown_why`; cause type by ordered
questions; fix kind is a fixed rule. Method unchanged from corpuslabel-v2-t2.)

You find the real cause of one error cluster from a RimWorld 1.6 Player.log (shared through
HugsLib): exceptions, Harmony stacks, assembly / TypeLoad errors and static constructor
errors. Stack traces name suspects, not causes; you decide by reading the mods' decompiled
code. You never see any automatic verdict on this cluster.

## What you have

1. **The evidence packet** `<PACKETS>/<ITEM>.txt`: the message, extra lines and stack, log lines
   around it, the mods the evidence points at (with the player's and the library's versions),
   facts from a research copy of the mods (current Workshop versions; textures stripped).
2. **The raw log** `<CORPUS>/raw/<gist>.txt` (header with the mod list and versions, then the
   Player.log). Search it with Grep; do not read it whole (it is large).
3. **The research environment**, loaded with this log's mod list. Run archo commands with
   exactly this command line (one command per call; the first call may take a minute):

       <EXE> exec -home <HOME> -corpus <CORPUS> -packet <PACKETS>/<ITEM>.txt -log <EXECLOGS>/<ITEM>.log -- <archo command>

   Refer to a mod as `pkg:<packageId>` (e.g. `source pkg:oskarpotocki.vanillafactionsexpanded.core`);
   outputs show mods as short refs like `#ux5i`, which also work as arguments.
   Useful commands:
   - `source pkg:<packageId>` decompiles a mod's assemblies (ilspycmd) and prints the folders of
     the .cs files; then use Grep/Read on those folders directly (faster than more exec calls).
   - `source pkg:<packageId> --type Ns.Type --all` prints one type with line numbers.
   - `source search "<text>" --in pkg:<packageId>` (add `--regex` for a regex) searches decompiled code.
   - `harmony --target Ns.Type.Method` who patches a game method; `harmony --mod pkg:<packageId>`.
   - `def <Type/defName>`, `patches --mod pkg:<packageId> --failed`, `files search "<text>"`,
     `mod pkg:<packageId>` (About.xml, dependencies), `why <A> <B>` (how two mods relate).
   Only fact commands are allowed (`log` and `problems` are blocked).
4. Game code: the decompiled RimWorld assemblies are not in the environment; reason about
   vanilla methods from their names and the stack, or decompile `D:/RimWorld16/RimWorldWin64_Data/Managed/Assembly-CSharp.dll`
   with `ilspycmd -t <Type> <dll>` if you must see a vanilla method (say so in the note).

Do **not** open `<PACKETS>/*.json`, `rimarcho-v4.jsonl`, `results*.jsonl`, other items' files or
labels: they contain automatic verdicts.

## Method

1. Read the packet. Identify the innermost frames that belong to mods (types not in `Verse`,
   `RimWorld`, `UnityEngine`, `System`), Harmony patch lines (`[Harmony … - PREFIX x]`) on the
   frames, and the exception type.
2. Decompile the suspect mod(s) and read the method that throws or the patch that runs there.
   Find the line that fails and why: which value is null, which type or member is missing,
   which assembly it expects.
3. Missing type / member / assembly (TypeLoad, MissingMethod, ReflectionTypeLoad, "Could not
   resolve type … in assembly X"): find which mod owns assembly X (`source search`, `mod`,
   the header's assembly list) and whether that mod is in the list, and whether the member
   exists in the library copy.
4. Static constructor errors: the outer exception names the type whose static constructor
   failed; the inner exception (`--->`) is the real error. Read that static constructor.
5. **Version check before concluding.** For each mod your conclusion rests on, compare the
   version in the log header (`[mv:]`/`[v:]`/`[ov:]` after its packageId, or the packet's
   "position …/… v…" line) with the library copy (packet: "research library: …", or `mod`).
   If they differ and the library code does not have the defect (e.g. the null check exists,
   the member exists), the answer is `version-mismatch` with fix `update`. If the versions
   differ and you cannot tell, lower the confidence and say so in the note.
6. It is fine to answer `unknown` (undecidable): e.g. an NRE inside vanilla code with several
   patchers and nothing showing which input is null. Do not guess.

## repro (only with a named source)

Does the library copy (current version, same list) still have the defect? `yes` = the failing
line, the missing member or the missing guard is still in the library code; `no` = the library
code no longer has it (null check added, member exists, line changed); `unknown` = the code is not
in the library or does not decide.

## unknown_why (whenever verdict or cause type is unknown)

With verdict `unknown`: `game-or-save` (fails in vanilla code, or on save data / references of
mods not in the list; no active mod's own code fails) · `machine` (the player's machine or
install: a file that cannot be read or written, a missing Unity module such as
UnityEngine.InputLegacyModule, out of memory) · `several-candidates` (two or more mods fit, the
code does not pick one) · `no-evidence` · `not-in-library` (the deciding mod's code is not in the
library). With a named source and cause type `unknown`: `reason-unclear` (say in the note which
question below you could not answer) or `not-in-library`.

## cause_type: answer in order, take the first "yes"

1. `missing-dependency` — the source needs a mod that is not in the list (declared dependency, or
   its code references that mod's assembly/types without a guard). Add it with role `missing`.
2. `optional-absent` — the source's code targets an optional mod/DLC that is not active and the
   guard is missing or broken.
3. `load-order` — all active; moving one mod before/after another fixes it. Add `partner`.
4. `incompatible-mods` — all active, order is not the issue, two mods conflict (one patches,
   replaces or removes what the other relies on). Add the other as `partner`.
5. `version-mismatch` — `repro` is `no` AND the player's copy differs: header version ≠ library
   version (`versions` `same: "no"`), or the header has no version and the packet's "Workshop
   updated <date>" is after the log's "uploaded:" date (`same: "unknown"`, `updated: "<date>"`).
6. `outdated-mod` — the source calls or references a type/member/field of the game or ANOTHER mod
   that no longer exists there (MissingMethod, TypeLoad of another assembly's type), and the
   library copy still does (`repro` yes/unknown).
7. `mod-bug` — the defect is in the source's own code with everything it needs present and
   current (a missing null check, a wrong assumption), and the library copy still has it.
8. none decidable → `unknown` with `unknown_why`.

`repro: "no"` without a version difference is never outdated-mod or mod-bug (→ `unknown`,
`reason-unclear`).

## fix.kind: a fixed rule (do not judge impact)

First row that applies: missing-dependency → `enable-dependency` (mods = the missing mod) ·
load-order → `reorder` (mods = the mod to move) · version-mismatch → `update` (mods = the mod to
update) · optional-absent → `ignore` · verdict `unknown` with `unknown_why` game-or-save or
machine → `ignore` · other verdict `unknown` → `unknown` · static-ctor or assembly class (the
packet's class line / item prefix SC, AS) with a named source → `disable` · exception or
harmony-stack class (EX, HS) with a named source → `report`. `fix.mods` = the source unless the
row says otherwise; empty for ignore/unknown.

## stage

`assembly-load` (loading DLLs, listing types, constructing `Mod` classes) · `xml-load` ·
`patch-apply` · `def-resolve` · `static-init` (`[StaticConstructorOnStartup]`) · `game-start`
(finishing load, main menu) · `save-load` · `map-gen` · `tick` · `ui` (OnGUI, windows) · `other`.

## difficulty

`L1` the packet alone names the mod and the reason (e.g. the mod's own type throws a clear
error) · `L2` needs combining facts across mods, no code · `L3` needs reading code.

## Output

Write the label as one JSON object to `<LABELS>/<ITEM>.json` (Write tool), then reply with
one line: the verdict, the source mods and the cause type. Schema:

```json
{
  "schema": "corpuslabel/3",
  "item": "<ITEM>", "gist": "<log id>", "key": "<the packet's key: line, without 'key: '>",
  "verdict": "cause | unknown",
  "causes": [{"pkg": "<packageId>", "role": "source | partner | missing"}],
  "stage": "...", "cause_type": "...",
  "repro": "yes | no | unknown",
  "unknown_why": "game-or-save | machine | several-candidates | no-evidence | not-in-library | reason-unclear",
  "fix": {"kind": "...", "mods": ["<packageId>"], "note": "<one sentence>"},
  "confidence": "high | medium | low",
  "difficulty": "L1 | L2 | L3",
  "evidence": ["<verbatim packet quote>", "log: <verbatim raw-log line>", "env: <verbatim exec output line>"],
  "code": [{"file": "<absolute path of the decompiled .cs file>", "type": "Ns.Type", "method": "Method",
            "lines": "<first>-<last>", "text": "<the decisive lines, verbatim>"}],
  "versions": [{"pkg": "<packageId>", "log": "<version in the log header, or \"\">", "installed": "<library modVersion, or \"\">", "same": "yes | no | unknown", "updated": "<library Workshop date from the packet, or \"\">"}],
  "model": "<your model name>", "prompt": "corpuslabel-v3-t2",
  "note": "<2-4 sentences: what fails, why, and how the code shows it>"
}
```

Checked by the validator (run it; fix your label until it passes):

    <EXE> validate -sample <V2>/sample.jsonl -packets <PACKETS> -labels <LABELS> -corpus <CORPUS> -execlogs <EXECLOGS>

(it checks every label in the folder; only lines for `<ITEM>` concern you).
- `code`: quote the decisive code — class, method and 1-based line numbers of the file as it is
  on disk; `text` must be copied from those lines (whitespace may differ). Required for an L3
  `cause`. 1–3 quotes.
- `evidence`: 1–4 quotes; unprefixed ones verbatim from the packet; `log:` ones verbatim from the
  raw log; `env:` ones verbatim from your exec transcript.
- `versions`: one entry per mod your conclusion rests on; `log` must equal the header version.
- packageIds of `source`/`partner` must be in the log's list; Ludeon packages are never a source.
- `repro` only with verdict `cause`; `unknown_why` only when verdict or cause type is `unknown`
  (leave the fields out otherwise); `fix.kind` must equal the rule above.
- `causes` empty unless `cause`; `fix.mods` required except for `ignore`/`unknown`;
  `missing-dependency` needs a `missing` cause; `load-order`/`incompatible-mods` need a `partner`.
