package lanzou

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

const maxFolderPages = 16

// folderPageSize is Lanzou's page length: a shorter page is the last one. Some nodes
// answer a request after the last page as expired (zt other than 1 or 2) instead of zt=2,
// so the listing must stop before asking (as RimCrow's get_all_files does).
const folderPageSize = 50

func statusIs(value any, want int) bool { return value == float64(want) || value == strconv.Itoa(want) }

func (r *runner) list(ctx context.Context, share, password string) (Listing, error) {
	body, pageURL, err := r.page(ctx, share, "GET", nil, nil)
	if err != nil {
		return Listing{}, err
	}
	r.report("folder", hostname(pageURL), 200)
	endpoint, err := extract(body, `url\s*:\s*'([^']*filemore[^']*)'`)
	if err != nil {
		return Listing{}, err
	}
	endpoint, err = resolveURL(pageURL, endpoint)
	if err != nil {
		return Listing{}, err
	}
	form := url.Values{"pg": {"1"}, "pwd": {password}}
	for _, field := range []string{"lx", "fid", "uid", "puid", "rep", "up", "ls"} {
		value, err := extract(body, fmt.Sprintf(`'%s'\s*:\s*'?([A-Za-z0-9]+)'?`, field))
		if err != nil {
			return Listing{}, err
		}
		form.Set(field, value)
	}
	stamp, err := extract(body, `var\s+\w+\s*=\s*'(\d{10})'`)
	if err != nil {
		return Listing{}, err
	}
	// t and k are taken as the first 10-digit and 32-hex variables. RimCrow finds them by
	// the variable names the page passes and tries up to four candidates; switch to that
	// if a page ever carries decoys before the real values.
	key, err := extract(body, `var\s+\w+\s*=\s*'([a-f0-9]{32})'`)
	if err != nil {
		return Listing{}, err
	}
	form.Set("t", stamp)
	form.Set("k", key)
	listing := Listing{}
	seen := map[string]string{}
	complete := false
	for page := 1; page <= maxFolderPages; page++ {
		form.Set("pg", strconv.Itoa(page))
		body, _, err = r.page(ctx, endpoint, "POST", form, ajaxHeaders(pageURL))
		if err != nil {
			return Listing{}, err
		}
		var data map[string]any
		if strictJSON([]byte(body), &data) != nil {
			return Listing{}, errors.New("invalid Lanzou folder JSON")
		}
		list, ok := data["text"].([]any)
		empty := statusIs(data["zt"], 2)
		if !empty && (!statusIs(data["zt"], 1) || !ok) {
			return Listing{}, fmt.Errorf("Lanzou folder listing failed on page %d: zt=%s info=%q", page, safeField(data["zt"]), safeField(data["info"]))
		}
		r.report("folder-page-"+strconv.Itoa(page), hostname(endpoint), 200)
		if empty || len(list) == 0 {
			complete = true
			break
		}
		before := len(seen)
		for _, entry := range list {
			object, ok := entry.(map[string]any)
			if !ok {
				return Listing{}, errors.New("invalid Lanzou folder entry")
			}
			id, validID := object["id"].(string)
			name, validName := object["name_all"].(string)
			if !validID || id == "" || !validName || name == "" || len(id) > maxValue || len(name) > maxValue {
				return Listing{}, errors.New("invalid Lanzou folder entry")
			}
			if old, ok := seen[id]; ok {
				if old != name {
					return Listing{}, errors.New("conflicting Lanzou folder entry")
				}
				continue
			}
			seen[id] = name
			listing.files = append(listing.files, file{name: name, id: id})
			if version := stableFilename(name); version != "" && (listing.Version == "" || selfupdate.Newer(listing.Version, version)) {
				listing.Version = version
			}
		}
		if len(seen) == before {
			return Listing{}, errors.New("Lanzou folder pagination made no progress")
		}
		if len(list) < folderPageSize {
			complete = true
			break
		}
	}
	if !complete {
		return Listing{}, errors.New("Lanzou folder exceeds 16-page limit")
	}
	if listing.Version != "" {
		name := selfupdate.ZipName(listing.Version, "windows", "amd64")
		if _, err := listing.find(name); err != nil {
			return Listing{}, err
		}
		_, err := listing.find(name + ".sha256")
		listing.HasChecksum = err == nil
	}
	return listing, nil
}

// safeField renders a response field for an error: printable text only, at most 160 bytes.
// Only zt and info pass through it, never the form (password, t, k).
func safeField(v any) string {
	var text string
	switch v := v.(type) {
	case nil:
		return "none"
	case string:
		text = v
	case float64:
		text = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		text = fmt.Sprint(v)
	}
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
	if len(text) > 160 {
		cut := 160
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	return text
}
