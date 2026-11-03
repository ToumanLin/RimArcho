# 06 · Interface · Details and descriptions, can it run wording, confirmation and feedback (§4–6)

> Part of [06 · Interface](README.md). Section numbers run through all of 06 and stay unchanged when split across files. References still use `06 §x.y`.

## 4. Details and descriptions

### 4.1 Structure

The details appear in the order below, using `view.LoadDetail`, `HeaderFacts`, `ModFields`, and `HistoryLine`:

```
┌thumbnail┐ Combat Extended                     ← bold text color; TUI no thumbnail, flush-left name (§9.3)
│         │ CETeam                              ← secondary
│         │ ◉ workshop  supports 1.5 1.6  #34    ← faint
└─────────┘

can it run                                      ← muted heading; no findings: ✓ no problems found
  ⨯ Incompatible with Yayo's Combat 3, both enabled     space disable this
  ! Should follow Harmony                              S sort

Archo ID   #4k2p                                ← faint labels / text values, click to copy
package ID ceteam.combatextended
location   D:\…\workshop\content\294100\2890901044    open folder
workshop   2890901044                                open page ← o opens
 tags      combat   core                            edit      ← g / #
alias      CE                                   ← only if set, likewise note
languages  ChineseSimplified   English          ← Languages/ folders
needs      ✓ Harmony                            ← ✓ enabled, ! disabled, ⨯ not installed
           ⨯ Vanilla Expanded Framework (not installed)
           ! Some Library (disabled)
conflicts  ⨯ Yayo's Combat 3                     ← declared incompatibility: ⨯ enabled, ! installed disabled, ✓ absent

description   workshop page                     ← source §4.3
  Combat Extended overhauls combat…
  (first 12 lines in detail column, enter full-screen)

history
  updated Oct 17   package ID changed Sep 30     ← latest 3 events
  see all in changes                            ← this Mod only (§2.9)

technical information   i expand                ← collapsed by default
```

**The field list** is shared by editor details and inventory copy details (`view.ModFields`, GUI `fields.js`, TUI `fieldLines`). Inventory details add status, used in, size, supported versions, update date, and last played in the middle (§2.9, `view.LibFields`). Each field occupies one row: Archo ID and package ID; location with open folder; Workshop with open page; tags with edit; alias and note when set; languages, dependencies, and conflicts. Clicking the Archo ID, package ID, path, or Workshop ID in the GUI copies it and shows a copied notice beside it. Other fields are display-only. Fields have no background or borders, and long values such as paths wrap in full. End-of-row buttons act on that row. Version, source, and size already appear below the name and are not repeated.

- The package ID is visible by default because players use it in other tools and forums. This is the sole exception to hiding developer terms.
- Each Can it run item uses a plain sentence and ends with a directly executable key (§5).
- Technical information appears only after expansion: #id references, paths, Workshop ID, file count and size, Def, Patch, and Harmony counts, Lineage and copies, and the unloadable reason (`view.UnloadableWhy`). In the TUI, i expands it and only then loads the mod command output; in the GUI, clicking expands it.
- All text wraps by display width, treating CJK characters as 2 cells, without overflowing the column or splitting a character:
  - Break at spaces or between CJK characters, without breaking Latin words. Honor punctuation rules for line beginnings and endings. Long paths break after a slash or backslash, retaining the line's indentation.
  - Status text, description subheadings, long dependency names, and bottom hints also wrap without truncation. Only the detail column's … enter for all marker and wide BBCode table cells use an ellipsis.
- **Focused details** show the full description without the 12-line cap. After scrolling, the first line is the accent-colored ▲ <name>. The i, ctrl+w, r, g, and enter keys act on the displayed Mod and keep focus. Other keys return focus to the list and act there. Clicking the details focuses them.

**Compact GUI editor details** (2026-10-25) show the name, author, fields, description, and all installed copies of the same Mod in the left column, using the abstract labels in §2.9. The selected button has a background without a radio dot. Clicking a copy only browses it and does not change load order. A separate use this one in the list action swaps it into the list while preserving its position and supporting undo. That action is disabled for copies ignored by the game or unable to load. Compact details have no separate Can it run section; full diagnostics remain in full-screen details and the findings page. A permanent clickable full details Enter ↵ at the right of the heading opens the currently browsed copy. Inventory keeps its own copy actions and fields; both use the same full-detail page.

### 4.2 Full-screen details

