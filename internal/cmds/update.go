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
	register(&Command{Name: "update", Summary: "check for a newer RimArcho and show update options; the data folder is not touched (17 §5.1)",
		Flags: []Flag{{Name: "check", Help: "only check and show what installing would do; nothing is downloaded"},
			{Name: "from", Value: "<zip>", Help: "install a release ZIP you downloaded; the original file is kept"}}, MaxArgs: 0, Run: updateCmd})
}

// updateCmd checks before automatic installation; --from also works offline. It replaces files once no
// other RimArcho runs from its folder and leaves restarting to the player.
func updateCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	picked := a.Str("from")
	if picked != "" && a.Bool("check") {
		return nil, fmt.Errorf("--from and --check cannot be used together")
	}
	cur := c.App.SelfVersionNow()
	if picked == "" {
		c.App.CleanSelfUpdate()
		// Record the check as the automatic one does, so status and the window agree.
		ran, err := c.App.RunAutomaticNow(ctx, "self")
		if !ran && err == nil { // another window is recording a check right now
			_, _, err = c.App.CheckSelfUpdate(ctx)
		}
		if err != nil && !a.Bool("check") {
			return nil, fmt.Errorf("cannot check for updates: %w", err)
		}
		info := c.App.SelfUpdateInfo()
		if err != nil {
			info.State = "check unavailable offline"
		}
		r := c.App.SelfUpdatePending()
		if a.Bool("check") || r == nil || !app.SelfUpdateSupported() {
			d, root := render.New("RimArcho " + cur)
			updateFacts(c, root, info, r)
			if r != nil && app.SelfUpdateSupported() {
				d.AddNext("archo update")
			}
			return d, nil
		}
		if Progress != nil {
			Progress(fmt.Sprintf("progress: downloading RimArcho %s (%s)", r.Version, updateSize(r)))
		}
	}
	waited := false
	step := func(s app.SelfUpdateStep) {
		if s.Phase == "wait" && !waited && Progress != nil {
			waited = true
			Progress("progress: waiting for " + othersText(s.Others) + " to close (Ctrl+C stops; nothing is changed until then)")
		}
	}
	var res *app.SelfUpdateResult
	var err error
	if picked != "" {
		res, err = c.App.InstallSelfUpdateFrom(ctx, picked, step)
	} else {
		res, err = c.App.InstallSelfUpdate(ctx, step)
	}
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
	if len(info.Sources) > 0 {
		sources := root.Section("sources")
		for _, source := range info.Sources {
			value := source.Status
			switch source.Status {
			case "ok":
				value += " " + source.Version
			case "unconfigured":
				value = "not configured"
			default:
				if source.Error != "" {
					value += ": " + source.Error
				}
			}
			sources.KV(source.Source, value)
		}
	}
	if info.Source != "" {
		root.KV("source", info.Source)
	}
	if info.Fallback != "" {
		root.KV("fallback", info.Fallback)
	}
	if info.Manual != "" {
		root.KV("manual", app.SelfUpdateManualInstructions(info.Manual))
	}
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
	if info.State == "check unavailable offline" {
		root.KV("check", info.State)
	}
	root.KV("checked", c.App.FormatTime(info.Checked))
	if !app.SelfUpdateSupported() {
		root.KV("update", "available")
		root.KV("page", r.Page)
		root.KV("how", c.App.SelfUpdateInstructions())
		if notes := strings.TrimSpace(r.Notes); notes != "" {
			root.Section("notes").Add(notes)
		}
		return
	}
	if err := r.Installable(); err != nil {
		root.KV("update", "available, but cannot be installed from here: "+err.Error())
		root.KV("page", r.Page)
		return
	}
	root.KV("update", "available")
	root.KV("download", fmt.Sprintf("%s, %s", r.Zip.Name, updateSize(r)))
	root.KV("integrity", "GitHub SHA-256 when available; ZIP CRC32 and program startup check")
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

func updateSize(r *selfupdate.Release) string {
	if r.Zip.Size <= 0 {
		return "size unknown"
	}
	return render.Size(r.Zip.Size)
}

// othersText names the processes an install is waiting for.
func othersText(others []*app.GameRun) string {
	var names []string
	for _, o := range others {
		names = append(names, fmt.Sprintf("%s %d", filepath.Base(o.Exe), o.PID))
	}
	return fmt.Sprintf("%d other RimArcho (%s)", len(others), strings.Join(names, ", "))
}
