package secret

import (
	"errors"
	"testing"
)

// The test writes to the real Credential Manager under a name no player setting uses, and removes it.
func TestRoundTrip(t *testing.T) {
	const name = "test/secret-roundtrip"
	t.Cleanup(func() { _ = Delete(name) })
	if err := Set(name, "sk-test-schlüssel-123"); err != nil {
		t.Fatal(err)
	}
	got, err := Get(name)
	if err != nil || got != "sk-test-schlüssel-123" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := Set(name, "second"); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(name); got != "second" {
		t.Fatalf("replaced value = %q", got)
	}
	if err := Delete(name); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := Delete(name); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}
