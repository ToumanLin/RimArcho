# GUI accessibility for agents: criteria and baseline

Dated investigation and proposal, 2026-11-02. This document defines screenshot-free GUI acceptance and records a synthetic baseline. The criteria were adopted the same day as 06 §11 rows A1–A18 (A*n* = A11Y-*n* here); the §3.1 rule below is still only a proposal.

## Result

The GUI does not yet meet screenshot-free operation criteria. All 22 sampled page/form states fail shared checks for page identity, control semantics, states/context, feedback and stable targeting. Twenty states have no unnamed recognized action-role nodes; Add subscription and Games each have one. This name-only success does not cover custom actions missing an action role.

The .NET UIA client's Control view exposed only the RimArcho window in both sampled states; the COM client sees the page (correction in gap 1). CDP exposed page content, but also the editor retained behind secondary pages. The canonical 485-Mod export exposed 26,894 nodes; its full-tree calls took 848.9, 581.7 and 533.1 ms. Other sampled pages exposed 433–1,223 nodes and took at most 34.9 ms per call.

## Agent scenarios and the MCP boundary

1. Identify the game, list and page; read a Can it run finding and its evidence; choose an action and verify its outcome.
2. Find a Mod and distinguish installed copies; read details; select records and verify enabled/selected state.
3. Reach first, last and filtered virtual rows; sort a table and change columns without losing the record’s identity.
4. Change a choice, cancel a menu, handle a warning and return to the opener without operating covered content.
5. Recognize background start/progress/success/failure/cancellation and find retry or undo later.
6. Read full paths, findings, errors and hints from the tree or keyboard-opened details, without hover.

Use CLI/MCP in 05 §1 and §5 for bulk inventory queries, large exports, plan edits, analysis and other domain work when the task is not GUI verification. It provides domain references directly. GUI navigation, focus, layout, prompts and displayed feedback require GUI checks. Every exposed GUI action still belongs in accessibility acceptance: an MCP workaround does not make a failed GUI criterion pass. Preserve 05 §1.2’s player-only boundaries for account sessions and broad configuration changes; accessibility does not authorize those actions.

## Measurement scope and isolation

