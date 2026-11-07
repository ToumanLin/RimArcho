# Error pattern library v0: draft MCP integration

> Status: integrated into the product (G11, `internal/patterns`, `archo pattern`, `analyze_log pattern=`, and the MCP `Instructions` checklist; see 06 §5). The following is the pre-integration draft, retained as a record. Basis: `docs/research/00-log-corpus-design.md` §3 G5, §9; data: `spikes/patterns/patterns-v0.json`; figures: `spikes/patterns/RESULT.txt`.

## 1. What the data tells us

6,734 logs for 1.6, with 610,910 problem clusters:

- **By phase**: loading (XML, patches, cross-references, assemblies, assets) 70%, save loading 21%, runtime 9%. The existing diagnostic workflow covers almost only loading-phase XML and runtime Assembly errors.
- **Existing workflow coverage** (by cluster): full 30%, partial 29%, **no coverage 41%**. The main gaps are save-loading errors (`save.*`, 1,704 logs have `loadID` errors), missing assets (`asset.texture-missing`, 1,821 logs), missing assembly dependencies (`asm.dependency-missing`, 774 logs), and configuration checks (`xml.config-error`, 2,004 logs).
- **The reliability of fix evidence depends on the phase**: between two uploads by the same player with an unchanged list, loading-phase errors almost never disappear on their own (0–3%), but save-loading and runtime errors do (5–23%, because the player did something else the second time). Thus, "the error disappeared after a list change" is meaningful evidence for loading-phase patterns; runtime patterns need comparison with a control.

## 2. Diagnostic workflow in MCP `Instructions` (proposed revision)

Reorder step 3, "Classify the failure", by the actual distribution, covering common cases first:

1. **Cross-references / missing Defs** (`xref.def`, `xref.sound`, `xref.getnamed`, 3,700 logs): check dependency candidates first; a missing `SoundDef` only affects sound and can be left alone.
2. **Patch failures** (`patch.op-failed`, `patch.xpath`, `patch.findmod`): keep the existing `get_patches failed=true` route.
3. **Missing XML fields or types** (`xml.no-field`, `xml.type-missing`): the Mod version does not match the game or framework Mod; first check that the framework Mod is enabled and loads earlier.
4. **Assembly version mismatch** (`asm.version-mismatch`, `asm.harmony-target`, `asm.dependency-missing`, `asm.static-ctor`, about 3,000 logs in total): add a route — use `get_mod` to inspect assemblies and dependencies; confirm dependencies are enabled, ordered correctly, and that two versions of the same library are not present. The fix is to update or disable, rather than edit XML.
5. **Save-loading errors** (`save.*`, `rt.post-load`): new. Explain that these usually come from adding or removing Mods during a playthrough. The Mod owning the type in `curParent` tells us whose data remains in the save; it does not mean that Mod has a bug. Fixes: ignore, re-enable the original Mod, or start a new save. Do not disable a Mod on this evidence.
6. **Missing assets** (`asset.texture-missing`, `asset.maintex`, `asset.texture-create`): new. The texture comes from a disabled Mod or DLC, or the file path has a case mismatch.
7. **Runtime exceptions** (`rt.tick`, `rt.ui`, `rt.ai-job`, `rt.collection-modified`, `rt.nre`): keep the Assembly route and add the connection between `Collection was modified` and the `AllComps` check.
8. **Noise** (`noise.*`): HugsLib upload failures, Analyzer status lines, and Harmony IL dump fragments; explain directly that these are not problems.

## 3. The `analyze_log` checklist

When an error cluster matches a pattern, append:

```
E12  xml-load  Could not find type named VFECore.ThingDefExtension …
     pattern g:xml.type-missing (community, 2,248 logs): XML names a C# class that no loaded assembly has
       causes: the framework mod that defines the class is not active or loads later; renamed in an update
       next:   analyze_log cluster=E12 → search_files VFECore.ThingDefExtension → list_problems code=dep
       evidence: in 156 repeat uploads with 1-3 list changes the error went away 16% (no change: 6%)
```

- Source type `community`, confidence no higher than `possible`; still follow the rule that an occurrence location is not a cause.
- `next` comes from the pattern's `route`, merged with the existing `Next` / `Checklist`, with the pattern first.
- Fallback patterns (`rt.nre`, `rt.other`, `xml.other`, etc.) do not show `pattern:`; use only the existing checklist.
- Show Mod-specific patterns (`m:<packageId>:<hash>`) only when the source Mod agrees with attribution: "This error from this Mod appeared in N logs (first / most recent month)", with "May have been fixed by a Mod update". Most Mod versions are missing in v0 (see known issues in RESULT), so use the first / most recent month for now.

## 4. New reference `pattern:<id>`

`get_pattern ref=pattern:g:xml.type-missing` returns matching rules, diagnostic route, common causes, fix kinds, and `resolution` counts ("disappeared / total" grouped by list changes, plus the unchanged control). A new tool is optional: this can be part of the `analyze_log cluster=E<n>` output.

## 5. Distribution

As with community sorting rules: `archo data update` downloads `patterns-v<n>.json` and records the dataset version; the product does not collect data online. v0 uses regexes over templates for matching. Product integration should instead match the templates and categories in `internal/logs` directly, keeping research and product consistent.

## 6. Gaps to fill before integration

- `parsed.jsonl` omits Mod entries with `[v:…]` / `[ov:…]` (about 5%, but present in every log; HugsLib itself uses `[ov:]`). Fix this and regenerate the library before using Mod versions and "disappeared after an update" evidence.
- Fallback patterns such as bare `NullReferenceException` need `StackShape` (innermost game method, and assembly owning the innermost Mod frame); v0 only has message templates.
- Compare agent conclusions before and after integration using the evaluation dataset (`00-log-corpus-design.md` §6.5), then decide the final `Instructions` wording.
