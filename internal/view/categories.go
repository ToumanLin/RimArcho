package view

import "github.com/ToumanLin/RimArcho/internal/app"

// Mod categories in the interface (16 §2.5): the row's marker, the category: filter and the
// details' field. The rules are app.CategoryOf.

// SetCategory sets or (key "") clears the player's category of a Mod and says so.
func SetCategory(a *app.App, ref, key string) (string, error) {
	m, err := a.SetCategory(ref, key)
	if err != nil {
		return "", err
	}
	if key == "" {
		return Tf("%s: automatic category", m.DisplayName()), nil
	}
	return Tf("%s: %s", m.DisplayName(), CategoryText(key)), nil
}

// CategoryText is a category's name.
func CategoryText(k string) string {
	switch k {
	case "game":
		return T("game & DLC")
	case "library":
		return T("library")
	case "content":
		return T("content")
	case "code":
		return T("code")
	case "patches":
		return T("patches")
	case "translation":
		return T("translation")
	case "textures":
		return T("textures")
	}
	return T("none")
}

// CategoryWhy is the category's reason: the facts of the automatic one, or that the player
// chose it (and what the automatic one would be).
func CategoryWhy(c app.Category) string {
	if c.Player {
		return Tf("set by you; automatic: %s", CategoryText(c.Auto))
	}
	return app.CategoryReasonWith(c.Auto, c.Why, Tf)
}

// CategoryField is the details' category row: List is the key, then "player" when the
// player chose it.
func CategoryField(c app.Category, id string) Field {
	f := Field{Kind: FieldCategory, Label: T("category"), Value: CategoryText(c.Key), Extra: CategoryWhy(c), Target: id}
	switch {
	case c.Player:
		f.List = []string{c.Key, "player"}
	case c.Key != "":
		f.List = []string{c.Key}
	}
	return f
}
