# Publishing RimArcho on the Steam Workshop and updating from it (2026-11-05)

ToDo "Auto Update": publish RimArcho on the RimWorld Workshop, upload updates without starting the game, and have RimArcho's own update pull from the Workshop instead of GitHub. This note covers whether that works and how. Nothing was uploaded or tested against Steam yet; **[C]** marks facts confirmed from a source, the code or a live call, **[I]** inferences.

## Summary

- **Uploading without the game is possible**, and RimArcho already has most of the code. The `archo steam-worker` child (`internal/steamworks/`) loads the game's `steam_api64.dll` with `steam_appid.txt` = 294100 and calls `ISteamUGC` through the flat API. Uploading adds one worker action: `CreateItem` (once), then `StartItemUpdate` → `SetItemContent` / `SetItemPreview` / `SetItemTags` / `SetItemTitle` / `SetItemDescription` → `SubmitItemUpdate` with a change note, and `GetItemUpdateProgress`. This is the same client API RimWorld's own "Upload on Steam" button uses; Steam only checks that the account owns RimWorld. The Steam client must be running and logged in. The game does not start, though Steam shows the account as "in RimWorld" while the worker runs.
- **Updating from the Workshop is possible** with existing pieces. RimArcho can subscribe, call `DownloadItem` and watch until the item is installed (`watch.go`), and it reads `timeupdated` from `appworkshop_294100.acf` and `time_updated` from `GetPublishedFileDetails`. The self-update swap (`selfupdate.Apply`, `VersionOf`, rollback, restart) does not care where the files came from; only the GitHub check, download and zip extraction need a Workshop counterpart.
- **GitHub stays.** Players without Steam (GOG, DRM-free) have no Workshop, and the first download for anyone not subscribing needs a zip. The Workshop becomes the preferred source when Steam and a RimWorld license are present.
- **Never run RimArcho from the Workshop folder.** Steam replaces Workshop files one by one (temp file, rename, write), not atomically, and Windows does not let a running exe be overwritten. RimArcho must copy the new files from the Workshop folder into its own program folder and swap there, as it does today.

## Uploading

| Way | How | Verdict |
| --- | --- | --- |
| Steamworks client API from the existing worker | `CreateItem(294100, k_EWorkshopFileTypeCommunity)`, then the update calls above. Read call results as `subscribe` does now. Check `m_bUserNeedsToAcceptWorkshopLegalAgreement`: the item stays hidden until the uploader accepts the Workshop agreement on its page. | **Recommended.** No credentials stored; reuses `worker_windows.go`. |
| SteamCMD `workshop_build_item <vdf>` | VDF keys `appid`, `publishedfileid` (0 creates; SteamCMD writes the new id back), `contentfolder`, `previewfile`, `visibility`, `title`, `description`, `changenote`. Valve calls it "for testing purposes" because it needs the account's credentials. | Fallback only. Logging in with the account the desktop Steam uses replaces that session and kicks Steam offline (seen in the VM), and Steam Guard asks for a code. |
| In-game upload | The game's dev-mode button. | What the ToDo wants to avoid. |

**[C]** Upload refusals to handle: `k_EResultAccessDenied` (no RimWorld license), `k_EResultInsufficientPrivilege` (community ban), `k_EResultBanned`, `k_EResultServiceReadOnly` (recent password or email change, usually 5 days). Limited Steam accounts cannot upload. The preview must be under 1 MB; tags cannot contain `,`; `SetItemContent` takes an absolute, unzipped folder. No documented content size limit; about 80 MB is in the normal range for RimWorld items.

**[C]** go-steamworks has no `ISteamUGC`, and Steamworks.NET or Facepunch would mean shipping a C# helper; neither is needed given the worker. RimSort has no uploader (it uses SteamworksPy for subscribe and download only).

**Where it goes:** `scripts/release.ps1` must not publish (07 §5). A separate step, for example `archo workshop publish <folder> --item <id> --note <text>` or `scripts/publish-workshop.ps1`, uploads the release folder after the GitHub release, with the item id kept in the repository (`About/PublishedFileId.txt`).

## What the item contains

**[I]** Make the item a valid RimWorld mod: the Archo Monitor mod (`About/`, `1.5/`, `1.6/`, `1.7/`, `LoadFolders.xml`) plus the program in a folder the game ignores:

```
About/About.xml, Preview.png, PublishedFileId.txt
1.5/ 1.6/ 1.7/            Archo Monitor assemblies
RimArcho/                  archo.exe, RimArcho.exe, README*, CHANGELOG.md, LICENSE, skills/, release.sha256
```

Tags `Mod` and the supported versions (`1.5`, `1.6`, `1.7`), as Harmony's item has. One subscription then brings both the program and the Monitor, and the Workshop page is the place players already look for mods.

Consequences for RimArcho:
- `FindMonitor` (`internal/app/monitor.go`) must accept the Workshop copy, and the plan logic that adds the Monitor link must not also add `_Archo_monitor_*` when the Workshop copy with the same packageId is enabled (a duplicate packageId is a load error).
- An item without `About.xml` was not tried; the game would most likely list it as a broken mod. Not an option.

