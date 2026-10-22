package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Mod folders (02 §0): the folders every environment shares. MCP reads them with
// status show=folders; changing them is left to the player, like environments.

func init() {
	register(&Command{Name: "folder list", Summary: "list the mod folders every environment shares (Workshop, SteamCMD, others)", MaxArgs: 0, Run: folderList})
	register(&Command{Name: "folder add", Args: "workshop|steamcmd|extra <dir>", Summary: "add a shared mod folder (a second Workshop folder replaces the first)",
		MinArgs: 2, MaxArgs: 2, Run: folderAdd})
	register(&Command{Name: "folder remove", Args: "<src:label>", Summary: "forget a shared mod folder (its files stay)", MinArgs: 1, MaxArgs: 1, Run: folderRemove})
}

func folderList(c *Ctx, a *Args) (*render.Doc, error) {
	fs, err := c.App.Folders()
	if err != nil {
		return nil, err
	}
	counts, err := c.App.Store.LiveCounts()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("mod folders: %d (every environment)", len(fs)))
	var rows [][]string
	hasWS := false
	for _, s := range fs {
		st := fmt.Sprintf("%d mods", counts[s.ID])
		switch s.Status {
		case source.StatusOK:
		case source.StatusRebindPending:
			st += "  [rebind-pending → " + s.PendingRoot + "]"
		default:
			st += "  [" + s.Status + "]"
		}
		kind := string(s.Kind)
		if app.OwnFolder(s) {
			kind += ", RimArcho's"
		}
		if s.Kind == source.Workshop && s.Status != source.StatusRetired {
			hasWS = true
		}
		rows = append(rows, []string{s.Ref(), kind, s.Root, st})
	}
	root.AddColumns(rows)
	if !hasWS {
		if ws := c.App.DetectWorkshop(); ws != "" {
			root.KV("steam", "Workshop folder found: "+ws)
			d.AddNext("archo folder add workshop " + quoteArg(ws))
		}
	}
	d.AddNext("archo folder add steamcmd|extra <dir>")
	return d, nil
}

func folderAdd(c *Ctx, a *Args) (*render.Doc, error) {
	kind, err := source.ParseKind(a.Pos[0])
	if err != nil {
		return nil, &UsageError{Msg: err.Error()}
	}
	ch, err := c.App.FolderAdd(kind, a.Pos[1])
	if err != nil {
		return nil, err
	}
	return folderDoc(ch), nil
}

func folderRemove(c *Ctx, a *Args) (*render.Doc, error) {
	ch, err := c.App.FolderRemove(a.Pos[0])
	if err != nil {
		return nil, err
	}
	return folderDoc(ch), nil
}

func folderDoc(ch app.SourceChange) *render.Doc {
	s := ch.Source
	d, root := render.New(fmt.Sprintf("%s  %s", s.Ref(), ch.Action))
	switch ch.Action {
	case "removed":
		root.KV("folder", s.Root+" (its files stay; its mods become tombstones on the next scan)")
	case "rebind-pending":
		root.KV("folder", s.PendingRoot)
		root.KV("from", ch.OldRoot)
		d.AddNext("archo env rebind " + s.Ref() + " yes|no")
	default:
		root.KV("folder", s.Root)
		if ch.OldRoot != "" {
			root.KV("from", ch.OldRoot)
		}
		if s.Status == source.StatusMissing {
			root.KV("status", "missing (not scanned until it is there)")
		}
	}
	if r := ch.Rebind; r != nil {
		root.KV("sample", fmt.Sprintf("%d/%d matched (%.0f%%, need %.0f%%)", r.Matched, len(r.Samples), r.Rate*100, source.RebindRate*100))
	}
	d.AddNext("archo scan")
	return d
}
