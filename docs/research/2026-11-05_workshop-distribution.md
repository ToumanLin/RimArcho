# Publishing RimArcho on the Steam Workshop and updating from it (2026-11-05)

ToDo "Auto Update": publish RimArcho on the RimWorld Workshop, upload updates without starting the game, and make RimArcho's own update pull from the Workshop instead of GitHub. **The pull side is anonymous SteamCMD** (`+login anonymous`, `workshop_download_item 294100 <id>`), the way RimArcho already downloads Workshop Mods; it needs no Steam client, no RimWorld license and no subscription. Uploading needs an account and is a separate, maintainer-only problem. Nothing was uploaded or tested against Steam yet; **[C]** marks facts confirmed from a source, the code or a live call, **[I]** inferences.

## Summary

- **Updating from the Workshop with anonymous SteamCMD fits the code almost as it is.** `runWorkshopBatches(exe, base, "anonymous", ids)` (`internal/app/subscribeworkshoprun.go`) already runs a hidden anonymous SteamCMD, retries (3 attempts, 5 s and 15 s backoff) and confirms each item from `Success. Downloaded item <id>`. **[C]** Public RimWorld items download anonymously: 12 of 12 downloads succeeded with four public items in [2026-10-26 parallel downloads](2026-10-26_steamcmd-parallel-downloads.md) §3, and the [RimSort guide](https://rimsort.github.io/RimSort/user-guide/steamcmd-browser) documents the same. `InstallSelfUpdate` does not care where the verified zip came from; only the check and the download change (§1).
- **The item is RimArcho, and its Mod part is Archo Monitor** (decided 2026-11-05, §1.5). Players subscribe, extract the zip and run `RimArcho.exe`; on first start RimArcho moves itself out of the Workshop folder (§1.6). The subscribed copy is the Monitor the game loads.
- **Put the release zip and its `.sha256` in the item**, the same two files as a GitHub release. Then `ParseSum`, the size and hash check and `selfupdate.Extract` are reused unchanged, the item holds no loose `.exe` for Steam to rewrite, and no Steam subscription is involved at all.
- **Players without Steam can update too.** GOG and DRM-free installs have no Steam account, and anonymous SteamCMD needs none. GitHub stays for the first download, for prereleases, and as the fallback when SteamCMD cannot run.
- **Uploading without the game** uses the existing `archo steam-worker` and `ISteamUGC` (`CreateItem`, `SubmitItemUpdate`), run only by the maintainer (§2).
- **Open question that decides the check design:** the Web API does not return a version number. Recommended: put the version in an item tag, and test that (§1.2).

## 1. Updating from the Workshop (anonymous SteamCMD)

### 1.1 What exists and what changes

| Step | Today (GitHub) | With the Workshop |
| --- | --- | --- |
| Check (`CheckSelfUpdate`) | `selfupdate.Client.Latest`: GitHub Releases, 15 s, ≤ 1 MB | `GetPublishedFileDetails` POST, no key (`SteamAPI`, `postWorkshopDetails`, already used for Mod pages): item `time_updated`, `tags` |
| Download (`Client.Download`) | checksum file, then zip into `<zip>.part`, SHA-256 while writing, size check, 200 MB limit | anonymous SteamCMD into a private base folder, then copy zip and checksum into `.archo-update/download/` and run the same checksum and size check on the local file |
| Extract | `selfupdate.Extract`: `Files` whitelist, no `..` or symlinks, 200 MB per entry, 500 MB total, both programs present | unchanged |
| Wait, `Apply`, `VersionOf`, rollback, restart, cleanup | 17 §5.1 steps 4–7 | unchanged |

Code touchpoints: a source choice in `internal/app/selfupdate.go`; in `internal/selfupdate` split `Download` into "fetch" and "verify a local zip against a checksum"; a Workshop branch next to `selfClient()`.

### 1.2 The check, and where the version comes from

**[C]** `GetPublishedFileDetails` gives `time_updated`, `title`, `description`, `tags`, `hcontent_file`, but nothing that is a version. Options:

| Option | Verdict |
| --- | --- |
| **Version in a tag** (`SetItemTags`, e.g. `v1.0.2`), read from `tags` | **Recommended.** Cheap (one POST), no SteamCMD start, and the existing `Newer(running, version)` and the "one notification per version" record keep working. **[I]** Workshop tags are free-form through the API; not tried for app 294100 (test 1). |
| Compare only `time_updated` | Fails the first time: a running GitHub-installed build has no stored Workshop time, so the same version would be offered again. Also changes on description edits. Fallback only ("a newer upload exists", no number). |
| Run SteamCMD as the check, read the version from the zip name | Needs SteamCMD and a download before the player confirmed anything; 17 §5.1 says a check never downloads. Rejected. |
| Keep reading the version from GitHub, only download from the Workshop | Contradicts the ToDo and makes the Workshop lag a dependency. Rejected. |

**Why not reuse the Mod update check.** **[C]** For SteamCMD Mods (`CheckSubscriptionSource`, `view/subscriptions.go`, `PendingSubscriptionUpdates`) RimArcho compares times only: the Web API `time_updated` against `max(ACF timeupdated, download time)`. It compares no hash with anything remote (`hcontent_file` is not read anywhere in the code). The xxh3-128 fingerprints in `internal/scan` describe the local folder (identity, snapshots, "did the Mod change"), not the remote item. The content comparison itself is SteamCMD's: on every `workshop_download_item` it compares the item's manifest with the one in `appworkshop_294100.acf` and transfers only changed chunks, or logs "has no changes". A time check is good enough for "something newer exists" on a Mod, but it carries no version, so RimArcho cannot tell whether it is already running what the item holds; that is why the version goes into a tag. Integrity is a separate step: the SHA-256 of the zip against the `.sha256` file (§1.4), plus Steam's own chunk hashes during the transfer.

Metadata failure (offline, item missing, `result` not 1) makes the check fall back to GitHub and say so. Prerelease channel: **[I]** the Workshop has no channels, so prerelease builds (`-rc1`) keep checking GitHub; the item carries stable releases only.

### 1.3 Download

**Subscribed players need no download.** **[I]** Steam updates a subscribed item on its own, so the new zip and `.sha256` are already in `steamapps/workshop/content/294100/<id>/` when the check sees a newer tag. RimArcho copies them into `.archo-update/download/` and verifies them as below. Confirm that the folder holds the version the tag names; if Steam has not updated it yet, use SteamCMD or wait. The steps below are for players who did not subscribe (GOG, GitHub installs).

1. **Account.** Always `anonymous`: call `SteamCMDAccount("anonymous")`, never `SteamCMDAccount("")`. With an empty override it returns `steamcmd.account`, and a named account opens the interactive console and, **[C]** when it is the account the desktop Steam is signed in with, signs Steam out (`SteamCMDSignsOutSteam`, VM run B11). An unattended update must never do that.
2. **Private base folder**, for example `%LOCALAPPDATA%\RimArcho\steamcmd-self`, not `steamcmd-mods`. The Mods folder is a registered source (a Workshop item with `About.xml` there would show up as a Mod), and **[C]** two SteamCMD processes sharing a library lost records and still exited 0 (parallel-downloads §3.3). The instance lock `accounts.Lock(exe)` is shared with Mod downloads, so the two never run at once.
3. **Reuse `runWorkshopBatches`** with one id. Do not call `downloadWorkshopGroup`: it registers sources, writes `source.json`, scans and manages links, none of which apply.
4. **Confirm.** **[C]** SteamCMD exits 0 even when an item failed; trust only `Success. Downloaded item <id>` plus the presence of the zip and its `.sha256`.
5. **Clean up the whole base folder after a successful swap.** SteamCMD decides what to transfer from the manifest recorded in the ACF, not from the files on disk. **[I]** If the content is deleted but `appworkshop_294100.acf` stays, it may log "has no changes" (parallel-downloads §3.4) and download nothing, leaving an empty folder; keep both or neither. Neither is simpler and costs one full download of a ~30 MB zip per release, which the maintainer expects to be quick. The "Confirm" check (item 4) catches the empty-folder case anyway: no zip, no success.
6. **SteamCMD missing.** Today `downloadWorkshopGroup` stops with "install SteamCMD in Settings". For the updater: offer to install RimArcho's managed SteamCMD as part of the confirmed install (an extra download and write, so it belongs in the confirmation text, 02 §1), or fall back to GitHub.
7. **Progress.** **[C]** The Workshop runner has no byte callback (parallel-downloads §2.1), so `SelfUpdateStep{Phase: "download"}` is indeterminate unless the staging folder's size is polled.

**[C]** Measured cost (four small items, one Windows machine): SteamCMD start, anonymous connect and quit with no download about 6.7 s; first start with its own update about 9.2 s; a no-change rerun about 4.7 s. Throughput on one large download was not measured here; the maintainer expects a ~30 MB zip to be quick (test 3 records the real number).

### 1.4 Verify

**[C]** Steam checks transfers against its own chunk hashes, but nothing checks the files after they land, and a sidecar checksum inside the same item only catches a truncated or half-written file, not an uploader that was hijacked. This is the same trust level as the GitHub path today, where the checksum sits in the same release. To also protect against a stolen publishing account, sign the zip with an ed25519 key whose public half is built into RimArcho; that is optional and works for both sources.

### 1.5 Source choice and what the item contains

- Settings › update source: automatic (Workshop, GitHub when the check or SteamCMD cannot work) / Workshop / GitHub. Installing still waits for the player (17 §5.1, 02 §1).
- Item contents: the Archo Monitor Mod folder (`monitor/out/ArchoMonitor/`, about 440 KB) plus the release assets:

```
About/About.xml, Preview.png, PublishedFileId.txt    (Monitor's About, item title "RimArcho")
LoadFolders.xml, 1.5/, 1.6/, 1.7/, Languages/         (Monitor)
archo-<version>-windows-amd64.zip
archo-<version>-windows-amd64.zip.sha256
```

  The zip and checksum are exactly the release assets (`ZipName`), so `selfupdate.Files` and the 200 MB limits apply unchanged; the game ignores the zip at the folder root. `SetItemContent` replaces the folder, so only the current version is in the item. Nobody subscribes for Monitor alone, so the 32 MB zip in every subscriber's download is not a cost. Linux and macOS zips would add to every pull; a separate item per platform is the way out when they become official (17 §5).
- **Versions.** The item's version tag is the launcher version. Monitor keeps its own `monitor/VERSION` (`modVersion` in About.xml), which often does not change between launcher releases.
- **Monitor identity.** Today only Instances under the Monitor source count as Monitor (19 §6). A subscribed copy plus the `_Archo_monitor_` link would be two Mods with packageId `toumanlin.archomonitor` in the game. Count every Instance with that packageId as Monitor, keep "highest `modVersion` wins", and on a tie prefer the Workshop copy: it needs no link, so it also works when `Mods/` is on exFAT (19 §6 "FAT and exFAT").
- 1.0.0 and 1.0.1 installs only know GitHub and keep updating from there until a version with the Workshop source is installed.
- **Decided 2026-11-05 (maintainer):** a free-form version tag and an unlisted item are acceptable (tests 1 and 2 still confirm they work); the item keeps a valid Mod part so a subscriber's game sees a Mod, not a broken folder; prereleases stay on GitHub only.

### 1.6 First start from the Workshop folder

The Workshop page says: subscribe, open the item folder, extract the zip, run `RimArcho.exe`. Steam rewrites the folder on every item update and deletes it on unsubscribe, so RimArcho must not run from there.

1. On start, detect a program folder under `steamapps/workshop/content/294100/` (any Steam library). **[I]** Compare the path, not the item id, so a test item works too.
2. Offer a default install folder (for example `%LOCALAPPDATA%\Programs\RimArcho`) with a way to pick another; say how much space it needs and warn when the drive is nearly full. This joins the first-run question about where RimArcho's managed folders go (default on C: Local, warn when C: is full) and the offer to import RimSort and RimCrow databases.
3. Copy the release files (`selfupdate.Files`) there, start the copy and exit; the copy removes what was extracted into the Workshop folder and keeps the item's own files (About, Monitor, zip, checksum), so the Mod and the next update are untouched.
4. Declining keeps running from the Workshop folder for this session only, with a warning that the next item update can break it. Self-update refuses there.
5. Uninstall (02 §2) works on the installed copy; the item stays until the player unsubscribes.

## 2. Uploading (maintainer side)

| Way | How | Verdict |
| --- | --- | --- |
| Steamworks client API from the existing worker | `CreateItem(294100, k_EWorkshopFileTypeCommunity)`, then `StartItemUpdate` → `SetItemContent` / `SetItemPreview` / `SetItemTags` / `SetItemTitle` / `SetItemDescription` → `SubmitItemUpdate` with a change note, `GetItemUpdateProgress`. Read call results as `subscribe` does now. Check `m_bUserNeedsToAcceptWorkshopLegalAgreement`: the item stays hidden until the uploader accepts the agreement on its page. | **Recommended.** No credentials stored; reuses `internal/steamworks/worker_windows.go`; Steam must be running and logged in, the game does not start. |
| SteamCMD `workshop_build_item <vdf>` | `appid`, `publishedfileid` (0 creates), `contentfolder`, `previewfile`, `visibility`, `title`, `description`, `changenote`. Valve calls it "for testing purposes": it needs the account's credentials. | Fallback. Logging in with the account the desktop Steam uses signs Steam out (VM run B11), and Steam Guard asks for a code. |
| In-game upload | The game's dev-mode button. | What the ToDo wants to avoid. |

**[C]** Refusals to handle: `k_EResultAccessDenied` (no RimWorld license), `k_EResultInsufficientPrivilege` (community ban), `k_EResultBanned`, `k_EResultServiceReadOnly` (recent password or email change, usually 5 days). Limited accounts cannot upload. The preview must be under 1 MB, tags cannot contain `,`, and `SetItemContent` takes an absolute, unzipped folder. go-steamworks has no `ISteamUGC`; RimSort has no uploader.

**Where it goes:** `scripts/release.ps1` must not publish (17 §5). A separate step, for example `archo workshop publish <folder> --item <id> --note <text>`, uploads the assembled item folder after the GitHub release, with the item id in `About/PublishedFileId.txt`. The Workshop therefore trails the GitHub release by that step.

## 3. Risks

- **A new dependency for updates.** Updating needs SteamCMD (downloaded from Valve, first start updates itself). Anonymous access to app 294100 Workshop items is a Valve setting RimArcho does not control; GitHub must keep working.
- **Zips and executables on the Workshop.** **[C]** No rule bans them, but Valve may "restrict or remove Workshop contributions for any reason". No RimWorld precedent of an item shipping a program zip was verified. Unsigned Go binaries draw antivirus false positives, which leads to reports.
- **The publishing account is an update channel** for every pulling client; use Steam Guard, and consider the signature in §1.4.
- **Unattended SteamCMD must stay anonymous** (§1.3 item 1), or it signs the player's Steam out.
- **Item visibility.** **[C]** Public items download anonymously; private, friends-only, delisted and banned items are not guaranteed (parallel-downloads and 2026-10-24 §3.4), and one item was downloadable anonymously although the public API returned `result=9`. Public versus unlisted is test 2.

## 4. Needs a real test (VM, a private or unlisted test item, the maintainer's own account)

1. Version in a tag: `SetItemTags` accepts `v1.0.2` for app 294100, and `GetPublishedFileDetails` returns it in `tags`.
2. Anonymous `workshop_download_item` of the test item when it is Public, Unlisted and Private, and what the API returns for each.
3. Item with only `About/` plus the zip and its `.sha256`: SteamCMD pulls it, and how long a zip of the real size takes (cold and after a no-change rerun).
4. Deleted content with the ACF kept: does SteamCMD skip the download (§1.3 item 5)?
5. The item subscribed (Monitor plus the zip): Monitor loads in 1.5, 1.6 and 1.7 without errors, RimArcho counts it as Monitor and reports no duplicate next to its own link, and Steam's next item update replaces the zip while the extracted files left by a declined move (§1.6 item 4) survive or not.
6. Creating, updating and the change note through the worker's flat API, including the legal-agreement flag.
7. `hcontent_file` from the Web API against the ACF `manifest`, only if the tag route fails.
8. Defender and SmartScreen on a zip from the Workshop path (Steam downloads carry no Mark-of-the-Web, so SmartScreen likely stays quiet).

## Sources

- Steamworks Workshop implementation, SteamCMD section: https://partner.steamgames.com/doc/features/workshop/implementation
- ISteamUGC reference: https://partner.steamgames.com/doc/api/ISteamUGC
- RimSort SteamCMD guide (anonymous RimWorld Workshop downloads): https://rimsort.github.io/RimSort/user-guide/steamcmd-browser
- RimWorld wiki, distribution and folder structure: https://rimworldwiki.com/wiki/Modding_Tutorials/Distribution, https://rimworldwiki.com/wiki/Modding_Tutorials/Mod_Folder_Structure
- Steam Subscriber Agreement §6.B and online conduct: https://store.steampowered.com/subscriber_agreement/english/, https://store.steampowered.com/online_conduct/
- Generic uploaders running under a game's app id: https://github.com/EliteMasterEric/SteamWorkshopUploader, https://github.com/nozwock/steam-workshop-uploader
- RimArcho: `internal/app/subscribeworkshop.go`, `internal/app/subscribeworkshoprun.go`, `internal/app/gamedl.go` (`SteamCMDAccount`, `SteamCMDSignsOutSteam`), `internal/app/selfupdate.go`, `internal/selfupdate/` (`download.go`, `extract.go`), `internal/app/workshopdetails.go`, `internal/steamworks/`; 17 §5.1; [2026-10-24 login sessions research](2026-10-24_login-sessions-loverslab-steamcmd.md), [2026-10-26 SteamCMD parallel downloads](2026-10-26_steamcmd-parallel-downloads.md)
