package testkit

import (
	"os"
	"path/filepath"
)

// DowngradeFacts restores the pre-v26 fact tables when a migration test rolls the
// rest of a fixture back to an older release. Every row remains synthetic.
func (e *Env) DowngradeFacts() {
	e.T.Helper()
	data, err := os.ReadFile(filepath.Join(TemplateDir(), "..", "store", "facts-v25.sql"))
	if err != nil {
		e.T.Fatal(err)
	}
	if _, err := e.App.Store.DB.Exec(string(data)); err != nil {
		e.T.Fatal(err)
	}
}
