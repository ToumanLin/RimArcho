# 06 · Interface · Visual design, motion, interface wording (§7)

> Part of [06 · Interface](README.md). Section numbers run through all of 06 and stay unchanged when split across files. References still use `06 §x.y`.

## 7. Visual design

**Principles**, refined minimalism like magpie / Linear:
1. **No borders in TUI.** Areas separated only by blank space, two-cell indentation, muted subheadings, no boxes / rules. GUI uses magpie cards: quiet base, hairline borders (`--hairline`, §7.3), 12px corners.
2. **One accent.** Accent, success green, error red, warning amber only. Accent for what is being operated: cursor, selected name, focused heading, input prompt.
3. **Most text gray.** Secondary / faint for most characters; only current object bright.
4. **Cursor faint background / changed color; selection faint accent.** Cursor row faint, focused name accent, TUI adds ▸; selected rows faint accent. Solid backgrounds only pills, current environment / focused filter. **No left vertical line / color strip on any box, row, card for selection, cursor, emphasis** (2026-10-23: template-like, no information). TUI same, full-row accent tint, deeper selected cursor.
5. **Bold indicates identity only**, Mod names / headings. No italics, underline, inversion.
6. **No developer terms**: finding codes, #id, decl, lineage only in technical / CLI / MCP. Exception: package ID default details. Names §7.2.
7. **Still interface**: TUI no animation / spinner; … in status for work. GUI motion only explains changes / destinations (§7.1), short, all disabled with `prefers-reduced-motion`.
8. **One TUI line per row**, no line spacing; ellipsize overflow and state hidden count, e.g. 12 more. Details wrap (§4.1).

**TUI dark palette**:

| Role | Value | Use |
| --- | --- | --- |
| Text | `#E6E6E6` | Names, values, keys |
| Secondary | `#7C8290` | Explanations, hints, authors |
| Faint | `#4A4F5A` | Numbers, subheadings, unfocused content |
| Accent | `#A5B4FC` | Cursor, selected names, focused headings, prompt |
| Pill | `#2C2E4A` | Current environment / focused filter |
| Focused row | `#1A1C24` | Cursor in focused area |
| Selected row | `#232539` | Whole row incl. wrapped continuation |
| Selected cursor | `#303450` | Focused selected cursor incl. continuation |
| Success | `#7EE2A8` | ✓ result |
| Warning | `#F2C46D` | Warning icons; magpie has no warning, Mod management must distinguish breaks / may be wrong |
| Error | `#FF8A80` | Error icons / failed results |

TUI dark only, terminal background; ConPTY cannot reliably detect background, same reason as §9.2 color detection; no automatic light theme.

**Log colors** (§2.10): exception to gray text, use game's log-window colors: Error red `#ff6666`, Warning yellow `#ffeb04`, Message white; Mod message `<color>` retained. Light theme changes contrast, not hue. Only raw log rows; UI text / icons retain palette.

**GUI palette**: magpie `app.css` MIT, copyright atop `style.css`, CSS variables; **settings > interface day / night controls effective theme** (2026-10-24, §7.4). Dark: background `--bg #1a1a1e`, card `--card #232327`, pill `--pill #2d2d33`, line `--line #333339`, text `--fg #ededf1`, secondary `--fg-2 #b9b9c1`, `--muted #8b8b94`, faint `--faint #56565f`, accent `--accent #a5b4fc`, soft `--accent-soft #2b2c47`, selected `--sel #33345a`, success `--green #6fd99b`, error `--red #f28b82`, warning `--amber #f2b544`, log `--log #f5a97f`. Light: `style.css` :root, accent #4f46e5. 28px rows, 13px text.

**Card spacing** (2026-10-22): all horizontal / vertical rounded-card gaps `--card-gap` 4px, no other value: editor columns / third-column details-staging-history / left filters and list-details, inventory filter / table, log / changes columns, tasks columns / right cards, all-lists columns. **Space-sharing card gaps draggable** (§3.1 rule 10, 2026-10-23), double-click reset, persist `ui.gui.<page>.*`; new cards must include splitters. Content-height stacked needs / settings / six full-details sections no drag. Visible gap stays 4px, hit area extends `--grab` 3px into each card, about 10px; resize cursor, accent hairline hover (§7.3), double-click unchanged. Sidebar right edge remains 6px inside sidebar.

