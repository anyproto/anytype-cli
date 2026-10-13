package grpcserver

import (
	"context"
	"errors"
	"testing"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"google.golang.org/grpc"
)

func recoverFixtureWallet(t *testing.T, f *observationFixture) {
	t.Helper()
	_, err := f.interceptor(context.Background(), &pb.RpcWalletRecoverRequest{AccountKey: "fixture"}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/WalletRecover"}, func(context.Context, any) (any, error) {
		return &pb.RpcWalletRecoverResponse{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFailedAccountSwitchCannotRestorePreSwitchBinding(t *testing.T) {
	for _, requestedId := range []string{"account-b", "account-a"} {
		for _, outcome := range []string{"protobuf_failure", "transport_failure", "missing_account"} {
			t.Run(requestedId+"/"+outcome, func(t *testing.T) {
				f := newObservationFixture(t)
				f.r.Account(&model.Account{Id: "account-a"})
				f.rebind("127.0.0.1:4500", bindResponse("127.0.0.1:4500", true), nil)
				recoverFixtureWallet(t, f)
				_, _ = f.interceptor(context.Background(), &pb.RpcAccountSelectRequest{Id: requestedId, JsonApiListenAddr: "127.0.0.1:4500"}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountSelect"}, func(context.Context, any) (any, error) {
					// A different-account switch may already have closed A. Even a
					// same-account error does not prove which Heart path executed.
					if outcome == "transport_failure" {
						return nil, errors.New("selection outcome lost")
					}
					if outcome == "missing_account" {
						return &pb.RpcAccountSelectResponse{}, nil
					}
					return &pb.RpcAccountSelectResponse{Error: &pb.RpcAccountSelectResponseError{Code: pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE}}, nil
				})
				f.event("127.0.0.1:4500", true)
				if s := f.snapshot(); s.APIBound || !s.AccountTransition || s.APIObservation != "account_start_outcome_unknown" {
					t.Fatal("failed selection retained usable evidence", s)
				}
				recoverFixtureWallet(t, f)
				f.selectAccount("127.0.0.1:4500", func() { f.event("127.0.0.1:4500", false) })
				if s := f.snapshot(); s.APIBound || s.APIAddress != "" || s.AccountTransition || s.AccountId != "account-a" || s.APIObservation != "unverified_event_order" {
					t.Fatal("return to A revived pre-switch listener", s)
				}
			})
		}
	}
}

func TestFailedAccountStartClosesFreshEventWindow(t *testing.T) {
	for _, create := range []bool{false, true} {
		f := newObservationFixture(t)
		method := "AccountSelect"
		var req any = &pb.RpcAccountSelectRequest{Id: "account-a", JsonApiListenAddr: "127.0.0.1:4500"}
		if create {
			method = "AccountCreate"
			req = &pb.RpcAccountCreateRequest{Name: "Fixture", JsonApiListenAddr: "127.0.0.1:4500"}
		}
		_, _ = f.interceptor(context.Background(), req, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/" + method}, func(context.Context, any) (any, error) {
			return nil, errors.New("startup outcome unknown")
		})
		f.event("127.0.0.1:4500", true)
		if s := f.snapshot(); s.APIBound || s.APIAddress != "" || !s.AccountTransition || s.APIObservation != "account_start_outcome_unknown" {
			t.Fatal("late startup event survived failed response", s)
		}
	}
}

func TestAccountCreateNeverRetainsPriorAccountBinding(t *testing.T) {
	for _, successfulCreate := range []bool{false, true} {
		f := newObservationFixture(t)
		f.r.Account(&model.Account{Id: "account-a"})
		f.rebind("127.0.0.1:4500", bindResponse("127.0.0.1:4500", true), nil)
		recoverFixtureWallet(t, f)
		_, _ = f.interceptor(context.Background(), &pb.RpcAccountCreateRequest{Name: "Fixture", JsonApiListenAddr: "127.0.0.1:4500"}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountCreate"}, func(context.Context, any) (any, error) {
			f.event("127.0.0.1:4500", false)
			if !successfulCreate {
				return nil, errors.New("create failed after closing previous app")
			}
			// Reusing the wallet may return the same ID, but creation never has
			// AccountSelect's already-running-account fast path.
			return &pb.RpcAccountCreateResponse{Account: &model.Account{Id: "account-a"}}, nil
		})
		if s := f.snapshot(); s.APIBound || s.APIAddress != "" {
			t.Fatal("creation inherited prior binding", s)
		}
		f.selectAccount("127.0.0.1:4500", nil)
		if s := f.snapshot(); s.APIBound || s.APIAddress != "" {
			t.Fatal("later selection inherited pre-create binding", s)
		}
	}
}

