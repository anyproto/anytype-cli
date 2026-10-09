package automation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/anyproto/anytype-cli/core/credentials"
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-cli/core/runtimeinfo"
)

func TestUnreadableSavedKeyBlocksAccountProvisioning(t *testing.T) {
	for _, command := range []string{"auth create", "auth login"} {
		e, rpc, _ := fixture(t, false)
		store := NewSecrets(e.StateDir, e.ConfigPath)
		store.Backend = ""
		store.GetKeyring = func(string, string) (string, error) { return "", os.ErrPermission }
		writes := 0
		store.SetKeyring = func(string, string, string) error { writes++; return nil }
		e.Secrets = store
		r := createRequest()
		if command == "auth login" {
			r = Request{Command: command, OperationId: "login-denied", ExpectedAccount: "account-a", Secret: "account-private-fixture"}
		}
		assertCode(t, run(e, r), "credential_storage_failed")
		if rpc.initialCalls+rpc.walletCalls+rpc.recoverCalls+rpc.sessionCalls+rpc.createCalls+rpc.selectCalls+writes != 0 {
			t.Fatal("unreadable credential treated as absence")
		}
	}
}

func TestPrivateBackendCreatePreservesLegacyKeyringAndReplays(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	t.Setenv(credentials.BackendEnv, "private_file")
	store := NewSecrets(e.StateDir, e.ConfigPath)
	legacyKey := "wrong"
	keyringCalls := 0
	store.GetKeyring = func(string, string) (string, error) { keyringCalls++; return "", os.ErrPermission }
	store.SetKeyring = func(_, name, value string) error {
		keyringCalls++
		if name == "account-key" {
			legacyKey = value
		}
		return nil
	}
	e.Secrets = store
	// The fake server's precondition uses this mirror; verify the real store at
	// the account RPC boundary rather than weakening the existing assertion.
	rpc.secrets["operation-account-create-0001"] = "account-private-fixture"
	rpc.afterCreate = func() {
		if key, _, err := store.Get("operation-account-create-0001"); err != nil || key != "account-private-fixture" {
			t.Fatal("recovery credential not durable", err)
		}
	}
	r := createRequest()
	out := run(e, r)
	if out.ExitCode() != 0 || out.Result.(map[string]any)["account"].(Account).CredentialStorage != "private_file" {
		t.Fatal(out)
	}
	t.Setenv(credentials.BackendEnv, "")
	restarted := NewSecrets(e.StateDir, e.ConfigPath)
	restarted.GetKeyring, restarted.SetKeyring = store.GetKeyring, store.SetKeyring
	e.Secrets = restarted
	if out = run(e, r); out.ExitCode() != 0 || rpc.createCalls != 1 || keyringCalls != 0 || legacyKey != "wrong" {
		t.Fatal(out, rpc.createCalls, keyringCalls, "legacy key changed", legacyKey != "wrong")
	}
}

func TestFullExpectedIdentityGuardsAllSavedKeysBeforeMutation(t *testing.T) {
	for _, source := range []string{"credential_store", "legacy_config", "retry"} {
		t.Run(source, func(t *testing.T) {
			e, rpc, _ := fixture(t, false)
			r := Request{Command: "auth login", OperationId: "login-saved-identity", ExpectedAccount: "account-a", Secret: "account-private-fixture"}
			if source == "legacy_config" {
				if err := localstate.WriteJSON(e.ConfigPath, map[string]string{"accountKey": "wrong"}); err != nil {
					t.Fatal(err)
				}
			} else {
				rpc.secrets["account-key"] = "wrong"
			}
			if source == "retry" {
				op, _, err := e.operation(r)
				if err != nil {
					t.Fatal(err)
				}
				op.AccountId = "account-a"
				if err = e.saveOp(op, "prepared"); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(e.DataPath, "other-account-data"), 0700); err != nil {
				t.Fatal(err)
			}
			assertCode(t, run(e, r), "account_mismatch")
			if rpc.initialCalls+rpc.walletCalls+rpc.recoverCalls+rpc.sessionCalls+rpc.selectCalls != 0 || rpc.secrets["operation-account-"+r.OperationId] != "" {
				t.Fatal("different durable identity caused side effects")
			}
			if source != "legacy_config" && rpc.secrets["account-key"] != "wrong" {
				t.Fatal("saved key replaced")
			}
		})
	}
}

