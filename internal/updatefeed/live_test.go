package updatefeed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// This opt-in experiment uses only HTTP and t.TempDir, never a game or Steam state.
// There is no published signed feed yet: the first download supplies an observation,
// not an authenticity assertion. A throwaway-signed manifest then verifies a repeat.
func TestLiveLanzou(t *testing.T) {
	if os.Getenv("RIMARCHO_LIVE_LANZOU") != "1" {
		t.Skip("set RIMARCHO_LIVE_LANZOU=1 to probe the maintainer's share")
	}
	name := os.Getenv("RIMARCHO_LIVE_LANZOU_VERSION")
	if name == "" {
		name = "1.0.10"
	}
	artifact := "archo-" + name + "-windows-amd64.zip"
	s := liveLanzouSource()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	observed, err := ProbeLanzou(ctx, nil, "https://rimarcho.lanzn.com/b01i608qja", "ctot", artifact, nil, func(e ProbeEvent) {
		t.Logf("stage=%s host=%s status=%d", e.Stage, e.Host, e.Status)
	})
	if err != nil {
		t.Fatal(err)
	}
	n, digest := observed.Size, observed.SHA256
	if expected := os.Getenv("RIMARCHO_LIVE_LANZOU_SHA256"); expected != "" && expected != digest {
		t.Fatal("observed archive differs from supplied SHA-256")
	}
	t.Logf("observed %s: bytes=%d sha256=%s final_host=%s", artifact, n, digest, observed.Host)
	now := time.Now().UTC()
	m := Manifest{Format: 1, Sequence: 1, Issued: now.Add(-time.Minute), Expires: now.Add(time.Hour), Version: name, Artifact: Artifact{Name: artifact, Size: n, SHA256: digest}, Sources: []Source{s}}
	payload, _ := json.Marshal(m)
	b, keys := signed(t, payload)
	feed, err := Parse(strings.NewReader(string(b)), keys, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Run the full recipe again in a new cookie jar and DNS/connection state. Do not
	// reuse a token as persisted metadata: resolving is deliberately inexpensive.
	archive, err := feed.Download(ctx, &http.Client{}, 0, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	independent := sha256.New()
	got, err := io.Copy(independent, archive)
	if err != nil || got != n || hex.EncodeToString(independent.Sum(nil)) != digest || archive.SHA256 != digest {
		t.Fatalf("verified archive mismatch: %v", err)
	}
	t.Logf("fresh signed-feed download verified: bytes=%d sha256=%s host=%s", got, archive.SHA256, hostname(archive.URL))
}

func liveLanzouSource() Source {
	s, err := LanzouSource("https://rimarcho.lanzn.com/b01i608qja", "ctot")
	if err != nil {
		panic(err)
	}
	return s
}
