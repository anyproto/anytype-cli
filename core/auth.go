package core

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func determineNetworkMode(networkConfigPath string) pb.RpcAccountNetworkMode {
	if networkConfigPath != "" {
		return pb.RpcAccount_CustomConfig
	}
	return pb.RpcAccount_DefaultConfig
}

func newAccountSelectRequest(accountId, apiAddr, rootPath string, networkMode pb.RpcAccountNetworkMode, networkConfigPath string) *pb.RpcAccountSelectRequest {
	return &pb.RpcAccountSelectRequest{
		Id:                          accountId,
		JsonApiListenAddr:           apiAddr,
		RootPath:                    rootPath,
		NetworkMode:                 networkMode,
		NetworkCustomConfigFilePath: networkConfigPath,
		// Headless deployments can run on networks where QUIC is unavailable
		// or unreliable. anytype-heart provides Yamux/TCP for this case.
		PreferYamuxTransport: true,
	}
}

// Authenticate performs the full authentication flow for a bot account using an account key.
// This includes wallet recovery, session creation, account recovery, account selection, and config persistence.
// If networkConfigPath is provided, connects to that custom network.
func Authenticate(accountKey, rootPath, apiAddr, networkConfigPath string) error {
	if rootPath == "" {
		rootPath = config.GetDataDir()
	}
	if apiAddr == "" {
		apiAddr = config.DefaultAPIAddress
	}

	networkMode := determineNetworkMode(networkConfigPath)

	var sessionToken string
	err := GRPCCallNoAuth(func(ctx context.Context, client service.ClientCommandsClient) error {
		resp, err := client.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
			Platform: runtime.GOOS,
			Version:  Version,
			Workdir:  config.GetWorkDir(),
		})
		if err != nil {
			return fmt.Errorf("failed to set initial parameters: %w", err)
		}
		if resp.Error.Code != pb.RpcInitialSetParametersResponseError_NULL {
			return fmt.Errorf("failed to set initial parameters: %s", resp.Error.Description)
		}

		resp2, err := client.WalletRecover(ctx, &pb.RpcWalletRecoverRequest{
			AccountKey: accountKey,
			RootPath:   rootPath,
		})
		if err != nil {
			return fmt.Errorf("wallet recovery failed: %w", err)
		}
		if resp2.Error.Code != pb.RpcWalletRecoverResponseError_NULL {
			return fmt.Errorf("wallet recovery failed: %s", resp2.Error.Description)
		}

		resp3, err := client.WalletCreateSession(ctx, &pb.RpcWalletCreateSessionRequest{
			Auth: &pb.RpcWalletCreateSessionRequestAuthOfAccountKey{
				AccountKey: accountKey,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		if resp3.Error.Code != pb.RpcWalletCreateSessionResponseError_NULL {
			return fmt.Errorf("failed to create session: %s", resp3.Error.Description)
		}
		sessionToken = resp3.Token
		return nil
	})
	if err != nil {
		return err
	}

	savedToKeyring, err := SaveSessionToken(sessionToken)
	if err != nil {
		return fmt.Errorf("failed to save session token: %w", err)
	}
	if !savedToKeyring {
		output.Warning("System keyring unavailable (requires D-Bus on Linux, Keychain on macOS, Credential Manager on Windows)")
		output.Warning("Storing credentials in config file: %s (insecure)", config.GetConfigManager().GetFilePath())
	}

	er, err := ListenForEvents(sessionToken)
	if err != nil {
		return fmt.Errorf("failed to start event listener: %w", err)
	}

	err = GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		_, err := client.AccountRecover(ctx, &pb.RpcAccountRecoverRequest{})
		if err != nil {
			return fmt.Errorf("account recovery failed: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	accountId, err := WaitForAccountId(er)
	if err != nil {
		return fmt.Errorf("error waiting for account Id: %w", err)
	}
	output.Info("Account Id: %s", accountId)

	var techSpaceId string
	err = GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		resp, err := client.AccountSelect(ctx, newAccountSelectRequest(accountId, apiAddr, rootPath, networkMode, networkConfigPath))
		if err != nil {
			return fmt.Errorf("failed to select account: %w", err)
		}
		if resp.Account != nil && resp.Account.Info != nil {
			techSpaceId = resp.Account.Info.TechSpaceId
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err := config.SetAccountIdToConfig(accountId); err != nil {
		output.Warning("Failed to save account Id: %v", err)
	}
	if techSpaceId != "" {
		if err := config.SetTechSpaceIdToConfig(techSpaceId); err != nil {
			output.Warning("Failed to save tech space Id: %v", err)
		}
	}

	return nil
}

// ValidateAccountKey checks if the provided account key is valid.
func ValidateAccountKey(accountKey string) error {
	if accountKey == "" {
		return fmt.Errorf("account key cannot be empty")
	}

	words := strings.Fields(accountKey)
	if len(words) >= 12 {
		return fmt.Errorf("this appears to be a mnemonic phrase, not an account key - the CLI only supports bot accounts created via 'anytype auth create'")
	}

	decoded, err := base64.StdEncoding.DecodeString(accountKey)
	if err != nil {
		return fmt.Errorf("invalid account key format: must be valid base64")
	}

	if len(decoded) < 32 {
		return fmt.Errorf("invalid account key format: insufficient key material")
	}

	return nil
}

// Login handles user interaction for login by prompting for account key if not provided,
// validating it, performing authentication, and saving the key to keychain.
func Login(accountKey, rootPath, apiAddr, networkConfigPath string) error {
	if accountKey == "" {
		output.Print("Enter account key: ")
		reader := bufio.NewReader(os.Stdin)
		accountKey, _ = reader.ReadString('\n')
		accountKey = strings.TrimSpace(accountKey)
	}

	if err := ValidateAccountKey(accountKey); err != nil {
		return err
	}

	if err := Authenticate(accountKey, rootPath, apiAddr, networkConfigPath); err != nil {
		return err
	}

	if networkConfigPath != "" {
		if err := config.SetNetworkConfigPathToConfig(networkConfigPath); err != nil {
			output.Warning("Failed to save network config path: %v", err)
		}
	}

	savedToKeyring, err := SaveAccountKey(accountKey)
	if err != nil {
		output.Warning("Failed to save account key: %v", err)
	} else if savedToKeyring {
		output.Success("Account key saved to keychain.")
	} else {
		output.Success("Account key saved to config file.")
	}

	return nil
}

// Logout logs out the current user by deleting stored credentials, clearing the config,
// and attempting to stop the account and close the wallet session on the server.
func Logout() error {
	token, _, err := GetStoredSessionToken()
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("not logged in")
		}
		return fmt.Errorf("failed to get stored token: %w", err)
	}

	if err := DeleteStoredAccountKey(); err != nil {
		return fmt.Errorf("failed to delete stored account key: %w", err)
	}

	if err := DeleteStoredSessionToken(); err != nil {
		return fmt.Errorf("failed to delete stored token: %w", err)
	}

	configMgr := config.GetConfigManager()
	if err := configMgr.Delete(); err != nil {
		output.Warning("Failed to clear config: %v", err)
	}

	CloseEventReceiver()

	err = GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		resp, err := client.AccountStop(ctx, &pb.RpcAccountStopRequest{
			RemoveData: false,
		})
		if err != nil {
			return fmt.Errorf("failed to stop account: %w", err)
		}
		if resp.Error.Code != pb.RpcAccountStopResponseError_NULL {
			return fmt.Errorf("failed to stop account: %s", resp.Error.Description)
		}

		resp2, err := client.WalletCloseSession(ctx, &pb.RpcWalletCloseSessionRequest{Token: token})
		if err != nil {
			return fmt.Errorf("failed to close session: %w", err)
		}
		if resp2.Error.Code != pb.RpcWalletCloseSessionResponseError_NULL {
			return fmt.Errorf("failed to close session: %s", resp2.Error.Description)
		}

		return nil
	})

	if err != nil {
		output.Warning("Could not notify server: %v", err)
	}

	return nil
}

