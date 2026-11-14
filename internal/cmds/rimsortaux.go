package cmds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/l10ndata"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "import rimsort", Summary: "import RimSort's notes, tags, colours and ignored warnings (as tags) from an instance's aux_metadata.db; repeatable", MaxArgs: 0, Run: importRimSortCmd,
		Flags: []Flag{{Name: "db", Value: "<path>", Help: `RimSort's instances\<instance>\aux_metadata.db (read from a copy)`},
			{Name: "dry-run", Help: "report what would be imported; write nothing"}, {Name: "all"}}})
}

func importRimSortCmd(c *Ctx, a *Args) (*render.Doc, error) {
	db := a.Str("db")
	if db == "" {
		return nil, &UsageError{Msg: `--db <path>: RimSort's database, %LOCALAPPDATA%\RimSort\instances\<instance>\aux_metadata.db`}
	}
	// the tag names in the player's language, as the window writes them (07 §6.4)
	tags := app.ManagerTagNamesIn(l10ndata.ResolveLang(Setting(c.App, "ui.lang")))
	rep, err := c.App.ImportRimSortAux(app.RimSortAuxOptions{DB: db, DryRun: a.Bool("dry-run"), Tags: tags})
	if err != nil {
		return nil, err
	}
	head := "import rimsort"
	if rep.DryRun {
		head += " (dry run: nothing written)"
	}
	d, root := render.New(head)
	root.KV("database", rep.DB+" (read from a copy)")
	root.KV("mods", fmt.Sprintf("%d with data: %d mapped (%d by path, %d by Workshop ID), %d not mapped", rep.Records, rep.ByPath+rep.ByPackage, rep.ByPath, rep.ByPackage, len(rep.Unmapped)))
	root.KV("notes", fmt.Sprintf("%d new, %d already set", rep.NoteNew, rep.NoteSame))
	root.KV("tags", fmt.Sprintf("%d new, %d already present, %d tags given a colour", rep.TagNew, rep.TagSame, rep.TagColors))
	root.KV("conflicts", fmt.Sprintf("%d (RimArcho's value kept)", len(rep.Conflicts)))
	if len(rep.NotMapped) > 0 {
		var ks []string
		for k, n := range rep.NotMapped {
			ks = append(ks, fmt.Sprintf("%s %d", k, n))
		}
		sort.Strings(ks)
		root.KV("not-mapped", strings.Join(ks, ", ")+" (not a colour RimArcho can read)")
	}
	limit := 6
	if a.Bool("all") {
		limit = 0
	}
	for _, g := range []struct {
		title string
		items []app.RCItem
	}{{"conflicts", rep.Conflicts}, {"not-mapped", rep.Unmapped}} {
		if len(g.items) == 0 {
			continue
		}
		n := root.Section(fmt.Sprintf("%s (%d)", g.title, len(g.items)))
		for _, it := range g.items {
			n.Addf("%s  %s", it.Key, it.Reason)
		}
		n.Truncate(limit, "archo import rimsort --db … --dry-run --all")
	}
	if rep.DryRun {
		d.AddNext("archo import rimsort --db " + quoteArg(rep.DB))
	} else {
		d.AddNext("archo tag list")
	}
	return d, nil
}
