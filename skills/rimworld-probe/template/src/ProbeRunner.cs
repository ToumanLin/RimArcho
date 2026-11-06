// Probe runner: lifecycle, logging, screenshot and exit. Do not edit; write the scenario in Scenario.cs.
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Reflection;
using HarmonyLib;
using RimWorld;
using UnityEngine;
using Verse;

namespace RimProbe
{
    /// <summary>
    /// Base class for one probe scenario. Exactly one non-abstract subclass must exist in the assembly.
    /// Order: OnStartup (game start) -> Setup (map ready) -> then, until the end, OnFrame every frame (also while
    /// paused) and Observe/Done every ObserveEvery ticks (only while time runs) -> Finish -> screenshot ->
    /// "[Probe] RESULT ..." -> the game quits (with -probequit).
    /// The run ends when Done returns true, EndNow() is called, or MaxTicks / MaxFrames is reached.
    /// </summary>
    public abstract class ProbeScenario
    {
        /// <summary>Called from [StaticConstructorOnStartup], in mod load order. Use it to record patch state.</summary>
        public virtual void OnStartup() { }

        /// <summary>Called once, when the first map has been running for a few frames. Build the situation here.</summary>
        public abstract void Setup(Map map);

        /// <summary>Called every frame after Setup, also while paused. Use it for UI: open windows, switch tabs, read gizmos.
        /// frame counts from 0 after Setup.</summary>
        public virtual void OnFrame(Map map, int frame) { }

        /// <summary>Return one line describing the state; it is logged only when it changes. Null logs nothing.</summary>
        public virtual string Observe(Map map, int tick) { return null; }

        /// <summary>Return true to end the run. Checked every frame and every observed tick.</summary>
        public virtual bool Done(Map map, int tick) { return false; }

        /// <summary>Called once at the end. Return the result summary (logged as "[Probe] RESULT ...").</summary>
        public virtual string Finish(Map map, int tick) { return ""; }

        /// <summary>Game ticks to run at most. 2500 ticks = 1 in-game hour, 60000 = 1 day.</summary>
        public virtual int MaxTicks => 30000;

        /// <summary>Frames to run at most; the limit that ends a paused scenario. About 60 frames per second.</summary>
        public virtual int MaxFrames => 36000;

        /// <summary>Call Observe and Done every this many ticks.</summary>
        public virtual int ObserveEvery => 1;

        /// <summary>The clock while the scenario runs. TimeSpeed.Paused for UI-only checks (then only OnFrame runs).
        /// Ultrafast needs dev mode (the runner turns it on).</summary>
        public virtual TimeSpeed Speed => Prefs.DevMode ? TimeSpeed.Ultrafast : TimeSpeed.Superfast;

        /// <summary>What the final screenshot centers on and selects. Null: no selection, camera stays.</summary>
        public Thing ScreenshotTarget;

        /// <summary>End the run at the next frame, from any hook.</summary>
        public void EndNow() { endRequested = true; }
        internal bool endRequested;

        // ---- steps and errors ----

        /// <summary>Name the current step. Every error and exception the game logs from now on is counted against it,
        /// and listed in the result. Steps before Setup are "startup" and "setup".</summary>
        public static void Step(string name)
        {
            ErrorCapture.Current = name;
            Log("step " + name);
        }

        /// <summary>Errors and exceptions logged during a step so far (all steps when step is null).</summary>
        public static int ErrorCount(string step = null) => ErrorCapture.Count(step);

        /// <summary>The first lines of the errors logged during a step (all steps when step is null).</summary>
        public static IEnumerable<string> Errors(string step = null) => ErrorCapture.Lines(step);

        // ---- helpers ----

        public static void Log(string line) { Verse.Log.Message("[Probe] " + line); }

        /// <summary>Patch owners on a method, e.g. Patches(AccessTools.Method(typeof(X), "Y")).</summary>
        public static string Patches(MethodBase method)
        {
            if (method == null) return "method-not-found";
            Patches p = Harmony.GetPatchInfo(method);
            if (p == null) return "none";
            string f(IEnumerable<Patch> l) => string.Join(",", l.Select(x => x.owner));
            return $"prefix=[{f(p.Prefixes)}] postfix=[{f(p.Postfixes)}] transpiler=[{f(p.Transpilers)}] finalizer=[{f(p.Finalizers)}]";
        }

        /// <summary>Spawned free colonists; generates and spawns more near the map center if there are fewer than count.</summary>
        public static List<Pawn> Colonists(Map map, int count)
        {
            List<Pawn> list = map.mapPawns.FreeColonistsSpawned.ToList();
            while (list.Count < count)
            {
                Pawn p = PawnGenerator.GeneratePawn(PawnKindDefOf.Colonist, Faction.OfPlayer);
                GenSpawn.Spawn(p, CellFinder.RandomClosewalkCellNear(map.Center, map, 8), map);
                list.Add(p);
            }
            return list.Take(count).ToList();
        }

