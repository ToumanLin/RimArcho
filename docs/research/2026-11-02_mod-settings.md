# Mod settings: storage, deployment and editing (research, 2026-11-02)

Status: research and proposal; no product decision or implementation. Based on worktree `claude/mod-config-a11y`, HEAD `f57470b4`. The owner must decide profile scope before implementation.

**The first issue to fix is settings identity during deployment.** In both inspected game versions, ordinary Mod settings are named after the folder the game sees and the Mod subclass's simple name. Deploying the same Mod through `_Archo_…`, `_Archo_env_…` or `!ArchoPack_…` changes that filename. RimArcho creates the links without translating settings filenames. Existing settings stay on disk, but the new registration usually starts with defaults and can write a separate file. Packs copy the old filenames unchanged, so “copy first-play settings” does not reliably carry ordinary Mod settings into a pack. A settings browser and explicit transfer/backup would be useful before general forms inferred from code.

## Evidence and limits

Scope: `D:\RimArcho-test\mod-config\BRIEF.md`. Read-only inspection of game assemblies, installed Mods, player Config and sibling manager sources; no game launch, runtime experiment, player-data write or copying player files into the repository. The only repository output is this report. Decompiled code and aggregate metadata are scratch evidence, not distributable game source. No commits, branch switches or product/spec changes.

Paths below are relative to this worktree unless absolute. `scratch/` means `D:\RimArcho-test\mod-config\`; sibling `RimSort/`, `RimModManager/` and `inventory/` mean directories under `D:\Game\RimModManager_Upgrade`. Line numbers describe the inspected snapshot; another worker may change other files concurrently.

| Evidence | Source and reproduction | Supports |
| --- | --- | --- |
| E1 | 1.6 `D:\rimworld16\RimWorldWin64_Data\Managed\Assembly-CSharp.dll`; game `Version.txt`: `1.6.4871 rev590`; SHA-256 `5CF1B5BE399D5B1C9C56CA72C9D35B4ECF307FEACF5859D04AC5A1AA5926356A` | Storage and Scribe |
| E2 | 1.7 `D:\SteamLibrary\steamapps\common\RimWorld\Assembly-CSharp.dll`; `Version.txt`: `1.7.4995 rev309`; SHA-256 `56399B6144D9F1695F9C53D054DC82972F7AB35E68D1A19A7129F2F5860CCD21` | Cross-version comparison |
| E3 | `internal/app/deploy.go:26`, `envlinks.go:21`, `pack.go:748`, `gametest.go:179`; `rg -n 'DeployName|packLinkName|prepareTestConfig' internal/app` | Deployment and copying |
| E4 | Installed HugsLib, XML Extensions, Mod Settings Framework and Pick Up And Haul assemblies, specified below; targeted `ilspycmd -t` output in scratch | Alternative stores and type/label examples |
| E5 | `scratch/survey.py`, `host-summary.json`, `shape-check.py`; repeat with `python D:\RimArcho-test\mod-config\survey.py` | Host shape and orphan candidates |
| E6 | Sibling manager paths in §5; `rg -n -i 'mod.?settings|copy_config_folder'` in their app/backend trees | Existing tools |

Tool: installed `%USERPROFILE%\.dotnet\tools\ilspycmd.exe`, version `10.1.0.8386`. CLR identity was confirmed with `[System.Reflection.AssemblyName]::GetAssemblyName(...)`; IL headers show managed framework/game-engine references. No obfuscation was apparent. Used static C# decompilation and checked filename arguments, default omission and dialog write calls against IL. No game or Mod assembly was executed.

Reproduce a targeted type without writing beside the DLL:

```powershell
ilspycmd --disable-updatecheck -t Verse.Mod 'D:\rimworld16\RimWorldWin64_Data\Managed\Assembly-CSharp.dll'
ilspycmd --disable-updatecheck -t Verse.LoadedModManager 'D:\SteamLibrary\steamapps\common\RimWorld\Assembly-CSharp.dll'
```

C# evidence is under `scratch/game16/` and `game17/`, named by fully qualified type. ILSpy's `-il -t` emitted the whole assembly in this installed version; relevant methods were retained in `settings-selected.il.txt`, and all bulk IL was removed. Filtered whole-assembly C# searches are `game16-settings-callers.txt` and `game17-settings-callers.txt`. They found only the two vanilla dialog call sites below. Snippets are excerpts, not complete runnable programs.

## 1. What 1.6 and 1.7 actually store

### Filename and folder

E1 and E2 give the same result. `Verse.ModContentPack.FolderName` returns `rootDirInt.Name` (`game16/Verse.ModContentPack.cs:76`). It is the directory entry used to register the Mod, including a junction's name; it is not the target's original basename or the packageId.

```csharp
// Verse.Mod.GetSettings<T>, both builds
modSettings = LoadedModManager.ReadModSettings<T>(intContent.FolderName, GetType().Name);
modSettings.Mod = this;

