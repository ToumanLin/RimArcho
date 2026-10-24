package app

// ResolvePageRepoForTest substitutes a local repository for a parsed HTTPS root.
// The hook exists only in test binaries; production extraction stays HTTPS-only.
func ResolvePageRepoForTest(a *App, resolve func(string) string) { a.pageRepo = resolve }
