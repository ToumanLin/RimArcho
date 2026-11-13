package updatefeed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func validateSource(s Source) error {
	if len(s.UserAgent) > 256 || strings.ContainsAny(s.UserAgent, "\r\n") {
		return errors.New("invalid source User-Agent")
	}
	if len(s.Hosts) == 0 || len(s.Hosts) > 8 || len(s.Steps) > 48 || s.Download == "" || len(s.Download) > maxValue || (s.Challenge != "" && s.Challenge != "acw-v2") {
		return errors.New("invalid source limits or challenge strategy")
	}
	for _, h := range s.Hosts {
		if len(h) > 253 || h != strings.ToLower(h) || !strings.Contains(h, ".") || strings.ContainsAny(h, "/:@* \r\n") || strings.HasSuffix(h, ".") {
			return errors.New("hosts must be exact lowercase DNS names")
		}
	}
	for _, step := range s.Steps {
		if !identifier.MatchString(step.Into) || step.Into == "artifact" {
			return errors.New("step destination must be an identifier other than artifact")
		}
		switch step.Op {
		case "request", "lanzou-list":
			if step.Method != "GET" && step.Method != "POST" {
				return errors.New("request method must be GET or POST")
			}
			if len(step.Form) > 16 || len(step.Headers) > 6 || (step.Method == "GET" && len(step.Form) != 0) {
				return errors.New("invalid request fields")
			}
			if step.Op == "lanzou-list" && (step.Method != "POST" || step.Form["pg"] != "1") {
				return errors.New("lanzou-list needs POST with pg=1")
			}
			for k := range step.Headers {
				switch http.CanonicalHeaderKey(k) {
				case "User-Agent", "Referer", "Origin", "X-Requested-With", "Accept", "Accept-Language":
				default:
					return errors.New("unsupported recipe header")
				}
			}
		case "regex":
			if len(step.Pattern) > 512 {
				return errors.New("regex exceeds 512 bytes")
			}
			re, err := regexp.Compile(step.Pattern)
			if err != nil || re.NumSubexp() != 1 {
				return errors.New("regex must compile with exactly one capture")
			}
		case "json", "select", "url", "value", "assert":
		default:
			return errors.New("unsupported recipe operation")
		}
		// Keep every scalar and map entry bounded, including currently unused fields.
		b, _ := json.Marshal(step)
		if len(b) > 16<<10 {
			return errors.New("step exceeds 16 KiB")
		}
	}
	return nil
}

