package workshoppublish

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/scan"
)

// ModItem is an author's own Mod staged for upload (13 §4.4, --mod): the folder as
// RimWorld's uploader would send it, minus development files.
type ModItem struct {
	Folder, Source, IDFile, Title, Preview string
	ID                                     uint64
	Tags                                   []string
	Files                                  int
	Bytes                                  int64
	// Excluded lists the top-most skipped paths, relative and slash-separated.
	Excluded []string
}

// ModOptions: ID publishes to that item; New creates an item, ignoring PublishedFileId.txt;
// Exclude adds relative paths (a file or a folder) to leave out.
type ModOptions struct {
	ID      uint64
	New     bool
	Exclude []string
}

// developmentDirs are left out wherever they appear: build output and editor state.
var developmentDirs = []string{"obj", "bin"}

// AssembleMod copies a Mod folder into a fresh staging folder under the system temp
// directory. Like RimWorld's own uploader, the item's title is About.xml's name, its tags are
// Mod and the supported game versions, and its preview is About/Preview.png. Unlike it,
// entries whose name starts with a dot (.git, .vs, .gitignore) and obj/ and bin/ folders are
// left out, and the result must pass the same content check as Archo Monitor.
func AssembleMod(source string, o ModOptions) (*ModItem, error) {
	if o.ID == RetiredItem {
		return nil, errors.New("3835688284 is the retired RimArcho item; it cannot be published to")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	if _, err := regular(filepath.Join(source, "About", "About.xml")); err != nil {
		return nil, fmt.Errorf("%s is not a Mod folder: no About/About.xml", source)
	}
	d := scan.ReadDecl(source, false)
	if d.Name == "" && d.ParseError == "" {
		return nil, errors.New("the Mod's About.xml has no <name>; the Workshop title comes from it")
	}
	tags, err := tagsOf(d, "the Mod's")
	if err != nil {
		return nil, err
	}
	exclude, err := normalizeExcludes(o.Exclude)
	if err != nil {
		return nil, err
	}
	idFile := filepath.Join(source, "About", "PublishedFileId.txt")
	id := o.ID
	if id == 0 && !o.New {
		if id, err = ReadID(idFile); err != nil {
			return nil, err
		}
		if id == 0 {
			return nil, errors.New("the Mod has no About/PublishedFileId.txt; pass --item <id> to update an existing item, or --new-item to create one")
		}
	}
	if id == RetiredItem {
		return nil, ErrRetiredItem
	}

	folder, err := os.MkdirTemp("", "archo-workshop-mod-")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(folder)
		}
	}()
	item := &ModItem{Folder: folder, Source: source, IDFile: idFile, Title: d.Name, ID: id, Tags: tags}
	if err := stage(item, exclude); err != nil {
		return nil, err
	}
	// The staged ID file is always the item being published (the worker writes a created one).
	if id != 0 {
		if err := WriteID(filepath.Join(folder, "About", "PublishedFileId.txt"), id); err != nil {
			return nil, err
		}
	}
	if err := checkContent(folder, "a Mod (pass --exclude <path> for development files)"); err != nil {
		return nil, err
	}
	if item.Preview, err = stagedPreview(folder); err != nil {
		return nil, err
	}
	ok = true
	return item, nil
}

func normalizeExcludes(paths []string) ([]string, error) {
	exclude := make([]string, 0, len(paths))
	for _, e := range paths {
		e = strings.Trim(filepath.ToSlash(filepath.Clean(e)), "/")
		if e == "" || e == "." || e == ".." || strings.HasPrefix(e, "../") || filepath.IsAbs(e) {
			return nil, fmt.Errorf("--exclude %q must be a path inside the Mod folder", e)
		}
		exclude = append(exclude, strings.ToLower(e))
	}
	return exclude, nil
}

// stage copies the Mod's files into item.Folder, skipping development files and exclude.
func stage(item *ModItem, exclude []string) error {
	source, folder := item.Source, item.Folder
	return filepath.WalkDir(source, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return err
		}
		slash := filepath.ToSlash(rel)
		lower := strings.ToLower(slash)
		name := strings.ToLower(e.Name())
		if strings.HasPrefix(name, ".") || e.IsDir() && slices.Contains(developmentDirs, name) || slices.Contains(exclude, lower) ||
			lower == "about/publishedfileid.txt" {
			if lower != "about/publishedfileid.txt" {
				item.Excluded = append(item.Excluded, slash)
			}
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a link; publish only regular Mod files, or --exclude %s", path, slash)
		}
		to := filepath.Join(folder, rel)
		if e.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		fi, err := e.Info()
		if err != nil {
			return err
		}
		item.Files++
		item.Bytes += fi.Size()
		return copyRegular(path, to)
	})
}

// stagedPreview finds About/Preview.png, which a Windows folder may spell preview.png.
func stagedPreview(folder string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(folder, "About"))
	if err != nil {
		return "", nil
	}
	for _, e := range entries {
		if !strings.EqualFold(e.Name(), "Preview.png") || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(folder, "About", e.Name())
		fi, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if fi.Size() >= 1<<20 {
			return "", errors.New("About/Preview.png must be smaller than 1 MB for the Workshop; pass --preview <smaller image>")
		}
		return path, nil
	}
	return "", nil
}

// Remove deletes the staging folder after a real publish; Steam has read it by then.
func (m *ModItem) Remove() error { return os.RemoveAll(m.Folder) }
