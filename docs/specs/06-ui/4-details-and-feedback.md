# 06 · Interface · Details and descriptions, can it run wording, confirmation and feedback (§4–6)

> Part of [06 · Interface](README.md). Section numbers run through all of 06 and stay unchanged when split across files. References still use `06 §x.y`.

## 4. Details and descriptions

### 4.1 Structure

Top to bottom, `view.LoadDetail`, `HeaderFacts`, `ModFields`, `HistoryLine`:

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

**Fields** (`view.ModFields`, GUI `fields.js`, TUI `fieldLines`): editor details / inventory copy details (§2.9, `view.LibFields` adds status, used in, size, supported versions, update date, last played in middle) share fields, one per row: Archo ID, package ID; location / open; Workshop / open; tags / edit; alias / note if set; languages, dependencies, conflicts. GUI clicks on ID, package, path, Workshop ID copy with copied notice; other fields display only. No field background / borders, long paths wrap fully, end buttons act on row. Version / source / size already below name, not repeated.

- Package ID visible by default for other tools / forums, sole developer-term exception.
- Every can it run item plain sentence / executable end key (§5).
- Technical info only after expansion: #id, paths, Workshop ID, file count / size, Def / Patch / Harmony counts, Lineage / copies, unloadable reason (`view.UnloadableWhy`). TUI i loads mod output then; GUI click expands.
- Display-width wrapping, CJK 2 cells, never overflow / split characters:
  - Break at spaces / CJK boundaries, not Latin words, honor prohibited line-start / end punctuation; long paths after slash / backslash, retain indentation.
  - Status, description subheads, long dependencies, bottom hints wrap, never truncate; only detail … enter for all / wide BBCode table cells ellipsize.
- **Focused details**: full description, no 12-line cap; scrolled first line accent ▲ <name>. i, ctrl+w, r, g, enter act on displayed Mod and keep focus; other keys return list and act there. Click details focuses.

**GUI compact editor details (2026-10-25)**: left name / author / fields / description, all installed same-Mod copies with §2.9 abstract labels. Selected button background, no radio dot. Clicking browses only; separate use this one in the list swaps preserving position / undo. Game-ignored / unloadable copies disable action. No standalone can it run section; full diagnostics in full-screen / findings. Permanent clickable full details Enter ↵ at heading right opens currently browsed copy. Inventory retains copy actions / fields; shared full page.

### 4.2 Full-screen details

**One page** (2026-10-22, `fulldetail.js`): Enter from editor / inventory all opens same independently designed page, not enlarged source. Left-column compact views are simplified versions, same `renderDetail`, fields, copy actions. Breadcrumbs library / lists / <plan> / <Mod> or library / mod library / all / <Mod>. No head text, keys in hints; right selected-copy open folder / Workshop.

Wide Mod column left, copies / used in / fields / history right; technical full-width below, single column below 900px. Six numbered cards, 1–6 focus (§3.5):

| # | Card | Content |
| --- | --- | --- |
| 1 | Mod | Large Preview.png, name, author, one-line status like editor, tags / tags … g, About.xml / Workshop description (§4.3) |
| 2 | Copies | Every Lineage instance (`API.ModCopies` → `view.LibRow`): source, status, current-plan presence, size, version, folder, player / muted generated note. Selected raised / left-accent copy shows inventory six copy actions plus plan actions, `view.OpTable` order, applicable only |
| 3 | Used in | Plans / packs using each copy, current-plan presence and space action add / remove / swap |
| 4 | Fields | §4.1, click values copies |
| 5 | History | Recent changes, all changes links to subscriptions / updates filtered Lineage |
| 6 | Technical | One tab per `view.Sections`: Def, patches, Harmony, rules, full history, future sections such as manual targets / log pattern cards. Renderer knows sections, not concrete kinds; new data section adds tab |

Current technical sections:

| Section | Content |
| --- | --- |
| Def | def --mod text with references replaced by names |
| Patches | patches --mod text |
| Harmony | harmony --mod attribute / resolved manual `Harmony.Patch` calls, latter marked `Harmony.Patch` call in code; manual: resolved / unresolved counts, then unresolved methods / reasons (03 §5.3) |
| Rules | Plain About.xml loadAfter / loadBefore / incompatibleWith and relevant player rules |
| Full history | All Mod events newest first |

