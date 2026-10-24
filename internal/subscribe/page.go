package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type PageInfo struct {
	Name         string
	Version      string
	DateModified time.Time
	Repos        []string
}

func (p *PageInfo) Stamp() string {
	d := ""
	if !p.DateModified.IsZero() {
		d = p.DateModified.Format(time.RFC3339)
	}
	if p.Version == "" {
		return d
	}
	if d == "" {
		return p.Version
	}
	return p.Version + " @ " + d
}

// ReadPage reads only the public page. Redirects, cookies and download buttons
// are deliberately excluded: one invocation makes one GET.
func ReadPage(ctx context.Context, client *http.Client, pageURL string) (*PageInfo, error) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("page URL must be HTTPS without login: %s", pageURL)
	}
	c := http.Client{Timeout: 20 * time.Second}
	if client != nil {
		c = *client
	}
	c.Jar = nil
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("page %s: HTTP %d", pageURL, resp.StatusCode)
	}
	const limit = 4 << 20
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > limit {
		return nil, fmt.Errorf("page exceeds 4 MB: %s", pageURL)
	}
	root, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	p := &PageInfo{}
	var title, date string
	seen := map[string]bool{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		attrs := map[string]string{}
		for _, a := range n.Attr {
			attrs[strings.ToLower(a.Key)] = a.Val
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script":
				if strings.EqualFold(strings.TrimSpace(attrs["type"]), "application/ld+json") {
					var text strings.Builder
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode {
							text.WriteString(c.Data)
						}
					}
					var v any
					if json.Unmarshal([]byte(text.String()), &v) == nil {
						readLD(v, p)
					}
				}
			case "meta":
				switch strings.ToLower(attrs["property"]) {
				case "og:title":
					title = attrs["content"]
				case "og:updated_time":
					date = attrs["content"]
				}
			case "a":
				if repo := repoRoot(attrs["href"]); repo != "" && !seen[repo] {
					p.Repos = append(p.Repos, repo)
					seen[repo] = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	if p.Name == "" {
		p.Name = title
	}
	if p.DateModified.IsZero() {
		p.DateModified = pageTime(date)
	}
	if p.Name == "" {
		p.Name = strings.Trim(u.Path, "/")
	}
	if p.Stamp() == "" {
		return nil, fmt.Errorf("page %s has no version or modification date", pageURL)
	}
	return p, nil
}

func pageTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05-0700", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func readLD(v any, p *PageInfo) {
	switch v := v.(type) {
	case []any:
		for _, x := range v {
			readLD(x, p)
		}
	case map[string]any:
		version, _ := v["softwareVersion"].(string)
		date, _ := v["dateModified"].(string)
		typ, _ := v["@type"].(string)
		software := version != "" || strings.HasSuffix(typ, "SoftwareApplication")
		if name, _ := v["name"].(string); name != "" && (p.Name == "" || software) {
			p.Name = name
		}
		if version != "" || date != "" {
			if p.Version == "" {
				p.Version = version
			}
			if parsed := pageTime(date); !parsed.IsZero() && (p.DateModified.IsZero() || software) {
				p.DateModified = parsed
			}
		}
		if graph, ok := v["@graph"]; ok {
			readLD(graph, p)
		}
	}
}

func repoRoot(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Host)
	if host != "github.com" && host != "gitlab.com" && host != "gitgud.io" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" || parts[0] == "-" {
		return ""
	}
	if host == "github.com" && (parts[0] == "orgs" || parts[0] == "users" || parts[0] == "topics" || parts[0] == "settings") {
		return ""
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	if repo == "" {
		return ""
	}
	return "https://" + host + "/" + parts[0] + "/" + repo
}
