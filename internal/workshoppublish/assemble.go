// Package workshoppublish assembles the maintainer's Archo Monitor Workshop item (17 §5.1).
package workshoppublish

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/scan"
)

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ValidateVersion permits stable release names only, including in dry runs.
func ValidateVersion(version string) error {
	if !stableVersion.MatchString(version) {
		return errors.New("Workshop version must look like X.Y.Z; prereleases stay on GitHub only (use --version for a development build)")
	}
	return nil
}

// RetiredItem is the RimArcho program item Steam removed (17 §5.1): never published to again.
const RetiredItem uint64 = 3835688284

// ErrRetiredItem: the ID files still name the retired item. The maintainer passes --item with
// the Archo Monitor item, or --new-item to create it once (17 §5.1, bootstrap).
var ErrRetiredItem = errors.New("monitor/About/PublishedFileId.txt names the retired RimArcho item 3835688284; pass --item <Archo Monitor item ID>, or --new-item to create the Archo Monitor item")

// Title is the item's title in every language.
const Title = "Archo Monitor"

// Description and DescriptionZh are the item's English and Simplified Chinese
// descriptions in the repository, Steam BBCode; publish uses them without --description
// and --description-zh and adds the version line (17 §5.1).
const (
	Description   = "UserGuide/monitor-item.steam.txt"
	DescriptionZh = "UserGuide/monitor-item.zh-CN.steam.txt"
)

// PreviewAsset is the Workshop page's preview (an animated GIF) in the repository.
const PreviewAsset = "docs/assets/release-post/workshop-preview.gif"

// Item describes the assembled folder and the source file that remembers its ID.
// Tags are the item's Workshop tags: Mod and the game versions Monitor supports, the
// tags RimWorld's own uploader sets.
type Item struct {
	Folder, Preview, Monitor, IDFile string
	ID                               uint64
	Tags                             []string
}

// Options: ID publishes to that item; New ignores the ID files and creates an item.
type Options struct {
	ID  uint64
	New bool
}

var gameVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// Tags reads a Mod folder's About.xml: Mod, then each supported game version.
func Tags(folder string) ([]string, error) {
	d := scan.ReadDecl(folder, false)
	if d.ParseError != "" {
		return nil, fmt.Errorf("Archo Monitor About.xml: %s", d.ParseError)
	}
	tags := []string{"Mod"}
	for _, v := range d.SupportedVersions {
		if !gameVersion.MatchString(v) {
			return nil, fmt.Errorf("Archo Monitor About.xml: supported version %q is not a game version like 1.6", v)
		}
		if !slices.Contains(tags, v) {
			tags = append(tags, v)
		}
	}
	if len(tags) == 1 {
		return nil, errors.New("Archo Monitor About.xml lists no supported game versions")
	}
	return tags, nil
}

