package app

// Archo Agent (15 §7): a separate download (Node, the Pi coding agent and Archo's Pi extensions) that
// RimArcho installs, configures and starts in a console window. It reaches RimArcho through `archo mcp`.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/netcfg"
	"github.com/ToumanLin/RimArcho/internal/secret"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

const (
	// AgentAPIEnv replaces the Archo Agent release service (VM tests), as UpdateAPIEnv does for RimArcho.
	AgentAPIEnv = "RIMARCHO_AGENT_API"
	// AgentRepoAPI is where Archo Agent bundles are published.
	AgentRepoAPI = "https://api.github.com/repos/ToumanLin/ArchoAgent"
	// AgentKeyName is the Credential Manager name of the model API key (secret.Prefix is added).
	AgentKeyName = "agent-api-key"
	// agentKeyEnv carries the key to the agent process; models.json names it, never the key itself.
	agentKeyEnv = "ARCHO_AGENT_API_KEY"
	// agentMaxTokens is the reply limit written for the configured model; the agent continues a reply
	// that reaches it. Models with a smaller limit reject larger requests, so it stays modest.
	agentMaxTokens = 16384
	agentContext   = 128000
)

// AgentLayout is what an Archo Agent bundle zip may contain (15 §7.1).
var AgentLayout = selfupdate.Layout{
	Files: []string{"archo-agent.cmd", "archo-agent.json", "package.json", "extensions", "node", "pi", "README.md", "LICENSE", "THIRD_PARTY.md"},
	Required: []string{"archo-agent.json", "node/node.exe", "package.json",
		"pi/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js"},
}

// AgentAPIs are the model API formats Archo Agent can use; the first is the default.
var AgentAPIs = []string{"openai-completions", "anthropic-messages", "openai-responses", "google-generative-ai"}

// AgentManifest is archo-agent.json at a bundle's root.
type AgentManifest struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Pi       string `json:"pi"`
	Node     string `json:"node"`
	Platform string `json:"platform"`
}

// AgentStatus is what `archo agent status` and the settings page show.
type AgentStatus struct {
	Installed bool          `json:"installed"`
	Dir       string        `json:"dir,omitempty"`
	Manifest  AgentManifest `json:"manifest"`
	API       string        `json:"api"`
	BaseURL   string        `json:"base_url"`
	Model     string        `json:"model"`
	Thinking  string        `json:"thinking"`
	HasKey    bool          `json:"has_key"`
	ConfigDir string        `json:"config_dir"`
}

// AgentBundlesDir holds installed bundles: <version>/ (<version>-<n>/ after reinstalling the same
// version) and current.txt naming the folder in use.
func (a *App) AgentBundlesDir() string { return filepath.Join(a.LocalDir(), "agent") }

// AgentConfigDir is the agent's own folder in the data directory: Pi's settings, models, MCP server,
// the redaction list, its sessions, and work/ (where it starts).
func (a *App) AgentConfigDir() string { return filepath.Join(a.Home, "agent") }

func (a *App) agentSetting(key, def string) string {
	if v := strings.TrimSpace(a.Store.Meta("cfg:" + key)); v != "" {
		return v
	}
	return def
}

// AgentStatus reads the installed bundle and the settings; it never contacts anything.
func (a *App) AgentStatus() AgentStatus {
	s := AgentStatus{
		API:       a.agentSetting("agent.api", AgentAPIs[0]),
		BaseURL:   a.agentSetting("agent.base_url", ""),
		Model:     a.agentSetting("agent.model", ""),
		Thinking:  a.agentSetting("agent.thinking", "high"),
		ConfigDir: a.AgentConfigDir(),
	}
	if dir, m, err := a.agentCurrent(); err == nil {
		s.Installed, s.Dir, s.Manifest = true, dir, m
	}
	if k, err := secret.Get(AgentKeyName); err == nil && k != "" {
		s.HasKey = true
	}
	return s
}

func (a *App) agentCurrent() (string, AgentManifest, error) {
	var m AgentManifest
	b, err := os.ReadFile(filepath.Join(a.AgentBundlesDir(), "current.txt"))
	if err != nil {
		return "", m, err
	}
	v := strings.TrimSpace(string(b))
	if v == "" || strings.ContainsAny(v, `/\:`) || v == "." || v == ".." {
		return "", m, errors.New("current.txt names no version")
	}
	dir := filepath.Join(a.AgentBundlesDir(), v)
	if m, err = readAgentManifest(dir); err != nil {
		return "", m, err
	}
	return dir, m, nil
}

func readAgentManifest(dir string) (AgentManifest, error) {
	var m AgentManifest
	b, err := os.ReadFile(filepath.Join(dir, "archo-agent.json"))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("archo-agent.json: %w", err)
	}
	if _, ok := selfupdate.Compare(m.Version, m.Version); m.Name != "archo-agent" || !ok {
		return m, errors.New("archo-agent.json is not an Archo Agent manifest")
	}
	return m, nil
}

// SetAgentKey stores the model API key in Credential Manager; "" removes it.
func SetAgentKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return secret.Delete(AgentKeyName)
	}
	return secret.Set(AgentKeyName, key)
}

