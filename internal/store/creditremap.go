package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// RemapCreditPlans moves the logical item in all editable/restorable list states
// together, merging copy notes and invalidating long-lived editor snapshots.
// Physical texture ledgers, fixed packs and deployment journals retain their IDs.
// No undo step is added: undo must never resurrect the retired physical copy.
func (s *Store) RemapCreditPlans(plans []int64, old, target string, now time.Time) error {
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE meta SET value=value WHERE key='schema_version'`); err != nil {
			return err
		}
		var same bool
		if err := tx.QueryRow(`SELECT o.lineage_id=n.lineage_id AND os.kind='steamcmd' AND ns.kind='workshop'
   FROM instance o JOIN instance n ON n.id=? JOIN source os ON os.id=o.source_id JOIN source ns ON ns.id=n.source_id WHERE o.id=?`, target, old).Scan(&same); err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("credit replacement identity changed")
		}
		for _, plan := range plans {
			if err := remapHistoryStages(tx, map[string]string{old: target}, &plan); err != nil {
				return err
			}
			changed := false
			for _, table := range []string{"workspace", "workspace_undo", "loadout_version"} {
				type row struct {
					id  int64
					raw string
				}
				var rows []row
				key := "id"
				if table == "workspace" {
					key = "plan_id"
				}
				if err := eachRow(tx, `SELECT `+key+`,entries FROM `+table+` WHERE plan_id=?`, func(r *sql.Rows) error {
					var x row
					if err := r.Scan(&x.id, &x.raw); err != nil {
						return err
					}
					rows = append(rows, x)
					return nil
				}, plan); err != nil {
					return err
				}
				for _, r := range rows {
					var list loadout.List
					if err := json.Unmarshal([]byte(r.raw), &list); err != nil {
						return fmt.Errorf("credit %s: %w", table, err)
					}
					if list.Index(old) < 0 {
						continue
					}
					out := loadout.List{}
					for _, en := range list {
						if en.Instance == target {
							continue
						}
						if en.Instance == old {
							en.Instance = target
						}
						out = append(out, en)
					}
					changed = true
					if _, err := tx.Exec(`UPDATE `+table+` SET entries=? WHERE `+key+`=?`, out.Encode(), r.id); err != nil {
						return err
					}
					if table == "loadout_version" {
						if _, err := tx.Exec(`UPDATE launch SET fingerprint=? WHERE plan_id=? AND fingerprint=?`, EntriesFingerprint(out.Encode()), plan, EntriesFingerprint(r.raw)); err != nil {
							return err
						}
					}
				}
			}
			var raw string
			err := tx.QueryRow(`SELECT value FROM meta WHERE key=?`, StageKey(plan)).Scan(&raw)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if raw != "" {
				var staged []map[string]any
				if err := json.Unmarshal([]byte(raw), &staged); err != nil {
					return err
				}
				stageChanged := false
				for _, en := range staged {
					for _, field := range []string{"i", "a"} {
						if en[field] == old {
							en[field] = target
							stageChanged = true
						}
					}
				}
				if stageChanged {
					changed = true
					b, err := json.Marshal(staged)
					if err != nil {
						return err
					}
					if _, err := tx.Exec(`UPDATE meta SET value=? WHERE key=?`, string(b), StageKey(plan)); err != nil {
						return err
					}
				}
			}
			if changed {
				if _, err := tx.Exec(`UPDATE plan SET updated_at=max(updated_at,?) WHERE id=?`, now.Unix(), plan); err != nil {
					return err
				}
			}
		}
		return mergeCopyNote(tx, old, target)
	})
}

// CreditPeer finds durable subscription confirmation or permanent failure shared
// by overlapping batches.
// A physical replacement is reusable only when InstanceID also matches.
func (s *Store) CreditPeer(it CreditItem) (*CreditItem, error) {
	var p CreditItem
	err := s.DB.QueryRow(`SELECT instance_id,state,target_id,result FROM credit_item
  WHERE workshop_id=? AND batch_id<>? AND state IN ('subscribed','switching','done','retained','failed')
  ORDER BY CASE WHEN instance_id=? AND state='done' THEN 0 WHEN state='failed' THEN 2 ELSE 1 END,batch_id LIMIT 1`, it.WorkshopID, it.BatchID, it.InstanceID).
		Scan(&p.InstanceID, &p.State, &p.TargetID, &p.Result)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}
