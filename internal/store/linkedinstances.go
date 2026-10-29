package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// MergeLinkedInstances repairs local records of shared directory links. References
// in every plan, version, undo step and staged list must follow the shared identity,
// including references to local records already tombstoned by a later full scan.
func (s *Store) MergeLinkedInstances(replace map[string]string) error {
	if len(replace) == 0 {
		return nil
	}
	return s.inTx(func(tx *sql.Tx) error {
		// Readers have their own connections. Acquire the write lock before reading
		// records so simultaneous repairs cannot upgrade stale read snapshots.
		if _, err := tx.Exec(`UPDATE meta SET value=value WHERE key='schema_version'`); err != nil {
			return err
		}
		pending := map[string]string{}
		lineages := map[string]string{}
		for old, keep := range replace {
			var oldReal, newReal, oldLineage, newLineage, oldKind, newEnv string
			if err := tx.QueryRow(`SELECT o.real_path, n.real_path, o.lineage_id, n.lineage_id, os.kind, ns.environment_id
				FROM instance o JOIN instance n ON n.id=? JOIN source os ON os.id=o.source_id JOIN source ns ON ns.id=n.source_id WHERE o.id=?`, keep, old).
				Scan(&oldReal, &newReal, &oldLineage, &newLineage, &oldKind, &newEnv); err != nil {
				if err == sql.ErrNoRows {
					var target string
					if err := tx.QueryRow(`SELECT new_id FROM id_alias WHERE old_id=? AND kind='instance'`, old).Scan(&target); err == nil && target == keep {
						continue // another reader already repaired this record
					}
				}
				return err
			}
			if old == keep || oldKind != "local" || newEnv != Shared || oldReal == "" || env.PathKey(oldReal) != env.PathKey(newReal) {
				return fmt.Errorf("%s is not a local link to shared instance %s", old, keep)
			}
			if oldLineage != newLineage {
				lineages[oldLineage] = newLineage
			}
			pending[old] = keep
			if _, err := tx.Exec(`INSERT OR IGNORE INTO instance_alias(instance_id, source_id, rel_path, real_path)
				SELECT ?, source_id, rel_path, real_path FROM instance WHERE id=?`, keep, old); err != nil {
				return err
			}
			for _, q := range []string{
				`UPDATE texture_file SET instance_id=?2 WHERE instance_id=?1`,
				`INSERT OR IGNORE INTO texture_mod(instance_id,snap_hash,scale,mips,files,bytes,made_at)
				 SELECT ?2,snap_hash,scale,mips,files,bytes,made_at FROM texture_mod WHERE instance_id=?1`,
				`DELETE FROM texture_mod WHERE instance_id=?1`,
				`UPDATE id_alias SET new_id=?2 WHERE new_id=?1 AND kind='instance'`,
			} {
				if _, err := tx.Exec(q, old, keep); err != nil {
					return err
				}
			}
		}
		if len(pending) == 0 {
			return nil
		}
		// Use the same reference rewriting as the shared-source migration, so history
		// and provenance cannot resurrect the obsolete local IDs.
		if err := remapHistoryStages(tx, pending, nil); err != nil {
			return err
		}
		if err := rewriteInstances14(tx, pending); err != nil {
			return err
		}
		if err := rewriteOrigins14(tx, pending, lineages); err != nil {
			return err
		}
		if len(lineages) > 0 {
			if err := rewriteLineages14(tx, lineages); err != nil {
				return err
			}
			if err := mergePlayerData14(tx, lineages); err != nil {
				return err
			}
			if err := mergeRules14(tx, lineages); err != nil {
				return err
			}
		}
		for _, q := range []string{`DELETE FROM patch_fact`, `DELETE FROM finding`, `DELETE FROM analysis`, `DELETE FROM log_cache`} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		if err := pruneFacts(tx, time.Now()); err != nil {
			return err
		}
		return recomputeAllPrimaries(tx)
	})
}
