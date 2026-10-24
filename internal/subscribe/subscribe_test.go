package subscribe_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestRecord(t *testing.T) {
	dir := t.TempDir()
	r := &subscribe.Record{Kind: "git", URL: "local", RefType: "branch", Ref: "main", Installed: "a", InstalledAt: time.Now().UTC().Truncate(time.Second), Latest: "b", CheckedAt: time.Now().UTC().Truncate(time.Second)}
	for _, latest := range []string{"", "a", "b"} {
		r.Latest = latest
		if r.HasUpdate() != (latest == "b") {
			t.Fatalf("HasUpdate(%q)", latest)
		}
		if err := subscribe.Write(dir, r); err != nil {
			t.Fatal(err)
		}
		got, err := subscribe.Read(dir)
		if err != nil || *got != *r {
			t.Fatalf("round trip: %+v %v", got, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, subscribe.File))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"kind", "url", "ref_type", "ref", "installed", "installed_at", "latest", "checked_at"} {
		if _, ok := fields[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	for _, k := range []string{"id", "repo", "workshop_id", "check_error"} {
		if _, ok := fields[k]; ok {
			t.Errorf("optional/removed field %s present", k)
		}
	}
	if err := subscribe.Write(dir, &subscribe.Record{Kind: "git", Repo: "repo"}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, subscribe.File))
	if !strings.Contains(string(b), `"repo": "repo"`) || strings.Contains(string(b), `"checked_at"`) {
		t.Fatalf("optional fields: %s", b)
	}
}

func TestGit(t *testing.T) {
	g := testkit.NewGitRepo(t, "")
	ctx := context.Background()
	second := g.Commit(t, "Defs/test.xml", "<Defs/>")
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(g.Bare)}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	for _, tc := range []struct{ typ, ref, url, want string }{
		{"branch", "main", g.Bare, second},
		{"branch", "", u.String(), second},
		{"tag", "v1", g.Bare, g.First},
		{"commit", g.First, g.Bare, g.First},
	} {
		t.Run(tc.typ+tc.ref, func(t *testing.T) {
			commit, err := subscribe.RemoteCommit(ctx, tc.url, tc.typ, tc.ref)
			if err != nil || commit != tc.want {
				t.Fatalf("remote: %s %v, want %s", commit, err, tc.want)
			}
			dir := filepath.Join(t.TempDir(), "clone")
			commit, err = subscribe.Clone(ctx, tc.url, tc.typ, tc.ref, dir)
			if err != nil || commit != tc.want {
				t.Fatalf("clone: %s %v", commit, err)
			}
			r := &subscribe.Record{Kind: "git", URL: tc.url, RefType: tc.typ, Ref: tc.ref, Installed: commit}
			if err := subscribe.Write(dir, r); err != nil {
				t.Fatal(err)
			}
			if dirty, err := subscribe.Dirty(dir); err != nil || dirty {
				t.Fatalf("metadata dirty: %v %v", dirty, err)
			}
			repo, err := git.PlainOpen(dir)
			if err != nil {
				t.Fatal(err)
			}
			shallow, err := repo.Storer.Shallow()
			if err != nil || (tc.typ == "branch" && len(shallow) == 0) || (tc.typ == "commit" && len(shallow) != 0) {
				t.Fatalf("shallow=%v err=%v", shallow, err)
			}
			if tc.typ == "branch" {
				if _, err := repo.CommitObject(plumbing.NewHash(g.First)); err == nil {
					t.Fatal("depth 1 clone fetched parent history")
				}
			}
			if b, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude")); err != nil || !strings.Contains(string(b), ".rimarcho/") {
				t.Fatalf("exclude: %q %v", b, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("player edits"), 0o644); err != nil {
				t.Fatal(err)
			}
			if dirty, err := subscribe.Dirty(dir); err != nil || dirty {
				t.Fatalf("untracked dirty: %v %v", dirty, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "About", "About.xml"), []byte("tracked edit"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := subscribe.Update(ctx, dir, r); err == nil || !strings.Contains(err.Error(), "dirty") {
				t.Fatalf("dirty update: %v", err)
			}
		})
	}
	// Moving an annotated tag must use the peeled commit on check and update.
	dir := filepath.Join(t.TempDir(), "tag")
	if _, err := subscribe.Clone(ctx, g.Bare, "tag", "v1", dir); err != nil {
		t.Fatal(err)
	}
	bare, err := git.PlainOpen(g.Bare)
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.Storer.SetReference(plumbing.NewHashReference(plumbing.NewTagReferenceName("v1"), plumbing.NewHash(second))); err != nil {
		t.Fatal(err)
	}
	if got, err := subscribe.Update(ctx, dir, &subscribe.Record{URL: g.Bare, RefType: "tag", Ref: "v1"}); err != nil || got != second {
		t.Fatalf("moved tag: %s %v", got, err)
	}
	if got, err := subscribe.RemoteCommit(ctx, g.Bare, "branch", "missing"); err == nil {
		t.Fatalf("missing branch: %s", got)
	}
}

func TestNormalizeRepo(t *testing.T) {
	for _, raw := range []string{"https://gitgud.io/Example/colony", "HTTPS://GITGUD.IO/Example/colony.git", "https://gitgud.io/Example/colony.git/", "https://gitgud.io/Example/colony/"} {
		if got := subscribe.NormalizeRepo(raw); got != "https://gitgud.io/Example/colony" {
			t.Errorf("normalize %s: %s", raw, got)
		}
	}
	dir := t.TempDir()
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(dir)}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	if subscribe.NormalizeRepo(dir) != subscribe.NormalizeRepo(u.String()) {
		t.Fatal("file URL and local path differ")
	}
	// The same folder through a junction (as with an 8.3 short name) is the same repository.
	if runtime.GOOS == "windows" {
		link := filepath.Join(t.TempDir(), "alias")
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, dir).CombinedOutput(); err != nil {
			t.Fatalf("mklink: %v %s", err, out)
		}
		if subscribe.NormalizeRepo(link) != subscribe.NormalizeRepo(dir) {
			t.Fatal("junction and target differ")
		}
	}
}

