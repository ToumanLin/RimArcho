# 16 · Interface · Visual design, motion, interface wording (§7)

> Part of [16 · Interface](README.md). Section numbers run through all of 16 and stay unchanged when split across files.

## 7. Visual design

**Principles** follow the refined minimalism of magpie and Linear:
1. **The TUI has no borders.** Blank space, two-cell indentation, and muted subheadings separate areas; there are no boxes or rules. The GUI uses magpie cards: a quiet base with hairline borders (`--hairline`, §7.3) and 12px corners.
2. **Use one accent.** The palette has an accent, success green, error red, and warning amber. The accent marks what the player is operating: the cursor, selected name, focused heading, or input prompt.
3. **Most text is gray.** Most characters use the secondary or faint color; only the object being operated is bright.
4. **The cursor uses a faint background and a changed text color; selection uses a faint accent background.** The cursor row has a faint background, and its name becomes accent-colored when focused; the TUI also adds ▸. Selected rows use a faint accent background. Solid backgrounds are reserved for pills, such as the current environment or focused filter. **Do not add a left vertical line or color strip to any box, row, or card to indicate selection, the cursor, or emphasis.** The TUI follows the same rule: selected rows have a full-row accent tint, with a deeper tint for the selected cursor row.
5. **Bold indicates identity only**, such as Mod names and headings. Do not use italics, underlining, or inverted colors.
6. **Hide developer terms.** Finding codes, #id, decl, and lineage belong only in technical information, the CLI, or MCP. The exception is the package ID, shown in details by default. See §7.2 for interface names.
7. **Keep the interface still.** The TUI has no animations or spinners; the status line uses … for work in progress. GUI motion only explains what changed or where something went (§7.1). It is brief and is disabled under `prefers-reduced-motion`. The one exception is Archo's figure while the player waits (§7.1 "Archo figure"): it moves continuously on one canvas, only during a wait, and becomes a still under `prefers-reduced-motion`, without hardware WebGL2, or when the player chooses still or off.
8. **Each TUI row occupies one line**, with no line spacing. Ellipsize overflow and state the hidden count, for example 12 more. Detail text wraps (§4.1).

**The TUI dark palette** uses the following colors:

| Role | Value | Use |
| --- | --- | --- |
| Text | `#E6E6E6` | Names, values, keys |
| Secondary | `#7C8290` | Explanations, hints, authors |
| Faint | `#4A4F5A` | Numbers, subheadings, unfocused content |
| Accent | `#A5B4FC` | Cursor, selected names, focused headings, prompt |
| Pill | `#2C2E4A` | Current environment or focused filter |
| Focused row | `#1A1C24` | Cursor in the focused area |
| Selected row | `#232539` | The whole selected row, including wrapped continuation lines |
| Selected cursor | `#303450` | The whole selected cursor row in the focused area, including continuation lines |
| Success | `#7EE2A8` | ✓ result |
| Warning | `#F2C46D` | Warning icons; magpie has no warning color, but Mod management must distinguish a failure from a possible problem |
| Error | `#FF8A80` | Error icons and failed results |

The TUI has only a dark palette and uses the terminal's background. ConPTY cannot reliably detect the terminal background, for the same reason as the color-detection limitation in §9.2, so the TUI does not automatically switch to a light theme.

**Log colors** (§2.10) are an exception to gray text. Raw log rows use the game's log-window colors: Error red `#ff6666`, Warning yellow `#ffeb04`, and Message white. A Mod message's `<color>` is retained. The light theme adjusts contrast without changing the hue. Interface text and icons retain the palette above.

**The GUI palette** comes from magpie's `app.css`, under MIT, with the copyright notice at the top of `style.css`. CSS variables define the colors. **The day/night control in settings > interface determines the effective theme** (§7.4). The dark values are background `--bg #1a1a1e`, card `--card #232327`, pill `--pill #2d2d33`, line `--line #333339`, text `--fg #ededf1`, secondary `--fg-2 #b9b9c1`, `--muted #8b8b94`, faint `--faint #56565f`, accent `--accent #a5b4fc`, soft accent `--accent-soft #2b2c47`, selection `--sel #33345a`, success `--green #6fd99b`, error `--red #f28b82`, warning `--amber #f2b544`, and log `--log #f5a97f`. The light values are in `style.css` :root, with accent #4f46e5. Rows are 28px high with 13px text.

