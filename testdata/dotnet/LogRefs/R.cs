using System;
namespace LogRefs {
  // Log attribution fixtures (internal/app logrules_test.go): names asked for by code, a type
  // that holds save references, and a reflection-only resolver.
  public static class Lookups {
    public static string Icon() { return "UI/LogRefs/Icon"; }
    public static string Def() { return "LogRefs_Missing"; }
  }
  public class Holder { public object target; }
  public static class Resolver {
    public static void Hook() { AppDomain.CurrentDomain.ReflectionOnlyAssemblyResolve += (s, e) => null; }
  }
}
