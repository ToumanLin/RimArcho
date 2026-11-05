package app

// What the saves commands (05 §4.2, internal/cmds/saves.go) need from the saves core beyond
// saves.go, savecheck.go and saveactions.go.

// InSaveList runs fn with the save's environment and matched plan as this App's current
// ones, then restores them: a fix's subscriptions go to the save's game, and launching
// from a save launches its plan (writing it to the game makes it current, 04 §1.3).
func (a *App) InSaveList(s *Save, fn func() error) error { return a.inSaveList(s, fn) }

// SaveIndex is the index of the save's environment, for printing its Mods' references.
func (a *App) SaveIndex(s *Save) (*Index, error) {
	var ix *Index
	err := a.inSaveList(s, func() error {
		var err error
		ix, err = a.Index()
		return err
	})
	return ix, err
}
