package view

// Mod categories in the interface (16 §2.5): the row's marker and the category: filter. The rules are app.CategoryOf.

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
