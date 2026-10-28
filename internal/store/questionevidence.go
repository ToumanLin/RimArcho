package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

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
	return questionEvidence(s.DB, q)
}

func questionEvidence(db questionReader, q *Question) (*QuestionEvidence, error) {
	v := &QuestionEvidence{}
	var err error
	v.New, err = questionSide(db, q.InstanceID, q.SnapshotID)
	if err != nil {
		return nil, err
	}
	var candidateSnap int64
	err = db.QueryRow(`SELECT id FROM snapshot WHERE instance_id=? AND scan_id<=? AND id<>?
		ORDER BY scan_id DESC, id DESC LIMIT 1`, q.CandidateID, q.ScanID, q.SnapshotID).Scan(&candidateSnap)
	if err != nil {
		return nil, err
	}
	v.Candidate, err = questionSide(db, q.CandidateID, candidateSnap)
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

// SummaryOf contains only counts and declaration differences, never file paths.
func SummaryOf(e *QuestionEvidence) identity.QuestionSummary {
	ws := func(id string) string {
		if identity.ValidWorkshopID(id) {
			return id
		}
		return ""
	}
	v := identity.QuestionSummary{Available: true, Common: e.Files.PayloadCommon, NewFiles: e.Files.PayloadA, PreviousFiles: e.Files.PayloadB}
	for _, pair := range []struct {
		a, b string
		bit  uint8
	}{
		{e.New.Name, e.Candidate.Name, identity.DifferentName},
		{e.New.PackageID, e.Candidate.PackageID, identity.DifferentPackage},
		{e.New.Authors, e.Candidate.Authors, identity.DifferentAuthor},
		{ws(e.New.WorkshopID), ws(e.Candidate.WorkshopID), identity.DifferentWorkshop},
	} {
		if !strings.EqualFold(pair.a, pair.b) {
			v.Differences |= pair.bit
		}
	}
	return v
}

func (q *Question) ComparisonSummary() identity.QuestionSummary {
	var v identity.QuestionSummary
	_ = json.Unmarshal([]byte(q.Summary), &v)
	return v
}

func saveQuestionSummary(tx *sql.Tx, q *Question) error {
	e, err := questionEvidence(tx, q)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	raw, err := json.Marshal(SummaryOf(e))
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE identity_question SET summary=? WHERE id=?`, string(raw), q.ID)
	return err
}

const schemaV22 = `ALTER TABLE identity_question ADD COLUMN summary TEXT NOT NULL DEFAULT '';`

func migrateV22(tx *sql.Tx) error {
	var qs []*Question
	if err := eachRow(tx, `SELECT id,scan_id,instance_id,observation_snapshot_id,candidate_instance_id FROM identity_question WHERE status='open'`, func(rows *sql.Rows) error {
		q := &Question{}
		if err := rows.Scan(&q.ID, &q.ScanID, &q.InstanceID, &q.SnapshotID, &q.CandidateID); err != nil {
			return err
		}
		qs = append(qs, q)
		return nil
	}); err != nil {
		return err
	}
	for _, q := range qs {
		if err := saveQuestionSummary(tx, q); err != nil {
			return err
		}
	}
	return nil
}