        /// <summary>Make and spawn a thing for the player on a clear spot near the map center.</summary>
        public static Thing SpawnBuilding(Map map, string defName, string stuffDefName = null, Rot4? rot = null)
        {
            ThingDef def = DefDatabase<ThingDef>.GetNamed(defName);
            ThingDef stuff = stuffDefName != null ? DefDatabase<ThingDef>.GetNamed(stuffDefName)
                : (def.MadeFromStuff ? GenStuff.DefaultStuffFor(def) : null);
            Thing t = ThingMaker.MakeThing(def, stuff);
            if (t.def.CanHaveFaction) t.SetFaction(Faction.OfPlayer);
            Rot4 r = rot ?? Rot4.North;
            return GenSpawn.Spawn(t, ClearSpot(map, def.size, r), map, r);
        }

        /// <summary>A cell near the map center where a thing of this size fits with a free 1-cell margin.</summary>
        public static IntVec3 ClearSpot(Map map, IntVec2 size, Rot4 rot)
        {
            foreach (IntVec3 c in GenRadial.RadialCellsAround(map.Center, 50f, true))
            {
                CellRect r = GenAdj.OccupiedRect(c, rot, size).ExpandedBy(1);
                if (r.Cells.All(x => x.InBounds(map) && x.Standable(map) && x.GetEdifice(map) == null
                    && x.GetFirstPawn(map) == null && !x.Fogged(map) && x.GetFirstItem(map) == null))
                    return c;
            }
            throw new Exception("no clear spot near the map center");
        }

        /// <summary>Call a method by name, private or not: Call(obj, "Toggle"), Call(typeof(X), "StaticM", arg).</summary>
        public static object Call(object target, string method, params object[] args)
        {
            Type t = target as Type ?? target.GetType();
            MethodInfo m = AccessTools.Method(t, method, args.Select(a => a?.GetType()).ToArray()) ?? AccessTools.Method(t, method);
            if (m == null) throw new MissingMethodException(t.FullName, method);
            return m.Invoke(target is Type ? null : target, args);
        }

        /// <summary>Read a field or property by name, private or not.</summary>
        public static object Get(object target, string member)
        {
            Type t = target as Type ?? target.GetType();
            object o = target is Type ? null : target;
            FieldInfo f = AccessTools.Field(t, member);
            if (f != null) return f.GetValue(o);
            PropertyInfo p = AccessTools.Property(t, member);
            if (p != null) return p.GetValue(o, null);
            throw new MissingMemberException(t.FullName, member);
        }

        /// <summary>Write a field or property by name, private or not.</summary>
        public static void Set(object target, string member, object value)
        {
            Type t = target as Type ?? target.GetType();
            object o = target is Type ? null : target;
            FieldInfo f = AccessTools.Field(t, member);
            if (f != null) { f.SetValue(o, value); return; }
            PropertyInfo p = AccessTools.Property(t, member);
            if (p != null) { p.SetValue(o, value, null); return; }
            throw new MissingMemberException(t.FullName, member);
        }

        /// <summary>The comp of a thing whose class has this full or short name (for comps from other mods).</summary>
        public static ThingComp CompNamed(ThingWithComps thing, string typeName)
        {
            return thing.AllComps.FirstOrDefault(c => c.GetType().FullName == typeName || c.GetType().Name == typeName);
        }

        /// <summary>Active non-official mods in load order.</summary>
        public static string LoadOrder()
        {
            return string.Join(">", LoadedModManager.RunningModsListForReading.Where(m => !m.IsOfficialMod).Select(m => m.PackageId));
        }

        internal static ProbeScenario instance;
        internal static ProbeScenario Instance
        {
            get
            {
                if (instance != null) return instance;
                Type[] types = typeof(ProbeScenario).Assembly.GetTypes()
                    .Where(t => !t.IsAbstract && typeof(ProbeScenario).IsAssignableFrom(t)).ToArray();
                if (types.Length != 1) throw new Exception($"expected exactly 1 ProbeScenario subclass, found {types.Length}");
                instance = (ProbeScenario)Activator.CreateInstance(types[0]);
                return instance;
            }
        }
    }


    /// <summary>Counts every error, exception and assert the game logs, per step (ProbeScenario.Step).</summary>
    internal static class ErrorCapture
    {
        const int KeepLines = 5;
        static string current = "startup";
        static readonly List<string> order = new List<string>();
        static readonly Dictionary<string, int> counts = new Dictionary<string, int>();
        static readonly Dictionary<string, List<string>> lines = new Dictionary<string, List<string>>();
        static bool started;

        internal static string Current
        {
            get => current;
            set { current = value ?? "unnamed"; Register(current); }
        }

        internal static void Start()
        {
            if (started) return;
            started = true;
            Register(current);
            Application.logMessageReceived += OnLog;
        }

        static void Register(string step)
        {
            if (counts.ContainsKey(step)) return;
            order.Add(step); counts[step] = 0; lines[step] = new List<string>();
        }

        static void OnLog(string message, string stackTrace, LogType type)
        {
            if (type != LogType.Error && type != LogType.Exception && type != LogType.Assert) return;
            if (message != null && message.StartsWith("[Probe]")) return;
            counts[current]++;
            if (lines[current].Count < KeepLines)
            {
                string first = (message ?? "").Split('\n')[0].Trim();
                lines[current].Add(first.Length > 300 ? first.Substring(0, 300) + "..." : first);
            }
        }

