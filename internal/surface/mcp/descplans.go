package mcp

// Descriptions of the tools in plans.go (05 §5.3): what the tool does, when to use it,
// what it writes, and an example call with the first lines of output.

const descLaunch = `Start the game with the current plan ("启动游戏", 08 §2). It first writes the plan to the game exactly like save_loadout (Mods/ links synced, pack links removed, ModsConfig.xml backed up and atomically replaced for the current game version), records the launch (and a launch version when the list differs from the newest one), then starts RimWorld directly or through Steam, as the environment says, without waiting for it. Refused while any game runs, in any environment: the text names the environment and what it is playing, and nothing is written. It starts a program and changes the list the game loads: call with dry_run=true first (the save_loadout preview plus the command line), show the player, and ask before the real call. Use save_loadout instead when the player starts the game some other way (Steam). After the game exits, analyze_log reads its Player.log.
Example: launch_game {"dry_run": true}
  launch: dry run (direct), nothing written
    path: C:\Users\me\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios\Config\ModsConfig.xml
    version: 1.6.4633 rev1261`

const descPlans = `The environment's plans: named mod lists, each with its own versions, undo steps and staging. Every other editing tool acts on the current plan. action=list (default) shows them (active count, versions, last played, which is current); new name=<n> creates one without switching to it — from=current copies the current plan, from=v<n> one of its versions, from=<list file> reads a list, no from = an empty list; use name=<n> makes a plan current (writes no game file: save_loadout or launch_game does); rename name=<old> new_name=<new> keeps the history; remove name=<n> deletes a plan with its whole history (never the last plan); import from=<file or RimCrow share code> name=<n> makes a list a new plan (exchange_list action=import replaces the current plan instead); versions name=<n> shows the game versions the plan is meant for (default: those every mod lists plus the game it was made on) and how many mods do not list each, versions=<1.5-1.7|default> sets them (not a list edit; writes no game file). Marked destructive because remove cannot be undone: ask the player before removing, naming the plan. The other actions change RimArcho's database only.
Example: manage_plans {"action": "new", "name": "raid night", "from": "current"}
  plan:raid night created: 3 active
    v1: import, copied from default v1
    current: plan:default (unchanged)`

const descHistory = `The current plan's history (read-only). action=log (default) lists its versions — each a list written to or read from the game: v<n>, time, source (import; external = changed outside RimArcho, e.g. in the game's mod menu; user = save_loadout; launch = launch_game; undo = a checked-out version written again), entries, note. action=diff shows what was added, removed, moved or replaced between from and to (v<n>, <n> or workspace; default: the newest version → the current list). action=steps shows the undo steps since then and the newest versions (all=true: every version). To go back to a version: edit_loadout action=checkout version=v<n> (one undoable edit; writes no game file). Use with list_changes when the player says "it worked last time".
Example: plan_history {"action": "diff", "from": "v1"}
  diff v1 → workspace: 1 change
    removed  #hnyg  Harmony  was 2`
