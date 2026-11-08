using HarmonyLib; using Verse; using System; using System.Reflection;
namespace CompThrower {
  // A profiler's prefix that replaces ThingWithComps.Tick with a foreach over AllComps (MCP
  // eval h7): it throws "Collection was modified" when a comp changes the list while ticking.
  [HarmonyPatch(typeof(ThingWithComps), "Tick")]
  public static class Tick_Patch {
    public static bool Prefix(ThingWithComps __instance) {
      foreach (ThingComp c in __instance.AllComps) c.CompTick();
      return false;
    }
  }
  // Targets chosen in TargetMethod(): simple bodies resolve statically (internal/harmony).
  [HarmonyPatch] public static class Dyn_Ctor {
    static MethodBase TargetMethod() { return AccessTools.Constructor(typeof(Pawn), new[] { typeof(int) }); }
    public static void Postfix() {}
  }
  [HarmonyPatch] public static class Dyn_Method {
    static MethodBase TargetMethod() { return AccessTools.Method(typeof(Pawn), "Kill"); }
    public static void Prefix() {}
  }
  [HarmonyPatch] public static class Dyn_Getter {
    static MethodBase TargetMethod() { return AccessTools.PropertyGetter(typeof(Thing), "HitPoints"); }
    public static void Postfix() {}
  }
  // A name computed at run time stays dynamic.
  [HarmonyPatch] public static class Dyn_Computed {
    static MethodBase TargetMethod() { return AccessTools.Method(typeof(Pawn), Environment.MachineName); }
    public static void Prefix() {}
  }
}
