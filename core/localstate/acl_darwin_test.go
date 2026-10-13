//go:build darwin && cgo

package localstate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func addTestACL(t *testing.T, path, entry string) {
	t.Helper()
	if out, err := exec.Command("/bin/chmod", "+a", entry, path).CombinedOutput(); err != nil {
		t.Fatalf("add fixture ACL: %v: %s", err, out)
	}
}

func TestPrivateFileChecksMacOSACLsOnOpenedDescriptor(t *testing.T) {
	for _, kind := range []string{"none", "deny", "read_allow", "deny_then_allow"} {
		t.Run(kind, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "private")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			// Rename after opening to prove the ACL check uses the descriptor.
			path := file.Name() + ".moved"
			if err = os.Rename(file.Name(), path); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if out, err := exec.Command("/bin/chmod", "-N", path).CombinedOutput(); err != nil {
					t.Errorf("remove fixture ACL: %v: %s", err, out)
				}
			})
			if kind == "deny" || kind == "deny_then_allow" {
				addTestACL(t, path, "everyone deny delete")
			}
			if kind == "read_allow" || kind == "deny_then_allow" {
				addTestACL(t, path, "everyone allow read")
			}
			err = CheckPrivateFile(file)
			if wantDenied := kind == "read_allow" || kind == "deny_then_allow"; wantDenied != errors.Is(err, os.ErrPermission) || !wantDenied && err != nil {
				t.Fatal(kind, err)
			}
		})
	}
}

func TestWriteJSONRejectsInheritedGrantBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := WriteJSON(path, map[string]string{"value": "previous"}); err != nil {
		t.Fatal(err)
	}
	addTestACL(t, dir, "everyone allow read,search,file_inherit,directory_inherit")
	if err := WriteJSON(path, map[string]string{"secret": "must-not-be-written"}); !errors.Is(err, os.ErrPermission) {
		t.Fatal("inherited grant accepted", err)
	}
	var result map[string]string
	if err := ReadPrivateJSON(path, &result); err != nil || result["value"] != "previous" {
		t.Fatal("failed replacement changed original", result, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("failed write left temporary files", entries, err)
	}
}
