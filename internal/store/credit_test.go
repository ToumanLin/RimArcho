package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateV19Credit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`ALTER TABLE workspace_undo DROP COLUMN staging; DROP TABLE credit_deployment; DROP TABLE credit_session; DROP TABLE credit_item; DROP TABLE credit_batch; UPDATE meta SET value='19' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("keeper", "preserved"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Meta("schema_version") != fmt.Sprint(SchemaVersion) || s.Meta("keeper") != "preserved" {
		t.Fatal("migration lost state")
	}
	empty, err := s.CreateCreditBatch(time.Now(), nil)
	if err != nil || empty != 0 {
		t.Fatalf("empty batch %d %v", empty, err)
	}
	id, err := s.CreateCreditBatch(time.Now(), []CreditItem{{WorkshopID: "8100", InstanceID: "copy", Name: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE credit_batch SET state='prompt' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateCreditBatch(time.Now(), []CreditItem{{WorkshopID: "8200", InstanceID: "copy2", Name: "test2"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE credit_batch SET state='prompt' WHERE id=?`, second); err != nil {
		t.Fatal(err)
	}
	if err := s.AnswerCredit([]int64{id}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.AnswerCredit([]int64{second, id}, true); err == nil {
		t.Fatal("dismissed batch reversed")
	}
	bs, err := s.CreditBatches("prompt")
	if err != nil || len(bs) != 1 || bs[0].ID != second {
		t.Fatalf("partial answer committed %+v %v", bs, err)
	}
}
