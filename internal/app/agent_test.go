package app_test

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// agentZip writes a synthetic Archo Agent bundle the way Windows tar writes one ("./" entries).
func agentZip(t *testing.T, dir, version string, extra map[string]string) string {
	t.Helper()
	p := filepath.Join(dir, "archo-agent-"+version+"-windows-amd64.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	files := map[string]string{
		"./archo-agent.json": `{"name":"archo-agent","version":"` + version + `","pi":"1.4.0","node":"24.22.2","platform":"windows-amd64"}`,
		"./node/node.exe":    "node",
		"./package.json":     "{}",
		"./pi/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js": "cli",
		"./extensions/redact.ts": "export default function () {}",
	}
	for k, v := range extra {
		files[k] = v
	}
	if _, err := w.Create("./"); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		e.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

func writeSum(t *testing.T, zipPath string, sum string) {
	t.Helper()
	if sum == "" {
		b, _ := os.ReadFile(zipPath)
		h := sha256.Sum256(b)
		sum = hex.EncodeToString(h[:])
	}
	if err := os.WriteFile(zipPath+".sha256", []byte(sum+"  "+filepath.Base(zipPath)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAgentInstallFromZip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Archo Agent is built for Windows only")
	}
	e := testkit.New(t)
	a := e.App
	ctx := context.Background()
	dir := t.TempDir()

	if a.AgentStatus().Installed {
		t.Fatal("installed before install")
	}
	// a checksum beside the zip must match
	bad := agentZip(t, dir, "0.1.0", nil)
	writeSum(t, bad, strings.Repeat("0", 64))
	if _, err := a.InstallAgent(ctx, bad, nil); err == nil {
		t.Fatal("installed a zip whose checksum does not match")
	}
	os.Remove(bad + ".sha256")
	// an entry outside the bundle layout is refused and nothing is installed
	evil := agentZip(t, t.TempDir(), "0.1.0", map[string]string{"./evil.exe": "x"})
	if _, err := a.InstallAgent(ctx, evil, nil); err == nil || !strings.Contains(err.Error(), "unexpected top-level entry") {
		t.Fatalf("evil zip: %v", err)
	}
	if a.AgentStatus().Installed {
		t.Fatal("a refused zip left an installation")
	}

	ok := agentZip(t, t.TempDir(), "0.1.0", nil)
	writeSum(t, ok, "")
	m, err := a.InstallAgent(ctx, ok, nil)
	if err != nil || m.Version != "0.1.0" {
		t.Fatalf("install: %+v %v", m, err)
	}
	s := a.AgentStatus()
	if !s.Installed || s.Manifest.Pi != "1.4.0" || !strings.HasPrefix(s.Dir, a.AgentBundlesDir()) {
		t.Fatalf("status after install: %+v", s)
	}
	// reinstalling the same version goes into a fresh folder and switches to it; the old
	// folder stays in use until the new one is complete
	first := s.Dir
	if _, err := a.InstallAgent(ctx, ok, nil); err != nil {
		t.Fatal(err)
	}
	if s := a.AgentStatus(); !s.Installed || s.Dir == first || s.Manifest.Version != "0.1.0" {
		t.Fatalf("status after reinstall: %+v (first %s)", s, first)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("replaced bundle kept: %v", err)
	}
	// a folder left by an interrupted install of the next version does not get in the way
	os.MkdirAll(filepath.Join(a.AgentBundlesDir(), "0.2.0", "node"), 0o755)
	// a newer bundle replaces the older one
	if _, err := a.InstallAgent(ctx, agentZip(t, t.TempDir(), "0.2.0", nil), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.AgentBundlesDir(), "0.1.0")); !os.IsNotExist(err) {
		t.Fatalf("old bundle kept: %v", err)
	}
	if v := a.AgentStatus().Manifest.Version; v != "0.2.0" {
		t.Fatalf("current = %s", v)
	}
	if err := a.RemoveAgent(); err != nil || a.AgentStatus().Installed {
		t.Fatalf("remove: %v", err)
	}
}

func TestAgentConfigNamesTheKeyNeverHoldsIt(t *testing.T) {
	e := testkit.New(t)
	a := e.App
	a.SelfProgram = t.TempDir()
	a.SteamSignedIn = func() (string, bool) { return "fakelogin", true }
	a.SteamSignedInID = func() uint64 { return 76561198000000001 }
	for k, v := range map[string]string{"agent.base_url": "https://api.example.test/v1", "agent.model": "example-model", "agent.thinking": "low"} {
		if err := a.Store.SetMeta("cfg:"+k, v); err != nil {
			t.Fatal(err)
		}
	}
	s := a.AgentStatus()
	if err := a.WriteAgentConfig(s); err != nil {
		t.Fatal(err)
	}
	pi := filepath.Join(a.AgentConfigDir(), "pi")
	read := func(name string) map[string]any {
		b, err := os.ReadFile(filepath.Join(pi, name))
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	prov := read("models.json")["providers"].(map[string]any)["archo-model"].(map[string]any)
	if prov["apiKey"] != "$ARCHO_AGENT_API_KEY" || prov["baseUrl"] != "https://api.example.test/v1" || prov["api"] != "openai-completions" {
		t.Fatalf("provider: %v", prov)
	}
	set := read("settings.json")
	if set["defaultModel"] != "example-model" || set["defaultThinkingLevel"] != "low" || set["enableInstallTelemetry"] != false {
		t.Fatalf("settings: %v", set)
	}
	srv := read("mcp.json")["mcpServers"].(map[string]any)["archo"].(map[string]any)
	if srv["exposure"] != "direct" || !strings.HasPrefix(srv["command"].(string), a.SelfProgram) || srv["args"].([]any)[0] != "mcp" {
		t.Fatalf("mcp: %v", srv)
	}
	vals := read("archo-redact.json")["values"].([]any)
	if len(vals) != 2 || vals[0].(map[string]any)["value"] != "fakelogin" || vals[1].(map[string]any)["placeholder"] != "<steam-id>" {
		t.Fatalf("redact: %v", vals)
	}
	if _, err := os.Stat(filepath.Join(a.AgentConfigDir(), "work")); err != nil {
		t.Fatal(err)
	}
}
