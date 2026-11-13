package lanzou

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveLanzouListing(t *testing.T) {
	if os.Getenv("RIMARCHO_LIVE_LANZOU_LIST") != "1" {
		t.Skip("set RIMARCHO_LIVE_LANZOU_LIST=1 for listing-only HTTP observation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := Client{ShareURL: DefaultShareURL, Password: DefaultPassword, Observe: func(e ProbeEvent) { t.Logf("stage=%s host=%s status=%d", e.Stage, e.Host, e.Status) }}
	listing, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("version=%s checksum_present=%t", listing.Version, listing.HasChecksum)
	if stableFilename("archo-"+listing.Version+"-windows-amd64.zip") == "" {
		t.Fatalf("the share lists no stable Windows zip (got %q)", listing.Version)
	}
}

func TestLiveLanzou(t *testing.T) {
	if os.Getenv("RIMARCHO_LIVE_LANZOU") != "1" {
		t.Skip("set RIMARCHO_LIVE_LANZOU=1 for a temporary ZIP probe")
	}
	result, err := ProbeLanzou(context.Background(), nil, DefaultShareURL, DefaultPassword, "archo-1.0.10-windows-amd64.zip", nil, func(e ProbeEvent) { t.Logf("stage=%s host=%s status=%d", e.Stage, e.Host, e.Status) })
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("final_host=%s bytes=%d sha256=%s", result.Host, result.Size, result.SHA256)
}
