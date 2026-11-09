package store_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestUpgradeDefContentKeepsFacts(t *testing.T) {
	s, path := openTemp(t)
	for _, q := range []string{
		`ALTER TABLE def_fact DROP COLUMN content_fields`,
		`ALTER TABLE fact_set DROP COLUMN def_content_version`,
		`INSERT INTO fact_set(id,instance_id,kind,content_key,files) VALUES(1,'old','def','old-key','["Defs/Old.xml"]'),(2,'old','dll','dll-key','["Assemblies/Old.dll"]')`,
		`INSERT INTO content_state(instance_id,game_mm,content_key,def_set_id,dll_set_id) VALUES('old','1.6','old-key',1,2)`,
		`INSERT INTO def_fact(set_id,def_type,def_name,file_ix,line) VALUES(1,'ThingDef','Old',0,1)`,
		`INSERT INTO type_fact(set_id,file_ix,type_name) VALUES(2,0,'Old.Type')`,
		`UPDATE meta SET value='34' WHERE key='schema_version'`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	u, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	u.MM = "1.6"
	if u.Meta("schema_version") != "35" {
		t.Fatal("not migrated")
	}
	defs, err := u.DefDefiners([]string{"Old"})
	if err != nil || len(defs["Old"]) != 1 {
		t.Fatalf("lost Def: %v %v", defs, err)
	}
	types, err := u.TypeDefiners([]string{"Old.Type"})
	if err != nil || len(types["Old.Type"]) != 1 {
		t.Fatalf("lost DLL: %v %v", types, err)
	}
	ready, err := u.ContentReady([]string{"old"})
	if err != nil || ready["old"] {
		t.Fatalf("old facts incorrectly ready: %v %v", ready, err)
	}
	fs, ok := u.CachedDefs("old", "old-key")
	if !ok || len(fs) != 1 || fs[0].Content != nil {
		t.Fatalf("old cache unusable: %v %v", fs, ok)
	}
}
