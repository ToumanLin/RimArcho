package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// schemaV15: the DDS files RimArcho writes next to a mod's PNGs (08 §5.2, 贴图优化). A file
// is recorded as pending before the encoder runs and done once checked, so files of a run
// that was cut short are still known (and cleaned, and left out of the mod's fingerprint).
// texture_mod is the mod's snapshot when its textures were last optimised: when it changes
// (an update), its DDS may show old textures.
const schemaV15 = `
CREATE TABLE texture_file(
  path_key TEXT PRIMARY KEY, path TEXT NOT NULL, instance_id TEXT NOT NULL, mod_dir TEXT NOT NULL,
  rel TEXT NOT NULL, src_size INTEGER NOT NULL DEFAULT 0, src_mtime INTEGER NOT NULL DEFAULT 0,
  dds_size INTEGER NOT NULL DEFAULT 0, dds_mtime INTEGER NOT NULL DEFAULT 0,
  scale INTEGER NOT NULL DEFAULT 100, mips INTEGER NOT NULL DEFAULT 1,
  state TEXT NOT NULL, made_at INTEGER NOT NULL);
CREATE INDEX texture_file_inst ON texture_file(instance_id);
CREATE INDEX texture_file_dir ON texture_file(mod_dir);
CREATE TABLE texture_mod(
  instance_id TEXT PRIMARY KEY, snap_hash BLOB, scale INTEGER NOT NULL DEFAULT 100,
  mips INTEGER NOT NULL DEFAULT 1, files INTEGER NOT NULL DEFAULT 0, bytes INTEGER NOT NULL DEFAULT 0,
  made_at INTEGER NOT NULL);
`

// TextureFile is one DDS RimArcho wrote.
type TextureFile struct {
	Path       string // absolute
	InstanceID string
	ModDir     string // the mod's real folder
	Rel        string // the DDS relative to ModDir, forward slashes
	SrcSize    int64  // the PNG it was made from
	SrcMTime   int64
	DDSSize    int64
	DDSMTime   int64
	Scale      int
	Mips       bool
	Pending    bool // recorded before the encoder ran; not checked yet
	MadeAt     time.Time
}

// TextureMod is a mod's state when its textures were last optimised.
type TextureMod struct {
	InstanceID string
	SnapHash   []byte
	Scale      int
	Mips       bool
	Files      int
	Bytes      int64
	MadeAt     time.Time
}

// PathKey is how texture files are keyed: the cleaned absolute path, lower case (Windows
// paths are not case sensitive).
func PathKey(p string) string { return strings.ToLower(filepath.Clean(p)) }

// PutTextureFiles records (or replaces) texture files.
func (s *Store) PutTextureFiles(fs []TextureFile) error {
	if len(fs) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	st, err := tx.Prepare(`INSERT OR REPLACE INTO texture_file(path_key, path, instance_id, mod_dir, rel, src_size, src_mtime,
		dds_size, dds_mtime, scale, mips, state, made_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer st.Close()
	for _, f := range fs {
		state := "done"
		if f.Pending {
			state = "pending"
		}
		if _, err := st.Exec(PathKey(f.Path), f.Path, f.InstanceID, PathKey(f.ModDir), f.Rel, f.SrcSize, f.SrcMTime,
			f.DDSSize, f.DDSMTime, f.Scale, b2i(f.Mips), state, f.MadeAt.UnixNano()); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DeleteTextureFiles forgets texture files (by path).
func (s *Store) DeleteTextureFiles(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	for _, p := range paths {
		if _, err := tx.Exec(`DELETE FROM texture_file WHERE path_key=?`, PathKey(p)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

const textureCols = `path, instance_id, mod_dir, rel, src_size, src_mtime, dds_size, dds_mtime, scale, mips, state, made_at`

func scanTextureFiles(rows *sql.Rows, err error) ([]TextureFile, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TextureFile
	for rows.Next() {
		var f TextureFile
		var mips int
		var state string
		var made int64
		if err := rows.Scan(&f.Path, &f.InstanceID, &f.ModDir, &f.Rel, &f.SrcSize, &f.SrcMTime, &f.DDSSize, &f.DDSMTime,
			&f.Scale, &mips, &state, &made); err != nil {
			return nil, err
		}
		f.Mips, f.Pending, f.MadeAt = mips != 0, state == "pending", time.Unix(0, made)
		out = append(out, f)
	}
	return out, rows.Err()
}

// TextureFilesOf lists the texture files of instances (all when ids is nil).
func (s *Store) TextureFilesOf(ids []string) ([]TextureFile, error) {
	if ids == nil {
		return scanTextureFiles(s.DB.Query(`SELECT ` + textureCols + ` FROM texture_file ORDER BY path_key`))
	}
	var out []TextureFile
	for _, id := range ids {
		fs, err := scanTextureFiles(s.DB.Query(`SELECT `+textureCols+` FROM texture_file WHERE instance_id=? ORDER BY path_key`, id))
		if err != nil {
			return nil, err
		}
		out = append(out, fs...)
	}
	return out, nil
}

// TextureFilesIn lists the texture files under a mod folder.
func (s *Store) TextureFilesIn(modDir string) ([]TextureFile, error) {
	return scanTextureFiles(s.DB.Query(`SELECT `+textureCols+` FROM texture_file WHERE mod_dir=? ORDER BY path_key`, PathKey(modDir)))
}

// GeneratedFiles maps each mod folder (PathKey) with texture files to their paths relative
// to it, lower case: the scan leaves them out of the fingerprint.
func (s *Store) GeneratedFiles() (map[string]map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT mod_dir, rel FROM texture_file`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]bool{}
	for rows.Next() {
		var dir, rel string
		if err := rows.Scan(&dir, &rel); err != nil {
			return nil, err
		}
		if out[dir] == nil {
			out[dir] = map[string]bool{}
		}
		out[dir][strings.ToLower(rel)] = true
	}
	return out, rows.Err()
}

