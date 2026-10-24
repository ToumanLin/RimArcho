package cmds

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// configKeys are the user settings stored in the database (meta "cfg:<key>"), with their
// allowed values; the first is the default.
var configKeys = map[string][]string{
	"ui.lang":  {"auto", "en", "zh"},
	"ui.icons": {"auto", "nerd", "compat"}, // auto: Nerd Font icons in the WezTerm host (06 §9.3)
	"ui.mouse": {"on", "off"},
	// 06 §4.3: whether descriptions and their images may be fetched from the Steam Workshop
	"workshop.fetch":  {"on", "off"},
	"workshop.images": {"on", "off"},
	// 06 §9.2: auto asks once whether to download WezTerm; none (a "no") runs in the
	// current terminal; wezterm always uses the host.
	"terminal.host": {"auto", "wezterm", "none"},
	// 06 §9.4: off draws no images at all in the TUI
	"ui.previews": {"on", "off"},
	// 04 §1.5: add Archo Monitor to new plans (and to each plan once on upgrade)
	"monitor.default": {"on", "off"},
	// 06 §2.14: off starts the window's WebView2 without GPU acceleration (less memory,
	// software drawing); read when the window opens
	"ui.gpu": {"on", "off"},
}

// freeKeys are settings with free values: their default and a check (06 §3.8, §12.4).
var freeKeys = map[string]struct {
	def   string
	check func(string) error
}{
	// Empty uses anonymous; passwords and Steam Guard codes stay in SteamCMD.
	"steamcmd.account": {"", app.ValidSteamCMDAccount},
	// a steamcmd the player already has; "" finds RimArcho's own copy, then PATH
	"steamcmd.path": {"", func(v string) error {
		if v != "" && !filepath.IsAbs(v) {
			return fmt.Errorf("steamcmd.path must be an absolute program path")
		}
		return nil
	}},
	// the tags offered first by the tag bar, comma separated; the preset is the default
	"tags.pinned": {DefaultPinnedTags, func(string) error { return nil }},
	// the lock library (04 §7): "" is %LOCALAPPDATA%\RimArcho\locked
	"lock.dir": {"", func(v string) error {
		if !filepath.IsAbs(v) {
			return fmt.Errorf("lock.dir must be an absolute folder path")
		}
		return nil
	}},
	// the Archo Monitor folder (08 §9): "" is %LOCALAPPDATA%\RimArcho\monitor
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
	// how long the cursor must rest on a mod before its thumbnail is drawn
	"ui.preview_dwell_ms": {"0", func(v string) error {
		if n, err := strconv.Atoi(v); err != nil || n < 0 || n > 5000 {
			return fmt.Errorf("ui.preview_dwell_ms must be a number of milliseconds from 0 to 5000")
		}
		return nil
	}},
}

// DefaultPinnedTags is the preset of common tags (06 §3.8).
const DefaultPinnedTags = "框架,美化,服装,种族,武器,建筑,家具,QoL,平衡,剧情,性能,语言包,测试中,待删除"

// Setting returns a setting of the app, or its default.
func Setting(a *app.App, key string) string {
	if v := a.Store.Meta("cfg:" + key); v != "" {
		return v
	}
	if f, ok := freeKeys[key]; ok {
		return f.def
	}
	if vs := configKeys[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func init() {
	register(&Command{Name: "config get", Args: "[<key>]", Summary: "show settings", MaxArgs: 1, Run: configGetCmd})
	register(&Command{Name: "config set", Args: "<key> <value>", Summary: "change a setting (ui.lang: auto|en|zh; ui.icons: auto|nerd|compat; ui.mouse: on|off; workshop.fetch, workshop.images: on|off; terminal.host: auto|wezterm|none; ui.previews: on|off; ui.gpu: on|off; ui.preview_dwell_ms: 0–5000; tags.pinned: comma-separated tags; monitor.default: on|off; monitor.dir: a folder; steamcmd.account: optional account name, empty or anonymous for anonymous login; steamcmd.path: a steamcmd program)", MinArgs: 2, MaxArgs: 2, Run: configSetCmd})
}

// ConfigValue returns a setting, or its default.
func ConfigValue(c *Ctx, key string) string { return Setting(c.App, key) }

func configGetCmd(c *Ctx, a *Args) (*render.Doc, error) {
	keys := make([]string, 0, len(configKeys)+len(freeKeys))
	for k := range configKeys {
		keys = append(keys, k)
	}
	for k := range freeKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
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
	if err := c.App.Store.SetMeta("cfg:"+k, v); err != nil {
		return nil, err
	}
	d, _ := render.New(fmt.Sprintf("set %s = %s", k, v))
	return d, nil
}
