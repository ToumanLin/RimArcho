# Archo placements: survey and proposal (2026-11-19)

Status: the player chose Can it run, Feedback, Import & export, Uninstall and Background tasks (not Start); implemented 2026-11-19 as 16 §7.1 "Placements". Idle cycling is built but off until the player decides. Peeks needed head room the pages lacked, added as padding above the card (about 25–50 px). Original proposal below. Today Archo appears in three places (16 §7.1 "Archo figure"): the start-up screen, page-loading cards and the first-run guide. This document surveys every page and the main dialogs for more places and proposes the best ones.

## Rules

Every placement must satisfy all four of the player's rules:

1. **Never take the visual centre.** Archo sits at an edge or in a corner, beside what the page is about.
2. **Use existing empty space.** Change the page's layout as little as possible: no new rows, columns or margins where I can avoid them.
3. **Feel alive.** The expression changes with the page's state (and idles between expressions); she is never a fixed picture.
4. **The whole head, hair included, stays visible.** Never clip her in a box or a grid cell. She ends only at the bottom: by the soft fade (`.archo-slot` mask), or behind a card whose top edge hides her below the chin.

16 §10 also applies: **nothing moves at rest.** The figure itself is cheap: each expression is a still (~20 KB) plus a loop (~230 KB animated WebP). Its CPU was too small to separate from the host's ±2 % noise in headless Edge.

## Two patterns

The mockups use two patterns. Both reuse the shipping figure (`archoFigure`, the 50:33 head-to-chest framing).

- **Peek.** She stands behind a card's top edge at its right end. The card hides only what is below her chin, as in the first-run guide. This works on pages whose first card has an empty right-hand header area above it: Feedback, Import & export, Uninstall. It needs no new row; the figure overlaps the empty space above the card.
- **Stand.** She stands in an empty area and fades out at the bottom. Examples: on the Start page's rule, or at the foot of a column that is shorter than the window.

## How she stays alive without moving at rest

- **On a state change** (a task finishes, a check finds problems, a send fails), the page sets the new expression. Its loop plays three times (6.5 s), then the still holds. The first-run guide already does exactly this (archo.js `rest`).
- **Idle cycling:** while a page is open and nothing changes, she moves to the next expression of that page's calm set every 45 s. Each change again plays three loops and holds. That is about 15 % of the time moving, never continuously.
- Only the figure in view animates. Leaving the page ends it (`end()` frees the frames).
- `ui.archo.motion` still / off and reduced motion behave as today: a still, or nothing. With off, every proposed placement disappears and the layout stays as it is without her.
- **Open question for the player:** whether idle cycling is wanted at all. Without it, she changes only with state, which is calmer and costs nothing at rest.

## Survey

Sizes are the slot width in CSS px (height = width × 33/50). Expressions are from the 18 in the pack. Ratings are against the four rules.

