# Operation log: design draft

> **Status (2026-11-21):** draft, not decided, not implemented. Nothing here is a specification yet; once accepted, the behaviour moves to 02 (file writes), 04 §4 (storage), 13 (command layer), 14/15 (MCP and Archo) and 16 §2.14 (feedback), and this note keeps only the reasoning.
> Basis: [02](../specs/02-file-writes-and-uninstall.md), [04 §4](../specs/04-scanning-and-identity.md), [07 §3](../specs/07-plans.md), [13 §1](../specs/13-commands-and-cli.md), [15](../specs/15-archo.md), `internal/feedback`, `internal/view/feedback.go`.

## 1. Problem

When a player reports "my Mod disappeared" or "my list changed", the maintainer cannot tell who did what, when. RimArcho keeps seven kinds of record, and none answers that question:

| Record | Where | What it can answer | What it cannot |
| --- | --- | --- | --- |
| Undo stack (`workspace_undo`) | `archo.db` | List edits of a plan, last 100 steps | Anything that is not a list edit; older steps |
| Version history (`loadout_version`) | `archo.db` | What was written to `ModsConfig.xml`, when, from which source | Everything else |
| Scan changes (`change_event`) | `archo.db` | What *changed on disk* between scans | Whether RimArcho or something else did it |
| Launches (`launch`) | `archo.db` | Each launch | Anything but launches |
| Background tasks (`tasks.json`) | data directory | Task state and result | Only background tasks; rolls over |
| `logs/gui.log` | data directory | One line per window start (timing) | It is not a log |
| `log.Printf` calls | stderr | Network fallbacks and a few warnings | Lost when the GUI runs |

Gaps that matter, in order of how often a report would need them:

1. File operations: recycle bin, unsubscribe, link create and remove, move to the lock library, Mod settings write, reset and copy, save rename, copy, backup and recycle, pack build and remove, uninstall, self-update.
2. Who did it. A CLI, MCP or Archo (agent) call has no trace a player can see. 15 says "No separate AI action log. Undo history is the record of changes", which holds only for list edits.
3. What was refused or failed. "I tried to delete it and nothing happened" leaves no evidence.

## 2. Goals and non-goals

Goals:

- One append-only record of **every action that changes the player's files or RimArcho's own player-visible state**, from every surface, including failures and refusals.
- Cheap: a few rows per player action, no measurable cost on page loads (16 §10, 18).
- Shown to the player (what has RimArcho done?), and attachable to feedback.

Non-goals:

- Not a replacement for undo (07 §3) or version history. Those stay as they are; the log refers to them (`loadout_version` number) instead of copying lists.
- Not a diagnostic or debug log. No reads, scans, page views, search, analysis, cache or timing.
- No telemetry. The log never leaves the machine except when the player attaches it to a report (§7).
- Dry runs are not logged: a preview writes nothing (02 §1).

## 3. What is recorded

Record an action when it passes through one of the primitives below. The table is the first-pass inventory, grouped by how the code is reached; the list follows the write list in 02 §1.

| `op` code | Primitive (function) | Subject |
| --- | --- | --- |
| `trash` | `App.ToTrash` (`internal/app/trash.go`), the single recycle-bin entry (02 §3). Covers Mod folders, packs, saves, Mod settings files, uninstall deletions | the path, plus Mod id or save name when known |
| `link.create`, `link.remove` | `internal/app/linklifecycle.go` (`withIncomingLinks` and its callers), `source.RemoveLinkChecked` | link path and target |
| `modsconfig.write` | the `ModsConfig.xml` writer behind `SaveWith` (07 §4) and `LaunchWith` | environment, plan, `loadout_version` number, count of Mods |
| `launch` | `LaunchWith`, `LaunchPackWith` | plan or pack, method; result of the process start |
| `workshop.subscribe`, `workshop.unsubscribe` | `workshopsub.go` (`steamworks.Subscribe/Unsubscribe/Resubscribe`), `credit.go`, `subscribeworkshop.go` | Workshop id, Mod name |
| `library.move`, `library.lock`, `library.rename`, `unhide` | `MoveToLockedFor`, locking, `Rename`, `Trash`/`unpin` in `hidden.go` | Mod, from, to |
| `modsettings.write`, `.reset`, `.apply`, `.copy`, `.recycle` | `modsettingsfiles.go`, `modsettingswrite.go`, `gui/modsettings.go` | Mod and setting file; for a single value only the file, not the value |
| `save.rename`, `.copy`, `.backup`, `.trash` | `saveactions.go` | save name |
| `pack.build`, `pack.update`, `pack.remove`, `pack.import` | `pack.go` | pack name |
| `env.add`, `.set`, `.remove`; `folder.add`, `.remove`; `plan.delete` | `envs.go`, `folders.go`, `store.DeletePlan` | name |
| `download`, `texture.optimize`, `tool.install`, `tool.remove` | game, SteamCMD, todds, texture tasks | what and where |
| `update.apply`, `update.rollback`, `uninstall` | `internal/selfupdate`, `app/selfupdate.go`, `app/uninstall.go` | versions or the chosen items |
| `config.set` | `archo config set` and Settings | key and new value for keys that change behaviour; never secrets (`internal/secret`) |

