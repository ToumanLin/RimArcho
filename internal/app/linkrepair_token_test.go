package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

func TestRememberLinkRepairReusesOccupiedEntries(t *testing.T) {
	for _, kind := range []string{"file", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			a, err := Open(filepath.Join(root, "home"))
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			mods := filepath.Join(root, "game", "Mods")
			if err := os.MkdirAll(mods, 0755); err != nil {
				t.Fatal(err)
			}
			if err := a.Store.InsertEnv(&env.Environment{ID: "main", Name: "main", GameRoot: filepath.Dir(mods)}); err != nil {
				t.Fatal(err)
			}
			occupied := filepath.Join(mods, "_Archo_occupied")
			if kind == "file" {
				err = os.WriteFile(occupied, []byte("keep"), 0644)
			} else {
				err = os.Mkdir(occupied, 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(mods, "_Archo_old")
			if err := source.CreateLink(link, filepath.Join(root, "missing")); err != nil {
				t.Fatal(err)
			}
			remember := func() string {
				t.Helper()
				p, err := a.PlanLinkRepair(true, []string{link})
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Links) != 2 {
					t.Fatalf("links: %+v", p.Links)
				}
				for _, l := range p.Links {
					if filepath.Base(l.Path) == "_Archo_occupied" && (l.Kind != "plain" || l.Code != CodeLinkOccupied || l.Action != "keep") {
						t.Fatalf("occupied: %+v", l)
					}
					if filepath.Base(l.Path) == "_Archo_old" && l.Action != "remove link" {
						t.Fatalf("link: %+v", l)
					}
				}
				return a.RememberLinkRepair(p)
			}
			token := remember()
			for i := 0; i < 10; i++ {
				if got := remember(); got != token {
					t.Fatalf("unchanged plan got token %s, want %s", got, token)
				}
			}
			if len(a.linkRepairPreviews) != 1 {
				t.Fatalf("retained %d tokens", len(a.linkRepairPreviews))
			}
			// An actual removable link still needs its original identity and target.
			if err := os.Rename(link, filepath.Join(root, "old-link")); err != nil {
				t.Fatal(err)
			}
			if err := source.CreateLink(link, filepath.Join(root, "other-missing")); err != nil {
				t.Fatal(err)
			}
			changed := remember()
			if changed == token {
				t.Fatal("changed removable link reused token")
			}
			if err := os.Rename(link, filepath.Join(root, "retargeted-link")); err != nil {
				t.Fatal(err)
			}
			if err := source.CreateLink(link, filepath.Join(root, "other-missing")); err != nil {
				t.Fatal(err)
			}
			replaced := remember()
			if replaced == changed {
				t.Fatal("same-target replacement reused token")
			}

			// A formerly occupied path becoming a link must not reuse the keep-only token.
			if err := os.Rename(occupied, filepath.Join(root, "kept-entry")); err != nil {
				t.Fatal(err)
			}
			if err := source.CreateLink(occupied, filepath.Join(root, "missing-occupied")); err != nil {
				t.Fatal(err)
			}
			p, err := a.PlanLinkRepair(true, []string{link, occupied})
			if err != nil {
				t.Fatal(err)
			}
			if got := a.RememberLinkRepair(p); got == replaced {
				t.Fatal("occupied-to-link transition reused token")
			}
		})
	}
}
