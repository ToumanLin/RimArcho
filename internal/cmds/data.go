package cmds

import (
	"fmt"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Community data (04 §5.2, 10 §5).
func init() {
	keys := make([]string, 0, len(app.Datasets))
	for _, d := range app.Datasets {
		keys = append(keys, d.Key)
	}
	register(&Command{Name: "data update", Args: "[<dataset>…]", Summary: "download the community data (" + strings.Join(keys, ", ") + "); the only command that fetches it", MaxArgs: len(keys), Run: dataUpdateCmd,
		Flags: []Flag{{Name: "dry-run", Help: "show the sources, download nothing"}}})
	register(&Command{Name: "data status", Summary: "the community data: source, version, when it was fetched, entries", MaxArgs: 0, Run: dataStatusCmd})
	for _, d := range app.Datasets {
		def := d.URL
		freeKeys["data."+d.Key+".url"] = struct {
			def   string
			check func(string) error
		}{def, func(v string) error {
			if strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://") || strings.ContainsAny(v, `/\`) {
				return nil
			}
			return fmt.Errorf("give an http(s) URL or a local file path")
		}}
	}
}

func dataUpdateCmd(c *Ctx, a *Args) (*render.Doc, error) {
	for _, k := range a.Pos {
		ok := false
		for _, d := range app.Datasets {
			ok = ok || d.Key == k
		}
		if !ok {
			return nil, &UsageError{Msg: fmt.Sprintf("unknown dataset %q", k)}
		}
	}
	res, err := c.App.UpdateData(a.Pos, a.Bool("dry-run"))
	title := "community data updated"
	if a.Bool("dry-run") {
		title = "data update: dry run, nothing downloaded"
	}
	d, root := render.New(title)
	for _, u := range res {
		line := u.Info.Source
		switch {
		case u.Err != nil:
			line = "failed: " + u.Err.Error()
		case u.Same:
			line = "unchanged since the last download"
		case !a.Bool("dry-run"):
			line = fmt.Sprintf("%d entries, %.1f MB, version %s", u.Info.Entries, float64(u.Info.Bytes)/(1<<20), u.Info.Version)
		}
		root.KV(u.Dataset.Key, line)
	}
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, d.String())
	}
	return d, nil
}

func dataStatusCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st := c.App.DataStatus()
	d, root := render.New("community data")
	stale := false
	for _, ds := range app.Datasets {
		di := st[ds.Key]
		if di == nil {
			root.KV(ds.Key, "not downloaded ("+ds.Desc+")")
			stale = true
			continue
		}
		age := c.App.Now().Sub(di.Fetched)
		line := fmt.Sprintf("%d entries, fetched %s (%d days ago), version %s", di.Entries, c.App.FormatDate(di.Fetched), int(age.Hours()/24), di.Version)
		if age > 30*24*time.Hour {
			line += "  [older than 30 days]"
			stale = true
		}
		n := root.KV(ds.Key, line)
		n.Add("source: " + di.Source)
	}
	if stale {
		d.AddNext("archo data update")
	}
	return d, nil
}
