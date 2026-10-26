package mcp

const descTestRun = `Launch the game directly with a plan in isolated test data, wait for the main menu (or a quick test map with map=true), capture startup errors, then stop the process. This can take minutes (timeout_seconds defaults to 600). Never touches the player's saves, configuration or logs, and never writes the game directory. Refuses while a game is already running. Mod folders the game cannot see are reported as missing; no links are created. Returns a JSON report with status, timings, peak memory, missing mods and error clusters attributed against the tested plan. Keeps the newest 10 run directories and reuses the test data's caches. Use to check whether a plan can finish loading; status menu/map with no error clusters is success.
Example: test_run {"plan": "small", "timeout_seconds": 300}
  {"env":"main","plan":"plan:small","status":"menu","load_seconds":42.5,"clusters_total":0}`

// Descriptions of the tools in manage.go (05 §5.3): what the tool does, when to use it,
// what it writes, and an example call with the first lines of output.

const descRules = `User rules on load order (shared by every Loadout of the environment): action=add with kind after|before (A must load after/before B), incompatible (A B), attach (A always right after B), top / bottom (A only: load A as early / late as its constraints allow; a preference, like the community loadTop/loadBottom), or ignore (target = a constraint ref printed by sort or list_problems, e.g. decl:#ab12:loadafter:other.pkg, to drop a wrong About.xml/community constraint — this is how an order.cycle is broken); action=list shows the rules with their r:<n> refs; action=remove rule=r:<n> deletes one. Rules apply to sort and to the order problems; they do not reorder by themselves (edit_loadout action=sort does). Writes RimArcho's database only, never game files. add_rule is the old name of this tool.
Example: manage_rules {"action": "add", "kind": "after", "a": "#76gh", "b": "pkg:brrainz.harmony", "note": "needs its patches"}
  r:1 added
    r:1  after  L#gpto B  L#u2mx Harmony  needs its patches
  next:`

const descRuleAlias = `Old name of manage_rules (same arguments; action defaults to add), kept for one minor version. Use manage_rules.
Example: add_rule {"kind": "ignore", "target": "decl:#iqmu:loadafter:tester.y"}
  r:1 added
    r:1  ignore  decl:#iqmu:loadafter:tester.y
  next:`

const descIgnore = `Ignore a problem (ref = p:<hex> from list_problems) so it leaves the default lists and counts until the files of its mod change; undo=true stops ignoring it. Use when the player has looked at a problem and decided it is fine (a known-harmless warning, a deliberate override) — ask them first, never just to make a list look clean. list_problems ignored=true lists what is ignored. Writes RimArcho's database only.
Example: ignore_problem {"ref": "p:c86d19"}
  ignored p:c86d19 mod.hidden
    #z3bw Alone is hidden from the game (About.xml renamed, probably by another mod manager)
    until: the mod changes (ignores are kept per lineage and expire when its files change)`

const descScan = `Scan the environment's sources (game Data, Mods/, Workshop, SteamCMD and extra folders) and reconcile what is on disk with RimArcho's records: new, updated, moved, copied and vanished mods become change events (list_changes), and a folder that might be a mod seen before becomes an identity question (resolve_identity action=list). Use after the player installed, updated or removed mods outside RimArcho, or when get_mod / list_changes look out of date; full=true re-hashes every semantic file (slow; for suspected stale data). Writes RimArcho's database only; no mod or game file is touched.
Example: scan {}
  scan:3  env:main  0s
    sources:
      src:official  1 mod`

const descExchange = `Exchange the Loadout with list files and Workshop collections. action=export returns the Loadout as text (format: text, archo — a RimArcho list, each mod with its version so whoever imports it sees which match —, modsconfig, rimsort, rml, rimcrow share code, rimcrow-xml, or workshop — Markdown Workshop links for subscribing by hand), or writes it to out=<new file> (an existing file is never overwritten). action=import from=<file or RimCrow share code> replaces the Loadout with that list (a RimArcho list, text, ModsConfig.xml, a save, .rml, RimSort, RimCrow, workshop IDs; from a RimArcho list the installed copy with the listed version is used when there is one). action=collection from=<Workshop collection link or ID> reads the collection over the network and compares it with the installed mods; import=true also replaces the Loadout with its installed mods in collection order. An import is one undoable Loadout edit (edit_loadout action=undo) and never writes game files; ask the player before replacing their Loadout. Mods that are not installed are listed, never downloaded.
Example: exchange_list {"action": "collection", "from": "3000000001"}
  collection Title 3000000001: 3 items
    installed: 2
      #hnyg Harmony`

