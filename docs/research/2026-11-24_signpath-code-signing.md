# Free code signing through SignPath Foundation

> **Status (2026-11-24):** the repository side is ready and inert: CI builds the Windows ZIP
> (`windows-release`, 17 §5), the signing step is skipped until the variables below exist, and
> README has the "Code signing policy" and "What goes over the network" sections. Nothing has been
> applied for. This note is the maintainer's checklist; once signing works, the setup facts move
> into 17 §5 and this file is removed.

Why: a player could not start `RimArcho.exe` at all ("no permission even as admin"). New unsigned
downloads with no reputation are blocked by Smart App Control and by some antivirus programs;
SmartScreen's "Run anyway" does not exist under Smart App Control. A paid certificate is out of
reach. SignPath Foundation signs open-source projects for free with its own certificate (the
publisher shown is "SignPath Foundation"), on condition that it signs only what CI built from the
public source and that each release is approved by hand.

Sources: [terms](https://signpath.org/terms.html), [GitHub trusted build system](https://docs.signpath.io/trusted-build-systems/github),
[the action](https://github.com/SignPath/github-action-submit-signing-request) (v3.2, commit
`a2c5b60d`), [artifact configuration](https://docs.signpath.io/artifact-configuration), read
2026-11-24.

## 1. What is in the repository

| Piece | Where |
| --- | --- |
| Windows release build in CI, the same `release.ps1` as locally: `-StageOnly`, sign, `-PackageOnly` | `.github/workflows/ci.yml`, job `windows-release` (only on `workflow_dispatch` with `version`) |
| Signing step, skipped while the variable `SIGNPATH_ORGANIZATION_ID` is unset | same job, "Sign with SignPath" |
| Artifact configuration to paste into SignPath | `scripts/signpath/artifact-configuration.xml` |
| Release flow that takes CI's ZIP | `scripts/release.ps1 -FromCI` (17 §5) |
| Project page wording | README.md and README.zh-CN.md, "Code signing policy"; release notes get a footer link (`publish-github.ps1`) |
| Privacy policy | README.md, "What goes over the network" |

CI's ZIP equals a local build of the same commit (checked 2026-11-24, run 45360840217 against a
local `release.ps1` build of commit `ccef58bc43f3`, version `1.1.3-signtest1`): the three Monitor
DLLs, docs and skills were byte-identical; the executables differed only by Go's VCS stamp (a
module pseudo-version taken from the nearest git tag: none in CI's shallow clone). `release.ps1`
now builds with `-buildvcs=false`. Re-check, run 45361915100 against a local build of commit
`86d859d5e7ba`, version `1.1.3-signtest2`, same UTC day: all 27 files byte-identical, same ZIP
entry names and the same ZIP size (65,106,756 bytes; only the entries' timestamps differ, so the
ZIP's own hash does). `archo version`: `archo 1.1.3-signtest2 (86d859d5e7ba, 2026-11-24)` for both. `release.ps1 -FromCI` on that commit took the run's ZIP and `rimarcho`, checked them and built the Linux tarball; `publish-github.ps1 -DryRun` accepted the result. Signing itself is untested until SignPath is set up.

## 2. What you do once

You must do these yourself: they need your accounts, and Claude never applies, signs in or handles
tokens.

1. **Two-factor authentication on GitHub** for `ToumanLin` (Settings › Password and
   authentication), if not on already. SignPath's terms require MFA for everyone on the team, on
   GitHub and on SignPath.
2. **Apply** at <https://signpath.org/apply>. The form is a HubSpot form that could not be read
   without a browser session, so its exact fields are unknown; have these ready:
   - Project: RimArcho, <https://github.com/ToumanLin/RimArcho>, MIT license (OSI-approved, no
     dual licensing).
   - Description: a RimWorld launcher and Mod manager for Windows (and experimental Linux): sorts
     and checks Mod lists, launches the game, manages Workshop and SteamCMD Mods, saves and logs;
     a CLI, TUI and MCP server for AI agents (`archo.exe`) and a window (`RimArcho.exe`).
   - Download page: <https://github.com/ToumanLin/RimArcho/releases> (also a Lanzou mirror for
     players in China, same ZIP).
   - Released: yes, since 0.5.0; actively maintained (releases every few days in November 2026).
   - Build system: GitHub Actions on GitHub-hosted runners; artifact: a ZIP with two Go
     executables and a .NET Framework / .NET 10 DLL (a RimWorld Mod, Archo Monitor).
   - Code signing policy: <https://github.com/ToumanLin/RimArcho#code-signing-policy>; privacy:
     <https://github.com/ToumanLin/RimArcho#what-goes-over-the-network>.
   - Team: one person, `ToumanLin`, as author, reviewer and approver.
3. **When accepted**, SignPath sets up an organization and a project for you. On
   <https://app.signpath.io>:
   - Turn on MFA for your SignPath account.
   - Note the **organization ID** (a GUID, in the URL and under Settings).
   - Project: note its **slug** (likely `RimArcho`). Link the trusted build system
     **GitHub.com** to it, if SignPath has not.
   - Artifact configuration: make the project's **default** configuration the contents of
     `scripts/signpath/artifact-configuration.xml` (CI passes no configuration slug, so the
     default is used). It requires the parameters `version` and `monitorVersion`, which CI sends.
   - Signing policies: OSS projects get `test-signing` and `release-signing`. Make yourself the
     approver of `release-signing`. Note both **slugs**.
   - Create an API token for a user with the **submitter** role on the project (a CI user if
     SignPath offers one, otherwise your own token).
