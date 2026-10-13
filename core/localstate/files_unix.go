//go:build !windows

package localstate

import (
	"os"
	"path/filepath"
)

func createPrivateTemp(dir string) (*os.File, error) {
	f, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return nil, err
	}
	// Check inherited permissions before writing any private data. In
	// particular, chmod(0600) does not remove inherited macOS ACL grants.
	if err = CheckPrivateFile(f); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, err
	}
	return f, nil
}

func replaceFile(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(to))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// CheckPrivateFile checks the opened file's mode and platform ACL semantics.
func CheckPrivateFile(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return os.ErrPermission
	}
	return checkPrivateACL(f)
}
