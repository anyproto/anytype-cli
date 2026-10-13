package localstate

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsPrivateFileRejectsPublicACL(t *testing.T) {
	file := filepath.Join(t.TempDir(), "credential.json")
	if err := WriteJSON(file, map[string]string{"secret": "fixture"}); err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err := ReadPrivateJSON(file, &value); err != nil {
		t.Fatal("private ACL was rejected", err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(file, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if err = ReadPrivateJSON(file, &value); !os.IsPermission(err) {
		t.Fatal("public ACL accepted", err)
	}
}
