// The caller in the null-trace fixture: passes null for the recipient through a base
// constructor call (MCP eval h4), and through a helper's parameter; lord AddPawn sites (h8).
using Verse; using Verse.AI.Group;
namespace TraceA {
  public class PlayLogEntry_Mutter : PlayLogEntry_Interaction {
    string text;
    public PlayLogEntry_Mutter(Pawn pawn, string text) : base(InteractionWorker.Chitchat, pawn, null, null) { this.text = text; }
  }
  public static class Mutterings {
    public static void Mutter(Pawn pawn) {
      try { Find.PlayLog.Add(new PlayLogEntry_Mutter(pawn, "hm")); } catch (System.Exception) { }
    }
    public static void Relay(Pawn a, Pawn r) { Find.PlayLog.Add(new PlayLogEntry_Interaction(InteractionWorker.Chitchat, a, r, null)); }
    public static void CallRelay(Pawn a) { Relay(a, null); }
  }
  public class LordToil_AttendClass {
    Lord lord;
    public void UpdateAllDuties(Pawn p) { lord.AddPawn(p); }
    public void Join(Pawn p) { if (p.GetLord() == null) lord.AddPawn(p); }
  }
}
