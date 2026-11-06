---
name: rimworld-probe
description: Check how RimWorld mods actually behave in the real game. Build a small probe mod from a ready skeleton, run it on a quicktest map in an isolated sandbox, and read [Probe] log lines and a screenshot. Use when a mod bug or conflict needs in-game confirmation, static reading of code and logs cannot settle a cause, or a fix must be shown to work. Windows, RimWorld 1.6 and earlier (Unity builds).
---

# RimWorld probe

A probe is a tiny test mod. It loads with the mods under test, sets up a situation on a quicktest map, lets the game run, logs what happens and exits. You write one class; the skeleton and `scripts/probe.mjs` do the rest.

## Files

| Path | What it is |
| --- | --- |
| `template/src/Scenario.cs` | The only file you edit. The default is a smoke test. |
| `template/src/ProbeRunner.cs` | Lifecycle, logging, screenshot, exit and helpers. Do not edit. |
| `template/Probe.csproj`, `template/About/About.xml` | Builds against the local game's own assemblies, offline. Change the packageId with `new --id`. |
| `scripts/probe.mjs` | `new`, `build`, `run`, `clean`. Usage is in its header. |

## Workflow

1. **Locate** the game folder (has `RimWorldWin64.exe` and `Version.txt`), the Harmony mod folder (packageId `brrainz.harmony`) and each mod under test (folder with `About/About.xml`; read its packageId and `modDependencies`).
2. **Create** the probe outside the game and mod folders: `node scripts/probe.mjs new <work>/probe`.
3. **Smoke test first.** Build and run the unchanged probe with only Harmony. Expect `[Probe] RESULT smoke ok ...`. If this fails, fix the environment before writing any scenario.
4. **Write the scenario** in `Scenario.cs` (see the next section), then `build`.
5. **Run each configuration** with its own `--name`:
   `node scripts/probe.mjs run --game <game> --work <work> --name <cfg> --mods "brrainz.harmony=<dir>,<pkg>=<dir>,probe.scenario=<work>/probe"`.
   `--mods` is the load order of the non-official mods; official content comes first.
6. **Read the output**: the `[Probe]` lines print to the console and go to `<work>/runs/<name>/probe-lines.txt`. The full log is `Player.log` next to it; the screenshot is `userdata/probe.png`. Look at the screenshot when the claim is visual.
7. **Clean up**: `node scripts/probe.mjs clean --work <work>` removes the sandbox junctions safely and keeps the run logs.

A run takes 1 to 3 minutes. Run commands in the background if your tool has a short timeout.

## Writing the scenario

```csharp
public class Scenario : ProbeScenario
{
    public override void OnStartup() { }                      // game start, in load order: record patch state
    public override void Setup(Map map) { }                   // map ready: build the situation
    public override void OnFrame(Map map, int frame) { }      // every frame, also while paused: UI work
    public override string Observe(Map map, int tick) { }     // while time runs: one state line, logged when it changes
    public override bool Done(Map map, int tick) { }          // true ends the run
    public override string Finish(Map map, int tick) { }      // the RESULT summary
    public override TimeSpeed Speed => TimeSpeed.Paused;      // for UI-only checks; default runs Ultrafast
    public override int MaxTicks => 30000;                    // 2500 ticks = 1 game hour
    public override int MaxFrames => 36000;                   // about 60 frames per second
}
```

The run ends when `Done` returns true, a hook calls `EndNow()`, or `MaxTicks` / `MaxFrames` is reached. `Observe` runs only while the clock runs; a paused scenario works in `OnFrame`, one small action per frame (open a window this frame, switch its tab the next, read it the one after), because windows draw between frames.

