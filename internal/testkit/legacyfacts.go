package testkit

import (
	"os"
	"path/filepath"
)

// DowngradeFacts restores the pre-v26 fact tables when a migration test rolls the
// rest of a fixture back to an older release. Every row remains synthetic.
func (e *Env) DowngradeFacts() {
	e.T.Helper()
	if _, err := e.App.Store.DB.Exec(`DROP TABLE source_library; ALTER TABLE source_use DROP COLUMN mode; ALTER TABLE source_use RENAME TO source_cache`); err != nil {
		e.T.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(TemplateDir(), "..", "store", "facts-v25.sql"))
	if err != nil {
		e.T.Fatal(err)
	}
	if _, err := e.App.Store.DB.Exec(string(data)); err != nil {
		e.T.Fatal(err)
	}
}
