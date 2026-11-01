# Lists / manage: interaction proposal for copying and continuing to edit

Date: 2026-10-24. Baseline: `dev/0.9.0`, `7386f87`. Branch: `codex/lists-management-ux`.

Status: implemented on this branch using the create-in-place approach. Research and reasoning remain below; the session's interactive prototype uses example lists, while the real GUI uses existing app/view capabilities. Effective specifications updated in 06 §2.17 and §3.1.

## Implementation and validation

- Management details show a direct copy entry; the new-list form supports any source list and full historical-version selection, distinguishing current edits.
- Import previews inside the form, reusing installed/missing Mods, version differences and subscription entries; submission validates preview identity/game and rejects stale content or content overwritten by another flow.
- Drafts persist per game during this app session; return, browse other lists and back/forward restore original input. Explicit discard offers undo.
- Primary action creates and edits; secondary only creates. Create-only status keeps creation undo. Once in editor, use its editing undo; do not conflate deleting the entire list with undoing edits.
- `go test ./internal/surface/gui ./internal/view ./internal/surface/cli` passed; ten Node list-page regressions passed, including old-request races, cross-game draft isolation, duplicate names and duplicate submissions.
- With isolated `tmp/list-ux-qa` data and real Go API, browser-verified historical copying, import then create-only, name retention, back/draft restoration, Alt+← / Alt+→ and entering editor immediately after creation; all existing browser page regressions passed.
- Actual dark page: [screenshot](implemented-dark.png). Expanded options, system light theme and DPI changes in native WebView2 remain for native acceptance; browser checks do not validate those platform combinations.
- Built `bin/RimArcho.exe` (Windows GUI entry); local artifacts are not included in Git.

## Research scope and evidence

Input was actual player feedback: "I want to copy an existing list, then edit the copy" (translated). This round used task-based cognitive walkthrough and heuristic evaluation, checking code/specifications; no additional user interviews, and no speculation presented as measurement.

Relevant project constraints:

- [Top-level design §4](../../00-top-level-design.md): know where you are, where you can go and how to return; minimum steps; undo; game writes only on request.
- [06 §2.17 list management](../../specs/06-ui/2.11-other-pages.md): retain list index/details/game versions/collections/sharing; forms inline or on subpages, never modal forms.
- [06 §3.1 interaction rules](../../specs/06-ui/3-interaction-rules.md): navigation does not mutate, keyboard access, Esc goes up, folding preserves input, background reads accept only latest result.
- [06 §7 visuals](../../specs/06-ui/7-visual-design.md): magpie / Linear style, one accent, quiet cards, 4px gaps, no left accent bars, player wording such as list.
- [04 §1, §3, §9](../../specs/04-plans-and-sorting.md): current content and historical versions are different copy sources; new list has its own history; import previews first; creation does not write game files.

