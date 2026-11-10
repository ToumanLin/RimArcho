package selfupdate_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

func TestInWorkshopFolder(t *testing.T) {
	for _, tc := range []struct{ path, item string }{
		{`D:/SteamLibrary/steamapps/workshop/content/294100/123`, `D:/SteamLibrary/steamapps/workshop/content/294100/123`},
		{`E:/Games/SteamApps/Workshop/Content/294100/000123/release/`, `E:/Games/SteamApps/Workshop/Content/294100/000123`},
		{`C:\Program Files (x86)\Steam\steamapps\workshop\content\294100\456\`, `C:/Program Files (x86)/Steam/steamapps/workshop/content/294100/456`},
		{`//server/library/steamapps/workshop/content/294100/9/app`, `//server/library/steamapps/workshop/content/294100/9`},
		{`D:/steamapps/workshop/content/294100/123evil`, ""},
		{`D:/steamapps/workshop/content/294100`, ""},
		{`D:/steamapps/workshop/content/294100/-1`, ""},
		{`D:/steamapps/workshop/content/123/456`, ""},
		{`D:/notsteamapps/workshop/content/294100/123`, ""},
		{`D:/steamapps/workshop/content/294100/123/../../elsewhere`, ""},
	} {
		item, ok := selfupdate.InWorkshopFolder(tc.path)
		if ok != (tc.item != "") || !strings.EqualFold(filepath.Clean(item), filepath.Clean(filepath.FromSlash(tc.item))) {
			t.Errorf("%q = %q,%v; want %q", tc.path, item, ok, tc.item)
		}
	}
}
