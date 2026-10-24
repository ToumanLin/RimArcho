package cmds

import (
	"encoding/json"
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	common := []Flag{{Name: "all", Help: "inspect every known game"}, {Name: "json", Help: "print structured JSON"}}
	register(&Command{Name: "links check", Summary: "check RimArcho links without changing files", MaxArgs: 0, Flags: common, Run: linksCheck})
	flags := append(append([]Flag{}, common...), Flag{Name: "dry-run", Help: "preview without changing files"}, Flag{Name: "yes", Help: "confirm the listed link removals and ledger recovery"}, Flag{Name: "include", Value: "<link-path>", Multi: true, Help: "explicitly include an unproven prefixed link after reviewing its evidence"})
	register(&Command{Name: "links repair", Summary: "preview or confirm link repair and unfinished-operation recovery", MaxArgs: 0, Flags: flags, Run: linksRepair})
}

func linksCheck(c *Ctx, args *Args) (*render.Doc, error) {
	p, err := c.App.PlanLinkRepair(args.Bool("all"), nil)
	if err != nil {
		return nil, err
	}
	for _, l := range p.Links {
		if l.Code == app.CodeLinkDangling || l.Code == app.CodeLinkUnknown || l.Code == app.CodeLinkRecovery {
			c.ExitCode = ExitError
		}
	}
	return linksDoc(p, args.Bool("json"), "links check")
}

func linksRepair(c *Ctx, args *Args) (*render.Doc, error) {
	if !args.Bool("dry-run") && !args.Bool("yes") {
		return nil, &UsageError{Msg: "preview with --dry-run, or confirm with --yes; unproven links also require --include <link-path>"}
	}
	if args.Bool("dry-run") && args.Bool("yes") {
		return nil, &UsageError{Msg: "choose --dry-run or --yes"}
	}
	p, err := c.App.PlanLinkRepair(args.Bool("all"), args.List("include"))
	if err != nil {
		return nil, err
	}
	title := "links repair preview"
	if args.Bool("yes") {
		if err := c.App.ApplyLinkRepair(p); err != nil {
			return nil, err
		}
		title = "links repair"
	}
	return linksDoc(p, args.Bool("json"), title)
}

func linksDoc(p *app.LinkRepairPlan, asJSON bool, title string) (*render.Doc, error) {
	if asJSON {
		b, err := json.MarshalIndent(p, "", "  ")
		return &render.Doc{Raw: string(b) + "\n"}, err
	}
	d, root := render.New(title)
	for _, l := range p.Links {
		n := root.Add(l.Game)
		n.KV("link", l.Path)
		n.KV("target", l.Target)
		n.KV("state", fmt.Sprintf("%s %s (%s)", l.Code, l.State, l.Kind))
		n.KV("reason", l.ShortReason())
		n.KV("evidence", l.Evidence)
		n.KV("action", l.Action)
	}
	if title != "links check" {
		root.KV("removed", fmt.Sprint(len(p.Removed)))
	}
	return d, nil
}
