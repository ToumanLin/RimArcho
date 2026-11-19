# UI reduction: what each page can lose

> Status: proposal for review (2026-11-19). Research only; nothing here is implemented. Decisions for the player are in §5.
> Input: player feedback 2026-11-18, "the UI still has a bit too much" (after the sidebar was consolidated the same day: Texture optimisation under Mod library, Feedback in the sidebar foot, Saves as its own group, Import & export last in Library). Companion note: [2026-11-18_ux-scenarios.md](2026-11-18_ux-scenarios.md) (where entries live; this note is about what each page shows).

## 1. Method

- **Screenshots**: the dev GUI (`spikes/guidev`, sandboxed like the jumpy harness: offline, fixture Steam root, analysis and saves index in process, scan children run by a sandboxed `archo`) on a synthetic home built with `internal/testkit`: 160 Mods with `testkit.ModNames`, 121 enabled, 2 lists, 1 pack, 8 saves in 3 colonies, 7 Mod settings files, a Player.log with 5 error groups, 5 Mods updated after the last launch. Dark theme, en, 1280×800 (a few full-height extras). Files: `D:\RimArcho-test\ui-audit\NN-<page>.png`; per-page DOM inventories in `inv\`; the fixture generator and harness in `tools\` and `fx\`.
- **Counting**: an *element* is anything the eye has to parse that is not the data itself: a button, link, chip, badge, key cap, hint group, heading number, column header, copy button, section label, explanatory line. A repeated row counts once (its template), plus one per distinct part of the template. The sidebar, breadcrumbs and status line are counted once in §2, not per page. Counts are by hand from the screenshots and are estimates (±10%).
- **Classes**: **C** core for the page's main task; **O** occasional; **R** rare, or duplicated elsewhere; **D** decoration or explanation that the page works without.
- **Not covered**: Search files with results (typing into the field in the harness gave no results; not investigated), Play review with content (the synthetic saves carry no Things, so the map is empty), overlays (command panel, `?`, context menus), light theme, zh.

## 2. Ranking

Impact = how often the page is used × how much it can lose. Global rules come first because they put the same clutter on every page.

| Rank | Page | Before | After | Main cuts |
| --- | --- | --- | --- | --- |
| 1 | Every page (global rules, §3) | 15–25 per page | 3–8 per page | hint line, key caps on buttons, area numbers, copy buttons only on demand |
| 2 | List editor | ~78 | ~48 | head actions into a menu; compact details |
| 3 | Can it run | ~55 | ~30 | evidence footers on demand; one "open game log" |
| 4 | Mod library › All mods (+ details shared with the editor) | ~60 | ~40 | duplicate details header; compact details |
| 5 | All saves | ~60 | ~38 | six save buttons into a menu; one "against list X" |
| 6 | All lists | ~50 | ~28 | rename/duplicate/delete into a menu; versions, memory, collections fold |
| 7 | Settings (whole page) | ~115 | ~60 | folder rows; launch background block folds |
| 8 | Updates | ~40 | ~26 | lead, duplicate chips and actions |
| 9 | Game log | ~30 | ~20 | developer chips, raw text folds |
| 10 | Game Config | ~30 | ~18 | duplicate remove, apply buttons |
| 11 | Add a subscription | ~22 | ~9 | three explainer cards become one line |
| 12 | Texture optimisation | ~24 | ~13 | encoder card becomes one line |
| 13 | Background tasks | ~28 | ~18 | queue sentence, empty items card |
| 14 | Mod settings | ~26 | ~18 | empty column, views, class name |
| 15 | Pack page | ~24 | ~17 | two exports, recycle bin |
| 16 | Play review | ~22 | ~15 | empty cards, two sorts |
| 17 | Search files | ~18 | ~12 | caps in the field, scope rows |
| 18 | Launch | ~33 | ~27 | already lean |
| 19 | Import & export | ~30 | ~25 | redesigned 11-18; small cuts |
| 20 | Feedback, Packs | ~20, ~13 | ~17, ~12 | leave |

## 3. Global: rules that put the same clutter on every page

These are the biggest lever: each one removes 3–10 elements from every page at once. Several are required by the specs as written, so they need a spec decision, not only code.

### 3.1 The hint line is always on (16 §3.1 rule 4, 16 §2.2) — **rule forces clutter**

Every page ends in up to 7 key groups (`↑↓ move  enter details  ctrl+o ctrl+w folder / Workshop page  / search  \ filter  v group by tag  ? all keys`): 7–9 key caps and 7 labels, about 14 elements, on every page, for every player. Mouse players never use it; keyboard players learn the keys in a few sessions and still see it forever. Many of its keys are also on the buttons (editor: `w`, `L`, `S`, `u`, `p`, `R`), so the same key is shown twice on one screen.

- **H1 (recommended)**: show the hint line only in *keyboard mode*: after the player presses a navigation or verb key, until the next mouse click. The status sentence keeps the bar. `?` stays a key and an entry in the command panel. Rule 4 becomes "visible while the keyboard is in use".
- **H2**: keep it always on, but at most 4 groups (the page's 3 most-used actions and `? all keys`). Needs the TestUI limit lowered from 7 to 4.
- **H3**: keep as is.

Per page: −14 (H1, mouse) or −6 (H2).

### 3.2 Key caps on buttons (16 §7.3 rule 13; 16 §2.9 "each button shows its key"; 16 §2.16 "action buttons show their keys") — **rule forces clutter**

Rule 13 only says *how* a shown key looks; the per-page texts say the key *must* be shown. Result: editor head 4 caps, editor list headers 2, Can it run items 2 each, Saves 3, tasks 3, updates 3, All lists 3. Together with §3.1 the same key is usually on screen twice.

- **K1 (recommended)**: a button's cap appears on hover and keyboard focus (and in its tooltip), never at rest, except the page's one solid primary action (launch `L`). In keyboard mode (H1) all caps show.
- **K2**: caps only in the hint line, never on buttons.

Per page: −2 to −6.

### 3.3 Area numbers before every card heading (16 §3.2, 16 §7.3 rule 8)

`1 colonies  2 Mods  3 details  4 in the saves`, `1 Bring in  2 From another manager  3 Share`, settings `1 about … 7 network`. Rule 8 already says numbers exist only because they are focus keys, so they are a keyboard aid shown to everyone.

- **N1 (recommended)**: show heading numbers only in keyboard mode (H1), like Windows access-key underlines. The heading keeps its place; only the digit fades in.

Per page: −2 to −7.

### 3.4 A copy button after every address and identifier (16 §3.1 rule 12) — **rule forces clutter**

Rule 12 requires "a trailing copy button" on every file, folder, URL and copyable ID. Details show 3–4 (Archo ID, package, path, Workshop ID), Settings › folders 8, Game Config 3, Saves 1, Texture optimisation 2, each task row 1, even an example URL on Add a subscription.

- **P1 (recommended)**: the copy button is drawn on hover of its row and on keyboard focus; it stays in the tab order, so rule 12's keyboard path is unchanged. Text stays selectable.

Per page: −1 to −9.

### 3.5 Explanatory sub-lines and provenance chips (16 §7.3 rule 7 already forbids most)

Rule 7 says the page heading "never explains the page's purpose" and filler goes. Current violations:

| Page | Line | Class |
| --- | --- | --- |
| Can it run | "found by checking the mods' files and shared logs", "seen in the game's log" | D |
| Updates | "First what waits for you to update, then what Steam already updated." | D |
| Add a subscription | "Paste an address: RimArcho recognises what it is…" plus three explainer cards | D |
| Game log | "its errors are in needs attention" (also uses the old name of Can it run) | D, stale |
| Texture optimisation | section labels "encoder", "what is there", "what converting does" | D |
| Mod settings | "a value missing from the file is at its default", "on/off, numbers and text change here…" | D → tooltip |
| Background tasks | "queue RimArcho  one of each kind at a time; different kinds can run together" | D → tooltip |
| Saves | category descriptions under "remove with care" / "safe to remove" (two lines each) | O → tooltip |

Proposal: remove the D lines; move the O lines into the tooltip of the thing they explain. No spec change; rule 7 already asks for it.

### 3.6 Sidebar

Today: toggle, 3 group labels, 10 entries, 3 chevrons, 3 foot entries, 2 badges (≈22). It was consolidated on 11-18 and reads well. Two small options, not recommended now: Play review as a view switch inside All saves (−1 entry); Background tasks shown in the foot only while something runs or failed (its progress is already in the status line). Leave for after the per-page cuts.

## 4. Pages

Tables list what changes; elements not listed stay (class C unless noted).

### 4.1 List editor (`05-list-editor.png`, `05b-list-editor-workshop-mod.png`) — ~78 → ~48

The most used page and the most crowded: an 8-button head, a details card that repeats facts, and 9 key caps in the hint line.

| Element | Class | Proposal |
| --- | --- | --- |
| Head: N problems `p` | C | keep (the only way to see this list's problems here) |
| Head: rescan `R` | R (also Mod library, Game Config, saves; scans follow file changes) | remove from the editor; `R` and the command panel keep it |
| Head: rename, delete, import/export | R | one `⋯` menu (rename `F2`, delete, import/export…) — 16 §2.5 says "There is no ⋯ menu"; change that |
| Head: "no changes" (disabled save) | D | show save changes `w` only when the list differs; "1 unsaved" and the save button merge into one "save 1 change" |
| Head: launch `L` | C | keep, the one solid button with its cap |
| Filter placeholder "search: words, tags:, source:, content:  /" | O | "search  /"; the syntax is in the popup already (16 §3.7) |
| Details header "4 details" + "view full details Enter ↵" | O | keep the link, drop "technical information, enter" at the bottom (same page) |
| Auto note card "subscribed on the Workshop, first seen on Oct 1 / by RimArcho, click to edit" | R | show only a note the player wrote; first-seen is in history |
| "number 5" under the name | R (row number in the list) | remove |
| Archo ID row + copy | R (16 §7 principle 6 says #id belongs to technical information; 16 §3.1 rule 12 lists it in details — the specs disagree) | move to full details › technical |
| web page + steam page | O | one "Workshop page" (`ctrl+w`); the other in full details |
| lock this version, resubscribe, unsubscribe `Del` | R | full details and the context menu; `Del` stays |
| Enabled header undo `u` | R (the status line's Undo after each edit) | show only while there is something to undo |
| Disabled header A–Z, individual/tags | O | keep |
| Hint line (7 groups, 9 caps) | see §3.1 | H1 |

### 4.2 Can it run (`02-can-it-run.png`) — ~55 → ~30

| Element | Class | Proposal |
| --- | --- | --- |
| Lead conclusion | C | keep |
| "the files changed since the last check" | O | keep (amber state) |
| check the files again | O | keep |
| head "game log" | R (sidebar, and every right-column item) | remove |
| column sub-lines "found by checking…", "seen in the game's log" | D | remove (§3.5) |
| Item evidence footer: source chip + provenance ("your rule  a load-order rule you added", "declared by the mod  About.xml supportedVersions", "inferred  file estimates…") | O | one muted "why?" link on hover/focus that expands it; 2 elements per item |
| Item's second button (sort `S` beside "move A after B") | O | keep; drop its cap (K1) |
| RAM / VRAM card: 3 lines + refresh | O (also on All lists) | one line "RAM 1.9–2.4 GB of 25 GB free, VRAM 0.7 of 23.6 GB" when it fits; the card only when it does not fit |
| Right column, per error group: "open game log" button + "log" chip + "Player.log" + time | R (same for every item) | say file and time once in the section head; the item itself opens the log; −4 per item |
| Section 3 head "the list  Nov 18 21:11" | O | keep the time, drop "the list" link (the lead names the list) |

### 4.3 Mod library › All mods (`07-library.png`, `07b-library-full.png`) — ~60 → ~40

Shares the details card with the editor, so §4.1's details cuts apply here too.

| Element | Class | Proposal |
| --- | --- | --- |
| Details header subtitle "[sbz] EPOE Retexture / local-main" | R (name and source repeat 100px below) | remove; the header is only "3 details" |
| Auto note card "put in the game's Mods folder, first seen…" | R | as §4.1 |
| `supports 1.6` in the meta line and "supported 1.6" field | R | one of them |
| Table head: expand-all / collapse-all icons | O (only meaningful with multi-copy Lineages) | show only when some Lineage has more than one copy, or move to the header menu (`s`) |
| `v` cap beside mods / tags | R (hint line) | remove (K1) |
| "rescan" page action | O | keep (16 §2.9 makes this its home) |

### 4.4 All saves (`15-saves.png`) — ~60 → ~38

| Element | Class | Proposal |
| --- | --- | --- |
| Save actions: rename `F2`, copy, back up, make a list, check this save, recycle… | 1 C (check), 5 O/R | keep "check this save"; the rest in a `⋯` menu; `F2` and `Del` keep working |
| "against the current list default" (twice) | R | once, in the card's header |
| can it load? box + launch `enter` | C | keep |
| Colony card: rename, check this colony | O / R (rename is also on the save) | keep "check this colony"; rename in the colony row's context menu |
| Category descriptions (2 lines each) | O | tooltip (§3.5) |
| "1 Defs in the save come from no installed Mod  show the names" | O | keep |
| location + copy + open location | O | path on hover-copy (P1); "open location" in `⋯` |

### 4.5 All lists (`04-lists-all.png`) — ~50 → ~28

| Element | Class | Proposal |
| --- | --- | --- |
| "+ new / import" `n` | C | keep (the UX scenarios note proposes routing import to Import & export; that removes the dual form, not this button) |
| rename, duplicate, delete | O | `⋯` menu; edit `enter` stays the one visible button |
| "ModsConfig.xml" line | R | remove (the list's file is a technical fact; it is in export) |
| Game versions: a 7-segment control 1.0–1.7 | O | show the chosen versions as text with "change…"; the segments open on demand |
| Loaded memory: 3 lines + "what is counted" | O (also Can it run) | one line; the rest behind "what is counted" |
| follows Steam collections: heading, "no collection yet", field, follow | R when empty | when the list follows none: one "follow a Steam collection…" link; the section only when it follows one |
| share: pack row, export… `e`, create a pack… `B` | O | keep; caps per K1 |
| hint line 7 groups | see §3.1 | H1 |

### 4.6 Settings (`19-settings.png`, `19b-settings-full.png`) — ~115 → ~60

The longest page. Most rows are needed by someone; the clutter is in two blocks.

| Element | Class | Proposal |
| --- | --- | --- |
| Folders: 8 rows × (path, copy, locate, move to…), plus "not used yet" | O / R | one "change…" menu (locate / move to) per row; copy on hover (P1); folders "not used yet" under "N more folders" |
| source library: "remove unused", "decompile the current list's mods", size | R (agent/expert feature) | into the folder row's menu |
| add a SteamCMD folder, add another mod folder | O | one "add a mod folder…" (the dialog asks which kind) |
| Launch page background block (preview, dim, blur, focus, rotation, two tabs, thumbnails, note) ≈ 14 | O | one row "launch page background  plain  [change…]" that opens the block in place |
| accent colour: 16 swatches + custom + "default" | O | keep (it is the control); option: current swatch + "more" |
| Workshop descriptions, images in descriptions, GPU acceleration, motion | O / R | an "advanced" fold at the end of Interface |
| proxy / GitHub mirror explanations with example URLs and copy buttons | D | tooltip; the example in the placeholder |
| about: "dev  a development build is not updated" | O | keep |

### 4.7 Updates (`08-library-updates.png`) — ~40 → ~26

| Element | Class | Proposal |
| --- | --- | --- |
| Lead "First what waits for you to update, then…" | D | remove (§3.5) |
| head "add a subscription" | R (the sidebar entry right above) | remove |
| update all `U` while "waiting for you to update 0" | D in that state | hide at 0 |
| "worth a look" chip on every row | R when every row has it | show only the rows that differ (e.g. "risky" in amber); "worth a look" becomes the default, unmarked |
| right card "what it means: Defs changed" | R (the row's sub-line) | remove when equal |
| footer "checked 3 sources, the last at…" + head "check for updates" | O / R | one footer line whose refresh icon checks again |

### 4.8 Game log (`03-game-log.png`) — ~30 → ~20

| Element | Class | Proposal |
| --- | --- | --- |
| Lead "its errors are in needs attention" | D, stale name | remove |
| "Archo Monitor is not installed: errors will not say…" | O | one line inside the analysis card, with "install" |
| Kind chips `harmony-stack`, `patch-failed`, `xref-missing` | R (developer terms, 16 §7 principle 6) | remove; the message says what it is; the kind stays in the raw text |
| Card 2 "the log text" (raw `log:E3 harmony-stack x500 first at …`) | O (experts, agents) | folded under the analysis card, one click away |
| Analysis: "common causes", "kinds of fix" under "try first" | O | keep "try first"; the other two under a "more" disclosure |

### 4.9 Game Config (`13-game-config.png`) — ~30 → ~18

| Element | Class | Proposal |
| --- | --- | --- |
| "remove Game Config" twice (list row and last row) | R | keep the one in the list row |
| head "rescan the mod folders" | R (Mod library rescan) | remove |
| name / extra arguments "apply" buttons | D | apply on Enter or leaving the field (as Settings proxy should too) |
| "1 all Game Configs" card with a single config | R | with one config, no list card; the page is that config |
| "copy the game folder…" | O | keep (the UX note's B1 also puts it on Launch) |

### 4.10 Add a subscription (`09-library-subscribe.png`) — ~22 → ~9

The field already recognises GitHub, LoversLab and Workshop input. Three cards with descriptions, example URLs and copy buttons explain what the field does.

- Replace the cards with one line under the field: "a GitHub repository, a LoversLab file page, or a Workshop link or ID"; the account state ("not logged in") appears only after a Workshop or LoversLab address is recognised.
- Remove the copyable example URLs (nobody copies an example).
- Keep the local-mods line and "open the Mods folder".

### 4.11 Texture optimisation (`12-library-textures.png`) — ~24 → ~13

- Encoder card when todds is missing: one line "needs the DDS encoder todds (3.7 MB, MPL-2.0)  [download todds…]"; source URL and target folder behind "details". When installed: one muted line.
- Remove the three section labels (§3.5).
- Keep mods and scale choices and count / convert.

### 4.12 Background tasks (`18-background-tasks.png`) — ~28 → ~18

- The queue sentence goes to the queue name's tooltip (§3.5).
- "3 every item" card hidden when the task has no per-item results (today it repeats the summary's "go to saves" button).
- The search field and filter appear once there are more than ~20 tasks.
- "take the done ones off the record" stays.

### 4.13 Mod settings (`11-library-mod-settings.png`) — ~26 → ~18

- Hide the "settings window" column when every row is `—` (no Archo Monitor data).
- The four views: "this list" visible, the other three behind "files ▾" (they are the clean-up views).
- Settings class (`X.Settings`) moves to a tooltip on the file name; the two explanation lines become the tooltip of the values heading.

### 4.14 Pack page (`06b-pack-detail.png`) — ~24 → ~17

- export and export the list → one "export ▾" (pack file / list only).
- move to the recycle bin → `⋯` (`Del` stays).
- Remove the label "the mods, in load order" (the column says load order).
- Hide "version folders left out" when every row is `—`.

### 4.15 Play review (`16-play-review.png`) — ~22 → ~15

- Cards 3 "details" and 4 "in the saves" are empty until a Mod is picked: draw them only then, and let the Mods card take the width.
- "by progress" and "most content first": one sort control.

### 4.16 Search files (`10-library-search-files.png`) — ~18 → ~12

- `alt+c` / `alt+r` caps inside the field: tooltip of Aa / .* (K1).
- files and mods scope rows: one line "in Defs, Patches, About of default ▾" that expands to the two fields.

### 4.17 Launch (`01-launch.png`) — ~33 → ~27

Already lean (16 §2.3). Small cuts: the `L` cap and the → in the launch block show on hover/focus; the → after each top-right line goes (the line is the link); `01 02 03` follow N1.

### 4.18 Import & export, Feedback, Packs list

Import & export was redesigned on 11-18. Small cuts: "paste" duplicates `ctrl+v` in the field (remove), "read" can be Enter (keep the button, drop nothing else), the manager avatar ("RP") is decoration. Feedback and the Packs list are already minimal.

## 5. Decisions for the player

1. **Hint line**: H1 keyboard mode only (recommended) / H2 always on, at most 4 groups / H3 as is. Needs 16 §3.1 rule 4 and §2.2 changed.
2. **Key caps on buttons**: K1 on hover and focus, primary launch always (recommended) / K2 only in the hint line. Needs 16 §2.9 and §2.16's "shows its key" changed.
3. **Area numbers**: N1 keyboard mode only (recommended) / as is.
4. **Copy buttons**: P1 on hover and focus, still tab-reachable (recommended) / as is. Needs 16 §3.1 rule 12 changed.
5. **List editor head**: rename, delete and import/export into a `⋯` menu, rescan off the editor, save only when there are changes (recommended) / keep 8 buttons. Needs 16 §2.5 ("There is no ⋯ menu") changed.
6. **Compact details** (editor and Mod library): drop Archo ID, number, the automatic note, lock / resubscribe / unsubscribe buttons and the second Workshop link; they stay in full details (recommended). Also settles the 16 §7 principle 6 vs 16 §3.1 rule 12 disagreement about #id.
7. **Can it run**: evidence on demand, one "open game log" per run, RAM as one line (recommended).
8. **Saves**: five of six save buttons into `⋯` (recommended).
9. **Settings**: folder rows one menu each, "not used yet" folded, background block folded, an "advanced" fold (recommended).
10. **Explanatory lines** (§3.5): remove the D lines everywhere; no spec change needed (recommended; can go first, it is the smallest change).

Suggested order: 10 and 1–4 first (one change each, every page benefits), then 5–6 (the editor), then 7–9 and the remaining page rows.

## 6. Notes

- Spec ↔ code discrepancies seen: the game log lead says "needs attention" (the old name of Can it run); compact details show Archo ID although 16 §7 principle 6 keeps #id in technical information; the details link shows "Enter ↵" as plain text instead of a cap (16 §7.3 rule 13); the editor marks a tagged Mod with a coloured strip at the row's left edge, which 16 §7 principle 4 forbids for emphasis unless tag colour is meant as an exception.
- Harness: headless Edge stopped answering screenshots after about ten page visits in one browser session; one browser per page worked. Not traced to the app; worth a look if players report the window freezing after a long tour.
