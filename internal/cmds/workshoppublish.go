package cmds

import (
	"fmt"
	"os"
	"path/filepath"
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
		{Name: "description", Value: "<file>", Help: "set the description from a Steam BBCode file, at most 8000 bytes (default: leave it)"},
		{Name: "description-zh", Value: "<file>", Help: "set the Simplified Chinese description from a Steam BBCode file"},
		{Name: "preview", Value: "<file>", Help: "Workshop page preview (PNG, JPG or GIF, under 1 MB); default: docs/assets/release-post/workshop-preview.gif, else Monitor's About/Preview.png"},
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
	var err error
	if r.Description, err = descriptionFile(a, "description"); err != nil {
		return r, err
	}
	if zh, err := descriptionFile(a, "description-zh"); err != nil {
		return r, err
	} else if zh != "" {
		r.Translations = []steamworks.PublishTranslation{{Language: "schinese", Description: zh}}
	}
	if a.Bool("preview") {
		if r.Preview, err = previewFile(a.Str("preview")); err != nil {
			return r, err
		}
	}
	return r, nil
}

// descriptionFile reads a description flag's file; an absent flag leaves Steam's text.
func descriptionFile(a *Args, flag string) (string, error) {
	if !a.Bool(flag) {
		return "", nil
	}
	path := a.Str(flag)
	if path == "" {
		return "", &UsageError{Msg: "--" + flag + " needs a file"}
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return "", &UsageError{Msg: "--" + flag + ": " + path + " is not a readable file"}
	}
	if fi.Size() > steamworks.DescriptionMax {
		return "", &UsageError{Msg: fmt.Sprintf("--%s: %s is %d bytes; Steam allows at most %d", flag, path, fi.Size(), steamworks.DescriptionMax)}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(b), "\uFEFF"))
	if text == "" || strings.ContainsRune(text, 0) {
		return "", &UsageError{Msg: "--" + flag + ": " + path + " is empty or contains NUL characters"}
	}
	return text, nil
}

// previewFile checks what Steam accepts as a preview: an image under 1 MB.
func previewFile(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif":
	default:
		return "", &UsageError{Msg: "--preview must be a PNG, JPG or GIF file"}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return "", &UsageError{Msg: "--preview: " + path + " is not a readable file"}
	}
	if fi.Size() >= 1<<20 {
		return "", &UsageError{Msg: "--preview: Workshop preview must be smaller than 1 MB"}
	}
	return abs, nil
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
	r.Content, r.IDFile = item.Folder, item.IDFile
	if r.Preview == "" {
		r.Preview = item.Preview
	}
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
	if r.Description != "" {
		n.KV("description", fmt.Sprintf("%d bytes (replaces the current one)", len(r.Description)))
	}
	for _, t := range r.Translations {
		n.KV("description "+t.Language, fmt.Sprintf("%d bytes, set in a second update", len(t.Description)))
	}
	return d, nil
}
