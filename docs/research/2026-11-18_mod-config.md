# Mod settings: how they work and how RimArcho could manage them

> Status: research note (2026-11-18), card imp03. Not implemented beyond what 16 §2.9 already specifies. Decisions in §5.

## 1. How RimWorld stores Mod settings

- A Mod with a `Verse.Mod` subclass that calls `GetSettings<T>()` gets one file: `<user data>\Config\Mod_<folder>_<Mod class>.xml` (`LoadedModManager.GetSettingsFilename`, same in 1.6 and 1.7; 09 §3). Root `<SettingsBlock><ModSettings Class="…">`, children written by the Mod's `ExposeData`.
- **The key is the folder name, not the packageId.** The same Mod as Workshop `2009463077`, a SteamCMD copy or a local folder has three different settings files. RimArcho's "settings follow the Mod" (09 §3) copies a missing file across folder names on launch; packs do the same under their own names (10 §3).
- The file is written when the player closes the Mod's settings window or the game exits, and read once at startup. Unknown children are ignored, missing ones fall back to defaults; a corrupt file usually means defaults plus a log error.
- **HugsLib** Mods keep settings in one shared `<user data>\HugsLib\ModSettings.xml`, one element per pack, keyed by Mod, not folder.
- Some Mods write elsewhere: their own folder under `Config\` or user data (e.g. presets), or inside the Mod folder. These are not discoverable by name.
- Other files in `Config\`: `ModsConfig.xml` (the list), `Prefs.xml`, `KeyPrefs.xml`, `KnowledgeDatabase.xml`, etc. — game settings, not Mod settings.
- Nothing ever deletes a settings file: uninstalling a Mod leaves it (**orphans**). They are harmless except for clutter and stale values if the Mod returns.

## 2. What the reference managers do

| | RimSort | RimCrow (`mgr_mod_settings.py`, `mgr_mod_residue.py`) |
| --- | --- | --- |
| Show per Mod | no | yes: "Mod settings manager" groups `Mod_*.xml` by Mod and settings class, matching by folder, Workshop ID and packageId with a confidence |
| Edit values | no | no (opens the file) |
| Several copies | no | "sync": copy one copy's file over another's (`sync_group_instance`) |
| Orphans | Troubleshooting › delete **all** Mod configs (keeps ModsConfig.xml and Prefs.xml) | residue scanner: settings files and folders of uninstalled Mods, with a whitelist |
| Per profile | instances point at a config folder | each profile has its own user data folder |
| Export | no | data bundle carries RimCrow's own data, not `Config\` |

## 3. What RimArcho has (16 §2.9, 02 §1, 09 §3, 10 §3)

- Mod library › Mod settings (`sea16`): current list's Mods with settings files and HugsLib packs, stored values, the last window Archo Monitor saw, editing of simple values while no game runs; CLI `mod-settings`, MCP `get_/set_mod_settings`.
- Settings follow the Mod across folder names on launch (create missing only, never overwrite).
- Game Configs: each has a user data folder (shared or own); Add Game Config "start with" copies the current config's Config files (I11 in the UX note).
- Packs copy the environment's Config once into `userdata/` and never export it (10 §4: "settings are not shared").
- Missing: orphan view, export/import of settings, compare between configs, reset one Mod.

## 4. Options

### 4.1 Showing

- **S1 (now)** — keep the page; add a filter "not in this list" and "not installed" (orphans) so the same table shows all files in `Config\`.
- **S2** — show a Mod's settings in its details pane in the editor (row "settings: stored · edit…"), so players find them next to the Mod.

### 4.2 Per Game Config

- **P1** — nothing new: a Game Config with its own saves folder already has its own `Config\`. Document it in the UI ("settings belong to this Game Config").
- **P2** — "copy settings from…" per Mod or for all, between Game Configs (preview, create-or-replace with a recycle-bin backup). Useful in the 1.7 scenario (S2 of the UX note).

### 4.3 Export with lists

- **X1** — list export (`archo` format) gains an optional "include Mod settings" that embeds the files of the list's Mods (keyed by packageId + class, folder name rewritten on import). Import offers "apply settings" with a per-Mod preview; existing files are kept unless chosen.
- **X2** — settings only travel inside packs (the modpack note's option), not with plain lists.

### 4.4 Cleanup

- **C1** — orphans listed with size and last write; "move to recycle bin" with confirmation. Whitelist not needed if deletion is always to the recycle bin.
- **C2** — per-Mod "reset to defaults" (recycle the file) for the "a setting broke my game" case; Can it run could suggest it when a log error names a settings load failure.

Writes must follow 02 §1: only while no game runs, recycle bin rather than delete, rows added to the file-writes table.

## 5. Recommendation and decisions

Recommend **S1 + C1 + C2** first (small, local, fixes clutter and the reset case), then **P2**, then **X1** together with modpacks. Decisions for the player:

1. Is deleting (to the recycle bin) settings files acceptable at all? 02 §1 now says never delete.
2. Should settings travel with a plain list export (X1) or only with packs (X2)?
3. Per-Mod settings in the editor's details (S2): worth the space?
