// A Mod built against the publicized stand-in game (05 §5.6): a subclass reads the protected
// field (allowed); without SUBONLY a patch class reads it too and calls an internal method,
// which the game refuses unless the assembly skips verification (-p:AllowUnsafeBlocks=true).
using Verse;
namespace AccessMod {
  public class MyWorker : StatWorker { public int Get() { return stat; } }
#if !SUBONLY
  public static class Patch { public static int Postfix(StatWorker w) { return w.stat + StatWorker.Secret(); } }
#endif
}