External references evaluate interactions, not justify new product capabilities: [NN/g visibility of system status](https://www.nngroup.com/articles/visibility-system-status/), [user control and freedom](https://www.nngroup.com/articles/user-control-and-freedom/). Normal text contrast acceptance is 4.5:1 under [WCAG 2.2 1.4.3](https://www.w3.org/TR/WCAG22/#contrast-minimum).

## Findings and impact

| Finding | Code evidence | Task impact |
| --- | --- | --- |
| Copy source depends on list selected before opening form; no source picker | `newList()` in `assets/js/planmanage.js` writes `source` into draft; `newForm()` offers only source-kind radios | Default mistaken for fixed prerequisite; player must leave form, switch list, retype |
| Historical source says copy a version it wrote | `internal/view/planmanagetext.go` | Requires understanding implicit "it", "write", "version" relationship; confused with current edits |
| Version API lists only save / launch / user origins | `PlanSources()` in `internal/surface/gui/planforms.go` | Relabeling historical versions alone may still omit copyable versions; check data scope too |
| select does not explicitly inherit text color; dark mode depends on browser defaults | `.xc-input` in `style.css` lacks color; general text rule covers input but not select / textarea | Matches reported dark-mode black text; handle control, native options and color scheme, then verify Windows WebView2 |
| Import from draft explicitly clears it | `newForm()` import callback first sets `M.draft = null`; `navigation.go()` calls `abandon()` when leaving plans | Name lost, another flow asks for it again; player doubts prior input's validity |
| Browsing another row discards draft | `pick()` and `abandon()`; 06 §2.17 explicitly says Esc / another row abandons | Browsing a list coupled to discarding input, violating task expectation and preventing return restoration |
| Copy hidden in ⋯; creation returns only to details | `menuOf()` and submit-completion path | Duplicate hard to discover; editing entry must be found again after creation |

The first three kinds block tasks or cause clear confusion; draft loss has highest priority. The root is more than missing controls: creation attaches what I want to create to what I just browsed, and import starts another task.

## Recommended approach: one creation form inside management

Keep two columns. Left shows all lists with an unfinished draft as a separate top item. Right switches between list details and creation form. A draft is not a created list, does not count toward lists or show current status.

### Entries and defaults

- Details header directly shows copy as new list alongside edit; edit stays primary. ⋯ keeps infrequent rename/delete actions.
- Copy as new list preselects current edits from this list; suggest original name + copy, or next available name on collision. All presets editable.
- New list opens unified form; copying may suggest browsed list. Existing draft changes entry to continue new list and restores without resetting name.
- Import as new list enters same form's import source. Keep existing draft name; only switch source.
- Requesting another copy with a draft restores it first and explains that source can be changed in the unfinished draft; never silently overwrite. Final implementation should offer explicit discard; this prototype emphasizes restoring one draft.

### Form order

1. **New list name**: always in same place. Explain empty/duplicate names inline; validation does not block editing other fields.
2. **Where to start**: copy list / empty / from save / import. Switching changes only source fields below, never page or other sources' inputs.
3. **Copy list** has two separate fields:
   - Source list: all available lists in current game. Selects copy source without switching edited list.
   - Version: defaults to current edits including unwritten changes; historical entries show version/date/time/Mod count, notes/origin as needed.
4. **Result preview**: enabled count, preserved order, allowed game versions. Historical copy names source list/vN; current content must not masquerade as latest saved version. Staging, old undo stack/history not copied.
5. **Submit**: primary create and edit, secondary create only, quiet continue later. Buttons clearly state destination.

Create and edit makes new list current and opens editor; create only makes it current, stays in management and selects it. Reuse `view.NewPlan()` current-list semantics, explicitly stated at form bottom before submit. Browsing sources, previewing or editing drafts does not change current list.

Success shows source, game-files-not-updated message and undo. Reuse existing creation undo; final integration must preserve conflict checks and never silently delete a list after later edits.

### Import and name ownership

Import within creation is **the source of new-list content**, not a jump to another destination-selection task.

- Name field never moves or gets overwritten. Source name is only a suggestion, auto-suggested only while user has not entered one.
- File/paste/link reading previews inline; no list before confirmation.
- Preview shows source, Mod count, installed/missing counts and game-version differences; reuse import analysis for missing content/version mismatch.
- Creation context already sets destination to new list, so do not ask again. Final buttons still create and edit / create only.
- Editor's import into existing list retains replace / add-only-missing options through its distinct entry, not squeezed into creation.
- Changing import input invalidates preview; only latest read applies. Failure retains name/input and offers retry; URL reads use background mechanism.

### Where I am, where I go, how I return

| State | Location | Next | Return and restoration |
| --- | --- | --- | --- |
| Management details | library / lists / manage | Edit, copy, new, import as new | Other-list selection only browses |
| Creation draft | library / lists / manage / new list | Change source/version, preview, create and edit | Back to manage / Esc / Alt+← retains draft |
| Import source | Same creation location, import selected | Create after preview | Switching back keeps name/source/version |
| Created | library / lists / new name | Edit directly | Return selects new list; creation undo available |

Drafts isolated by game survive normal navigation, back/forward and browsing lists. Initial scope is this app session; no promise of restart/crash recovery. Disk persistence later would need expiry/cleanup rules. Store drafts separately from page history; return restores view without repeating creation/import.

## Alternative: focused creation subpage

Reuse fields/state model on a dedicated subpage with global sidebar, library / lists / manage / new list breadcrumbs and back to manage. Limit reading width; temporarily hide management's internal list index.

Benefits: more narrow-window space, clearer focus. Cost: returning to browse source details. Recommended version preserves spatial relationships for comparing lists and deriving copies. Both use nonmodal forms rather than dialogs.

## Visuals and accessibility

- Existing light/dark panels, accent, 12px corners and 4px gaps. Explicit text/background/color-scheme for input/select/option/textarea.
- Secondary text remains readable, not decorative faint text; normal text reaches 4.5:1 on actual background. Errors have text; success/warnings never color alone.
- Native buttons/inputs/selects, all tabbable with visible focus. Source changes focus corresponding control, not page top; input arrows keep editing/selection semantics.
- Esc closes temporary menus before going back; never clears draft. Subpage navigation never writes. Native dropdown light/dark must be tested in real Windows WebView2.
- Preserve splitter drag/keyboard/per-page persistence; prototype uses fixed proportions without simulated dragging.

## Implementation impact and acceptance

Main files: `planmanage.js`, `exchange.js`, `navigation.js`, `planforms.go`, `planmanagetext.go`, `style.css`. Update 06 §2.17 Esc/row-change abandonment and 06 §3.1 form navigation; no need to overwrite existing specifications with this proposal before review.

1. Open new from A, name it, choose B and B's historical version: result comes from that B version, independent of current A.
2. Source-list change clears previous version ref/resets to current edits; no history explains why while allowing current copy.
3. Current includes unwritten changes, history exactly selected version. API lists all actually copyable origins with time/count/source; unavailable entries explain why.
4. Name → import → preview → copy → back to manage → another list → restore draft: name and all source fields retained.
5. Game switch leaks no draft; deleted source keeps name and asks to reselect; consecutive version responses apply only latest selection.
6. Failed import preview retries inline; changed input invalidates old preview; creation never offers replacement of existing list.
7. Empty/duplicate names and duplicate submit never create multiple lists; errors retain input. Success current/sidebar/management selection agree; result/undo scope explicit.
8. Light/dark controls/native options/explanations readable; 1024px/800px/narrow no overflow; keyboard completes source → history → create → return.

Prototype covers any source, linked history, empty/save/import switching, paste preview, leave/restore draft, create-and-edit, create-only, undo entry and two layouts. Link parsing, missing-Mod detection, format compatibility, actual undo conflict protection and DB behavior need validation after integrating existing capabilities; examples do not claim backend validation complete.

### Prototype checks this round

- JavaScript syntax passed; renderer accepted snippet.
- Browser: source CE → v5 → create only yielded CE v5/158 Mods; undo returned original.
- Duplicate-name error retained input; no-history list only current edits; both layouts switched.
- Name → import three package IDs → preview → back → restore retained name/content; create/edit showed three entries.
- Empty list created with input Enter, editor zero enabled.
- 1024px dark, 800px light, 320px light checked. Common body/explanation contrast on cards about 5.50:1–13.41:1; not accessibility certification of every control state.
- Final integration pending: real WebView2 native dropdown, real back/forward stack, async races, game switching, formats and disk-write boundaries.