func TestUpdateTrackedRefAndUntracked(t *testing.T) {
	for _, refType := range []string{"branch", "default", "tag", "commit"} {
		t.Run(refType, func(t *testing.T) {
			g := testkit.NewGitRepo(t, "")
			ctx := context.Background()
			typ, ref := refType, "main"
			if typ == "default" {
				typ, ref = "branch", ""
			}
			if typ == "tag" {
				ref = "v1"
			}
			if typ == "commit" {
				ref = g.First
			}
			dir := filepath.Join(t.TempDir(), "clone")
			if _, err := subscribe.Clone(ctx, g.Bare, typ, ref, dir); err != nil {
				t.Fatal(err)
			}
			second := g.Commit(t, "Defs/test.xml", "<Defs/>")
			bare, err := git.PlainOpen(g.Bare)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []plumbing.ReferenceName{plumbing.NewBranchReferenceName("unrelated"), plumbing.NewTagReferenceName("unrelated")} {
				if err := bare.Storer.SetReference(plumbing.NewHashReference(name, plumbing.NewHash(second))); err != nil {
					t.Fatal(err)
				}
			}
			if typ == "tag" {
				if err := bare.Storer.SetReference(plumbing.NewHashReference(plumbing.NewTagReferenceName("v1"), plumbing.NewHash(second))); err != nil {
					t.Fatal(err)
				}
			}
			dds := filepath.Join(dir, "Textures", "generated.dds")
			if err := os.MkdirAll(filepath.Dir(dds), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dds, []byte("generated DDS"), 0o644); err != nil {
				t.Fatal(err)
			}
			if dirty, err := subscribe.Dirty(dir); err != nil || dirty {
				t.Fatalf("untracked DDS blocks update: %v %v", dirty, err)
			}
			want := second
			if typ == "commit" {
				want = g.First
			}
			if got, err := subscribe.Update(ctx, dir, &subscribe.Record{URL: "https://unused.invalid/page", Repo: g.Bare, RefType: typ, Ref: ref}); err != nil || got != want {
				t.Fatalf("update: %s %v", got, err)
			}
			if b, err := os.ReadFile(dds); err != nil || string(b) != "generated DDS" {
				t.Fatalf("untracked file lost: %q %v", b, err)
			}
			r, _ := git.PlainOpen(dir)
			if typ != "commit" {
				for _, name := range []plumbing.ReferenceName{plumbing.NewRemoteReferenceName("origin", "unrelated"), plumbing.NewTagReferenceName("unrelated")} {
					if _, err := r.Reference(name, true); err == nil {
						t.Errorf("fetched unrelated ref %s", name)
					}
				}
			}
		})
	}
}

