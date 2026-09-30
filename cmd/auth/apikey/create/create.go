package create

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

// Server calls, replaceable in tests.
var (
	listSpaces   = core.ListSpaces
	techSpaceId  = config.GetTechSpaceIdFromConfig
	createAPIKey = core.CreateAPIKey
)

func NewCreateCmd() *cobra.Command {
	var flags core.GrantFlags

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new API key",
		Long: `Create a new API key for programmatic access to Anytype.

You must choose which spaces the key can access and whether it can write:
  --space <id|name> (repeatable) or --all-spaces
  --read-only or --read-write

Keys limited to specific spaces, or read-only keys, work with the JSON API v2 only.`,
		Example: `  anytype auth apikey create my-app --space "Personal" --read-only
  anytype auth apikey create my-app --all-spaces --read-write`,
		Args: cmdutil.ExactArgs(1, "cannot create API key: name argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if err := core.ValidateAPIKeyName(name); err != nil {
				return output.Error("Failed to create API key: %w", err)
			}
			if err := core.ValidateGrantFlags(flags); err != nil {
				if errors.Is(err, core.ErrSpaceChoiceRequired) {
					return output.Error("Failed to create API key: %w%s", err, spaceChoices())
				}
				return output.Error("Failed to create API key: %w", err)
			}

			var resolved []core.ResolvedSpace
			if len(flags.Spaces) > 0 {
				spaces, err := listSpaces()
				if err != nil {
					return output.Error("Failed to list spaces: %w", err)
				}
				techId, err := techSpaceId()
				if err != nil {
					return output.Error("Failed to read tech space Id: %w", err)
				}
				resolved, err = core.ResolveSpaces(flags.Spaces, spaces, techId)
				if err != nil {
					return output.Error("Failed to create API key: %w", err)
				}
				for _, space := range resolved {
					if space.IsTech {
						output.Warning("The key can access the tech space, which holds account internals; writing to it can break your account")
					}
				}
			}

			grant, err := core.BuildGrant(flags, resolved)
			if err != nil {
				return output.Error("Failed to create API key: %w", err)
			}

			created, err := createAPIKey(name, grant)
			if err != nil {
				return output.Error("Failed to create API key: %w", err)
			}

			output.Success("API key created successfully")
			output.Info("Name: %s", name)
			output.Info("Key: %s", created.Key)
			output.Info("Access: %s", core.DescribeGrant(created.App.Grant, resolved))
			if core.GrantWorksOnV1(created.App.Grant) {
				output.Info("Works with: JSON API v1 and v2")
			} else {
				output.Info("Works with: JSON API v2 only")
			}

			return nil
		},
	}

	cmd.Flags().StringArrayVar(&flags.Spaces, "space", nil, "Space the key can access, by Id or exact name (repeatable)")
	cmd.Flags().BoolVar(&flags.AllSpaces, "all-spaces", false, "Let the key access all spaces, including ones created later")
	cmd.Flags().BoolVar(&flags.ReadOnly, "read-only", false, "Let the key read but not change data")
	cmd.Flags().BoolVar(&flags.ReadWrite, "read-write", false, "Let the key read and change data")

	return cmd
}

// spaceChoices lists the user's spaces to help pick --space values. It is best
// effort: without a running server the error is returned without the list.
func spaceChoices() string {
	spaces, err := listSpaces()
	if err != nil || len(spaces) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nYour spaces:")
	for _, space := range spaces {
		fmt.Fprintf(&b, "\n  %s (%s)", space.Name, space.SpaceId)
	}
	return b.String()
}
