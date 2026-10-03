package modrinth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := NewClient("test-agent/1.0", 10*time.Second)
	c.baseURL = srv.URL
	return c, srv
}

func TestGetProject(t *testing.T) {
	project := Project{
		ID:           "AABBCCDD",
		Slug:         "sodium",
		Title:        "Sodium",
		Description:  "A Minecraft mod to improve frame rates",
		ServerSide:   "optional",
		ClientSide:   "required",
		GameVersions: []string{"1.20.4", "1.21.1"},
		Loaders:      []string{"fabric", "forge"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project/sodium" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") != "test-agent/1.0" {
			t.Errorf("User-Agent = %q, want test-agent/1.0", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(project)
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	got, err := client.GetProject(context.Background(), "sodium")
	if err != nil {
		t.Fatalf("GetProject failed: %v", err)
	}

	if got.ID != project.ID {
		t.Errorf("ID = %q, want %q", got.ID, project.ID)
	}
	if got.Slug != project.Slug {
		t.Errorf("Slug = %q, want %q", got.Slug, project.Slug)
	}
	if got.Title != project.Title {
		t.Errorf("Title = %q, want %q", got.Title, project.Title)
	}
	if got.ServerSide != project.ServerSide {
		t.Errorf("ServerSide = %q, want %q", got.ServerSide, project.ServerSide)
	}
}

func TestGetVersions(t *testing.T) {
	versions := []Version{
		{
			ID:            "ver1",
			ProjectID:     "AABBCCDD",
			Name:          "v1.0.0",
			VersionNumber: "1.0.0",
			GameVersions:  []string{"1.21.1"},
			Loaders:       []string{"fabric"},
			VersionType:   "release",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project/AABBCCDD/version" {
			http.NotFound(w, r)
			return
		}
		// Check that filters are passed as query params.
		gv := r.URL.Query().Get("game_versions")
		if gv == "" {
			t.Error("expected game_versions query parameter")
		}
		loaders := r.URL.Query().Get("loaders")
		if loaders == "" {
			t.Error("expected loaders query parameter")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(versions)
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	got, err := client.GetVersions(context.Background(), "AABBCCDD", []string{"1.21.1"}, []string{"fabric"})
	if err != nil {
		t.Fatalf("GetVersions failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(got))
	}
	if got[0].ID != "ver1" {
		t.Errorf("version ID = %q, want ver1", got[0].ID)
	}
	if got[0].VersionNumber != "1.0.0" {
		t.Errorf("VersionNumber = %q, want 1.0.0", got[0].VersionNumber)
	}
}

func TestGetVersions_NoFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No query params should be present.
		if r.URL.RawQuery != "" {
			t.Errorf("unexpected query string: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Version{})
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	_, err := client.GetVersions(context.Background(), "test", nil, nil)
	if err != nil {
		t.Fatalf("GetVersions failed: %v", err)
	}
}

func TestVersionFromHash(t *testing.T) {
	version := Version{
		ID:            "ver-hash",
		ProjectID:     "proj1",
		VersionNumber: "2.0.0",
		VersionType:   "release",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version_file/abc123def456" {
			http.NotFound(w, r)
			return
		}
		algo := r.URL.Query().Get("algorithm")
		if algo != "sha512" {
			t.Errorf("algorithm = %q, want sha512", algo)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(version)
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	got, err := client.VersionFromHash(context.Background(), "abc123def456", "sha512")
	if err != nil {
		t.Fatalf("VersionFromHash failed: %v", err)
	}

	if got.ID != "ver-hash" {
		t.Errorf("ID = %q, want ver-hash", got.ID)
	}
}

func TestVersionsFromHashes(t *testing.T) {
	result := HashLookupResult{
		"hash1": Version{ID: "v1", ProjectID: "p1"},
		"hash2": Version{ID: "v2", ProjectID: "p2"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version_files" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	got, err := client.VersionsFromHashes(context.Background(), []string{"hash1", "hash2"}, "sha512")
	if err != nil {
		t.Fatalf("VersionsFromHashes failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(got))
	}
	if got["hash1"].ID != "v1" {
		t.Errorf("got[hash1].ID = %q, want v1", got["hash1"].ID)
	}
	if got["hash2"].ID != "v2" {
		t.Errorf("got[hash2].ID = %q, want v2", got["hash2"].ID)
	}
}

func TestDownloadFile(t *testing.T) {
	fileContent := "this is the downloaded mod file content"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "39")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fileContent))
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := NewClient("test-agent/1.0", 10*time.Second)

	dir := t.TempDir()
	dest := filepath.Join(dir, "test-mod.jar")

	err := client.DownloadFile(context.Background(), srv.URL+"/file.jar", dest)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != fileContent {
		t.Errorf("file content = %q, want %q", string(data), fileContent)
	}
}

func TestDownloadFile_HTTPError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := NewClient("test-agent/1.0", 10*time.Second)

	dir := t.TempDir()
	dest := filepath.Join(dir, "test-mod.jar")

	err := client.DownloadFile(context.Background(), srv.URL+"/file.jar", dest)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}

	// Temp file should be cleaned up.
	if _, err := os.Stat(dest); err == nil {
		t.Error("destination file should not exist after failure")
	}
}

func TestHTTP404(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	_, err := client.GetProject(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestHTTP429RateLimit(t *testing.T) {
	attempts := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		// On the 4th attempt (attempt > maxRetries), we should still get rate limit error.
		// Actually maxRetries is 3, so attempts 1-3 get 429, and attempt 4 (the last retry) also gets 429.
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	_, err := client.GetProject(context.Background(), "test")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
}

func TestHTTP429RateLimit_EventualSuccess(t *testing.T) {
	attempts := 0
	project := Project{ID: "proj1", Title: "Test"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(project)
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	got, err := client.GetProject(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if got.ID != "proj1" {
		t.Errorf("ID = %q, want proj1", got.ID)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestHTTP500(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	_, err := client.GetProject(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !errors.Is(err, ErrServer) {
		t.Errorf("error = %v, want ErrServer", err)
	}
}

func TestInvalidJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	_, err := client.GetProject(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestContextCancellation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.GetProject(ctx, "test")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	c := NewClient("test/1.0", 0)
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", c.httpClient.Timeout)
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	c := NewClient("test/1.0", 60*time.Second)
	if c.httpClient.Timeout != 60*time.Second {
		t.Errorf("timeout = %v, want 60s", c.httpClient.Timeout)
	}
}

func TestToJSONArray(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"a"}, `["a"]`},
		{[]string{"a", "b", "c"}, `["a","b","c"]`},
	}

	for _, tt := range tests {
		got := toJSONArray(tt.input)
		if got != tt.want {
			t.Errorf("toJSONArray(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 5, "this ..."},
	}

	for _, tt := range tests {
		got := truncate(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestRetryAfterDuration(t *testing.T) {
	tests := []struct {
		name      string
		headerVal string
		wantMin   time.Duration
		wantMax   time.Duration
	}{
		{"empty header", "", 1 * time.Second, 1 * time.Second},
		{"2 seconds", "2", 2 * time.Second, 2 * time.Second},
		{"invalid", "not-a-number", 1 * time.Second, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.headerVal != "" {
				resp.Header.Set("Retry-After", tt.headerVal)
			}
			got := retryAfterDuration(resp)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("retryAfterDuration() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	version := Version{
		ID:            "ver-single",
		ProjectID:     "proj1",
		VersionNumber: "3.0.0",
		VersionType:   "release",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version/ver-single" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(version)
	})

	client, srv := newTestClient(t, handler)
	defer srv.Close()

	got, err := client.GetVersion(context.Background(), "ver-single")
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if got.VersionNumber != "3.0.0" {
		t.Errorf("VersionNumber = %q, want 3.0.0", got.VersionNumber)
	}
}