func (a *App) agentClient() *selfupdate.Client {
	api := os.Getenv(AgentAPIEnv)
	if api == "" {
		api = AgentRepoAPI
	}
	c := &selfupdate.Client{API: api, UserAgent: UserAgent, Asset: AgentZipName}
	if a.HTTP != nil {
		c.HTTP = a.HTTP
	}
	return c
}

// AgentZipName names a bundle release asset; only Windows x64 is built.
func AgentZipName(version, goos, goarch string) string {
	return "archo-agent-" + version + "-" + goos + "-" + goarch + ".zip"
}

// CheckAgent returns the latest published bundle (nil when its assets are missing).
func (a *App) CheckAgent(ctx context.Context) (*selfupdate.Release, error) {
	// Release metadata never comes through an automatic mirror (17 §2.2, §5.1).
	r, err := a.agentClient().Latest(netcfg.WithoutAutoMirror(ctx), false)
	if err != nil {
		return nil, err
	}
	if r.Zip.URL == "" || r.Sum.URL == "" {
		return r, fmt.Errorf("release %s has no %s and checksum", r.Version, AgentZipName(r.Version, runtime.GOOS, runtime.GOARCH))
	}
	return r, nil
}

// InstallAgent downloads the latest bundle (or, with zipPath, uses that file and the .sha256 beside it
// when there is one), verifies and extracts it, and makes it current. step reports progress.
func (a *App) InstallAgent(ctx context.Context, zipPath string, step func(phase string, frac float64)) (AgentManifest, error) {
	if runtime.GOOS != "windows" {
		return AgentManifest{}, errors.New("Archo Agent is only built for Windows")
	}
	if step == nil {
		step = func(string, float64) {}
	}
	base := a.AgentBundlesDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		return AgentManifest{}, err
	}
	if zipPath == "" {
		step("check", 0)
		r, err := a.CheckAgent(ctx)
		if err != nil {
			return AgentManifest{}, err
		}
		if _, m, err := a.agentCurrent(); err == nil && !selfupdate.Newer(m.Version, r.Version) {
			return m, nil
		}
		step("download", 0)
		// The checksum comes without an automatic mirror; the zip may come through one
		// and must match it (17 §2.2).
		sum, err := a.agentClient().Checksum(netcfg.WithoutAutoMirror(ctx), r)
		if err != nil {
			return AgentManifest{}, err
		}
		zipPath, err = a.agentClient().DownloadWithChecksum(ctx, r, filepath.Join(base, "download"), sum, func(done, total int64) {
			if total > 0 {
				step("download", float64(done)/float64(total))
			}
		})
		if err != nil {
			return AgentManifest{}, err
		}
	} else if sum, err := os.ReadFile(zipPath + ".sha256"); err == nil {
		step("verify", 0)
		info, err := os.Stat(zipPath)
		if err != nil {
			return AgentManifest{}, err
		}
		if err := selfupdate.VerifyLocal(ctx, zipPath, filepath.Base(zipPath), sum, info.Size()); err != nil {
			return AgentManifest{}, err
		}
	}
	step("extract", 0)
	tmp := filepath.Join(base, "new")
	_ = os.RemoveAll(tmp)
	if err := selfupdate.ExtractLayout(zipPath, tmp, AgentLayout); err != nil {
		return AgentManifest{}, err
	}
	m, err := readAgentManifest(tmp)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return m, err
	}
	if strings.ContainsAny(m.Version, `/\:`) {
		_ = os.RemoveAll(tmp)
		return m, fmt.Errorf("archo-agent.json names version %q", m.Version)
	}
	// The new bundle goes into a folder nothing points to yet (a reinstall of the same version
	// gets <version>-<n>); current.txt then switches to it in one write. A crash before that
	// leaves the bundle in use as it was.
	name := m.Version
	for n := 2; ; n++ {
		if _, err := os.Lstat(filepath.Join(base, name)); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s-%d", m.Version, n)
	}
	if err := os.Rename(tmp, filepath.Join(base, name)); err != nil {
		_ = os.RemoveAll(tmp)
		return m, err
	}
	if err := writeFileAtomic(filepath.Join(base, "current.txt"), []byte(name+"\n")); err != nil {
		_ = os.RemoveAll(filepath.Join(base, name))
		return m, err
	}
	// older bundles and the download go; one still running stays until next time
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if e.IsDir() && e.Name() != name {
			_ = os.RemoveAll(filepath.Join(base, e.Name()))
		}
	}
	step("done", 1)
	return m, nil
}

// RemoveAgent deletes the installed bundles; settings, the key and the agent's folder in the data
// directory stay (Uninstall removes those, 02 §2).
func (a *App) RemoveAgent() error {
	if err := os.RemoveAll(a.AgentBundlesDir()); err != nil {
		return fmt.Errorf("Archo Agent may still be running; close it and try again: %w", err)
	}
	return nil
}