**There is one shared page**, decided on 2026-10-22 and implemented in `fulldetail.js`. Enter from the editor or inventory All opens the same independently designed page, rather than an enlarged source view. The compact left-column views are simplified versions using the same `renderDetail`, field list, and copy actions. Breadcrumbs are library / lists / <plan> / <Mod> or library / mod library / all / <Mod>. There is no page-heading text; keys appear in the hint line. The right side has open folder and Workshop actions for the selected copy.

A wide Mod column occupies the left. Copies, Used in, Fields, and History occupy the right, with Technical spanning the full width below. Below 900px, the layout becomes one column. The six cards are numbered, and 1–6 focus them (§3.5):

| # | Card | Content |
| --- | --- | --- |
| 1 | Mod | Large Preview.png, name, author, the editor's one-line status, tags and tags … g, and the About.xml or Workshop description (§4.3) |
| 2 | Copies | Every instance in the Mod's Lineage (`API.ModCopies` → `view.LibRow`): source, status, current-plan presence, size, version, folder, and the player's note or a muted generated note. The selected raised copy with a left accent shows the inventory's six copy actions and plan actions in `view.OpTable` order, listing only applicable actions. |
| 3 | Used in | The plans and packs using each copy, its current-plan presence, and the space action to add, remove, or swap it |
| 4 | Fields | The §4.1 field list; clicking a value copies it |
| 5 | History | Recent changes; all changes opens updates filtered to this Lineage |
| 6 | Technical | One tab per `view.Sections` section: Def, patches, Harmony, rules, full history, and future sections such as manual Harmony targets or log-pattern cards. The renderer handles sections without knowing their concrete kinds; a new data section adds a tab. |

The current technical sections are:

| Section | Content |
| --- | --- |
| Def | def --mod text with references replaced by names |
| Patches | patches --mod text |
| Harmony | harmony --mod text for attribute patches and resolved manual `Harmony.Patch` calls; the latter are marked `Harmony.Patch` call in code. A manual: line gives resolved and unresolved counts, followed by each unresolved method and reason (03 §5.3). |
| Rules | Plain wording for About.xml loadAfter, loadBefore, and incompatibleWith declarations, and relevant player rules |
| Full history | All Mod change events, newest first |

The keys follow the shared table in §3.5:
- Left/right moves to the previous or next Mod in the source list, moving that list's cursor. In the editor, it skips uninstalled entries.
- Esc or backspace returns to the source page with the cursor on the last viewed Mod.
- `↑↓`, `j` / `k`, `PgUp` / `PgDn`, and `Home` / `End` scroll. The `tab` / `shift+tab` keys change technical tabs; 1–6 focus cards.
- The [ and ] keys select another copy. Menu or shift+F10 opens that copy's context menu. The space, m, t, x, g, n, Del, ctrl+o, and ctrl+w keys act on the selected copy as in the inventory (§3.5).
- The hints show left/right previous/next, esc back, space <action>, [ ] another copy, F10 menu, 1–6 sections, and ? all keys.

The TUI retains its editor full-screen details with Tab subsections and its inventory copy details. The new shared page is implemented in the GUI first.

### 4.3 Description source and rendering

**The measurement** was made on 2026-10-19 in the player's environment, read-only, using 1274 Workshop Mods with About.xml. The public, keyless `ISteamRemoteStorage/GetPublishedFileDetails/v1` endpoint returned 1264 descriptions in 13 batches. The table counts plain-text characters after removing BBCode and images:

| Length | About.xml | Workshop |
| --- | --- | --- |
| Empty | 25 | 1 |
| 1–79 characters | 355 | 19 |
| 80–299 | 376 | 108 |
| 300–999 | 276 | 345 |
| ≥ 1000 | 242 | 791 |

- About.xml is empty or shorter than 80 characters for 30%, or 380 Mods; 305 of those have Workshop descriptions of at least 300 characters. Only 18 have just one or two sentences and no images in both sources. Workshop descriptions reach 300 characters for 90%, compared with 41% for About.xml.
- The two sources begin identically for 504 Mods because the author copied the text. Only 5 use descriptionsByVersion.
- Workshop descriptions have images for 55%, or 696 Mods, with 7981 images in total. Of these, 139 are image-led, with at least 3 images and fewer than 100 text characters per image. Almost all mix text and images; only 1 has fewer than 80 characters and is otherwise images only.
- Image formats are PNG 7351, GIF 323, extensionless 244, JPEG 55, and WebP 6. The host `i.imgur.com` accounts for 72%, followed by raw.githubusercontent.com, img.litet.net, i.postimg.cc, and images.steamusercontent.com.
- The most common BBCode tags are img, * (list item), url, b, h1, list, h2, td, h3, i, tr, hr, th, u, table, code, nl, strike, olist, and quote.

