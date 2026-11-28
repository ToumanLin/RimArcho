package store_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// TestNewerSchema: a database a newer RimArcho wrote is refused with a message that says so
// and names both schemas, not "unsupported schema version" (04 §4).
func TestNewerSchema(t *testing.T) {
	db := filepath.Join(t.TempDir(), "archo.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("schema_version", fmt.Sprint(store.SchemaVersion+8)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	_, err = store.Open(db)
	var newer *store.ErrNewerSchema
	want := fmt.Sprintf("this data was written by a newer RimArcho (schema %d); this ", store.SchemaVersion+8)
	if !errors.As(err, &newer) || !strings.HasPrefix(err.Error(), want) ||
		!strings.Contains(err.Error(), fmt.Sprintf(" supports schema %d at most, so update it", store.SchemaVersion)) {
		t.Fatalf("open: %v", err)
	}
}
