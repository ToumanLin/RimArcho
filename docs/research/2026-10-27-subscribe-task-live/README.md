# Live subscription-task integration test · 2026-10-27

Validated on a real Windows machine with a copy of real data, real Steam public API, real SteamCMD, and real GUI pages. No `-steam-api`, `/dev/pagecheck`, or download stand-in was used. The branch first rebased onto `dev/0.9.0` containing `d64c974`; original `de6cdaa` became `40f54af` after rebase.

## Isolation and operations

Copied `%APPDATA%/RimArcho/archo.db*` and `config.toml` into a system temporary directory. Changed only the copy: SteamCMD source root pointed to a temporary directory, `cfg:steamcmd-mods.dir` to a separate temporary download directory per round, tool path reused managed SteamCMD, account anonymous. Browser profile, APPDATA, and LOCALAPPDATA were also isolated to temporary directories. Scanning read existing game and Mod files; no game launch, deployment, real list editing, or real Mod deletion was performed.

Selected public Mods by size from the copied inventory, then confirmed 30 entries with real `GetPublishedFileDetails` POST responses: `result=1`, `consumer_app_id=294100`, total size 811,767 bytes. Added `3226502149`, `3309016770`, `3778487830`, and `3809376831`, all returning real `result=9` responses. Full public IDs, titles, and sizes are in [observations.json](observations.json). To show "not installed" in the copy, these IDs' copied inventory states were set to gone; original Mod files stayed untouched, and each round used a new download directory.

Built `./spikes/guidev` and `./cmd/archo`, launched with `guidev -home <copy> -exe <temporary archo.exe>`. Chrome used `--headless=new` and a separate profile; CDP communication followed `scripts/check-accounts-page.cjs`. Previewed 34 Workshop links on the real import page, clicked "subscribe to all 34", selected "download with SteamCMD", and opened background tasks. Read DOM text, captured CDP screenshots, and sampled real `Tasks` responses about every 250 ms; retained the running script to verify actually submitted IDs.

Checked processes before each download start and continuation. When another worktree's SteamCMD was present, waited for it to exit without terminating it or running concurrently. Every actual download used `+force_install_dir` inside this test's temporary directory, never the player's real `steamcmd-mods` download root.

## Measurements

| Check | Observation |
| --- | --- |
| Complete download across batches | First round downloaded all 30 public entries successfully; two internal calls contained 25 and 5 entries, while the task page showed only one task, no batch rows or steps |
| Preflight | The 4 deleted entries showed "no longer on the Workshop" before the first download; none of their IDs appeared in download scripts, that round's console_log, or credit_item |
| Live progress | First-round samples included 11%, 12%, 13%, 15%…95%, 99%, with specific Mod names; the final version's full cancel/continue/error-retry flow produced 104 task snapshots, one task ID, at most one row, zero steps, and monotonic percentages |
| Cancellation | In the latest round the page's "cancel" was clicked automatically after 15 successes; one more was confirmed before stopping. Final counts: completed 16, cancelled 14, permanent failures 4, progress 45%, `20 / 34` |
| Process ownership | Before cancellation, the sole SteamCMD was PID 99540, parent PID 75692 (this round's guidev), with a command targeting the temporary directory and unique script; after cancellation SteamCMD was absent and the script cleaned up |
| Preserve successes | All 16 confirmed entries had `.rimarcho/source.json` after cancellation and were live in the copied DB; unconfirmed entries were not counted as successes or credited |
| Continue | Clicked "continue 14 items"; reused `t1793088101148-1`, still 45%, `20 / 34`; the new script contained exactly those 14 remaining IDs, without redownloading 16 successes or 4 deleted entries |
| Real error and automatic retry | On continuation, verified PID 44624's parent and temporary target, then force-terminated only that PID under control. At sampling it showed 52%, `22 / 34`; two more entries completed during process lookup and termination, so the final automatic retry script contained only 10 unconfirmed entries; no retry click needed |
| Retry progress | After interruption it stayed at 59%, `24 / 34`, without jumping to 99% for 10 temporary failures; it then increased with successful entries, ending at 100%, `34 / 34` |
| Final storage | All 30 public entries succeeded: 30 origin files, 30 live instances; 4 deleted entries remained permanent failures |

Processes, script ID sets, progress timeline, and actual DOM text are in [observations.json](observations.json).

## Actual page text

The left side had one task row titled "**subscribe 34 mods**", status "partly done", second line "**completed 30, failed 4: no longer on the Workshop**".

The right summary contained only "**4 mods are no longer on the Workshop (deleted or private)**", no URLs or four repeated reasons.

"every item" showed 34 entries: `Better Animal Info`, `FrameRateControl`, etc. used real titles; the 4 deleted entries were named `Workshop <ID>` because the public API no longer returns titles. Their reasons were all "**no longer on the Workshop**", without `failed:`. The failed-item menu actually contained "**open the Workshop page**", without a retry-this-item action.

Primary button: "**go to subscriptions and updates**"; also "take off the record", no "retry 4". After cancellation the button was "**continue 14 items**", and remaining rows showed "cancelled".

## Integration findings and fixes

1. Previously, continuing created a new "subscribe 13 mods" task starting at 0%. It now requeues the original task, executes only remaining IDs, and merges previous results and byte weights. Also fixed cancellation of a queued continuation overwriting earlier successes.
2. Previously, failures awaiting automatic retry counted as completed too early, making progress jump to 99% after abnormal exit. During execution, only successes and definite permanent failures count; final failures are settled when the operation ends.
3. After one real process interruption, SteamCMD returned bare `failed (Failure)` for the interrupted entry; the old classifier did not retry it automatically. This error is now treated as unconfirmed, still limited to two extra rounds. The identically named login-stage `FAILED (Failure)` retains its login classification. The final rerun verified automatic recovery from real abnormal exit; changed behavior of the bare Failure branch was separately checked by injected tests.

## Validation scope and limits

`go build ./... && go vet ./... && go test ./internal/...` all passed.

This run actually validated anonymous SteamCMD, downloads across batches, deleted-item preflight, page text, live progress, cancellation, continuation on the same task, and automatic retry after a real process error. API-request failure fallback, non-RimWorld app IDs, timeout/network loss, exhaustion of two consecutive rounds, disk/ownership/permission errors, login, and Steam-client subscription cancellation remain mainly covered by offline unit tests. Windows process-tree ownership also has a real helper-process test; as required, no second SteamCMD was launched concurrently during real downloads to manufacture contention.

Cancellation may leave partial files already written by SteamCMD. In one run another entry's About.xml had landed without success confirmation, and scanning could recognize that folder. The task still marked it cancelled, wrote no success origin or credit, and requested download again on continuation. Directory existence cannot substitute for success confirmation.

After processes exited, the data copy, downloads, temporary programs, and Chrome profile were moved to the recycle bin; only observations with local private paths removed remain in this directory (screenshots were not committed).
