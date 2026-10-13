//go:build darwin && !cgo

package localstate

import (
	"errors"
	"os"
)

// The CLI already requires cgo for Heart. A standalone non-cgo consumer must
// not silently downgrade macOS ACL checks to mode bits.
func checkPrivateACL(_ *os.File) error {
	return errors.New("macOS private file checks require a cgo build")
}