const descPacks = `Packs: a Loadout compiled into a self-contained folder with its own mod copies, list and saves (sorted, checked, version folders this game does not load left out). action=list shows the packs; build name=<n> compiles the current Loadout (textures=true converts the copies' PNGs to DDS and deletes them, texture_scale also scales them down; refused while problems block it — see list_problems); launch name=<n> links the pack's mods into Mods/, uses its own list and save folder and starts the game; export name=<n> file=<new .archopack> (list=true: only its list, as a RimArcho list .archolist.json — each mod with its version, for sharing); import file=<…> unpacks a pack, or compares a RimArcho list with the installed mods (as_plan=true also loads it into the Loadout, undoable); remove name=<n> moves a pack to the recycle bin (unfinished=true: deletes what an interrupted build copied); stop name=<n> stops a build running in another RimArcho window; unlink removes the packs' !ArchoPack_ links from Mods/ (only the links: list_problems reports pack.links-left when a pack's links are still there while it is not played, and a game started from Steam would load the pack's copies). build, launch, remove and unlink take dry_run=true: call it first and show the player. A build copies gigabytes, launch starts the game and changes Mods/, remove deletes: ask the player before each.
check name=<n> (or file=<archive>) compares the game build and DLCs and verifies mod copies; archives are checked without extraction. check is read-only and needs no confirmation. Errors block launch; build differences are warnings.
Example: manage_packs {"action": "build", "name": "first", "dry_run": true}
  pack build: dry run, nothing written
    folder: D:\RimArcho\packs\first
    mods: 2 carried, 1 from the game (Core and DLCs)`

const descIdentity = `RimArcho's identity of mods (instances #id grouped in lineages L#id). action=list shows open questions ("is this new folder the mod that vanished?"; all=true includes answered ones); resolve question=q:<n> answer=same (the same mod came back: it keeps its ID, lineage, notes, tags and history), copy (a copy of it: new instance, same lineage) or unrelated; split refs=[#id] moves an instance into a lineage of its own; merge refs=[L#keep, #id or L#other] merges the second lineage into the first. Use when status or scan reports questions, or when copies of one mod are tracked apart (or two different mods together). Answers rewrite RimArcho's history and are hard to undo: ask the player, quoting both candidates from action=list. No game or mod file is touched.
Example: resolve_identity {"action": "list"}
  questions: 1 open
    q:1  score 1.00  scan:3
      new:     #np4v  Leftover  [local Leftover Again]`

const descHidden = `Mods another manager hid by renaming About.xml to About.xml.disabled (problems mod.hidden, mod.hidden-residue, mod.pinned). action=unhide ref=<mod> renames it back so the game sees the mod again (an existing About.xml is never overwritten); action=trash ref=<mod> moves a hidden or unsubscribed Workshop folder to the recycle bin (local Mods/ folders are refused). Both rescan afterwards. These change mod folders on disk: call with dry_run=true first, show the player and ask.
Example: manage_hidden {"action": "unhide", "ref": "ws:4300", "dry_run": true}
  dry run, nothing changed: would be renamed
    from: D:\Steam\steamapps\workshop\content\294100\4300\About\About.xml.disabled
    to: D:\Steam\steamapps\workshop\content\294100\4300\About\About.xml`

const descMaintenance = `RimArcho's own data. action=data_update downloads the community data (datasets: rules, steamdb, replacements; default all) — the only call that fetches it, over the network; status show=data shows what is there. action=import_rimcrow db=<RimCrow's data\mod_manager.db> imports RimCrow's aliases, notes, groups (as tags) and user rules (rules=<user_rules.json>; include_disabled_rules=true); RimCrow's files are only read, the import can be repeated and keeps RimArcho's values on conflict. action=gc removes expired tombstones and superseded snapshots (their history is gone afterwards). All take dry_run=true: call it first, show the player, then ask before the real run.
Example: maintenance {"action": "data_update", "datasets": ["rules"], "dry_run": true}
  data update: dry run, nothing downloaded
    rules: https://raw.githubusercontent.com/RimSort/Community-Rules-Database/main/communityRules.json`