- Worktree `claude/mod-config-a11y`, HEAD `f57470b43c11428c1703f16e0fa9aebbddb99f89`. Source hashes are in scratch `provenance.json`. Another worker was active, so this is a dated sample, not release certification.
- Windows 11 Pro 10.0.26200; Go 1.27.2; Node 22.17.0; Wails/WebView2 composition hosting as configured in `go.mod` and `internal/surface/gui/gui.go:87`. Browser viewport 1440×880; English interface text. The exact installed WebView2 runtime version was not recorded.
- Used the existing `D:\RimArcho-test\memprobe\home` and fixture in place. Editor: 485 enabled +869 disabled =1,354 rows. Inventory keyboard position reached 1,353 / 1,353. No fixture regeneration or home copy.
- Before launch, read-only SQLite checks verified that the only environment’s game/user-data paths were under `D:\RimArcho-test\memprobe\fixture`, `managed_link` was empty and automatic update checks were off. No fixture directory junctions or existing listeners on 19347/19348 were found.
- The reviewed memprobe host used synthetic detection roots with `SkipRegistry`, offline HTTP, refused game launch, disabled the monitor, and redirected lock/keep paths. The scratch host additionally refused **all subprocess command arguments**, disabled SteamCMD/tool-path lookup and replaced game-process probing. APPDATA, LOCALAPPDATA and temporary paths were redirected to the authorized scratch directory. The real RimArcho profile, game, Workshop, Steam and RimWorld Config were not targets of this run.
- A scratch Go overlay replaced the build input for `cmd/archo/main.go`; repository source was untouched. Build: `go build -overlay D:\RimArcho-test\a11y-criteria\overlay.json -o D:\RimArcho-test\a11y-criteria\a11yhost.exe ./cmd/archo`. GUI launch followed the isolation checks above.
- Developer module navigation set up the 22 states; `Accessibility.getFullAXTree` ran three times per state. This does not prove unaided route discovery. DOM inventories were diagnostic cross-checks, not a substitute observation channel for acceptance. No screenshots were taken.
- PowerShell `UIAutomationClient` / `UIAutomationTypes` traversed `TreeWalker.ControlViewWalker` from the probe’s own HWND. The first hidden-window lookup failed; both successful samples followed explicitly showing that sandboxed window without activation. The second also used CDP `Page.bringToFront`. These were the actual Wails window, not browser mocks.
- Collection ended at 20:09 PST. The probe and six descendants were stopped; `cleanup.json` records their PIDs. Ports 19347/19348 no longer listened. Evidence is under `D:\RimArcho-test\a11y-criteria\`; this report is the only repository artifact.

## Proposed acceptance criteria

All requirements and new tree budgets below are **proposals**. Existing 06 §3, §8 and §10 remain authoritative. Release acceptance requires no failed or unmeasured applicable criterion across pages and interaction variants. A static tree cannot establish keyboard coverage, live-event delivery or focus restoration by itself.

| ID | One-sentence requirement | Why the agent needs it | Measurable check | Pass threshold |
| --- | --- | --- | --- | --- |
| A11Y-1 | Expose one named active content landmark, named navigation/breadcrumbs and the current destination; exclude inactive pages from the tree and focus order. | Know which page is active. | Dump every route and Back/Forward transition; compare tree boundaries with the rendered page inventory. | 1 named active content region; 0 inactive-page nodes/focus targets; current route identifiable without DOM classes. |
| A11Y-2 | Give every node with an action role a non-empty accessible name, including icon buttons and editable fields. | Find the intended action or field. | Count unnamed action-role nodes in AX/UIA, including disabled controls. | 0 unnamed action-role nodes in every state; run A11Y-3 alongside this name-only check. |
| A11Y-3 | Represent every GUI action with its correct semantic role and an applicable UIA pattern or keyboard action. | Discover custom controls that a button audit misses. | Compare rendered action inventory, including delegated clicks, with tree roles; operate each component type. | 100% of inventoried actions represented; 0 actionable generic/text/image-only nodes; 0 invalid role hierarchies. |
| A11Y-4 | Expose current, checked, expanded, selected, disabled, read-only/editable, value and busy states wherever applicable. | Predict an action and verify its effect. | Compare before/after trees with fixture state, distinguishing cursor from selection. | 100% applicable states agree; one current navigation destination; current and selected rows distinguishable; disabled actions have readable reasons. |
| A11Y-5 | Use meaningful names and sufficient named context to distinguish controls sharing a name. | Avoid choosing the wrong clear, edit or splitter. | Reject symbol-only, number-only and generic action names; inspect equal role/name/context collisions. | 0 meaningless action names; 0 ambiguous targets within the nearest named semantic context; repeated actions have distinct labelled field/row context. |
| A11Y-6 | Expose modal prompts as named modal dialogs, move focus inside, contain Tab/Shift+Tab and restore focus on dismissal. | Handle an interruption without operating the background. | Open every prompt variant; cycle both tab directions through all controls plus one; cancel and inspect focus. | 1 named modal dialog; 0 background focus targets/actions; safe initial focus; return to the connected opener or a named fallback. |
| A11Y-7 | Expose menus, dropdowns and command results as named collections with opener relationships and readable active, selected and disabled items. | Inspect candidates, choose and cancel. | Open by keyboard; inspect expanded/controls/active-descendant or focus; navigate, cancel, reopen and confirm. | 0 unresolved relationships while open; all candidates named; active differs from committed selection; arrows do not commit; Esc restores opener; Tab closes a nonmodal dropdown. |
| A11Y-8 | Expose data tables as labelled tables/grids with associated headers/cells, counts, column positions and sort state. | Read a row with the correct column meanings. | Validate table/grid → row → header/cell hierarchy and sort/count/index changes after column hiding or reordering. | 0 orphan headers/cells; 100% rendered cells associated with headers; accurate counts, indices and sort state. |
| A11Y-9 | Expose virtual collections’ total, filtered total, item positions and identity, and a discoverable route to any item. | Reach an unmounted Mod without guessed scrolling. | Use first/last, paging and known-name search in the large fixture; compare mounted items, indices and selection. | Accurate total and 1-based absolute index on every mounted item through ARIA/UIA or explicitly associated text; 100% requested targets reached; no duplicate/skipped identities. |
| A11Y-10 | Make required content, full values and explanations readable through tree text/descriptions or keyboard-opened details. | Read findings, paths and reasons without hovering. | Compare visible text, title/tooltips, status/source icons and truncated values with tree content and reachable details. | 0 required facts available only as pixels, color, unexposed title or hover; full values recoverable; informative images named and decorative icons hidden. |
| A11Y-11 | Expose each async operation’s start, measured/indeterminate progress and terminal success, cancellation or failure with an operation identity. | Know whether to wait, continue or retry. | Script synthetic scan, analysis, build/download and slow-page success/failure/cancel timelines; inspect event order. | Start acknowledgement ≤100 ms; progress and terminal states visible ≤1 s after the supplied event; determinate value/min/max; indeterminate never falsely claims 0% complete; no stale result on another operation. |
| A11Y-12 | Expose errors and transient feedback as status/alert content with readable scope and a durable outcome/recovery route. | Notice results and recover after navigation. | Inject success/refusal/failure through shared renderers; inspect live semantics and find retained outcome/retry/undo after a key and navigation. | 0 silent failures or plain-text-only live updates; invalid fields linked to errors; task outcomes retained until dismissed; available retry/undo actions named and operable. |
| A11Y-13 | Make every GUI action executable through keyboard or UIA and expose the required shortcut or action route. | Complete tasks without coordinates. | Execute an action-to-key/pattern matrix for every page, native picker, menu and secondary screen using synthetic fixtures. | 100% action coverage; 0 unintended keyboard traps; navigation keys make no edits except documented binary-choice behavior. |
| A11Y-14 | Provide discoverable keyboard/UIA alternatives for every drag, double-click and hover interaction. | Resize, reorder and inspect without a pointer. | Pair pointer handlers with reachable alternatives; test list moves, column order/width, splitters/reset and badge explanations. | 0 pointer-only actions; alternatives produce the same result/undo; essential hover content also available from focus or details. |
| A11Y-15 | Provide predictable dismissal/back navigation and readable focus/cursor position after recovery. | Resume at the correct object. | Test Esc, Back and Forward across details, menus, prompts and filters; compare route, opener, cursor and selection. | ≤3 Esc presses reach the 06 §3.1 base state; named close/cancel actions; focus returns to opener/named fallback; history navigation makes no edits. |
| A11Y-16 | Re-identify controls and records after redraw, reordering and virtual recycling through stable semantic context or exposed stable identifiers. | Avoid acting on a recycled row. | Resolve saved tree targets after refresh, sort, insertion, navigation and locale changes against fixture identities. | 100% resolve uniquely to the intended control/record or explicitly become unavailable; position is never a record identity; refnums are optional diagnostics. |
| A11Y-17 | Expose equivalent actionable names, roles, states, content and navigation in native Windows UIA and development CDP AX. | Support desktop agents on the shipped window. | Compare UIA Control view and CDP on library and another page, plus dropdown/dialog/virtual-row transitions; operate native patterns/keys. | 0 missing required UIA controls/facts/states or semantic mismatches; native operation succeeds; CDP alone cannot pass. |
| A11Y-18 | Bound accessibility-tree size and retrieval cost on the large synthetic fixture, including text previews. | Avoid parsing thousands of unnecessary nodes per action. | At 1440×880 with 1,354 editor rows, count non-ignored nodes including inline text boxes, dump bytes and maximum of 3 warm full-tree calls; inspect virtual overscan. | ≤2,000 exposed nodes; ≤2 MiB raw full AX JSON; all 3 calls ≤200 ms; mounted rows ≤viewport capacity +16 per collection; retain existing 06 §10 resource limits. These tree budgets are proposals. |

## Baseline by page

`P` = the specified observation passed in the sampled state; `F` = an observed failure or failure in an identical shared component; `U` = applicable behavior not fully measured; `N` = no subject in that sampled state. N does not exempt unopened dialogs, unloaded results or later task states. No page passed the complete set.

Shared failures apply to every page: A11Y-1 (unnamed editor main and retained editor), A11Y-3 (custom actions without roles), A11Y-4 (current navigation is CSS-only), A11Y-5 (indistinguishable splitters), A11Y-7 (shared command palette lacks a semantic result collection), A11Y-11/12 (global terminal/error feedback uses a plain footer), A11Y-16 (retained virtual rows lack usable record-identity/state relationships). These are shared-source conclusions supported by dumps, not 22 independently completed workflows. Actual scan/build timelines remain unmeasured even though the shared feedback path already fails.

A11Y-2 counts recognized action-role nodes; A11Y-3 audits omitted custom actions. Counts include the shell and retained editor. Meaningless counts below cover symbol/number-only action names, not all contextual ambiguity. AX latency is maximum of three warm calls including CDP transfer/JSON parsing, not input/page-switch latency or a memory benchmark.

| GUI state / route | Refnum prefix | Exposed AX nodes | Action-role nodes | Unnamed | Meaningless | Max AX ms | Dump KiB |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Launch / home | hom | 433 | 32 | 0 | 0 | 17.4 | 275.3 |
| Plan editor / plan | edt | 610 | 44 | 0 | 0 | 22.2 | 329.9 |
| Mod library / library | lib | 1,223 | 55 | 0 | 1 | 34.9 | 618.2 |
| Can it run / needs | run | 635 | 41 | 0 | 0 | 20.2 | 349.8 |
| Game log / log (empty) | log | 554 | 35 | 0 | 0 | 20.2 | 309.2 |
| Updates / changes | chg | 650 | 48 | 0 | 0 | 21.7 | 360.2 |
| Add subscription / subscribe | sub | 616 | 47 | 1 | 0 | 19.8 | 347.0 |
| Search files / search (empty results) | sea (fallback) | 632 | 50 | 0 | 2 | 20.5 | 351.4 |
| All lists / plans | lst | 693 | 56 | 0 | 0 | 24.1 | 377.7 |
| New list / plan:new | lst | 623 | 49 | 0 | 0 | 20.4 | 344.3 |
| Import / plan-import | imp | 584 | 43 | 0 | 0 | 21.3 | 325.5 |
| Export / plan-export (explicit plan) | exp | 26,894 | 50 | 0 | 0 | 848.9 | 8429.5 |
| Packs / packs (empty) | pak | 587 | 36 | 0 | 0 | 19.5 | 326.0 |
| Create pack / build (preview) | bld | 720 | 45 | 0 | 0 | 22.7 | 382.4 |
| Texture optimization / textures | tex | 614 | 41 | 0 | 0 | 21.0 | 340.4 |
| Games / env | env | 630 | 48 | 1 | 0 | 21.5 | 352.7 |
| Settings / settings | set | 1,091 | 131 | 0 | 0 | 33.5 | 603.4 |
| Feedback / feedback | fbk | 637 | 45 | 0 | 0 | 21.8 | 350.7 |
| Background tasks / tasks (empty) | tsk | 574 | 38 | 0 | 0 | 21.9 | 323.1 |
| Full details / full (Core) | ful | 650 | 46 | 0 | 0 | 21.3 | 347.5 |
| Add game guide / guide (step 1) | gui | 536 | 39 | 0 | 0 | 14.8 | 304.9 |
| Uninstall / uninstall (preview) | del | 522 | 30 | 0 | 0 | 14.0 | 297.6 |

| Route | A11Y-1 | A11Y-2 | A11Y-3 | A11Y-4 | A11Y-5 | A11Y-6 | A11Y-7 | A11Y-8 | A11Y-9 |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| home | F | P | F | F | F | N | F | N | N |
| plan | F | P | F | F | F | N | F | N | F |
| library | F | P | F | F | F | N | F | F | F |
| needs | F | P | F | F | F | N | F | N | N |
| log | F | P | F | F | F | N | F | N | N |
| changes | F | P | F | F | F | N | F | N | N |
| subscribe | F | F | F | F | F | N | F | N | N |
| search | F | P | F | F | F | N | F | N | N |
| plans | F | P | F | F | F | N | F | N | N |
| plan-new | F | P | F | F | F | N | F | N | N |
| plan-import | F | P | F | F | F | N | F | N | N |
| plan-export | F | P | F | F | F | N | F | N | N |
| packs | F | P | F | F | F | N | F | F | N |
| build | F | P | F | F | F | N | F | N | N |
| textures | F | P | F | F | F | N | F | N | N |
| env | F | F | F | F | F | N | F | N | N |
| settings | F | P | F | F | F | N | F | N | N |
| feedback | F | P | F | F | F | N | F | N | N |
| tasks | F | P | F | F | F | N | F | N | N |
| full | F | P | F | F | F | N | F | N | N |
| guide | F | P | F | F | F | N | F | N | N |
| uninstall | F | P | F | F | F | N | F | N | N |

| Route | A11Y-10 | A11Y-11 | A11Y-12 | A11Y-13 | A11Y-14 | A11Y-15 | A11Y-16 | A11Y-17 | A11Y-18 |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| home | U | F | F | U | U | U | F | U | P |
| plan | F | F | F | U | U | U | F | U | P |
| library | U | F | F | U | U | U | F | F | P |
| needs | U | F | F | U | U | U | F | U | P |
| log | U | F | F | U | U | U | F | U | P |
| changes | U | F | F | U | U | U | F | U | P |
| subscribe | U | F | F | U | U | U | F | U | P |
| search | U | F | F | U | U | U | F | U | P |
| plans | U | F | F | U | U | U | F | U | P |
| plan-new | U | F | F | U | U | U | F | U | P |
| plan-import | U | F | F | U | U | U | F | U | P |
| plan-export | U | F | F | U | U | U | F | U | F |
| packs | U | F | F | U | U | U | F | U | P |
| build | U | F | F | U | U | U | F | U | P |
| textures | U | F | F | U | U | U | F | U | P |
| env | U | F | F | U | U | U | F | U | P |
| settings | U | F | F | U | U | U | F | U | P |
| feedback | U | F | F | U | U | U | F | U | P |
| tasks | U | F | F | U | U | U | F | U | P |
| full | U | F | F | U | U | U | F | U | P |
| guide | U | F | F | U | U | U | F | U | P |
| uninstall | U | F | F | U | U | U | F | F | P |

A11Y-2 passed its name-only check in 20/22 states; A11Y-18 passed measured size/transfer subchecks in 21/22. Overscan bounds are supported by source, but viewport capacity was not independently swept. A11Y-17 failed in both .NET-client samples (the COM client sees the page, gap 1); other cells remain U. A11Y-13/14 are U throughout because a complete action matrix was not executed. Base-state N under A11Y-6 is superseded by the failing interaction samples below when those modal components are opened.

### Interaction observations

| Sample | Observed result | Criterion result / evidence |
| --- | --- | --- |
| Editor `2`, End, Ctrl+Space, Esc, Home | Footer reached `Enabled 485 / 485`, then `1 selected`, then `1 / 485`; zero selected AX rows; focus remained the web document | A11Y-4/9 F despite working navigation; `editor-end.ax.json`, `editor-selected.ax.json`, `editor-home.ax.json` |
| Inventory End, Home | Reached `1353 / 1353`, then `1 / 1353`; virtual content changed without row/cell/selection structure | A11Y-9 F despite working boundary navigation; `library-end.ax.json` |
| Inventory `s`, Esc | Named column-action buttons appeared and first button received focus; Esc closed the menu and returned to the document | A11Y-7 F: no named menu/menuitem boundary; partial recovery success; `library-columns-menu.ax.json` |
| Settings language: Enter, Down, Esc | Name `language`, value `follow system`, expanded false→true→false; active descendant moved while committed selection stayed unchanged; focus stayed on the same control | These A11Y-4/7 subchecks pass for `choices.js`; confirmation, disabled skipping and typeahead were surveyed but not executed; `choice-*.ax.json` |
| URL prompt with synthetic value and no-op save callback | Named modal dialog, named textbox and alert, initial input focus; after six Tabs focus reached sidebar `start`; next Esc left the prompt open | A11Y-6/15 F; 22 shell buttons remained eligible for focus; `url-dialog.ax.json`, `url-after-tabs.ax.json`. `url-closed.ax.json` records the attempted close, not successful dismissal |
| General `popups.confirm` warning with no-op callback | No dialog/alertdialog; initial focus stayed on background language control; Tab moved in the background; Esc dismissed it | A11Y-6 F; `general-confirm*.ax.json`; production renderer, no destructive operation |
| Failure through `state.say` | Ordinary text, no status/alert for that failure; appended undo anchor has no href/role | A11Y-12 F; `failure-status.ax.json`; renderer test, not a real failing scan/build |
| Ctrl+K, then three Esc | Named/focused command input and status footer; popup dismissed and editor cursor retained | Partial A11Y-15 success; A11Y-7 F for generic result collection; `command-panel.ax.json`, `recovery-three-esc.ax.json` |
| Native uninstall preview and library (.NET client; see the correction in gap 1) | Only one UIA Window, `RimArcho`, class `WailsWebviewWindow`, not offscreen; Window/Transform patterns only; zero descendants or Invoke/Value/Selection patterns | A11Y-17 F; traversals 61/62 ms; `uninstall-native.uia.json`, `library-native.uia.json`. Native row names/counts/states could not be compared |

### Coverage limits and unreached variants

All 19 PREFIX entries in `refnums.js:21` were reached, plus the editor, Search files (fallback `sea`) and New list form. Export used the actual plan ID. Full details opened Core from the editor.

Unmeasured variants and reasons:

- Pack detail, Mods in a built pack, unfinished-build progress and launch confirmation: no packs/builds in the fixture; building or launching was outside the read/preview run.
- Guide steps 2–3 and first-run with zero environments: would add a game, scan or need another home. Existing homes were used in place and subprocesses refused.
- Populated task rows/items, queued/failed/cancelled work, download byte progress, scan/analysis/build events, slow-read retries and actual live-region delivery: tasks empty, network/background commands disabled. Source surveyed; full timelines not run.
- Populated Player.log/monitor results, search matches/source viewer, expanded update/source details and deep Def/Patch/Harmony pages: sampled empty/base states and Core summary only; source-search/decompilation and full drill-down matrix not run.
- Native file/folder pickers, account windows, native pattern/keyboard operation, locale changes, narrow layouts, cold-start AX cost, full memory benchmarks, event subscriptions and other desktop sessions: not exercised. CDP input is WebView keyboard input, not proof of desktop input or UIA event delivery.
- Refnums overlay enabled/renumbered: source surveyed, not toggled. Include it in later budget/decorative-overlay checks.
- Archo, Unplayed, Worth Playing and save management: 06 §2 says these destinations are hidden/not implemented; no reachable page to measure.

## Frontend survey

Paths in this table are relative to `internal/surface/gui/assets/` unless fully qualified. References name the inspected snapshot; scratch retains hashes.

| Pattern | Evidence and implication |
| --- | --- |
| Choice semantics work | `js/choices.js:28` supplies roles/names; `:43` expanded state; `:54` checked state; `:107` named listbox; `:112` options and selected/disabled states; `:135` relationships; `:153` keyboard handling. CDP confirmed value, active descendant and cancellation. `js/screens.js:1641` associates setting descriptions. Preserve these patterns. |
| Address/icon buttons work locally | `js/fields.js:23` creates copy buttons with full-path names and Enter/Space handling; `:45` gives editable addresses roles/names/keyboard picker actions. `js/screens.js:1676` names an icon-only external-link button. Icon-only is not inherently unnamed; check computed names/descriptions, including title fallbacks. |
| Splitter semantics work, names collide | `js/separator.js:17` exposes disabled/min/max/current values; `:33` exposes separator/orientation/name; `:38` handles arrows/reset. But 5–7 splitters share the same resize instructions and lack named affected-panel context (A11Y-5). |
| Retained editor contaminates pages | `index.html:29` makes the editor the only main; secondary pages live in sibling `#screen` at `:44`. `js/screens.js:63` opens the screen without hiding/inerting the board. Trees retain editor rows, filters and actions. Every sampled main and both navigation landmarks were unnamed. Breadcrumb text does not identify the active content boundary (A11Y-1). |
| Navigation state is CSS-only | `js/shell.js:221` uses `.on`/`.parent` for current/parent state without corresponding current/expanded semantics; `:330` renders unnamed breadcrumb navigation. This fails shared A11Y-4 even where local choice states work. |
| Editor virtualization lacks collection semantics | `js/lists.js:208` bounds mounted rows; `:219` creates generic draggable divs; `:222` stores a positional `data-i`; `:228` encodes cursor/selection in CSS. No list/option relationship, set size, per-item position, active descendant or selected state. Home/End and footer counts help navigation but do not establish node identity/state (A11Y-4/9/16). |
| Inventory headers lack table context | `js/library.js:291` bounds rows; `:312` creates generic `.lib-row` and span cells with CSS selection/click handlers; `:345` creates a clickable disclosure span. `js/datatable.js:84` adds columnheader and aria-sort. The sampled inventory had 5 columnheaders but **0 table/grid, row or cell nodes**. |
| Other tables are partly semantic | `js/packs.js:44`, `:58`, `:89`, `:137` add row/table roles. Span cells and shared headers still lack a complete hierarchy: empty Packs exposed 1 table and 5 columnheaders without a header row. Task items and textures also use `createTable`; populated variants remain unmeasured. |
| Variable-height helper is bounded, not a complete accessibility layer | `js/virtualrows.js:75` mounts a viewport window with roughly six rows’ margin each side; `:94` updates selection only when callers already supply treeitems. It does not establish collection role, total or absolute position. |
| Missing/meaningless names | `js/subscribe.js:128` creates an unlabelled Add subscription input; `js/screens.js:1664` creates the unlabelled editable Games field. `js/filterbar.js:42` uses `×` plus title `clear`: CDP still names two Search files controls `×`. `js/fields.js:132` makes identifier values copy buttons; inventory exposed a button named only `3000000000`. An identifier is useful content but does not explain its action. |
| General confirmations fail dialog focus/semantics | `js/popups.js:12` and `:219` create generic confirmation layers without modal semantics or focus handling. The RAM-specific variant adds dialog attributes at `:514`, which does not repair general confirmations. `js/screens.js:1790` names the URL dialog and restores its opener but does not contain Tab/inert background; focus escaped in the sample (A11Y-6/15). |
| Menu/palette actions need collection semantics | `js/popups.js:382` implements menu keys/Esc using a generic container and plain buttons. `:48` creates generic option rows. `js/command-panel.js:24` names close/input, `:27` supplies a status footer, `:115` restores/focuses input; results still lack collection/item/selection semantics. |
| Shared feedback is not live semantic feedback | `index.html:46` and `js/state.js:83` use an ordinary status span and an undo anchor lacking href/role. `js/activity.js:1` reports input to Go for memory policy; it does not announce accessibility status. `js/tasks.js:299` renders CSS task states; `:335` uses width-based progress; `:462` puts download bytes in title without a progressbar. Readable text in a later dump is not event delivery (A11Y-11/12). |
| Existing busy/live patterns to preserve | `js/screens.js:157` sets/clears page-read busy state; `js/motion.js:183` labels skeleton status; `js/filesearchtree.js:16` makes summaries live; `js/planmanage.js:531` has import status/alert; `js/home.js:517` supplies launch-wait progress/busy; `js/packs.js:532` makes output live. These local successes do not establish lifecycle delivery everywhere. |
| Hover/tooltip and decorative art need auditing | `js/fulltitle.js:18` adds titles for clipping; full DOM text often remains readable, so truncation alone is not failure. `js/badgepop.js:10` supplies tooltip semantics; `:96` wires mouse hover/pinning and promises equivalent Enter details in its comment. Verify content parity before calling it pointer-only. `js/icons.js:2` emits decorative SVGs without hiding them: sampled trees exposed 81–96 image nodes, many unnamed. Preserve informative status/source meaning as text/description, then hide redundant art. |
| Drag alternatives already exist | `js/separator.js:38` has arrow/reset keys; `js/datatable.js:58` offers named move-left/right, width/reset and visibility actions alongside dragging. `js/main.js:579` and `:625` route list movement/drag to common operations; 06 §8.5 lists their keys. No blanket drag-only failure was established; complete action coverage remains U. |
| Refnums identify locations, not immutable records | `js/refnums.js:21` defines prefixes, `:33` signature inputs, `:157` strips digits/uses sibling position, `:179` draws separate frames, `:202` persists signatures and `refnums/<prefix>.txt`. `window.refnum()` helps locate current DOM elements. This adds no stable record identity/action role to target AX/UIA nodes; row positions can change on recycling/sort. Refnums cannot substitute for A11Y-16. |
| Export is unbounded | `js/planmanage.js:984` inserts the entire serialized export into a `pre`: 26,894 exposed nodes, 8,631,804 raw dump bytes and 533.1–848.9 ms per tree call for 485 Mods. Copy/save actions do not bound preview exposure (A11Y-18). |

