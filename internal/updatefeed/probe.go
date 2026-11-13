package updatefeed

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// ProbeEvent contains only stage identifiers, hostnames and HTTP status codes.
// It never exposes request paths, query tokens, form values, headers or cookies.
type ProbeEvent struct {
	Stage  string
	Host   string
	Status int
}

type ProbeResult struct {
	Host   string // Last contacted host on failure; final byte host on success.
	Size   int64
	SHA256 string // Observed only; does not establish release authenticity.
}

// ProbeLanzou measures a share using the same bounded resolver as Feed.Download.
// Extra hosts must be explicitly supplied by the tester; destination checks still
// apply. Bytes are quarantined in a temporary directory and deleted before return.
// No signed feed is available to this diagnostic, so it never authorizes an install.
func ProbeLanzou(ctx context.Context, client *http.Client, share, password, filename string, hosts []string, observe func(ProbeEvent)) (result ProbeResult, err error) {
	source, err := LanzouSource(share, password)
	if err != nil {
		return result, err
	}
	if filename == "" || len(filename) > maxValue {
		return result, errors.New("an exact filename is required")
	}
	for _, host := range hosts {
		found := false
		for _, existing := range source.Hosts {
			found = found || existing == host
		}
		if !found {
			source.Hosts = append(source.Hosts, host)
		}
	}
	if err := validateSource(source); err != nil {
		return result, err
	}
	r, err := newRunner(client, source, "")
	if err != nil {
		return result, err
	}
	defer r.client.CloseIdleConnections()
	r.observe = func(event ProbeEvent) {
		if event.Host != "" {
			result.Host = event.Host
		}
		if observe != nil {
			observe(event)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp("", "rimarcho-lanzou-probe-*")
	if err != nil {
		return result, errors.New("cannot create probe temporary directory")
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	target, err := r.resolve(ctx, filename)
	if err != nil {
		return result, err
	}
	r.report("download-node", hostname(target), 0)
	archive, err := r.transfer(ctx, target, nil, dir)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, archive.Close()) }()
	var prefix [4]byte
	if _, err := io.ReadFull(archive, prefix[:]); err != nil || string(prefix[:]) != "PK\x03\x04" {
		return result, errors.New("downloaded file is not a ZIP")
	}
	result = ProbeResult{Host: hostname(archive.URL), Size: archive.Size, SHA256: archive.SHA256}
	r.report("download-complete", result.Host, 0)
	return result, nil
}

func hostname(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