## Updating from the Workshop

1. **Check.** If the player subscribes to the item: compare the ACF `timeupdated` / `manifest` for the item with the running version. With the Web API (`GetPublishedFileDetails`, no key, already used in `workshopdetails.go`), compare `time_updated` to see a new upload before Steam has downloaded it. The version itself is read from `RimArcho/release.sha256` or from `archo.exe version` in the Workshop folder. Release notes: the item's change notes are only on the HTML page `steamcommunity.com/sharedfiles/filedetails/changelog/<id>`, so the notification keeps linking the GitHub release or the CHANGELOG in the item.
2. **Download.** **[C]** Steam is only guaranteed to update Workshop items before the game launches (and on "verify integrity"); updates while the game is closed are common but not promised. RimArcho calls `DownloadItem` through the worker and waits with the existing `watch` (installed flag, no pending download, non-empty folder).
3. **Verify.** **[C]** Steam checks transfers against per-file SHA-1 in its manifests, but nothing checks the files after they land, and anyone with the publishing account can push an update. To keep today's guarantee (07 §5.1: a zip whose SHA-256 matches the release), ship `release.sha256` listing each file's SHA-256 and check it before swapping. A signature (an ed25519 key whose public half is built into RimArcho) would also protect against a stolen Steam account; the GitHub path has the same gap today.
4. **Stage and swap.** Copy `RimArcho/` from the Workshop folder into `.archo-update/new-<version>/` (same volume as the program, as `Apply` needs renames), run the same file checks `Extract` runs (allowed names, sizes, both programs present), then reuse `Apply`, `VersionOf`, rollback and restart unchanged.
5. **Source choice.** Workshop when Steam is running, the account owns RimWorld and the player subscribes to the item; GitHub otherwise, or when the Workshop step fails. Settings › update source: automatic / Workshop / GitHub.

**First install from the Workshop:** the player subscribes and finds `RimArcho.exe` in the Workshop folder. Run from there, it should copy itself to a program folder (for example `%LOCALAPPDATA%\Programs\RimArcho`), start that copy and exit, so the running program is never a file Steam replaces. Uninstall then also offers to unsubscribe.

## Risks

- **Executables on the Workshop.** **[C]** No rule bans them, but Valve may "restrict or remove Workshop contributions for any reason", and conduct rules ban malware. No RimWorld precedent of an item shipping an exe was verified. Unsigned Go binaries draw antivirus false positives, which leads to reports. The GitHub path must keep working if the item is taken down.
- **Account.** The publishing account becomes an update channel for every subscriber; use Steam Guard, and consider the signature above.
- **Players see "in RimWorld"** while the worker runs (checks, downloads, uploads). Already true for subscriptions today.
- **Non-atomic Steam updates** can leave the Workshop folder half-written; the hash check in step 3 covers that.
- **1.0.0 and 1.0.1 installs** only know GitHub; they keep updating from GitHub until a version with the Workshop source is installed.

## Needs a real test (VM, a private or unlisted item, the user's own account)

1. Create, update and change note through the worker's flat API, including the legal-agreement flag.
2. `workshop_build_item` with app 294100, if a fallback is wanted.
3. Whether Steam updates a subscribed item while RimWorld is closed, and how long `DownloadItem` takes.
4. Steam updating the item while a copy of `RimArcho.exe` runs elsewhere (should be unaffected) and while one runs from the Workshop folder (to know what the first-install path must avoid).
5. The game's mod list with the item enabled: the `RimArcho/` folder ignored, Monitor loads, and the duplicate-packageId case with the linked Monitor.
6. Defender and SmartScreen on Steam-delivered exes (Steam downloads carry no Mark-of-the-Web, so SmartScreen likely stays quiet).
7. Whether `hcontent_file` from the Web API equals the ACF `manifest`, so a pending update can be detected before download.

## Sources

- Steamworks Workshop implementation, SteamCMD section: https://partner.steamgames.com/doc/features/workshop/implementation
- ISteamUGC reference: https://partner.steamgames.com/doc/api/ISteamUGC
- RimWorld wiki, distribution and folder structure: https://rimworldwiki.com/wiki/Modding_Tutorials/Distribution, https://rimworldwiki.com/wiki/Modding_Tutorials/Mod_Folder_Structure
- Steam Subscriber Agreement §6.B and online conduct: https://store.steampowered.com/subscriber_agreement/english/, https://store.steampowered.com/online_conduct/
- Generic uploaders running under a game's app id: https://github.com/EliteMasterEric/SteamWorkshopUploader, https://github.com/nozwock/steam-workshop-uploader
- RimArcho: `internal/steamworks/` (worker, `watch.go`), `internal/app/workshopdetails.go`, `internal/scan/acf.go`, `internal/selfupdate/`, `internal/app/monitor.go`; [2026-10-24 login sessions research](2026-10-24_login-sessions-loverslab-steamcmd.md)
