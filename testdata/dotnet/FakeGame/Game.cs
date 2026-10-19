namespace Verse {
  public class Thing { public virtual void Tick() {} public int HitPoints { get; set; } }
  public class ThingWithComps : Thing { public override void Tick() {} }
  public class Pawn : ThingWithComps { public override void Tick() {} public void Kill(int how) {} }
}
