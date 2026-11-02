# RimArcho start-page layout drafts

This round explores only the user's "game → list/pack → launch game" request. The interface keeps established project wording: plans appear as "lists", environments as "games". Earlier off-project branding, copy, colors and statistics cards have been removed.

## Basis and reuse

- `docs/00-top-level-design.md`: one-stop launcher; first answer "what am I playing, and will it run"; minimum steps.
- `docs/00a-architecture.md` §1: relationships between games, lists, packs and launch records.
- `docs/specs/06-ui.md` §2.1–2.3: sidebar, top navigation, breadcrumbs, vanilla game main menu, recent lists across games, two bottom-left activity lines and launch warning strip.
- `docs/specs/06-ui.md` §2.14: original game art or solid background; §3.1: separate movement and applying, reduce popovers; §7–7.2: magpie gray/white and pale-purple palette, small text, short copy, no left accent bars, player wording.
- `docs/specs/04-plans-and-sorting.md` §1: multiple lists per game, persistent edits separate from writing to the game.
- `docs/specs/08-launch-and-packs.md` §1–2, §5.3: applying does not write game files; launching executes directly; one running game at a time; packs keep their own saves and settings.

Previews load the repository's `style.css`, `shell.js` and `icons.js` directly. The original comparison runs existing `home.js` directly. No duplicate lookalike navigation or visual components. Only Wails APIs are replaced by browser example data shaped as `HomeRow` / `Home`.

## Four views

- **Original comparison**: existing `home.js` rendering, the starting point for this change.
- **A · Bottom row**: keep recent lists across games and bottom-left activity; make launch area three controls in one bottom row.
- **B · Top row**: same three controls at top-left; otherwise identical to A.
- **C · Columns**: games on left, that game's lists/packs in middle, launch at bottom-right. Unlike the original, browse by game rather than mixing games.

Original and A/B show recent lists from all games; dropdown shows only selected game's lists/packs. Clicking a list row or arrows only moves cursor; apply button/Enter applies. Explicit dropdown selection applies directly, still without game-file writes. Remember each game's last selected item.

Shared: original sidebar/top layout, tasks entry, last game/current-list changes, launch hints, launch despite problems, disabled launch while running, unchecked explanation, no-game empty state, keyboard navigation. No new welcome text, decorative titles, branding section, persistent success marks, DLC stats, playtime cards or launch confirmation dialogs.

Top/bottom comparison bars sit outside product canvas. Switch original art/solid and problems/no problems/unchecked/running/no game. Original art reads local game's Core `About/Preview.png` directly; no game assets added to repository.

## Local preview

Run from anywhere:

```powershell
node D:/Game/RimModManager_Upgrade/RimArcho/docs/design/home-drafts/server.cjs
```

Open `http://127.0.0.1:8766/?design=original` and compare via top tabs. Set `RIMARCHO_DRAFT_BACKGROUND` to an absolute local image path to choose a background; `RIMARCHO_DRAFT_PORT` sets the port.

Draft launch buttons only simulate running; all example data stays in browser. Production launch fixes are separate from this exploration; no draft is connected to the real home page.

## Validation

Actually rendered original/A/B/C at 1440 × 880 desktop size; checked linked game/list selection, C's per-game display, solid background, launch allowed with hints, unchecked explanation, disabled running launch, no-game empty state and existing navigation-position switching. Launch results use original status line, no new confirmation dialog. Screenshots: `previews/original.jpg`, `a.jpg`, `b.jpg`, `c.jpg`; comparison: `previews/overview-v2.jpg`.