| Page / dialog | Where | Expressions and when | Size | Fit |
| --- | --- | --- | --- | --- |
| **Start (launch)** | Standing on the long rule under the list's name, at its right end, above the launch column. The hero text is on the left, so that space is empty. | Ready to play: smug / wink / neutral idling. Problems break the game: surprised, then skeptic. Game running: sleepy (she waits while you play). Last game had errors: dizzy. Just launched: thumbsup. | 176 | **Best.** Empty space, no layout change, many meaningful states. |
| **Can it run** | Foot of the right column ("when you last played"), under its last card. The column is short, so the space below is empty. | Analysing: think. Something breaks the game: surprised. Only warnings: skeptic. Nothing to handle: thumbsup. Analysis failed: shake. | 176 | **Good.** On a long right column she moves below the fold, which is acceptable. |
| **Feedback** | Peeking over the content card's top-right corner, in the page title row's empty right side. | Idle: neutral. Typing: think. Sending: ponder. Sent: salute. Send failed: cry. A maintainer replied (unseen reply): wink. | 128 | **Good.** No layout change, but tight: at 128 px her hair is 8 px under the window's top. Larger sizes need the content card moved down about 40 px. |
| **Import & export** | Peeking over the first ("Bring in") card, left of its "read it" button. | Idle: neutral / ponder. Reading a file: think. Preview ready: smug. Import done: thumbsup. Nothing found or error: shake. | 140 | **Good.** |
| **Uninstall** | Peeking over the uninstall card's top-right corner. | Preview: cry (she is sad to go). Running: think. Done: salute (goodbye). Failed: dizzy. | 140 | **Good,** and the strongest emotional moment. The page is shown only while uninstalling. |
| **Background tasks** | Standing in the empty bottom-right of the detail pane ("every item"). | Selected task running: think. Done: thumbsup. Failed: cry. Several failed: dizzy. Nothing selected: sleepy. | 176 | **Good.** It moves only when the detail is short, so hide her when the item list fills the pane. |
| Texture optimisation | Right end of the "Count first…" line under the cards. | Counting: think. Savings found: yum. Converting: think. Done: thumbsup. | 140 | Possible. That line is short; she would stand below the last card. |
| Updates | Foot of the left "waiting for you to update" column when it says everything is up to date. | Up to date: thumbsup. Checking: think. Updates waiting: ponder. | 140 | Possible. It shows only in the up-to-date state. |
| Search files | Bottom-right corner of the empty file viewer ("select a file"). | No search yet: sleepy. Searching: think. No results: shake. | 140 | Possible. In an empty pane she is close to being its centre (rule 1), so she must stay in the corner. |
| Play review | Corner of the empty "no save has been read yet" pane. | Empty: sleepy. Reading saves: think. | 140 | Possible, empty states only. |
| Settings › about | Right of "RimArcho" in the about row. | Click: laugh / derp / spiral (easter egg). | 96 | Weak. The row is short, so it needs a taller row (rule 2). |
| Game log | "Archo log analysis" card header. | Analysing: think. Found the cause: smug. Nothing found: shake. | — | Rejected. The card's header has no height for a head; adding it changes the layout. |
| Lists, list editor, Mod library, Mod settings, Packs, a pack, Saves, Game | — | — | — | Rejected. Dense lists and panes fill the width and height; any spot either clips her or covers data. |
| Launch memory warning, confirm dialogs | — | — | — | Rejected. Dialogs are sized to their text; she would need a wider dialog (rule 2) and would compete with the question (rule 1). |
| Add a subscription | Under the three source cards. | Idle: neutral. Link recognised: wink. | 140 | Weak. The space is there but nothing changes often. |

## Recommended first set

1. Start
2. Can it run
3. Feedback
4. Import & export
5. Uninstall
6. Background tasks

All six are mocked up below as real screenshots. Each is the product's figure, placed by a script over the GUI dev harness (`spikes/guidev`) on a synthetic home. The mockups place her `position: fixed` and emulate the card's edge with a clip. An implementation would put the slot in the page's own DOM: `position: absolute` in the card's container with the card above her, as `.guide-archo` does.

| # | Page | en, dark | zh, dark |
| --- | --- | --- | --- |
| 1 | Start (smug) | `D:\RimArcho-test\archo-gifs\mock\en-dark\m1-home.png` | `…\zh-dark\m1-home.png` |
| 2 | Can it run (skeptic) | `…\en-dark\m2-can-it-run.png` | `…\zh-dark\m2-can-it-run.png` |
| 3 | Feedback (neutral) | `…\en-dark\m3-feedback.png` | `…\zh-dark\m3-feedback.png` |
| 4 | Import & export (ponder) | `…\en-dark\m4-import-export.png` | `…\zh-dark\m4-import-export.png` |
| 5 | Uninstall (cry) | `…\en-dark\m5-uninstall.png` | `…\zh-dark\m5-uninstall.png` |
| 6 | Background tasks (thumbsup) | `…\en-dark\m6-tasks.png` | `…\zh-dark\m6-tasks.png` |

Each has a `-zoom.png` cropped to the page. Contact sheets: `D:\RimArcho-test\archo-gifs\inspect\mock-en.png`, `mock-zh.png`. The survey of every page as it is today: `D:\RimArcho-test\archo-gifs\survey\`.

## Implementation notes for later

- **One helper.** Add `archoSpot(container, {size, pattern: "peek"|"stand", calm, state})` in `archo.js`, built on `show(..., {rest: true})` plus idle cycling. Each page maps its state to an expression in one small function, like `guideExpression()`.
- **Layout.** Peek places the slot absolutely with the card above it (`z-index`, as in the guide); stand is absolutely placed in the empty area. Neither adds rows. With the window narrower than about 900 px, or the area not tall enough, the spot is not shown rather than clipped.
- **Measure.** Native WebView2 CPU at rest with idle cycling on, per 16 §10 (≤ 2 % of a core for the process tree), in the VM.
- **Spec and tests.**
  - Spec: 16 §7.1 "Archo figure" gains a "Placements" list.
  - `TestEndlessAnimationsOnlyWhileWorking` is unaffected, because the loops are images, not CSS animations.
  - Testing: an `archo.mjs` test for idle cycling and for ending a figure when its page is left.