func TestSameAccountLoginGuardsLiveNetworkBeforeSaving(t *testing.T) {
	for _, savedConfig := range []bool{false, true} {
		e, rpc, _ := fixture(t, true)
		path := filepath.Join(t.TempDir(), "network.yml")
		if err := os.WriteFile(path, []byte("networkId: other-network\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if savedConfig {
			if err := localstate.WriteJSON(e.ConfigPath, map[string]string{"accountId": "account-a", "networkId": rpc.snapshot.NetworkId, "networkConfigPath": path}); err != nil {
				t.Fatal(err)
			}
		}
		r := Request{Command: "auth login", OperationId: "login-network", ExpectedAccount: "account-a", Secret: "account-private-fixture", NetworkConfig: path}
		assertCode(t, run(e, r), "settings_conflict")
		if !savedConfig {
			if _, err := os.Stat(e.ConfigPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("config written", err)
			}
		}
		if rpc.initialCalls+rpc.recoverCalls+rpc.sessionCalls+rpc.selectCalls != 0 || rpc.secrets["account-key"] != "" {
			t.Fatal("settings conflict mutated credentials/account")
		}
	}
}

func TestSameAccountLoginPreservesLiveListener(t *testing.T) {
	for _, requested := range []string{"", "127.0.0.1:4700"} {
		e, rpc, _ := fixture(t, true)
		rpc.snapshot.APIAddress, rpc.snapshot.APIBound = "127.0.0.1:4600", true
		r := Request{Command: "auth login", OperationId: "login-listener", ExpectedAccount: "account-a", Secret: "account-private-fixture", ListenAddress: requested}
		out := run(e, r)
		if requested != "" {
			assertCode(t, out, "settings_conflict")
			continue
		}
		cfg, err := e.config()
		if out.ExitCode() != 0 || err != nil || cfg.ApiListenAddr != "127.0.0.1:4600" || rpc.selectCalls != 0 {
			t.Fatal(out, cfg, err)
		}
		if out = run(e, r); out.ExitCode() != 0 {
			t.Fatal(out)
		}
	}
}

func TestCompleteCreateReceiptRecoversAfterServiceRestart(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	r := createRequest()
	if out := run(e, r); out.ExitCode() != 0 {
		t.Fatal(out)
	}
	*rpc.snapshot = runtimeinfo.Snapshot{SchemaVersion: 1}
	r.IncludeSecret = true
	out := run(e, r)
	if out.ExitCode() != 0 || rpc.createCalls != 1 || rpc.walletCalls != 1 || rpc.selectCalls != 1 || out.Result.(map[string]any)["account_key"] != "account-private-fixture" {
		t.Fatal("completed creation did not deterministically select", out, rpc.createCalls, rpc.selectCalls)
	}
	var op operation
	if err := localstate.ReadJSON(e.opPath(r.OperationId), &op); err != nil || op.Stage != "complete" {
		t.Fatal(op, err)
	}
	if out = run(e, r); out.ExitCode() != 0 || rpc.selectCalls != 1 || rpc.createCalls != 1 {
		t.Fatal(out)
	}
}

func TestWildcardListenerAliasPreservesAccountSettingsOnReplay(t *testing.T) {
	for _, login := range []bool{false, true} {
		e, rpc, _ := fixture(t, login)
		r := createRequest()
		if login {
			r = Request{Command: "auth login", OperationId: "login-wildcard", ExpectedAccount: "account-a", Secret: "account-private-fixture"}
			rpc.snapshot.APIAddress, rpc.snapshot.APIBound = "[::]:4500", true
		} else {
			rpc.afterCreate = func() { rpc.snapshot.APIAddress = "[::]:4500" }
		}
		r.ListenAddress = "0.0.0.0:4500"
		if out := run(e, r); out.ExitCode() != 0 {
			t.Fatal(out)
		}
		if out := run(e, r); out.ExitCode() != 0 {
			t.Fatal(out)
		}
		cfg, err := e.config()
		if err != nil || cfg.ApiListenAddr != "0.0.0.0:4500" {
			t.Fatal("listener spelling changed", cfg, err)
		}
		out := run(e, Request{Command: "inspect"})
		if out.ExitCode() != 0 {
			t.Fatal(out)
		}
		result := out.Result.(map[string]any)
		if result["api"].(map[string]any)["origin"] != "http://127.0.0.1:4500" || result["mcp"].(map[string]any)["url"] != "http://127.0.0.1:4500/mcp/full" {
			t.Fatal("wildcard exposed unusable client origin", out)
		}
	}
}
