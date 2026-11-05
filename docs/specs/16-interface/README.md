# 16 · Interface

> Status: two interfaces, the GUI (window) and the TUI (terminal). The GUI implements every page in §2 except play review (§2.11), Worth playing (§2.12) and the Archo panel (§2.13, planned for 1.1); the TUI is the keyboard plan editor with secondary screens and text panels for the other pages (§9.1), without the Saves, game log and create-a-pack pages or the versions and collections of All lists.
> Basis: 00 (positioning, three questions, principles), 00a (architecture), 01 (scope), 05 and 06 (findings and logs), 07 and 08 (plans and sorting), 09 and 10 (launch and packs), 12 (saves, unplayed and worth playing), 13 and 14 (commands and MCP), 15 (Archo).

Design principle: players **always know where they are, where they can go and how to get back**, and finish tasks in **as few steps as possible**. "How to get back" has two layers: in the interface, every overlay and secondary screen goes back one level with `esc` or the breadcrumbs (§3.1 rule 5); in the data, every change appears in plan history and can be undone (§2.5, area 3; 07 §3).

## Files

Section numbers run through all of 16. Find the file by number:

| File | Sections |
| --- | --- |
| [1-layers-and-view-models](1-layers-and-view-models.md) | Layers and shared view models (§1) |
| [2.1-navigation-and-launch](2.1-navigation-and-launch.md) | Information architecture; sidebar, top bar, Launch, Can it run (§2–2.4) |
| [2.5-plans-and-packs](2.5-plans-and-packs.md) | Plan editor, building a pack, packs, environments (§2.5–2.8) |
| [2.9-mod-library-and-game-logs](2.9-mod-library-and-game-logs.md) | Mod library, updates, game logs (§2.9–2.10) |
| [2.11-play-review-and-saves](2.11-play-review-and-saves.md) | Play review and Saves (§2.11) |
| [2.12-worth-playing-and-archo-panel](2.12-worth-playing-and-archo-panel.md) | Worth playing, Archo panel, Texture optimisation (§2.12–2.13) |
| [2.14-settings](2.14-settings.md) | Settings (§2.14) |
| [2.15-first-run-guide](2.15-first-run-guide.md) | First-run guide (§2.15) |
| [2.16-background-tasks](2.16-background-tasks.md) | Background tasks (§2.16) |
| [2.16.1-scheduled-behavior](2.16.1-scheduled-behavior.md) | Scheduled behavior table (§2.16.1) |
| [2.17-all-lists](2.17-all-lists.md) | All lists (§2.17) |
| [3-interaction-rules](3-interaction-rules.md) | Interaction rules (§3) |
| [4-details-and-feedback](4-details-and-feedback.md) | Details and description, wording for can it run, confirmation and feedback (§4–6) |
| [7-visual-design](7-visual-design.md) | Visual design, motion, interface wording (§7) |
| [8-gui-technology](8-gui-technology.md) | GUI technology (§8) |
| [9-tui](9-tui.md) | TUI (§9) |
| [10-resources-and-acceptance](10-resources-and-acceptance.md) | Resource targets and acceptance (§10–11) |
