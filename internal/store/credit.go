package store

import (
	"database/sql"
	"fmt"
	"time"
)

const schemaV20 = `
CREATE TABLE credit_batch(
 id INTEGER PRIMARY KEY, created_at INTEGER NOT NULL,
 state TEXT NOT NULL DEFAULT 'counting' CHECK(state IN ('counting','prompt','replacing','done','dismissed')),
 sessions INTEGER NOT NULL DEFAULT 0);
CREATE TABLE credit_item(
 batch_id INTEGER NOT NULL REFERENCES credit_batch(id), workshop_id TEXT NOT NULL,
 instance_id TEXT NOT NULL, name TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'pending',
 target_id TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt INTEGER NOT NULL DEFAULT 0, result TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(batch_id, workshop_id));
CREATE TABLE credit_session(
 batch_id INTEGER NOT NULL REFERENCES credit_batch(id), launch_id INTEGER NOT NULL,
 environment_id TEXT NOT NULL, started_at INTEGER NOT NULL,
 PRIMARY KEY(batch_id, environment_id, launch_id, started_at));
CREATE TABLE credit_deployment(
 batch_id INTEGER NOT NULL, workshop_id TEXT NOT NULL, environment_id TEXT NOT NULL,
 plan_id INTEGER NOT NULL, done INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(batch_id, workshop_id, environment_id),
 FOREIGN KEY(batch_id, workshop_id) REFERENCES credit_item(batch_id, workshop_id));
`

// CreditBatch is one explicit bulk SteamCMD download, independent of task history.
type CreditBatch struct {
	ID       int64  `json:"id"`
	Created  int64  `json:"createdAt"`
	State    string `json:"state"`
	Sessions int    `json:"sessions"`
}

type CreditItem struct {
	BatchID     int64  `json:"batchID"`
	WorkshopID  string `json:"workshopID"`
	InstanceID  string `json:"instanceID"`
	Name        string `json:"name"`
	State       string `json:"state"` // pending, subscribed, switching, done, retained, failed
	TargetID    string `json:"targetID"`
	Attempts    int    `json:"attempts"`
	NextAttempt int64  `json:"nextAttempt"`
	Result      string `json:"result"`
}

func (i CreditItem) Terminal() bool {
	return i.State == "done" || i.State == "retained" || i.State == "failed"
}

