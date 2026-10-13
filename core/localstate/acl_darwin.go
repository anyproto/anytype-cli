//go:build darwin && cgo

package localstate

/*
#include <errno.h>
#include <sys/acl.h>

static int private_acl(int fd) {
	acl_t acl = acl_get_fd_np(fd, ACL_TYPE_EXTENDED);
	// An open descriptor with no extended ACL reports ENOENT on macOS.
	if (acl == NULL) return errno == ENOENT ? 0 : -1;
	acl_entry_t entry;
	int result = -1;
	int id = ACL_FIRST_ENTRY;
	for (;;) {
		// Darwin returns -1/EINVAL at the end, unlike POSIX's zero.
		if (acl_get_entry(acl, id, &entry) != 0) {
			if (errno == EINVAL) result = 0;
			break;
		}
		acl_tag_t tag;
		// Deny entries cannot broaden access. Reject all granting entries,
		// rather than guessing membership or evaluating ACL ordering.
		if (acl_get_tag_type(entry, &tag) != 0 || tag != ACL_EXTENDED_DENY) break;
		id = ACL_NEXT_ENTRY;
	}
	acl_free(acl);
	return result;
}
*/
import "C"

import (
	"os"
	"runtime"
)

func checkPrivateACL(f *os.File) error {
	result := C.private_acl(C.int(f.Fd()))
	runtime.KeepAlive(f)
	if result != 0 {
		return os.ErrPermission
	}
	return nil
}
