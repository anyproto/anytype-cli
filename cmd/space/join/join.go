package join

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewJoinCmd() *cobra.Command {
	var (
		networkId     string
		inviteCid     string
		inviteFileKey string
	)

	cmd := &cobra.Command{
		Use:     "join <invite-link>",
		Short:   "Join a space",
		Long:    "Join a space using an invite link (https://<host>/<cid>#<key> or anytype://invite/?cid=<cid>&key=<key>)",
		Example: `  anytype space join "anytype://invite/?cid=<cid>&key=<key>"`,
		Args:    cmdutil.ExactArgs(1, "cannot join space: invite-link argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := args[0]
			var spaceId string

			if networkId == "" {
				networkId = resolveNetworkId()
			}

			if inviteCid == "" || inviteFileKey == "" {
				parsedCid, parsedKey, err := parseInviteLink(input)
				if err != nil {
					return output.Error("Invalid invite link: %w", err)
				}
				if inviteCid == "" {
					inviteCid = parsedCid
				}
				if inviteFileKey == "" {
					inviteFileKey = parsedKey
				}
			}

			if inviteCid == "" {
				return output.Error("Invite link missing cid")
			}
			if inviteFileKey == "" {
				return output.Error("Invite link missing key")
			}

			info, err := core.ViewSpaceInvite(inviteCid, inviteFileKey)
			if err != nil {
				return output.Error("Failed to view invite: %w", err)
			}

			output.Info("Joining space '%s' created by %s...", info.SpaceName, info.CreatorName)
			spaceId = info.SpaceId

			if err := core.JoinSpace(networkId, spaceId, inviteCid, inviteFileKey); err != nil {
				return output.Error("Failed to join space: %w", err)
			}

			output.Success("Successfully sent join request to space '%s'", spaceId)
			return nil
		},
	}

	cmd.Flags().StringVar(&networkId, "network", "", "Network `id` to join")
	cmd.Flags().StringVar(&inviteCid, "invite-cid", "", "Invite `cid` (extracted from invite link if not provided)")
	cmd.Flags().StringVar(&inviteFileKey, "invite-key", "", "Invite file `key` (extracted from invite link if not provided)")

	return cmd
}

// resolveNetworkId returns the network ID using this fallback chain:
// cached config → YAML file → default Anytype network.
func resolveNetworkId() string {
	if cachedId, err := config.GetNetworkIdFromConfig(); err == nil && cachedId != "" {
		return cachedId
	}

	if yamlPath, _ := config.GetNetworkConfigPathFromConfig(); yamlPath != "" {
		if id, err := config.ReadNetworkIdFromYAML(yamlPath); err == nil && id != "" {
			_ = config.SetNetworkIdToConfig(id)
			return id
		}
	}

	return config.AnytypeNetworkAddress
}

// parseInviteLink parses web invites (https://<host>/<cid>#<key>) and
// app deep links (anytype://invite/?cid=<cid>&key=<key>).
func parseInviteLink(input string) (cid string, key string, err error) {
	u, err := url.Parse(input)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse URL: %w", err)
	}

	switch u.Scheme {
	case "https", "http":
		if u.Host == "" {
			return "", "", fmt.Errorf("invite link missing host")
		}
		path := strings.Trim(u.Path, "/")
		if path == "" {
			return "", "", fmt.Errorf("invite link missing cid in path")
		}
		parts := strings.Split(path, "/")
		cid = parts[len(parts)-1]
		if cid == "" {
			return "", "", fmt.Errorf("invite link missing cid in path")
		}
		key = u.Fragment
		if key == "" {
			return "", "", fmt.Errorf("invite link missing key (should be after #)")
		}
		return cid, key, nil

	case "anytype":
		if !strings.HasPrefix(u.Path, "/invite") && u.Host != "invite" {
			return "", "", fmt.Errorf("unsupported anytype:// path (expected anytype://invite/...)")
		}
		query := u.Query()
		cid = query.Get("cid")
		key = query.Get("key")
		if cid == "" {
			return "", "", fmt.Errorf("invite link missing cid parameter")
		}
		if key == "" {
			return "", "", fmt.Errorf("invite link missing key parameter (if pasting into a shell, quote the entire invite URL)")
		}
		return cid, key, nil

	default:
		return "", "", fmt.Errorf("unsupported scheme %q (expected http, https, or anytype)", u.Scheme)
	}
}
