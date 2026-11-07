# RimArcho top-level design (v2)

> Status: in force. 1.0 answers the first of the three questions (§2); the others are planned for later versions (§5).
> This document covers only what RimArcho is, who it serves and which questions it answers, in players' words. Technical details (domain model, module layers, Mod identity and change tracking) are in [00a-architecture](00a-architecture.md). Feature specifications are 01–19 in [specs/](specs/README.md).

## 1. Positioning

**RimArcho is a one-stop launcher that understands RimWorld players.**

Pick Mods, sort them, check whether they can run, launch the game and find out why something broke, all in one place. Players need not switch between a manager, logs, Workshop pages and forums.

"Understands players" means two things: it knows the origin and changes of every Mod copy you have, without guessing; every conclusion comes with evidence.

## 2. Three questions to answer

| # | Player's question | How to answer it | Version |
| --- | --- | --- | --- |
| 1 | **What am I playing, and will it run?** | Plans (enabled lists) and sorting, Mod origins and changes, missing dependencies and conflicts, Patch and Harmony analysis, log attribution, packs, launching | 1.0 |
| 2 | **What haven't I played yet?** | Read saves to find installed Mods and content untouched across multiple games (12 §3) | 1.1 |
| 3 | **What's worth playing?** | Packs shared between players; an AI assistant recommends content and play goals based on what you have not played | Longer term |

**Through 1.0, RimArcho answers the first question.**

The questions build on each other: the first ensures the game can run; the answer to the second feeds the third. First find what has not been played, then choose what is worth playing.

The third question also asks something different of the AI assistant: for the first two, AI follows players' instructions (fix sorting, translate, analyze errors); for the third, AI must understand how players play and what they want to play. In other words, **from taking instructions to understanding players**.

The details of the third question (a sharing website and the data needed for recommendations) will be decided later.

## 3. Users

### 3.1 Players

The primary users, especially those with hundreds of Mods.

- **Serve users of pirated copies too.** RimArcho does not require a Steam account or game ownership to manage local games and Mods. Offer anonymous options wherever possible for actions needing authorization; explain actions that truly require an account and provide a usable local path.
- **Every feature must work well without AI.**
- Players need not understand Mod internals. Interfaces and prompts use players' words; technical details are available in details when needed.

### 3.2 AI assistant: Archo

**A second kind of user, designed as a user**, rather than an attachment to player features. When designing each feature, also ask what an AI assistant needs to use it.

- **Name and appearance**: the built-in AI assistant is called **Archo**, represented by the project icon (`internal/icon/portrait.svg`); the bow at the top left of the portrait is the mark for places too small for the portrait (16 §7). Use this name and portrait wherever the interface mentions the assistant, for example "ask Archo to analyze this error". External agents connecting through MCP use their own names.

- **Two ways to connect, with the same capabilities**: RimArcho's built-in AI workbench (on by default, can be disabled); external agents (Claude Code, Codex, etc.) through `archo mcp`.
- **What it needs**: every object has a name it can reference directly; every conclusion has evidence; every player action has a corresponding tool; every tool states whether it is read-only or changes things.
- **The same rules as players**: changes enter undo history; writing game files requires player confirmation; get player consent before first sending each task category's content (descriptions, logs, source snippets) to a model (15 §3).

## 4. Principles

1. **Always know where you are, where you can go and how to return.** Avoid complicated paths; complete tasks in as few steps as possible.
2. **Every conclusion tells you why.** State whether evidence comes from a Mod's declaration, inference, logs or community data. Label inferences.
3. **Undo mistakes at any time.** Every edit enters history, for comparison and rollback.
4. **Respect your files.** Write game files only on request (save the enabled list, copy locally, create links for SteamCMD and extra-folder Mods), with a preview and backup first.
5. **Players and AI assistants use the same capabilities.** The window, terminal UI, CLI and MCP use the same code; players and AI receive the same facts.
6. **Leave your computer free.** No resident background process; exit when heavy work finishes.

See 00a §3 for the corresponding technical principles.

## 5. Version roadmap

| Version | Question answered | Main content |
| --- | --- | --- |
| 1.0 | What am I playing, and will it run | Plans (multiple per environment), launching, packs, locked versions, can it run, log attribution and live tracking, import/export, sidebar UI and first-run guide |
| 1.1 | What haven't I played yet | Untouched content, Archo; save reading and save management are implemented ahead of 1.1, in 1.0.5 |
| 1.2 | What haven't I played yet (continued) | One-click Workshop collection subscription and unsubscription, playthrough themes and content conflicts |
| Longer term | What's worth playing | Player-shared packs, content and play-goal recommendations that understand the player |

See 01 for the details and status of each item.

## 6. Decision record

Record only conclusions still in force. Do not renumber; missing numbers are retired decisions.

