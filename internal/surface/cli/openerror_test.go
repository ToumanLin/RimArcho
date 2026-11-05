package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// RimArcho.exe has no console: an open failure shows a message box in the player's language,
// with its own text for a pre-1.0 database; a console program only writes stderr (04 §4).
func TestOpenFailureMessageBox(t *testing.T) {
	old := showOpenError
	defer func() { showOpenError = old; view.SetLang("en") }()
	var title, text string
	calls := 0
	showOpenError = func(ti, te string) { title, text = ti, te; calls++ }

	home := t.TempDir()
	db := filepath.Join(home, "archo.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("schema_version", fmt.Sprint(store.FirstReleaseSchema-1)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	_, openErr := app.Open(home)
	if !errors.Is(openErr, store.ErrPreRelease) {
		t.Fatalf("open: %v", openErr)
	}

	(&upgradeReport{dbPath: db}).openFailed(openErr)
	if calls != 0 {
		t.Fatal("a console program showed a message box")
	}
	(&upgradeReport{window: true, dbPath: db}).openFailed(openErr)
	if calls != 1 || title != "RimArcho" || !strings.Contains(text, db) || !strings.Contains(text, "before 1.0") || !strings.Contains(text, "Move or rename") {
		t.Fatalf("pre-1.0: %d %q %q", calls, title, text)
	}
	other := errors.New("store: unsupported schema version 99 (want 29)")
	(&upgradeReport{window: true, dbPath: db}).openFailed(other)
	if !strings.Contains(text, "could not open its database") || !strings.Contains(text, other.Error()) {
		t.Fatalf("other error: %q", text)
	}

	s, err = store.OpenSession(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("cfg:ui.lang", "zh"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	(&upgradeReport{window: true, dbPath: db}).openFailed(openErr)
	if strings.Contains(text, "Move or rename") || !strings.Contains(text, db) {
		t.Fatalf("zh: %q", text)
	}
}
