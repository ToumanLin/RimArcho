package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Archo Monitor (19 §6): where the bundled monitor is, which versions are installed, and
// whether the current plan has it; `install` copies, registers and applies the default.
func init() {
	register(&Command{Name: "monitor", Summary: "Archo Monitor, the in-game companion: versions, source, place in the plan", MaxArgs: 0, Run: monitorCmd})
	register(&Command{Name: "monitor install", Summary: "install or update the bundled Archo Monitor and add it to plans by default (19 §6)", MaxArgs: 0,
		Flags: []Flag{{Name: "dry-run", Help: "show what would be copied and registered"}}, Run: monitorInstallCmd})
}

func monitorCmd(c *Ctx, a *Args) (*render.Doc, error) {
	s, err := c.App.MonitorState()
	if err != nil {
		return nil, err
	}
	d, root := render.New("Archo Monitor")
	if s.Bundled != nil {
		root.KV("bundled", fmt.Sprintf("%s (%s)", s.Bundled.Version, s.Bundled.Dir))
	} else {
		root.KV("bundled", "none found (looked in "+strings.Join(s.Tried, ", ")+")")
	}
	inst := "none"
	if len(s.Installed) > 0 {
		inst = strings.Join(s.Installed, ", ")
	}
	root.KV("installed", fmt.Sprintf("%s (%s)", inst, s.Dir))
	if s.Source != nil {
		root.KV("source", fmt.Sprintf("src:%s %s", s.Source.Label, s.Source.Root))
	} else {
		root.KV("source", "not registered in this environment")
	}
	root.KV("plan", MonitorPlanLine(c, s))
	def := "on"
	if !s.Default {
		def = "off"
	}
	root.KV("default", def+" (archo config set monitor.default on|off: add it to new plans and on every sort)")
	if s.Bundled != nil && (len(s.Installed) == 0 || s.Installed[0] != s.Bundled.Version || s.Source == nil) {
		d.AddNext("archo monitor install")
	}
	return d, nil
}

// MonitorPlanLine says whether the current plan has the monitor (status, monitor).
func MonitorPlanLine(c *Ctx, s *app.MonitorStatus) string {
	switch {
	case s.InPlan != nil:
		where := fmt.Sprintf("at %d", s.Position)
		if s.AfterHarmony {
			where = "after Harmony"
		}
		v := strings.TrimSpace(s.InPlan.Decl().ModVersion)
		return fmt.Sprintf("ArchoMonitor %s in the plan (%s)", v, where)
	case s.Newest != nil:
		ref := s.Newest.Mod.ID
		if st := c.Loadout(); st != nil {
			ref = st.Ix.Ref(ref)
		}
		return "not in the plan (archo enable " + ref + ")"
	}
	return "not installed"
}

func monitorInstallCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.EnsureMonitor(a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	title := "Archo Monitor"
	if r.DryRun {
		title += " (dry run: nothing written)"
	}
	d, root := render.New(title)
	if r.Bundled == nil {
		root.KV("bundled", "none found (looked in "+strings.Join(r.Tried, ", ")+")")
	}
	for _, l := range MonitorLines(r, true) {
		root.KV("monitor", l)
	}
	return d, nil
}

// MonitorLines describe a check (19 §6); all includes the plans left alone and why.
func MonitorLines(r *app.MonitorResult, all bool) []string {
	var out []string
	if r.Bundled != nil {
		switch {
		case r.Copied:
			out = append(out, fmt.Sprintf("installed %s to %s", r.Bundled.Version, r.Target))
		case r.Replaced:
			out = append(out, fmt.Sprintf("replaced %s (a development build changed)", r.Target))
		case all:
			out = append(out, fmt.Sprintf("up to date: %s (%s)", r.Bundled.Version, r.Target))
		}
	}
	if r.Deferred != "" {
		out = append(out, r.Deferred)
	}
	if r.Registered {
		out = append(out, "registered src:"+app.MonitorLabel+" (an extra source; the game sees it through an _Archo_ link)")
	}
	if r.Merged > 0 {
		out = append(out, fmt.Sprintf("merged %d older monitor lineage(s) into the newest", r.Merged))
	}
	for _, p := range r.Plans {
		switch {
		case p.Change != "":
			out = append(out, fmt.Sprintf("plan:%s: %s (one undoable edit)", p.Plan, p.Change))
		case all && p.Reason != "":
			out = append(out, fmt.Sprintf("plan:%s: not added: %s", p.Plan, p.Reason))
		}
	}
	return append(out, r.Notes...)
}