Keys, shared §3.5:
- Left / right source-list previous / next, moving source cursor; skip uninstalled editor entries.
- Esc / backspace return source, cursor last viewed Mod.
- `↑↓` / `j` `k` / `PgUp` `PgDn` / `Home` `End` scroll; `tab` / `shift+tab` technical tabs; 1–6 cards.
- [ ] selects copy; menu / shift+F10 its context menu; space m t x g n Del ctrl+o ctrl+w act on selected copy like inventory (§3.5).
- Hints: left right previous / next, esc back, space <action>, [ ] another copy, F10 menu, 1–6 sections, ? all keys.

TUI retains editor full details with Tab subsections and inventory copy details; new page GUI first.

### 4.3 Description source and rendering

**Measurement**, 2026-10-19 player environment, 1274 Workshop Mods with About.xml, read-only. Public keyless `ISteamRemoteStorage/GetPublishedFileDetails/v1`, 13 batches returned 1264. Plain text length excluding BBCode / images:

| Length | About.xml | Workshop |
| --- | --- | --- |
| Empty | 25 | 1 |
| 1–79 characters | 355 | 19 |
| 80–299 | 376 | 108 |
| 300–999 | 276 | 345 |
| ≥ 1000 | 242 | 791 |

- Empty / under 80 About.xml: 30%, 380; 305 have Workshop ≥300. Only 18 have one-two sentences / no images on both. Workshop ≥300 90%, About.xml 41%.
- 504 begin identically, author copied; only 5 descriptionsByVersion.
- Workshop 55%, 696, have images, 7981 total; 139 image-led (≥3, <100 characters per image). Almost all mixed; only 1 under-80 image-only.
- PNG 7351, GIF 323, extensionless 244, JPEG 55, WebP 6; `i.imgur.com` 72%, then raw.githubusercontent.com, img.litet.net, i.postimg.cc, images.steamusercontent.com.
- Common BBCode: img, * list item, url, b, h1, list, h2, td, h3, i, tr, hr, th, u, table, code, nl, strike, olist, quote.

**Rules:**

1. **Sources**: About.xml description, current-game descriptionsByVersion if present, read on demand, never database. Workshop fetched online.
2. **Selection**: with Workshop, About under 300 or identical beginnings ignoring images means Workshop only, description / workshop page heading. Both long / different: About then Workshop, two headings. Without Workshop (local, offline, failure), About with muted `view.DescNote` reason.
3. **When**: selected Mod only, background one-ID request, no whole-list prefetch. GUI editor debounce local read then immediate uncached fetch; GUI full / TUI delayed after local, §2.16.1 G12. Cache `<data directory>/cache/workshop/<Workshop ID>.json`; time_updated ≥ local appworkshop timeupdated means fresh, no request. No manifest expiry §2.16.1 C01. Full-details r forces.
4. **Offline controls**: `archo config set workshop.fetch on|off` default on; workshop.images on|off default on separately disables images. Off cache / About only. Network only public Steam endpoint / image URLs in description, RimArcho User-Agent.
5. **BBCode**, display-width wraps / §7: h1–h3 bold text with blank lines, b bold, i / u normal, strike muted; list / olist bullets / numbers, two-cell nested indentation; url link text clickable GUI / muted domain TUI (§9.3); quote / code two-cell secondary, GUI 14px / no left line (§7.3); tables align / truncate wide columns; hr blank, nl newline, noparse literal, unknown strip tags retain content. Go block parsing, frontend headings / lists / tables / quotes / code / links / images (§8.3).
6. **OCR**: not in UI; visual-model recognition belongs to Archo tasks (09 §5).

### 4.4 Images

- **Thumbnail**: About/Preview.png case-insensitive, JPEG content accepted; downscale without enlargement to 360px PNG, snapshot-hash cache `<data directory>/cache/previews/<snapshot hash>.png`; read-only Mod directory.
- **Description images**: download on demand, decode / downscale to 720px PNG, `<data directory>/cache/workshop-img/`; imgur.com/x.png → i.imgur.com. GIF first frame; WebP / failures placeholders.
- **No inline images in compact details**: thumbnail / text only; first description image becomes muted [images ×3 enter to view full-screen], TUI [images ×3]. Images only full-screen; TUI one textual [image `i.imgur.com`] each.
- workshop.images off disables description downloads.
- GUI `<img>` scales proportionally, description images use `loading="lazy"` and request only on visibility; local `/img/` only (§8.3). TUI no images (§9.3).
- Gallery / enlarged image browsing GUI only.

## 5. Can it run wording