Not recorded: list edits (undo has them; one `modsconfig.write` row marks the moment they reached the game), background checks under the master switch (16 §2.16), cache and meta writes, community data refreshes, feedback sending (`feedback-reports.json` already records it).

Choosing the primitive rather than each caller is the main design choice. The recycle bin has one entry point by rule (`TestArchTrash`), so every caller, present and future, is logged for free. Where an operation has no single primitive (subscribe, Mod settings), the hook goes in the lowest function that touches the disk or Steam, never in a surface.

## 4. Record shape and storage

New table in `archo.db` (schema 43; one entry in `upgrades`, `internal/store/store.go`):

```
op_log(id INTEGER PRIMARY KEY, at INTEGER NOT NULL,        -- Unix milliseconds
       origin TEXT NOT NULL,                              -- gui | tui | cli | mcp | agent | auto | child
       env_id TEXT NOT NULL DEFAULT '',                   -- environment, when one applies
       op TEXT NOT NULL,                                  -- code from §3
       subject TEXT NOT NULL DEFAULT '',                  -- human label: Mod name / plan / pack / save
       target TEXT NOT NULL DEFAULT '',                   -- path or id; stored as is, scrubbed when it leaves (§7)
       result TEXT NOT NULL,                              -- ok | failed | refused | partial
       detail TEXT NOT NULL DEFAULT '',                   -- reason or error text, counts; short
       batch INTEGER NOT NULL DEFAULT 0)                  -- id of the parent row for multi-step actions
CREATE INDEX op_log_at ON op_log(at);
```

Decisions:

- **In `archo.db`, not a text file.** WAL already handles GUI, TUI, CLI and MCP processes writing at once (04 §4: `_txlock=immediate`, `busy_timeout`); a text file would need its own cross-process locking and rotation. Uninstall already deletes the database with the data directory, so no new uninstall item, but 02 §1 gets a row.
- **Global table, `env_id` as a column.** Players run several environments over one game (03 §6); a shared action (Workshop unsubscribe, lock library) belongs to none of them.
- **Retention.** Keep the newer of 90 days and the latest 500 rows, trimmed at start-up in the background after the first page draws (the same place as `PruneFacts`). A row is about 200 bytes, so 500 rows are about 100 KB; the bound exists to stop a looping agent from filling the database, not for size.
- **Volume guard.** One player action writes one row for its main effect. A batch (uninstalling 40 Mods, launching with 300 links) writes one parent row and, for links, only *counts* in `detail` unless a child failed: failed children get their own row with `batch` pointing to the parent. This keeps a launch at 1–2 rows.
- **A failed write must not fail the action.** Logging errors are swallowed (with one `log.Printf`). The log is evidence, not a gate. The one exception is none: no operation waits for it.

### Where the origin comes from

Surfaces are separate processes: `RimArcho.exe` (GUI), `archo.exe` run bare (TUI), with arguments (CLI), `archo mcp` (MCP). So the origin is set **once per process** on the `App`, not per call:

- GUI, TUI, CLI set it in their `main`; MCP sets `mcp`.
- Archo (the built-in assistant) uses the MCP tool table (13 §1.1). Its launcher passes `ARCHO_ORIGIN=agent` to the MCP server it starts; any other MCP client is `mcp`. The check that the launcher is the only place that sets it is a test.
- Child processes the GUI starts (`childcommand.go`, Monitor installation, background scans) inherit `ARCHO_ORIGIN` from the parent, so their rows say `gui`, not `cli`. Background automation under the master switch (16 §2.16) records `auto`.

This avoids adding a context parameter to the hundreds of `App` methods the GUI and TUI call directly (13 §1.1: they do not go through `cmds.Execute`). `cmds.IsMCP` stays as it is for text hints.

### Multi-step actions

Only five flows need an explicit parent: launch (`LaunchWith`), pack launch, uninstall, self-update and gradual replacement (`creditreplace.go`). Each opens a handle at the start (`op := a.Ops.Begin("launch", subject)`), passes it down to the primitives it calls, and closes it with the result. Everything else is a single row from a single primitive. There is no goroutine-local state, so concurrent operations in the GUI cannot attach to the wrong parent.

## 5. Package and code structure

- New package `internal/oplog`: the `Log` type (append, recent, prune), `Row`, and the `op` constants. It depends only on `store`. `internal/app` owns one `*oplog.Log` (`App.Ops`) and the origin.
- A nil `Ops` is valid and does nothing (tests and `testkit` homes without a log), so no existing test changes.
- Architecture checks (`TestArch*`, 17 §4): add a check that the primitives named in §3 call `Ops` (a table of function names, like the trash check), so a new write path cannot silently skip it. Add `oplog` to the dependency rules.
- Display text is not stored. `op` is a stable English code; `view` turns codes and `detail` templates into the player's language (16 §2.1 text rules, `view.T`), and `subject`/`target` keep their original text.

