package lanzou

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// ProbeEvent deliberately omits paths, form values, query tokens and cookies.
type ProbeEvent struct {
	Stage, Host string
	Status      int
}
type ProbeResult struct {
	Host   string
	Size   int64
	SHA256 string
}

// ProbeLanzou observes a ZIP without requiring a sidecar. This diagnostic never
// authorizes installation. Its temporary file and directory are deleted on return.
func ProbeLanzou(ctx context.Context, httpClient *http.Client, share, password, filename string, hosts []string, observe func(ProbeEvent)) (result ProbeResult, err error) {
	c := Client{HTTP: httpClient, ShareURL: share, Password: password, ExtraHosts: hosts}
	c.Observe = func(e ProbeEvent) {
		if e.Host != "" {
			result.Host = e.Host
		}
		if observe != nil {
			observe(e)
		}
	}
	r, share, err := c.runner()
	if err != nil {
		return result, err
	}
	defer r.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	listing, err := r.list(ctx, share, password)
	if err != nil {
		return result, err
	}
	id, err := listing.find(filename)
	if err != nil {
		return result, err
	}
	target, err := r.resolve(ctx, share, id)
	if err != nil {
		return result, err
	}
	dir, err := os.MkdirTemp("", "rimarcho-lanzou-probe-*")
	if err != nil {
		return result, errors.New("cannot create probe temporary directory")
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	archive, err := r.transfer(ctx, target, dir, MaxArchive)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, archive.Close()) }()
	var prefix [4]byte
	if _, err := io.ReadFull(archive.file, prefix[:]); err != nil || string(prefix[:]) != "PK\x03\x04" {
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
