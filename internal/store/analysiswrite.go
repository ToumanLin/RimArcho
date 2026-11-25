package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
)

const analysisStageSchema = `
CREATE TABLE IF NOT EXISTS analysis_stage_content(
 analysis_id INTEGER NOT NULL REFERENCES analysis(id) ON DELETE CASCADE,
 instance_id TEXT NOT NULL, content_key TEXT NOT NULL, folders TEXT NOT NULL,
 def_files INTEGER NOT NULL, patch_files INTEGER NOT NULL, parse_errors TEXT NOT NULL,
 def_set_id INTEGER NOT NULL REFERENCES fact_set(id), dll_set_id INTEGER REFERENCES fact_set(id),
 PRIMARY KEY(analysis_id,instance_id));
CREATE INDEX IF NOT EXISTS analysis_stage_def ON analysis_stage_content(def_set_id);
CREATE INDEX IF NOT EXISTS analysis_stage_dll ON analysis_stage_content(dll_set_id);
`

func upgradeAnalysisStage(tx *sql.Tx, _ func(int64, int64)) error {
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM pragma_table_info('analysis') WHERE name='publication')`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec(`ALTER TABLE analysis ADD COLUMN publication INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	_, err := tx.Exec(analysisStageSchema)
	return err
}

// publication is zero during staging, negative for retired runs and positive for
// current runs. Positive values order publication even when saves overlap.
const activeAnalysis = `publication>0`
const analysisChunkRows = 4000
const analysisStageLifetime = time.Hour

// SaveAnalysis saves an analysis run and atomically publishes its views and findings.
func (s *Store) SaveAnalysis(gameID, setKey, runKey, mm string, keep []string, out *analyze.Output, started, finished time.Time) (int64, error) {
	return s.SaveAnalysisContext(context.Background(), gameID, setKey, runKey, mm, keep, out, started, finished)
}

// SaveAnalysisContext leaves the previous publication current if interrupted before commit.
func (s *Store) SaveAnalysisContext(ctx context.Context, gameID, setKey, runKey, mm string, keep []string, out *analyze.Output, started, finished time.Time) (int64, error) {
	// The token keeps an invalidated writer from adopting a reused row ID.
	token := rand.Text()
	row, err := s.DB.ExecContext(ctx, `INSERT INTO analysis(game_id,set_key,run_key,game_mm,version,started_at,finished_at,publication)
		VALUES(?,?,?,?,?,?,?,0)`, gameID, setKey, token, mm, analyze.Version, started.Unix(), time.Now().Unix())
	if err != nil {
		return 0, err
	}
	id, err := row.LastInsertId()
	if err != nil {
		return 0, err
	}
	lastYield := time.Now()
	chunk := func(write func(*sql.Tx) error) error {
		if err := yieldAnalysisWrite(ctx, &lastYield); err != nil {
			return err
		}
		return s.inTxContext(ctx, func(tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx, `UPDATE analysis SET finished_at=? WHERE id=? AND run_key=? AND publication=0`, time.Now().Unix(), id, token)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return errors.New("store: analysis save was invalidated")
			}
			return write(tx)
		})
	}
	published := false
	defer func() {
		if !published {
			// A killed process leaves a pending run for pruning after its heartbeat expires.
			if _, err := s.DB.Exec(`UPDATE analysis SET publication=-1 WHERE id=? AND run_key=? AND publication=0`, id, token); err == nil {
				_ = s.cleanAnalysisWrites()
			}
		}
	}()
	if err := stageContent(chunk, id, out); err != nil {
		return 0, err
	}
	for patches := range slices.Chunk(out.Patches, analysisChunkRows) {
		if err := chunk(func(tx *sql.Tx) error { return saveAnalysisPatches(ctx, tx, id, patches) }); err != nil {
			return 0, err
		}
	}
	for findings := range slices.Chunk(out.Findings, analysisChunkRows) {
		if err := chunk(func(tx *sql.Tx) error { return saveAnalysisFindings(ctx, tx, id, findings) }); err != nil {
			return 0, err
		}
	}
	if err := chunk(func(tx *sql.Tx) error {
		return publishAnalysis(tx, id, gameID, setKey, runKey, mm, keep, out.Stats, finished)
	}); err != nil {
		return 0, err
	}
	published = true
	// Publication is already committed. Maintenance retries interrupted cleanup.
	_ = s.cleanAnalysisWrites()
	return id, nil
}

