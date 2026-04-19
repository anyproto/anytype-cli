package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/mod/semver"

	"github.com/anyproto/anytype-cli/core/config"
)

const (
	cacheFileName   = "update-check.json"
	refreshInterval = 24 * time.Hour
	checkDeadline   = 400 * time.Millisecond
	hintWait        = 200 * time.Millisecond
	latestURL       = "https://api.github.com/repos/anyproto/anytype-cli/releases/latest"
)

type cache struct {
	Latest    string    `json:"latest"`
	CheckedAt time.Time `json:"checkedAt"`
}

var disabled atomic.Bool

// Disable suppresses the check for the rest of the process. Used by the
// interactive shell, which re-executes rootCmd for each nested command.
func Disable() { disabled.Store(true) }

func Start(ctx context.Context) <-chan string {
	if disabled.Load() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, checkDeadline)
	ch := make(chan string, 1)
	go func() {
		defer cancel()
		defer close(ch)
		if v, ok := latestVersion(ctx); ok {
			ch <- v
		}
	}()
	return ch
}

func Hint(ch <-chan string, current string) (string, bool) {
	if ch == nil || !strings.HasPrefix(current, "v") {
		return "", false
	}

	var v string
	select {
	case v = <-ch:
	case <-time.After(hintWait):
		return "", false
	}
	if v == "" {
		return "", false
	}

	base := current
	if i := strings.Index(base, "-"); i != -1 {
		base = base[:i]
	}
	if semver.Compare(base, v) >= 0 {
		return "", false
	}
	return fmt.Sprintf("A new version (%s) is available. Run: anytype update", v), true
}

func latestVersion(ctx context.Context) (string, bool) {
	path := cachePath()
	c, cacheErr := readCache(path)
	if cacheErr == nil && time.Since(c.CheckedAt) < refreshInterval {
		return c.Latest, true
	}

	v, err := fetchLatest(ctx)
	if err == nil {
		_ = writeCache(path, cache{Latest: v, CheckedAt: time.Now()})
		return v, true
	}

	if cacheErr == nil && c.Latest != "" {
		return c.Latest, true
	}
	return "", false
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
