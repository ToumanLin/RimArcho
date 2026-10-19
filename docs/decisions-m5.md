# M5 decisions and measurements

Commits 8210427..HEAD after 784261b, local only, not pushed. Scope:
- the M4 rulings (decisions-m4 §7.1–7.4)
- `copy-local` (U10) and player data with `import rimcrow` (U11)
- `loadout export/import`
- the TUI Changes view and mouse support
- packaging, README and CHANGELOG
- the final MVP acceptance pass (`docs/mvp-acceptance.md`)

The old inspector and the two skills are untouched.

## 1. M4 rulings

| Ruling | Implementation |
|---|---|
| 1 skip-prefix | `harmony.skip-prefix` is info (analyzer version 4). `Problems()` raises it to warning, adding the cluster as evidence and `archo log --cluster` as the first suggestion, when the method (overload stripped) is a frame of an error cluster in the problem log. Only info rows fold. On the real log **0 escalate**: none of the 99 bool-prefix targets appears anywhere in that Player.log (checked with grep), so 134 warnings became 134 info |
| 2 newest log | `ProblemLog` = the selected log, or the current Player.log when that is newer. CLI and MCP always include it. The TUI sets `App.LazyLog` until the Problems view opens, so the status-bar counts at start do not parse the log. Opening the view marks the log as seen and clears the "new log" badge |
| 3 log's own version | `logState` attributes against the log's major.minor when it differs and no `--game-version` was given. The header reads `analysed: as 1.6 (log version); environment is 1.7`. `log` and `problems` first bring the analysis for that version up to date (`EnsureLogAnalysis`) |
| 4 source search scope | `source search` needs `--in`, `--implicated` or `--active`. `--active` prints the estimate (assemblies, DLL MB, cache ≈ 3 × DLL size, ≈ 1.5 s per assembly) and needs `--yes`. Real: 424 assemblies, 407 uncached, 27 MB of DLLs, about 82 MB of cache, about 11 min. MCP `read_source` defaults to `--implicated` (the candidates of the newest log's error clusters, else the subjects of Harmony warnings); `scope: "active"` opts in. Log next-steps search the cluster's own candidates (`--in …`) |

## 2. Judgment calls

1. **Analyses are kept per game version (schema v8).** Found while measuring. The store kept one analysis per
   environment, so after ruling 3 the 1.7 analysis and the 1.6 log analysis evicted each other, and every
   `problems` / `list_problems` re-ran both (9 s per call). Analysis runs and all per-instance facts are now
   partitioned by `game_mm`; `Store.MM` follows the Loadout's game version. The migration drops the cached
   facts, which the next analysis rebuilds. Regression test: `TestAnalysisPerGameVersion`.
2. **origin.json field names** follow 04 §6 (`origin_instance`, `origin_lineage`, `workshop_id`, `origin_snapshot`,
   `copied_at`). The M1 reader used `instance_id`…, but nothing had ever written the file, so the tags were
   simply changed.
3. **copy-local**:
   - Links that resolve inside the mod folder are copied as content. Links to outside, and loops, are skipped
     and listed in the output.
   - The upstream `.git` and an existing `.rimarcho/` are not copied.
   - The copy is assembled in `<name>.archo-tmp` and renamed into place.
   - The git baseline uses `add --all --force`, so ignored files are included (inspector behaviour), runs
     with a fixed RimArcho identity and `--no-verify`, and does not sign.
   - A rescan follows, so the output can print the copy's ref and `archo swap <orig> <copy>`. The Loadout is
     never changed.
4. **Player data lives on lineages** (schema v7: `lineage_data`, `lineage_tag`).
   - The alias is the display name everywhere, and `find` matches it.
   - Log attribution still matches the About.xml name: the game prints `[Name]`, not the alias.
   - Added the `alias`, `note` and `tag` commands and `list --filter tag:`.
5. **import rimcrow**:
   - `--db` is required. RimCrow keeps its DB in `<install>/data/mod_manager.db`, and there is no registry
     or settings key to find it.
   - The DB and its WAL are copied to a temp dir under the RimArcho home and opened read-only from there. The
     copy is removed afterwards.
   - User data is keyed by packageId. A record maps by RimCrow's asset path(s) first and by packageId second.
     A packageId with several lineages is reported and not imported.
   - Aliases and notes never overwrite a different RimArcho value; such cases are listed as conflicts.
   - Groups and tags both become tags. Interlocks become `attach` rules. `user_rules.json`
     loadAfter/loadBefore/incompatibleWith become rules.
   - loadTop/loadBottom, sign_color, user_mod_type and ignored_issues have no equivalent; they are counted.
   - **User rules switched off in RimCrow are not imported** unless `--include-disabled-rules` is given. The
     user's 5 rules are off in RimCrow, so importing them silently would change sort results.
   - Idempotent through set semantics (same value = no-op; rules matched by kind + lineages). No import log
     table is needed.
6. **about-disabled guidance**:
   - The import report counts the folders that have only `About.xml.disabled`, by source: real 64 Workshop
     and 4 local.
   - The guidance: rename the file back or re-enable it in RimCrow, then `archo scan`; or unsubscribe or
     delete. RimArcho does not rename files (02 §4.1).
7. **loadout import**:
   - It reads a text list (`#` comments), a ModsConfig.xml, or a save's `<meta><modIds>`. For saves it stops
     reading after `</meta>`, and the mod names from the save label missing entries.
   - It replaces the workspace in one undoable step and never writes the game file.
   - `export` never overwrites `--out`.
8. **Changes view**:
   - It shows the last 10 scans. Answering a question or loading a version shows the command first (y to
     run).
   - `I` dismisses the external version: it clears `workspace.conflict`, and the version stays in `loadout
     log`.
9. **Mouse**:
   - The wheel scrolls and a left click selects; a second click on a change expands it. Panels and prompts
     stay keyboard-only.
   - `config set ui.mouse off` turns cell-motion mouse reporting off, for terminals where it interferes with
     text selection.
10. **files search parallel**: it walks per mod and reads files with 8 workers, keeping the walk order. On
    482 mods (10k files) it now takes 0.37 s, down from 1.1 s, which brings MCP `search_files` under 500 ms.
11. **Version info**:
    - `internal/buildinfo` is set through `-ldflags -X` by `scripts/release.ps1`, which runs the tests, builds
      with CGO off and `-trimpath`, and zips with README and CHANGELOG; it never publishes.
    - There is no Windows version resource (it would need a resource compiler step).
    - `archo <command> --help` was added because the README promises it.

## 3. Incidents

- **App state touched in the real data directory.** A TUI memory run that I stopped early (to fix the lazy-log
  regression) never reached its restore step. It left `log.selected` / `log.seen` pointing at the real
  Player.log in `%APPDATA%\RimArcho\archo.db`. These are RimArcho's own settings, not game files. They were
  reset to empty afterwards. The game, the Workshop, `Mods/`, ModsConfig.xml and the RimCrow DB were not touched;
  md5s in §5.
- **The TUI parsed the log at start.** The status-bar counts load problems at start, and with ruling 2 that
  parsed the 8.7 MB log (list-shown working set 29 → 49 MB). Fixed with `App.LazyLog` before the final
  measurement.

## 4. Acceptance

See `docs/mvp-acceptance.md`: U1–U14 pass, as do A1–A11, B1–B5, C1–C6, S1–S5, I1–I5 (I4 as a scripted
self-check), T1–T3 and T5, and every 01 §5.2 metric. Exceptions: T4 needs a usability test with a person,
and the first log call after a new Player.log takes 568 ms once (§7; the first call of a session was 553 ms
before §7).

New tests in M5:
- `TestCopyLocal`, `TestImportRimCrow`, `TestLoadoutExchange`
- `TestSkipPrefixEscalation`, `TestLogOwnVersion`, `TestSourceSearchScope`, `TestAnalysisPerGameVersion`
- `tui.TestChangesViewAndMouse`
- MCP `copy_local` annotations

## 5. Measurements and checks

See `docs/mvp-acceptance.md` §2 for the table.

Real-environment commands run in M5 (read-only on game data; writes went only to `%APPDATA%\RimArcho` or a
temp home):
- `problems`, `log`, `source search --active` (estimate only)
- `import rimcrow --dry-run`
- `env detect`, `env add`, `scan`, `analyze` and `files search` against a fresh temp home
- `spikes/tuimem`, `spikes/tuistart`, `spikes/mcplat`

## 6. Open questions for the user

1. **RimCrow user rules**: your 5 user rules are switched off in RimCrow (`user_mod_rules_enabled: false`).
   Should `import rimcrow` bring them in anyway (`--include-disabled-rules`), or leave them out?
2. **The 64 about-disabled Workshop mods**: RimCrow disabled them by renaming `About.xml`. RimArcho lists them
   but will not rename files in Workshop folders. Do you want to re-enable them (in RimCrow, or by renaming),
   unsubscribe from them, or leave them as they are?
3. **Running the real import**: the dry run maps 1105 of 1160 records and would add 1128 tags from your 125
   groups. Run `archo import rimcrow --db "D:\Game\RimModManager\data\mod_manager.db"` for real?
4. **Your ModsConfig.xml is a 1.6 list for a 1.7 game.** The game resets it on start. Saving once from
   RimArcho (`archo save`) rewrites it for 1.7. That is your decision, because it writes the game file.
5. **Retiring the old tools**: the old inspector (`tools/ai_mod_decompile`) and the two `rimworld-mod-*`
   skills are still in place. Remove them once you have tried `archo mcp` with Claude Code or Codex?
6. **T4 usability test**: someone who has not used RimArcho should try enable → sort → save → launch in the
   TUI without the README, to close T4.

## 7. Post-review polish (stale logs, cached log attribution)

Found in the final review: on the real environment `archo problems` showed 27 errors, and 26 came from the
1.6 Player.log written before the 1.7 update. The first MCP call of a session took 553 ms.

1. **Stale logs** (03 §6.3, 06 §4).
   - Log problems are grouped under one row per log: file, game version, time, current or stale.
   - A log is stale when another game version (major.minor) wrote it, or when it is older than the
     Loadout's last change. That change is the last unsaved edit if there is one, else the ModsConfig.xml
     mtime. The file time is used rather than the version's `created_at`, because an imported or external
     version is recorded when RimArcho notices it, which can be after the game session that wrote the log.
   - On the real environment the log is stale only because of the version. ModsConfig.xml (19:35) is older
     than the log (19:41), so that session did run with this list.
   - Stale problems live in `ProblemSet.Stale`. They are not counted in `problems`, `status` or the TUI
     status bar, and they do not escalate `harmony.skip-prefix`. They are folded by default and listed with
     `--code`, `--subject` or `--all`; `problems show` still finds them.
   - Real result: 1 error and 46 warnings (was 27 and 132). The log row reads `26 error, 86 warning, not
     counted`.
2. **Cached log attribution** (schema v9, `log_cache`).
   - Measured on the real log: parsing and clustering take about 60 ms. The cross-reference referrer
     search during attribution takes about 455 ms. So caching only the parsed clusters would not have met
     the target, and the whole attributed report is stored instead.
   - The key is path, size and mtime, plus a fingerprint of the attribution inputs: game version, analysis
     run, enabled list, and each installed mod's ID, snapshot, names and folder. One row per log path.
   - `status` and `mod` (MCP `status`, `get_mod`) use the stored report only (`App.LogCacheOnly`) and never
     parse. When nothing is stored, `status` says the log is not analysed yet.
   - The TUI also uses the stored report for its status bar until the Problems view opens.
   - First MCP call of a session: 49–57 ms. The first `list_problems` or `analyze_log` after a new log:
     568 ms, once.
3. **Incident.** Checking the real Problems view through a test harness set `log.selected` / `log.seen`
   in `%APPDATA%\RimArcho\archo.db` again; this is the same mechanism as §3. Both were cleared afterwards.
   For the cold-cache measurement, the `log_cache` row was deleted from that database. The game, Workshop,
   `Mods/`, ModsConfig.xml (md5 `680ab615…` unchanged) and the RimCrow DB were not touched.

New tests: `TestStaleLog`, `TestLogCache`, `tui.TestProblemsViewLogs`; the golden `harmonylog` gains the
current → stale sequence.
