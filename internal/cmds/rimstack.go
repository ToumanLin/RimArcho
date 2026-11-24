package cmds

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/l10ndata"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "import rimstack", Summary: "import RimStack's notes, tags, ignored warnings (as a tag), Loaders/Last categories (as top/bottom rules) and load-order rules; repeatable", MaxArgs: 0, Run: importRimStackCmd,
		Flags: []Flag{{Name: "dir", Value: "<folder>", Help: `RimStack's data folder (default: %LOCALAPPDATA%\RimStack, ~/.local/share/RimStack); only read`},
			{Name: "dry-run", Help: "report what would be imported; write nothing"}, {Name: "all"}}})
}

// rimStackDir is the folder given, or the first place RimStack keeps its data that holds
// its notes or rules.
func rimStackDir(given string) (string, error) {
	if given != "" {
		if st, err := os.Stat(given); err != nil || !st.IsDir() {
			return "", fmt.Errorf("--dir %s: not a folder", given)
		}
		return given, nil
	}
	for _, d := range app.RimStackDataDirs() {
		for _, f := range []string{"notes.json", "rules.json"} {
			if st, err := os.Stat(filepath.Join(d, f)); err == nil && st.Mode().IsRegular() {
				return d, nil
			}
		}
	}
	return "", &UsageError{Msg: "no RimStack data found; --dir <folder>: RimStack's data folder, " + strings.Join(app.RimStackDataDirs(), " or ")}
}

func importRimStackCmd(c *Ctx, a *Args) (*render.Doc, error) {
	dir, err := rimStackDir(a.Str("dir"))
	if err != nil {
		return nil, err
	}
	dry := a.Bool("dry-run")
	// the tag names in the player's language, as the window writes them (07 §6.4)
	tags := app.ManagerTagNamesIn(l10ndata.ResolveLang(Setting(c.App, "ui.lang")))
	var notes, rules *app.RimCrowReport
	notesPath, rulesPath := filepath.Join(dir, "notes.json"), filepath.Join(dir, "rules.json")
	if _, err := os.Stat(notesPath); err == nil {
		if notes, err = c.App.ImportRimStack(app.RimStackOptions{Notes: notesPath, DryRun: dry, Tags: tags}); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(rulesPath); err == nil {
		if rules, err = c.App.ImportRimStackRules(rulesPath, dry); err != nil {
			return nil, err
		}
	}
	if notes == nil && rules == nil {
		return nil, errors.New(dir + ": neither notes.json nor rules.json is there")
	}
	head := "import rimstack"
	if dry {
		head += " (dry run: nothing written)"
	}
	d, root := render.New(head)
	root.KV("folder", dir+" (only read)")
	var conflicts, unmapped, skipped []app.RCItem
	if notes != nil {
		root.KV("mods", fmt.Sprintf("%d with data: %d mapped by packageId, %d not mapped", notes.Records, notes.ByPackage, len(notes.Unmapped)))
		root.KV("notes", fmt.Sprintf("%d new, %d already set", notes.NoteNew, notes.NoteSame))
		root.KV("tags", fmt.Sprintf("%d new, %d already present (ignored warnings and trusted versions: one tag)", notes.TagNew, notes.TagSame))
		root.KV("categories", fmt.Sprintf("%d new top/bottom rules, %d already present (Loaders = top, Last = bottom)", notes.RuleNew, notes.RuleSame))
		conflicts, unmapped = notes.Conflicts, notes.Unmapped
	} else {
		root.KV("notes", "no notes.json")
	}
	if rules != nil {
		root.KV("rules", fmt.Sprintf("%d: %d new before rules, %d already present, %d skipped", rules.RulesTotal, rules.RuleNew, rules.RuleSame, len(rules.RulesSkipped)))
		skipped = rules.RulesSkipped
	} else {
		root.KV("rules", "no rules.json")
	}
	notMapped := map[string]int{}
	for _, r := range []*app.RimCrowReport{notes, rules} {
		if r != nil {
			for k, n := range r.NotMapped {
				notMapped[k] += n
			}
		}
	}
	if len(notMapped) > 0 {
		var ks []string
		for k, n := range notMapped {
			ks = append(ks, fmt.Sprintf("%s %d", k, n))
		}
		sort.Strings(ks)
		root.KV("not-mapped", strings.Join(ks, ", ")+" (no RimArcho equivalent)")
	}
	limit := 6
	if a.Bool("all") {
		limit = 0
	}
	for _, g := range []struct {
		title string
		items []app.RCItem
	}{{"conflicts", conflicts}, {"not-mapped", unmapped}, {"skipped-rules", skipped}} {
		if len(g.items) == 0 {
			continue
		}
		n := root.Section(fmt.Sprintf("%s (%d)", g.title, len(g.items)))
		for _, it := range g.items {
			n.Addf("%s  %s", it.Key, it.Reason)
		}
		n.Truncate(limit, "archo import rimstack --dry-run --all")
	}
	if dry {
		next := "archo import rimstack"
		if a.Str("dir") != "" {
			next += " --dir " + quoteArg(dir)
		}
		d.AddNext(next)
	} else {
		d.AddNext("archo tag list")
		d.AddNext("archo rule list")
	}
	d.AddNext(fmt.Sprintf("archo loadout import %s --new", quoteArg(filepath.Join(dir, "lists", "<name>.json"))))
	return d, nil
}
