package runtimeinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestRuntimeRecordSanitizedAndNonceChanges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "runtime.json")
	r, err := New(file, "cfg", "data")
	if err != nil {
		t.Fatal(err)
	}
	first := r.Nonce()
	r.Account(&model.Account{Id: "account", Info: &model.AccountInfo{TechSpaceId: "tech", ProfileObjectId: "profile", NetworkId: "network", MetaDataKey: "must-not-leak"}})
	r.API("127.0.0.1:1234", true)
	var s Snapshot
	if err = localstate.ReadJSON(file, &s); err != nil {
		t.Fatal(err)
	}
	if s.AccountId != "account" || !s.APIBound || s.Nonce != first {
		t.Fatal(s)
	}
	b, _ := os.ReadFile(file)
	if strings.Contains(string(b), "must-not-leak") {
		t.Fatal("sensitive metadata persisted")
	}
	r.Stopped()
	s = Snapshot{}
	if err = localstate.ReadJSON(file, &s); err != nil {
		t.Fatal(err)
	}
	if s.AccountId != "" || s.APIBound {
		t.Fatal("stop retained live identity")
	}
	r2, err := New(file, "cfg", "data")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Nonce() == first {
		t.Fatal("nonce reused")
	}
}
func TestWriteFailureInvalidatesNonce(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "runtime.json")
	r, err := New(file, "cfg", "data")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	r.Transition()
	if r.Nonce() != "" {
		t.Fatal("failed observation still advertised")
	}
}

func TestSameAccountBindingRecoveryAndInvalidation(t *testing.T) {
	for _, test := range []struct {
		name        string
		transition  func(*Recorder, string)
		account     string
		wantAddress string
		wantBound   bool
	}{
		{"wallet recovery same account", func(r *Recorder, _ string) { r.WalletTransition(); r.Transition() }, "account-a", "127.0.0.1:4500", true},
		{"direct selection same account", func(r *Recorder, _ string) { r.Transition() }, "account-a", "127.0.0.1:4500", true},
		{"different account", func(r *Recorder, _ string) { r.WalletTransition(); r.Transition() }, "account-b", "", false},
		{"different data root", func(r *Recorder, dir string) { r.WalletTransition(); r.DataPath(filepath.Join(dir, "other")) }, "account-a", "", false},
		{"stop invalidates candidate", func(r *Recorder, _ string) { r.WalletTransition(); r.Stopped(); r.Transition() }, "account-a", "", false},
		{"failed binding replaces candidate", func(r *Recorder, _ string) { r.WalletTransition(); r.API("127.0.0.1:4600", false) }, "account-a", "127.0.0.1:4600", false},
		{"fresh binding replaces candidate", func(r *Recorder, _ string) { r.WalletTransition(); r.API("127.0.0.1:4600", true) }, "account-b", "127.0.0.1:4600", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "runtime.json")
			recorder, err := New(file, "cfg", filepath.Join(dir, "data"))
			if err != nil {
				t.Fatal(err)
			}
			if recorder.AccountActivity() {
				t.Fatal("fresh server has account activity")
			}
			recorder.Account(&model.Account{Id: "account-a"})
			recorder.API("127.0.0.1:4500", true)
			test.transition(recorder, dir)
			if !recorder.AccountActivity() {
				t.Fatal("transition lost account activity")
			}
			var before Snapshot
			if err = localstate.ReadJSON(file, &before); err != nil {
				t.Fatal(err)
			}
			if !before.AccountTransition {
				t.Fatal("transition not recorded")
			}
			recorder.Account(&model.Account{Id: test.account})
			var after Snapshot
			if err = localstate.ReadJSON(file, &after); err != nil {
				t.Fatal(err)
			}
			if after.AccountId != test.account || after.APIAddress != test.wantAddress || after.APIBound != test.wantBound || after.AccountTransition {
				t.Fatal(after)
			}
		})
	}
}

func TestAPIListenerEquivalenceDoesNotConflateUnicastOrUnknownAddresses(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"0.0.0.0:4500", "[::]:4500", true},
		{"[::]:4500", "0.0.0.0:4500", true},
		{"0.0.0.0:4500", "[::]:4600", false},
		{"127.0.0.1:4500", "[::1]:4500", false},
		{"127.0.0.1:4500", "0.0.0.0:4500", false},
		{"", "", false},
		{"localhost:4500", "localhost:4500", false},
	} {
		if got := SameAPIAddress(tc.a, tc.b); got != tc.same {
			t.Fatal(tc, got)
		}
	}
}

func TestSelectionRequiresKnownIdentityAndRootBeforeRetainingBinding(t *testing.T) {
	for _, id := range []string{"account-a", "account-b", ""} {
		for _, changedRoot := range []bool{false, true} {
			dir := t.TempDir()
			file := filepath.Join(dir, "runtime.json")
			r, err := New(file, "config", filepath.Join(dir, "data"))
			if err != nil {
				t.Fatal(err)
			}
			r.Account(&model.Account{Id: "account-a"})
			r.API("127.0.0.1:4500", true)
			r.WalletTransition()
			if changedRoot {
				r.DataPath(filepath.Join(dir, "other-data"))
			}
			r.AccountSelection(id)
			// Even returning to the old identity cannot restore evidence after a
			// different/unknown selection was allowed to enter the native handler.
			r.Account(&model.Account{Id: "account-a"})
			var s Snapshot
			if err = localstate.ReadJSON(file, &s); err != nil {
				t.Fatal(err)
			}
			wantBound := id == "account-a" && !changedRoot
			if s.APIBound != wantBound {
				t.Fatal(id, changedRoot, s)
			}
		}
	}
}
