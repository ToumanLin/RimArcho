// Authoring tool for the h8 eval case (never shipped in a case). On a -quicktest map it builds a Progression: Education
// classroom and a Socio Butterfly birthday party that holds the students, saves "autostart", and quits.
using System;
using System.Collections.Generic;
using System.Linq;
using ProgressionEducation;
using RecreationalSexWithEuterpe;
using RimWorld;
using UnityEngine;
using Verse;
using Verse.AI.Group;

namespace PeEvalSetup
{
    public class PeSetupComponent : GameComponent
    {
        int ticks;
        int stage;
        public PeSetupComponent(Game game) { }

        public override void GameComponentTick()
        {
            Map map = Find.CurrentMap;
            if (map == null || stage >= 3) return;
            ticks++;
            try
            {
                if (stage == 0 && ticks >= 300) { Build(map); stage = 1; ticks = 0; }
                else if (stage == 1 && ticks >= 120) { Party(map); stage = 2; ticks = 0; }
                else if (stage == 2 && ticks >= 60)
                {
                    GameDataSaveLoader.SaveGame("autostart");
                    Log.Message("[PeSetup] saved autostart");
                    stage = 3;
                    Application.Quit();
                }
            }
            catch (Exception e) { Log.Error("[PeSetup] " + e); stage = 3; }
        }

        static bool Clear(Map map, IntVec3 c) => c.InBounds(map) && c.Standable(map) && c.GetEdifice(map) == null && c.GetTerrain(map).affordances.Contains(TerrainAffordanceDefOf.Heavy);

        // a clear w x h rectangle near the colonists
        static IntVec3 FindRect(Map map, IntVec3 near, int w, int h)
        {
            for (int r = 0; r < 40; r++)
                for (int dx = -r; dx <= r; dx++)
                    for (int dz = -r; dz <= r; dz++)
                    {
                        if (Math.Max(Math.Abs(dx), Math.Abs(dz)) != r) continue;
                        IntVec3 o = near + new IntVec3(dx, 0, dz); bool ok = true;
                        for (int x = 0; x < w && ok; x++) for (int z = 0; z < h && ok; z++) ok = Clear(map, o + new IntVec3(x, 0, z));
                        if (ok) return o;
                    }
            throw new Exception("no clear area");
        }

        static Thing Place(Map map, string def, IntVec3 cell, Rot4 rot)
        {
            ThingDef d = DefDatabase<ThingDef>.GetNamed(def);
            Thing t = ThingMaker.MakeThing(d, d.MadeFromStuff ? GenStuff.DefaultStuffFor(d) : null);
            t.SetFaction(Faction.OfPlayer);
            return GenSpawn.Spawn(t, cell, map, rot);
        }

        void Build(Map map)
        {
            var pawns = map.mapPawns.FreeColonistsSpawned.ToList();
            if (pawns.Count < 3) throw new Exception("need 3 colonists, have " + pawns.Count);
            Pawn teacher = pawns[0]; var students = pawns.Skip(1).Take(2).ToList();
            IntVec3 o = FindRect(map, teacher.Position, 8, 6);
            // board against the north edge facing south, two desks below it, a bell beside it
            Thing board = Place(map, "PE_LessonStone", o + new IntVec3(1, 0, 5), Rot4.South);
            Place(map, "PE_StoneSchoolDesk", o + new IntVec3(1, 0, 3), Rot4.South);
            Place(map, "PE_StoneSchoolDesk", o + new IntVec3(5, 0, 3), Rot4.South);
            Place(map, "PE_SchoolBell", o + new IntVec3(6, 0, 5), Rot4.South);
            Find.WindowStack.TryRemove(typeof(Dialog_RenameClassroom), false);   // the board opens a rename dialog, which pauses the game
            var comp = board.TryGetComp<CompLearningBoard>();
            Classroom room = comp.classroom;
            if (room == null) throw new Exception("board made no classroom");
            EducationManager.Instance.AddClassroom(room);

            SkillDef skill = SkillDefOf.Social;   // taught at the plain school desks; crafting needs workbenches
            teacher.skills.GetSkill(skill).Level = 14; teacher.skills.GetSkill(SkillDefOf.Social).Level = 10; teacher.skills.GetSkill(SkillDefOf.Intellectual).Level = 10;
            foreach (Pawn s in students) s.skills.GetSkill(skill).Level = 2;
            int hour = GenLocalDate.HourOfDay(map);
            var sg = new StudyGroup(teacher, students, "Social skills for beginners", 20000, hour, (hour + 6) % 24);
            sg.classroom = room;
            sg.subjectLogic = new SkillClassLogic(sg) { SkillFocus = skill };
            EducationManager.Instance.AddStudyGroup(sg);
            TimeAssignmentUtility.GenerateTimeAssignmentDef(sg);
            EducationManager.ApplyScheduleToPawns(sg);
            var rep = sg.ValidateClassStatus();
            Log.Message("[PeSetup] class built at " + o + ", hour " + hour + ", valid: " + rep.Accepted + " " + rep.Reason);
        }

        void Party(Map map)
        {
            var sg = EducationManager.Instance.StudyGroups.First();
            Thing spot = ThingMaker.MakeThing(ThingDefOf.PartySpot);
            spot.SetFaction(Faction.OfPlayer);
            IntVec3 c = sg.students[0].Position;
            GenSpawn.Spawn(spot, CellFinder.StandableCellNear(c, map, 6), map);
            var job = new LordJob_Birthday(spot, sg.students[0]);
            LordMaker.MakeNewLord(Faction.OfPlayer, job, map, sg.students);
            Log.Message("[PeSetup] birthday party started for " + sg.students.Count + " students");
        }
    }
}
