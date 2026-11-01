package app

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// CopyResult describes a finished copy-local (04 §7.2).
type CopyResult struct {
	Dir      string // the new folder under the local source
	Files    int
	Bytes    int64
	Skipped  []string // links that point outside the mod folder, relative to it
	GitError string   // --git failed after the copy succeeded
	Git      bool
	// onFile is called after each copied file (a pack build's progress); an error stops
	// the copy.
	onFile func(bytes int64) error
	// dry: count what would be copied, write nothing.
	dry bool
	// keepAll: copy .git and .rimarcho too (moving a locked copy, 04 §7).
	keepAll bool
	// generated: files RimArcho wrote into the mod (lower-cased paths relative to it; the
	// DDS of Texture optimisation), not part of the mod, so a copy leaves them out.
	generated map[string]bool
}

// generatedIn are the files RimArcho wrote into a mod folder (for CopyResult.generated).
func (a *App) generatedIn(dir string) map[string]bool {
	rows, err := a.Store.TextureFilesIn(dir)
	if err != nil || len(rows) == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, r := range rows {
		out[strings.ToLower(r.Rel)] = true
	}
	return out
}

// LocalSource is the environment's local source (the game's Mods folder).
func (ix *Index) LocalSource() *source.Source {
	for _, s := range ix.Sources {
		if s.Kind == source.Local {
			return s
		}
	}
	return nil
}

// copyLocal copies an instance into the folder root (the lock library) with an origin
// marker (04 §7.2). It never overwrites an existing folder (a taken name gets a numeric
// suffix), never copies .git and never follows links that leave the mod folder. The copy
// is assembled next to its final place and renamed, so a failure leaves nothing behind.
// dry checks the name and counts the files without writing.
func (a *App) copyLocal(st *LoadoutState, m *store.Mod, root, name string, git, dry bool) (*CopyResult, error) {
	src := st.Ix.ModDir(m)
	if real, err := source.RealPath(src); err == nil {
		src = real
	}
	if name == "" {
		name = filepath.Base(m.Rel) + "_local"
	}
	if err := plainName(name); err != nil {
		return nil, err
	}
	dst := filepath.Join(root, freeName(root, name))
	res := &CopyResult{Dir: dst, dry: dry, generated: a.generatedIn(src)}
	if dry {
		return res, copyTree(src, src, "", "", res)
	}
	tmp := dst + ".archo-tmp"
	os.RemoveAll(tmp)
	if err := copyTree(src, src, tmp, "", res); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	o := scan.Origin{InstanceID: m.ID, LineageID: m.LineageID, WorkshopID: m.WorkshopID(), SnapshotHash: hex.EncodeToString(m.SnapHash),
		CopiedAt: a.Now().UTC().Format("2006-01-02T15:04:05Z")}
	b, _ := json.MarshalIndent(o, "", "  ")
	if err := os.MkdirAll(filepath.Join(tmp, ".rimarcho"), 0o755); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(scan.OriginFile)), append(b, '\n'), 0o644); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if git {
		if err := gitBaseline(dst, m.DisplayName(), m.ID); err != nil {
			res.GitError = err.Error()
		} else {
			res.Git = true
		}
	}
	return res, nil
}

// copyTree copies dir (whose path relative to the mod root is rel) into dst.
func copyTree(root, dir, dst, rel string, res *CopyResult) error {
	if !res.dry {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, de := range ents {
		p := filepath.Join(dir, de.Name())
		r := filepath.ToSlash(filepath.Join(rel, de.Name()))
		if !res.keepAll && strings.EqualFold(de.Name(), ".git") {
			continue // the copy gets its own history (--git), never the upstream one
		}
		if !res.keepAll && rel == "" && strings.EqualFold(de.Name(), ".rimarcho") {
			continue // a fresh origin marker is written
		}
		if res.generated[strings.ToLower(r)] {
			continue // RimArcho's DDS: not the mod's
		}
		target := p
		if source.IsLink(p) {
			t, err := source.RealPath(p)
			if err != nil || !within(root, t) || within(t, dir) {
				res.Skipped = append(res.Skipped, r) // outside the mod folder, or a loop
				continue
			}
			target = t
		}
		fi, err := os.Stat(target)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if err := copyTree(root, target, filepath.Join(dst, de.Name()), r, res); err != nil {
				return err
			}
			continue
		}
		if !res.dry {
			if err := copyFile(target, filepath.Join(dst, de.Name()), fi); err != nil {
				return err
			}
		}
		res.Files++
		res.Bytes += fi.Size()
		if res.onFile != nil {
			if err := res.onFile(fi.Size()); err != nil {
				return err
			}
		}
	}
	return nil
}

// within reports whether p is root or below it (case-insensitive, as on Windows).
func within(root, p string) bool {
	r := strings.ToLower(filepath.Clean(root))
	q := strings.ToLower(filepath.Clean(p))
	return q == r || strings.HasPrefix(q, r+string(os.PathSeparator))
}

func copyFile(src, dst string, fi os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}

// gitBaseline makes the copy a repository with one commit of everything, ignored files
// included (the baseline must be the exact copy).
func gitBaseline(dir, name, origin string) error {
	run := func(args ...string) error {
		cmd := exec.Command("git", append([]string{"-c", "core.autocrlf=false", "-c", "user.name=RimArcho", "-c", "user.email=rimarcho@localhost", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH; the copy was made without a repository")
	}
	if err := run("init", "-q"); err != nil {
		return err
	}
	if err := run("add", "--all", "--force", "."); err != nil {
		return err
	}
	return run("commit", "-q", "--no-verify", "-m", fmt.Sprintf("Baseline: copy of %s (#%s)", name, origin))
}

// plainName checks a folder name given with --name: no path, no reserved characters.
func plainName(name string) error {
	if name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsAny(name, `<>:"|?*/`) || strings.ContainsRune(name, filepath.Separator) {
		return fmt.Errorf("--name must be a plain folder name: %q", name)
	}
	return nil
}