4. **Repository settings** (Settings › Secrets and variables › Actions), or from a terminal:

   ```powershell
   gh variable set SIGNPATH_ORGANIZATION_ID --repo ToumanLin/RimArcho --body <organization id>
   ```

   ```powershell
   gh variable set SIGNPATH_PROJECT_SLUG --repo ToumanLin/RimArcho --body RimArcho
   ```

   ```powershell
   gh variable set SIGNPATH_SIGNING_POLICY_SLUG --repo ToumanLin/RimArcho --body test-signing
   ```

   ```powershell
   gh secret set SIGNPATH_API_TOKEN --repo ToumanLin/RimArcho
   ```

   (`gh secret set` asks for the token; paste it there, never into a file or a chat.) Setting
   `SIGNPATH_ORGANIZATION_ID` is what turns signing on.
5. **Trial with test signing**: dispatch a prerelease build and check the result.

   ```powershell
   gh workflow run ci.yml --repo ToumanLin/RimArcho --ref dev/1.1 -f version=1.1.3-rc1
   ```

   The job's "Use the signed files" step prints each file's signature and fails unless all five
   are `Valid` (a test certificate may show `UnknownError` instead, because it is not trusted;
   then switch to step 6 before judging). If SignPath rejects the metadata (product name or
   version), see §4.
6. Switch to release signing: `gh variable set SIGNPATH_SIGNING_POLICY_SLUG --body release-signing`.

## 3. What you do for each release

1. Push the release commit, then `gh workflow run ci.yml --ref <branch> -f version=X.Y.Z`.
2. SignPath emails you a signing request. **Approve it on signpath.io** (the approval click the
   terms require for every release). The job waits up to an hour; an unapproved request fails the
   job, and you dispatch again.
3. When the whole run is green: `pwsh scripts/release.ps1 -Version X.Y.Z -FromCI`. It downloads
   the signed ZIP and the Linux GUI, checks the checksum, version and commit, prints the
   signatures, builds the Linux tarball and prints the publishing steps.
4. Publish as before (17 §5 steps 7–10). Lanzou gets the same signed ZIP; the Workshop's Monitor
   is taken from the signed staging folder.

If CI or SignPath is down, `release.ps1 -Version X.Y.Z` still builds an unsigned release locally.

## 4. Terms checked against what RimArcho does

None rules RimArcho out, but three need your judgement before applying.

1. **Privacy, installation-time display.** "Software that collects user data and transfers it to
   systems not specified by the user must a) describe this behavior in a privacy policy, b)
   display this policy during installation c) include installation options to disable these
   functions." README's "What goes over the network" covers (a). RimArcho has no installer; the
   window's automatic checks are on by default and send the Workshop IDs of SteamCMD Mods to the
   Steam Web API and, with a connected account, the LoversLab session to LoversLab. Most of that
   goes to systems the player chose (their Steam Workshop Mods, their LoversLab account), but the
   first-run guide shows no privacy notice and offers no switch for the automatic checks, so (b)
   and (c) are not met literally. Smallest fix: a first-run step (or a line on its last page) that
   links the network list and has the "automatically check for updates" switch. Not done here.
2. **No hacking tools.** "Software must not include features designed to identify or exploit
   security vulnerabilities or circumvent security measures of their execution environment."
   RimArcho has none of those. Closest: the Lanzou resolver answers Lanzou's `acw_sc__v2` bot
   check with a fixed permutation and XOR (`internal/lanzou/network.go`, "Page JavaScript is never
   evaluated") and sends a browser user agent, as the RimCrow manager does; it never automates
   Lanzou's human-verification page. That is a remote site's bot check, not the execution
   environment's security, and README discloses it. Decompiling Mods (`archo source`, with the
   player's own ilspycmd) and Archo Monitor's Harmony patches of the game's logging are ordinary
   Mod-development tools.
3. **Metadata.** "Set all product name attributes to your project's name." and "Set all product
   version attributes to the same value in each build." The executables carry ProductName
   `RimArcho` and the release version; Monitor's DLL now carries Product `RimArcho` (it was
   `ArchoMonitor`) and its own version, `monitor/VERSION`, which differs from RimArcho's. The
   artifact configuration enforces each. If SignPath wants one version for every file, either drop
   the DLLs from the configuration or stamp the release version into Monitor's FileVersion; ask
   them first.

Also checked, no conflict: the license (MIT) and every component is open source; the ZIP bundles
no third-party binary (RimWorld's reference assemblies, `Krafs.Rimworld.Ref`, and Harmony are
compile-time only; `steam_api64.dll` is copied from the player's own game at run time; SteamCMD and
todds are downloaded on the player's request from Valve and GitHub). Self-update replaces the
program only after the player confirms, with the next signed release; the terms say nothing about
updaters. Archo Agent is not in 1.1 builds (`buildinfo.Agent` is empty); if it ships, its model
calls go to the endpoint the player sets, which is a system the user specified, and its bundle
download needs a line in the network list. RimArcho changes no system configuration (the registry
is only read), and uninstall exists (`archo uninstall`), as the terms require for software with
installation steps.

## 5. Open

- Monitor's DLLs match only with the same .NET SDK feature band (CI pins `10.0.4xx`; the local SDK
  was 10.0.401).
- The network list's gaps that have no switch (followed collections, the open feature-request
  list, Lanzou ignoring the proxy, go-git ignoring the proxy) and the first-run notice in §4.1 are
  product decisions, not part of this setup.
