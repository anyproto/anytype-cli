package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/runtimeinfo"
	heartconfig "github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/pb"
	heartcore "github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func identityFromKey(key string) (string, error) {
	derived, err := heartcore.WalletDeriveFromAccountMasterNode(key)
	if err != nil {
		return "", problem("invalid_account_key", "The private input is not a valid bot account key.", 2)
	}
	return derived.Identity.GetPublic().Account(), nil
}

func validateAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	p, pErr := strconv.Atoi(port)
	if err != nil || pErr != nil || p < 1 || p > 65535 || net.ParseIP(host) == nil {
		return problem("invalid_address", "Use a numeric IP address and a fixed port from 1 to 65535.", 2)
	}
	return nil
}

func (e *Engine) authMutation(ctx context.Context, r Request, op *operation, retry bool) Envelope {
	fail := func(err error) Envelope {
		out := Failure(err)
		out.Result = map[string]any{"operation_id": op.Id, "account_id": op.AccountId, "stage": op.Stage}
		return out
	}
	if r.Command == "auth create" && r.ExpectedAccount != "none" {
		return fail(problem("expected_account_required", "Creation requires --expect-account none.", 2))
	}
	c, err := e.config()
	if err != nil {
		return fail(err)
	}
	s, err := e.live(ctx)
	if err != nil {
		return fail(err)
	}
	addr := config.ResolveAPIListenAddr("", false, c.ApiListenAddr)
	if c.ApiListenAddr == "" && s.AccountId != "" && s.APIBound && s.APIAddress != "" {
		addr = s.APIAddress
	}
	if r.ListenAddress != "" {
		if c.AccountId != "" && r.ListenAddress != addr {
			return fail(problem("settings_conflict", "Existing account API settings must be preserved.", 2))
		}
		addr = r.ListenAddress
	}
	if s.AccountId != "" && s.APIBound && !runtimeinfo.SameAPIAddress(s.APIAddress, addr) {
		return fail(problem("settings_conflict", "The observed live API listener must be preserved.", 2))
	}
	if err = validateAddress(addr); err != nil {
		return fail(err)
	}
	network := c.NetworkConfigPath
	networkDigest := ""
	selectedNetworkId := ""
	if network != "" {
		network, err = filepath.Abs(network)
		if err != nil {
			return fail(problem("invalid_network_config", "The saved network configuration path is invalid.", 2))
		}
	}
	if r.NetworkConfig != "" {
		requestedNetwork, pathErr := filepath.Abs(r.NetworkConfig)
		if pathErr != nil {
			return fail(problem("invalid_network_config", "The network configuration path is invalid.", 2))
		}
		if c.AccountId != "" && network != requestedNetwork {
			return fail(problem("settings_conflict", "Existing account network settings must be preserved.", 2))
		}
		network = requestedNetwork
	}
	if network != "" {
		networkId, readErr := config.ReadNetworkIdFromYAML(network)
		if readErr != nil || networkId == "" {
			return fail(problem("invalid_network_config", "The selected network configuration must be readable and contain a networkId.", 2))
		}
		selectedNetworkId = networkId
		data, readErr := os.ReadFile(network)
		if readErr != nil {
			return fail(problem("invalid_network_config", "The selected network configuration cannot be read.", 2))
		}
		digest := sha256.Sum256(data)
		networkDigest = hex.EncodeToString(digest[:])
	} else {
		// Read the embedded default (or Heart's explicit environment override)
		// into a fresh in-memory config; this does not initialize account storage.
		cfg := heartconfig.New()
		nodes, readErr := cfg.GetNodeConfWithError()
		if readErr != nil || nodes.NetworkId == "" {
			return fail(problem("invalid_network_config", "The default network configuration cannot be resolved.", 2))
		}
		selectedNetworkId = nodes.NetworkId
	}
	if c.NetworkId != "" && c.NetworkId != selectedNetworkId || s.AccountId != "" && s.NetworkId != selectedNetworkId {
		return fail(problem("settings_conflict", "The selected network must match the configured and observed live account network.", 2))
	}
	settings := addr + "|" + network + "|" + networkDigest
	if op.Settings != "" && op.Settings != settings {
		return fail(problem("settings_conflict", "Instance listener/network settings changed since this operation began.", 2))
	}
	op.Settings = settings
	// Installation arguments and account selection must agree about the listener.
	svc, err := e.Service.Status(ctx)
	if err != nil {
		return fail(err)
	}
	if svc.Installed && (!svc.BindingVerified || svc.ListenAddress != addr) {
		return fail(problem("service_binding_conflict", "The installed service has a different or unverified binding. Preserve its settings or repair it explicitly.", 2))
	}
	if svc.Running && svc.PID > 0 && svc.PID != s.PID {
		return fail(problem("service_instance_mismatch", "The service manager and native RPC endpoint identify different processes.", 2))
	}
	savedKey, _, keyErr := e.Secrets.Get("account-key")
	if keyErr != nil && !errors.Is(keyErr, os.ErrNotExist) {
		return fail(problem("credential_storage_failed", "Cannot read existing account credentials; restore access before provisioning.", 2))
	}
	if !retry {
		if err = guard(s, r.ExpectedAccount, r.Command == "auth login"); err != nil {
			return fail(err)
		}
		if r.ExpectedAccount == "none" {
			entries, dirErr := os.ReadDir(e.DataPath)
			if c.AccountId != "" || c.AccountKey != "" || savedKey != "" || dirErr != nil && !errors.Is(dirErr, os.ErrNotExist) || len(entries) > 0 {
				return fail(problem("existing_account_data", "This instance already has credentials or account data. Reuse or explicitly recover it; creation cannot replace it.", 2))
			}
		}
	}
	if c.AccountId != "" && c.AccountId != op.AccountId && c.AccountId != r.ExpectedAccount {
		return fail(problem("account_mismatch", "Configured account does not match this operation.", 2))
	}
	if retry && s.AccountId != "" && s.AccountId != op.AccountId {
		return fail(problem("account_mismatch", "The live account no longer belongs to this operation.", 2))
	}
	expected := r.ExpectedAccount
	if expected == "none" {
		expected = op.AccountId
	}
	for _, durableKey := range []string{savedKey, c.AccountKey} {
		if durableKey == "" {
			continue
		}
		id, deriveErr := e.deriveIdentity(durableKey)
		if deriveErr != nil || expected == "" || id != expected {
			return fail(problem("account_mismatch", "Saved account credentials do not match this operation. Recover the existing account explicitly.", 2))
		}
	}
	key, _, secretErr := e.Secrets.Get("operation-account-" + op.Id)
	if secretErr != nil && !errors.Is(secretErr, os.ErrNotExist) {
		return fail(problem("credential_storage_failed", "Cannot read the operation credential.", 2))
	}
	if r.Command == "auth login" {
		if r.ExpectedAccount == "" {
			return fail(problem("expected_account_required", "Login requires --expect-account with the full account ID.", 2))
		}
		if r.Secret != "" {
			id, err := e.deriveIdentity(r.Secret)
			if err != nil {
				return fail(err)
			}
			if r.ExpectedAccount != "none" && id != r.ExpectedAccount {
				return fail(problem("account_mismatch", "The supplied key belongs to a different account.", 2))
			}
			if key != "" && key != r.Secret {
				return fail(problem("operation_conflict", "The private input differs from the credential saved for this operation.", 2))
			}
			key = r.Secret
		}
		if key == "" {
			return fail(problem("private_input_required", "Provide the account key through --key-stdin on the first login attempt.", 2))
		}
		id, err := e.deriveIdentity(key)
		if err != nil {
			return fail(err)
		}
		if r.ExpectedAccount != "none" && id != r.ExpectedAccount {
			return fail(problem("account_mismatch", "Saved operation credential does not match the expected account.", 2))
		}
		if op.AccountId != "" && op.AccountId != id {
			return fail(problem("operation_conflict", "The supplied account differs from this operation’s durable identity.", 2))
		}
		op.AccountId = id
		if !retry {
			if err = e.saveOp(op, "prepared"); err != nil {
				return fail(err)
			}
		}
		if _, err = e.Secrets.Set("operation-account-"+op.Id, key); err != nil {
			return fail(problem("credential_storage_failed", "Cannot durably store the account credential.", 2))
		}
	} else if key == "" {
		if retry && op.Stage != "prepared" {
			return fail(uncertain())
		}
		if secretErr != nil && !errors.Is(secretErr, os.ErrNotExist) {
			return fail(problem("credential_storage_failed", "Cannot read the operation credential.", 2))
		}
		if err = e.initial(ctx); err != nil {
			return fail(err)
		}
		if err = e.saveOp(op, "wallet_sent"); err != nil {
			return fail(err)
		}
		response, rpcErr := e.Client.WalletCreate(ctx, &pb.RpcWalletCreateRequest{RootPath: e.DataPath})
		if rpcErr != nil {
			return fail(rpcError(rpcErr))
		}
		// Save recovery material immediately, before session/account creation.
		if response.GetAccountKey() != "" {
			key = response.AccountKey
			if _, err = e.Secrets.Set("operation-account-"+op.Id, key); err != nil {
				return fail(problem("credential_storage_failed", "Wallet outcome is uncertain because its recovery credential could not be saved. Do not create another wallet.", 2))
			}
		}
		if err = protoError("Create wallet", response.GetError().GetCode().String()); err != nil {
			return fail(err)
		}
		if key == "" {
			return fail(uncertain())
		}
		id, err := e.deriveIdentity(key)
		if err != nil {
			return fail(err)
		}
		op.AccountId = id
		if err = e.saveOp(op, "wallet_saved"); err != nil {
			return fail(err)
		}
	}
	id, err := e.deriveIdentity(key)
	if err != nil {
		return fail(err)
	}
	if op.AccountId != "" && op.AccountId != id {
		return fail(problem("account_mismatch", "The saved recovery credential does not match the operation identity.", 2))
	}
	if op.AccountId == "" {
		op.AccountId = id
		if err = e.saveOp(op, "wallet_saved"); err != nil {
			return fail(err)
		}
	}
	// A live account with the exact identity proves AccountCreate completed only
	// after profile readback confirms the requested name. Never repeat that RPC.
	if s.AccountId == op.AccountId && !s.AccountTransition {
		a, aErr := e.account(ctx, s)
		if aErr == nil {
			if r.Command == "auth create" && a.DisplayName != r.Name {
				return fail(problem("account_name_mismatch", "The created account exists but its observed name differs. Inspect it; automatic rename/replacement is disabled.", 2))
			}
			return e.finishAccount(r, op, key, a, s.TechSpaceId, s.NetworkId, addr, network, false)
		}
		if op.Stage == "complete" {
			return fail(aErr)
		}
	}
	if r.Command == "auth create" && op.Stage == "account_sent" {
		return fail(uncertain())
	}
	if err = e.initial(ctx); err != nil {
		return fail(err)
	}
	// Recover only this operation's durably stored identity. No new wallet is
	// minted on retry; WalletRecover is deterministic for the saved key.
	response, rpcErr := e.Client.WalletRecover(ctx, &pb.RpcWalletRecoverRequest{RootPath: e.DataPath, AccountKey: key})
	if rpcErr != nil {
		return fail(rpcError(rpcErr))
	}
	if err = protoError("Recover wallet", response.GetError().GetCode().String()); err != nil {
		return fail(err)
	}
	session, rpcErr := e.Client.WalletCreateSession(ctx, &pb.RpcWalletCreateSessionRequest{Auth: &pb.RpcWalletCreateSessionRequestAuthOfAccountKey{AccountKey: key}})
	if rpcErr != nil {
		return fail(rpcError(rpcErr))
	}
	if err = protoError("Create session", session.GetError().GetCode().String()); err != nil {
		return fail(err)
	}
	if session.AccountId != op.AccountId || session.Token == "" {
		return fail(problem("account_mismatch", "The server authenticated an unexpected identity.", 2))
	}
	if _, err = e.Secrets.Set("session-token", session.Token); err != nil {
		return fail(problem("credential_storage_failed", "The session could not be saved; resume this operation.", 2))
	}
	authCtx, _, err := e.authContext(ctx)
	if err != nil {
		return fail(err)
	}
	mode := pb.RpcAccount_DefaultConfig
	if network != "" {
		mode = pb.RpcAccount_CustomConfig
	}
	var account *model.Account
	if r.Command == "auth create" && op.Stage != "account_observed" && op.Stage != "select_sent" && op.Stage != "complete" {
		if err = e.saveOp(op, "account_sent"); err != nil {
			return fail(err)
		}
		result, rpcErr := e.Client.AccountCreate(authCtx, &pb.RpcAccountCreateRequest{Name: r.Name, StorePath: e.DataPath, JsonApiListenAddr: addr, NetworkMode: mode, NetworkCustomConfigFilePath: network})
		if rpcErr != nil {
			return fail(rpcError(rpcErr))
		}
		// AccountCreate can report an error with a partially created account. Keep
		// the recovery key and sent marker, and never interpret that as success.
		if err = protoError("Create account", result.GetError().GetCode().String()); err != nil {
			return fail(err)
		}
		account = result.Account
	} else {
		if err = e.saveOp(op, "select_sent"); err != nil {
			return fail(err)
		}
		result, rpcErr := e.Client.AccountSelect(authCtx, &pb.RpcAccountSelectRequest{Id: op.AccountId, RootPath: e.DataPath, JsonApiListenAddr: addr, NetworkMode: mode, NetworkCustomConfigFilePath: network})
		if rpcErr != nil {
			return fail(rpcError(rpcErr))
		}
		if err = protoError("Select account", result.GetError().GetCode().String()); err != nil {
			return fail(err)
		}
		account = result.Account
	}
	if account == nil || account.Id != op.AccountId || account.Info == nil {
		return fail(uncertain())
	}
	if err = e.saveOp(op, "account_observed"); err != nil {
		return fail(err)
	}
	fresh, err := e.live(ctx)
	if err != nil {
		return fail(err)
	}
	if fresh.AccountId != op.AccountId {
		return fail(problem("account_mismatch", "Account changed during provisioning.", 2))
	}
	if fresh.NetworkId != selectedNetworkId || fresh.APIBound && !runtimeinfo.SameAPIAddress(fresh.APIAddress, addr) {
		return fail(problem("settings_conflict", "The observed account network or API listener differs from the operation settings.", 2))
	}
	a, err := e.account(ctx, fresh)
	if err != nil {
		return fail(err)
	}
	if r.Command == "auth create" && a.DisplayName != r.Name {
		return fail(problem("account_name_mismatch", "The account exists, but its observed name does not match the requested name.", 2))
	}
	return e.finishAccount(r, op, key, a, account.Info.TechSpaceId, account.Info.NetworkId, addr, network, true)
}