| # | Decision | Conclusion |
| --- | --- | --- |
| D2 | Primary users | Players. AI adoption among players is still low; every feature must work well without AI. See D12 for the role of AI assistants |
| D3 | Interface | GUI first, with sidebar pages organized around the three questions; TUI is the keyboard version of the list editor. Both share view models and render separately (16) |
| D4 | Data relationship with RimCrow | Runs independently; supports one-time import of aliases, groups (as tags), user rules and lists (07 §6) |
| D5 | Assembly analysis | Read .NET metadata natively in Go for Harmony patches and type references; invoke ilspycmd on demand to view source (05 §5.3, §5.4) |
| D6 | AI write permissions | External agents use MCP; their harness decides whether writes are allowed. Built-in Archo uses existing actions and undo history; writing game files, deleting and backing up still require player confirmation (15 §2) |
| D8 | Mod identity and change tracking | RimArcho maintains identity and reconciles each scan; ask players to confirm unmatched cases (00a §4, 04). Keep tombstones for 90 days by default, configurable |
| D9 | GUI technology | Wails v3 + native HTML/CSS/JS, no framework or build step; does not render the TUI into a window (16 §8) |
| D10 | Version scope | See §5 and 01 §1 |
| D11 | Positioning | A one-stop launcher that understands RimWorld players and answers three questions: what am I playing and will it run; what haven't I played yet; what's worth playing. 1.0 answers the first |
| D12 | AI assistants' role | Design them as a second kind of user, not an attachment to player features |
| D13 | AI assistant's name | The built-in AI assistant is Archo, represented by `internal/icon/portrait.svg`; the portrait's bow is the small-size mark (16 §7) |
| D14 | Plans and packs | An uncompiled enabled list is a **plan** (a "list" in the interface), following Workshop updates; a compiled fixed artifact is a **pack**, with every Mod version locked and its own saves (07, 10) |
| D15 | Multiple plans per environment | One environment can have multiple plans. Plans share the game's default saves, with no option for separate saves; build a pack for isolation (07 §1) |
| D16 | Launch model | Only one game may run at a time. Write a plan to ModsConfig.xml only when launching it, so multiple environments can share the game's default user data directory (09 §1) |
| D17 | Locked versions | Lock individual Mods: copy to a new Instance in the same Lineage, unaffected by Workshop updates (07 §5) |
| D18 | Save management | Save analysis is read-only. Save management may rename and copy saves when the player asks, and back them up or move them to the recycle bin after player confirmation (12 §4). The Saves page belongs to the Library, not Play review: saves are managed like plans and packs, and checking whether a save still loads is needed while playing, not only in review (16 §2.11) |
| D19 | Subscribing and unsubscribing | Steam handles subscriptions; for unsubscription, request it from Steam, then have RimArcho move the local folder to the recycle bin instead of waiting for Steam's slow cleanup (1.2) |
| D20 | Page width | By page shape: multi-column pages (lists with details, tables) fill the window, with draggable column widths like the editor; single-column pages share a 960px reading width. Filling all pages would force forms into several columns (16 §7.3 item 12) |
| D21 | Archo Monitor updates and running games | When a check replaces an old Monitor in a plan with a new version, do not wait for the game to exit or swap again at launch; changing only the Monitor version does not make the plan changed: this run's logs still count (06 §4), and the old version's `Mods/` link does not compete with the new version before the next write (09 §3). Rationale: in the VM, the first scan finished 27 seconds after launch; the new version had not been scanned at launch and could not be swapped then. Waiting for game exit would likewise make the just-ended run stale. Monitor only records; it does not change how other Mods load, so its version does not change which plan the logs describe (19 §6) |
| D22 | Shape of can it run | GUI has two columns: the left contains findings from file checks and shared logs (breaks the game, worth knowing, for you to confirm); the right contains observations from game logs. All actions for each item are a bottom row of buttons, without `⋯`; completed items stay in place with undo (16 §2.4, §5). "worth knowing" is what RimArcho learned from shared logs: a core feature, in its own section, not faded, before identity questions. Hidden or locked Mods and outdated converted textures do not affect whether the plan can run; they belong in the Mod library and Texture optimization pages. Rationale: the page had five kinds of sources, and each item's content was scattered across rows, `⋯` menus, expansions and popovers. Split into "predicted" and "actually happened" columns: the right updates live while the game runs; the left stays still. Candidates and comparison: `docs/design/canitrun-drafts/` |
| D23 | MCP progressive disclosure | On MCP connection, agents read only short Instructions and one-to-three-sentence tool descriptions; action details, examples, diagnostic workflows and the error pattern catalog live in the read-only `guide` tool (CLI `archo guide`), fetched as needed (14 §3, which also sets their size budget). By default `archo mcp` serves only the core group of tools, the rest with `--tools all` (counts: 14 §1). Rationale: with everything in the descriptions, a connection read 8816 characters of Instructions plus 44509 characters of tool descriptions (about 13k tokens), and `tools/list` with parameter schemas was 81 KB, mostly unused in a session; in a c3 pilot the full tool list was about 29% of an agent's tokens. Use a tool rather than an MCP resource: all clients support tools, resource support varies, and agents rarely read resources on their own. Making it a CLI command keeps "every MCP tool is a CLI command, with identical output" (13 §1.1); evaluation scripts and the TUI command panel can also use it |
| D24 | TUI has no bundled terminal | TUI runs in the player's terminal; no WezTerm download or launch, bundled fonts or image drawing (16 §9.2, §9.3). Rationale: bundling a terminal adds about 66 MB of downloads and a resident process (about 91 MB idle), plus configuration generation, font packaging and OSC 1337 drawing/clearing to maintain. Icons and thumbnails are only a bonus; GUI handles images and large previews better. The `:` command line runs every command directly, like CLI (16 §9.4) |
| D25 | RimArcho self-updates | Checks join automatic maintenance (`self` category, same timer and interval levels); installation requires player confirmation (02 §1). Three player requirements: **channel follows the running version**: stable sees only stable; prerelease also sees newer prereleases, without a setting; **show a line in Settings › Updates under "RimArcho" and at the top right of the launch page**, with one quiet notification on discovery and no dialog (16 §3.1 rule 7); **if other RimArcho processes are open, download and verify first, then wait for all to close before replacement**, without refusing or forcing: a new version may migrate the database and break older versions still using it; asking players to close windows and come back to click adds a step. Replace by renaming the running exe, which Windows allows; roll back if the new program cannot report its new version. Leave the data directory alone; back up the database yourself before downgrading (17 §5.1) |