// Verse.LoadedModManager.GetSettingsFilename, both builds
return Path.Combine(GenFilePaths.ConfigFolderPath,
    GenText.SanitizeFilename($"Mod_{modIdentifier}_{modHandleName}.xml"));

// Verse.ModSettings.Write, both builds
LoadedModManager.WriteModSettings(Mod.Content.FolderName, Mod.GetType().Name, this);
```

The normal scheme is `Config/Mod_<visible root folder>_<Mod subclass simple name>.xml`. The final component is **not** the settings class name. The latter may appear in XML's `Class` attribute. For example `Mod_1279012058_Modbase.xml` contains `Class="PickUpAndHaul.Settings"`. Underscores can occur in either component, so splitting at the first or last underscore cannot identify every file. Multiple Mod subclasses can create multiple files for one package; simple class-name collisions and filename sanitization can cause collisions.

`GenText.SanitizeFilename` replaces invalid-character-separated segments with `_` and trims trailing dots (`game16/Verse.GenText.cs:326`). Do not substitute another sanitizer when predicting paths.

`GenFilePaths.ConfigFolderPath` is `FolderUnderSaveData("Config")` (1.6 `:179`). Default Windows location in both builds is `%USERPROFILE%\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios\Config`, ultimately from the engine persistent-data path. `-savedatafolder=<root>` overrides the whole userdata root, yielding `<root>/Config`. Neither default filename nor root automatically separates 1.6 and 1.7. The game's path getters create absent directories if executed; this research only read their code.

### Reads, writes and failures

| Event | Verified behavior in both builds |
| --- | --- |
| Startup | `LoadedModManager.LoadAllActiveMods` calls `CreateModClasses` (`game16/Verse.LoadedModManager.cs:61`, `:221`). Settings load on a Mod's first `GetSettings<T>()`, often in its constructor; not an unconditional preload of every Config file. Later calls return the cached object. Pick Up And Haul's constructor is an example (`scratch/puah/PickUpAndHaul.Modbase.cs:15`). |
| Settings dialog closes | `RimWorld.Dialog_ModSettings.PreClose` calls that Mod's virtual `WriteSettings` (`game16/…:29`; identical 1.7). |
| Options dialog closes | `RimWorld.Dialog_Options.PreClose` saves Prefs, then calls `selectedMod.WriteSettings()` when selected (`game16/…:770`, `game17/…:873`). |
| Other times | Mods can call `WriteSettings` or `ModSettings.Write` themselves, or override persistence. Vanilla `Mod.WriteSettings` writes only if its settings object was acquired. |
| Exit | No vanilla “write all Mod settings on application exit” call was found in either whole assembly. Do not promise exit persistence. HugsLib has its own quit hook, below. Patches can change this. |

`LoadedModManager.ReadModSettings<T>` checks existence, initializes Scribe, calls `Scribe_Deep.Look(ref target, "ModSettings")`, finalizes loading, then returns `new T()` if target is null. A missing file produces a constructed settings object without creating a settings file. Constructor/field defaults matter.

Malformed XML causes `ScribeLoader.InitLoading` to log, stop Scribe and throw; `ReadModSettings` catches, warns about fresh settings and falls back. Missing/invalid deep types can instead log within `ScribeExtractor.SaveableFromNode` or `Scribe_Deep`, return null and reach the same fallback. The broken file is not repaired/deleted on read; a later write can overwrite it.

Missing value nodes return the `Scribe_Values.Look` supplied default. Syntactically valid XML with an invalid scalar is different: `ScribeExtractor.ValueFromNode<T>` can log and return `default(T)`, rather than the caller's nonzero default (`game16/Verse.ScribeExtractor.cs:14`). Validate values, not only XML syntax.

Unknown nodes are not automatically rejected: `ExposeData` requests particular labels, so unrequested keys are generally ignored, unless the Mod explicitly reads them. They are **not automatically preserved** by the next game write, which serializes the object afresh. This differs from Def XML's field-validation errors.

`WriteModSettings` initializes root `SettingsBlock`, deep-writes `ModSettings`, and finalizes in `finally` (1.6 `:552`, 1.7 `:565`). `ScribeSaver.InitSaving` uses `FileMode.Create` and `FileShare.None` (`game16/Verse.ScribeSaver.cs:27`): a direct rewrite, without backup/atomic replacement. Concurrent offline edits can be lost because the in-memory object is stale even when the game is not currently writing.

## 2. Consequence of RimArcho's links

**Finding F1 (high confidence, E1–E3): ordinary settings can fork silently when the visible folder changes.** This is independent of ModsConfig.xml's `_steam` packageId token. Launching with Steam versus direct launch does not itself change filenames; selecting the Workshop registration versus a linked/local copy does.

| Registration selected by the game | Example normal filename, Mod subclass `ExampleMod` |
| --- | --- |
| Workshop folder `1234567890` | `Mod_1234567890_ExampleMod.xml` |
| Local copy `_1234567890_` | `Mod__1234567890__ExampleMod.xml` |
| Extra/SteamCMD source label `cmd`, relative folder `1234567890` | `Mod__Archo_cmd_1234567890_ExampleMod.xml` |
| Another game's local copy | `Mod__Archo_env_<InstanceID>_ExampleMod.xml` |
| Pack `Colony`, bundled folder `1234567890` | `Mod_!ArchoPack_Colony_1234567890_ExampleMod.xml` |

This is a derived example, not a newly created host file. `DeployName` sanitizes the source label **and `m.Rel`**, replacing `/`, `\` and `:` (`internal/app/deploy.go:26`). `source.Elsewhere` uses `envLinkPrefix + m.ID`; the prefix is `_Archo_env_` (`envlinks.go:21`). `deployCandidate` registers that folder as local (`deploy.go:98`). Windows `source.CreateLink` creates a junction (`internal/source/link_windows.go:68`); the game's folder-name property does not resolve it back to the physical target name.

The impact is conditional:

- Existing game-visible local/Workshop Mods, including Mods exposed through another tool's non-Archo alias, are not automatically redeployed (`deploy.go:43`). Their filename can remain unchanged.
- A new linked registration reads its new expected filename. If absent, it constructs defaults; writing creates the new file. The old file remains recoverable. This is apparent settings loss followed by a fork, not automatic deletion of the original.
- Changing source label/relative directory, locking/moving a copy, using a different local Instance or renaming the Mod subclass may select another file. The same shared Instance with unchanged deployment name and shared userdata can share its file across games; it does not fork on every launch.
- Overridden persistence, HugsLib and fixed custom filenames can behave differently. Do not assume every Mod follows the normal scheme.

No migration of `Mod_<old folder>_<handle>.xml` to `Mod_<new folder>_<handle>.xml` was found in `internal/`. Deployment manipulates links; normal save writes ModsConfig.xml only. The host contains three `_Archo_env_…` files whose settings classes also occur under numeric filenames; all three pairs have different bytes (E5). This corroborates separate persisted states, but does not prove when values diverged or which copy is preferred.

**Finding F2 (high confidence, E1–E3): copying Config is insufficient for packs and some tests.**

- `internal/app/pack.go:748` constructs `!ArchoPack_<sanitized pack name>_<pack folder>`. `packFolder` uses sanitized basename plus uniqueness suffix (`:604`). Renaming a pack can change settings identity too.
- First-play copying (`pack.go:930`) runs only when pack `userdata/Config` is absent. It copies top-level `.xml` except ModsConfig.xml, preserving filenames, without translation for pack links. Copies are best-effort: failures are omitted, without an all-or-nothing guarantee. Existing Config prevents retry of that initialization.
- Graphics/keys and fixed-name Config stores can carry over; Workshop or `_Archo_…` files normally do not serve new pack links. A **same-content player copy reused from Mods/** is the exception (`pack.go:828`, `:834`): the existing registration can keep its matching filename. Do not claim every pack loses every setting.
- Pack copying excludes sibling `HugsLib/` and other custom directories. Settings are independent afterward; later environment edits are not synchronized.
- Tests (`internal/app/gametest.go:179`) refresh top-level Config XML on every run, excluding ModsConfig.xml, Knowledge.xml, Prefs.xml and KeyPrefs.xml. They render a fresh ModsConfig.xml, remove old preferences/Knowledge, and retain caches. They do not translate filenames, copy HugsLib or create deployment links. Matching registrations can use copied files; not-yet-visible deployable Mods are reported missing. Isolated files absent from the source are not comprehensively swept, so reused userdata can retain stale Mod settings.

The brief's references have moved: tests are **08 §12**, first-play copy is **08 §5.3**, and the file-write boundary is **01 §4**, not §3. Also qualify 08 §5.3's promise to copy “Mod settings”: code copies bytes but does not preserve filename identity for new pack registrations. Code behavior is reported here; specs are unchanged.

## 3. Settings beyond the standard per-Mod file

| Store | Verified location and format | Identification and implications |
| --- | --- | --- |
| HugsLib | `<userdata>/HugsLib/ModSettings.xml`; `<settings><ModIdentifier><handle>text</handle>…</ModIdentifier>…</settings>` | `HugsLib.Core.PersistentDataManager.GetSettingsFilePath` combines SaveDataFolderPath, `HugsLib` and filename (`scratch/hugs/…:91`). `ModSettingsManager` specifies filename (`:16`), loads packs and writes root `settings` (`:148`, `:162`). ModIdentifier is author-chosen, not necessarily packageId. |
| XML Extensions | Framework file, e.g. `Config/Mod_2574315206_XmlMod.xml`, `Class="XmlExtensions.XmlModBaseSettings"`; `dataDict` is a string→string Scribe dictionary, keys `modId;key`; also PinnedMods/UI flags | `XmlMod` calls `GetSettings<XmlModBaseSettings>`; `XmlModBaseSettings.ExposeData:25`; `SettingsManager.cs:45`, `:82`. One file covers many XML-only Mods. Framework redeployment can fork all clients' settings together. |
| Mod Settings Framework | Fixed `Config/ModSettingsFrameworkMod_Settings.xml`; `SettingsBlock/ModSettings/modSettingsPerModId`, dictionary keys are lowercase player-facing packageIds, values deep ModSettingsContainer objects | Custom filename/read/write bypass the normal scheme (`scratch/msf/ModSettingsFramework.ModSettingsFrameworkMod.cs:47`, `:78`, `:83`). Containers persist bool `patchOperationStates` and float `patchOperationValues` dictionaries (`ModSettingsContainer.cs:137`). Folder changes alone do not rename this store. |
| Custom Config files | Host has `TrueTerrainColorsCache.xml` (816,375 bytes), `SongDatabase.xml` and the fixed framework file alongside `Mod_*.xml` | Filename alone cannot establish editable configuration, owner or whether data is a regenerable cache. The first two files' provenance was not traced to author code. Do not label them orphan settings. |
| Other userdata | Host has sibling CharacterEditor, OgreStack, PawnEditor, Planning, RimSaves, etc.; no contents or personal preset/save names copied | Presence is evidence of heterogeneous userdata, not a schema. Mods can use XML, JSON, binary or external paths. Beyond inspected frameworks, exact locations/formats need adapters or author documentation; they cannot be enumerated from Verse.ModSettings. |
| Per-save data | `.rws` save XML: Game.ExposeData deep-writes components (`scratch/game16/Verse.Game.cs:443`); World.ExposeData deep-writes world components (`RimWorld.Planet.World.cs:136`) | Mods can store settings in GameComponent/WorldComponent; MapComponent is another possible store. These belong to save work (10), not Config cleanup. No saves inspected. |

Framework DLLs: `D:\rimworld16\steamapps\workshop\content\294100\818773962\v1.6\Assemblies\HugsLib.dll`, `2574315206\1.6\Assemblies\XmlExtensions.dll` and `2023507013\1.6\Assemblies\0ModSettingsFramework.dll` under that same Workshop root. Conclusions describe these installed 1.6 builds; framework behavior on 1.7 was not independently compared.

HugsLib loads on manager construction. Its dialog `PostClose` calls `SaveChanges` (`scratch/hugs/HugsLib.Settings.Dialog_ModSettings.cs:138`); `HugsLibController.OnApplicationQuit` also calls it (`:441`). It writes only with unsaved handle changes. `ModSettingsPack.WriteXml` preserves unclaimed loaded values and omits default handles (`:210`), differing from vanilla unknown-key rewrites. `GetHandle<T>(name, title, description, defaultValue, validator, enumPrefix)` (`:81`) supplies metadata in client code, not the XML. File-wide reset affects several Mods; resetting one requires a group-aware operation.

XML Extensions' `SettingsMenuDef` exposes tKey, setting containers and pre-open/on-frame/post-close actions (`scratch/xmlext/XmlExtensions.SettingsMenuDef.cs:14`). MSF's XML patch operations expose their own controls/defaults. An offline editor could parse supported declarative subsets, but generic XML rendering cannot reproduce those actions or game effects.

## 4. XML, defaults and recoverable labels

A short **real file with no player values**, `Config/Mod_1279012058_Modbase.xml`:

```xml
<SettingsBlock>
  <ModSettings Class="PickUpAndHaul.Settings" />
