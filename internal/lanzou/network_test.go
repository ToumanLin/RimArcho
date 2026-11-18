package lanzou

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestDNSPinning(t *testing.T) {
	calls := 0
	var destination string
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		calls++
		destination = address
		return nil, errors.New("synthetic dial")
	}
	for _, answers := range [][]netip.Addr{
		{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("169.254.169.254")},
	} {
		lookup := func(context.Context, string, string) ([]netip.Addr, error) { return answers, nil }
		_, err := dialPublic(context.Background(), "tcp", "example.com:443", lookup, dial)
		if err == nil || calls != 0 {
			t.Fatal("non-public DNS answer reached the dialer")
		}
	}
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
	}
	_, _ = dialPublic(context.Background(), "tcp", "example.com:443", lookup, dial)
	if calls != 1 || destination != "1.1.1.1:443" {
		t.Fatal("dial did not pin the checked address")
	}
}
