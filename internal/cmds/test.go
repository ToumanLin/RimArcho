package cmds

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/gametest"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "test run", Args: "[<plan>]", Summary: "run the game with isolated data, capture startup errors, then stop it", MaxArgs: 1,
		Flags: []Flag{
			{Name: "env", Value: "<name>", Help: "test another environment"},
			{Name: "map", Help: "load a quick test map instead of the main menu"},
			{Name: "timeout", Value: "<duration>", Help: "maximum run time (default 10m)"},
			{Name: "settle", Value: "<duration>", Help: "wait after the menu marker (default 5s)"},
			{Name: "json", Help: "print the report as JSON"},
		}, Run: testRunCmd})
}

func testRunCmd(c *Ctx, args *Args) (*render.Doc, error) {
	o := app.TestRunOptions{Env: args.Str("env"), Map: args.Bool("map"), Progress: c.progressCtx()}
	if len(args.Pos) > 0 {
		o.Plan = args.Pos[0]
	}
	for _, flag := range []struct {
		name string
		dst  *time.Duration
	}{{"timeout", &o.Timeout}, {"settle", &o.Settle}} {
		if args.Bool(flag.name) {
			d, err := time.ParseDuration(args.Str(flag.name))
			if err != nil || d <= 0 {
				return nil, &UsageError{Msg: fmt.Sprintf("--%s needs a positive duration, such as 30s or 10m", flag.name)}
			}
			*flag.dst = d
		}
	}
	r, err := c.App.TestRun(c.Context, o)
	if err != nil {
		return nil, err
	}
	if !r.Success() {
		c.ExitCode = ExitError
	}
	if args.Bool("json") {
		b, err := json.MarshalIndent(r, "", "  ")
		return &render.Doc{Raw: string(b) + "\n"}, err
	}
	d, root := render.New("test run: " + r.Plan + "  env:" + r.Env)
	root.KV("status", r.Status)
	for _, l := range r.Links {
		root.Addf("%s: %s -> %s: %s", l.Code, l.Path, l.Target, l.ShortReason())
	}
	root.KV("game", r.GameVersion)
	root.KV("load", fmt.Sprintf("%.1fs (total %.1fs)", r.LoadSeconds, r.TotalSeconds))
	root.KV("peak memory", fmt.Sprintf("%d MB", r.PeakMemoryMB))
	root.KV("requested", fmt.Sprint(r.Requested))
	missing := root.Section(fmt.Sprintf("missing: %d", len(r.Missing)))
	for _, m := range r.Missing {
		missing.Add(m.PackageID + "  " + m.Name)
	}
	clusters := root.Section(fmt.Sprintf("error clusters: %d", r.ClustersTotal))
	for _, cluster := range r.Clusters {
		prefix := ""
		if cluster.Delta == "new" {
			prefix = "new "
		}
		n := clusters.Addf("%s%s x%d: %s", prefix, cluster.Class, cluster.Count, cluster.Message)
		if cluster.Class == gametest.ClassSettings {
			n.KV("source", "game preferences")
			continue
		}
		mods := "no mod identified"
		if len(cluster.Mods) > 0 {
			mods = strings.Join(cluster.Mods, ", ")
		}
		n.KV("mods", mods+" ("+cluster.Confidence+")")
		if cluster.Note != "" {
			n.KV("note", cluster.Note)
		}
	}
	if more := r.ClustersTotal - len(r.Clusters); more > 0 {
		clusters.Addf("… %d more (full log: %s)", more, r.LogPath)
	}
	if b := r.Baseline; b != nil {
		root.Addf("compared with run %s (%s): %d new, %d present, %d gone", filepath.Base(b.RunDir), b.Status, b.New, b.Present, b.Gone)
		gone := root.Section(fmt.Sprintf("gone: %d", b.Gone))
		for _, cluster := range r.Gone {
			gone.Addf("%s x%d: %s", cluster.Class, cluster.Count, cluster.Message)
		}
	} else {
		root.Add("compared with: no earlier run of this plan")
	}
	root.KV("attribution", r.Attribution)
	root.KV("run dir", r.RunDir)
	return d, nil
}