**Fonts**: sizes §7.3. TUI player's terminal (§9.2); GUI system Segoe UI Variable Text, Segoe UI, Microsoft YaHei UI; tabular numbers, Cascadia Mono code.

**Wording**:
- Short verb-first Chinese UI sentences, e.g. enable Vehicle Framework, not perform enable operation.
- Short muted subheadings: Enabled 485, description, needs.
- Empty states one sentence / key: staging is empty; t stages selected mods.
- No middle-dot separators (§7.3).
- Unknown faint —, never unknown.
- Every UI string has Chinese translation; GUI tests check every used string.

**Icons**: TUI defaults compatibility Unicode; ui.icons nerd uses Nerd Font Font Awesome 4 U+F000–U+F2E0, included in all Nerd Fonts (§9.3). GUI simple custom SVG `js/icons.js`, matching meanings:

| Meaning | Nerd Font | Compatibility | Color |
| --- | --- | --- | --- |
| Official Core / DLC source | shield `U+F132` | `⬥` U+2B25 | Faint |
| Workshop incl. SteamCMD | steam `U+F1B6` | `◉` U+25C9 | Faint |
| Local | folder `U+F07B` | `▢` U+25A2 | Faint |
| Locked Workshop copy | clone `U+F24D` | `⬦` U+2B26 | Faint |
| Git | git `U+F1D3` | `∆` U+2206 | Faint |
| Error, load failure | times-circle `U+F057` | `⨯` U+2A2F | Error red |
| Warning, possible issue | exclamation-triangle `U+F071` | `!` | Amber |
| Corpus hint, counting §5 | info-circle `U+F05A` | `ⓘ` U+24D8 | Secondary; GUI circled i |
| Last-run error | bug `U+F188` | `◍` U+25CD | Error red |
| Unloadable disabled residue | ban `U+F05E` | `◌` U+25CC | Faint |
| No problem | Hidden | Hidden | — |
| Different from game's list | circle `U+F111` | `⬤` U+2B24 | Accent |
| Locked, Core immovable | lock `U+F023` | Hidden | Faint |
| Update / change | refresh `U+F021` | `⇣` U+21E3 | Secondary |
| Dependency | link `U+F0C1` | `›` U+203A | Secondary |
| Staging | inbox `U+F01C` | `◫` U+25EB | Secondary |
| Launch | play `U+F04B` | `►` U+25BA | Text |
| Save | floppy `U+F0C7` | `↲` U+21B2 | Text |
| Automatic sort | sort-amount-asc `U+F160` | `↨` U+21A8 | Text |
| Search | search `U+F002` | `/` | Secondary |

- At most source / status two icons; status for findings / corpus hints, absent otherwise. GUI hover explains, click pins (§4.2), `--pop-bg`, hairline, `--shadow`, selectable text (§3.1 rule 13).
- Lock currently omitted: Core position via `official.order` finding.
- Shared cursor ▸, success ✓, prompt ❯, progress …, all one cell. Failure times-circle full, ⨯ compat; Cascadia Mono lacks ✗.
- Compat glyphs East Asian Neutral N, width 1 with either runewidth EastAsianWidth value, present / one cell in default Cascadia Mono. Nerd private-use icons and … Ambiguous A; RimArcho must disable EastAsianWidth. Terminals rendering ambiguous width 2, e.g. WezTerm treat_east_asian_ambiguous_width_as_wide / some CJK consoles, misalign Nerd; use compat. `TestIconWidths` verifies both one cell.

**App icon**: player-supplied `internal/icon/portrait.svg`, 1024×1024 vector, #4f46e5 outline glow; also Archo image (§8.6).

