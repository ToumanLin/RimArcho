// The thrower in the null-trace fixture: a postfix on the game constructor that dereferences
// its recipient (MCP eval h4, Cyanobot's Genes).
using HarmonyLib; using Verse;
namespace TraceB {
  [HarmonyPatch(typeof(PlayLogEntry_Interaction), MethodType.Constructor)]
  public static class PlayLogEntry_Interaction_Patch {
    public static void Postfix(ref InteractionDef ___intDef, InteractionDef intDef, Pawn initiator, Pawn recipient) {
      if (intDef == null) return;
      if (recipient.genes == null) ___intDef = intDef;
    }
  }
  public static class Helper {
    // A local, a field of a parameter: other dereference shapes.
    public static int Len(Pawn p) { string n = p.Name; return n.Length; }
  }
}