func (e *Engine) deriveIdentity(key string) (string, error) {
	if e.IdentityFromKey != nil {
		return e.IdentityFromKey(key)
	}
	return identityFromKey(key)
}
func (e *Engine) initial(ctx context.Context) error {
	r, err := e.Client.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{Platform: runtime.GOOS, Version: core.GetVersion(), Workdir: e.WorkPath})
	if err != nil {
		return rpcError(err)
	}
	return protoError("Initialize client", r.GetError().GetCode().String())
}
func (e *Engine) finishAccount(r Request, op *operation, key string, a Account, tech, networkId, addr, network string, changed bool) Envelope {
	storage, err := e.Secrets.Set("account-key", key)
	if err != nil {
		return Failure(problem("credential_storage_failed", "Account exists; resume to save its credential.", 2))
	}
	if err = e.saveConfig(map[string]any{"accountId": a.Id, "techSpaceId": tech, "networkId": networkId, "networkConfigPath": network, "apiListenAddr": addr}); err != nil {
		return Failure(err)
	}
	if err = e.saveOp(op, "complete"); err != nil {
		return Failure(err)
	}
	a.CredentialStorage = storage
	result := map[string]any{"operation_id": op.Id, "account": a}
	if r.IncludeSecret {
		result["account_key"] = key
	}
	return Success(result, changed)
}
