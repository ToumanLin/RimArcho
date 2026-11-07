package store_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// Enabling a list moves it to the front of the enable history, once (16 §2.1).
func TestPlansEnabledHistory(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var ids []int64
	for _, n := range []string{"A", "B", "C"} {
		p, err := s.CreatePlan("env", n, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	for _, i := range []int{0, 2, 1, 2} {
		if err := s.SetCurrentPlan("env", ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := s.PlansEnabled(), []int64{ids[2], ids[1], ids[0]}; !slices.Equal(got, want) {
		t.Fatalf("history %v, want %v", got, want)
	}
}
