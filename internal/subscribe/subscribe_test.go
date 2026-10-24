package subscribe_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
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
	if len(fields) != 11 {
		t.Fatalf("JSON fields: %s", b)
	}
	for _, k := range []string{"kind", "url", "id", "workshop_id", "ref_type", "ref", "installed", "installed_at", "latest", "checked_at", "check_error"} {
		if _, ok := fields[k]; !ok {
			t.Errorf("missing %s", k)
		}
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
			if dirty, err := subscribe.Dirty(dir); err != nil || !dirty {
				t.Fatalf("untracked not dirty: %v %v", dirty, err)
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
