// Package lanzou is a bounded resolver for RimArcho release ZIPs.
// It never installs or executes downloads, or reads game/application settings.
package lanzou

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

const DefaultShareURL = "https://rimarcho.lanzn.com/b01i608qja"
const DefaultPassword = "ctot"
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"

type Client struct {
	HTTP               *http.Client
	ShareURL, Password string
	ExtraHosts         []string
	Observe            func(ProbeEvent)
	testOrigin         string // Synthetic tests only; unavailable to callers.
}

func (c Client) runner() (*runner, string, error) {
	share := c.ShareURL
	if share == "" {
		share = DefaultShareURL
	}
	u, err := url.Parse(share)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, "", errors.New("share URL must be an HTTPS folder link without query or fragment")
	}
	hosts := []string{u.Hostname(), "apifile.woozooo.com", "developer4.lanrar.com"}
	for _, host := range c.ExtraHosts {
		if len(host) > 253 || host != strings.ToLower(host) || !strings.Contains(host, ".") || strings.ContainsAny(host, "/:@* \r\n") || strings.HasSuffix(host, ".") {
			return nil, "", errors.New("extra hosts must be exact lowercase DNS names")
		}
		hosts = append(hosts, host)
	}
	if len(hosts) > 8 {
		return nil, "", errors.New("too many allowed hosts")
	}
	r, err := newRunner(c.HTTP, hosts, c.testOrigin)
	if err != nil {
		return nil, "", err
	}
	r.observe = c.Observe
	if _, err := r.checkURL(share); err != nil {
		r.client.CloseIdleConnections()
		return nil, "", err
	}
	return r, share, nil
}

var archiveName = regexp.MustCompile(`^archo-((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))-windows-amd64\.zip$`)

func stableFilename(name string) string {
	m := archiveName.FindStringSubmatch(name)
	if m == nil || len(m[1]) > 64 {
		return ""
	}
	if _, ok := selfupdate.Compare(m[1], m[1]); !ok {
		return ""
	}
	return m[1]
}

type file struct{ name, id string }
type Listing struct {
	Version     string
	HasChecksum bool // Exact <highest stable ZIP>.sha256 filename was listed.
	files       []file
}

func (l Listing) find(name string) (string, error) {
	id := ""
	for _, f := range l.files {
		if f.name == name {
			if id != "" {
				return "", errors.New("ambiguous filename in Lanzou folder")
			}
			id = f.id
		}
	}
	if id == "" {
		return "", errors.New("exact filename not found in Lanzou folder")
	}
	return id, nil
}

// List stops after the folder/password/pagination requests. It never fetches a
// file page, iframe, download API or download node, even when a version is found.
func (c Client) List(ctx context.Context) (Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	r, share, err := c.runner()
	if err != nil {
		return Listing{}, err
	}
	defer r.client.CloseIdleConnections()
	return r.list(ctx, share, c.Password)
}