func yieldAnalysisWrite(ctx context.Context, last *time.Time) error {
	if time.Since(*last) < 30*time.Millisecond {
		return ctx.Err()
	}
	// Competing SQLite writers back off and can miss several brief gaps between commits.
	// not scheduled: throttle for an analysis write already in progress
	pause := time.NewTimer(20 * time.Millisecond)
	defer pause.Stop()
	select {
	case <-pause.C:
		*last = time.Now()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func publishAnalysis(tx *sql.Tx, id int64, gameID, setKey, runKey, mm string, keep []string, stats analyze.Stats, finished time.Time) error {
	st, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	kept, err := json.Marshal(keep)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE analysis SET run_key=?,finished_at=?,stats=?,
		publication=(SELECT coalesce(max(publication),0)+1 FROM analysis WHERE game_id=? AND publication>0) WHERE id=?`,
		runKey, finished.Unix(), string(st), gameID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE analysis SET publication=-1 WHERE game_id=? AND id<>? AND publication>0
		AND ((set_key=? AND game_mm=?) OR set_key NOT IN (SELECT value FROM json_each(?) WHERE type='text'))`,
		gameID, id, setKey, mm, string(kept)); err != nil {
		return err
	}
	if err := publishContent(tx, id, mm); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM analysis_stage_content WHERE analysis_id=?`, id); err != nil {
		return err
	}
	return pruneFactViews(context.Background(), tx, finished)
}

func saveAnalysisPatches(ctx context.Context, tx *sql.Tx, id int64, patches []*analyze.PatchFact) error {
	ins, err := tx.PrepareContext(ctx, `INSERT INTO patch_fact(analysis_id, instance_id, seq, file, line, class, xpath, descr, result, hits, reasons, detail, targets, leaf)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, p := range patches {
		if _, err := ins.ExecContext(ctx, id, p.Instance, p.Seq, p.File, p.Line, p.Class, p.XPath, p.Desc, p.Result, p.Hits,
			strings.Join(p.Reasons, " "), p.Detail, strings.Join(p.Targets, " "), p.Leaf); err != nil {
			return err
		}
	}
	return nil
}

func saveAnalysisFindings(ctx context.Context, tx *sql.Tx, id int64, findings []*analyze.Finding) error {
	ins, err := tx.PrepareContext(ctx, `INSERT INTO finding(analysis_id, layer, code, severity, key, subject_instance, data) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, f := range findings {
		b, err := json.Marshal(f)
		if err != nil {
			return err
		}
		if _, err := ins.ExecContext(ctx, id, f.Layer, f.Code, f.Severity, f.Key, f.SubjectInstance(), string(b)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) cleanAnalysisWrites() error {
	lastYield := time.Now()
	exec := func(query string, args ...any) (sql.Result, error) {
		if err := yieldAnalysisWrite(context.Background(), &lastYield); err != nil {
			return nil, err
		}
		return s.DB.Exec(query, args...)
	}
	for _, table := range []string{"patch_fact", "finding"} {
		for {
			res, err := exec(`DELETE FROM `+table+` WHERE rowid IN (
				SELECT rowid FROM `+table+` WHERE analysis_id IN (SELECT id FROM analysis WHERE publication<0) LIMIT ?)`, analysisChunkRows)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n < analysisChunkRows {
				break
			}
		}
	}
	if _, err := exec(`DELETE FROM analysis WHERE publication<0
		AND NOT EXISTS(SELECT 1 FROM patch_fact WHERE analysis_id=analysis.id)
		AND NOT EXISTS(SELECT 1 FROM finding WHERE analysis_id=analysis.id)`); err != nil {
		return err
	}
	rows, err := s.DB.Query(`SELECT id, content_key FROM fact_set WHERE ` + unusedFactSet)
	if err != nil {
		return err
	}
	type unusedSet struct {
		id  int64
		key string
	}
	var sets []unusedSet
	for rows.Next() {
		var set unusedSet
		if err := rows.Scan(&set.id, &set.key); err != nil {
			rows.Close()
			return err
		}
		sets = append(sets, set)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, set := range sets {
		if err := s.cleanFactSet(exec, set.id, set.key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) cleanFactSet(exec func(string, ...any) (sql.Result, error), id int64, key string) error {
	const retired = "~retired:"
	tables := []string{"def_fact", "assembly_fact", "type_fact", "harmony_fact", "harmony_id"}
	if !strings.HasPrefix(key, retired) {
		queries := make([]string, 0, len(tables))
		for _, table := range tables {
			queries = append(queries, `SELECT 1 FROM `+table+` WHERE set_id=?1`)
		}
		var large bool
		if err := s.DB.QueryRow(`SELECT EXISTS (`+strings.Join(queries, " UNION ALL ")+` LIMIT 1 OFFSET ?2)`, id, analysisChunkRows).Scan(&large); err != nil {
			return err
		}
		if !large {
			_, err := exec(`DELETE FROM fact_set WHERE id=? AND content_key=? AND `+unusedFactSet, id, key)
			return err
		}
		// A set opened for chunked deletion must be unavailable for reuse.
		token := retired + rand.Text()
		res, err := exec(`UPDATE fact_set SET content_key=? WHERE id=? AND content_key=? AND `+unusedFactSet, token, id, key)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		key = token
	}
	for _, table := range tables {
		for {
			res, err := exec(`DELETE FROM `+table+` WHERE rowid IN (
				SELECT rowid FROM `+table+` WHERE set_id=?1
				AND EXISTS(SELECT 1 FROM fact_set WHERE id=?1 AND content_key=?2) LIMIT ?3)`, id, key, analysisChunkRows)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n < analysisChunkRows {
				break
			}
		}
	}
	_, err := exec(`DELETE FROM fact_set WHERE id=? AND content_key=? AND `+unusedFactSet, id, key)
	return err
}
