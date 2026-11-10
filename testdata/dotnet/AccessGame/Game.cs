// A stand-in game for the member access check (05 §5.6). Built as is it is the game; built with
// -p:DefineConstants=PUB it is the publicized copy a Mod compiles against (every member public).
namespace Verse {
  public class StatWorker {
#if PUB
    public int stat;
    public static int Secret() { return 1; }
#else
    protected int stat;
    internal static int Secret() { return 1; }
#endif
    public int Value() { return stat; }
  }
}
