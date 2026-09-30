package create

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// stubServer replaces the server calls and records whether a key was created.
func stubServer(t *testing.T, spaces []core.SpaceListItem) *bool {
	t.Helper()
	created := false
	origList, origTech, origCreate := listSpaces, techSpaceId, createAPIKey
	listSpaces = func() ([]core.SpaceListItem, error) { return spaces, nil }
	techSpaceId = func() (string, error) { return "bafyreitech.tech", nil }
	createAPIKey = func(name string, grant *model.AccountAuthAppGrant) (*core.CreatedAPIKey, error) {
		created = true
		return &core.CreatedAPIKey{Key: "secret", App: &model.AccountAuthAppInfo{AppName: name, Scope: model.AccountAuth_JsonAPI, Grant: grant}}, nil
	}
	t.Cleanup(func() { listSpaces, techSpaceId, createAPIKey = origList, origTech, origCreate })
	return &created
}

func runCreate(args ...string) error {
	cmd := NewCreateCmd()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestCreateCommandFlags(t *testing.T) {
	cmd := NewCreateCmd()
	for _, name := range []string{"space", "all-spaces", "read-only", "read-write"} {
		if cmd.Flag(name) == nil {
			t.Errorf("flag --%s not found", name)
		}
	}
}

func TestCreateRequiresExplicitChoices(t *testing.T) {
	spaces := []core.SpaceListItem{{SpaceId: "bafyreia.one", Name: "Personal"}}

	tests := []struct {
		name    string
		args    []string
		wantErr error
		wantMsg string
	}{
		{"no space choice lists the spaces", []string{"my-app", "--read-only"}, core.ErrSpaceChoiceRequired, "Personal (bafyreia.one)"},
		{"no permission choice", []string{"my-app", "--all-spaces"}, core.ErrPermChoiceRequired, ""},
		{"no choices at all", []string{"my-app"}, core.ErrSpaceChoiceRequired, ""},
		{"conflicting space flags", []string{"my-app", "--all-spaces", "--space", "Personal", "--read-only"}, core.ErrConflictingSpaceFlags, ""},
		{"conflicting permission flags", []string{"my-app", "--all-spaces", "--read-only", "--read-write"}, core.ErrConflictingPermFlags, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := stubServer(t, spaces)

			err := runCreate(tt.args...)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantMsg)
			}
			if *created {
				t.Error("no key may be created without both choices")
			}
		})
	}
}

func TestCreateRejectsInvalidName(t *testing.T) {
	created := stubServer(t, nil)

	err := runCreate(strings.Repeat("a", 129), "--all-spaces", "--read-only")

	if err == nil {
		t.Fatal("expected an error for a name over 128 bytes")
	}
	if *created {
		t.Error("no key may be created with an invalid name")
	}
}

func TestCreateRejectsUnknownSpace(t *testing.T) {
	created := stubServer(t, []core.SpaceListItem{{SpaceId: "bafyreia.one", Name: "Personal"}})

	err := runCreate("my-app", "--space", "Nope", "--read-only")

	if err == nil || !strings.Contains(err.Error(), `"Nope" not found`) {
		t.Fatalf("error = %v, want unknown space error", err)
	}
	if *created {
		t.Error("no key may be created for an unknown space")
	}
}

func TestCreateSendsResolvedGrant(t *testing.T) {
	stubServer(t, []core.SpaceListItem{{SpaceId: "bafyreia.one", Name: "Personal"}})
	var sent *model.AccountAuthAppGrant
	createAPIKey = func(name string, grant *model.AccountAuthAppGrant) (*core.CreatedAPIKey, error) {
		sent = grant
		return &core.CreatedAPIKey{Key: "secret", App: &model.AccountAuthAppInfo{AppName: name, Grant: grant}}, nil
	}

	err := runCreate("my-app", "--space", "Personal", "--read-write")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sent == nil || len(sent.SpaceIds) != 1 || sent.SpaceIds[0] != "bafyreia.one" || sent.Perm != model.AccountAuthAppGrant_ReadWrite {
		t.Errorf("sent grant = %+v, want Personal read-write", sent)
	}
}