### Differences from current documentation

- 06 §3.10 agrees with the inspected choice implementation and tested dropdown. It does not describe every other popup/menu.
- 06 §3.1 rules 5/7 distinguish product dialogs from player-opened temporary layers. This does not settle accessibility roles: modal warnings need a modal focus boundary; a nonmodal command panel needs a named collection and dismissal route.
- Rule 6’s keyboard-parity wording is historically scoped to the TUI while referencing GUI tests. Operation reachability tests do not prove native UIA or DOM focus behavior. The proposal below makes GUI coverage explicit.
- 06 §8.2’s virtual-row description is directionally correct with bounded overscan, but says nothing about accessible positions/totals and does not cover the unbounded export preview.
- 06 §10/11 specify load/input/memory targets and graphical scenarios, not native semantic parity or tree budgets. Baseline failures here are against the proposals, not an already adopted accessibility acceptance table.

## Gaps in blocking order

1. **~~Native UIA exposes no content controls (A11Y-17).~~ Corrected the same day: this was the .NET client, not the window.** The baseline's walker was .NET Framework `System.Windows.Automation`. The COM client (`CUIAutomation` / `CUIAutomation8`), which desktop agents and screen readers use, sees 533 elements on the library and 516 on settings in the unchanged build. The page sits in WebView2's own child window `Chrome_WidgetWin_1` ("RimArcho - Web content", msedge.dll provider, WebView2's process). Names, the language dropdown's value and expanded state agree with CDP; `InvokePattern` and `ExpandCollapsePattern` work. A `WM_GETOBJECT` bridge returning `ICoreWebView2CompositionController2.get_AutomationProvider` (slot 11) was built and measured: every COM call returned `S_OK`, but neither client's tree changed (COM 533 → 533; .NET 0 → 0), so it was not merged. Evidence and the patch: `D:\RimArcho-test\uia-provider\` (`REPORT.md`, `bridge.patch`). Remaining A11Y-17 work: compare a dialog and list rows; columnheaders appear as UIA DataItems because they have no table (A11Y-8).
2. **Active-page boundaries and inactive content are wrong (A11Y-1/3/4/5).** Secondary pages expose the editor and its only main landmark; navigation state is CSS-only. Name the active content, hide/inert inactive pages and expose current navigation to reduce wrong-target risk everywhere.
3. **Lists/tables do not identify records, selection and cells reliably (A11Y-4/8/9/16).** Add coherent collection/row/cell semantics, filtered totals, absolute indices, current/selected state and record identity while retaining virtualization. The footer is useful but insufficient.
4. **Modal and popup focus is unreliable (A11Y-6/7/15).** General confirmations lack a dialog boundary/initial focus; URL prompts allow sidebar focus. Apply a shared modal focus/restore contract and separate nonmodal collection semantics; preserve working dropdown cancellation.
5. **Export exposes tens of thousands of text nodes (A11Y-18).** Bound/page its preview while retaining accessible full-payload copy/save and CLI/MCP export. Measure tree size as well as mounted rows.
6. **Async outcomes/errors lack a common accessible delivery path (A11Y-11/12).** Reuse existing status/alert/busy patterns, expose measured/indeterminate progress and retain identifiable outcomes. Verify all event paths with synthetic timelines.
7. **Some actions remain unnamed, meaningless or ambiguous (A11Y-2/5/10).** Label subscription/Games inputs, name clear/copy-ID actions by intent, identify affected splitters and associate repeated setting actions with fields. Audit icons and title-only facts.
8. **Target stability and pointer alternatives need complete verification (A11Y-13/14/16).** Keep refnums diagnostic, expose record-aware targeting and execute the full action matrix.

## Proposed specification text — not adopted

### Proposed Accessibility block in 06 §11

> GUI accessibility is accepted when an agent can complete the page/overlay scenarios using only the native Windows UI Automation tree and UIA patterns or keyboard input. Screenshots, coordinates, hover, DOM queries and private JavaScript state are outside that agent’s observation/action channel. CDP AX checks support development and must agree with native UIA for required semantics.
>
> Test every implemented §2 destination and secondary details/import/export/guide state, dialogs, menus, and task success/failure/cancellation with synthetic fixtures including large virtual lists. Verify named active page identity, control roles/names/states, readable content/feedback, accurate virtual totals/positions, stable record targeting, full keyboard/pattern coverage and bounded focus/recovery. Missing native controls, unnamed actions, unreachable items, silent failures or pointer-only actions fail acceptance. Record unmeasured variants; they do not count as passed.
>
> Preserve virtualization. At 1440×880 with the large synthetic fixture, the proposed budget is at most 2,000 non-ignored AX nodes, 2 MiB raw full-tree JSON and 200 ms for each of three warm full-tree calls, including export/previews. Check bounded mounted-row overscan and existing §10 resource limits separately. Use this research’s criteria as the review checklist until accepted requirements move into their owning specification sections.

The adopted specification should own the requirements and link to dated measurements, avoiding two maintained normative copies.

### Proposed additional rule in 06 §3.1

> **GUI actions and feedback must be understandable and operable from the accessibility tree.** Use semantic controls with names, roles, values/states and enough context; identify the current page and exclude inactive pages. Every pointer action has a discoverable keyboard or UIA equivalent. Virtual collections expose totals, positions and record identity without mounting every item. Modal prompts contain focus and return it on close; nonmodal layers expose their contents and dismissal route. Background work/errors expose readable status, progress and durable outcomes. Refnums aid element identification but do not replace this contract. Apply keyboard parity to GUI and TUI explicitly.

## Open decisions

- Is native UIA mandatory for the first release of accessibility support? This proposal says yes for desktop agents; CDP-only development operation is a separately labelled capability.
- Missing UIA content: answered, see gap 1 (a .NET client limit; COM UIA sees the page). No window change needed.
- Which stable record reference should the provider expose, and in which UIA/CDP property, while keeping names readable? Backend DOM IDs, generated `choice-*` IDs and positional refnums are unsuitable persistence contracts.
- Are 2,000 nodes /2 MiB /200 ms the right initial VM budgets? They cover normal sampled pages but are new proposals. Validate populated long logs, large findings and exports before adoption.
- Should shortcuts use accessible descriptions, shortcut properties or named help routes? Keycaps often become computed-name content; avoid obscuring intent or duplicate announcements.
- Which outcomes must survive navigation or require acknowledgement? Tasks already have durable history; incidental page reads and copy feedback need lighter treatment.
- Native picker/player-only account accessibility needs a separate fixture/procedure respecting 05 §1.2; these windows were not exercised.

## Evidence and validation

Scratch collectors: `capture.mjs`, `scenarios.mjs`, `uia.ps1`. `baseline.json`/`capture.log` hold canonical counts/timing; each route has `.ax.json` and `.dom.json`; `scenarios.json` records focus/key observations. `provenance.json`, `main.go`, `overlay.json`, host logs and `cleanup.json` preserve setup/termination evidence. These are local evidence paths, not new product interfaces. No optional repository checker was added.

Covered all 22 listed base states and eight representative interaction groups. Full action coverage, native pattern invocation and async timelines remain unmeasured as listed above. No frontend/Go/spec/changelog/release-test file was edited and no commit was created. Go/VM regression suites were not run for this documentation-only investigation; no behavior change is represented as implemented.

Scratch after cleanup: 29088939 bytes (27.74 MiB), 84 files, including the supplied brief. The executable and duplicate contextual-export dumps were removed; canonical evidence and rebuild inputs were retained. This is below the 2 GB cap.


## Fix round (2026-11-02, later)

A1, A6, A8 and A9 (with A4 for lists and A15) were fixed in two parallel changes and reviewed together; 06 §11 holds the updated status.

- **Page identity (A1):** `#board` and the plan header are inert while `#screen` shows; the screen is the only main region, named by the last breadcrumb (or the current tab or sidebar entry); the sidebar (`pages`) and breadcrumb (`breadcrumbs`, the one new language key) are named navigation with `aria-current`. COM UIA on the library: 536 → 399 elements.
- **Dialogs (A6, A15):** one modal contract in `popups.js` for confirmations, the RAM warning and the URL, rename and note prompts; uninstall keeps its own. A confirmation starts on the dialog itself so Enter keeps confirming, as its key cap shows (06 §6); the first version put focus on Cancel and made Enter cancel, which review reverted. `ctrl+\` (reference numbers) passes through the dialog's key capture. A keyboard-opened confirmation: COM UIA 387 → 23 elements, focus on the named dialog, no focus ring.
- **Lists and tables (A8, A9):** editor columns are listboxes with options (`aria-setsize`, `aria-posinset`, `aria-selected`, per-record ids, the cursor as `aria-activedescendant`); the library is a treegrid with `aria-rowindex` / `aria-rowcount`, levels and expansion; `datatable.js` gives tasks, packs, pack Mods and textures header rows, rows and cells. Review named grouped copies `<Mod>, <copy>` (they read only `└ <copy>`), kept a row's visible note (not installed) as its description, and dropped table names taken from the breadcrumb (stale across page switches, and the same name as the main region).
- **Visual parity:** screenshots of the 1.0.0 build and the change at 1440×880 match except GPU-raster edges on the editor's preview image and shadows (≤ 2 RGB levels), and the list splitter's focus highlight, which 1.0.0 showed behind a dialog because focus stayed there.
- **Checks:** the guidev `dialogcheck`, `pagecheck` and `esccheck` had gone stale against the current UI (they failed on 1.0.0 too). They were updated; checks the first update dropped were restored (share-code and collection imports finishing, the status undo after a history checkout, the new-list breadcrumb, no popup from the guide's import). `pagecheck` also exposed a bug on create a pack: counting textures removed the textures choice; fixed (`packs.js` removes only the step's own buttons).
- **Limits:** native checks ran in the sandboxed host with the synthetic 1,354-Mod home; Narrator, real data and the VM are pending (release-vm-tests). Treegrid rows reach UIA as tree items whose `PositionInSet` counts mounted siblings; agents read the absolute row from the cells' grid row.

## Fix round 2 (2026-11-03): results and progress

A11, A12 and the undo of A3, done by Codex (`task_247a598f33`) and reworked in review; 06 §11 holds the status.

- **Status line:** `#status` is the one live region: `status` (polite) for results and information, `alert` (assertive) for `fail`, `aria-busy` while `busy`. The busy text is itself the progressbar, valued from an `n / m` count it shows. The same kind, text and undo again keeps the nodes, so nothing is delivered twice. Undo is `role="button"` with `tabindex="-1"`. No hidden copy of the text.
- **Progress:** `feedbacksemantics.js` `progress()` names a bar by visible text and sets value and range, or none while unknown (never a false 0%). Covered: task rows and summary (the shown `n / m` as value text), download bytes (the `title` text as value text), pack builds (named by the pack card's name or the build page's "creating pack …" line), texture conversion, the guide's download, SteamCMD query and scan, the inline import and pack-cost waits, page skeletons (`role="progressbar"`, was `status`). No progressbar inside a button: the launch button is `aria-busy` while starting, the tasks button already reads "… 50%".
- **Field errors:** `linkError` / `fieldError` set `aria-invalid` and add the error node to `aria-describedby` (other descriptions stay); `fieldStatusError` ties a field to a refusal said on the status line until the next edit or message. The 17 surfaces: URL prompt, new-list name, rename dialog, inline rename, inline import (text and file), shared import (paste or file, whichever failed; destination name), settings text fields (now named by their label), Steam account name, collection follow, add subscription (described by "Not recognised" or the dry run that cannot go, not the whole card), pack name, the guide's collection link and pack chooser (whichever failed), command panel (`exit N` or its error). The file search query already had it.
- **Ordering:** rescan and check the mods' files show their busy line before the reply (`state.busyStart`); a refused start takes back only its own line. A pack build or texture run whose end event comes before its reply keeps that outcome. Late task reads, older subscription preview failures and guide events after the scan or download ended are dropped.
- **Review change:** Codex tied each status message to a ticket that any later message invalidated. Since the next key clears the status line (06 §2.2), pressing a key during a pack build or texture conversion would have dropped its end, failures included, and a scan or analysis started by the backend after an earlier one would never have shown. Ends now always land: they name their own work. Only a reply that arrives after its own operation ended is dropped.
- **Messages without a subject:** A12 was relaxed the same day. A direct reply to the action just taken (`done`, `opened the page`, `opened the folder`, `opened the Workshop page`, `could not copy`, `draft discarded`, `exit N`) needs no subject: the agent reads it at once and keeps it in its context, as a sighted player does. Only work that ends on its own must name its job and stay on the tasks page. Successes already did (`scan complete`, `analysis done`, `created pack <name>`); the failures of the scan, the files' check, a pack build and texture conversion carried only the reason and now start with the job as a task row does (`gui.failedText`; the task record keeps the bare reason). Texture cleanup said nothing when it ended and now reports through its task record (`TestBackgroundEndsNameTheirJob`).
- **UIA notes:** a field's linked error is its FullDescription (30159); HelpText (30013) keeps the placeholder. An indeterminate progressbar still offers RangeValue (0, 0–100) in WebView2; its Value (30045) is empty and AriaProperties has no `valuenow`.
- **Evidence:** guidev `pagecheck` (64 + 9 feedback cases), `esccheck` (31), `dialogcheck`, `gapcheck`; sandboxed native host with the synthetic 1,354-Mod home: 29 states through CDP and COM UIA, screenshots against dev/1.0 differ only by run-to-run edge noise (≤ 6 RGB levels, one button edge); scan and analysis start in 24–36 ms with their refusal 45–50 ms later, a pack build starts in 4 ms with its refusal at 62 ms. Not reached: real downloads, game launch, Narrator speech, every settings subtype; scratch in `D:\RimArcho-test\a11y-fix\C`.

## Relaxed criteria (2026-11-03)

The rows were first written as if for a screen reader user. An agent reads the tree after each action, keeps what it read, understands a control from where it sits in the tree, and does not mind an extra key press or a few tens of milliseconds. Ten rows were relaxed to that; 06 §11 holds the new wording.

- **Merged into A13:** A3 (an action counts when a UIA pattern or a key performs it, whatever its role) and A14 (dragging, double-clicking and hovering are actions like any other).
- **Narrowed:** A4 drops "disabled with its reason" and read-only (a refusal says why; busy is A11). A5 fails a bare `×` or ID only where its row, field or panel doesn't say what it acts on; splitters may share a name. A10 fails only a fact hidden in a `title`, hover, color or image; unnamed decorative icons are noise. A11 allows 1 s for the start. A15 drops the count of Esc presses (G5 keeps it for players). A16 asks for names or ids unique in their list, not re-tests after a language change. A17 asks that UIA works on the measured pages; CDP may differ in detail. A18 exempts export, whose file agents get from `loadout export` / MCP `exchange_list`.
- **Kept as written:** A1, A2, A6, A7, A8, A9, A12 (relaxed earlier), A13.
