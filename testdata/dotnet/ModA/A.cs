using HarmonyLib; using Verse; using System.Collections.Generic;
namespace ModA {
  public static class Boot { public static void Init() { new Harmony("tester.moda.harmony").PatchAll(); } }
  // A bool prefix on Pawn.Tick: can skip the original.
  [HarmonyPatch(typeof(Pawn), "Tick")]
  public static class Pawn_Tick { public static bool Prefix(Pawn __instance) { return true; } }
  // Class-level type, method-level name; getter via MethodType.
  [HarmonyPatch(typeof(Thing))]
  public static class Thing_HitPoints {
    [HarmonyPatch("HitPoints", MethodType.Getter)] [HarmonyPostfix] public static void After(ref int __result) {}
  }
  // A type whose method throws, for stack-frame attribution.
  public class CompBroken { public void CompTick() { throw new System.ArgumentOutOfRangeException(); } }
}
