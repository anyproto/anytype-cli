// Package localstate provides the default instance's durable coordination and
// private files. It deliberately has no dependency on RPCs or credential APIs.
package localstate

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/gofrs/flock"
)

var ErrBusy = errors.New("another CLI operation is in progress")

func Dir() string { return filepath.Join(config.GetConfigDir(), "automation") }

func Lock(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f := flock.New(filepath.Join(dir, "instance.lock"))
	ok, err := f.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		_ = f.Close()
		return nil, ErrBusy
	}
	return func() { _ = f.Unlock(); _ = f.Close() }, nil
}

// WriteJSON replaces a private file without truncating the previous version.
// Contents are synced before a platform-specific durable replacement.
func WriteJSON(path string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := createPrivateTemp(dir)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceFile(f.Name(), path)
}

// ReadPrivateJSON checks the opened file's privacy before reading secrets.
// Symlinks and replacement between lstat/open are rejected.
func ReadPrivateJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrPermission
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return os.ErrPermission
	}
	if err = CheckPrivateFile(f); err != nil {
		return err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

func ReadJSON(path string, value any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

// HumanMutation identifies commands sharing mutable native instance state. Shell
// lines and ordinary process invocations use the same classification.
func HumanMutation(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return false
		}
	}
	var words []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			words = append(words, a)
		}
	}
	if len(words) < 2 {
		return false
	}
	switch words[0] {
	case "auth":
		return words[1] != "status" && (len(words) < 3 || words[1] != "apikey" || words[2] != "list")
	case "space":
		return words[1] != "list"
	case "service":
		return words[1] != "status"
	case "config":
		return words[1] != "get"
	}
	return false
}
