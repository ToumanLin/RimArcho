package vdf

import "testing"

func TestParse(t *testing.T) {
	n, err := Parse([]byte(`"libraryfolders"
{
	// comment
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"apps" { "294100" "1" }
	}
	"1"
	{
		"path"		"D:\\SteamLibrary"
	}
}`))
	if err != nil {
		t.Fatal(err)
	}
	lf := n.Get("LibraryFolders")
	if lf == nil || len(lf.Children) != 2 {
		t.Fatalf("children: %+v", lf)
	}
	if got := lf.Str("0", "path"); got != `C:\Program Files (x86)\Steam` {
		t.Fatalf("path = %q", got)
	}
	if got := lf.Str("0", "apps", "294100"); got != "1" {
		t.Fatalf("apps = %q", got)
	}
	if _, err := Parse([]byte(`"a" { "b" "c"`)); err == nil {
		t.Fatal("unterminated object accepted")
	}
}
