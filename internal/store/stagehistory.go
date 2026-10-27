package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/loadout"
)

const schemaV21 = `ALTER TABLE workspace_undo ADD COLUMN staging TEXT;`

// lockHistory reserves the SQLite writer before reading history/staging. In WAL
// mode a deferred read transaction cannot upgrade after a background analysis
// commits; busy_timeout only helps when the write reservation is acquired first.
func lockHistory(tx *sql.Tx) error {
	_, err := tx.Exec(`UPDATE meta SET value=value WHERE key='schema_version'`)
	return err
}

func stageMeta(tx *sql.Tx, plan int64) (string, error) {
	var raw string
	err := tx.QueryRow(`SELECT value FROM meta WHERE key=?`, StageKey(plan)).Scan(&raw)
	if err == sql.ErrNoRows {
		err = nil
	}
	return raw, err
}

func setStage(tx *sql.Tx, plan int64, raw string) error {
	if raw == "" {
		_, err := tx.Exec(`DELETE FROM meta WHERE key=?`, StageKey(plan))
		return err
	}
	_, err := tx.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, StageKey(plan), raw)
	return err
}

// pruneStage preserves the raw representation when membership has not changed,
// including absent/null/empty lists. An enabled item must not survive invisibly.
func pruneStage(raw, entries string) (string, error) {
	if raw == "" {
		return raw, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return "", fmt.Errorf("staging list unreadable: %w", err)
	}
	list := loadout.Decode(entries)
	keep := items[:0]
	for _, it := range items {
		var id string
		if err := json.Unmarshal(it["i"], &id); err != nil {
			return "", err
		}
		if list.Index(id) < 0 {
			keep = append(keep, it)
		}
	}
	if len(keep) == len(items) {
		return raw, nil
	}
	b, err := json.Marshal(keep)
	return string(b), err
}

// EditWorkspace commits entries, staging, the undo snapshot and redo invalidation
// together. staged=nil retains staging except for items enabled by this edit.
func (s *Store) EditWorkspace(w *Workspace, before, op string, staged *string) error {
	return s.inTx(func(tx *sql.Tx) error {
		if err := lockHistory(tx); err != nil {
			return err
		}
		raw, err := stageMeta(tx, w.PlanID)
		if err != nil {
			return err
		}
		previous, err := pruneStage(raw, before)
		if err != nil {
			return err
		}
		after := previous
		if staged != nil {
			after = *staged
		}
		after, err = pruneStage(after, w.Entries)
		if err != nil {
			return err
		}
		if before != w.Entries || previous != after {
			if err := pushStep(tx, w.PlanID, "undo", before, op, previous); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM workspace_undo WHERE plan_id=? AND stack='redo'`, w.PlanID); err != nil {
				return err
			}
		}
		if err := setWorkspace(tx, w); err != nil {
			return err
		}
		return setStage(tx, w.PlanID, after)
	})
}

// RestoreStep swaps both workspace payloads and the history stacks atomically.
// Legacy steps retain unrelated staging, pruning enabled memberships in both
// the current and target states so undo cannot preserve a hidden ghost item.
func (s *Store) RestoreStep(w *Workspace, before, from, to string, step Step) error {
	return s.inTx(func(tx *sql.Tx) error {
		if err := lockHistory(tx); err != nil {
			return err
		}
		var newest int64
		if err := tx.QueryRow(`SELECT id FROM workspace_undo WHERE plan_id=? AND stack=? ORDER BY id DESC LIMIT 1`, w.PlanID, from).Scan(&newest); err != nil {
			return err
		}
		if newest != step.ID {
			return fmt.Errorf("history changed; reload and try again")
		}
		raw, err := stageMeta(tx, w.PlanID)
		if err != nil {
			return err
		}
		previous, err := pruneStage(raw, before)
		if err != nil {
			return err
		}
		after := previous
		if step.Staging != nil {
			after = *step.Staging
		}
		after, err = pruneStage(after, w.Entries)
		if err != nil {
			return err
		}
		if err := pushStep(tx, w.PlanID, to, before, step.Op, previous); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM workspace_undo WHERE id=?`, step.ID); err != nil {
			return err
		}
		if err := setWorkspace(tx, w); err != nil {
			return err
		}
		return setStage(tx, w.PlanID, after)
	})
}

// FinishBatch collapses preview steps or restores the pre-session payload.
func (s *Store) FinishBatch(w *Workspace, last int64, entries, staging, op string, revert bool) error {
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM workspace_undo WHERE plan_id=? AND stack='undo' AND id>?`, w.PlanID, last); err != nil {
			return err
		}
		if revert {
			w.Entries = entries
			if err := setWorkspace(tx, w); err != nil {
				return err
			}
			return setStage(tx, w.PlanID, staging)
		}
		current, err := stageMeta(tx, w.PlanID)
		if err != nil {
			return err
		}
		if w.Entries == entries && current == staging {
			return nil
		}
		return pushStep(tx, w.PlanID, "undo", entries, op, staging)
	})
}

// remapHistoryStages follows physical-copy identity changes in restorable staging
// items and anchors, just as the entries payload follows them.
func remapHistoryStages(tx *sql.Tx, mapping map[string]string, plan *int64) error {
	query := `SELECT id, staging FROM workspace_undo WHERE staging IS NOT NULL AND staging<>''`
	var args []any
	if plan != nil {
		query += ` AND plan_id=?`
		args = append(args, *plan)
	}
	type row struct {
		id  int64
		raw string
	}
	var all []row
	if err := eachRow(tx, query, func(r *sql.Rows) error {
		var x row
		if err := r.Scan(&x.id, &x.raw); err != nil {
			return err
		}
		all = append(all, x)
		return nil
	}, args...); err != nil {
		return err
	}
	for _, x := range all {
		var items []map[string]any
		if err := json.Unmarshal([]byte(x.raw), &items); err != nil {
			return err
		}
		changed := false
		seen := map[string]bool{}
		out := items[:0]
		for _, item := range items {
			for _, field := range []string{"i", "a"} {
				id, _ := item[field].(string)
				if target := mapping[id]; target != "" {
					item[field] = target
					changed = true
				}
			}
			id, _ := item["i"].(string)
			if seen[id] {
				changed = true
				continue
			}
			seen[id] = true
			out = append(out, item)
		}
		if changed {
			raw, err := json.Marshal(out)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE workspace_undo SET staging=? WHERE id=?`, string(raw), x.id); err != nil {
				return err
			}
		}
	}
	return nil
}
