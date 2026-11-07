package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func init() {
	for _, key := range []string{"the managed folder overlaps %s", "the install folder overlaps %s", "%s already contains %s; choose an empty install folder", "release is missing %s", "release file changed while copying: %s", "release entry is not a plain file or directory: %s"} {
		interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
	}
}

// ManagerImportText is the preview and result of a player-data migration.
func ManagerImportText(r *app.RimCrowReport) string {
	lines := []string{
		Tf("aliases: %d new, %d already present", r.AliasNew, r.AliasSame),
		Tf("notes: %d new, %d already present", r.NoteNew, r.NoteSame),
		Tf("tags: %d new, %d already present", r.TagNew, r.TagSame),
		Tf("rules: %d new, %d already present, %d skipped", r.RuleNew, r.RuleSame, len(r.RulesSkipped)),
	}

	if r.DB == "" {
		lines = lines[3:]
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
		lines = append(lines, fmt.Sprintf("%s: %d (%s)", k, r.NotMapped[k], T("no RimArcho equivalent")))
	}
	if r.RulesDisabled {
		lines = append(lines, T("disabled RimCrow rules are skipped"))
	}
	if r.CrowDisabled > 0 || len(r.AboutDisabled) > 0 {
		lines = append(lines, Tf("%d folders have About.xml.disabled; enable them in RimCrow, then scan again (%d disabled in RimCrow)", len(r.AboutDisabled), r.CrowDisabled))
	}
	return strings.Join(lines, "\n")
}