**Bow mark** (2026-10-25): portrait's upper-left bow used where portrait too small. GUI simplified `js/icons.js` bowMark, three wings / knot, lower-left glow matching portrait; bowMark(24) extra light face per wing, bowMark(16) solid. Fixed brand colors independent of chosen accent: light glow #4f46e5, dark #a5b4fc with #4a4d5c gray outline so black wings remain visible. Currently 14px preceding Archo name, 16px drawing, game-log Archo log analysis card / ask Archo to analyze button; future Archo task rows too (§2.11). Portrait for large panel etc. No sidebar mark (2026-10-25, §2.1).

**Choice controls** (§3.10): selected segment `--seg-thumb` / `--seg-edge`, unselected text `--ctl-fg`. Single-choice right arrow; menu field name atop, current checked left / rounded highlight, separate keyboard focus background. Card colors / hairline / menu shadow, shared theme variables.

### 7.1 Motion (GUI)

Four `style.css` :root durations; entry / movement ease-out `--ease` cubic-bezier(.2, .8, .2, 1), base 250ms; exits faster, `--ease-in` cubic-bezier(.4, 0, 1, 1):

| Variable | Duration | Use |
| --- | --- | --- |
| `--t-press` | 120ms | Hover, press, color fades |
| `--t-enter` | 200ms | Page / menu / status entry |
| `--t-exit` | 150ms | Menu close, card collapse, ease-in |
| `--t-layout` | 250ms | Row movement, sidebar highlight, expansion, number pulse |

Menu close / card content collapse use exit / ease-in; layout / `motion.js` FLIP share 250ms.

Motion inventory, js/motion.js / `style.css` motion:
- **Page entry** fades / rises 4px; same-page redraw after action / subview no motion.
- **Sidebar highlight** slides old to new, size changes immediately, position only animated; own child highlight within same entry.
- **Sidebar children** grid row 0fr → 1fr, layout duration, entry fade, arrow rotates 90°. Old collapse / new expansion simultaneous, following entries move smoothly. Highlight goes directly to final destination computed from expanding / collapsing children by motion.js. Collapsed children excluded from tab order.
- **List changes**, enable / disable / move / sort / undo / drag: record visible Mod row positions, FLIP surviving rows after redraw; affected rows (if unknown, moved / new rows) flash soft accent, fade about 400ms. Only rendered virtual rows, dozens; one style calculation per batch.
- **Numbers**: badge / count, list headings, needs sections, unsaved softly pulse on change.
- **Status** new sentence slides up.
- **Loading** lazy game log / details / full-detail pages shimmer placeholders, no loading… text; screen readers retain old text.
- **Cards / overlays** existing slight scale fade retained.

Rules:
- Continuous / moving animation only opacity / transform for WebView2 compositing, no relayout / repaint. Flash fading overlay, not background-color animation; shimmer / progress moving gradient layers. Exceptions: one-off 120ms hover colors, few sidebar grid rows once at 250ms.
- No `backdrop-filter` blur: WebView2 repaints it every frame.
- Under `prefers-reduced-motion: reduce`, CSS sets all animation and transition durations to 0; `motion.js` skips FLIP, flashing, pulsing, and sliding.
- Scrolling no animations: 1341 rows, 400 consecutive scrolls, none running, visible-row redraw median 0.5ms in development.

### 7.2 Interface wording

Decided 2026-10-22. GUI / TUI in both languages use player wording; code / CLI / MCP / specs keep domain names: plan, environment, Instance. Thus spec plan becomes UI list.

English UI text is translation lookup key; changing English requires callers too. app / modchanges CLI-worded keys, e.g. the current plan, stay; `view.enText` overrides UI display. History operation names checkout / swap / restore mapped by stepWords.

Check: internal/surface/cli TestWording (`view.WordingProblems`) reads all translation maps / panel. Chinese forbidden terms in final column, English plan / environment / upstream / workspace / swap / pinned / checkout / not written fail; conflicting translations for one English key also fail, because later registration overwrites. Necessary exceptions in wordingExempt, e.g. decompilation.

