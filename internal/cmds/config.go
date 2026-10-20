package cmds

import (
	"fmt"
	"slices"
	"sort"

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
}

func init() {
	register(&Command{Name: "config get", Args: "[<key>]", Summary: "show settings", MaxArgs: 1, Run: configGetCmd})
	register(&Command{Name: "config set", Args: "<key> <value>", Summary: "change a setting (ui.lang: auto|en|zh; ui.icons: auto|nerd|compat; ui.mouse: on|off; workshop.fetch, workshop.images: on|off; terminal.host: auto|wezterm|none)", MinArgs: 2, MaxArgs: 2, Run: configSetCmd})
}

// ConfigValue returns a setting, or its default.
func ConfigValue(c *Ctx, key string) string {
	if v := c.App.Store.Meta("cfg:" + key); v != "" {
		return v
	}
	return configKeys[key][0]
}

func configGetCmd(c *Ctx, a *Args) (*render.Doc, error) {
	keys := make([]string, 0, len(configKeys))
	for k := range configKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(a.Pos) == 1 {
		if _, ok := configKeys[a.Pos[0]]; !ok {
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
	allowed, ok := configKeys[k]
	if !ok {
		return nil, &UsageError{Msg: fmt.Sprintf("unknown setting %q", k)}
	}
	if !slices.Contains(allowed, v) {
		return nil, &UsageError{Msg: fmt.Sprintf("%s must be one of %v", k, allowed)}
	}
	if err := c.App.Store.SetMeta("cfg:"+k, v); err != nil {
		return nil, err
	}
	d, _ := render.New(fmt.Sprintf("set %s = %s", k, v))
	return d, nil
}