func TestUpdatePreservesUntrackedCollision(t *testing.T) {
	g := testkit.NewGitRepo(t, "")
	dir := filepath.Join(t.TempDir(), "clone")
	ctx := context.Background()
	if _, err := subscribe.Clone(ctx, g.Bare, "branch", "main", dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "texture.dds")
	if err := os.WriteFile(path, []byte("local DDS"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.Commit(t, "texture.dds", "upstream DDS")
	if _, err := subscribe.Update(ctx, dir, &subscribe.Record{URL: g.Bare, RefType: "branch", Ref: "main"}); err == nil || !strings.Contains(err.Error(), "untracked") {
		t.Fatalf("collision not refused: %v", err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "local DDS" {
		t.Fatalf("local data changed: %q %v", b, err)
	}
	r, _ := git.PlainOpen(dir)
	h, _ := r.Head()
	if h.Hash().String() != g.First {
		t.Fatal("checkout advanced despite collision")
	}
}

func TestDirtyTrackedChanges(t *testing.T) {
	g := testkit.NewGitRepo(t, "")
	for _, change := range []string{"modified", "deleted", "staged", "added"} {
		t.Run(change, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "clone")
			if _, err := subscribe.Clone(context.Background(), g.Bare, "branch", "main", dir); err != nil {
				t.Fatal(err)
			}
			path := "About/About.xml"
			if change == "added" {
				path = "new.xml"
			}
			var err error
			if change == "deleted" {
				err = os.Remove(filepath.Join(dir, filepath.FromSlash(path)))
			} else {
				err = os.WriteFile(filepath.Join(dir, filepath.FromSlash(path)), []byte("local edit"), 0o644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if change == "staged" || change == "added" {
				r, _ := git.PlainOpen(dir)
				w, _ := r.Worktree()
				if _, err := w.Add(path); err != nil {
					t.Fatal(err)
				}
			}
			if dirty, err := subscribe.Dirty(dir); err != nil || !dirty {
				t.Fatalf("tracked %s not dirty: %v %v", change, dirty, err)
			}
		})
	}
}

func TestFolderNameAndValidation(t *testing.T) {
	for input, want := range map[string]string{"https://example.invalid/org/mod.git": "mod", `C:\repos\My Mod.git`: "My Mod", "file:///tmp/a%20b.git": "a b", "https://example.invalid/CON.git": "_CON", "https://example.invalid/.git": "repository"} {
		if got := subscribe.FolderName(input); got != want {
			t.Errorf("FolderName(%q)=%q want %q", input, got, want)
		}
	}
	for _, tc := range []struct{ url, typ, ref string }{{"ssh://example.invalid/mod", "branch", ""}, {"https://user:pass@example.invalid/mod", "branch", ""}, {"local", "tag", ""}, {"local", "commit", "abcd"}, {"local", "branch", "../bad"}, {"local", "other", "main"}} {
		if err := subscribe.Validate(tc.url, tc.typ, tc.ref); err == nil {
			t.Errorf("accepted %+v", tc)
		}
	}
}
