// The scenario: the only file to edit. This default is a smoke test that proves the toolchain works.
using System.Linq;
using RimWorld;
using Verse;

namespace RimProbe
{
    public class Scenario : ProbeScenario
    {
        Thing marker;

        public override void OnStartup()
        {
            // Example: record who has patched a method at this point of startup.
            // Log("startup " + Patches(HarmonyLib.AccessTools.Method(typeof(Pawn_Ownership), "ClaimBedIfNonMedical")));
        }

        public override void Setup(Map map)
        {
            Step("spawn-wall");   // errors from here on count against this step
            marker = SpawnBuilding(map, "Wall", "WoodLog");
            ScreenshotTarget = marker;
            Log($"setup colonists={map.mapPawns.FreeColonistsSpawnedCount} map={map.Size} wall at {marker.Position}");
            Step("watch");
        }

        // For UI checks, also override OnFrame (called every frame, even paused) and Speed => TimeSpeed.Paused.

        public override int MaxTicks => 2500;
        public override int ObserveEvery => 250;

        public override string Observe(Map map, int tick)
        {
            // One line per observation; logged only when it changes.
            return string.Join(" ", map.mapPawns.FreeColonistsSpawned.Select(p => $"{p.LabelShort}:{p.CurJobDef?.defName ?? "none"}"));
        }

        public override string Finish(Map map, int tick)
        {
            return $"smoke ok colonists={map.mapPawns.FreeColonistsSpawnedCount} ticks={tick} wallSpawned={marker.Spawned}";
        }
    }
}
