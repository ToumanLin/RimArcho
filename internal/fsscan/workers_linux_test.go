package fsscan

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeSys builds a synthetic sysfs and /dev: an HDD sda with partition sda1, an NVMe SSD
// with partition nvme0n1p2, dm-0 (LVM on the SSD), dm-1 (on the HDD, reporting 0 itself as
// device-mapper may), md0 over both, and loop0 without queue/rotational.
func fakeSys(t *testing.T) (sys, dev string) {
	t.Helper()
	root := t.TempDir()
	sys, dev = filepath.Join(root, "sys"), filepath.Join(root, "dev")
	file := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
	}
	pci := filepath.Join(sys, "devices", "pci0000:00", "block")
	virt := filepath.Join(sys, "devices", "virtual", "block")
	file(filepath.Join(pci, "sda", "queue", "rotational"), "1\n")
	file(filepath.Join(pci, "sda", "sda1", "partition"), "1\n")
	file(filepath.Join(pci, "nvme0n1", "queue", "rotational"), "0\n")
	file(filepath.Join(pci, "nvme0n1", "nvme0n1p2", "partition"), "2\n")
	file(filepath.Join(virt, "dm-0", "queue", "rotational"), "1\n")
	link(filepath.Join(pci, "nvme0n1", "nvme0n1p2"), filepath.Join(virt, "dm-0", "slaves", "nvme0n1p2"))
	file(filepath.Join(virt, "dm-1", "queue", "rotational"), "0\n")
	link(filepath.Join(pci, "sda", "sda1"), filepath.Join(virt, "dm-1", "slaves", "sda1"))
	link(filepath.Join(pci, "sda", "sda1"), filepath.Join(virt, "md0", "slaves", "sda1"))
	link(filepath.Join(pci, "nvme0n1", "nvme0n1p2"), filepath.Join(virt, "md0", "slaves", "nvme0n1p2"))
	file(filepath.Join(virt, "loop0", "size"), "0\n")
	for name, target := range map[string]string{
		"8:1": filepath.Join(pci, "sda", "sda1"), "8:0": filepath.Join(pci, "sda"),
		"259:2": filepath.Join(pci, "nvme0n1", "nvme0n1p2"), "253:0": filepath.Join(virt, "dm-0"),
		"253:1": filepath.Join(virt, "dm-1"), "9:0": filepath.Join(virt, "md0"), "7:0": filepath.Join(virt, "loop0"),
	} {
		link(target, filepath.Join(sys, "dev", "block", name))
		link(target, filepath.Join(sys, "class", "block", filepath.Base(target)))
	}
	file(filepath.Join(dev, "dm-0"), "")
	link(filepath.Join(dev, "dm-0"), filepath.Join(dev, "mapper", "ubuntu--vg-root"))
	return sys, dev
}

func TestLinuxDeviceKind(t *testing.T) {
	t.Parallel()
	sys, dev := fakeSys(t)
	for _, c := range []struct {
		major, minor uint32
		want         diskKind
	}{
		{8, 1, diskRotating},   // HDD partition
		{8, 0, diskRotating},   // whole HDD
		{259, 2, diskSSD},      // NVMe partition
		{253, 0, diskSSD},      // LVM on the SSD, though dm-0 itself says 1
		{253, 1, diskRotating}, // dm on the HDD, though dm-1 itself says 0
		{9, 0, diskRotating},   // md over an HDD and an SSD
		{7, 0, diskUnknown},    // no queue/rotational
		{0, 45, diskUnknown},   // anonymous device (btrfs subvolume, tmpfs, overlay)
		{8, 99, diskUnknown},   // not in sysfs
	} {
		if got := deviceKind(sys, dev, c.major, c.minor); got != c.want {
			t.Errorf("%d:%d: %d, want %d", c.major, c.minor, got, c.want)
		}
	}
	for source, want := range map[string]diskKind{
		"/dev/mapper/ubuntu--vg-root": diskSSD, "/dev/sda1": diskRotating, "/dev/nvme0n1p2": diskSSD,
		"tmpfs": diskUnknown, "server:/export": diskUnknown, "/dev/missing": diskUnknown, "": diskUnknown,
	} {
		if got := sourceKind(sys, dev, source); got != want {
			t.Errorf("source %q: %d, want %d", source, got, want)
		}
	}
}

func TestLinuxMountSource(t *testing.T) {
	t.Parallel()
	info := filepath.Join(t.TempDir(), "mountinfo")
	body := "22 1 0:21 / / rw,relatime shared:1 - btrfs /dev/nvme0n1p2 rw,subvol=/@\n" +
		"23 22 0:22 /@home /home rw,relatime shared:2 - btrfs /dev/nvme0n1p2 rw,subvol=/@home\n" +
		"24 22 8:1 / /mnt/Game\\040Disk rw,relatime shared:3 - ext4 /dev/sda1 rw\n" +
		"25 22 0:23 / /mnt/Game rw - tmpfs tmpfs rw\n" +
		"broken line\n"
	if err := os.WriteFile(info, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/home/p/.steam":          "/dev/nvme0n1p2",
		"/mnt/Game Disk/RimWorld": "/dev/sda1",
		"/mnt/Game/RimWorld":      "tmpfs",
		"/mnt/GameX":              "/dev/nvme0n1p2",
		"/":                       "/dev/nvme0n1p2",
	} {
		if got := mountSource(info, path); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
	if mountSource(info+"-missing", "/") != "" {
		t.Fatal("a missing mountinfo gives a source")
	}
}

// TestLinuxStorageKindLive only checks that a real path gets an answer without failing.
func TestLinuxStorageKindLive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	k := storageKind(dir)
	if k != storageKind(dir) {
		t.Fatal("the cached kind differs")
	}
	if Workers(dir) < 1 {
		t.Fatal("no workers")
	}
	t.Logf("%s: kind %d", dir, k)
}