**The rendering rules are:**

1. **Sources:** read the About.xml description on demand from disk, using the current game's descriptionsByVersion entry if present. Do not store it in the database. Fetch the Workshop description online.
2. **Selection:** when a Workshop description exists and About.xml has fewer than 300 characters or begins identically after ignoring images, show only the Workshop description under description / workshop page. If both are long and different, show About.xml first and Workshop second under two headings. Without Workshop content, such as for a local Mod, offline use, or a failed fetch, show About.xml with a muted `view.DescNote` explaining why.
3. **Fetch timing:** request only the selected Mod in a background single-ID request, without prefetching the whole list. The GUI editor debounces the local read, then immediately fetches an uncached description. Full-screen GUI details and the TUI delay after the local result; see §2.16.1 G12. The cache is `<data directory>/cache/workshop/<Workshop ID>.json`. A cached time_updated at least as recent as the local appworkshop timeupdated is fresh and needs no request. See §2.16.1 C01 for expiry when there is no manifest timestamp. Full-details r forces a refresh.
4. **Offline controls:** `archo config set workshop.fetch on|off` defaults to on; workshop.images on|off also defaults to on and separately disables image downloads. When fetching is off, use only the cache and About.xml. Network requests use only the public Steam endpoint and image URLs in the description, with the RimArcho User-Agent.
5. **BBCode** wraps by display width and follows §7. The h1–h3 tags use bold text with a blank line before and after; b is bold, i and u use normal text, and strike is muted. The list and olist tags use bullets and numbers with two-cell indentation per level. The url tag shows clickable link text in the GUI and a muted domain after the text in the TUI (§9.3). The quote and code tags use two-cell indentation and the secondary color; in the GUI they use 14px indentation without a left line (§7.3). Tables align columns and truncate wide cells. The hr tag becomes a blank line, nl a newline, and noparse literal content. Unknown tags are stripped while retaining their content. Go parses blocks, and the frontend renders headings, lists, tables, quotes, code, links, and images (§8.3).
6. **OCR** is not part of the UI. Visual-model recognition belongs in Archo tasks (09 §5).

### 4.4 Images

- **The thumbnail** comes from About/Preview.png, matched case-insensitively; JPEG content is also accepted. It is downscaled without enlargement to a 360px-wide PNG and cached by snapshot hash at `<data directory>/cache/previews/<snapshot hash>.png`. The Mod directory is read-only.
- **Description images** are downloaded on demand, decoded, downscaled to 720px wide, and cached as PNG in `<data directory>/cache/workshop-img/`. Links such as imgur.com/x.png are changed to i.imgur.com. A GIF uses its first frame; WebP and decode failures show placeholders.
- **Compact details have no inline description images**, only the thumbnail and text. The first description image is replaced by one muted [images ×3 enter to view full-screen] line, or [images ×3] in the TUI. Description images appear only in full-screen details. The TUI lists a textual [image `i.imgur.com`] placeholder for each image.
- workshop.images off disables description-image downloads.
- The GUI uses `<img>` with proportional scaling. Description images use `loading="lazy"` and are requested only when visible, through the local `/img/` handler (§8.3). The TUI draws no images (§9.3).
- Galleries and enlarged-image browsing are GUI-only.

## 5. Can it run wording

**Errors from the last run** (`view.lastRunNeed`) show the Mod name once in the title, followed by the error and warning occurrence counts, including both when present. The explanation lists at most the two most frequent actual categories and their counts. With only one category, it does not repeat the count. Texture errors include a representative resource path; remaining categories are summarized by their count. Evidence adds only the log file, time, or companion-log evidence, without repeating the total. A possible attribution adds source not yet certain; no candidate uses source unknown. A subject outside the current list adds not in the list because a save or another Mod may have used its content (2026-10-28). The game-log action carries the log path and cluster ID, opening §2.10 in the GUI. Unattributed items select no cluster, and a stale log opens its own file. The TUI uses problems --code log --subject … for a subject, or log --file … for a stale log. Current-run items (`view.LiveNeeds`) say A: N× message, without a this run prefix; evidence contains only the filename because the heading identifies the run and the body contains the count (2026-10-28). They also use game-log: the action opens the active Player.log and selects the cluster; the TUI uses archo log. The old text-only view log and evidence action differed from last-run behavior (2026-10-28). Other view actions are unchanged.

**Each item** is generated from findings and environment questions. It has a group, a sentence containing Mod names, actions that operate on it (`view.Action`), and a counted flag. Finding codes, #refs, pkg:, and p: never appear on the page; they belong in technical information, the CLI, and MCP.

