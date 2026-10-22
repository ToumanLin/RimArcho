# Player.log regression corpus

Trailing line whitespace is removed. Each `.log` contains one complete entry, except `31-duplicate-stack.log`, which
contains an original and its duplicate. Excerpts come from successful corpus
records for RimWorld 1.6 and 1.7. No upload headers or source identifiers are
included. The Waylith format marker is supplied by the test in memory so its
fixtures contain only entry lines.

`golden.json` owns the per-file source version, coverage, entry count, entry
shape, and ordered cluster class/template/key/count expectations. `frequency`
counts cluster records with that template across successful 1.6 corpus records;
it does not count entry occurrences or distinct logs. The set includes the
three leading templates in each of the ten classes, plus format examples and
four Waylith entries. Cases with incorrect parser output are documented in
[KNOWN_ISSUES.md](KNOWN_ISSUES.md) and skipped without approving that output.

Run from the module root:

```powershell
go test ./internal/logs
go vet ./internal/logs
$env:RIMARCHO_UPDATE_LOG_CORPUS = '1'
go test ./internal/logs -run '^TestCorpusRegression$' -count=1
Remove-Item Env:RIMARCHO_UPDATE_LOG_CORPUS
```

The update command refreshes active snapshots and preserves skipped cases.
Add source-version and coverage metadata to the golden before adding a fixture;
the test rejects missing fixture metadata and golden records without files.
Review every regenerated expectation against the excerpt before committing.

Personal local source paths were replaced with `C:\NeutralSource\Source.cs`,
retaining source line numbers and stack symbols. Steam installation prefixes
were replaced with `C:\Steam`, retaining the game/workshop directory layout.
Existing HugsLib tokens such as `[Rimworld_dir]` are preserved. The selected
entries contain no user names, Windows user folders, save names, colony names,
or pawn names. Mod names, packageIds, def names, types, frames, reference tokens,
and assembly hashes are retained. Game object load IDs are retained as parser
inputs; they are not pawn display names.