| Domain concept | UI wording | English | Avoid |
| --- | --- | --- | --- |
| Plan / loadout | Current / new list, list history | list | plan, workspace |
| Enabled portion | Enabled | Enabled | enabled list |
| Three editor lists | Areas: Enabled, Disabled, Staging | areas | list, ambiguous with plan |
| Add / remove plan entry, editor / library same | Enable / disable, disable in every list | enable / disable; disable in every list | add to current plan, take out, remove from plan |
| Staging / stage restore | Stage / put back | stage / put back | — |
| save | Save changes / no changes, N unsaved, saved versions | save; save changes / no changes; N unsaved; saved versions | write to game, unwritten, not yet written, written versions |
| checkout | Go back to this version; history step go back | go back to this version | load version, load list |
| External ModsConfig changes | Use outside change | use the outside change | load into list |
| Environment | Game; `env remove` removes the game without changing files | game; remove the game | environment, delete environment |
| Source | Mod folders in settings, source in filters / details, Workshop / local | mod folders; source | — |
| Instance | This one, another copy, N copies | this one, another copy | instance, Instance |
| Lineage | Copies of same Mod | copies of the same mod | Lineage |
| Copy | Only RimArcho-made lock-library / pack copies | copy | calling original Workshop installation a copy |
| pin / unpin / pin --update | Lock, unlock, update locked version, keep old; source locked version | lock this version / unlock / update the locked version | pin, including other tools' pins |
| Upstream changed after lock | Newer version, use Workshop version | a newer version is out | upstream, original updated, use original; original means Core / DLC |
| library move-to-locked | Move into lock library | move into the lock library | collect into lock library, back up locked version |
| swap | Switch version… | switch version… | replace with another copy, replace instance |
| unhide | Unhide | unhide | restore to game |
| trash | Recycle | move to the recycle bin | delete, reserved for real deletion: lists, DDS, uninstall |
| Mod inventory | Mod library | mod library | inventory |
| Allowed game versions (04 §1.6) | Game versions / reset default | game versions; reset to default | allowed / compatible versions |
| plan follow / unfollow / sync (04 §9.6) | Follow collection, unlink, catch up; add new only / also disable removed | follow a Steam collection / unlink / catch up with the collection; add new only / also disable removed | subscribe collection, subscription only for Steam Mods; sync |
| Build pack | Create / recreate / unfinished pack | create a pack | compile |
| Full / light pack | Pack always includes files; list-only RimArcho list, export format / pack export list | pack; RimArcho list | full pack, light pack |

### 7.3 Visual rules

Launch (§2.3) exception: own sizes / rules / radii, exempt rules 1, 3, 4; four sizes 12px, 17px, 26px, responsive headline (2026-10-28).

Decided 2026-10-23, GUI; TUI exceptions per rule. Checks below.

1. **Two radii**: --r-lg 12px cards / panels / menus; --r-sm 6px buttons / inputs / chips / thumbnails / small blocks. Single-select / segmented outer frames lg. Circles / pills 50% / 999px excluded. No third value, stylesheet variables.
2. **Concentric nested corners**: inner within 8px of outer edge, inner radius = outer minus padding, including switch thumb / menu options. Card-edge blocks 6px gap, 12−6=6, consistent with rule 1; distant blocks exempt.
3. **Hairlines only**: all borders / separators / menu group rules --hairline; `resolution >= 2dppx` 0.5px, 100% / 150% 1px, lighter than --line, since 0.5px blurs / disappears there. No ≥2px; hover splitters accent hairline; quotes indent / muted, no line.
4. **Four font sizes, minimum 12px**:

   | Size | Use |
   | --- | --- |
   | 12px | Secondary, tables, subheads, sidebar groups / children, badges, chips |
   | 13px | Body, rows, buttons, sidebar entries |
   | 15px | Breadcrumbs, page names, summary titles |
   | 32px | Launch headline only |

   Use weight / color / spacing for more hierarchy, no extra sizes. TUI single terminal size, inapplicable.
