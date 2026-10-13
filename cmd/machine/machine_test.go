package machine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/anyproto/anytype-cli/core/automation"
)

type recorder struct {
	calls   int
	request automation.Request
}

func (r *recorder) Run(_ context.Context, request automation.Request) automation.Envelope {
	r.calls++
	r.request = request
	return automation.Success(map[string]string{"ok": "yes"}, false)
}
func TestMachineParserPrivateChannels(t *testing.T) {
	r := new(recorder)
	var out bytes.Buffer
	code := Execute(context.Background(), []string{"auth", "login", "--json", "--key-stdin", "--expect-account", "none", "--operation-id", "login-0001"}, strings.NewReader("private-key\n"), &out, r)
	if code != 0 || r.calls != 1 || r.request.Secret != "private-key" {
		t.Fatal(code, r)
	}
	if strings.Contains(out.String(), "private-key") {
		t.Fatal("private stdin leaked")
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal("non-JSON stdout", err)
	}
}
func TestParseErrorsNeverEchoArguments(t *testing.T) {
	for _, args := range [][]string{{"auth", "login", "private-key", "--json"}, {"space", "join", "https://invite#private-key", "--json"}, {"--json", "--private-key=secret"}, {"unknown-private-key", "--json"}, {"auth", "create", "--json"}} {
		r := new(recorder)
		var out bytes.Buffer
		code := Execute(context.Background(), args, strings.NewReader(""), &out, r)
		if code != 2 || r.calls != 0 {
			t.Fatalf("parse %v: %d", args, code)
		}
		if strings.Contains(out.String(), "private-key") {
			t.Fatal("argument echoed")
		}
		var envelope automation.Envelope
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
	}
}
func TestMachineHelpIsSingleEnvelope(t *testing.T) {
	r := new(recorder)
	var out bytes.Buffer
	if code := Execute(context.Background(), []string{"auth", "apikey", "create", "--json", "--help"}, strings.NewReader(""), &out, r); code != 0 {
		t.Fatal(code)
	}
	var envelope automation.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err, out.String())
	}
	if r.calls != 0 || envelope.State != "ready" {
		t.Fatal(envelope)
	}
}
func TestSecretInputLimit(t *testing.T) {
	r := new(recorder)
	var out bytes.Buffer
	code := Execute(context.Background(), []string{"auth", "login", "--json", "--key-stdin"}, strings.NewReader(strings.Repeat("x", 65537)), &out, r)
	if code != 2 || r.calls != 0 {
		t.Fatal(code)
	}
}
func TestRequestedStopsAtDelimiter(t *testing.T) {
	if Requested([]string{"auth", "create", "--", "--json"}) {
		t.Fatal("name selected machine parser")
	}
	if !Requested([]string{"--json=true", "inspect"}) {
		t.Fatal("json not recognized")
	}
}
func TestSecretOutputRejectsPublicFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = os.Chmod(file.Name(), 0644); err != nil {
		t.Fatal(err)
	}
	r := new(recorder)
	code := Execute(context.Background(), []string{"auth", "create", "Agent", "--json", "--include-secret"}, strings.NewReader(""), file, r)
	if code != 2 || r.calls != 0 {
		t.Fatal("public secret output accepted")
	}
}