**Card spacing:** all horizontal and vertical gaps between rounded cards use `--card-gap`, 4px, with no other value. This covers the editor columns; details, staging, and history in the third column; left filters and the list or details below them; Mod library filters and table; log and change-history columns; task columns and right-side cards; and All lists columns. **Gaps between cards sharing a fixed space are draggable** (§3.1 rule 10). Double-clicking resets them, and sizes persist in `ui.gui.<page>.*`. New cards of this kind must include splitters. Gaps between content-height cards stacked down the page are not draggable: Can it run sections, settings groups, and the six full-detail sections. A draggable gap remains visibly 4px wide, but its hit area extends `--grab`, 3px, into each card, for about 10px total. The resize cursor, the accent hairline on hover (§7.3), and double-click reset still apply. The sidebar's right-edge hit area is 6px, inside the sidebar.

**Fonts:** see §7.3 for sizes. The TUI uses the player's terminal font (§9.2). The GUI uses the system fonts Segoe UI Variable Text, Segoe UI, and Microsoft YaHei UI, with tabular numbers and Cascadia Mono for code.

**Wording** follows these rules:
- The Chinese UI uses short sentences beginning with a verb, for example enable Vehicle Framework rather than perform enable operation.
- Subheadings are short and muted, such as Enabled 485, description, and needs.
- An empty state has one sentence that identifies the key to use, for example staging is empty; t stages selected mods.
- Do not use middle-dot separators (§7.3).
- An unknown value appears as a faint — rather than the word unknown.
- Every UI string has a Chinese translation; GUI tests check every string used by the pages.

**Icons:** the TUI defaults to the compatibility Unicode characters below. With ui.icons nerd, it uses Nerd Font's Font Awesome 4 range, U+F000–U+F2E0, which all Nerd Fonts include (§9.3). The GUI uses simple custom SVGs in `js/icons.js`, with the same meanings.

| Meaning | Nerd Font | Compatibility | Color |
| --- | --- | --- | --- |
| Official source: Core and DLC | shield `U+F132` | `⬥` U+2B25 | Faint |
| Workshop including SteamCMD | steam `U+F1B6` | `◉` U+25C9 | Faint |
| Local | folder `U+F07B` | `▢` U+25A2 | Faint |
| Locked Workshop copy | clone `U+F24D` | `⬦` U+2B26 | Faint |
| Git | git `U+F1D3` | `∆` U+2206 | Faint |
| Error causing load failure | times-circle `U+F057` | `⨯` U+2A2F | Error red |
| Warning of a possible issue | exclamation-triangle `U+F071` | `!` | Amber |
| Corpus hint; counting follows §5 | info-circle `U+F05A` | `ⓘ` U+24D8 | Secondary; a circled i in the GUI |
| Last-run error | bug `U+F188` | `◍` U+25CD | Error red |
| Unloadable disabled residue | ban `U+F05E` | `◌` U+25CC | Faint |
| No problem | Hidden | Hidden | — |
| Different from the game's list | circle `U+F111` | `⬤` U+2B24 | Accent |
| Locked, Core immovable | lock `U+F023` | Hidden | Faint |
| Update or change | refresh `U+F021` | `⇣` U+21E3 | Secondary |
| Dependency | link `U+F0C1` | `›` U+203A | Secondary |
| Staging | inbox `U+F01C` | `◫` U+25EB | Secondary |
| Launch | play `U+F04B` | `►` U+25BA | Text |
| Save | floppy `U+F0C7` | `↲` U+21B2 | Text |
| Automatic sort | sort-amount-asc `U+F160` | `↨` U+21A8 | Text |
| Search | search `U+F002` | `/` | Secondary |

- A row has at most two icons: source and status. A status icon appears for a finding or corpus hint and is absent otherwise. In the GUI, hovering explains it and clicking pins the explanation (§5). The popover uses `--pop-bg`, a hairline, and `--shadow`; its text is selectable (§3.1 rule 13).
- The lock icon is not shown; the `official.order` finding expresses Core's position.
- Both modes share the cursor ▸, success ✓, prompt ❯, and progress … markers, all drawn in one cell. Failure uses times-circle in full mode and ⨯ in compatibility mode; Cascadia Mono lacks ✗.
- Compatibility glyphs have East Asian Neutral width N. Their width is 1 with either value of runewidth's EastAsianWidth, and default Cascadia Mono has a one-cell glyph for each. Nerd private-use icons and … have Ambiguous width A, so RimArcho must disable EastAsianWidth when measuring. A terminal that renders ambiguous characters in 2 cells, such as WezTerm with treat_east_asian_ambiguous_width_as_wide or some CJK consoles, will misalign Nerd icons; use compatibility mode there. `TestIconWidths` verifies that both icon sets occupy one cell.

