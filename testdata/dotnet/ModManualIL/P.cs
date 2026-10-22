using HarmonyLib; using Verse; using System; using System.Linq; using System.Reflection; using System.Collections.Generic;
namespace ModIL {
  public static class Patches {
    public static bool Pre() { return true; }
    public static void Post() {}
    public static IEnumerable<object> Trans(IEnumerable<object> i) { return i; }
    public static Exception Fin(Exception e) { return e; }
  }
  // One Harmony.Patch call per pattern the IL scan resolves (internal/harmony/manual.go).
  public static class Boot {
    static readonly Type patchType = typeof(Patches);
    public static void Init(Harmony h) {
      // AccessTools.Method(typeof(T), "Name"); HarmonyMethod(Type, string) through a static field
      h.Patch(AccessTools.Method(typeof(Pawn), "Kill"), prefix: new HarmonyMethod(patchType, nameof(Patches.Pre)));
      // "Type:Name" form through a local; HarmonyMethod(typeof(T).GetMethod("Name"))
      var m = AccessTools.Method("Verse.Thing:Tick");
      h.Patch(m, postfix: new HarmonyMethod(typeof(Patches).GetMethod("Post")));
      // property getter and setter; patch method from AccessTools.Method
      h.Patch(AccessTools.PropertyGetter(typeof(Thing), nameof(Thing.HitPoints)), postfix: new HarmonyMethod(typeof(Patches), "Post"));
      h.Patch(AccessTools.PropertySetter(typeof(Thing), "HitPoints"), transpiler: new HarmonyMethod(AccessTools.Method(typeof(Patches), "Trans")));
      // constructor
      h.Patch(AccessTools.Constructor(typeof(Pawn)), finalizer: new HarmonyMethod(typeof(Patches), "Fin"));
      // Type.GetMethod with an argument list
      h.Patch(typeof(Pawn).GetMethod("Kill", new[] { typeof(int) }), prefix: new HarmonyMethod(typeof(Patches), "Pre"));
      // Type.GetProperty(..).GetGetMethod(); patch method from a delegate
      h.Patch(typeof(ThingWithComps).GetProperty("HitPoints").GetGetMethod(), postfix: new HarmonyMethod(((Action)Patches.Post).Method));
      // ldtoken + GetMethodFromHandle (expression tree); implicit MethodInfo -> HarmonyMethod
      h.Patch(SymbolExtensions.GetMethodInfo(() => Patches.Post()), postfix: typeof(Patches).GetMethod("Post"));
      // a helper method of the mod
      Helper(h, typeof(Pawn), "Tick", nameof(Patches.Post));
      // foreach over an array and over a list
      foreach (var t in new[] { typeof(Thing), typeof(ThingWithComps) }) h.Patch(AccessTools.Method(t, "Tick"), prefix: new HarmonyMethod(typeof(Patches), "Pre"));
      var list = new List<MethodBase> { AccessTools.Method(typeof(Pawn), "Kill"), AccessTools.Method(typeof(Thing), "Tick") };
      foreach (var x in list) h.Patch(x, finalizer: new HarmonyMethod(typeof(Patches), "Fin"));
      // AccessTools.TypeByName
      h.Patch(AccessTools.Method(AccessTools.TypeByName("Verse.Pawn"), "Kill"), postfix: new HarmonyMethod(typeof(Patches), "Post"));
      // LINQ search with a name lambda
      h.Patch(typeof(Pawn).GetMethods().Where(x => x.Name.Contains("Kill")).First(), postfix: new HarmonyMethod(typeof(Patches), "Fin"));
      // not resolvable: a computed name
      h.Patch(AccessTools.Method(typeof(Pawn), "Ki" + Environment.TickCount), postfix: new HarmonyMethod(typeof(Patches), "Fin"));
    }
    static void Helper(Harmony h, Type t, string name, string patch) {
      h.Patch(AccessTools.Method(t, name), postfix: new HarmonyMethod(typeof(Patches), patch));
    }
    // not resolvable: a helper nobody in the assembly calls
    public static void Unused(Harmony h, MethodBase original) { h.Patch(original, transpiler: new HarmonyMethod(typeof(Patches), "Trans")); }
  }
  // A wrapper other assemblies call (ModWrapperUser): Patch(MethodBase, HarmonyMethod, HarmonyMethod).
  public static class Wrapper {
    public static Harmony H;
    public static void Patch(MethodBase original, HarmonyMethod prefix = null, HarmonyMethod postfix = null) { H.Patch(original, prefix, postfix); }
  }
  // Performance Optimizer style: a virtual DoPatches, patch methods found with GetType(), ternaries.
  public abstract class Opt {
    public virtual void DoPatches() {}
    protected MethodInfo M(string name) { return AccessTools.Method(GetType(), name); }
    protected void P(Type t, string name, MethodInfo prefix = null, MethodInfo postfix = null) {
      var hp = prefix != null ? new HarmonyMethod(prefix) : null;
      var hq = postfix != null ? new HarmonyMethod(postfix) : null;
      Wrapper.H.Patch(AccessTools.Method(t, name), hp, hq);
    }
  }
  public class OptA : Opt {
    public override void DoPatches() { P(typeof(Pawn), "Tick", M("Pre")); }
    public static bool Pre() { return true; }
  }
}
