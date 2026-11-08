# 16 · Interface · Layers and shared view models (§1)

> Part of [16 · Interface](README.md). Section numbers run through all of 16 and stay unchanged when split across files.

## 1. Layers and shared view models

```
surface/tui   (Bubble Tea)        surface/gui   (Wails services + assets/*.html|css|js)
      │  Only layout, drawing, key/mouse mapping  │  Only serialize view models for the frontend and push events
      └──────────────┬──────────────────────┘
                internal/view     Interface-independent view models
                     │
                internal/app      Business logic
```

- The TUI uses Bubble Tea + Bubbles + Lip Gloss; the GUI uses Wails v3 + native HTML/CSS/JavaScript (§8). The GUI does not render the TUI into a window or run the CLI and parse its text.
- `internal/view` has no dependency on Bubble Tea, Lip Gloss, or Wails. Its input is `*app.App` state; its output is plain Go structs that serialize directly to JSON.

| File | Content | Main functions |
| --- | --- | --- |
| `text.go` | Language selection and lookup; language detection (`lang_windows.go`, `lang_other.go`); English interface overrides in `enText` | `T`, `Tf`, `Table`, `ExtraText`, `SetLang`, `Lang` |
| `columns.go` | Mods in each of the three columns; each row's number, source type, name (alias first), status, dimming, and unsaved state | `BuildBoard`, `ModItem`, `RowStatus`, `SourceKind`, `SourceText` |
| `query.go` | Search syntax parsing and matching, subsequence fallback, content type cache (§3.7) | `ParseQuery`, `Query.Filter`, `Subsequence`, `ContentCache.Get` |
| `needs.go`, `evidence.go` | Groups and sections for can it run (`Section`, `SectionOf`; items outside this page use `SectionElsewhere` / `Needs.Elsewhere`), links between columns (`Need.Related`), plain wording (`Text`: one sentence; GUI cards: `Title` / `Why`), evidence (`Evidence{Kind, Label, Source}`), merging rules, counts, row status; **one-step actions are data** (`Action{Kind, …}`), with the interface choosing the key or button; `Do` performs data mutations (§5) | `BuildNeeds`, `Do`, `Humanize`, `TokName`, `EdgeWords`, `ShortDate`, `WorkshopSearch`; page lead and automatic fix count (`needs_layout.go`: `NeedsLead`, `AutoFixCount`) |
| `live.go` | Game runtime status and errors from "this run" (§2.10) | `LiveTracker.Update`, `LiveNeeds`, `LiveText` |
| `detail.go` | Data and order for each detail layer: header facts, dependencies, description source selection, history, full-screen subsections (§4) | `LoadDetail`, `HeaderFacts`, `Facts`, `Deps`, `DescNote`, `HistoryLine`, `EventText`, `Sections`, `UnloadableWhy`, `WorkshopURL` |
| `changes.go` | Change history (§2.9): `internal/modchanges` builds the page (shared with CLI / MCP, given translated text); this supplies the interface language; one sentence after scanning | `BuildChanges`, `ChangeFiles`, `ChangeHome`, `ChangeSummary`, `LineageIDs` |
| `library.go` | Mod library › All mods: Lineage rows and their copies, size, status, used-in lists in the right column, filter groups and counts, effective conditions, each copy's actions (data), preview and execution of file-changing actions (§2.9); filtering itself belongs to `app.Library`, shared with `archo library` | `BuildLibrary`, `LibPreview`, `LibDo`, `LibStateText` |
| `history.go` | History area: each undo step and version (§2.5) | `HistoryItems`, `StepText` |
| `tags.go` | Tag candidates (§3.8) | `TagCandidates`, `PinnedTags` |
| `rules.go` | Editor sorting rules (§2.5): one sentence per rule (clickable Mod names), form kinds and candidates for the other Mod, add / edit / delete / restore | `BuildRules`, `RuleCandidates`, `SaveRule`, `RemoveRule`, `RestoreRule` |
| `filesearch*.go` | Mod library > search files (§2.9): bounded streaming Mod → file → lines and restricted file viewer, shared with TUI | `FileSearchRun`, `BuildFileSearchInfo`, `ReadSearchFile`, `SearchFolder` |
| `modsettings.go` | Mod library > Mod settings (§2.9): one row per settings file of the current list, and one file's stored values as text | `ModSettingsRows`, `ModSettingsFileDetail`, `SettingText` |
| `verbs.go` | Verbs: enable / disable, stage, put back, move, place at a position, position input candidates (§3.5, §3.6) | `Toggle`, `Stage`, `Unstage`, `StageAll`, `MoveBy`, `Place`, `PosOptions`, `ObjectOf`, `Positions`, `PlacedText`, `NowAt` |
| `preview.go` | Sort, save, launch: direct execution and read-only preview (§6); undo, redo, put all back, switch copies | `Run`, `Outcome`, `RefusalLines`, `DryRun`, `Apply`, `UndoRedo`, `RestoreAll`, `Swap` |
| `palette.go` | Command table (names in the interface language, English aliases, **command IDs rather than keys**), scoring and ranking (§3.5) | `Commands`, `CommandEntries`, `PalScore`, `PalRank` |
| `packs.go`, `packpage.go` | Pack list, each pack's page, create-a-pack page (four choices, summary, reasons for disabled controls), build progress, unfinished builds (§2.6, §2.7) | `PackRows`, `BuildPackPage`, `BuildPageOf`, `DefaultPackName`, `ProgressOf`, `UnfinishedRows` |
| `logpage.go`, `monitorlog.go` | Game log page and its companion log area (§2.10) | `LogFiles`, `BuildLogPage`, `MonitorOf`, `LiveMonitor`, `MonitorKind`, `MonitorAdd` |
| `settings.go` | Settings and environment pages (§2.8, §2.14) | `SettingsPage`, `BuildEnvPage`, `EnvGroups`, `SettingCommand` |
| `feedback.go` | Settings community links and feedback page (§2.14): attachments, body, links, preview, sending (`internal/feedback` assembles the body and links) | `CommunityLinks`, `FeedbackAttachments`, `ComposeFeedback`, `FeedbackPreview`, `SendFeedback` |
| `exchange.go` | List import and export (07 §7) | `ReadImport`, `PreviewImport`, `ApplyImport`, `ExportFormats`, `ExportText`, `ExportTo`, `DefaultExportDir` |

`internal/app` provides typed interfaces to the surfaces: `SetTags` / `SetAlias` / `SetNote`, identity question answers, log selection, `ChildScan` (scan subprocess without a console window), and per-Mod Def / Patch / Harmony summaries. Surfaces do not assemble CLI arguments for editing. `cmds.Execute` is used only for read-only cases that are text by nature: sort / write / launch previews (CLI `--dry-run` text), "show" actions and Def / patches / Harmony subsections of full-screen details, and archo commands the player enters in the command line input.

Rules:
1. Business rules belong only in `app` or `view`. A surface-level condition such as "if this is a Workshop Mod, then…" is a defect.
2. The same operation in both interfaces calls the same function. Given the same state, TUI and GUI receive identical `Needs`, `Board`, and detail data (G1).
3. Interface translations live in feature JSON files under `internal/l10ndata/locale/<language>/`, embedded with `go:embed`. Add a locale directory to bundle another language. `view` owns selection and lookup; GUI receives the selected base and supplemental tables.
4. Every disk-writing action meets three conditions: it has a preview (`--dry-run`, "preview…" in the interface command panel), can be undone or explains beforehand why it cannot, and reports its result in the status line (02 §1). Only actions beyond RimArcho's undo require confirmation first (§6).
