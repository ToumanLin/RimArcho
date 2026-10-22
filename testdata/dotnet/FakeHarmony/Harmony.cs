// Minimal stand-in for HarmonyLib: only the names and signatures RimArcho reads from metadata.
using System;
using System.Linq.Expressions;
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
  public class HarmonyMethod {
    public HarmonyMethod(MethodInfo m) {}
    public HarmonyMethod(Type methodType, string methodName, Type[] argumentTypes = null) {}
    public static implicit operator HarmonyMethod(MethodInfo m) { return null; }
  }
  public class Harmony {
    public Harmony(string id) {}
    public void PatchAll() {}
    public MethodInfo Patch(MethodBase original, HarmonyMethod prefix = null, HarmonyMethod postfix = null, HarmonyMethod transpiler = null, HarmonyMethod finalizer = null) { return null; }
  }
  public static class AccessTools {
    public static MethodInfo Method(Type type, string name, Type[] parameters = null, Type[] generics = null) { return null; }
    public static MethodInfo Method(string typeColonName, Type[] parameters = null, Type[] generics = null) { return null; }
    public static MethodInfo PropertyGetter(Type type, string name) { return null; }
    public static MethodInfo PropertySetter(Type type, string name) { return null; }
    public static ConstructorInfo Constructor(Type type, Type[] parameters = null, bool searchForStatic = false) { return null; }
    public static Type TypeByName(string name) { return null; }
  }
  public static class SymbolExtensions {
    public static MethodInfo GetMethodInfo(Expression<Action> expression) { return null; }
  }
}
