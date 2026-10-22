# Labelling prompt — version corpuslabel-v2-t2 (tier 2: code-level research)

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

## cause_type: take the FIRST rule that applies

1. `unknown` — verdict `unknown`, or a source is named but none of the rules below can be decided.
2. `missing-dependency` — the source needs a mod that is not in the list (declared dependency,
   or its code references that mod's assembly/types without a guard). Add it with role `missing`.
3. `optional-absent` — the source's code targets an optional mod/DLC that is not active and
   the guard is missing or broken.
4. `load-order` — all active; moving one mod before/after another fixes it. Add `partner`.
5. `incompatible-mods` — all active, order is not the issue, two mods conflict (one patches,
   replaces or removes what the other relies on). Add the other as `partner`.
6. `version-mismatch` — the player's copy differs from the library's and the library copy no
   longer has the defect (`versions` entry with `"same": "no"`).
7. `outdated-mod` — the source calls or references a type/member/field that no longer exists
   in the current game or in the mod it builds on, and the current copy still does.
8. `mod-bug` — the defect is in the source's own code with everything it needs present and
   current (a missing null check, wrong assumption).

## fix.kind (must fit the cause type)

missing-dependency → `enable-dependency` (mods = the missing mod) · optional-absent → `ignore`
or `enable-dependency` · load-order → `reorder` (mods = the mod to move; note says where) ·
incompatible-mods → `disable` / `report` / `ignore` · version-mismatch → `update` ·
outdated-mod and mod-bug → `disable` / `report` / `ignore` · unknown → `unknown` / `ignore` (also `report` / `disable` when a source is named).
By impact: `ignore` = log noise or a cosmetic glitch the game recovers from; `report` = one
feature of the mod breaks; `disable` = loading, saving or a core loop breaks (exceptions every
tick, a failed static constructor or Mod constructor aborting the mod's setup).

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
  "schema": "corpuslabel/2",
  "item": "<ITEM>", "gist": "<log id>", "key": "<the packet's key: line, without 'key: '>",
  "verdict": "cause | unknown",
  "causes": [{"pkg": "<packageId>", "role": "source | partner | missing"}],
  "stage": "...", "cause_type": "...",
  "fix": {"kind": "...", "mods": ["<packageId>"], "note": "<one sentence>"},
  "confidence": "high | medium | low",
  "difficulty": "L1 | L2 | L3",
  "evidence": ["<verbatim packet quote>", "log: <verbatim raw-log line>", "env: <verbatim exec output line>"],
  "code": [{"file": "<absolute path of the decompiled .cs file>", "type": "Ns.Type", "method": "Method",
            "lines": "<first>-<last>", "text": "<the decisive lines, verbatim>"}],
  "versions": [{"pkg": "<packageId>", "log": "<version in the log header, or \"\">", "installed": "<library modVersion, or \"\">", "same": "yes | no | unknown"}],
  "model": "<your model name>", "prompt": "corpuslabel-v2-t2",
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
- `causes` empty unless `cause`; `fix.mods` required except for `ignore`/`unknown`;
  `missing-dependency` needs a `missing` cause; `load-order`/`incompatible-mods` need a `partner`.
