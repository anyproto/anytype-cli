package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/zalando/go-keyring"
)

func storeFixture(t *testing.T) *Store {
	t.Helper()
	t.Setenv(BackendEnv, "")
	dir := t.TempDir()
	s := New(filepath.Join(dir, "automation"), filepath.Join(dir, "config.json"))
	s.DeleteKeyring = func(string, string) error { return os.ErrPermission }
	s.GetKeyring = func(string, string) (string, error) { return "", os.ErrNotExist }
	s.SetKeyring = func(string, string, string) error { return os.ErrPermission }
	return s
}

func TestKeyringAbsenceIsDistinctFromUnreadableEvidence(t *testing.T) {
	for _, readErr := range []error{keyring.ErrNotFound, os.ErrNotExist, os.ErrPermission, errors.New("credential service unavailable")} {
		s := storeFixture(t)
		s.GetKeyring = func(string, string) (string, error) { return "", readErr }
		_, _, err := s.Get("account-key")
		missing := errors.Is(readErr, keyring.ErrNotFound) || errors.Is(readErr, os.ErrNotExist)
		if missing != errors.Is(err, os.ErrNotExist) || !missing && !errors.Is(err, readErr) {
			t.Fatal("credential evidence misclassified", err)
		}
	}
}

func TestExplicitPrivateBackendPreservesKeyringAndSurvivesRestart(t *testing.T) {
	s := storeFixture(t)
	t.Setenv(BackendEnv, "private_file")
	s.Backend = os.Getenv(BackendEnv)
	keyringCalls := 0
	s.GetKeyring = func(string, string) (string, error) { keyringCalls++; return "legacy-secret", nil }
	s.SetKeyring = func(string, string, string) error { keyringCalls++; return nil }
	s.DeleteKeyring = func(string, string) error { keyringCalls++; return nil }
	if _, _, err := s.Get("account-key"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("selector read wrote state")
	}
	if backend, err := s.Set("account-key", "new-secret"); err != nil || backend != "private_file" {
		t.Fatal(backend, err)
	}
	t.Setenv(BackendEnv, "")
	restarted := New(s.Dir, s.ConfigPath)
	restarted.GetKeyring, restarted.SetKeyring, restarted.DeleteKeyring = s.GetKeyring, s.SetKeyring, s.DeleteKeyring
	if value, backend, err := restarted.Get("account-key"); err != nil || value != "new-secret" || backend != "private_file" {
		t.Fatal(value, backend, err)
	}
	if backend, err := restarted.Set("session-token", "new-session"); err != nil || backend != "private_file" {
		t.Fatal(backend, err)
	}
	if err := restarted.Delete("account-key"); err != nil || keyringCalls != 0 {
		t.Fatal("private backend touched legacy keyring", keyringCalls, err)
	}
}

func TestPrivateSelectorDoesNotMigrateExistingKeyringMarker(t *testing.T) {
	s := storeFixture(t)
	s.SetKeyring = func(string, string, string) error { return nil }
	if _, err := s.Set("account-key", "keyring-secret"); err != nil {
		t.Fatal(err)
	}
	s.Backend = "private_file"
	s.GetKeyring = func(string, string) (string, error) { return "", os.ErrPermission }
	s.SetKeyring = func(string, string, string) error { return os.ErrPermission }
	if _, backend, err := s.Get("account-key"); !errors.Is(err, os.ErrPermission) || backend != "system_keyring" {
		t.Fatal(backend, err)
	}
	if _, err := s.Set("account-key", "replacement"); !errors.Is(err, os.ErrPermission) {
		t.Fatal("keyring marker migrated", err)
	}
	var marker storedValue
	if err := privateRead(s.file("account-key"), &marker); err != nil || marker.Backend != "system_keyring" || marker.Secret != "" {
		t.Fatal(marker, err)
	}
}
func TestReadDoesNotWriteOrProbeKeyring(t *testing.T) {
	s := storeFixture(t)
	calls := 0
	s.SetKeyring = func(string, string, string) error { calls++; return nil }
	if _, _, err := s.Get("account-key"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("read mutated keyring")
	}
	if _, err := os.Stat(s.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read created directory")
	}
}
func TestFallbackOverridesStaleKeyring(t *testing.T) {
	s := storeFixture(t)
	s.GetKeyring = func(string, string) (string, error) { return "stale", nil }
	backend, err := s.Set("account-key", "new-fixture")
	if err != nil || backend != "private_file" {
		t.Fatal(backend, err)
	}
	value, backend, err := s.Get("account-key")
	if err != nil || value != "new-fixture" || backend != "private_file" {
		t.Fatal(backend, err)
	}
	if err = s.Delete("account-key"); err != nil {
		t.Fatal(err)
	}
	value, _, err = s.Get("account-key")
	if err != nil || value != "" {
		t.Fatal("stale key reappeared")
	}
}
func TestPrivateFileRejectsPublicAndSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX chmod/symlink semantics; Windows ACL rejection is covered in localstate")
	}
	for _, kind := range []string{"public", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			s := storeFixture(t)
			if _, err := s.Set("account-key", "fixture"); err != nil {
				t.Fatal(err)
			}
			file := s.file("account-key")
			if kind == "public" {
				if err := os.Chmod(file, 0644); err != nil {
					t.Fatal(err)
				}
			} else {
				target := file + ".target"
				if err := os.Rename(file, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, file); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := s.Get("account-key"); err == nil {
				t.Fatal("insecure file accepted")
			}
		})
	}
}

func TestKeyringMarkerRoundTrip(t *testing.T) {
	s := storeFixture(t)
	s.SetKeyring = func(string, string, string) error { return nil }
	s.GetKeyring = func(string, string) (string, error) { return "keyring-fixture", nil }
	if backend, err := s.Set("account-key", "keyring-fixture"); err != nil || backend != "system_keyring" {
		t.Fatal(backend, err)
	}
	if value, backend, err := s.Get("account-key"); err != nil || value != "keyring-fixture" || backend != "system_keyring" {
		t.Fatal(value, backend, err)
	}
}
func TestReadsLegacyConfigWithoutMutation(t *testing.T) {
	s := storeFixture(t)
	if err := localstate.WriteJSON(s.ConfigPath, map[string]string{"accountKey": "legacy-fixture"}); err != nil {
		t.Fatal(err)
	}
	value, backend, err := s.Get("account-key")
	if err != nil || value != "legacy-fixture" || backend != "legacy_config_file" {
		t.Fatal(backend, err)
	}
	if _, err := os.Stat(s.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspection migrated storage")
	}
}
func TestDeleteRemovesReachableKeyringAndLegacySecret(t *testing.T) {
	s := storeFixture(t)
	deleted := ""
	s.DeleteKeyring = func(_, name string) error { deleted = name; return nil }
	if err := localstate.WriteJSON(s.ConfigPath, map[string]string{"accountKey": "old", "apiListenAddr": "127.0.0.1:4000"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("account-key"); err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := localstate.ReadJSON(s.ConfigPath, &values); err != nil {
		t.Fatal(err)
	}
	if deleted != "account-key" || values["accountKey"] != "" || values["apiListenAddr"] == "" {
		t.Fatal("deletion lost compatibility", values)
	}
}
