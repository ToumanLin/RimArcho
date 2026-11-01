# List management page draft

Interactive second-draft prototype for "library / lists / manage". Colors/sidebar follow current GUI (`style.css` variables). Specifications: 04 §1.6, §9.6 and 06 §2.17.

- Left: all lists for current game, ungrouped. Right: selected list's three sections:
  - Game versions: small buttons with version numbers only; current game has a dot.
  - Follow a Steam collection: last check/missing count; view differences and catch up when changed.
  - Sharing: export…, create a pack….
- Export is a subpage: library / lists / <list> / export. Choose format left, preview right, save/copy; `esc` returns.
- New/import at top-right open dialogs.

Open `index.html` directly in browser. Example data only, no file reads/writes.