</SettingsBlock>
```

An empty object is valid: omitted keys can mean defaults. The inspected Pick Up And Haul DLL is `D:\rimworld16\steamapps\workshop\content\294100\1279012058\1.6\Assemblies\PickUpAndHaul.dll`:

```csharp
// PickUpAndHaul.Settings.ExposeData (scratch/puah/PickUpAndHaul.Settings.cs:62)
Scribe_Values.Look(ref _allowAnimals, "allowAnimals", defaultValue: true);
Scribe_Values.Look(ref _maximumOccupiedCapacityToConsiderHauling,
    "maximumOccupiedCapacityToConsiderHauling", 0.8f);
```

Both games' `Scribe_Values.Look<T>` contain this early return (1.6 `Verse.Scribe_Values.cs:29`; identical 1.7):

```csharp
// Verse.Scribe_Values.Look<T>, Saving branch
if (!forceSave && (value != null || defaultValue == null)
    && (value == null || value.Equals(defaultValue)))
{
    return;
}
```

XML is a partial set of stored values, not a schema/complete settings list. Absence is not false/zero or evidence that a Mod has no settings. Conversely forceSave/custom serialization can persist defaults. A default change between Mod builds changes the meaning of absent nodes; copying files does not freeze gameplay semantics across versions.

| API | Disk representation and limits |
| --- | --- |
| Scribe_Values | Illustrative `<allowAnimals>False</allowAnimals>`; scalar bool/int/float/string/enum text has no type annotation. Escaped text, vectors and custom parseable types occur; null can use `IsNull="True"`. |
| Scribe_Collections list/set | `<items><li>…</li><li>…</li></items>`; item encoding depends on LookMode. Empty, absent and null collections differ. |
| Scribe_Collections dictionary | `<data><keys><li>…</li></keys><values><li>…</li></values></data>`; parallel lists whose counts/order must agree. Host Megafauna/XML Extensions files contain this structure. |
| Scribe_Deep | Nested fields, optionally Class when runtime type differs from declared generic T; IsNull for null. Normal top-level ModSettings usually gets Class because its writer uses the base type. A concrete-typed custom writer may omit Class, as the host's fixed MSF file does. |
| Scribe_Defs | Def name text, e.g. illustrative `<selectedThing>Steel</selectedThing>`; type/context comes from code and the enabled Def set. Invalid Def references can log and resolve null. |

Evidence: both builds' Scribe_Values, Scribe_Collections, Scribe_Deep, Scribe_Defs and ScribeExtractor in scratch. Deep construction, collection modes, compatibility code, post-load hooks and conditional ExposeData prevent universal inference from XML alone.

Static extraction can recover field→XML-label mappings, declared types, constant Look defaults, some initializer defaults, enums and simple slider bounds. Track fields and inherited ExposeData calls; wrappers/computed defaults lower confidence. Never execute Mod code or instantiate game objects to discover defaults.

Labels are separate from saved keys. `PickUpAndHaul.Settings.DoSettingsWindowContents:42` binds `_allowAnimals` to `"PUAH.allowAnimals".Translate()` and tooltip `"PUAH.allowAnimalsTooltip".Translate()`. English Keyed strings are at `D:\rimworld16\steamapps\workshop\content\294100\1279012058\Languages\English\Keyed\Translations.xml:6`; SettingsCategory returns literal `Pick Up And Haul` (`PickUpAndHaul.Modbase.cs:24`). Resolve effective version folders, selected language and fallback language.

That example transforms stored occupied-capacity 0.8 into the UI's **20% minimum free inventory space** (`Settings.cs:45`). A raw inferred float control misrepresents its meaning. Dynamic translations, callbacks, Def-generated choices and arbitrary window code make labels/tooltips/ranges best-effort evidence, not an executable form specification.

Display should distinguish “not stored; known default X”, “stored; differs from known default”, “stored; equals known default”, and “default/type unknown”. “Changed since backup” is a separate comparison. Do not call every stored node changed. Resetting a known scalar normally removes its node; writing all absent defaults would override future author defaults.

## 5. Other managers

These are source findings, not UI/runtime testing (E6).

| Manager | What exists | Limits relevant to RimArcho |
| --- | --- | --- |
| RimSort | Opens Config (`RimSort/app/views/main_content_panel.py:1727`); file search includes configs folder (`controllers/file_search_controller.py:815`). Troubleshooting deletes Mod configs (`controllers/troubleshooting_controller.py:218`). Instance config paths can differ; cloning copies Config (`services/instance_service.py:100`). | Delete protects only ModsConfig.xml and Prefs.xml, then unlinks all other top-level files, after an irreversible-action confirmation (`:298`); not schema-aware reset/orphan detection. No dedicated Mod-settings viewer/typed editor or automatic per-list settings profile found in the inspected Python app. Instance cloning is coarse manual isolation. ModsConfig backups/export (`troubleshooting_controller.py:379`, `:418`) do not back up Mod settings. |
| RimCrow | Dedicated settings modal: per-Mod files, installed/disabled/uninstalled/unknown groups, preview/open, overwrite active from another file, delete inactive files and cleanup candidates (`RimModManager/frontend/src/features/mod/ModSettingsManagerModal.vue:434`, `:438`, `:464`). Inventory confirms intended UI (`inventory/ui-pages.md:301`). Backend `managers/mgr_mod_settings.py:23` scans Mod_*.xml, matches folder aliases/Workshop/package candidates, reads Class and groups variants; `sync_group_instance:107` requires same Mod/settings group and active target. | Sync uses direct `shutil.copyfile` (`:157`); no backup, atomic write or running-game guard appears in that operation or `backend/api.py:3453`. UI confirms overwrite. Matching heuristics are prior art, not authoritative game identity. Preview infrastructure is not evidence of a typed form. No independent settings snapshot/profile-switching mechanism found in this manager path; active environment changes the inspected Config path. |

No third-party backup-Mod capability is claimed: an authoritative external source was not verified. Backup of a load order must not be presented as backup of its settings.

## 6. What RimArcho already has

| File:line / specification | Existing responsibility |
| --- | --- |
| `internal/env/detect.go:216`, `:249`; `internal/env/env.go:123` | Default userdata discovery and Config/ModsConfig.xml path composition; custom/own/shared roots recorded on the environment. |
| `internal/env/modsconfig.go:19` | Read-only parsed view. Its “M1 never writes” comment is historical; app/loadout writes exist. |
| `internal/loadout/modsconfig.go:34`, `:115`, `:204`, `:208` | Parse/render enabled list, preserve unowned XML, single .rimarcho.bak, temporary-file/flush/rename. This is for ModsConfig.xml, not general Mod settings. |
| `internal/app/loadout.go:1003`, `:1033`; `internal/app/launch.go:133` | Save/launch projection, links and userdata override. Ordinary save may run while game runs (08 §2); do not reuse that permission for settings edits. |
| `internal/app/deploy.go:26`, `:98`; `envlinks.go:21`; `internal/source/link_windows.go:68` | Normal/cross-environment names and physical junction creation; no settings translation. |
| `internal/app/pack.go:748`, `:930` | Pack registration names and first-play top-level XML copying. |
| `internal/app/gametest.go:179`; `internal/gametest/settings.go:16`, `:22`, `:48`; `paths.go:62`; `run.go:106` | Test copies, preference reset/error classification, path checks and menu-marker handling. |
| `internal/gametest/settings_test.go:14`, `:22`, `:48` | Synthetic tests prove old preferences removed while synthetic ModSettings.xml/cache survive; classify preference failures. They do not prove settings survive folder-name changes or framework directories are copied. |
| `docs/specs/01-scope-and-status.md` 01 §4, rows at `:93`, `:108` and test-data row | Write allowlist permits environment ModsConfig.xml/backup, pack first-play copies and managed test data. It does not authorize general player Mod-settings writes/deletions. A product feature needs an explicit addition. |
| 02 §2.4; 04 §4.1–4.3; 08 §1–4, §5.3, §12 | Shared userdata, list serialization/attribution, deployment, process checks, packs/tests. Settings contents have no equivalent ownership/history/profile contract. |
| 05 §1 / §4 / §5; 06 §4.2, §2.9, §2.14, §9 | Shared operation layer and possible UI homes. `config set` edits RimArcho settings, not Mod settings. No general Mod-settings API/view/edit command found. |

Production searches covered `internal/` and `docs/specs/`, distinguishing testkit fixtures from application code. No test suite was run: only documentation changed, and actual game/data execution is excluded by the brief.

## 7. Real-world shape on this host

E5 surveyed the default Windows Config folder's top level, parsing XML in place. No player values exported. This is a snapshot, not a benchmark or complete installed-Mod registry.

| Measure | Observation |
| --- | --- |
| All regular files | 253; 5,398,970 bytes (about 5.15 MiB); median 317 bytes |
| Extensions | 251 XML, one .txt, one .bak |
| Standard Mod_*.xml | 244; 4,533,388 bytes; all parse, all have root SettingsBlock |
| Class metadata | 243 have Class; one custom file has no Class |
| Numeric/wrapped-numeric folder patterns | 240 files, including nine `Mod__<ID>__…` names |
| Other standard filenames | Three _Archo_env_… names and one readable folder name |
| Repeated settings classes | 12 groups / 25 files; every group has at least two different byte contents. Class equality alone does not establish common ownership/compatible versions. |
| Structures | 292 nodes with parallel keys/values, 825 list-containing nodes, five IsNull attributes; node counts, not file counts |

The Archo examples are RimSaves, Core (AM settings) and HATweakerMod; generated Instance IDs remain in scratch metadata. Their numeric counterpart files remain present. These are existing artifacts; this task created none.

Largest files:

| File | Bytes |
| --- | ---: |
| Mod_3009304695_ModlistConfigurator.xml | 1,250,269 |
| Mod_3586136174_ModlistConfigurator.xml | 854,826 |
| TrueTerrainColorsCache.xml | 816,375 |
| Mod_Caravan Item Selection Enhanced_Settings_CaravanUI.xml | 627,085 |
| Mod_2138635288_TabSortingMod.xml | 447,426 |
| Mod_3530785055_ASGSMod.xml | 404,449 |

**Orphan estimate, not a deletion list.** Surveyed first-level entries with About/About.xml in six roots: 1.6 Mods (3,745), its Workshop (1,225), Steam 1.7 Mods (265), Steam Workshop (1,283), and both inspected RimCrow Mod roots (22 each). Full roots are in `host-summary.json`. Counts are directory entries, including aliases/copies, not distinct packages. Exact visible-folder-prefix matching found 239/244 files. Four of the 240 numeric/wrapped-numeric files had IDs absent from those roots: 2576219203, 2598436855, 2854967442, 3009304695. The fifth unmatched file has a readable folder name.

These are **candidate stale identities**. Three of the four numeric candidates have the same Class under another installed numeric identity, supporting rename/replacement possibilities, not whole-Mod uninstallation. Other roots, locked copies, retired environments and manual renames were not exhaustively reconciled. A complete count of truly uninstalled Mods could not be determined. HugsLib's separate settings file is 44,508 bytes, root settings, three groups; outside the Config counts.

## Proposal: inspect first, preserve identity before general editing

Everything below is proposed, not implemented. F1/E1–E3 leads to explicit filename mapping/transfers; F2/E3–E4 leads to framework-aware copying. E4–E5 leads to conservative editing/cleanup with uncertain ownership visible.

### Display across surfaces

Put a **Mod settings** action on shared full Mod details (06 §4.2), reachable from list/inventory. A view under mod library (06 §2.9) supplies the cross-Mod file list, unknown files and backups. RimArcho's Settings (06 §2.14) can link there and open the selected game's Config folder. Identify the game/userdata root or pack being inspected; shared roots must visibly be shared.

Start with filename, size/time, owner/confidence, the current launch's expected filename, other variants, read errors and a read-only XML tree/raw preview. Unknown fields remain inspectable. Explain “this copy will use a different settings file” before launch when applicable. List all files for multi-handle Mods; group framework clients inside their shared file rather than inventing one disk file per client.

TUI uses the same list/details through its command panel, with navigable tree/full paths; read-only first, no key bindings chosen here. CLI could use `archo mod-settings list/show/diff`; MCP could add `read_mod_settings`, returning the same text through the shared command layer (05 §1). Names are provisional. Use stable file references constrained to the selected userdata root instead of arbitrary writable paths. Future mutations need explicit actions, dry-run support and surface parity.

Typed forms come later: extract supported patterns from the exact effective DLL/content hash and game build using the source library (03 §5.4). Show provenance/confidence and the default states in §4. Resolve Keyed labels when proved; otherwise show keys. Do not decompile the library on page opening. Enumerate cheaply, parse selected files, bound preview/tree size and make optional inference cancellable to meet 06 §10 / 07 §6.

### Editing and management safeguards

1. **Exclude running games.** Before mutation, recheck 08 §4 across environments/shared userdata roots. Refuse unknown process state; FileShare.None is insufficient. Recheck file identity/hash after preview for outside edits.
2. **Back up first.** Keep original bytes and a manifest of root, game build, Mod/Instance/Lineage, visible folder, handle/class, source DLL hash and time. Backup failure means no edit. Retention/location remain decisions; restore also backs up current state.
3. **Validate and atomically replace.** Disable external entities/DTDs; bound parsing; validate supported types, finite numbers/enums/bounds and dictionary cardinality. Write/flush a same-directory temporary file, then replace. Revalidate roots/paths and refuse traversal, redirected files or unsafe links. New files require ownership evidence. Product writes need a new 01 §4 entry.
4. **Preserve unknown content.** Change only the selected scalar/node; retain unknown nodes/attributes, ordering and untouched text. Do not serialize through a struct that drops keys. If preservation is unsafe, retain read-only and offer export/open externally. Valid XML is not proof that the game/Mod accepts it.
5. **Offer narrow useful edits.** Known bools/enums/numeric/string scalars with recovered semantics, removal of known keys to inherit defaults, explicit backed-up whole-file reset. Exclude arbitrary Class edits, deep objects, Def references and collection restructuring from the first editor. Explain the game can drop unknown keys on its next write.
6. **Separate cleanup from disablement.** Disabled, absent from current list, unmatched filename and uninstalled differ. Show candidate evidence/variants and require selection. Backup/recycle instead of permanent deletion, after extending write/recycle boundaries. Do not remove shared framework files because one client vanished; per-client cleanup needs adapters.
7. **Transfer identity explicitly.** Resolve old/new visible folders, owning instance/lineage, Mod handle and class/schema; preview copying to the filename the next launch reads. Preserve source. Never silently pick a variant or overwrite a destination. PackageId/Class alone is insufficient. Using raw Workshop IDs as deployment names is not a general fix: separate copies and registration rules still require distinct identity.

Reset means “remove selected persisted state and let this Mod build construct defaults”, not “write a guessed empty object”. Explain restart requirements and whether a framework group or whole file is selected. Backup/restore must distinguish standard files, framework groups and custom stores; unknown stores stay excluded by default.

### Profiles, exports and version boundaries

Profile scope is **undecided**. The owner must choose per-game versus global availability before a schema is selected. Physical settings apply to a userdata directory that several environments may share; keep this explicit regardless of profile catalog scope. Per-list/per-plan settings could be launch-applied snapshots, or lists could keep shared live files with manual restore. Do not silently switch contents when merely browsing/changing a list.

For an approved profile design, record game major/minor/build and Mod content identity; isolate 1.6/1.7 profile application by default. This would be RimArcho policy: both games currently share default Config. Cross-version restore needs preview and explicit compatibility decisions; Class equality is insufficient.

Plan exports could optionally carry selected snapshots/provenance, excluding absolute user paths, saves, caches and decompiled game code. Pack build/import could carry a settings snapshot plus destination-name mapping and HugsLib/MSF/XML Extensions adapters, initialized once into pack userdata. Sharing is opt-in because settings can hold personal paths/presets. Exclude unknown custom files until identified; never automatically export all Config/userdata.

### Phases and owner decisions

| Phase | Smallest useful outcome | Specs to change if approved |
| --- | --- | --- |
| A: read-only inventory | Open Config, standard/framework list/preview, expected-name/variant diagnostics, ownership uncertainty, backup export | 03 §4 if prelaunch findings; 05 §4/§5; 06 §2.9/§4.2/§9; 01 §4 and 07 §3 for backup storage |
| B: preserve deployment settings | Explicit one-time transfer across registrations; fix pack first-play mapping; define test copying/cleanup | 08 §3/§5.3/§12; 04 §4.2/§7; 02 §2.4; 01 §4; shared actions in 05/06 |
| C: safe management/editing | Supported scalar edits with preservation; backed-up reset/restore/selected cleanup; framework group adapters | 01 §4; 03 §5.4 inference; 05 §4/§5; 06 §2.9/§4.2/§6; 07 §3; 08 §4/§7 |
| D: optional profiles/export | Only after scope decisions: launch application, version policy, optional pack/plan settings | 02 §0/§2.4; 04 §1/§3/§9; 08 §2/§5; 05/06; 10 only for later save-specific settings |

Implementation phases require CHANGELOG/release-VM-test updates under AGENTS.md. VM acceptance must cover numeric→Archo→pack transitions, divergent variants, handle/class renames, shared userdata, malformed scalars/files, default/absent/unknown keys, dictionaries, HugsLib groups, stale test files, 1.6/1.7 and a running game. Unit fixtures must be synthetic; actual game reading belongs only in the VM.

Owner decisions still required:

1. Per-game or global profiles; list-owned or reusable snapshots? Until answered, do not choose storage scope.
2. Offer transfers at launch when exactly one compatible source exists, or keep migration separate? What wins when destination exists?
3. Share settings for locked copies in one Lineage or keep independent states? What evidence permits transfer across a changed DLL/handle/class?
4. Capture pack settings at build or first launch? Which framework groups initially, and how should initialization failures/retries work?
5. Faithful framework settings or clean test runs? Sweep stale isolated Mod files every run?
6. Which typed patterns/frameworks first; should unsupported data remain read-only? Backup location/retention?
7. Is conservative candidate cleanup enough, or must complete source/Lineage reconciliation precede orphan removal?

Unverified: live game/patch behavior, arbitrary-Mod/framework schema compatibility on 1.7, truly uninstalled counts, arbitrary custom-store ownership/formats, and external backup-Mod capabilities. These gaps do not affect the statically verified folder-name problem. All seven questions have evidence or explicit limits; the proposal is ready for a later owner decision.
