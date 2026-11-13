package lanzou

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func extract(body, pattern string) (string, error) {
	m := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if m == nil || len(m[1]) > maxValue {
		return "", errors.New("Lanzou page format changed")
	}
	return html.UnescapeString(m[1]), nil
}

func resolveURL(base, ref string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", errors.New("invalid Lanzou base URL")
	}
	u, err = u.Parse(ref)
	if err != nil {
		return "", errors.New("invalid Lanzou relative URL")
	}
	return u.String(), nil
}

func ajaxHeaders(ref string) http.Header {
	u, _ := url.Parse(ref)
	return http.Header{"Referer": {ref}, "Origin": {u.Scheme + "://" + u.Host}, "X-Requested-With": {"XMLHttpRequest"}}
}

func (r *runner) page(ctx context.Context, raw, method string, form url.Values, headers http.Header) (string, string, error) {
	if headers == nil {
		headers = http.Header{}
	}
	if len(form) > 16 || len(form.Encode()) > 16<<10 {
		return "", "", errors.New("Lanzou form exceeds limit")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := r.send(ctx, raw, method, form, headers)
		if err != nil {
			return "", "", err
		}
		b, err := r.metadata(resp)
		if err != nil {
			return "", "", err
		}
		if strings.Contains(string(b), "acw_sc__v2") {
			if method != "GET" || attempt != 0 || !r.challenge(resp, b) {
				return "", "", errors.New("metadata cookie challenge failed")
			}
			raw = resp.Request.URL.String()
			continue
		}
		return string(b), resp.Request.URL.String(), nil
	}
	return "", "", errors.New("metadata cookie challenge failed")
}

// resolve is fixed Go parsing, shared by the probe and release-pair downloader.
// IDs retain their query state; the allowlist is checked before every request.
func (r *runner) resolve(ctx context.Context, share, id string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	raw, err := resolveURL(share, id)
	if err != nil {
		return "", err
	}
	body, fileURL, err := r.page(ctx, raw, "GET", nil, ajaxHeaders(share))
	if err != nil {
		return "", err
	}
	r.report("file-page", hostname(fileURL), 200)
	frame, err := extract(body, `<iframe[^>]+src="([^"]+)"`)
	if err != nil {
		return "", err
	}
	raw, err = resolveURL(fileURL, frame)
	if err != nil {
		return "", err
	}
	body, frameURL, err := r.page(ctx, raw, "GET", nil, ajaxHeaders(fileURL))
	if err != nil {
		return "", err
	}
	r.report("iframe", hostname(frameURL), 200)
	values := map[string]string{}
	for _, name := range []string{"domain1", "ajaxdata", "wp_sign"} {
		values[name], err = extract(body, fmt.Sprintf(`var\s+%s\s*=\s*'([^']*)'`, name))
		if err != nil {
			return "", err
		}
	}
	body, _, err = r.page(ctx, values["domain1"], "POST", url.Values{"action": {"downprocess"}, "websignkey": {values["ajaxdata"]}, "signs": {values["ajaxdata"]}, "sign": {values["wp_sign"]}, "websign": {"2"}, "kd": {"1"}, "ves": {"1"}}, ajaxHeaders(frameURL))
	if err != nil {
		return "", err
	}
	var data map[string]any
	if strictJSON([]byte(body), &data) != nil || !statusIs(data["zt"], 1) {
		return "", errors.New("Lanzou download API failed")
	}
	domain, okDomain := data["dom"].(string)
	token, okToken := data["url"].(string)
	if !okDomain || !okToken || domain == "" || token == "" || token == "0" {
		return "", errors.New("Lanzou download API returned no link")
	}
	target := strings.TrimRight(domain, "/") + "/file/" + strings.TrimLeft(token, "/")
	if _, err := r.checkURL(target); err != nil {
		return "", err
	}
	r.report("download-node", hostname(target), 0)
	return target, nil
}