**errors from the last run** (`view.lastRunNeed`): title Mod once / error-warning occurrence counts, both if present. Explanation top two actual categories by frequency / counts, omit count repetition if one, representative texture resource path, remainder aggregate. Evidence only log file / time / companion evidence, no duplicate count. possible attribution adds source not yet certain; none source unknown. Subject outside current list adds not in the list, save / another Mod may use content (2026-10-28). game-log action path / cluster ID, GUI §2.10; unattributed no cluster, stale log own file. TUI problems --code log --subject …, stale log --file …. This run (`view.LiveNeeds`) A: N× message, no this run prefix, evidence filename only, heading / body already identify run / count (2026-10-28). Also game-log opens active Player.log / cluster, TUI archo log; old text-only view log and evidence was inconsistent (2026-10-28). Other view actions unchanged.

**Each item** generated from findings / environment questions: group, sentence with Mod names, `view.Action`, counted flag. No finding codes / #refs / pkg: / p: on page, technical / CLI / MCP only.

**Names / navigation**: Go `Need.Content` gives `NeedPart` title / body / explanation / folded fragments, original strings retained for terminals / text clients. Candidates only this item's Mods, RequiredBy, action refs incl. targets / options, original finding subjects; merged items retain original subjects, never whole-library name matching. Each name has installed copy ID, else known Workshop URL, else plain. Shared rendering needs / editor panel / row popover: installed name locates editor row, clears hiding filter; uninstalled opens Workshop. Tab-focusable, Enter / Space opens; never whole sentence linked to first Mod, no JS substring matching. Identity questions same. `Reason.Content` / `Reason.Evidence` same names / evidence in popovers. Package index once per snapshot, fragments only relevant names, no per-item library traversal. Normal inline flow, not fact-field flex spacing in titles / explanations / folded / overlays. Popover's own Mod plain, others independently linked, shared fragments never mutated.

**Corpus hints**: `Evidence.Kind == EvCorpus` always uses `StatusHint`, separate worth knowing `SectionHint` (§2.4). Learned from shared logs, not faded: accent heading / icon, bold title, normal evidence. Editor / popovers use neutral information icons (§7), errors / warnings before hints. Excluded sidebar, editor problem-button / section / Mod counts, auto-dependencies; hints-only button says hints, still opens; has:problem excludes. Evidence shared logs as of YYYY-MM from `known.Data.Cutoff` embedded `known.json`, ratios in source; association never guaranteed current-run failure. Declared / inferred dependencies separate. Editor evidence row only for corpus to retain cutoff; ordinary duplicate evidence omitted. Needs / popovers retain evidence.

