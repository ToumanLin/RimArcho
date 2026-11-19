# UI reduction: less to read, fewer words to learn

> Status: proposal for review (2026-11-19, second version). Research only; nothing here is implemented. Decisions for the user are in §9.
> Input: player feedback 2026-11-18: the UI is still a bit much, closely related entries sit far apart, and the order should follow how the app is used. The user reads this as **too much text and too high a learning cost**, not too many elements, so this version measures words, jargon and getting-started paths. The first version (element counts) is in git history at `0cdf8221`; its cuts are kept here only where they also cut reading or concepts (§8). §7 adds the user's second principle: **visuals over text, with the text hidden but never deleted**, because the AI agent is the second user.
> Chinese rewrites: [2026-11-19_ui-text.zh-CN.md](2026-11-19_ui-text.zh-CN.md), keyed by the same IDs (T1…). Companion note: [2026-11-18_ux-scenarios.md](2026-11-18_ux-scenarios.md).

## 1. Method

- **Harness**: the dev GUI (`spikes/guidev`, sandboxed: offline, fixture Steam root, analysis and saves index in process, background scans by a sandboxed `archo`) on a synthetic `internal/testkit` home: 160 Mods named by `testkit.ModNames`, 121 enabled, 2 lists, 1 pack, 8 saves in 3 colonies, 7 Mod settings files, a Player.log with 5 error groups, 5 Mods updated since the last launch, and a 1.7 game folder outside the Steam library. Four servers: populated and first-run homes, each in en and zh. Dark, 1280×800. Screenshots and per-screen text dumps: `D:\RimArcho-test\ui-audit\v2\{en,zh,walk}`; scripts in `D:\RimArcho-test\ui-audit\tools`.
- **Words at a glance**: every visible text node inside the 1280×800 viewport and not covered by another layer, grouped by block. **UI words** are what RimArcho itself says (headings, sub-lines, buttons, chips, empty states, notes, confirmations). **Data** (Mod and list names, paths, sizes, dates, IDs, log lines, descriptions) and key caps are counted apart: the player came to read data. zh is counted in characters and converted to word-equivalents at 1.6 characters per word (plus Latin words), so the two columns compare. The sidebar (26–38 words) and the status line are the same on every page and are left out of page totals.
- **Paths**: the six tasks of the UX scenarios note, clicked through with the mouse in one browser session per path. A path's **score** = clicks + UI words read ÷ 25 + 3 × new concepts + 10 × dead ends. Lower is better. The weights are a judgement call: a new concept costs about as much as reading 75 words, and a dead end about as much as three concepts.
- **Not verified**: anything that needs the network or a native file dialog (Workshop download, SteamCMD, folder pickers); zh walks (the zh path numbers use each page's zh/en ratio); light theme; ja beyond the locale file. The "From another manager" card listed a RimPy entry the fixture does not contain; the import survey probably read the host's own profile (read only). Not traced; it does not change any count here by more than a few words.

## 2. Findings in one paragraph

Pages are not long, but they **explain themselves in full sentences that the player reads every time**: Can it run puts 235 UI words in front of the player, Saves 183, All mods 168, Import & export 135, Add a subscription 112. Most of it is in four kinds of sentence: the page explaining its purpose (forbidden by 16 §7.3 rule 7 but still present on six pages), where a finding came from ("declared by the mod  About.xml supportedVersions"), how a feature works ("the PNGs stay; RimArcho writes a .dds next to each…"), and a key hint line on every page. Separately, a new player meets **about 25 RimArcho words in the first hour**. Only two are explained where they appear, and four collide with plain words: "Game Config" next to the game's own Config folder, and "lock", "stage" and "scan / rescan / check the files again / read the mods" for overlapping ideas. The worst moments are dead ends where the GUI hands the player a **CLI command** ("use archo env set --launch direct", "(archo monitor install)").

## 3. Words per page

UI words at a glance, sorted by en total (UI + hint line). Caps = key caps in the page body.

| Page | en UI | en hints | Caps | zh UI (characters) | zh hints | Data words (en) | After §6 rewrites (en UI + hints, est.; §7 visuals cut more) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Can it run | 235 | 9 | 2 | 250 (295) | 10 | 13 | ~110 |
| Saves | 183 | 18 | 3 | 198 (268) | 20 | 34 | ~110 |
| All mods (and the editor's details) | 168 | 17 | 3 | 201 (237) | 19 | 146 | ~110 |
| Import & export | 135 | 4 | 0 | 131 (180) | 5 | 9 | ~85 |
| Updates | 112 | 18 | 4 | 104 (130) | 19 | 17 | ~70 |
| Add a subscription | 112 | 13 | 0 | 104 (136) | 12 | 12 | ~25 |
| Game log | 116 | 10 | 0 | 139 (171) | 11 | 170 | ~70 |
| Background tasks | 106 | 12 | 4 | 108 (159) | 12 | 13 | ~70 |
| Feedback | 102 | 9 | 0 | 108 (75) | 10 | 4 | ~100 (leave) |
| All lists | 84 | 17 | 4 | 105 (105) | 20 | 20 | ~60 |
| Mod settings | 75 | 23 | 0 | 78 (95) | 22 | 49 | ~50 |
| Settings (first screen) | 85 | 9 | 0 | 103 (139) | 10 | 5 | ~65 |
| Texture optimisation | 88 | 9 | 0 | 90 (118) | 10 | 21 | ~45 |
| Play review | 83 | 18 | 1 | 100 (137) | 20 | 10 | ~45 |
| List editor | 64 | 21 | 7 | 79 (111) | 26 | 205 | ~45 |
| Game Config | 54 | 9 | 0 | 63 (87) | 10 | 30 | ~45 |
| Pack page | 39 | 20 | 1 | 45 (59) | 21 | 136 | ~30 |
| Launch | 35 | 0 | 1 | 46 (52) | 0 | 18 | ~30 |
| Packs | 15 | 13 | 0 | 18 (25) | 14 | 7 | ~15 |
| Search files | 15 | 6 | 3 | 17 (20) | 7 | 3 | ~12 |

Two patterns:

- **zh is not shorter than en**. In word-equivalents zh is 5–25% longer on most pages: zh translations add particles and keep every clause. Where zh *is* plainer, it is because the translator rewrote rather than translated (Can it run's order warning: en has 20 words with "loadBefore" in them, zh says in 12 characters that the wrong order can stop a dependency or part of the content working and that sort fixes it). §5 takes those zh rewrites back into en.
- **The hint line is 9–26 words on every page except Launch**, more than the page's own lead. It is the single largest item a mouse player reads and never uses (§8, H1).

## 4. Jargon

Words a player has to learn that are RimArcho's own, or a modder's, not a RimWorld player's. "First met" is the first screen on the first-run path (§5, P1) or, if not on it, the page. "Explained" means the UI says what it is where the word appears, not in the glossary.

| Term (en / zh / ja) | First met | Explained? | Problem | Proposal |
| --- | --- | --- | --- | --- |
| Game Config / `游戏配置` / `ゲーム構成` | Launch "01 Game Config", sidebar; the guide never says it (its field is just "name") | No | The core unit of the app, named after a settings file. Collides with the game's Config folder ("open the Config folder" on Mod settings) and with "copy the settings of main" when adding one | Call it **game** in the sidebar and Launch ("01 game"), and keep "Game Config" only on its own page with one line: "a game folder with its own saves, settings and current list". zh `游戏`, ja `ゲーム` (decision D2) |
| managed folders / `托管目录` | Guide step 1, the very first screen | Partly ("keep SteamCMD, downloaded Mods and tools") | First word a new player reads is an internal concept, explained with another one (SteamCMD) | Skip the step: use the default and say where in Settings (D5). If kept: "Where should RimArcho keep its downloads?" |
| SteamCMD | Guide step 1; Add a subscription; Add Game Config › download a version | Once ("Valve's SteamCMD") on Add Game Config only | Shown before the player needs it | Say "Valve's download tool (SteamCMD)" on first use, nowhere on the first screen |
| list (UI) / plan (CLI, MCP, docs) | Guide step 3 "first list" | Implicitly | Consistent in the GUI. "plan" never leaks into the GUI | Keep |
| pack / `整合包` / `MODパック` | Launch "02 list or pack"; first-run import "A pack (with Mod files)" | Only in the Packs empty state (27 words) | en "pack" reads as a Mod pack or a texture pack; zh `整合包` and ja `MODパック` are familiar words | en **modpack**? Not recommended (packs are not the community's modpacks either). Keep "pack", explain once in the "list or pack" tooltip and the Packs page lead: "a frozen copy of a list, with its own saves" (T26) |
| Staging / `暂存` / `一時保留` | Editor, third column "3 Staging 0" | No (key hint "t stage" only) | Unclear what it holds; visible even when empty | Keep the word, hide the column while empty, and give its empty state one line: "Mods set aside, with their old place kept" |
| lock / lock this version / lock library / `锁定` | All mods details, Updates "lock this one", Settings › folders | No | Sounds like "freeze the list" or "disable" | **keep this version** / `保留这个版本`, and hide the lock library row in Settings until something is kept (D3) |
| scan, rescan, check the files again, read the mods, rescan the mod folders | Guide step 3, editor, All mods, Game Config, Can it run | No | Five phrasings for two actions (re-read Mod folders; re-check the list) | Two verbs: **rescan** (folders) and **check again** (the list's problems) (D3) |
| Archo ID (#plgh) | Editor and All mods details | No | Developer identifier (16 §7 principle 6) | Move to technical information |
| package (packageId) / `包名` | Details | No | Modder's term, but players searching for compatibility use it | Keep in details, below the plainer fields |
| lineage, instance, hash | Export preview (raw JSON: "lineage", "instance", "hash"); game log "instance" | No | Internal model leaking into a preview the player reads | Hide the raw preview behind "show the file" (T25) |
| functional / not functional / `功能型` / `非功能型` | Play review | en avoids the word; zh uses it | zh shows the jargon en avoided | zh rewrite in the zh-CN note (T14) |
| Archo Monitor | Game log notice; Settings › game log; Mod settings empty state | One clause, plus a **CLI command** "(archo monitor install)" | Dead end for a GUI player | "in-game helper (Archo Monitor)" on first mention; an **install** button instead of the command (T15) |
| error group, unknown source, patched methods, Harmony, Def, Patch, xref | Can it run, Game log | No | Modder vocabulary in the player's crash path | Plain first line ("502 errors in the game's code, cause not found"); the modder terms stay in the log text (T1, T16) |
| strong requirement / loadBefore | Can it run | Inline, in more jargon | — | Take over zh's wording (T2) |
| declared by the mod, About.xml supportedVersions, your rule, inferred, OS memory snapshot | Can it run evidence lines | No | Provenance for experts, read by everyone | On hover or focus (T4) |
| share code, Rentry, .rml, RimCrow / RimSort / RimPy formats, .archolist | Import & export, Export | Partly | Eight export formats, all at the same level | Two groups: "for a friend" (share code, RimArcho list) and "for another manager" (the rest) (T25, D7) |
| ModsConfig.xml, About.xml, Player.log | First-run import ("its ModsConfig.xml enables 121 Mods"), All lists, Can it run | No | File names as UI words | "the game's current list (121 Mods)"; file names in tooltips |
| todds, DDS | Texture optimisation | Partly | — | "the texture converter (todds)" once |
| record (take off the record) | Background tasks | — | zh is plain (`清除已完成`), en is not | en "clear finished" (T12) |
| follow a collection / `跟着` vs `关联` | All lists | No | zh shows two words on one card: the heading says `跟着 Steam 合集`, the button says `关联` | zh: one word (`跟着` per the glossary) on both |
| number N (details meta line) | Details | No | Load position, unexplained | Remove (the list shows it) |

The **glossary** (`docs/glossary.md`) covers most of these, but its UI column is the only place several are defined. It does not mention "managed folders", "check the files again" or "worth a look", and it has `跟着合集` where the zh UI's button says `关联`. **ja** follows en term for term, including `ゲーム構成` and `非機能型`, so every en rename needs a ja change too.

## 5. Getting-started paths

| # | Task | Clicks | UI words read (en / zh est.) | New concepts | Dead ends | Score |
| --- | --- | --- | --- | --- | --- | --- |
| P1 | First install to playing | 5 | ~260 / ~300 | 9 | 1 | **53** |
| P4 | The game crashed | 3 | ~390 / ~430 | 7 | 1 | **49** |
| P3 | Adding a Mod | 7 | ~345 / ~370 | 6 | 1 (not verified online) | **49** |
| P6 | Trying 1.7 | 6+ | ~300 / ~340 | 7 | 1 | **49** |
| P5 | Sharing with a friend | 4 | ~260 / ~300 | 6 | 0 | **32** |
| P2 | Using the game's current order (later, not first run) | 3 | ~240 / ~260 | 3 | 0 | **22** |

### P1. First install to playing (score 53)

Guide step 1 "managed folders" → "use this folder" → step 2 "find the game", "add and continue" → step 3 "read the mods", "next" → Import & export in first-list mode, "use this list" → Launch, "launch the game".

- Good: Import & export now hosts the first-list step (A1 of the UX note is in), and "The game's current list" is first and filled.
- Concepts met: managed folders, SteamCMD, a nameless "name" field that is a Game Config, scan, list, pack, share code / Rentry, another manager (RimPy), ModsConfig.xml, Game Config on Launch, "problems break the game".
- Words: the guide's lead restates its own step bar on every step (19 words, three times). Step 3 shows 99 words of import choices; four of its five rows are for players who are not new.
- Dead end: with Steam not found, launch fails with "Steam not found / use archo env set --launch direct": a CLI command, and `launch.go` returns it untranslated, so zh and ja players read English. (The harness has no Steam client; a DRM-free install hits the same refusal.)
- Smaller: the step bar disappears on the first-list step; "1 problems break the game" (plural bug).

Fixes, by how much they cut: launch refusal with a "start the game directly" button (−10, T18); default the managed folder and drop step 1 (−1 click, −2 concepts, −44 words: −9); remove the guide lead (−57 words: −2); first-list step shows only "the game's current list" and "something else…" that opens the full page (−80 words, −3 concepts: −12). After: ~20.

### P4. The game crashed (score 49)

Launch "last game  2 errors →" → **Can it run** (not the game log) → "open game log" on an error group → Game log.

- 235 words on Can it run before the player reaches the errors in the right column. The left column ("before you start") is about the *next* launch; the player came about the *last* one.
- Top error group "unknown source — 502 errors, 1 warning / errors in patched methods ×501; exceptions ×1; 1 other messages": no cause and no action beyond reading the log.
- Game log: "its errors are in needs attention" (old page name), the Monitor notice ends in a CLI command, the analysis card adds 60 words of statistics.
- Concepts: error group, unknown source, patched methods / Harmony, strong requirement / loadBefore, About.xml supportedVersions, Archo Monitor, inferred.

Fixes: "last game  N errors" opens the Game log on its first error, and Can it run's right column leads with "what to try" (−1 click, −235 words read: −10); the Monitor notice gets an install button (−10); plain first lines for error groups (T16) and evidence on hover (T4) (−2 concepts, −60 words: −8). After: ~21.

### P3. Adding a Mod (score 49, partly unverified)

Sidebar "mod library" (opens All mods, 168 words) → "add a subscription" (112 words) → paste a Workshop link → Enter → then, to play it: lists → the list → find it in Disabled → space.

- The subscription page explains three source kinds in three cards before the field does anything; the field already recognises them.
- After subscribing, nothing says the Mod is not in any list yet. The harness is offline, so the success state was not seen; the spec (16 §2.9) describes no "enable in <list>" step.
- The offline failure line reads "completed 0, failed 1: download was not confirmed, ctrl+j" (a key in a sentence).
- Concepts: subscription vs download, SteamCMD, GitHub / LoversLab sources, scan, Disabled / Enabled.

Fixes: subscription cards to one line (−85 words: −3, T8); "add to <current list>" offered when the download finishes (−3 clicks, −1 dead end: −13, D4); "mod library" opens on the page the player used last (−168 words read: −7). After: ~26.

### P6. Trying 1.7 (score 49)

Game Config → "Add Game Config" (95 words) → "a game you have" lists only installs found in Steam libraries; the fixture's 1.7 folder outside them needs "add this folder" (native picker, not walked) — or "download a version" → SteamCMD first ("size unknown") → (network, not walked) → then the list must allow 1.7: All lists › game versions, another page.

- Good: Add Game Config's "Mod list" defaults to "the game's current list (121 Mods)" (§4.3 of the UX note is in).
- Concepts: Game Config, SteamCMD, "start with: copy the settings of main / game defaults", saves folder (own / shared), a Mod list per Game Config, game versions per list, version warnings.
- Dead end: making a list run on 1.7 is on a different page with no link from the done step.

Fixes: the done step offers "let <list> run on 1.7" (UX note §4.3) (−2 clicks, −1 dead end: −12); lead and the "copies game settings…" line to tooltips (−28 words: −1); "Game Config" → "game" (−1 concept: −3). After: ~33.

### P5. Sharing with a friend (score 32)

lists → all lists → "export…" → 8 formats, the RimArcho list preselected, the file's raw JSON as preview → "save as a file…" or "copy".

- The preview shows `"lineage"`, `"instance"`, `"hash"` to the player.
- Eight formats at one level; the right one for a friend on RimArcho or RimCrow is the share code, fifth in the list.
- The pack route (with Mod files) is 6+ clicks through Packs and is not mentioned on the export page.

Fixes: two groups, "for a friend" first (share code, RimArcho list, pack) (−3 concepts at first sight: −9); preview folded behind "show the file" (−2 concepts: −6). After: ~17.

### P2. Using the game's current order later (score 22)

Import & export → "The game's current list  read it" → preview → "import".

- When the game's list is the same as the current list, the preview still defaults to "import as a new list" named "ModsConfig" (a file name), with "the list already is this" under the other two choices.

Fix: when identical, say "default already matches the game" and offer nothing to import; otherwise default the new list's name to the Game Config's name (−1 concept, avoids a duplicate list). After: ~15.

## 6. Rewrites

Exact new text. The zh column is in the zh-CN note under the same ID. "Hover" means the text leaves the visible page but stays on it: it becomes the tooltip of the element it explains, shown on hover **and keyboard focus**, and that element's accessible description, because 16 §11 A10 fails any fact that lives only in a `title`, hover or colour (§7.1). "When" means it is shown only in that state. The view-model text (`Need.Text`, `Need.Why` and the rest) is unchanged, so the TUI, CLI and MCP keep every word. Word counts are en.

| ID | Page | Today (en) | Words | New (en) | Words |
| --- | --- | --- | --- | --- | --- |
| T1 | Can it run, lead | "default is not safe to play yet: 2 to handle before you start, 2 error groups in the last run" | 20 | "default: 2 to fix before you play, 2 errors last time" | 11 |
| T2 | Can it run, order item | "a strong requirement (a dependency or loadBefore): breaking it goes with more errors in shared logs; sorting puts it right" | 20 | "In the wrong order this Mod may not work; sort fixes it." | 12 |
| T3 | Can it run, version item | "they may still work, but their authors have not said they do on this version" | 15 | "They may still work; their authors have not marked 1.6." | 10 |
| T4 | Can it run, every item | "declared by the mod  About.xml supportedVersions", "your rule  a load-order rule you added", "inferred  file estimates and OS memory snapshot" | 5–7 each | hover, on the item's ⓘ | 0 |
| T5 | Can it run, column heads | "found by checking the mods' files and shared logs", "seen in the game's log" | 13 | remove (16 §7.3 rule 7) | 0 |
| T6 | Can it run, memory (superseded by V1) | "RAM: loading needs 1.9 GB–2.4 GB; available now 26.8 GB of 63.1 GB / VRAM: estimated 675.8 MB; dedicated capacity 23.6 GB / GPU: …" | 31 | "Memory: needs 1.9–2.4 GB, 26.8 GB free." The full card only when it does not fit | 7 |
| T7 | Updates, lead | "First what waits for you to update, then what Steam already updated." | 12 | remove | 0 |
| T8 | Add a subscription | lead (19) + three cards (17 + 23 + 10) + local line (17) | 86 | field placeholder "paste a Workshop, GitHub or LoversLab link"; one line "Local Mods: copy them into the Mods folder. [open it]" | 17 |
| T9 | Mod settings, values | "a value missing from the file is at its default", "on/off, numbers and text change here while the game is closed; the file is backed up first" | 28 | hover on the values heading | 0 |
| T10 | Textures, footer | "not counted yet: “count the textures” reads every PNG to show what is there and what converting saves" | 18 | "Count first to see what converting saves." | 7 |
| T11 | Textures, scale card | "the PNGs stay; RimArcho writes a .dds next to each and can take them all back" | 16 | "You can undo it: the PNGs stay." | 7 |
| T12 | Tasks | "take the done ones off the record"; "one of each kind at a time; different kinds can run together" | 19 | "clear finished"; the queue sentence on hover | 2 |
| T13 | Saves, categories | "research, recipes, appearance and the like reset and the colony goes on; or only Mods that are safe to remove need them"; "nothing of theirs is in the save, as far as the save can tell" | 36 | "The colony survives; some progress resets."; "Not used in this save." | 10 |
| T14 | Play review | lead "what your Mods add and how much of it your saves show: 1 colony, 3 saves"; "121 Mods add nothing a save can show (interface, performance, fixes): saves cannot tell whether they were used" | 34 | lead "1 colony, 3 saves"; "121 interface, performance and fix Mods: a save can't show them" | 15 |
| T15 | Game log, Monitor | "Archo Monitor is not installed: errors will not say which file or def was loading (archo monitor install)" | 18 | "Install the in-game helper to see which file caused an error. [install]" | 12 |
| T16 | Game log, lead and groups | "its errors are in needs attention"; group titles "harmony-stack", "xref-missing", "patch-failed" | 6 + chips | remove the lead; titles "error in the game's code", "a missing item", "a patch that failed" | — |
| T17 | Game log, analysis | "after changing 1–3 mods it went away in 53% of 255 repeat uploads (13% without any change)" | 18 | behind "how sure is this?" | 0 |
| T18 | Launch refusal | "Steam not found / use archo env set --launch direct" (English in every language) | 8 | "Steam isn't running or installed. [start the game directly]", translated | 8 |
| T19 | Guide, lead | "choose RimArcho's folder, find the game, read the mods once, then pick where your first list comes from" | 19 | remove (the step bar says it) | 0 |
| T20 | Guide, step 1 | "Where should RimArcho keep SteamCMD, downloaded Mods and tools?" | 10 | skip the step (D5); if kept: "Where should RimArcho keep its downloads?" | 7 |
| T21 | Guide, step 3 | "RimArcho reads every mod folder once; later scans only read what changed" | 12 | "Reading your Mods…" | 3 |
| T22 | Add Game Config | lead "choose the game, then how this Game Config starts: its settings, Mod list and saves folder"; "copies game settings and Mod settings; no Mod files or saves are copied" | 28 | remove the lead; the second line on hover of "start with" | 0 |
| T23 | Import & export rows | "a Steam collection or Rentry link, a share code, or a copied list"; "a data folder or file kept elsewhere; it is only read" | 24 | remove (the field's placeholder and the button say it) | 0 |
| T24 | Import preview, identical list | default "import as a new list" named "ModsConfig"; "the list already is this" ×2 | 12 | "default already matches the game." and no import button | 5 |
| T25 | Export | "each mod with its version: whoever imports it sees which of theirs match"; "include Mod settings  by package and settings class; the receiver chooses whether to apply them"; raw JSON preview | 28 + preview | "for RimArcho users"; "include Mod settings (they choose to apply them)"; preview behind "show the file" | 11 |
| T26 | Packs, empty | "no packs yet: a pack fixes the current list — every mod as it is now, without the files of other game versions — and plays with its own saves" | 27 | "No packs yet. A pack is a frozen copy of a list, with its own saves." | 15 |
| T27 | Settings | "Checks RimArcho, mods, external tools and community data while RimArcho is open."; "see what a new player sees; nothing you choose there is saved" | 24 | remove the first; "nothing you choose is saved" | 5 |
| T28 | Editor details | automatic note "subscribed on the Workshop, first seen on Oct 1 / by RimArcho, click to edit"; "number 5"; "technical information, enter" | 20 | only a note the player wrote; remove "number"; keep one "full details" link | 3 |
| T29 | Launch, problem line | "▲ 1 problems break the game" | 5 | "▲ 1 problem breaks the game" (plural) | 5 |
| T30 | Status, failed download | "completed 0, failed 1: download was not confirmed, ctrl+j" | 9 | "The download didn't finish. [see why]" | 5 |

## 7. Visuals for numbers and states

The user's principle: what a glance can read beats what has to be read, but the AI agent is the second user, so **text is hidden, never deleted**. The user's own example is Can it run › RAM / VRAM:

```
today (31 words)
  RAM: loading needs 2.3 GB–3.3 GB; available now 29.3 GB of 63.1 GB
  VRAM: estimated 1.9 GB–1.9 GB; dedicated capacity 23.6 GB
  GPU: NVIDIA GeForce RTX 4090 (by the system's preference)

proposed (4 words)
  RAM   [██████████|▓▓▓▒▒|··························]  64 GB
  VRAM  [██|▓▓|·········································]  24 GB
         in use     need   free
```

### 7.1 Where the hidden text goes

Every visual below keeps its words in four places. Nothing is deleted, only taken off the visible page:

1. **The view model** (16 §1). The GUI and TUI receive the same structs; the sentences stay in them. Where a visual needs numbers, they are already there or need one field (table below). Nothing in `internal/view` is removed.
2. **The accessibility tree** (16 §11 A10: "never only in a `title`, hover, color or image"). The visual is a `role="img"` or `role="meter"` / `progressbar` element whose accessible name is the full sentence, such as `aria-label="RAM: loading needs 2.3–3.3 GB; available now 29.3 GB of 63.1 GB"`. An agent reading UIA gets exactly today's text. Meters also expose `aria-valuenow` / `aria-valuemax`.
3. **Hover and keyboard focus**: the same sentence as a tooltip. The visual is focusable when it carries a number that is not printed beside it.
4. **CLI and MCP** (13, 14): unchanged. They print from `app` and `cmds`, not from the GUI (`plan cost`, MCP `estimate_memory`, `archo log`, `saves show`, `loadout import` previews), so no output changes.
5. The **TUI** (16 §9) keeps its text lines. Block bars (`▇▇░`) are an option for later, not part of this proposal; a terminal row is already one line per fact (16 §7 principle 8).

| Visual | Numbers it needs | In the view model today? |
| --- | --- | --- |
| V1 memory bars | total, available, need low / high, VRAM capacity and estimate | Yes: `Needs.Memory` is the whole `app.MemoryAssessment` (`needs.go`), next to `Need.Text`; All lists has the same assessment |
| V2 run summary | counted items, error groups | Yes: the counts behind `NeedsLead` |
| V3 error-group bars | count per kind per group | Partly: the kinds and counts are in the group's text ("×501; exceptions ×1"); add a `[]{kind, n}` field |
| V4 log cluster bars, likelihood meter | cluster count; pattern percentage and sample size | Yes: cluster `Count`; the pattern's numbers are formatted into the sentence, add them as fields |
| V5 import bar and diff marks | installed / missing; added / removed / moved | Partly: the preview has the lists; the replace / merge diff is a sentence, add counts |
| V6 update marks | changed Defs, Patches, files per Mod; risk level | Yes (`modchanges` page rows) |
| V7 texture before / after | PNG count and size, DDS size after | Yes (count result) |
| V8 size bars | size per row | Yes (table cells) |
| V10 disk meter | free and total bytes | Free only; add the total |

### 7.2 Proposals

Ranked with the text cuts in §7.4. Sketches use `█` for what exists or is used, `▓` for the low estimate, `▒` for the range up to the high estimate, `·` for free, `▲` `✕` `✓` `○` for the status icons RimArcho already draws.

**V1 Memory: one stacked bar per resource** (Can it run, worth knowing; All lists' memory block; the launch dialog's RAM warning).

```
RAM   [███████████|▓▓▓▒▒|·························]  64 GB
VRAM  [██|▓▓|········································]  24 GB
```

In use = total − available (grey); the need is the accent, the range part hatched (`▒`) so low and high read without colour; free is the track. Over budget, the need runs past the end into red, and the item moves to breaks the game as today. The GPU line moves into the VRAM bar's text. All lists' three lines (Vanilla and DLC / Mods / budget) become the same bar split into "game" and "Mods" segments. Hidden text: `Need.Text` as each bar's accessible name and tooltip, the GPU and the "saves are not counted" note in the item's details. Saves ~27 words on Can it run, ~30 on All lists.

**V2 Run summary instead of the lead sentence** (Can it run; the Launch problem line).

```
default   ▲ 2 to fix before you play    ✕ 2 errors last time
```

Two status icons with numbers; T1's sentence is the line's accessible name. Saves ~5 words beyond T1; the counts match the section headings, which already pulse on change (16 §7.1).

**V3 Error groups as count bars** (Can it run, last run).

```
unknown source   [████████████████████████████▌]  502
Add Cube         [▏]                                 1
```

A bar per group, scaled to the largest, the count at the end; within the bar the kinds (patched methods / exceptions / other) are shades, named in the tooltip. Today: "unknown source — 502 errors, 1 warning / errors in patched methods ×501; exceptions ×1; 1 other messages" plus "log  Player.log  Nov 18 21:51" per group. Hidden text: the group's `Text` as the row's name; file and time once in the section head. Saves ~15 words per group.

**V4 Game log: cluster counts and likelihood** (Game log).

```
✕ harmony-stack  ████████████████████  ×500
✕ patch-failed   ▏                      ×1
try first: disable the mod that errors   [■■■■■□□□□□] 53%
```

The ×count becomes a bar beside it; "after changing 1–3 mods it went away in 53% of 255 repeat uploads (13% without any change)" becomes a 10-step meter, the sentence its name (T17 already folds it). Severity "medium" becomes a three-step meter. Saves ~20 words.

**V5 Import preview: one bar and diff marks** (Import & export preview, the first-list step).

```
121 Mods  [███████████████████████████████████░░]  ✓ 121  ✕ 0
replace default:   +7   −12   ↕31
```

Installed / not installed as a two-part bar; the destination's effect as the + − ↕ marks Saves already uses for Mod changes. Today: "RimSort list: 212 mods, 205 installed, 7 not installed" and "7 added  12 removed  31 moved  212 enabled after it". Saves ~12 words per destination.

**V6 Updates: change marks instead of chips and sentences** (Mod library › Updates).

```
Add Cube        ● Defs +1
HugsLib         ● Defs +1  Patches ~2
sources   ✓ Workshop   ✓ local   checked 21:52
```

One risk dot (amber for "worth a look", none when harmless) instead of the identical "worth a look" chip on every row (six of six rows today); counts by kind as marks. The sources' "not checked yet / checked" lines become status icons. Saves ~25 words.

**V7 Texture optimisation: before and after** (after "count the textures").

```
now     [██████████████████████████████]  1.8 GB  · 4,210 PNG
after   [█████████▌]                         0.6 GB
encoder ✓ todds 0.4.1
```

Replaces the result sentences and the encoder card's three lines (T10, T11 already shorten them). Saves ~25 words.

**V8 Size and use as in-cell bars** (All mods size, pack page size, Packs size; "used in 3 lists").

```
Combat Extended     ███████████▌ 412 MB   ●●●
HugsLib             ▏           1.6 KB    ●●●
```

A faint bar behind the size, scaled to the largest row; used-in as one dot per list (named in the tooltip). The numbers stay printed. Saves 0–3 words per row, gains "where is my space" at a glance.

**V9 Saves: drop the words the bar already shows** (All saves, right card). "What this save needs" already has `.sv-nbar`; the two category descriptions repeat it (T13). The colony timeline (`.sv-chart`) and Mod changes' + − ↕ marks stay as they are. Saves ~36 words with T13.

**V10 Guide: disk meter** (first-run step 1, if kept; Settings › folders).

```
D:\…\RimArcho   [██████████████████········]  440 GB free of 1.8 TB
```

Small; matters on a nearly full drive, where the warning turns the free part amber.

**V11 Tasks and Launch: icons for states that are words today.** Task rows "done" / "failed" next to ✓ / ✕ drop the word (the icon's name keeps it); Launch's "last game  Nov 18  2 errors" becomes `last game  Nov 18  ✕ 2`. Saves ~2 words per row.

Not proposed: the Game versions control on All lists (already a segmented picture of versions), the editor's unsaved dot, sidebar badges, Play review's seen meters, the saves timeline, progress bars. These are the house style the proposals reuse.

### 7.3 Existing visuals to reuse, and the rules that bind them

| Existing | Where | Style to reuse |
| --- | --- | --- |
| `.prog` | guide, exchange, pack build, uninstall, tasks | 6px track (`--pill`), accent fill, `--r-sm`, width transition at `--t-layout` |
| `.sv-nbar` | Saves, what this save needs | 8px segmented bar, 2px gaps, `--r-sm`; the model for V1, V3, V5 |
| `.rv-meter` | Play review seen meters | 4px, 56 / 120px wide; the model for V4's meter and V8's in-cell bars |
| `.sv-chart` | Saves, over time | SVG line, `--line` ticks; the model for any later sparkline |
| status icons ✓ ✕ ▲ ○ | Can it run, tasks, Launch | 16 §7 icon set (Nerd and compat forms) |
| + − ↕ marks | Saves' Mod changes | green +, red −, muted ↕ |

16 §7 rules that constrain the visuals:

- **One accent, plus success green, error red, warning amber** (principle 2). The estimate or the thing being looked at is the accent; existing use is muted grey; free space is the track. No new hues.
- **Most text is grey; only the operated object is bright** (principle 3): the bar's numbers are secondary text, never bold.
- **No left vertical lines or colour bars** (principle 4, §7.3 rule 6, TestStyleRules): every proposal is horizontal; nothing marks a row with a strip.
- **Hairlines only, two radii** (§7.3 rules 1, 3): bars are fills on a track with `--r-sm`, no borders; TestStyleRules already enforces this.
- **Four font sizes, minimum 12px** (rule 4): the number at a bar's end is 12px.
- **Keep the interface still** (principle 7, §7.1): bars may grow to a new value with the 250ms layout motion and nothing else; no shimmer except existing progress.
- **Not by colour alone** (A10): low and high differ by hatch, severity by icon shape, so the bar reads in either theme and for colour-blind players.

### 7.4 Ranking: text cuts and visuals together

By en words removed from the visible page and how much faster the page reads. "Clarity" is a judgement: **high** when the visual answers the page's question without reading (does it fit, how bad is it, what changed), **medium** when it speeds a comparison, **low** when it only trims.

| Rank | Item | Pages | Words saved (en, visible) | Clarity | Kind |
| --- | --- | --- | --- | --- | --- |
| 1 | H1 hint line in keyboard mode | 18 of 20 | 9–26 per page | medium | rule |
| 2 | V1 memory bars (with T6) | Can it run, All lists, launch warning | ~57 | high | visual |
| 3 | T8 subscription cards to one line | Add a subscription | ~69 | medium | text |
| 4 | V3 error-group bars + T4 evidence on demand | Can it run | ~45 | high | visual + text |
| 5 | T13 + V9 save categories | Saves | ~36 | medium | text, reuse |
| 6 | V6 update marks + T7 | Updates | ~37 | high | visual + text |
| 7 | V4 log bars and likelihood meter + T15–T17 | Game log | ~45 | high | visual + text |
| 8 | T19–T22 guide and Add Game Config sentences | first run, P6 | ~60 over the path | medium | text |
| 9 | V5 import bar and diff marks + T23, T24 | Import & export | ~35 | high | visual + text |
| 10 | V7 texture before / after + T10, T11 | Textures | ~45 | high | visual + text |
| 11 | T25 export groups and folded preview | Export | ~20 visible, the whole JSON | medium | text |
| 12 | T14 play review lead and note | Play review | ~19 | low | text |
| 13 | V2 run summary (with T1) | Can it run, Launch | ~14 | medium | visual |
| 14 | T26–T28 empty states, settings, details | several | ~45 total | low | text |
| 15 | V8 in-cell size and use bars | All mods, packs | 0–3 per row | medium | visual |
| 16 | V11 state icons | Tasks, Launch | ~2 per row | low | visual |
| 17 | V10 disk meter | guide, Settings | ~3 | low | visual |

Can it run, the user's example page, goes from 235 visible UI words to about 70 with V1–V3, T1–T5.

## 8. What stays from the first version

Kept, because they also cut reading or concepts:

- **H1 hint line only in keyboard mode** (16 §3.1 rule 4, §2.2). The largest single block of words on 18 of 20 pages (9–26 words). A spec change.
- **Key caps on buttons on hover/focus**, except launch `L` (16 §7.3 rule 13; §2.9, §2.16 "shows its key"). Caps are short, but each one is a symbol to decode; the editor shows 7 in the body and 9 in the hint line.
- **Explanatory lines** removed or moved to hover (16 §7.3 rule 7 already requires it): now T5, T7, T9, T12, T19, T22, T23, T27.
- **Archo ID, number and the automatic note out of compact details** (T28): jargon and repeated facts.
- **Evidence on demand** in Can it run (T4).
- **RAM / VRAM as one line** (T6).
- **One "open the log" per run** in Can it run, not one per error group.
- **Empty states and cards hidden while empty**: Staging, Play review's details and "in the saves" cards, the tasks "every item" card, "update all" at 0.

Dropped or marked, because they trade discoverability for fewer elements and do not cut reading:

| First-version cut | Status |
| --- | --- |
| Editor head: rename, delete, import/export into `⋯` | **Dropped**: the labels are one word each, and a menu hides them |
| Saves: five of six save buttons into `⋯` | **Dropped**, same reason. Instead shorten "recycle…" and keep the row |
| All lists: rename, duplicate, delete into `⋯` | **Dropped** |
| Lock, resubscribe, unsubscribe moved to full details | **Trades discoverability**; only if D3 renames lock |
| Copy buttons on hover | **Neutral for reading**; optional, not recommended now |
| Area numbers in keyboard mode only | **Neutral for reading**; goes with H1 if H1 is chosen |
| Settings folder rows as one menu each | **Trades discoverability**; instead hide rows "not used yet" (cuts words) |
| Mod settings: three views behind "files ▾" | **Dropped** |

## 9. Decisions for the user

1. **D1: Rewrites T1–T30**: apply all, en and zh (and ja to match en). *Recommended*: yes; they need no spec change except where the spec quotes the old text, and they cut 30–60% of the words on the five heaviest pages.
2. **D2: "Game Config" in navigation and Launch**: rename to "game" (`游戏`, `ゲーム`) outside its own page, or keep it everywhere with one explaining line. *Recommended*: rename. It is the app's central idea, first met unexplained, and its name collides with the game's Config folder. **Outcome (2026-11-19):** the rename shipped (f5e95520) and was reversed the same day: the player had already decided on "Game Config", so it is kept everywhere, with the Game Config page's one explaining line (the second option).
3. **D3: Verbs**: "keep this version" for lock, and only "rescan" and "check again" for the five scan phrasings. *Recommended*: yes, with the glossary updated in the same change.
4. **D4: After a download**: offer "add to <current list>" when a subscribed Mod finishes downloading. *Recommended*: yes; it closes the adding-a-Mod dead end. Needs a 16 §2.9 addition.
5. **D5: First run**: skip the managed-folder step (use the default, changeable in Settings), and make the first-list step show "the game's current list" plus one "something else…". *Recommended*: yes; it removes the first two jargon words a new player meets.
6. **D6: Crash path**: "last game  N errors" opens the Game log at its first error instead of Can it run. *Recommended*: yes. Can it run is about the next launch.
7. **D7: Export**: group formats as "for a friend" and "for another manager", preview folded. *Recommended*: yes.
8. **D8: Hint line**: keyboard mode only (H1), at most 4 groups (H2), or as is. *Recommended*: H1. It needs 16 §3.1 rule 4 and §2.2 changed.
9. **D9: No CLI commands in GUI text**: replace the two found (launch refusal, Monitor install) with buttons, and add a static check that no `archo ` command text reaches a GUI string. *Recommended*: yes; the check belongs in 17 §4's static checks.

10. **D10: Adopt "visual first, text on demand" as a 16 §7 principle.** Proposed wording, as principle 9 after "Each TUI row occupies one line":

    > **9. Show quantities, not sentences about them; keep the sentence underneath.** A count, proportion, progress, state, comparison or change that can be read at a glance is drawn: a bar, stacked bar, meter, status icon, + − ↕ mark or small timeline, in the styles that exist (`.prog`, `.sv-nbar`, `.rv-meter`, `.sv-chart`, the §7 icons). The words are never deleted. The view model keeps the full sentence (§1), the visual carries it as its accessible name or description (§11 A10) and as a tooltip on hover and keyboard focus, and CLI and MCP output (13, 14) is unchanged. A visual does not carry a fact by colour alone: a hatch, an icon's shape or a printed number carries it too. Reasons, actions and names stay text. The TUI keeps text unless its row has room for a block bar.

    *Recommended*: yes, together with V1 as the first application (the user's own example), then V3–V7 page by page. A static check could follow: a `role="meter"` / `"img"` / `"progressbar"` element must have a nonempty accessible name.

Suggested order: D9, D1 and D10 with V1 first (text, plus the memory bars), then D6, D4, D5 (small flow changes), then V3–V7 under D10, then D2, D3 and D8 (renames and spec changes).
