package lanzou

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

const maxFolderPages = 16

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
			return Listing{}, errors.New("Lanzou folder listing failed")
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
