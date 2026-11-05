# Steam credit: GUI API contract

Backend entry points: `internal/surface/gui/credit.go`, `subscribe.go`; business rules belong to [11 §1](../specs/11-downloads-and-accounts.md#1-subscriptions-git-download-pages-steamcmd-2026-10-24). These are Wails GUI methods; errors use existing Promise rejection. No new HTTP routes. The frontend dialog calls them from `assets/js/credit.js`.

#%23 Subscribe all

```go
Subscribe(r SubscriptionRequest) (string, error)
SubscribePreview(r SubscriptionRequest) (*view.Preview, error)
```

Full `SubscriptionRequest` JSON (existing fields stay compatible; only `batch` is new):

```json
{
  "kind": "workshop",
  "url": "8100 8200",
  "refType": "",
  "ref": "",
  "git": false,
  "from": "",
  "steamcmd": true,
  "batch": true
}
```

`kind` retains `workshop/git/page`. For workshop, `url` holds space-separated IDs or Workshop URLs; `refType/ref` is for Git; `git/from` is for download pages. Omitted booleans are false. `batch` applies only to actual workshop + steamcmd execution; previews create no batch. The return value remains a localized "task queued" string; failures appear in task results. One request records one business batch; only IDs downloaded successfully in this request and scanned as SteamCMD instances are recorded.

Import getAll, followed collections and bulk subscribe from needs attention first show the two download methods, then call `Subscribe({kind:"workshop",url:ids.join(" "),steamcmd,batch:true})`. Other calls may omit batch.

#%23 Query and answer

```go
CreditPrompt() (*app.CreditPromptData, error)
CreditAnswer(batchIDs []int64, replace bool) (*CreditAnswerResult, error)
```

`CreditPrompt()` takes no arguments and returns a JSON object or `null`:

```json
{"batchIDs":[1,2],"modCount":14,"examples":["Harmony","Camera+","Dubs Break Mod"]}
```

| Field | Type | Meaning |
| --- | --- | --- |
| batchIDs | number[] | Due batches in creation order; pass back unchanged when answering |
| modCount | number | Mods still represented by live SteamCMD instances, deduplicated across batches by Workshop ID |
| examples | string[] | Up to three names explaining the question's scope |

Returns `null` when no batch is due, no original copy remains eligible for handoff, Steam is unavailable or any registered game is running. Temporary unavailability does not consume the question. Querying does not change batch state; the frontend must coalesce concurrent queries and show only one dialog at a time. Call when a game exits, the window regains foreground or the app first becomes ready; do not open another dialog while one is visible. Preserve unsubmitted answers on API failure.

Wails calls `CreditAnswer(prompt.batchIDs, true)` or `CreditAnswer(prompt.batchIDs, false)`, without a request object. Normal return:

```json
{"batchIDs":[1,2],"state":"replacing","taskID":"t1793000000000-1"}
```

Dismissal returns:

```json
{"batchIDs":[1,2],"state":"dismissed","taskID":""}
```

| Field | Type | Meaning |
| --- | --- | --- |
| batchIDs | number[] | Batch IDs submitted in this answer |
| state | string | `dismissed` or `replacing`; may be `done` when an accepted batch is already complete |
| taskID | string | Corresponding background task ID; empty for dismissal or completed work |

Empty IDs, unknown batches, batches not yet due or attempts to reverse dismissal reject the whole answer, with no partial commit. Repeated dismissal is idempotent; accepting batches already replacing/done creates no new subscription obligation. Persist acceptance before starting the task; even if startup then fails, the next launch recovers from SQLite. Multiple acceptances may share one running task ID. Close this dialog only after API success; dismissed batches never return.

#%23 Background task, progress and cancellation

Task kind: `steam-credit-replace`, lane: `self`. Use the existing `Tasks()` `view.TaskPage` and its corresponding `rows`; this task uses existing fields:

```json
{
  "id":"t1793000000000-1",
  "kind":"steam-credit-replace",
  "lane":"self",
  "state":"run",
  "title":"replace SteamCMD mods with Steam subscriptions",
  "obj":"SteamCMD mods",
  "line":"waiting for Steam or a safe time to replace",
  "count":"3 / 14",
  "pct":21,
  "bar":true,
  "items":[{
    "ref":"c4fe1092",
    "name":"Harmony",
    "state":"ok",
    "stateText":"done",
    "rank":4,
    "why":"switched to Workshop; SteamCMD copy moved to recycle bin",
    "size":"",
    "bytes":0,
    "got":0,
    "downloadSize":0
  }],
  "acts":[{"id":"stop","label":"stop the rest","key":"x"}]
}
```

Only existing TaskRow fields consumed by this task are listed; other fields stay unchanged. The total includes every recorded item; progress is terminal items / total, including explicit failures and retained copies. Do not invent Steam download byte percentages. Items use `wait` while waiting, `run` while processing, `ok` for completion/retention (see `why`), and `fail` for unrecoverable errors. Task completion is `done/part/fail`, stopping is `stop`. Failures retain the original EResult and Workshop page; unfinished items stay in SQLite.

Task events reuse `tasks`: a change payload is `{}`, at most once per 500 ms; send completion immediately:

```json
{"ended":{"ok":false,"id":"t1793000000000-1","kind":"steam-credit-replace","ref":"","text":"localized task result"}}
```

`ended.ok` is true only for task state `done`. Call `Tasks()` again after a change; do not infer per-item states from events. No new `credit-progress` event.

Stop with `TaskAct("stop", [taskID], "")`: cancel the current Steam worker and stop at an item handoff boundary. Resume with `TaskAct("resume", [taskID], "")`, or `CreditAnswer(batchIDs,true)` to reawaken unfinished obligations. Stopping does not undo subscriptions, replaced plans or recycled copies, and does not show the credit question again. The next GUI launch resumes automatically; no resident process. Clearing task history does not affect resumption. Explicit permanent failures are not retried by "resume". Waiting for scans, file tasks or client scan results uses a fixed short deferral without increasing subscription failure counts. Closing the window cancels the worker and waits at most three seconds for the replacement runner; subscription confirmations and handoff checkpoints persist.

#%23 Frontend text

The frontend retains English sentences as keys (`T()` / `Tf()`); Chinese translations are registered in `internal/view/subscriptionstext.go`. Implementation: `assets/js/credit.js`.

| English key | Translation meaning |
| --- | --- |
| from Steam (slow, but credits the modder with a subscription; Steam manages it) | Subscribe through Steam (slow, but counts toward the author's subscriptions; managed by Steam) |
| from SteamCMD (fast, but does not credit the modder; RimArcho manages it) | Download through SteamCMD (fast, but does not count toward the author's subscriptions; managed by RimArcho) |
| Do you want to credit the modders by subscribing from Steam? | Subscribe on Steam to give the authors subscription credit? |
| %d mods, such as %s | %d Mods in total, for example %s |
| replace these SteamCMD mods with Steam subscriptions, little by little | Gradually replace this batch of SteamCMD Mods with Steam subscriptions in the background |
| dismiss | No thanks |

#%23 Limits and validation

Lists are shared by all games; EnvID records only the game at creation time. Before handoff, check every environment: wherever the old SteamCMD instance can load (including after deployment), the new workshop instance must also load. Otherwise mark the whole item `retained`, name the environment and change no shared list or deployment. Corrupt staging JSON likewise makes the whole item `retained`, names the list and does not enter backoff retry. After the existing writer hands off deployed game projections, remap all lists' current content, historical versions, undo/redo, staging and anchors to the workshop ID in one transaction. Reuse note merging to preserve both texts and existing origins; add no undo step that could restore the old copy. Do not rewrite fixed packs.

Besides loadability and corruption above, mark an item `retained` if a fixed pack, an actual inbound link (including managed_link ledger entries pointing into a removed game directory) or game deployment not yet handed off still uses the original copy. History/undo alone does not cause retention. Retained items are finished; do not repeatedly try to recycle them in the background. Overlapping batches share persisted subscription confirmations and unrecoverable failures without subscribing twice. When an earlier batch replaced the same original copy, later batches mark the item `done`, because "already replaced with Workshop". Subscribe, CreditPrompt and CreditAnswer signatures/JSON, task kinds and `tasks` events stay unchanged. DDS texture_mod/texture_file records bind to the original directory, file state and content snapshot; do not transfer them directly or claim the client directory is optimized. Records remain with the original physical copy; when present, append to success: "texture records are bound to the old folder and content snapshot; optimise Workshop textures again".

RimArcho processes sharing a data directory use an OS lock to serialize list editing, read synchronization, saving, undo/redo, staging, launching (including CLI and packs) and handoff. List edits hold the lock across read and save; reject conflicting late snapshots. The remap transaction increments `meta.credit.remap`; long-lived TUI edit batches must reload when it changes. Scans run outside the lock; still check processes before handoff and recycling. Launch waits up to ten seconds for the lock, then returns a retryable timeout error. While waiting, Go sends one GUI `launch-wait` event with payload `{"text":"waiting for background replacement to finish"}` (localized text). The frontend may show this text in the existing launch status area, without a new call; stop displaying it on launch success or failure. English i18n key: `waiting for background replacement to finish`; timeout diagnostic: `waiting for background replacement timed out; try launching again`. Steam and direct game launches do not honor this lock, leaving a small race between process checks and external launches. The real client may download again; downloads and recycling may temporarily require double space. Steam uses the currently signed-in client account; no new account binding or account-switch confirmation API.

Unit tests use fake Steam/SteamCMD runners, never real accounts. They cover creation gates, success filtering, session deduplication and instance matching, merging/deferral/permanent dismissal, retry/pause/cancel, launch races, cross-game deployment resumption, failures and reference retention, recycling/undo after real LaunchWith history writes, whole-item refusal for cross-environment shared lists, note merging and no false DDS ledger migration, managed_link inbound-link retention, corrupt staging terminal states, old edit-batch rejection, short launch waits, transaction rollback, cross-process locking, overlapping-batch deduplication, short waits not increasing Attempts, bounded shutdown of slow workers, JSON and GUI task recovery. Real-account subscriptions and handoff in real games still need integration acceptance.