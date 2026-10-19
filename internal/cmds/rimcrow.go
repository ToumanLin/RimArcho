package cmds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/render"
)

func init() {
	register(&Command{Name: "import rimcrow", Summary: "one-time import of RimCrow's aliases, notes, groups (as tags) and user rules; repeatable", MaxArgs: 0, Run: importRimCrowCmd,
		Flags: []Flag{{Name: "db", Value: "<path>", Help: `RimCrow's data\mod_manager.db (read from a copy)`}, {Name: "rules", Value: "<path>", Help: `default: data\rules\user_rules.json next to the database`},
			{Name: "dry-run", Help: "report what would be imported; write nothing"}, {Name: "include-disabled-rules", Help: "also import user rules that are switched off in RimCrow"}, {Name: "all"}}})
}

func importRimCrowCmd(c *Ctx, a *Args) (*render.Doc, error) {
	db := a.Str("db")
	if db == "" {
		return nil, &UsageError{Msg: `--db <path>: RimCrow's database, <RimCrow folder>\data\mod_manager.db`}
	}
	rep, err := c.App.ImportRimCrow(app.RimCrowOptions{DB: db, Rules: a.Str("rules"), DryRun: a.Bool("dry-run"), IncludeDisabledRules: a.Bool("include-disabled-rules")})
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	head := "import rimcrow"
	if rep.DryRun {
		head += " (dry run: nothing written)"
	}
	d, root := render.New(head)
	root.KV("database", rep.DB+" (read from a copy)")
	mapped := rep.ByPath + rep.ByPackage
	root.KV("mods", fmt.Sprintf("%d with data: %d mapped (%d by path, %d by packageId), %d not mapped", rep.Records, mapped, rep.ByPath, rep.ByPackage, len(rep.Unmapped)))
	root.KV("aliases", fmt.Sprintf("%d new, %d already set", rep.AliasNew, rep.AliasSame))
	root.KV("notes", fmt.Sprintf("%d new, %d already set", rep.NoteNew, rep.NoteSame))
	root.KV("tags", fmt.Sprintf("%d new, %d already present (%d groups, %d memberships)", rep.TagNew, rep.TagSame, rep.Groups, rep.Memberships))
	rules := fmt.Sprintf("%d: %d new, %d already present, %d skipped", rep.RulesTotal, rep.RuleNew, rep.RuleSame, len(rep.RulesSkipped))
	if rep.RulesDisabled {
		rules += " (user rules are switched off in RimCrow)"
	}
	if rep.Rules == "" {
		rules += " (no user_rules.json found)"
	}
	root.KV("rules", rules)
	root.KV("conflicts", fmt.Sprintf("%d (RimArcho's value kept)", len(rep.Conflicts)))
	if len(rep.NotMapped) > 0 {
		var ks []string
		for k, n := range rep.NotMapped {
			ks = append(ks, fmt.Sprintf("%s %d", k, n))
		}
		sort.Strings(ks)
		root.KV("not-mapped", strings.Join(ks, ", ")+" (no RimArcho equivalent)")
	}
	limit := 6
	if a.Bool("all") {
		limit = 0
	}
	list := func(title string, items []app.RCItem) {
		if len(items) == 0 {
			return
		}
		n := root.Section(fmt.Sprintf("%s (%d)", title, len(items)))
		for _, it := range items {
			n.Addf("%s  %s", it.Key, it.Reason)
		}
		n.Truncate(limit, "archo import rimcrow --db … --dry-run --all")
	}
	list("conflicts", rep.Conflicts)
	list("not-mapped", rep.Unmapped)
	list("rules-skipped", rep.RulesSkipped)
	// 02 §4.1: mods RimCrow switched off by renaming About.xml.
	if len(rep.AboutDisabled) > 0 || rep.CrowDisabled > 0 {
		var kinds []string
		for k, n := range rep.DisabledKinds {
			kinds = append(kinds, fmt.Sprintf("%d %s", n, k))
		}
		sort.Strings(kinds)
		n := root.KV("about-disabled", fmt.Sprintf("%d folders here have only About/About.xml.disabled (%s); RimCrow lists %d as disabled", len(rep.AboutDisabled), strings.Join(kinds, ", "), rep.CrowDisabled))
		n.Add("the game cannot load them and RimArcho does not rename files in Workshop folders")
		n.Add("to use one again: rename About.xml.disabled back to About.xml (or re-enable it in RimCrow), then archo scan")
		n.Add("to drop one: unsubscribe in Steam (workshop) or delete the folder (local)")
		for i, id := range rep.AboutDisabled {
			if i == 3 {
				n.Addf("… %d more (archo list --filter unloadable)", len(rep.AboutDisabled)-3)
				break
			}
			n.Add(modLabel(ix, id))
		}
	}
	if rep.DryRun {
		d.AddNext("archo import rimcrow --db " + quoteArg(rep.DB))
	} else {
		d.AddNext("archo tag list", "archo rule list")
	}
	return d, nil
}

func quoteArg(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}