5. **No middle-dot separators**. Parallel facts spaced, GUI 12px / TUI 2 cells; sentence commas, ranges en dash, hierarchy slash / ›. Hint groups wider: GUI 16px / TUI 3 cells. Spec titles exempt; UI examples use two spaces.
6. **No left vertical / color bars** (§7 principle 4). Quotes 14px indentation / muted, no line. Sidebar child tree guide retained, not selection (§2.1).
7. **No explanatory filler**. Keep sentence only if removing it loses information needed now. Head (§2.2) status / conclusion, e.g. Main colony not ready: fix these 2 problems, plus actions; never page-purpose explanation. Safety text, e.g. no game-file writes, in confirmation / tooltip. Empty state one sentence (§7 wording).
8. **Eyebrows only informative**: category / context absent from title. Launch no type / game eyebrow since 2026-10-26, already in 01 / 02 (§2.3). No uppercase English labels. Heading numbers only meaningful focus keys 1–9 (§3.5).
9. **Optical alignment**:
   - Icons visual-centered in full row-height boxes, not baseline.
   - All numbers use `tabular-nums`; numeric columns are right-aligned.
   - Round icons 1px larger than square peers.
   - Equal button padding, icon side 2px less for optical centering.
   - Align icons / names / right counts throughout column, as sidebar (§2.1).
10. **Motion** ease-out entry / movement, base 250ms, values §7.1.
11. **No text overlap** (2026-10-24): text stays in own column at any width, wraps / ellipsizes with full hover. Addresses §3.1 rule 12 never cover neighbors; narrow windows wrap right controls below, not over explanations. TUI truncates to width, likewise no overlap.
12. **Two page widths by shape** (2026-10-28): multicolumn list / details full-window, 12px side padding, draggable resettable widths like editor 38/31/31: inventory, log list 62%, needs left 55% / two columns since 2026-10-29, all lists list 38%, packs, subscriptions, tasks. Single column centered 960px reading width so labels / values close: games, textures, settings. Others guide / uninstall / build / exchange retain 1200px. Two `.page:is(...)` stylesheet rules.

**Checks**:
- GUI TestStyleRules reads `style.css`: only var(--r-lg), var(--r-sm), 50%, 999px, inherit, calculated inner choice radii; only table's font sizes; only var(--hairline), 0 / none to remove. Reports line / value. No border-left / border-right or `box-shadow: inset` accent bars.
- TestWording (§7.2) checks no middle dots in translation maps / `view.Text` (`view.WordingTextProblems`) and GUI literals.

### 7.4 Accent themes (GUI, 2026-10-24)

- Settings > interface > accent (`ui.accent`, 06 §2.14): default leaves existing indigo unchanged; 15 Radix Colors 3.0.0 scales: indigo, blue, cyan, teal, green, grass, orange, amber, red, crimson, pink, plum, purple, violet, iris. Each 12 light <name> / dark <name>Dark levels in assets/js/accent.js with MIT notice.
- **Only four accent variables change**, success / error / amber / grays unchanged, §7 principle 2:

  | Variable | Light | Dark |
  | --- | --- | --- |
  | --accent, fill / border / cursor | 9 | 11 |
  | --accent-soft, pills / cursor tint | 3 | 3 |
  | --sel, selection | 4 | 5 |
  | --accent-fg, text on fill | White, amber level 12 | 1 |

  Also expose all 12 levels as `--accent-1` … `--accent-12`; accent text level 11 `var(--accent-11, var(--accent))`, clearer on white than level 9.
- **Day / night** (2026-10-24): `ui.theme` auto default follows OS, light / dark manual. Gray, log, accent share effective theme. Restore local cached theme before styles to avoid flash, `Init.theme` syncs database; disabled local storage still uses startup data.
- **Immediate**: Init includes accent, root inline vars before first paint; setting change refetch / apply without reload; manual light / dark swaps immediately, OS changes apply only with `ui.theme=auto`. Swatches actual effective --accent, ring / ✓ selected, hover / adjacent name. Default removes inline vars. TUI retains the §7 palette.
- Known: some light scales' level-9 white contrast <4.5:1, Radix tradeoff; 13px medium buttons unchanged for now.
- `internal/surface/gui/accent_test.go` verifies 12+12 valid colors per choice, new CSS passes TestStyleRules.
