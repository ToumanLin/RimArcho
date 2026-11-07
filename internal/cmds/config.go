package cmds

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/l10ndata"
	"github.com/ToumanLin/RimArcho/internal/netcfg"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// configKeys are the user settings stored in the database (meta "cfg:<key>"), with their
// allowed values; the first is the default.
var configKeys = map[string][]string{
	"updates.source":   {"auto", "github"},
	"updates.auto":     {"on", "off"},
	"updates.interval": {"normal", "frequent", "relaxed"},
	"ui.accent":        {"default", "indigo", "blue", "cyan", "teal", "green", "grass", "orange", "amber", "red", "crimson", "pink", "plum", "purple", "violet", "iris"},
	"ui.lang":          l10ndata.LanguageCodes(),
	"ui.theme":         {"auto", "light", "dark"},
	"ui.icons":         {"compat", "nerd"}, // nerd: the player's terminal already uses a Nerd Font
	"ui.mouse":         {"on", "off"},
	// 16 §4.3: whether descriptions and their images may be fetched from the Steam Workshop
	"workshop.fetch":  {"on", "off"},
	"workshop.images": {"on", "off"},
	// 07 §1.5: add Archo Monitor to new plans (and to each plan once on upgrade)
	"monitor.default": {"on", "off"},
	// 16 §2.14: off starts the window's WebView2 without GPU acceleration (less memory,
	// software drawing); read when the window opens
	"ui.gpu":          {"on", "off"},
	"ui.archo.motion": {"live", "still", "off"},
}

// freeKeys are settings with free values: their default and a check (16 §3.8, §2.14).
var freeKeys = map[string]struct {
	def   string
	check func(string) error
}{
	// Empty uses anonymous; passwords and Steam Guard codes stay in SteamCMD.
	"steamcmd.account": {"", app.ValidSteamCMDAccount},
	"local.dir": {"", func(v string) error {
		if !filepath.IsAbs(v) {
			return fmt.Errorf("local.dir must be an absolute folder path")
		}
		return nil
	}},
	// a steamcmd the player already has; "" finds RimArcho's own copy, then PATH
	"steamcmd.path": {"", func(v string) error {
		if v != "" && !filepath.IsAbs(v) {
			return fmt.Errorf("steamcmd.path must be an absolute program path")
		}
		return nil
	}},
	// the tags offered first by the tag bar, comma separated; the preset is the default
	"tags.pinned": {DefaultPinnedTags, func(string) error { return nil }},
	// the lock library (07 §5): "" is %LOCALAPPDATA%\RimArcho\locked
	"lock.dir": {"", func(v string) error {
		if !filepath.IsAbs(v) {
			return fmt.Errorf("lock.dir must be an absolute folder path")
		}
		return nil
	}},
	// the Archo Monitor folder (19 §6): "" is %LOCALAPPDATA%\RimArcho\monitor
	"monitor.dir": {"", func(v string) error {
		if !filepath.IsAbs(v) {
			return fmt.Errorf("monitor.dir must be an absolute folder path")
		}
		return nil
	}},
	"subscribed.dir": {"", func(v string) error {
		if !filepath.IsAbs(v) {
			return fmt.Errorf("subscribed.dir must be an absolute folder path")
		}
		return nil
	}},
	// 17 §2.2: RimArcho's own requests. system: HTTPS_PROXY / HTTP_PROXY, then the Windows
	// system proxy; off; or a proxy URL.
	"network.proxy": {"system", netcfg.CheckProxy},
	// a GitHub mirror tried before GitHub itself; "" is direct
	"network.github_mirror": {"", netcfg.CheckMirror},
	"steamcmd-mods.dir": {"", func(v string) error {
		if !filepath.IsAbs(v) {
			return fmt.Errorf("steamcmd-mods.dir must be an absolute folder path")
		}
		return nil
	}},
}

// DefaultPinnedTags is the preset of common tags (16 §3.8).
var DefaultPinnedTags = l10ndata.DefaultPinnedTags

// Setting returns a setting of the app, or its default.
func Setting(a *app.App, key string) string {
	v := a.Store.Meta("cfg:" + key)
	if vs := configKeys[key]; len(vs) > 0 {
		if slices.Contains(vs, v) {
			return v
		}
		return vs[0]
	}
	if v != "" {
		return v
	}
	if f, ok := freeKeys[key]; ok {
		return f.def
	}
	return ""
}

// SettingChoices returns the allowed values in display order, with the default first.
func SettingChoices(key string) []string { return slices.Clone(configKeys[key]) }

// SettingKeys are every setting `config get` lists, sorted.
func SettingKeys() []string {
	keys := make([]string, 0, len(configKeys)+len(freeKeys))
	for k := range configKeys {
		keys = append(keys, k)
	}
	for k := range freeKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	register(&Command{Name: "config get", Args: "[<key>]", Summary: "show settings", MaxArgs: 1, Run: configGetCmd})
	register(&Command{Name: "config set", Args: "<key> <value>", Summary: "change a setting (local.dir: unused managed-folder root; updates.source: auto|github; updates.auto: on|off; updates.interval: normal|frequent|relaxed; ui.lang: auto or a bundled locale; ui.theme: auto|light|dark; ui.accent: default|indigo|blue|cyan|teal|green|grass|orange|amber|red|crimson|pink|plum|purple|violet|iris; ui.icons: compat|nerd; ui.mouse: on|off; workshop.fetch, workshop.images: on|off; ui.gpu: on|off; ui.archo.motion: off|still|live; tags.pinned: comma-separated tags; monitor.default: on|off; monitor.dir: a folder; steamcmd.account: optional account name, empty or anonymous for anonymous login; steamcmd.path: a steamcmd program; network.proxy: system|off|a proxy URL; network.github_mirror: empty or a mirror prefix such as https://gh-proxy.org/)", MinArgs: 2, MaxArgs: 2, Run: configSetCmd})
}

// ConfigValue returns a setting, or its default.
func ConfigValue(c *Ctx, key string) string { return Setting(c.App, key) }

func configGetCmd(c *Ctx, a *Args) (*render.Doc, error) {
	keys := SettingKeys()
	if len(a.Pos) == 1 {
		if !slices.Contains(keys, a.Pos[0]) {
			return nil, &UsageError{Msg: fmt.Sprintf("unknown setting %q (known: %v)", a.Pos[0], keys)}
		}
		keys = a.Pos
	}
	d, root := render.New("settings")
	for _, k := range keys {
		root.KV(k, ConfigValue(c, k))
	}
	return d, nil
}

func configSetCmd(c *Ctx, a *Args) (*render.Doc, error) {
	k, v := a.Pos[0], a.Pos[1]
	if f, ok := freeKeys[k]; ok {
		if err := f.check(v); err != nil {
			return nil, &UsageError{Msg: err.Error()}
		}
	} else {
		allowed, ok := configKeys[k]
		if !ok {
			return nil, &UsageError{Msg: fmt.Sprintf("unknown setting %q", k)}
		}
		if !slices.Contains(allowed, v) {
			return nil, &UsageError{Msg: fmt.Sprintf("%s must be one of %v", k, allowed)}
		}
	}
	if k == "local.dir" {
		if err := c.App.SetManagedRoot(v); err != nil {
			return nil, err
		}
	} else if err := c.App.Store.SetMeta("cfg:"+k, v); err != nil {
		return nil, err
	}
	if strings.HasPrefix(k, "network.") {
		c.App.ApplyNetwork()
	}
	d, _ := render.New(fmt.Sprintf("set %s = %s", k, v))
	return d, nil
}
