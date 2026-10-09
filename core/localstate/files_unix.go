//go:build !windows

package localstate

import (
	"os"
	"path/filepath"
)

func createPrivateTemp(dir string) (*os.File, error) { return os.CreateTemp(dir, ".write-*") }

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

// CheckPrivateFile applies POSIX permission semantics to an opened regular file.
func CheckPrivateFile(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return os.ErrPermission
	}
	return nil
}