var substitution = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9_]*)\}`)

func expand(s string, vars map[string]string) (string, error) {
	var out strings.Builder
	appendPart := func(part string) error {
		if out.Len()+len(part) > maxValue {
			return errors.New("recipe value exceeds 8 KiB")
		}
		out.WriteString(part)
		return nil
	}
	start := 0
	for _, loc := range substitution.FindAllStringSubmatchIndex(s, -1) {
		if err := appendPart(s[start:loc[0]]); err != nil {
			return "", err
		}
		value, ok := vars[s[loc[2]:loc[3]]]
		if !ok {
			return "", errors.New("recipe references an undefined value")
		}
		if err := appendPart(value); err != nil {
			return "", err
		}
		start = loc[1]
	}
	if err := appendPart(s[start:]); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (r *runner) resolve(ctx context.Context, artifact string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	vars := map[string]string{"artifact": artifact}
	for i, s := range r.source.Steps {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var value string
		var err error
		switch s.Op {
		case "lanzou-list":
			value, err = r.lanzouList(ctx, s, vars, artifact)
		case "request":
			value, vars[s.Into+"_url"], err = r.requestMetadata(ctx, s, vars)
		case "regex":
			var input string
			input, err = expandedInput(s.Input, vars)
			if err != nil {
				break
			}
			m := regexp.MustCompile(s.Pattern).FindStringSubmatch(input)
			if m == nil {
				err = errors.New("regex did not match")
			} else {
				value = m[1]
			}
		case "value":
			value, err = expand(s.Value, vars)
		case "url":
			var base, ref string
			base, err = expand(s.Base, vars)
			if err != nil {
				break
			}
			ref, err = expand(s.Value, vars)
			if err != nil {
				break
			}
			var u *url.URL
			u, err = url.Parse(base)
			if err != nil {
				break
			}
			u, err = u.Parse(ref)
			if err == nil {
				value = u.String()
			}
		case "assert":
			var want string
			value, err = expand(s.Input, vars)
			if err != nil {
				break
			}
			want, err = expand(s.Equal, vars)
			if err == nil && value != want {
				err = errors.New("source assertion failed")
			}
		case "json", "select":
			var input string
			input, err = expandedInput(s.Input, vars)
			if err != nil {
				break
			}
			var root any
			if err = strictJSON([]byte(input), &root); err != nil {
				break
			}
			var node any
			node, err = pointer(root, s.Path)
			if err != nil {
				break
			}
			if s.Op == "select" {
				var match string
				match, err = expand(s.Equal, vars)
				if err != nil {
					break
				}
				list, ok := node.([]any)
				if !ok {
					err = errors.New("selection needs a JSON array")
					break
				}
				var selected any
				count := 0
				for _, entry := range list {
					object, ok := entry.(map[string]any)
					if ok && object[s.Match] == match {
						selected = object[s.Field]
						count++
					}
				}
				if count != 1 {
					err = errors.New("selection needs exactly one matching filename")
					break
				}
				node = selected
			}
			switch n := node.(type) {
			case string:
				value = n
			case float64:
				value = strconv.FormatFloat(n, 'f', -1, 64)
			case bool:
				value = strconv.FormatBool(n)
			default:
				err = errors.New("JSON extraction needs a scalar")
			}
		}
		if err != nil {
			if s.Op == "url" {
				err = errors.New("recipe URL could not be resolved")
			}
			return "", fmt.Errorf("recipe step %d (%s): %w", i+1, s.Op, err)
		}
		// Metadata bodies are the only large variables; extracted values stay small.
		if s.Op != "request" && len(value) > maxValue {
			return "", errors.New("extracted value exceeds 8 KiB")
		}
		vars[s.Into] = value
		r.report("step-"+s.Into, "", 0)
		if len(vars) > 97 {
			return "", errors.New("recipe exceeds variable limit")
		}
	}
	target, err := expand(r.source.Download, vars)
	if err != nil {
		return "", err
	}
	_, err = r.checkURL(target)
	return target, err
}

func (r *runner) requestMetadata(ctx context.Context, s Step, vars map[string]string) (string, string, error) {
	raw, err := expand(s.URL, vars)
	if err != nil {
		return "", "", err
	}
	headers := http.Header{}
	form := url.Values{}
	for k, v := range s.Headers {
		x, err := expand(v, vars)
		if err != nil {
			return "", "", err
		}
		headers.Set(k, x)
	}
	for k, v := range s.Form {
		x, err := expand(v, vars)
		if err != nil {
			return "", "", err
		}
		form.Set(k, x)
	}
	if len(form.Encode()) > 16<<10 {
		return "", "", errors.New("form exceeds 16 KiB")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := r.send(ctx, raw, s.Method, form, headers)
		if err != nil {
			return "", "", err
		}
		b, err := r.metadata(resp)
		if err != nil {
			return "", "", err
		}
		if s.Method == "GET" && strings.Contains(string(b), "acw_sc__v2") {
			if attempt != 0 || !r.challenge(resp, b) {
				return "", "", errors.New("metadata cookie challenge failed")
			}
			raw = resp.Request.URL.String()
			continue
		}
		return string(b), resp.Request.URL.String(), nil
	}
	return "", "", errors.New("metadata cookie challenge failed")
}

// Input names a variable rather than interpolating a potentially 1 MiB page.
func expandedInput(name string, vars map[string]string) (string, error) {
	v, ok := vars[name]
	if !ok {
		return "", errors.New("recipe input is undefined")
	}
	return v, nil
}

func pointer(root any, path string) (any, error) {
	if path == "" {
		return root, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("JSON path must be a JSON pointer")
	}
	for _, part := range strings.Split(path[1:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch n := root.(type) {
		case map[string]any:
			var ok bool
			root, ok = n[part]
			if !ok {
				return nil, errors.New("JSON pointer not found")
			}
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(n) {
				return nil, errors.New("JSON array index not found")
			}
			root = n[i]
		default:
			return nil, errors.New("JSON pointer traverses a scalar")
		}
	}
	return root, nil
}
