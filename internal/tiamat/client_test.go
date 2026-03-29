package tiamat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := New(ts.URL, "")
	ok, body, err := c.Healthz(context.Background())
	if err != nil || !ok || body != "ok" {
		t.Fatalf("ok=%v body=%q err=%v", ok, body, err)
	}
}

func TestNormalizeLocalhostToIPv4(t *testing.T) {
	if got := normalizeLocalhostToIPv4("http://localhost:8080"); got != "http://127.0.0.1:8080" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeLocalhostToIPv4("https://localhost:8443/foo"); got != "https://127.0.0.1:8443/foo" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeLocalhostToIPv4("http://127.0.0.1:8080"); got != "http://127.0.0.1:8080" {
		t.Fatalf("got %q", got)
	}
}

func TestHubDisabled_503(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"hub API disabled on server"}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "secret")
	_, err := c.Status(context.Background())
	if !errors.Is(err, ErrHubDisabled) {
		t.Fatalf("expected ErrHubDisabled, got %v", err)
	}
}
