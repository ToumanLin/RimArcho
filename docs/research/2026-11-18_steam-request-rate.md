# Steam request rate (2026-11-18)

Player feedback 2026-11-18: after using RimArcho, Steam "refuses to open Mod detail pages" (rate limited). This note lists every path where RimArcho talks to Steam, how many requests each makes for a library of about 1,000 subscribed Workshop Mods, and what changed. Code read on `dev/1.1` at `73c9befd`; no real Steam endpoint was contacted.

## 1. Request paths

"Web API" is the keyless `api.steampowered.com/ISteamRemoteStorage/*` POST. Counts are for 1,000 Workshop Mods.

| Path | Trigger | Requests for 1,000 Mods | Batching | Cache | Retries / backoff | Concurrency |
| --- | --- | --- | --- | --- | --- | --- |
| Mod description, `app.Describe` → `fetchWorkshop` (Web API `GetPublishedFileDetails`) | Cursor rests on a Workshop Mod whose description is not cached: GUI editor (75 ms debounce, then at once), GUI Mod library details (**no debounce**: each step reads, and fetches as soon as the local read is in), full-screen details and TUI (400 ms rest), CLI `describe`, MCP | 1 per Mod viewed; stepping through the whole library = up to 1,000 single-ID requests, several per second while holding an arrow key in Mod library | 1 ID per request | Disk, `cache/workshop/<id>.json`: fresh while `time_updated` ≥ the ACF time, else 7 days. **Answers with `result ≠ 1` (removed, private, rate limited) are never fresh** when the ACF has a time: re-fetched on every view. **HTTP failures (429, 5xx, offline) are not cached**: re-fetched on every view | None | Unlimited (one per detail call) |
| Workshop client / SteamCMD update check, `CheckSubscriptionSource` → `workshopDetails` | Player presses check (Workshop client); automatic A03 every 24 h for SteamCMD copies only | 10 (+1 per failed batch, +1 batch for items Steam called busy) | 100 IDs | Result in `meta`; **the descriptions in the answer were thrown away**, so every detail view fetched them again | One retry after 1 s; item results 2/16/20/25/84 retried once after 1 s | 4 requests at once |
| Subscribe preflight, `preflightWorkshop` | Subscribing to Workshop items (Steam or SteamCMD path) | ⌈n/100⌉ per subscribe; 10 for 1,000 | 100 IDs, 3 s each | None (one-shot) | Stops at the first failure | Sequential |
| Collection read, `ReadCollection` (`GetCollectionDetails` + `workshopTitles`) | Import / follow a collection; A14 on opening All lists (≥ 10 min per collection) | 1 + ⌈n/100⌉ per collection | 100 IDs | Followed: 10 min (`CheckEvery`) | None | Sequential |
| Self-update Monitor stamp, `updatechannel.checkMonitor` | A01 "self" daily, `archo update --check` | 1 per day | 1 ID | `meta` | None | 1 |
| Description images, `DescImage` | Full-screen details only, each image in the description | 0 for Steam unless authors host images there (`images.steamusercontent.com` is the 5th host, 16 §4.3); i.imgur.com is 72% | — | Disk forever (`cache/workshop-img`) | Failures not cached: each full-screen view tries again | Unlimited downloads, 2 decodes |
| Steam client worker (`steamworks`, SteamAPI via the game's DLL) | Subscribe, unsubscribe, update all, `workshop state`, credit prompt probe (only when a SteamCMD credit batch is pending, on game exit / window focus) | Local IPC; Subscribe/Download 50 IDs per worker call | 50 | — | S05 resend once | 1 worker at a time |
| SteamCMD downloads | Player subscribes with SteamCMD | 25 items per SteamCMD run, Steam content servers | 25 | Files on disk | S03: two extra rounds after 5 s, 15 s | 1 process |
| SteamCMD branch list (`app_info_print`) | Download-game page | 1 SteamCMD run per 10 min | — | In-process 10 min (C02) | 30 s cooldown | 1 |
| SteamCMD / todds tool check | A06 weekly | 1 GET to `client-update.akamai.steamstatic.com` | — | `meta` | None | 1 |
| `steam://` and Workshop links | Player clicks "open Workshop page" | 1 per click, in the Steam client or browser | — | — | — | — |

Nothing scrapes `steamcommunity.com` pages: every Workshop link is opened by the player.

## 2. What plausibly explains the throttling

1. **Detail fetches have no rate limit and no memory of failure.** Browsing the Mod library with the arrow keys fetches every uncached Mod the cursor stops on, without the debounce the editor has. Once Steam answers 429 (or `result 84`, "rate limited"), nothing remembers it: each further cursor stop sends another request, which keeps the limit in force. The Web API and the Workshop pages the Steam client shows are both Valve services; a per-address limit on one plausibly shows on the other.
2. **Removed or private Mods are re-fetched on every view** (their `result 9` answer never counts as fresh), and so is every Mod after the update check, which already had its description.
3. **A second, unrelated cause to rule out:** a SteamCMD download with the same account the Steam client uses logs the client off (`Session Replaced`, seen in the VM 2026-10-24 and 10-27; B11 in `2026-10-27_VM情景5-6-9.md`). The client then cannot open Workshop pages until restarted. Ask the player whether they set `steamcmd.account` to their own account.

The update check (10 requests) and the others are small and not timer-driven beyond once a day.

## 3. Changes (this branch)

- **One gate per Steam Web API origin, process-wide** (`app/steamgate.go`, 11 §2.1, 16 §2.16.1 S13): at most 2 requests at once, a burst of 4 then 1 per second. A 429 or 403 answer, a `result 84`/`25` item, or 3 server errors in a row start a cooldown (`Retry-After` if longer, else 1 minute, doubling to 30 minutes); during it requests fail at once with "Steam is limiting requests" and its end time, without contacting Steam. The cooldown is stored in `cache/steam/cooldown.json`, so the CLI, MCP and a restarted window honour it too.
- **Description cache** (16 §4.3, C01): an answer for a removed or private item is fresh for a day; transient answers (busy, rate limited) are not written. The update check writes the descriptions it receives into the cache, so details of checked Mods need no request.
- **Detail fetch timing** (G12): the GUI editor and Mod library wait 400 ms after the local result, like full-screen details and the TUI, and fetch only if the Mod is still shown.

Estimated effect for 1,000 Mods: stepping through the whole library sends at most one request per Mod the cursor rests on for 0.4 s, at most 1 per second sustained, and none while Steam is limiting; after one update check, none at all.

## 4. Needs VM verification

- Browse the Mod library and editor quickly over uncached Workshop Mods; descriptions still appear once the cursor rests; nothing visibly slower otherwise.
- Real Steam 429 cannot be triggered safely; the cooldown is covered by fake-server tests only.
- Ask the player about `steamcmd.account` (§2 item 3).
