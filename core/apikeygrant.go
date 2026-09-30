package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrSpaceChoiceRequired   = errors.New("choose which spaces the key can access: --space <id|name> (repeatable) or --all-spaces")
	ErrPermChoiceRequired    = errors.New("choose the key's permission: --read-only or --read-write")
	ErrConflictingSpaceFlags = errors.New("--space and --all-spaces cannot be combined")
	ErrConflictingPermFlags  = errors.New("--read-only and --read-write cannot be combined")
)

// GrantFlags is the user's access choice for an API key. Both choices are
// required: the CLI never picks spaces or a permission on the user's behalf.
type GrantFlags struct {
	Spaces    []string
	AllSpaces bool
	ReadOnly  bool
	ReadWrite bool
}

// ResolvedSpace is a space argument resolved to a full space Id.
type ResolvedSpace struct {
	Id     string
	Name   string
	IsTech bool
}

// ValidateGrantFlags checks that exactly one space choice and exactly one
// permission choice were made.
func ValidateGrantFlags(flags GrantFlags) error {
	hasSpaces := len(flags.Spaces) > 0
	if hasSpaces && flags.AllSpaces {
		return ErrConflictingSpaceFlags
	}
	if !hasSpaces && !flags.AllSpaces {
		return ErrSpaceChoiceRequired
	}
	if flags.ReadOnly && flags.ReadWrite {
		return ErrConflictingPermFlags
	}
	if !flags.ReadOnly && !flags.ReadWrite {
		return ErrPermChoiceRequired
	}
	return nil
}

// ResolveSpaces resolves each argument to a full space Id. An exact Id wins over
// a name; a name must match exactly one space. The tech space is only reachable
// by its explicit Id. Duplicates are dropped, keeping first-appearance order.
func ResolveSpaces(args []string, spaces []SpaceListItem, techSpaceId string) ([]ResolvedSpace, error) {
	var resolved []ResolvedSpace
	seen := make(map[string]struct{})

	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			return nil, errors.New("empty space argument")
		}

		space, err := resolveSpace(arg, spaces, techSpaceId)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[space.Id]; ok {
			continue
		}
		seen[space.Id] = struct{}{}
		resolved = append(resolved, space)
	}

	return resolved, nil
}

func resolveSpace(arg string, spaces []SpaceListItem, techSpaceId string) (ResolvedSpace, error) {
	if techSpaceId != "" && arg == techSpaceId {
		return ResolvedSpace{Id: techSpaceId, IsTech: true}, nil
	}
	for _, space := range spaces {
		if space.SpaceId == arg {
			return ResolvedSpace{Id: space.SpaceId, Name: space.Name}, nil
		}
	}

	var matches []SpaceListItem
	for _, space := range spaces {
		if space.Name == arg {
			matches = append(matches, space)
		}
	}
	switch len(matches) {
	case 0:
		return ResolvedSpace{}, fmt.Errorf("space %q not found; run 'anytype space list' to see your spaces", arg)
	case 1:
		return ResolvedSpace{Id: matches[0].SpaceId, Name: matches[0].Name}, nil
	default:
		candidates := make([]string, len(matches))
		for i, m := range matches {
			candidates[i] = fmt.Sprintf("%s (%s)", m.Name, m.SpaceId)
		}
		return ResolvedSpace{}, fmt.Errorf("space name %q is ambiguous, pass the space Id instead: %s", arg, strings.Join(candidates, ", "))
	}
}

// BuildGrant turns validated flags and resolved spaces into the grant sent to
// the server. It never returns a nil or empty grant: a key without a grant would
// be unrestricted.
func BuildGrant(flags GrantFlags, resolved []ResolvedSpace) (*model.AccountAuthAppGrant, error) {
	if err := ValidateGrantFlags(flags); err != nil {
		return nil, err
	}

	perm := model.AccountAuthAppGrant_Read
	if flags.ReadWrite {
		perm = model.AccountAuthAppGrant_ReadWrite
	}

	if flags.AllSpaces {
		return &model.AccountAuthAppGrant{AllSpaces: true, Perm: perm}, nil
	}
	if len(resolved) == 0 {
		return nil, ErrSpaceChoiceRequired
	}

	spaceIds := make([]string, len(resolved))
	for i, space := range resolved {
		spaceIds[i] = space.Id
	}
	return &model.AccountAuthAppGrant{SpaceIds: spaceIds, Perm: perm}, nil
}

// ValidateAPIKeyName mirrors the server's rule: the name is required and at most
// domain.MaxIntegrationNameLen bytes. It is never truncated, because on API v2
// the name decides which objects the key may delete.
func ValidateAPIKeyName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("API key name is required")
	}
	if len(name) > domain.MaxIntegrationNameLen {
		return fmt.Errorf("API key name is %d bytes, the maximum is %d", len(name), domain.MaxIntegrationNameLen)
	}
	return nil
}

// GrantWorksOnV1 reports whether the JSON API v1 accepts a key with this grant.
// v1 cannot enforce space grants, so it only admits keys that grant no less
// than an unrestricted key: no grant, or all spaces with read-write.
func GrantWorksOnV1(grant *model.AccountAuthAppGrant) bool {
	return grant == nil || (grant.AllSpaces && grant.Perm == model.AccountAuthAppGrant_ReadWrite)
}

// DescribeGrant renders a grant for people, naming spaces where resolved names
// are known.
func DescribeGrant(grant *model.AccountAuthAppGrant, resolved []ResolvedSpace) string {
	if grant == nil {
		return "unrestricted"
	}
	perm := "read-only"
	if grant.Perm == model.AccountAuthAppGrant_ReadWrite {
		perm = "read-write"
	}
	if grant.AllSpaces {
		return perm + ", all spaces"
	}

	byId := make(map[string]ResolvedSpace, len(resolved))
	for _, space := range resolved {
		byId[space.Id] = space
	}
	names := make([]string, len(grant.SpaceIds))
	for i, id := range grant.SpaceIds {
		space, ok := byId[id]
		switch {
		case ok && space.IsTech:
			names[i] = fmt.Sprintf("tech space (%s)", id)
		case ok && space.Name != "":
			names[i] = fmt.Sprintf("%s (%s)", space.Name, id)
		default:
			names[i] = id
		}
	}
	noun := "spaces"
	if len(names) == 1 {
		noun = "space"
	}
	return fmt.Sprintf("%s, %d %s: %s", perm, len(names), noun, strings.Join(names, ", "))
}
