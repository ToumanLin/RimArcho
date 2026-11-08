using Verse; using System.Collections.Generic;
namespace CompRemover {
  // Removes itself from its pawn's comps while ticking, after swapping to the end so that
  // the game's cached-count loop survives (SSR Body's June pattern, MCP eval h7).
  public class CompSelfRemover : ThingComp {
    public override void CompTick() {
      List<ThingComp> all = parent.AllComps;
      all.Swap(all.IndexOf(this), all.Count - 1);
      parent.AllComps.Remove(this);
    }
  }
  // Names its comp class in code: XML lists only the properties class.
  public class CompProperties_Remover : CompProperties { public CompProperties_Remover() { compClass = typeof(CompSelfRemover); } }
  // Adds a comp from a UI action: not a tick method.
  public static class Gizmo { public static void Attach(ThingWithComps t, ThingComp c) { t.AllComps.Add(c); } }
  // Reads the list only.
  public static class Reader { public static int Count(ThingWithComps t) { return t.AllComps.Count; } }
}