**The app icon** is the player-supplied `internal/icon/portrait.svg`, a 1024×1024 vector with a #4f46e5 outline glow. It is also Archo's image (§8.6).

**The bow mark** uses the portrait's upper-left bow where the portrait would be too small to recognize. The GUI draws a simplified version through bowMark in `js/icons.js`: three wings and a knot, with a lower-left glow matching the portrait's lighting. bowMark(24) adds a light face to each wing; bowMark(16) uses solid shapes. The brand colors are fixed regardless of the chosen accent. In the light theme the glow is #4f46e5; in the dark theme it is #a5b4fc with a #4a4d5c gray outline so the black wings remain visible. It appears as a 14px mark, using the 16px drawing, before the Archo name in the game log's Archo log analysis card and on the ask Archo to analyze button. Archo task rows will use it too (§2.11). The portrait is reserved for larger displays such as the Archo panel. The sidebar has no mark (§2.1).

**Choice controls** follow §3.10. A selected segment uses `--seg-thumb` and `--seg-edge`, and unselected text uses `--ctl-fg`. A single-choice button has a right-side arrow. Its menu shows the field name at the top, a check to the left of the current option, and a rounded highlight. Keyboard focus has a separate background. The panel uses card colors, a hairline, and the menu shadow; both themes share the existing variables.

### 7.1 Motion (GUI)

Four durations are defined in `style.css` :root. Entry and movement use the ease-out curve `--ease`, cubic-bezier(.2, .8, .2, 1), with a 250ms base. Exits are faster and use `--ease-in`, cubic-bezier(.4, 0, 1, 1).

| Variable | Duration | Use |
| --- | --- | --- |
| `--t-press` | 120ms | Hover, press, color fades |
| `--t-enter` | 200ms | Page, menu, and status entry |
| `--t-exit` | 150ms | Menu close and card collapse, with ease-in |
| `--t-layout` | 250ms | Row movement, sidebar highlight, expansion, number pulse |

Menu closing and card-content collapse use the exit duration and ease-in curve. Layout movement and FLIP in `motion.js` share the 250ms timing.

The motions are implemented in `js/motion.js` and the motion section of `style.css`:
- **Page entry** fades in and rises 4px. A redraw of the same page after an action or subview switch has no motion.
- **The sidebar highlight** slides from the old entry to the new one. Its size changes immediately; only its position is animated. Children have their own highlight that slides within the same entry.
- **Sidebar children** expand their grid row from 0fr to 1fr using the layout duration, fade in using the entry duration, and rotate the arrow 90°. The old children collapse while the new children expand, and following entries move smoothly. The highlight slides directly to the final destination that motion.js computes from the expanding and collapsing children. Collapsed children are excluded from the tab order.
- **List changes** include enabling, disabling, moving, sorting, undoing, and dragging. Before an action, the visible Mod rows' positions are recorded. After redraw, surviving rows move from their old positions using FLIP. Affected rows flash a soft accent and fade over about 400ms; if the affected rows are unknown, moved or new rows flash. Only the rendered virtual rows, a few dozen, are processed, with one style calculation per batch.
- **Numbers** softly pulse on change: sidebar badges and counts, list headings, Can it run section counts, and the unsaved count.
- **The status line** slides each new sentence in from below.
- **Loading placeholders** shimmer in lazily loaded game logs, log details, and full-detail pages without loading… text. Screen readers retain the original text.
- **Cards and overlays** retain their existing slight scale and fade.
- **Archo figure** (`js/archo.js`, `js/boot.js`, assets in `assets/archo/`): Archo's chibi figure keeps the player company in three places.
  - **Start-up screen.** From 0.21 s after the page starts until the first page or the first-run guide has drawn, the window shows the figure centred, 176 px wide, with one usage tip below it. A start that finishes within 0.21 s shows nothing. The still is chosen by `boot.js` before `Init` from the cached setting; the tip and the live figure follow `Init`. A dialog at start (the Workshop install offer, the managed-folder question) removes the screen first.
  - **Loading cards.** `beginPageRead`'s placeholder card holds a 120 px slot on the left and, on the right, a tip line above three shimmer lines. The figure and the tip appear only after the read has taken 0.21 s; a faster read shows the plain placeholder. The tip changes every 8 s in a long wait. On the first-run guide's page, the tips are those for a new player.
  - **First-run guide.** A 156 px figure at the right end of the step bar, standing behind the step's card, whose top edge hides her below the shoulders (16 §2.15); shown at once. Its expression follows the guide: wink at step 1, ponder on the download way, think while a download or scan runs, salute when it is done, surprised at an error, smile at step 3.
  - Waits rotate the calm expressions neutral, smile, laugh, wink, think, ponder and yum, one per wait in turn (the turn is kept across starts), and a long wait moves to the next one every 4 s.
  - Every figure uses the model's `wide` framing (head to chest, her whole width plus a margin), so hair, ribbons and gestures are never cut at the sides; she ends only at the bottom, by a fade (start-up screen, loading cards) or behind a card (the guide).
  - Each figure first shows a still (one WebP, 360 × 291 px, about 25 KB). With `ui.archo.motion` live (§2.14), motion allowed and a hardware WebGL2 context (`failIfMajorPerformanceCaveat`), the live figure cross-fades over it. There is one live figure: a WebGL2 puppet created on first use from the small package (about 350 KB; 2.4 MB of textures; about 0.15 ms of CPU per frame at 24 fps). Its canvas moves to whichever figure is shown; a second figure at the same time stays a still and takes the canvas when the first ends. It draws only while visible (IntersectionObserver, `visibilitychange`) and is destroyed 10 s after the last figure ends, which frees its textures and context. If creating it fails, the stills stay for the window.
  - Figures fade in with `--t-enter` and out with `--t-exit`; the canvas cross-fades with `--t-layout`. Tips come from `view.Tips` (`Tips`), read once per window after the first wait.

