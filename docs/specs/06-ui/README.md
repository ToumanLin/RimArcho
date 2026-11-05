# 06 · Interface

> Status: rewritten on 2026-10-21. Two interfaces, GUI (window) and TUI (terminal): information architecture, pages, interaction rules, details, visual design, and their respective implementations. The plan editor, can it run, details, settings, history area, GUI sidebar, top bar, home, first-run guide, Mod inventory > all, plans page (multiple plans, §2.17), and TUI page mapping (§9.1) are implemented; the Saves page (in Library since 2026-11-04, §2.11) is implemented in the GUI from save heads, its play facts pending; the Archo panel is pending (1.1). Background tasks (§2.16), interaction rules 7 and 8, and tables (§3.9) were added on 2026-10-23; all lists (§2.17, implemented in GUI; versions and collections pending in TUI) were added the same day. Split into this folder by section on 2026-10-23 without changing section numbers. Interface rules were added the same day: §3.1 rules 7–10 (dialogs, collapsing, background work must not block foreground work, draggable splitters), the scope of data tables in §3.9, and visual rules in §7.3; the code now follows these rules (01 §5.1 step 15). On 2026-10-30, the TUI stopped bundling WezTerm or drawing images; completion and history were added to the `:` command line (§9.2–9.4). On 2026-10-31, TUI names and keys were aligned with the GUI, and file search and sorting rules screens were added; differences from the GUI are recorded in §9.5. Settings were reordered by frequency of use the same day, with a community group and feedback subpage added (§2.14).
> Basis: 00 (positioning, three questions, principles), 00a (architecture), 01 (scope), 03 (findings and logs), 04 (plans and sorting), 05 (commands), 08 (launch and packs), 09 (Archo), 10 (unplayed and worth playing).

Design principle: let players **always know where they are, where they can go, and how to get back**, and complete tasks in **as few steps as possible**, without a complicated path. "How to get back" has two layers: in the interface, every overlay and secondary screen returns one level with `esc` or breadcrumbs (§3.1 rule 5); in the data, every change appears in plan history and can be undone (§2.5, third column; 04 §3).

## Files

Section numbers run through all of 06. Find the file by number:

| File | Sections |
| --- | --- |
| [1-layers-and-view-models](1-layers-and-view-models.md) | Layers and shared view models (§1) |
| [2.1-navigation-and-launch](2.1-navigation-and-launch.md) | Information architecture; sidebar, top bar, launch, can it run (§2–2.4) |
| [2.5-plans-and-packs](2.5-plans-and-packs.md) | Plan editor, building, packs, environments (§2.5–2.8) |
| [2.9-inventory-and-logs](2.9-inventory-and-logs.md) | Mod inventory, updates, game log (§2.9–2.10) |
| [2.11-other-pages](2.11-other-pages.md) | Review and saves, worth playing, Archo, settings, first-run guide, background tasks, all lists (§2.11–2.17) |
| [2.16-scheduled-behavior](2.16-scheduled-behavior.md) | Scheduled behavior table and pending items (§2.16.1) |
| [3-interaction-rules](3-interaction-rules.md) | Interaction rules (§3) |
| [4-details-and-feedback](4-details-and-feedback.md) | Details and description, wording for can it run, confirmation and feedback (§4–6) |
| [7-visual-design](7-visual-design.md) | Visual design, motion, interface wording (§7) |
| [8-gui-technology](8-gui-technology.md) | GUI technology (§8) |
| [9-tui](9-tui.md) | TUI (§9) |
| [10-resources-and-acceptance](10-resources-and-acceptance.md) | Resource targets and acceptance (§10–11) |
