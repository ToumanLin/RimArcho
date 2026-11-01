# Subscriptions and updates page drafts

Two interactive candidates for "library / mod library / subscriptions and updates". Colors/sidebar follow current GUI (`style.css` variables). Example data only; no file reads/writes.

Start with the three reasons players open this page:

1. Add subscriptions (Git, LoversLab, Workshop / SteamCMD)
2. Update subscriptions managed by RimArcho (Git, LoversLab, SteamCMD)
3. See changes from automatic Steam Workshop updates

Changes shared by both:

- **One paste field replaces the subscribe form.** No type choice first: pasted URL identifies Git repo, LoversLab file page, Workshop link/ID or dropped zip. Ask only the needed decision (what to track, which file, Steam or SteamCMD). Suggest Git when a LoversLab page links GitHub. The abstract "download page" no longer appears in UI.
- **Do not add local Mods here**; only a sentence and "open Mods folder".
- **Remove the sources section.** One "14:02 checked 4 sources ▾" line expands per-source counts, conclusions and accounts; missing Workshop entries are an amber count.
- **Choose LoversLab update files in place**, without expanding a top-of-page form.
- Each update says what changed (Git commits, author notes), then offers buttons.

| | A: One page | B: Three views by intent |
| --- | --- | --- |
| Structure | Paste → waiting for you → Steam already updated → earlier, read down one page | Top tabs: updates / changes / add; updates/changes use list left, details right |
| Add | Paste field always at top; `ctrl+v` anywhere | Dedicated add view with three source cards explaining fit/account status |
| Best for | A quick glance at what needs attention each visit | Inspecting a change or first-time addition without knowing its source |
| Cost | Inline details only; long commit lists lengthen page | Another switch; work split between two views |

## C: B's second draft (current direction)

- **One subscriptions and updates page**: left list has waiting for you (Git/LoversLab/SteamCMD, update all), already updated (automatic Workshop changes, ordered by affects launch / worth a look / content only with row labels), subscription changes, earlier. Right details: pending update shows changes/update; completed update explains implications/file changes.
- **Scope bar above left list**: `list "Main colony"` / `all mods` (`a` switches), with baseline (since last played / last 10 scans). List scope hides updates used only elsewhere and says N other updates / see whole library.
- **Add subscription is a mod library subpage** (library / mod library / add subscription): paste field/three source cards. Top-right add button and `+` navigate there; `esc` returns; pasting a URL on updates page also navigates there.

Open `a.html` / `b.html` / `c.html` through a local server (shared `shared.css`, `shared.js`), for example:

```bash
python -m http.server 8765 --directory docs/design/subscriptions-drafts
```

A's top try-it buttons insert example URLs; B uses source-card clicks.