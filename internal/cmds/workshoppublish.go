package cmds

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/workshoppublish"
)

var workshopPublishCommand = &Command{
	Name: "workshop publish", Summary: "publish the Archo Monitor Workshop item from the repository root (maintainer), or with --mod one of your own Mods, through the signed-in Steam client",
	Flags: []Flag{
		{Name: "mod", Value: "<folder>", Help: "publish this Mod folder to its item (About/PublishedFileId.txt) instead of Archo Monitor; title from About.xml, description kept unless --description"},
		{Name: "exclude", Value: "<path>", Multi: true, Help: "with --mod: leave out this file or folder, relative to the Mod (repeatable; dot-entries, obj/ and bin/ are always left out)"},
		{Name: "version", Value: "X.Y.Z", Help: "stable release version (default: this program's version)"},
		{Name: "item", Value: "<id>", Help: "Archo Monitor item ID (default: monitor/About/PublishedFileId.txt; absent creates an item; never the retired 3835688284)"},
		{Name: "new-item", Help: "create the Archo Monitor item, ignoring PublishedFileId.txt (once, when it still names the retired item)"},
		{Name: "note", Value: "<text>", Help: "Workshop change note"},
		{Name: "visibility", Value: "public|unlisted|private|friends", Help: "set visibility (default: leave Steam's current/default value)"},
		{Name: "description", Value: "<file>", Help: "description from a Steam BBCode file, at most 8000 bytes with the version line added (default: UserGuide/monitor-item.steam.txt)"},
		{Name: "description-zh", Value: "<file>", Help: "Simplified Chinese description from a Steam BBCode file (default: UserGuide/monitor-item.zh-CN.steam.txt, else leave it)"},
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
	if a.Bool("mod") {
		return modPublishOptions(a)
	}
	if a.Bool("exclude") {
		return steamworks.PublishRequest{}, &UsageError{Msg: "--exclude works only with --mod"}
	}
	r := steamworks.PublishRequest{Version: buildinfo.Version, Title: workshoppublish.Title, Note: a.Str("note")}
	if v := a.Str("version"); v != "" {
		r.Version = v
	} else if a.Bool("version") {
		return r, &UsageError{Msg: "--version needs X.Y.Z"}
	}
	if err := workshoppublish.ValidateVersion(r.Version); err != nil {
		return r, &UsageError{Msg: err.Error()}
	}
	if err := commonPublishOptions(a, &r); err != nil {
		return r, err
	}
	// Every publish sets the descriptions: the last line of each names the release, and
	// the update check reads it there (17 §5.1).
	var err error
	if r.Description, err = descriptionFile(a, "description", workshoppublish.Description); err != nil {
		return r, err
	}
	if r.Description == "" {
		return r, &UsageError{Msg: "--description needs a file: " + workshoppublish.Description + " is absent, and the description must name the release for the update check"}
	}
	if zh, err := descriptionFile(a, "description-zh", workshoppublish.DescriptionZh); err != nil {
		return r, err
	} else if zh != "" {
		r.Translations = []steamworks.PublishTranslation{{Language: "schinese", Description: zh}}
	}
	texts := []*string{&r.Description}
	for i := range r.Translations {
		texts = append(texts, &r.Translations[i].Description)
	}
	for _, d := range texts {
		if *d = selfupdate.Stamp(*d, r.Version); len(*d) > steamworks.DescriptionMax {
			return r, &UsageError{Msg: fmt.Sprintf("a Workshop description with its version line is %d bytes; Steam allows at most %d", len(*d), steamworks.DescriptionMax)}
		}
	}
	return r, nil
}

// commonPublishOptions reads the flags Archo Monitor and --mod share.
func commonPublishOptions(a *Args, r *steamworks.PublishRequest) error {
	if a.Bool("item") {
		id, err := workshoppublish.ParseID(a.Str("item"))
		if err != nil {
			return &UsageError{Msg: err.Error()}
		}
		if id == workshoppublish.RetiredItem {
			return &UsageError{Msg: "3835688284 is the retired RimArcho item; it cannot be published to"}
		}
		if a.Bool("new-item") {
			return &UsageError{Msg: "--item and --new-item cannot be used together"}
		}
		r.ID = id
	}
	if a.Bool("visibility") {
		v, ok := map[string]uint32{"public": 0, "friends": 1, "private": 2, "unlisted": 3}[a.Str("visibility")]
		if !ok {
			return &UsageError{Msg: "--visibility must be public, unlisted, private or friends"}
		}
		r.Visibility = &v
	}
	if strings.ContainsRune(r.Note, 0) || len(r.Note) > 8000 {
		return &UsageError{Msg: "--note must be at most 8000 bytes and contain no NUL characters"}
	}
	if a.Bool("preview") {
		var err error
		if r.Preview, err = previewFile(a.Str("preview")); err != nil {
			return err
		}
	}
	return nil
}

// modPublishOptions: an author's own Mod. No release stamp and no default description:
// without --description the item keeps its Steam description, as RimWorld's uploader does.
func modPublishOptions(a *Args) (steamworks.PublishRequest, error) {
	r := steamworks.PublishRequest{Version: "mod", Note: a.Str("note")}
	if a.Str("mod") == "" {
		return r, &UsageError{Msg: "--mod needs a Mod folder"}
	}
	if a.Bool("version") {
		return r, &UsageError{Msg: "--version applies to Archo Monitor only; a Mod's version is in its About.xml"}
	}
	if err := commonPublishOptions(a, &r); err != nil {
		return r, err
	}
	var err error
	if a.Bool("description") {
		if r.Description, err = descriptionFile(a, "description", ""); err != nil {
			return r, err
		}
	}
	if a.Bool("description-zh") {
		zh, err := descriptionFile(a, "description-zh", "")
		if err != nil {
			return r, err
		}
		r.Translations = []steamworks.PublishTranslation{{Language: "schinese", Description: zh}}
	}
	return r, nil
}

// descriptionFile reads a description flag's file. Without the flag it reads the
// repository's file at fallback (relative to the repository root, the working
// directory), and "" when that is absent.
func descriptionFile(a *Args, flag, fallback string) (string, error) {
	path := a.Str(flag)
	if !a.Bool(flag) {
		if _, err := os.Stat(filepath.FromSlash(fallback)); errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		path = filepath.FromSlash(fallback)
	} else if path == "" {
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
	if a.Bool("mod") {
		return modPublishCmd(c, a, r)
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	item, err := workshoppublish.Assemble(root, r.Version, workshoppublish.Options{ID: r.ID, New: a.Bool("new-item")})
	if err != nil {
		return nil, err
	}
	r.ID, r.Tags = item.ID, item.Tags
	r.Content, r.IDFile = item.Folder, item.IDFile
	if r.Preview == "" {
		r.Preview = item.Preview
	}
	if !a.Bool("dry-run") {
		id, legal, err := c.App.WorkshopPublish(c.Context, r, Progress)
		if err != nil {
			return nil, err
		}
		d, n := render.New(fmt.Sprintf("ws:%d Archo Monitor published", id))
		n.KV("version", r.Version)
		n.KV("item", fmt.Sprintf("https://steamcommunity.com/sharedfiles/filedetails/?id=%d", id))
		if legal {
			n.KV("action", "accept the Steam Workshop legal agreement on the item page; the item stays hidden until accepted")
		}
		if a.Bool("new-item") {
			n.KV("next", fmt.Sprintf("set updatechannel.MonitorWorkshopItem to %d and commit monitor/About/PublishedFileId.txt before building the next release (17 §5.1)", id))
		}
		return d, nil
	}
	d, n := render.New("Archo Monitor Workshop dry run: assembled and validated; Steam was not contacted")
	n.KV("content", item.Folder)
	n.KV("monitor", "built Mod from "+item.Monitor+" (no program, archive or checksum)")
	n.KV("title", r.Title)
	if r.ID == 0 {
		n.KV("item", "would create a RimWorld Workshop item (app 294100)")
		n.KV("new ID", "would save to "+item.IDFile)
	} else {
		n.KV("item", fmt.Sprintf("would update ws:%d", r.ID))
	}
	n.KV("tags", strings.Join(r.Tags, ", ")+" (replaces all tags)")
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
		n.KV("description", fmt.Sprintf("%d bytes (replaces the current one), ending %q", len(r.Description), lastLine(r.Description)))
	}
	for _, t := range r.Translations {
		n.KV("description "+t.Language, fmt.Sprintf("%d bytes, set in a second update, ending %q", len(t.Description), lastLine(t.Description)))
	}
	return d, nil
}

// modPublishCmd stages and publishes an author's own Mod (13 §4.4, --mod).
func modPublishCmd(c *Ctx, a *Args, r steamworks.PublishRequest) (*render.Doc, error) {
	item, err := workshoppublish.AssembleMod(a.Str("mod"), workshoppublish.ModOptions{ID: r.ID, New: a.Bool("new-item"), Exclude: a.List("exclude")})
	if err != nil {
		return nil, err
	}
	r.ID, r.Tags, r.Title = item.ID, item.Tags, item.Title
	r.Content, r.IDFile = item.Folder, item.IDFile
	if r.Preview == "" {
		r.Preview = item.Preview
	}
	if !a.Bool("dry-run") {
		defer item.Remove()
		id, legal, err := c.App.WorkshopPublish(c.Context, r, Progress)
		if err != nil {
			return nil, err
		}
		d, n := render.New(fmt.Sprintf("ws:%d %s published", id, r.Title))
		n.KV("item", fmt.Sprintf("https://steamcommunity.com/sharedfiles/filedetails/?id=%d", id))
		if legal {
			n.KV("action", "accept the Steam Workshop legal agreement on the item page; the item stays hidden until accepted")
		}
		if a.Bool("new-item") {
			n.KV("id file", "saved to "+item.IDFile)
		}
		return d, nil
	}
	d, n := render.New(r.Title + " Workshop dry run: staged and validated; Steam was not contacted")
	n.KV("content", fmt.Sprintf("%s (%d files, %s, from %s)", item.Folder, item.Files, sizeText(item.Bytes), item.Source))
	if len(item.Excluded) > 0 {
		n.KV("left out", strings.Join(item.Excluded, ", "))
	}
	n.KV("title", r.Title)
	if r.ID == 0 {
		n.KV("item", "would create a RimWorld Workshop item (app 294100)")
		n.KV("new ID", "would save to "+item.IDFile)
	} else {
		n.KV("item", fmt.Sprintf("would update ws:%d", r.ID))
	}
	n.KV("tags", strings.Join(r.Tags, ", ")+" (replaces all tags)")
	visibility := "leave Steam's current/default value"
	if r.Visibility != nil {
		visibility = a.Str("visibility")
	}
	n.KV("visibility", visibility)
	if r.Note != "" {
		n.KV("change note", r.Note)
	}
	preview := "leave the item page's preview"
	if r.Preview != "" {
		preview = r.Preview
	}
	n.KV("preview", preview)
	description := "leave the item's description"
	if r.Description != "" {
		description = fmt.Sprintf("%d bytes (replaces the current one)", len(r.Description))
	}
	n.KV("description", description)
	for _, t := range r.Translations {
		n.KV("description "+t.Language, fmt.Sprintf("%d bytes, set in a second update", len(t.Description)))
	}
	return d, nil
}

func sizeText(b int64) string {
	if b < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
}

func lastLine(s string) string { return s[strings.LastIndexByte(s, '\n')+1:] }