// CreateWallet creates a new wallet and account, establishes a session,
// saves credentials, and returns the account key, account ID, and whether credentials were saved to keyring.
// If networkConfigPath is provided, the account will be created on that custom network.
func CreateWallet(name, rootPath, apiAddr, networkConfigPath string) (string, string, bool, error) {
	if rootPath == "" {
		rootPath = config.GetDataDir()
	}
	if apiAddr == "" {
		apiAddr = config.DefaultAPIAddress
	}

	networkMode := determineNetworkMode(networkConfigPath)

	var sessionToken string
	var accountKey string

	err := GRPCCallNoAuth(func(ctx context.Context, client service.ClientCommandsClient) error {
		_, err := client.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
			Platform: runtime.GOOS,
			Version:  Version,
			Workdir:  config.GetWorkDir(),
		})
		if err != nil {
			return fmt.Errorf("failed to set initial parameters: %w", err)
		}

		createResp, err := client.WalletCreate(ctx, &pb.RpcWalletCreateRequest{
			RootPath: rootPath,
		})
		if err != nil {
			return fmt.Errorf("wallet creation failed: %w", err)
		}
		accountKey = createResp.AccountKey

		sessionResp, err := client.WalletCreateSession(ctx, &pb.RpcWalletCreateSessionRequest{
			Auth: &pb.RpcWalletCreateSessionRequestAuthOfAccountKey{
				AccountKey: accountKey,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		sessionToken = sessionResp.Token
		return nil
	})

	if err != nil {
		return "", "", false, err
	}

	savedToKeyring, err := SaveSessionToken(sessionToken)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to save session token: %w", err)
	}
	if !savedToKeyring {
		output.Warning("System keyring unavailable (requires D-Bus on Linux, Keychain on macOS, Credential Manager on Windows)")
		output.Warning("Storing credentials in config file: %s (insecure)", config.GetConfigManager().GetFilePath())
	}

	_, err = ListenForEvents(sessionToken)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to start event listener: %w", err)
	}

	var accountId string
	err = GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		resp, err := client.AccountCreate(ctx, &pb.RpcAccountCreateRequest{
			Name:                        name,
			StorePath:                   rootPath,
			JsonApiListenAddr:           apiAddr,
			NetworkMode:                 networkMode,
			NetworkCustomConfigFilePath: networkConfigPath,
		})
		if err != nil {
			return fmt.Errorf("account creation failed: %w", err)
		}
		accountId = resp.Account.Id
		return nil
	})
	if err != nil {
		return "", "", false, err
	}

	var techSpaceId string
	err = GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		resp, err := client.AccountSelect(ctx, &pb.RpcAccountSelectRequest{
			Id:                          accountId,
			JsonApiListenAddr:           apiAddr,
			RootPath:                    rootPath,
			NetworkMode:                 networkMode,
			NetworkCustomConfigFilePath: networkConfigPath,
		})
		if err != nil {
			return fmt.Errorf("failed to select account: %w", err)
		}
		if resp.Account != nil && resp.Account.Info != nil {
			techSpaceId = resp.Account.Info.TechSpaceId
		}
		return nil
	})
	if err != nil {
		return "", "", false, err
	}

	accountKeySavedToKeyring, err := SaveAccountKey(accountKey)
	if err != nil {
		output.Warning("Failed to save account key: %v", err)
	}

	if err := config.SetAccountIdToConfig(accountId); err != nil {
		output.Warning("Failed to save account Id: %v", err)
	}
	if techSpaceId != "" {
		if err := config.SetTechSpaceIdToConfig(techSpaceId); err != nil {
			output.Warning("Failed to save tech space Id: %v", err)
		}
	}
	if networkConfigPath != "" {
		if err := config.SetNetworkConfigPathToConfig(networkConfigPath); err != nil {
			output.Warning("Failed to save network config path: %v", err)
		}
	}

	return accountKey, accountId, accountKeySavedToKeyring, nil
}