// TextureCounts is how many texture files and bytes each instance has.
func (s *Store) TextureCounts() (map[string][2]int64, error) {
	rows, err := s.DB.Query(`SELECT instance_id, count(*), coalesce(sum(dds_size), 0) FROM texture_file GROUP BY instance_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][2]int64{}
	for rows.Next() {
		var id string
		var n, b int64
		if err := rows.Scan(&id, &n, &b); err != nil {
			return nil, err
		}
		out[id] = [2]int64{n, b}
	}
	return out, rows.Err()
}

// PutTextureMod records a mod's state after its textures were optimised.
func (s *Store) PutTextureMod(m TextureMod) error {
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO texture_mod(instance_id, snap_hash, scale, mips, files, bytes, made_at) VALUES(?,?,?,?,?,?,?)`,
		m.InstanceID, m.SnapHash, m.Scale, b2i(m.Mips), m.Files, m.Bytes, m.MadeAt.UnixNano())
	return err
}

// DeleteTextureMod forgets a mod's optimisation.
func (s *Store) DeleteTextureMod(id string) error {
	_, err := s.DB.Exec(`DELETE FROM texture_mod WHERE instance_id=?`, id)
	return err
}

// TextureMods lists every mod whose textures were optimised.
func (s *Store) TextureMods() (map[string]TextureMod, error) {
	rows, err := s.DB.Query(`SELECT instance_id, snap_hash, scale, mips, files, bytes, made_at FROM texture_mod`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]TextureMod{}
	for rows.Next() {
		var m TextureMod
		var mips int
		var made int64
		if err := rows.Scan(&m.InstanceID, &m.SnapHash, &m.Scale, &mips, &m.Files, &m.Bytes, &made); err != nil {
			return nil, err
		}
		m.Mips, m.MadeAt = mips != 0, time.Unix(0, made)
		out[m.InstanceID] = m
	}
	return out, rows.Err()
}

// SnapshotsKey changes when a scan adds or removes snapshots (for caching SnapshotsWithPNG).
func (s *Store) SnapshotsKey() string {
	var n, mx int64
	s.DB.QueryRow(`SELECT count(*), coalesce(max(id), 0) FROM snapshot`).Scan(&n, &mx)
	return fmt.Sprintf("%d.%d", n, mx)
}

// SnapshotsWithPNG are the snapshots with a PNG under a Textures/ folder (the copies whose
// textures can be converted). It reads every file row: cache it with SnapshotsKey.
func (s *Store) SnapshotsWithPNG() (map[int64]bool, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT snapshot_id FROM snapshot_file WHERE rel_path LIKE '%textures/%.png'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
