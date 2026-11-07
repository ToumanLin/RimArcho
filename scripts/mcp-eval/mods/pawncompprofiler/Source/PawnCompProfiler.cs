// Pawn Comp Profiler: times every comp tick of every pawn and lists the slowest comp types.
// Dev mode: Debug actions > Pawn Comp Profiler. Settings: Options > Mod settings > Pawn Comp Profiler.
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Linq;
using System.Text;
using HarmonyLib;
using LudeonTK;
using UnityEngine;
using Verse;

namespace PawnCompProfiler
{
    public class ProfilerSettings : ModSettings
    {
        public bool enabled = true;

        public override void ExposeData()
        {
            Scribe_Values.Look(ref enabled, "enabled", true);
        }
    }

    public class ProfilerMod : Mod
    {
        public static ProfilerSettings Settings;

        public ProfilerMod(ModContentPack content) : base(content)
        {
            Settings = GetSettings<ProfilerSettings>();
            new Harmony("tallowbyte.pawncompprofiler").PatchAll();
        }

        public override string SettingsCategory() => "Pawn Comp Profiler";

        public override void DoSettingsWindowContents(Rect inRect)
        {
            var list = new Listing_Standard();
            list.Begin(inRect);
            list.CheckboxLabeled("Time pawn comp ticks", ref Settings.enabled, "Adds a few microseconds per pawn per tick.");
            if (list.ButtonText("Reset timings")) CompTimes.Reset();
            list.End();
        }
    }

    public static class CompTimes
    {
        class Entry { public long ticks; public long calls; }

        static readonly Dictionary<Type, Entry> times = new Dictionary<Type, Entry>();

        public static void Add(Type type, long elapsed)
        {
            if (!times.TryGetValue(type, out var e)) times[type] = e = new Entry();
            e.ticks += elapsed;
            e.calls++;
        }

        public static void Reset() => times.Clear();

        public static string Report(int top)
        {
            var sb = new StringBuilder("Pawn comp tick times (total ms, calls, µs per call):\n");
            foreach (var kv in times.OrderByDescending(kv => kv.Value.ticks).Take(top))
            {
                double ms = kv.Value.ticks * 1000.0 / Stopwatch.Frequency;
                sb.AppendLine($"  {kv.Key.FullName}: {ms:F1} ms, {kv.Value.calls} calls, {ms * 1000 / Math.Max(1, kv.Value.calls):F2} µs");
            }
            return sb.ToString();
        }

        [DebugAction("Pawn Comp Profiler", "Log slowest pawn comps", allowedGameStates = AllowedGameStates.Playing)]
        public static void LogSlowest() => Log.Message(Report(20));

        [DebugAction("Pawn Comp Profiler", "Reset timings", allowedGameStates = AllowedGameStates.Playing)]
        public static void ResetAction() => Reset();
    }

    // Pawns carry most of the comps that mods add. Replaces ThingWithComps.Tick for pawns with the same loop,
    // timed per comp; every other thing keeps the vanilla tick.
    [HarmonyPatch(typeof(ThingWithComps), "Tick")]
    public static class ThingWithComps_Tick_Patch
    {
        public static bool Prefix(ThingWithComps __instance)
        {
            if (!ProfilerMod.Settings.enabled || !(__instance is Pawn pawn)) return true;
            foreach (ThingComp comp in pawn.AllComps)
            {
                long start = Stopwatch.GetTimestamp();
                comp.CompTick();
                CompTimes.Add(comp.GetType(), Stopwatch.GetTimestamp() - start);
            }
            return false;
        }
    }
}
