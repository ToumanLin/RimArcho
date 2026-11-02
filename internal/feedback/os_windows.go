package feedback

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// System is "Windows 11 Pro 25H2 (26200.6899)", read from the registry. Windows 11 still
// calls itself "Windows 10" in ProductName; build 22000 and later is 11.
func System() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows"
	}
	defer k.Close()
	str := func(name string) string { v, _, _ := k.GetStringValue(name); return strings.TrimSpace(v) }
	name, display, build := str("ProductName"), str("DisplayVersion"), str("CurrentBuildNumber")
	if name == "" {
		name = "Windows"
	}
	if n, err := strconv.Atoi(build); err == nil && n >= 22000 {
		name = strings.Replace(name, "Windows 10", "Windows 11", 1)
	}
	out := name
	if display != "" {
		out += " " + display
	}
	if build != "" {
		if ubr, _, err := k.GetIntegerValue("UBR"); err == nil {
			build += fmt.Sprintf(".%d", ubr)
		}
		out += " (" + build + ")"
	}
	return out
}
