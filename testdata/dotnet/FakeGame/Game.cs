using System.Collections.Generic;
namespace Verse {
  public class Thing { public virtual void Tick() {} public int HitPoints { get; set; } }
  public class ThingWithComps : Thing {
    private List<ThingComp> comps = new List<ThingComp>();
    public List<ThingComp> AllComps { get { return comps; } }
    public override void Tick() {}
  }
  public class Pawn : ThingWithComps { public Pawn() {} public Pawn(int id) {} public override void Tick() {} public void Kill(int how) {} }
  public class ThingComp { public ThingWithComps parent; public virtual void CompTick() {} public virtual void CompTickRare() {} }
  public class CompProperties { public System.Type compClass; }
  public static class GenList { public static void Swap<T>(this IList<T> list, int a, int b) {} }
}
