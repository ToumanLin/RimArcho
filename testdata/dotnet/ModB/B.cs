using HarmonyLib; using Verse; using System.Collections.Generic;
namespace ModB {
  public static class Boot { public static void Init() { new Harmony("tester.modb").PatchAll(); } }
  [HarmonyPatch(typeof(Pawn), nameof(Pawn.Tick))]
  public static class Pawn_Tick_B {
    public static void Postfix() {}
    public static IEnumerable<object> Transpiler(IEnumerable<object> instructions) { return instructions; }
  }
  [HarmonyPatch(typeof(Pawn), "Kill", new System.Type[] { typeof(int) })]
  public static class Pawn_Kill { public static void Prefix() {} }
}
