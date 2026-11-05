package store

import (
	"path/filepath"
	"strings"
	"testing"
)

// The batch lookups of save attribution (10 §4.6) start from the name index, then join the
// version view, as DefsNamed does (03 §3).
func TestSaveOwnerLookupsStartFromName(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, c := range []struct{ q, outer, inner string }{
		{defDefinersQ(2), "SEARCH f USING INDEX def_fact_lookup (def_name=?)", "SEARCH c USING INDEX content_state_def (def_set_id=? AND game_mm=?)"},
		{typeDefinersQ(2), "SEARCH f USING COVERING INDEX type_fact_lookup (type_name=?)", "SEARCH c USING INDEX content_state_dll (dll_set_id=? AND game_mm=?)"},
	} {
		rows, err := s.DB.Query(`EXPLAIN QUERY PLAN `+c.q, "a", "b", "1.6")
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			rows.Scan(&id, &parent, &unused, &detail)
			plan = append(plan, detail)
		}
		rows.Close()
		if len(plan) < 2 || plan[0] != c.outer || !strings.HasPrefix(plan[1], c.inner) {
			t.Errorf("%s:\n%s", c.q, strings.Join(plan, "\n"))
		}
	}
}