**Names and navigation:** Go supplies `NeedPart` fragments for titles, bodies, explanations, and folded items through `Need.Content`. The original strings remain for terminals and text clients. Name candidates come only from this item's Mods, RequiredBy, action references including targets and options, and original finding subjects. Merged items retain the original subjects; names are not matched against the whole library. Each name carries an installed copy ID, or a known Workshop URL when uninstalled, or remains plain text when the source is unknown. Can it run, the editor problem panel, and row popovers share rendering. Clicking an installed name locates its editor row and clears a hiding filter if needed; clicking an uninstalled name opens Workshop. Each name is Tab-focusable and opens with Enter or Space. The whole sentence is never linked to the first Mod, and JavaScript does not search strings for names. Identity questions follow the same navigation rule. `Reason.Content` and `Reason.Evidence` supply the same names and evidence in popovers. A package index is built once per snapshot; each item generates fragments only from relevant names, without traversing the library again. Fragments use normal inline flow in titles, explanations, folded items, and overlays, without the flex spacing used between fact fields. A popover's own Mod name stays plain; other relevant Mods retain independent links. Shared fragment data is never mutated.

**Corpus hints:** items with `Evidence.Kind == EvCorpus` always use `StatusHint` and occupy the separate worth knowing section, `SectionHint` (§2.4). They represent learning from shared logs and are not faded: the section heading and icon use the accent, the title stays bold, and evidence displays normally. Editor panels and row popovers use neutral information icons (§7), with errors and warnings before hints. Hints are excluded from sidebar counts, the editor problem-button count, section and Mod problem counts, and automatic dependency completion. A hints-only editor button says hints and still opens the panel. The has:problem filter excludes hints. Evidence says shared logs as of YYYY-MM, taking the cutoff from `known.Data.Cutoff` in embedded `known.json`; measured ratios remain in the source. A corpus association is never presented as a guaranteed failure in this run. Declared dependencies and corpus-inferred dependencies remain separate items. The editor panel shows an evidence row only for corpus hints to retain the cutoff date, omitting duplicate evidence for ordinary findings. Can it run and row popovers retain their evidence.

