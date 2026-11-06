package cmds

import (
	"fmt"
	"os"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/workshoppublish"
)

var workshopPublishCommand = &Command{
	Name: "workshop publish", Summary: "maintainer only: assemble and publish RimArcho from the repository root through the signed-in Steam client",
	Flags: []Flag{
		{Name: "version", Value: "X.Y.Z", Help: "stable release version (default: this program's version)"},
		{Name: "item", Value: "<id>", Help: "Workshop item ID (default: Monitor's PublishedFileId.txt; absent creates an item)"},
		{Name: "note", Value: "<text>", Help: "Workshop change note"},
		{Name: "visibility", Value: "public|unlisted|private|friends", Help: "set visibility (default: leave Steam's current/default value)"},
		{Name: "dry-run", Help: "assemble and validate locally without contacting Steam"},
	},
	Run: workshopPublishCmd,
}

func init() { register(workshopPublishCommand) }

// WorkshopPublishOffline lets CLI help, bad arguments and dry runs avoid opening a
// player database. The normal publish uses the configured environment's Steam DLL.
func WorkshopPublishOffline(argv []string) bool {
	c, rest := Find(argv)
	if c != workshopPublishCommand {
		return false
	}
	if len(rest) > 0 && (rest[len(rest)-1] == "--help" || rest[len(rest)-1] == "-h") {
		return true
	}
	a, err := parse(c, rest)
	if err != nil {
		return true
	}
	if _, err := publishOptions(a); err != nil {
		return true
	}
	return a.Bool("dry-run")
}

func publishOptions(a *Args) (steamworks.PublishRequest, error) {
	r := steamworks.PublishRequest{Version: buildinfo.Version, Title: "RimArcho", Note: a.Str("note")}
	if v := a.Str("version"); v != "" {
		r.Version = v
	} else if a.Bool("version") {
		return r, &UsageError{Msg: "--version needs X.Y.Z"}
	}
	if err := workshoppublish.ValidateVersion(r.Version); err != nil {
		return r, &UsageError{Msg: err.Error()}
	}
	if a.Bool("item") {
		id, err := workshoppublish.ParseID(a.Str("item"))
		if err != nil {
			return r, &UsageError{Msg: err.Error()}
		}
		r.ID = id
	}
	if a.Bool("visibility") {
		v, ok := map[string]uint32{"public": 0, "friends": 1, "private": 2, "unlisted": 3}[a.Str("visibility")]
		if !ok {
			return r, &UsageError{Msg: "--visibility must be public, unlisted, private or friends"}
		}
		r.Visibility = &v
	}
	if strings.ContainsRune(r.Note, 0) || len(r.Note) > 8000 {
		return r, &UsageError{Msg: "--note must be at most 8000 bytes and contain no NUL characters"}
	}
	return r, nil
}

func workshopPublishCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if IsMCP(c.Context) {
		return nil, fmt.Errorf("Workshop publishing is a maintainer-only CLI command")
	}
	r, err := publishOptions(a)
	if err != nil {
		return nil, err
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	item, err := workshoppublish.Assemble(root, r.Version, r.ID)
	if err != nil {
		return nil, err
	}
	r.ID = item.ID
	r.Content, r.Preview, r.IDFile = item.Folder, item.Preview, item.IDFile
	if !a.Bool("dry-run") {
		id, legal, err := c.App.WorkshopPublish(c.Context, r, Progress)
		if err != nil {
			return nil, err
		}
		d, n := render.New(fmt.Sprintf("ws:%d RimArcho published", id))
		n.KV("version", r.Version)
		n.KV("item", fmt.Sprintf("https://steamcommunity.com/sharedfiles/filedetails/?id=%d", id))
		if legal {
			n.KV("action", "accept the Steam Workshop legal agreement on the item page; the item stays hidden until accepted")
		}
		return d, nil
	}
	d, n := render.New("RimArcho Workshop dry run: assembled and validated; Steam was not contacted")
	n.KV("content", item.Folder)
	n.KV("release", item.ZipName+" and "+item.ZipName+".sha256")
	n.KV("title", r.Title)
	if r.ID == 0 {
		n.KV("item", "would create a RimWorld Workshop item (app 294100)")
		n.KV("new ID", "would save to "+item.IDFile)
	} else {
		n.KV("item", fmt.Sprintf("would update ws:%d", r.ID))
	}
	n.KV("tags", "v"+r.Version+" (replaces all tags)")
	visibility := "leave Steam's current/default value"
	if r.Visibility != nil {
		visibility = a.Str("visibility")
	}
	n.KV("visibility", visibility)
	if r.Note != "" {
		n.KV("change note", r.Note)
	}
	if r.Preview != "" {
		n.KV("preview", r.Preview)
	}
	return d, nil
}
