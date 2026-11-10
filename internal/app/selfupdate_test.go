package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// selfRelease is a fake GitHub (17 §5.1): one release 0.9.1 with its zip and checksum.
type selfRelease struct {
	description    string // the Workshop item's description, stamped with its release
	tags           []string
	workshopResult int
	workshopHTTP   int
	workshopHits   atomic.Int32
	stableHits     atomic.Int32
	srv            *httptest.Server
	zip            []byte
	sum            string       // the checksum file's content
	cut            bool         // the zip download breaks off halfway
	zipHits        atomic.Int32 // zip downloads
	lists          atomic.Int32 // /releases (prerelease) lookups
	program        string       // the program folder with the old files
}

func newSelfRelease(t *testing.T) *selfRelease {
	t.Helper()
	f := &selfRelease{program: filepath.Join(t.TempDir(), "RimArcho"), description: selfupdate.Stamp("About RimArcho", "0.9.1"), tags: []string{"Mod", "1.6"}, workshopResult: 1}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range selfupdate.Files {
		entry := name
		if name == "monitor" {
			entry = `monitor\ArchoMonitor\About\About.xml` // Windows PowerShell 5 writes "\"
		}
		w, err := z.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(w, "new %s", name)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f.zip = buf.Bytes()
	h := sha256.Sum256(f.zip)
	zipName := selfupdate.ZipName("0.9.1", runtime.GOOS, runtime.GOARCH)
	f.sum = hex.EncodeToString(h[:]) + "  " + zipName + "\n"
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := func(tag string, pre bool) string {
			return fmt.Sprintf(`{"tag_name":%q,"name":"RimArcho %s","body":"- fixed things\n- new things","html_url":"%s/releases/tag/%s","published_at":"2026-10-30T12:00:00Z","prerelease":%v,
				"assets":[{"name":%q,"browser_download_url":"%s/dl/zip","size":%d},{"name":%q,"browser_download_url":"%s/dl/sum","size":%d}]}`,
				tag, tag, f.srv.URL, tag, pre, zipName, f.srv.URL, len(f.zip), zipName+".sha256", f.srv.URL, len(f.sum))
		}
		switch r.URL.Path {
		case "/workshop":
			f.workshopHits.Add(1)
			if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("itemcount") != "1" || r.Form.Get("publishedfileids[0]") != "8100" {
				t.Error("unexpected Workshop request", r.Method, r.Form)
			}
			if f.workshopHTTP != 0 {
				w.WriteHeader(f.workshopHTTP)
				return
			}
			fmt.Fprintf(w, `{"response":{"publishedfiledetails":[{"publishedfileid":"8100","result":%d,"title":"RimArcho","description":%q,"tags":[`, f.workshopResult, f.description)
			for i, tag := range f.tags {
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"tag":%q}`, tag)
			}
			fmt.Fprint(w, `]}]}}`)
		case "/releases/tags/v0.9.1":
			fmt.Fprint(w, release("v0.9.1", false))
		case "/releases/latest":
			f.stableHits.Add(1)
			fmt.Fprint(w, release("v0.9.1", false))
		case "/releases":
			f.lists.Add(1)
			fmt.Fprint(w, "["+release("v0.9.1", false)+"]")
		case "/dl/sum":
			fmt.Fprint(w, f.sum)
		case "/dl/zip":
			f.zipHits.Add(1)
			if f.cut {
				w.Header().Set("Content-Length", strconv.Itoa(len(f.zip)))
				w.WriteHeader(http.StatusOK)
				w.Write(f.zip[:len(f.zip)/2])
				w.(http.Flusher).Flush()
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
			w.Write(f.zip)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	for _, name := range selfupdate.Files {
		path := filepath.Join(f.program, name)
		if name == "monitor" {
			path = filepath.Join(path, "ArchoMonitor", "About", "About.xml")
		}
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte("old "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// use points the app at the fake: version 0.9.0 running from the program folder, the new
// archo answering 0.9.1, no other RimArcho open.
func (f *selfRelease) use(a *app.App) {
	a.SelfVersion, a.SelfUpdateAPI, a.SelfProgram = "0.9.0", f.srv.URL, f.program
	a.SelfVersionOf = func(context.Context, string) (string, error) { return "0.9.1", nil }
	a.SelfOthers = func(string) []*app.GameRun { return nil }
	a.SelfWaitPoll = time.Millisecond
}

// files reads the program folder's copies of the release files ("old …" or "new …").
func (f *selfRelease) files(t *testing.T) string {
	t.Helper()
	var out []string
	for _, name := range selfupdate.Files {
		path := filepath.Join(f.program, name)
		if name == "monitor" {
			path = filepath.Join(path, "ArchoMonitor", "About", "About.xml")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	return strings.Join(out, "|")
}

func allWith(prefix string) string {
	var out []string
	for _, name := range selfupdate.Files {
		out = append(out, prefix+" "+name)
	}
	return strings.Join(out, "|")
}

func TestSelfUpdateCheck(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(true)
	f := newSelfRelease(t)
	f.use(e.App)
	ctx := context.Background()

	e.App.SelfVersion = "dev"
	if e.App.AutomaticDue("self") {
		t.Fatal("a development build checks GitHub")
	}
	e.App.SelfVersion = "0.9.0"
	if !e.App.AutomaticDue("self") {
		t.Fatal("never checked, not due")
	}
	if ran, err := e.App.RunAutomatic(ctx, "self", nil); !ran || err != nil {
		t.Fatal(ran, err)
	}
	st := e.App.AutomaticStatus("self")
	if st.Pending != 1 || st.Latest != "0.9.1" || st.State != "new version available" {
		t.Fatal(st)
	}
	r := e.App.SelfUpdatePending()
	if r == nil || r.Version != "0.9.1" || r.Installable() != nil || !strings.Contains(r.Notes, "fixed things") {
		t.Fatal(r)
	}
	if out := e.MustRun("status"); !strings.Contains(out, "RimArcho 0.9.1 available") {
		t.Fatal(out)
	}
	if e.App.AutomaticDue("self") {
		t.Fatal("checked again within the interval")
	}
	e.Advance(24 * time.Hour)
	if !e.App.AutomaticDue("self") {
		t.Fatal("daily check not due")
	}
	// Once 0.9.1 runs, the record made by 0.9.0 no longer counts.
	e.App.SelfVersion = "0.9.1"
	if e.App.SelfUpdatePending() != nil || strings.Contains(e.MustRun("status"), "RimArcho 0.9.1") {
		t.Fatal("stale update after installing it")
	}
	// A prerelease build looks at the list with prereleases; a stable one never does.
	if f.lists.Load() != 0 {
		t.Fatal("stable build listed prereleases")
	}
	e.App.SelfVersion = "0.9.1-rc1"
	if _, _, err := e.App.CheckSelfUpdate(ctx); err != nil || f.lists.Load() != 1 || e.App.SelfUpdatePending() == nil {
		t.Fatal(err, f.lists.Load())
	}
	if out := e.MustRun("update", "--check"); !strings.Contains(out, "latest: 0.9.1") || !strings.Contains(out, "update: available") || !strings.Contains(out, "fixed things") {
		t.Fatal(out)
	}
}

func TestSelfUpdateWorkshopCheck(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("program replacement and Workshop self-update are Windows-only (D2)")
	}
	t.Setenv(app.UpdateWorkshopItemEnv, "8100")
	for _, mode := range []string{"workshop", "1.0.5 tag", "no version", "bad result", "offline", "empty item", "github", "prerelease"} {
		t.Run(mode, func(t *testing.T) {
			e := testkit.New(t)
			f := newSelfRelease(t)
			f.use(e.App)
			e.App.SteamAPI = f.srv.URL + "/workshop"
			e.App.SteamCMDRun = func(string, []string) (string, error) { t.Fatal("check started SteamCMD"); return "", nil }
			switch mode {
			case "1.0.5 tag": // an item published by 1.0.5: no stamp, a v<version> tag
				f.description, f.tags = "About RimArcho", []string{"Mod", "v0.9.1"}
			case "no version": // edited on the web: no stamp, and the tags are the game's
				f.description, f.tags = "About RimArcho, mentioning RimArcho v0.9.1 in a sentence", []string{"Mod", "1.6", "not-a-version"}
			case "bad result":
				f.workshopResult = 9
			case "offline":
				f.workshopHTTP = 503
			case "empty item":
				t.Setenv(app.UpdateWorkshopItemEnv, "")
			case "github":
				e.MustRun("config", "set", "updates.source", "github")
			case "prerelease":
				e.App.SelfVersion = "0.9.1-rc1"
			}
			out := e.MustRun("update", "--check")
			source := "GitHub"
			if mode == "workshop" || mode == "1.0.5 tag" {
				source = "Workshop"
			}
			info := e.App.SelfUpdateInfo()
			if info.Source != source || info.Release == nil || info.Release.Version != "0.9.1" || !strings.Contains(out, "source: "+source) {
				t.Fatal(info, out)
			}
			wantHits := int32(1)
			if mode == "empty item" || mode == "github" || mode == "prerelease" {
				wantHits = 0
			}
			if f.workshopHits.Load() != wantHits {
				t.Fatal("Workshop requests", f.workshopHits.Load())
			}
			if mode == "workshop" {
				if f.stableHits.Load() != 0 || info.Release.WorkshopItem != "8100" {
					t.Fatal(info)
				}
				before := e.App.AutomaticStatus("self")
				e.MustRun("config", "set", "updates.source", "github")
				e.MustRun("update", "--check")
				if notice := view.AutomaticNotice(e.App, before, e.App.AutomaticStatus("self")); notice != "" {
					t.Fatal("repeated notification", notice)
				}
			}
		})
	}
}

func TestSelfUpdateWorkshopInstall(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("Workshop carries Windows amd64 only")
	}
	t.Setenv(app.UpdateWorkshopItemEnv, "8100")
	for _, mode := range []string{"subscribed", "steamcmd", "missing tool", "unconfirmed", "missing files", "corrupt", "no matching release"} {
		t.Run(mode, func(t *testing.T) {
			e := testkit.New(t)
			f := newSelfRelease(t)
			f.use(e.App)
			e.App.SteamAPI = f.srv.URL + "/workshop"
			e.MustRun("config", "set", "steamcmd.account", "desktop-player")
			e.MustRun("config", "set", "updates.source", "auto")
			if mode == "no matching release" {
				f.description = selfupdate.Stamp("About RimArcho", "0.9.2")
			}
			if _, _, err := e.App.CheckSelfUpdate(context.Background()); err != nil {
				t.Fatal(err)
			}
			put := func(dir string) {
				e.MkdirAll(dir)
				e.WriteFile(filepath.Join(dir, selfupdate.ZipName("0.9.1", "windows", "amd64")), string(f.zip))
				e.WriteFile(filepath.Join(dir, selfupdate.ZipName("0.9.1", "windows", "amd64")+".sha256"), f.sum)
			}
			if mode == "subscribed" {
				// Discover a second Steam library through the existing VDF reader.
				library := filepath.Join(e.Root, "OtherLibrary")
				e.WriteFile(filepath.Join(e.Steam, "steamapps", "libraryfolders.vdf"), fmt.Sprintf(`"libraryfolders" { "1" { "path" %q } }`, strings.ReplaceAll(library, `\`, `/`)))
				put(filepath.Join(library, "steamapps", "workshop", "content", "294100", "8100"))
				e.App.FindTool = func(string) (string, bool) { t.Fatal("subscribed copy started tool lookup"); return "", false }
			} else {
				e.App.FindTool = func(string) (string, bool) {
					return filepath.Join(e.Root, deps.SteamCMDExecutable()), mode != "missing tool"
				}
			}
			base := filepath.Join(e.App.LocalDir(), "steamcmd-self")
			calls := 0
			e.App.SteamCMDRun = func(_ string, args []string) (string, error) {
				calls++
				if args[1] != base || args[2] != "+login" || args[3] != "anonymous" {
					t.Fatal(args)
				}
				script, err := os.ReadFile(filepath.Join(e.Root, args[5]))
				if err != nil || !strings.Contains(string(script), "workshop_download_item 294100 8100") {
					t.Fatal(string(script), err)
				}
				if mode != "missing files" {
					put(filepath.Join(base, "steamapps", "workshop", "content", "294100", "8100"))
				}
				if mode == "unconfirmed" {
					return "ERROR! Download item 8100 failed (AccessDenied).", nil
				}
				if mode == "corrupt" {
					e.WriteFile(filepath.Join(base, "steamapps", "workshop", "content", "294100", "8100", selfupdate.ZipName("0.9.1", "windows", "amd64")), "broken")
				}
				return "Success. Downloaded item 8100", nil
			}
			_, err := e.App.InstallSelfUpdate(context.Background(), func(s app.SelfUpdateStep) {
				if mode == "steamcmd" && s.Phase == "download" && (s.Done != 0 || s.Total != 0) {
					t.Error("determinate SteamCMD progress", s)
				}
			})
			if mode == "no matching release" {
				if err == nil || f.files(t) != allWith("old") {
					t.Fatal("wrong version installed", err)
				}
				e.App.CleanSelfUpdate()
			} else if err != nil || f.files(t) != allWith("new") {
				t.Fatal(err)
			}
			if _, err := os.Stat(base); !os.IsNotExist(err) {
				t.Fatal("private base retained", err)
			}
			if mode == "subscribed" || mode == "steamcmd" {
				if f.zipHits.Load() != 0 {
					t.Fatal("unnecessary GitHub download")
				}
			} else if mode != "no matching release" && f.zipHits.Load() != 1 {
				t.Fatal("missing GitHub fallback")
			}
			if mode == "steamcmd" && calls != 1 {
				t.Fatal("SteamCMD calls", calls)
			}
			if sources, err := e.App.Store.AllSources(); err != nil {
				t.Fatal(err)
			} else {
				for _, source := range sources {
					if strings.Contains(source.Root, "steamcmd-self") {
						t.Fatal("registered updater as Mod source")
					}
				}
			}
		})
	}
}

func TestSelfUpdateInstallWaitsForOthersThenSwaps(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("program replacement and Workshop self-update are Windows-only (D2)")
	}
	t.Parallel()
	e := testkit.New(t)
	f := newSelfRelease(t)
	f.use(e.App)
	ctx := context.Background()
	if _, err := e.App.RunAutomatic(ctx, "self", nil); err != nil {
		t.Fatal(err)
	}
	others := 3 // another window is open for the first three looks
	e.App.SelfOthers = func(string) []*app.GameRun {
		if others > 0 {
			others--
			return []*app.GameRun{{PID: 4242, Exe: filepath.Join(f.program, "RimArcho.exe")}}
		}
		return nil
	}
	var phases []string
	res, err := e.App.InstallSelfUpdate(ctx, func(s app.SelfUpdateStep) {
		if len(phases) == 0 || phases[len(phases)-1] != s.Phase {
			phases = append(phases, s.Phase)
		}
		if s.Phase == "wait" && f.files(t) != allWith("old") {
			t.Error("files replaced while another RimArcho runs")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.From != "0.9.0" || res.To != "0.9.1" || res.Program != f.program {
		t.Fatal(res)
	}
	if got := strings.Join(phases, " "); got != "download unpack wait replace check" {
		t.Fatal(got)
	}
	if got := f.files(t); got != allWith("new") {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(e.Home, "archo.db")); err != nil {
		t.Fatal("data folder touched", err)
	}
	if st := e.App.AutomaticStatus("self"); st.Pending != 0 {
		t.Fatal(st)
	}
	dir := filepath.Join(f.program, app.UpdateDirName)
	olds, _ := filepath.Glob(filepath.Join(dir, "old-0.9.0-*"))
	if len(olds) != 1 {
		t.Fatal(olds)
	}
	if b, err := os.ReadFile(filepath.Join(olds[0], selfupdate.Files[0])); err != nil || string(b) != "old "+selfupdate.Files[0] {
		t.Fatal("old program not kept for the restart", string(b), err)
	}
	// The next start removes what the update left; the folder's lock file stays.
	e.App.SelfVersion = "0.9.1"
	e.App.CleanSelfUpdate()
	left, _ := os.ReadDir(dir)
	for _, d := range left {
		if d.Name() != "lock" {
			t.Fatal("left after clean-up:", d.Name())
		}
	}
	if f.files(t) != allWith("new") {
		t.Fatal("clean-up touched the program")
	}
}

func TestSelfUpdateFailuresChangeNothing(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("program replacement and Workshop self-update are Windows-only (D2)")
	}
	t.Parallel()
	ctx := context.Background()
	for _, tc := range []struct {
		name, want string
		setup      func(f *selfRelease, a *app.App)
	}{
		{"checksum", "does not match its checksum", func(f *selfRelease, a *app.App) {
			f.sum = strings.Repeat("0", 64) + "  " + selfupdate.ZipName("0.9.1", runtime.GOOS, runtime.GOARCH) + "\n"
		}},
		{"interrupted download", "download failed; nothing was changed", func(f *selfRelease, a *app.App) { f.cut = true }},
		{"new program fails", "the new program did not start, so RimArcho 0.9.0 was put back", func(f *selfRelease, a *app.App) {
			a.SelfVersionOf = func(context.Context, string) (string, error) { return "", errors.New("exit status 3221225781") }
		}},
		{"new program wrong version", "was put back", func(f *selfRelease, a *app.App) {
			a.SelfVersionOf = func(context.Context, string) (string, error) { return "0.9.0", nil }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testkit.New(t)
			f := newSelfRelease(t)
			f.use(e.App)
			if _, err := e.App.RunAutomatic(ctx, "self", nil); err != nil {
				t.Fatal(err)
			}
			tc.setup(f, e.App)
			_, err := e.App.InstallSelfUpdate(ctx, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal(err)
			}
			if got := f.files(t); got != allWith("old") {
				t.Fatal("program changed:", got)
			}
			dir := filepath.Join(f.program, app.UpdateDirName)
			if parts, _ := filepath.Glob(filepath.Join(dir, "download", "*")); tc.name != "new program fails" && tc.name != "new program wrong version" && len(parts) != 0 {
				t.Fatal("bad download kept:", parts)
			}
			if news, _ := filepath.Glob(filepath.Join(dir, "new-*")); len(news) != 0 {
				t.Fatal("unpacked files kept:", news)
			}
			if e.App.SelfUpdatePending() == nil {
				t.Fatal("a failed install forgot the update")
			}
		})
	}
}

func TestSelfUpdateStopWhileWaitingAndOneInstallAtATime(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("program replacement and Workshop self-update are Windows-only (D2)")
	}
	t.Parallel()
	e := testkit.New(t)
	f := newSelfRelease(t)
	f.use(e.App)
	if _, err := e.App.RunAutomatic(context.Background(), "self", nil); err != nil {
		t.Fatal(err)
	}
	e.App.SelfOthers = func(string) []*app.GameRun { return []*app.GameRun{{PID: 7, Exe: "archo.exe"}} }
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan struct{})
	done := make(chan error)
	go func() {
		var once bool
		_, err := e.App.InstallSelfUpdate(ctx, func(s app.SelfUpdateStep) {
			if s.Phase == "wait" && !once {
				once = true
				close(waiting)
			}
		})
		done <- err
	}()
	<-waiting
	// A second window's install is refused while the first holds the folder.
	other, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.InstallSelfUpdate(context.Background(), nil); !errors.Is(err, app.ErrUpdateBusy) {
		t.Fatal(err)
	}
	// Nor does a starting RimArcho clean the folder under it.
	other.CleanSelfUpdate()
	if zips, _ := filepath.Glob(filepath.Join(f.program, app.UpdateDirName, "download", "*.zip")); len(zips) != 1 {
		t.Fatal("clean-up during an install", zips)
	}
	cancel()
	err = <-done
	if err == nil || !strings.Contains(err.Error(), "stopped while waiting") {
		t.Fatal(err)
	}
	if f.files(t) != allWith("old") {
		t.Fatal("program changed")
	}
	// The verified download is kept: installing again does not fetch it again.
	e.App.SelfOthers = func(string) []*app.GameRun { return nil }
	if _, err := e.App.InstallSelfUpdate(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if n := f.zipHits.Load(); n != 1 {
		t.Fatal("zip downloaded", n, "times")
	}
	if f.files(t) != allWith("new") {
		t.Fatal("not installed")
	}
}

// TestSelfUpdateKilledMidSwapIsSettledNextStart: a process killed after the files moved
// but before the new program was confirmed leaves its record; the next start finishes the
// install when the new program runs, or puts the old one back when it does not, and only
// then removes the update folder's leftovers (17 §5.1 step 5).
func TestSelfUpdateKilledMidSwapIsSettledNextStart(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("program replacement and Workshop self-update are Windows-only (D2)")
	}
	t.Parallel()
	for _, next := range []string{"new program runs", "new program fails", "install again"} {
		t.Run(next, func(t *testing.T) {
			e := testkit.New(t)
			f := newSelfRelease(t)
			f.use(e.App)
			if _, err := e.App.RunAutomatic(context.Background(), "self", nil); err != nil {
				t.Fatal(err)
			}
			e.App.SelfVersionOf = func(context.Context, string) (string, error) {
				runtime.Goexit() // the process dies during the check
				return "", nil
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				e.App.InstallSelfUpdate(context.Background(), nil)
			}()
			<-done
			dir := filepath.Join(f.program, app.UpdateDirName)
			if _, err := os.Stat(filepath.Join(dir, "swap.json")); err != nil {
				t.Fatal("no record of the swap", err)
			}
			if news, _ := filepath.Glob(filepath.Join(dir, "new-*")); len(news) != 1 {
				t.Fatal("staged files removed while the swap is open", news)
			}
			// The next start: a fresh session over the same data folder.
			a, err := e.App.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			f.use(a)
			want := allWith("new")
			switch next {
			case "new program fails":
				a.SelfVersionOf = func(context.Context, string) (string, error) { return "", errors.New("exit status 3221225781") }
				want = allWith("old")
				a.CleanSelfUpdate()
			case "install again":
				res, err := a.InstallSelfUpdate(context.Background(), nil)
				if err != nil || res.To != "0.9.1" {
					t.Fatal(res, err)
				}
				if f.zipHits.Load() != 1 {
					t.Fatal("downloaded again", f.zipHits.Load())
				}
				a.SelfVersion = "0.9.1"
				a.CleanSelfUpdate()
			default:
				a.SelfVersion = "0.9.1" // the new program started and cleans up
				a.CleanSelfUpdate()
			}
			if got := f.files(t); got != want {
				t.Fatal(got)
			}
			left, _ := os.ReadDir(dir)
			for _, d := range left {
				if d.Name() != "lock" {
					t.Fatal("left after clean-up:", d.Name())
				}
			}
			if pending := a.SelfUpdatePending() != nil; pending != (next == "new program fails") {
				t.Fatal("update still offered:", pending)
			}
		})
	}
}
