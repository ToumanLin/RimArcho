# TUI redesign: navigation bar and visual system (2026-11-09)

Status: **option A implemented** on branch `claude/tui-redesign-1a5eed` (16 §9.6). The maintainer chose A; questions 2–4 in §5 took the proposal's defaults: faint shift digits on the navigation row, `ui.tui.theme` auto / dark / light with detection, and the position on the status line. P8 below was partly wrong; see the correction in its row.

Why now: Linux players are asking for RimArcho, and the Linux plan (`2026-11-09_linux-support-plan.md` on `dev/1.1`) ships the TUI before the GUI there. The TUI has to look good and navigate like the GUI. The page-switch lag of shift+1–8 is a separate task (`codex/tui-page-lag`); this document covers looks and navigation only.

The mockups in colour, with a dark/light terminal switch and every screen of each option, are on the private Artifact page *RimArcho TUI Redesign* (https://claude.ai/artifact/3ALrFFzHE52wiXs86WkXNd). The excerpts below are the same mockups without colour.

## 1. Method

- Every "today" screen was rendered on `dev/1.1` at 09f8d108 through `model.View()` on a synthetic home (`internal/testkit`: game 1.6, 41 Mods, 26 enabled, Harmony disabled so that Can it run has a missing dependency), at 120×35 and 80×24, with 24-bit colour: the editor, Can it run `@`, Launch `!`, Mod library `^`, updates, the command palette, a text panel (packs) and a status sentence. The capture test was temporary and is not committed.
- Mockups were drawn cell by cell by a small layout script, so every line is exactly the terminal width. They use the compatibility icons. Every glyph they use was checked with `go-runewidth` (`EastAsianWidth: true`) to be one cell, except CJK names.

## 2. Problems today

| # | Problem | Where |
| --- | --- | --- |
| P1 | The top line holds seven things: brand, environment pill, list, can it run N, N unsaved, position and `L launch`. At 80 columns launch is dropped and the position is cut to `Enabled  1 / 2…`. | `view.go` `topBar` |
| P2 | No navigation. The pages behind shift+1–8 are invisible. On secondary screens the bar becomes `RimArcho › can it run`, and the environment, unsaved count and Can it run count disappear. | all secondary screens |
| P3 | Breadcrumbs differ by screen. Can it run, Launch and Mod library use `RimArcho › …`; the command palette and text panels keep the editor bar and append `› command palette` or `› packs` to the position. | `topBar`, `position` |
| P4 | The left edge moves. The filter bar, area headings, Mod library rows and updates headings start at column 0; editor rows, Can it run, Launch and panels at column 2. | `bar.go`, `library.go`, `changes.go` |
| P5 | Headings are faint `#4A4F5A` (`Disabled 16`, `details`, the filter placeholder): about 2.4:1 on Windows Terminal's `#0C0C0C`. Numbers, icons and explanations share that colour, so there is no hierarchy. | `theme.go`, `columns.go` |
| P6 | Enabled's cursor `▸` is drawn in the gap to the left of the row number, next to the Disabled column, far from the name it marks. | `columns.go` |
| P7 | The status icon `⨯` sits at the right end of the name column, away from the name, and at 80 columns on the screen edge. | `columns.go` |
| P8 | Mod library details wrap key hints between the key and its label (`ctrl+w` / `open the Workshop page`). **Correction:** the first version of this row also said the detail column breaks paths mid-word and that `Archo ID  #j5pq` contradicts 16 §9.1. Both were wrong: 16 §4.1 places Archo ID in details on purpose (§9.1's rule is about results and errors), and paths already break at `\`. Only the key-hint split was real; it is fixed (`keyLines`). | `library.go` |
| P9 | Updates has no blank line between its three areas, its counts are not in a column, and `list “default”  the last scan  Oct 1 10:00  a every mod` mixes a heading with a key. | `changes.go` |
| P10 | Launch puts everything in the top-left corner over 25 empty rows; `► launch the game  L` looks like any other line; `watch for:` is not on the label column. | `home.go` |
| P11 | Can it run headings read `1 breaks the game  2` (focus key, name, count). An item's key line has the same colour as its evidence line. | `needs.go` |
| P12 | Every command palette row ends in `command`; there are no groups; the input cursor is `_` while the filter bar uses `▌`. | `palette.go` |
| P13 | Text panels truncate long lines (`…an…`) instead of wrapping them, and their title appears only in the top bar's position. | `view.go` `panelLines` |
| P14 | At 80 columns the editor's hint line ends at `t stage`, losing `? all keys`, which 16 §2.2 keeps last. | `hintLine` |
| P15 | There is only a dark palette. Body text `#E6E6E6` is nearly invisible in a light terminal, which is common on Linux. | `theme.go`, 16 §7 |
| P16 | The status line's `● game running` uses U+25CF, which is East Asian Ambiguous: two cells in terminals that widen ambiguous characters. `TestIconWidths` covers only the icon sets. | `liveStatus` |
| P17 | `navKeys` matches the characters `!@#$%^&*` of the US layout. On German or French layouts shift+2 gives `"` or another character, and the page keys do nothing. Out of scope here; a follow-up. | `ops.go` |

Today's editor at 120×35 (top):

```text
RimArcho   main  1.6   list default  can it run 2  ⬤ 1 unsaved                              Enabled  3 / 26   L launch ►

/ search: words, tag:, author:, type:, has:, version:, -not  /                                                  \ filter
1 Disabled 16                    2 Enabled 26                               4 details
  ◉  Alpha Sidearms Collect…          1  ⬥  Core                            Log Publisher from HugsLib
  ▢  Better Sidearms Rework           2  ◉  HugsLib                         Dubwise
  ◉  Biotech Storage Addon       ▸    3  ▢  Log Publisher from HugsLib      ▢ local  supports 1.6  number 3
  ▢  Deep Storage Expanded …          4  ◉  Add Cube
  ▢  Dubs Sidearms Retextur…          5  ▢  Echoes of the Rim           ⨯   ✓ no problems found
  ◉  Harmony                          6  ▢  Weapons Dominator
  ◉  Industrial Storage Fra…          7  ◉  Progression: Attire             Archo ID   #j5pq
  ▢  Odyssey Storage Reborn…          8  ▢  [sbz] EPOE Retexture        ⨯   package    tester.m1
```

## 3. Shared visual system (every option)

This keeps 16 §7: no borders, one accent, mostly grey text, the cursor as a faint background with an accent name, and bold for identity only.

- **Grid.** Content starts at column 2. Columns 0–1 are a *slot*: the cursor `▸` in lists and the focus digit on headings, so digits, cursors and names line up. Side-by-side columns are 2 cells apart and each has its own slot, so no `▸` floats between columns. One blank line separates sections; there are none inside a section (§7 rule 8 stays).
- **Hierarchy.** Area headings use the secondary colour in bold, or accent bold when focused, with the count in faint after two spaces. Can it run headings keep their semantic colours. Faint is only for numbers, icons, placeholders and technical leftovers, never for headings.
- **Rows.** An editor row is `▸ 12 ◉ ⨯ name … ⬤`: a fixed status column follows the source icon, so names always start in the same column, and the unsaved mark takes the column's last cell. The cursor row has a faint background across its column and its name in accent bold.
- **Details.** Labels are faint and values use the text colour. Paths break at `\`, with continuation lines on the value column. There is no `#id` (following §9.1: the code changes, not the spec).
- **Status line.** The left side has the sentence, or the position when there is none (`Enabled  3 / 26`, as in the GUI, 16 §2.2). The right side has the running game, with the one-cell `⬤` instead of `●`.
- **Hint line.** Keys are bold in the text colour and labels secondary, with 3 cells between groups. When the line does not fit, items drop from the middle; `? all keys` always stays.
- **Palette.** The dark values stay as in §7, except faint `#4A4F5A` becomes `#5C6270` and secondary `#7C8290` becomes `#858B99`. A light set comes from the GUI light theme: text `#1C1C21`, secondary `#5F5F68`, faint `#8E8E97`, accent `#4F46E5`, pill `#E4E3FB`, focused row `#F0F0F4`, selected `#ECEBFB`, selected cursor `#DCDAF8`, success `#188A50`, warning `#B45309`, error `#D64545`. The setting `ui.tui.theme` is auto, dark or light. Auto asks the terminal through termenv (OSC 11 or `COLORFGBG`, both common on Linux) and falls back to dark when it cannot (Windows ConPTY, §9.2).

## 4. Navigation options

All three take their entries, labels, list and pack sub-entries and shift keys from `view.Nav`, the GUI's sidebar model. They leave out Saves (no shift key and no TUI page), drop the `RimArcho` brand from the bar (the GUI has none; the terminal title is set instead), and move the position to the status line.

### A. Two-row top bar (recommended)

- Row 1 has the eight entries. A faint digit before each is its shift key; the current entry is a pill; Can it run carries its amber count. The right end has the environment and version, and `background N` while tasks run.
- Row 2 is the *context row*. Pages with sub-entries (lists, packs, Mod library) show the sub-page switches there, as in the second row of the GUI's top navigation; other pages show their conclusion (Can it run). Its right end has the page actions (`w save  L launch`, `F fix 1  A recheck  l choose log`), like the GUI's `.page-acts`. The hint line keeps only the cursor object's actions.
- When the labels do not fit (below about 112 columns with full labels, or 90 with short ones), the other entries collapse to their digits and only the current one keeps its name: `1 2 3 [4 lists] 5 6 7 8`. Sub-entries that do not fit end in `+N`.
- Cost: one row more than today (17 editor rows at 80×24 instead of 18).

```text
  1 start  2 can it run 2  3 game log   4 lists   5 packs  6 mod library  7 texture optimisation  8 game      main 1.6
  all lists   default ►   Vanilla+   Combat run   Testing                              ⬤ 1 unsaved   w save   L launch

  / search: words, tag:, author:, type:, has:, version:, -not                                                   \ filter

1 Disabled  16                    2 Enabled  26                                 4 details
  ◉ Alpha Sidearms Collection T…     1 ⬥   Core                                   Log Publisher from HugsLib
  ▢ Better Sidearms Rework           2 ◉   HugsLib                                Dubwise
  ◉ Biotech Storage Addon         ▸  3 ▢   Log Publisher from HugsLib             ▢ local  supports 1.6  3rd in the list
  ▢ Deep Storage Expanded Tweaks     4 ◉   Add Cube                          ⬤
  ▢ Dubs Sidearms Retexture          5 ▢ ⨯ Echoes of the Rim                      ✓ no problems found
```

At 80×24:

```text
  1 2 3  4 lists  5 6 7 8                                             main 1.6
  all lists   default ►   Vanilla+  +2         ⬤ 1 unsaved   w save   L launch

  / search: words, tag:, author:, type:, has:, version:, -not           \ filter

1 Disabled  16                       2 Enabled  26
  ◉ Alpha Sidearms Collection Twea…     1 ⬥   Core
  ▢ Better Sidearms Rework              2 ◉   HugsLib
  ◉ Biotech Storage Addon            ▸  3 ▢   Log Publisher from HugsLib
```

Can it run, with the status and hint lines:

```text
  1 start   2 can it run 2   3 game log  4 lists  5 packs  6 mod library  7 texture optimisation  8 game      main 1.6
  default is not ready: 2 to handle before you start, 1 error group in the last …   F fix 1   A recheck   l choose log

1 breaks the game  2
▸ ⨯ missing dependency: Harmony
    required by Echoes of the Rim, Multi Analyzer Retexture, Vanilla Armour Expanded and 3 more
    ≣ declared by the mod  About.xml modDependencies
    space enable Harmony   I ignore it   c subscriptions
  ⨯ [sbz] EPOE Retexture needs tester.missing, which is not installed
    ≣ declared by the mod  About.xml modDependencies
…
  ✓ enabled Harmony and moved it before Echoes of the Rim   u undo                       ⬤ game running since 14:05  3
  ↑↓ choose   ←→ sections   esc back   ? all keys
```

### B. One-row bar: digit strip and breadcrumb

- A single row. The left end is the strip of the eight page digits, with the current page expanded into a named pill; then the sub-page switches or the position; the right end has the environment, can it run N, N unsaved and `L launch`.
- The page's conclusion and page actions stay in the body's first line and the hint line, as today.
- 80 columns has the same shape; sub-entries fold into `+N` sooner.
- Cost: no extra row, but pages are known only by number; a new player cannot tell that 5 is packs.

```text
  1 2 3  4 lists  5 6 7 8     all lists   default ►   Vanilla+  +2    main 1.6   can it run 2   ⬤ 1 unsaved   L launch

  / search: words, tag:, author:, type:, has:, version:, -not                                                   \ filter

1 Disabled  16                    2 Enabled  26                                 4 details
  ◉ Alpha Sidearms Collection T…     1 ⬥   Core                                   Log Publisher from HugsLib
```

### C. Side rail, ctrl+b to the top

- At 100 columns or more, a 22-column rail copies the GUI sidebar: the playing and library groups, sub-entries on a `⎸` tree guide, the current entry as a pill, and background tasks and settings at the foot. `ctrl+b` switches between the rail and option A's top bar, stored in `ui.tui.nav` like the GUI's `ui.gui.nav`. Below 100 columns option A's bar is used.
- The body's first row is option A's context row.
- Cost: at 120 columns the body has 98 columns, so the editor and Mod library detail columns shrink or disappear. `⎸` (U+23B8) must be checked in Cascadia Mono and common Linux fonts.

```text
 playing                all lists   default ►   Vanilla+   Combat run   Testing        ⬤ 1 unsaved   w save   L launch
 1 start
 2 can it run      2    / search: words, tag:, author:, type:, has:, version:, -not                             \ filter
 3 game log
                      1 Disabled  16              2 Enabled  26                         4 details
 library                ◉ Alpha Sidearms Collec…     1 ⬥   Core                           Log Publisher from HugsLib
 4 lists                ▢ Better Sidearms Rework     2 ◉   HugsLib                        Dubwise
   ⎸ all lists          ◉ Biotech Storage Addon   ▸  3 ▢   Log Publisher from HugsL…      ▢ local  supports 1.6  3rd in…
   ⎸ default ►          ▢ Deep Storage Expanded…     4 ◉   Add Cube                  ⬤
   ⎸ Vanilla+           ▢ Dubs Sidearms Retextu…     5 ▢ ⨯ Echoes of the Rim              ✓ no problems found
   ⎸ Combat run         ◉ Harmony                    6 ▢   Weapons Dominator
   ⎸ Testing            ◉ Industrial Storage Fr…     7 ◉   Progression: Attire            package    tester.m1
 5 packs                ▢ Odyssey Storage Rebor…     8 ▢ ⨯ [sbz] EPOE Retexture           path       D:\Games\RimWorld\…
 6 mod library          ◉ Outer Storage Plus - …     9 ▢ ⨯ Multi Analyzer Retexture                  LogPublisher
 7 textures             ▢ Rimefeller Storage Ex…    10 ◉   Mod Options Sort               tags       none
 8 game                 ▢ Royal Storage Core        11 ▢   Tab-sorting                    languages  English
```

## 5. Open questions for the maintainer

1. Which option? A is recommended: every page name is visible, the second row matches the GUI's sub-pages and page actions, and at 80 columns it narrows to B's digit strip.
2. Should the bar show the shift digits? The mockups show faint digits. They look like the area focus keys 1–9 and differ only by position.
3. Light terminals: add `ui.tui.theme` (auto / dark / light) with detection? This changes the §7 sentence "The TUI has only a dark palette".
4. Move the position (`Enabled 3 / 26`) from the bar to the left of the status line, as in the GUI?
5. Anything else, such as a Launch page closer to the GUI's three columns, or command palette groups.

## 6. After the choice

Done as below, except P17. Answers taken: 1 A; 2 yes; 3 yes; 4 yes; 5 nothing more for now.

Rebase onto `dev/1.1` (with the page-lag fix), keep Bubble Tea and the existing keys, and in the same commits update 16 §9 (and §3 and §7 where touched), CHANGELOG and `UserGuide/release-vm-tests.md`. Check both icon modes and 80×24. P17 (non-US layouts) is a separate follow-up.
