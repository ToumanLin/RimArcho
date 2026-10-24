package source_test

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"golang.org/x/sys/windows"
)

func TestLinkHealthOfflineVolumeThroughJunction(t *testing.T) {
	drives, err := windows.GetLogicalDrives()
	if err != nil {
		t.Fatal(err)
	}
	missing := ""
	for letter := 'Z'; letter >= 'A'; letter-- {
		if drives&(1<<uint(letter-'A')) == 0 {
			missing = fmt.Sprintf(`%c:\offline-mod`, letter)
			break
		}
	}
	if missing == "" {
		t.Skip("no unused drive letter to model an offline volume")
	}
	e := testkit.New(t)
	e.AddEnv(false)
	ancestor := filepath.Join(e.Root, "offline-alias")
	if err := source.CreateLink(ancestor, missing); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(e.Game, "Mods", "_Archo_offline")
	if err := source.CreateLink(link, filepath.Join(ancestor, "mod")); err != nil {
		t.Fatal(err)
	}
	l, err := source.ReadLink(link)
	if err != nil || l.State != source.TargetUnknown {
		t.Fatalf("offline target misclassified: %+v %v", l, err)
	}
	e.MustRun("links", "repair", "--all", "--yes", "--include", link)
	if !source.IsLink(link) {
		t.Fatal("removed an offline-volume link")
	}
}

func TestLinkHealthInaccessibleAncestor(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(true)
	target := filepath.Join(e.Root, "offline")
	e.MkdirAll(target)
	link := filepath.Join(e.Game, "Mods", "_Archo_inaccessible")
	e.Junction(link, filepath.Join(target, "missing"))
	ptr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(ptr, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	l, err := source.ReadLink(link)
	if err != nil || l.State != source.TargetUnknown || l.RawTarget == "" {
		t.Fatalf("inaccessible misclassified: %+v %v", l, err)
	}
	links, err := e.App.InspectGameLinks()
	if err != nil || len(links) != 1 || links[0].Code != app.CodeLinkUnknown || links[0].Action != "keep" {
		t.Fatalf("inspection: %+v %v", links, err)
	}
	e.MustRun("links", "repair", "--all", "--yes", "--include", link)
	if !source.IsLink(link) {
		t.Fatal("inaccessible link removed")
	}
}

func TestLinkHealthUnknownReparseType(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(true)
	path := filepath.Join(e.Game, "Mods", "_Archo_unknown")
	e.MkdirAll(path)
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(ptr, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	// Non-Microsoft tags carry a GUID; NTFS accepts application-defined tags.
	data := make([]byte, 24)
	binary.LittleEndian.PutUint32(data, 0x42)
	data[8] = 42
	var n uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &n, nil); err != nil {
		t.Skipf("custom reparse tag unavailable: %v", err)
	}
	defer func() {
		if err := windows.DeviceIoControl(h, windows.FSCTL_DELETE_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &n, nil); err != nil {
			t.Errorf("remove synthetic reparse metadata: %v", err)
		}
	}()
	l, err := source.ReadLink(path)
	if err != nil || l.Kind != "unknown" || l.State != source.TargetUnknown {
		t.Fatalf("unknown reparse: %+v %v", l, err)
	}
	links, err := e.App.InspectGameLinks()
	if err != nil || len(links) != 1 || links[0].Code != app.CodeLinkUnknown {
		t.Fatalf("inspection: %+v %v", links, err)
	}
	if err := source.RemoveLinkChecked(l); err == nil {
		t.Fatal("unknown reparse type removed")
	}
	e.MustRun("links", "repair", "--all", "--yes", "--include", path)
	if _, err := os.Lstat(path); err != nil {
		t.Fatal("unknown reparse entry removed")
	}
}
