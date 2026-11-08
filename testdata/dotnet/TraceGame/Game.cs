// A stand-in game assembly for the null trace, call-site table, lord and diff tests
// (internal/dotnet, internal/app). Names follow RimWorld's; bodies are minimal.
using System.Collections.Generic;
using System.Runtime.CompilerServices;
[assembly: InternalsVisibleTo("TraceDiff")]
namespace Verse {
  public class Pawn { public object genes; public string Name; }
  public class InteractionDef {}
  public class RulePackDef {}
  public abstract class LogEntry {}
  public class PlayLogEntry_Interaction : LogEntry {
    protected InteractionDef intDef; protected Pawn initiator; protected Pawn recipient;
    public PlayLogEntry_Interaction() {}
    public PlayLogEntry_Interaction(InteractionDef intDef, Pawn initiator, Pawn recipient, List<RulePackDef> extraSentencePacks) {
      this.intDef = intDef; this.initiator = initiator; this.recipient = recipient;
    }
  }
  public class PlayLog { public void Add(LogEntry e) {} }
  public static class Find { public static PlayLog PlayLog = new PlayLog(); }
  public class InteractionWorker {
    public static InteractionDef Chitchat = new InteractionDef();
    Pawn other;
    public void Interacted(Pawn initiator, Pawn recipient, List<RulePackDef> extra) { Find.PlayLog.Add(new PlayLogEntry_Interaction(Chitchat, initiator, recipient, extra)); }
    public void Social(Pawn a) { Find.PlayLog.Add(new PlayLogEntry_Interaction(Chitchat, a, other, null)); }
    public void Third(Pawn a, Pawn b) { var e = new PlayLogEntry_Interaction(Chitchat, b, a, new List<RulePackDef>()); Find.PlayLog.Add(e); }
  }
  internal static class Secret { internal static int Hidden() { return 1; } }
}
namespace Verse.AI.Group {
  public class Lord { public void AddPawn(Verse.Pawn p) {} }
  public static class LordUtility { public static Lord GetLord(this Verse.Pawn p) { return null; } }
}
