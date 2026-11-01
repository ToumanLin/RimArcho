package tui

// Tests never open the file manager or the browser: a test that runs open the location / open page
// (TestEveryOpRuns runs every op) would otherwise start Explorer on a temporary folder that
// is gone by the time it looks, and open real Workshop pages.
func init() {
	openDir = func(string) error { return nil }
	openURL = func(string) error { return nil }
	copyText = func(string) error { return nil }
}
