using HarmonyLib; using Verse; using System.Reflection;
namespace ModManual {
  // Patches only from code: no HarmonyPatch attributes at all (C6).
  public static class Boot {
    public static void Init() {
      var h = new Harmony("tester.manual");
      h.Patch(typeof(ThingWithComps).GetMethod("Tick"), postfix: new HarmonyMethod(typeof(Boot).GetMethod(nameof(After))));
    }
    public static void After() {}
  }
}
