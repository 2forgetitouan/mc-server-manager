package modrinth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const BaseURL = "https://api.modrinth.com/v2"

// Sentinel errors returned by Client methods.
var (
	ErrNotFound    = errors.New("modrinth: resource not found")
	ErrRateLimited = errors.New("modrinth: rate limited")
	ErrServer      = errors.New("modrinth: server error")
)

const maxRetries = 3

// Client is an HTTP client for the Modrinth v2 API.
type Client struct {
	httpClient *http.Client
	userAgent  string
	baseURL    string
}

// NewClient creates a new Modrinth API client. The userAgent string is sent
// with every request as required by the Modrinth API guidelines. If timeout
// is zero a default of 30 seconds is used.
func NewClient(userAgent string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		userAgent:  userAgent,
		baseURL:    BaseURL,
	}
}

// GetProject fetches a project by its ID or slug.
func (c *Client) GetProject(ctx context.Context, idOrSlug string) (*Project, error) {
	path := fmt.Sprintf("/project/%s", url.PathEscape(idOrSlug))
	var project Project
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &project); err != nil {
		return nil, fmt.Errorf("fetching project %q: %w", idOrSlug, err)
	}
	return &project, nil
}

// GetVersions lists versions for a project, optionally filtered by game
// versions and loaders. Both filter parameters accept nil to skip filtering.
func (c *Client) GetVersions(ctx context.Context, projectID string, gameVersions []string, loaders []string) ([]Version, error) {
	path := fmt.Sprintf("/project/%s/version", url.PathEscape(projectID))

	params := url.Values{}
	if len(gameVersions) > 0 {
		params.Set("game_versions", toJSONArray(gameVersions))
	}
	if len(loaders) > 0 {
		params.Set("loaders", toJSONArray(loaders))
	}
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var versions []Version
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &versions); err != nil {
		return nil, fmt.Errorf("fetching versions for project %q: %w", projectID, err)
	}
	return versions, nil
}

// GetVersion fetches a single version by its ID.
func (c *Client) GetVersion(ctx context.Context, versionID string) (*Version, error) {
	path := fmt.Sprintf("/version/%s", url.PathEscape(versionID))
	var version Version
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &version); err != nil {
		return nil, fmt.Errorf("fetching version %q: %w", versionID, err)
	}
	return &version, nil
}

// VersionFromHash looks up a version by file hash.
func (c *Client) VersionFromHash(ctx context.Context, hash string, algorithm string) (*Version, error) {
	path := fmt.Sprintf("/version_file/%s?algorithm=%s", url.PathEscape(hash), url.QueryEscape(algorithm))
	var version Version
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &version); err != nil {
		return nil, fmt.Errorf("looking up version by hash %q: %w", hash[:min(len(hash), 16)]+"...", err)
	}
	return &version, nil
}

// VersionsFromHashes looks up multiple versions by their file hashes in a
// single request.
func (c *Client) VersionsFromHashes(ctx context.Context, hashes []string, algorithm string) (HashLookupResult, error) {
	body := struct {
		Hashes    []string `json:"hashes"`
		Algorithm string   `json:"algorithm"`
	}{
		Hashes:    hashes,
		Algorithm: algorithm,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling hash lookup request: %w", err)
	}

	var result HashLookupResult
	if err := c.doRequest(ctx, http.MethodPost, "/version_files", bytes.NewReader(payload), &result); err != nil {
		return nil, fmt.Errorf("looking up versions by hashes: %w", err)
	}
	return result, nil
}

// DownloadFile downloads a file from the given URL into dest. It writes to a
// temporary file first and returns once the download is complete. The caller is
// responsible for any atomic rename.
func (c *Client) DownloadFile(ctx context.Context, fileURL string, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return fmt.Errorf("creating download request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", fileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: unexpected status %d", fileURL, resp.StatusCode)
	}

	// Create a temporary file in the same directory as dest so that a rename
	// (by the caller) is atomic on the same filesystem.
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".mc-download-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	// Clean up the temp file on any failure.
	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	written, err := io.Copy(tmp, resp.Body)
	if err != nil {
		return fmt.Errorf("writing download to %s: %w", tmpPath, err)
	}

	// Validate content length if the server provided one.
	if resp.ContentLength > 0 && written != resp.ContentLength {
		return fmt.Errorf("download size mismatch for %s: got %d bytes, expected %d", fileURL, written, resp.ContentLength)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file %s: %w", tmpPath, err)
	}

	// Rename the temp file to the final destination.
	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, dest, err)
	}

	success = true
	return nil
}

// doRequest executes an HTTP request against the Modrinth API. It handles
// rate-limiting retries, error status codes, and JSON decoding.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, result interface{}) error {
	fullURL := c.baseURL + path

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// If the body is a *bytes.Reader, reset it for retries.
		if br, ok := body.(*bytes.Reader); ok && attempt > 0 {
			br.Seek(0, io.SeekStart)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("User-Agent", c.userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("executing request %s %s: %w", method, path, err)
		}

		// Handle rate limiting.
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()

			if attempt >= maxRetries {
				return fmt.Errorf("%w: exceeded %d retries for %s %s", ErrRateLimited, maxRetries, method, path)
			}

			wait := retryAfterDuration(resp)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
				continue
			}
		}

		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response body for %s %s: %w", method, path, err)
		}

		switch {
		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("%w: %s %s", ErrNotFound, method, path)
		case resp.StatusCode >= 500:
			return fmt.Errorf("%w: %s %s returned %d", ErrServer, method, path, resp.StatusCode)
		case resp.StatusCode < 200 || resp.StatusCode >= 300:
			return fmt.Errorf("unexpected status %d for %s %s: %s", resp.StatusCode, method, path, truncate(string(respBody), 200))
		}

		if result != nil {
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("decoding JSON response for %s %s: %w", method, path, err)
			}
		}

		return nil
	}

	// Should not be reached.
	return fmt.Errorf("request loop exited unexpectedly for %s %s", method, path)
}

// retryAfterDuration reads the Retry-After header and returns the duration to
// wait. Falls back to 1 second if the header is missing or unparseable.
func retryAfterDuration(resp *http.Response) time.Duration {
	val := resp.Header.Get("Retry-After")
	if val == "" {
		return 1 * time.Second
	}

	// Try parsing as seconds.
	if seconds, err := strconv.Atoi(val); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP-date.
	if t, err := http.ParseTime(val); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}

	return 1 * time.Second
}

// toJSONArray produces a JSON array string like `["a","b"]` for use in query
// parameters.
func toJSONArray(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// truncate returns s truncated to n characters, with "..." appended if it was
// shortened.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