| Internal code | Player sees | One-step action / TUI key | Group |
| --- | --- | --- | --- |
| `official.order` | Core / DLC first | S sort | breaks the game |
| `order.violated` strong warning | Single A should follow B, wrong order may disable dependencies / content; multiple aggregate 10 Mods out of order, counts moved Mods; Enter expands | S sort | breaks the game |
| `order.violated` weak / harmless info (04 §5.3) | Merge with official / strong S item, `Need.Also` also satisfies 5 weaker requirements; otherwise separate 5 unmet weaker requirements, author / community suggestions not associated with more shared-log errors | S sort; standalone f expands | Merged error item or worth knowing |
| `order.cycle` | Merge with S item, Also priority-resolved cycle among 3 Mods / abandoned edges, viewing action there; standalone sorting resolves 3-Mod cycle by priority, explanation lists dropped requirements, info uncounted (03 §4.3.1) | Enter, v if merged, plain requirements; automatic, no confirmation / manual ignore | Merged error or worth knowing |
| `pkg.duplicate-active` | Two copies enabled | Space choose retained copy | breaks the game |
| `entry.missing` | One X listed but uninstalled, Workshop name if known; multiple merge since 2026-10-25 for imports: N uninstalled, subscription / download restores list positions. Their `dep.missing` not separate | s one / all subscriptions if IDs known, choose Steam / SteamCMD, background; TUI prints archo subscribe workshop …. One o opens / searches, Space removes; multiple x remove all / f list | breaks the game |
| `mod.unloadable` | A enabled but game cannot load | Space disable | breaks the game |
| `mod.replaced`, community replacements | A maintained replacement B | Installed B: Space enable B / disable A; else o Workshop B | breaks the game |
| `version.unsupported` aggregate | 293 not declared 1.7, 288 declare 1.6, ignored excluded | Enter list; M ignore these N for 1.7, confirm | breaks the game |
| List saved for old version | Saved for 1.5, game resets on start, save once | w save | breaks the game |
| `dep.missing` | A needs uninstalled B | o declared Workshop URL else search | breaks the game |
| `dep.inactive` | A needs disabled B | Space enable B | breaks the game |
| `dep.observed-missing` corpus | A may lack B, not installed / disabled; try if matching errors, includes sound / texture-only observations | Installed Space enable, else Workshop | worth knowing |
| `combo.known-bad` corpus | A / B together often error, or this A version; try disabling one / updating if errors | Space choose disable; version Space disable A | worth knowing |
| `mod.known-error` corpus | Shared logs often show A errors, check update if encountered | Enter error | worth knowing |
| `patch.no-match` target inactive | Some A content inactive, B target disabled | Space enable B / Enter view | breaks the game |
| `def.parent-missing` | N A contents unloadable, one-line missing basics / corpus error ratio; kinds / advice in details, no repeated list evidence | Enter focused finding / fixes: enable provider, subscribe / declared candidates, subject Workshop requirements; group-ignore when log condition permits (03 §4.3) | breaks the game |
| `incompat.active` | A incompatible B | Space choose disable | breaks the game |
| log.* | A — 8 warnings, texture missing representative path below | Enter game log / Mod cluster, TUI subject technical page | errors from last run |
| Live log | A: N× message, this-run heading, newest within phase first (§2.4) | Enter log / cluster, TUI archo log | Last-run group, this run |
| Identity question | Are X / Y same Mod? | s / c / u, twice confirmation | for you to confirm |
| External ModsConfig conflict | Changed outside, plan also edited since | Diff / L load, twice, replaces list / I ignore | for you to confirm |
| `mod.pinned` upstream changed | A locked, Workshop later updated | mod <Workshop instance> --diff <copy>, pin --update, confirmed unpin | Elsewhere inventory (§2.4, §2.9) |
| `mod.hidden-residue` | Hidden / unsubscribed A folder remains | Confirm trash recycle | Elsewhere inventory |
| `mod.hidden` | A hidden, likely other manager | Keep hidden ignore, confirmed unhide / recycle | Elsewhere inventory |
| `texture.stale` | A textures changed since conversion | Reconvert, confirmed delete converted | Elsewhere textures |