The following rules apply:
- Continuous or moving animations change only opacity and transform so WebView2 can composite without relayout or repaint. A flash uses a fading overlay instead of animating the background color; shimmer and progress use moving gradient layers. The exceptions are one-off 120ms hover-color transitions and the few sidebar grid rows expanding once over 250ms.
- Do not use `backdrop-filter` blur; WebView2 repaints it on every frame.
- Under `prefers-reduced-motion: reduce`, CSS sets all animation and transition durations to 0; `motion.js` skips FLIP, flashing, pulsing, and sliding.
- Scrolling triggers no animations.

### 7.2 Interface wording

Both languages of the GUI and TUI use the player's wording. Code, the CLI, MCP, and specifications keep domain names such as plan, environment, and Instance. A plan in the specification is therefore a list in the UI.

English UI text is the translation lookup key, so changing it also requires changing the callers. Keys that app and modchanges supply in CLI wording, such as the current plan, keep that wording; `view.enText` overrides how the UI shows them. History operation names such as checkout, swap, and restore come from app operation codes and are mapped to player wording by stepWords.

`TestWording` in `internal/surface/cli` (`view.WordingProblems`) checks the language resources and command names. It fails if a Chinese text contains a term from the Avoid column below (its Chinese forms are the `@retired.` entries in the zh language resources, each with its replacement), or if the English contains plan, environment, upstream, workspace, swap, pinned, checkout, not written, light pack, or full pack (`retiredEn`). A key translated differently within one JSON file or across files also fails, including supplemental resources; identical duplicates are allowed. English interface overrides live in `view.enText`. Necessary exceptions are listed in `wordingExempt`, for example decompilation.

