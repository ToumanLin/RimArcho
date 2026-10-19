using HarmonyLib; using Verse; using System.Collections.Generic;
namespace ModC {
  [HarmonyPatch(typeof(Pawn), "Tick")]
  public static class Pawn_Tick_C {
    [HarmonyTranspiler] public static IEnumerable<object> Rewrite(IEnumerable<object> instructions) { return instructions; }
  }
}
