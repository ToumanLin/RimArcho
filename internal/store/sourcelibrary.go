package store

import (
	"database/sql"
	"time"
)

// SourceArtifact identifies generated code. Folder is relative to the managed root.
// GameBuild distinguishes game releases even when their assembly bytes are equal.
type SourceArtifact struct {
	Hash, GameBuild, Folder, Kind, PackageID, DLLName, ILSpyVersion string
	Created                                                         time.Time
	Bytes                                                           int64
}

func scanSourceArtifact(row interface{ Scan(...any) error }) (*SourceArtifact, error) {
	var r SourceArtifact
	var at int64
	err := row.Scan(&r.Hash, &r.GameBuild, &r.Folder, &r.Kind, &r.PackageID, &r.DLLName, &r.ILSpyVersion, &at, &r.Bytes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	r.Created = time.Unix(at, 0).UTC()
	return &r, err
}

const sourceArtifactCols = `dll_hash,game_build,folder,kind,package_id,dll_name,ilspy_version,created_at,bytes`

func (s *Store) SourceArtifact(hash, build string) (*SourceArtifact, error) {
	return scanSourceArtifact(s.DB.QueryRow(`SELECT `+sourceArtifactCols+` FROM source_library WHERE dll_hash=? AND game_build=?`, hash, build))
}

// ClaimSourceArtifact preserves the first readable location chosen for shared code.
func (s *Store) ClaimSourceArtifact(r SourceArtifact) (*SourceArtifact, error) {
	_, err := s.DB.Exec(`INSERT INTO source_library(`+sourceArtifactCols+`) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(dll_hash,game_build) DO NOTHING`,
		r.Hash, r.GameBuild, r.Folder, r.Kind, r.PackageID, r.DLLName, r.ILSpyVersion, r.Created.Unix(), r.Bytes)
	if err != nil {
		return nil, err
	}
	return s.SourceArtifact(r.Hash, r.GameBuild)
}

func (s *Store) SourceArtifacts() ([]*SourceArtifact, error) {
	rows, err := s.DB.Query(`SELECT ` + sourceArtifactCols + ` FROM source_library ORDER BY folder`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SourceArtifact
	for rows.Next() {
		r, err := scanSourceArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSourceArtifact(hash, build, version string, bytes int64) error {
	_, err := s.DB.Exec(`UPDATE source_library SET ilspy_version=?,bytes=? WHERE dll_hash=? AND game_build=?`, version, bytes, hash, build)
	return err
}

func (s *Store) ForgetSourceArtifact(hash, build string) error {
	_, err := s.DB.Exec(`DELETE FROM source_library WHERE dll_hash=? AND game_build=?`, hash, build)
	return err
}

// RecordSourceUse covers project/type attempts and successful reuse alike.
func (s *Store) RecordSourceUse(hash, instance, file, mode, status, errText string, at time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO source_use(dll_hash,instance_id,file,status,error,decompiled_at,mode) VALUES(?,?,?,?,?,?,?)
	ON CONFLICT(dll_hash,instance_id) DO UPDATE SET file=excluded.file,status=excluded.status,error=excluded.error,decompiled_at=excluded.decompiled_at,mode=excluded.mode`,
		hash, instance, file, status, errText, at.Unix(), mode)
	return err
}

// SourceLibraryBytes uses persisted accounting; Settings never walks generated files.
func (s *Store) SourceLibraryBytes() int64 {
	var n int64
	s.DB.QueryRow(`SELECT coalesce(sum(bytes),0) FROM source_library`).Scan(&n)
	return n
}
