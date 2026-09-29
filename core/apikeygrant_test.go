package core

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testTechSpaceId = "bafyreitech.tech"

var testSpaces = []SpaceListItem{
	{SpaceId: "bafyreia.one", Name: "Personal"},
	{SpaceId: "bafyreib.two", Name: "Team"},
	{SpaceId: "bafyreic.three", Name: "Team"},
	{SpaceId: "bafyreid.four", Name: "Work"},
}

func TestValidateGrantFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   GrantFlags
		wantErr error
	}{
		{"spaces and read-only", GrantFlags{Spaces: []string{"Work"}, ReadOnly: true}, nil},
		{"all spaces and read-write", GrantFlags{AllSpaces: true, ReadWrite: true}, nil},
		{"no space choice", GrantFlags{ReadOnly: true}, ErrSpaceChoiceRequired},
		{"no permission choice", GrantFlags{AllSpaces: true}, ErrPermChoiceRequired},
		{"nothing chosen reports spaces first", GrantFlags{}, ErrSpaceChoiceRequired},
		{"spaces and all spaces together", GrantFlags{Spaces: []string{"Work"}, AllSpaces: true, ReadOnly: true}, ErrConflictingSpaceFlags},
		{"read-only and read-write together", GrantFlags{AllSpaces: true, ReadOnly: true, ReadWrite: true}, ErrConflictingPermFlags},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGrantFlags(tt.flags)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateGrantFlags() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveSpaces(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		want        []ResolvedSpace
		wantErrText string
	}{
		{
			name: "by exact id",
			args: []string{"bafyreid.four"},
			want: []ResolvedSpace{{Id: "bafyreid.four", Name: "Work"}},
		},
		{
			name: "by unique name",
			args: []string{"Personal"},
			want: []ResolvedSpace{{Id: "bafyreia.one", Name: "Personal"}},
		},
		{
			name: "id and name of the same space are deduplicated",
			args: []string{"Work", "bafyreid.four", "Work"},
			want: []ResolvedSpace{{Id: "bafyreid.four", Name: "Work"}},
		},
		{
			name: "order of first appearance is kept",
			args: []string{"Work", "Personal"},
			want: []ResolvedSpace{{Id: "bafyreid.four", Name: "Work"}, {Id: "bafyreia.one", Name: "Personal"}},
		},
		{
			name: "tech space by explicit id",
			args: []string{testTechSpaceId},
			want: []ResolvedSpace{{Id: testTechSpaceId, IsTech: true}},
		},
		{
			name:        "ambiguous name lists candidates",
			args:        []string{"Team"},
			wantErrText: "bafyreic.three",
		},
		{
			name:        "unknown name or id",
			args:        []string{"Nope"},
			wantErrText: `space "Nope" not found`,
		},
		{
			name:        "name match is exact, not case-insensitive",
			args:        []string{"work"},
			wantErrText: `space "work" not found`,
		},
		{
			name:        "empty argument",
			args:        []string{""},
			wantErrText: "empty space",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveSpaces(tt.args, testSpaces, testTechSpaceId)
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("ResolveSpaces() error = %v, want it to contain %q", err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveSpaces() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResolveSpaces() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuildGrant(t *testing.T) {
	resolved := []ResolvedSpace{{Id: "bafyreia.one", Name: "Personal"}, {Id: "bafyreid.four", Name: "Work"}}

	tests := []struct {
		name     string
		flags    GrantFlags
		resolved []ResolvedSpace
		want     *model.AccountAuthAppGrant
	}{
		{
			name:     "listed spaces, read-only",
			flags:    GrantFlags{Spaces: []string{"Personal", "Work"}, ReadOnly: true},
			resolved: resolved,
			want:     &model.AccountAuthAppGrant{SpaceIds: []string{"bafyreia.one", "bafyreid.four"}, Perm: model.AccountAuthAppGrant_Read},
		},
		{
			name:     "listed spaces, read-write",
			flags:    GrantFlags{Spaces: []string{"Personal", "Work"}, ReadWrite: true},
			resolved: resolved,
			want:     &model.AccountAuthAppGrant{SpaceIds: []string{"bafyreia.one", "bafyreid.four"}, Perm: model.AccountAuthAppGrant_ReadWrite},
		},
		{
			name:  "all spaces, read-write",
			flags: GrantFlags{AllSpaces: true, ReadWrite: true},
			want:  &model.AccountAuthAppGrant{AllSpaces: true, Perm: model.AccountAuthAppGrant_ReadWrite},
		},
		{
			name:  "all spaces, read-only",
			flags: GrantFlags{AllSpaces: true, ReadOnly: true},
			want:  &model.AccountAuthAppGrant{AllSpaces: true, Perm: model.AccountAuthAppGrant_Read},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildGrant(tt.flags, tt.resolved)
			if err != nil {
				t.Fatalf("BuildGrant() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildGrant() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuildGrantNeverReturnsNilOrEmpty(t *testing.T) {
	tests := []struct {
		name     string
		flags    GrantFlags
		resolved []ResolvedSpace
	}{
		{"no choices", GrantFlags{}, nil},
		{"space flag but nothing resolved", GrantFlags{Spaces: []string{"x"}, ReadOnly: true}, nil},
		{"no permission", GrantFlags{AllSpaces: true}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildGrant(tt.flags, tt.resolved)
			if err == nil {
				t.Fatalf("BuildGrant() = %+v, want an error", got)
			}
			if got != nil {
				t.Errorf("BuildGrant() returned a grant alongside error: %+v", got)
			}
		})
	}
}

func TestValidateAPIKeyName(t *testing.T) {
	tests := []struct {
		name    string
		keyName string
		wantErr bool
	}{
		{"normal name", "my-integration", false},
		{"exactly 128 bytes", strings.Repeat("a", 128), false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"129 bytes", strings.Repeat("a", 129), true},
		{"multibyte over the byte limit", strings.Repeat("é", 65), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAPIKeyName(tt.keyName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKeyName(%q) error = %v, wantErr %v", tt.keyName, err, tt.wantErr)
			}
		})
	}
}

func TestGrantWorksOnV1(t *testing.T) {
	tests := []struct {
		name  string
		grant *model.AccountAuthAppGrant
		want  bool
	}{
		{"all spaces read-write", &model.AccountAuthAppGrant{AllSpaces: true, Perm: model.AccountAuthAppGrant_ReadWrite}, true},
		{"all spaces read-only", &model.AccountAuthAppGrant{AllSpaces: true, Perm: model.AccountAuthAppGrant_Read}, false},
		{"listed spaces read-write", &model.AccountAuthAppGrant{SpaceIds: []string{"a"}, Perm: model.AccountAuthAppGrant_ReadWrite}, false},
		{"listed spaces read-only", &model.AccountAuthAppGrant{SpaceIds: []string{"a"}}, false},
		{"no grant", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GrantWorksOnV1(tt.grant); got != tt.want {
				t.Errorf("GrantWorksOnV1() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDescribeGrant(t *testing.T) {
	tests := []struct {
		name     string
		grant    *model.AccountAuthAppGrant
		resolved []ResolvedSpace
		want     string
	}{
		{"all spaces read-write", &model.AccountAuthAppGrant{AllSpaces: true, Perm: model.AccountAuthAppGrant_ReadWrite}, nil, "read-write, all spaces"},
		{"all spaces read-only", &model.AccountAuthAppGrant{AllSpaces: true}, nil, "read-only, all spaces"},
		{
			"named spaces",
			&model.AccountAuthAppGrant{SpaceIds: []string{"bafyreia.one", "bafyreitech.tech"}},
			[]ResolvedSpace{{Id: "bafyreia.one", Name: "Personal"}, {Id: "bafyreitech.tech", IsTech: true}},
			"read-only, 2 spaces: Personal (bafyreia.one), tech space (bafyreitech.tech)",
		},
		{"spaces without names", &model.AccountAuthAppGrant{SpaceIds: []string{"bafyreia.one"}, Perm: model.AccountAuthAppGrant_ReadWrite}, nil, "read-write, 1 space: bafyreia.one"},
		{"no grant", nil, nil, "unrestricted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DescribeGrant(tt.grant, tt.resolved); got != tt.want {
				t.Errorf("DescribeGrant() = %q, want %q", got, tt.want)
			}
		})
	}
}