## 6. Where players and agents see it

- **GUI**: a "What RimArcho did" list. Candidate placement: under Background tasks (16 §2.1), since both answer "what has the program been doing". A row is: time, an icon for the operation, the subject, and a result mark; failures and refusals are the visible ones. Visual first (project preference): filter chips by result and by origin; the target path and error text on expand. Not a new top-level page.
- **TUI**: the same list in the command panel (`ops`), newest first.
- **CLI and MCP**: `archo ops [--since <time>] [--op <code>] [--failed] [--all]`, MCP tool `list_operations`, same text word for word (13 §1.1). Default output is the latest 20 rows. This is also how an agent answers "what did I just change?" without guessing, which fixes the 15 gap: the sentence "No separate AI action log" becomes "Archo's actions are in the operation log (origin `agent`)".
- **Not an undo.** A row never offers "undo". The recycle bin and undo stack keep their own routes; a `trash` row can say "in the recycle bin" and a `modsconfig.write` row names its version (`archo history`, 07 §3.3) so the player knows where to go.

## 7. Feedback attachment

Today the feedback form attaches three optional parts: enabled Mods, paths, game-log errors (`view.FeedbackAttachments`). Add a fourth, **recent operations**:

- Key `ops`; heading "recent operations"; off by default for most categories, **on by default for "my files changed or are missing"** (the data-loss template in the feedback-template design).
- Content: the latest 30 rows in the CLI text format, so a maintainer reads the same text an agent would. `feedback.Scrub` turns the Windows user name in `target`/`detail` into `%USERPROFILE%`, as for the other parts.
- The player sees the exact text in the preview before sending (16 §2.14), and the existing rule that a report is public on GitHub applies. Mod names and paths inside the data directory are the only personal content; no account names, tokens or secrets are ever recorded (`internal/secret`).
- It survives the link-length limit like the other sections: a folded `<details>` block, shortened with "… N more rows" (`Draft.More`).

## 8. Privacy and safety

- The log is local data in `archo.db`; sending is explicit and previewed.
- Never recorded: passwords, Steam Guard or TOTP codes, API keys, account session contents, Mod setting *values* (only the file), save contents. `config.set` records keys, and values only for a fixed allow-list of non-secret keys.
- It does not widen what RimArcho writes: one table inside the existing data directory (02 §1 row "RimArcho's own data directory"), no new files, no network.
- The log is **evidence, not authority**: nothing in RimArcho reads it to decide an action.

## 9. Cost

- Writes: one small insert per player action, inside the primitive's own transaction where it has one, otherwise its own. Far below the cost of the file operation it describes.
- Reads: only when the list, `archo ops` or feedback is opened; indexed by time.
- Start-up: the prune is a single `DELETE` on an indexed column, done after the first page draws. No effect on the load thresholds in 16 §10; add one row to 18 (rows and database size after a launch-heavy session).

## 10. Decisions needed

1. **Scope of §3.** Everything above, or first only the destructive and irreversible ones (`trash`, `workshop.unsubscribe`, `modsconfig.write`, `link.*`, `save.*`, `update.*`, `uninstall`) and add the rest later? Recommendation: the destructive set first; the list is then short enough to read, and reports about lost files are the case that motivated this.
2. **Retention.** 90 days / 500 rows, or longer? A looping agent is the only reason for a row cap.
3. **GUI placement.** Under Background tasks (recommended) or inside Settings.
4. **Archo origin.** An environment variable set by the launcher (proposed), or the MCP `clientInfo` name. The variable is simpler and testable; `clientInfo` is spoofable by any client but nothing depends on it.
5. **Whether `config.set` is logged.** It helps explain "my settings changed", but it also writes a row per toggle in Settings. Recommendation: log only keys that change files or network behaviour.

## 11. Rollout and checks

If accepted:

1. Schema 43, `internal/oplog`, `App.Ops`, origin plumbing; hooks in the destructive set; `archo ops` and the MCP tool with golden tests (`TestCLIParity`).
2. GUI and TUI list; the feedback attachment; the matching rows in 02 §1, 04 §4, 13, 14, 15, 16 §2.1 and §2.14, 18, `CHANGELOG.md` and `UserGuide/release-vm-tests.md` (a row: do each action, find its row, including a refusal and a failure).
3. Remaining operations from §3.

Tests: a unit test per primitive that the row is written with the right `op`, `origin`, `result`; an architecture test that every primitive in the §3 table calls `Ops`; a concurrency test with the GUI, a CLI child and MCP writing together; a failure test where the log cannot be written and the action still succeeds; a scrub test for the feedback text. VM round: delete a Mod, unsubscribe, launch, force a refusal (game running), then read the rows in the GUI and attach them to a report.
