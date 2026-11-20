package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func testStaticFiles(t *testing.T) http.FileSystem {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"index.html":      "<h1>Test app</h1>",
		"assets/logo.png": "test asset",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return http.Dir(root)
}

func TestHealthz(t *testing.T) {
	cfg := &apiConfig{}
	cfg.fileserverHits.Store(5)
	response := httptest.NewRecorder()
	cfg.routes(testStaticFiles(t)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))

	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); body != "OK" {
		t.Errorf("body = %q, want OK", body)
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
	}
	if hits := cfg.fileserverHits.Load(); hits != 5 {
		t.Errorf("hits = %d, want 5", hits)
	}
}

func TestRoutesRejectUnprefixedAPIPaths(t *testing.T) {
	cfg := &apiConfig{}
	cfg.fileserverHits.Store(5)
	mux := cfg.routes(testStaticFiles(t))

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/healthz"},
		{method: http.MethodGet, path: "/metrics"},
		{method: http.MethodPost, path: "/reset"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
			if response.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
			if hits := cfg.fileserverHits.Load(); hits != 5 {
				t.Errorf("hits = %d, want 5", hits)
			}
		})
	}
}

func TestRoutesRejectUnsupportedMethods(t *testing.T) {
	files := testStaticFiles(t)
	for _, path := range []string{"/api/reset", "/api/metrics", "/api/healthz"} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
			if path == "/api/reset" && method == http.MethodPost {
				continue
			}
			if path != "/api/reset" && (method == http.MethodGet || method == http.MethodHead) {
				continue
			}
			t.Run(method+" "+path, func(t *testing.T) {
				cfg := &apiConfig{}
				cfg.fileserverHits.Store(5)
				response := httptest.NewRecorder()
				cfg.routes(files).ServeHTTP(response, httptest.NewRequest(method, path, nil))
				if response.Code != http.StatusMethodNotAllowed {
					t.Errorf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
				}
				if hits := cfg.fileserverHits.Load(); hits != 5 {
					t.Errorf("hits = %d, want 5", hits)
				}
			})
		}
	}
}

func TestFileServerMissingFileCountsHit(t *testing.T) {
	cfg := &apiConfig{}
	response := httptest.NewRecorder()
	cfg.routes(testStaticFiles(t)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/missing.txt", nil))

	if response.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if hits := cfg.fileserverHits.Load(); hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
}

func TestInitialMetrics(t *testing.T) {
	cfg := &apiConfig{}
	response := httptest.NewRecorder()
	cfg.routes(testStaticFiles(t)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))

	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); body != "Hits: 0" {
		t.Errorf("body = %q, want Hits: 0", body)
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
	}
	if hits := cfg.fileserverHits.Load(); hits != 0 {
		t.Errorf("hits = %d, want 0", hits)
	}
}

func TestResetAlreadyZero(t *testing.T) {
	cfg := &apiConfig{}
	mux := cfg.routes(testStaticFiles(t))
	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/reset", nil))
		if response.Code != http.StatusOK {
			t.Errorf("reset %d status = %d, want %d", i, response.Code, http.StatusOK)
		}
		if body := response.Body.String(); body != "" {
			t.Errorf("reset %d body = %q, want empty", i, body)
		}
		if hits := cfg.fileserverHits.Load(); hits != 0 {
			t.Errorf("after reset %d hits = %d, want 0", i, hits)
		}
	}
}

func TestMiddlewareMetricsIncConcurrent(t *testing.T) {
	cfg := &apiConfig{}
	handler := cfg.middlewareMetricsInc(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	const requests = 100
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/", nil))
			if response.Code != http.StatusNoContent {
				t.Errorf("status = %d, want %d", response.Code, http.StatusNoContent)
			}
		}()
	}
	wg.Wait()
	if hits := cfg.fileserverHits.Load(); hits != requests {
		t.Errorf("hits = %d, want %d", hits, requests)
	}
}

func TestMiddlewareMetricsInc(t *testing.T) {
	cfg := &apiConfig{}
	calls := 0
	handler := cfg.middlewareMetricsInc(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("X-Test", "preserved")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("wrapped response"))
	}))

	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/", nil))
		if response.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
		}
		if got := response.Header().Get("X-Test"); got != "preserved" {
			t.Errorf("X-Test = %q, want preserved", got)
		}
		if got := response.Body.String(); got != "wrapped response" {
			t.Errorf("body = %q, want wrapped response", got)
		}
	}

	if calls != 3 {
		t.Errorf("next handler calls = %d, want 3", calls)
	}
	if hits := cfg.fileserverHits.Load(); hits != 3 {
		t.Errorf("hits = %d, want 3", hits)
	}
}

func TestFileServerMetricsAfterReset(t *testing.T) {
	cfg := &apiConfig{}
	mux := cfg.routes(testStaticFiles(t))

	for _, step := range []struct {
		path   string
		status int
		hits   int32
	}{
		{path: "/app/", status: http.StatusOK, hits: 1},
		{path: "/api/reset", status: http.StatusOK, hits: 0},
		{path: "/api/metrics", status: http.StatusOK, hits: 0},
		{path: "/app", status: http.StatusTemporaryRedirect, hits: 0},
		{path: "/app/", status: http.StatusOK, hits: 1},
		{path: "/app/assets/logo.png", status: http.StatusOK, hits: 2},
		{path: "/api/metrics", status: http.StatusOK, hits: 2},
	} {
		response := httptest.NewRecorder()
		method := http.MethodGet
		if step.path == "/api/reset" {
			method = http.MethodPost
		}
		mux.ServeHTTP(response, httptest.NewRequest(method, step.path, nil))
		if response.Code != step.status {
			t.Fatalf("%s status = %d, want %d", step.path, response.Code, step.status)
		}
		if hits := cfg.fileserverHits.Load(); hits != step.hits {
			t.Fatalf("after %s hits = %d, want %d", step.path, hits, step.hits)
		}
		if step.path == "/api/metrics" && step.hits == 2 && response.Body.String() != "Hits: 2" {
			t.Errorf("metrics body = %q, want Hits: 2", response.Body.String())
		}
	}
}

func TestMetricsAndReset(t *testing.T) {
	cfg := &apiConfig{}
	mux := cfg.routes(testStaticFiles(t))

	cfg.fileserverHits.Store(5)
	for _, step := range []struct {
		path string
		body string
	}{
		{path: "/api/metrics", body: "Hits: 5"},
		{path: "/api/metrics", body: "Hits: 5"},
		{path: "/api/reset", body: ""},
		{path: "/api/metrics", body: "Hits: 0"},
	} {
		response := httptest.NewRecorder()
		method := http.MethodGet
		if step.path == "/api/reset" {
			method = http.MethodPost
		}
		mux.ServeHTTP(response, httptest.NewRequest(method, step.path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("%s status = %d, want %d", step.path, response.Code, http.StatusOK)
		}
		if body := response.Body.String(); body != step.body {
			t.Errorf("%s body = %q, want %q", step.path, body, step.body)
		}
		if step.path == "/api/metrics" {
			if contentType := response.Header().Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
				t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", contentType)
			}
		}
	}
}
