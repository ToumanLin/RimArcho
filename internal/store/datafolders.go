package store

import (
	"database/sql"
	"os"
	"path/filepath"
)

// upgradeDataFolders (41 → 42): packs and the source library moved their defaults from the
// data directory to RimArcho's local folder (17 §3). A home that never chose these folders
// and has the old default folder keeps it: the old path becomes its setting, as `folder
// set` would write it, so nothing moves and "move to…" is how the player relocates it. A
// home without the old folder takes the new default. The data directory is the database's
// own folder, read from SQLite, so the step needs nothing from the caller.
func upgradeDataFolders(tx *sql.Tx, _ func(int64, int64)) error {
	var file string
	if err := tx.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&file); err != nil {
		return err
	}
	if file == "" {
		return nil // an in-memory database has no data directory
	}
	home := filepath.Dir(filepath.Clean(file))
	for _, f := range []struct{ id, key string }{{"packs", "pack.dir"}, {"source", "source.dir"}} {
		var set string
		tx.QueryRow(`SELECT value FROM meta WHERE key=?`, "cfg:"+f.key).Scan(&set)
		old := filepath.Join(home, f.id)
		if st, err := os.Stat(old); set != "" || err != nil || !st.IsDir() {
			continue
		}
		for k, v := range map[string]string{"cfg:" + f.key: old, "directory.used:" + f.id: old} {
			if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
