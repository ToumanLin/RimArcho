package app

// ResolvePageRepoForTest substitutes a local repository for a parsed HTTPS root.
// The hook exists only in test binaries; production extraction stays HTTPS-only.
func ResolvePageRepoForTest(a *App, resolve func(string) string) { a.pageRepo = resolve }

// DownloadWorkshopAsForTest downloads Workshop items through SteamCMD with an explicit
// per-call account (11 §3): no anonymous first pass. No Workshop caller passes one yet.
func DownloadWorkshopAsForTest(a *App, ids []string, account string) (*SubscribeOp, error) {
	login, err := a.workshopLogin(account)
	if err != nil {
		return nil, err
	}
	return a.downloadWorkshopGroup(ids, "", login, false, true)
}
