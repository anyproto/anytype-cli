//go:build !darwin && !windows

package localstate

import "os"

// POSIX ACL effective group/named-user permissions are bounded by the mode's
// group mask, already rejected by CheckPrivateFile when nonzero.
func checkPrivateACL(_ *os.File) error { return nil }