**Steps and errors.** Call `Step("name")` before each part of the scenario. Every error and exception the game logs from then on counts against that step, and the RESULT line ends with `errors=[setup:0 open-card:2 ...]`, with the first lines of each listed above it. `ErrorCount(step)` and `Errors(step)` read them inside the scenario. Steps before your first call are `startup` (from the probe's load), `setup` and `run`. Errors logged before the probe loads, such as XML load errors, are only in `Player.log`.

Helpers on the base class: `Log`, `Patches(method)` (Harmony owners on a method), `Colonists(map, n)` (spawned colonists, generates more if needed), `SpawnBuilding(map, defName, stuff)`, `ClearSpot`, `CompNamed(thing, typeName)` (a comp from another mod without referencing it), `Call` / `Get` / `Set` (private members by name), `LoadOrder()`, and `ScreenshotTarget` (what the final screenshot selects and centers on). A window still open at the end shows in the screenshot.

Do not reference the assemblies of the mods under test. Reach their types by name with `CompNamed`, `Call`, `Get`, `Set`, `AccessTools.TypeByName`. The probe then builds with the game alone and runs in every configuration.

## Finding the code to call

The game and every mod are .NET assemblies; their source is one decompile away. Decompile once into the work folder and search the text, instead of guessing names:

- Whole assembly (1.6: about 30 s and 45 MB; do it once): `ilspycmd -p -o <work>/decomp/game <game>/RimWorldWin64_Data/Managed/Assembly-CSharp.dll`; the same for each mod DLL under test (`<mod>/<version>/Assemblies/*.dll`).
- One type: `ilspycmd -t <Namespace.Type> <dll>`. All type names: `ilspycmd -l c <dll>`.

Three ways in:

1. **From what the player sees.** Find the visible text of the button, menu entry or dialog in the English language files: `<game>/Data/<Core or DLC>/Languages/English/Keyed/*.xml` (other languages are `.tar`), or `<mod>/Languages/English/Keyed/` for a mod. Its key appears in the code as `"Key".Translate()`. That leads to the gizmo (`Command_Action.action`, `Command_Toggle.toggleAction`), the float-menu option (`FloatMenuOption` action) or the dialog button, and so to the method the player's click really calls. Call that method, or the gizmo's own action.
2. **From what a pawn does.** `Observe` can log `pawn.CurJobDef.defName`. The job comes from a `JobGiver_*` (needs, duties, the think tree in `Data/*/Defs/ThinkTreeDefs`) or a `WorkGiver_*` (work types, `Data/*/Defs/WorkGiverDefs`). Search for `JobDefOf.<Name>` and the giver's class; its `TryGiveJob`, `HasJobOnThing` and `JobOnThing` hold the checks the AI makes.
3. **From a decision that went the wrong way.** Read down the decision function the AI or UI uses. Every `return false` / `return null` on the way is a precondition; call the same checks in the scenario and log each result. Stop at the first one that differs from what you expect.

Mods change these paths. `Patches(method)` shows who patched a method you call or read; read those patch bodies in the mod's decompiled source. Defs are best read at runtime (`DefDatabase<ThingDef>.GetNamed(...)` and its fields), because XML patches from other mods are already applied there.

## Method

- **Act through the same entry points as the player and the AI.** Call the method the UI button or the dialog calls, not an internal shortcut. Change the state the AI reacts to (a need level, a designation, a setting), then let ticks run and watch what the AI decides. Setting the outcome directly proves nothing.
- **One variable per configuration, and a control.** For a suspected conflict: the mod alone, both mods in each load order, and a control that keeps the suspected mechanism and drops the other mod.
- **Name steps.** A failure in one step should not be read as a failure of the whole scenario. With `Step` the result says where each error came from.
- **Log decisions, not only outcomes.** When the expected thing does not happen, call the game's own decision functions for the same subject (the checks the AI or the UI runs before it acts) and log each result. A `false` in that list is the cause; do not guess.
- **Check preconditions before blaming a mod.** A quicktest map is random: the scenario, the ideoligion and its precepts, traits, relations, terrain, season. Any of these can block what you test. Log the relevant ones in `Setup`. If one blocks the test, remove it (for example leave a DLC off with `--no-dlc`) and say so in the report.
- **Let time run.** Read state after the AI has had ticks to act, not in the first frame. For "does X keep happening", keep observing for a while after it first happens.
- **Patch timing.** Mods patch from `[StaticConstructorOnStartup]` in load order. Put the probe between two mods and log `Patches(...)` in `OnStartup` to see which patched first.
- **Repeat only what looks flaky.** If a result is surprising, run the same configuration again before concluding.

## Rules

- Never edit files of the game, the mods under test or Steam. Copy a mod into the work folder before changing it, and pass the copy in `--mods`.
- Keep the work folder outside the game and mod folders. The sandbox uses junctions: delete it only with `clean`, never with a recursive delete, which could follow a junction into the real game.
- The sandbox does not use the player's saves or config. If `run` warns that the real `Prefs.xml` changed, report it and leave the file alone.
- Close the probe's game before starting another run; the runner waits for its own game only.

## Limits

- RimWorld 1.7 (Waylith, .NET 10) is not supported: different assemblies, and the log path is fixed.
- The game runs windowed at 1280x720 with sound off; the screenshot is the game window.
- `-quicktest` builds a small colony with a random world each run. Results that depend on the world need several runs or a scenario that sets the condition itself.
