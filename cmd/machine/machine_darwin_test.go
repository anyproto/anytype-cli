//go:build darwin && cgo

package machine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyproto/anytype-cli/core/automation"
)

func TestSecretOutputRejectsInheritedReadACL(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("/bin/chmod", "+a", "everyone allow read,search,file_inherit,directory_inherit", dir).CombinedOutput(); err != nil {
		t.Fatalf("add fixture ACL: %v: %s", err, out)
	}
	file, err := os.OpenFile(filepath.Join(dir, "output.json"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	r := new(recorder)
	code := Execute(context.Background(), []string{"auth", "create", "Agent", "--json", "--include-secret"}, strings.NewReader(""), file, r)
	if code != 2 || r.calls != 0 {
		t.Fatal("shared ACL passed secret-output gate", code, r.calls)
	}
	if _, err = file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var out automation.Envelope
	if err = json.NewDecoder(file).Decode(&out); err != nil || out.Error == nil || out.Error.Code != "private_output_required" {
		t.Fatal("missing actionable private-output failure", out, err)
	}
}
