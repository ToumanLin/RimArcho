package testkit

import "fmt"

// ModNames returns n Mod names shaped like a large library's: a few well-known names, then
// generated ones ("Simple Storage Retexture"). Long names hold most letters, so short typed
// queries such as "sotr" are in order in many of them, as in a real library (16 §3.5).
func ModNames(n int) []string {
	out := []string{
		"HugsLib", "Log Publisher from HugsLib", "Harmony", "Add Cube", "Echoes of the Rim",
		"Weapons Dominator", "Progression: Attire", "[sbz] EPOE Retexture", "Multi Analyzer Retexture",
		"Mod Options Sort", "Tab-sorting", "Vanilla Hair Expanded", "Vanilla Armour Expanded",
		"Vanilla Factions Expanded - Mechanoids", "Vanilla Expanded Framework", "Subscription Tracker Redux",
	}
	first := []string{"Simple", "Better", "Dubs", "Alpha", "Medieval", "Realistic", "Common", "Tactical",
		"Ancient", "Mechanoid", "Rimefeller", "Industrial", "Spacer", "Tribal", "Outer", "Deep", "Royal",
		"Biotech", "Odyssey", "Vanilla"}
	second := []string{"Storage", "Sidearms", "Factions", "Weapons", "Animals", "Apparel", "Armory",
		"Psycasts", "Turrets", "Textures", "Hair", "Traits", "Outposts", "Quests", "Robots", "Doors",
		"Furniture", "Shields", "Research", "Genes"}
	third := []string{"Expanded", "Continued", "Framework", "Patch", "Retexture", "Redux", "Reborn", "Core",
		"Plus", "Overhaul", "Extended", "Collection", "Rework", "for Combat Extended", "Addon"}
	fourth := []string{"", " (Continued)", " - Retextured", "", " for Royalty", " Tweaks", " Patch for Rimworld", ""}
	for i := 0; len(out) < n; i++ {
		name := first[i%len(first)] + " " + second[(i/len(first))%len(second)] + " " + third[(i*7)%len(third)] + fourth[(i*3)%len(fourth)]
		if k := i / (len(first) * len(second)); k > 0 {
			name += fmt.Sprintf(" %d", k+1)
		}
		out = append(out, name)
	}
	return out[:n]
}
