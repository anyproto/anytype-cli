package core

import (
	"context"
	"errors"
	"testing"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAppLinkClient is an in-memory app link store. It embeds the client
// interface so only the methods under test need implementing.
type fakeAppLinkClient struct {
	service.ClientCommandsClient

	// oldServer makes UpdateApp unknown, like anytype-heart before v0.51.3.
	oldServer bool
	// dropGrant makes CreateApp ignore the grant, like an old server would.
	dropGrant bool

	apps        []*model.AccountAuthAppInfo
	createCalls []*pb.RpcAccountLocalLinkCreateAppRequest
	revoked     []string
}

func (f *fakeAppLinkClient) AccountLocalLinkUpdateApp(_ context.Context, in *pb.RpcAccountLocalLinkUpdateAppRequest, _ ...grpc.CallOption) (*pb.RpcAccountLocalLinkUpdateAppResponse, error) {
	if f.oldServer {
		return nil, status.Error(codes.Unimplemented, "unknown method AccountLocalLinkUpdateApp")
	}
	if in.AppHash == "" {
		return &pb.RpcAccountLocalLinkUpdateAppResponse{Error: &pb.RpcAccountLocalLinkUpdateAppResponseError{
			Code: pb.RpcAccountLocalLinkUpdateAppResponseError_BAD_INPUT, Description: "app hash is required",
		}}, nil
	}
	return &pb.RpcAccountLocalLinkUpdateAppResponse{Error: &pb.RpcAccountLocalLinkUpdateAppResponseError{}}, nil
}

func (f *fakeAppLinkClient) AccountLocalLinkCreateApp(_ context.Context, in *pb.RpcAccountLocalLinkCreateAppRequest, _ ...grpc.CallOption) (*pb.RpcAccountLocalLinkCreateAppResponse, error) {
	f.createCalls = append(f.createCalls, in)
	key := "key-" + in.App.AppName
	stored := &model.AccountAuthAppInfo{
		AppHash:  "hash-" + in.App.AppName,
		AppName:  in.App.AppName,
		AppKey:   key,
		Scope:    in.App.Scope,
		ExpireAt: in.App.ExpireAt,
		Grant:    in.App.Grant,
	}
	if f.dropGrant {
		stored.Grant = nil
	}
	f.apps = append(f.apps, stored)
	return &pb.RpcAccountLocalLinkCreateAppResponse{Error: &pb.RpcAccountLocalLinkCreateAppResponseError{}, AppKey: key}, nil
}

func (f *fakeAppLinkClient) AccountLocalLinkListApps(_ context.Context, _ *pb.RpcAccountLocalLinkListAppsRequest, _ ...grpc.CallOption) (*pb.RpcAccountLocalLinkListAppsResponse, error) {
	return &pb.RpcAccountLocalLinkListAppsResponse{Error: &pb.RpcAccountLocalLinkListAppsResponseError{}, App: f.apps}, nil
}

func (f *fakeAppLinkClient) AccountLocalLinkRevokeApp(_ context.Context, in *pb.RpcAccountLocalLinkRevokeAppRequest, _ ...grpc.CallOption) (*pb.RpcAccountLocalLinkRevokeAppResponse, error) {
	f.revoked = append(f.revoked, in.AppHash)
	return &pb.RpcAccountLocalLinkRevokeAppResponse{Error: &pb.RpcAccountLocalLinkRevokeAppResponseError{}}, nil
}

var testGrant = &model.AccountAuthAppGrant{SpaceIds: []string{"bafyreia.one"}, Perm: model.AccountAuthAppGrant_Read}

func TestCreateAPIKeySendsJsonAPIScopeAndGrant(t *testing.T) {
	client := &fakeAppLinkClient{}

	created, err := createAPIKey(context.Background(), client, "my-app", testGrant)

	if err != nil {
		t.Fatalf("createAPIKey() unexpected error: %v", err)
	}
	if len(client.createCalls) != 1 {
		t.Fatalf("CreateApp called %d times, want 1", len(client.createCalls))
	}
	sent := client.createCalls[0].App
	if sent.Scope != model.AccountAuth_JsonAPI {
		t.Errorf("scope = %v, want JsonAPI", sent.Scope)
	}
	if sent.Grant != testGrant {
		t.Errorf("grant = %+v, want %+v", sent.Grant, testGrant)
	}
	if created.Key != "key-my-app" || created.App.AppHash != "hash-my-app" {
		t.Errorf("created = %+v, want key and stored app", created)
	}
	if len(client.revoked) != 0 {
		t.Errorf("revoked %v, want nothing revoked", client.revoked)
	}
}

func TestCreateAPIKeyRejectsNilGrant(t *testing.T) {
	client := &fakeAppLinkClient{}

	_, err := createAPIKey(context.Background(), client, "my-app", nil)

	if err == nil {
		t.Fatal("createAPIKey() with nil grant succeeded, want an error")
	}
	if len(client.createCalls) != 0 {
		t.Error("CreateApp must not be called without a grant")
	}
}

func TestCreateAPIKeyRefusesOldServer(t *testing.T) {
	client := &fakeAppLinkClient{oldServer: true}

	_, err := createAPIKey(context.Background(), client, "my-app", testGrant)

	if !errors.Is(err, ErrServerTooOld) {
		t.Fatalf("createAPIKey() error = %v, want ErrServerTooOld", err)
	}
	if len(client.createCalls) != 0 {
		t.Error("CreateApp must not be called on a server that cannot store grants")
	}
}

func TestCreateAPIKeyRevokesKeyWhenServerDropsGrant(t *testing.T) {
	client := &fakeAppLinkClient{dropGrant: true}

	_, err := createAPIKey(context.Background(), client, "my-app", testGrant)

	if err == nil {
		t.Fatal("createAPIKey() succeeded although the server dropped the grant")
	}
	if len(client.revoked) != 1 || client.revoked[0] != "hash-my-app" {
		t.Errorf("revoked = %v, want the new key revoked", client.revoked)
	}
}

func TestGrantsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b *model.AccountAuthAppGrant
		want bool
	}{
		{"both nil", nil, nil, true},
		{"one nil", testGrant, nil, false},
		{"same spaces and perm", testGrant, &model.AccountAuthAppGrant{SpaceIds: []string{"bafyreia.one"}, Perm: model.AccountAuthAppGrant_Read}, true},
		{"space order does not matter", &model.AccountAuthAppGrant{SpaceIds: []string{"a", "b"}}, &model.AccountAuthAppGrant{SpaceIds: []string{"b", "a"}}, true},
		{"different perm", testGrant, &model.AccountAuthAppGrant{SpaceIds: []string{"bafyreia.one"}, Perm: model.AccountAuthAppGrant_ReadWrite}, false},
		{"different spaces", testGrant, &model.AccountAuthAppGrant{SpaceIds: []string{"bafyreib.two"}}, false},
		{"all spaces vs list", &model.AccountAuthAppGrant{AllSpaces: true}, &model.AccountAuthAppGrant{SpaceIds: []string{"a"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := grantsEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("grantsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
