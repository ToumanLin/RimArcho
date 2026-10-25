package accounts

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const runtimeID = "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

type runtimeRead func(registry.Key, string, string) string

func runtimeDLL() (string, error) {
	return findRuntimeDLL(func(root registry.Key, path, name string) string {
		key, err := registry.OpenKey(root, path, registry.READ)
		if err != nil {
			return ""
		}
		defer key.Close()
		value, _, _ := key.GetStringValue(name)
		return value
	})
}

// Runtime folder layout and version validation follow Wails' pure-Go loader
// (webviewloader_LICENSE). Clients supports installations without ClientState.
func findRuntimeDLL(read runtimeRead) (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[runtime.GOARCH]
	var best [4]uint64
	found := ""
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, prefix := range []string{`SOFTWARE\Microsoft\EdgeUpdate\`, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\`} {
			clients := prefix + `Clients\` + runtimeID
			version, location := read(root, clients, "pv"), read(root, clients, "location")
			folders := []string{}
			if filepath.IsAbs(location) {
				folders = append(folders, filepath.Join(location, version))
			}
			// Retain the standard ClientState registration supported by Wails.
			folders = append(folders, read(root, prefix+`ClientState\`+runtimeID, "EBWebView"))
			for _, folder := range folders {
				if !filepath.IsAbs(folder) {
					continue
				}
				parts := strings.Split(filepath.Base(folder), ".")
				if len(parts) != 4 {
					continue
				}
				var v [4]uint64
				valid := true
				for i, part := range parts {
					number, err := strconv.ParseUint(part, 10, 32)
					if err != nil {
						valid = false
						break
					}
					v[i] = number
				}
				if !valid || v[0] < 86 || slices.Compare(v[:], best[:]) <= 0 {
					continue
				}
				dll := filepath.Join(folder, "EBWebView", arch, "EmbeddedBrowserWebView.dll")
				if stat, err := os.Stat(dll); err == nil && !stat.IsDir() {
					found, best = dll, v
				}
			}
		}
	}
	if found == "" {
		return "", helperFailure("WebView2 runtime not found", 0)
	}
	return found, nil
}

// Wails supplies ICoreWebView2EnvironmentOptions and Options2 with empty
// arguments/language, minimum version 86.0.616.0, and SSO/exclusive access off.
// Ported from its ISC-licensed loader; see webviewloader_LICENSE.
type environmentOptions struct{ primary, secondary comObject }

func newEnvironmentOptions() *environmentOptions {
	options := &environmentOptions{}
	unknown := windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	base, _ := windows.GUIDFromString("{2fde08a8-1e9a-4766-8c05-95a9ceb9d1c5}")
	second, _ := windows.GUIDFromString("{ff85c98a-1ba7-4a6b-90c8-2b752c89e9e2}")
	query := windows.NewCallback(func(_ *comObject, id *windows.GUID, out **comObject) uintptr {
		*out = nil
		switch *id {
		case unknown, base:
			*out = &options.primary
		case second:
			*out = &options.secondary
		default:
			return 0x80004002 // E_NOINTERFACE
		}
		return 0
	})
	ref := windows.NewCallback(func(*comObject) uintptr { return 1 })
	nop := windows.NewCallback(func(*comObject, uintptr) uintptr { return 1 }) // S_FALSE, as in Wails
	stringGetter := func(value string) uintptr {
		return windows.NewCallback(func(_ *comObject, out **uint16) uintptr {
			hr, _, _ := windows.NewLazySystemDLL("shlwapi.dll").NewProc("SHStrDupW").Call(ptr(wide(value)), ptr(out))
			// SHStrDupW returns a CoTaskMem allocation, owned by the COM caller.
			return hr
		})
	}
	boolGetter := windows.NewCallback(func(_ *comObject, out *int32) uintptr { *out = 0; return 0 })
	options.primary.table = &[80]uintptr{
		query,                      // 0 IUnknown.QueryInterface
		ref,                        // 1 AddRef
		ref,                        // 2 Release
		stringGetter(""),           // 3 get_AdditionalBrowserArguments
		nop,                        // 4 put_AdditionalBrowserArguments
		stringGetter(""),           // 5 get_Language
		nop,                        // 6 put_Language
		stringGetter("86.0.616.0"), // 7 get_TargetCompatibleBrowserVersion
		nop,                        // 8 put_TargetCompatibleBrowserVersion
		boolGetter,                 // 9 get_AllowSingleSignOnUsingOSPrimaryAccount
		nop,                        // 10 put_AllowSingleSignOnUsingOSPrimaryAccount
	}
	options.secondary.table = &[80]uintptr{query, ref, ref,
		boolGetter, // 3 ICoreWebView2EnvironmentOptions2.get_ExclusiveUserDataFolderAccess
		nop,        // 4 put_ExclusiveUserDataFolderAccess
	}
	return options
}