// Assemble copies a built Archo Monitor, and nothing else, into a fresh folder under dist/
// (17 §5.1 Upload): the item is a Mod, never a program. Each run uses a fresh folder, so a
// previous release cannot leave extra files. About.xml stays Monitor's.
func Assemble(root, version string, o Options) (*Item, error) {
	if err := ValidateVersion(version); err != nil {
		return nil, err
	}
	if o.ID == RetiredItem {
		return nil, errors.New("3835688284 is the retired RimArcho item; publish to the Archo Monitor item")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var monitor string
	for _, p := range []string{
		filepath.Join(root, "dist", "archo-"+version+"-windows-amd64", "monitor", "ArchoMonitor"),
		filepath.Join(root, "monitor", "ArchoMonitor"),
		filepath.Join(root, "monitor", "out", "ArchoMonitor"),
	} {
		if _, err := os.Stat(filepath.Join(p, "About", "About.xml")); err == nil {
			monitor = p
			break
		}
	}
	if monitor == "" {
		return nil, errors.New("no built Archo Monitor found; run scripts/release.ps1 or monitor/build.ps1 first")
	}
	for _, p := range []string{"About/About.xml", "LoadFolders.xml", "1.5/Assemblies/ArchoMonitor.dll", "1.6/Assemblies/ArchoMonitor.dll", "1.7/Assemblies/ArchoMonitor.dll"} {
		if _, err := regular(filepath.Join(monitor, filepath.FromSlash(p))); err != nil {
			return nil, err
		}
	}
	if fi, err := os.Stat(filepath.Join(monitor, "Languages")); err != nil || !fi.IsDir() {
		return nil, errors.New("built Archo Monitor has no Languages directory")
	}
	tags, err := Tags(monitor)
	if err != nil {
		return nil, err
	}
	preview := filepath.Join(monitor, "About", "Preview.png")
	if fi, err := regular(preview); errors.Is(err, os.ErrNotExist) {
		preview = ""
	} else if err != nil {
		return nil, err
	} else if fi.Size() >= 1<<20 {
		return nil, errors.New("Workshop preview must be smaller than 1 MB")
	}
	// The repository's Workshop preview, when present, replaces Monitor's Preview.png on
	// the item page. It stays outside the item folder: the game shows Preview.png.
	asset := filepath.Join(root, filepath.FromSlash(PreviewAsset))
	if fi, err := regular(asset); errors.Is(err, os.ErrNotExist) {
		asset = ""
	} else if err != nil {
		return nil, err
	} else if fi.Size() >= 1<<20 {
		return nil, fmt.Errorf("%s must be smaller than 1 MB", PreviewAsset)
	}
	idFile := filepath.Join(root, "monitor", "About", "PublishedFileId.txt")
	id := o.ID
	if !o.New {
		for _, f := range []string{idFile, filepath.Join(monitor, "About", "PublishedFileId.txt")} {
			if id != 0 {
				break
			}
			if id, err = ReadID(f); err != nil {
				return nil, err
			}
		}
		if id == RetiredItem {
			return nil, ErrRetiredItem
		}
	}
	// Unique staging folders avoid deleting an earlier dry run or following its links.
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		return nil, err
	}
	folder, err := os.MkdirTemp(filepath.Join(root, "dist"), "workshop-"+version+"-")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(folder)
		}
	}()
	if err := copyTree(monitor, folder); err != nil {
		return nil, err
	}
	// The staged ID file is always the item being published, never a copied old one.
	staged := filepath.Join(folder, "About", "PublishedFileId.txt")
	if err := os.Remove(staged); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if id != 0 {
		if err := WriteID(staged, id); err != nil {
			return nil, err
		}
	}
	if err := CheckContent(folder); err != nil {
		return nil, err
	}
	if asset != "" {
		preview = asset
	} else if preview != "" {
		preview = filepath.Join(folder, "About", "Preview.png")
	}
	ok = true
	return &Item{Folder: folder, Preview: preview, Monitor: monitor, IDFile: idFile, ID: id, Tags: tags}, nil
}

// CheckContent refuses a folder that holds anything but a Mod: a program (.exe), an archive
// or checksum (.zip, .sha256, .tar.gz, .gz), or a .dll outside a game version's Assemblies
// folder. Steam removed item 3835688284 for carrying a program (17 §5.1).
func CheckContent(folder string) error {
	var bad []string
	err := filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(folder, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		name := strings.ToLower(d.Name())
		switch {
		case strings.HasSuffix(name, ".exe"), strings.HasSuffix(name, ".zip"), strings.HasSuffix(name, ".sha256"), strings.HasSuffix(name, ".gz"):
			bad = append(bad, rel)
		case strings.HasSuffix(name, ".dll"):
			if parts := strings.Split(rel, "/"); len(parts) != 3 || !gameVersion.MatchString(parts[0]) || parts[1] != "Assemblies" {
				bad = append(bad, rel)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(bad) > 0 {
		return fmt.Errorf("the Workshop item may hold only the Archo Monitor Mod, but the staging folder has %s", strings.Join(bad, ", "))
	}
	return nil
}

// ReadID treats an absent or empty file as a new item, and rejects malformed IDs.
func ReadID(path string) (uint64, error) {
	b, err := readSmall(path, 128)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(strings.TrimPrefix(string(b), string(rune(0xFEFF))))
	if s == "" {
		return 0, nil
	}
	id, err := ParseID(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	return id, nil
}

func ParseID(s string) (uint64, error) {
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("Workshop item ID must be a positive decimal integer")
		}
	}
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("Workshop item ID must be a positive decimal integer")
	}
	return id, nil
}

func WriteID(path string, id uint64) error {
	return os.WriteFile(path, []byte(strconv.FormatUint(id, 10)+"\n"), 0o644)
}

func regular(path string) (fs.FileInfo, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return fi, nil
}

func readSmall(path string, limit int64) ([]byte, error) {
	fi, err := regular(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return os.ReadFile(path)
}

func copyRegular(src, dst string) error {
	if _, err := regular(src); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

// copyTree copies regular files and refuses links.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a link; publish only regular Mod files", path)
		}
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		return copyRegular(path, to)
	})
}
