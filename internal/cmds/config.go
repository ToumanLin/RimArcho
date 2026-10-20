package cmds

import (
	"fmt"
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
	"ui.icons": {"auto", "nerd", "compat"}, // auto: Nerd Font icons in the WezTerm host (06 §1.3)
	"ui.mouse": {"on", "off"},
	// 06 §6.4: whether descriptions and their images may be fetched from the Steam Workshop
	"workshop.fetch":  {"on", "off"},
	"workshop.images": {"on", "off"},
	// 06 §1.1: auto asks once whether to download WezTerm; none (a "no") runs in the
	// current terminal; wezterm always uses the host.
	"terminal.host": {"auto", "wezterm", "none"},
	// 06 §12.4: off draws no images at all in the TUI
	"ui.previews": {"on", "off"},
}

// freeKeys are settings with free values: their default and a check (06 §12.3, §12.4).
var freeKeys = map[string]struct {
	def   string
	check func(string) error
}{
	// the tags offered first by the tag bar, comma separated; the preset is the default
	"tags.pinned": {DefaultPinnedTags, func(string) error { return nil }},
	// how long the cursor must rest on a mod before its thumbnail is drawn
	"ui.preview_dwell_ms": {"0", func(v string) error {
		if n, err := strconv.Atoi(v); err != nil || n < 0 || n > 5000 {
			return fmt.Errorf("ui.preview_dwell_ms must be a number of milliseconds from 0 to 5000")
		}
		return nil
	}},
}

// DefaultPinnedTags is the preset of common tags (06 §12.3).
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
	register(&Command{Name: "config set", Args: "<key> <value>", Summary: "change a setting (ui.lang: auto|en|zh; ui.icons: auto|nerd|compat; ui.mouse: on|off; workshop.fetch, workshop.images: on|off; terminal.host: auto|wezterm|none; ui.previews: on|off; ui.preview_dwell_ms: 0–5000; tags.pinned: comma-separated tags)", MinArgs: 2, MaxArgs: 2, Run: configSetCmd})
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
