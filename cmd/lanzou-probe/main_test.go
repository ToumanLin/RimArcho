package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/updatefeed"
)

func TestProbeOutputAndExitStatus(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var out bytes.Buffer
		code := run(context.Background(), []string{"-url", "https://example.com/share", "-password", "private-password", "-filename", "test.zip"}, &out, func(_ context.Context, share, password, filename string, _ []string, report func(updatefeed.ProbeEvent)) (updatefeed.ProbeResult, error) {
			if share != "https://example.com/share" || password != "private-password" || filename != "test.zip" {
				t.Fatal("arguments not passed")
			}
			report(updatefeed.ProbeEvent{Stage: "http", Host: "example.com", Status: 200})
			if fail {
				return updatefeed.ProbeResult{Host: "example.com"}, errors.New("Lanzou requires human verification in a browser; this source failed")
			}
			return updatefeed.ProbeResult{Host: "example.com", Size: 123, SHA256: strings.Repeat("a", 64)}, nil
		})
		if strings.Contains(out.String(), "private-password") || strings.Contains(out.String(), "/share") {
			t.Fatal("output exposed credentials or URL")
		}
		if fail {
			if code != 1 || !strings.Contains(out.String(), "human verification") || !strings.Contains(out.String(), "bytes=unavailable sha256=unavailable") {
				t.Fatal(out.String())
			}
		} else if code != 0 || !strings.Contains(out.String(), "final_host=example.com bytes=123 sha256=") || !strings.Contains(out.String(), "deleted") {
			t.Fatal(out.String())
		}
	}
}

func TestInvalidArgumentsDoNotEchoSecrets(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), []string{"-bearer-token-secret"}, &out, nil)
	if code != 2 || strings.Contains(out.String(), "bearer-token-secret") {
		t.Fatal(out.String())
	}
}
