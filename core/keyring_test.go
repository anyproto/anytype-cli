package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func isolatedCredentials(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldPaths := credentialPaths
	oldUnavailable := keyringUnavailable
	credentialPaths = func() (string, string) { return filepath.Join(dir, "automation"), filepath.Join(dir, "config.json") }
	keyringUnavailable = true
	t.Cleanup(func() { credentialPaths = oldPaths; keyringUnavailable = oldUnavailable })
	return dir
}
func TestCredentialFallbackRoundTrip(t *testing.T) {
	for _, name := range []string{keyringAccountKeyUser, keyringTokenUser} {
		t.Run(name, func(t *testing.T) {
			isolatedCredentials(t)
			inKeyring, err := saveCredential(name, "private-fixture")
			if err != nil || inKeyring {
				t.Fatalf("save: %v %v", inKeyring, err)
			}
			value, inKeyring, err := getCredential(name)
			if err != nil || value != "private-fixture" || inKeyring {
				t.Fatalf("read: %v %v", inKeyring, err)
			}
			if err = credentialStore().Delete(name); err != nil {
				t.Fatal(err)
			}
			_, _, err = getCredential(name)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("delete: %v", err)
			}
		})
	}
}
func TestCredentialFilePermissions(t *testing.T) {
	dir := isolatedCredentials(t)
	if _, err := SaveAccountKey("fixture"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "automation/secrets"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("permissions %v", info.Mode())
		}
	}
}
func TestEmptyCredentialRetrieval(t *testing.T) {
	isolatedCredentials(t)
	if _, _, err := GetStoredAccountKey(); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
