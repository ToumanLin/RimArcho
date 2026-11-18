package store_test

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestUpgradeCapturedLaunchAndCreditRoots(t *testing.T) {
	s, path := openTemp(t)
	e := &env.Environment{ID: "environment", Name: "before", GameRoot: filepath.Join(t.TempDir(), "game"), UserDataRoot: filepath.Join(t.TempDir(), "data"), CreatedAt: time.Unix(1, 0)}
	if err := s.InsertEnv(e); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLaunchRecord(&store.LaunchRecord{EnvID: e.ID, Started: time.Unix(2, 0), Method: "direct", Command: "unchanged"}); err != nil {
		t.Fatal(err)
	}
	batch, err := s.CreateCreditBatch(time.Unix(3, 0), []store.CreditItem{{WorkshopID: "8100", InstanceID: "synthetic", Name: "Synthetic"}})
	if err != nil {
		t.Fatal(err)
	}
	item := store.CreditItem{BatchID: batch, WorkshopID: "8100"}
	// Model a historical pending obligation, without new context columns.
	for _, q := range []string{
		`ALTER TABLE launch DROP COLUMN game_id`, `ALTER TABLE launch DROP COLUMN user_data_root`, `ALTER TABLE launch DROP COLUMN env_name`,
		`DROP TABLE credit_deployment`,
		`CREATE TABLE credit_deployment(batch_id INTEGER NOT NULL,workshop_id TEXT NOT NULL,environment_id TEXT NOT NULL,plan_id INTEGER NOT NULL,done INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(batch_id,workshop_id,environment_id),FOREIGN KEY(batch_id,workshop_id) REFERENCES credit_item(batch_id,workshop_id))`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec(`INSERT INTO credit_deployment VALUES(?,?,?,?,0)`, batch, "8100", e.ID, 17); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("schema_version", "38"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	u, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	launches, err := u.Launches(e.ID, 1)
	if err != nil || len(launches) != 1 || launches[0].GameID != e.GameID || launches[0].EnvName != e.Name || launches[0].UserDataRoot != "" || launches[0].Command != "unchanged" {
		t.Fatalf("legacy launch: %+v %v", launches, err)
	}
	ds, err := u.CreditDeployments(item)
	if err != nil || len(ds) != 1 || ds[0].GameID != e.GameID || ds[0].UserDataRoot != e.UserDataRoot || ds[0].EnvName != e.Name || ds[0].PlanID != 17 {
		t.Fatalf("deployment snapshot: %+v %v", ds, err)
	}
	e.UserDataRoot = filepath.Join(t.TempDir(), "new-root")
	e.Name = "after"
	if err := u.UpdateEnv(e); err != nil {
		t.Fatal(err)
	}
	after, err := u.CreditDeployments(item)
	if err != nil || !reflect.DeepEqual(ds, after) {
		t.Fatalf("root edited historical obligation: %+v %v", after, err)
	}
	fresh, _ := openTemp(t)
	if !reflect.DeepEqual(schemaShape(t, u), schemaShape(t, fresh)) {
		t.Fatal("fresh/upgraded schema differs")
	}
	var foreign int
	if err := u.DB.QueryRow(`SELECT count(*) FROM pragma_foreign_key_check`).Scan(&foreign); err != nil || foreign != 0 {
		t.Fatalf("foreign keys: %d %v", foreign, err)
	}
}
