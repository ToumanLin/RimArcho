package lanzou

import (
	"net/http"
	"testing"
	"time"
)

type unexpectedTransport struct{ t *testing.T }

func (r unexpectedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Fatal("Lanzou used the global proxy/mirror wrapper")
	return nil, nil
}

func TestDirectTransportIgnoresWrappedDefault(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = unexpectedTransport{t}
	t.Cleanup(func() { http.DefaultTransport = old })
	r, err := newRunner(nil, []string{"example.com"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer r.client.CloseIdleConnections()
	transport, ok := r.client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.DialContext == nil || transport.TLSHandshakeTimeout != 5*time.Second || transport.ResponseHeaderTimeout != 10*time.Second || transport.MaxResponseHeaderBytes != 16<<10 || transport.MaxConnsPerHost != 2 || !transport.DisableCompression || !transport.DisableKeepAlives {
		t.Fatalf("lost direct transport bounds: %+v", transport)
	}
	if _, err := newRunner(&http.Client{Transport: unexpectedTransport{t}}, []string{"example.com"}, ""); err == nil {
		t.Fatal("explicit unbounded transport accepted")
	}
}
