package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	dry := []Flag{{Name: "dry-run", Help: "show what would happen"}}
	register(&Command{Name: "subscribe git", Args: "<url>", Summary: "install a Git mod and track its branch, tag or commit", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "branch", Value: "name", Help: "track a branch (default: remote HEAD)"}, {Name: "tag", Value: "name", Help: "track a tag"}, {Name: "commit", Value: "sha", Help: "pin a full commit SHA"}, dry[0]}, Run: subscribeGitCmd})
	register(&Command{Name: "subscribe list", Summary: "list installed subscriptions and their last update check", Run: subscriptionsCmd})
	register(&Command{Name: "subscribe check", Args: "[ref…]", Summary: "check upstream references for updates (default: all subscriptions)", MaxArgs: -1, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		op, err := c.App.CheckSubscriptions(a.Pos)
		return subscriptionDoc(op, err)
	}})
	register(&Command{Name: "subscribe update", Args: "[ref…]", Summary: "update subscriptions in place (refused while the game runs)", MaxArgs: -1, Flags: dry, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		op, err := c.App.UpdateSubscriptions(a.Pos, a.Bool("dry-run"))
		return subscriptionDoc(op, err)
	}})
	register(&Command{Name: "subscribe remove", Args: "<ref…>", Summary: "move subscription folders to the recycle bin; warn when plans use them", MinArgs: 1, MaxArgs: -1, Flags: dry, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		op, err := c.App.RemoveSubscriptions(a.Pos, a.Bool("dry-run"))
		return subscriptionDoc(op, err)
	}})
}

func subscribeGitCmd(c *Ctx, a *Args) (*render.Doc, error) {
	refType, ref, count := "branch", "", 0
	for _, typ := range []string{"branch", "tag", "commit"} {
		if a.Bool(typ) {
			count++
			refType, ref = typ, a.Str(typ)
			if ref == "" {
				return nil, fmt.Errorf("--%s requires a non-empty value", typ)
			}
		}
	}
	if count > 1 {
		return nil, fmt.Errorf("choose only one of --branch, --tag or --commit")
	}
	op, err := c.App.SubscribeGit(a.Pos[0], refType, ref, a.Bool("dry-run"))
	return subscriptionDoc(op, err)
}

func subscriptionsCmd(c *Ctx, _ *Args) (*render.Doc, error) {
	ss, err := c.App.Subscriptions()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("%d subscriptions", len(ss)))
	for _, s := range ss {
		r := s.Record
		n := root.Add(fmt.Sprintf("%s %s  %s", s.Ref, s.Mod.DisplayName(), r.Kind))
		n.KV("folder", s.Folder)
		if r.URL != "" {
			n.KV("url", r.URL)
		}
		if r.WorkshopID != "" {
			n.KV("workshop", r.WorkshopID)
		}
		if r.RefType != "" {
			ref := r.Ref
			if ref == "" {
				ref = "remote HEAD"
			}
			n.KV("tracking", r.RefType+" "+ref)
		}
		if r.Installed != "" {
			n.KV("installed", r.Installed)
		}
		state := "not checked"
		if !r.CheckedAt.IsZero() {
			state = "up to date"
			if r.HasUpdate() {
				state = "update available: " + r.Latest
			}
			n.KV("checked", r.CheckedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		if r.Kind == "steamcmd" {
			state = "update check unavailable"
		}
		if r.CheckError != "" {
			state = "check failed: " + r.CheckError
		}
		n.KV("state", state)
	}
	return d, nil
}

func subscriptionDoc(op *app.SubscribeOp, err error) (*render.Doc, error) {
	if op == nil {
		return nil, err
	}
	head := fmt.Sprintf("%s: %d subscriptions", op.Action, len(op.Items))
	if op.DryRun {
		head = "dry run, nothing changed: " + head
	}
	d, root := render.New(head)
	for _, it := range op.Items {
		n := root.Add(it.Ref + " " + it.Name)
		n.KV("folder", it.Folder)
		if it.To != "" {
			commit := it.To
			if it.From != "" {
				commit = it.From + " → " + it.To
			}
			n.KV("commit", commit)
		}
		if it.Result != "" {
			n.KV("result", it.Result)
		}
		if len(it.UsedBy) > 0 {
			n.KV("warning", usedByWarning(it.UsedBy))
		}
	}
	for _, note := range op.Notes {
		root.KV("note", note)
	}
	return d, err
}
