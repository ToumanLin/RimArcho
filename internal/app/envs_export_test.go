package app

// SetTestFault makes f decide whether a named step of a's Game Config work fails (nil: none).
func SetTestFault(a *App, f func(step string) error) {
	if f == nil {
		testFaults.Delete(a)
		return
	}
	testFaults.Store(a, f)
}
