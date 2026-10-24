package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

func subscriptionLines(c *Ctx, n *render.Node, r *subscribe.Record) {
	if r == nil {
		return
	}
	n.KV("subscription", r.Kind)
	n.KV("origin URL", r.URL)
	n.KV("repository", r.Repo)
	if r.Kind == "steamcmd" {
		n.KV("workshop ID", r.WorkshopID)
		if !r.InstalledAt.IsZero() {
			n.KV("last download", c.App.FormatTime(r.InstalledAt))
		}
		return
	}
	if r.RefType != "" {
		tracking := r.RefType + " " + r.Ref
		if r.RefType == "branch" && r.Ref == "" {
			tracking = "default branch"
		}
		n.KV("tracking", tracking)
	}
	installed := r.Installed
	if r.Kind == "git" && len(installed) > 7 {
		installed = installed[:7]
	}
	if !r.InstalledAt.IsZero() {
		installed += "  " + c.App.FormatTime(r.InstalledAt)
	}
	n.KV("installed", installed)
	checked := "not checked"
	if !r.CheckedAt.IsZero() {
		state := "up to date"
		if r.HasUpdate() {
			state = "update available"
		}
		checked = c.App.FormatTime(r.CheckedAt) + "  " + state
	}
	if r.CheckError != "" {
		checked = c.App.FormatTime(r.CheckedAt) + "  failed: " + r.CheckError
	}
	n.KV("last check", checked)
}

func init() {
	dry := []Flag{{Name: "dry-run", Help: "show what would happen"}}
	register(&Command{Name: "subscribe git", Args: "<url>", Summary: "install a Git mod and track its branch, tag or commit", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "branch", Value: "name", Help: "track a branch (default: remote HEAD)"}, {Name: "tag", Value: "name", Help: "track a tag"}, {Name: "commit", Value: "sha", Help: "pin a full commit SHA"}, dry[0]}, Run: subscribeGitCmd})
	register(&Command{Name: "subscribe list", Summary: "list installed subscriptions and their last update check", Run: subscriptionsCmd})
	register(&Command{Name: "subscribe page", Args: "<url>", Summary: "inspect a download page or install its Git repository or a downloaded archive", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "git", Help: "subscribe to the page's single linked repository"}, {Name: "from", Value: "zip|folder", Help: "install files downloaded in your browser"}, dry[0]}, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
			if a.Bool("from") && (a.Str("from") == "" || a.Bool("git")) {
				return nil, fmt.Errorf("--from requires a zip or folder and cannot be combined with --git")
			}
			op, err := c.App.SubscribePage(a.Pos[0], a.Bool("git"), a.Str("from"), a.Bool("dry-run"))
			return subscriptionDoc(op, err)
		}})
	register(&Command{Name: "subscribe workshop", Args: "<id|url…>", Summary: "subscribe through Steam, with SteamCMD fallback when Steam is unavailable", MinArgs: 1, MaxArgs: -1,
		Flags: []Flag{{Name: "steamcmd", Help: "download with SteamCMD directly"}, dry[0]}, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
			op, err := c.App.SubscribeWorkshop(a.Pos, a.Bool("steamcmd"), a.Bool("dry-run"))
			return subscriptionDoc(op, err)
		}})
	register(&Command{Name: "subscribe check", Args: "[ref…]", Summary: "check upstream references for updates (default: all subscriptions)", MaxArgs: -1, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		op, err := c.App.CheckSubscriptions(a.Pos)
		return subscriptionDoc(op, err)
	}})
	register(&Command{Name: "subscribe update", Args: "[ref…]", Summary: "update subscriptions; page subscriptions require --from (refused while the game runs)", MaxArgs: -1, Flags: append([]Flag{{Name: "from", Value: "zip|folder", Help: "replace one page subscription with downloaded files"}}, dry...), Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		if a.Bool("from") {
			if len(a.Pos) != 1 || a.Str("from") == "" {
				return nil, fmt.Errorf("--from requires one subscription reference and a zip or folder")
			}
			op, err := c.App.UpdateSubscriptionFrom(a.Pos[0], a.Str("from"), a.Bool("dry-run"))
			return subscriptionDoc(op, err)
		}
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
		if r.Repo != "" {
			n.KV("repo", r.Repo)
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
			n.KV("version", commit)
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
