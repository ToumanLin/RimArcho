// Two builds of one Mod assembly for the assembly diff (V2: -p:DefineConstants=V2 -p:Version=1.1.0): one method
// changed, one added, a reference to an internal game member, an attribute, a new version.
using Verse;
namespace TraceDiff {
  public static class Stable { public static int Same(int x) { return x + 1; } }
  public static class Changed {
#if V2
    public static int Value(Pawn p) { return p.Name.Length + Secret.Hidden(); }
    [System.Obsolete("use Value")] public static int Added() { return 2; }
#else
    public static int Value(Pawn p) { return p.Name.Length; }
#endif
  }
}
