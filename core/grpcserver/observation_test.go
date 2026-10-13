package grpcserver

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-cli/core/runtimeinfo"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"google.golang.org/grpc"
)

func TestObservationTransitionsAndEffectiveListener(t *testing.T) {
	file := filepath.Join(t.TempDir(), "runtime.json")
	recorder, err := runtimeinfo.New(file, "config", "data")
	if err != nil {
		t.Fatal(err)
	}
	interceptor := observationInterceptor(recorder)
	_, err = interceptor(context.Background(), &pb.RpcAccountCreateRequest{JsonApiListenAddr: "127.0.0.1:4500"}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountCreate"}, func(context.Context, any) (any, error) {
		return &pb.RpcAccountCreateResponse{Account: &model.Account{Id: "account", Info: &model.AccountInfo{ProfileObjectId: "profile", TechSpaceId: "tech", MetaDataKey: "secret"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := &observationStream{recorder: recorder}
	if err = stream.Send(&pb.Event{Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfAccountJsonApiStatus{AccountJsonApiStatus: &pb.EventAccountJsonApiStatus{Success: true, ListenAddr: "127.0.0.1:4500", Error: "must-not-persist"}}}}}); err != nil {
		t.Fatal(err)
	}
	var snapshot runtimeinfo.Snapshot
	if err = localstate.ReadJSON(file, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.AccountId != "account" || snapshot.AccountTransition || !snapshot.APIBound || snapshot.APIAddress != "127.0.0.1:4500" {
		t.Fatal(snapshot)
	}
	_, err = interceptor(context.Background(), &pb.RpcWalletRecoverRequest{}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/WalletRecover"}, func(context.Context, any) (any, error) { return &pb.RpcWalletRecoverResponse{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	snapshot = runtimeinfo.Snapshot{}
	if err = localstate.ReadJSON(file, &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.AccountTransition || snapshot.AccountId != "" || snapshot.APIBound {
		t.Fatal("wallet transition advertised stale identity", snapshot)
	}
}

func TestSuccessfulAPIDisableClearsBindingWithoutStatusEvent(t *testing.T) {
	for _, outcome := range []string{"success", "protobuf_error", "transport_error", "nonempty_request"} {
		t.Run(outcome, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "runtime.json")
			r, err := runtimeinfo.New(file, "config", "data")
			if err != nil {
				t.Fatal(err)
			}
			r.Account(&model.Account{Id: "account-a"})
			r.API("127.0.0.1:4500", true)
			request := &pb.RpcAccountChangeJsonApiAddrRequest{}
			if outcome == "nonempty_request" {
				request.ListenAddr = "127.0.0.1:4600"
			}
			_, _ = observationInterceptor(r)(context.Background(), request, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountChangeJsonApiAddr"}, func(context.Context, any) (any, error) {
				response := &pb.RpcAccountChangeJsonApiAddrResponse{}
				if outcome == "protobuf_error" {
					response.Error = &pb.RpcAccountChangeJsonApiAddrResponseError{Code: pb.RpcAccountChangeJsonApiAddrResponseErrorCode(101)}
				}
				if outcome == "transport_error" {
					return response, errors.New("failed")
				}
				return response, nil
			})
			var snapshot runtimeinfo.Snapshot
			if err = localstate.ReadJSON(file, &snapshot); err != nil {
				t.Fatal(err)
			}
			if outcome == "success" {
				if snapshot.APIBound || snapshot.APIAddress != "" || snapshot.AccountId != "account-a" {
					t.Fatal(snapshot)
				}
			} else if snapshot.APIBound || snapshot.APIAddress != "" || snapshot.APIObservation != "rpc_outcome_unknown" {
				t.Fatal("unproven rebind retained binding claim", snapshot)
			}
		})
	}
}
func TestPartialAccountCreateRemainsUncertain(t *testing.T) {
	file := filepath.Join(t.TempDir(), "runtime.json")
	recorder, err := runtimeinfo.New(file, "config", "data")
	if err != nil {
		t.Fatal(err)
	}
	_, err = observationInterceptor(recorder)(context.Background(), &pb.RpcAccountCreateRequest{}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountCreate"}, func(context.Context, any) (any, error) {
		return &pb.RpcAccountCreateResponse{Account: &model.Account{Id: "partial"}, Error: &pb.RpcAccountCreateResponseError{Code: pb.RpcAccountCreateResponseErrorCode(101)}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot runtimeinfo.Snapshot
	if err = localstate.ReadJSON(file, &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.AccountTransition || snapshot.AccountId != "" {
		t.Fatal(snapshot)
	}
}

func TestSameAccountWalletRecoverSelectRetainsObservedBinding(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "runtime.json")
	data := filepath.Join(dir, "data")
	recorder, err := runtimeinfo.New(file, "cfg", data)
	if err != nil {
		t.Fatal(err)
	}
	recorder.Account(&model.Account{Id: "account-a"})
	recorder.API("127.0.0.1:4500", true)
	interceptor := observationInterceptor(recorder)
	_, err = interceptor(context.Background(), &pb.RpcWalletRecoverRequest{RootPath: data}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/WalletRecover"}, func(context.Context, any) (any, error) { return &pb.RpcWalletRecoverResponse{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	// Heart's already-selected-account fast path emits no new API event.
	_, err = interceptor(context.Background(), &pb.RpcAccountSelectRequest{Id: "account-a", RootPath: data}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountSelect"}, func(context.Context, any) (any, error) {
		return &pb.RpcAccountSelectResponse{Account: &model.Account{Id: "account-a"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot runtimeinfo.Snapshot
	if err = localstate.ReadJSON(file, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.AccountId != "account-a" || !snapshot.APIBound || snapshot.APIAddress != "127.0.0.1:4500" || snapshot.AccountTransition {
		t.Fatal(snapshot)
	}
}
