# Known parser issues

These cases are skipped by `TestCorpusRegression`. Their golden records contain
source metadata and the expected entry count, but no approved entry or cluster
snapshots. Updating the golden preserves skips. Fix the parser, review the
result, and remove the relevant skip before generating expectations.

## KI-1: generic XML Extensions patch errors

`32-xml-extensions.log` is one XML Extensions failure block. Its message becomes
`[Steamworld Defenses] Verse.PatchOperation: Error`, which is classified as
`mod-report`. It should be `patch-failed`, with the `Source file:` continuation
participating in the cluster key as it does for other patch failures.

## KI-2: generated static-constructor frame names

`30-mono-native.log` and `37-waylith-warn.log` contain generated closures inside
`.cctor`. `splitMember` splits at the dot inside `<.cctor>` and produces method
names `cctor>b__0_0` and `cctor>b__123_1`, respectively. These should normalize
to the enclosing static constructor, `.cctor`, on
`CombatExtended.Compatibility.PawnKindPatcher` and
`Waylith.Graphics.GraphicsDevice`. The malformed top frame affects cluster keys.
The Mono native line itself is correctly retained as continuation text.

## KI-3: Waylith callers with source locations

`38-waylith-info.log` contains two AutoTranslation caller frames ending in
`[0x...] in [Rimworld_dir]...:line`. Caller detection only accepts an address at
the end of the line. Those two frames become exception frames, causing an INFO
migration message to classify as `exception`. All remaining frames should be
caller frames and the entry should classify as `other`. INFO should still yield
no clusters.

## KI-4: generated Waylith logger frame

`39-waylith-error.log` contains the caller
`MonoMod.Utils.DynamicMethodDefinition.Verse.Log.Error_Patch3`. The parser keeps
it as a caller frame because logger filtering recognizes only direct Verse.Log
and Brutal.Logging types. This logging frame should be removed, as direct
`Verse.Log.Error` callers are. The real exception stack and its six Harmony
patch lines should remain. The cluster's class and exception top frame are
otherwise correct.
