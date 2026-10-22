package gui

// Tests never open the file manager or the browser: a test that runs 打开位置 / 打开网页
// (TestEveryOpRuns runs every op) would otherwise start Explorer on a temporary folder that
// is gone by the time it looks, and open real Workshop pages.
func init() {
	openDir = func(string) error { return nil }
	openURL = func(string) error { return nil }
}
