package store

import (
	"database/sql"
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
)

// QuestionSide is recorded metadata plus the current folder, loaded only on demand.
type QuestionSide struct {
	PackageID, Name, Authors, WorkshopID, Folder, SourceID string
	Files                                                  scan.SemanticSet
}

type QuestionEvidence struct {
	New, Candidate *QuestionSide
	Files          identity.FileEvidence
}

type questionReader interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

func questionSide(db questionReader, id string, snapshot int64) (*QuestionSide, error) {
	v := &QuestionSide{}
	var snapID int64
	err := db.QueryRow(`SELECT n.id, n.package_id, n.name, n.authors,
		CASE WHEN s.kind IN ('workshop','steamcmd') THEN i.rel_path ELSE n.workshop_id_declared END,
		coalesce(i.real_path, ''), coalesce(i.source_id, '') FROM snapshot n
		LEFT JOIN instance i ON i.id=? LEFT JOIN source s ON s.id=i.source_id
		WHERE n.id=CASE WHEN ?=0 THEN i.current_snapshot_id ELSE ? END`, id, snapshot, snapshot).
		Scan(&snapID, &v.PackageID, &v.Name, &v.Authors, &v.WorkshopID, &v.Folder, &v.SourceID)
	if err != nil {
		return nil, err
	}
	v.Files, err = semanticSet(db, snapID)
	return v, err
}

// QuestionEvidence uses the observation and the candidate's snapshot at the time of
// the question. Folder locations and folder-derived Workshop IDs are current. No mod
// files are opened and ordinary page models do not call this method.
func (s *Store) QuestionEvidence(q *Question) (*QuestionEvidence, error) {
	v := &QuestionEvidence{}
	var err error
	v.New, err = questionSide(s.DB, q.InstanceID, q.SnapshotID)
	if err != nil {
		return nil, err
	}
	var candidateSnap int64
	err = s.DB.QueryRow(`SELECT id FROM snapshot WHERE instance_id=? AND scan_id<=? AND id<>?
		ORDER BY scan_id DESC, id DESC LIMIT 1`, q.CandidateID, q.ScanID, q.SnapshotID).Scan(&candidateSnap)
	if err != nil {
		return nil, err
	}
	v.Candidate, err = questionSide(s.DB, q.CandidateID, candidateSnap)
	if err != nil {
		return nil, err
	}
	v.Files = identity.CompareFiles(v.New.Files, v.Candidate.Files)
	return v, nil
}

// recheckQuestions removes obsolete open guesses in the scan transaction. Player
// decisions are never selected. Partial scans leave questions about skipped sources alone.
func recheckQuestions(tx *sql.Tx, envID string, scanned []string) (map[int64]bool, error) {
	rows, err := tx.Query(`SELECT id, instance_id, candidate_instance_id FROM identity_question
		WHERE status='open' AND environment_id IN ('', ?) ORDER BY id`, envID)
	if err != nil {
		return nil, err
	}
	type pending struct {
		id           int64
		a, b         string
		check, valid bool
	}
	var qs []*pending
	for rows.Next() {
		q := &pending{}
		if err := rows.Scan(&q.id, &q.a, &q.b); err != nil {
			rows.Close()
			return nil, err
		}
		qs = append(qs, q)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, id := range scanned {
		active[id] = true
	}
	cache := map[string]*QuestionSide{}
	load := func(id string) (*QuestionSide, error) {
		if v, ok := cache[id]; ok {
			return v, nil
		}
		v, err := questionSide(tx, id, 0)
		if err == sql.ErrNoRows {
			cache[id] = nil
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		cache[id] = v
		return v, nil
	}
	degree := map[string]int{}
	for _, q := range qs {
		a, err := load(q.a)
		if err != nil {
			return nil, err
		}
		b, err := load(q.b)
		if err != nil {
			return nil, err
		}
		q.check = a == nil || b == nil || (active[a.SourceID] && active[b.SourceID])
		if !q.check {
			continue
		}
		q.valid = a != nil && b != nil && q.a != q.b && identity.QuestionScore(a.PackageID, a.WorkshopID, a.Files,
			b.PackageID, b.WorkshopID, b.Files) >= identity.QuestionSimilarity
		if q.valid {
			degree[q.a]++
			degree[q.b]++
		}
	}
	removed := map[int64]bool{}
	for _, q := range qs {
		if q.check && (!q.valid || degree[q.a] > 1 || degree[q.b] > 1) {
			if _, err := tx.Exec(`DELETE FROM identity_question WHERE id=? AND status='open'`, q.id); err != nil {
				return nil, fmt.Errorf("recheck q:%d: %w", q.id, err)
			}
			removed[q.id] = true
		}
	}
	return removed, nil
}
