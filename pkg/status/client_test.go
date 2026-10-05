package status

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestClientSetupRequiredTrue(t *testing.T) {
	client := NewClient("http://127.0.0.1:8080/setup/status", 1*time.Second)
	client.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"setup_required": true}`)),
			Header:     make(http.Header),
		}
	}))

	res := client.Check(context.Background())
	if !res.Available {
		t.Fatalf("expected available=true, got err: %v", res.Error)
	}
	if !res.SetupRequired {
		t.Errorf("expected setup_required=true, got false")
	}
}

func TestClientSetupRequiredFalse(t *testing.T) {
	client := NewClient("http://127.0.0.1:8080/setup/status", 1*time.Second)
	client.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"setup_required": false}`)),
			Header:     make(http.Header),
		}
	}))

	res := client.Check(context.Background())
	if !res.Available {
		t.Fatalf("expected available=true, got err: %v", res.Error)
	}
	if res.SetupRequired {
		t.Errorf("expected setup_required=false, got true")
	}
}

func TestClientFallbackOn404(t *testing.T) {
	client := NewClient("http://127.0.0.1:8080/setup/status", 1*time.Second)
	client.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		if req.URL.Path == "/setup/status" {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString("not found")),
				Header:     make(http.Header),
			}
		}
		if req.URL.Path == "/api/v1/setup/status" {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"setup_required": true}`)),
				Header:     make(http.Header),
			}
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("not found")),
			Header:     make(http.Header),
		}
	}))

	res := client.Check(context.Background())
	if !res.Available {
		t.Fatalf("expected fallback to succeed, got error: %v", res.Error)
	}
	if !res.SetupRequired {
		t.Errorf("expected setup_required=true from fallback endpoint, got false")
	}
}