- `view.Action.Kind`: sort, save, enable, disable, choose-disable (one option), open-url, problem (focused explanation / fix), view (read-only command text), list (title / rows), fold, ignore-rule, load-external, dismiss-conflict, answer, unhide, unpin, trash, ignore. `Action.Key` TUI key, GUI button label is action, e.g. enable B, sort the list, open in Workshop.
- **Problem details** (`view.BuildProblemPanel`, GUI ProblemDetail / `popups.problem`): missing parent, inactive patch target, known-error viewing shows meaning, affected content, evidence, fixes only, never full Mod / all patches. Player-opened temporary GUI layer, Esc leaves; Tab / left-right buttons, Enter executes; close before other layer. Late reads cannot reopen / overwrite new layer / game. Cached explanation / options first; provider lookup says searching installed mods…, background updates buttons. Closing / another layer / environment cancels, completion also releases context. Enable providers before subscriptions; first available primary, subject Workshop secondary. If all missing templates have installed providers, enable only, no candidates / generic find-provider advice. Complete unsuccessful lookup says installed Mods including disabled cannot provide basics; incomplete says not all checked, never asks retry. Only candidate-only view warns optional combination. Each affected content appends muted file:line; no duplicate evidence if only location, other evidence retained. Initial read independent log session, supplemental private read session, no navigation / edit locks. TUI same-reference focused CLI. Locked diffs / list diffs / subject logs keep dedicated viewers; identity / version aggregates unchanged.
- `patch.no-match` only target-inactive, installed but disabled; absent target / wrong version routing unfixable, technical only. `def.abstract-collision` technical only.
- `version.unsupported` only warning Mods stopped at previous version; all older info omitted; corpus G8 reversed levels (03 §4.3.1).
- **Version reminders** (03 §4.2): `Needs.Reasons` per Mod not declared 1.7, declares 1.6, actual versions, singular; sole action ignore this Mod's 1.7 reminder, completion says this Mod’s 1.7 reminder was ignored, no aggregate view list. Shared VersionReason in details, disabled also view / ignore; ignored muted done / restore 1.7, completion says this Mod’s 1.7 reminder was restored; update adding support still retains restore. TUI M / U.
- Needs / editor aggregate muted N ignored 1.7 reminders / view or restore, no count / icon. Counts only current-list undeclared-support ignored Mods; all ignored still muted 0 problems entry. Events use Go Status / Reasons directly, no per-Mod reconstruction from aggregate.
- **Version list** (`versionpanel.js`, temporary §3.1 rule 7): only view list calls `API.VersionWarnings`, no Needs / Board row list. Once atop not declared 1.7, ignore all right; scroll rows clickable Mod name locates editor / clears filter, actual supports 1.6, compact ignore. Groups previous major.minor first, e.g. 1.6 / 352, older-only next, e.g. 1.5 or earlier / 13, others by declaration; group ignore. Batch / all confirm global scope / recoverability; individual direct. Ignored panel same, restore / group / all without confirmation. Preserve panel / scroll, reread after actions; late results after close / game / list change ignored. Tab / shift+Tab controls, Enter / Space execute, arrows / page keys scroll, Esc close. TUI grouped picker on demand, individual locate / ignore / restore.
- **Merge**: same finding solved by one key merges, Enter expands. Current `order.violated` S all, same-Mod parent-missing one focused view, same-Mod patches to same disabled target (X's 6 patches inactive, enable once). Dependencies normalized target, explanation requiring Mods; missing entries separate by installed-copy availability. Fold f, order Enter; GUI see N one by one below explanation, not button; editor inline lists. Each expanded row separate semantic list, no leading commas. Logs per Mod.
- **Counts**: sidebar / top = Counted items, excludes hints, folded stale log, elsewhere. Merged counts 1, reflects tasks not findings. Row status includes counted / corpus, explanations and counts calculated separately.
- **Buttons** (2026-10-29 GUI): all actions bottom aligned title, no ⋯; first primary / rest secondary. `fold` is a textual See N individually link, not a button. `choose-disable` / duplicate-copy choices have separate equal buttons, e.g. disable Combat Extended / disable Yayo’s Combat 3, no primary; Space focuses first, left/right switch, Enter execute. Identity three equal. TUI retains picker.
- **Inline confirmation** (2026-10-29 GUI): needs `Action.Confirm` (§6) replaces button row with `Action.Warn` consequence / OK Cancel Enter / Esc; link fix says repair above links marked remove, rows are preview. No dialog. Editor panel / popover same action still dialog.
- **Completed items** (2026-10-29 GUI): after this-page action completes reread, not incidental mid-action refresh. If original vanishes without replacement of same `Need.Kind` (7 out-of-order → 6 counts replacement), retain original location as muted ✓ / action result / old title. Sort / save use completion status, later analysis complete never overwrites. Action solving multiple marks all. Only latest undoable row has undo / u, restores previous list step / findings. Uncounted, cursor can stop for undo; cleared on page leave / game change.
- **Cross-column links** (2026-10-29 `Need.Related`): last-run subject also left dependency (`dep.*` `RequiredBy`) / hint subject creates links both ways: last run Vanilla Furniture Expanded — 12 errors; related Vanilla Factions Expanded needs disabled Vanilla Expanded Framework. Click jumps / briefly highlights other column item. Current-run excluded.
- **Automatic fix F**: enable installed required dependencies / reorder (`API.CompleteDependencies`), one undo. Button count = solved counted enable-dependency items + S items, hidden if none.
- **Self-explaining row icons** (2026-10-25 GUI): hover warning / error shows underlying titles / sentences, explanations below, severe first; uninstalled / unloadable explicit. Click pins, selectable; same icon / Esc / outside / list scroll closes. Unpinned muted click to pin. Pinned item has same needs shortcuts (§2.4), first primary, identity equal, no fold. Click closes then executes via `screens.js` runAction / `Reason.Actions`, same confirmation / undo / switch disabling. `Needs.Reasons` / `Board.reasons` by Instance ID, same items as icon; every icon has explanation. No new keyboard key, Enter details has findings (§3.1 rule 6). js/badgepop.js.
- **Identity questions** (`view.questionNeed`): GUI needs / editor / pinned popover directly shows same / copy / unrelated, equal, no menu. Hover explains list / change-history impact; confirmation repeats and no-u warning; keys s / c / u. Stored summary main reason, no generic similar / vague inference chip; why this question muted link separately below.
- **Why this question**: v / link opens comparison, Esc returns. `popups.identity` / `view.IdentityQuestionPanel`: one-sentence conclusion, new / previously recorded columns compare name, package ID, author, Workshop ID, folder with symbols / background for matches; omit both-missing. Folder basename, full hover / click copy; shared files below, cap [02 §5.4](../02-environments-identity-and-scanning.md#54-%E5%BE%85%E7%A1%AE%E8%AE%A4%E9%A1%B9). Three equal answers / impact; click or s/c/u closes layer before confirmation, no nesting. Close initially focused, Enter no default answer. TUI brief text panel.
- **Absent here**:
  - Info Def overrides / patch contention / shared Harmony targets only technical / CLI / MCP.
  - Harmony warnings without log evidence technical, not actionable.
  - Stale logs outside current plan latest launch / game version (03 §6.4) muted earlier log / Sep 29 / 1.6 row, uncounted.

## 6. Confirmation and feedback

- **Sort / save / launch direct** (`view.Run`), one click / key, status `view.Outcome.Text`; sort u undo, GUI link / TUI u undoes. Refusal (running game, outside edits, no user data, Steam missing…) status without confirmation; `Outcome.Refused` commands show read-only GUI dialog / TUI panel for diff / load / ignore etc.
- **Preview** optional, never action gate: panel sort / save / launch `view.DryRun`, CLI --dry-run text, read-only, no continue; `archo sort|save|launch --dry-run`.
- **One-step undoable actions no confirmation**: sort, history checkout, external ModsConfig load L; workspace edits preserve prior list, status undo (`view.Action.Undo`).
- **Confirm what RimArcho cannot undo**:
  - TUI repeat key: first status ✗ press R again: …, same executes, other cancels / cancelled.
  - GUI dialog displays `view.Action.Warn`; needs inline (§5).
  - Current: plan delete; identity s/c/u merges / splits Lineage not undoable; unhide / unpin rename About.xml; trash, pack recycle, unfinished copied-file deletion; settings forget folder, download community data about 50 MB, gc.
- **Recycle / rename** unhide / unpin / trash preview --dry-run then confirm (08 §6, §7).
- **Uninstall** preview then one confirmation; contents / focus §2.14, scope 01 §4.1.
- **Results** plain surface wording, enabled Beta / Harmony now position 3; errors through `view.Humanize`, instance refs → names.
- **Long work** nonblocking status (§8.4), every task recorded (§2.16). Library file / Steam actions queue after confirmation; one failure does not stop others, per-item results.
- **Changes never pop dialogs**: scans / new game errors update status / counts only.

### 6.1 Sort outcome and completion API

GUI button / S calls `api.Run("sort")`, returns view.Outcome. Sort completes normally; omit following four arrays if empty. Missing dependencies only enabled About.xml required dependencies incl. current-version entries, not optional loadAfter. Cycles resolved automatically (04 §5.2); cycles for inspection, not sort failure.

JSON contract, ? optional:

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

missingMods: entries unresolved to current-game instances, deduplicated normalized packageId; **not necessarily absent files**. Other game's Mods/ may provide deployable copy: installed / canEnable true, instance/ref, existing enable action. Workshop ID from list metadata, known Lineage / instance, cached public index in order; omit unknown, no DLC Workshop. No sort network. Only !installed && workshopId for subscribe all; installed but unenableable shows reason / copy-check guidance.

replacements reuse Use This Instead / version filters, downloaded via update data; absent data means no guesses. Same-package Workshop branches distinguished by Workshop ID; installed unloadable canReplace=false / blockedReason. before/after dropped directed edge, strength harmless/weak/strong, kind sorter constraint.

Backend exports:

```go
func (s *API) CompleteDependencies() (*view.Result, error)
func (s *API) ReplaceRecommended(refs []string) (*view.Result, error)
```

`api.CompleteDependencies()` enables all available installed required dependencies transitively, sorts, one undo. Returns `{text, undo?, enabled?: string[], workshopIds?: string[], missingDependencies?: MissingDependency[]}`; enabled new instance IDs, deduplicated Workshop IDs, remaining missing dependencies.

`api.ReplaceRecommended(replacements.filter(r => r.canReplace).map(r => r.original.ref))` revalidates current suggestions, replaces enabled originals at original positions, deduplicates enabled targets; all-or-nothing batch / one undo. No auto-sort / subscription / Mod-file edits. `{text, undo?}`; unavailable / stale error leaves original unchanged. Reject chains within batch, split them.

Frontend deduplicates needed IDs, asks Steam / SteamCMD, then existing `api.Subscribe({kind:"workshop", url:"https://steamcommunity.com/sharedfiles/filedetails/?id=" + id, steamcmd: useSteamCMD, batch:true})`. After download / scan, sort / enable / replace again; subscription never automatically replaces original.
