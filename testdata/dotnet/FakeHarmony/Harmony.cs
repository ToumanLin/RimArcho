// Minimal stand-in for HarmonyLib: only the names and signatures RimArcho reads from metadata.
using System;
using System.Reflection;
namespace HarmonyLib {
  public enum MethodType { Normal, Getter, Setter, Constructor, StaticConstructor, Enumerator, Async }
  [AttributeUsage(AttributeTargets.Class | AttributeTargets.Method, AllowMultiple = true)]
  public class HarmonyPatch : Attribute {
    public HarmonyPatch() {}
    public HarmonyPatch(Type declaringType) {}
    public HarmonyPatch(Type declaringType, string methodName) {}
    public HarmonyPatch(Type declaringType, string methodName, params Type[] argumentTypes) {}
    public HarmonyPatch(Type declaringType, MethodType methodType) {}
    public HarmonyPatch(string methodName) {}
    public HarmonyPatch(MethodType methodType) {}
    public HarmonyPatch(string methodName, MethodType methodType) {}
  }
  public class HarmonyPrefix : Attribute {}
  public class HarmonyPostfix : Attribute {}
  public class HarmonyTranspiler : Attribute {}
  public class HarmonyMethod { public HarmonyMethod(MethodInfo m) {} }
  public class Harmony {
    public Harmony(string id) {}
    public void PatchAll() {}
    public MethodInfo Patch(MethodBase original, HarmonyMethod prefix = null, HarmonyMethod postfix = null, HarmonyMethod transpiler = null) { return null; }
  }
}
