package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func init() {
	for _, key := range []string{"the managed folder overlaps %s", "the install folder overlaps %s", "%s already contains %s; choose an empty install folder", "release is missing %s", "release file changed while copying: %s", "release entry is not a plain file or directory: %s", "missing or unreadable Config/ModsConfig.xml: %s", "unreadable Config/ModsConfig.xml: %s"} {
		interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
	}
}

// ManagerTags names the tags a migration makes for another manager's colour marks and
// ignored warnings (07 §6.4), in the player's language.
func ManagerTags() app.ManagerTagNames {
	return app.ManagerTagNamesIn(Lang())
}

// ManagerImportText is the preview and result of a player-data migration.
func ManagerImportText(r *app.RimCrowReport) string { return managerImportText(r, true) }

// ManagerImportPreviewText leaves profiles to the GUI checklist.
func ManagerImportPreviewText(r *app.RimCrowReport) string { return managerImportText(r, false) }

func managerImportText(r *app.RimCrowReport, profiles bool) string {
	lines := []string{
		Tf("aliases: %d new, %d already present", r.AliasNew, r.AliasSame),
		Tf("notes: %d new, %d already present", r.NoteNew, r.NoteSame),
		Tf("tags: %d new, %d already present", r.TagNew, r.TagSame),
		Tf("tag colours: %d", r.TagColors),
		Tf("rules: %d new, %d already present, %d skipped", r.RuleNew, r.RuleSame, len(r.RulesSkipped)),
	}

	if r.NotesAndTags {
		lines = lines[1:4]
	}
	if r.DB == "" {
		lines = lines[4:]
	} else {
		lines = append([]string{Tf("mods with data: %d, matched: %d, not matched: %d", r.Records, r.ByPath+r.ByPackage, len(r.Unmapped))}, lines...)
	}

	for _, group := range []struct {
		name  string
		items []app.RCItem
	}{{"conflicts (existing values kept)", r.Conflicts}, {"not mapped", r.Unmapped}, {"skipped rules", r.RulesSkipped}} {
		if len(group.items) == 0 {
			continue
		}
		lines = append(lines, T(group.name))
		for _, item := range group.items {

			reason := item.Reason
			if strings.Contains(reason, "lineages") {
				reason = "more than one installed Mod matches this packageId"
			}
			lines = append(lines, item.Key+": "+Text(reason))
		}
	}
	keys := make([]string, 0, len(r.NotMapped))
	for k := range r.NotMapped {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "profile source flags (use_workshop_mods/use_self_mods)" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %d (%s)", T(k), r.NotMapped[k], T("no RimArcho equivalent")))
	}
	if r.RulesDisabled {
		lines = append(lines, T("disabled RimCrow rules are skipped"))
	}
	if r.CrowDisabled > 0 || len(r.AboutDisabled) > 0 {
		lines = append(lines, Tf("%d folders have About.xml.disabled; enable them in RimCrow, then scan again (%d disabled in RimCrow)", len(r.AboutDisabled), r.CrowDisabled))
	}
	if profiles {
		for _, p := range r.Profiles {
			lines = append(lines, fmt.Sprintf("%s  %s  %s  %s", p.Name, p.Game, p.UserData, ProfileStatus(p.Status)))
		}
	}
	if len(r.Profiles) > 0 {
		lines = append(lines, T("folders stay shared with RimCrow; launching writes their Config and saves; deleting the profile in RimCrow deletes its saves"), T("profile source flags are not imported; list entries decide what loads; check unresolved Mods and which copies are selected"))
	}
	return strings.Join(lines, "\n")
}

// ProfileStatus translates the stable status and leaves diagnostic paths intact.
func ProfileStatus(status string) string {
	if strings.HasPrefix(status, "skipped: ") {
		return Tf("skipped: %s", Text(strings.TrimPrefix(status, "skipped: ")))
	}
	if strings.HasPrefix(status, "incomplete: ") {
		return Tf("incomplete: %s", Text(strings.TrimPrefix(status, "incomplete: ")))
	}
	return Text(status)
}