| Internal code | Player sees | One-step action / TUI key | Group |
| --- | --- | --- | --- |
| `official.order` | Core and DLC must come first | S sort | breaks the game |
| `order.violated` strong warning | One requirement: A should follow B; the wrong order may prevent dependencies or content from working. Multiple requirements: 10 Mods are out of order, counting the Mods that need moving; Enter expands the individual requirements. | S sort | breaks the game |
| `order.violated` weak / harmless info (04 §5.3) | Merge into the official or strong S item, adding `Need.Also`: also satisfies 5 weaker requirements. Without that item, show 5 unmet weaker requirements and explain that violating these author or community suggestions did not correlate with more shared-log errors. | S sort; f expands a standalone item | Merged error item, otherwise worth knowing |
| `order.cycle` | Merge into the S item with an Also line explaining the priority-resolved cycle among 3 Mods and the abandoned requirements; attach the viewing action there. Without an S item, explain that sorting resolves the 3-Mod cycle by priority and list the dropped requirements. This is uncounted info (03 §4.3.1). | Enter, or v when merged, views the requirements in plain wording; handled automatically without confirmation or manual ignore | Merged error item, otherwise worth knowing |
| `pkg.duplicate-active` | Two copies of the same Mod are enabled | Space chooses the copy to retain | breaks the game |
| `entry.missing` | One item: X is listed but not installed, using its Workshop name when known. Multiple items merge, common with imported lists since 2026-10-25: N listed Mods are not installed; subscribing and downloading restores their list positions. Dependencies on these entries do not create separate `dep.missing` items. | With known IDs, s subscribes to one or all through background tasks after choosing Steam / SteamCMD; the TUI prints archo subscribe workshop …. For one, o opens or searches Workshop and Space removes the entry. For multiple, x removes all and f lists them. | breaks the game |
| `mod.unloadable` | A is enabled but the game cannot load it | Space disables A | breaks the game |
| `mod.replaced`, community replacements | A has a maintained replacement, B | If B is installed, Space enables B and disables A; otherwise o opens B in Workshop | breaks the game |
| `version.unsupported` aggregate | 293 Mods do not declare 1.7 support; 288 declare 1.6. Mods with ignored reminders are excluded. | Enter opens the list; M ignores these N Mods' 1.7 reminders after confirmation | breaks the game |
| List saved for old version | The list was saved for 1.5 and the game will reset it on startup; save it once | w saves | breaks the game |
| `dep.missing` | A needs B, which is not installed | o opens the declared Workshop URL, or searches if none exists | breaks the game |
| `dep.inactive` | A needs B, which is disabled | Space enables B | breaks the game |
| `dep.observed-missing` corpus | A may lack B, which is not installed or is disabled; try it if the matching errors occur. This includes sound-only and texture-only observations. | Space enables an installed B; otherwise open Workshop | worth knowing |
| `combo.known-bad` corpus | A and B together often produce errors, or this version of A does; try disabling one or updating if those errors occur | Space chooses which to disable; for a version-specific hint, Space disables A | worth knowing |
| `mod.known-error` corpus | Shared logs often show errors from A; check for an update if they occur | Enter views the error | worth knowing |
| `patch.no-match` target inactive | Some of A's content is inactive because target B is disabled | Space enables B; Enter views the finding | breaks the game |
| `def.parent-missing` | N pieces of A's content cannot load; one line describes the missing basics and corpus error ratio. Details contain the kinds and advice without repeated list evidence. | Enter opens focused details and fixes: enable a provider, subscribe to declared candidates, or inspect the subject's Workshop requirements. Group-ignore is available when the log condition permits (03 §4.3). | breaks the game |
| `incompat.active` | A is incompatible with B | Space chooses which to disable | breaks the game |
| log.* | A — 8 warnings, with a representative missing-texture path below | Enter opens the game log and Mod cluster; the TUI opens the subject's technical page | errors from last run |
| Live log | A: N× message, under the current-run heading, newest first within each phase (§2.4) | Enter opens the log and cluster; the TUI uses archo log | Last-run group, this run |
| Identity question | Are X and Y the same Mod? | s / c / u, with a second press to confirm | for you to confirm |
| External ModsConfig conflict | The file changed outside, and the plan has also been edited since | View the diff; L loads after a second press and replaces the list; I ignores | for you to confirm |
| `mod.pinned` upstream changed | A is locked, and its Workshop version was updated afterward | mod <Workshop instance> --diff <copy>, pin --update, or confirmed unpin | Elsewhere in inventory (§2.4, §2.9) |
| `mod.hidden-residue` | A's hidden or unsubscribed folder remains | Confirm trash to recycle it | Elsewhere in inventory |
| `mod.hidden` | A is hidden, likely by another manager | Ignore to keep it hidden, or confirm unhide or recycle | Elsewhere in inventory |
| `texture.stale` | A's textures changed after conversion | Reconvert, or confirm deletion of converted textures | Elsewhere on textures |

