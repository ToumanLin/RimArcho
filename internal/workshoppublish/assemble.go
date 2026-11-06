// Package workshoppublish assembles the maintainer's Workshop item (17 §5.1).
package workshoppublish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ValidateVersion permits stable release names only, including in dry runs.
func ValidateVersion(version string) error {
	if !stableVersion.MatchString(version) {
		return errors.New("Workshop version must look like X.Y.Z; prereleases stay on GitHub only (use --version for a development build)")
	}
	return nil
}

// Item describes the assembled folder and the source file that remembers its ID.
type Item struct {
	Folder, Preview, ZipName, IDFile string
	ID                               uint64
}

// Assemble copies a built Monitor and the matching release assets from a repository
// root. Each run uses a fresh folder, so a previous release cannot leave extra files.
// The source About.xml is untouched; only the staged Mod name becomes RimArcho.
func Assemble(root, version string, id uint64) (*Item, error) {
	if err := ValidateVersion(version); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	name := selfupdate.ZipName(version, "windows", "amd64")
	zip := filepath.Join(root, "dist", name)
	if fi, err := regular(zip); err != nil {
		return nil, err
	} else if fi.Size() > 200<<20 {
		return nil, errors.New("release zip exceeds 200 MB")
	}
	sum := zip + ".sha256"
	b, err := readSmall(sum, 64<<10)
	if err != nil {
		return nil, err
	}
	want, err := selfupdate.ParseSum(b, name)
	if err != nil {
		return nil, err
	}
	var monitor string
	for _, p := range []string{
		filepath.Join(root, "dist", strings.TrimSuffix(name, ".zip"), "monitor", "ArchoMonitor"),
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
	preview := filepath.Join(monitor, "About", "Preview.png")
	if fi, err := regular(preview); errors.Is(err, os.ErrNotExist) {
		preview = ""
	} else if err != nil {
		return nil, err
	} else if fi.Size() >= 1<<20 {
		return nil, errors.New("Workshop preview must be smaller than 1 MB")
	}
	idFile := filepath.Join(root, "monitor", "About", "PublishedFileId.txt")
	if id == 0 {
		id, err = ReadID(idFile)
		if err != nil {
			return nil, err
		}
	}
	if id == 0 {
		id, err = ReadID(filepath.Join(monitor, "About", "PublishedFileId.txt"))
		if err != nil {
			return nil, err
		}
	}
	// Unique staging folders avoid deleting an earlier dry run or following its links.
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
	if err := setName(filepath.Join(folder, "About", "About.xml")); err != nil {
		return nil, err
	}
	if id != 0 {
		if err := WriteID(filepath.Join(folder, "About", "PublishedFileId.txt"), id); err != nil {
			return nil, err
		}
	}
	hash := sha256.New()
	if err := copyRegular(zip, filepath.Join(folder, name), hash); err != nil {
		return nil, err
	}
	if hex.EncodeToString(hash.Sum(nil)) != want {
		return nil, errors.New("release zip does not match its checksum")
	}
	if err := os.WriteFile(filepath.Join(folder, name+".sha256"), b, 0o644); err != nil {
		return nil, err
	}
	if preview != "" {
		preview = filepath.Join(folder, "About", "Preview.png")
	}
	ok = true
	return &Item{Folder: folder, Preview: preview, ZipName: name, IDFile: idFile, ID: id}, nil
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
	s := strings.TrimSpace(strings.TrimPrefix(string(b), "\uFEFF"))
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

func copyRegular(src, dst string, hash io.Writer) error {
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
	w := io.Writer(out)
	if hash != nil {
		w = io.MultiWriter(out, hash)
	}
	_, err = io.Copy(w, in)
	return errors.Join(err, out.Close())
}

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
		return copyRegular(path, to, nil)
	})
}

// Token streaming preserves every other metadata field and handles XML escaping.
func setName(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	d := xml.NewDecoder(strings.NewReader(strings.TrimPrefix(string(b), "\uFEFF")))
	var out strings.Builder
	e := xml.NewEncoder(&out)
	found := false
	depth := 0
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if start, ok := t.(xml.StartElement); ok && depth == 1 && strings.EqualFold(start.Name.Local, "name") {
			if err := d.Skip(); err != nil {
				return err
			}
			if err := e.EncodeElement("RimArcho", start); err != nil {
				return err
			}
			found = true
			continue
		}
		switch t.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
		if err := e.EncodeToken(t); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("%s has no name element", path)
	}
	if err := e.Flush(); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out.String()), 0o644)
}