func (s *Store) CreateCreditBatch(at time.Time, items []CreditItem) (int64, error) {
	if len(items) == 0 {
		return 0, nil
	}
	var id int64
	err := s.inTx(func(tx *sql.Tx) error {
		r, err := tx.Exec(`INSERT INTO credit_batch(created_at) VALUES(?)`, at.Unix())
		if err != nil {
			return err
		}
		id, err = r.LastInsertId()
		if err != nil {
			return err
		}
		for _, it := range items {
			if _, err := tx.Exec(`INSERT INTO credit_item(batch_id,workshop_id,instance_id,name) VALUES(?,?,?,?)`, id, it.WorkshopID, it.InstanceID, it.Name); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}

func (s *Store) CreditBatches(state string) ([]CreditBatch, error) {
	rows, err := s.DB.Query(`SELECT id,created_at,state,sessions FROM credit_batch WHERE (?='' OR state=?) ORDER BY id`, state, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CreditBatch{}
	for rows.Next() {
		var b CreditBatch
		if err := rows.Scan(&b.ID, &b.Created, &b.State, &b.Sessions); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) CreditItems(batchID int64) ([]CreditItem, error) {
	rows, err := s.DB.Query(`SELECT batch_id,workshop_id,instance_id,name,state,target_id,attempts,next_attempt,result FROM credit_item WHERE batch_id=? ORDER BY workshop_id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CreditItem{}
	for rows.Next() {
		var i CreditItem
		if err := rows.Scan(&i.BatchID, &i.WorkshopID, &i.InstanceID, &i.Name, &i.State, &i.TargetID, &i.Attempts, &i.NextAttempt, &i.Result); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) SaveCreditItem(i CreditItem) error {
	_, err := s.DB.Exec(`UPDATE credit_item SET state=?,target_id=?,attempts=?,next_attempt=?,result=? WHERE batch_id=? AND workshop_id=?`, i.State, i.TargetID, i.Attempts, i.NextAttempt, i.Result, i.BatchID, i.WorkshopID)
	return err
}

// CountCreditSession is atomic and idempotent, even across connections. Call only with
// the exact SteamCMD instances enabled by a successfully started game.
func (s *Store) CountCreditSession(launchID int64, instances []string) error {
	return s.inTx(func(tx *sql.Tx) error {
		var counting bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM credit_batch WHERE state='counting')`).Scan(&counting); err != nil {
			return err
		}
		if !counting {
			return nil
		}
		for _, id := range instances {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO credit_session(batch_id,launch_id,environment_id,started_at)
    SELECT b.id,l.id,l.environment_id,l.started_at FROM credit_batch b JOIN credit_item i ON i.batch_id=b.id
    JOIN launch l ON l.id=?
    JOIN instance m ON m.id=i.instance_id JOIN source src ON src.id=m.source_id
    WHERE b.state='counting' AND i.instance_id=? AND src.kind='steamcmd' AND src.status='ok' AND m.status='live'`, launchID, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`UPDATE credit_batch SET sessions=(SELECT count(*) FROM credit_session s WHERE s.batch_id=credit_batch.id),
   state=CASE WHEN (SELECT count(*) FROM credit_session s WHERE s.batch_id=credit_batch.id)>=3 THEN 'prompt' ELSE 'counting' END WHERE state='counting'`)
		return err
	})
}

// AnswerCredit validates the whole answer before changing anything. Repeated answers
// are idempotent; a dismissal can never be reversed by a stale browser response.
func (s *Store) AnswerCredit(ids []int64, replace bool) error {
	if len(ids) == 0 {
		return fmt.Errorf("choose at least one credit batch")
	}
	state := "dismissed"
	if replace {
		state = "replacing"
	}
	return s.inTx(func(tx *sql.Tx) error {
		for _, id := range ids {
			var current string
			if err := tx.QueryRow(`SELECT state FROM credit_batch WHERE id=?`, id).Scan(&current); err != nil {
				return fmt.Errorf("credit batch %d: %w", id, err)
			}
			if current != "prompt" && current != state && !(replace && current == "done") {
				return fmt.Errorf("credit batch %d cannot answer in state %s", id, current)
			}
		}
		for _, id := range ids {
			if _, err := tx.Exec(`UPDATE credit_batch SET state=? WHERE id=? AND state='prompt'`, state, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) FinishCreditBatches() error {
	_, err := s.DB.Exec(`UPDATE credit_batch SET state='done' WHERE state='replacing' AND NOT EXISTS
  (SELECT 1 FROM credit_item i WHERE i.batch_id=credit_batch.id AND i.state NOT IN ('done','retained','failed'))`)
	return err
}

type CreditDeployment struct {
	EnvID  string
	PlanID int64
}

func (s *Store) AddCreditDeployment(i CreditItem, envID string, planID int64) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO credit_deployment(batch_id,workshop_id,environment_id,plan_id) VALUES(?,?,?,?)`, i.BatchID, i.WorkshopID, envID, planID)
	return err
}
func (s *Store) CreditDeployments(i CreditItem) ([]CreditDeployment, error) {
	rows, err := s.DB.Query(`SELECT environment_id,plan_id FROM credit_deployment WHERE batch_id=? AND workshop_id=? AND done=0 ORDER BY environment_id`, i.BatchID, i.WorkshopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CreditDeployment
	for rows.Next() {
		var d CreditDeployment
		if err := rows.Scan(&d.EnvID, &d.PlanID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Store) FinishCreditDeployment(i CreditItem, envID string) error {
	_, err := s.DB.Exec(`UPDATE credit_deployment SET done=1 WHERE batch_id=? AND workshop_id=? AND environment_id=?`, i.BatchID, i.WorkshopID, envID)
	return err
}
