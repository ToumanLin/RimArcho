package accounts

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestRuntimeClientsRegistrationAndHighestInstalledVersion(t *testing.T) {
	type key struct {
		root       registry.Key
		path, name string
	}
	values := map[key]string{}
	root := t.TempDir()
	dll := func(version string) string {
		p := filepath.Join(root, version, "EBWebView", map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[runtime.GOARCH], "EmbeddedBrowserWebView.dll")
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("offline fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	machine := `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + runtimeID
	values[key{registry.LOCAL_MACHINE, machine, "pv"}] = "155.0.4283.62"
	values[key{registry.LOCAL_MACHINE, machine, "location"}] = root
	// No HKLM ClientState; HKCU ClientState has an empty EBWebView, as in the VM.
	read := func(r registry.Key, p, n string) string { return values[key{r, p, n}] }
	want := dll("155.0.4283.62")
	if got, err := findRuntimeDLL(read); err != nil || got != want {
		t.Fatal(got, err)
	}
	user := `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + runtimeID
	values[key{registry.CURRENT_USER, user, "pv"}] = "156.0.1.2"
	values[key{registry.CURRENT_USER, user, "location"}] = root
	if got, err := findRuntimeDLL(read); err != nil || got != want {
		t.Fatal("missing DLL selected", got, err)
	}
	want = dll("156.0.1.2")
	if got, err := findRuntimeDLL(read); err != nil || got != want {
		t.Fatal("highest version not selected", got, err)
	}
	values[key{registry.LOCAL_MACHINE, machine, "pv"}] = "999.0.0.0"
	if got, err := findRuntimeDLL(read); err != nil || got != want {
		t.Fatal(got, err)
	}
	values = map[key]string{}
	values[key{registry.CURRENT_USER, `SOFTWARE\Microsoft\EdgeUpdate\ClientState\` + runtimeID, "EBWebView"}] = filepath.Join(root, "155.0.4283.62")
	if got, err := findRuntimeDLL(read); err != nil || got != dll("155.0.4283.62") {
		t.Fatal("ClientState fallback", got, err)
	}
	values = map[key]string{}
	if _, err := findRuntimeDLL(read); err == nil || err.Error() != "WebView2 runtime not found" {
		t.Fatal(err)
	}
}

func TestEnvironmentOptionsCOMABI(t *testing.T) {
	options := newEnvironmentOptions()
	if got := comString(&options.primary, 7); got != "86.0.616.0" {
		t.Fatal(got)
	}
	if got := comString(&options.primary, 3); got != "" {
		t.Fatal(got)
	}
	if comInt(&options.primary, 9) != 0 {
		t.Fatal("OS SSO enabled")
	}
	second := options.primary.query("{ff85c98a-1ba7-4a6b-90c8-2b752c89e9e2}")
	if second == nil || comInt(second, 3) != 0 {
		t.Fatal("Options2 exclusive access")
	}
	if second.query("{00000000-0000-0000-C000-000000000046}") != &options.primary {
		t.Fatal("IUnknown identity")
	}
	if options.primary.query("{00000000-0000-0000-0000-000000000000}") != nil {
		t.Fatal("unknown interface accepted")
	}
	runtime.KeepAlive(options)
}

func TestHelperLogDoesNotExposeArbitraryErrors(t *testing.T) {
	dir := t.TempDir()
	writeHelperLog(dir, errors.New("password=cookie-secret URL-with-token"))
	path := filepath.Join(dir, "login-helper.log")
	b, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(b), "secret") {
		t.Fatal(string(b), err)
	}
	writeHelperLog(dir, helperFailure("environment create failed", 0x80004005))
	b, err = os.ReadFile(path)
	if err != nil || string(b) != "environment create failed 0x80004005\n" {
		t.Fatal(string(b), err)
	}
}
