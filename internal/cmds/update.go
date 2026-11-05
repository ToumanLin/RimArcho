package cmds

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

func init() {
	register(&Command{Name: "update", Summary: "check GitHub for a newer RimArcho and install it; the data folder is not touched (17 §5.1)",
		Flags: []Flag{{Name: "check", Help: "only check and show what installing would do; nothing is downloaded"}}, MaxArgs: 0, Run: updateCmd})
}

// updateCmd checks first in both forms; installing replaces this program's files once no
// other RimArcho runs from its folder and leaves restarting to the player.
func updateCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	c.App.CleanSelfUpdate()
	cur := c.App.SelfVersionNow()
	// Record the check as the automatic one does, so status and the window agree.
	ran, err := c.App.RunAutomaticNow(ctx, "self")
	if !ran && err == nil { // another window is recording a check right now
		_, _, err = c.App.CheckSelfUpdate(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot check for updates: %w", err)
	}
	info := c.App.SelfUpdateInfo()
	r := c.App.SelfUpdatePending()
	if a.Bool("check") || r == nil {
		d, root := render.New("RimArcho " + cur)
		updateFacts(c, root, info, r)
		if r != nil {
			d.AddNext("archo update")
		}
		return d, nil
	}
	if Progress != nil {
		Progress(fmt.Sprintf("progress: downloading RimArcho %s (%s)", r.Version, render.Size(r.Zip.Size)))
	}
	waited := false
	res, err := c.App.InstallSelfUpdate(ctx, func(s app.SelfUpdateStep) {
		if s.Phase == "wait" && !waited && Progress != nil {
			waited = true
			Progress("progress: waiting for " + othersText(s.Others) + " to close (Ctrl+C stops; nothing is changed until then)")
		}
	})
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("updated RimArcho %s → %s", res.From, res.To))
	root.KV("program", res.Program)
	root.KV("data", "unchanged: "+c.App.Home)
	root.Add("open RimArcho again to use the new version")
	return d, nil
}

func updateFacts(c *Ctx, root *render.Node, info app.SelfUpdate, r *selfupdate.Release) {
	if r == nil {
		if info.State == "development build" {
			root.KV("update", "a development build is not updated")
			return
		}
		latest := "none published yet"
		if info.Release != nil {
			latest = info.Release.Version
		}
		root.KV("latest", latest)
		state := info.State
		if state == "" || state == "new version available" { // recorded before this version was installed
			state = "up to date"
		}
		root.KV("update", state)
		if !info.Checked.IsZero() {
			root.KV("checked", c.App.FormatTime(info.Checked))
		}
		return
	}
	latest := r.Version
	if !r.Published.IsZero() {
		latest += " (" + r.Published.In(c.App.Loc).Format("2006-01-02") + ")"
	}
	root.KV("latest", latest)
	root.KV("checked", c.App.FormatTime(info.Checked))
	if err := r.Installable(); err != nil {
		root.KV("update", "available, but cannot be installed from here: "+err.Error())
		root.KV("page", r.Page)
		return
	}
	root.KV("update", "available")
	root.KV("download", fmt.Sprintf("%s, %s, checked against %s", r.Zip.Name, render.Size(r.Zip.Size), r.Sum.Name))
	if program, err := c.App.SelfProgramDir(); err == nil {
		root.KV("replaces", strings.Join(selfupdate.Files, ", ")+" in "+program)
		root.KV("keeps", "the data folder "+c.App.Home)
		if others := c.App.OtherArchoIn(program); len(others) > 0 {
			root.KV("waits for", othersText(others)+" to close first")
		}
	}
	root.KV("page", r.Page)
	if notes := strings.TrimSpace(r.Notes); notes != "" {
		n := root.Section("notes")
		for _, line := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
			n.Add(line)
		}
		n.Truncate(40, "see the page")
	}
}

// othersText: "2 other RimArcho (RimArcho.exe 1234, archo.exe 5678)".
func othersText(others []*app.GameRun) string {
	var names []string
	for _, o := range others {
		names = append(names, fmt.Sprintf("%s %d", filepath.Base(o.Exe), o.PID))
	}
	return fmt.Sprintf("%d other RimArcho (%s)", len(others), strings.Join(names, ", "))
}