func TestNativeMoveAndNetworkRestartInvalidateBinding(t *testing.T) {
	for _, method := range []string{"AccountMove", "AccountChangeNetworkConfigAndRestart"} {
		for _, outcome := range []string{"success", "protobuf_error", "transport_error"} {
			t.Run(method+"/"+outcome, func(t *testing.T) {
				f := newObservationFixture(t)
				f.r.Account(&model.Account{Id: "account-a"})
				f.rebind("127.0.0.1:4500", bindResponse("127.0.0.1:4500", true), nil)
				var request any = &pb.RpcAccountMoveRequest{NewPath: "/fixture/new-data"}
				var response any = &pb.RpcAccountMoveResponse{}
				if method == "AccountChangeNetworkConfigAndRestart" {
					request = &pb.RpcAccountChangeNetworkConfigAndRestartRequest{}
					response = &pb.RpcAccountChangeNetworkConfigAndRestartResponse{}
				}
				_, _ = f.interceptor(context.Background(), request, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/" + method}, func(context.Context, any) (any, error) {
					if s := f.snapshot(); s.APIBound || !s.AccountTransition {
						t.Fatal("pre-close binding survived handler entry", s)
					}
					// Heart may close the old app before an error. A successful
					// network restart does not imply a successful HTTP bind.
					f.event("127.0.0.1:4500", false)
					if outcome == "transport_error" {
						return nil, errors.New("native result lost")
					}
					if outcome == "protobuf_error" {
						if method == "AccountMove" {
							return &pb.RpcAccountMoveResponse{Error: &pb.RpcAccountMoveResponseError{Code: pb.RpcAccountMoveResponseError_UNKNOWN_ERROR}}, nil
						}
						return &pb.RpcAccountChangeNetworkConfigAndRestartResponse{Error: &pb.RpcAccountChangeNetworkConfigAndRestartResponseError{Code: pb.RpcAccountChangeNetworkConfigAndRestartResponseError_UNKNOWN_ERROR}}, nil
					}
					return response, nil
				})
				f.event("127.0.0.1:4500", true)
				s := f.snapshot()
				if s.APIBound || s.APIAddress != "" {
					t.Fatal("closed binding revived by delayed event", s)
				}
				if method == "AccountMove" && outcome == "success" {
					if s.AccountId != "" || s.AccountTransition || s.APIObservation != "stopped" {
						t.Fatal("successful move did not observe stopped app", s)
					}
				} else if !s.AccountTransition {
					t.Fatal("unverified account lifecycle presented as ready", s)
				}
				f.selectAccount("127.0.0.1:4500", func() { f.event("127.0.0.1:4500", false) })
				if s = f.snapshot(); s.APIBound || s.AccountTransition {
					t.Fatal("selection revived pre-close evidence", s)
				}
				f.rebind("127.0.0.1:4500", bindResponse("127.0.0.1:4500", true), nil)
				if s = f.snapshot(); !s.APIBound || s.AccountTransition {
					t.Fatal("explicit selection and rebind did not recover", s)
				}
			})
		}
	}
}

func TestNativeMoveAndRestartParticipateInOverlapTracking(t *testing.T) {
	for _, method := range []string{"AccountMove", "AccountChangeNetworkConfigAndRestart"} {
		t.Run(method, func(t *testing.T) {
			f := newObservationFixture(t)
			f.r.Account(&model.Account{Id: "account-a"})
			entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			go func() {
				defer close(finished)
				_, _ = f.interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/" + method}, func(context.Context, any) (any, error) {
					close(entered)
					<-release
					return nil, errors.New("native result lost")
				})
			}()
			<-entered
			f.rebind("127.0.0.1:4500", bindResponse("127.0.0.1:4500", true), nil)
			close(release)
			<-finished
			f.event("127.0.0.1:4500", true)
			if s := f.snapshot(); s.APIBound || !s.AccountTransition || s.APIObservation != "concurrent_lifecycle" {
				t.Fatal("overlapping response invented lifecycle order", s)
			}
		})
	}
}
