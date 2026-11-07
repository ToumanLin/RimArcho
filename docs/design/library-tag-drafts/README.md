# Mod library Tags view drafts

`index.html` is the interactive draft (synthetic data, open it in a browser) for the Mod library's Tags view and Tag manager, approved on 2026-11-05 and implemented as 16 §2.9.

What the implementation settled from the draft's open questions:

- Tags were already global (one set for every game, like the Lineages), not per game, so rename, merge, delete and notes apply everywhere (07 §6.1).
- Pinned tags with no Mods are listed greyed as on top, unused (the interface no longer says pinned, a word it once used for locked versions).
- The view switch is `v`; the Tags view is GUI only for now.
- The CLI got `archo tag rename|merge|delete|note`; MCP extends `set_player_data` with `tag_rename`, `tag_merge`, `tag_delete` and `tag_note` instead of adding a `manage_tags` tool.

Differences from the draft: merge and delete confirm in the shared confirmation dialog (16 §6) rather than inline; group rows show no total size (the page has sizes only as text); the tag title is 15px, as 16 §7 allows.

Revised on 2026-11-06 after the first build was reviewed (too much indent; the panel looked unlike the rest of the details and was awkward to use):

- Under a tag, a copy's name lines up with the Mod names, its `└` where a chevron would be, instead of one more step in.
- The Tag manager is built from the Mod details' parts: the note card, the title, the facts line, field rows with an action on the right (order, similar), and action buttons. Merge into… is a picker dialog like the position input, not a panel inside the details.
- The checkbox list of every tag is gone. All tags are chips that jump to their group; tags kept on top with no Mods are one faint line, not fourteen greyed rows (the Chinese preset pins fourteen).

Second review the same day: ctrl+click picks several chips, with a bar to merge, delete or clear them; a chip click keeps the panel's scroll; keep on top moved onto the title row and the order field went; the Mods view's copies line up with the Mod names too; the table heading got collapse all / expand all.