| Domain concept | UI wording | English | Avoid |
| --- | --- | --- | --- |
| Plan / loadout | Current / new list, list history | list | plan, workspace |
| Enabled portion | Enabled | Enabled | enabled list |
| Three editor lists | Areas: Enabled, Disabled, Staging | areas | list, ambiguous with plan |
| Add or remove a plan entry, the same in the editor and library | Enable / disable, disable in every list | enable / disable; disable in every list | add to current plan, take out, remove from plan |
| Staging / stage restore | Stage / put back | stage / put back | — |
| save | Save; the editor button says Save changes / no changes, the count says N unsaved, and versions are saved versions | save; save changes / no changes; N unsaved; saved versions | write to game, unwritten, not yet written, written versions |
| checkout | Go back to this version; history step go back | go back to this version | load version, load list |
| External ModsConfig changes | Use outside change | use the outside change | load into list |
| Environment | Game; `env remove` removes the game without changing files | game; remove the game | environment, delete environment |
| Source | Mod folders in settings, source in filters and details, such as Workshop or local | mod folders; source | — |
| Instance | This one, another copy, N copies | this one, another copy | instance, Instance |
| Lineage | Copies of same Mod | copies of the same mod | Lineage |
| Copy | Only copies made by RimArcho in the lock library or a pack | copy | calling original Workshop installation a copy |
| pin / unpin / pin --update | Lock this version / unlock / update the locked version; retain the old copy, and show locked version as the source | lock this version / unlock / update the locked version | pin, including other tools' pins |
| Upstream changed after lock | Newer version, use Workshop version | a newer version is out | upstream, original updated, use original; original means Core and DLC |
| library move-to-locked | Move into lock library | move into the lock library | collect into lock library, back up locked version |
| swap | Switch version… | switch version… | replace with another copy, replace instance |
| unhide | Unhide | unhide | restore to game |
| trash | Recycle | move to the recycle bin | delete, reserved for real deletion: lists, DDS, uninstall |
| Installed Mods | Mod library | mod library | inventory |
| Allowed game versions (07 §1.6) | Game versions / reset default | game versions; reset to default | allowed / compatible versions |
| plan follow / unfollow / sync (07 §7.6) | Follow collection, unlink, catch up; add new only / also disable removed | follow a Steam collection / unlink / catch up with the collection; add new only / also disable removed | subscribe collection, subscription only for Steam Mods; sync |
| Build pack | Create / recreate / unfinished pack | create a pack | compile |
| Full / light pack | A pack always includes files; list-only sharing exports a RimArcho list, an export format also available through the pack page's export list action | pack; RimArcho list | full pack, light pack |

### 7.3 Visual rules

These rules are for the GUI; each rule states any TUI exception. The checks are listed below.

Launch (§2.3) is an exception: its sizes, rules, and radii follow its own design and are exempt from rules 1, 3, and 4. It still has four font sizes: 12px, 17px, 26px, and a responsive headline.

1. **Use two radii.** --r-lg is 12px for cards, panels, and menus. --r-sm is 6px for buttons, inputs, chips, thumbnails, and other small blocks. Single-choice buttons and segmented outer frames use the large radius. Circles and pills using 50% or 999px are excluded. There is no third value; the stylesheet uses the radius variables throughout.
2. **Nested corners are concentric.** When an inner element is within 8px of an outer edge, its radius equals the outer radius minus the padding. This also applies to switch thumbs and menu options. A card-edge block has a 6px gap, so 12−6=6, consistent with rule 1. Blocks farther from the edge are exempt.
3. **Use hairlines only.** All borders, separators, and menu-group rules use --hairline: 0.5px at `resolution >= 2dppx`, or 1px at 100% and 150% scaling, in a color lighter than --line. At those scales, 0.5px would blur or disappear. Do not use lines of 2px or more. Splitter hover lines use an accent hairline; quotes use indentation and muted text without a line.
4. **Use four font sizes, with a minimum of 12px.**

   | Size | Use |
   | --- | --- |
   | 12px | Secondary text, tables, subheadings, sidebar groups and children, badges, chips |
   | 13px | Body text, rows, buttons, sidebar entries |
   | 15px | Breadcrumbs, page names, summary titles |
   | 32px | Launch headline only |

   Add hierarchy through weight, color, or spacing, without extra sizes. This rule does not apply to the TUI, which has one terminal font size.
5. **Do not use middle-dot separators.** Space parallel facts apart by 12px in the GUI or 2 cells in the TUI. Use commas in sentences, an en dash for ranges, and a slash or › for hierarchy. Hint groups have wider spacing: 16px in the GUI or 3 cells in the TUI. Specification titles are exempt because they are not UI text; UI examples in the specifications use two spaces to represent spacing.
6. **Do not add left vertical lines or color bars** (§7 principle 4). Quotes use 14px indentation and muted text without a line. The sidebar child tree guide remains because it does not indicate selection (§2.1).
7. **Omit explanatory filler.** Keep a sentence only if removing it would lose information the player needs now. The page heading (§2.2) contains the current status or conclusion, such as Main colony not ready: fix these 2 problems, and page actions. It does not explain the page's purpose. Safety text, such as no game-file writes, belongs in a confirmation or tooltip. An empty state remains one sentence (§7 wording).
8. **Use eyebrows only when informative.** Text above the main title supplies a category or context absent from that title. Launch has no type or game eyebrow, because its 01 and 02 areas already name them (§2.3). Do not add uppercase English labels. Heading numbers are useful only as the focus keys 1–9 (§3.5); omit numbers that have no corresponding key.
9. **Align optically.**
   - Center icons visually with the text in boxes as high as the row, rather than aligning to the baseline.
   - All numbers use `tabular-nums`; numeric columns are right-aligned.
   - Round icons are 1px larger than square icons on the same row.
   - Button padding is equal on both sides, except that the icon side has 2px less to center the combined icon and text visually.
   - Align icons, names, and right-side counts to the same edges throughout a column, as in the sidebar (§2.1).
