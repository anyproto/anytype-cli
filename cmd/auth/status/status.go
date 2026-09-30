package status

import (
	"context"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long:  "Display current authentication status, including account information, server status, and stored credentials.",
		RunE: func(cmd *cobra.Command, args []string) error {
			hasAccountKey := false
			accountKey := ""
			accountKeyInKeyring := false
			if ak, inKeyring, err := core.GetStoredAccountKey(); err == nil && ak != "" {
				hasAccountKey = true
				accountKey = ak
				accountKeyInKeyring = inKeyring
			}

			hasToken := false
			token := ""
			tokenInKeyring := false
			if t, inKeyring, err := core.GetStoredSessionToken(); err == nil {
				hasToken = true
				token = t
				tokenInKeyring = inKeyring
			}

			accountId, _ := config.GetAccountIdFromConfig()

			isServerRunning := false
			err := core.GRPCCallNoAuth(func(ctx context.Context, client service.ClientCommandsClient) error {
				_, err := client.AppGetVersion(ctx, &pb.RpcAppGetVersionRequest{})
				return err
			})
			isServerRunning = err == nil

			// If server is running and we have a token, we're logged in (server auto-logs in on restart using stored account key)
			isLoggedIn := isServerRunning && hasToken

			// Display status based on priority: server -> credentials -> login
			if !isServerRunning {
				output.Print("Server is not running. Start it with 'anytype service start' or 'anytype serve' (foreground mode).")
				if hasAccountKey || hasToken || accountId != "" {
					storageLocation := "config file"
					if accountKeyInKeyring || tokenInKeyring {
						storageLocation = "keychain"
					}
					output.Print("Credentials are stored in %s.", storageLocation)
				}
				return nil
			}

			if !hasAccountKey && !hasToken && accountId == "" {
				output.Print("Not authenticated. Run 'anytype auth login' to authenticate or 'anytype auth create' to create a new account.")
				return nil
			}

			output.Print("\033[1manytype\033[0m")

			if isLoggedIn && accountId != "" {
				storageLocation := "config file"
				if tokenInKeyring {
					storageLocation = "keychain"
				}
				output.Print("  ✓ Logged in to account \033[1m%s\033[0m (%s)", accountId, storageLocation)
			} else if hasToken || hasAccountKey {
				storageLocation := "config file"
				if accountKeyInKeyring || tokenInKeyring {
					storageLocation = "keychain"
				}
				output.Print("  ✗ Not logged in (credentials stored in %s)", storageLocation)
				if !isLoggedIn && hasToken {
					output.Print("    Note: Server is not running or session expired. Start it with 'anytype service start' or 'anytype serve' (foreground mode).")
				}
			} else {
				output.Print("  ✗ Not logged in")
			}

			output.Print("  - Active session: \033[1m%v\033[0m", isLoggedIn)
			if isLoggedIn {
				storedAddr, _ := config.GetApiListenAddrFromConfig()
				apiAddr := config.ResolveAPIListenAddr("", false, storedAddr)
				output.Print("  - JSON API: \033[1m%s\033[0m", config.APIURL(apiAddr))
			}

			if hasAccountKey {
				if len(accountKey) > 8 {
					output.Print("  - Account Key: \033[1m%s****\033[0m", accountKey[:8])
				} else {
					output.Print("  - Account Key: \033[1mstored\033[0m")
				}
			}

			if hasToken {
				if len(token) > 8 {
					output.Print("  - Session Token: \033[1m%s****\033[0m", token[:8])
				} else {
					output.Print("  - Session Token: \033[1mstored\033[0m")
				}
			}

			return nil
		},
	}

	return cmd
}
