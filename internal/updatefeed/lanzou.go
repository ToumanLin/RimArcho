package updatefeed

import (
	"errors"
	"fmt"
	"net/url"
)

// Exact, dated page patterns, independently written from live HTTP responses.
// LanzouSource builds an unwired recipe example, not an application provider.
func LanzouSource(folder, password string) (Source, error) {
	u, err := url.Parse(folder)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Source{}, errors.New("share URL must be an HTTPS folder link without query or fragment")
	}
	root := u.Scheme + "://" + u.Host
	headers := func(ref, origin string) map[string]string {
		return map[string]string{"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36", "Referer": ref, "Origin": origin, "X-Requested-With": "XMLHttpRequest"}
	}
	s := Source{UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36", Name: "lanzou", Challenge: "acw-v2", Hosts: []string{u.Hostname(), "apifile.woozooo.com", "developer4.lanrar.com"}, Download: "${nodeurl}"}
	s.Steps = append(s.Steps, Step{Op: "request", Into: "folder", Method: "GET", URL: folder, Headers: map[string]string{"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"}})
	regex := func(into, input, pattern string) {
		s.Steps = append(s.Steps, Step{Op: "regex", Into: into, Input: input, Pattern: pattern})
	}
	regex("endpoint", "folder", `url\s*:\s*'([^']*filemore[^']*)'`)
	for _, field := range []string{"lx", "fid", "uid", "puid", "rep", "up", "ls"} {
		// Fields observed with integer or quoted literal values. Capture either without JS.
		regex(field, "folder", fmt.Sprintf(`'%s'\s*:\s*'?([A-Za-z0-9]+)'?`, field))
	}
	regex("timestamp", "folder", `var\s+\w+\s*=\s*'(\d{10})'`)
	regex("key", "folder", `var\s+\w+\s*=\s*'([a-f0-9]{32})'`)
	s.Steps = append(s.Steps, Step{Op: "lanzou-list", Into: "id", Method: "POST", URL: root + "${endpoint}", Headers: headers(folder, root), Form: map[string]string{"lx": "${lx}", "fid": "${fid}", "uid": "${uid}", "puid": "${puid}", "rep": "${rep}", "up": "${up}", "ls": "${ls}", "t": "${timestamp}", "k": "${key}", "pg": "1", "pwd": password}},
		Step{Op: "url", Into: "fileurl", Base: root + "/", Value: "${id}"},
		Step{Op: "request", Into: "filepage", Method: "GET", URL: "${fileurl}", Headers: headers(folder, root)})
	regex("iframe", "filepage", `<iframe[^>]+src="([^"]+)"`)
	s.Steps = append(s.Steps, Step{Op: "url", Into: "frameurl", Base: "${filepage_url}", Value: "${iframe}"}, Step{Op: "request", Into: "frame", Method: "GET", URL: "${frameurl}", Headers: headers("${fileurl}", root)})
	for _, v := range []struct{ into, name string }{{"api", "domain1"}, {"websign", "ajaxdata"}, {"sign", "wp_sign"}} {
		regex(v.into, "frame", fmt.Sprintf(`var\s+%s\s*=\s*'([^']*)'`, v.name))
	}
	s.Steps = append(s.Steps, Step{Op: "request", Into: "apiresult", Method: "POST", URL: "${api}", Headers: headers("${frameurl}", root), Form: map[string]string{"action": "downprocess", "websignkey": "${websign}", "signs": "${websign}", "sign": "${sign}", "websign": "2", "kd": "1", "ves": "1"}},
		Step{Op: "json", Into: "status", Input: "apiresult", Path: "/zt"}, Step{Op: "assert", Into: "checked", Input: "${status}", Equal: "1"},
		Step{Op: "json", Into: "dom", Input: "apiresult", Path: "/dom"}, Step{Op: "json", Into: "token", Input: "apiresult", Path: "/url"},
		Step{Op: "value", Into: "nodeurl", Value: "${dom}/file/${token}"})
	return s, validateSource(s)
}
