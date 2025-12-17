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
		Use:   "join <invite-link>",
		Short: "Join a space",
		Long:  "Join a space using an invite link (https://<host>/<cid>#<key>)",
		Args:  cmdutil.ExactArgs(1, "cannot join space: invite-link argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := args[0]
			var spaceId string

			if networkId == "" {
				if storedNetworkId, err := config.GetNetworkIdFromConfig(); err == nil && storedNetworkId != "" {
					networkId = storedNetworkId
				} else {
					networkId = config.AnytypeNetworkAddress
				}
			}

			if inviteCid == "" || inviteFileKey == "" {
				parsedCid, parsedKey, err := parseInviteLinkParts(input)
				if err != nil {
					return output.Error("invalid invite link: %w", err)
				}

				if inviteCid == "" {
					if parsedCid == "" {
						return output.Error("invalid invite link: missing Cid in path")
					}
					inviteCid = parsedCid
				}
				if inviteFileKey == "" {
					if parsedKey == "" {
						return output.Error("invalid invite link: missing key (should be after #)")
					}
					inviteFileKey = parsedKey
				}
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

func parseInviteLinkParts(input string) (string, string, error) {
	u, err := url.Parse(input)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return "", "", fmt.Errorf("unsupported scheme %q (expected http or https)", u.Scheme)
	}

	if u.Host == "" {
		return "", "", fmt.Errorf("invite link missing host")
	}

	var cid string
	if path := strings.Trim(u.Path, "/"); path != "" {
		parts := strings.Split(path, "/")
		cid = parts[len(parts)-1]
	}

	key := u.Fragment

	return cid, key, nil
}

func parseInviteLink(input string) (string, string, error) {
	// Convenience wrapper that enforces both cid and key presence.
	// The command path uses parseInviteLinkParts to allow partial override via flags.
	cid, key, err := parseInviteLinkParts(input)
	if err != nil {
		return "", "", err
	}
	if cid == "" {
		return "", "", fmt.Errorf("invite link missing Cid in path")
	}
	if key == "" {
		return "", "", fmt.Errorf("invite link missing key (should be after #)")
	}
	return cid, key, nil
}
