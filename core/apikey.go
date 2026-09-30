package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ErrServerTooOld means the running server cannot store API key grants, so a
// key created on it would silently get unrestricted access.
var ErrServerTooOld = errors.New("the running Anytype service is older than this CLI and cannot restrict API keys; restart it with: anytype service restart")

// CreatedAPIKey is a newly created key: the secret and the key as the server
// stored it.
type CreatedAPIKey struct {
	Key string
	App *model.AccountAuthAppInfo
}

// CreateAPIKey creates a JSON API key limited to the given grant.
func CreateAPIKey(name string, grant *model.AccountAuthAppGrant) (*CreatedAPIKey, error) {
	var created *CreatedAPIKey
	err := GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		var err error
		created, err = createAPIKey(ctx, client, name, grant)
		return err
	})
	return created, err
}

func createAPIKey(ctx context.Context, client service.ClientCommandsClient, name string, grant *model.AccountAuthAppGrant) (*CreatedAPIKey, error) {
	if grant == nil {
		return nil, errors.New("an API key must be limited to a grant")
	}
	if err := ensureGrantSupport(ctx, client); err != nil {
		return nil, err
	}

	resp, err := client.AccountLocalLinkCreateApp(ctx, &pb.RpcAccountLocalLinkCreateAppRequest{
		App: &model.AccountAuthAppInfo{
			AppName: name,
			Scope:   model.AccountAuth_JsonAPI,
			Grant:   grant,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}
	if resp.Error != nil && resp.Error.Code != pb.RpcAccountLocalLinkCreateAppResponseError_NULL {
		return nil, fmt.Errorf("API error: %s", resp.Error.Description)
	}

	// Never trust the server to have applied what was asked: read the key back
	// and revoke it if its access differs from the request or can't be checked.
	stored, err := findAppByKey(ctx, client, resp.AppKey)
	if err != nil {
		return nil, removeUnverifiedKey(ctx, client, name, resp.AppKey, fmt.Errorf("could not verify the new API key: %v", err))
	}
	if stored.Scope != model.AccountAuth_JsonAPI || !grantsEqual(stored.Grant, grant) {
		return nil, removeUnverifiedKey(ctx, client, name, resp.AppKey, fmt.Errorf("the server did not store the requested access: %w", ErrServerTooOld))
	}

	return &CreatedAPIKey{Key: resp.AppKey, App: stored}, nil
}

// cleanupTimeout bounds the revocation of a key that failed verification. It
// starts fresh because the create call may have used up the caller's deadline.
const cleanupTimeout = 10 * time.Second

// removeUnverifiedKey revokes a key whose access could not be confirmed and
// returns the error to report. Errors from here on are formatted with %v, not
// wrapped: GRPCCall rewrites any error carrying an Unavailable status into
// "anytype is not running", which would hide that a key was left behind.
func removeUnverifiedKey(ctx context.Context, client service.ClientCommandsClient, name, key string, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()

	app, err := findAppByKey(ctx, client, key)
	if err != nil {
		return fmt.Errorf("%w; the key %q may still exist and could not be revoked (%v): check 'anytype auth apikey list' and revoke it", cause, name, err)
	}
	if err := revokeAPIKey(ctx, client, app.AppHash); err != nil {
		return fmt.Errorf("%w; revoking the key failed (%v): revoke it with 'anytype auth apikey revoke %s'", cause, err, app.AppHash)
	}
	return fmt.Errorf("%w; the key was revoked", cause)
}

// ensureGrantSupport probes for AccountLocalLinkUpdateApp, which arrived with
// grants. An empty app hash makes a capable server answer BAD_INPUT without
// changing anything; an older server does not know the method.
func ensureGrantSupport(ctx context.Context, client service.ClientCommandsClient) error {
	_, err := client.AccountLocalLinkUpdateApp(ctx, &pb.RpcAccountLocalLinkUpdateAppRequest{})
	if status.Code(err) == codes.Unimplemented {
		return ErrServerTooOld
	}
	if err != nil {
		return fmt.Errorf("failed to check server capabilities: %w", err)
	}
	return nil
}

func findAppByKey(ctx context.Context, client service.ClientCommandsClient, key string) (*model.AccountAuthAppInfo, error) {
	resp, err := client.AccountLocalLinkListApps(ctx, &pb.RpcAccountLocalLinkListAppsRequest{})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Code != pb.RpcAccountLocalLinkListAppsResponseError_NULL {
		return nil, fmt.Errorf("API error: %s", resp.Error.Description)
	}
	for _, app := range resp.App {
		if app.AppKey == key {
			return app, nil
		}
	}
	return nil, errors.New("the new key is not in the server's key list")
}

// grantsEqual compares grants by meaning: space order is irrelevant.
func grantsEqual(a, b *model.AccountAuthAppGrant) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.AllSpaces != b.AllSpaces || a.Perm != b.Perm {
		return false
	}
	as, bs := slices.Clone(a.SpaceIds), slices.Clone(b.SpaceIds)
	slices.Sort(as)
	slices.Sort(bs)
	return slices.Equal(as, bs)
}

// ListAPIKeys lists all API keys
func ListAPIKeys() (*pb.RpcAccountLocalLinkListAppsResponse, error) {
	var resp *pb.RpcAccountLocalLinkListAppsResponse

	err := GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		var err error
		resp, err = client.AccountLocalLinkListApps(ctx, &pb.RpcAccountLocalLinkListAppsRequest{})
		if err != nil {
			return fmt.Errorf("failed to list API keys: %w", err)
		}

		if resp.Error != nil && resp.Error.Code != pb.RpcAccountLocalLinkListAppsResponseError_NULL {
			return fmt.Errorf("API error: %s", resp.Error.Description)
		}

		return nil
	})

	return resp, err
}

// RevokeAPIKey revokes an API key by appId
func RevokeAPIKey(appId string) error {
	return GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		return revokeAPIKey(ctx, client, appId)
	})
}

func revokeAPIKey(ctx context.Context, client service.ClientCommandsClient, appId string) error {
	resp, err := client.AccountLocalLinkRevokeApp(ctx, &pb.RpcAccountLocalLinkRevokeAppRequest{
		AppHash: appId,
	})
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	if resp.Error != nil && resp.Error.Code != pb.RpcAccountLocalLinkRevokeAppResponseError_NULL {
		return fmt.Errorf("API error: %s", resp.Error.Description)
	}

	return nil
}