        internal static int Count(string step) => step == null ? counts.Values.Sum() : (counts.TryGetValue(step, out int n) ? n : 0);

        internal static IEnumerable<string> Lines(string step) =>
            step == null ? order.SelectMany(s => lines[s]) : (lines.TryGetValue(step, out List<string> l) ? l : Enumerable.Empty<string>());

        internal static string Summary() => "errors=[" + string.Join(" ", order.Select(s => $"{s}:{counts[s]}")) + "]";

        internal static IEnumerable<string> Details() =>
            order.Where(s => counts[s] > 0).SelectMany(s => lines[s].Select(l => $"error in {s}: {l}"));
    }

    [StaticConstructorOnStartup]
    internal static class ProbeStartup
    {
        static ProbeStartup()
        {
            ErrorCapture.Start();   // later than most load errors; those stay in Player.log
            try
            {
                ProbeScenario.Log($"startup scenario={ProbeScenario.Instance.GetType().Name} order={ProbeScenario.LoadOrder()}");
                ProbeScenario.Instance.OnStartup();
            }
            catch (Exception e) { ProbeScenario.Log("EXCEPTION in OnStartup " + e); }
        }
    }

    public class ProbeMapComponent : MapComponent
    {
        const int StartFrames = 120;   // let the map finish its first frames before Setup
        static bool started;
        int frames, runFrames, startTick, lastObserved = -1, shotFrame = -1, finishFrame = -1;
        bool running;
        string lastState, shotPath, result;

        public ProbeMapComponent(Map map) : base(map) { }

        ProbeScenario S => ProbeScenario.Instance;
        int Ticks => Find.TickManager.TicksGame - startTick;

        public override void MapComponentUpdate()
        {
            if (Find.CurrentMap != map) return;
            if (!started)
            {
                if (++frames < StartFrames) return;
                started = true;
                try
                {
                    ErrorCapture.Current = "setup";
                    ProbeScenario.Log("setup");
                    S.Setup(map);
                    if (ErrorCapture.Current == "setup") ErrorCapture.Current = "run";
                    startTick = Find.TickManager.TicksGame;
                    running = true;
                    Find.TickManager.CurTimeSpeed = S.Speed;
                }
                catch (Exception e) { Fail("Setup", e); }
                return;
            }
            if (running)
            {
                if (Find.TickManager.CurTimeSpeed != S.Speed) Find.TickManager.CurTimeSpeed = S.Speed;   // autopause, letters
                try
                {
                    S.OnFrame(map, runFrames++);
                    if (S.endRequested || S.Done(map, Ticks) || runFrames >= S.MaxFrames) End();
                }
                catch (Exception e) { Fail("OnFrame", e); }
            }
            if (shotFrame >= 0 && Time.frameCount >= shotFrame)
            {
                shotFrame = -1;
                ScreenCapture.CaptureScreenshot(shotPath);
                finishFrame = Time.frameCount + 30;
            }
            if (finishFrame >= 0 && Time.frameCount >= finishFrame)
            {
                finishFrame = -1;
                foreach (string line in ErrorCapture.Details()) ProbeScenario.Log(line);
                ProbeScenario.Log($"RESULT {result} {ErrorCapture.Summary()} shot={shotPath}");
                Quit();
            }
        }

        public override void MapComponentTick()
        {
            if (!running) return;
            int t = Ticks;
            if (t - lastObserved < S.ObserveEvery && t < S.MaxTicks) return;
            lastObserved = t;
            try
            {
                string state = S.Observe(map, t);
                if (state != null && state != lastState) { ProbeScenario.Log($"t={t} {state}"); lastState = state; }
                if (S.endRequested || S.Done(map, t) || t >= S.MaxTicks) End();
            }
            catch (Exception e) { Fail("Observe", e); }
        }

        void End()
        {
            if (!running) return;
            running = false;
            int t = Ticks;
            Find.TickManager.CurTimeSpeed = TimeSpeed.Paused;
            try { result = S.Finish(map, t); }
            catch (Exception e) { Fail("Finish", e); return; }
            ProbeScenario.Log($"end t={t} frames={runFrames}");
            Find.WindowStack.TryRemove(typeof(LudeonTK.EditWindow_Log), false);   // dev mode opens it on errors; it would cover the screenshot
            if (S.ScreenshotTarget != null && S.ScreenshotTarget.Spawned)
            {
                Find.Selector.ClearSelection();
                Find.Selector.Select(S.ScreenshotTarget);
                Find.CameraDriver.SetRootPosAndSize(S.ScreenshotTarget.TrueCenter(), 9f);
            }
            shotPath = Path.Combine(GenFilePaths.SaveDataFolderPath, "probe.png");
            shotFrame = Time.frameCount + 90;   // let the camera and the inspect pane settle
        }

        void Fail(string phase, Exception e)
        {
            running = false;
            ProbeScenario.Log($"EXCEPTION in {phase} {e}");
            Quit();
        }

        static void Quit()
        {
            if (GenCommandLine.CommandLineArgPassed("probequit")) Application.Quit();
        }
    }
}
