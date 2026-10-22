using HarmonyLib; using Verse;
namespace ModWU {
  // Calls a Harmony.Patch wrapper of another assembly (ModManualIL's Wrapper.Patch).
  public static class P {
    public static void Init() { ModIL.Wrapper.Patch(AccessTools.Method(typeof(Thing), "Tick"), new HarmonyMethod(typeof(P), "Pre")); }
    public static bool Pre() { return true; }
  }
}