// agentReady returns the bundle and key, or an error saying what the player has to do first.
func (a *App) agentReady() (string, string, AgentStatus, error) {
	s := a.AgentStatus()
	switch {
	case !s.Installed:
		return "", "", s, errors.New("Archo Agent is not installed: install it in Settings, or run `archo agent install`")
	case s.BaseURL == "" || s.Model == "":
		return "", "", s, errors.New("no model is set: set agent.base_url and agent.model in Settings, or with `archo config set`")
	}
	key, err := secret.Get(AgentKeyName)
	if err != nil || key == "" {
		return "", "", s, errors.New("no API key is set: set it in Settings, or with `archo agent key`")
	}
	return s.Dir, key, s, nil
}

// WriteAgentConfig writes Pi's files for the current settings (15 §7.2). The key is never written:
// models.json names the environment variable that carries it.
func (a *App) WriteAgentConfig(s AgentStatus) error {
	programDir, err := a.SelfProgramDir()
	if err != nil {
		return err
	}
	cfg := a.AgentConfigDir()
	pi := filepath.Join(cfg, "pi")
	if err := os.MkdirAll(filepath.Join(cfg, "work"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(pi, 0o755); err != nil {
		return err
	}
	model := map[string]any{"id": s.Model, "name": s.Model, "reasoning": s.Thinking != "off",
		"input": []string{"text"}, "contextWindow": agentContext, "maxTokens": agentMaxTokens}
	files := map[string]any{
		"models.json": map[string]any{"providers": map[string]any{"archo-model": map[string]any{
			"name": "Archo model", "baseUrl": s.BaseURL, "api": s.API, "apiKey": "$" + agentKeyEnv,
			"models": []any{model}}}},
		"settings.json": map[string]any{"defaultProvider": "archo-model", "defaultModel": s.Model,
			"defaultThinkingLevel": s.Thinking, "quietStartup": "header", "enableInstallTelemetry": false},
		"mcp.json": map[string]any{"mcpServers": map[string]any{"archo": map[string]any{
			"command": filepath.Join(programDir, archoExe()), "args": []string{"mcp"}, "exposure": "direct",
			"description": "RimArcho: the player's RimWorld installation, Mods, plans, logs and saves"}}},
		"archo-redact.json": map[string]any{"values": a.agentRedactValues()},
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := json.MarshalIndent(files[n], "", "  ")
		if err != nil {
			return err
		}
		if err := writeFileAtomic(filepath.Join(pi, n), append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func archoExe() string {
	if runtime.GOOS == "windows" {
		return "archo.exe"
	}
	return "archo"
}

type redactValue struct {
	Value       string `json:"value"`
	Placeholder string `json:"placeholder"`
}

// agentRedactValues are the private values RimArcho knows besides the Windows user name (15 §3.2).
func (a *App) agentRedactValues() []redactValue {
	var out []redactValue
	seen := map[string]bool{}
	add := func(v, p string) {
		v = strings.TrimSpace(v)
		if v == "" || strings.EqualFold(v, "anonymous") || seen[strings.ToLower(v)] {
			return
		}
		seen[strings.ToLower(v)] = true
		out = append(out, redactValue{v, p})
	}
	signedIn, signedInID := a.SteamSignedIn, a.SteamSignedInID
	if signedIn == nil {
		signedIn = env.SteamSignedIn
	}
	if signedInID == nil {
		signedInID = env.SteamSignedInID
	}
	if name, ok := signedIn(); ok {
		add(name, "<steam-account>")
	}
	if id := signedInID(); id != 0 {
		add(fmt.Sprint(id), "<steam-id>")
	}
	add(a.Store.Meta("cfg:steamcmd.account"), "<steam-account>")
	return out
}

// RunAgent writes the configuration and starts Archo Agent. With console, it opens in a new console
// window and RunAgent returns at once (GUI); otherwise it runs in this terminal until it exits (CLI).
func (a *App) RunAgent(console bool, stdin io.Reader, stdout, stderr io.Writer) error {
	dir, key, s, err := a.agentReady()
	if err != nil {
		return err
	}
	if err := a.WriteAgentConfig(s); err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(dir, "node", "node.exe"),
		filepath.Join(dir, "pi", "node_modules", "@earendil-works", "pi-coding-agent", "dist", "bundle", "cli.js"),
		"-e", dir)
	cmd.Dir = filepath.Join(a.AgentConfigDir(), "work")
	cmd.Env = append(os.Environ(),
		"PI_CODING_AGENT_DIR="+filepath.Join(a.AgentConfigDir(), "pi"),
		agentKeyEnv+"="+key,
		"PI_SKIP_VERSION_CHECK=1", "PI_TELEMETRY=0", "PI_OFFLINE=1",
		"RIMARCHO_HOME="+a.Home)
	if console {
		return startAgentConsole(cmd.Path, cmd.Args[1:], cmd.Dir, cmd.Env)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return cmd.Run()
}

// CheckAgentBaseURL accepts an http(s) URL with a host.
func CheckAgentBaseURL(v string) error {
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("expected an http:// or https:// URL")
	}
	return nil
}

// writeFileAtomic writes through a temporary file in the same folder, so a reader never sees half a file.
func writeFileAtomic(name string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
