package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
)

// Subscribing through the Steam client (16 §2.9, 11 §2).
func init() {
	dry := []Flag{{Name: "dry-run", Help: "show what would happen"}}
	register(&Command{Name: "workshop unsubscribe", Args: "<ref…>", Summary: "unsubscribe in Steam: Steam deletes the Workshop folder (refused while the game runs; plans that use it show it missing)",
		MinArgs: 1, MaxArgs: -1, Flags: dry, Run: func(c *Ctx, a *Args) (*render.Doc, error) { return workshopCmd(c, a, steamworks.Unsubscribe) }})
	register(&Command{Name: "workshop subscribe", Args: "<ref…>", Summary: "subscribe again in Steam to an unsubscribed leftover",
		MinArgs: 1, MaxArgs: -1, Flags: dry, Run: func(c *Ctx, a *Args) (*render.Doc, error) { return workshopCmd(c, a, steamworks.Subscribe) }})
	register(&Command{Name: "workshop resubscribe", Args: "<ref…>", Summary: "resubscribe and wait for Steam to download mods; local edits are lost (refused while the game runs)",
		MinArgs: 1, MaxArgs: -1, Flags: dry, Run: func(c *Ctx, a *Args) (*render.Doc, error) { return workshopCmd(c, a, steamworks.Resubscribe) }})
	register(&Command{Name: "workshop state", Args: "<ref…>", Summary: "ask Steam for item state; without Steam, show local folders with account state unknown",
		MinArgs: 1, MaxArgs: -1, Run: workshopStateCmd})
}

func workshopCmd(c *Ctx, a *Args, action string) (*render.Doc, error) {
	last := map[int]int{}
	b := app.BatchCalls{
		Stopped: func() bool { return c.Context != nil && c.Context.Err() != nil },
		Progress: func(i int, got, total int64) {
			if Progress == nil {
				return
			}
			bucket := 0
			if total > 0 {
				bucket = min(int(float64(got)*10/float64(total)), 10)
			}
			old, seen := last[i]
			if !seen || bucket != old {
				last[i] = bucket
				if total > 0 {
					Progress(fmt.Sprintf("progress: %s downloading %d%% (%d / %d bytes)", a.Pos[i], bucket*10, got, total))
				} else {
					Progress("progress: " + a.Pos[i] + " waiting for Steam")
				}
			}
		},
		Done: func(i int, err error) {
			if Progress == nil {
				return
			}
			if err == nil {
				Progress("progress: " + a.Pos[i] + " downloaded")
			} else {
				Progress("progress: " + a.Pos[i] + " " + err.Error())
			}
		},
	}
	op, err := c.App.Workshop(action, a.Pos, a.Bool("dry-run"), b)
	if err != nil && op == nil {
		return nil, steamErr(err)
	}
	ix, ierr := c.App.Index()
	if ierr != nil {
		return nil, ierr
	}
	verb := map[string]string{steamworks.Subscribe: "subscribed", steamworks.Unsubscribe: "unsubscribed", steamworks.Resubscribe: "resubscribed"}[action]
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
			if action == steamworks.Resubscribe {
				what = "RimArcho moves to the recycle bin before Steam re-downloads; local edits are lost:"
				if op.DryRun {
					what = "RimArcho would move to the recycle bin before Steam re-downloads; local edits are lost:"
				}
			}
			n.KV("folder", what+" "+it.Folder)
		}
		if len(it.UsedBy) > 0 {
			n.KV("warning", usedByWarning(it.UsedBy))
		}
		for _, line := range it.Links.Lines() {
			n.KV("affected link", line)
		}
		if it.Result != "" {
			n.KV("steam", it.Result+" · "+steamworks.StateText(it.State))
		}
	}
	switch {
	case op.DryRun:
		d.AddNext("archo workshop " + action + " " + strings.Join(a.Pos, " "))
	case action == steamworks.Resubscribe && err == nil:
		root.KV("note", "Steam downloads have finished; the next scan sees the downloaded folders")
	case action == steamworks.Resubscribe:
		root.KV("note", "some downloads did not finish; check Steam and the item results")
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
	head := fmt.Sprintf("Steam state of %d Workshop items", len(op.Items))
	if op.Local {
		head = fmt.Sprintf("Local state of %d Workshop items (Steam account state unknown)", len(op.Items))
	}
	d, root := render.New(head)
	if op.Local {
		root.KV("note", op.Note+"; showing local folders only; subscription and update state unknown")
	}
	for _, it := range op.Items {
		state := steamworks.StateText(it.State)
		if op.Local {
			state = "no local folder found"
			if it.Installed {
				state = "local folder present"
			}
		}
		n := root.Add(fmt.Sprintf("%s %s  ws:%s  %s", ix.Ref(it.ID), it.Name, it.WorkshopID, state))
		if op.Local {
			n.KV("folder", it.Folder)
		}
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
