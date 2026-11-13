package updatefeed

import (
	"context"
	"errors"
	"strconv"
)

const maxFolderPages = 16

// lanzouList scans through the empty page, so a later ambiguous filename or
// failed page cannot silently authorize a partial folder listing.
func (r *runner) lanzouList(ctx context.Context, step Step, vars map[string]string, artifact string) (string, error) {
	form := make(map[string]string, len(step.Form))
	for k, v := range step.Form {
		form[k] = v
	}
	step.Form = form
	seen := map[string]bool{}
	selected := ""
	for page := 1; page <= maxFolderPages; page++ {
		step.Form["pg"] = strconv.Itoa(page)
		body, _, err := r.requestMetadata(ctx, step, vars)
		if err != nil {
			return "", err
		}
		var root map[string]any
		if err := strictJSON([]byte(body), &root); err != nil {
			return "", errors.New("invalid Lanzou folder JSON")
		}
		status := root["zt"]
		empty := status == "2" || status == float64(2)
		list, ok := root["text"].([]any)
		if !empty && (status != "1" && status != float64(1) || !ok) {
			return "", errors.New("Lanzou folder listing failed")
		}
		r.report("folder-page-"+strconv.Itoa(page), "", 0)
		if empty || len(list) == 0 {
			if selected == "" {
				return "", errors.New("signed filename not found in Lanzou folder")
			}
			return selected, nil
		}
		before := len(seen)
		for _, entry := range list {
			object, ok := entry.(map[string]any)
			if !ok {
				return "", errors.New("invalid Lanzou folder entry")
			}
			id, validID := object["id"].(string)
			name, validName := object["name_all"].(string)
			if !validID || id == "" || !validName || name == "" || len(id) > maxValue || len(name) > maxValue {
				return "", errors.New("invalid Lanzou folder entry")
			}
			if name == artifact {
				if selected != "" && selected != id {
					return "", errors.New("ambiguous signed filename in Lanzou folder")
				}
				selected = id
			}
			seen[id] = true
		}
		if len(seen) == before {
			return "", errors.New("Lanzou folder pagination made no progress")
		}
	}
	return "", errors.New("Lanzou folder exceeds 16-page limit")
}
