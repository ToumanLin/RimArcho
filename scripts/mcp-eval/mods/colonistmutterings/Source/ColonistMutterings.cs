// Colonist Mutterings: colonists now and then say a short line to themselves, shown above their head and in their
// social log. No settings; safe to add or remove mid-save.
using System;
using System.Collections.Generic;
using RimWorld;
using Verse;

namespace ColonistMutterings
{
    public class MapComponent_Mutterings : MapComponent
    {
        // each colonist mutters about once an in-game hour (40 checks of 60 ticks)
        const int CheckTicks = 60, Checks = 40;

        public MapComponent_Mutterings(Map map) : base(map) { }

        public override void MapComponentTick()
        {
            int t = Find.TickManager.TicksGame;
            if (t % CheckTicks != 0) return;
            foreach (Pawn pawn in map.mapPawns.FreeColonistsSpawned)
                if ((t / CheckTicks + pawn.thingIDNumber) % Checks == 0 && pawn.Awake() && !pawn.Downed) Mutter(pawn);
        }

        static void Mutter(Pawn pawn)
        {
            try
            {
                string text = Lines.For(pawn);
                Find.PlayLog.Add(new PlayLogEntry_Mutter(pawn, text));
                MoteMaker.ThrowText(pawn.DrawPos, pawn.Map, text, 4f);
            }
            catch (Exception e)
            {
                Log.Error("[Colonist Mutterings] Error creating mutter: " + e);
            }
        }
    }

    // A one-pawn line in the social log; shown with the chitchat icon.
    public class PlayLogEntry_Mutter : PlayLogEntry_Interaction
    {
        string text;

        public PlayLogEntry_Mutter() { }

        public PlayLogEntry_Mutter(Pawn pawn, string text) : base(InteractionDefOf.Chitchat, pawn, null, null)
        {
            this.text = text;
        }

        protected override string ToGameStringFromPOV_Worker(Thing pov, bool forceLog) => $"{initiator.LabelShort}: \"{text}\"";

        public override void ExposeData()
        {
            base.ExposeData();
            Scribe_Values.Look(ref text, "text");
        }
    }

    static class Lines
    {
        static readonly string[] tired = { "I could sleep for a week.", "Just five more minutes...", "Is it bedtime yet?" };
        static readonly string[] hungry = { "Something smells good.", "When do we eat?", "My stomach is talking to me." };
        static readonly string[] low = { "What a day.", "Nothing goes right.", "Why do I even bother?" };
        static readonly string[] fine = { "Nice weather today.", "Back to work, I suppose.", "This place is growing on me.", "Hm." };

        public static string For(Pawn pawn)
        {
            List<string[]> sets = new List<string[]> { fine };
            if (pawn.needs?.rest != null && pawn.needs.rest.CurLevel < 0.3f) sets.Add(tired);
            if (pawn.needs?.food != null && pawn.needs.food.CurLevel < 0.3f) sets.Add(hungry);
            if (pawn.needs?.mood != null && pawn.needs.mood.CurLevel < 0.35f) sets.Add(low);
            return sets.RandomElement().RandomElement();
        }
    }
}
