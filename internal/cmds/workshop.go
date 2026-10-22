package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
)

// Subscribing through the Steam client (06 §2.9, 08 §10).
func init() {
	dry := []Flag{{Name: "dry-run", Help: "show what would happen"}}
	register(&Command{Name: "workshop unsubscribe", Args: "<ref…>", Summary: "unsubscribe in Steam: Steam deletes the Workshop folder (refused while enabled, in a plan, or the game runs)",
		MinArgs: 1, MaxArgs: -1, Flags: dry, Run: func(c *Ctx, a *Args) (*render.Doc, error) { return workshopCmd(c, a, steamworks.Unsubscribe) }})
	register(&Command{Name: "workshop subscribe", Args: "<ref…>", Summary: "subscribe again in Steam to an unsubscribed leftover",
		MinArgs: 1, MaxArgs: -1, Flags: dry, Run: func(c *Ctx, a *Args) (*render.Doc, error) { return workshopCmd(c, a, steamworks.Subscribe) }})
	register(&Command{Name: "workshop state", Args: "<ref…>", Summary: "ask Steam whether items are subscribed, installed, waiting for an update",
		MinArgs: 1, MaxArgs: -1, Run: workshopStateCmd})
}

func workshopCmd(c *Ctx, a *Args, action string) (*render.Doc, error) {
	op, err := c.App.Workshop(action, a.Pos, a.Bool("dry-run"))
	if err != nil && op == nil {
		return nil, steamErr(err)
	}
	ix, ierr := c.App.Index()
	if ierr != nil {
		return nil, ierr
	}
	verb := map[string]string{steamworks.Subscribe: "subscribed", steamworks.Unsubscribe: "unsubscribed"}[action]
	items := fmt.Sprintf("%d Workshop items", len(op.Items))
	if len(op.Items) == 1 {
		items = "1 Workshop item"
	}
	head := verb + " " + items
	if op.DryRun {
		head = "dry run, nothing changed: would " + action + " " + items
	}
	d, root := render.New(head)
	for _, it := range op.Items {
		n := root.Add(fmt.Sprintf("%s %s  ws:%s", ix.Ref(it.ID), it.Name, it.WorkshopID))
		if it.Folder != "" {
			what := "Steam deletes"
			if op.DryRun {
				what = "Steam would delete"
			}
			n.KV("folder", what+" "+it.Folder)
		}
		if it.Result != "" {
			n.KV("steam", it.Result+" · "+steamworks.StateText(it.State))
		}
	}
	switch {
	case op.DryRun:
		d.AddNext("archo workshop " + action + " " + strings.Join(a.Pos, " "))
	case action == steamworks.Subscribe:
		root.KV("note", "Steam downloads it in the background; the next scan sees it subscribed")
	default:
		root.KV("note", "the next scan sees the folders gone")
	}
	return d, err
}

func workshopStateCmd(c *Ctx, a *Args) (*render.Doc, error) {
	op, err := c.App.WorkshopState(a.Pos)
	if err != nil {
		return nil, steamErr(err)
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("Steam state of %d Workshop items", len(op.Items)))
	for _, it := range op.Items {
		root.Add(fmt.Sprintf("%s %s  ws:%s  %s", ix.Ref(it.ID), it.Name, it.WorkshopID, steamworks.StateText(it.State)))
	}
	return d, nil
}

// steamErr: when Steam is out of reach, say where to do it by hand.
func steamErr(err error) error {
	var u *app.SteamUnavailable
	if errors.As(err, &u) && len(u.URLs) > 0 {
		return fmt.Errorf("%w; do it on the Workshop page: %s", err, strings.Join(u.URLs, " "))
	}
	return err
}
