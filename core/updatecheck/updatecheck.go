package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"github.com/anyproto/anytype-cli/core/config"
)

const (
	cacheFileName   = "update-check.json"
	refreshInterval = 24 * time.Hour
	fetchTimeout    = 3 * time.Second
	latestURL       = "https://api.github.com/repos/anyproto/anytype-cli/releases/latest"
)

type cache struct {
	Latest    string    `json:"latest"`
	CheckedAt time.Time `json:"checkedAt"`
}

var (
	mu       sync.RWMutex
	latest   string
	disabled bool
)

// Disable turns off the update check for the lifetime of this process.
// Useful for long-running modes like the interactive shell.
func Disable() {
	mu.Lock()
	disabled = true
	mu.Unlock()
}

// Start seeds the latest-version result from the on-disk cache and, if the
// cache is stale or missing, kicks off a background refresh. Silent on failure.
func Start(ctx context.Context) {
	mu.RLock()
	off := disabled
	mu.RUnlock()
	if off {
		return
	}

	path := cachePath()
	c, err := readCache(path)
	if err == nil {
		mu.Lock()
		latest = c.Latest
		mu.Unlock()
		if time.Since(c.CheckedAt) < refreshInterval {
			return
		}
	}

	go func() {
		v, err := fetchLatest(ctx)
		if err != nil {
			return
		}
		mu.Lock()
		latest = v
		mu.Unlock()
		_ = writeCache(path, cache{Latest: v, CheckedAt: time.Now()})
	}()
}

// Hint returns a user-facing message if the cached latest version is newer
// than current. Returns ("", false) when disabled, on dev builds, when no
// cached value is available, or when current is already up to date.
func Hint(current string) (string, bool) {
	mu.RLock()
	off := disabled
	v := latest
	mu.RUnlock()
	if off || v == "" {
		return "", false
	}
	if !strings.HasPrefix(current, "v") {
		return "", false
	}
	base := current
	if idx := strings.Index(base, "-"); idx != -1 {
		base = base[:idx]
	}
	if semver.Compare(base, v) >= 0 {
		return "", false
	}
	return fmt.Sprintf("A new version (%s) is available. Run: anytype update", v), true
}

func cachePath() string {
	return filepath.Join(config.GetConfigDir(), cacheFileName)
}

func readCache(path string) (cache, error) {
	var c cache
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	return c, nil
}

func writeCache(path string, c cache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fetchLatest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", latestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}