- The action types in `view.Action.Kind` are sort, save, enable, disable, choose-disable (select one option to disable), open-url, problem (a focused explanation and fix), view (read-only command text), list (a title and rows), fold (expand or collapse merged items), ignore-rule, load-external, dismiss-conflict, answer, unhide, unpin, trash, and ignore. `Action.Key` is the TUI key. A GUI button is labeled with the action, such as enable B, sort the list, or open in Workshop.
- **Problem details** (`view.BuildProblemPanel`, GUI ProblemDetail and `popups.problem`) show only the meaning, affected content, evidence, and fixes for missing parents, inactive patch targets, or known errors. They do not open the full Mod card or all its patches. The GUI uses a temporary layer opened by the player; Esc leaves, Tab or left/right focuses buttons, and Enter executes. Opening another layer first closes this one. Late reads cannot reopen a closed layer or overwrite a newer layer or game. The panel first shows the cached explanation and options. If installed providers need a lookup, it says searching installed mods… and updates the buttons when background results arrive. Closing the layer, opening another, or leaving the environment cancels the lookup; completion also releases the cancellation context. Enabling providers precedes subscription candidates. When either is available, the first button is primary and the subject's Workshop action stays secondary. If every missing template has an installed provider, show only enable buttons, without candidates or generic find-provider advice. A completed unsuccessful lookup says that installed Mods, including disabled ones, cannot provide the basics; an incomplete lookup says not all installed Mods were checked, without asking the player to retry. Only a candidate-only view warns that the candidate may be an optional combination. Each affected content item appends a muted file:line. Evidence that only repeats that location is omitted; other explanatory evidence remains. The initial read uses an independent log session and supplemental lookup uses a private read session, neither holding navigation or edit locks. The TUI uses focused CLI text for the same finding reference. Locked-version diffs, list diffs, and subject logs retain their dedicated viewers; identity questions and version aggregates are unchanged.
- `patch.no-match` appears only for an installed but disabled target, target-inactive. An absent target or incorrect version routing is not player-fixable and stays in technical information. The same applies to `def.abstract-collision`.
- `version.unsupported` appears only when some Mods stopped at the previous version, at warning level. If all declare only older versions, they are information-level and omitted. Corpus G8 reversed these severity levels (03 §4.3.1).
- **Version reminders** (03 §4.2) use `Needs.Reasons` to say, for each Mod, not declared 1.7, declares 1.6, listing its actual declared versions. The explanation is singular, and the sole action is ignore this Mod's 1.7 reminder. Completion says this Mod’s 1.7 reminder was ignored, without the aggregate view list action. Details share VersionReason; disabled Mods can also be viewed or ignored. Ignored reminders show the muted completion text and restore 1.7, whose completion says this Mod’s 1.7 reminder was restored. The restore action remains even after an update declares support. The TUI uses M / U.
- Can it run and the editor panel show muted N ignored 1.7 reminders beside the version aggregate, with view or restore, without a count or row icon. This counts only current-list Mods that still lack declared support and have an ignored reminder. Even when all are ignored, the editor retains a muted 0 problems entry. Background events use Go's Status and Reasons directly instead of reconstructing per-Mod wording from the aggregate.
- **The version list** (`versionpanel.js`, a temporary layer under §3.1 rule 7) calls `API.VersionWarnings` only when view list is opened. Needs and Board do not generate or carry the row list. The top says not declared 1.7 once, with ignore all on the right. Each scrollable row has a clickable Mod name that locates the editor row and clears a hiding filter, its actual supports 1.6 declaration, and a compact ignore action. Groups put the previous major.minor version first, such as 1.6  352, then older-only declarations, such as 1.5 or earlier  13, then other declarations. Each group has an ignore action. Batch and all-item ignore confirm the global scope and recoverability; individual ignore runs directly. The ignored list uses the same panel with individual, group, and all-item restore actions and no confirmation. After an action, retain the panel and scroll position and reread the list. Ignore late results after closing or changing the game or list. Tab / shift+Tab moves among controls, Enter / Space executes, arrow and page keys scroll, and Esc closes. The TUI opens a grouped picker on demand, with locate, ignore, and restore for an individual Mod.
- **Merging:** findings of the same kind that one key can solve together merge into one item. Current examples are `order.violated` solved by S, same-Mod parent-missing findings shown in one focused view, and same-Mod patches targeting the same disabled Mod, such as X's 6 patches inactive, solved by enabling that target once. Dependencies merge by normalized target and name every requiring Mod in the explanation. Missing list entries merge separately according to installed-copy availability. The expansion key is f, or Enter for order violations. In the GUI, See N one by one is a text link below the explanation rather than a button; the editor uses inline folded lists. Expanded rows form a semantic list without leading commas. Logs remain separate per Mod.
- **Counts:** the sidebar and top-bar numbers equal the Counted items on the page, excluding hints, folded stale logs, and items handled elsewhere. A merged item counts as 1, so the number reflects tasks rather than individual findings. Row status includes counted findings and corpus hints; icon explanations and counts are calculated separately.
- **Buttons** (2026-10-29, GUI) show every action at the bottom of an item, aligned with the title, without a ⋯ menu. The first is primary and the others secondary. A fold action is the textual See N individually link. A choose-disable or duplicate-copy choice has one equal button per option, such as disable Combat Extended / disable Yayo’s Combat 3, without a primary button. Space focuses the first option, left/right moves between them, and Enter executes. Identity questions also have three equal answers. The TUI retains its picker.
- **Inline confirmation** (2026-10-29, GUI) applies to `Action.Confirm` on Can it run (§6). The button row becomes the `Action.Warn` consequence and OK / Cancel with Enter / Esc. Link repair says to repair the links marked remove above; the rows themselves are the preview. There is no dialog. The same action in an editor panel or popover still uses a dialog.
- **Completed items** (2026-10-29, GUI) are determined by the reread after an action on this page completes, rather than incidental refreshes during it. If the original item vanishes without a replacement of the same `Need.Kind`, retain its original position as a muted ✓ row with the action's result and old title. A change from 7 out-of-order Mods to 6 is a replacement. Sort and save use their completion status; a later Analysis complete message never overwrites it. An action solving several items marks them all. Only the latest undoable row has undo / u; it undoes the previous list step and restores those findings. Completed rows are uncounted, remain cursor-accessible for undo, and clear on leaving the page or changing the game.
- **Cross-column links** (2026-10-29, `Need.Related`) appear when a last-run subject is also a left-column dependency's requiring Mod (`dep.*`, `RequiredBy`) or a hint subject. Both items gain links, such as last run Vanilla Furniture Expanded — 12 errors and related Vanilla Factions Expanded needs disabled Vanilla Expanded Framework. Clicking jumps to and briefly highlights the other column's item. Current-run items are excluded.
- **Automatic fix F** enables installed required dependencies and reorders through `API.CompleteDependencies`, with one undo. Its button count is the number of solved counted dependency-enable items plus S items. The button is hidden if there are none.
- **Row icons explain themselves** (2026-10-25, GUI). Hovering a warning or error icon shows the underlying titles or sentences with explanations below, most severe first. Uninstalled and unloadable entries say so explicitly. Clicking pins the popover so its text can be selected. Clicking the same icon, pressing Esc, clicking outside, or scrolling the list closes it. An unpinned popover ends with muted click to pin. Each pinned item has the same action buttons as Can it run (§2.4): the first is primary, identity answers are equal, and fold is omitted because popovers have no expandable part. Clicking closes the popover before executing through runAction in `screens.js` and `Reason.Actions`, with the same confirmation, undo, and switch-time disabling. `Needs.Reasons`, exposed as `Board.reasons` by Instance ID, uses the same items as the icon, so every icon has an explanation. There is no extra keyboard key; Enter details already lists the findings (§3.1 rule 6). The implementation is js/badgepop.js.
- **Identity questions** (`view.questionNeed`) directly show same / copy / unrelated in GUI Can it run, the editor panel, and pinned popovers. No answer is primary or hidden in a menu. Hover explains the impact on lists and change history; confirmation repeats it and says u cannot undo it. The keys remain s / c / u. The stored summary gives the main reason without a generic similar explanation or vague inference chip. Why this question is a separate muted link below the text.
- **Why this question** opens a temporary comparison through v or the link; Esc returns to the original location. `popups.identity` and `view.IdentityQuestionPanel` starts with a one-sentence conclusion, then new and previously recorded columns compare the name, package ID, author, Workshop ID, and folder. Symbols and backgrounds distinguish matches and differences; rows missing on both sides are omitted. Folders show only the basename, with the full path on hover and click-to-copy. Shared files appear below, capped as in [02 §5.4](../02-environments-identity-and-scanning.md#54-identity-questions). The three answers remain equal and include impact hints. Clicking or s/c/u closes the layer before the existing confirmation, without nesting. Close has initial focus; Enter chooses no default answer. The TUI retains a brief text panel.
- **The following do not appear here:**
  - Information-level Def overrides, patch contention, and shared Harmony targets appear only in technical details, the CLI, and MCP.
  - Harmony warnings without log evidence remain technical because the player cannot act on them.
  - Logs outside the current plan's latest launch or from another game version (03 §6.4) collapse to an uncounted muted earlier log  Sep 29  1.6 row.

## 6. Confirmation and feedback

- **Sort, save, and launch run directly** through `view.Run`, with one click or key. The status line shows `view.Outcome.Text`; sorting supports u undo, shown as a GUI Undo link or a TUI u hint. A refusal, such as a running game, outside edits, missing user data, or missing Steam, shows its reason without a confirmation. If `Outcome.Refused` includes commands, the GUI opens a read-only dialog and the TUI a panel listing remedies such as diff, load, or ignore.
- **Preview** remains optional and never gates an action. The command-panel sort, save, and launch previews use `view.DryRun` and the CLI's --dry-run text. They are read-only and have no continue button. The CLI form is `archo sort|save|launch --dry-run`.
- **Actions undoable in one step need no confirmation.** Besides sorting, these include history checkout and loading an external ModsConfig.xml with L. Each is a workspace edit that preserves the replaced list in history and offers status-line undo (`view.Action.Undo`).
- **Confirm actions that RimArcho cannot undo.**
  - The TUI requires the same key twice. The first press shows ✗ press R again: … in the status line; repeating the key executes, while another key cancels and reports cancelled.
  - The GUI dialog shows `view.Action.Warn`; Can it run uses inline confirmation (§5).
  - The GUI dialog is modal (§11 A6): focus starts on the dialog itself, so `enter` confirms as the confirm button's key cap shows, while `tab` reaches Cancel first and `enter` there cancels; `esc` cancels; the page behind is inert; on close, focus returns to the opener, or to the page if the opener is gone. The RAM warning and the uninstall confirmation start on Cancel and have no `enter` key cap. Prompts (URL, rename, note) start in their field. `ctrl+b`, `ctrl+k` and `ctrl+\` still work over a dialog.
  - These actions currently include deleting a plan; identity answers s/c/u, whose Lineage merge or split cannot be undone; unhide and unpin, which rename About.xml; trash, recycling a pack, or deleting files copied by an unfinished build; and settings actions to forget a folder, download about 50 MB of community data, or run gc.
- **Recycle and rename actions** unhide, unpin, and trash show a --dry-run preview before confirmation (08 §6, §7).
- **Uninstall** shows a preview followed by one confirmation. See §2.14 for its contents and focus, and 01 §4.1 for its scope.
- **Result text** uses plain surface wording, such as enabled Beta or Harmony now position 3. Command errors pass through `view.Humanize`, which replaces instance references with Mod names.
- **Long work** does not block the interface and reports progress in the status line (§8.4). Every task is recorded (§2.16). Library file and Steam actions queue after confirmation; one failure does not stop the others, and results are recorded per item.
- **Changes never open dialogs.** Scan results and new game errors update only the status line and counts.

### 6.1 Sort outcome and completion API

The GUI button and S key call `api.Run("sort")`, which returns view.Outcome. Sorting completes normally. The four arrays below are omitted when empty. Missing dependencies include only required About.xml dependencies of enabled instances, including current-game-version entries; optional ordering such as loadAfter is excluded. Cycles are resolved automatically (04 §5.2). The cycles array is for inspection and does not indicate sort failure.

The JSON contract uses ? for optional fields:

```ts
type SortMod = {
  name: string; packageId: string; workshopId?: string;
  instance?: string; ref?: string;
};
type SortOutcome = {
  what: "sort"; text: string; undo?: boolean; refused?: string[];
  missingDependencies?: {
    kind: "installed" | "workshop" | "unknown";
    name: string; packageId: string; requiredBy: string[];
    instance?: string; ref?: string; workshopId?: string;
  }[];
  missingMods?: (SortMod & { installed: boolean; canEnable: boolean })[];
  replacements?: {
    original: SortMod; replacement: SortMod;
    installed: boolean; enabled: boolean; canReplace: boolean;
    blockedReason?: string; reason: string; source: "Use This Instead";
  }[];
  cycles?: {
    members: SortMod[];
    broken: { before: SortMod; after: SortMod;
              kind: string; strength: string; reason: string }[];
    resolution: string;
  }[];
};
```

missingMods contains entries that do not resolve to instances in the current game, deduplicated by normalized packageId. This **does not necessarily mean the files are absent**. Another game's Mods/ may provide a deployable copy, giving installed and canEnable as true, instance and ref, and the existing enable action. The Workshop ID is taken first from list metadata, then known Lineage or instance data, then the cached public index. Unknown IDs are omitted, and DLC has no Workshop ID. Sorting makes no network requests. Only !installed && workshopId entries are eligible for subscribe all. Installed but unenableable entries show the reason and guidance to check the copy.

replacements reuses Use This Instead data and version filters, downloaded through update data. Without the data, no replacement is guessed. Workshop branches with the same package ID are distinguished by Workshop ID. An installed but unloadable replacement has canReplace=false and blockedReason. The before and after fields describe the dropped directed edge; strength is harmless/weak/strong, and kind is the sorter constraint type.

Backend exports:

```go
func (s *API) CompleteDependencies() (*view.Result, error)
func (s *API) ReplaceRecommended(refs []string) (*view.Result, error)
```

`api.CompleteDependencies()` transitively enables all available installed required dependencies and sorts, with one undo. It returns `{text, undo?, enabled?: string[], workshopIds?: string[], missingDependencies?: MissingDependency[]}`. The enabled field contains newly enabled instance IDs, workshopIds contains deduplicated Workshop IDs, and missingDependencies contains the remaining missing dependencies.

`api.ReplaceRecommended(replacements.filter(r => r.canReplace).map(r => r.original.ref))` revalidates the current suggestions and replaces enabled originals at their original positions, deduplicating already enabled targets. The batch is all-or-nothing with one undo. It does not automatically sort, subscribe, or edit Mod files. It returns `{text, undo?}`. An unavailable or stale suggestion returns an error and leaves the original unchanged. Chains within one batch are rejected and must be split.

The frontend deduplicates the needed IDs, asks the player to choose Steam or SteamCMD, then uses the existing `api.Subscribe({kind:"workshop", url:"https://steamcommunity.com/sharedfiles/filedetails/?id=" + id, steamcmd: useSteamCMD, batch:true})`. After downloading and scanning, sort, enable, or replace again. Subscription never automatically replaces the original.
