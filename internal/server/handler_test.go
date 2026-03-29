package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"sauron/internal/tiamat"
)

type stubTiamatControl struct {
	startFn  func(context.Context) error
	stopFn   func(context.Context) error
	canStart bool
	canStop  bool
}

func (s stubTiamatControl) Start(ctx context.Context) error {
	if s.startFn != nil {
		return s.startFn(ctx)
	}
	return nil
}

func (s stubTiamatControl) Stop(ctx context.Context) error {
	if s.stopFn != nil {
		return s.stopFn(ctx)
	}
	return nil
}

func (s stubTiamatControl) CanStart() bool { return s.canStart }

func (s stubTiamatControl) CanStop() bool { return s.canStop }

func TestStaticCSS(t *testing.T) {
	ts := httptest.NewServer((&Handler{
		Client:   tiamat.New("http://example.com", ""),
		TokenSet: false,
	}).Routes())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/static/css/tiamat-theme.css")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestDashboard_IncludesNav(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case "/api/hub/v1/status":
			if r.Header.Get("Authorization") != "Bearer x" {
				t.Fatal("missing bearer")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uptime_seconds": 1.5,
				"version":        "test",
				"db_ok":          true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()

	h := &Handler{
		Client:   tiamat.New(up.URL, "x"),
		TokenSet: true,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	raw, _ := io.ReadAll(res.Body)
	htm := string(raw)
	if !strings.Contains(htm, "hx-boost") || !strings.Contains(htm, "Sauron") {
		snippet := htm
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		t.Fatalf("unexpected body: %s", snippet)
	}
	if strings.Contains(htm, "/restart") {
		t.Fatal("restart link should be removed from nav")
	}
}

func TestStatusPartial_Refreshes(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/hub/v1/status" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uptime_seconds":1,"version":"v","db_ok":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer up.Close()

	h := &Handler{Client: tiamat.New(up.URL, "x"), TokenSet: true}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/partials/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}

var noRedirectClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func TestPostTiamatStart_notConfigured(t *testing.T) {
	h := &Handler{Client: tiamat.New("http://127.0.0.1:1", ""), TiamatControl: nil}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	res, err := noRedirectClient.PostForm(srv.URL+"/tiamat/start", map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "start_nc=1") {
		t.Fatalf("location %q", loc)
	}
}

func TestPostTiamatStart_alreadyUp(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer up.Close()

	h := &Handler{
		Client: tiamat.New(up.URL, ""),
		TiamatControl: stubTiamatControl{
			canStart: true,
		},
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	res, err := noRedirectClient.PostForm(srv.URL+"/tiamat/start", map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "start_already=1") {
		t.Fatalf("location %q", loc)
	}
}

func TestPostTiamatStart_stubFlipsHealthz(t *testing.T) {
	var mu sync.Mutex
	healthzOK := false
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		ok := healthzOK
		mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	st := stubTiamatControl{
		canStart: true,
		startFn: func(ctx context.Context) error {
			mu.Lock()
			healthzOK = true
			mu.Unlock()
			return nil
		},
	}

	h := &Handler{
		Client:        tiamat.New(up.URL, ""),
		TiamatControl: st,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	res, err := noRedirectClient.PostForm(srv.URL+"/tiamat/start", map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "start_ok=1") {
		t.Fatalf("location %q", loc)
	}
}

func TestPostTiamatStop_notConfigured(t *testing.T) {
	h := &Handler{Client: tiamat.New("http://127.0.0.1:1", ""), TiamatControl: nil}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	res, err := noRedirectClient.PostForm(srv.URL+"/tiamat/stop", map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "stop_nc=1") {
		t.Fatalf("location %q", loc)
	}
}

func TestPostTiamatStop_alreadyDown(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.NotFound(w, r)
	}))
	defer up.Close()

	h := &Handler{
		Client: tiamat.New(up.URL, ""),
		TiamatControl: stubTiamatControl{
			canStop: true,
		},
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	res, err := noRedirectClient.PostForm(srv.URL+"/tiamat/stop", map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "stop_already=1") {
		t.Fatalf("location %q", loc)
	}
}

func TestPostTiamatStop_stubFlipsHealthzDown(t *testing.T) {
	var mu sync.Mutex
	healthzOK := true
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		ok := healthzOK
		mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	st := stubTiamatControl{
		canStop: true,
		stopFn: func(ctx context.Context) error {
			mu.Lock()
			healthzOK = false
			mu.Unlock()
			return nil
		},
	}

	h := &Handler{
		Client:        tiamat.New(up.URL, ""),
		TiamatControl: st,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	res, err := noRedirectClient.PostForm(srv.URL+"/tiamat/stop", map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "stop_ok=1") {
		t.Fatalf("location %q", loc)
	}
}
