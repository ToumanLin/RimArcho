// Package selfupdate checks GitHub Releases for a newer RimArcho, downloads and verifies the
// release zip, and swaps the program files in place (17 §5.1).
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const DefaultAPI = "https://api.github.com/repos/ToumanLin/RimArcho"

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type Release struct {
	Version    string    `json:"version"`
	Tag        string    `json:"tag"`
	Name       string    `json:"name"`
	Notes      string    `json:"notes"`
	Page       string    `json:"page"`
	Published  time.Time `json:"published"`
	Prerelease bool      `json:"prerelease"`
	Zip        Asset     `json:"zip"`
	Sum        Asset     `json:"sum"`
}

// Installable reports missing platform assets or checksum files.
func (r *Release) Installable() error {
	if r == nil || r.Zip.Name == "" || r.Zip.URL == "" {
		return errors.New("release has no zip for this platform")
	}
	if r.Sum.Name == "" || r.Sum.URL == "" {
		return errors.New("release has no checksum file")
	}
	return nil
}

type Client struct {
	HTTP         *http.Client
	API          string
	UserAgent    string
	GOOS, GOARCH string
}

var ErrNoRelease = errors.New("no release published yet")

type githubRelease struct {
	Tag        string    `json:"tag_name"`
	Name       string    `json:"name"`
	Body       string    `json:"body"`
	Page       string    `json:"html_url"`
	Published  time.Time `json:"published_at"`
	Prerelease bool      `json:"prerelease"`
	Draft      bool      `json:"draft"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// Latest finds the newest stable release, or the highest version including prereleases.
func (c *Client) Latest(ctx context.Context, prerelease bool) (*Release, error) {
	api := c.API
	if api == "" {
		api = DefaultAPI
	}
	suffix := "/releases/latest"
	if prerelease {
		suffix = "/releases?per_page=30"
	}
	resp, err := c.get(ctx, c.metadataClient(), strings.TrimRight(api, "/")+suffix)
	if err != nil {
		return nil, fmt.Errorf("release lookup: %w", err)
	}
	defer resp.Body.Close()
	if !prerelease && resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release lookup: HTTP %d", resp.StatusCode)
	}
	b, err := readLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("release metadata: %w", err)
	}
	var selected githubRelease
	if !prerelease {
		if err := json.Unmarshal(b, &selected); err != nil {
			return nil, fmt.Errorf("release metadata: %w", err)
		}
	} else {
		var releases []githubRelease
		if err := json.Unmarshal(b, &releases); err != nil {
			return nil, fmt.Errorf("release metadata: %w", err)
		}
		found := false
		for _, r := range releases {
			if _, ok := Compare(r.Tag, r.Tag); r.Draft || !ok {
				continue
			}
			if !found || Newer(selected.Tag, r.Tag) {
				selected, found = r, true
			}
		}
		if !found {
			return nil, ErrNoRelease
		}
	}
	goos, goarch := c.GOOS, c.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	r := &Release{
		Version: strings.TrimPrefix(selected.Tag, "v"), Tag: selected.Tag,
		Name: selected.Name, Notes: selected.Body, Page: selected.Page,
		Published: selected.Published, Prerelease: selected.Prerelease,
	}
	name := ZipName(r.Version, goos, goarch)
	for _, a := range selected.Assets {
		asset := Asset{Name: a.Name, URL: a.URL, Size: a.Size}
		switch a.Name {
		case name:
			r.Zip = asset
		case name + ".sha256":
			r.Sum = asset
		}
	}
	if r.Zip.Name == "" {
		r.Sum = Asset{}
	}
	return r, nil
}

// ZipName returns the release archive name for a platform.
func ZipName(version, goos, goarch string) string {
	return "archo-" + version + "-" + goos + "-" + goarch + ".zip"
}

func (c *Client) metadataClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (c *Client) get(ctx context.Context, client *http.Client, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = "RimArcho"
	}
	req.Header.Set("User-Agent", ua)
	return client.Do(req)
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("response exceeds size limit")
	}
	return b, nil
}