10. **Motion** uses ease-out for entry and movement, with a 250ms base. See §7.1 for the values.
11. **Text must not overlap.** At any width, text stays in its own column, wrapping or ellipsizing with the full text on hover. Addresses follow §3.1 rule 12 and never cover adjacent text, inputs, or buttons. In narrow windows, right-side controls wrap below the explanation. The TUI truncates to the terminal width and likewise prevents overlap.
12. **Page width follows the page's shape.** Pages with multiple list and detail columns fill the window with 12px side padding and draggable widths that reset on double-click, like the editor's 38% / 31% / 31% split. These include the Mod library, game log (list 62%), Can it run (two columns, left 55%), All lists (list 38%), Search files (left 38%), Mod settings (table 55%), packs, subscriptions, and tasks. The single-column game, texture optimisation, and settings pages share a centered 960px reading width, so labels and values stay close. The guide, uninstall, build, and exchange pages are 1200px wide. Two `.page:is(...)` stylesheet rules implement these widths.
13. **Keys are key caps.** A key shown in a button, a dialog button or the hint line is a cap (`kbd`: code font, hairline frame, small radius), never plain text beside its label: confirm [enter ↵], not confirm enter. Several keys for one action are separate caps; enter shows ↵. Every GUI key label goes through `keyCaps` (state.js). The TUI keeps its bold key text.

**Checks** enforce the rules:
- The GUI TestStyleRules reads `style.css`. It permits only var(--r-lg), var(--r-sm), 50%, 999px, inherit, and the calculated inner choice radii; only the table's font sizes; and only var(--hairline), with 0 or none to remove a line. It reports the source line and invalid value. It prohibits border-left, border-right, or `box-shadow: inset` accent bars.
- TestWording (§7.2) also rejects middle dots in translation maps and `view.Text` (`view.WordingTextProblems`), including GUI literals. The approved Search files summary and UTF-8 file-type labels (§2.9) are narrow exceptions.

### 7.4 Accent themes (GUI)

- Settings > interface > accent (`ui.accent`, §2.14) offers default, which leaves the existing indigo values unchanged without overriding variables, and 15 Radix Colors 3.0.0 scales: indigo, blue, cyan, teal, green, grass, orange, amber, red, crimson, pink, plum, purple, violet, and iris. Each has 12 light <name> levels and 12 dark <name>Dark levels. The original values are in assets/js/accent.js with the MIT notice.
- **Only four accent variables change.** Success green, error red, warning amber, and grays retain their values, following §7 principle 2:

  | Variable | Light | Dark |
  | --- | --- | --- |
  | --accent, fill, border, and cursor | 9 | 11 |
  | --accent-soft, pills and cursor tint | 3 | 3 |
  | --sel, selection | 4 | 5 |
  | --accent-fg, text on fill | White; level 12 for amber | 1 |

  All 12 levels are also exposed as `--accent-1` … `--accent-12`. Accent text uses level 11, `var(--accent-11, var(--accent))`, which is clearer on white than level 9.
- **Day/night selection** uses `ui.theme`: auto is the default and follows the OS; light and dark are manual choices. Grays, log colors, and accents use the same effective theme. A manual theme ignores system changes. The local cached theme is restored before styles load to avoid a flash, then `Init.theme` synchronizes the database value. If local storage is disabled, startup data still supplies the theme.
- **Changes apply immediately.** Init includes accent, and the root inline variables are set before first paint. Changing a setting refetches and applies the values without reloading the page. Manual light or dark selection switches immediately; OS changes apply only with `ui.theme=auto`. Swatches show the actual effective --accent color. A ring and ✓ mark the selection, and hover and adjacent text show the name. Default removes the inline variables. The TUI retains the §7 palette.
- A known limitation is that white on level 9 in some light scales has contrast below 4.5:1, a Radix tradeoff. Buttons keep 13px medium text.
- `internal/surface/gui/accent_test.go` verifies that each choice has 12+12 valid colors in accent.js. New CSS rules also pass TestStyleRules.
